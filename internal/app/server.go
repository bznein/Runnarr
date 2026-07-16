package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "runnarr_session"

type Server struct {
	cfg       Config
	store     *Store
	imports   *ImportService
	logger    *slog.Logger
	adminHash []byte
}

func NewServer(cfg Config, db *pgxpool.Pool, logger *slog.Logger) (*Server, error) {
	store := NewStore(db)
	adminHash := []byte(cfg.AdminPasswordHash)
	if len(adminHash) == 0 {
		var err error
		adminHash, err = bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
	}
	return &Server{
		cfg:       cfg,
		store:     store,
		imports:   NewImportService(store),
		logger:    logger,
		adminHash: adminHash,
	}, nil
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(s.recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/session/login", s.handleLogin)
		r.Get("/session", s.handleSession)

		r.Group(func(r chi.Router) {
			r.Use(s.requireSession)
			r.Get("/config", s.handleConfig)
			r.Post("/session/logout", s.handleLogout)
			r.Get("/activities", s.handleListActivities)
			r.Get("/activities/{id}", s.handleGetActivity)
			r.Get("/stats/summary", s.handleSummary)
			r.Get("/imports", s.handleListImports)
			r.Post("/imports", s.handleImport)
		})
	})

	r.NotFound(s.serveSPA)
	return r
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := bcrypt.CompareHashAndPassword(s.adminHash, []byte(body.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	csrf, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	sessionID, err := s.store.CreateSession(r.Context(), csrf, 30*24*time.Hour)
	if err != nil {
		s.logger.Error("create session", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	http.SetCookie(w, s.sessionCookie(sessionID, 30*24*time.Hour))
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"csrfToken":     csrf,
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	csrf, err := s.store.GetSession(r.Context(), cookie.Value)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"csrfToken":     csrf,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.store.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, s.sessionCookie("", -1*time.Hour))
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"mapTileURL": s.cfg.MapTileURL,
		"baseURL":    s.cfg.BaseURL,
	})
}

func (s *Server) handleListActivities(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	activities, err := s.store.ListActivities(r.Context(), limit, offset, r.URL.Query().Get("sport"))
	if err != nil {
		s.logger.Error("list activities", "error", err)
		writeError(w, http.StatusInternalServerError, "could not list activities")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"activities": activities})
}

func (s *Server) handleGetActivity(w http.ResponseWriter, r *http.Request) {
	activity, err := s.store.GetActivity(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "activity not found")
		return
	}
	if err != nil {
		s.logger.Error("get activity", "error", err)
		writeError(w, http.StatusInternalServerError, "could not get activity")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"activity": activity})
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Summary(r.Context())
	if err != nil {
		s.logger.Error("summary", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load summary")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleListImports(w http.ResponseWriter, r *http.Request) {
	imports, err := s.store.ListImports(r.Context())
	if err != nil {
		s.logger.Error("list imports", "error", err)
		writeError(w, http.StatusInternalServerError, "could not list imports")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imports": imports})
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(90 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	activity, importFile, err := s.imports.ImportFile(r.Context(), header.Filename, header.Header.Get("Content-Type"), file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"activity": activity,
		"import":   importFile,
	})
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		csrf, err := s.store.GetSession(r.Context(), cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if isMutating(r.Method) && r.Header.Get("X-CSRF-Token") != csrf {
			writeError(w, http.StatusForbidden, "invalid CSRF token")
			return
		}
		ctx := context.WithValue(r.Context(), csrfContextKey{}, csrf)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("panic", "value", recovered)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	requested := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if requested == "." {
		requested = "index.html"
	}
	path := filepath.Join(s.cfg.StaticDir, requested)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	indexPath := filepath.Join(s.cfg.StaticDir, "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		http.ServeFile(w, r, indexPath)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Runnarr API is running. Build the frontend with `npm run build` in web/ for the full UI.\n"))
}

func (s *Server) sessionCookie(value string, ttl time.Duration) *http.Cookie {
	maxAge := int(ttl.Seconds())
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.HasPrefix(s.cfg.BaseURL, "https://"),
	}
}

type csrfContextKey struct{}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
