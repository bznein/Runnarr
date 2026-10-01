package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const raceSelect = `select id::text,name,coalesce(race_date::text,''),status,discipline,kind,distance_m,
    coalesce(group_id::text,''),coalesce(activity_id::text,''),coalesce(planned_activity_id::text,''),coalesce(course_id::text,''),
    data,course_snapshot,recording_hash,revision,created_at,updated_at from races`

func scanRace(row interface{ Scan(...any) error }) (Race, error) {
	var r Race
	var input RaceInput
	var data, snapshot []byte
	err := row.Scan(&r.ID, &r.Name, &r.Date, &r.Status, &r.Discipline, &r.Kind, &r.DistanceM, &r.GroupID, &r.ActivityID, &r.PlannedActivityID, &r.CourseID, &data, &snapshot, &r.RecordingHash, &r.Revision, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(data, &input); err != nil {
		return r, err
	}
	// Indexed columns and live foreign keys are authoritative after cascading deletions.
	input.Name, input.Date, input.Status, input.Discipline, input.Kind, input.DistanceM = r.Name, r.Date, r.Status, r.Discipline, r.Kind, r.DistanceM
	input.GroupID, input.ActivityID, input.PlannedActivityID, input.CourseID, input.Revision = r.GroupID, r.ActivityID, r.PlannedActivityID, r.CourseID, r.Revision
	if input.ActivityID == "" {
		input.RaceOnly = false
	}
	r.RaceInput = input
	if len(snapshot) > 0 {
		if err = json.Unmarshal(snapshot, &r.CourseSnapshot); err != nil {
			return r, err
		}
	}
	return r, nil
}
func (s *Store) GetRace(ctx context.Context, id string) (Race, error) {
	return scanRace(s.db.QueryRow(ctx, raceSelect+` where id=$1 and user_id=$2`, id, scopedUserID(ctx)))
}

func (s *Store) ListRaces(ctx context.Context, o raceListOptions) (RacePage, error) {
	page := RacePage{Races: []Race{}}
	if o.Limit <= 0 || o.Limit > 100 {
		o.Limit = 100
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	where := ` where user_id=$1`
	args := []any{scopedUserID(ctx)}
	add := func(clause string, value any) { args = append(args, value); where += fmt.Sprintf(clause, len(args)) }
	if o.Query != "" {
		add(` and name ilike '%%'||$%d||'%%'`, o.Query)
	}
	for _, item := range [][2]string{{"discipline", o.Discipline}, {"kind", o.Kind}, {"status", o.Status}, {"group_id", o.GroupID}} {
		if item[1] != "" {
			add(` and `+item[0]+`=$%d`, item[1])
		}
	}
	if o.From != "" {
		if !validRaceDate(o.From) {
			return page, ErrRaceInvalid
		}
		add(` and race_date >= $%d::date`, o.From)
	}
	if o.To != "" {
		if !validRaceDate(o.To) {
			return page, ErrRaceInvalid
		}
		add(` and race_date <= $%d::date`, o.To)
	}
	switch o.View {
	case "upcoming":
		where += ` and status in ('wishlist','planned','registered','postponed')`
	case "history":
		where += ` and status in ('finished','dns','dnf','disqualified','cancelled')`
	case "", "all":
	default:
		return page, ErrRaceInvalid
	}
	order := `race_date`
	switch o.Sort {
	case "", "date":
	case "name":
		order = `lower(name)`
	case "distance":
		order = `distance_m`
	case "time":
		order = `coalesce((data->'result'->>'chipTimeMs')::bigint,(data->'result'->>'gunTimeMs')::bigint,(data->'result'->>'manualTimeMs')::bigint)`
	default:
		return page, ErrRaceInvalid
	}
	direction := "desc"
	if o.Order == "asc" || o.Order == "" && o.View == "upcoming" {
		direction = "asc"
	} else if o.Order != "" && o.Order != "desc" {
		return page, ErrRaceInvalid
	}
	args = append(args, o.Limit+1, o.Offset)
	listSelect := strings.Replace(raceSelect, "data,course_snapshot,recording_hash", "data,null::jsonb,recording_hash", 1)
	rows, err := s.db.Query(ctx, listSelect+where+fmt.Sprintf(` order by %s %s nulls last,id limit $%d offset $%d`, order, direction, len(args)-1, len(args)), args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanRace(rows)
		if err != nil {
			return page, err
		}
		page.Races = append(page.Races, r)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Races) > o.Limit {
		page.HasMore = true
		page.Races = page.Races[:o.Limit]
		page.NextOffset = o.Offset + o.Limit
	}
	return page, nil
}

func raceNullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func raceLinkOwner(ctx context.Context, tx pgx.Tx, table, id, user string) error {
	if id == "" {
		return nil
	}
	// table is only supplied by the fixed callers below, never by an HTTP parameter.
	var found string
	err := tx.QueryRow(ctx, `select id::text from `+table+` where id=$1 and user_id=$2 for share`, id, user).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: linked object is unavailable", ErrRaceInvalid)
	}
	return err
}
func raceStoreError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return fmt.Errorf("%w: activity is already attached to a race or name is already used", ErrRaceConflict)
		case "22P02", "23514", "22007", "22008":
			return ErrRaceInvalid
		case "23503":
			return fmt.Errorf("%w: linked object no longer exists", ErrRaceConflict)
		}
	}
	return err
}

