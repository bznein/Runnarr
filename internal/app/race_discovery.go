package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type raceActivitySignal struct {
	ID           string    `json:"id"`
	Source       string    `json:"source"`
	SourceID     string    `json:"sourceId"`
	Name         string    `json:"name"`
	Sport        string    `json:"sport"`
	StartTime    time.Time `json:"startTime"`
	DistanceM    float64   `json:"distanceM"`
	MovingTimeS  int       `json:"movingTimeS"`
	ElapsedTimeS int       `json:"elapsedTimeS"`
	ExplicitRace bool      `json:"-"`
	HasRecovery  bool      `json:"-"`
	LinkedRaceID string    `json:"linkedRaceId,omitempty"`
	Reasons      []string  `json:"reasons"`
	Confidence   string    `json:"confidence,omitempty"`
}

var raceNamePattern = regexp.MustCompile(`(?i)\b(race|parkrun|marathon|half[ -]?marathon|ultra|time[ -]?trial|championships?)\b`)
var raceTrainingPattern = regexp.MustCompile(`(?i)\b(training|recovery|tempo|intervals?|warm[ -]?up|cool[ -]?down|easy|taper|race[ -]?pace|marathon[ -]?pace|race[ -]?prep)\b`)
var raceStandardDistances = []float64{1500, 1609.344, 3000, 5000, 8046.72, 10000, 16093.44, 21097.5, 42195, 50000, 80467.2, 100000, 160934.4}

func raceDiscoveryReasons(a raceActivitySignal, comparisonPaces []float64) ([]string, string) {
	if !raceRunningSport(a.Sport) {
		return nil, ""
	}
	if a.ExplicitRace {
		return []string{"Provider identifies this activity as a race"}, "strong"
	}
	if raceTrainingPattern.MatchString(a.Name) || a.HasRecovery {
		return nil, ""
	}
	if raceNamePattern.MatchString(a.Name) {
		return []string{"Activity name mentions a race, parkrun, or time trial"}, "strong"
	}
	if a.DistanceM <= 0 || a.MovingTimeS <= 0 || a.ElapsedTimeS <= 0 || float64(a.ElapsedTimeS)/float64(a.MovingTimeS) > 1.03 || a.ElapsedTimeS < a.MovingTimeS || len(comparisonPaces) < 5 {
		return nil, ""
	}
	near := false
	for _, d := range raceStandardDistances {
		if math.Abs(a.DistanceM-d)/d <= 0.03 {
			near = true
			break
		}
	}
	if !near {
		return nil, ""
	}
	faster := 0
	pace := float64(a.ElapsedTimeS) / a.DistanceM
	for _, p := range comparisonPaces {
		if p < pace {
			faster++
		}
	}
	if float64(faster)/float64(len(comparisonPaces)) > 0.1 {
		return nil, ""
	}
	return []string{"Within 3% of a standard race distance", "Among the fastest 10% of comparable runs in the preceding year", "Stops account for at most 3% of moving time"}, "possible"
}

const raceSignalSelect = `select a.id::text,a.source,a.source_id,coalesce(nullif(a.local_name,''),a.name),a.sport_type,a.start_time,coalesce(a.distance_m,0),coalesce(a.moving_time_s,0),coalesce(a.elapsed_time_s,0),
    lower(coalesce(a.raw#>>'{garmin_workout,activity,eventType,typeKey}',''))='race',
    exists(select 1 from activity_intervals i where i.activity_id=a.id and lower(i.category) in ('recovery','recover')),
    coalesce((select r.id::text from races r where r.activity_id=a.id and r.user_id=a.user_id),'') from activities a`

func scanRaceSignal(row interface{ Scan(...any) error }) (raceActivitySignal, error) {
	var a raceActivitySignal
	err := row.Scan(&a.ID, &a.Source, &a.SourceID, &a.Name, &a.Sport, &a.StartTime, &a.DistanceM, &a.MovingTimeS, &a.ElapsedTimeS, &a.ExplicitRace, &a.HasRecovery, &a.LinkedRaceID)
	return a, err
}

