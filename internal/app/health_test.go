package app

import (
	"testing"
	"time"
)

func TestGarminHealthSyncRangeDefaultsToLast90Days(t *testing.T) {
	now := time.Date(2026, 7, 16, 18, 0, 0, 0, time.UTC)
	from, to, err := garminHealthSyncRange(GarminHealthSyncOptions{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := from.Format("2006-01-02"), "2026-04-18"; got != want {
		t.Fatalf("from = %s, want %s", got, want)
	}
	if got, want := to.Format("2006-01-02"), "2026-07-16"; got != want {
		t.Fatalf("to = %s, want %s", got, want)
	}
	if daysInclusive(from, to) != 90 {
		t.Fatalf("days = %d, want 90", daysInclusive(from, to))
	}
}

func TestGarminHealthSyncRangeRejectsInvertedRange(t *testing.T) {
	_, _, err := garminHealthSyncRange(GarminHealthSyncOptions{
		From: time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
	}, time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected inverted range error")
	}
}

func TestNormalizeGarminHealthDay(t *testing.T) {
	day := GarminBridgeHealthDay{
		Date: "2026-07-16",
		Raw: map[string]any{
			"stats": map[string]any{
				"totalSteps":         float64(12345),
				"totalKilocalories":  float64(2450),
				"activeKilocalories": float64(720),
			},
			"heartRates": map[string]any{
				"averageHeartRate": float64(61),
				"maxHeartRate":     float64(142),
			},
			"restingHeartRate": map[string]any{
				"value": float64(48),
			},
			"sleep": map[string]any{
				"dailySleepDTO": map[string]any{
					"sleepTimeSeconds":  float64(27000),
					"deepSleepSeconds":  float64(4200),
					"lightSleepSeconds": float64(16800),
					"remSleepSeconds":   float64(4800),
					"awakeSleepSeconds": float64(1200),
				},
				"sleepScores": map[string]any{
					"overall": map[string]any{"value": float64(84)},
				},
			},
			"stress": map[string]any{
				"avgStressLevel": float64(31),
				"maxStressLevel": float64(76),
			},
			"bodyBattery": []any{
				map[string]any{
					"bodyBatteryValuesArray": []any{
						[]any{"2026-07-16T00:00:00", float64(42)},
						[]any{"2026-07-16T01:00:00", float64(50)},
						[]any{"2026-07-16T02:00:00", float64(68)},
					},
				},
			},
			"hrv": map[string]any{
				"lastNightAvg": float64(55),
				"status":       "balanced",
			},
			"bodyComposition": map[string]any{
				"totalAverage": map[string]any{
					"weight":  float64(72.4),
					"bodyFat": float64(14.2),
				},
			},
		},
	}

	metric := normalizeGarminHealthDay(day)
	assertIntPtr(t, "steps", metric.Steps, 12345)
	assertIntPtr(t, "total calories", metric.TotalCaloriesKcal, 2450)
	assertIntPtr(t, "active calories", metric.ActiveCaloriesKcal, 720)
	assertFloatPtr(t, "resting HR", metric.RestingHeartRateBPM, 48)
	assertFloatPtr(t, "average HR", metric.AvgHeartRateBPM, 61)
	assertFloatPtr(t, "max HR", metric.MaxHeartRateBPM, 142)
	assertIntPtr(t, "sleep", metric.SleepDurationS, 27000)
	assertIntPtr(t, "deep sleep", metric.DeepSleepS, 4200)
	assertFloatPtr(t, "sleep score", metric.SleepScore, 84)
	assertFloatPtr(t, "stress avg", metric.StressAvg, 31)
	assertFloatPtr(t, "stress max", metric.StressMax, 76)
	assertFloatPtr(t, "body battery min", metric.BodyBatteryMin, 42)
	assertFloatPtr(t, "body battery max", metric.BodyBatteryMax, 68)
	assertFloatPtr(t, "body battery avg", metric.BodyBatteryAvg, 53.333333333333336)
	assertFloatPtr(t, "hrv avg", metric.HRVAvgMS, 55)
	if metric.HRVStatus != "balanced" {
		t.Fatalf("HRV status = %q, want balanced", metric.HRVStatus)
	}
	assertFloatPtr(t, "weight", metric.WeightKG, 72.4)
	assertFloatPtr(t, "body fat", metric.BodyFatPct, 14.2)
	if metric.Provider != garminProvider || metric.Date != "2026-07-16" {
		t.Fatalf("provider/date = %s/%s", metric.Provider, metric.Date)
	}
}

func assertIntPtr(t *testing.T, name string, value *int, want int) {
	t.Helper()
	if value == nil || *value != want {
		t.Fatalf("%s = %#v, want %d", name, value, want)
	}
}

func assertFloatPtr(t *testing.T, name string, value *float64, want float64) {
	t.Helper()
	if value == nil || !almostEqual(*value, want) {
		t.Fatalf("%s = %#v, want %f", name, value, want)
	}
}

func almostEqual(left, right float64) bool {
	diff := left - right
	if diff < 0 {
		diff = -diff
	}
	return diff < 0.000001
}
