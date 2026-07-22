package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName       = "runnarr_session"
	publicSessionCookieName = "__Host-runnarr_session"
	oidcStateCookieName     = "runnarr_oidc_state"
	oidcNonceCookieName     = "runnarr_oidc_nonce"
	sessionAbsoluteTTL      = 30 * 24 * time.Hour
)

type Server struct {
	cfg              Config
	store            *Store
	imports          *ImportService
	garmin           *GarminService
	media            *MediaService
	logger           *slog.Logger
	syncCancelsMu    sync.Mutex
	syncCancels      map[string]context.CancelFunc
	writebackRetryMu sync.Mutex
	writebackRetries map[string]struct{}
	oidcMu           sync.Mutex
	oidc             *oidcClient
	loginLimiter     *loginRateLimiter
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
	if err := store.EnsureBootstrap(context.Background(), cfg.AdminUsername, string(adminHash)); err != nil {
		return nil, fmt.Errorf("bootstrap users: %w", err)
	}
	adminUser, err := store.GetUserByUsername(context.Background(), cfg.AdminUsername)
	if err != nil {
		return nil, fmt.Errorf("load bootstrap admin: %w", err)
	}
	garmin := NewGarminService(cfg, store)
	garmin.legacyUserID = adminUser.ID
	return &Server{
		cfg:              cfg,
		store:            store,
		imports:          NewImportService(store),
		garmin:           garmin,
		media:            NewMediaService(cfg, store),
		logger:           logger,
		syncCancels:      make(map[string]context.CancelFunc),
		writebackRetries: make(map[string]struct{}),
		loginLimiter:     newLoginRateLimiter(),
	}, nil
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(s.recoverer)
	r.Use(s.securityHeaders)
	r.Use(s.limitRequestBody)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/session/login", s.handleLogin)
		r.Get("/session", s.handleSession)
		r.Get("/auth/google/login", s.handleOIDCLogin)
		r.Get("/auth/google/callback", s.handleOIDCCallback)
		r.Get("/providers/google/callback", s.handleGoogleCallback)

		r.Group(func(r chi.Router) {
			r.Use(s.requireSession)
			r.Post("/session/support", s.handleStartSupport)
			r.Delete("/session/support", s.handleStopSupport)
			r.Post("/session/password", s.handleChangePassword)
			r.Get("/users", s.handleListUsers)
			r.Post("/users", s.handleCreateUser)
			r.Patch("/users/{id}", s.handleUpdateUser)
			r.Post("/users/{id}/password", s.handleResetUserPassword)
			r.Get("/preferences", s.handleGetPreferences)
			r.Patch("/preferences", s.handleUpdatePreferences)
			r.Get("/config", s.handleConfig)
			r.Get("/config/training-sheet", s.handleTrainingSheetConfig)
			r.Patch("/config/training-sheet", s.handleUpdateTrainingSheetConfig)
			r.Patch("/config/climb-detection", s.handleUpdateClimbDetection)
			r.Post("/tools/pace", s.handleToolsPace)
			r.Post("/tools/vdot", s.handleToolsVDOT)
			r.Post("/session/logout", s.handleLogout)
			r.Get("/activities", s.handleListActivities)
			r.Get("/activities/{id}/gpx", s.handleExportActivityGPX)
			r.Post("/activities/{id}/climbs-preview", s.handleActivityClimbsPreview)
			r.Get("/activities/{id}", s.handleGetActivity)
			r.Get("/activities/{id}/planned-match-candidates", s.handlePlannedMatchCandidates)
			r.Post("/activities/{id}/planned-match-preview", s.handlePlannedMatchPreview)
			r.Post("/activities/{id}/planned-match-apply", s.handleApplyPlannedMatchPreview)
			r.Post("/activities/{id}/planned-match", s.handleMatchPlannedActivity)
			r.Post("/activities/{id}/planned-writeback", s.handleTrainingSheetWriteback)
			r.Delete("/activities/{id}/planned-match", s.handleUnmatchPlannedActivity)
			r.Patch("/activities/{id}", s.handleRenameActivity)
			r.Delete("/activities/{id}", s.handleDeleteActivity)
			r.Post("/activities/{id}/media", s.handleUploadActivityMedia)
			r.Delete("/activities/{id}/media/{mediaId}", s.handleDeleteActivityMedia)
			r.Get("/activity-media/{mediaId}/original", s.handleServeOriginalMedia)
			r.Get("/activity-media/{mediaId}/thumbnail", s.handleServeThumbnailMedia)
			r.Get("/activity-types", s.handleActivityTypes)
			r.Get("/stats/summary", s.handleSummary)
			r.Get("/stats/calendar", s.handleCalendar)
			r.Get("/health/daily", s.handleDailyHealthMetrics)
			r.Get("/gears", s.handleListGears)
			r.Get("/gears/{id}", s.handleGetGear)
			r.Get("/imports", s.handleListImports)
			r.Post("/imports", s.handleImport)
			r.Get("/providers/garmin/status", s.handleGarminStatus)
			r.Get("/providers/google/status", s.handleGoogleStatus)
			r.Get("/providers/google/connect", s.handleGoogleConnect)
			r.Post("/providers/google/disconnect", s.handleGoogleDisconnect)
			r.Post("/providers/garmin/connect", s.handleGarminConnect)
			r.Post("/providers/garmin/sync", s.handleGarminSync)
			r.Post("/providers/garmin/health-sync", s.handleGarminHealthSync)
			r.Post("/providers/garmin/gear-sync", s.handleGarminGearSync)
			r.Post("/training-sheet/sync", s.handleTrainingSheetSync)
			r.Get("/planned-activities", s.handlePlannedActivities)
			r.Get("/sync-jobs", s.handleSyncJobs)
			r.Post("/sync-jobs/{jobID}/cancel", s.handleCancelSyncJob)
		})
	})

	r.NotFound(s.serveSPA)
	return r
}

