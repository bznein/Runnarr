package app

import (
	"errors"
	"math"
	"testing"
)

func TestCompileGarminWorkoutUsesPaceToleranceAndSkipLastRest(t *testing.T) {
	parsed := parseWorkoutPrescription("15mins warm up//5x7mins@3:35(2mins)//15mins cool down", nil)
	compiled, err := compileGarminWorkout(parsed.Definition, 5, "owner-token")
	if err != nil {
		t.Fatal(err)
	}
	segment := compiled.Payload["workoutSegments"].([]any)[0].(map[string]any)
	steps := segment["workoutSteps"].([]any)
	repeat := steps[1].(map[string]any)
	if repeat["skipLastRestStep"] != true {
		t.Fatalf("repeat = %#v", repeat)
	}
	work := repeat["workoutSteps"].([]any)[0].(map[string]any)
	wantLow := 1000.0 / 220.0
	wantHigh := 1000.0 / 210.0
	if math.Abs(work["targetValueOne"].(float64)-wantLow) > 0.000001 || math.Abs(work["targetValueTwo"].(float64)-wantHigh) > 0.000001 {
		t.Fatalf("pace bounds = %#v", work)
	}
	if compiled.OwnershipMarker == "" || compiled.DefinitionHash == "" || compiled.Payload["description"] != compiled.OwnershipMarker {
		t.Fatalf("compiled = %#v", compiled)
	}
}

func TestCompileGarminWorkoutExplicitRangeIgnoresTolerance(t *testing.T) {
	parsed := parseWorkoutPrescription("10mins warm up//20mins@4:05-4:10(continuous)//10mins cool down", nil)
	compiled, err := compileGarminWorkout(parsed.Definition, 20, "owner-token")
	if err != nil {
		t.Fatal(err)
	}
	steps := compiled.Payload["workoutSegments"].([]any)[0].(map[string]any)["workoutSteps"].([]any)
	work := steps[1].(map[string]any)
	if math.Abs(work["targetValueOne"].(float64)-(1000.0/250.0)) > 0.000001 || math.Abs(work["targetValueTwo"].(float64)-(1000.0/245.0)) > 0.000001 {
		t.Fatalf("pace bounds = %#v", work)
	}
}

func TestGarminWorkoutOwnershipRequiresIDAndMarker(t *testing.T) {
	remote := map[string]any{"workoutId": "123", "description": "runnarr:owner:hash"}
	if err := garminWorkoutRemoteOwned(remote, "123", "runnarr:owner:hash"); err != nil {
		t.Fatal(err)
	}
	numericRemote := map[string]any{"workoutId": float64(1648315012), "description": "runnarr:owner:hash"}
	if err := garminWorkoutRemoteOwned(numericRemote, "1648315012", "runnarr:owner:hash"); err != nil {
		t.Fatalf("numeric Garmin workout ID should retain its integer representation: %v", err)
	}
	fractionalRemote := map[string]any{"workoutId": 123.5, "description": "runnarr:owner:hash"}
	if err := garminWorkoutRemoteOwned(fractionalRemote, "123", "runnarr:owner:hash"); !errors.Is(err, errGarminWorkoutOwnership) {
		t.Fatalf("fractional Garmin workout ID must not establish ownership: %v", err)
	}
	for _, test := range []struct {
		id, marker string
	}{
		{"different", "runnarr:owner:hash"},
		{"123", "different"},
		{"", "runnarr:owner:hash"},
	} {
		if err := garminWorkoutRemoteOwned(remote, test.id, test.marker); !errors.Is(err, errGarminWorkoutOwnership) {
			t.Fatalf("ownership error = %v", err)
		}
	}
	matchingNameOnly := map[string]any{"workoutId": "123", "workoutName": "Runnarr running workout deadbeef"}
	if err := garminWorkoutRemoteOwned(matchingNameOnly, "123", "runnarr:owner:hash"); !errors.Is(err, errGarminWorkoutOwnership) {
		t.Fatalf("a matching name must never establish ownership: %v", err)
	}
}

func TestVerifyGarminManagedTemplateRequiresNormalizedAndRawOwnership(t *testing.T) {
	template := garminManagedTemplate{ProviderWorkoutID: "123", OwnershipMarker: "runnarr:owner:hash"}
	owned := GarminBridgeWorkout{
		ID:          "123",
		Description: "runnarr:owner:hash",
		Raw:         map[string]any{"workoutId": "123", "description": "runnarr:owner:hash"},
	}
	if err := verifyGarminManagedTemplate(template, owned); err != nil {
		t.Fatal(err)
	}
	owned.Raw["description"] = "changed outside Runnarr"
	if err := verifyGarminManagedTemplate(template, owned); !errors.Is(err, errGarminWorkoutOwnership) {
		t.Fatalf("raw ownership mismatch = %v", err)
	}
}

func TestNormalizeGarminScheduledWorkoutReadsNestedWorkout(t *testing.T) {
	response := normalizeGarminScheduledWorkout(GarminBridgeScheduledWorkout{Raw: map[string]any{
		"workoutScheduleId": float64(1728684280),
		"calendarDate":      "2026-08-01",
		"workout": map[string]any{
			"workoutId": float64(1648393193),
		},
	}})
	if response.ID != "1728684280" || response.WorkoutID != "1648393193" || response.Date != "2026-08-01" {
		t.Fatalf("normalized schedule = %#v", response)
	}
}

func TestGarminScheduleUncertainOutcomeDoesNotRetry(t *testing.T) {
	if !garminScheduleRetryUnsafe("Garmin scheduling failed with an uncertain outcome; no automatic retry was attempted: timeout") {
		t.Fatal("timeout after scheduling must not be retried")
	}
	if !garminScheduleRetryUnsafe("Garmin returned an unverifiable scheduled workout; no automatic retry was attempted") {
		t.Fatal("unverifiable schedule must not be retried")
	}
	if garminScheduleRetryUnsafe("Garmin workout ownership could not be verified") {
		t.Fatal("a read-only ownership check can be retried")
	}
}

func TestParseWorkoutDistanceUsesMetres(t *testing.T) {
	parsed := parseWorkoutPrescription("10mins warm up//6x400m@3:30(90secs)//10mins cool down", nil)
	if parsed.Status == workoutParseError {
		t.Fatalf("parse failed: %#v", parsed.Messages)
	}
	condition := parsed.Definition.Steps[1].Children[0].EndCondition
	if condition.Type != workoutEndDistance || condition.Value != 400 {
		t.Fatalf("condition = %#v", condition)
	}
}
