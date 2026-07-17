package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

var ErrActivitySyncExcluded = errors.New("activity is excluded from provider sync")
var ErrInvalidActivityName = errors.New("activity name must be between 1 and 160 characters")

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) CreateSession(ctx context.Context, csrf string, ttl time.Duration) (string, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		insert into auth_sessions(csrf_token, expires_at)
		values($1, now() + $2::interval)
		returning id::text
	`, csrf, fmt.Sprintf("%d seconds", int(ttl.Seconds()))).Scan(&id)
	return id, err
}

func (s *Store) GetSession(ctx context.Context, id string) (csrf string, err error) {
	err = s.db.QueryRow(ctx, `
		update auth_sessions
		set last_seen_at = now()
		where id = $1 and expires_at > now()
		returning csrf_token
	`, id).Scan(&csrf)
	return csrf, err
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `delete from auth_sessions where id = $1`, id)
	return err
}

func (s *Store) UpsertImportFile(ctx context.Context, file ImportFile) (ImportFile, error) {
	var saved ImportFile
	err := s.db.QueryRow(ctx, `
		insert into import_files(filename, content_type, sha256, size_bytes, parser, status, error)
		values($1, $2, $3, $4, $5, $6, $7)
		on conflict (sha256) do update set
			filename = excluded.filename,
			content_type = excluded.content_type,
			parser = excluded.parser
		returning id::text, filename, content_type, sha256, size_bytes, parser, status, error, created_at
	`, file.Filename, file.ContentType, file.SHA256, file.SizeBytes, file.Parser, file.Status, file.Error).
		Scan(&saved.ID, &saved.Filename, &saved.ContentType, &saved.SHA256, &saved.SizeBytes, &saved.Parser, &saved.Status, &saved.Error, &saved.CreatedAt)
	return saved, err
}

func (s *Store) UpdateImportStatus(ctx context.Context, id, status, message string) error {
	_, err := s.db.Exec(ctx, `
		update import_files
		set status = $2, error = $3
		where id = $1
	`, id, status, message)
	return err
}

func (s *Store) ListImports(ctx context.Context) ([]ImportFile, error) {
	rows, err := s.db.Query(ctx, `
		select id::text, filename, content_type, sha256, size_bytes, parser, status, error, created_at
		from import_files
		order by created_at desc
		limit 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ImportFile, 0)
	for rows.Next() {
		var item ImportFile
		if err := rows.Scan(&item.ID, &item.Filename, &item.ContentType, &item.SHA256, &item.SizeBytes, &item.Parser, &item.Status, &item.Error, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertDailyHealthMetric(ctx context.Context, metric DailyHealthMetric) (DailyHealthMetric, error) {
	raw := metric.Raw
	if raw == nil {
		raw = map[string]any{}
	}
	rawBytes, err := json.Marshal(raw)
	if err != nil {
		return DailyHealthMetric{}, err
	}

	row := s.db.QueryRow(ctx, `
		insert into daily_health_metrics(
			provider,
			metric_date,
			steps,
			total_calories_kcal,
			active_calories_kcal,
			resting_heart_rate_bpm,
			avg_heart_rate_bpm,
			max_heart_rate_bpm,
			sleep_duration_s,
			deep_sleep_s,
			light_sleep_s,
			rem_sleep_s,
			awake_sleep_s,
			sleep_score,
			stress_avg,
			stress_max,
			body_battery_avg,
			body_battery_min,
			body_battery_max,
			body_battery_start,
			body_battery_end,
			body_battery_gained,
			body_battery_drained,
			hrv_avg_ms,
			hrv_status,
			weight_kg,
			body_fat_pct,
			raw
		)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28)
		on conflict(provider, metric_date) do update set
			steps = excluded.steps,
			total_calories_kcal = excluded.total_calories_kcal,
			active_calories_kcal = excluded.active_calories_kcal,
			resting_heart_rate_bpm = excluded.resting_heart_rate_bpm,
			avg_heart_rate_bpm = excluded.avg_heart_rate_bpm,
			max_heart_rate_bpm = excluded.max_heart_rate_bpm,
			sleep_duration_s = excluded.sleep_duration_s,
			deep_sleep_s = excluded.deep_sleep_s,
			light_sleep_s = excluded.light_sleep_s,
			rem_sleep_s = excluded.rem_sleep_s,
			awake_sleep_s = excluded.awake_sleep_s,
			sleep_score = excluded.sleep_score,
			stress_avg = excluded.stress_avg,
			stress_max = excluded.stress_max,
			body_battery_avg = excluded.body_battery_avg,
			body_battery_min = excluded.body_battery_min,
			body_battery_max = excluded.body_battery_max,
			body_battery_start = excluded.body_battery_start,
			body_battery_end = excluded.body_battery_end,
			body_battery_gained = excluded.body_battery_gained,
			body_battery_drained = excluded.body_battery_drained,
			hrv_avg_ms = excluded.hrv_avg_ms,
			hrv_status = excluded.hrv_status,
			weight_kg = excluded.weight_kg,
			body_fat_pct = excluded.body_fat_pct,
			raw = excluded.raw,
			updated_at = now()
		returning
			id::text,
			provider,
			to_char(metric_date, 'YYYY-MM-DD'),
			steps,
			total_calories_kcal,
			active_calories_kcal,
			resting_heart_rate_bpm,
			avg_heart_rate_bpm,
			max_heart_rate_bpm,
			sleep_duration_s,
			deep_sleep_s,
			light_sleep_s,
			rem_sleep_s,
			awake_sleep_s,
			sleep_score,
			stress_avg,
			stress_max,
			body_battery_avg,
			body_battery_min,
			body_battery_max,
			body_battery_start,
			body_battery_end,
			body_battery_gained,
			body_battery_drained,
			hrv_avg_ms,
			hrv_status,
			weight_kg,
			body_fat_pct,
			raw,
			created_at,
			updated_at
	`,
		metric.Provider,
		metric.Date,
		optionalInt(metric.Steps),
		optionalInt(metric.TotalCaloriesKcal),
		optionalInt(metric.ActiveCaloriesKcal),
		optionalFloat(metric.RestingHeartRateBPM),
		optionalFloat(metric.AvgHeartRateBPM),
		optionalFloat(metric.MaxHeartRateBPM),
		optionalInt(metric.SleepDurationS),
		optionalInt(metric.DeepSleepS),
		optionalInt(metric.LightSleepS),
		optionalInt(metric.REMSleepS),
		optionalInt(metric.AwakeSleepS),
		optionalFloat(metric.SleepScore),
		optionalFloat(metric.StressAvg),
		optionalFloat(metric.StressMax),
		optionalFloat(metric.BodyBatteryAvg),
		optionalFloat(metric.BodyBatteryMin),
		optionalFloat(metric.BodyBatteryMax),
		optionalFloat(metric.BodyBatteryStart),
		optionalFloat(metric.BodyBatteryEnd),
		optionalFloat(metric.BodyBatteryGained),
		optionalFloat(metric.BodyBatteryDrained),
		optionalFloat(metric.HRVAvgMS),
		strings.TrimSpace(metric.HRVStatus),
		optionalFloat(metric.WeightKG),
		optionalFloat(metric.BodyFatPct),
		rawBytes,
	)
	return scanDailyHealthMetric(row)
}

func (s *Store) ListDailyHealthMetrics(ctx context.Context, provider string, from, to time.Time) ([]DailyHealthMetric, error) {
	rows, err := s.db.Query(ctx, `
		select
			id::text,
			provider,
			to_char(metric_date, 'YYYY-MM-DD'),
			steps,
			total_calories_kcal,
			active_calories_kcal,
			resting_heart_rate_bpm,
			avg_heart_rate_bpm,
			max_heart_rate_bpm,
			sleep_duration_s,
			deep_sleep_s,
			light_sleep_s,
			rem_sleep_s,
			awake_sleep_s,
			sleep_score,
			stress_avg,
			stress_max,
			body_battery_avg,
			body_battery_min,
			body_battery_max,
			body_battery_start,
			body_battery_end,
			body_battery_gained,
			body_battery_drained,
			hrv_avg_ms,
			hrv_status,
			weight_kg,
			body_fat_pct,
			raw,
			created_at,
			updated_at
		from daily_health_metrics
		where provider = $1 and metric_date between $2 and $3
		order by metric_date asc
	`, provider, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]DailyHealthMetric, 0)
	for rows.Next() {
		metric, err := scanDailyHealthMetric(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, metric)
	}
	return out, rows.Err()
}

func (s *Store) SaveImportedActivity(ctx context.Context, source, sourceID string, sourceFileID *string, activity ImportedActivity) (string, error) {
	raw := activity.Raw
	if raw == nil {
		raw = map[string]any{}
	}
	rawBytes, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}

	avgPace := averagePace(activity.DistanceM, activity.MovingTimeS)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if isProviderSyncedSource(source, sourceID, sourceFileID == nil) {
		var excluded bool
		err = tx.QueryRow(ctx, `
			select exists(
				select 1
				from sync_excluded_activities
				where source = $1 and source_id = $2
			)
		`, source, sourceID).Scan(&excluded)
		if err != nil {
			return "", err
		}
		if excluded {
			err = ErrActivitySyncExcluded
			return "", err
		}
	}

	var id string
	err = tx.QueryRow(ctx, `
		insert into activities(
			source, source_id, source_file_id, name, sport_type, start_time, distance_m,
			moving_time_s, elapsed_time_s, elevation_gain_m, avg_heart_rate, max_heart_rate,
			avg_pace_s_per_km, avg_grade_adjusted_pace_s_per_km, calories_kcal, original_provider_url, summary_polyline, raw
		)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		on conflict (source, source_id) do update set
			source_file_id = excluded.source_file_id,
			name = excluded.name,
			sport_type = excluded.sport_type,
			start_time = excluded.start_time,
			distance_m = excluded.distance_m,
			moving_time_s = excluded.moving_time_s,
			elapsed_time_s = excluded.elapsed_time_s,
			elevation_gain_m = excluded.elevation_gain_m,
			avg_heart_rate = excluded.avg_heart_rate,
			max_heart_rate = excluded.max_heart_rate,
			avg_pace_s_per_km = excluded.avg_pace_s_per_km,
			avg_grade_adjusted_pace_s_per_km = excluded.avg_grade_adjusted_pace_s_per_km,
			calories_kcal = excluded.calories_kcal,
			original_provider_url = case
				when excluded.original_provider_url <> '' then excluded.original_provider_url
				else activities.original_provider_url
			end,
			summary_polyline = excluded.summary_polyline,
			raw = excluded.raw,
			updated_at = now()
		returning id::text
	`, source, sourceID, sourceFileID, fallbackName(activity), activity.SportType, activity.StartTime,
		activity.DistanceM, activity.MovingTimeS, activity.ElapsedTimeS, activity.ElevationGainM,
		activity.AvgHeartRate, activity.MaxHeartRate, avgPace, activity.AvgGradeAdjustedPaceSPKM,
		activity.CaloriesKcal, activity.OriginalProviderURL, activity.SummaryPolyline, rawBytes).Scan(&id)
	if err != nil {
		return "", err
	}

	if _, err = tx.Exec(ctx, `delete from activity_samples where activity_id = $1`, id); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `delete from activity_laps where activity_id = $1`, id); err != nil {
		return "", err
	}

	for _, sample := range activity.Samples {
		_, err = tx.Exec(ctx, `
			insert into activity_samples(
				activity_id, sample_index, timestamp, elapsed_s, distance_m, latitude,
				longitude, elevation_m, heart_rate, cadence, power, speed_mps
			) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		`, id, sample.Index, sample.Timestamp, sample.ElapsedS, sample.DistanceM, sample.Latitude,
			sample.Longitude, sample.ElevationM, sample.HeartRate, sample.Cadence, sample.Power, sample.SpeedMPS)
		if err != nil {
			return "", err
		}
	}

	for _, lap := range activity.Laps {
		_, err = tx.Exec(ctx, `
			insert into activity_laps(
				activity_id, lap_index, start_time, elapsed_time_s, distance_m,
				elevation_gain_m, elevation_loss_m, avg_grade_adjusted_pace_s_per_km
			)
			values($1,$2,$3,$4,$5,$6,$7,$8)
		`, id, lap.Index, lap.StartTime, lap.ElapsedTimeS, lap.DistanceM,
			lap.ElevationGainM, lap.ElevationLossM, lap.AvgGradeAdjustedPaceSPKM)
		if err != nil {
			return "", err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) ListActivities(ctx context.Context, limit, offset int, filters ActivityFilters) ([]Activity, error) {
	page, err := s.ListActivityPage(ctx, limit, offset, filters)
	return page.Activities, err
}

func (s *Store) ListActivityPage(ctx context.Context, limit, offset int, filters ActivityFilters) (ActivityListPage, error) {
	limit, offset = normalizeActivityPage(limit, offset)
	where, args := activityFilterWhere(filters, 1)
	orderBy := activityOrderBy(filters.SortBy, filters.SortOrder)
	args = append(args, limit+1, offset)
	limitParam := len(args) - 1
	offsetParam := len(args)
	rows, err := s.db.Query(ctx, activitySelectSQL+where+fmt.Sprintf(` %s limit $%d offset $%d`, orderBy, limitParam, offsetParam), args...)
	if err != nil {
		return ActivityListPage{}, err
	}
	defer rows.Close()
	activities, err := scanActivities(rows)
	if err != nil {
		return ActivityListPage{}, err
	}
	hasMore := len(activities) > limit
	if hasMore {
		activities = activities[:limit]
	}
	if err := s.attachActivityGear(ctx, activities); err != nil {
		return ActivityListPage{}, err
	}
	page := ActivityListPage{
		Activities: activities,
		Limit:      limit,
		Offset:     offset,
		HasMore:    hasMore,
	}
	if hasMore {
		page.NextOffset = offset + limit
	}
	return page, nil
}

func normalizeActivityPage(limit, offset int) (int, int) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (s *Store) IsActivitySyncExcluded(ctx context.Context, source, sourceID string) (bool, error) {
	var excluded bool
	err := s.db.QueryRow(ctx, `
		select exists(
			select 1
			from sync_excluded_activities
			where source = $1 and source_id = $2
		)
	`, source, sourceID).Scan(&excluded)
	return excluded, err
}

func (s *Store) GetActivity(ctx context.Context, id string) (Activity, error) {
	var activity Activity
	row := s.db.QueryRow(ctx, activitySelectSQL+` where id = $1`, id)
	if err := scanActivity(row, &activity); err != nil {
		return activity, err
	}

	samples, err := s.listSamples(ctx, id)
	if err != nil {
		return activity, err
	}
	laps, err := s.listLaps(ctx, id)
	if err != nil {
		return activity, err
	}
	activity.Samples = samples
	activity.Laps = laps
	gear, err := s.ListActivityGear(ctx, id)
	if err != nil {
		return activity, err
	}
	activity.Gear = gear
	activity.Climbs = detectActivityClimbs(samples)
	return activity, nil
}

func (s *Store) ActivityExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `select exists(select 1 from activities where id = $1)`, id).Scan(&exists)
	return exists, err
}

