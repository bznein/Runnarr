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

const googleSheetsTokenID = "default"

func (s *Store) GetTrainingSheetConfig(ctx context.Context) (TrainingSheetConfig, error) {
	config := TrainingSheetConfig{CheckEveryHours: 24, PlanYear: time.Now().UTC().Year()}
	var lastSynced sql.NullTime
	err := s.db.QueryRow(ctx, `
		select training_sheet_enabled, training_sheet_sheet_url,
			training_sheet_check_every_hours, plan_year, training_sheet_last_synced_at
		from app_settings where id = $1
	`, appSettingsID).Scan(&config.Enabled, &config.SheetURL, &config.CheckEveryHours, &config.PlanYear, &lastSynced)
	if errors.Is(err, pgx.ErrNoRows) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	config.SheetURL = strings.TrimSpace(config.SheetURL)
	if config.CheckEveryHours <= 0 || config.CheckEveryHours > 720 { config.CheckEveryHours = 24 }
	if config.PlanYear <= 0 { config.PlanYear = time.Now().UTC().Year() }
	if lastSynced.Valid { config.LastSyncedAt = lastSynced.Time.UTC().Format(time.RFC3339) }
	return config, nil
}

func (s *Store) SetTrainingSheetConfig(ctx context.Context, config TrainingSheetConfig) error {
	checkEveryHours := config.CheckEveryHours
	if checkEveryHours <= 0 || checkEveryHours > 720 { checkEveryHours = 24 }
	planYear := config.PlanYear
	if planYear <= 0 { planYear = time.Now().UTC().Year() }
	_, err := s.db.Exec(ctx, `
		update app_settings
		set training_sheet_enabled = $2, training_sheet_sheet_url = $3,
			training_sheet_check_every_hours = $4, plan_year = $5, updated_at = now()
		where id = $1
	`, appSettingsID, config.Enabled, strings.TrimSpace(config.SheetURL), checkEveryHours, planYear)
	return err
}

func (s *Store) TouchTrainingSheetConfigLastSyncedAt(ctx context.Context, syncedAt time.Time) error {
	_, err := s.db.Exec(ctx, `update app_settings set training_sheet_last_synced_at = $2, updated_at = now() where id = $1`, appSettingsID, syncedAt)
	return err
}

func (s *Store) LatestTrainingSheetScheduledSync(ctx context.Context) (time.Time, error) {
	var createdAt time.Time
	err := s.db.QueryRow(ctx, `
		select created_at from sync_jobs
		where provider = $1 and kind = 'scheduled'
		order by created_at desc limit 1
	`, trainingSheetProvider).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) { return time.Time{}, nil }
	return createdAt, err
}

type googleSheetsTokenRecord struct {
	AccessTokenCiphertext  []byte
	RefreshTokenCiphertext []byte
	TokenExpiresAt         *time.Time
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
			source, source_id, workbook_id, sheet_id, sheet_title, plan_cell,
			planned_date, name, sport_type, notes, status, source_url, raw,
			last_seen_at, updated_at
		) values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now(), now())
		on conflict(source, source_id) do update set
			workbook_id = excluded.workbook_id,
			sheet_id = excluded.sheet_id,
			sheet_title = excluded.sheet_title,
			plan_cell = excluded.plan_cell,
			planned_date = excluded.planned_date,
			name = excluded.name,
			sport_type = excluded.sport_type,
			notes = excluded.notes,
			source_url = excluded.source_url,
			raw = excluded.raw,
			last_seen_at = now(),
			updated_at = now()
	`, planned.Source, planned.SourceID, planned.WorkbookID, planned.SheetID, planned.SheetTitle,
		planned.PlanCell, planned.PlannedDate, planned.Name, planned.SportType, planned.Notes,
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
		set name = $3, sport_type = $4, start_time = $5, local_notes = $6,
			original_provider_url = $7, updated_at = now()
		where source = $1 and source_id = $2
	`, planned.Source, planned.SourceID, planned.Name, planned.SportType, planned.PlannedDate.UTC(), planned.Notes, planned.SourceURL)
	return err
}

