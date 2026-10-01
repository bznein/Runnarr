package app

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func raceTestPtr[T any](v T) *T { return &v }
func raceTestFinished() Race {
	return Race{ID: "one", RaceInput: RaceInput{Name: "Test 10K", Date: "2026-09-20", Timezone: "Europe/Dublin", Status: "finished", Discipline: "road", Kind: "race", DistanceM: raceTestPtr(10000.0), Result: RaceResult{Confirmed: true, ChipTimeMS: raceTestPtr(int64(2400000))}}}
}
func raceTestRecording() Activity {
	a := Activity{StartTime: time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC), DistanceM: 10200, ElapsedTimeS: 2500, MovingTimeS: 2400}
	for i := 0; i <= 102; i++ {
		d := float64(i) * 100
		elapsed := int(math.Round(d / 10200 * 2500))
		ts := a.StartTime.Add(time.Duration(elapsed) * time.Second)
		a.Samples = append(a.Samples, ActivitySample{Index: i, DistanceM: &d, ElapsedS: raceTestPtr(elapsed - 1), Timestamp: &ts})
	}
	return a
}
func TestRaceValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*RaceInput)
	}{
		{"bad date", func(r *RaceInput) { r.Date = "2026-02-30" }},
		{"unknown date confirmed", func(r *RaceInput) { r.Date = "" }},
		{"invalid distance", func(r *RaceInput) { r.DistanceM = raceTestPtr(math.NaN()) }},
		{"invalid timezone", func(r *RaceInput) { r.Timezone = "not/a/timezone" }},
		{"DST gap", func(r *RaceInput) { r.Date = "2026-03-29"; r.StartTime = "01:30" }},
		{"zero result", func(r *RaceInput) { r.Result.ChipTimeMS = raceTestPtr(int64(0)) }},
		{"chip after gun", func(r *RaceInput) { r.Result.GunTimeMS = raceTestPtr(int64(2300000)) }},
		{"place outside field", func(r *RaceInput) { r.Result.OverallPlace = raceTestPtr(11); r.Result.OverallTotal = raceTestPtr(10) }},
		{"partial coordinates", func(r *RaceInput) { r.Latitude = raceTestPtr(50.0) }},
		{"unsafe link", func(r *RaceInput) { r.ResultsURL = "javascript:alert(1)" }},
		{"credential link", func(r *RaceInput) { r.ResultsURL = "https://user:pass@example.com" }},
		{"unlinked confirmation", func(r *RaceInput) { r.RaceOnly = true }},
		{"unordered splits", func(r *RaceInput) {
			r.Result.SplitBasis = "chip"
			r.Result.Checkpoints = []RaceCheckpoint{{"5k", 5000, 1200000}, {"4k", 4000, 1300000}}
		}},
		{"late split", func(r *RaceInput) {
			r.Result.SplitBasis = "chip"
			r.Result.Checkpoints = []RaceCheckpoint{{"half", 5000, 2500000}}
		}},
		{"unknown split basis", func(r *RaceInput) { r.Result.Checkpoints = []RaceCheckpoint{{"half", 5000, 1200000}} }},
		{"checklist conflicting dates", func(r *RaceInput) {
			r.Checklist = []RaceChecklistItem{{Label: "Travel", DueDate: "2026-09-19", DaysBefore: raceTestPtr(1)}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := raceTestFinished().RaceInput
			tc.change(&r)
			if validateRace(&r) == nil {
				t.Fatal("accepted invalid race")
			}
		})
	}
	r := RaceInput{Name: " Future ultra ", Status: "wishlist", Discipline: "trail", Kind: "race"}
	if err := validateRace(&r); err != nil {
		t.Fatal(err)
	}
	if r.Name != "Future ultra" || r.Timezone != "UTC" || r.Checklist == nil || r.Goals == nil || r.Result.Checkpoints == nil {
		t.Fatalf("missing defaults: %#v", r)
	}
}
func TestRaceCutoffUsesEventMidnightAndDST(t *testing.T) {
	r := raceTestFinished()
	r.Date = "2026-10-25"
	got, err := raceCutoff(r)
	if err != nil || got.UTC().Format(time.RFC3339) != "2026-10-24T23:00:00Z" {
		t.Fatalf("%v %v", got, err)
	}
	r.StartTime = "09:00"
	got, _ = raceCutoff(r)
	if got.UTC().Hour() != 9 {
		t.Fatal(got)
	}
}
func TestRaceHalfwayUsesFullElapsedAndCompatibleOfficialClock(t *testing.T) {
	r := raceTestFinished()
	r.RaceOnly = true
	a := raceTestRecording()
	h := raceHalfway(r, &a)
	if h.FirstHalfMS == nil || *h.FirstHalfMS != 1200000 || *h.SecondHalfMS != 1200000 || !h.Scaled || h.Source != "estimated" || *h.WatchFirstHalfMS != 1250000 {
		t.Fatalf("%#v", h)
	}
	r.Result.ChipTimeMS = nil
	r.Result.GunTimeMS = raceTestPtr(int64(2600000))
	h = raceHalfway(r, &a)
	if h.Scaled || *h.FirstHalfMS != 1250000 || h.Basis != "watch elapsed" {
		t.Fatalf("gun scaling: %#v", h)
	}
	r.Result.GunTimeMS = nil
	r.Result.ManualTimeMS = raceTestPtr(int64(2400000))
	h = raceHalfway(r, &a)
	if !h.Scaled || h.Basis != "manual" {
		t.Fatalf("manual clock: %#v", h)
	}
	r.RaceOnly = false
	r.Result.SplitBasis = "gun"
	r.Result.GunTimeMS = raceTestPtr(int64(2500000))
	r.Result.Checkpoints = []RaceCheckpoint{{"Halfway", 5000, 1210000}}
	h = raceHalfway(r, nil)
	if h.Source != "official" || *h.SecondHalfMS != 1290000 || *h.DifferenceMS != 80000 {
		t.Fatalf("official: %#v", h)
	}
}
func TestRaceHalfwayRefusesUnreliableRecordings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Race, *Activity)
	}{
		{"unconfirmed", func(r *Race, a *Activity) { r.RaceOnly = false }},
		{"changed content", func(r *Race, a *Activity) { r.RecordingHash = "old" }},
		{"distance reset", func(r *Race, a *Activity) { a.Samples[50].DistanceM = raceTestPtr(10.0) }},
		{"reversed timestamp", func(r *Race, a *Activity) { a.Samples[50].Timestamp = &a.StartTime }},
		{"duplicate advancing time", func(r *Race, a *Activity) { a.Samples[50].Timestamp = a.Samples[49].Timestamp }},
		{"missing start", func(r *Race, a *Activity) { a.Samples = a.Samples[20:] }},
		{"missing finish", func(r *Race, a *Activity) { a.Samples = a.Samples[:90] }},
		{"large gap", func(r *Race, a *Activity) { a.Samples = append(a.Samples[:49], a.Samples[53:]...) }},
		{"no times", func(r *Race, a *Activity) {
			for i := range a.Samples {
				a.Samples[i].Timestamp = nil
				a.Samples[i].ElapsedS = nil
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := raceTestFinished()
			r.RaceOnly = true
			a := raceTestRecording()
			tc.change(&r, &a)
			h := raceHalfway(r, &a)
			if h.FirstHalfMS != nil || h.Reason == "" {
				t.Fatalf("accepted: %#v", h)
			}
		})
	}
}
func TestRaceHalfwayInterpolationAndHash(t *testing.T) {
	r := raceTestFinished()
	r.RaceOnly = true
	a := raceTestRecording()
	a.Samples = append(a.Samples[:51], a.Samples[52:]...)
	h := raceHalfway(r, &a)
	if h.FirstHalfMS == nil || *h.FirstHalfMS != 1200000 {
		t.Fatalf("%#v", h)
	}
	hash := raceRecordingHash(a.StartTime, a.DistanceM, a.ElapsedTimeS, a.Samples)
	a.Samples[1], a.Samples[2] = a.Samples[2], a.Samples[1]
	if hash != raceRecordingHash(a.StartTime, a.DistanceM, a.ElapsedTimeS, a.Samples) {
		t.Fatal("storage ordering changed hash")
	}
	a.Samples[0].DistanceM = raceTestPtr(1.0)
	if hash == raceRecordingHash(a.StartTime, a.DistanceM, a.ElapsedTimeS, a.Samples) {
		t.Fatal("content change did not change hash")
	}
}
func TestRacePerformanceEligibilityAndReference(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	a := raceTestFinished()
	a.ID = "recent"
	b := raceTestFinished()
	b.ID = "older"
	b.Date = "2025-01-01"
	b.Result.ChipTimeMS = raceTestPtr(int64(2300000))
	c := a
	c.ID = "trail"
	c.Discipline = "trail"
	c.Result.ChipTimeMS = raceTestPtr(int64(2200000))
	d := a
	d.ID = "excluded"
	d.Result.Excluded = true
	d.Result.ChipTimeMS = raceTestPtr(int64(2000000))
	e := a
	e.ID = "dnf"
	e.Status = "dnf"
	f := a
	f.ID = "pending"
	f.Result.Confirmed = false
	p := buildRacePerformance([]Race{a, b, c, d, e, f}, RaceSettings{}, now)
	if p.Reference == nil || p.Reference.ID != "recent" {
		t.Fatalf("reference %#v", p.Reference)
	}
	if p.Results[0].PersonalBest || !p.Results[0].SeasonBest || !p.Results[1].PersonalBest || !p.Results[2].PersonalBest || p.Results[2].Metrics.VDOT != nil || p.Results[3].PersonalBest || p.Results[4].Metrics.Eligible || p.Results[5].Metrics.Eligible {
		t.Fatalf("eligibility: %#v", p.Results)
	}
	p = buildRacePerformance([]Race{b}, RaceSettings{}, now)
	if p.Reference != nil {
		t.Fatal("stale fallback")
	}
	p = buildRacePerformance([]Race{a, b}, RaceSettings{VDOTReferenceID: "older"}, now)
	if p.Reference == nil || p.Reference.ID != "older" {
		t.Fatal("override ignored")
	}
	p = buildRacePerformance([]Race{d}, RaceSettings{VDOTReferenceID: "excluded"}, now)
	if p.Reference != nil {
		t.Fatal("excluded reference")
	}
}
func TestRaceAgeGradeUsesExactRoadTableAndBirthday(t *testing.T) {
	r := raceTestFinished()
	settings := RaceSettings{BirthDate: "1986-09-21", GradingTable: "M"}
	m := raceMetrics(r, settings)
	if m.Age == nil || *m.Age != 39 || m.AgeGrade == nil {
		t.Fatalf("%#v", m)
	}
	// Read a known bundled standard/factor independently from the formula under test.
	var standard, factor float64
	for _, table := range raceAgeTables.Distances {
		if table.DistanceM == 10000 {
			standard = table.Standards["M"]
			factor = table.Factors["M"]["39"]
		}
	}
	want := standard / (factor * 2400) * 100
	if math.Abs(*m.AgeGrade-want) > 1e-9 {
		t.Fatalf("got %f want %f", *m.AgeGrade, want)
	}
	r.Date = "2026-09-21"
	if *raceMetrics(r, settings).Age != 40 {
		t.Fatal("birthday")
	}
	r.Discipline = "track"
	m = raceMetrics(r, settings)
	if m.AgeGrade != nil || m.VDOT == nil {
		t.Fatal("track grading")
	}
	r.Discipline = "road"
	r.DistanceM = raceTestPtr(10001.0)
	if raceMetrics(r, settings).AgeGrade != nil {
		t.Fatal("interpolated distance")
	}
	r.DistanceM = raceTestPtr(10000.0)
	r.AgeOverride = raceTestPtr(120)
	if raceMetrics(r, settings).AgeGrade != nil {
		t.Fatal("extrapolated age")
	}
	r.AgeOverride = raceTestPtr(50)
	r.TableOverride = "F"
	m = raceMetrics(r, settings)
	if m.AgeGrade == nil || m.Table != "F" || *m.Age != 50 {
		t.Fatal("override")
	}
}

