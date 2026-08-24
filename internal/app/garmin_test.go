package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http/httptest"
	"os"
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

func TestGarminSyncOldestDefaultsToToday(t *testing.T) {
	now := time.Date(2026, 7, 22, 15, 30, 0, 0, time.UTC)
	if got := garminSyncOldest(GarminSyncOptions{}, now); !got.Equal(time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("default oldest = %s, want start of today", got)
	}

	if got := garminSyncOldest(GarminSyncOptions{AllData: true}, now); !got.Equal(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("all-data oldest = %s, want epoch", got)
	}
}

func TestDecodeGarminSyncOptionsSupportsAllData(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/providers/garmin/sync", strings.NewReader(`{"allData":true,"oldest":"2026-07-22"}`))
	options, err := decodeGarminSyncOptions(request)
	if err != nil {
		t.Fatal(err)
	}
	if !options.AllData || !options.Oldest.IsZero() {
		t.Fatalf("options = %#v, want explicit all-data with no date", options)
	}
}

func TestApplyGarminLapMetadata(t *testing.T) {
	existingGap := 300.0
	activity := ImportedActivity{
		Laps: []ActivityLap{{Index: 0}, {Index: 1, AvgGradeAdjustedPaceSPKM: &existingGap}, {Index: 2}},
	}

	applyGarminLapMetadata(&activity.Laps, []GarminBridgeLap{
		{Index: 0, AvgGradeAdjustedSpeedMPS: floatPtr(4)},
		{Index: 1},
		{Index: 2, AvgGradeAdjustedSpeedMPS: floatPtr(0)},
	})

	if activity.Laps[0].AvgGradeAdjustedPaceSPKM == nil || *activity.Laps[0].AvgGradeAdjustedPaceSPKM != 250 {
		t.Fatalf("lap 0 GAP = %#v, want 250", activity.Laps[0].AvgGradeAdjustedPaceSPKM)
	}
	if activity.Laps[1].AvgGradeAdjustedPaceSPKM == nil || *activity.Laps[1].AvgGradeAdjustedPaceSPKM != existingGap {
		t.Fatalf("lap 1 GAP = %#v, want existing value", activity.Laps[1].AvgGradeAdjustedPaceSPKM)
	}
	if activity.Laps[2].AvgGradeAdjustedPaceSPKM != nil {
		t.Fatalf("lap 2 GAP = %#v, want nil", activity.Laps[2].AvgGradeAdjustedPaceSPKM)
	}
}

func TestApplyGarminWorkoutMetadata(t *testing.T) {
	activity := ImportedActivity{
		Laps: []ActivityLap{{Index: 0}, {Index: 1}, {Index: 2}},
	}
	stepIndex := 1
	repeatIndex := 2
	workout := &ActivityWorkout{Provider: garminProvider, ProviderWorkoutID: "workout-1", Name: "5x7"}
	response := GarminBridgeActivityWorkout{
		Available: true,
		Workout:   workout,
		Intervals: []ActivityInterval{{Index: 0, Category: "active", WorkoutStepIndex: &stepIndex, WorkoutRepeatIndex: &repeatIndex, LapIndexes: []int{0, 1}}},
		Laps:      []GarminBridgeLap{{Index: 0, IntensityType: "active", AvgHeartRate: floatPtr(160), Raw: map[string]any{"lapIndex": 1}}},
		Raw:       map[string]any{"typedSplits": map[string]any{"count": 1}},
	}

	applyGarminWorkoutMetadata(&activity, response)

	if !activity.ReplaceWorkoutMetadata {
		t.Fatal("workout metadata should be marked for replacement")
	}
	if activity.Workout != workout || len(activity.Intervals) != 1 {
		t.Fatalf("workout metadata = %#v, intervals = %#v", activity.Workout, activity.Intervals)
	}
	if activity.Laps[0].IntensityType != "active" || activity.Laps[0].WorkoutRepeatIndex == nil || *activity.Laps[0].WorkoutRepeatIndex != 2 {
		t.Fatalf("lap metadata = %#v", activity.Laps[0])
	}
	if activity.Laps[1].WorkoutStepIndex == nil || *activity.Laps[1].WorkoutStepIndex != 1 {
		t.Fatalf("interval mapping did not reach child lap: %#v", activity.Laps[1])
	}
	if activity.Raw["garmin_workout"] == nil {
		t.Fatalf("raw workout payload missing: %#v", activity.Raw)
	}
}

