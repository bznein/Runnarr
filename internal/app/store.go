package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func scopedUserID(ctx context.Context) string {
	id, _ := userIDFromContext(ctx)
	return id
}

var ErrActivitySyncExcluded = errors.New("activity is excluded from provider sync")
var ErrSyncJobAlreadyRunning = errors.New("a sync job is already running")
var ErrInvalidActivityName = errors.New("activity name must be between 1 and 160 characters")
var ErrInvalidActivityNotes = errors.New("activity notes must be 5000 characters or fewer")
var ErrInvalidActivityFeedback = errors.New("activity feedback must be 5000 characters or fewer")
var ErrInvalidActivityRPE = errors.New("activity RPE must be between 1 and 10")

const appSettingsID = "default"

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) UpsertImportFile(ctx context.Context, file ImportFile) (ImportFile, error) {
	var saved ImportFile
	err := s.db.QueryRow(ctx, `
		insert into import_files(user_id, filename, content_type, sha256, size_bytes, parser, status, error)
		values($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (user_id, sha256) do update set
			filename = excluded.filename,
			content_type = excluded.content_type,
			parser = excluded.parser
		returning id::text, filename, content_type, sha256, size_bytes, parser, status, error, created_at
	`, scopedUserID(ctx), file.Filename, file.ContentType, file.SHA256, file.SizeBytes, file.Parser, file.Status, file.Error).
		Scan(&saved.ID, &saved.Filename, &saved.ContentType, &saved.SHA256, &saved.SizeBytes, &saved.Parser, &saved.Status, &saved.Error, &saved.CreatedAt)
	return saved, err
}

func (s *Store) UpdateImportStatus(ctx context.Context, id, status, message string) error {
	_, err := s.db.Exec(ctx, `
		update import_files
		set status = $2, error = $3
		where id = $1 and user_id = $4
	`, id, status, message, scopedUserID(ctx))
	return err
}