func (s *Store) SaveRace(ctx context.Context, id string, input RaceInput) (Race, error) {
	if err := validateRace(&input); err != nil {
		return Race{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Race{}, err
	}
	defer tx.Rollback(ctx)
	user := scopedUserID(ctx)
	// Match the importer's lock order: activity, then race. Samples stay stable while confirming.
	if err = raceLinkOwner(ctx, tx, "activities", input.ActivityID, user); err != nil {
		return Race{}, err
	}
	var current Race
	if id != "" {
		current, err = scanRace(tx.QueryRow(ctx, raceSelect+` where id=$1 and user_id=$2 for update`, id, user))
		if err != nil {
			return Race{}, err
		}
		if current.Revision != input.Revision {
			return Race{}, ErrRaceConflict
		}
	}
	for _, link := range [][2]string{{"race_groups", input.GroupID}, {"planned_activities", input.PlannedActivityID}, {"courses", input.CourseID}} {
		if err = raceLinkOwner(ctx, tx, link[0], link[1], user); err != nil {
			return Race{}, err
		}
	}
	var hash string
	if input.ActivityID != "" {
		var source, sport string
		if err = tx.QueryRow(ctx, `select source,sport_type from activities where id=$1 and user_id=$2`, input.ActivityID, user).Scan(&source, &sport); err != nil {
			return Race{}, err
		}
		if source == trainingSheetProvider || !raceRunningSport(sport) {
			return Race{}, fmt.Errorf("%w: link a recorded running activity", ErrRaceInvalid)
		}
		if input.RaceOnly {
			a, err := raceRecordingTx(ctx, tx, input.ActivityID)
			if err != nil {
				return Race{}, err
			}
			hash = raceRecordingHash(a.StartTime, a.DistanceM, a.ElapsedTimeS, a.Samples)
		}
	}
	snapshot := current.CourseSnapshot
	if input.RemoveCourse {
		snapshot = nil
		input.CourseID = ""
	} else if input.CourseID != "" && (input.CourseID != current.CourseID || input.RefreshCourse || snapshot == nil) {
		course, err := readCourse(ctx, tx, input.CourseID, false)
		if err != nil {
			return Race{}, err
		}
		if course.SportType != CourseSportRun {
			return Race{}, fmt.Errorf("%w: select a running course", ErrRaceInvalid)
		}
		snapshot = &course
	}
	input.RefreshCourse, input.RemoveCourse = false, false
	data, err := json.Marshal(input)
	if err != nil {
		return Race{}, err
	}
	var snapshotJSON any
	if snapshot != nil {
		b, err := json.Marshal(snapshot)
		if err != nil {
			return Race{}, err
		}
		snapshotJSON = b
	}
	if id == "" {
		err = tx.QueryRow(ctx, `insert into races(user_id,name,race_date,status,discipline,kind,distance_m,group_id,activity_id,planned_activity_id,course_id,data,course_snapshot,recording_hash)
            values($1,$2,$3::date,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) returning id::text`, user, input.Name, raceNullable(input.Date), input.Status, input.Discipline, input.Kind, input.DistanceM, raceNullable(input.GroupID), raceNullable(input.ActivityID), raceNullable(input.PlannedActivityID), raceNullable(input.CourseID), data, snapshotJSON, hash).Scan(&id)
	} else {
		_, err = tx.Exec(ctx, `update races set name=$3,race_date=$4::date,status=$5,discipline=$6,kind=$7,distance_m=$8,group_id=$9,activity_id=$10,planned_activity_id=$11,course_id=$12,data=$13,course_snapshot=$14,recording_hash=$15,revision=revision+1,updated_at=now() where id=$1 and user_id=$2`, id, user, input.Name, raceNullable(input.Date), input.Status, input.Discipline, input.Kind, input.DistanceM, raceNullable(input.GroupID), raceNullable(input.ActivityID), raceNullable(input.PlannedActivityID), raceNullable(input.CourseID), data, snapshotJSON, hash)
	}
	if err != nil {
		return Race{}, raceStoreError(err)
	}
	if current.Date != input.Date || current.StartTime != input.StartTime || current.Timezone != input.Timezone || !raceSameDistance(current.DistanceM, input.DistanceM) {
		if _, err = tx.Exec(ctx, `delete from race_prediction_snapshots where race_id=$1 and user_id=$2`, id, user); err != nil {
			return Race{}, err
		}
	}
	if input.ActivityID != "" {
		_, err = tx.Exec(ctx, `insert into race_discovery(user_id,source,source_id,activity_id,state) select user_id,source,source_id,id,'confirmed' from activities where id=$1 and user_id=$2 on conflict(user_id,source,source_id) do update set state='confirmed',activity_id=excluded.activity_id`, input.ActivityID, user)
		if err != nil {
			return Race{}, err
		}
	}
	result, err := scanRace(tx.QueryRow(ctx, raceSelect+` where id=$1 and user_id=$2`, id, user))
	if err != nil {
		return Race{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Race{}, raceStoreError(err)
	}
	return result, nil
}
func raceSameDistance(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func raceRunningSport(sport string) bool { return sport == "Run" || sport == "Treadmill Run" }

func (s *Store) DeleteRace(ctx context.Context, id string, revision int) error {
	tag, err := s.db.Exec(ctx, `delete from races where id=$1 and user_id=$2 and revision=$3`, id, scopedUserID(ctx), revision)
	if err == nil && tag.RowsAffected() == 0 {
		_, err = s.GetRace(ctx, id)
		if err == nil {
			return ErrRaceConflict
		}
	}
	return err
}
func (s *Store) RaceSettings(ctx context.Context) (RaceSettings, error) {
	result := RaceSettings{}
	err := s.db.QueryRow(ctx, `select revision,coalesce(birth_date::text,''),grading_table,coalesce(vdot_reference_id::text,''),predictions_enabled,weather_enabled,prediction_synced_at,prediction_attempted_at,prediction_backfilled,prediction_error from race_settings where user_id=$1`, scopedUserID(ctx)).Scan(&result.Revision, &result.BirthDate, &result.GradingTable, &result.VDOTReferenceID, &result.PredictionsEnabled, &result.WeatherEnabled, &result.PredictionSyncedAt, &result.PredictionAttemptedAt, &result.PredictionBackfilled, &result.PredictionError)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return result, err
}
func (s *Store) SaveRaceSettings(ctx context.Context, input RaceSettings) (RaceSettings, error) {
	if !validRaceDate(input.BirthDate) || input.BirthDate > time.Now().UTC().Format("2006-01-02") || input.GradingTable != "" && input.GradingTable != "M" && input.GradingTable != "F" {
		return RaceSettings{}, ErrRaceInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RaceSettings{}, err
	}
	defer tx.Rollback(ctx)
	user := scopedUserID(ctx)
	if err = raceLinkOwner(ctx, tx, "races", input.VDOTReferenceID, user); err != nil {
		return RaceSettings{}, err
	}
	var revision int
	err = tx.QueryRow(ctx, `insert into race_settings(user_id,birth_date,grading_table,vdot_reference_id,predictions_enabled,weather_enabled)
        values($1,$2::date,$3,$4,$5,$6) on conflict(user_id) do update set birth_date=excluded.birth_date,grading_table=excluded.grading_table,vdot_reference_id=excluded.vdot_reference_id,predictions_enabled=excluded.predictions_enabled,weather_enabled=excluded.weather_enabled,revision=race_settings.revision+1 where race_settings.revision=$7 returning revision`, user, raceNullable(input.BirthDate), input.GradingTable, raceNullable(input.VDOTReferenceID), input.PredictionsEnabled, input.WeatherEnabled, input.Revision).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return RaceSettings{}, ErrRaceConflict
	}
	if err != nil {
		return RaceSettings{}, raceStoreError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return RaceSettings{}, err
	}
	return s.RaceSettings(ctx)
}

func raceResourceTable(kind string) string {
	if kind == "templates" {
		return "race_checklist_templates"
	}
	return "race_groups"
}
func (s *Store) RaceResources(ctx context.Context, kind string) ([]RaceResource, error) {
	items := "'[]'::jsonb"
	if kind == "templates" {
		items = "items"
	}
	rows, err := s.db.Query(ctx, `select id::text,name,revision,`+items+` from `+raceResourceTable(kind)+` where user_id=$1 order by lower(name),id`, scopedUserID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RaceResource{}
	for rows.Next() {
		var item RaceResource
		var b []byte
		if err = rows.Scan(&item.ID, &item.Name, &item.Revision, &b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &item.Items); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
func (s *Store) SaveRaceResource(ctx context.Context, kind, id string, input RaceResource) (RaceResource, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 160 {
		return input, ErrRaceInvalid
	}
	if err := validateRaceChecklist(input.Items); err != nil {
		return input, err
	}
	for i := range input.Items {
		input.Items[i].Done = false
	}
	if input.Items == nil {
		input.Items = []RaceChecklistItem{}
	}
	b, err := json.Marshal(input.Items)
	if err != nil {
		return input, err
	}
	table := raceResourceTable(kind)
	args := []any{scopedUserID(ctx), input.Name}
	if id == "" {
		columns, values := "user_id,name", "$1,$2"
		if kind == "templates" {
			columns += ",items"
			values += ",$3"
			args = append(args, b)
		}
		err = s.db.QueryRow(ctx, `insert into `+table+`(`+columns+`) values(`+values+`) returning id::text,revision`, args...).Scan(&input.ID, &input.Revision)
	} else {
		args = append(args, id, input.Revision)
		extra := ""
		if kind == "templates" {
			extra = ",items=$5"
			args = append(args, b)
		}
		err = s.db.QueryRow(ctx, `update `+table+` set name=$2,revision=revision+1`+extra+` where user_id=$1 and id=$3 and revision=$4 returning id::text,revision`, args...).Scan(&input.ID, &input.Revision)
		if errors.Is(err, pgx.ErrNoRows) {
			return input, ErrRaceConflict
		}
	}
	return input, raceStoreError(err)
}
func (s *Store) DeleteRaceResource(ctx context.Context, kind, id string, revision int) error {
	tag, err := s.db.Exec(ctx, `delete from `+raceResourceTable(kind)+` where user_id=$1 and id=$2 and revision=$3`, scopedUserID(ctx), id, revision)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrRaceConflict
	}
	return err
}

func (s *Store) RaceSummaries(ctx context.Context, from, to string) ([]RaceSummary, error) {
	rows, err := s.db.Query(ctx, `select id::text,name,race_date::text,status,coalesce(activity_id::text,''),coalesce(planned_activity_id::text,''),coalesce(data->>'priority',''),coalesce(data->>'timezone','UTC'),coalesce(data->>'startTime',''),distance_m from races where user_id=$1 and race_date between $2::date and $3::date order by race_date,id`, scopedUserID(ctx), from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RaceSummary{}
	for rows.Next() {
		var r RaceSummary
		if err = rows.Scan(&r.ID, &r.Name, &r.Date, &r.Status, &r.ActivityID, &r.PlannedActivityID, &r.Priority, &r.Timezone, &r.StartTime, &r.DistanceM); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
func (s *Store) attachRaceSummaries(ctx context.Context, activities []Activity) error {
	if len(activities) == 0 {
		return nil
	}
	ids := make([]string, len(activities))
	for i := range activities {
		ids[i] = activities[i].ID
	}
	rows, err := s.db.Query(ctx, `select id::text,name,coalesce(race_date::text,''),status,activity_id::text from races where user_id=$1 and activity_id=any($2::uuid[])`, scopedUserID(ctx), ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := map[string]*RaceSummary{}
	for rows.Next() {
		var r RaceSummary
		if err = rows.Scan(&r.ID, &r.Name, &r.Date, &r.Status, &r.ActivityID); err != nil {
			return err
		}
		byID[r.ActivityID] = &r
	}
	for i := range activities {
		activities[i].Race = byID[activities[i].ID]
	}
	return rows.Err()
}

func saveRaceImportTx(ctx context.Context, tx pgx.Tx, id string, a ImportedActivity) error {
	hash := raceRecordingHash(a.StartTime, a.DistanceM, a.ElapsedTimeS, a.Samples)
	_, err := tx.Exec(ctx, `update races set data=jsonb_set(data,'{raceOnly}','false'::jsonb),recording_hash='',revision=revision+1,updated_at=now() where activity_id=$1 and user_id=$2 and recording_hash<>'' and recording_hash<>$3`, id, scopedUserID(ctx), hash)
	if err != nil {
		return err
	}
	if !raceRunningSport(a.SportType) {
		return nil
	}
	_, err = tx.Exec(ctx, `insert into race_discovery_pending(activity_id,user_id) values($1,$2) on conflict(activity_id) do update set queued_at=now()`, id, scopedUserID(ctx))
	return err
}

// The activity row is share-locked by SaveRace; read the full original series in
// the same transaction so a one-connection pool is sufficient.
func raceRecordingTx(ctx context.Context, tx pgx.Tx, id string) (Activity, error) {
	var a Activity
	if err := tx.QueryRow(ctx, `select start_time,coalesce(distance_m,0),coalesce(elapsed_time_s,0) from activities where id=$1 and user_id=$2`, id, scopedUserID(ctx)).Scan(&a.StartTime, &a.DistanceM, &a.ElapsedTimeS); err != nil {
		return a, err
	}
	rows, err := tx.Query(ctx, `select sample_index,timestamp,elapsed_s,distance_m from activity_samples where activity_id=$1 order by sample_index`, id)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var p ActivitySample
		if err := rows.Scan(&p.Index, &p.Timestamp, &p.ElapsedS, &p.DistanceM); err != nil {
			return a, err
		}
		a.Samples = append(a.Samples, p)
	}
	return a, rows.Err()
}
