package app

import (
	"strings"
	"testing"
)

func TestParseWorkoutPrescriptionRepeatsAndFinalRecovery(t *testing.T) {
	result := parseWorkoutPrescription("15mins easy warm up//3x5mins@3:35(2mins), then 7x40secs uphill HARD(jog down to start recovery)//15mins easy cool down", nil)
	if result.Status == workoutParseError {
		t.Fatalf("parse failed: %#v", result.Messages)
	}
	if len(result.Definition.Steps) != 4 {
		t.Fatalf("steps = %#v", result.Definition.Steps)
	}
	first, last := result.Definition.Steps[1], result.Definition.Steps[2]
	if first.SkipLastRecovery {
		t.Fatal("earlier repeat group skipped its final recovery")
	}
	if !last.SkipLastRecovery || len(last.Children) != 2 || last.Children[1].EndCondition.Type != workoutEndLapButton {
		t.Fatalf("final repeat = %#v", last)
	}
}

func TestParseWorkoutPrescriptionNestedSetsAndPaceAlias(t *testing.T) {
	sets := parseWorkoutPrescription("15mins warm up//5sets of: 3mins@3:25, 1min jog, 1min@3:15, 1min jog//15mins cool down", nil)
	if sets.Status == workoutParseError || len(sets.Definition.Steps) != 3 || sets.Definition.Steps[1].RepeatCount != 5 {
		t.Fatalf("sets = %#v", sets)
	}
	alias := parseWorkoutPrescription("15mins warm up//4x2mins:3:40(1min jog)//10mins cool down", nil)
	if alias.Status != workoutParseWarning || alias.Definition.Steps[1].Children[0].Target.PaceSecondsPerKM == nil || *alias.Definition.Steps[1].Children[0].Target.PaceSecondsPerKM != 220 {
		t.Fatalf("alias = %#v", alias)
	}
}

func TestParseWorkoutPrescriptionContinuousUsesExactAnalysisRanges(t *testing.T) {
	table := &trainingSheetWorkoutTable{Rows: []trainingSheetWorkoutTableRow{
		{Kind: trainingSheetRowExact, Group: "range:0-15"},
		{Kind: trainingSheetRowExact, Group: "range:15-25"},
		{Kind: trainingSheetRowExact, Group: "range:25-35"},
		{Kind: trainingSheetRowExact, Group: "range:35-45"},
	}}
	result := parseWorkoutPrescription("10mins warm up//45mins@4:15(continuous)//10mins cool down", table)
	if result.Status == workoutParseError {
		t.Fatalf("parse failed: %#v", result.Messages)
	}
	want := []float64{600, 900, 600, 600, 600, 600}
	if len(result.Definition.Steps) != len(want) {
		t.Fatalf("steps = %#v", result.Definition.Steps)
	}
	for index, duration := range want {
		if result.Definition.Steps[index].EndCondition.Value != duration {
			t.Fatalf("step %d duration = %v, want %v", index, result.Definition.Steps[index].EndCondition.Value, duration)
		}
	}
}

func TestParseWorkoutPrescriptionContinuousRejectsInvalidRanges(t *testing.T) {
	table := &trainingSheetWorkoutTable{Rows: []trainingSheetWorkoutTableRow{
		{Kind: trainingSheetRowExact, Group: "range:0-5"},
		{Kind: trainingSheetRowExact, Group: "range:10-20"},
	}}
	result := parseWorkoutPrescription("10mins warm up//20mins@4:20(continuous)//10mins cool down", table)
	if result.Status != workoutParseError || !strings.Contains(result.Messages[len(result.Messages)-1].Message, "contiguous") {
		t.Fatalf("result = %#v", result)
	}
}

