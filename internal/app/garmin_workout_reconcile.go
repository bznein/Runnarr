package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const garminWorkoutHorizonDays = 7

type garminManagedTemplate struct {
	ID                string
	DefinitionHash    string
	ProviderWorkoutID string
	Name              string
	OwnershipMarker   string
	Payload           map[string]any
	Remote            map[string]any
	Status            string
	Error             string
}

type garminManagedSchedule struct {
	ID                 string
	WorkoutID          string
	TemplateID         string
	WorkoutRevision    int
	ScheduledDate      string
	ProviderScheduleID string
	DesiredState       string
	Status             string
	Error              string
	Remote             map[string]any
}

type WorkoutReconcileAction struct {
	WorkoutID string `json:"workoutId,omitempty"`
	Action    string `json:"action"`
	Date      string `json:"date,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

type WorkoutReconcileResult struct {
	Enabled bool                     `json:"enabled"`
	From    string                   `json:"from,omitempty"`
	To      string                   `json:"to,omitempty"`
	Actions []WorkoutReconcileAction `json:"actions"`
}

func (s *Store) ListWorkoutReconcileCandidates(ctx context.Context, from, to string) ([]Workout, error) {
	rows, err := s.db.Query(ctx, `
		select workouts.id::text
		from workouts
		where workouts.user_id = $1 and (
			(workouts.archived_at is null and workouts.scheduled_date between $2::date and $3::date)
			or exists (
				select 1 from garmin_workout_schedules
				where user_id = workouts.user_id and workout_id = workouts.id
					and status in ('pending', 'scheduled', 'removing', 'error')
			)
		)
		order by workouts.scheduled_date nulls last, workouts.created_at
	`, scopedUserID(ctx), from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	workouts := make([]Workout, 0, len(ids))
	for _, id := range ids {
		workout, err := s.GetWorkout(ctx, id)
		if err != nil {
			return nil, err
		}
		workouts = append(workouts, workout)
	}
	return workouts, nil
}

func (s *Store) GetGarminWorkoutTemplateByHash(ctx context.Context, hash string) (garminManagedTemplate, error) {
	return scanGarminManagedTemplate(s.db.QueryRow(ctx, `
		select id::text, definition_hash, provider_workout_id, name, ownership_marker,
			payload, remote, status, error
		from garmin_workout_templates
		where user_id = $1 and definition_hash = $2 and deleted_at is null
	`, scopedUserID(ctx), hash))
}

func (s *Store) GetGarminWorkoutTemplate(ctx context.Context, id string) (garminManagedTemplate, error) {
	return scanGarminManagedTemplate(s.db.QueryRow(ctx, `
		select id::text, definition_hash, provider_workout_id, name, ownership_marker,
			payload, remote, status, error
		from garmin_workout_templates
		where user_id = $1 and id = $2 and deleted_at is null
	`, scopedUserID(ctx), id))
}

func scanGarminManagedTemplate(row pgx.Row) (garminManagedTemplate, error) {
	var template garminManagedTemplate
	var payload, remote []byte
	err := row.Scan(&template.ID, &template.DefinitionHash, &template.ProviderWorkoutID,
		&template.Name, &template.OwnershipMarker, &payload, &remote, &template.Status, &template.Error)
	if err != nil {
		return template, err
	}
	_ = json.Unmarshal(payload, &template.Payload)
	_ = json.Unmarshal(remote, &template.Remote)
	return template, nil
}

func (s *Store) CreateGarminWorkoutTemplate(ctx context.Context, compiled compiledGarminWorkout) (garminManagedTemplate, error) {
	payload, err := json.Marshal(compiled.Payload)
	if err != nil {
		return garminManagedTemplate{}, err
	}
	var id string
	err = s.db.QueryRow(ctx, `
		insert into garmin_workout_templates(
			user_id, definition_hash, name, ownership_marker, payload, status
		) values($1, $2, $3, $4, $5, 'pending')
		on conflict(user_id, definition_hash) where deleted_at is null
		do update set payload = excluded.payload, name = excluded.name, updated_at = now()
		returning id::text
	`, scopedUserID(ctx), compiled.DefinitionHash, compiled.Name, compiled.OwnershipMarker, payload).Scan(&id)
	if err != nil {
		return garminManagedTemplate{}, err
	}
	return s.GetGarminWorkoutTemplate(ctx, id)
}

func (s *Store) RecordGarminWorkoutTemplateProviderID(ctx context.Context, id, providerID string, remote map[string]any) error {
	remoteBytes, err := json.Marshal(remote)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		update garmin_workout_templates
		set provider_workout_id = $3, remote = $4, updated_at = now()
		where id = $1 and user_id = $2 and deleted_at is null
	`, id, scopedUserID(ctx), providerID, remoteBytes)
	return err
}

