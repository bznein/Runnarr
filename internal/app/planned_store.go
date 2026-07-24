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

const (
	googleSheetsTokenID             = "default"
	plannedActivityStatusPending    = "pending"
	plannedActivityStatusCompleted  = "completed"
	plannedActivityStatusSuperseded = "superseded"
)

var (
	errPlannedMatchInvalid      = errors.New("activity cannot be matched to a planned activity")
	errPlannedMatchConflict     = errors.New("planned activity is already matched")
	errPlannedMatchDateMismatch = errors.New("planned activity date does not match activity date")
)

type pgxQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Store) GetTrainingSheetConfig(ctx context.Context) (TrainingSheetConfig, error) {
	config := TrainingSheetConfig{CheckEveryHours: 24, PlanYear: time.Now().UTC().Year()}
	var lastSynced sql.NullTime
	err := s.db.QueryRow(ctx, `
		select training_sheet_enabled, training_sheet_sheet_url,
			training_sheet_check_every_hours, plan_year, training_sheet_last_synced_at
		from user_settings where user_id = $1
	`, scopedUserID(ctx)).Scan(&config.Enabled, &config.SheetURL, &config.CheckEveryHours, &config.PlanYear, &lastSynced)
	if errors.Is(err, pgx.ErrNoRows) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	config.SheetURL = strings.TrimSpace(config.SheetURL)
	if config.CheckEveryHours <= 0 || config.CheckEveryHours > 720 {
		config.CheckEveryHours = 24
	}
	if config.PlanYear <= 0 {
		config.PlanYear = time.Now().UTC().Year()
	}
	if lastSynced.Valid {
		config.LastSyncedAt = lastSynced.Time.UTC().Format(time.RFC3339)
	}
	return config, nil
}

func (s *Store) SetTrainingSheetConfig(ctx context.Context, config TrainingSheetConfig) error {
	checkEveryHours := config.CheckEveryHours
	if checkEveryHours <= 0 || checkEveryHours > 720 {
		checkEveryHours = 24
	}
	planYear := config.PlanYear
	if planYear <= 0 {
		planYear = time.Now().UTC().Year()
	}
	_, err := s.db.Exec(ctx, `
		update user_settings
		set training_sheet_enabled = $2, training_sheet_sheet_url = $3,
			training_sheet_check_every_hours = $4, plan_year = $5, updated_at = now()
		where user_id = $1
	`, scopedUserID(ctx), config.Enabled, strings.TrimSpace(config.SheetURL), checkEveryHours, planYear)
	return err
}

func (s *Store) TouchTrainingSheetConfigLastSyncedAt(ctx context.Context, syncedAt time.Time) error {
	_, err := s.db.Exec(ctx, `update user_settings set training_sheet_last_synced_at = $2, updated_at = now() where user_id = $1`, scopedUserID(ctx), syncedAt)
	return err
}

func (s *Store) LatestTrainingSheetScheduledSync(ctx context.Context) (time.Time, error) {
	var createdAt time.Time
	err := s.db.QueryRow(ctx, `
		select created_at from sync_jobs
		where user_id = $1 and provider = $2 and kind = 'scheduled'
		order by created_at desc limit 1
	`, scopedUserID(ctx), trainingSheetProvider).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	return createdAt, err
}

type googleSheetsTokenRecord struct {
	AccessTokenCiphertext  []byte
	RefreshTokenCiphertext []byte
	TokenExpiresAt         *time.Time
	Scopes                 []string
}

