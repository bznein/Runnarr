package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

//go:embed data/race-age-grades/tables.json
var raceAgeGradeJSON []byte

type raceAgeGradeTables struct {
	Version      string `json:"version"`
	SourceCommit string `json:"sourceCommit"`
	Distances    []struct {
		DistanceM float64                       `json:"distanceM"`
		Standards map[string]float64            `json:"standards"`
		Factors   map[string]map[string]float64 `json:"factors"`
	} `json:"distances"`
}

var raceAgeTables = func() raceAgeGradeTables {
	var tables raceAgeGradeTables
	if err := json.Unmarshal(raceAgeGradeJSON, &tables); err != nil {
		panic(err)
	}
	return tables
}()

type RaceMetrics struct {
	Eligible       bool                  `json:"eligible"`
	TimeMS         *int64                `json:"timeMs,omitempty"`
	Basis          string                `json:"basis,omitempty"`
	VDOT           *float64              `json:"vdot,omitempty"`
	VDOTReason     string                `json:"vdotReason,omitempty"`
	AgeGrade       *float64              `json:"ageGrade,omitempty"`
	AgeGradeReason string                `json:"ageGradeReason,omitempty"`
	Age            *int                  `json:"age,omitempty"`
	Table          string                `json:"table,omitempty"`
	TableVersion   string                `json:"tableVersion"`
	Equivalents    []toolsVDOTEquivalent `json:"equivalents,omitempty"`
}

func raceMetrics(r Race, settings RaceSettings) RaceMetrics {
	result := RaceMetrics{TableVersion: raceAgeTables.Version}
	if r.Result.Confirmed {
		result.TimeMS = raceResultTime(r.Result)
		result.Basis = raceResultBasis(r.Result)
	}
	if r.Status != "finished" || r.Date == "" || !r.Result.Confirmed || r.DistanceM == nil || result.TimeMS == nil || r.Result.Excluded {
		reason := "A confirmed finished result with date, distance and time is required"
		if r.Result.Excluded {
			reason = "Excluded from performance comparisons"
		}
		result.VDOTReason, result.AgeGradeReason = reason, reason
		return result
	}
	result.Eligible = true
	if (r.Discipline == "road" || r.Discipline == "track") && *r.DistanceM >= 1500 && *r.DistanceM <= 42195 {
		if v, err := toolsVDOTFromDistanceAndTime(*r.DistanceM, float64(*result.TimeMS)/1000); err == nil {
			result.VDOT = &v
			result.Equivalents, _ = toolsVDOTEquivalents(v)
		} else {
			result.VDOTReason = "Result is outside the VDOT calculation range"
		}
	} else {
		result.VDOTReason = "VDOT supports road/track results from 1500 m through marathon"
	}
	result.Table = settings.GradingTable
	if r.TableOverride != "" {
		result.Table = r.TableOverride
	}
	if r.AgeOverride != nil {
		v := *r.AgeOverride
		result.Age = &v
	} else if settings.BirthDate != "" && r.Date != "" {
		birth, be := time.Parse("2006-01-02", settings.BirthDate)
		date, de := time.Parse("2006-01-02", r.Date)
		if be == nil && de == nil && !date.Before(birth) {
			age := date.Year() - birth.Year()
			if date.Month() < birth.Month() || date.Month() == birth.Month() && date.Day() < birth.Day() {
				age--
			}
			result.Age = &age
		}
	}
	switch {
	case r.Discipline != "road":
		result.AgeGradeReason = "The bundled age-grade tables apply to road running"
	case result.Age == nil || result.Table == "":
		result.AgeGradeReason = "Enter birth date/reference table or per-race overrides"
	default:
		for _, table := range raceAgeTables.Distances {
			if math.Abs(table.DistanceM-*r.DistanceM) > 0.000001 {
				continue
			}
			factor := table.Factors[result.Table][strconv.Itoa(*result.Age)]
			standard := table.Standards[result.Table]
			if factor > 0 && standard > 0 {
				grade := 100 * standard / factor / (float64(*result.TimeMS) / 1000)
				result.AgeGrade = &grade
			}
			break
		}
		if result.AgeGrade == nil {
			result.AgeGradeReason = "Distance or age is not listed in the bundled road table"
		}
	}
	return result
}

type RaceHalfway struct {
	Source           string `json:"source,omitempty"`
	Basis            string `json:"basis,omitempty"`
	FirstHalfMS      *int64 `json:"firstHalfMs,omitempty"`
	SecondHalfMS     *int64 `json:"secondHalfMs,omitempty"`
	DifferenceMS     *int64 `json:"differenceMs,omitempty"`
	WatchFirstHalfMS *int64 `json:"watchFirstHalfMs,omitempty"`
	Scaled           bool   `json:"scaled"`
	Reason           string `json:"reason,omitempty"`
}