func (s *Store) MarkGarminWorkoutTemplateUploaded(ctx context.Context, id string, remote map[string]any) error {
	remoteBytes, err := json.Marshal(remote)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		update garmin_workout_templates
		set remote = $3, status = 'uploaded', error = '', uploaded_at = coalesce(uploaded_at, now()),
			last_seen_at = now(), updated_at = now()
		where id = $1 and user_id = $2 and deleted_at is null
	`, id, scopedUserID(ctx), remoteBytes)
	return err
}

func (s *Store) MarkGarminWorkoutTemplateError(ctx context.Context, id, message string) error {
	_, err := s.db.Exec(ctx, `
		update garmin_workout_templates set status = 'error', error = $3, updated_at = now()
		where id = $1 and user_id = $2 and deleted_at is null
	`, id, scopedUserID(ctx), message)
	return err
}

func (s *Store) MarkGarminWorkoutTemplateDeleted(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `
		update garmin_workout_templates
		set status = 'deleted', error = '', deleted_at = now(), updated_at = now()
		where id = $1 and user_id = $2 and deleted_at is null
	`, id, scopedUserID(ctx))
	return err
}

func (s *Store) GetActiveGarminWorkoutSchedule(ctx context.Context, workoutID string) (garminManagedSchedule, error) {
	var schedule garminManagedSchedule
	var remote []byte
	err := s.db.QueryRow(ctx, `
		select id::text, workout_id::text, coalesce(template_id::text, ''), workout_revision,
			scheduled_date::text, provider_schedule_id, desired_state, status, error, remote
		from garmin_workout_schedules
		where user_id = $1 and workout_id = $2
			and status in ('pending', 'scheduled', 'removing', 'error')
		order by created_at desc limit 1
	`, scopedUserID(ctx), workoutID).Scan(&schedule.ID, &schedule.WorkoutID, &schedule.TemplateID,
		&schedule.WorkoutRevision, &schedule.ScheduledDate, &schedule.ProviderScheduleID,
		&schedule.DesiredState, &schedule.Status, &schedule.Error, &remote)
	if err != nil {
		return schedule, err
	}
	_ = json.Unmarshal(remote, &schedule.Remote)
	return schedule, nil
}

func (s *Store) UpsertPendingGarminWorkoutSchedule(ctx context.Context, workout Workout, templateID string) (garminManagedSchedule, error) {
	var id string
	command, err := s.db.Exec(ctx, `
		update garmin_workout_schedules
		set template_id = $3, workout_revision = $4, scheduled_date = $5::date,
			desired_state = 'scheduled', status = 'pending', error = '',
			last_attempt_at = now(), updated_at = now()
		where user_id = $1 and workout_id = $2
			and status in ('pending', 'scheduled', 'removing', 'error')
	`, scopedUserID(ctx), workout.ID, templateID, workout.Revision, workout.ScheduledDate)
	if err != nil {
		return garminManagedSchedule{}, err
	}
	if command.RowsAffected() == 0 {
		err = s.db.QueryRow(ctx, `
			insert into garmin_workout_schedules(
				user_id, workout_id, template_id, workout_revision, scheduled_date,
				desired_state, status, last_attempt_at
			) values($1, $2, $3, $4, $5::date, 'scheduled', 'pending', now())
			returning id::text
		`, scopedUserID(ctx), workout.ID, templateID, workout.Revision, workout.ScheduledDate).Scan(&id)
		if err != nil {
			return garminManagedSchedule{}, err
		}
	}
	return s.GetActiveGarminWorkoutSchedule(ctx, workout.ID)
}

func (s *Store) MarkGarminWorkoutScheduleScheduled(ctx context.Context, id string, remote GarminBridgeScheduledWorkout) error {
	remoteBytes, err := json.Marshal(remote.Raw)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		update garmin_workout_schedules
		set provider_schedule_id = $3, status = 'scheduled', error = '', remote = $4,
			scheduled_at = now(), last_attempt_at = now(), updated_at = now()
		where id = $1 and user_id = $2
	`, id, scopedUserID(ctx), remote.ID, remoteBytes)
	return err
}

