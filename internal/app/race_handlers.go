package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (s *Server) raceRoutes(r chi.Router) {
	r.Get("/races", s.handleListRaces)
	r.Post("/races", s.handleSaveRace)
	r.Get("/races/performance", s.handleRacePerformance)
	r.Get("/races/settings", s.handleRaceSettings)
	r.Put("/races/settings", s.handleSaveRaceSettings)
	r.Get("/races/review", s.handleRaceDiscovery)
	r.Post("/races/review/scan", s.handleRaceScan)
	r.Post("/races/review/{id}/dismiss", s.handleDismissRace)
	r.Post("/races/predictions/sync", s.handleRacePredictionSync)
	r.Get("/race-groups", s.handleRaceResources)
	r.Post("/race-groups", s.handleSaveRaceResource)
	r.Put("/race-groups/{id}", s.handleSaveRaceResource)
	r.Delete("/race-groups/{id}", s.handleDeleteRaceResource)
	r.Get("/race-checklists", s.handleRaceResources)
	r.Post("/race-checklists", s.handleSaveRaceResource)
	r.Put("/race-checklists/{id}", s.handleSaveRaceResource)
	r.Delete("/race-checklists/{id}", s.handleDeleteRaceResource)
	r.Get("/races/{id}", s.handleGetRace)
	r.Put("/races/{id}", s.handleSaveRace)
	r.Delete("/races/{id}", s.handleDeleteRace)
	r.Get("/races/{id}/activity-candidates", s.handleRaceCandidates)
	r.Post("/races/{id}/forecast", s.handleRaceForecast)
	r.Get("/races/{id}/report", s.handleRaceReport)
	r.Put("/races/{id}/report", s.handleSaveRaceReport)
	r.Post("/races/{id}/report/preview", s.handlePreviewRaceReport)
}
func (s *Server) writeRaceError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	err = raceStoreError(err)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, 404, "race or linked object not found")
	case errors.Is(err, ErrRaceConflict), errors.Is(err, ErrSyncJobAlreadyRunning):
		writeError(w, 409, err.Error())
	case errors.Is(err, ErrRaceInvalid):
		writeError(w, 400, err.Error())
	default:
		s.logger.Error("race request failed", "error", err)
		writeError(w, 500, "could not complete race request")
	}
	return true
}
func raceOptions(r *http.Request) raceListOptions {
	q := r.URL.Query()
	return raceListOptions{View: q.Get("view"), Query: q.Get("q"), Discipline: q.Get("discipline"), Kind: q.Get("kind"), Status: q.Get("status"), GroupID: q.Get("groupId"), From: q.Get("from"), To: q.Get("to"), Sort: q.Get("sort"), Order: q.Get("order"), Limit: queryInteger(r, "limit"), Offset: queryInteger(r, "offset")}
}
func (s *Server) handleListRaces(w http.ResponseWriter, r *http.Request) {
	page, err := s.store.ListRaces(r.Context(), raceOptions(r))
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, page)
}
func (s *Server) handleSaveRace(w http.ResponseWriter, r *http.Request) {
	var input RaceInput
	if err := decodeJSONBody(r, &input); err != nil {
		writeError(w, 400, "invalid race JSON")
		return
	}
	id := chi.URLParam(r, "id")
	race, err := s.store.SaveRace(r.Context(), id, input)
	if s.writeRaceError(w, err) {
		return
	}
	// Capture the current reference while it can still be evidence of a pre-race prediction.
	if cutoff, cutoffErr := raceCutoff(race); cutoffErr == nil && time.Now().Before(cutoff) {
		if err = s.captureCurrentRacePrediction(r.Context(), race); err != nil {
			s.logger.Error("capture race prediction", "error", err)
		}
	}
	status := 200
	if id == "" {
		status = 201
	}
	writeJSON(w, status, race)
}
func (s *Server) captureCurrentRacePrediction(ctx context.Context, r Race) error {
	settings, err := s.store.RaceSettings(ctx)
	if err != nil {
		return err
	}
	races, err := s.store.AllRaces(ctx, raceListOptions{})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	predictions, err := s.store.RacePredictions(ctx, now.AddDate(-1, 0, 0).Format("2006-01-02"), now.Format("2006-01-02"))
	if err != nil {
		return err
	}
	return s.store.captureRacePrediction(ctx, r, buildRacePerformance(races, settings, now), predictions, now)
}