func TestApplyGarminWorkoutMetadataKeepsWeatherWhenWorkoutDataIsUnavailable(t *testing.T) {
	temperatureF := 68.0
	apparentTemperatureF := 66.2
	dewPointF := 53.6
	humidity := 55.0
	windMPH := 7.0
	windGustMPH := 11.0
	directionDeg := 225.0
	latitude := 53.35
	longitude := -6.26
	observedAt := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	activity := ImportedActivity{}

	applyGarminWorkoutMetadata(&activity, GarminBridgeActivityWorkout{
		Available: false,
		Raw:       map[string]any{"weather": map[string]any{"latitude": latitude, "longitude": longitude, "temp": temperatureF}},
		Weather: &GarminBridgeWeather{
			ObservedAt:           &observedAt,
			Condition:            " Partly cloudy ",
			TemperatureF:         &temperatureF,
			ApparentTemperatureF: &apparentTemperatureF,
			DewPointF:            &dewPointF,
			RelativeHumidityPct:  &humidity,
			WindSpeedMPH:         &windMPH,
			WindGustMPH:          &windGustMPH,
			WindDirectionDeg:     &directionDeg,
			WindDirection:        " sw ",
			Latitude:             &latitude,
			Longitude:            &longitude,
			StationID:            " station-1 ",
			StationName:          " Dublin ",
			StationTimezone:      " Europe/Dublin ",
			Raw:                  map[string]any{"latitude": latitude, "privateStation": "station-1"},
		},
	})

	if activity.ReplaceWorkoutMetadata {
		t.Fatal("unavailable workout data should not replace workout metadata")
	}
	if activity.Raw["garmin_workout"] == nil {
		t.Fatal("raw Garmin weather payload should be retained when workout metadata is unavailable")
	}
	if activity.Weather == nil {
		t.Fatal("weather was not applied")
	}
	if got := *activity.Weather.TemperatureC; math.Abs(got-20) > 0.0001 {
		t.Fatalf("temperature = %v, want 20 C", got)
	}
	if got := *activity.Weather.WindSpeedKPH; math.Abs(got-11.265408) > 0.0001 {
		t.Fatalf("wind speed = %v, want 11.265408 kph", got)
	}
	if activity.Weather.Condition != "Partly cloudy" || activity.Weather.WindDirection != "SW" {
		t.Fatalf("weather labels = %#v", activity.Weather)
	}
	encoded, err := json.Marshal(Activity{Weather: activity.Weather})
	if err != nil {
		t.Fatal(err)
	}
	publicJSON := string(encoded)
	if !strings.Contains(publicJSON, `"provider":"garmin"`) {
		t.Fatalf("public activity weather omitted provider attribution: %s", publicJSON)
	}
	for _, privateField := range []string{"dewPoint", "windGust", "latitude", "longitude", "station", "raw", "privateStation"} {
		if strings.Contains(publicJSON, privateField) {
			t.Fatalf("public activity weather leaked %q: %s", privateField, publicJSON)
		}
	}
}

func TestActivityWeatherLocationUsesGarminCoordinatesBeforeSamples(t *testing.T) {
	garminLatitude, garminLongitude := 53.1, -7.2
	sampleLatitude, sampleLongitude := 51.5, -0.1
	activity := ImportedActivity{
		Weather: &ActivityWeather{Latitude: &garminLatitude, Longitude: &garminLongitude},
		Samples: []ActivitySample{{Latitude: &sampleLatitude, Longitude: &sampleLongitude}},
	}

	latitude, longitude, ok := activityWeatherLocation(activity)
	if !ok || latitude != garminLatitude || longitude != garminLongitude {
		t.Fatalf("location = (%v, %v, %v), want Garmin weather coordinates", latitude, longitude, ok)
	}
	if activityWeatherHasDisplayData(activity.Weather) {
		t.Fatal("coordinate-only Garmin weather should allow fallback")
	}
}