type climbDetectionUpdateRequest struct {
	Settings    *ClimbDetectionSettings `json:"settings"`
	Preset      string                  `json:"preset"`
	Sensitivity *int                    `json:"sensitivity"`
}

type climbDetectionPayload struct {
	MapTileURL     string               `json:"mapTileURL"`
	BaseURL        string               `json:"baseURL"`
	ClimbDetection ClimbDetectionConfig `json:"climbDetection"`
}

type climbPreviewRequest struct {
	Sensitivity int `json:"sensitivity"`
}

type climbPreviewPayload struct {
	Climbs []ActivityClimb `json:"climbs"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.LocalAuthEnabled {
		writeError(w, http.StatusNotFound, "local password login is disabled")
		return
	}
	if s.loginLimiter != nil && !s.loginLimiter.allow(requestClientKey(r, s.cfg.TrustProxy), time.Now()) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many login attempts")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := s.store.GetUserByUsername(r.Context(), body.Username)
	hash := dummyPasswordHash
	if err == nil {
		hash, err = s.store.PasswordHash(r.Context(), user.ID)
	}
	if err != nil || user.Disabled || bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	csrf, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	sessionID, err := s.store.CreateSession(r.Context(), user.ID, csrf, sessionAbsoluteTTL)
	if err != nil {
		s.logger.Error("create session", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	http.SetCookie(w, s.sessionCookie(sessionID, sessionAbsoluteTTL))
	_ = s.store.TouchUserLogin(r.Context(), user.ID)
	writeJSON(w, http.StatusOK, sessionResponse(SessionRecord{CSRF: csrf, Actor: user, Effective: user}))
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(s.authCookieName())
	if err != nil || cookie.Value == "" {
		writeJSON(w, http.StatusOK, s.unauthenticatedSessionResponse())
		return
	}
	record, err := s.store.GetSessionRecord(r.Context(), cookie.Value)
	if err != nil {
		writeJSON(w, http.StatusOK, s.unauthenticatedSessionResponse())
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse(record))
}

func (s *Server) handleToolsPace(w http.ResponseWriter, r *http.Request) {
	var body toolsPaceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	result, err := calculateToolsPace(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleToolsVDOT(w http.ResponseWriter, r *http.Request) {
	var body toolsVDOTRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	result, err := calculateToolsVDOT(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(s.authCookieName()); err == nil {
		_ = s.store.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, s.sessionCookie("", -1*time.Hour))
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

const dummyPasswordHash = "$2a$10$7EqJtq98hPqEX7fNZaFWoOeO5s8M8eOe3N5m0f0lK9P3Z9P0h8YqG"

func (s *Server) authCookieName() string {
	if s.cfg.PublicMode {
		return publicSessionCookieName
	}
	return sessionCookieName
}

func (s *Server) unauthenticatedSessionResponse() map[string]any {
	return map[string]any{
		"authenticated":     false,
		"publicMode":        s.cfg.PublicMode,
		"localLoginEnabled": s.cfg.LocalAuthEnabled,
		"googleOIDCEnabled": s.cfg.OIDCClientID != "" && s.cfg.OIDCClientSecret != "" && len(s.cfg.OIDCAllowedEmails) > 0,
	}
}

func sessionResponse(record SessionRecord) map[string]any {
	return map[string]any{
		"authenticated": true,
		"csrfToken":     record.CSRF,
		"actor":         sessionUser(record.Actor),
		"user":          sessionUser(record.Effective),
		"supportMode":   record.Support,
		"canWrite":      !record.Support,
	}
}

func sessionUser(user User) SessionUser {
	return SessionUser{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Role: user.Role}
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	payload, err := s.climbDetectionPayload(r.Context())
	if err != nil {
		s.logger.Error("load config", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load config")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleUpdateClimbDetection(w http.ResponseWriter, r *http.Request) {
	var body climbDetectionUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var settings ClimbDetectionSettings
	preset := strings.TrimSpace(body.Preset)
	if body.Sensitivity != nil {
		settings = climbDetectionSettingsForSensitivity(*body.Sensitivity)
		if preset == "" {
			preset = "custom"
		}
	} else if body.Settings != nil {
		settings = *body.Settings
		if err := validateClimbDetectionSettings(settings); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		writeError(w, http.StatusBadRequest, "either settings or sensitivity must be provided")
		return
	}
	if preset == "" {
		preset = "custom"
	}
	if err := s.store.SetClimbDetectionSettings(r.Context(), preset, settings); err != nil {
		s.logger.Error("update climb detection settings", "error", err)
		writeError(w, http.StatusInternalServerError, "could not save climb detection settings")
		return
	}
	payload, err := s.climbDetectionPayload(r.Context())
	if err != nil {
		s.logger.Error("load config", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load config")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleActivityClimbsPreview(w http.ResponseWriter, r *http.Request) {
	var body climbPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	activity, err := s.store.GetActivity(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "activity not found")
		return
	}
	if err != nil {
		s.logger.Error("get activity for climb preview", "error", err)
		writeError(w, http.StatusInternalServerError, "could not get activity")
		return
	}

	payload := climbPreviewPayload{
		Climbs: detectActivityClimbsWithSettings(activity.Samples, climbDetectionSettingsForSensitivity(body.Sensitivity)),
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) climbDetectionPayload(ctx context.Context) (climbDetectionPayload, error) {
	climbDetection, err := s.store.GetClimbDetectionSettings(ctx)
	if err != nil {
		return climbDetectionPayload{}, err
	}
	return climbDetectionPayload{
		MapTileURL:     s.cfg.MapTileURL,
		BaseURL:        s.cfg.BaseURL,
		ClimbDetection: climbDetection,
	}, nil
}

func (s *Server) handleListActivities(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	page, err := s.store.ListActivityPage(r.Context(), limit, offset, activityFiltersFromQuery(r))
	if err != nil {
		s.logger.Error("list activities", "error", err)
		writeError(w, http.StatusInternalServerError, "could not list activities")
		return
	}
	writeJSON(w, http.StatusOK, page)
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
	media, err := s.store.ListActivityMedia(r.Context(), activity.ID)
	if err != nil {
		s.logger.Error("list activity media", "activity_id", activity.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "could not load activity media")
		return
	}
	activity.Media = media
	writeJSON(w, http.StatusOK, map[string]any{"activity": activity})
}

func (s *Server) handleExportActivityGPX(w http.ResponseWriter, r *http.Request) {
	activity, err := s.store.GetActivity(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "activity not found")
		return
	}
	if err != nil {
		s.logger.Error("get activity for GPX export", "error", err)
		writeError(w, http.StatusInternalServerError, "could not get activity")
		return
	}

	includeSensors := parseQueryBool(r.URL.Query().Get("includeSensors"))
	data, err := exportActivityGPX(activity, includeSensors)
	if errors.Is(err, ErrActivityGPXNoRoute) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		s.logger.Error("export activity GPX", "activity_id", activity.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "could not export GPX")
		return
	}

	w.Header().Set("Content-Type", "application/gpx+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": activityGPXFilename(activity)}))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleDeleteActivity(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")
	mediaItems, mediaErr := s.store.ListActivityMedia(r.Context(), activityID)
	if mediaErr != nil {
		s.logger.Warn("list activity media before delete", "activity_id", activityID, "error", mediaErr)
	}

	result, err := s.store.DeleteActivity(r.Context(), activityID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "activity not found")
		return
	}
	if err != nil {
		s.logger.Error("delete activity", "error", err)
		writeError(w, http.StatusInternalServerError, "could not delete activity")
		return
	}
	if mediaErr == nil {
		s.media.RemoveActivityMediaFiles(mediaItems)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleUploadActivityMedia(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxMediaUploadBytes + 1); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	media, err := s.media.UploadActivityMedia(r.Context(), chi.URLParam(r, "id"), header.Filename, file)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "activity not found")
		return
	}
	if isMediaClientError(err) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		s.logger.Error("upload activity media", "error", err)
		writeError(w, http.StatusInternalServerError, "could not upload media")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"media": media})
}

func (s *Server) handleDeleteActivityMedia(w http.ResponseWriter, r *http.Request) {
	_, err := s.media.DeleteActivityMedia(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "mediaId"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	if err != nil {
		s.logger.Error("delete activity media", "error", err)
		writeError(w, http.StatusInternalServerError, "could not delete media")
		return
	}
	writeJSON(w, http.StatusOK, DeleteActivityMediaResult{Deleted: true})
}

func (s *Server) handleServeOriginalMedia(w http.ResponseWriter, r *http.Request) {
	s.handleServeActivityMedia(w, r, false)
}

func (s *Server) handleServeThumbnailMedia(w http.ResponseWriter, r *http.Request) {
	s.handleServeActivityMedia(w, r, true)
}

func (s *Server) handleServeActivityMedia(w http.ResponseWriter, r *http.Request, thumbnail bool) {
	media, err := s.store.GetActivityMedia(r.Context(), chi.URLParam(r, "mediaId"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	if err != nil {
		s.logger.Error("get activity media", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load media")
		return
	}

	var path string
	contentType := media.ContentType
	filename := media.OriginalFilename
	if thumbnail {
		path, err = s.media.ThumbnailFilePath(media)
		contentType = "image/jpeg"
		filename = strings.TrimSuffix(media.OriginalFilename, filepath.Ext(media.OriginalFilename)) + "-thumbnail.jpg"
	} else {
		path, err = s.media.OriginalFilePath(media)
	}
	if err != nil {
		s.logger.Error("resolve activity media path", "media_id", media.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "could not load media")
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, "media file not found")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filename}))
	http.ServeFile(w, r, path)
}

func (s *Server) handleRenameActivity(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     *string         `json:"name"`
		Notes    *string         `json:"notes"`
		Feedback *string         `json:"feedback"`
		RPE      json.RawMessage `json:"rpe"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Name == nil && body.Notes == nil && body.Feedback == nil && len(body.RPE) == 0 {
		writeError(w, http.StatusBadRequest, "missing activity update")
		return
	}
	var activity Activity
	var err error
	id := chi.URLParam(r, "id")
	if body.Name != nil {
		activity, err = s.store.RenameActivity(r.Context(), id, *body.Name)
	}
	if err == nil && body.Notes != nil {
		activity, err = s.store.UpdateActivityNotes(r.Context(), id, *body.Notes)
	}
	var rpeSet bool
	var rpe *int
	if len(body.RPE) > 0 {
		rpeSet = true
		if string(body.RPE) != "null" {
			var value int
			if json.Unmarshal(body.RPE, &value) != nil {
				writeError(w, http.StatusBadRequest, "rpe must be an integer or null")
				return
			}
			rpe = &value
		}
	}
	if err == nil && (body.Feedback != nil || rpeSet) {
		activity, err = s.store.UpdateActivityFeedback(r.Context(), id, body.Feedback, rpeSet, rpe)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "activity not found")
		return
	}
	if errors.Is(err, ErrInvalidActivityName) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, ErrInvalidActivityNotes) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, ErrInvalidActivityFeedback) || errors.Is(err, ErrInvalidActivityRPE) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		s.logger.Error("update activity", "error", err)
		writeError(w, http.StatusInternalServerError, "could not update activity")
		return
	}
	if body.Feedback != nil || rpeSet {
		if planned, matchErr := s.store.GetMatchedPlannedActivity(r.Context(), id); matchErr == nil {
			s.queueTrainingSheetWriteback(r.Context(), planned.ID, id)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"activity": activity})
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