func (s *Store) MarkGarminWorkoutScheduleError(ctx context.Context, workout Workout, templateID, message string) error {
	schedule, err := s.UpsertPendingGarminWorkoutSchedule(ctx, workout, templateID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		update garmin_workout_schedules
		set status = 'error', error = $3, last_attempt_at = now(), updated_at = now()
		where id = $1 and user_id = $2
	`, schedule.ID, scopedUserID(ctx), message)
	return err
}

func (s *Store) MarkGarminWorkoutScheduleRemoved(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `
		update garmin_workout_schedules
		set desired_state = 'absent', status = 'removed', error = '', removed_at = now(),
			last_attempt_at = now(), updated_at = now()
		where id = $1 and user_id = $2
	`, id, scopedUserID(ctx))
	return err
}

func (s *Store) MarkGarminWorkoutScheduleExistingError(ctx context.Context, id, message string) error {
	_, err := s.db.Exec(ctx, `
		update garmin_workout_schedules
		set status = 'error', error = $3, last_attempt_at = now(), updated_at = now()
		where id = $1 and user_id = $2
	`, id, scopedUserID(ctx), message)
	return err
}

func (s *Store) MarkGarminWorkoutScheduleUnverified(ctx context.Context, id string, remote GarminBridgeScheduledWorkout, message string) error {
	remoteBytes, err := json.Marshal(remote.Raw)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		update garmin_workout_schedules
		set provider_schedule_id = $3, status = 'error', error = $4, remote = $5,
			last_attempt_at = now(), updated_at = now()
		where id = $1 and user_id = $2
	`, id, scopedUserID(ctx), remote.ID, message, remoteBytes)
	return err
}

func (s *Store) ListStaleGarminWorkoutTemplates(ctx context.Context, before string) ([]garminManagedTemplate, error) {
	rows, err := s.db.Query(ctx, `
		select id::text, definition_hash, provider_workout_id, name, ownership_marker,
			payload, remote, status, error
		from garmin_workout_templates template
		where template.user_id = $1 and template.status in ('uploaded', 'error') and template.deleted_at is null
			and template.provider_workout_id <> ''
			and exists (
				select 1 from garmin_workout_schedules schedule
				where schedule.template_id = template.id and schedule.scheduled_date < $2::date
			)
			and not exists (
				select 1 from garmin_workout_schedules schedule
				where schedule.template_id = template.id
					and schedule.status in ('pending', 'scheduled', 'removing', 'error')
			)
	`, scopedUserID(ctx), before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]garminManagedTemplate, 0)
	for rows.Next() {
		template, err := scanGarminManagedTemplate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, template)
	}
	return result, rows.Err()
}

func (s *GarminService) PreviewWorkoutReconcile(ctx context.Context) (WorkoutReconcileResult, error) {
	config, err := s.store.GetWorkoutConfig(ctx)
	if err != nil {
		return WorkoutReconcileResult{}, err
	}
	result, workouts, _, err := s.workoutReconcileInputs(ctx, config)
	if err != nil || !result.Enabled {
		return result, err
	}
	for _, workout := range workouts {
		schedule, scheduleErr := s.store.GetActiveGarminWorkoutSchedule(ctx, workout.ID)
		desired := workoutDesiredForGarmin(workout, result.From, result.To)
		switch {
		case desired && errors.Is(scheduleErr, pgx.ErrNoRows):
			result.Actions = append(result.Actions, WorkoutReconcileAction{WorkoutID: workout.ID, Action: "schedule", Date: workout.ScheduledDate, Status: "pending"})
		case desired && scheduleErr == nil && (schedule.ScheduledDate != workout.ScheduledDate || schedule.WorkoutRevision != workout.Revision):
			result.Actions = append(result.Actions, WorkoutReconcileAction{WorkoutID: workout.ID, Action: "replace", Date: workout.ScheduledDate, Status: "pending"})
		case !desired && scheduleErr == nil:
			result.Actions = append(result.Actions, WorkoutReconcileAction{WorkoutID: workout.ID, Action: "unschedule", Date: schedule.ScheduledDate, Status: "pending"})
		case scheduleErr != nil && !errors.Is(scheduleErr, pgx.ErrNoRows):
			return WorkoutReconcileResult{}, scheduleErr
		}
	}
	return result, nil
}

