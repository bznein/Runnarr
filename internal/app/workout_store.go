package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const workoutSelectSQL = `
	select workouts.id::text, workouts.source, coalesce(workouts.planned_activity_id::text, ''),
		coalesce(workouts.copied_from_workout_id::text, ''), workouts.name, workouts.sport_type,
		workouts.source_text, workouts.source_hash, workouts.definition, workouts.parse_status,
		workouts.parse_messages, coalesce(workouts.scheduled_date::text, ''), workouts.pace_tolerance_s,
		workouts.garmin_excluded, workouts.revision, workouts.generated_at, workouts.archived_at,
		workouts.created_at, workouts.updated_at,
		coalesce(schedule.status, ''), coalesce(schedule.error, ''),
		coalesce(template.provider_workout_id, ''), coalesce(schedule.provider_schedule_id, ''),
		schedule.scheduled_at
	from workouts
	left join lateral (
		select * from garmin_workout_schedules
		where user_id = workouts.user_id and workout_id = workouts.id
		order by created_at desc limit 1
	) schedule on true
	left join garmin_workout_templates template on template.id = schedule.template_id
`

func (s *Store) GetWorkoutConfig(ctx context.Context) (WorkoutConfig, error) {
	config := WorkoutConfig{HorizonDays: 7}
	err := s.db.QueryRow(ctx, `
		select workout_sync_enabled, workout_default_pace_tolerance_s, workout_timezone
		from user_settings where user_id = $1
	`, scopedUserID(ctx)).Scan(&config.SyncEnabled, &config.DefaultPaceToleranceSeconds, &config.Timezone)
	return config, err
}

func (s *Store) SetWorkoutConfig(ctx context.Context, config WorkoutConfig) error {
	if config.DefaultPaceToleranceSeconds < 0 || config.DefaultPaceToleranceSeconds > 60 {
		return fmt.Errorf("%w: pace tolerance must be between 0 and 60 seconds", ErrWorkoutInvalid)
	}
	config.Timezone = strings.TrimSpace(config.Timezone)
	if config.Timezone != "" {
		if _, err := time.LoadLocation(config.Timezone); err != nil {
			return fmt.Errorf("%w: timezone must be a valid IANA timezone", ErrWorkoutInvalid)
		}
	}
	if config.SyncEnabled && config.Timezone == "" {
		return fmt.Errorf("%w: timezone is required before enabling workout sync", ErrWorkoutInvalid)
	}
	_, err := s.db.Exec(ctx, `
		update user_settings
		set workout_sync_enabled = $2, workout_default_pace_tolerance_s = $3,
			workout_timezone = $4, updated_at = now()
		where user_id = $1
	`, scopedUserID(ctx), config.SyncEnabled, config.DefaultPaceToleranceSeconds, config.Timezone)
	return err
}

func (s *Store) GarminWorkoutOwnerToken(ctx context.Context) (string, error) {
	var token string
	err := s.db.QueryRow(ctx, `select garmin_workout_owner_token::text from user_settings where user_id = $1`, scopedUserID(ctx)).Scan(&token)
	return token, err
}

func (s *Store) ListWorkouts(ctx context.Context, filter string) ([]Workout, error) {
	conditions := ` where workouts.user_id = $1 and workouts.archived_at is null`
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "drafts":
		conditions += ` and workouts.scheduled_date is null`
	case "attention":
		conditions += ` and (workouts.parse_status = 'error' or coalesce(schedule.status, '') = 'error')`
	case "excluded":
		conditions += ` and workouts.garmin_excluded`
	case "past":
		conditions += ` and workouts.scheduled_date < current_date`
	case "", "upcoming":
		conditions += ` and (workouts.scheduled_date is null or workouts.scheduled_date >= current_date)`
	default:
		return nil, fmt.Errorf("%w: unknown workout filter", ErrWorkoutInvalid)
	}
	rows, err := s.db.Query(ctx, workoutSelectSQL+conditions+` order by workouts.scheduled_date nulls first, workouts.updated_at desc`, scopedUserID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Workout, 0)
	for rows.Next() {
		workout, err := scanWorkout(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, workout)
	}
	return result, rows.Err()
}