func (s *Store) ListPlannedActivities(ctx context.Context, from, to time.Time) ([]PlannedActivity, error) {
	rows, err := s.db.Query(ctx, `
		select id::text, source, source_id, workbook_id, sheet_id, sheet_title, plan_cell,
			planned_date, name, sport_type, notes, status, source_url, raw, created_at, updated_at
		from planned_activities
		where planned_date >= $1::date and planned_date < $2::date
		order by planned_date, plan_cell
	`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	planned := make([]PlannedActivity, 0)
	for rows.Next() {
		var item PlannedActivity
		var rawBytes []byte
		if err := rows.Scan(&item.ID, &item.Source, &item.SourceID, &item.WorkbookID, &item.SheetID,
			&item.SheetTitle, &item.PlanCell, &item.PlannedDate, &item.Name, &item.SportType,
			&item.Notes, &item.Status, &item.SourceURL, &rawBytes, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if len(rawBytes) > 0 {
			_ = json.Unmarshal(rawBytes, &item.Raw)
		}
		if item.Raw == nil {
			item.Raw = map[string]any{}
		}
		planned = append(planned, item)
	}
	return planned, rows.Err()
}

func (s *Store) GetTrainingSheetPlanYear(ctx context.Context) (int, error) {
	var year int
	err := s.db.QueryRow(ctx, `select coalesce(plan_year, 0) from app_settings where id = $1`, appSettingsID).Scan(&year)
	if errors.Is(err, pgx.ErrNoRows) || year <= 0 {
		return time.Now().UTC().Year(), nil
	}
	return year, err
}

func (s *Store) SetTrainingSheetPlanYear(ctx context.Context, year int) error {
	if year < 1900 || year > 9999 {
		return fmt.Errorf("plan year must be between 1900 and 9999")
	}
	_, err := s.db.Exec(ctx, `update app_settings set plan_year = $2, updated_at = now() where id = $1`, appSettingsID, year)
	return err
}

func (s *Store) SaveGoogleSheetsTokens(ctx context.Context, accessToken, refreshToken []byte, expiresAt *time.Time) error {
	_, err := s.db.Exec(ctx, `
		insert into google_sheets_tokens(id, access_token_ciphertext, refresh_token_ciphertext, token_expires_at, updated_at)
		values($1, $2, $3, $4, now())
		on conflict(id) do update set
			access_token_ciphertext = excluded.access_token_ciphertext,
			refresh_token_ciphertext = excluded.refresh_token_ciphertext,
			token_expires_at = excluded.token_expires_at,
			updated_at = now()
	`, googleSheetsTokenID, accessToken, refreshToken, expiresAt)
	return err
}

func (s *Store) LoadGoogleSheetsTokens(ctx context.Context) (googleSheetsTokenRecord, error) {
	var record googleSheetsTokenRecord
	var expires sql.NullTime
	err := s.db.QueryRow(ctx, `
		select access_token_ciphertext, refresh_token_ciphertext, token_expires_at
		from google_sheets_tokens where id = $1
	`, googleSheetsTokenID).Scan(&record.AccessTokenCiphertext, &record.RefreshTokenCiphertext, &expires)
	if err != nil {
		return record, err
	}
	if expires.Valid {
		record.TokenExpiresAt = &expires.Time
	}
	return record, nil
}

func (s *Store) DeleteGoogleSheetsTokens(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `delete from google_sheets_tokens where id = $1`, googleSheetsTokenID)
	return err
}

func (s *Store) CreateGoogleOAuthState(ctx context.Context, state, sessionID string, expiresAt time.Time) error {
	if _, err := s.db.Exec(ctx, `delete from google_oauth_states where expires_at < now()`); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `insert into google_oauth_states(state, session_id, expires_at) values($1, $2, $3)`, state, sessionID, expiresAt)
	return err
}

func (s *Store) ConsumeGoogleOAuthState(ctx context.Context, state string) (string, error) {
	var sessionID string
	err := s.db.QueryRow(ctx, `
		delete from google_oauth_states
		where state = $1 and expires_at > now()
		returning session_id
	`, state).Scan(&sessionID)
	return sessionID, err
}