func (s *GarminService) ReconcileWorkouts(ctx context.Context) (WorkoutReconcileResult, error) {
	config, err := s.store.GetWorkoutConfig(ctx)
	if err != nil {
		return WorkoutReconcileResult{}, err
	}
	result, workouts, location, err := s.workoutReconcileInputs(ctx, config)
	if err != nil || !result.Enabled {
		return result, err
	}

	remoteSchedules, err := s.listWorkoutSchedulesForReconcile(ctx, workouts, location)
	if err != nil {
		return result, fmt.Errorf("list Garmin workout calendar: %w", err)
	}
	tokenStore := s.tokenStore(ctx)
	for _, workout := range workouts {
		schedule, scheduleErr := s.store.GetActiveGarminWorkoutSchedule(ctx, workout.ID)
		if scheduleErr != nil && !errors.Is(scheduleErr, pgx.ErrNoRows) {
			return result, scheduleErr
		}
		desired := workoutDesiredForGarmin(workout, result.From, result.To)
		if !desired {
			if scheduleErr == nil {
				action := s.removeManagedWorkoutSchedule(ctx, tokenStore, schedule, remoteSchedules)
				result.Actions = append(result.Actions, action)
			}
			continue
		}

		tolerance := config.DefaultPaceToleranceSeconds
		if workout.PaceToleranceSeconds != nil {
			tolerance = *workout.PaceToleranceSeconds
		}
		ownerToken, err := s.store.GarminWorkoutOwnerToken(ctx)
		if err != nil {
			return result, err
		}
		compiled, err := compileGarminWorkout(workout.Definition, tolerance, ownerToken)
		if err != nil {
			message := "could not compile workout: " + err.Error()
			if scheduleErr == nil {
				_ = s.store.MarkGarminWorkoutScheduleExistingError(ctx, schedule.ID, message)
			} else {
				_ = s.store.MarkGarminWorkoutScheduleError(ctx, workout, "", message)
			}
			result.Actions = append(result.Actions, WorkoutReconcileAction{WorkoutID: workout.ID, Action: "schedule", Date: workout.ScheduledDate, Status: "error", Message: message})
			continue
		}
		template, err := s.ensureManagedWorkoutTemplate(ctx, tokenStore, compiled)
		if err != nil {
			message := err.Error()
			templateID := template.ID
			if scheduleErr == nil {
				_ = s.store.MarkGarminWorkoutScheduleExistingError(ctx, schedule.ID, message)
			} else {
				_ = s.store.MarkGarminWorkoutScheduleError(ctx, workout, templateID, message)
			}
			result.Actions = append(result.Actions, WorkoutReconcileAction{WorkoutID: workout.ID, Action: "schedule", Date: workout.ScheduledDate, Status: "conflict", Message: message})
			continue
		}

		if scheduleErr == nil && schedule.TemplateID == template.ID && schedule.WorkoutRevision == workout.Revision && schedule.ScheduledDate == workout.ScheduledDate {
			if remote, ok := remoteSchedules[schedule.ProviderScheduleID]; ok && remote.WorkoutID == template.ProviderWorkoutID && remote.Date == workout.ScheduledDate {
				if schedule.Status != "scheduled" {
					if err := s.store.MarkGarminWorkoutScheduleScheduled(ctx, schedule.ID, remote); err != nil {
						return result, err
					}
				}
				action := "none"
				if schedule.Status != "scheduled" {
					action = "recover"
				}
				result.Actions = append(result.Actions, WorkoutReconcileAction{WorkoutID: workout.ID, Action: action, Date: workout.ScheduledDate, Status: "scheduled"})
				continue
			}
			if schedule.Status == "error" && schedule.ProviderScheduleID == "" && garminScheduleRetryUnsafe(schedule.Error) {
				result.Actions = append(result.Actions, WorkoutReconcileAction{WorkoutID: workout.ID, Action: "schedule", Date: workout.ScheduledDate, Status: "conflict", Message: schedule.Error})
				continue
			}
		}

		if scheduleErr == nil {
			action := s.removeManagedWorkoutSchedule(ctx, tokenStore, schedule, remoteSchedules)
			result.Actions = append(result.Actions, action)
			if action.Status == "conflict" || action.Status == "error" {
				continue
			}
		}
		pending, err := s.store.UpsertPendingGarminWorkoutSchedule(ctx, workout, template.ID)
		if err != nil {
			return result, err
		}
		remote, err := s.bridge.ScheduleWorkout(ctx, tokenStore, template.ProviderWorkoutID, workout.ScheduledDate)
		if err != nil {
			message := "Garmin scheduling failed with an uncertain outcome; no automatic retry was attempted: " + err.Error()
			_ = s.store.MarkGarminWorkoutScheduleExistingError(ctx, pending.ID, message)
			result.Actions = append(result.Actions, WorkoutReconcileAction{WorkoutID: workout.ID, Action: "schedule", Date: workout.ScheduledDate, Status: "error", Message: message})
			continue
		}
		if remote.ID == "" || remote.WorkoutID != template.ProviderWorkoutID {
			message := "Garmin returned an unverifiable scheduled workout; no automatic retry was attempted"
			if remote.ID == "" {
				_ = s.store.MarkGarminWorkoutScheduleExistingError(ctx, pending.ID, message)
			} else {
				_ = s.store.MarkGarminWorkoutScheduleUnverified(ctx, pending.ID, remote, message)
			}
			result.Actions = append(result.Actions, WorkoutReconcileAction{WorkoutID: workout.ID, Action: "schedule", Date: workout.ScheduledDate, Status: "conflict", Message: message})
			continue
		}
		if remote.Date == "" {
			remote.Date = workout.ScheduledDate
		}
		if err := s.store.MarkGarminWorkoutScheduleScheduled(ctx, pending.ID, remote); err != nil {
			return result, err
		}
		remoteSchedules[remote.ID] = remote
		result.Actions = append(result.Actions, WorkoutReconcileAction{WorkoutID: workout.ID, Action: "schedule", Date: workout.ScheduledDate, Status: "scheduled"})
	}

	stale, err := s.store.ListStaleGarminWorkoutTemplates(ctx, result.From)
	if err != nil {
		return result, err
	}
	for _, template := range stale {
		remote, err := s.bridge.GetWorkout(ctx, tokenStore, template.ProviderWorkoutID)
		if errors.Is(err, ErrGarminNotFound) {
			if err := s.store.MarkGarminWorkoutTemplateDeleted(ctx, template.ID); err != nil {
				return result, err
			}
			result.Actions = append(result.Actions, WorkoutReconcileAction{Action: "delete_template", Status: "deleted"})
			continue
		}
		if err != nil {
			_ = s.store.MarkGarminWorkoutTemplateError(ctx, template.ID, "could not verify remote workout before cleanup: "+err.Error())
			continue
		}
		if err := verifyGarminManagedTemplate(template, remote); err != nil {
			_ = s.store.MarkGarminWorkoutTemplateError(ctx, template.ID, err.Error())
			continue
		}
		if err := s.bridge.DeleteWorkout(ctx, tokenStore, template.ProviderWorkoutID); err != nil {
			_ = s.store.MarkGarminWorkoutTemplateError(ctx, template.ID, "Garmin template cleanup failed: "+err.Error())
			continue
		}
		if err := s.store.MarkGarminWorkoutTemplateDeleted(ctx, template.ID); err != nil {
			return result, err
		}
		result.Actions = append(result.Actions, WorkoutReconcileAction{Action: "delete_template", Status: "deleted"})
	}
	return result, nil
}

