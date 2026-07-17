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
	garmin    *GarminService
	media     *MediaService
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
		garmin:    NewGarminService(cfg, store),
		media:     NewMediaService(cfg, store),
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
			r.Patch("/activities/{id}", s.handleRenameActivity)
			r.Delete("/activities/{id}", s.handleDeleteActivity)
			r.Post("/activities/{id}/media", s.handleUploadActivityMedia)
			r.Delete("/activities/{id}/media/{mediaId}", s.handleDeleteActivityMedia)
			r.Get("/activity-media/{mediaId}/original", s.handleServeOriginalMedia)
			r.Get("/activity-media/{mediaId}/thumbnail", s.handleServeThumbnailMedia)
			r.Get("/activity-types", s.handleActivityTypes)
			r.Get("/stats/summary", s.handleSummary)
			r.Get("/health/daily", s.handleDailyHealthMetrics)
			r.Get("/gears", s.handleListGears)
			r.Get("/gears/{id}", s.handleGetGear)
			r.Get("/imports", s.handleListImports)
			r.Post("/imports", s.handleImport)
			r.Get("/providers/garmin/status", s.handleGarminStatus)
			r.Post("/providers/garmin/connect", s.handleGarminConnect)
			r.Post("/providers/garmin/sync", s.handleGarminSync)
			r.Post("/providers/garmin/health-sync", s.handleGarminHealthSync)
			r.Post("/providers/garmin/gear-sync", s.handleGarminGearSync)
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
		"mapTileURL": s.cfg.MapTileURL,
		"baseURL":    s.cfg.BaseURL,
	})
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
		Name  *string `json:"name"`
		Notes *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Name == nil && body.Notes == nil {
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
	if err != nil {
		s.logger.Error("update activity", "error", err)
		writeError(w, http.StatusInternalServerError, "could not update activity")
		return
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

func (s *Server) StartBackgroundSync(ctx context.Context) {
	go s.runGarminScheduledSync(ctx)
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
	if _, connected, err := s.garmin.Status(ctx); err != nil || !connected {
		return
	}
	if running, err := s.store.HasRunningSyncJob(ctx, garminProvider); err != nil || running {
		if err != nil {
			s.logger.Error("check running scheduled garmin sync", "error", err)
		}
		return
	}
	jobID, err := s.store.CreateSyncJob(ctx, garminProvider, "scheduled")
	if err != nil {
		s.logger.Error("create scheduled garmin sync job", "error", err)
		return
	}
	syncCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	oldest := time.Now().UTC().AddDate(0, 0, -30)
	if _, err := s.finishGarminSyncJob(syncCtx, jobID, GarminSyncOptions{Oldest: oldest}); err != nil {
		s.logger.Error("scheduled garmin sync", "error", err)
	}
	s.runGarminScheduledHealthSyncOnce(ctx)
}

func (s *Server) runGarminManualSyncJob(jobID string, opts GarminSyncOptions) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
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
		_ = s.store.FinishSyncJob(ctx, jobID, "failed", err.Error(), nil)
		return nil, err
	}
	gearPayload, gearErr := s.garmin.SyncGear(ctx, progress)
	mergeGearSyncPayload(payload, gearPayload, gearErr)
	if gearErr != nil {
		s.logger.Error("garmin gear refresh after activity sync", "job_id", jobID, "error", gearErr)
	}
	if err := s.store.FinishSyncJob(ctx, jobID, "completed", "", payload); err != nil {
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
	syncCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	if _, err := s.finishGarminHealthSyncJob(ctx, jobID, opts); err != nil {
		s.logger.Error("manual garmin health sync", "job_id", jobID, "error", err)
	}
}

func (s *Server) runGarminManualGearSyncJob(jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
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
		_ = s.store.FinishSyncJob(ctx, jobID, "failed", err.Error(), nil)
		return nil, err
	}
	if err := s.store.FinishSyncJob(ctx, jobID, "completed", "", payload); err != nil {
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
		_ = s.store.FinishSyncJob(ctx, jobID, "failed", err.Error(), nil)
		return nil, err
	}
	if err := s.store.FinishSyncJob(ctx, jobID, "completed", "", payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeGarminSyncOptions(r *http.Request) (GarminSyncOptions, error) {
	var body struct {
		Oldest string `json:"oldest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return GarminSyncOptions{}, errors.New("invalid JSON body")
	}
	if strings.TrimSpace(body.Oldest) == "" {
		return GarminSyncOptions{}, nil
	}
	oldest, err := time.Parse("2006-01-02", strings.TrimSpace(body.Oldest))
	if err != nil {
		return GarminSyncOptions{}, errors.New("oldest must use YYYY-MM-DD format")
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

func isMediaClientError(err error) bool {
	return errors.Is(err, ErrEmptyMediaFile) ||
		errors.Is(err, ErrMediaFileTooLarge) ||
		errors.Is(err, ErrUnsupportedMediaType)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