func (s *Store) UpsertPlannedActivity(ctx context.Context, planned PlannedActivity) error {
	raw := planned.Raw
	if raw == nil {
		raw = map[string]any{}
	}
	rawBytes, err := json.Marshal(raw)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
	insert into planned_activities(
			user_id, source, source_id, workbook_id, sheet_id, sheet_title, plan_cell,
			feedback_cell, planned_date, name, sport_type, notes, status, source_url, raw,
			last_seen_at, updated_at
		) values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, now(), now())
		on conflict(user_id, source, source_id) do update set
			workbook_id = excluded.workbook_id,
			sheet_id = excluded.sheet_id,
			sheet_title = excluded.sheet_title,
			plan_cell = excluded.plan_cell,
			feedback_cell = excluded.feedback_cell,
			planned_date = excluded.planned_date,
			name = excluded.name,
			sport_type = excluded.sport_type,
			notes = excluded.notes,
			status = case when planned_activities.status = 'superseded' then excluded.status else planned_activities.status end,
			source_url = excluded.source_url,
			raw = excluded.raw,
			last_seen_at = now(),
			updated_at = now()
	`, scopedUserID(ctx), planned.Source, planned.SourceID, planned.WorkbookID, planned.SheetID, planned.SheetTitle,
		planned.PlanCell, planned.FeedbackCell, planned.PlannedDate, planned.Name, planned.SportType, planned.Notes,
		planned.Status, planned.SourceURL, rawBytes)
	if err != nil {
		return err
	}

	_, err = s.SaveImportedActivity(ctx, planned.Source, planned.SourceID, nil, ImportedActivity{
		Name:                planned.Name,
		SportType:           planned.SportType,
		LocalNotes:          planned.Notes,
		StartTime:           planned.PlannedDate.UTC(),
		OriginalProviderURL: planned.SourceURL,
		Raw:                 raw,
	})
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		update activities
		set name = $4, sport_type = $5, start_time = $6, local_notes = $7,
			original_provider_url = $8, updated_at = now()
		where user_id = $1 and source = $2 and source_id = $3
	`, scopedUserID(ctx), planned.Source, planned.SourceID, planned.Name, planned.SportType, planned.PlannedDate.UTC(), planned.Notes, planned.SourceURL)
	return err
}