func (s *Store) discoverRaceActivity(ctx context.Context, id string) error {
	user := scopedUserID(ctx)
	a, err := scanRaceSignal(s.db.QueryRow(ctx, raceSignalSelect+` where a.id=$1 and a.user_id=$2 and a.source<>'training_sheet'`, id, user))
	if err != nil {
		return err
	}
	if a.LinkedRaceID != "" {
		return nil
	}
	paces := []float64{}
	if raceRunningSport(a.Sport) && !a.ExplicitRace && !raceNamePattern.MatchString(a.Name) && !raceTrainingPattern.MatchString(a.Name) && !a.HasRecovery && a.DistanceM > 0 {
		rows, err := s.db.Query(ctx, `select elapsed_time_s/distance_m from activities where user_id=$1 and source<>'training_sheet' and sport_type in ('Run','Treadmill Run') and start_time >= $2 and start_time < $3 and distance_m between $4 and $5 and elapsed_time_s>0`, user, a.StartTime.AddDate(-1, 0, 0), a.StartTime, a.DistanceM*.9, a.DistanceM*1.1)
		if err != nil {
			return err
		}
		for rows.Next() {
			var p float64
			if err = rows.Scan(&p); err != nil {
				rows.Close()
				return err
			}
			paces = append(paces, p)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
	}
	reasons, confidence := raceDiscoveryReasons(a, paces)
	if len(reasons) == 0 {
		_, err = s.db.Exec(ctx, `delete from race_discovery where user_id=$1 and source=$2 and source_id=$3 and state='pending'`, user, a.Source, a.SourceID)
		return err
	}
	raw, err := json.Marshal(reasons)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `insert into race_discovery(user_id,source,source_id,activity_id,reasons,confidence) values($1,$2,$3,$4,$5,$6) on conflict(user_id,source,source_id) do update set activity_id=excluded.activity_id,reasons=excluded.reasons,confidence=excluded.confidence where race_discovery.state='pending'`, user, a.Source, a.SourceID, a.ID, raw, confidence)
	return err
}
func (s *Store) RaceDiscovery(ctx context.Context, offset int) ([]raceActivitySignal, bool, error) {
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(ctx, raceSignalSelect+` join race_discovery d on d.activity_id=a.id and d.user_id=a.user_id where a.user_id=$1 and d.state='pending' and not exists(select 1 from races r where r.activity_id=a.id) order by a.start_time desc,a.id limit 51 offset $2`, scopedUserID(ctx), offset)
	if err != nil {
		return nil, false, err
	}
	result := []raceActivitySignal{}
	for rows.Next() {
		a, err := scanRaceSignal(rows)
		if err != nil {
			rows.Close()
			return nil, false, err
		}
		result = append(result, a)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(result) > 50
	if more {
		result = result[:50]
	}
	for i := range result {
		var b []byte
		if err = s.db.QueryRow(ctx, `select reasons,confidence from race_discovery where user_id=$1 and source=$2 and source_id=$3`, scopedUserID(ctx), result[i].Source, result[i].SourceID).Scan(&b, &result[i].Confidence); err != nil {
			return nil, false, err
		}
		if err = json.Unmarshal(b, &result[i].Reasons); err != nil {
			return nil, false, err
		}
	}
	return result, more, nil
}
func (s *Store) DismissRaceDiscovery(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `update race_discovery set state='dismissed' where user_id=$1 and activity_id=$2 and state='pending'`, scopedUserID(ctx), id)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}
func (s *Store) RaceActivityCandidates(ctx context.Context, r Race, query string) ([]raceActivitySignal, error) {
	args := []any{scopedUserID(ctx)}
	where := ` where a.user_id=$1 and a.source<>'training_sheet' and a.sport_type in ('Run','Treadmill Run') and not exists(select 1 from races r where r.activity_id=a.id)`
	if strings.TrimSpace(query) != "" {
		args = append(args, query)
		where += ` and coalesce(nullif(a.local_name,''),a.name) ilike '%'||$2||'%'`
	} else {
		start, err := raceCutoff(r)
		if err != nil {
			return []raceActivitySignal{}, nil
		}
		zone := start.Location()
		day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, zone)
		args = append(args, day.AddDate(0, 0, -1), day.AddDate(0, 0, 2))
		where += ` and a.start_time >= $2 and a.start_time < $3`
	}
	rows, err := s.db.Query(ctx, raceSignalSelect+where+` order by a.start_time desc,a.id limit 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []raceActivitySignal{}
	zone, _ := time.LoadLocation(r.Timezone)
	if zone == nil {
		zone = time.UTC
	}
	for rows.Next() {
		a, err := scanRaceSignal(rows)
		if err != nil {
			return nil, err
		}
		a.Reasons = []string{}
		if a.StartTime.In(zone).Format("2006-01-02") == r.Date {
			a.Reasons = append(a.Reasons, "Same event-local date")
		}
		if r.DistanceM != nil && math.Abs(a.DistanceM-*r.DistanceM) / *r.DistanceM <= .05 {
			a.Reasons = append(a.Reasons, "Distance within 5%")
		}
		if strings.Contains(strings.ToLower(a.Name), strings.ToLower(r.Name)) {
			a.Reasons = append(a.Reasons, "Event name matches")
		}
		result = append(result, a)
	}
	sort.SliceStable(result, func(i, j int) bool { return len(result[i].Reasons) > len(result[j].Reasons) })
	return result, rows.Err()
}
func (s *Server) runRaceScanJob(jobID string) {
	lookup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	user, err := s.store.SyncJobUserID(lookup, jobID)
	cancel()
	if err != nil {
		s.logger.Error("load race scan owner", "error", err)
		return
	}
	ctx, cleanup := s.cancellableSyncJobContext(context.Background(), user, jobID, 2*time.Hour)
	defer cleanup()
	cursor := "00000000-0000-0000-0000-000000000000"
	processed := 0
	began := time.Now().UTC()
	for ctx.Err() == nil {
		rows, queryErr := s.store.db.Query(ctx, `select id::text from activities where user_id=$1 and source<>'training_sheet' and sport_type in ('Run','Treadmill Run') and id>$2::uuid and created_at<=$3 order by id limit 100`, user, cursor, began)
		if queryErr != nil {
			err = queryErr
			break
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if queryErr = rows.Scan(&id); queryErr != nil {
				break
			}
			ids = append(ids, id)
		}
		rows.Close()
		if queryErr == nil {
			queryErr = rows.Err()
		}
		if queryErr != nil {
			err = queryErr
			break
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			if err = s.store.discoverRaceActivity(ctx, id); err != nil && err != pgx.ErrNoRows {
				break
			}
			err = nil
			processed++
			cursor = id
		}
		if err != nil {
			break
		}
		if err = s.store.UpdateSyncJobProgress(ctx, jobID, map[string]any{"processed": processed, "stage": "Reviewing local running history"}); err != nil {
			break
		}
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	status, message := "completed", ""
	if err != nil {
		status, message = "failed", err.Error()
	}
	if err = s.finishSyncJob(ctx, jobID, status, message, map[string]any{"processed": processed}); err != nil {
		s.logger.Error("finish race scan", "error", err)
	}
}
func (s *Server) processRaceImports(ctx context.Context) error {
	rows, err := s.store.db.Query(ctx, `select p.activity_id::text,p.user_id::text,p.queued_at from race_discovery_pending p join users u on u.id=p.user_id where not u.disabled order by p.queued_at limit 50`)
	if err != nil {
		return err
	}
	type pending struct {
		id, user string
		at       time.Time
	}
	items := []pending{}
	for rows.Next() {
		var p pending
		if err = rows.Scan(&p.id, &p.user, &p.at); err != nil {
			rows.Close()
			return err
		}
		items = append(items, p)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, p := range items {
		userCtx := withUserID(ctx, p.user)
		err = s.store.discoverRaceActivity(userCtx, p.id)
		if err != nil && err != pgx.ErrNoRows {
			return fmt.Errorf("discover imported race: %w", err)
		}
		if _, err = s.store.db.Exec(ctx, `delete from race_discovery_pending where activity_id=$1 and user_id=$2 and queued_at=$3`, p.id, p.user, p.at); err != nil {
			return err
		}
	}
	return nil
}
