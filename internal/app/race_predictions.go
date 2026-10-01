package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type RacePrediction struct {
	Date       string          `json:"date"`
	DistanceM  float64         `json:"distanceM"`
	TimeMS     *int64          `json:"timeMs,omitempty"`
	FetchedAt  time.Time       `json:"fetchedAt"`
	Backfilled bool            `json:"backfilled"`
	Raw        json.RawMessage `json:"-"`
}
type racePredictionBridge interface {
	FetchRacePredictions(context.Context, string, string, string) (json.RawMessage, error)
}

func (b PythonGarminBridge) FetchRacePredictions(ctx context.Context, tokenStore, from, to string) (json.RawMessage, error) {
	var response json.RawMessage
	err := b.run(ctx, map[string]any{"action": "race-predictions", "tokenStore": tokenStore, "from": from, "to": to}, &response)
	return response, err
}

func normalizeRacePredictions(raw json.RawMessage, from, to string, now time.Time) ([]RacePrediction, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	result := []RacePrediction{}
	seen := map[string]bool{}
	fields := []struct {
		keys     []string
		distance float64
	}{{[]string{"raceTime5K", "raceTime5k"}, 5000}, {[]string{"raceTime10K", "raceTime10k"}, 10000}, {[]string{"raceTimeHalf", "raceTimeHalfMarathon"}, 21097.5}, {[]string{"raceTimeMarathon"}, 42195}}
	var walk func(any, string, int)
	walk = func(value any, inherited string, depth int) {
		if depth > 12 {
			return
		}
		switch v := value.(type) {
		case []any:
			for _, item := range v {
				walk(item, inherited, depth+1)
			}
		case map[string]any:
			date := inherited
			for _, key := range []string{"calendarDate", "date"} {
				if text, ok := v[key].(string); ok && validRaceDate(text) && text != "" {
					date = text
					break
				}
			}
			recognized := false
			for _, field := range fields {
				for _, key := range field.keys {
					if _, ok := v[key]; ok {
						recognized = true
					}
				}
			}
			if recognized && date != "" && date >= from && date <= to {
				record, _ := json.Marshal(v)
				for _, field := range fields {
					key := fmt.Sprintf("%s/%g", date, field.distance)
					if seen[key] {
						continue
					}
					seen[key] = true
					p := RacePrediction{Date: date, DistanceM: field.distance, FetchedAt: now, Backfilled: date < now.UTC().Format("2006-01-02"), Raw: record}
					for _, name := range field.keys {
						if seconds, ok := v[name].(float64); ok && isPositiveFloat(seconds) && seconds < 365*86400 {
							ms := int64(math.Round(seconds * 1000))
							if ms > 0 {
								p.TimeMS = &ms
							}
							break
						}
					}
					result = append(result, p)
				}
				return
			}
			for key, item := range v {
				parent := date
				if validRaceDate(key) && key != "" {
					parent = key
				}
				walk(item, parent, depth+1)
			}
		}
	}
	walk(payload, "", 0)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Date == result[j].Date {
			return result[i].DistanceM < result[j].DistanceM
		}
		return result[i].Date < result[j].Date
	})
	return result, nil
}
func (s *Store) RacePredictions(ctx context.Context, from, to string) ([]RacePrediction, error) {
	rows, err := s.db.Query(ctx, `select prediction_date::text,distance_m,time_ms,fetched_at,backfilled from race_predictions where user_id=$1 and prediction_date between $2::date and $3::date order by prediction_date,distance_m`, scopedUserID(ctx), from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RacePrediction{}
	for rows.Next() {
		var p RacePrediction
		if err = rows.Scan(&p.Date, &p.DistanceM, &p.TimeMS, &p.FetchedAt, &p.Backfilled); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
func (s *Store) saveRacePredictions(ctx context.Context, raw json.RawMessage, from, to string, rows []RacePrediction, now time.Time) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	user := scopedUserID(ctx)
	if _, err = tx.Exec(ctx, `insert into race_prediction_fetches(user_id,date_from,date_to,fetched_at,raw) values($1,$2::date,$3::date,$4,$5)`, user, from, to, now, []byte(raw)); err != nil {
		return err
	}
	for _, p := range rows {
		_, err = tx.Exec(ctx, `insert into race_predictions(user_id,prediction_date,distance_m,time_ms,fetched_at,backfilled,raw) values($1,$2::date,$3,$4,$5,$6,$7)
            on conflict(user_id,prediction_date,distance_m) do update set time_ms=excluded.time_ms,fetched_at=excluded.fetched_at,backfilled=race_predictions.backfilled and excluded.backfilled,raw=excluded.raw where excluded.time_ms is not null or race_predictions.time_ms is null`, user, p.Date, p.DistanceM, p.TimeMS, p.FetchedAt, p.Backfilled, []byte(p.Raw))
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `update race_settings set prediction_synced_at=$2,prediction_backfilled=true,prediction_error='' where user_id=$1`, user, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Server) queueRacePredictionSync(ctx context.Context) (string, error) {
	settings, err := s.store.RaceSettings(ctx)
	if err != nil {
		return "", err
	}
	if !settings.PredictionsEnabled {
		return "", fmt.Errorf("%w: enable Garmin prediction sync first", ErrRaceInvalid)
	}
	if _, connected, err := s.garmin.Status(ctx); err != nil {
		return "", err
	} else if !connected {
		return "", fmt.Errorf("%w: Garmin is not connected", ErrRaceInvalid)
	}
	id, err := s.store.CreateSyncJob(ctx, garminProvider, "race_predictions")
	if err != nil {
		return "", err
	}
	go s.runRacePredictionSync(id)
	return id, nil
}
func (s *Server) runRacePredictionSync(jobID string) {
	lookup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	user, err := s.store.SyncJobUserID(lookup, jobID)
	cancel()
	if err != nil {
		s.logger.Error("load prediction sync owner", "error", err)
		return
	}
	ctx, cleanup := s.cancellableSyncJobContext(context.Background(), user, jobID, 15*time.Minute)
	defer cleanup()
	settings, err := s.store.RaceSettings(ctx)
	now := time.Now().UTC()
	days := 14
	if !settings.PredictionBackfilled {
		days = 365
	}
	from, to := now.AddDate(0, 0, -(days-1)).Format("2006-01-02"), now.Format("2006-01-02")
	saved := 0
	if err == nil && !settings.PredictionsEnabled {
		err = fmt.Errorf("prediction sync is disabled")
	}
	if err == nil {
		_, err = s.store.db.Exec(ctx, `update race_settings set prediction_attempted_at=$2 where user_id=$1`, user, now)
	}
	if err == nil {
		bridge, ok := s.garmin.bridge.(racePredictionBridge)
		if !ok {
			err = fmt.Errorf("Garmin bridge does not support race predictions")
		} else {
			err = s.store.UpdateSyncJobProgress(ctx, jobID, map[string]any{"stage": "Fetching Garmin race predictions", "from": from, "to": to})
			if err == nil {
				var raw json.RawMessage
				raw, err = bridge.FetchRacePredictions(ctx, s.garmin.tokenStore(ctx), from, to)
				if err == nil {
					var predictions []RacePrediction
					predictions, err = normalizeRacePredictions(raw, from, to, now)
					if err == nil {
						err = s.store.saveRacePredictions(ctx, raw, from, to, predictions, now)
						for _, p := range predictions {
							if p.TimeMS != nil {
								saved++
							}
						}
					}
				}
			}
		}
	}
	status, message := "completed", ""
	if err != nil {
		status, message = "failed", err.Error()
		persist, done := s.syncJobPersistenceContext(user)
		_, saveErr := s.store.db.Exec(persist, `update race_settings set prediction_error=$2 where user_id=$1`, user, message)
		done()
		if saveErr != nil {
			s.logger.Error("record prediction failure", "error", saveErr)
		}
	}
	if finishErr := s.finishSyncJob(ctx, jobID, status, message, map[string]any{"from": from, "to": to, "saved": saved}); finishErr != nil {
		s.logger.Error("finish prediction sync", "error", finishErr)
	}
}

type RacePredictionComparison struct {
	Source       string     `json:"source"`
	Date         string     `json:"date"`
	ReferenceID  string     `json:"referenceId,omitempty"`
	TimeMS       int64      `json:"timeMs"`
	DifferenceMS *int64     `json:"differenceMs,omitempty"`
	Backfilled   bool       `json:"backfilled"`
	CapturedAt   *time.Time `json:"capturedAt,omitempty"`
	FetchedAt    *time.Time `json:"fetchedAt,omitempty"`
	Version      string     `json:"version,omitempty"`
}

func latestRacePrediction(predictions []RacePrediction, r Race) *RacePrediction {
	if r.DistanceM == nil || r.Date == "" {
		return nil
	}
	var latest *RacePrediction
	for _, p := range predictions {
		if p.Date >= r.Date || p.TimeMS == nil || math.Abs(p.DistanceM-*r.DistanceM) > 0.000001 {
			continue
		}
		if latest == nil || p.Date > latest.Date {
			copy := p
			latest = &copy
		}
	}
	return latest
}
func (s *Store) captureRacePrediction(ctx context.Context, r Race, performance RacePerformance, predictions []RacePrediction, now time.Time) error {
	cutoff, err := raceCutoff(r)
	if err != nil || !now.Before(cutoff) || r.DistanceM == nil {
		return nil
	}
	comparisons := []RacePredictionComparison{}
	if p := latestRacePrediction(predictions, r); p != nil {
		comparisons = append(comparisons, RacePredictionComparison{Source: "Garmin", Date: p.Date, TimeMS: *p.TimeMS, Backfilled: !p.FetchedAt.Before(cutoff), CapturedAt: &now, FetchedAt: &p.FetchedAt})
	}
	if ref := performance.Reference; ref != nil && ref.Metrics.VDOT != nil && ref.Date < r.Date && (r.Discipline == "road" || r.Discipline == "track") && *r.DistanceM >= 1500 && *r.DistanceM <= 42195 {
		if seconds, err := toolsVDOTTimeForDistance(*r.DistanceM, *ref.Metrics.VDOT); err == nil {
			comparisons = append(comparisons, RacePredictionComparison{Source: "VDOT equivalent", Date: ref.Date, ReferenceID: ref.ID, TimeMS: int64(math.Round(seconds * 1000)), CapturedAt: &now, Version: "Daniels-Gilbert v1"})
		}
	}
	if len(comparisons) == 0 {
		return nil
	}
	data, err := json.Marshal(comparisons)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `insert into race_prediction_snapshots(race_id,user_id,cutoff,captured_at,data) select id,user_id,$3,$4,$5 from races where id=$1 and user_id=$2 and revision=$6
        on conflict(race_id) do update set cutoff=excluded.cutoff,captured_at=excluded.captured_at,data=excluded.data where race_prediction_snapshots.captured_at<excluded.captured_at`, r.ID, scopedUserID(ctx), cutoff, now, data, r.Revision)
	return err
}
func (s *Store) RacePredictionComparisons(ctx context.Context, r Race) ([]RacePredictionComparison, error) {
	result := []RacePredictionComparison{}
	cutoff, err := raceCutoff(r)
	if err != nil || r.DistanceM == nil {
		return result, nil
	}
	var b []byte
	err = s.db.QueryRow(ctx, `select data from race_prediction_snapshots where race_id=$1 and user_id=$2 and cutoff=$3 and captured_at<$3`, r.ID, scopedUserID(ctx), cutoff).Scan(&b)
	if err == nil {
		if err = json.Unmarshal(b, &result); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	hasGarmin := false
	for _, p := range result {
		if p.Source == "Garmin" {
			hasGarmin = true
		}
	}
	if !hasGarmin {
		var p RacePrediction
		err = s.db.QueryRow(ctx, `select prediction_date::text,distance_m,time_ms,fetched_at,backfilled from race_predictions where user_id=$1 and distance_m=$2 and prediction_date<$3::date and time_ms is not null order by prediction_date desc limit 1`, scopedUserID(ctx), *r.DistanceM, r.Date).Scan(&p.Date, &p.DistanceM, &p.TimeMS, &p.FetchedAt, &p.Backfilled)
		if err == nil {
			result = append(result, RacePredictionComparison{Source: "Garmin", Date: p.Date, TimeMS: *p.TimeMS, Backfilled: !p.FetchedAt.Before(cutoff), FetchedAt: &p.FetchedAt})
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	if r.Result.Confirmed && r.Status == "finished" {
		if actual := raceResultTime(r.Result); actual != nil {
			for i := range result {
				delta := *actual - result[i].TimeMS
				result[i].DifferenceMS = &delta
			}
		}
	}
	return result, nil
}

func (s *Server) runRaceMaintenance(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	lastEnrichment := time.Time{}
	for {
		if err := s.processRaceImports(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("race import review", "error", err)
		}
		if time.Since(lastEnrichment) >= 15*time.Minute {
			lastEnrichment = time.Now()
			if err := s.maintainRaceEnrichment(ctx); err != nil && ctx.Err() == nil {
				s.logger.Error("race enrichment", "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) maintainRaceEnrichment(ctx context.Context) error {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if u.Disabled {
			continue
		}
		userCtx := withUserID(ctx, u.ID)
		settings, err := s.store.RaceSettings(userCtx)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if settings.PredictionsEnabled && (settings.PredictionAttemptedAt == nil || now.Sub(*settings.PredictionAttemptedAt) >= 24*time.Hour) {
			if _, err = s.queueRacePredictionSync(userCtx); err != nil && !errors.Is(err, ErrSyncJobAlreadyRunning) && !errors.Is(err, ErrRaceInvalid) {
				s.logger.Error("schedule race predictions", "error", err)
			}
		}
		races, err := s.store.AllRaces(userCtx, raceListOptions{})
		if err != nil {
			return err
		}
		perf := buildRacePerformance(races, settings, now)
		predictions, err := s.store.RacePredictions(userCtx, now.AddDate(-1, 0, 0).Format("2006-01-02"), now.Format("2006-01-02"))
		if err != nil {
			return err
		}
		for _, r := range races {
			if !strings.Contains("|wishlist|planned|registered|postponed|", "|"+r.Status+"|") {
				continue
			}
			if err = s.store.captureRacePrediction(userCtx, r, perf, predictions, now); err != nil {
				return err
			}
			if settings.WeatherEnabled {
				if _, err = s.refreshRaceForecast(userCtx, r); err != nil && ctx.Err() == nil {
					s.logger.Warn("race forecast", "race_id", r.ID, "error", err)
				}
			}
		}
	}
	return nil
}