func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	filters := activityFiltersFromQuery(r)
	dateFrom, dateTo, err := calendarDateRangeFromQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	filters.DateFrom = dateFrom
	filters.DateTo = dateTo
	calendar, err := s.store.ActivityCalendar(r.Context(), filters)
	if err != nil {
		s.logger.Error("calendar activities", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load calendar")
		return
	}
	writeJSON(w, http.StatusOK, calendar)
}

func (s *Server) handleDailyHealthMetrics(w http.ResponseWriter, r *http.Request) {
	from, to, err := healthRangeFromQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	metrics, err := s.store.ListDailyHealthMetrics(r.Context(), garminProvider, from, to)
	if err != nil {
		s.logger.Error("list daily health metrics", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load health metrics")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"metrics": metrics})
}

func (s *Server) handleListGears(w http.ResponseWriter, r *http.Request) {
	gears, err := s.store.ListGears(r.Context())
	if err != nil {
		s.logger.Error("list gears", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load gear")
		return
	}
	active := make([]Gear, 0)
	retired := make([]Gear, 0)
	for _, gear := range gears {
		if gear.Retired {
			retired = append(retired, gear)
		} else {
			active = append(active, gear)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"gear": gears, "active": active, "retired": retired})
}

func (s *Server) handleGetGear(w http.ResponseWriter, r *http.Request) {
	gear, err := s.store.GetGear(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "gear not found")
		return
	}
	if err != nil {
		s.logger.Error("get gear", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load gear")
		return
	}
	activities, err := s.store.ListGearActivities(r.Context(), gear.ID)
	if err != nil {
		s.logger.Error("list gear activities", "gear_id", gear.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "could not load gear activities")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"gear": gear, "activities": activities})
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
	if err := r.ParseMultipartForm(maxImportUploadBytes + 1); err != nil {
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

func (s *Server) handleGarminStatus(w http.ResponseWriter, r *http.Request) {
	conn, connected, err := s.garmin.Status(r.Context())
	if err != nil {
		s.logger.Error("garmin status", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load Garmin status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"connected":  connected,
		"connection": conn,
	})
}

func (s *Server) handleGarminConnect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		MFACode  string `json:"mfaCode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	conn, err := s.garmin.Connect(r.Context(), body.Email, body.Password, body.MFACode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "connection": conn})
}

func (s *Server) handleGarminSync(w http.ResponseWriter, r *http.Request) {
	opts, err := decodeGarminSyncOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	running, err := s.store.HasRunningSyncJob(r.Context(), garminProvider)
	if err != nil {
		s.logger.Error("check running garmin sync", "error", err)
		writeError(w, http.StatusInternalServerError, "could not check sync state")
		return
	}
	if running {
		writeError(w, http.StatusConflict, "Garmin sync is already running")
		return
	}
	jobID, err := s.store.CreateSyncJob(r.Context(), garminProvider, "manual")
	if err != nil {
		if errors.Is(err, ErrSyncJobAlreadyRunning) {
			writeError(w, http.StatusConflict, "Garmin sync is already running")
			return
		}
		s.logger.Error("create garmin sync job", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create sync job")
		return
	}
	go s.runGarminManualSyncJob(jobID, opts)
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": jobID, "status": "running"})
}

func (s *Server) handleGarminHealthSync(w http.ResponseWriter, r *http.Request) {
	opts, err := decodeGarminHealthSyncOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	running, err := s.store.HasRunningSyncJob(r.Context(), garminProvider)
	if err != nil {
		s.logger.Error("check running garmin health sync", "error", err)
		writeError(w, http.StatusInternalServerError, "could not check sync state")
		return
	}
	if running {
		writeError(w, http.StatusConflict, "Garmin sync is already running")
		return
	}
	jobID, err := s.store.CreateSyncJob(r.Context(), garminProvider, "health_manual")
	if err != nil {
		if errors.Is(err, ErrSyncJobAlreadyRunning) {
			writeError(w, http.StatusConflict, "Garmin sync is already running")
			return
		}
		s.logger.Error("create garmin health sync job", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create sync job")
		return
	}
	go s.runGarminManualHealthSyncJob(jobID, opts)
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": jobID, "status": "running"})
}

func (s *Server) handleGarminGearSync(w http.ResponseWriter, r *http.Request) {
	running, err := s.store.HasRunningSyncJob(r.Context(), garminProvider)
	if err != nil {
		s.logger.Error("check running garmin gear sync", "error", err)
		writeError(w, http.StatusInternalServerError, "could not check sync state")
		return
	}
	if running {
		writeError(w, http.StatusConflict, "Garmin sync is already running")
		return
	}
	jobID, err := s.store.CreateSyncJob(r.Context(), garminProvider, "gear_manual")
	if err != nil {
		if errors.Is(err, ErrSyncJobAlreadyRunning) {
			writeError(w, http.StatusConflict, "Garmin sync is already running")
			return
		}
		s.logger.Error("create garmin gear sync job", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create sync job")
		return
	}
	go s.runGarminManualGearSyncJob(jobID)
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

func (s *Server) handleCancelSyncJob(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(chi.URLParam(r, "jobID"))
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "missing sync job id")
		return
	}
	status, active, err := s.store.RequestSyncJobCancellation(r.Context(), jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "sync job not found")
		return
	}
	if err != nil {
		s.logger.Error("request sync job cancellation", "job_id", jobID, "error", err)
		writeError(w, http.StatusInternalServerError, "could not cancel sync job")
		return
	}
	if active {
		s.cancelSyncJob(jobID)
		writeJSON(w, http.StatusAccepted, map[string]any{
			"jobId":           jobID,
			"status":          "cancelling",
			"cancelRequested": true,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jobId":           jobID,
		"status":          status,
		"cancelRequested": false,
	})
}

func (s *Server) StartBackgroundSync(ctx context.Context) {
	if err := s.store.ReconcileRunningSyncJobs(context.Background()); err != nil {
		s.logger.Error("reconcile sync jobs after startup", "error", err)
	}
	go s.runGarminScheduledSync(ctx)
	go s.runTrainingSheetScheduledSync(ctx)
}

func (s *Server) cancellableSyncJobContext(parent context.Context, userID, jobID string, timeout time.Duration) (context.Context, func()) {
	userCtx := withUserID(parent, userID)
	timeoutCtx, timeoutCancel := context.WithTimeout(userCtx, timeout)
	syncCtx, cancel := context.WithCancel(timeoutCtx)

	s.syncCancelsMu.Lock()
	s.syncCancels[jobID] = cancel
	s.syncCancelsMu.Unlock()

	checkCtx, checkCancel := s.syncJobPersistenceContext(userID)
	requested, err := s.store.SyncJobCancellationRequested(checkCtx, jobID)
	checkCancel()
	if err != nil {
		s.logger.Error("check sync job cancellation", "job_id", jobID, "error", err)
	} else if requested {
		cancel()
	}

	cleanup := func() {
		s.syncCancelsMu.Lock()
		delete(s.syncCancels, jobID)
		s.syncCancelsMu.Unlock()
		timeoutCancel()
		cancel()
	}
	return syncCtx, cleanup
}

func (s *Server) cancelSyncJob(jobID string) {
	s.syncCancelsMu.Lock()
	cancel, ok := s.syncCancels[jobID]
	s.syncCancelsMu.Unlock()
	if ok {
		cancel()
	}
}

func (s *Server) syncJobPersistenceContext(userID string) (context.Context, context.CancelFunc) {
	return context.WithTimeout(withUserID(context.Background(), userID), 30*time.Second)
}

func (s *Server) finishSyncJob(ctx context.Context, jobID, status, message string, payload map[string]any) error {
	userID, err := userIDFromContext(ctx)
	if err != nil {
		return err
	}
	persistCtx, cancel := s.syncJobPersistenceContext(userID)
	defer cancel()
	return s.store.FinishSyncJob(persistCtx, jobID, status, message, payload)
}

func (s *Server) runGarminScheduledSync(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runGarminScheduledSyncOnce(ctx)
		}
	}
}

func (s *Server) runGarminScheduledSyncOnce(ctx context.Context) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		s.logger.Error("list users for scheduled Garmin sync", "error", err)
		return
	}
	for _, user := range users {
		if user.Disabled {
			continue
		}
		userCtx := withUserID(ctx, user.ID)
		if _, connected, err := s.garmin.Status(userCtx); err != nil || !connected {
			continue
		}
		if running, err := s.store.HasRunningSyncJob(userCtx, garminProvider); err != nil || running {
			if err != nil {
				s.logger.Error("check running scheduled garmin sync", "user_id", user.ID, "error", err)
			}
			continue
		}
		jobID, err := s.store.CreateSyncJob(userCtx, garminProvider, "scheduled")
		if err != nil {
			s.logger.Error("create scheduled garmin sync job", "user_id", user.ID, "error", err)
			continue
		}
		oldest := time.Now().UTC().AddDate(0, 0, -30)
		go s.runGarminScheduledSyncJob(ctx, user.ID, jobID, oldest)
	}
}

func (s *Server) runGarminScheduledSyncJob(parent context.Context, userID, jobID string, oldest time.Time) {
	syncCtx, cleanup := s.cancellableSyncJobContext(parent, userID, jobID, 30*time.Minute)
	defer cleanup()
	if _, err := s.finishGarminSyncJob(syncCtx, jobID, GarminSyncOptions{Oldest: oldest}); err != nil {
		s.logger.Error("scheduled garmin sync", "user_id", userID, "error", err)
		return
	}
	s.runGarminScheduledHealthSyncOnce(withUserID(parent, userID))
}

func (s *Server) runGarminManualSyncJob(jobID string, opts GarminSyncOptions) {
	lookupCtx, lookupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	userID, err := s.store.SyncJobUserID(lookupCtx, jobID)
	lookupCancel()
	if err != nil {
		s.logger.Error("load Garmin sync owner", "job_id", jobID, "error", err)
		return
	}
	ctx, cleanup := s.cancellableSyncJobContext(context.Background(), userID, jobID, 6*time.Hour)
	defer cleanup()
	if _, err := s.finishGarminSyncJob(ctx, jobID, opts); err != nil {
		s.logger.Error("manual garmin sync", "job_id", jobID, "error", err)
	}
}

func (s *Server) finishGarminSyncJob(ctx context.Context, jobID string, opts GarminSyncOptions) (map[string]any, error) {
	progress := func(payload map[string]any) {
		if err := s.store.UpdateSyncJobProgress(ctx, jobID, payload); err != nil {
			s.logger.Error("update garmin sync progress", "job_id", jobID, "error", err)
		}
	}
	payload, err := s.garmin.Sync(ctx, opts, progress)
	if err != nil {
		_ = s.finishSyncJob(ctx, jobID, "failed", err.Error(), nil)
		return nil, err
	}
	gearPayload, gearErr := s.garmin.SyncGear(ctx, progress)
	if gearErr != nil && ctx.Err() != nil {
		_ = s.finishSyncJob(ctx, jobID, "failed", gearErr.Error(), payload)
		return nil, gearErr
	}
	mergeGearSyncPayload(payload, gearPayload, gearErr)
	if gearErr != nil {
		s.logger.Error("garmin gear refresh after activity sync", "job_id", jobID, "error", gearErr)
	}
	if err := s.finishSyncJob(ctx, jobID, "completed", "", payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Server) runGarminScheduledHealthSyncOnce(ctx context.Context) {
	due, err := s.garminScheduledHealthSyncDue(ctx)
	if err != nil {
		s.logger.Error("check scheduled garmin health sync", "error", err)
		return
	}
	if !due {
		return
	}
	if running, err := s.store.HasRunningSyncJob(ctx, garminProvider); err != nil || running {
		if err != nil {
			s.logger.Error("check running scheduled garmin health sync", "error", err)
		}
		return
	}
	jobID, err := s.store.CreateSyncJob(ctx, garminProvider, "health_scheduled")
	if err != nil {
		s.logger.Error("create scheduled garmin health sync job", "error", err)
		return
	}
	syncCtx, cleanup := s.cancellableSyncJobContext(ctx, scopedUserID(ctx), jobID, 30*time.Minute)
	defer cleanup()
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -(garminHealthScheduledRefreshDays - 1))
	if _, err := s.finishGarminHealthSyncJob(syncCtx, jobID, GarminHealthSyncOptions{From: from, To: to}); err != nil {
		s.logger.Error("scheduled garmin health sync", "error", err)
	}
}

func (s *Server) garminScheduledHealthSyncDue(ctx context.Context) (bool, error) {
	createdAt, ok, err := s.store.LatestSyncJobCreatedAt(ctx, garminProvider, "health_scheduled")
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	return time.Since(createdAt) >= 4*time.Hour, nil
}

func (s *Server) runGarminManualHealthSyncJob(jobID string, opts GarminHealthSyncOptions) {
	lookupCtx, lookupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	userID, err := s.store.SyncJobUserID(lookupCtx, jobID)
	lookupCancel()
	if err != nil {
		s.logger.Error("load Garmin health sync owner", "job_id", jobID, "error", err)
		return
	}
	ctx, cleanup := s.cancellableSyncJobContext(context.Background(), userID, jobID, 6*time.Hour)
	defer cleanup()
	if _, err := s.finishGarminHealthSyncJob(ctx, jobID, opts); err != nil {
		s.logger.Error("manual garmin health sync", "job_id", jobID, "error", err)
	}
}

func (s *Server) runGarminManualGearSyncJob(jobID string) {
	lookupCtx, lookupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	userID, err := s.store.SyncJobUserID(lookupCtx, jobID)
	lookupCancel()
	if err != nil {
		s.logger.Error("load Garmin gear sync owner", "job_id", jobID, "error", err)
		return
	}
	ctx, cleanup := s.cancellableSyncJobContext(context.Background(), userID, jobID, 2*time.Hour)
	defer cleanup()
	if _, err := s.finishGarminGearSyncJob(ctx, jobID); err != nil {
		s.logger.Error("manual garmin gear sync", "job_id", jobID, "error", err)
	}
}

func (s *Server) finishGarminGearSyncJob(ctx context.Context, jobID string) (map[string]any, error) {
	progress := func(payload map[string]any) {
		if err := s.store.UpdateSyncJobProgress(ctx, jobID, payload); err != nil {
			s.logger.Error("update garmin gear sync progress", "job_id", jobID, "error", err)
		}
	}
	payload, err := s.garmin.SyncGear(ctx, progress)
	if err != nil {
		_ = s.finishSyncJob(ctx, jobID, "failed", err.Error(), nil)
		return nil, err
	}
	if err := s.finishSyncJob(ctx, jobID, "completed", "", payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Server) finishGarminHealthSyncJob(ctx context.Context, jobID string, opts GarminHealthSyncOptions) (map[string]any, error) {
	progress := func(payload map[string]any) {
		if err := s.store.UpdateSyncJobProgress(ctx, jobID, payload); err != nil {
			s.logger.Error("update garmin health sync progress", "job_id", jobID, "error", err)
		}
	}
	payload, err := s.garmin.SyncHealth(ctx, opts, progress)
	if err != nil {
		_ = s.finishSyncJob(ctx, jobID, "failed", err.Error(), nil)
		return nil, err
	}
	if err := s.finishSyncJob(ctx, jobID, "completed", "", payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeGarminSyncOptions(r *http.Request) (GarminSyncOptions, error) {
	var body struct {
		Oldest  string `json:"oldest"`
		AllData bool   `json:"allData"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return GarminSyncOptions{}, errors.New("invalid JSON body")
	}
	if body.AllData {
		return GarminSyncOptions{AllData: true}, nil
	}
	if strings.TrimSpace(body.Oldest) == "" {
		return GarminSyncOptions{}, nil
	}
	oldest, err := time.Parse("2006-01-02", strings.TrimSpace(body.Oldest))
	if err != nil {
		return GarminSyncOptions{}, errors.New("oldest must use YYYY-MM-DD format")
	}
	if time.Now().UTC().Sub(oldest) > 10*365*24*time.Hour {
		return GarminSyncOptions{}, errors.New("oldest cannot be more than ten years ago")
	}
	return GarminSyncOptions{Oldest: oldest}, nil
}

func decodeGarminHealthSyncOptions(r *http.Request) (GarminHealthSyncOptions, error) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return GarminHealthSyncOptions{}, errors.New("invalid JSON body")
	}
	from, err := parseOptionalAPIDate(body.From, "from")
	if err != nil {
		return GarminHealthSyncOptions{}, err
	}
	to, err := parseOptionalAPIDate(body.To, "to")
	if err != nil {
		return GarminHealthSyncOptions{}, err
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return GarminHealthSyncOptions{}, errors.New("from must be before or equal to to")
	}
	return GarminHealthSyncOptions{From: from, To: to}, nil
}