func (s *Store) PlannedActivityExists(ctx context.Context, source, sourceID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		select exists(
			select 1 from planned_activities where user_id = $1 and source = $2 and source_id = $3
		)
	`, scopedUserID(ctx), source, sourceID).Scan(&exists)
	return exists, err
}

func (s *Store) SupersedeStaleTrainingSheetPlans(ctx context.Context, workbookID string, planYear int) (int64, error) {
	workbookID = strings.TrimSpace(workbookID)
	if workbookID == "" {
		return 0, fmt.Errorf("training sheet workbook ID is required")
	}
	start, end := trainingSheetPlanYearBounds(planYear)
	result, err := s.db.Exec(ctx, `
		update planned_activities
		set status = 'superseded', updated_at = now()
		where user_id = $1 and source = $2 and workbook_id <> $3
			and planned_date >= $4::date and planned_date < $5::date
			and status = 'pending'
	`, scopedUserID(ctx), trainingSheetProvider, workbookID, start, end)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func (s *Store) ListPlannedActivities(ctx context.Context, from, to time.Time) ([]PlannedActivity, error) {
	rows, err := s.db.Query(ctx, `
		select `+plannedActivityColumns+`
		from planned_activities
		where user_id = $1 and planned_date >= $2::date and planned_date < $3::date
			and planned_date >= current_date
			and status = 'pending'
		order by planned_date, plan_cell
	`, scopedUserID(ctx), from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	planned := make([]PlannedActivity, 0)
	for rows.Next() {
		var item PlannedActivity
		if err := scanPlannedActivity(rows, &item); err != nil {
			return nil, err
		}
		planned = append(planned, item)
	}
	return planned, rows.Err()
}

func (s *Store) PlannedActivityMatchCandidates(ctx context.Context, activityID string, windowDays int) (PlannedActivityMatchResponse, error) {
	var activitySource string
	var activitySportType string
	var activityDate time.Time
	if err := s.db.QueryRow(ctx, `select source, date(start_time), sport_type from activities where id = $1 and user_id = $2`, activityID, scopedUserID(ctx)).Scan(&activitySource, &activityDate, &activitySportType); err != nil {
		return PlannedActivityMatchResponse{}, err
	}
	if windowDays != 7 && windowDays != 30 {
		windowDays = 7
	}
	response := PlannedActivityMatchResponse{Candidates: make([]PlannedActivity, 0)}
	if activitySource == trainingSheetProvider {
		return response, nil
	}
	if !isRunningSport(activitySportType) {
		return response, nil
	}
	rows, err := s.db.Query(ctx, `
		select `+plannedActivityColumns+`
		from planned_activities
		where user_id = $3 and planned_date between ($1::date - $2::int) and ($1::date + $2::int)
			and status = 'pending'
		order by
			case when planned_date = $1::date then 0 else 1 end,
			abs(planned_date - $1::date),
			plan_cell, name
	`, activityDate, windowDays, scopedUserID(ctx))
	if err != nil {
		return response, err
	}
	defer rows.Close()
	for rows.Next() {
		var item PlannedActivity
		if err := scanPlannedActivity(rows, &item); err != nil {
			return response, err
		}
		response.Candidates = append(response.Candidates, item)
		if response.SuggestedID == "" {
			response.SuggestedID = item.ID
		}
	}
	if err := rows.Err(); err != nil {
		return response, err
	}
	if windowDays == 7 {
		if err := s.db.QueryRow(ctx, `
			select exists(
				select 1
				from planned_activities
				where user_id = $3 and status = 'pending'
					and (planned_date < ($1::date - $2::int) or planned_date > ($1::date + $2::int))
			)
		`, activityDate, windowDays, scopedUserID(ctx)).Scan(&response.HasMore); err != nil {
			return response, err
		}
	}

	var matched PlannedActivity
	err = scanPlannedActivity(s.db.QueryRow(ctx, `select `+plannedActivityColumns+` from planned_activities where user_id = $2 and matched_activity_id = $1`, activityID, scopedUserID(ctx)), &matched)
	if errors.Is(err, pgx.ErrNoRows) {
		return response, nil
	}
	if err != nil {
		return response, err
	}
	response.Matched = &matched
	response.Writeback, _ = s.GetTrainingSheetWriteback(ctx, matched.ID)
	return response, nil
}

type plannedActivityReflection struct {
	Feedback        *string
	RPESet          bool
	RPE             *int
	ManualOverrides map[string]string
}

func (s *Store) MatchPlannedActivity(ctx context.Context, activityID, plannedActivityID string) (PlannedActivity, error) {
	return s.matchPlannedActivity(ctx, activityID, plannedActivityID, nil)
}

func (s *Store) MatchPlannedActivityWithReflection(ctx context.Context, activityID, plannedActivityID string, reflection plannedActivityReflection) (PlannedActivity, error) {
	return s.matchPlannedActivity(ctx, activityID, plannedActivityID, &reflection)
}

func (s *Store) matchPlannedActivity(ctx context.Context, activityID, plannedActivityID string, reflection *plannedActivityReflection) (PlannedActivity, error) {
	feedback := ""
	manualOverrides := []byte(`{}`)
	manualOverridesSet := reflection != nil
	if reflection != nil && reflection.Feedback != nil {
		var err error
		feedback, err = localActivityFeedbackValue(*reflection.Feedback)
		if err != nil {
			return PlannedActivity{}, err
		}
	}
	if reflection != nil && reflection.ManualOverrides != nil {
		var err error
		manualOverrides, err = json.Marshal(reflection.ManualOverrides)
		if err != nil {
			return PlannedActivity{}, err
		}
	}
	if reflection != nil && reflection.RPESet && reflection.RPE != nil && (*reflection.RPE < 1 || *reflection.RPE > 10) {
		return PlannedActivity{}, ErrInvalidActivityRPE
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PlannedActivity{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	var activitySource string
	var activitySportType string
	var activityDate time.Time
	if err = tx.QueryRow(ctx, `select source, date(start_time), sport_type from activities where id = $1 and user_id = $2 for update`, activityID, scopedUserID(ctx)).Scan(&activitySource, &activityDate, &activitySportType); err != nil {
		return PlannedActivity{}, err
	}
	if activitySource == trainingSheetProvider {
		return PlannedActivity{}, errPlannedMatchInvalid
	}
	if !isRunningSport(activitySportType) {
		return PlannedActivity{}, errPlannedMatchInvalid
	}

	var planned PlannedActivity
	if err = scanPlannedActivity(tx.QueryRow(ctx, `select `+plannedActivityColumns+` from planned_activities where id = $1 and user_id = $2 for update`, plannedActivityID, scopedUserID(ctx)), &planned); err != nil {
		return PlannedActivity{}, err
	}
	updateReflection := func() error {
		if reflection == nil || (reflection.Feedback == nil && !reflection.RPESet) {
			return nil
		}
		_, updateErr := tx.Exec(ctx, `
			update activities
			set local_feedback = case when $2 then $3 else local_feedback end,
				rpe = case when $4 then $5 else rpe end,
				updated_at = now()
			where id = $1 and user_id = $6
		`, activityID, reflection.Feedback != nil, feedback, reflection.RPESet, reflection.RPE, scopedUserID(ctx))
		return updateErr
	}
	if planned.MatchedActivityID != "" {
		if planned.MatchedActivityID == activityID {
			if err = updateReflection(); err != nil {
				return PlannedActivity{}, err
			}
			if _, err = tx.Exec(ctx, `
				insert into training_sheet_writebacks(planned_activity_id, activity_id, manual_overrides)
				values($1, $2, $3::jsonb)
				on conflict(planned_activity_id) do update set
					activity_id = excluded.activity_id,
					manual_overrides = case when $4 then excluded.manual_overrides else training_sheet_writebacks.manual_overrides end,
					updated_at = now()
			`, planned.ID, activityID, manualOverrides, manualOverridesSet); err != nil {
				return PlannedActivity{}, err
			}
			if err = tx.Commit(ctx); err != nil {
				return PlannedActivity{}, err
			}
			committed = true
			return planned, nil
		}
		return PlannedActivity{}, errPlannedMatchConflict
	}
	if planned.Status != plannedActivityStatusPending {
		return PlannedActivity{}, errPlannedMatchConflict
	}
	if err = updateReflection(); err != nil {
		return PlannedActivity{}, err
	}
	matchedAt := time.Now().UTC()
	if _, err = tx.Exec(ctx, `
		update planned_activities
		set status = 'completed', matched_activity_id = $2, matched_at = $3, updated_at = now()
		where id = $1 and user_id = $4
	`, plannedActivityID, activityID, matchedAt, scopedUserID(ctx)); err != nil {
		return PlannedActivity{}, err
	}
	planned.Status = plannedActivityStatusCompleted
	planned.MatchedActivityID = activityID
	planned.MatchedAt = &matchedAt
	if _, err = tx.Exec(ctx, `
		insert into training_sheet_writebacks(planned_activity_id, activity_id, manual_overrides)
		values($1, $2, $3::jsonb)
		on conflict(planned_activity_id) do update set
			activity_id = excluded.activity_id,
			manual_overrides = case when $4 then excluded.manual_overrides else training_sheet_writebacks.manual_overrides end,
			updated_at = now()
	`, planned.ID, activityID, manualOverrides, manualOverridesSet); err != nil {
		return PlannedActivity{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PlannedActivity{}, err
	}
	committed = true
	return planned, nil
}

func (s *Store) UnmatchPlannedActivity(ctx context.Context, activityID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	var source, workbookID string
	err = tx.QueryRow(ctx, `
		select source, workbook_id
		from planned_activities
		where matched_activity_id = $1 and user_id = $2
		for update
	`, activityID, scopedUserID(ctx)).Scan(&source, &workbookID)
	if err == nil {
		currentWorkbookID, workbookErr := configuredTrainingSheetWorkbookID(ctx, tx)
		if workbookErr != nil {
			return workbookErr
		}
		status := plannedActivityStatusAfterUnmatch(source, workbookID, currentWorkbookID)
		_, err = tx.Exec(ctx, `
			update planned_activities
			set status = $3, matched_activity_id = null, matched_at = null, updated_at = now()
			where matched_activity_id = $1 and user_id = $2
		`, activityID, scopedUserID(ctx), status)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err = tx.Exec(ctx, `delete from training_sheet_writebacks where activity_id = $1 and exists (select 1 from activities where activities.id = $1 and activities.user_id = $2)`, activityID, scopedUserID(ctx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

const plannedActivityColumns = `
	id::text, source, source_id, workbook_id, sheet_id, sheet_title, plan_cell,
	feedback_cell, planned_date, name, sport_type, notes, status, source_url, raw,
	matched_activity_id::text, matched_at, created_at, updated_at`

func scanPlannedActivity(row interface{ Scan(...any) error }, item *PlannedActivity) error {
	var rawBytes []byte
	var matchedActivityID sql.NullString
	var matchedAt sql.NullTime
	if err := row.Scan(
		&item.ID, &item.Source, &item.SourceID, &item.WorkbookID, &item.SheetID, &item.SheetTitle,
		&item.PlanCell, &item.FeedbackCell, &item.PlannedDate, &item.Name, &item.SportType, &item.Notes, &item.Status,
		&item.SourceURL, &rawBytes, &matchedActivityID, &matchedAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return err
	}
	if matchedActivityID.Valid {
		item.MatchedActivityID = matchedActivityID.String
	}
	if matchedAt.Valid {
		item.MatchedAt = &matchedAt.Time
	}
	if len(rawBytes) > 0 {
		_ = json.Unmarshal(rawBytes, &item.Raw)
	}
	if item.Raw == nil {
		item.Raw = map[string]any{}
	}
	return nil
}

func (s *Store) GetTrainingSheetPlanYear(ctx context.Context) (int, error) {
	var year int
	err := s.db.QueryRow(ctx, `select coalesce(plan_year, 0) from user_settings where user_id = $1`, scopedUserID(ctx)).Scan(&year)
	if errors.Is(err, pgx.ErrNoRows) || year <= 0 {
		return time.Now().UTC().Year(), nil
	}
	return year, err
}

func configuredTrainingSheetWorkbookID(ctx context.Context, queryer pgxQueryRower) (string, error) {
	var sheetURL string
	err := queryer.QueryRow(ctx, `
		select coalesce(training_sheet_sheet_url, '')
		from user_settings
		where user_id = $1
	`, scopedUserID(ctx)).Scan(&sheetURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	sheetID, _, err := parseTrainingSheetID(sheetURL)
	if err != nil {
		return "", nil
	}
	return sheetID, nil
}

func plannedActivityStatusAfterUnmatch(source, workbookID, currentWorkbookID string) string {
	if source == trainingSheetProvider && strings.TrimSpace(currentWorkbookID) != "" && strings.TrimSpace(workbookID) != strings.TrimSpace(currentWorkbookID) {
		return plannedActivityStatusSuperseded
	}
	return plannedActivityStatusPending
}

func trainingSheetPlanYearBounds(planYear int) (time.Time, time.Time) {
	start := time.Date(planYear, time.January, 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(1, 0, 0)
}

func (s *Store) SetTrainingSheetPlanYear(ctx context.Context, year int) error {
	if year < 1900 || year > 9999 {
		return fmt.Errorf("plan year must be between 1900 and 9999")
	}
	_, err := s.db.Exec(ctx, `update user_settings set plan_year = $2, updated_at = now() where user_id = $1`, scopedUserID(ctx), year)
	return err
}

func (s *Store) SaveGoogleSheetsTokens(ctx context.Context, accessToken, refreshToken []byte, expiresAt *time.Time, scopes []string) error {
	_, err := s.db.Exec(ctx, `
		insert into google_sheets_tokens(id, user_id, access_token_ciphertext, refresh_token_ciphertext, token_expires_at, scopes, updated_at)
		values($1, $2, $3, $4, $5, $6, now())
		on conflict(user_id) do update set
			access_token_ciphertext = excluded.access_token_ciphertext,
			refresh_token_ciphertext = excluded.refresh_token_ciphertext,
			token_expires_at = excluded.token_expires_at,
			scopes = excluded.scopes,
			updated_at = now()
	`, googleSheetsTokenID+":"+scopedUserID(ctx), scopedUserID(ctx), accessToken, refreshToken, expiresAt, scopes)
	return err
}

func (s *Store) LoadGoogleSheetsTokens(ctx context.Context) (googleSheetsTokenRecord, error) {
	var record googleSheetsTokenRecord
	var expires sql.NullTime
	err := s.db.QueryRow(ctx, `
		select access_token_ciphertext, refresh_token_ciphertext, token_expires_at, scopes
		from google_sheets_tokens where user_id = $1
	`, scopedUserID(ctx)).Scan(&record.AccessTokenCiphertext, &record.RefreshTokenCiphertext, &expires, &record.Scopes)
	if err != nil {
		return record, err
	}
	if expires.Valid {
		record.TokenExpiresAt = &expires.Time
	}
	return record, nil
}

func (s *Store) DeleteGoogleSheetsTokens(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `delete from google_sheets_tokens where user_id = $1`, scopedUserID(ctx))
	return err
}

func (s *Store) CreateGoogleOAuthState(ctx context.Context, state, sessionID string, expiresAt time.Time) error {
	if _, err := s.db.Exec(ctx, `delete from google_oauth_states where expires_at < now()`); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `insert into google_oauth_states(state, session_id, user_id, expires_at) values($1, $2, $3, $4)`, state, sessionID, scopedUserID(ctx), expiresAt)
	return err
}

func (s *Store) ConsumeGoogleOAuthState(ctx context.Context, state string) (string, string, error) {
	var sessionID, userID string
	err := s.db.QueryRow(ctx, `
		delete from google_oauth_states
		where state = $1 and expires_at > now()
		returning session_id, user_id::text
	`, state).Scan(&sessionID, &userID)
	return sessionID, userID, err
}