func (s *Store) UpsertGear(ctx context.Context, gear Gear) (Gear, error) {
	raw := gear.Raw
	if raw == nil {
		raw = map[string]any{}
	}
	statsRaw := gear.StatsRaw
	if statsRaw == nil {
		statsRaw = map[string]any{}
	}
	rawBytes, err := json.Marshal(raw)
	if err != nil {
		return Gear{}, err
	}
	statsBytes, err := json.Marshal(statsRaw)
	if err != nil {
		return Gear{}, err
	}
	row := s.db.QueryRow(ctx, `
		insert into gears(
			provider, provider_gear_id, name, gear_type, brand, model, retired,
			total_distance_m, max_distance_m, first_used_at, last_used_at,
			default_activity_types, raw, stats_raw
		)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		on conflict(provider, provider_gear_id) do update set
			name = excluded.name,
			gear_type = excluded.gear_type,
			brand = excluded.brand,
			model = excluded.model,
			retired = excluded.retired,
			total_distance_m = excluded.total_distance_m,
			max_distance_m = excluded.max_distance_m,
			first_used_at = excluded.first_used_at,
			last_used_at = excluded.last_used_at,
			default_activity_types = excluded.default_activity_types,
			raw = excluded.raw,
			stats_raw = excluded.stats_raw,
			updated_at = now()
		returning id::text, provider, provider_gear_id, name, gear_type, brand, model, retired,
			total_distance_m, max_distance_m, first_used_at, last_used_at, default_activity_types,
			raw, stats_raw, created_at, updated_at
	`, gear.Provider, gear.ProviderGearID, strings.TrimSpace(gear.Name), strings.TrimSpace(gear.GearType),
		strings.TrimSpace(gear.Brand), strings.TrimSpace(gear.Model), gear.Retired,
		optionalFloat(gear.TotalDistanceM), optionalFloat(gear.MaxDistanceM), nullTimePtr(gear.FirstUsedAt),
		nullTimePtr(gear.LastUsedAt), gear.DefaultActivityTypes, rawBytes, statsBytes)
	return scanGear(row)
}