func TestParseWorkoutPrescriptionPaceRangesAndHashIgnoreResults(t *testing.T) {
	result := parseWorkoutPrescription("10mins warm up//50mins@4:05-4:10(continuous)//10mins cool down", nil)
	target := result.Definition.Steps[1].Target
	if target.PaceFastSecondsKM == nil || *target.PaceFastSecondsKM != 245 || target.PaceSlowSecondsKM == nil || *target.PaceSlowSecondsKM != 250 {
		t.Fatalf("target = %#v", target)
	}
	first := &trainingSheetWorkoutTable{Rows: []trainingSheetWorkoutTableRow{{Kind: trainingSheetRowExact, Group: "range:0-10", Label: "0-10"}}}
	second := &trainingSheetWorkoutTable{Rows: []trainingSheetWorkoutTableRow{{Kind: trainingSheetRowExact, Group: "range:0-10", Label: "changed display"}}}
	if workoutSourceHash("workout", first) != workoutSourceHash("workout", second) {
		t.Fatal("display/result changes altered source hash")
	}
}

func TestParseWorkoutPrescriptionSurges(t *testing.T) {
	result := parseWorkoutPrescription("45mins easy, conversational pace, with a fast & relaxed 30-second surge, every 5 minutes", nil)
	if result.Status == workoutParseError {
		t.Fatalf("parse failed: %#v", result.Messages)
	}
	if result.Definition.EstimatedDurationS != 45*60 || len(result.Definition.Steps) != 1 {
		t.Fatalf("definition = %#v", result.Definition)
	}
	repeat := result.Definition.Steps[0]
	if repeat.Kind != workoutStepRepeat || repeat.RepeatCount != 9 || repeat.SkipLastRecovery || len(repeat.Children) != 2 {
		t.Fatalf("repeat = %#v", repeat)
	}
	if repeat.Children[0].Kind != workoutStepWork || repeat.Children[0].EndCondition == nil || repeat.Children[0].EndCondition.Value != 270 {
		t.Fatalf("steady step = %#v", repeat.Children[0])
	}
	if repeat.Children[1].Kind != workoutStepWork || repeat.Children[1].EndCondition == nil || repeat.Children[1].EndCondition.Value != 30 {
		t.Fatalf("surge step = %#v", repeat.Children[1])
	}
	for _, child := range repeat.Children {
		if child.Target.Type != workoutTargetNone {
			t.Fatalf("surge child has target: %#v", child)
		}
	}
}

func TestParseWorkoutPrescriptionSurgesKeepsRemainder(t *testing.T) {
	result := parseWorkoutPrescription("47 min with surges", nil)
	if result.Status == workoutParseError || result.Definition.EstimatedDurationS != 47*60 || len(result.Definition.Steps) != 2 {
		t.Fatalf("definition = %#v, messages = %#v", result.Definition, result.Messages)
	}
	if result.Definition.Steps[0].RepeatCount != 9 {
		t.Fatalf("repeat = %#v", result.Definition.Steps[0])
	}
	remainder := result.Definition.Steps[1]
	if remainder.Kind != workoutStepWork || remainder.EndCondition == nil || remainder.EndCondition.Value != 120 || remainder.Description != "Final run" {
		t.Fatalf("remainder = %#v", remainder)
	}

	short := parseWorkoutPrescription("7mins w/surges", nil)
	if short.Status == workoutParseError || len(short.Definition.Steps) != 3 || short.Definition.EstimatedDurationS != 7*60 {
		t.Fatalf("short definition = %#v, messages = %#v", short.Definition, short.Messages)
	}
	for _, step := range short.Definition.Steps {
		if step.Kind != workoutStepWork {
			t.Fatalf("short step = %#v", step)
		}
	}
}

func TestWorkoutPrescriptionForPlannedUsesSurgeTitleFallback(t *testing.T) {
	full := "45mins easy, conversational pace, with a fast & relaxed 30-second surge, every 5 minutes"
	if got := workoutPrescriptionForPlanned(PlannedActivity{Name: "45mins w/surges", Notes: full}); got != full {
		t.Fatalf("detailed prescription = %q", got)
	}
	if got := workoutPrescriptionForPlanned(PlannedActivity{Name: "50mins w/surges", Notes: "Easy conversational run"}); got != "50mins w/surges" {
		t.Fatalf("title fallback = %q", got)
	}
	if got := workoutPrescriptionForPlanned(PlannedActivity{Name: "45mins", Notes: "Easy conversational run"}); got != "" {
		t.Fatalf("unstructured prescription = %q", got)
	}
}
