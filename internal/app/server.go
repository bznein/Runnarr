package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
const stravaStateCookieName = "runnarr_strava_state"

type Server struct {
	cfg       Config
	store     *Store
	imports   *ImportService
	strava    *StravaService
	intervals *IntervalsService
	logger    *slog.Logger
	adminHash []byte
}

func NewServer(cfg Config, db *pgxpool.Pool, logger *slog.Logger) (*Server, error) {
	store := NewStore(db)
	cipher, err := NewTokenCipher(cfg.EncryptionKey())
	if err != nil {
		return nil, err
	}
	adminHash := []byte(cfg.AdminPasswordHash)
	if len(adminHash) == 0 {
		adminHash, err = bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
	}
	return &Server{
		cfg:       cfg,
		store:     store,
		imports:   NewImportService(store),
		strava:    NewStravaService(cfg, store, cipher),
		intervals: NewIntervalsService(store, cipher),
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
		r.Get("/providers/strava/webhook", s.handleStravaWebhookValidation)
		r.Post("/providers/strava/webhook", s.handleStravaWebhookEvent)

		r.Group(func(r chi.Router) {
			r.Use(s.requireSession)
			r.Get("/config", s.handleConfig)
			r.Post("/session/logout", s.handleLogout)
			r.Get("/activities", s.handleListActivities)
			r.Get("/activities/{id}", s.handleGetActivity)
			r.Delete("/activities/{id}", s.handleDeleteActivity)
			r.Get("/activity-types", s.handleActivityTypes)
			r.Get("/stats/summary", s.handleSummary)
			r.Get("/imports", s.handleListImports)
			r.Post("/imports", s.handleImport)
			r.Get("/providers/strava/status", s.handleStravaStatus)
			r.Get("/providers/strava/connect", s.handleStravaConnect)
			r.Get("/providers/strava/callback", s.handleStravaCallback)
			r.Post("/providers/strava/sync", s.handleStravaSync)
			r.Get("/providers/intervals/status", s.handleIntervalsStatus)
			r.Post("/providers/intervals/connect", s.handleIntervalsConnect)
			r.Post("/providers/intervals/sync", s.handleIntervalsSync)
			r.Get("/sync-jobs", s.handleSyncJobs)
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
		"mapTileURL":       s.cfg.MapTileURL,
		"stravaConfigured": s.cfg.StravaConfigured(),
		"baseURL":          s.cfg.BaseURL,
	})
}

func (s *Server) handleListActivities(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	activities, err := s.store.ListActivities(r.Context(), limit, offset, activityFiltersFromQuery(r))
	if err != nil {
		s.logger.Error("list activities", "error", err)
		writeError(w, http.StatusInternalServerError, "could not list activities")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"activities": activities})
}

func (s *Server) handleActivityTypes(w http.ResponseWriter, r *http.Request) {
	sports, err := s.store.ListSportTypes(r.Context())
	if err != nil {
		s.logger.Error("list activity types", "error", err)
		writeError(w, http.StatusInternalServerError, "could not list activity types")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"activityTypes": sports})
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

