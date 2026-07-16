package app

import (
	"archive/zip"
	"bytes"
	"context"
	"math"
	"strings"
	"testing"
	"time"
)

func TestParseGarminActivityDownloadRejectsArchiveWithoutActivityFile(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	file, err := zw.Create("readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("not an activity")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = parseGarminActivityDownload(context.Background(), "123", buf.Bytes())
	if err == nil || !strings.Contains(err.Error(), "supported FIT, TCX, or GPX") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyGarminMetadata(t *testing.T) {
	start := time.Date(2026, 7, 15, 8, 30, 0, 0, time.UTC)
	activity := ImportedActivity{
		Name:      "123",
		SportType: "Run",
		Raw:       map[string]any{"format": "fit"},
	}
	applyGarminMetadata(&activity, GarminBridgeActivity{
		ID:                       "123",
		Name:                     "Morning Run",
		SportType:                "running",
		StartTime:                start,
		AvgGradeAdjustedSpeedMPS: floatPtr(4),
	})

	if activity.Name != "Morning Run" {
		t.Fatalf("name = %q", activity.Name)
	}
	if activity.SportType != "Run" {
		t.Fatalf("sport = %q", activity.SportType)
	}
	if activity.Raw["provider"] != garminProvider || activity.Raw["garmin_id"] != "123" {
		t.Fatalf("raw metadata = %#v", activity.Raw)
	}
	if activity.OriginalProviderURL != "https://connect.garmin.com/modern/activity/123" {
		t.Fatalf("original provider URL = %q", activity.OriginalProviderURL)
	}
	if activity.AvgGradeAdjustedPaceSPKM == nil || *activity.AvgGradeAdjustedPaceSPKM != 250 {
		t.Fatalf("GAP = %#v, want 250", activity.AvgGradeAdjustedPaceSPKM)
	}
	if activity.Raw["garmin_avg_grade_adjusted_speed_mps"] != float64(4) {
		t.Fatalf("raw GAP speed = %#v", activity.Raw["garmin_avg_grade_adjusted_speed_mps"])
	}
}

func TestApplyGarminLapMetadata(t *testing.T) {
	activity := ImportedActivity{
		Laps: []ActivityLap{
			{Index: 0},
			{Index: 1},
			{Index: 2},
		},
	}

	applyGarminLapMetadata(&activity, []GarminBridgeLap{
		{Index: 0, AvgGradeAdjustedSpeedMPS: floatPtr(4)},
		{Index: 1},
		{Index: 2, AvgGradeAdjustedSpeedMPS: floatPtr(0)},
	})

	if activity.Laps[0].AvgGradeAdjustedPaceSPKM == nil || *activity.Laps[0].AvgGradeAdjustedPaceSPKM != 250 {
		t.Fatalf("lap 0 GAP = %#v, want 250", activity.Laps[0].AvgGradeAdjustedPaceSPKM)
	}
	if activity.Laps[1].AvgGradeAdjustedPaceSPKM != nil {
		t.Fatalf("lap 1 GAP = %#v, want nil", activity.Laps[1].AvgGradeAdjustedPaceSPKM)
	}
	if activity.Laps[2].AvgGradeAdjustedPaceSPKM != nil {
		t.Fatalf("lap 2 GAP = %#v, want nil", activity.Laps[2].AvgGradeAdjustedPaceSPKM)
	}
}

func TestGradeAdjustedPaceFromSpeedMPS(t *testing.T) {
	speed := 3.2
	pace := gradeAdjustedPaceFromSpeedMPS(&speed)
	if pace == nil || math.Abs(*pace-312.5) > 0.0001 {
		t.Fatalf("pace = %#v, want 312.5", pace)
	}
	if pace := gradeAdjustedPaceFromSpeedMPS(nil); pace != nil {
		t.Fatalf("nil speed pace = %#v, want nil", pace)
	}
	zero := 0.0
	if pace := gradeAdjustedPaceFromSpeedMPS(&zero); pace != nil {
		t.Fatalf("zero speed pace = %#v, want nil", pace)
	}
}
