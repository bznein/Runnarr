package app

import (
	"encoding/json"
	"testing"
)

func TestRestoreActivityWorkoutRepeatMetadata(t *testing.T) {
	var workout ActivityWorkout
	if err := json.Unmarshal([]byte(`{
		"provider":"garmin",
		"steps":[{"index":1,"order":1,"type":"warmup"},{"index":1,"order":2,"type":"repeat","children":[{"index":101,"order":3,"type":"repeat"}]}],
		"raw":{"workoutSegments":[
			{"workoutSteps":[{"stepOrder":1}]},
			{"workoutSteps":[{"stepOrder":2,"skipLastRestStep":true,"workoutSteps":[{"stepOrder":3,"skipLastRestStep":false}]}]}
		]}
	}`), &workout); err != nil {
		t.Fatal(err)
	}
	restoreActivityWorkoutRepeatMetadata(&workout)
	if workout.Steps[0].SkipLastRecovery != nil {
		t.Fatal("missing optional flag became a value")
	}
	repeat := workout.Steps[1]
	if repeat.SkipLastRecovery == nil || !*repeat.SkipLastRecovery {
		t.Fatal("outer repeat did not recover skipped final rest")
	}
	if repeat.Children[0].SkipLastRecovery == nil || *repeat.Children[0].SkipLastRecovery {
		t.Fatal("nested explicit false was not preserved")
	}
	keep := false
	workout.Steps[1].SkipLastRecovery = &keep
	restoreActivityWorkoutRepeatMetadata(&workout)
	if *workout.Steps[1].SkipLastRecovery {
		t.Fatal("existing normalized flag was overwritten")
	}
}

func TestRestoreActivityWorkoutRepeatMetadataIgnoresMalformedRaw(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"workoutSegments":[null,{"workoutSteps":[null]}]}`, `{"workoutSegments":[{"workoutSteps":[{"skipLastRestStep":"true"}]}]}`} {
		workout := ActivityWorkout{Provider: garminProvider, Steps: []ActivityWorkoutStep{{Index: 1}}}
		if err := json.Unmarshal([]byte(raw), &workout.Raw); err != nil {
			t.Fatal(err)
		}
		restoreActivityWorkoutRepeatMetadata(&workout)
		if workout.Steps[0].SkipLastRecovery != nil {
			t.Fatalf("raw %s manufactured a flag", raw)
		}
	}
}
