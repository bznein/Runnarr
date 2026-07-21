package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	adminRole = "admin"
	userRole  = "user"
)

type SessionRecord struct {
	CSRF      string
	Actor     User
	Effective User
	Support   bool
}

func normalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeDisplayName(username, displayName string) string {
	if value := strings.TrimSpace(displayName); value != "" {
		return value
	}
	return username
}

func (s *Store) EnsureBootstrap(ctx context.Context, username, passwordHash string) error {
	username = normalizeUsername(username)
	if username == "" || passwordHash == "" {
		return errors.New("bootstrap admin credentials are required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var userID string
	err = tx.QueryRow(ctx, `select id::text from users order by created_at, id limit 1`).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			insert into users(username, display_name, role, password_hash)
			values($1, $2, $3, $4)
			returning id::text
		`, username, username, adminRole, passwordHash).Scan(&userID)
	}
	if err != nil {
		return err
	}

	legacyAssignments := []string{
		"auth_sessions",
		"import_files",
		"provider_connections",
		"activities",
		"sync_jobs",
		"sync_excluded_activities",
		"daily_health_metrics",
		"gears",
		"planned_activities",
		"google_sheets_tokens",
	}
	for _, table := range legacyAssignments {
		if _, err = tx.Exec(ctx, fmt.Sprintf("update %s set user_id = $1 where user_id is null", table), userID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `
		update google_oauth_states states
		set user_id = sessions.user_id
		from auth_sessions sessions
		where states.session_id = sessions.id::text and states.user_id is null
	`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `update google_oauth_states set user_id = $1 where user_id is null`, userID); err != nil {
		return err
	}

	if err = ensureUserSettingsTx(ctx, tx, userID); err != nil {
		return err
	}
	if err = ensureUserOwnershipTx(ctx, tx); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func ensureUserSettingsTx(ctx context.Context, tx pgx.Tx, userID string) error {
	_, err := tx.Exec(ctx, `
		insert into user_settings(
			user_id, climb_smoothing_radius_m, min_climb_distance_m,
			min_climb_elevation_gain_m, min_climb_average_grade_pct,
			max_climb_merge_dip_distance_m, max_climb_merge_elevation_loss_m,
			climb_start_gain_m, climb_detection_preset,
			training_sheet_enabled, training_sheet_sheet_url,
			training_sheet_check_every_hours, training_sheet_last_synced_at,
			plan_year
		)
		select $1, climb_smoothing_radius_m, min_climb_distance_m,
			min_climb_elevation_gain_m, min_climb_average_grade_pct,
			max_climb_merge_dip_distance_m, max_climb_merge_elevation_loss_m,
			climb_start_gain_m, climb_detection_preset,
			training_sheet_enabled, training_sheet_sheet_url,
			training_sheet_check_every_hours, training_sheet_last_synced_at,
			plan_year
		from app_settings
		where id = $2
		on conflict(user_id) do nothing
	`, userID, appSettingsID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		insert into user_settings(
			user_id, climb_smoothing_radius_m, min_climb_distance_m,
			min_climb_elevation_gain_m, min_climb_average_grade_pct,
			max_climb_merge_dip_distance_m, max_climb_merge_elevation_loss_m,
			climb_start_gain_m, climb_detection_preset
		)
		values($1, 75, 300, 15, 2.5, 150, 8, 3, 'balanced')
		on conflict(user_id) do nothing
	`, userID)
	return err
}

func ensureUserOwnershipTx(ctx context.Context, tx pgx.Tx) error {
	for _, statement := range []string{
		`alter table auth_sessions alter column user_id set not null`,
		`alter table import_files alter column user_id set not null`,
		`alter table provider_connections alter column user_id set not null`,
		`alter table activities alter column user_id set not null`,
		`alter table sync_jobs alter column user_id set not null`,
		`alter table sync_excluded_activities alter column user_id set not null`,
		`alter table daily_health_metrics alter column user_id set not null`,
		`alter table gears alter column user_id set not null`,
		`alter table planned_activities alter column user_id set not null`,
		`alter table google_sheets_tokens alter column user_id set not null`,
		`alter table google_oauth_states alter column user_id set not null`,
		`create unique index if not exists import_files_user_sha256_idx on import_files(user_id, sha256)`,
		`create unique index if not exists provider_connections_user_provider_idx on provider_connections(user_id, provider)`,
		`create unique index if not exists activities_user_source_idx on activities(user_id, source, source_id)`,
		`create unique index if not exists sync_excluded_activities_user_source_idx on sync_excluded_activities(user_id, source, source_id)`,
		`create unique index if not exists daily_health_metrics_user_date_idx on daily_health_metrics(user_id, provider, metric_date)`,
		`create unique index if not exists gears_user_provider_idx on gears(user_id, provider, provider_gear_id)`,
		`create unique index if not exists planned_activities_user_source_idx on planned_activities(user_id, source, source_id)`,
		`create unique index if not exists google_sheets_tokens_user_idx on google_sheets_tokens(user_id)`,
		`alter table auth_sessions add constraint auth_sessions_user_fk foreign key (user_id) references users(id)`,
		`alter table auth_sessions add constraint auth_sessions_support_user_fk foreign key (support_user_id) references users(id)`,
		`alter table import_files add constraint import_files_user_fk foreign key (user_id) references users(id)`,
		`alter table provider_connections add constraint provider_connections_user_fk foreign key (user_id) references users(id)`,
		`alter table activities add constraint activities_user_fk foreign key (user_id) references users(id)`,
		`alter table sync_jobs add constraint sync_jobs_user_fk foreign key (user_id) references users(id)`,
		`alter table sync_excluded_activities add constraint sync_excluded_activities_user_fk foreign key (user_id) references users(id)`,
		`alter table daily_health_metrics add constraint daily_health_metrics_user_fk foreign key (user_id) references users(id)`,
		`alter table gears add constraint gears_user_fk foreign key (user_id) references users(id)`,
		`alter table planned_activities add constraint planned_activities_user_fk foreign key (user_id) references users(id)`,
		`alter table google_sheets_tokens add constraint google_sheets_tokens_user_fk foreign key (user_id) references users(id)`,
		`alter table google_oauth_states add constraint google_oauth_states_user_fk foreign key (user_id) references users(id)`,
	} {
		if strings.Contains(statement, " add constraint ") {
			parts := strings.Fields(statement)
			constraintName := parts[5]
			if _, err := tx.Exec(ctx, fmt.Sprintf(`do $$ begin
				if not exists (select 1 from pg_constraint where conname = '%s') then
					%s;
				end if;
			end $$`, constraintName, statement)); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	return scanUser(s.db.QueryRow(ctx, userSelectSQL+` where lower(username) = lower($1)`, normalizeUsername(username)))
}

func (s *Store) GetUser(ctx context.Context, id string) (User, error) {
	return scanUser(s.db.QueryRow(ctx, userSelectSQL+` where id = $1`, id))
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.Query(ctx, userSelectSQL+` order by disabled, lower(username)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, username, displayName, role, passwordHash string) (User, error) {
	username = normalizeUsername(username)
	role = strings.TrimSpace(role)
	if role != adminRole {
		role = userRole
	}
	user, err := scanUser(s.db.QueryRow(ctx, `
		insert into users(username, display_name, role, password_hash)
		values($1, $2, $3, $4)
		returning `+userSelectColumns, username, normalizeDisplayName(username, displayName), role, passwordHash))
	if err != nil {
		return User{}, err
	}
	if err := s.ensureUserSettings(ctx, user.ID); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) UpdateUser(ctx context.Context, id, displayName, role string, disabled *bool) (User, error) {
	role = strings.TrimSpace(role)
	if role != adminRole {
		role = userRole
	}
	var user User
	err := s.db.QueryRow(ctx, `
		update users
		set display_name = $2, role = $3, disabled = coalesce($4, disabled), updated_at = now()
		where id = $1
		returning `+userSelectColumns, id, strings.TrimSpace(displayName), role, disabled).Scan(
		&user.ID, &user.Username, &user.DisplayName, &user.Role, &user.Disabled,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func (s *Store) SetUserPassword(ctx context.Context, id, passwordHash string) error {
	var updatedID string
	err := s.db.QueryRow(ctx, `
		update users
		set password_hash = $2, updated_at = now()
		where id = $1
		returning id::text
	`, id, passwordHash).Scan(&updatedID)
	return err
}

func (s *Store) PasswordHash(ctx context.Context, id string) (string, error) {
	var hash string
	err := s.db.QueryRow(ctx, `select password_hash from users where id = $1`, id).Scan(&hash)
	return hash, err
}

func (s *Store) TouchUserLogin(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `update users set last_login_at = now(), updated_at = now() where id = $1`, id)
	return err
}

func (s *Store) CountActiveAdmins(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `select count(*) from users where role = 'admin' and not disabled`).Scan(&count)
	return count, err
}

func (s *Store) DisableUserSessions(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `delete from auth_sessions where user_id = $1`, id)
	return err
}

func (s *Store) ensureUserSettings(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `
		insert into user_settings(user_id, climb_smoothing_radius_m, min_climb_distance_m,
			min_climb_elevation_gain_m, min_climb_average_grade_pct,
			max_climb_merge_dip_distance_m, max_climb_merge_elevation_loss_m,
			climb_start_gain_m, climb_detection_preset)
		values($1, 75, 300, 15, 2.5, 150, 8, 3, 'balanced')
		on conflict(user_id) do nothing
	`, id)
	return err
}

func (s *Store) GetUserPreferences(ctx context.Context) (UserPreference, error) {
	var preference UserPreference
	err := s.db.QueryRow(ctx, `
		select theme_preference, activity_table_columns, gear_sort_by
		from user_settings where user_id = $1
	`, scopedUserID(ctx)).Scan(&preference.ThemePreference, &preference.ActivityTableColumns, &preference.GearSortBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserPreference{ThemePreference: "system", GearSortBy: "distance_percent"}, nil
	}
	return preference, err
}

func (s *Store) UpdateUserPreferences(ctx context.Context, preference UserPreference) error {
	theme := strings.TrimSpace(preference.ThemePreference)
	if theme != "light" && theme != "dark" && theme != "system" {
		theme = "system"
	}
	gearSort := strings.TrimSpace(preference.GearSortBy)
	if gearSort == "" {
		gearSort = "distance_percent"
	}
	columns := preference.ActivityTableColumns
	if columns == nil {
		columns = []string{}
	}
	_, err := s.db.Exec(ctx, `
		update user_settings
		set theme_preference = $2, activity_table_columns = $3, gear_sort_by = $4, updated_at = now()
		where user_id = $1
	`, scopedUserID(ctx), theme, columns, gearSort)
	return err
}

const userSelectColumns = `id::text, username, display_name, role, disabled, last_login_at, created_at, updated_at`
const userSelectSQL = `select ` + userSelectColumns + ` from users`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role, &user.Disabled,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func (s *Store) CreateSession(ctx context.Context, userID, csrf string, ttl time.Duration) (string, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		insert into auth_sessions(user_id, csrf_token, expires_at)
		values($1, $2, now() + $3::interval)
		returning id::text
	`, userID, csrf, fmt.Sprintf("%d seconds", int(ttl.Seconds()))).Scan(&id)
	return id, err
}

func (s *Store) GetSessionRecord(ctx context.Context, id string) (SessionRecord, error) {
	var record SessionRecord
	var actor, effective User
	var supportID *string
	err := s.db.QueryRow(ctx, `
		update auth_sessions sessions
		set last_seen_at = now()
		from users actor
		where sessions.id = $1 and sessions.expires_at > now()
			and sessions.user_id = actor.id and not actor.disabled
		returning sessions.csrf_token, actor.id::text, actor.username, actor.display_name,
			actor.role, actor.disabled, actor.last_login_at, actor.created_at, actor.updated_at,
			sessions.support_user_id
	`, id).Scan(&record.CSRF, &actor.ID, &actor.Username, &actor.DisplayName, &actor.Role,
		&actor.Disabled, &actor.LastLoginAt, &actor.CreatedAt, &actor.UpdatedAt, &supportID)
	if err != nil {
		return record, err
	}
	record.Actor = actor
	record.Effective = actor
	if supportID != nil && *supportID != "" {
		effective, err = s.GetUser(ctx, *supportID)
		if err != nil {
			return record, err
		}
		record.Effective = effective
		record.Support = true
	}
	return record, nil
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `delete from auth_sessions where id = $1`, id)
	return err
}

func (s *Store) SetSessionSupport(ctx context.Context, sessionID, targetID string) error {
	_, err := s.db.Exec(ctx, `
		update auth_sessions sessions
		set support_user_id = $3
		from users actor, users target
		where sessions.id = $1 and sessions.user_id = actor.id and actor.role = 'admin'
			and target.id = $2
	`, sessionID, targetID, targetID)
	return err
}

func (s *Store) ClearSessionSupport(ctx context.Context, sessionID string) error {
	_, err := s.db.Exec(ctx, `update auth_sessions set support_user_id = null where id = $1`, sessionID)
	return err
}