func raceHalfway(r Race, a *Activity) RaceHalfway {
	finish := func(first int64, total *int64, source, basis string, scaled bool) RaceHalfway {
		result := RaceHalfway{Source: source, Basis: basis, FirstHalfMS: &first, Scaled: scaled}
		if total != nil {
			second := *total - first
			delta := second - first
			result.SecondHalfMS = &second
			result.DifferenceMS = &delta
		}
		return result
	}
	if r.DistanceM == nil {
		return RaceHalfway{Reason: "Enter the race distance"}
	}
	for _, p := range r.Result.Checkpoints {
		if math.Abs(p.DistanceM-*r.DistanceM/2) < 0.000001 {
			return finish(p.TimeMS, raceBasisTime(r.Result, r.Result.SplitBasis), "official", r.Result.SplitBasis, false)
		}
	}
	unavailable := func(reason string) RaceHalfway { return RaceHalfway{Reason: reason} }
	if a == nil {
		return unavailable("Link an activity or enter an official halfway checkpoint")
	}
	if !r.RaceOnly {
		return unavailable("Confirm that the complete recording contains only the race")
	}
	if !isPositiveFloat(a.DistanceM) || a.ElapsedTimeS <= 0 || len(a.Samples) < 2 {
		return unavailable("Recording lacks usable distance/time samples")
	}
	if r.RecordingHash != "" && r.RecordingHash != raceRecordingHash(a.StartTime, a.DistanceM, a.ElapsedTimeS, a.Samples) {
		return unavailable("Recording changed; confirm the race-only recording again")
	}
	type point struct{ d, t float64 }
	points := make([]point, 0, len(a.Samples))
	lastD, lastT := -1.0, -1.0
	for _, s := range a.Samples {
		if s.DistanceM == nil {
			continue
		}
		d := *s.DistanceM
		var elapsed float64
		if s.Timestamp != nil {
			elapsed = s.Timestamp.Sub(a.StartTime).Seconds()
		} else if s.ElapsedS != nil {
			elapsed = float64(*s.ElapsedS)
		} else {
			continue
		}
		if math.IsNaN(d) || math.IsInf(d, 0) || d < 0 || elapsed < 0 || d < lastD || (elapsed < lastT || elapsed == lastT && d > lastD) {
			return unavailable("Recording has distance resets or invalid timestamps")
		}
		if elapsed > float64(a.ElapsedTimeS)+1 || d > a.DistanceM*1.01 {
			return unavailable("Sample boundaries disagree with the recording totals")
		}
		points = append(points, point{d, elapsed})
		lastD, lastT = d, elapsed
	}
	if len(points) < 2 || points[0].d > 250 || points[0].t > 60 || a.DistanceM-points[len(points)-1].d > 250 || float64(a.ElapsedTimeS)-points[len(points)-1].t > 60 {
		return unavailable("Recording is missing usable start or finish boundaries")
	}
	target := a.DistanceM / 2
	var crossing *float64
	for i, p := range points {
		if p.d == target {
			value := p.t
			crossing = &value
			break
		}
		if i == 0 || p.d < target {
			continue
		}
		previous := points[i-1]
		if previous.d > target || p.d-previous.d > 250 || p.t-previous.t > 60 || p.d <= previous.d {
			return unavailable("Samples around halfway are too sparse for a reliable estimate")
		}
		value := previous.t + (p.t-previous.t)*(target-previous.d)/(p.d-previous.d)
		crossing = &value
		break
	}
	if crossing == nil {
		return unavailable("Recording does not bracket halfway")
	}
	watchFirst := int64(math.Round(*crossing * 1000))
	watchTotal := int64(a.ElapsedTimeS) * 1000
	total, basis := &watchTotal, "watch elapsed"
	scaled := false
	first := watchFirst
	// Gun time includes the wait before crossing the start and is not a compatible race-only clock.
	if r.Result.Confirmed {
		compatible := r.Result.ChipTimeMS
		if compatible == nil && r.Result.GunTimeMS == nil {
			compatible = r.Result.ManualTimeMS
		}
		if compatible != nil {
			total = compatible
			basis = raceResultBasis(r.Result)
			first = int64(math.Round(float64(watchFirst) * float64(*total) / float64(watchTotal)))
			scaled = true
		}
	}
	if first <= 0 || first >= *total {
		return unavailable("Estimated halfway falls outside the finish time")
	}
	result := finish(first, total, "estimated", basis, scaled)
	result.WatchFirstHalfMS = &watchFirst
	return result
}