func (s *Store) ListImports(ctx context.Context) ([]ImportFile, error) {
	rows, err := s.db.Query(ctx, `
		select id::text, filename, content_type, sha256, size_bytes, parser, status, error, created_at
		from import_files
		where user_id = $1
		order by created_at desc
		limit 100
	`, scopedUserID(ctx))
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
			user_id,
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
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29)
		on conflict(user_id, provider, metric_date) do update set
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
		scopedUserID(ctx),
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
			-- Raw Garmin payloads are retained for diagnostics, but are not needed by the range view.
			null::jsonb as raw,
			created_at,
			updated_at
		from daily_health_metrics
		where user_id = $1 and provider = $2 and metric_date between $3 and $4
		order by metric_date asc
	`, scopedUserID(ctx), provider, from.Format("2006-01-02"), to.Format("2006-01-02"))
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

	avgPace := activity.AvgPaceSPKM
	if avgPace == nil {
		avgPace = averagePace(activity.DistanceM, activity.MovingTimeS)
	}

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
				where user_id = $1 and source = $2 and source_id = $3
				)
		`, scopedUserID(ctx), source, sourceID).Scan(&excluded)
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
			user_id, source, source_id, source_file_id, name, sport_type, start_time, distance_m,
			moving_time_s, elapsed_time_s, elevation_gain_m, avg_heart_rate, max_heart_rate,
			avg_pace_s_per_km, avg_grade_adjusted_pace_s_per_km, calories_kcal, original_provider_url, summary_polyline, raw
		)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		on conflict (user_id, source, source_id) do update set
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
	`, scopedUserID(ctx), source, sourceID, sourceFileID, fallbackName(activity), activity.SportType, activity.StartTime,
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
	if activity.ReplaceWorkoutMetadata {
		if _, err = tx.Exec(ctx, `delete from activity_intervals where activity_id = $1`, id); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `delete from activity_workouts where activity_id = $1`, id); err != nil {
			return "", err
		}
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
		lapRawBytes, marshalErr := marshalJSONObject(lap.Raw)
		if marshalErr != nil {
			return "", marshalErr
		}
		_, err = tx.Exec(ctx, `
			insert into activity_laps(
				activity_id, lap_index, start_time, elapsed_time_s, moving_time_s, distance_m,
				avg_pace_s_per_km, elevation_gain_m, elevation_loss_m, avg_grade_adjusted_pace_s_per_km,
				avg_heart_rate, max_heart_rate, avg_power, max_power, normalized_power,
				avg_run_cadence, avg_ground_contact_time_ms, avg_respiration_rate, avg_temperature_c,
				intensity_type, workout_step_index, workout_repeat_index, raw
			)
			values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
		`, id, lap.Index, lap.StartTime, lap.ElapsedTimeS, lap.MovingTimeS, lap.DistanceM,
			lap.AvgPaceSPKM, lap.ElevationGainM, lap.ElevationLossM, lap.AvgGradeAdjustedPaceSPKM,
			lap.AvgHeartRate, lap.MaxHeartRate, lap.AvgPower, lap.MaxPower, lap.NormalizedPower,
			lap.AvgRunCadence, lap.AvgGroundContactTimeMS, lap.AvgRespirationRate, lap.AvgTemperatureC,
			lap.IntensityType, lap.WorkoutStepIndex, lap.WorkoutRepeatIndex, lapRawBytes)
		if err != nil {
			return "", err
		}
	}

	if activity.ReplaceWorkoutMetadata {
		if activity.Workout != nil {
			stepsBytes, marshalErr := json.Marshal(activity.Workout.Steps)
			if marshalErr != nil {
				return "", marshalErr
			}
			workoutRawBytes, marshalErr := marshalJSONObject(activity.Workout.Raw)
			if marshalErr != nil {
				return "", marshalErr
			}
			_, err = tx.Exec(ctx, `
				insert into activity_workouts(activity_id, provider, provider_workout_id, name, sport_type, steps, raw)
				values($1,$2,$3,$4,$5,$6,$7)
				on conflict (activity_id) do update set
					provider = excluded.provider,
					provider_workout_id = excluded.provider_workout_id,
					name = excluded.name,
					sport_type = excluded.sport_type,
					steps = excluded.steps,
					raw = excluded.raw,
					updated_at = now()
			`, id, activity.Workout.Provider, activity.Workout.ProviderWorkoutID, activity.Workout.Name,
				activity.Workout.SportType, stepsBytes, workoutRawBytes)
			if err != nil {
				return "", err
			}
		}

		for _, interval := range activity.Intervals {
			intervalRawBytes, marshalErr := marshalJSONObject(interval.Raw)
			if marshalErr != nil {
				return "", marshalErr
			}
			_, err = tx.Exec(ctx, `
				insert into activity_intervals(
					activity_id, interval_index, category, provider_type, workout_step_index, workout_repeat_index,
					start_time, end_time, elapsed_time_s, moving_time_s, distance_m,
					avg_pace_s_per_km, avg_grade_adjusted_pace_s_per_km, avg_heart_rate, max_heart_rate,
					avg_power, max_power, normalized_power, avg_run_cadence, avg_ground_contact_time_ms,
					avg_respiration_rate, avg_temperature_c, elevation_gain_m, elevation_loss_m, calories_kcal,
					lap_indexes, raw
				) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)
			`, id, interval.Index, interval.Category, interval.ProviderType, interval.WorkoutStepIndex, interval.WorkoutRepeatIndex,
				interval.StartTime, interval.EndTime, interval.ElapsedTimeS, interval.MovingTimeS, interval.DistanceM,
				interval.AvgPaceSPKM, interval.AvgGradeAdjustedPaceSPKM, interval.AvgHeartRate, interval.MaxHeartRate,
				interval.AvgPower, interval.MaxPower, interval.NormalizedPower, interval.AvgRunCadence, interval.AvgGroundContactTimeMS,
				interval.AvgRespirationRate, interval.AvgTemperatureC, interval.ElevationGainM, interval.ElevationLossM, interval.CaloriesKcal,
				interval.LapIndexes, intervalRawBytes)
			if err != nil {
				return "", err
			}
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func marshalJSONObject(value map[string]any) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(value)
}

func (s *Store) ListActivities(ctx context.Context, limit, offset int, filters ActivityFilters) ([]Activity, error) {
	page, err := s.ListActivityPage(ctx, limit, offset, filters)
	return page.Activities, err
}

func (s *Store) ListActivityPage(ctx context.Context, limit, offset int, filters ActivityFilters) (ActivityListPage, error) {
	limit, offset = normalizeActivityPage(limit, offset)
	where, args := activityFilterWhereForUser(filters, 1, scopedUserID(ctx))
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

func (s *Store) ActivityCalendar(ctx context.Context, filters ActivityFilters) (ActivityCalendar, error) {
	filters.IncludeTrainingSheet = true
	userID := scopedUserID(ctx)
	where, args := activityFilterWhereForUser(filters, 1, userID)
	dayExpression := "date(start_time)"
	if filters.CalendarTimezone != "" {
		timezoneArg := 1
		if userID != "" {
			timezoneArg++
		}
		dayExpression = calendarActivityDateExpression(timezoneArg)
	}
	rows, err := s.db.Query(ctx, fmt.Sprintf(`
		select
			%s as day,
			id::text,
			source,
			coalesce(nullif(local_name, ''), name),
			start_time,
			sport_type,
			coalesce(distance_m, 0),
			coalesce(moving_time_s, 0)
		from activities
		`+where+`
		order by day, start_time
	`, dayExpression), args...)
	if err != nil {
		return ActivityCalendar{}, err
	}
	defer rows.Close()

	activityByDay := map[string][]CalendarActivity{}
	for rows.Next() {
		var day time.Time
		var item CalendarActivity
		if err := rows.Scan(&day, &item.ID, &item.Source, &item.Name, &item.StartTime, &item.SportType, &item.DistanceM, &item.MovingTimeS); err != nil {
			return ActivityCalendar{}, err
		}
		dayKey := day.Format("2006-01-02")
		activityByDay[dayKey] = append(activityByDay[dayKey], item)
	}
	if err := rows.Err(); err != nil {
		return ActivityCalendar{}, err
	}

	healthMetrics, err := s.ListDailyHealthMetrics(ctx, garminProvider, filters.DateFrom, filters.DateTo)
	if err != nil {
		return ActivityCalendar{}, err
	}
	healthByDay := make(map[string]bool, len(healthMetrics))
	for _, metric := range healthMetrics {
		healthByDay[metric.Date] = true
	}
	orderedDays := make([]string, 0, len(activityByDay)+len(healthByDay))
	for day := range activityByDay {
		orderedDays = append(orderedDays, day)
	}
	for day := range healthByDay {
		if _, seen := activityByDay[day]; !seen {
			orderedDays = append(orderedDays, day)
		}
	}
	sort.Strings(orderedDays)

	calendar := ActivityCalendar{
		MonthStart: formatCalendarMonthDate(filters.DateFrom),
		MonthEnd:   formatCalendarMonthDate(filters.DateTo),
		Days:       make([]CalendarDay, 0, len(orderedDays)),
	}
	for _, day := range orderedDays {
		activities := activityByDay[day]
		if activities == nil {
			activities = make([]CalendarActivity, 0)
		}
		calendar.Days = append(calendar.Days, CalendarDay{
			Date:          day,
			ActivityCount: len(activities),
			HasHealthData: healthByDay[day],
			Activities:    activities,
		})
	}

	return calendar, nil
}

func (s *Store) CalendarDay(ctx context.Context, date time.Time, timezone string) (CalendarDayView, error) {
	filters := ActivityFilters{
		DateFrom:             date,
		DateTo:               date,
		CalendarTimezone:     timezone,
		IncludeTrainingSheet: true,
	}
	where, args := activityFilterWhereForUser(filters, 1, scopedUserID(ctx))
	rows, err := s.db.Query(ctx, `
		select
			id::text,
			source,
			coalesce(nullif(local_name, ''), name),
			start_time,
			sport_type,
			coalesce(distance_m, 0),
			coalesce(moving_time_s, 0)
		from activities
		`+where+`
		order by start_time
	`, args...)
	if err != nil {
		return CalendarDayView{}, err
	}
	defer rows.Close()

	activities := make([]CalendarActivity, 0)
	for rows.Next() {
		var activity CalendarActivity
		if err := rows.Scan(&activity.ID, &activity.Source, &activity.Name, &activity.StartTime, &activity.SportType, &activity.DistanceM, &activity.MovingTimeS); err != nil {
			return CalendarDayView{}, err
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return CalendarDayView{}, err
	}

	healthMetrics, err := s.ListDailyHealthMetrics(ctx, garminProvider, date, date)
	if err != nil {
		return CalendarDayView{}, err
	}
	var health *DailyHealthMetric
	if len(healthMetrics) > 0 {
		health = &healthMetrics[0]
	}

	return CalendarDayView{
		Date:       date.Format("2006-01-02"),
		Health:     health,
		Activities: activities,
	}, nil
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

func formatCalendarMonthDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format("2006-01-02")
}

func (s *Store) IsActivitySyncExcluded(ctx context.Context, source, sourceID string) (bool, error) {
	var excluded bool
	err := s.db.QueryRow(ctx, `
		select exists(
			select 1
			from sync_excluded_activities
			where user_id = $1 and source = $2 and source_id = $3
			)
	`, scopedUserID(ctx), source, sourceID).Scan(&excluded)
	return excluded, err
}

func (s *Store) GetActivity(ctx context.Context, id string) (Activity, error) {
	var activity Activity
	row := s.db.QueryRow(ctx, activitySelectSQL+` where activities.id = $1 and activities.user_id = $2`, id, scopedUserID(ctx))
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
	workout, intervals, err := s.getActivityWorkout(ctx, id)
	if err != nil {
		return activity, err
	}
	activity.Workout = workout
	activity.Intervals = intervals
	gear, err := s.ListActivityGear(ctx, id)
	if err != nil {
		return activity, err
	}
	activity.Gear = gear
	climbSettings, err := s.GetClimbDetectionSettings(ctx)
	if err != nil {
		return activity, err
	}
	activity.Climbs = detectActivityClimbsWithSettings(samples, climbSettings.Settings)
	return activity, nil
}

func (s *Store) GetClimbDetectionSettings(ctx context.Context) (ClimbDetectionConfig, error) {
	settings := ClimbDetectionConfig{
		Preset:      defaultClimbDetectionPreset,
		Settings:    defaultClimbDetectionSettings(),
		Sensitivity: defaultClimbDetectionSensitivity,
	}

	row := s.db.QueryRow(ctx, `
		select
			climb_smoothing_radius_m,
			min_climb_distance_m,
			min_climb_elevation_gain_m,
			min_climb_average_grade_pct,
			max_climb_merge_dip_distance_m,
			max_climb_merge_elevation_loss_m,
			climb_start_gain_m,
			climb_detection_preset
		from user_settings
		where user_id = $1
	`, scopedUserID(ctx))

	var smoothingRadius, minDistance, minElevationGain, minGradePct, maxDipDistance, maxDipLoss, startGain float64
	var preset string
	if err := row.Scan(&smoothingRadius, &minDistance, &minElevationGain, &minGradePct, &maxDipDistance, &maxDipLoss, &startGain, &preset); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return settings, nil
		}
		return settings, err
	}

	configured := ClimbDetectionSettings{
		ClimbSmoothingRadiusM:       smoothingRadius,
		MinClimbDistanceM:           minDistance,
		MinClimbElevationGainM:      minElevationGain,
		MinClimbAverageGradePct:     minGradePct,
		MaxClimbMergeDipDistanceM:   maxDipDistance,
		MaxClimbMergeElevationLossM: maxDipLoss,
		ClimbStartGainM:             startGain,
	}
	if err := validateClimbDetectionSettings(configured); err == nil {
		settings.Settings = configured
	}
	if preset != "" {
		settings.Preset = preset
	}
	settings.Sensitivity = climbDetectionSensitivityFromSettings(settings.Settings)
	return settings, nil
}

func (s *Store) SetClimbDetectionSettings(ctx context.Context, preset string, settings ClimbDetectionSettings) error {
	if err := validateClimbDetectionSettings(settings); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		update user_settings
		set
			climb_smoothing_radius_m = $2,
			min_climb_distance_m = $3,
			min_climb_elevation_gain_m = $4,
			min_climb_average_grade_pct = $5,
			max_climb_merge_dip_distance_m = $6,
			max_climb_merge_elevation_loss_m = $7,
			climb_start_gain_m = $8,
			climb_detection_preset = $9,
			updated_at = now()
		where user_id = $1
	`, scopedUserID(ctx),
		settings.ClimbSmoothingRadiusM,
		settings.MinClimbDistanceM,
		settings.MinClimbElevationGainM,
		settings.MinClimbAverageGradePct,
		settings.MaxClimbMergeDipDistanceM,
		settings.MaxClimbMergeElevationLossM,
		settings.ClimbStartGainM,
		preset,
	)
	return err
}

