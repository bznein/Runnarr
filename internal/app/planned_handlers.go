package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (s *Server) googleAuth() *GoogleSheetsAuthService {
	return NewGoogleSheetsAuthService(s.store, s.cfg)
}

func (s *Server) handleGoogleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.googleAuth().Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load Google Sheets status")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleGoogleConnect(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "login is required")
		return
	}
	authURL, err := s.googleAuth().AuthorizationURL(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, s.cfg.BaseURL+"/settings?google=denied", http.StatusFound)
		return
	}
	state, code := r.URL.Query().Get("state"), r.URL.Query().Get("code")
	if state == "" || code == "" {
		writeError(w, http.StatusBadRequest, "Google OAuth callback is incomplete")
		return
	}
	sessionCookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "login session is missing")
		return
	}
	sessionID, err := s.store.ConsumeGoogleOAuthState(r.Context(), state)
	if err != nil || sessionID != sessionCookie.Value {
		writeError(w, http.StatusBadRequest, "invalid Google OAuth state")
		return
	}
	if err := s.googleAuth().Exchange(r.Context(), code); err != nil {
		s.logger.Error("Google OAuth exchange", "error", err)
		http.Redirect(w, r, s.cfg.BaseURL+"/settings?google=error", http.StatusFound)
		return
	}
	http.Redirect(w, r, s.cfg.BaseURL+"/settings?google=connected", http.StatusFound)
}

func (s *Server) handleGoogleDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteGoogleSheetsTokens(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not disconnect Google Sheets")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"connected": false})
}

func (s *Server) handlePlannedActivities(w http.ResponseWriter, r *http.Request) {
	from := time.Now().UTC().AddDate(0, 0, -30)
	to := time.Now().UTC().AddDate(0, 0, 180)
	if raw := r.URL.Query().Get("from"); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from date")
			return
		}
		from = parsed
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to date")
			return
		}
		to = parsed.AddDate(0, 0, 1)
	}
	items, err := s.store.ListPlannedActivities(r.Context(), from, to)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			items = []PlannedActivity{}
		} else {
			writeError(w, http.StatusInternalServerError, "could not load planned activities")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"planned": items})
}

func (s *Server) handlePlannedMatchCandidates(w http.ResponseWriter, r *http.Request) {
	response, err := s.store.PlannedActivityMatchCandidates(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "activity not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load planned activity matches")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type plannedActivityMatchRequest struct {
	PlannedActivityID string `json:"plannedActivityId"`
}

func (s *Server) handleMatchPlannedActivity(w http.ResponseWriter, r *http.Request) {
	var body plannedActivityMatchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.PlannedActivityID) == "" {
		writeError(w, http.StatusBadRequest, "plannedActivityId is required")
		return
	}
	planned, err := s.store.MatchPlannedActivity(r.Context(), chi.URLParam(r, "id"), strings.TrimSpace(body.PlannedActivityID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "activity or planned activity not found")
		return
	}
	if errors.Is(err, errPlannedMatchConflict) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, errPlannedMatchInvalid) || errors.Is(err, errPlannedMatchDateMismatch) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not match planned activity")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"planned": planned})
}

func (s *Server) handleUnmatchPlannedActivity(w http.ResponseWriter, r *http.Request) {
	if err := s.store.UnmatchPlannedActivity(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, "could not unmatch planned activity")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"matched": false})
}

type trainingSheetConfigUpdateRequest struct {
	Enabled         *bool   `json:"enabled"`
	SheetURL        *string `json:"sheetURL"`
	CheckEveryHours *int    `json:"checkEveryHours"`
	PlanYear        *int    `json:"planYear"`
	RestoreDefaults *bool   `json:"restoreDefaults"`
}