func (s *Server) handleDeleteActivity(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.DeleteActivity(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "activity not found")
		return
	}
	if err != nil {
		s.logger.Error("delete activity", "error", err)
		writeError(w, http.StatusInternalServerError, "could not delete activity")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Summary(r.Context(), activityFiltersFromQuery(r))
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

func (s *Server) handleStravaStatus(w http.ResponseWriter, r *http.Request) {
	conn, connected, err := s.strava.Status(r.Context())
	if err != nil {
		s.logger.Error("strava status", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load Strava status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": s.cfg.StravaConfigured(),
		"connected":  connected,
		"connection": conn,
	})
}

func (s *Server) handleStravaConnect(w http.ResponseWriter, r *http.Request) {
	state, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create OAuth state")
		return
	}
	authURL, err := s.strava.AuthorizationURL(state)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stravaStateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   10 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.HasPrefix(s.cfg.BaseURL, "https://"),
	})
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handleStravaCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(stravaStateCookieName)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		writeError(w, http.StatusBadRequest, "invalid OAuth state")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: stravaStateCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
		http.Redirect(w, r, "/settings?strava=denied", http.StatusFound)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing OAuth code")
		return
	}
	if _, err := s.strava.ExchangeCode(r.Context(), code); err != nil {
		s.logger.Error("strava callback", "error", err)
		http.Redirect(w, r, "/settings?strava=error", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/settings?strava=connected", http.StatusFound)
}

func (s *Server) handleStravaSync(w http.ResponseWriter, r *http.Request) {
	jobID, err := s.store.CreateSyncJob(r.Context(), stravaProvider, "manual")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create sync job")
		return
	}
	payload, err := s.strava.Sync(r.Context())
	if err != nil {
		_ = s.store.FinishSyncJob(r.Context(), jobID, "failed", err.Error(), map[string]any{})
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.FinishSyncJob(r.Context(), jobID, "completed", "", payload); err != nil {
		writeError(w, http.StatusInternalServerError, "could not finish sync job")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobId": jobID, "result": payload})
}

func (s *Server) handleIntervalsStatus(w http.ResponseWriter, r *http.Request) {
	conn, connected, err := s.intervals.Status(r.Context())
	if err != nil {
		s.logger.Error("intervals status", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load Intervals.icu status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"connected":  connected,
		"connection": conn,
	})
}

func (s *Server) handleIntervalsConnect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKey string `json:"apiKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	conn, err := s.intervals.Connect(r.Context(), body.APIKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "connection": conn})
}

func (s *Server) handleIntervalsSync(w http.ResponseWriter, r *http.Request) {
	opts, err := decodeIntervalsSyncOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	running, err := s.store.HasRunningSyncJob(r.Context(), intervalsProvider)
	if err != nil {
		s.logger.Error("check running intervals sync", "error", err)
		writeError(w, http.StatusInternalServerError, "could not check sync state")
		return
	}
	if running {
		writeError(w, http.StatusConflict, "Intervals.icu sync is already running")
		return
	}
	jobID, err := s.store.CreateSyncJob(r.Context(), intervalsProvider, "manual")
	if err != nil {
		s.logger.Error("create intervals sync job", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create sync job")
		return
	}
	go s.runIntervalsManualSyncJob(jobID, opts)
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": jobID, "status": "running"})
}

func (s *Server) handleSyncJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ListSyncJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list sync jobs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *Server) handleStravaWebhookValidation(w http.ResponseWriter, r *http.Request) {
	if s.cfg.StravaWebhookVerifyToken != "" && r.URL.Query().Get("hub.verify_token") != s.cfg.StravaWebhookVerifyToken {
		writeError(w, http.StatusForbidden, "invalid verify token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"hub.challenge": r.URL.Query().Get("hub.challenge")})
}

func (s *Server) handleStravaWebhookEvent(w http.ResponseWriter, r *http.Request) {
	var event map[string]any
	_ = json.NewDecoder(r.Body).Decode(&event)
	if jobID, err := s.store.CreateSyncJob(r.Context(), stravaProvider, "webhook"); err == nil {
		_ = s.store.FinishSyncJob(r.Context(), jobID, "queued", "", event)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) StartBackgroundSync(ctx context.Context) {
	go s.runIntervalsScheduledSync(ctx)
}

func (s *Server) runIntervalsScheduledSync(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runIntervalsScheduledSyncOnce(ctx)
		}
	}
}

func (s *Server) runIntervalsScheduledSyncOnce(ctx context.Context) {
	if _, connected, err := s.intervals.Status(ctx); err != nil || !connected {
		return
	}
	if running, err := s.store.HasRunningSyncJob(ctx, intervalsProvider); err != nil || running {
		if err != nil {
			s.logger.Error("check running scheduled intervals sync", "error", err)
		}
		return
	}
	syncCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	oldest := time.Now().UTC().AddDate(0, 0, -30)
	if _, _, err := s.runIntervalsSync(syncCtx, "scheduled", IntervalsSyncOptions{Oldest: oldest}); err != nil {
		s.logger.Error("scheduled intervals sync", "error", err)
	}
}

func (s *Server) runIntervalsManualSyncJob(jobID string, opts IntervalsSyncOptions) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	if _, err := s.finishIntervalsSyncJob(ctx, jobID, opts); err != nil {
		s.logger.Error("manual intervals sync", "job_id", jobID, "error", err)
	}
}

func (s *Server) runIntervalsSync(ctx context.Context, kind string, opts IntervalsSyncOptions) (string, map[string]any, error) {
	jobID, err := s.store.CreateSyncJob(ctx, intervalsProvider, kind)
	if err != nil {
		return "", nil, fmt.Errorf("could not create sync job: %w", err)
	}
	payload, err := s.finishIntervalsSyncJob(ctx, jobID, opts)
	return jobID, payload, err
}

func (s *Server) finishIntervalsSyncJob(ctx context.Context, jobID string, opts IntervalsSyncOptions) (map[string]any, error) {
	progress := func(payload map[string]any) {
		if err := s.store.UpdateSyncJobProgress(ctx, jobID, payload); err != nil {
			s.logger.Error("update intervals sync progress", "job_id", jobID, "error", err)
		}
	}
	payload, err := s.intervals.Sync(ctx, opts, progress)
	if err != nil {
		_ = s.store.FinishSyncJob(ctx, jobID, "failed", err.Error(), nil)
		return nil, err
	}
	if err := s.store.FinishSyncJob(ctx, jobID, "completed", "", payload); err != nil {
		return nil, fmt.Errorf("could not finish sync job: %w", err)
	}
	return payload, nil
}

func decodeIntervalsSyncOptions(r *http.Request) (IntervalsSyncOptions, error) {
	var body struct {
		Oldest string `json:"oldest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return IntervalsSyncOptions{}, errors.New("invalid JSON body")
	}
	if strings.TrimSpace(body.Oldest) == "" {
		return IntervalsSyncOptions{}, nil
	}
	oldest, err := time.Parse("2006-01-02", strings.TrimSpace(body.Oldest))
	if err != nil {
		return IntervalsSyncOptions{}, errors.New("oldest must use YYYY-MM-DD format")
	}
	return IntervalsSyncOptions{Oldest: oldest}, nil
}

func activityFiltersFromQuery(r *http.Request) ActivityFilters {
	values := r.URL.Query()
	return ActivityFilters{
		SportTypes:         compactQueryValues(values["sport"], values["sports"]),
		ExcludedSportTypes: compactQueryValues(values["excludeSport"], values["excludeSports"]),
		Search:             strings.TrimSpace(values.Get("search")),
		DateFrom:           parseActivityFilterDate(values.Get("dateFrom")),
		DateTo:             parseActivityFilterDate(values.Get("dateTo")),
		SortBy:             strings.TrimSpace(values.Get("sortBy")),
		SortOrder:          strings.TrimSpace(values.Get("sortOrder")),
	}
}

func parseActivityFilterDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}
	}
	return date
}

func compactQueryValues(groups ...[]string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0)
	for _, group := range groups {
		for _, raw := range group {
			for _, part := range strings.Split(raw, ",") {
				value := strings.TrimSpace(part)
				if value == "" || seen[value] {
					continue
				}
				seen[value] = true
				out = append(out, value)
			}
		}
	}
	return out
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