func (s *Store) ActivityExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `select exists(select 1 from activities where id = $1 and user_id = $2)`, id, scopedUserID(ctx)).Scan(&exists)
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
			user_id, provider, provider_gear_id, name, gear_type, brand, model, retired,
			total_distance_m, max_distance_m, first_used_at, last_used_at,
			default_activity_types, raw, stats_raw
		)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		on conflict(user_id, provider, provider_gear_id) do update set
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
	`, scopedUserID(ctx), gear.Provider, gear.ProviderGearID, strings.TrimSpace(gear.Name), strings.TrimSpace(gear.GearType),
		strings.TrimSpace(gear.Brand), strings.TrimSpace(gear.Model), gear.Retired,
		optionalFloat(gear.TotalDistanceM), optionalFloat(gear.MaxDistanceM), nullTimePtr(gear.FirstUsedAt),
		nullTimePtr(gear.LastUsedAt), gear.DefaultActivityTypes, rawBytes, statsBytes)
	return scanGear(row, false)
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

	if _, err = tx.Exec(ctx, `delete from activity_gears where gear_id = $1 and exists (select 1 from gears where id = $1 and user_id = $2)`, gearID, scopedUserID(ctx)); err != nil {
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
		where user_id = $3 and source = $1 and source_id = any($2)
	`, provider, uniqueSourceIDs, scopedUserID(ctx))
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
			where gears.id = $1 and gears.user_id = $2
		`, gearID, scopedUserID(ctx)); err != nil {
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
		select gears.id::text, gears.provider, gears.provider_gear_id, gears.name, gears.gear_type, gears.brand, gears.model, gears.retired,
			gears.total_distance_m, gears.max_distance_m, gears.first_used_at, gears.last_used_at,
			coalesce(gear_activity_counts.activity_count, 0) as activity_count,
			gears.default_activity_types, gears.raw, gears.stats_raw, gears.created_at, gears.updated_at
		from gears
		left join (
			select gear_id, count(*)::int as activity_count
			from activity_gears
			group by gear_id
		) gear_activity_counts on gear_activity_counts.gear_id = gears.id
		where gears.user_id = $1
		order by retired, lower(name), provider_gear_id
	`, scopedUserID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	gears := make([]Gear, 0)
	for rows.Next() {
		gear, err := scanGear(rows, true)
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
			total_distance_m, max_distance_m, first_used_at, last_used_at,
			coalesce((select count(*)::int from activity_gears where gear_id = id), 0) as activity_count,
			default_activity_types, raw, stats_raw, created_at, updated_at
		from gears
		where id = $1 and user_id = $2
	`, id, scopedUserID(ctx)), true)
}

func (s *Store) ListGearActivities(ctx context.Context, gearID string) ([]Activity, error) {
	rows, err := s.db.Query(ctx, activitySelectSQL+`
		join activity_gears on activity_gears.activity_id = activities.id
		where activity_gears.gear_id = $1 and activities.user_id = $2
		order by activities.start_time desc, activities.id desc
	`, gearID, scopedUserID(ctx))
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
		where activity_gears.activity_id = $1 and gears.user_id = $2
		order by gears.retired, lower(gears.name), gears.provider_gear_id
	`, activityID, scopedUserID(ctx))
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
		where activity_gears.activity_id::text = any($1) and gears.user_id = $2
		order by gears.retired, lower(gears.name), gears.provider_gear_id
	`, activityIDs, scopedUserID(ctx))
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
		where id = $1 and user_id = $2
		for update
	`, id, scopedUserID(ctx)).Scan(&source, &sourceID, &hasSourceFile, &name, &sportType, &startTime)
	if err != nil {
		return DeleteActivityResult{}, err
	}

	result := DeleteActivityResult{Deleted: true}
	if isProviderSyncedSource(source, sourceID, !hasSourceFile) {
		_, err = tx.Exec(ctx, `
			insert into sync_excluded_activities(user_id, source, source_id, name, sport_type, start_time)
			values($1, $2, $3, $4, $5, $6)
			on conflict(user_id, source, source_id) do update set
				name = excluded.name,
				sport_type = excluded.sport_type,
				start_time = excluded.start_time,
				reason = 'deleted_from_runnarr'
		`, scopedUserID(ctx), source, sourceID, name, sportType, startTime)
		if err != nil {
			return DeleteActivityResult{}, err
		}
		result.ExcludedFromSync = true
		result.SyncExclusionMessage = "This synced activity will be ignored in future imports."
	}
	if _, err = tx.Exec(ctx, `
		update planned_activities
		set status = 'pending', matched_activity_id = null, matched_at = null, updated_at = now()
		where matched_activity_id = $1 and user_id = $2
	`, id, scopedUserID(ctx)); err != nil {
		return DeleteActivityResult{}, err
	}

	if _, err = tx.Exec(ctx, `delete from activities where id = $1 and user_id = $2`, id, scopedUserID(ctx)); err != nil {
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
		where activity_id = $1 and exists (select 1 from activities where activities.id = activity_media.activity_id and activities.user_id = $2)
		order by coalesce(capture_time, created_at) asc, created_at asc
	`, activityID, scopedUserID(ctx))
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
		where id = $1 and exists (select 1 from activities where activities.id = activity_media.activity_id and activities.user_id = $2)
	`, id, scopedUserID(ctx))
	return media, scanActivityMedia(row, &media)
}

