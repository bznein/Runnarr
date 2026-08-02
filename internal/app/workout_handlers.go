package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type workoutMutationRequest struct {
	Name                    *string            `json:"name"`
	SourceText              *string            `json:"sourceText"`
	Definition              *WorkoutDefinition `json:"definition"`
	ScheduledDate           *string            `json:"scheduledDate"`
	PaceToleranceSeconds    *int               `json:"paceToleranceSeconds"`
	UseDefaultPaceTolerance bool               `json:"useDefaultPaceTolerance"`
	GarminExcluded          *bool              `json:"garminExcluded"`
}

func (s *Server) handleWorkoutConfig(w http.ResponseWriter, r *http.Request) {
	config, err := s.store.GetWorkoutConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load workout settings")
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) handleUpdateWorkoutConfig(w http.ResponseWriter, r *http.Request) {
	current, err := s.store.GetWorkoutConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load workout settings")
		return
	}
	var body struct {
		SyncEnabled                 *bool   `json:"syncEnabled"`
		DefaultPaceToleranceSeconds *int    `json:"defaultPaceToleranceSeconds"`
		Timezone                    *string `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.SyncEnabled != nil {
		current.SyncEnabled = *body.SyncEnabled
	}
	if body.DefaultPaceToleranceSeconds != nil {
		current.DefaultPaceToleranceSeconds = *body.DefaultPaceToleranceSeconds
	}
	if body.Timezone != nil {
		current.Timezone = *body.Timezone
	}
	if current.SyncEnabled {
		if _, connected, statusErr := s.garmin.Status(r.Context()); statusErr != nil {
			writeError(w, http.StatusInternalServerError, "could not check Garmin connection")
			return
		} else if !connected {
			writeError(w, http.StatusBadRequest, "connect Garmin before enabling workout sync")
			return
		}
	}
	if err := s.store.SetWorkoutConfig(r.Context(), current); err != nil {
		if errors.Is(err, ErrWorkoutInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "could not save workout settings")
		return
	}
	if current.SyncEnabled {
		if _, err := s.queueGarminWorkoutReconcile(r.Context()); err != nil && !errors.Is(err, ErrSyncJobAlreadyRunning) {
			s.logger.Error("queue Garmin workout sync after config update", "error", err)
		}
	}
	writeJSON(w, http.StatusOK, current)
}

func (s *Server) handlePreviewWorkoutReconcile(w http.ResponseWriter, r *http.Request) {
	result, err := s.garmin.PreviewWorkoutReconcile(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not preview Garmin workout sync")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleWorkoutReconcile(w http.ResponseWriter, r *http.Request) {
	config, err := s.store.GetWorkoutConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load workout settings")
		return
	}
	if !config.SyncEnabled {
		writeError(w, http.StatusBadRequest, "Garmin workout sync is disabled")
		return
	}
	if _, connected, err := s.garmin.Status(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check Garmin connection")
		return
	} else if !connected {
		writeError(w, http.StatusBadRequest, "Garmin is not connected")
		return
	}
	jobID, err := s.queueGarminWorkoutReconcile(r.Context())
	if errors.Is(err, ErrSyncJobAlreadyRunning) {
		writeError(w, http.StatusConflict, "Garmin sync is already running")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start Garmin workout sync")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": jobID, "status": "running"})
}

func (s *Server) handleListWorkouts(w http.ResponseWriter, r *http.Request) {
	workouts, err := s.store.ListWorkouts(r.Context(), r.URL.Query().Get("filter"))
	if errors.Is(err, ErrWorkoutInvalid) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load workouts")
		return
	}
	writeJSON(w, http.StatusOK, WorkoutList{Workouts: workouts})
}

func (s *Server) handleGetWorkout(w http.ResponseWriter, r *http.Request) {
	workout, err := s.store.GetWorkout(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "workout not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load workout")
		return
	}
	writeJSON(w, http.StatusOK, workout)
}

func (s *Server) handleParseWorkout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SourceText string `json:"sourceText"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	writeJSON(w, http.StatusOK, parseWorkoutPrescription(body.SourceText, nil))
}

