package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
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

func TestGearActivitySourceIDsPaginatesAndCompacts(t *testing.T) {
	firstPage := make([]GarminBridgeGearActivity, garminGearActivityPageLimit)
	firstPage[0] = GarminBridgeGearActivity{ID: " activity-1 "}
	firstPage[1] = GarminBridgeGearActivity{ID: "activity-2"}
	firstPage[2] = GarminBridgeGearActivity{ID: "activity-1"}
	secondPage := []GarminBridgeGearActivity{{ID: "activity-3"}}
	bridge := stubGarminBridge{
		gearActivityPages: map[int][]GarminBridgeGearActivity{
			0:                           firstPage,
			garminGearActivityPageLimit: secondPage,
		},
	}
	service := &GarminService{bridge: bridge, tokenDir: "tokens"}

	sourceIDs, fetched, err := service.gearActivitySourceIDs(context.Background(), "shoe-1")
	if err != nil {
		t.Fatal(err)
	}
	if fetched != garminGearActivityPageLimit+len(secondPage) {
		t.Fatalf("fetched = %d, want %d", fetched, garminGearActivityPageLimit+len(secondPage))
	}
	want := []string{"activity-1", "activity-2", "activity-3"}
	if strings.Join(sourceIDs, ",") != strings.Join(want, ",") {
		t.Fatalf("source IDs = %#v, want %#v", sourceIDs, want)
	}
}

func TestGarminBridgeGearResponseAcceptsArrayDefaults(t *testing.T) {
	var response GarminBridgeGearResponse
	err := json.Unmarshal([]byte(`{"userProfilePk":"123","gear":[],"rawDefaults":[{"activityType":"running"}]}`), &response)
	if err != nil {
		t.Fatal(err)
	}
	if response.UserProfilePK != "123" {
		t.Fatalf("user profile pk = %q, want 123", response.UserProfilePK)
	}
	if response.RawDefaults == nil {
		t.Fatal("raw defaults should be preserved")
	}
}

type stubGarminBridge struct {
	gearActivityPages map[int][]GarminBridgeGearActivity
}

func (b stubGarminBridge) Connect(context.Context, string, string, string, string) (GarminBridgeProfile, error) {
	return GarminBridgeProfile{}, nil
}

func (b stubGarminBridge) ListActivities(context.Context, string, int, int) ([]GarminBridgeActivity, error) {
	return nil, nil
}

func (b stubGarminBridge) ListActivitySplits(context.Context, string, string) ([]GarminBridgeLap, error) {
	return nil, nil
}

func (b stubGarminBridge) DownloadActivity(context.Context, string, string) ([]byte, error) {
	return nil, nil
}

func (b stubGarminBridge) FetchHealthDay(context.Context, string, string) (GarminBridgeHealthDay, error) {
	return GarminBridgeHealthDay{}, nil
}

func (b stubGarminBridge) ListGear(context.Context, string) (GarminBridgeGearResponse, error) {
	return GarminBridgeGearResponse{}, nil
}

func (b stubGarminBridge) ListGearActivities(_ context.Context, _ string, _ string, start, _ int) ([]GarminBridgeGearActivity, error) {
	return b.gearActivityPages[start], nil
}