func (s *Store) GetActivityMediaByHash(ctx context.Context, activityID, hash string) (ActivityMedia, error) {
	var media ActivityMedia
	row := s.db.QueryRow(ctx, `
		select id::text, activity_id::text, original_filename, content_type, size_bytes, sha256,
			original_path, thumbnail_path, width, height, capture_time, latitude, longitude, created_at
		from activity_media
		where activity_id = $1 and sha256 = $2 and exists (select 1 from activities where activities.id = activity_media.activity_id and activities.user_id = $3)
	`, activityID, hash, scopedUserID(ctx))
	return media, scanActivityMedia(row, &media)
}

func (s *Store) DeleteActivityMedia(ctx context.Context, activityID, mediaID string) (ActivityMedia, error) {
	var media ActivityMedia
	row := s.db.QueryRow(ctx, `
		delete from activity_media
		where activity_id = $1 and id = $2 and exists (select 1 from activities where activities.id = activity_media.activity_id and activities.user_id = $3)
		returning id::text, activity_id::text, original_filename, content_type, size_bytes, sha256,
			original_path, thumbnail_path, width, height, capture_time, latitude, longitude, created_at
	`, activityID, mediaID, scopedUserID(ctx))
	return media, scanActivityMedia(row, &media)
}

func (s *Store) UpdateActivityMediaLocation(ctx context.Context, activityID, mediaID string, latitude, longitude *float64) (ActivityMedia, error) {
	var media ActivityMedia
	row := s.db.QueryRow(ctx, `
		update activity_media
		set latitude = $3, longitude = $4
		where activity_id = $1 and id = $2 and exists (select 1 from activities where activities.id = activity_media.activity_id and activities.user_id = $5)
		returning id::text, activity_id::text, original_filename, content_type, size_bytes, sha256,
			original_path, thumbnail_path, width, height, capture_time, latitude, longitude, created_at
	`, activityID, mediaID, latitude, longitude, scopedUserID(ctx))
	return media, scanActivityMedia(row, &media)
}

