package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPlannedMatchDurationColorTrusted(t *testing.T) {
	tests := []struct {
		name  string
		color string
		want  bool
	}{
		{name: "default white", color: "#ffffff", want: true},
		{name: "near white", color: "#f7f6f8", want: true},
		{name: "purple", color: "#674ea7", want: true},
		{name: "lavender", color: "#d9d2e9", want: true},
		{name: "blue", color: "#3d85c6", want: false},
		{name: "yellow", color: "#ffd966", want: false},
		{name: "missing", color: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := plannedMatchDurationColorTrusted(tt.color); got != tt.want {
				t.Fatalf("plannedMatchDurationColorTrusted(%q) = %v, want %v", tt.color, got, tt.want)
			}
		})
	}
}

func TestParsePlannedMatchTitleDuration(t *testing.T) {
	tests := []struct {
		title string
		want  int
		ok    bool
	}{
		{title: "40mins easy", want: 40 * 60, ok: true},
		{title: "2h long run", want: 2 * 60 * 60, ok: true},
		{title: "2 hours", want: 2 * 60 * 60, ok: true},
		{title: "1h30m steady", want: 90 * 60, ok: true},
		{title: "1.5 hrs", want: 90 * 60, ok: true},
		{title: "5x3mins", ok: false},
		{title: "2 x 20 minutes", ok: false},
		{title: "Intervals", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got, ok := parsePlannedMatchTitleDuration(tt.title)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("parsePlannedMatchTitleDuration(%q) = %d/%v, want %d/%v", tt.title, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPlannedActivityExpectedDurationUsesOnlyTrustedTitleColors(t *testing.T) {
	white := PlannedActivity{Name: "40mins", Raw: map[string]any{"planCellBackgroundColor": "#ffffff"}}
	if got, ok := plannedActivityExpectedDurationS(white); !ok || got != 2400 {
		t.Fatalf("white duration = %d/%v, want 2400/true", got, ok)
	}
	for _, planned := range []PlannedActivity{
		{Name: "25mins", Raw: map[string]any{"planCellBackgroundColor": "#3d85c6"}},
		{Name: "50mins", Raw: map[string]any{"planCellBackgroundColor": "#ffd966"}},
		{Name: "40mins", Raw: map[string]any{}},
	} {
		if got, ok := plannedActivityExpectedDurationS(planned); ok || got != 0 {
			t.Fatalf("untrusted duration for %#v = %d/%v, want 0/false", planned.Raw, got, ok)
		}
	}
}

func TestPlannedActivityStructuredSignals(t *testing.T) {
	tests := []struct {
		name       string
		planned    PlannedActivity
		structured bool
		known      bool
	}{
		{name: "workout table", planned: PlannedActivity{Raw: map[string]any{"workoutTable": map[string]any{"rows": []any{map[string]any{"label": "rep"}}}}}, structured: true, known: true},
		{name: "repeat title", planned: PlannedActivity{Name: "5x3mins"}, structured: true, known: true},
		{name: "interval notes", planned: PlannedActivity{Name: "Session", Notes: "Intervals with recovery"}, structured: true, known: true},
		{name: "continuous", planned: PlannedActivity{Name: "Easy run", Raw: map[string]any{"planCellBackgroundColor": "#3d85c6"}}, structured: false, known: true},
		{name: "unknown", planned: PlannedActivity{}, structured: false, known: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structured, known := plannedActivityStructured(tt.planned)
			if structured != tt.structured || known != tt.known {
				t.Fatalf("plannedActivityStructured() = %v/%v, want %v/%v", structured, known, tt.structured, tt.known)
			}
		})
	}
}

func TestAssessPlannedActivityMatch(t *testing.T) {
	activityDate := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	continuousPlan := func(name string) PlannedActivity {
		return PlannedActivity{ID: name, Name: name, PlannedDate: activityDate, Raw: map[string]any{"planCellBackgroundColor": "#ffffff"}}
	}

	strong := assessPlannedActivityMatch(activityDate, 42*60, 0, false, continuousPlan("40mins"))
	if strong.MatchScore != 98 || strong.MatchLevel != plannedMatchLevelStrong || strong.suggestionBlocked {
		t.Fatalf("strong assessment = %#v", strong)
	}

	grossMismatch := assessPlannedActivityMatch(activityDate, 35*60, 0, false, continuousPlan("2h"))
	if grossMismatch.MatchScore != 59 || grossMismatch.MatchLevel != plannedMatchLevelWeak || !grossMismatch.suggestionBlocked {
		t.Fatalf("duration mismatch assessment = %#v", grossMismatch)
	}

	boundary := assessPlannedActivityMatch(activityDate, 40*60, 0, false, continuousPlan("60mins"))
	if boundary.MatchScore != 87 || boundary.suggestionBlocked {
		t.Fatalf("1.5x duration boundary = %#v", boundary)
	}

	structuredMismatch := assessPlannedActivityMatch(activityDate, 40*60, 0, true, continuousPlan("40mins"))
	if structuredMismatch.MatchScore != 59 || !structuredMismatch.suggestionBlocked || !strings.Contains(strings.Join(structuredMismatch.MatchReasons, " "), "activity has intervals") {
		t.Fatalf("structure mismatch assessment = %#v", structuredMismatch)
	}

	unknownDuration := assessPlannedActivityMatch(activityDate, 40*60, 0, false, continuousPlan("Easy run"))
	if unknownDuration.MatchScore != 80 || unknownDuration.MatchLevel != plannedMatchLevelStrong || unknownDuration.suggestionBlocked {
		t.Fatalf("unknown-duration assessment = %#v", unknownDuration)
	}

	unknownPlan := PlannedActivity{ID: "unknown", PlannedDate: activityDate}
	dateOnly := assessPlannedActivityMatch(activityDate, 40*60, 0, false, unknownPlan)
	if dateOnly.MatchScore != 65 || dateOnly.MatchLevel != plannedMatchLevelPossible {
		t.Fatalf("date-only assessment = %#v", dateOnly)
	}

	farPlan := continuousPlan("40mins")
	farPlan.PlannedDate = activityDate.AddDate(0, 0, 7)
	far := assessPlannedActivityMatch(activityDate, 40*60, 0, false, farPlan)
	if far.MatchScore != 70 || far.MatchLevel != plannedMatchLevelPossible {
		t.Fatalf("far assessment = %#v", far)
	}
}

func TestSuggestedPlannedActivityRequiresStrongClearWinner(t *testing.T) {
	candidate := func(id string, score int, blocked bool) PlannedActivityMatchCandidate {
		return PlannedActivityMatchCandidate{PlannedActivity: PlannedActivity{ID: id}, MatchScore: score, suggestionBlocked: blocked}
	}
	tests := []struct {
		name       string
		candidates []PlannedActivityMatchCandidate
		want       string
	}{
		{name: "single strong", candidates: []PlannedActivityMatchCandidate{candidate("one", 80, false)}, want: "one"},
		{name: "clear winner", candidates: []PlannedActivityMatchCandidate{candidate("one", 91, false), candidate("two", 81, false)}, want: "one"},
		{name: "ambiguous", candidates: []PlannedActivityMatchCandidate{candidate("one", 90, false), candidate("two", 81, false)}, want: ""},
		{name: "weak", candidates: []PlannedActivityMatchCandidate{candidate("one", 79, false)}, want: ""},
		{name: "blocked", candidates: []PlannedActivityMatchCandidate{candidate("one", 90, true)}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := suggestedPlannedActivityID(tt.candidates); got != tt.want {
				t.Fatalf("suggestedPlannedActivityID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlannedActivityMatchCandidateJSONShape(t *testing.T) {
	candidate := PlannedActivityMatchCandidate{
		PlannedActivity:   PlannedActivity{ID: "planned-1", Name: "Easy run", WorkoutID: "workout-1"},
		MatchScore:        80,
		MatchLevel:        plannedMatchLevelStrong,
		MatchReasons:      []string{"same day"},
		suggestionBlocked: true,
	}
	payload, err := json.Marshal(candidate)
	if err != nil {
		t.Fatalf("marshal candidate: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal candidate: %v", err)
	}
	if decoded["id"] != "planned-1" || decoded["workoutId"] != "workout-1" || decoded["matchScore"] != float64(80) || decoded["matchLevel"] != plannedMatchLevelStrong {
		t.Fatalf("candidate JSON = %#v", decoded)
	}
	if _, ok := decoded["PlannedActivity"]; ok {
		t.Fatalf("candidate JSON nests planned activity: %#v", decoded)
	}
	if _, ok := decoded["suggestionBlocked"]; ok {
		t.Fatalf("candidate JSON exposes internal blocker: %#v", decoded)
	}
}