func TestActivityWeatherReferenceUsesRouteMidpointTimeAndCoordinates(t *testing.T) {
	start := time.Date(2026, 8, 24, 6, 24, 0, 0, time.UTC)
	garminLatitude, garminLongitude := 53.1, -7.2
	startLatitude, startLongitude := 52.945, -7.218
	midpointLatitude, midpointLongitude := 52.910, -7.296
	finishLatitude, finishLongitude := 52.945, -7.218
	startElapsed, midpointElapsed, finishElapsed := 0, 2700, 5400
	startSampleTime := start
	midpointSampleTime := start.Add(45 * time.Minute)
	finishSampleTime := start.Add(90 * time.Minute)
	activity := ImportedActivity{
		StartTime:    start,
		ElapsedTimeS: 5400,
		Weather:      &ActivityWeather{Latitude: &garminLatitude, Longitude: &garminLongitude},
		Samples: []ActivitySample{
			{Timestamp: &startSampleTime, ElapsedS: &startElapsed, Latitude: &startLatitude, Longitude: &startLongitude},
			{Timestamp: &midpointSampleTime, ElapsedS: &midpointElapsed, Latitude: &midpointLatitude, Longitude: &midpointLongitude},
			{Timestamp: &finishSampleTime, ElapsedS: &finishElapsed, Latitude: &finishLatitude, Longitude: &finishLongitude},
		},
	}

	referenceTime, latitude, longitude, ok := activityWeatherReference(activity)
	if !ok || !referenceTime.Equal(midpointSampleTime) || latitude != midpointLatitude || longitude != midpointLongitude {
		t.Fatalf("reference = (%v, %v, %v, %v), want route midpoint", referenceTime, latitude, longitude, ok)
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

func TestPythonGarminBridgeTreatsMissingWorkoutDeleteAsSuccess(t *testing.T) {
	script := t.TempDir() + "/not-found-bridge.sh"
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' '{\"error\":\"API call client error (404): API Error 404\",\"code\":\"NotFoundException\"}'\nexit 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge := PythonGarminBridge{Python: "sh", Script: script}

	if _, err := bridge.GetWorkout(context.Background(), "tokens", "missing"); !errors.Is(err, ErrGarminNotFound) {
		t.Fatalf("GetWorkout error = %v, want ErrGarminNotFound", err)
	}
	if err := bridge.DeleteWorkout(context.Background(), "tokens", "missing"); err != nil {
		t.Fatalf("DeleteWorkout error = %v, want nil", err)
	}
}

func TestGarminBridgeNotFoundClassificationPreservesOtherErrors(t *testing.T) {
	for _, test := range []struct {
		code, message string
	}{
		{code: "NotFoundException", message: "missing"},
		{code: "GarminConnectConnectionError", message: "API call client error (404): API Error 404"},
	} {
		err := &garminBridgeError{Code: test.code, Message: test.message}
		if !errors.Is(err, ErrGarminNotFound) {
			t.Fatalf("error %#v was not classified as ErrGarminNotFound", err)
		}
	}
	other := &garminBridgeError{Code: "RuntimeError", Message: "provider unavailable"}
	if errors.Is(other, ErrGarminNotFound) {
		t.Fatalf("error %#v was classified as ErrGarminNotFound", other)
	}
}

type stubGarminBridge struct {
	gearActivityPages map[int][]GarminBridgeGearActivity
	uploadedCourse    GarminBridgeCourse
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

func (b stubGarminBridge) GetActivityWorkout(context.Context, string, string) (GarminBridgeActivityWorkout, error) {
	return GarminBridgeActivityWorkout{}, nil
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

func (b stubGarminBridge) ListWorkouts(context.Context, string, int, int) ([]GarminBridgeWorkout, error) {
	return nil, nil
}

func (b stubGarminBridge) GetWorkout(context.Context, string, string) (GarminBridgeWorkout, error) {
	return GarminBridgeWorkout{}, nil
}

func (b stubGarminBridge) UploadWorkout(context.Context, string, map[string]any) (GarminBridgeWorkout, error) {
	return GarminBridgeWorkout{}, nil
}

func (b stubGarminBridge) UploadCourse(context.Context, string, string, []byte, string, CourseSport, string) (GarminBridgeCourse, error) {
	return b.uploadedCourse, nil
}

func (b stubGarminBridge) GetCourse(context.Context, string, string) (GarminBridgeCourse, error) {
	return b.uploadedCourse, nil
}

func (b stubGarminBridge) DeleteWorkout(context.Context, string, string) error {
	return nil
}

func (b stubGarminBridge) ListScheduledWorkouts(context.Context, string, int, int) ([]GarminBridgeScheduledWorkout, error) {
	return nil, nil
}

func (b stubGarminBridge) GetScheduledWorkout(context.Context, string, string) (GarminBridgeScheduledWorkout, error) {
	return GarminBridgeScheduledWorkout{}, nil
}

func (b stubGarminBridge) ScheduleWorkout(context.Context, string, string, string) (GarminBridgeScheduledWorkout, error) {
	return GarminBridgeScheduledWorkout{}, nil
}

func (b stubGarminBridge) UnscheduleWorkout(context.Context, string, string) error {
	return nil
}