func (s *Server) handleTrainingSheetConfig(w http.ResponseWriter, r *http.Request) {
	config, err := s.store.GetTrainingSheetConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load training sheet config")
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) handleUpdateTrainingSheetConfig(w http.ResponseWriter, r *http.Request) {
	var body trainingSheetConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	current, err := s.store.GetTrainingSheetConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load training sheet config")
		return
	}
	next := current
	if body.RestoreDefaults != nil && *body.RestoreDefaults {
		next = TrainingSheetConfig{CheckEveryHours: defaultTrainingSheetCheckEveryHours, PlanYear: time.Now().UTC().Year()}
	}
	if body.Enabled != nil {
		next.Enabled = *body.Enabled
	}
	if body.SheetURL != nil {
		next.SheetURL = strings.TrimSpace(*body.SheetURL)
	}
	if body.CheckEveryHours != nil {
		next.CheckEveryHours = *body.CheckEveryHours
	}
	if body.PlanYear != nil {
		next.PlanYear = *body.PlanYear
	}
	if next.Enabled && next.SheetURL == "" {
		writeError(w, http.StatusBadRequest, "sheetURL must be set when training sheet sync is enabled")
		return
	}
	if next.CheckEveryHours < 1 || next.CheckEveryHours > 720 {
		writeError(w, http.StatusBadRequest, "checkEveryHours must be between 1 and 720")
		return
	}
	if next.PlanYear < 1900 || next.PlanYear > 9999 {
		writeError(w, http.StatusBadRequest, "planYear must be between 1900 and 9999")
		return
	}
	if err := s.store.SetTrainingSheetConfig(r.Context(), next); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save training sheet config")
		return
	}
	writeJSON(w, http.StatusOK, next)
}

func (s *Server) handleTrainingSheetSync(w http.ResponseWriter, r *http.Request) {
	config, err := s.store.GetTrainingSheetConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load training sheet config")
		return
	}
	if !config.Enabled {
		writeError(w, http.StatusBadRequest, "training sheet sync is disabled")
		return
	}
	if strings.TrimSpace(config.SheetURL) == "" {
		writeError(w, http.StatusBadRequest, "sheet URL is not configured")
		return
	}
	status, err := s.googleAuth().Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check Google Sheets status")
		return
	}
	if !status.Connected {
		writeError(w, http.StatusBadRequest, "Google Sheets is not connected")
		return
	}
	running, err := s.store.HasRunningSyncJob(r.Context(), trainingSheetProvider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check sync state")
		return
	}
	if running {
		writeError(w, http.StatusConflict, "training sheet sync is already running")
		return
	}
	jobID, err := s.store.CreateSyncJob(r.Context(), trainingSheetProvider, "manual")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create sync job")
		return
	}
	go s.runTrainingSheetManualSyncJob(jobID, config)
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": jobID, "status": "running"})
}

func (s *Server) runTrainingSheetScheduledSync(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTrainingSheetScheduledSyncOnce(ctx)
		}
	}
}

func (s *Server) runTrainingSheetScheduledSyncOnce(ctx context.Context) {
	config, err := s.store.GetTrainingSheetConfig(ctx)
	if err != nil || !config.Enabled || strings.TrimSpace(config.SheetURL) == "" {
		return
	}
	status, err := s.googleAuth().Status(ctx)
	if err != nil || !status.Connected {
		return
	}
	lastCreated, err := s.store.LatestTrainingSheetScheduledSync(ctx)
	if err != nil || (!lastCreated.IsZero() && time.Since(lastCreated) < time.Duration(config.CheckEveryHours)*time.Hour) {
		return
	}
	running, err := s.store.HasRunningSyncJob(ctx, trainingSheetProvider)
	if err != nil || running {
		return
	}
	jobID, err := s.store.CreateSyncJob(ctx, trainingSheetProvider, "scheduled")
	if err != nil {
		s.logger.Error("create scheduled training sheet sync job", "error", err)
		return
	}
	syncCtx, cancel := context.WithTimeout(ctx, 90*time.Minute)
	defer cancel()
	if _, err := s.finishTrainingSheetSyncJob(syncCtx, jobID, config); err != nil {
		s.logger.Error("scheduled training sheet sync", "error", err)
	}
}

func (s *Server) runTrainingSheetManualSyncJob(jobID string, config TrainingSheetConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	if _, err := s.finishTrainingSheetSyncJob(ctx, jobID, config); err != nil {
		s.logger.Error("manual training sheet sync", "job_id", jobID, "error", err)
	}
}

func (s *Server) finishTrainingSheetSyncJob(ctx context.Context, jobID string, config TrainingSheetConfig) (map[string]any, error) {
	progress := func(payload map[string]any) { _ = s.store.UpdateSyncJobProgress(ctx, jobID, payload) }
	payload, err := NewPlannedTrainingSheetService(s.store, s.logger).Sync(ctx, config, progress)
	if err != nil {
		_ = s.store.FinishSyncJob(ctx, jobID, "failed", err.Error(), payload)
		return payload, err
	}
	if err := s.store.TouchTrainingSheetConfigLastSyncedAt(ctx, time.Now().UTC()); err != nil {
		_ = s.store.FinishSyncJob(ctx, jobID, "failed", err.Error(), payload)
		return payload, err
	}
	_ = s.store.FinishSyncJob(ctx, jobID, "completed", "", payload)
	return payload, nil
}