func garminScheduleRetryUnsafe(message string) bool {
	return strings.HasPrefix(message, "Garmin scheduling failed with an uncertain outcome") ||
		strings.HasPrefix(message, "Garmin returned an unverifiable scheduled workout")
}

func (s *GarminService) workoutReconcileInputs(ctx context.Context, config WorkoutConfig) (WorkoutReconcileResult, []Workout, *time.Location, error) {
	result := WorkoutReconcileResult{Enabled: config.SyncEnabled, Actions: []WorkoutReconcileAction{}}
	if !config.SyncEnabled {
		return result, nil, time.UTC, nil
	}
	location, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return result, nil, nil, fmt.Errorf("invalid workout timezone: %w", err)
	}
	now := time.Now().In(location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	end := today.AddDate(0, 0, garminWorkoutHorizonDays-1)
	result.From = today.Format("2006-01-02")
	result.To = end.Format("2006-01-02")
	workouts, err := s.store.ListWorkoutReconcileCandidates(ctx, result.From, result.To)
	return result, workouts, location, err
}

func workoutDesiredForGarmin(workout Workout, from, to string) bool {
	return workout.ArchivedAt == nil && !workout.GarminExcluded && workout.ParseStatus != workoutParseError &&
		workout.ScheduledDate >= from && workout.ScheduledDate <= to
}

