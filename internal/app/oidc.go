package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
)

type oidcClient struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    oauth2.Config
}

type oidcClaims struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Nonce         string `json:"nonce"`
}

func (s *Server) oidcClient(ctx context.Context) (*oidcClient, error) {
	if s.cfg.OIDCClientID == "" || s.cfg.OIDCClientSecret == "" {
		return nil, errors.New("Google OIDC is not configured")
	}
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	if s.oidc != nil {
		return s.oidc, nil
	}
	providerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(providerCtx, s.cfg.OIDCIssuerURL)
	if err != nil {
		return nil, err
	}
	client := &oidcClient{
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: s.cfg.OIDCClientID}),
		oauth: oauth2.Config{
			ClientID:     s.cfg.OIDCClientID,
			ClientSecret: s.cfg.OIDCClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  s.cfg.OIDCRedirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
	}
	s.oidc = client
	return client, nil
}

func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PublicMode == false && s.cfg.OIDCClientID == "" {
		writeError(w, http.StatusNotFound, "Google OIDC is not configured")
		return
	}
	if s.loginLimiter != nil && !s.loginLimiter.allow("oidc:"+requestClientKey(r, s.cfg.TrustProxy), time.Now()) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many login attempts")
		return
	}
	client, err := s.oidcClient(r.Context())
	if err != nil {
		s.logger.Error("initialize Google OIDC", "error", err)
		writeError(w, http.StatusServiceUnavailable, "Google login is temporarily unavailable")
		return
	}
	state, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create login state")
		return
	}
	nonce, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create login nonce")
		return
	}
	if err := s.store.CreateOIDCAuthState(r.Context(), state, nonce, 10*time.Minute); err != nil {
		s.logger.Error("create OIDC state", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create login state")
		return
	}
	http.SetCookie(w, s.oidcStateCookie(state, 10*time.Minute))
	http.SetCookie(w, s.oidcNonceCookie(nonce, 10*time.Minute))
	authURL := client.oauth.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce))
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, s.cfg.BaseURL+"/login?error=google_denied", http.StatusFound)
		return
	}
	state, code := r.URL.Query().Get("state"), r.URL.Query().Get("code")
	stateCookie, err := r.Cookie(oidcStateCookieName)
	nonceCookie, nonceCookieErr := r.Cookie(oidcNonceCookieName)
	if err != nil || nonceCookieErr != nil || state == "" || code == "" || subtle.ConstantTimeCompare([]byte(state), []byte(stateCookie.Value)) != 1 {
		writeError(w, http.StatusBadRequest, "invalid Google login state")
		return
	}
	http.SetCookie(w, s.oidcStateCookie("", -time.Hour))
	http.SetCookie(w, s.oidcNonceCookie("", -time.Hour))
	nonceHash, err := s.store.ConsumeOIDCAuthState(r.Context(), state)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid Google login state")
		return
	}
	if subtle.ConstantTimeCompare([]byte(nonceHash), []byte(hashOIDCValue(nonceCookie.Value))) != 1 {
		writeError(w, http.StatusBadRequest, "invalid Google login state")
		return
	}
	client, err := s.oidcClient(r.Context())
	if err != nil {
		s.logger.Error("initialize Google OIDC", "error", err)
		writeError(w, http.StatusServiceUnavailable, "Google login is temporarily unavailable")
		return
	}
	exchangeCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	token, err := client.oauth.Exchange(exchangeCtx, code)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Google login failed")
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		writeError(w, http.StatusUnauthorized, "Google login did not return an identity token")
		return
	}
	idToken, err := client.verifier.Verify(exchangeCtx, rawIDToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Google identity token is invalid")
		return
	}
	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil || claims.Subject == "" || claims.Nonce == "" || subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonceCookie.Value)) != 1 {
		writeError(w, http.StatusUnauthorized, "Google identity token is invalid")
		return
	}
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	if email == "" || !claims.EmailVerified {
		writeError(w, http.StatusForbidden, "Google account email is not verified")
		return
	}
	allowedUsername, allowed := s.cfg.OIDCAllowedEmails[email]
	if !allowed {
		writeError(w, http.StatusForbidden, "Google account is not allowed")
		return
	}
	userID, err := s.store.OIDCIdentity(r.Context(), s.cfg.OIDCIssuerURL, claims.Subject)
	if errors.Is(err, pgx.ErrNoRows) {
		user, userErr := s.store.GetUserByUsername(r.Context(), allowedUsername)
		if userErr != nil || user.Disabled {
			writeError(w, http.StatusForbidden, "Google account is not allowed")
			return
		}
		userID = user.ID
		if err := s.store.LinkOIDCIdentity(r.Context(), userID, s.cfg.OIDCIssuerURL, claims.Subject, email); err != nil {
			if errors.Is(err, ErrOIDCIdentityConflict) {
				writeError(w, http.StatusForbidden, "Google account is not allowed")
				return
			}
			writeError(w, http.StatusInternalServerError, "could not link Google identity")
			return
		}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load Google identity")
		return
	}
	user, err := s.store.GetUser(r.Context(), userID)
	if err != nil || user.Disabled || normalizeUsername(user.Username) != allowedUsername {
		writeError(w, http.StatusForbidden, "Google account is not allowed")
		return
	}
	csrf, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	sessionID, err := s.store.CreateSession(r.Context(), user.ID, csrf, sessionAbsoluteTTL)
	if err != nil {
		s.logger.Error("create OIDC session", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	http.SetCookie(w, s.sessionCookie(sessionID, sessionAbsoluteTTL))
	_ = s.store.TouchUserLogin(r.Context(), user.ID)
	http.Redirect(w, r, s.cfg.BaseURL+"/", http.StatusFound)
}

func (s *Server) oidcStateCookie(value string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    value,
		Path:     "/api/auth",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.PublicMode || strings.HasPrefix(s.cfg.BaseURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	}
}

func (s *Server) oidcNonceCookie(value string, ttl time.Duration) *http.Cookie {
	cookie := s.oidcStateCookie(value, ttl)
	cookie.Name = oidcNonceCookieName
	return cookie
}