func mergeGearSyncPayload(payload map[string]any, gearPayload map[string]any, gearErr error) {
	if payload == nil {
		return
	}
	if gearPayload != nil {
		payload["gear"] = gearPayload["gear"]
		payload["gearSaved"] = gearPayload["saved"]
		payload["gearAssignments"] = gearPayload["assignments"]
		payload["gearLocalAssignments"] = gearPayload["localAssignments"]
	}
	if gearErr == nil {
		return
	}
	warnings, _ := payload["warnings"].([]string)
	warnings = append(warnings, "Gear refresh failed: "+gearErr.Error())
	payload["warnings"] = warnings
}

func healthRangeFromQuery(r *http.Request) (time.Time, time.Time, error) {
	values := r.URL.Query()
	from, err := parseOptionalAPIDate(values.Get("from"), "from")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := parseOptionalAPIDate(values.Get("to"), "to")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	from, to, err = garminHealthSyncRange(GarminHealthSyncOptions{From: from, To: to}, time.Now().UTC())
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to, nil
}

func parseOptionalAPIDate(value, field string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must use YYYY-MM-DD format", field)
	}
	return parsed, nil
}

func calendarDateRangeFromQuery(r *http.Request) (time.Time, time.Time, error) {
	values := r.URL.Query()
	dateFrom, err := parseOptionalAPIDate(values.Get("dateFrom"), "dateFrom")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	dateTo, err := parseOptionalAPIDate(values.Get("dateTo"), "dateTo")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if dateFrom.IsZero() && dateTo.IsZero() {
		now := time.Now().UTC()
		dateFrom = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		dateTo = dateFrom.AddDate(0, 1, 0).Add(-24 * time.Hour)
	}
	if dateFrom.IsZero() {
		dateFrom = dateTo
	}
	if dateTo.IsZero() {
		dateTo = dateFrom
	}
	if dateFrom.After(dateTo) {
		return time.Time{}, time.Time{}, errors.New("dateFrom must be before or equal to dateTo")
	}
	if dateTo.Sub(dateFrom) > 370*24*time.Hour {
		return time.Time{}, time.Time{}, errors.New("calendar range cannot exceed 370 days")
	}
	return dateFrom, dateTo, nil
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
		if s.cfg.PublicMode && isMutating(r.Method) && !s.sameOriginRequest(r) {
			writeError(w, http.StatusForbidden, "request origin is not allowed")
			return
		}
		cookie, err := r.Cookie(s.authCookieName())
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		record, err := s.store.GetSessionRecord(r.Context(), cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if isMutating(r.Method) && r.Header.Get("X-CSRF-Token") != record.CSRF {
			writeError(w, http.StatusForbidden, "invalid CSRF token")
			return
		}
		if record.Support && isMutating(r.Method) && !supportMutationAllowed(r.URL.Path) {
			writeError(w, http.StatusForbidden, "support mode is read-only")
			return
		}
		ctx := context.WithValue(r.Context(), csrfContextKey{}, record.CSRF)
		ctx = withPrincipal(ctx, UserPrincipal{
			ID:            record.Effective.ID,
			Username:      record.Effective.Username,
			DisplayName:   record.Effective.DisplayName,
			Role:          record.Actor.Role,
			SupportTarget: record.Support,
			ActorID:       record.Actor.ID,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func supportMutationAllowed(path string) bool {
	return path == "/api/session/logout" ||
		path == "/api/session/support" ||
		path == "/api/session/password" ||
		strings.HasPrefix(path, "/api/users")
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

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self' https://accounts.google.com; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; connect-src 'self'; font-src 'self' data:")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if s.cfg.PublicMode && strings.HasPrefix(s.cfg.BaseURL, "https://") {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil {
			limit := int64(1 << 20)
			switch {
			case r.URL.Path == "/api/imports":
				limit = 80<<20 + 1<<20
			case strings.HasSuffix(r.URL.Path, "/media"):
				limit = maxMediaUploadBytes + 1<<20
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sameOriginRequest(r *http.Request) bool {
	base, err := url.Parse(s.cfg.BaseURL)
	if err != nil {
		return false
	}
	for _, value := range []string{r.Header.Get("Origin"), r.Header.Get("Referer")} {
		if value == "" {
			continue
		}
		candidate, err := url.Parse(value)
		if err == nil && strings.EqualFold(candidate.Scheme, base.Scheme) && strings.EqualFold(candidate.Host, base.Host) {
			return true
		}
		return false
	}
	return false
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
	root, err := filepath.Abs(s.cfg.StaticDir)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	path := filepath.Join(root, requested)
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	indexPath := filepath.Join(root, "index.html")
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
		Name:     s.authCookieName(),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.PublicMode || strings.HasPrefix(s.cfg.BaseURL, "https://"),
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

func isMediaClientError(err error) bool {
	return errors.Is(err, ErrEmptyMediaFile) ||
		errors.Is(err, ErrMediaFileTooLarge) ||
		errors.Is(err, ErrUnsupportedMediaType) ||
		errors.Is(err, ErrMediaImageTooLarge)
}

func parseQueryBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
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