func (s *Server) handleCreateWorkout(w http.ResponseWriter, r *http.Request) {
	var body workoutMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	workout, err := workoutFromMutation(Workout{Source: workoutSourceManual, SportType: "Run"}, body, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.store.CreateManualWorkout(r.Context(), workout)
	if err != nil {
		if errors.Is(err, ErrWorkoutInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create workout")
		return
	}
	s.queueGarminWorkoutReconcileAfterMutation(r.Context(), "create")
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateWorkout(w http.ResponseWriter, r *http.Request) {
	current, err := s.store.GetWorkout(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "workout not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load workout")
		return
	}
	var body workoutMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if current.Source == workoutSourceTrainingSheet {
		if body.Name != nil || body.SourceText != nil || body.Definition != nil || body.ScheduledDate != nil {
			writeError(w, http.StatusConflict, ErrWorkoutReadOnly.Error())
			return
		}
		pace := current.PaceToleranceSeconds
		if body.UseDefaultPaceTolerance {
			pace = nil
		} else if body.PaceToleranceSeconds != nil {
			pace = body.PaceToleranceSeconds
		}
		excluded := current.GarminExcluded
		if body.GarminExcluded != nil {
			excluded = *body.GarminExcluded
		}
		updated, err := s.store.SaveWorkoutOperations(r.Context(), current.ID, pace, excluded)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not update workout")
			return
		}
		s.queueGarminWorkoutReconcileAfterMutation(r.Context(), "update")
		writeJSON(w, http.StatusOK, updated)
		return
	}
	updated, err := workoutFromMutation(current, body, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err = s.store.SaveManualWorkout(r.Context(), updated)
	if errors.Is(err, ErrWorkoutPlanCompleted) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update workout")
		return
	}
	s.queueGarminWorkoutReconcileAfterMutation(r.Context(), "update")
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDuplicateWorkout(w http.ResponseWriter, r *http.Request) {
	workout, err := s.store.DuplicateWorkout(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "workout not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not duplicate workout")
		return
	}
	s.queueGarminWorkoutReconcileAfterMutation(r.Context(), "duplicate")
	writeJSON(w, http.StatusCreated, workout)
}

func (s *Server) handleDeleteWorkout(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.store.DeleteWorkout(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, ErrWorkoutReadOnly) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "workout not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete workout")
		return
	}
	s.queueGarminWorkoutReconcileAfterMutation(r.Context(), "delete")
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": deleted, "archived": !deleted})
}

func (s *Server) queueGarminWorkoutReconcileAfterMutation(ctx context.Context, mutation string) {
	if _, err := s.queueGarminWorkoutReconcile(ctx); err != nil && !errors.Is(err, ErrSyncJobAlreadyRunning) {
		s.logger.Error("queue Garmin workout sync after mutation", "mutation", mutation, "error", err)
	}
}

func workoutFromMutation(current Workout, body workoutMutationRequest, creating bool) (Workout, error) {
	if body.Name != nil {
		current.Name = strings.TrimSpace(*body.Name)
	}
	if body.ScheduledDate != nil {
		current.ScheduledDate = strings.TrimSpace(*body.ScheduledDate)
	}
	if err := ensureWorkoutDate(current.ScheduledDate); err != nil {
		return Workout{}, err
	}
	if body.UseDefaultPaceTolerance {
		current.PaceToleranceSeconds = nil
	} else if body.PaceToleranceSeconds != nil {
		current.PaceToleranceSeconds = body.PaceToleranceSeconds
	}
	if body.GarminExcluded != nil {
		current.GarminExcluded = *body.GarminExcluded
	}
	if body.SourceText != nil && body.Definition != nil {
		return Workout{}, errors.New("provide sourceText or definition, not both")
	}
	if body.SourceText != nil {
		parsed := parseWorkoutPrescription(*body.SourceText, nil)
		current.SourceText = strings.TrimSpace(*body.SourceText)
		current.SourceHash = workoutSourceHash(current.SourceText, nil)
		current.Definition = parsed.Definition
		current.ParseStatus = parsed.Status
		current.ParseMessages = parsed.Messages
	}
	if body.Definition != nil {
		current.SourceText = ""
		current.SourceHash = ""
		current.Definition = *body.Definition
		numberWorkoutSteps(current.Definition.Steps)
		current.Definition.Version = 1
		current.Definition.SportType = "Run"
		current.Definition.EstimatedDurationS = estimateWorkoutDuration(current.Definition.Steps)
		current.ParseStatus = workoutParseReady
		current.ParseMessages = []WorkoutParseMessage{}
	}
	if creating && body.SourceText == nil && body.Definition == nil {
		return Workout{}, errors.New("sourceText or definition is required")
	}
	if current.ParseStatus == "" {
		current.ParseStatus = workoutParseReady
	}
	if strings.TrimSpace(current.Name) == "" {
		current.Name = "Running workout"
	}
	return current, nil
}