func (s *GarminService) ensureManagedWorkoutTemplate(ctx context.Context, tokenStore string, compiled compiledGarminWorkout) (garminManagedTemplate, error) {
	template, err := s.store.GetGarminWorkoutTemplateByHash(ctx, compiled.DefinitionHash)
	if errors.Is(err, pgx.ErrNoRows) {
		template, err = s.store.CreateGarminWorkoutTemplate(ctx, compiled)
	}
	if err != nil {
		return template, err
	}
	if template.ProviderWorkoutID == "" {
		if template.Status == "error" {
			return template, errors.New("previous Garmin upload was inconclusive; no retry was attempted to avoid creating duplicates")
		}
		remote, uploadErr := s.bridge.UploadWorkout(ctx, tokenStore, compiled.Payload)
		if uploadErr != nil {
			message := "Garmin workout upload failed; no automatic retry will be attempted: " + uploadErr.Error()
			_ = s.store.MarkGarminWorkoutTemplateError(ctx, template.ID, message)
			return template, errors.New(message)
		}
		if remote.ID == "" {
			message := "Garmin workout upload returned no ID; no automatic retry will be attempted"
			_ = s.store.MarkGarminWorkoutTemplateError(ctx, template.ID, message)
			return template, errors.New(message)
		}
		template.ProviderWorkoutID = remote.ID
		_ = s.store.RecordGarminWorkoutTemplateProviderID(ctx, template.ID, remote.ID, remote.Raw)
		if verifyErr := verifyGarminManagedTemplate(template, remote); verifyErr != nil {
			remote, err = s.bridge.GetWorkout(ctx, tokenStore, remote.ID)
			if err != nil {
				message := "uploaded Garmin workout could not be ownership-verified; it was not scheduled: " + err.Error()
				_ = s.store.MarkGarminWorkoutTemplateError(ctx, template.ID, message)
				return template, errors.New(message)
			}
		}
		if err := verifyGarminManagedTemplate(template, remote); err != nil {
			message := "uploaded Garmin workout could not be ownership-verified; it was not scheduled or modified"
			_ = s.store.MarkGarminWorkoutTemplateError(ctx, template.ID, message)
			return template, errors.New(message)
		}
		if err := s.store.MarkGarminWorkoutTemplateUploaded(ctx, template.ID, remote.Raw); err != nil {
			return template, err
		}
		template.Status = "uploaded"
		template.Remote = remote.Raw
		return template, nil
	}
	remote, err := s.bridge.GetWorkout(ctx, tokenStore, template.ProviderWorkoutID)
	if err != nil {
		message := "tracked Garmin workout could not be ownership-verified; it was left untouched: " + err.Error()
		_ = s.store.MarkGarminWorkoutTemplateError(ctx, template.ID, message)
		return template, errors.New(message)
	}
	if err := verifyGarminManagedTemplate(template, remote); err != nil {
		message := "Garmin ownership conflict: tracked workout ID or Runnarr marker does not match; remote workout was left untouched"
		_ = s.store.MarkGarminWorkoutTemplateError(ctx, template.ID, message)
		return template, errors.New(message)
	}
	if err := s.store.MarkGarminWorkoutTemplateUploaded(ctx, template.ID, remote.Raw); err != nil {
		return template, err
	}
	return template, nil
}

func verifyGarminManagedTemplate(template garminManagedTemplate, remote GarminBridgeWorkout) error {
	if remote.ID != template.ProviderWorkoutID || remote.Description != template.OwnershipMarker {
		return errGarminWorkoutOwnership
	}
	return garminWorkoutRemoteOwned(remote.Raw, template.ProviderWorkoutID, template.OwnershipMarker)
}