func (s *Store) ReplaceGearAssignmentsForGear(ctx context.Context, gearID string, provider string, sourceIDs []string) (int, error) {
	uniqueSourceIDs := compactStrings(sourceIDs)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx, `delete from activity_gears where gear_id = $1`, gearID); err != nil {
		return 0, err
	}
	if len(uniqueSourceIDs) == 0 {
		if err = tx.Commit(ctx); err != nil {
			return 0, err
		}
		return 0, nil
	}

	rows, err := tx.Query(ctx, `
		select id::text
		from activities
		where source = $1 and source_id = any($2)
	`, provider, uniqueSourceIDs)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	activityIDs := make([]string, 0)
	for rows.Next() {
		var activityID string
		if err = rows.Scan(&activityID); err != nil {
			return 0, err
		}
		activityIDs = append(activityIDs, activityID)
	}
	if err = rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()
	for _, activityID := range activityIDs {
		if _, err = tx.Exec(ctx, `
			insert into activity_gears(activity_id, gear_id)
			values($1, $2)
			on conflict(activity_id, gear_id) do update set updated_at = now()
		`, activityID, gearID); err != nil {
			return 0, err
		}
	}
	if len(activityIDs) > 0 {
		if _, err = tx.Exec(ctx, `
			update gears
			set first_used_at = gear_usage.first_used_at,
				last_used_at = gear_usage.last_used_at,
				updated_at = now()
			from (
				select min(activities.start_time) as first_used_at,
					max(activities.start_time) as last_used_at
				from activity_gears
				join activities on activities.id = activity_gears.activity_id
				where activity_gears.gear_id = $1
			) gear_usage
			where gears.id = $1
		`, gearID); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(activityIDs), nil
}