func (s *Store) RenameActivity(ctx context.Context, id, requestedName string) (Activity, error) {
	var sourceName string
	if err := s.db.QueryRow(ctx, `select name from activities where id = $1 and user_id = $2`, id, scopedUserID(ctx)).Scan(&sourceName); err != nil {
		return Activity{}, err
	}
	localName, err := localActivityNameOverride(requestedName, sourceName)
	if err != nil {
		return Activity{}, err
	}
	if _, err := s.db.Exec(ctx, `
		update activities
		set local_name = $2, updated_at = now()
		where id = $1 and user_id = $3
	`, id, localName, scopedUserID(ctx)); err != nil {
		return Activity{}, err
	}
	return s.GetActivity(ctx, id)
}

func (s *Store) UpdateActivityNotes(ctx context.Context, id, requestedNotes string) (Activity, error) {
	notes, err := localActivityNotesValue(requestedNotes)
	if err != nil {
		return Activity{}, err
	}
	if _, err := s.db.Exec(ctx, `
		update activities
		set local_notes = $2, updated_at = now()
		where id = $1 and user_id = $3
	`, id, notes, scopedUserID(ctx)); err != nil {
		return Activity{}, err
	}
	return s.GetActivity(ctx, id)
}

func (s *Store) UpdateActivityFeedback(ctx context.Context, id string, requestedFeedback *string, rpeSet bool, requestedRPE *int) (Activity, error) {
	feedback := ""
	if requestedFeedback != nil {
		var err error
		feedback, err = localActivityFeedbackValue(*requestedFeedback)
		if err != nil {
			return Activity{}, err
		}
	}
	if requestedRPE != nil && (*requestedRPE < 1 || *requestedRPE > 10) {
		return Activity{}, ErrInvalidActivityRPE
	}
	if _, err := s.db.Exec(ctx, `
		update activities
		set local_feedback = case when $2 then $3 else local_feedback end,
			rpe = case when $4 then $5 else rpe end,
			updated_at = now()
		where id = $1 and user_id = $6
	`, id, requestedFeedback != nil, feedback, rpeSet, requestedRPE, scopedUserID(ctx)); err != nil {
		return Activity{}, err
	}
	return s.GetActivity(ctx, id)
}

func (s *Store) Summary(ctx context.Context, filters ActivityFilters) (SummaryStats, error) {
	var stats SummaryStats
	stats.Recent = make([]Activity, 0)
	stats.WeeklyDistance = make([]WeeklyBucket, 0)
	stats.DistanceBuckets = make([]SummaryBucket, 0)
	stats.SummaryPeriod = normalizeSummaryPeriod(filters.SummaryPeriod)
	where, args := activityFilterWhereForUser(filters, 1, scopedUserID(ctx))
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

	bucketTrunc, bucketWindow := summaryBucketSQL(stats.SummaryPeriod)
	weeklyConditions, weeklyArgs := activityFilterConditionsForUser(filters, 1, scopedUserID(ctx))
	weeklyConditions = append([]string{`start_time >= now() - ` + bucketWindow}, weeklyConditions...)
	weeklyRows, err := s.db.Query(ctx, `
		select date_trunc('`+bucketTrunc+`', start_time)::timestamptz as bucket_start, coalesce(sum(distance_m), 0)
		from activities
		where `+strings.Join(weeklyConditions, " and ")+`
		group by bucket_start
		order by bucket_start
	`, weeklyArgs...)
	if err != nil {
		return stats, err
	}
	defer weeklyRows.Close()
	for weeklyRows.Next() {
		var bucket SummaryBucket
		if err := weeklyRows.Scan(&bucket.Start, &bucket.DistanceM); err != nil {
			return stats, err
		}
		stats.DistanceBuckets = append(stats.DistanceBuckets, bucket)
		if stats.SummaryPeriod == "weekly" {
			stats.WeeklyDistance = append(stats.WeeklyDistance, WeeklyBucket{WeekStart: bucket.Start, DistanceM: bucket.DistanceM})
		}
	}
	return stats, weeklyRows.Err()
}

func summaryBucketSQL(period string) (string, string) {
	switch normalizeSummaryPeriod(period) {
	case "monthly":
		return "month", "interval '12 months'"
	case "yearly":
		return "year", "interval '5 years'"
	default:
		return "week", "interval '12 weeks'"
	}
}

func (s *Store) ListSportTypes(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		select distinct sport_type
		from activities
		where user_id = $1 and sport_type <> ''
		order by sport_type
	`, scopedUserID(ctx))
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
			user_id, provider, provider_account_id, display_name, access_token_ciphertext,
			refresh_token_ciphertext, token_expires_at, scopes, metadata
		)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9)
		on conflict(user_id, provider) do update set
			provider_account_id = excluded.provider_account_id,
			display_name = excluded.display_name,
			access_token_ciphertext = excluded.access_token_ciphertext,
			refresh_token_ciphertext = excluded.refresh_token_ciphertext,
			token_expires_at = excluded.token_expires_at,
			scopes = excluded.scopes,
			metadata = excluded.metadata,
			updated_at = now()
	`, scopedUserID(ctx), conn.Provider, conn.ProviderAccountID, conn.DisplayName, conn.AccessTokenCiphertext,
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
		where user_id = $1 and provider = $2
	`, scopedUserID(ctx), provider).Scan(&conn.ID, &conn.Provider, &conn.ProviderAccountID, &conn.DisplayName,
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
	return s.CreateSyncJobWithPayload(ctx, provider, kind, nil)
}

func (s *Store) CreateSyncJobWithPayload(ctx context.Context, provider, kind string, payload map[string]any) (string, error) {
	payloadBytes := []byte(`{}`)
	if payload != nil {
		var err error
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			return "", err
		}
	}
	var id string
	err := s.db.QueryRow(ctx, `
		insert into sync_jobs(user_id, provider, kind, status, payload, started_at)
		values($1, $2, $3, 'running', $4, now())
		returning id::text
	`, scopedUserID(ctx), provider, kind, payloadBytes).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "sync_jobs_active_user_provider_idx" {
		return "", ErrSyncJobAlreadyRunning
	}
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
		where id = $1 and user_id = $3 and status = 'running'
	`, id, payloadBytes, scopedUserID(ctx))
	return err
}

