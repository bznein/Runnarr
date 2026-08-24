package app

import (
	"math"
	"testing"
)

func TestApplyGarminDisplayedIntervalPacesUsesAverageSpeed(t *testing.T) {
	oldPace := 1000 / 5.181694935944121
	activity := Activity{Intervals: []ActivityInterval{
		{Category: "warmup", AvgPaceSPKM: &oldPace, Raw: map[string]any{"averageSpeed": 3.7}},
		{Category: "active", AvgPaceSPKM: &oldPace, Raw: map[string]any{
			"averageSpeed": 5.09499979019165, "averageMovingSpeed": 5.181694935944121,
		}},
	}}

	if !applyGarminDisplayedIntervalPaces(&activity) {
		t.Fatal("expected Garmin provider pace to be applied")
	}
	want := 1000 / 5.09499979019165
	if activity.Intervals[1].AvgPaceSPKM == nil || math.Abs(*activity.Intervals[1].AvgPaceSPKM-want) > 0.0001 {
		t.Fatalf("active interval pace = %#v, want %.6f", activity.Intervals[1].AvgPaceSPKM, want)
	}
	if math.Abs(*activity.Intervals[0].AvgPaceSPKM-oldPace) > 0.0001 {
		t.Fatalf("warmup pace changed to %#v", activity.Intervals[0].AvgPaceSPKM)
	}
}

func TestApplyGarminDisplayedIntervalPacesRejectsMissingOrInvalidValues(t *testing.T) {
	activity := Activity{Intervals: []ActivityInterval{
		{Category: "active", Raw: map[string]any{"averageSpeed": ""}},
		{Category: "active", Raw: map[string]any{"averageSpeed": 0}},
	}}
	if applyGarminDisplayedIntervalPaces(&activity) {
		t.Fatal("invalid provider values should not be applied")
	}
}

func TestTrainingSheetReconciliationFingerprintIncludesLiveValues(t *testing.T) {
	first := []TrainingSheetReconciliationChange{{Range: "'Week'!B12", CurrentValue: "3:08", ProposedValue: "3:16"}}
	second := []TrainingSheetReconciliationChange{{Range: "'Week'!B12", CurrentValue: "3:09", ProposedValue: "3:16"}}
	if trainingSheetReconciliationFingerprint("planned", "activity", first) == trainingSheetReconciliationFingerprint("planned", "activity", second) {
		t.Fatal("fingerprint should change when the live sheet value changes")
	}
}

func TestTrainingSheetReconciliationDiffWritesOnlyChangedPaces(t *testing.T) {
	updates := []googleValueRangeUpdate{
		{Range: "'Week'!B12", Values: [][]any{{"'3:16"}}},
		{Range: "'Week'!B13", Values: [][]any{{"'3:20"}}},
	}
	existing := [][][]string{{{"3:08"}}, {{"3:20"}}}

	changes, writes := trainingSheetReconciliationDiff(updates, []string{"Set 1", "Set 2"}, existing)
	if len(changes) != 1 || len(writes) != 1 {
		t.Fatalf("changes = %#v, writes = %#v", changes, writes)
	}
	if changes[0].CurrentValue != "3:08" || changes[0].ProposedValue != "3:16" || changes[0].Label != "Set 1" {
		t.Fatalf("change = %#v", changes[0])
	}
}
