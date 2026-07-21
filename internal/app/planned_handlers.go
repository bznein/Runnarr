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
	sessionID, userID, err := s.store.ConsumeGoogleOAuthState(r.Context(), state)
	if err != nil || sessionID != sessionCookie.Value {
		writeError(w, http.StatusBadRequest, "invalid Google OAuth state")
		return
	}
	if err := s.googleAuth().Exchange(withUserID(r.Context(), userID), code); err != nil {
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
	windowDays := 7
	switch r.URL.Query().Get("windowDays") {
	case "", "7":
		windowDays = 7
	case "30":
		windowDays = 30
	default:
		writeError(w, http.StatusBadRequest, "windowDays must be 7 or 30")
		return
	}
	response, err := s.store.PlannedActivityMatchCandidates(r.Context(), chi.URLParam(r, "id"), windowDays)
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
	jobID, enqueueErr := s.store.CreateTrainingSheetWritebackJob(r.Context(), planned.ID, chi.URLParam(r, "id"))
	if enqueueErr != nil {
		s.logger.Error("queue training sheet writeback", "activity_id", chi.URLParam(r, "id"), "planned_activity_id", planned.ID, "error", enqueueErr)
	} else {
		go s.runTrainingSheetWritebackJob(jobID, planned.ID, chi.URLParam(r, "id"))
	}
	writeJSON(w, http.StatusOK, map[string]any{"planned": planned, "writebackJobId": jobID})
}

func (s *Server) handleTrainingSheetWriteback(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")
	planned, err := s.store.GetMatchedPlannedActivity(r.Context(), activityID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "activity is not matched to a planned activity")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load matched planned activity")
		return
	}
	if running, err := s.store.HasRunningSyncJob(r.Context(), trainingSheetProvider); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check write-back state")
		return
	} else if running {
		writeError(w, http.StatusConflict, "a training sheet job is already running")
		return
	}
	jobID, err := s.store.CreateTrainingSheetWritebackJob(r.Context(), planned.ID, activityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create write-back job")
		return
	}
	go s.runTrainingSheetWritebackJob(jobID, planned.ID, activityID)
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": jobID, "status": "running"})
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
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		s.logger.Error("list users for scheduled training sheet sync", "error", err)
		return
	}
	for _, user := range users {
		if user.Disabled {
			continue
		}
		userCtx := withUserID(ctx, user.ID)
		config, err := s.store.GetTrainingSheetConfig(userCtx)
		if err != nil || !config.Enabled || strings.TrimSpace(config.SheetURL) == "" {
			continue
		}
		status, err := s.googleAuth().Status(userCtx)
		if err != nil || !status.Connected {
			continue
		}
		lastCreated, err := s.store.LatestTrainingSheetScheduledSync(userCtx)
		if err != nil || (!lastCreated.IsZero() && time.Since(lastCreated) < time.Duration(config.CheckEveryHours)*time.Hour) {
			continue
		}
		running, err := s.store.HasRunningSyncJob(userCtx, trainingSheetProvider)
		if err != nil || running {
			continue
		}
		jobID, err := s.store.CreateSyncJob(userCtx, trainingSheetProvider, "scheduled")
		if err != nil {
			s.logger.Error("create scheduled training sheet sync job", "user_id", user.ID, "error", err)
			continue
		}
		go s.runTrainingSheetScheduledSyncJob(ctx, user.ID, jobID, config)
	}
}

func (s *Server) runTrainingSheetScheduledSyncJob(parent context.Context, userID, jobID string, config TrainingSheetConfig) {
	userCtx := withUserID(parent, userID)
	syncCtx, cancel := context.WithTimeout(userCtx, 90*time.Minute)
	defer cancel()
	if _, err := s.finishTrainingSheetSyncJob(syncCtx, jobID, config); err != nil {
		s.logger.Error("scheduled training sheet sync", "user_id", userID, "error", err)
	}
}

func (s *Server) runTrainingSheetManualSyncJob(jobID string, config TrainingSheetConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	userID, err := s.store.SyncJobUserID(ctx, jobID)
	if err != nil {
		s.logger.Error("load training sheet sync owner", "job_id", jobID, "error", err)
		return
	}
	ctx = withUserID(ctx, userID)
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

func (s *Server) runTrainingSheetWritebackJob(jobID, plannedID, activityID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	userID, err := s.store.SyncJobUserID(ctx, jobID)
	if err != nil {
		s.logger.Error("load training sheet writeback owner", "job_id", jobID, "error", err)
		return
	}
	ctx = withUserID(ctx, userID)
	payload, err := NewTrainingSheetWritebackService(s.store, s.googleAuth()).Write(ctx, plannedID, activityID)
	if payload != nil {
		_ = s.store.UpdateSyncJobProgress(ctx, jobID, payload)
	}
	if err != nil {
		_ = s.store.FinishSyncJob(ctx, jobID, "failed", err.Error(), payload)
		return
	}
	_ = s.store.FinishSyncJob(ctx, jobID, "completed", "", payload)
}