func (s *Store) HasRunningSyncJob(ctx context.Context, provider string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		select exists(
			select 1
			from sync_jobs
			where user_id = $1 and provider = $2 and status = 'running'
		)
	`, scopedUserID(ctx), provider).Scan(&exists)
	return exists, err
}

func (s *Store) LatestSyncJobCreatedAt(ctx context.Context, provider, kind string) (time.Time, bool, error) {
	var createdAt time.Time
	err := s.db.QueryRow(ctx, `
		select created_at
		from sync_jobs
		where user_id = $1 and provider = $2 and kind = $3
		order by created_at desc
		limit 1
	`, scopedUserID(ctx), provider, kind).Scan(&createdAt)
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
			set status = case when cancel_requested_at is not null then 'canceled' else $2 end,
				error = case when cancel_requested_at is not null then 'Canceled by user' else $3 end,
				finished_at = now()
			where id = $1 and user_id = $4
		`, id, status, message, scopedUserID(ctx))
		return err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		update sync_jobs
		set status = case when cancel_requested_at is not null then 'canceled' else $2 end,
			error = case when cancel_requested_at is not null then 'Canceled by user' else $3 end,
			payload = $4, finished_at = now()
		where id = $1 and user_id = $5
	`, id, status, message, payloadBytes, scopedUserID(ctx))
	return err
}

func (s *Store) RequestSyncJobCancellation(ctx context.Context, id string) (string, bool, error) {
	var status string
	var cancelRequestedAt pgtype.Timestamptz
	err := s.db.QueryRow(ctx, `
		update sync_jobs
		set cancel_requested_at = coalesce(cancel_requested_at, now())
		where id = $1 and user_id = $2 and status = 'running'
		returning status, cancel_requested_at
	`, id, scopedUserID(ctx)).Scan(&status, &cancelRequestedAt)
	if err == nil {
		return status, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}

	err = s.db.QueryRow(ctx, `
		select status, cancel_requested_at
		from sync_jobs
		where id = $1 and user_id = $2
	`, id, scopedUserID(ctx)).Scan(&status, &cancelRequestedAt)
	if err != nil {
		return "", false, err
	}
	return status, false, nil
}

func (s *Store) SyncJobCancellationRequested(ctx context.Context, id string) (bool, error) {
	var requested bool
	err := s.db.QueryRow(ctx, `
		select cancel_requested_at is not null
		from sync_jobs
		where id = $1 and user_id = $2
	`, id, scopedUserID(ctx)).Scan(&requested)
	return requested, err
}

func (s *Store) ReconcileRunningSyncJobs(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		update sync_jobs
		set status = case when cancel_requested_at is not null then 'canceled' else 'failed' end,
			error = case when cancel_requested_at is not null
				then 'Canceled by user'
				else 'Sync stopped because the server restarted'
			end,
			finished_at = now()
		where status = 'running'
	`)
	return err
}