func (s *Store) ListGears(ctx context.Context) ([]Gear, error) {
	rows, err := s.db.Query(ctx, `
		select id::text, provider, provider_gear_id, name, gear_type, brand, model, retired,
			total_distance_m, max_distance_m, first_used_at, last_used_at, default_activity_types,
			raw, stats_raw, created_at, updated_at
		from gears
		order by retired, lower(name), provider_gear_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	gears := make([]Gear, 0)
	for rows.Next() {
		gear, err := scanGear(rows)
		if err != nil {
			return nil, err
		}
		gears = append(gears, gear)
	}
	return gears, rows.Err()
}

func (s *Store) GetGear(ctx context.Context, id string) (Gear, error) {
	return scanGear(s.db.QueryRow(ctx, `
		select id::text, provider, provider_gear_id, name, gear_type, brand, model, retired,
			total_distance_m, max_distance_m, first_used_at, last_used_at, default_activity_types,
			raw, stats_raw, created_at, updated_at
		from gears
		where id = $1
	`, id))
}

func (s *Store) ListGearActivities(ctx context.Context, gearID string) ([]Activity, error) {
	rows, err := s.db.Query(ctx, activitySelectSQL+`
		join activity_gears on activity_gears.activity_id = activities.id
		where activity_gears.gear_id = $1
		order by activities.start_time desc, activities.id desc
	`, gearID)
	if err != nil {
		return nil, err
	}
	activities, err := scanActivities(rows)
	if err != nil {
		return nil, err
	}
	if err := s.attachActivityGear(ctx, activities); err != nil {
		return nil, err
	}
	return activities, nil
}

func (s *Store) ListActivityGear(ctx context.Context, activityID string) ([]GearSummary, error) {
	rows, err := s.db.Query(ctx, gearSummarySelectSQL+`
		join activity_gears on activity_gears.gear_id = gears.id
		where activity_gears.activity_id = $1
		order by gears.retired, lower(gears.name), gears.provider_gear_id
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	gear := make([]GearSummary, 0)
	for rows.Next() {
		item, err := scanGearSummary(rows)
		if err != nil {
			return nil, err
		}
		gear = append(gear, item)
	}
	return gear, rows.Err()
}

func (s *Store) attachActivityGear(ctx context.Context, activities []Activity) error {
	if len(activities) == 0 {
		return nil
	}
	activityIDs := make([]string, 0, len(activities))
	activityIndex := make(map[string]int, len(activities))
	for index, activity := range activities {
		activityIDs = append(activityIDs, activity.ID)
		activityIndex[activity.ID] = index
	}
	rows, err := s.db.Query(ctx, `
		select activity_gears.activity_id::text, gears.id::text, gears.provider_gear_id, gears.name,
			gears.gear_type, gears.brand, gears.model, gears.retired, gears.total_distance_m,
			gears.max_distance_m, gears.default_activity_types, gears.last_used_at
		from activity_gears
		join gears on gears.id = activity_gears.gear_id
		where activity_gears.activity_id::text = any($1)
		order by gears.retired, lower(gears.name), gears.provider_gear_id
	`, activityIDs)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var activityID string
		var gear GearSummary
		if err := scanGearSummaryWithActivity(rows, &activityID, &gear); err != nil {
			return err
		}
		index, ok := activityIndex[activityID]
		if !ok {
			continue
		}
		activities[index].Gear = append(activities[index].Gear, gear)
	}
	return rows.Err()
}

