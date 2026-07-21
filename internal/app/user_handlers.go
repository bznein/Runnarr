package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) handleStartSupport(w http.ResponseWriter, r *http.Request) {
	principal, err := principalFromContext(r.Context())
	if err != nil || principal.ActorID == "" || principal.Role != adminRole {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	var body struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.UserID) == "" {
		writeError(w, http.StatusBadRequest, "userId is required")
		return
	}
	if body.UserID == principal.ActorID {
		writeError(w, http.StatusBadRequest, "cannot enter support mode for yourself")
		return
	}
	target, err := s.store.GetUser(r.Context(), body.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load user")
		return
	}
	if target.Disabled {
		writeError(w, http.StatusBadRequest, "cannot enter support mode for a disabled user")
		return
	}
	cookie, err := r.Cookie(s.authCookieName())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "login session is missing")
		return
	}
	if err := s.store.SetSessionSupport(r.Context(), cookie.Value, body.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "cannot enter support mode for that user")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not enter support mode")
		return
	}
	record, err := s.store.GetSessionRecord(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load support session")
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse(record))
}

func (s *Server) handleStopSupport(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(s.authCookieName())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "login session is missing")
		return
	}
	if err := s.store.ClearSessionSupport(r.Context(), cookie.Value); err != nil {
		writeError(w, http.StatusInternalServerError, "could not leave support mode")
		return
	}
	record, err := s.store.GetSessionRecord(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load session")
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse(record))
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	principal, err := principalFromContext(r.Context())
	if err != nil || principal.ActorID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	hash, err := s.store.PasswordHash(r.Context(), principal.ActorID)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.CurrentPassword)) != nil {
		writeError(w, http.StatusBadRequest, "current password is invalid")
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	if err := s.store.SetUserPassword(r.Context(), principal.ActorID, string(newHash)); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update password")
		return
	}
	if err := s.store.DisableUserSessions(r.Context(), principal.ActorID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke existing sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (UserPrincipal, bool) {
	principal, err := principalFromContext(r.Context())
	if err != nil || principal.ActorID == "" || principal.Role != adminRole {
		writeError(w, http.StatusForbidden, "administrator access required")
		return UserPrincipal{}, false
	}
	return principal, true
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var body struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if normalizeUsername(body.Username) == "" || len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "username and a password of at least 8 characters are required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	user, err := s.store.CreateUser(r.Context(), body.Username, body.DisplayName, body.Role, string(hash))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusConflict, "username is already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	current, err := s.store.GetUser(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load user")
		return
	}
	var body struct {
		DisplayName *string `json:"displayName"`
		Role        *string `json:"role"`
		Disabled    *bool   `json:"disabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	displayName := current.DisplayName
	if body.DisplayName != nil {
		displayName = strings.TrimSpace(*body.DisplayName)
	}
	role := current.Role
	if body.Role != nil {
		role = strings.TrimSpace(*body.Role)
	}
	disabled := current.Disabled
	if body.Disabled != nil {
		disabled = *body.Disabled
	}
	if id == principal.ActorID && disabled {
		writeError(w, http.StatusBadRequest, "you cannot disable your own account")
		return
	}
	if current.Role == adminRole && (!strings.EqualFold(role, adminRole) || disabled) {
		count, err := s.store.CountActiveAdmins(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not check administrator count")
			return
		}
		if count <= 1 {
			writeError(w, http.StatusBadRequest, "at least one active administrator is required")
			return
		}
	}
	user, err := s.store.UpdateUser(r.Context(), id, displayName, role, &disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update user")
		return
	}
	if disabled {
		_ = s.store.DisableUserSessions(r.Context(), id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	if err := s.store.SetUserPassword(r.Context(), chi.URLParam(r, "id"), string(hash)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not reset password")
		return
	}
	_ = s.store.DisableUserSessions(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	preferences, err := s.store.GetUserPreferences(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load preferences")
		return
	}
	writeJSON(w, http.StatusOK, preferences)
}

func (s *Server) handleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	var preferences UserPreference
	if err := json.NewDecoder(r.Body).Decode(&preferences); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.store.UpdateUserPreferences(r.Context(), preferences); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save preferences")
		return
	}
	saved, err := s.store.GetUserPreferences(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load saved preferences")
		return
	}
	writeJSON(w, http.StatusOK, saved)
}