func (s *Store) ListSyncJobs(ctx context.Context) ([]SyncJob, error) {
	rows, err := s.db.Query(ctx, `
		select id::text, provider, kind, status, payload, error, created_at, started_at, finished_at, cancel_requested_at
		from sync_jobs
		where user_id = $1
		order by created_at desc
		limit 50
	`, scopedUserID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]SyncJob, 0)
	for rows.Next() {
		var job SyncJob
		var payloadBytes []byte
		var startedAt, finishedAt, cancelRequestedAt pgtype.Timestamptz
		if err := rows.Scan(&job.ID, &job.Provider, &job.Kind, &job.Status, &payloadBytes, &job.Error, &job.CreatedAt, &startedAt, &finishedAt, &cancelRequestedAt); err != nil {
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
		if cancelRequestedAt.Valid {
			job.CancelRequestedAt = &cancelRequestedAt.Time
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) SyncJobUserID(ctx context.Context, id string) (string, error) {
	var userID string
	err := s.db.QueryRow(ctx, `select user_id::text from sync_jobs where id = $1`, id).Scan(&userID)
	return userID, err
}

func (s *Store) listSamples(ctx context.Context, activityID string) ([]ActivitySample, error) {
	rows, err := s.db.Query(ctx, `
		select sample_index, timestamp, elapsed_s, distance_m, latitude, longitude, elevation_m, heart_rate, cadence, power, speed_mps
		from activity_samples
		where activity_id = $1 and exists (select 1 from activities where activities.id = activity_samples.activity_id and activities.user_id = $2)
		order by sample_index
	`, activityID, scopedUserID(ctx))
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

func (s *Store) getActivityWorkout(ctx context.Context, activityID string) (*ActivityWorkout, []ActivityInterval, error) {
	var workout ActivityWorkout
	var stepsBytes, rawBytes []byte
	err := s.db.QueryRow(ctx, `
		select provider, provider_workout_id, name, sport_type, steps, raw
		from activity_workouts
		where activity_id = $1 and exists (select 1 from activities where activities.id = activity_workouts.activity_id and activities.user_id = $2)
	`, activityID, scopedUserID(ctx)).Scan(&workout.Provider, &workout.ProviderWorkoutID, &workout.Name, &workout.SportType, &stepsBytes, &rawBytes)
	var workoutPtr *ActivityWorkout
	if errors.Is(err, pgx.ErrNoRows) {
		workoutPtr = nil
	} else if err != nil {
		return nil, nil, err
	} else {
		if len(stepsBytes) > 0 {
			if err := json.Unmarshal(stepsBytes, &workout.Steps); err != nil {
				return nil, nil, err
			}
		}
		if len(rawBytes) > 0 {
			if err := json.Unmarshal(rawBytes, &workout.Raw); err != nil {
				return nil, nil, err
			}
		}
		workoutPtr = &workout
	}

	rows, err := s.db.Query(ctx, `
		select interval_index, category, provider_type, workout_step_index, workout_repeat_index,
			start_time, end_time, elapsed_time_s, moving_time_s, distance_m,
			avg_pace_s_per_km, avg_grade_adjusted_pace_s_per_km, avg_heart_rate, max_heart_rate,
			avg_power, max_power, normalized_power, avg_run_cadence, avg_ground_contact_time_ms,
			avg_respiration_rate, avg_temperature_c, elevation_gain_m, elevation_loss_m, calories_kcal,
			lap_indexes, raw
		from activity_intervals
		where activity_id = $1
		order by interval_index
	`, activityID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	intervals := make([]ActivityInterval, 0)
	for rows.Next() {
		var interval ActivityInterval
		var startTime, endTime pgtype.Timestamptz
		var workoutStepIndex, workoutRepeatIndex sql.NullInt32
		var avgPace, avgGap, avgHR, maxHR, avgPower, maxPower, normalizedPower sql.NullFloat64
		var avgCadence, avgGroundContact, avgRespiration, avgTemperature sql.NullFloat64
		var elevationGain, elevationLoss sql.NullFloat64
		var calories sql.NullInt32
		var lapIndexes []int32
		var intervalRaw []byte
		if err := rows.Scan(&interval.Index, &interval.Category, &interval.ProviderType, &workoutStepIndex, &workoutRepeatIndex,
			&startTime, &endTime, &interval.ElapsedTimeS, &interval.MovingTimeS, &interval.DistanceM,
			&avgPace, &avgGap, &avgHR, &maxHR, &avgPower, &maxPower, &normalizedPower, &avgCadence, &avgGroundContact,
			&avgRespiration, &avgTemperature, &elevationGain, &elevationLoss, &calories, &lapIndexes, &intervalRaw); err != nil {
			return nil, nil, err
		}
		if startTime.Valid {
			interval.StartTime = &startTime.Time
		}
		if endTime.Valid {
			interval.EndTime = &endTime.Time
		}
		interval.WorkoutStepIndex = intPtrFromNull(workoutStepIndex)
		interval.WorkoutRepeatIndex = intPtrFromNull(workoutRepeatIndex)
		interval.AvgPaceSPKM = floatPtrFromNull(avgPace)
		interval.AvgGradeAdjustedPaceSPKM = floatPtrFromNull(avgGap)
		interval.AvgHeartRate = floatPtrFromNull(avgHR)
		interval.MaxHeartRate = floatPtrFromNull(maxHR)
		interval.AvgPower = floatPtrFromNull(avgPower)
		interval.MaxPower = floatPtrFromNull(maxPower)
		interval.NormalizedPower = floatPtrFromNull(normalizedPower)
		interval.AvgRunCadence = floatPtrFromNull(avgCadence)
		interval.AvgGroundContactTimeMS = floatPtrFromNull(avgGroundContact)
		interval.AvgRespirationRate = floatPtrFromNull(avgRespiration)
		interval.AvgTemperatureC = floatPtrFromNull(avgTemperature)
		interval.ElevationGainM = floatPtrFromNull(elevationGain)
		interval.ElevationLossM = floatPtrFromNull(elevationLoss)
		interval.CaloriesKcal = intPtrFromNull(calories)
		for _, lapIndex := range lapIndexes {
			interval.LapIndexes = append(interval.LapIndexes, int(lapIndex))
		}
		if len(intervalRaw) > 0 {
			_ = json.Unmarshal(intervalRaw, &interval.Raw)
		}
		intervals = append(intervals, interval)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return workoutPtr, intervals, nil
}

func (s *Store) listLaps(ctx context.Context, activityID string) ([]ActivityLap, error) {
	rows, err := s.db.Query(ctx, `
		select lap_index, start_time, elapsed_time_s, moving_time_s, distance_m,
			avg_pace_s_per_km, elevation_gain_m, elevation_loss_m, avg_grade_adjusted_pace_s_per_km,
			avg_heart_rate, max_heart_rate, avg_power, max_power, normalized_power,
			avg_run_cadence, avg_ground_contact_time_ms, avg_respiration_rate, avg_temperature_c,
			intensity_type, workout_step_index, workout_repeat_index, raw
		from activity_laps
		where activity_id = $1 and exists (select 1 from activities where activities.id = activity_laps.activity_id and activities.user_id = $2)
		order by lap_index
	`, activityID, scopedUserID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	laps := make([]ActivityLap, 0)
	for rows.Next() {
		var lap ActivityLap
		var ts pgtype.Timestamptz
		var avgPace, gain, loss, avgGradeAdjustedPace sql.NullFloat64
		var avgHR, maxHR, avgPower, maxPower, normalizedPower sql.NullFloat64
		var avgCadence, avgGroundContact, avgRespiration, avgTemperature sql.NullFloat64
		var intensityType sql.NullString
		var workoutStepIndex, workoutRepeatIndex sql.NullInt32
		var rawBytes []byte
		if err := rows.Scan(&lap.Index, &ts, &lap.ElapsedTimeS, &lap.MovingTimeS, &lap.DistanceM, &avgPace, &gain, &loss, &avgGradeAdjustedPace,
			&avgHR, &maxHR, &avgPower, &maxPower, &normalizedPower, &avgCadence, &avgGroundContact, &avgRespiration, &avgTemperature,
			&intensityType, &workoutStepIndex, &workoutRepeatIndex, &rawBytes); err != nil {
			return nil, err
		}
		lap.AvgPaceSPKM = floatPtrFromNull(avgPace)
		if ts.Valid {
			lap.StartTime = &ts.Time
		}
		lap.ElevationGainM = floatPtrFromNull(gain)
		lap.ElevationLossM = floatPtrFromNull(loss)
		lap.AvgGradeAdjustedPaceSPKM = floatPtrFromNull(avgGradeAdjustedPace)
		lap.AvgHeartRate = floatPtrFromNull(avgHR)
		lap.MaxHeartRate = floatPtrFromNull(maxHR)
		lap.AvgPower = floatPtrFromNull(avgPower)
		lap.MaxPower = floatPtrFromNull(maxPower)
		lap.NormalizedPower = floatPtrFromNull(normalizedPower)
		lap.AvgRunCadence = floatPtrFromNull(avgCadence)
		lap.AvgGroundContactTimeMS = floatPtrFromNull(avgGroundContact)
		lap.AvgRespirationRate = floatPtrFromNull(avgRespiration)
		lap.AvgTemperatureC = floatPtrFromNull(avgTemperature)
		if intensityType.Valid {
			lap.IntensityType = intensityType.String
		}
		lap.WorkoutStepIndex = intPtrFromNull(workoutStepIndex)
		lap.WorkoutRepeatIndex = intPtrFromNull(workoutRepeatIndex)
		if len(rawBytes) > 0 {
			_ = json.Unmarshal(rawBytes, &lap.Raw)
		}
		laps = append(laps, lap)
	}
	return laps, rows.Err()
}

const activitySelectSQL = `
	select activities.id::text, activities.source, activities.source_id, coalesce(nullif(activities.local_name, ''), activities.name),
		activities.name, activities.local_name, activities.local_notes, activities.local_feedback, activities.rpe, activities.sport_type, activities.start_time, activities.distance_m,
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
	var calories, rpe sql.NullInt32
	if err := row.Scan(&activity.ID, &activity.Source, &activity.SourceID, &activity.Name, &activity.SourceName, &activity.LocalName, &activity.Notes, &activity.Feedback, &rpe, &activity.SportType,
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
	activity.RPE = intPtrFromNull(rpe)
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

func scanGear(row rowScanner, includeActivityCount bool) (Gear, error) {
	var gear Gear
	var totalDistance, maxDistance sql.NullFloat64
	var firstUsed, lastUsed pgtype.Timestamptz
	var rawBytes, statsBytes []byte
	dest := []any{
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
	}
	if includeActivityCount {
		dest = append(dest, &gear.ActivityCount)
	}
	dest = append(dest,
		&gear.DefaultActivityTypes,
		&rawBytes,
		&statsBytes,
		&gear.CreatedAt,
		&gear.UpdatedAt,
	)
	if err := row.Scan(dest...); err != nil {
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

func activityFilterWhereForUser(filters ActivityFilters, startArg int, userID string) (string, []any) {
	conditions, args := activityFilterConditionsForUser(filters, startArg, userID)
	if len(conditions) == 0 {
		return "", args
	}
	return " where " + strings.Join(conditions, " and "), args
}

func activityFilterConditions(filters ActivityFilters, startArg int) ([]string, []any) {
	return activityFilterConditionsForUser(filters, startArg, "")
}

func activityFilterConditionsForUser(filters ActivityFilters, startArg int, userID string) ([]string, []any) {
	conditions := make([]string, 0, 5)
	args := make([]any, 0, 5)
	nextArg := startArg
	if userID != "" {
		conditions = append(conditions, fmt.Sprintf("activities.user_id = $%d", nextArg))
		args = append(args, userID)
		nextArg++
	}
	timezoneArg := 0
	activityDateExpression := "date(start_time)"
	if filters.CalendarTimezone != "" {
		timezoneArg = nextArg
		args = append(args, filters.CalendarTimezone)
		nextArg++
		activityDateExpression = calendarActivityDateExpression(timezoneArg)
	}
	if !filters.IncludeTrainingSheet {
		conditions = append(conditions, "source <> 'training_sheet'")
	} else {
		if timezoneArg > 0 {
			conditions = append(conditions, fmt.Sprintf(`(source <> 'training_sheet' or (%s >= date(now() at time zone $%d) and not exists (
			select 1 from planned_activities
			where planned_activities.source = 'training_sheet'
				and planned_activities.source_id = activities.source_id
				and planned_activities.user_id = activities.user_id
				and planned_activities.status = 'completed'
			)))`, activityDateExpression, timezoneArg))
		} else {
			conditions = append(conditions, `(source <> 'training_sheet' or (date(start_time) >= current_date and not exists (
			select 1 from planned_activities
			where planned_activities.source = 'training_sheet'
				and planned_activities.source_id = activities.source_id
				and planned_activities.user_id = activities.user_id
				and planned_activities.status = 'completed'
			)))`)
		}
	}
	if strings.TrimSpace(filters.Search) != "" {
		conditions = append(conditions, fmt.Sprintf("coalesce(nullif(local_name, ''), name) ilike $%d", nextArg))
		args = append(args, "%"+strings.TrimSpace(filters.Search)+"%")
		nextArg++
	}
	if !filters.DateFrom.IsZero() {
		if filters.CalendarTimezone != "" {
			conditions = append(conditions, fmt.Sprintf("%s >= $%d::date", activityDateExpression, nextArg))
			args = append(args, filters.DateFrom.Format("2006-01-02"))
		} else {
			conditions = append(conditions, fmt.Sprintf("start_time >= $%d", nextArg))
			args = append(args, filters.DateFrom)
		}
		nextArg++
	}
	if !filters.DateTo.IsZero() {
		if filters.CalendarTimezone != "" {
			conditions = append(conditions, fmt.Sprintf("%s <= $%d::date", activityDateExpression, nextArg))
			args = append(args, filters.DateTo.Format("2006-01-02"))
		} else {
			conditions = append(conditions, fmt.Sprintf("start_time < $%d", nextArg))
			args = append(args, filters.DateTo.AddDate(0, 0, 1))
		}
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

func calendarActivityDateExpression(timezoneArg int) string {
	if timezoneArg <= 0 {
		return "date(start_time)"
	}
	return fmt.Sprintf("case when source = 'training_sheet' then date(start_time) else date(start_time at time zone $%d) end", timezoneArg)
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

func localActivityFeedbackValue(requestedFeedback string) (string, error) {
	feedback := strings.TrimSpace(requestedFeedback)
	if utf8.RuneCountInString(feedback) > 5000 {
		return "", ErrInvalidActivityFeedback
	}
	return feedback, nil
}

func localActivityNotesValue(requestedNotes string) (string, error) {
	notes := strings.TrimSpace(requestedNotes)
	if utf8.RuneCountInString(notes) > 5000 {
		return "", ErrInvalidActivityNotes
	}
	return notes, nil
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