func (s *Store) DeleteActivity(ctx context.Context, id string) (DeleteActivityResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return DeleteActivityResult{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var source, sourceID, name, sportType string
	var hasSourceFile bool
	var startTime time.Time
	err = tx.QueryRow(ctx, `
		select source, source_id, source_file_id is not null, name, sport_type, start_time
		from activities
		where id = $1
		for update
	`, id).Scan(&source, &sourceID, &hasSourceFile, &name, &sportType, &startTime)
	if err != nil {
		return DeleteActivityResult{}, err
	}

	result := DeleteActivityResult{Deleted: true}
	if isProviderSyncedSource(source, sourceID, !hasSourceFile) {
		_, err = tx.Exec(ctx, `
			insert into sync_excluded_activities(source, source_id, name, sport_type, start_time)
			values($1, $2, $3, $4, $5)
			on conflict(source, source_id) do update set
				name = excluded.name,
				sport_type = excluded.sport_type,
				start_time = excluded.start_time,
				reason = 'deleted_from_runnarr'
		`, source, sourceID, name, sportType, startTime)
		if err != nil {
			return DeleteActivityResult{}, err
		}
		result.ExcludedFromSync = true
		result.SyncExclusionMessage = "This synced activity will be ignored in future imports."
	}

	if _, err = tx.Exec(ctx, `delete from activities where id = $1`, id); err != nil {
		return DeleteActivityResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return DeleteActivityResult{}, err
	}
	return result, nil
}

func (s *Store) CreateActivityMedia(ctx context.Context, media ActivityMedia) (ActivityMedia, error) {
	var saved ActivityMedia
	row := s.db.QueryRow(ctx, `
		insert into activity_media(
			activity_id, original_filename, content_type, size_bytes, sha256,
			original_path, thumbnail_path, width, height, capture_time, latitude, longitude
		)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		on conflict(activity_id, sha256) do update set
			original_filename = activity_media.original_filename
		returning id::text, activity_id::text, original_filename, content_type, size_bytes, sha256,
			original_path, thumbnail_path, width, height, capture_time, latitude, longitude, created_at
	`, media.ActivityID, media.OriginalFilename, media.ContentType, media.SizeBytes, media.SHA256,
		media.OriginalPath, media.ThumbnailPath, media.Width, media.Height, media.CaptureTime, media.Latitude, media.Longitude)
	return saved, scanActivityMedia(row, &saved)
}

func (s *Store) ListActivityMedia(ctx context.Context, activityID string) ([]ActivityMedia, error) {
	rows, err := s.db.Query(ctx, `
		select id::text, activity_id::text, original_filename, content_type, size_bytes, sha256,
			original_path, thumbnail_path, width, height, capture_time, latitude, longitude, created_at
		from activity_media
		where activity_id = $1
		order by coalesce(capture_time, created_at) asc, created_at asc
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ActivityMedia, 0)
	for rows.Next() {
		var media ActivityMedia
		if err := scanActivityMedia(rows, &media); err != nil {
			return nil, err
		}
		out = append(out, media)
	}
	return out, rows.Err()
}

func (s *Store) GetActivityMedia(ctx context.Context, id string) (ActivityMedia, error) {
	var media ActivityMedia
	row := s.db.QueryRow(ctx, `
		select id::text, activity_id::text, original_filename, content_type, size_bytes, sha256,
			original_path, thumbnail_path, width, height, capture_time, latitude, longitude, created_at
		from activity_media
		where id = $1
	`, id)
	return media, scanActivityMedia(row, &media)
}

func (s *Store) GetActivityMediaByHash(ctx context.Context, activityID, hash string) (ActivityMedia, error) {
	var media ActivityMedia
	row := s.db.QueryRow(ctx, `
		select id::text, activity_id::text, original_filename, content_type, size_bytes, sha256,
			original_path, thumbnail_path, width, height, capture_time, latitude, longitude, created_at
		from activity_media
		where activity_id = $1 and sha256 = $2
	`, activityID, hash)
	return media, scanActivityMedia(row, &media)
}

func (s *Store) DeleteActivityMedia(ctx context.Context, activityID, mediaID string) (ActivityMedia, error) {
	var media ActivityMedia
	row := s.db.QueryRow(ctx, `
		delete from activity_media
		where activity_id = $1 and id = $2
		returning id::text, activity_id::text, original_filename, content_type, size_bytes, sha256,
			original_path, thumbnail_path, width, height, capture_time, latitude, longitude, created_at
	`, activityID, mediaID)
	return media, scanActivityMedia(row, &media)
}

func (s *Store) RenameActivity(ctx context.Context, id, requestedName string) (Activity, error) {
	var sourceName string
	if err := s.db.QueryRow(ctx, `select name from activities where id = $1`, id).Scan(&sourceName); err != nil {
		return Activity{}, err
	}
	localName, err := localActivityNameOverride(requestedName, sourceName)
	if err != nil {
		return Activity{}, err
	}
	if _, err := s.db.Exec(ctx, `
		update activities
		set local_name = $2, updated_at = now()
		where id = $1
	`, id, localName); err != nil {
		return Activity{}, err
	}
	return s.GetActivity(ctx, id)
}

func (s *Store) Summary(ctx context.Context, filters ActivityFilters) (SummaryStats, error) {
	var stats SummaryStats
	stats.Recent = make([]Activity, 0)
	stats.WeeklyDistance = make([]WeeklyBucket, 0)
	where, args := activityFilterWhere(filters, 1)
	if err := s.db.QueryRow(ctx, `
		select count(*), coalesce(sum(distance_m), 0), coalesce(sum(moving_time_s), 0), coalesce(sum(elevation_gain_m), 0)
		from activities
	`+where, args...).Scan(&stats.ActivityCount, &stats.DistanceM, &stats.MovingTimeS, &stats.ElevationGainM); err != nil {
		return stats, err
	}

	recentRows, err := s.db.Query(ctx, activitySelectSQL+where+` order by start_time desc limit 5`, args...)
	if err != nil {
		return stats, err
	}
	stats.Recent, err = scanActivities(recentRows)
	recentRows.Close()
	if err != nil {
		return stats, err
	}

	weeklyConditions, weeklyArgs := activityFilterConditions(filters, 1)
	weeklyConditions = append([]string{`start_time >= now() - interval '12 weeks'`}, weeklyConditions...)
	weeklyRows, err := s.db.Query(ctx, `
		select date_trunc('week', start_time)::timestamptz as week_start, coalesce(sum(distance_m), 0)
		from activities
		where `+strings.Join(weeklyConditions, " and ")+`
		group by week_start
		order by week_start
	`, weeklyArgs...)
	if err != nil {
		return stats, err
	}
	defer weeklyRows.Close()
	for weeklyRows.Next() {
		var bucket WeeklyBucket
		if err := weeklyRows.Scan(&bucket.WeekStart, &bucket.DistanceM); err != nil {
			return stats, err
		}
		stats.WeeklyDistance = append(stats.WeeklyDistance, bucket)
	}
	return stats, weeklyRows.Err()
}

func (s *Store) ListSportTypes(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		select distinct sport_type
		from activities
		where sport_type <> ''
		order by sport_type
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var sport string
		if err := rows.Scan(&sport); err != nil {
			return nil, err
		}
		out = append(out, sport)
	}
	return out, rows.Err()
}

func (s *Store) UpsertProviderConnection(ctx context.Context, conn StoredProviderConnection) error {
	metadata, err := json.Marshal(map[string]any{})
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		insert into provider_connections(
			provider, provider_account_id, display_name, access_token_ciphertext,
			refresh_token_ciphertext, token_expires_at, scopes, metadata
		)
		values($1,$2,$3,$4,$5,$6,$7,$8)
		on conflict(provider) do update set
			provider_account_id = excluded.provider_account_id,
			display_name = excluded.display_name,
			access_token_ciphertext = excluded.access_token_ciphertext,
			refresh_token_ciphertext = excluded.refresh_token_ciphertext,
			token_expires_at = excluded.token_expires_at,
			scopes = excluded.scopes,
			metadata = excluded.metadata,
			updated_at = now()
	`, conn.Provider, conn.ProviderAccountID, conn.DisplayName, conn.AccessTokenCiphertext,
		conn.RefreshTokenCiphertext, nullTime(conn.TokenExpiresAt), conn.Scopes, metadata)
	return err
}

func (s *Store) GetProviderConnection(ctx context.Context, provider string) (StoredProviderConnection, error) {
	var conn StoredProviderConnection
	var scopes []string
	var expires pgtype.Timestamptz
	err := s.db.QueryRow(ctx, `
		select id::text, provider, provider_account_id, display_name, access_token_ciphertext,
			refresh_token_ciphertext, token_expires_at, scopes, connected_at, updated_at
		from provider_connections
		where provider = $1
	`, provider).Scan(&conn.ID, &conn.Provider, &conn.ProviderAccountID, &conn.DisplayName,
		&conn.AccessTokenCiphertext, &conn.RefreshTokenCiphertext, &expires, &scopes, &conn.ConnectedAt, &conn.UpdatedAt)
	if err != nil {
		return conn, err
	}
	if expires.Valid {
		conn.TokenExpiresAt = expires.Time
	}
	conn.Scopes = scopes
	return conn, nil
}

func (s *Store) CreateSyncJob(ctx context.Context, provider, kind string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		insert into sync_jobs(provider, kind, status, started_at)
		values($1, $2, 'running', now())
		returning id::text
	`, provider, kind).Scan(&id)
	return id, err
}

func (s *Store) UpdateSyncJobProgress(ctx context.Context, id string, payload map[string]any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		update sync_jobs
		set payload = $2
		where id = $1 and status = 'running'
	`, id, payloadBytes)
	return err
}