func (s *GarminService) removeManagedWorkoutSchedule(ctx context.Context, tokenStore string, schedule garminManagedSchedule, remoteSchedules map[string]GarminBridgeScheduledWorkout) WorkoutReconcileAction {
	action := WorkoutReconcileAction{WorkoutID: schedule.WorkoutID, Action: "unschedule", Date: schedule.ScheduledDate}
	if schedule.ProviderScheduleID == "" {
		if err := s.store.MarkGarminWorkoutScheduleRemoved(ctx, schedule.ID); err != nil {
			action.Status, action.Message = "error", err.Error()
			return action
		}
		action.Status = "removed"
		return action
	}
	template, err := s.store.GetGarminWorkoutTemplate(ctx, schedule.TemplateID)
	if err != nil {
		action.Status, action.Message = "conflict", "local Runnarr ownership record is missing; Garmin was left untouched"
		_ = s.store.MarkGarminWorkoutScheduleExistingError(ctx, schedule.ID, action.Message)
		return action
	}
	remote, exists := remoteSchedules[schedule.ProviderScheduleID]
	if !exists {
		remote, err = s.bridge.GetScheduledWorkout(ctx, tokenStore, schedule.ProviderScheduleID)
		if errors.Is(err, ErrGarminNotFound) {
			if err := s.store.MarkGarminWorkoutScheduleRemoved(ctx, schedule.ID); err != nil {
				action.Status, action.Message = "error", err.Error()
				return action
			}
			action.Status = "removed"
			return action
		}
		if err != nil || remote.ID != schedule.ProviderScheduleID {
			action.Status, action.Message = "conflict", "Garmin calendar entry could not be verified by ID; it was left untouched"
			_ = s.store.MarkGarminWorkoutScheduleExistingError(ctx, schedule.ID, action.Message)
			return action
		}
	}
	remoteTemplate, err := s.bridge.GetWorkout(ctx, tokenStore, template.ProviderWorkoutID)
	if err != nil || verifyGarminManagedTemplate(template, remoteTemplate) != nil {
		action.Status, action.Message = "conflict", "Garmin workout ownership could not be verified; calendar entry was left untouched"
		_ = s.store.MarkGarminWorkoutScheduleExistingError(ctx, schedule.ID, action.Message)
		return action
	}
	if remote.WorkoutID != template.ProviderWorkoutID {
		action.Status, action.Message = "conflict", "Garmin calendar entry points to a non-Runnarr workout; it was left untouched"
		_ = s.store.MarkGarminWorkoutScheduleExistingError(ctx, schedule.ID, action.Message)
		return action
	}
	if err := s.bridge.UnscheduleWorkout(ctx, tokenStore, schedule.ProviderScheduleID); err != nil {
		action.Status, action.Message = "error", "Garmin unschedule failed: "+err.Error()
		_ = s.store.MarkGarminWorkoutScheduleExistingError(ctx, schedule.ID, action.Message)
		return action
	}
	if err := s.store.MarkGarminWorkoutScheduleRemoved(ctx, schedule.ID); err != nil {
		action.Status, action.Message = "error", err.Error()
		return action
	}
	action.Status = "removed"
	return action
}

func (s *GarminService) listWorkoutSchedulesForReconcile(ctx context.Context, workouts []Workout, location *time.Location) (map[string]GarminBridgeScheduledWorkout, error) {
	months := make(map[string]time.Time)
	for _, workout := range workouts {
		date, err := time.ParseInLocation("2006-01-02", workout.ScheduledDate, location)
		if err == nil {
			months[date.Format("2006-01")] = date
		}
		schedule, err := s.store.GetActiveGarminWorkoutSchedule(ctx, workout.ID)
		if err == nil {
			date, parseErr := time.ParseInLocation("2006-01-02", schedule.ScheduledDate, location)
			if parseErr == nil {
				months[date.Format("2006-01")] = date
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	keys := make([]string, 0, len(months))
	for key := range months {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(map[string]GarminBridgeScheduledWorkout)
	for _, key := range keys {
		date := months[key]
		items, err := s.bridge.ListScheduledWorkouts(ctx, s.tokenStore(ctx), date.Year(), int(date.Month()))
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.ID != "" {
				result[item.ID] = item
			}
		}
	}
	return result, nil
}