type RaceDetail struct {
	Race        Race                       `json:"race"`
	Activity    *Activity                  `json:"activity,omitempty"`
	Metrics     RaceMetrics                `json:"metrics"`
	Halfway     RaceHalfway                `json:"halfway"`
	BuildUp     []RaceBuildWeek            `json:"buildUp"`
	Comparisons []RacePredictionComparison `json:"comparisons"`
	Forecast    RaceForecast               `json:"forecast"`
	PlanWarning string                     `json:"planWarning,omitempty"`
}

func (s *Store) RaceDetail(ctx context.Context, id string) (RaceDetail, error) {
	result := RaceDetail{}
	r, err := s.GetRace(ctx, id)
	if err != nil {
		return result, err
	}
	result.Race = r
	settings, err := s.RaceSettings(ctx)
	if err != nil {
		return result, err
	}
	result.Metrics = raceMetrics(r, settings)
	if r.ActivityID != "" {
		a, err := s.GetActivity(ctx, r.ActivityID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return result, err
		}
		if err == nil {
			result.Activity = &a
		}
	}
	result.Halfway = raceHalfway(r, result.Activity)
	if result.Activity != nil {
		result.Activity.Samples = nil
		result.Activity.Intervals = nil
		result.Activity.Climbs = nil
		result.Activity.Workout = nil
	}
	result.BuildUp, err = s.RaceBuildUp(ctx, r)
	if err != nil {
		return result, err
	}
	result.Comparisons, err = s.RacePredictionComparisons(ctx, r)
	if err != nil {
		return result, err
	}
	result.Forecast, err = s.RaceForecast(ctx, r)
	if err != nil {
		return result, err
	}
	if !settings.WeatherEnabled {
		result.Forecast.Reason = "Race weather is disabled"
	} else if r.Date == "" {
		result.Forecast.Reason = "Set a race date for a forecast"
	} else if r.Latitude == nil {
		result.Forecast.Reason = "Choose a forecast location"
	} else if len(result.Forecast.Hours) == 0 {
		result.Forecast.Reason = "No saved forecast; refresh when race day is within 16 days"
	}
	if r.PlannedActivityID != "" {
		var date, activity string
		err = s.db.QueryRow(ctx, `select planned_date::date::text,coalesce(matched_activity_id::text,'') from planned_activities where id=$1 and user_id=$2`, r.PlannedActivityID, scopedUserID(ctx)).Scan(&date, &activity)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return result, err
		}
		if err == nil && (date != r.Date || activity != "" && activity != r.ActivityID) {
			result.PlanWarning = "The linked training row has a different date or activity. Its existing match and sheet writeback are unchanged."
		}
	}
	return result, nil
}
func (s *Server) handleGetRace(w http.ResponseWriter, r *http.Request) {
	detail, err := s.store.RaceDetail(r.Context(), chi.URLParam(r, "id"))
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, detail)
}
func (s *Server) handleDeleteRace(w http.ResponseWriter, r *http.Request) {
	revision := queryInteger(r, "revision")
	if revision <= 0 {
		writeError(w, 400, "revision is required")
		return
	}
	if s.writeRaceError(w, s.store.DeleteRace(r.Context(), chi.URLParam(r, "id"), revision)) {
		return
	}
	writeJSON(w, 200, map[string]bool{"deleted": true})
}
func (s *Server) handleRaceSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.RaceSettings(r.Context())
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, settings)
}
func (s *Server) handleSaveRaceSettings(w http.ResponseWriter, r *http.Request) {
	var input RaceSettings
	if err := decodeJSONBody(r, &input); err != nil {
		writeError(w, 400, "invalid race settings")
		return
	}
	settings, err := s.store.SaveRaceSettings(r.Context(), input)
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, settings)
}
func (s *Server) handleRacePerformance(w http.ResponseWriter, r *http.Request) {
	options := raceOptions(r)
	options.View = "history"
	races, err := s.store.AllRaces(r.Context(), options)
	if s.writeRaceError(w, err) {
		return
	}
	settings, err := s.store.RaceSettings(r.Context())
	if s.writeRaceError(w, err) {
		return
	}
	now := time.Now().UTC()
	result := buildRacePerformance(races, settings, now)
	// Filters change comparisons, not the user's selected current VDOT reference.
	all, err := s.store.AllRaces(r.Context(), raceListOptions{View: "history"})
	if s.writeRaceError(w, err) {
		return
	}
	reference := buildRacePerformance(all, settings, now)
	result.Reference, result.ReferenceReason = reference.Reference, reference.ReferenceReason
	from, to := options.From, options.To
	if from == "" {
		from = "1900-01-01"
	}
	if to == "" {
		to = now.Format("2006-01-02")
	}
	result.Predictions, err = s.store.RacePredictions(r.Context(), from, to)
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, result)
}
func raceResourceKind(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/api/race-checklists") {
		return "templates"
	}
	return "groups"
}
func (s *Server) handleRaceResources(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.RaceResources(r.Context(), raceResourceKind(r))
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) handleSaveRaceResource(w http.ResponseWriter, r *http.Request) {
	var input RaceResource
	if err := decodeJSONBody(r, &input); err != nil {
		writeError(w, 400, "invalid race resource")
		return
	}
	saved, err := s.store.SaveRaceResource(r.Context(), raceResourceKind(r), chi.URLParam(r, "id"), input)
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, saved)
}
func (s *Server) handleDeleteRaceResource(w http.ResponseWriter, r *http.Request) {
	if s.writeRaceError(w, s.store.DeleteRaceResource(r.Context(), raceResourceKind(r), chi.URLParam(r, "id"), queryInteger(r, "revision"))) {
		return
	}
	writeJSON(w, 200, map[string]bool{"deleted": true})
}
func (s *Server) handleRaceDiscovery(w http.ResponseWriter, r *http.Request) {
	items, more, err := s.store.RaceDiscovery(r.Context(), queryInteger(r, "offset"))
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, map[string]any{"activities": items, "hasMore": more})
}
func (s *Server) handleRaceScan(w http.ResponseWriter, r *http.Request) {
	id, err := s.store.CreateSyncJob(r.Context(), "races", "discovery")
	if s.writeRaceError(w, err) {
		return
	}
	go s.runRaceScanJob(id)
	writeJSON(w, 202, map[string]any{"jobId": id, "status": "running"})
}
func (s *Server) handleDismissRace(w http.ResponseWriter, r *http.Request) {
	if s.writeRaceError(w, s.store.DismissRaceDiscovery(r.Context(), chi.URLParam(r, "id"))) {
		return
	}
	writeJSON(w, 200, map[string]bool{"dismissed": true})
}
func (s *Server) handleRaceCandidates(w http.ResponseWriter, r *http.Request) {
	race, err := s.store.GetRace(r.Context(), chi.URLParam(r, "id"))
	if s.writeRaceError(w, err) {
		return
	}
	items, err := s.store.RaceActivityCandidates(r.Context(), race, r.URL.Query().Get("q"))
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, map[string]any{"activities": items})
}
func (s *Server) handleRacePredictionSync(w http.ResponseWriter, r *http.Request) {
	id, err := s.queueRacePredictionSync(r.Context())
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 202, map[string]any{"jobId": id, "status": "running"})
}
func (s *Server) handleRaceForecast(w http.ResponseWriter, r *http.Request) {
	race, err := s.store.GetRace(r.Context(), chi.URLParam(r, "id"))
	if s.writeRaceError(w, err) {
		return
	}
	forecast, err := s.refreshRaceForecast(r.Context(), race)
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, forecast)
}