func (s *Store) HasRunningSyncJob(ctx context.Context, provider string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		select exists(
			select 1
			from sync_jobs
			where provider = $1 and status = 'running'
		)
	`, provider).Scan(&exists)
	return exists, err
}

func (s *Store) LatestSyncJobCreatedAt(ctx context.Context, provider, kind string) (time.Time, bool, error) {
	var createdAt time.Time
	err := s.db.QueryRow(ctx, `
		select created_at
		from sync_jobs
		where provider = $1 and kind = $2
		order by created_at desc
		limit 1
	`, provider, kind).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return createdAt, true, nil
}

func (s *Store) FinishSyncJob(ctx context.Context, id, status, message string, payload map[string]any) error {
	if payload == nil {
		_, err := s.db.Exec(ctx, `
			update sync_jobs
			set status = $2, error = $3, finished_at = now()
			where id = $1
		`, id, status, message)
		return err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		update sync_jobs
		set status = $2, error = $3, payload = $4, finished_at = now()
		where id = $1
	`, id, status, message, payloadBytes)
	return err
}

func (s *Store) ListSyncJobs(ctx context.Context) ([]SyncJob, error) {
	rows, err := s.db.Query(ctx, `
		select id::text, provider, kind, status, payload, error, created_at, started_at, finished_at
		from sync_jobs
		order by created_at desc
		limit 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]SyncJob, 0)
	for rows.Next() {
		var job SyncJob
		var payloadBytes []byte
		var startedAt, finishedAt pgtype.Timestamptz
		if err := rows.Scan(&job.ID, &job.Provider, &job.Kind, &job.Status, &payloadBytes, &job.Error, &job.CreatedAt, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		if len(payloadBytes) > 0 {
			_ = json.Unmarshal(payloadBytes, &job.Payload)
		}
		if startedAt.Valid {
			job.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			job.FinishedAt = &finishedAt.Time
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) listSamples(ctx context.Context, activityID string) ([]ActivitySample, error) {
	rows, err := s.db.Query(ctx, `
		select sample_index, timestamp, elapsed_s, distance_m, latitude, longitude, elevation_m, heart_rate, cadence, power, speed_mps
		from activity_samples
		where activity_id = $1
		order by sample_index
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	samples := make([]ActivitySample, 0)
	for rows.Next() {
		var sample ActivitySample
		var ts pgtype.Timestamptz
		var elapsed sql.NullInt32
		var distance, lat, lon, elevation, speed sql.NullFloat64
		var hr, cadence, power sql.NullInt32
		if err := rows.Scan(&sample.Index, &ts, &elapsed, &distance, &lat, &lon, &elevation, &hr, &cadence, &power, &speed); err != nil {
			return nil, err
		}
		if ts.Valid {
			sample.Timestamp = &ts.Time
		}
		sample.ElapsedS = intPtrFromNull(elapsed)
		sample.DistanceM = floatPtrFromNull(distance)
		sample.Latitude = floatPtrFromNull(lat)
		sample.Longitude = floatPtrFromNull(lon)
		sample.ElevationM = floatPtrFromNull(elevation)
		sample.HeartRate = intPtrFromNull(hr)
		sample.Cadence = intPtrFromNull(cadence)
		sample.Power = intPtrFromNull(power)
		sample.SpeedMPS = floatPtrFromNull(speed)
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (s *Store) listLaps(ctx context.Context, activityID string) ([]ActivityLap, error) {
	rows, err := s.db.Query(ctx, `
		select lap_index, start_time, elapsed_time_s, distance_m,
			elevation_gain_m, elevation_loss_m, avg_grade_adjusted_pace_s_per_km
		from activity_laps
		where activity_id = $1
		order by lap_index
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	laps := make([]ActivityLap, 0)
	for rows.Next() {
		var lap ActivityLap
		var ts pgtype.Timestamptz
		var gain, loss, avgGradeAdjustedPace sql.NullFloat64
		if err := rows.Scan(&lap.Index, &ts, &lap.ElapsedTimeS, &lap.DistanceM, &gain, &loss, &avgGradeAdjustedPace); err != nil {
			return nil, err
		}
		if ts.Valid {
			lap.StartTime = &ts.Time
		}
		lap.ElevationGainM = floatPtrFromNull(gain)
		lap.ElevationLossM = floatPtrFromNull(loss)
		lap.AvgGradeAdjustedPaceSPKM = floatPtrFromNull(avgGradeAdjustedPace)
		laps = append(laps, lap)
	}
	return laps, rows.Err()
}

const activitySelectSQL = `
	select activities.id::text, activities.source, activities.source_id, coalesce(nullif(activities.local_name, ''), activities.name),
		activities.name, activities.local_name, activities.sport_type, activities.start_time, activities.distance_m,
		activities.moving_time_s, activities.elapsed_time_s, activities.elevation_gain_m, activities.avg_heart_rate,
		activities.max_heart_rate, activities.avg_pace_s_per_km, activities.avg_grade_adjusted_pace_s_per_km,
		activities.calories_kcal, activities.original_provider_url, activities.summary_polyline, activities.created_at
	from activities
`

const gearSummarySelectSQL = `
	select gears.id::text, gears.provider_gear_id, gears.name, gears.gear_type, gears.brand,
		gears.model, gears.retired, gears.total_distance_m, gears.max_distance_m,
		gears.default_activity_types, gears.last_used_at
	from gears
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanActivities(rows pgx.Rows) ([]Activity, error) {
	defer rows.Close()
	out := make([]Activity, 0)
	for rows.Next() {
		var activity Activity
		if err := scanActivity(rows, &activity); err != nil {
			return nil, err
		}
		out = append(out, activity)
	}
	return out, rows.Err()
}

func scanActivity(row rowScanner, activity *Activity) error {
	var avgHR, maxHR, avgPace, avgGradeAdjustedPace sql.NullFloat64
	var calories sql.NullInt32
	if err := row.Scan(&activity.ID, &activity.Source, &activity.SourceID, &activity.Name, &activity.SourceName, &activity.LocalName, &activity.SportType,
		&activity.StartTime, &activity.DistanceM, &activity.MovingTimeS, &activity.ElapsedTimeS,
		&activity.ElevationGainM, &avgHR, &maxHR, &avgPace, &avgGradeAdjustedPace, &calories,
		&activity.OriginalProviderURL, &activity.SummaryPolyline, &activity.CreatedAt); err != nil {
		return err
	}
	activity.AvgHeartRate = floatPtrFromNull(avgHR)
	activity.MaxHeartRate = floatPtrFromNull(maxHR)
	activity.AvgPaceSPKM = floatPtrFromNull(avgPace)
	activity.AvgGradeAdjustedPaceSPKM = floatPtrFromNull(avgGradeAdjustedPace)
	activity.CaloriesKcal = intPtrFromNull(calories)
	return nil
}

func scanActivityMedia(row rowScanner, media *ActivityMedia) error {
	var capture pgtype.Timestamptz
	var latitude, longitude sql.NullFloat64
	if err := row.Scan(&media.ID, &media.ActivityID, &media.OriginalFilename, &media.ContentType, &media.SizeBytes,
		&media.SHA256, &media.OriginalPath, &media.ThumbnailPath, &media.Width, &media.Height,
		&capture, &latitude, &longitude, &media.CreatedAt); err != nil {
		return err
	}
	if capture.Valid {
		media.CaptureTime = &capture.Time
	}
	media.Latitude = floatPtrFromNull(latitude)
	media.Longitude = floatPtrFromNull(longitude)
	return nil
}

func scanGear(row rowScanner) (Gear, error) {
	var gear Gear
	var totalDistance, maxDistance sql.NullFloat64
	var firstUsed, lastUsed pgtype.Timestamptz
	var rawBytes, statsBytes []byte
	if err := row.Scan(
		&gear.ID,
		&gear.Provider,
		&gear.ProviderGearID,
		&gear.Name,
		&gear.GearType,
		&gear.Brand,
		&gear.Model,
		&gear.Retired,
		&totalDistance,
		&maxDistance,
		&firstUsed,
		&lastUsed,
		&gear.DefaultActivityTypes,
		&rawBytes,
		&statsBytes,
		&gear.CreatedAt,
		&gear.UpdatedAt,
	); err != nil {
		return Gear{}, err
	}
	gear.TotalDistanceM = floatPtrFromNull(totalDistance)
	gear.MaxDistanceM = floatPtrFromNull(maxDistance)
	if firstUsed.Valid {
		gear.FirstUsedAt = &firstUsed.Time
	}
	if lastUsed.Valid {
		gear.LastUsedAt = &lastUsed.Time
	}
	if len(rawBytes) > 0 {
		_ = json.Unmarshal(rawBytes, &gear.Raw)
	}
	if len(statsBytes) > 0 {
		_ = json.Unmarshal(statsBytes, &gear.StatsRaw)
	}
	if gear.Raw == nil {
		gear.Raw = map[string]any{}
	}
	if gear.StatsRaw == nil {
		gear.StatsRaw = map[string]any{}
	}
	return gear, nil
}

func scanGearSummary(row rowScanner) (GearSummary, error) {
	var gear GearSummary
	if err := scanGearSummaryWithActivity(row, nil, &gear); err != nil {
		return GearSummary{}, err
	}
	return gear, nil
}

func scanGearSummaryWithActivity(row rowScanner, activityID *string, gear *GearSummary) error {
	var totalDistance, maxDistance sql.NullFloat64
	var lastUsed pgtype.Timestamptz
	dest := []any{
		&gear.ID,
		&gear.ProviderGearID,
		&gear.Name,
		&gear.GearType,
		&gear.Brand,
		&gear.Model,
		&gear.Retired,
		&totalDistance,
		&maxDistance,
		&gear.DefaultActivityTypes,
		&lastUsed,
	}
	if activityID != nil {
		dest = append([]any{activityID}, dest...)
	}
	if err := row.Scan(dest...); err != nil {
		return err
	}
	gear.TotalDistanceM = floatPtrFromNull(totalDistance)
	gear.MaxDistanceM = floatPtrFromNull(maxDistance)
	if lastUsed.Valid {
		gear.LastUsedAt = &lastUsed.Time
	}
	return nil
}

func scanDailyHealthMetric(row rowScanner) (DailyHealthMetric, error) {
	var metric DailyHealthMetric
	var steps sql.NullInt32
	var totalCalories sql.NullInt32
	var activeCalories sql.NullInt32
	var restingHR sql.NullFloat64
	var avgHR sql.NullFloat64
	var maxHR sql.NullFloat64
	var sleepDuration sql.NullInt32
	var deepSleep sql.NullInt32
	var lightSleep sql.NullInt32
	var remSleep sql.NullInt32
	var awakeSleep sql.NullInt32
	var sleepScore sql.NullFloat64
	var stressAvg sql.NullFloat64
	var stressMax sql.NullFloat64
	var bodyBatteryAvg sql.NullFloat64
	var bodyBatteryMin sql.NullFloat64
	var bodyBatteryMax sql.NullFloat64
	var bodyBatteryStart sql.NullFloat64
	var bodyBatteryEnd sql.NullFloat64
	var bodyBatteryGained sql.NullFloat64
	var bodyBatteryDrained sql.NullFloat64
	var hrvAvg sql.NullFloat64
	var weightKG sql.NullFloat64
	var bodyFatPct sql.NullFloat64
	var rawBytes []byte

	if err := row.Scan(
		&metric.ID,
		&metric.Provider,
		&metric.Date,
		&steps,
		&totalCalories,
		&activeCalories,
		&restingHR,
		&avgHR,
		&maxHR,
		&sleepDuration,
		&deepSleep,
		&lightSleep,
		&remSleep,
		&awakeSleep,
		&sleepScore,
		&stressAvg,
		&stressMax,
		&bodyBatteryAvg,
		&bodyBatteryMin,
		&bodyBatteryMax,
		&bodyBatteryStart,
		&bodyBatteryEnd,
		&bodyBatteryGained,
		&bodyBatteryDrained,
		&hrvAvg,
		&metric.HRVStatus,
		&weightKG,
		&bodyFatPct,
		&rawBytes,
		&metric.CreatedAt,
		&metric.UpdatedAt,
	); err != nil {
		return DailyHealthMetric{}, err
	}

	metric.Steps = intPtrFromNull(steps)
	metric.TotalCaloriesKcal = intPtrFromNull(totalCalories)
	metric.ActiveCaloriesKcal = intPtrFromNull(activeCalories)
	metric.RestingHeartRateBPM = floatPtrFromNull(restingHR)
	metric.AvgHeartRateBPM = floatPtrFromNull(avgHR)
	metric.MaxHeartRateBPM = floatPtrFromNull(maxHR)
	metric.SleepDurationS = intPtrFromNull(sleepDuration)
	metric.DeepSleepS = intPtrFromNull(deepSleep)
	metric.LightSleepS = intPtrFromNull(lightSleep)
	metric.REMSleepS = intPtrFromNull(remSleep)
	metric.AwakeSleepS = intPtrFromNull(awakeSleep)
	metric.SleepScore = floatPtrFromNull(sleepScore)
	metric.StressAvg = floatPtrFromNull(stressAvg)
	metric.StressMax = floatPtrFromNull(stressMax)
	metric.BodyBatteryAvg = floatPtrFromNull(bodyBatteryAvg)
	metric.BodyBatteryMin = floatPtrFromNull(bodyBatteryMin)
	metric.BodyBatteryMax = floatPtrFromNull(bodyBatteryMax)
	metric.BodyBatteryStart = floatPtrFromNull(bodyBatteryStart)
	metric.BodyBatteryEnd = floatPtrFromNull(bodyBatteryEnd)
	metric.BodyBatteryGained = floatPtrFromNull(bodyBatteryGained)
	metric.BodyBatteryDrained = floatPtrFromNull(bodyBatteryDrained)
	metric.HRVAvgMS = floatPtrFromNull(hrvAvg)
	metric.WeightKG = floatPtrFromNull(weightKG)
	metric.BodyFatPct = floatPtrFromNull(bodyFatPct)
	if len(rawBytes) > 0 {
		_ = json.Unmarshal(rawBytes, &metric.Raw)
	}
	if metric.Raw == nil {
		metric.Raw = map[string]any{}
	}
	return metric, nil
}

func activityFilterWhere(filters ActivityFilters, startArg int) (string, []any) {
	conditions, args := activityFilterConditions(filters, startArg)
	if len(conditions) == 0 {
		return "", args
	}
	return " where " + strings.Join(conditions, " and "), args
}

func activityFilterConditions(filters ActivityFilters, startArg int) ([]string, []any) {
	conditions := make([]string, 0, 5)
	args := make([]any, 0, 5)
	nextArg := startArg
	if strings.TrimSpace(filters.Search) != "" {
		conditions = append(conditions, fmt.Sprintf("coalesce(nullif(local_name, ''), name) ilike $%d", nextArg))
		args = append(args, "%"+strings.TrimSpace(filters.Search)+"%")
		nextArg++
	}
	if !filters.DateFrom.IsZero() {
		conditions = append(conditions, fmt.Sprintf("start_time >= $%d", nextArg))
		args = append(args, filters.DateFrom)
		nextArg++
	}
	if !filters.DateTo.IsZero() {
		conditions = append(conditions, fmt.Sprintf("start_time < $%d", nextArg))
		args = append(args, filters.DateTo.AddDate(0, 0, 1))
		nextArg++
	}
	if len(filters.SportTypes) > 0 {
		conditions = append(conditions, fmt.Sprintf("sport_type = any($%d)", nextArg))
		args = append(args, filters.SportTypes)
		nextArg++
	}
	if len(filters.ExcludedSportTypes) > 0 {
		conditions = append(conditions, fmt.Sprintf("sport_type <> all($%d)", nextArg))
		args = append(args, filters.ExcludedSportTypes)
	}
	return conditions, args
}

func activityOrderBy(sortBy, sortOrder string) string {
	sortExpression := "start_time"
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "", "date":
		sortExpression = "start_time"
	case "duration":
		sortExpression = "coalesce(nullif(moving_time_s, 0), elapsed_time_s, 0)"
	case "distance":
		sortExpression = "distance_m"
	case "elevation_gain":
		sortExpression = "elevation_gain_m"
	case "avg_pace":
		sortExpression = "coalesce(avg_pace_s_per_km, 0)"
	case "calories":
		sortExpression = "coalesce(calories_kcal, 0)"
	}

	direction := "desc"
	switch strings.ToLower(strings.TrimSpace(sortOrder)) {
	case "asc":
		direction = "asc"
	case "desc":
		direction = "desc"
	}

	orderBy := fmt.Sprintf("order by %s %s", sortExpression, direction)
	if sortExpression != "start_time" {
		orderBy += ", start_time desc"
	}
	return orderBy + ", id desc"
}

func isProviderSyncedSource(source, sourceID string, noSourceFile bool) bool {
	return source != "" && source != "file" && sourceID != "" && noSourceFile
}

func fallbackName(activity ImportedActivity) string {
	if activity.Name != "" {
		return activity.Name
	}
	if !activity.StartTime.IsZero() {
		return activity.SportType + " on " + activity.StartTime.Format("2006-01-02")
	}
	return activity.SportType
}

func averagePace(distanceM float64, movingTimeS int) *float64 {
	if distanceM <= 0 || movingTimeS <= 0 {
		return nil
	}
	value := float64(movingTimeS) / (distanceM / 1000)
	return &value
}

func localActivityNameOverride(requestedName, sourceName string) (string, error) {
	name := strings.TrimSpace(requestedName)
	if name == "" {
		return "", ErrInvalidActivityName
	}
	if name == strings.TrimSpace(sourceName) {
		return "", nil
	}
	if utf8.RuneCountInString(name) > 160 {
		return "", ErrInvalidActivityName
	}
	return name, nil
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func nullTimePtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return *t
}

func optionalInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func compactStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func optionalFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func floatPtrFromNull(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	v := value.Float64
	return &v
}

func intPtrFromNull(value sql.NullInt32) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int32)
	return &v
}