type RacePerformancePoint struct {
	Race
	Metrics      RaceMetrics `json:"metrics"`
	PersonalBest bool        `json:"personalBest"`
	SeasonBest   bool        `json:"seasonBest"`
}
type RacePerformance struct {
	Results         []RacePerformancePoint `json:"results"`
	Reference       *RacePerformancePoint  `json:"reference,omitempty"`
	ReferenceReason string                 `json:"referenceReason,omitempty"`
	Predictions     []RacePrediction       `json:"predictions"`
	TableVersion    string                 `json:"tableVersion"`
}

func buildRacePerformance(races []Race, settings RaceSettings, now time.Time) RacePerformance {
	result := RacePerformance{Results: []RacePerformancePoint{}, Predictions: []RacePrediction{}, TableVersion: raceAgeTables.Version}
	best, season := map[string]int64{}, map[string]int64{}
	recent := now.AddDate(0, 0, -90).Format("2006-01-02")
	today := now.Format("2006-01-02")
	for _, r := range races {
		m := raceMetrics(r, settings)
		p := RacePerformancePoint{Race: r, Metrics: m}
		result.Results = append(result.Results, p)
		if !m.Eligible || r.Date > today || r.Date == "" {
			continue
		}
		key := fmt.Sprintf("%s:%.6f", r.Discipline, *r.DistanceM)
		yearKey := key + ":" + r.Date[:4]
		if v, ok := best[key]; !ok || *m.TimeMS < v {
			best[key] = *m.TimeMS
		}
		if v, ok := season[yearKey]; !ok || *m.TimeMS < v {
			season[yearKey] = *m.TimeMS
		}
		if m.VDOT == nil {
			continue
		}
		if settings.VDOTReferenceID != "" {
			if r.ID == settings.VDOTReferenceID {
				copy := p
				result.Reference = &copy
			}
		} else if r.Date >= recent && (result.Reference == nil || *m.VDOT > *result.Reference.Metrics.VDOT || *m.VDOT == *result.Reference.Metrics.VDOT && r.Date > result.Reference.Date) {
			copy := p
			result.Reference = &copy
		}
	}
	for i, p := range result.Results {
		if !p.Metrics.Eligible || p.Date > today || p.Date == "" {
			continue
		}
		key := fmt.Sprintf("%s:%.6f", p.Discipline, *p.DistanceM)
		result.Results[i].PersonalBest = *p.Metrics.TimeMS == best[key]
		result.Results[i].SeasonBest = *p.Metrics.TimeMS == season[key+":"+p.Date[:4]]
	}
	if result.Reference == nil {
		result.ReferenceReason = "No eligible result in the last 90 days; choose a reference result"
		if settings.VDOTReferenceID != "" {
			result.ReferenceReason = "The selected reference result is no longer eligible"
		}
	}
	return result
}
func (s *Store) AllRaces(ctx context.Context, o raceListOptions) ([]Race, error) {
	result := []Race{}
	o.Limit = 100
	o.Offset = 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := s.ListRaces(ctx, o)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Races...)
		if !page.HasMore {
			break
		}
		o.Offset = page.NextOffset
	}
	return result, nil
}

type RaceBuildWeek struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	DistanceM  float64 `json:"distanceM"`
	DurationS  int     `json:"durationS"`
	ElevationM float64 `json:"elevationM"`
	Count      int     `json:"count"`
	LongestM   float64 `json:"longestM"`
}

func (s *Store) RaceBuildUp(ctx context.Context, r Race) ([]RaceBuildWeek, error) {
	result := []RaceBuildWeek{}
	cutoff, err := raceCutoff(r)
	if err != nil {
		return result, nil
	}
	zone := cutoff.Location()
	end := time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, zone)
	start := end.AddDate(0, 0, -84)
	for i := 0; i < 12; i++ {
		from := start.AddDate(0, 0, 7*i)
		to := from.AddDate(0, 0, 6)
		result = append(result, RaceBuildWeek{From: from.Format("2006-01-02"), To: to.Format("2006-01-02")})
	}
	rows, err := s.db.Query(ctx, `select start_time,coalesce(distance_m,0),coalesce(moving_time_s,0),coalesce(elevation_gain_m,0) from activities where user_id=$1 and source<>'training_sheet' and sport_type in ('Run','Treadmill Run') and start_time >= $2 and start_time < $3 order by start_time`, scopedUserID(ctx), start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var date time.Time
		var distance, gain float64
		var duration int
		if err = rows.Scan(&date, &distance, &duration, &gain); err != nil {
			return nil, err
		}
		local := date.In(zone).Format("2006-01-02")
		i := sort.Search(len(result), func(i int) bool { return result[i].To >= local })
		if i == len(result) {
			continue
		}
		w := &result[i]
		w.DistanceM += distance
		w.ElevationM += gain
		w.DurationS += duration
		w.Count++
		w.LongestM = max(w.LongestM, distance)
	}
	return result, rows.Err()
}