// Golden values copied from the licensed AgeGrade.10k source file.
func TestRaceAgeGradeGolden2025RoadStandard(t *testing.T) {
	r := raceTestFinished()
	r.AgeOverride = raceTestPtr(40)
	r.TableOverride = "M"
	got := raceMetrics(r, RaceSettings{})
	want := 1584.0 / .9636 / 2400 * 100
	if got.AgeGrade == nil || math.Abs(*got.AgeGrade-want) > 1e-9 {
		t.Fatalf("got %#v want %f", got, want)
	}
	r.TableOverride = "F"
	got = raceMetrics(r, RaceSettings{})
	want = 1726.0 / .9623 / 2400 * 100
	if got.AgeGrade == nil || math.Abs(*got.AgeGrade-want) > 1e-9 {
		t.Fatalf("got %#v want %f", got, want)
	}
}

func TestRaceDiscoveryRequiresCorroboratingSignals(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    raceActivitySignal
		want bool
	}{
		{"explicit", raceActivitySignal{Sport: "Run", Name: "Easy", ExplicitRace: true}, true},
		{"name", raceActivitySignal{Sport: "Run", Name: "Local parkrun"}, true},
		{"training", raceActivitySignal{Sport: "Run", Name: "Marathon pace training"}, false},
		{"nonrunning", raceActivitySignal{Sport: "Ride", Name: "Race", ExplicitRace: true}, false},
		{"recovery", raceActivitySignal{Sport: "Run", Name: "Race workout", HasRecovery: true}, false},
		{"numeric", raceActivitySignal{Sport: "Run", DistanceM: 10040, ElapsedTimeS: 2400, MovingTimeS: 2390}, true},
		{"stops", raceActivitySignal{Sport: "Run", DistanceM: 10040, ElapsedTimeS: 2500, MovingTimeS: 2300}, false},
		{"distance alone", raceActivitySignal{Sport: "Run", DistanceM: 10000, ElapsedTimeS: 4000, MovingTimeS: 4000}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reasons, _ := raceDiscoveryReasons(tc.a, []float64{.30, .31, .32, .33, .34})
			if (len(reasons) > 0) != tc.want {
				t.Fatal(reasons)
			}
		})
	}
}
func TestRacePredictionsRetainGapsAndNeverUseSameDay(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"predictions":[{"calendarDate":"2026-09-19","raceTime5K":1000.125,"raceTime10K":2200,"raceTimeHalf":null,"raceTimeMarathon":-1},{"calendarDate":"2026-09-20","raceTime10K":2100},{"raceTime10K":2000}]}`)
	rows, err := normalizeRacePredictions(raw, "2026-09-01", "2026-10-01", now)
	if err != nil || len(rows) != 8 {
		t.Fatalf("%#v %v", rows, err)
	}
	if *rows[0].TimeMS != 1000125 || rows[2].TimeMS != nil || rows[3].TimeMS != nil || !rows[0].Backfilled || len(rows[0].Raw) == 0 {
		t.Fatal(rows)
	}
	p := latestRacePrediction(rows, raceTestFinished())
	if p == nil || p.Date != "2026-09-19" || *p.TimeMS != 2200000 {
		t.Fatalf("%#v", p)
	}
}
func TestRaceForecastUsesSharedLimiterAndNullableHours(t *testing.T) {
	r := raceTestFinished()
	r.Latitude = raceTestPtr(53.12349)
	r.Longitude = raceTestPtr(-6.12349)
	at := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	reserved := 0
	service := &OpenMeteoWeatherService{forecastURL: "https://forecast.test", now: func() time.Time { return at }, reserveLimit: func(context.Context, time.Time, int) (time.Duration, error) { reserved++; return 0, nil }}
	service.client = &http.Client{Transport: openMeteoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		q := req.URL.Query()
		if q.Get("latitude") != "53.123" || q.Get("longitude") != "-6.123" || q.Get("timeformat") != "unixtime" || q.Get("timezone") != "Europe/Dublin" || q.Has("name") {
			t.Fatal(q)
		}
		body, _ := json.Marshal(map[string]any{"hourly": map[string]any{"time": []int64{at.Unix(), at.Add(time.Hour).Unix()}, "temperature_2m": []any{12, nil}, "precipitation_probability": []any{nil, 0}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	hours, raw, err := service.fetchRaceForecast(context.Background(), r)
	if err != nil || reserved != 1 || len(hours) != 2 || hours[0].RainProbability != nil || hours[1].TemperatureC != nil || *hours[1].RainProbability != 0 || len(raw) == 0 {
		t.Fatalf("%#v %v", hours, err)
	}
}
func TestRaceReportOmitPrivateLogisticsAndLabelEstimates(t *testing.T) {
	r := raceTestFinished()
	r.TravelNotes = "SECRET HOTEL"
	r.Bib = "SECRET BIB"
	r.RegistrationNotes = "SECRET REGISTRATION"
	r.RaceOnly = true
	a := raceTestRecording()
	detail := RaceDetail{Race: r, Metrics: raceMetrics(r, RaceSettings{BirthDate: "1986-09-21", GradingTable: "M"}), Halfway: raceHalfway(r, &a)}
	draft := defaultRaceReport()
	draft.Reflections = "A strong finish."
	text := raceReportMarkdown(detail, draft)
	for _, secret := range []string{"SECRET", "1986-09-21"} {
		if strings.Contains(text, secret) {
			t.Fatal("private data in report")
		}
	}
	for _, expected := range []string{"A strong finish.", "estimated", "scaled", "40:00"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %s: %s", expected, text)
		}
	}
}
func TestRaceChecklistResourceRouting(t *testing.T) {
	for _, path := range []string{"/api/race-checklists", "/api/race-checklists/123"} {
		if raceResourceKind(httptest.NewRequest("GET", path, nil)) != "templates" {
			t.Fatal(path)
		}
	}
}

func TestRaceReportRejectsNullSectionsAndAllowsEmptySelection(t *testing.T) {
	if validateRaceReport(RaceReport{}) == nil {
		t.Fatal("null sections would break the editor")
	}
	if err := validateRaceReport(RaceReport{Sections: []string{}}); err != nil {
		t.Fatal(err)
	}
	if validateRaceReport(RaceReport{Sections: []string{"private-logistics"}}) == nil {
		t.Fatal("unknown report section")
	}
}