func (s *Store) ListManualCalendarWorkouts(ctx context.Context, from, to time.Time) ([]CalendarActivity, error) {
	rows, err := s.db.Query(ctx, `
		select workouts.id::text, workouts.name, workouts.scheduled_date,
			workouts.sport_type, coalesce((workouts.definition->>'estimatedDurationS')::integer, 0)
		from workouts
		join planned_activities on planned_activities.id = workouts.planned_activity_id
		where workouts.user_id = $1 and workouts.source = 'manual' and workouts.archived_at is null
			and workouts.scheduled_date between $2::date and $3::date
			and planned_activities.status = $4
		order by workouts.scheduled_date, workouts.name
	`, scopedUserID(ctx), from.Format("2006-01-02"), to.Format("2006-01-02"), plannedActivityStatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]CalendarActivity, 0)
	for rows.Next() {
		var item CalendarActivity
		var date time.Time
		if err := rows.Scan(&item.ID, &item.Name, &date, &item.SportType, &item.MovingTimeS); err != nil {
			return nil, err
		}
		item.WorkoutID = item.ID
		item.Source = "manual_workout"
		item.StartTime = date
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GetWorkout(ctx context.Context, id string) (Workout, error) {
	return scanWorkout(s.db.QueryRow(ctx, workoutSelectSQL+` where workouts.id = $1 and workouts.user_id = $2`, id, scopedUserID(ctx)))
}

func (s *Store) UpsertSheetWorkoutForPlanned(ctx context.Context, planned PlannedActivity, notify bool) error {
	if planned.Source != trainingSheetProvider {
		return nil
	}
	var plannedID string
	err := s.db.QueryRow(ctx, `
		select id::text from planned_activities
		where user_id = $1 and source = $2 and source_id = $3
	`, scopedUserID(ctx), planned.Source, planned.SourceID).Scan(&plannedID)
	if err != nil {
		return err
	}
	prescription := workoutPrescriptionForPlanned(planned)
	if prescription == "" {
		var archivedID string
		err = s.db.QueryRow(ctx, `
			update workouts set archived_at = now(), garmin_excluded = true, updated_at = now()
			where user_id = $1 and planned_activity_id = $2 and source = 'training_sheet' and archived_at is null
			returning id::text
		`, scopedUserID(ctx), plannedID).Scan(&archivedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if notify {
			if workout, loadErr := s.GetWorkout(ctx, archivedID); loadErr == nil {
				s.publishWorkoutChangeNotification(ctx, workout, "archived")
			}
		}
		return nil
	}
	table := workoutTableFromPlanned(planned)
	parsed := parseWorkoutPrescription(prescription, table)
	definitionBytes, err := json.Marshal(parsed.Definition)
	if err != nil {
		return err
	}
	messagesBytes, err := json.Marshal(parsed.Messages)
	if err != nil {
		return err
	}
	sourceHash := workoutSourceHash(prescription, table)
	var previous struct {
		ID, Name, SportType, SourceHash, ParseStatus, ScheduledDate string
	}
	previousErr := s.db.QueryRow(ctx, `
		select id::text, name, sport_type, source_hash, parse_status, coalesce(scheduled_date::text, '')
		from workouts where user_id = $1 and planned_activity_id = $2 and source = 'training_sheet' and archived_at is null
	`, scopedUserID(ctx), plannedID).Scan(&previous.ID, &previous.Name, &previous.SportType, &previous.SourceHash, &previous.ParseStatus, &previous.ScheduledDate)
	if previousErr != nil && !errors.Is(previousErr, pgx.ErrNoRows) {
		return previousErr
	}
	var workoutID string
	err = s.db.QueryRow(ctx, `
		insert into workouts(
			user_id, source, planned_activity_id, name, sport_type, source_text, source_hash,
			definition, parse_status, parse_messages, scheduled_date
		) values($1, 'training_sheet', $2, $3, $4, $5, $6, $7, $8, $9, $10)
		on conflict(user_id, planned_activity_id)
			where source = 'training_sheet' and archived_at is null
		do update set
			name = excluded.name, sport_type = excluded.sport_type,
			source_text = excluded.source_text, source_hash = excluded.source_hash,
			definition = excluded.definition, parse_status = excluded.parse_status,
			parse_messages = excluded.parse_messages, scheduled_date = excluded.scheduled_date,
			revision = case when workouts.source_hash <> excluded.source_hash then workouts.revision + 1 else workouts.revision end,
			generated_at = case when workouts.source_hash <> excluded.source_hash then now() else workouts.generated_at end,
			updated_at = now()
		returning id::text
	`, scopedUserID(ctx), plannedID, planned.Name, planned.SportType, prescription, sourceHash,
		definitionBytes, parsed.Status, messagesBytes, planned.PlannedDate.Format("2006-01-02")).Scan(&workoutID)
	if err != nil {
		return err
	}
	if notify {
		created := errors.Is(previousErr, pgx.ErrNoRows)
		changed := created || previous.Name != planned.Name || previous.SportType != planned.SportType || previous.SourceHash != sourceHash ||
			previous.ParseStatus != parsed.Status || previous.ScheduledDate != planned.PlannedDate.Format("2006-01-02")
		if changed {
			if workout, loadErr := s.GetWorkout(ctx, workoutID); loadErr == nil {
				change := "updated"
				if created {
					change = "generated"
				}
				s.publishWorkoutChangeNotification(ctx, workout, change)
			}
		}
	}
	return nil
}

func workoutPrescriptionForPlanned(planned PlannedActivity) string {
	if notes := strings.TrimSpace(planned.Notes); isStructuredWorkoutPrescription(notes) {
		return notes
	}
	if name := strings.TrimSpace(planned.Name); workoutSurgesPattern.MatchString(name) {
		return name
	}
	return ""
}

func (s *Store) BackfillSheetWorkouts(ctx context.Context, notify bool) error {
	rows, err := s.db.Query(ctx, `
		select id::text, source, source_id, workbook_id, sheet_id, sheet_title, plan_cell,
			feedback_cell, planned_date, name, sport_type, notes, status, source_url, raw,
			created_at, updated_at
		from planned_activities
		where user_id = $1 and source = $2 and status <> $3
		order by planned_date
	`, scopedUserID(ctx), trainingSheetProvider, plannedActivityStatusSuperseded)
	if err != nil {
		return err
	}
	defer rows.Close()
	plans := make([]PlannedActivity, 0)
	for rows.Next() {
		var planned PlannedActivity
		var rawBytes []byte
		if err := rows.Scan(&planned.ID, &planned.Source, &planned.SourceID, &planned.WorkbookID, &planned.SheetID,
			&planned.SheetTitle, &planned.PlanCell, &planned.FeedbackCell, &planned.PlannedDate, &planned.Name,
			&planned.SportType, &planned.Notes, &planned.Status, &planned.SourceURL, &rawBytes,
			&planned.CreatedAt, &planned.UpdatedAt); err != nil {
			return err
		}
		if len(rawBytes) > 0 {
			_ = json.Unmarshal(rawBytes, &planned.Raw)
		}
		plans = append(plans, planned)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, planned := range plans {
		if err := s.UpsertSheetWorkoutForPlanned(ctx, planned, notify); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateManualWorkout(ctx context.Context, workout Workout) (Workout, error) {
	definitionBytes, messagesBytes, err := marshalWorkoutParts(workout)
	if err != nil {
		return Workout{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Workout{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `
		insert into workouts(user_id, source, copied_from_workout_id, name, sport_type, source_text,
			source_hash, definition, parse_status, parse_messages, scheduled_date, pace_tolerance_s, garmin_excluded)
		values($1, 'manual', nullif($2, '')::uuid, $3, 'Run', $4, $5, $6, $7, $8, nullif($9, '')::date, $10, $11)
		returning id::text
	`, scopedUserID(ctx), workout.CopiedFromWorkoutID, workout.Name, workout.SourceText, workout.SourceHash,
		definitionBytes, workout.ParseStatus, messagesBytes, workout.ScheduledDate,
		workout.PaceToleranceSeconds, workout.GarminExcluded).Scan(&id)
	if err != nil {
		return Workout{}, err
	}
	if workout.ScheduledDate != "" {
		plannedID, err := upsertManualWorkoutPlanTx(ctx, tx, scopedUserID(ctx), id, workout.Name, workout.ScheduledDate)
		if err != nil {
			return Workout{}, err
		}
		if _, err := tx.Exec(ctx, `update workouts set planned_activity_id = $2 where id = $1`, id, plannedID); err != nil {
			return Workout{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Workout{}, err
	}
	return s.GetWorkout(ctx, id)
}

func (s *Store) SaveManualWorkout(ctx context.Context, workout Workout) (Workout, error) {
	definitionBytes, messagesBytes, err := marshalWorkoutParts(workout)
	if err != nil {
		return Workout{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Workout{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var source, plannedID string
	if err := tx.QueryRow(ctx, `
		select source, coalesce(planned_activity_id::text, '') from workouts
		where id = $1 and user_id = $2 for update
	`, workout.ID, scopedUserID(ctx)).Scan(&source, &plannedID); err != nil {
		return Workout{}, err
	}
	if source != workoutSourceManual {
		return Workout{}, ErrWorkoutReadOnly
	}
	if workout.ScheduledDate == "" && plannedID != "" {
		var status string
		if err := tx.QueryRow(ctx, `select status from planned_activities where id = $1 and user_id = $2 for update`, plannedID, scopedUserID(ctx)).Scan(&status); err != nil {
			return Workout{}, err
		}
		if status == plannedActivityStatusCompleted {
			return Workout{}, ErrWorkoutPlanCompleted
		}
		if _, err := tx.Exec(ctx, `update workouts set planned_activity_id = null where id = $1`, workout.ID); err != nil {
			return Workout{}, err
		}
		if _, err := tx.Exec(ctx, `delete from planned_activities where id = $1 and user_id = $2`, plannedID, scopedUserID(ctx)); err != nil {
			return Workout{}, err
		}
		plannedID = ""
	}
	if workout.ScheduledDate != "" {
		var err error
		plannedID, err = upsertManualWorkoutPlanTx(ctx, tx, scopedUserID(ctx), workout.ID, workout.Name, workout.ScheduledDate)
		if err != nil {
			return Workout{}, err
		}
	}
	_, err = tx.Exec(ctx, `
		update workouts set planned_activity_id = nullif($3, '')::uuid, name = $4, source_text = $5,
			source_hash = $6, definition = $7, parse_status = $8, parse_messages = $9,
			scheduled_date = nullif($10, '')::date, pace_tolerance_s = $11,
			garmin_excluded = $12, revision = revision + 1, generated_at = now(), updated_at = now()
		where id = $1 and user_id = $2
	`, workout.ID, scopedUserID(ctx), plannedID, workout.Name, workout.SourceText, workout.SourceHash,
		definitionBytes, workout.ParseStatus, messagesBytes, workout.ScheduledDate,
		workout.PaceToleranceSeconds, workout.GarminExcluded)
	if err != nil {
		return Workout{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Workout{}, err
	}
	return s.GetWorkout(ctx, workout.ID)
}

func (s *Store) SaveWorkoutOperations(ctx context.Context, workoutID string, paceTolerance *int, garminExcluded bool) (Workout, error) {
	if paceTolerance != nil && (*paceTolerance < 0 || *paceTolerance > 60) {
		return Workout{}, fmt.Errorf("%w: pace tolerance must be between 0 and 60 seconds", ErrWorkoutInvalid)
	}
	command, err := s.db.Exec(ctx, `
		update workouts set pace_tolerance_s = $3, garmin_excluded = $4,
			revision = case when pace_tolerance_s is distinct from $3 then revision + 1 else revision end,
			updated_at = now()
		where id = $1 and user_id = $2 and archived_at is null
	`, workoutID, scopedUserID(ctx), paceTolerance, garminExcluded)
	if err != nil {
		return Workout{}, err
	}
	if command.RowsAffected() == 0 {
		return Workout{}, pgx.ErrNoRows
	}
	return s.GetWorkout(ctx, workoutID)
}

func (s *Store) DuplicateWorkout(ctx context.Context, id string) (Workout, error) {
	original, err := s.GetWorkout(ctx, id)
	if err != nil {
		return Workout{}, err
	}
	original.ID = ""
	original.Source = workoutSourceManual
	original.PlannedActivityID = ""
	original.CopiedFromWorkoutID = id
	original.Name = "Copy of " + original.Name
	original.SourceText = ""
	original.SourceHash = ""
	original.ScheduledDate = ""
	original.GarminExcluded = false
	original.Garmin = WorkoutGarminState{}
	original.ParseStatus = workoutParseReady
	original.ParseMessages = []WorkoutParseMessage{}
	return s.CreateManualWorkout(ctx, original)
}

func (s *Store) DeleteWorkout(ctx context.Context, id string) (bool, error) {
	var source, plannedID string
	var hasDeployment bool
	err := s.db.QueryRow(ctx, `
		select source, coalesce(planned_activity_id::text, ''),
			exists(select 1 from garmin_workout_schedules where workout_id = workouts.id)
		from workouts where id = $1 and user_id = $2
	`, id, scopedUserID(ctx)).Scan(&source, &plannedID, &hasDeployment)
	if err != nil {
		return false, err
	}
	if source != workoutSourceManual {
		return false, ErrWorkoutReadOnly
	}
	if plannedID == "" && !hasDeployment {
		_, err = s.db.Exec(ctx, `delete from workouts where id = $1 and user_id = $2`, id, scopedUserID(ctx))
		return true, err
	}
	_, err = s.db.Exec(ctx, `
		update workouts set archived_at = now(), garmin_excluded = true, updated_at = now()
		where id = $1 and user_id = $2
	`, id, scopedUserID(ctx))
	if err == nil && plannedID != "" {
		_, err = s.db.Exec(ctx, `
			update planned_activities set status = $3, updated_at = now()
			where id = $1 and user_id = $2 and status = $4
		`, plannedID, scopedUserID(ctx), plannedActivityStatusSuperseded, plannedActivityStatusPending)
	}
	return false, err
}

func upsertManualWorkoutPlanTx(ctx context.Context, tx pgx.Tx, userID, workoutID, name, date string) (string, error) {
	var plannedID string
	raw, _ := json.Marshal(map[string]any{"workoutId": workoutID})
	err := tx.QueryRow(ctx, `
		insert into planned_activities(
			user_id, source, source_id, workbook_id, sheet_id, sheet_title, plan_cell,
			planned_date, name, sport_type, notes, status, source_url, raw, last_seen_at, updated_at
		) values($1, 'manual', $2, '', '', '', '', $3::date, $4, 'Run', '', 'pending', '', $5, now(), now())
		on conflict(user_id, source, source_id) do update set
			planned_date = excluded.planned_date, name = excluded.name,
			status = case when planned_activities.status = 'completed' then planned_activities.status else 'pending' end,
			raw = excluded.raw, last_seen_at = now(), updated_at = now()
		returning id::text
	`, userID, "workout:"+workoutID, date, name, raw).Scan(&plannedID)
	return plannedID, err
}

func marshalWorkoutParts(workout Workout) ([]byte, []byte, error) {
	if strings.TrimSpace(workout.Name) == "" || len(workout.Name) > 160 {
		return nil, nil, fmt.Errorf("%w: workout name is required and must be at most 160 characters", ErrWorkoutInvalid)
	}
	if workout.ParseStatus != workoutParseError {
		if err := validateWorkoutDefinition(workout.Definition); err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrWorkoutInvalid, err)
		}
	}
	definitionBytes, err := json.Marshal(workout.Definition)
	if err != nil {
		return nil, nil, err
	}
	messages := workout.ParseMessages
	if messages == nil {
		messages = []WorkoutParseMessage{}
	}
	messagesBytes, err := json.Marshal(messages)
	return definitionBytes, messagesBytes, err
}

func scanWorkout(row pgx.Row) (Workout, error) {
	var workout Workout
	var definitionBytes, messagesBytes []byte
	var scheduledDate sql.NullString
	var tolerance sql.NullInt32
	var archivedAt, garminScheduledAt sql.NullTime
	err := row.Scan(&workout.ID, &workout.Source, &workout.PlannedActivityID, &workout.CopiedFromWorkoutID,
		&workout.Name, &workout.SportType, &workout.SourceText, &workout.SourceHash,
		&definitionBytes, &workout.ParseStatus, &messagesBytes, &scheduledDate, &tolerance,
		&workout.GarminExcluded, &workout.Revision, &workout.GeneratedAt, &archivedAt,
		&workout.CreatedAt, &workout.UpdatedAt, &workout.Garmin.Status, &workout.Garmin.Error,
		&workout.Garmin.ProviderWorkoutID, &workout.Garmin.ProviderScheduleID, &garminScheduledAt)
	if err != nil {
		return Workout{}, err
	}
	if err := json.Unmarshal(definitionBytes, &workout.Definition); err != nil {
		return Workout{}, err
	}
	if err := json.Unmarshal(messagesBytes, &workout.ParseMessages); err != nil {
		return Workout{}, err
	}
	if workout.ParseMessages == nil {
		workout.ParseMessages = []WorkoutParseMessage{}
	}
	if scheduledDate.Valid {
		workout.ScheduledDate = scheduledDate.String
	}
	if tolerance.Valid {
		value := int(tolerance.Int32)
		workout.PaceToleranceSeconds = &value
	}
	if archivedAt.Valid {
		workout.ArchivedAt = &archivedAt.Time
	}
	if garminScheduledAt.Valid {
		workout.Garmin.ScheduledAt = &garminScheduledAt.Time
	}
	return workout, nil
}

func ensureWorkoutDate(value string) error {
	if value == "" {
		return nil
	}
	_, err := time.Parse("2006-01-02", value)
	if err != nil {
		return fmt.Errorf("%w: scheduled date must use YYYY-MM-DD", ErrWorkoutInvalid)
	}
	return nil
}

func workoutFromParse(name, source string, parsed WorkoutParseResult) Workout {
	return Workout{
		Source:        workoutSourceManual,
		Name:          strings.TrimSpace(name),
		SportType:     "Run",
		SourceText:    strings.TrimSpace(source),
		SourceHash:    workoutSourceHash(source, nil),
		Definition:    parsed.Definition,
		ParseStatus:   parsed.Status,
		ParseMessages: parsed.Messages,
	}
}
