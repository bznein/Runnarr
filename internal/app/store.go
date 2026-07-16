package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

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

	var id string
	err = tx.QueryRow(ctx, `
		insert into activities(
			source, source_id, source_file_id, name, sport_type, start_time, distance_m,
			moving_time_s, elapsed_time_s, elevation_gain_m, avg_heart_rate, max_heart_rate,
			avg_pace_s_per_km, summary_polyline, raw
		)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
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
			summary_polyline = excluded.summary_polyline,
			raw = excluded.raw,
			updated_at = now()
		returning id::text
	`, source, sourceID, sourceFileID, fallbackName(activity), activity.SportType, activity.StartTime,
		activity.DistanceM, activity.MovingTimeS, activity.ElapsedTimeS, activity.ElevationGainM,
		activity.AvgHeartRate, activity.MaxHeartRate, avgPace, activity.SummaryPolyline, rawBytes).Scan(&id)
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
			insert into activity_laps(activity_id, lap_index, start_time, elapsed_time_s, distance_m)
			values($1,$2,$3,$4,$5)
		`, id, lap.Index, lap.StartTime, lap.ElapsedTimeS, lap.DistanceM)
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
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	where, args := activityFilterWhere(filters, 1)
	orderBy := activityOrderBy(filters.SortBy, filters.SortOrder)
	args = append(args, limit, offset)
	limitParam := len(args) - 1
	offsetParam := len(args)
	rows, err := s.db.Query(ctx, activitySelectSQL+where+fmt.Sprintf(` %s limit $%d offset $%d`, orderBy, limitParam, offsetParam), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActivities(rows)
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
	return activity, nil
}

func (s *Store) DeleteActivity(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `delete from activities where id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
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
		insert into sync_jobs(provider, kind, status)
		values($1, $2, 'running')
		returning id::text
	`, provider, kind).Scan(&id)
	return id, err
}

func (s *Store) FinishSyncJob(ctx context.Context, id, status, message string, payload map[string]any) error {
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
		select lap_index, start_time, elapsed_time_s, distance_m
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
		if err := rows.Scan(&lap.Index, &ts, &lap.ElapsedTimeS, &lap.DistanceM); err != nil {
			return nil, err
		}
		if ts.Valid {
			lap.StartTime = &ts.Time
		}
		laps = append(laps, lap)
	}
	return laps, rows.Err()
}

const activitySelectSQL = `
	select id::text, source, source_id, name, sport_type, start_time, distance_m,
		moving_time_s, elapsed_time_s, elevation_gain_m, avg_heart_rate, max_heart_rate,
		avg_pace_s_per_km, summary_polyline, created_at
	from activities
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
	var avgHR, maxHR, avgPace sql.NullFloat64
	if err := row.Scan(&activity.ID, &activity.Source, &activity.SourceID, &activity.Name, &activity.SportType,
		&activity.StartTime, &activity.DistanceM, &activity.MovingTimeS, &activity.ElapsedTimeS,
		&activity.ElevationGainM, &avgHR, &maxHR, &avgPace, &activity.SummaryPolyline, &activity.CreatedAt); err != nil {
		return err
	}
	activity.AvgHeartRate = floatPtrFromNull(avgHR)
	activity.MaxHeartRate = floatPtrFromNull(maxHR)
	activity.AvgPaceSPKM = floatPtrFromNull(avgPace)
	return nil
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
		conditions = append(conditions, fmt.Sprintf("name ilike $%d", nextArg))
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

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
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
