package app

import (
	"context"
	"testing"

	fit "github.com/tormoder/fit"
)

func TestGPXParser(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test">
  <trk>
    <name>Morning Run</name>
    <trkseg>
      <trkpt lat="53.349800" lon="-6.260300">
        <ele>10</ele>
        <time>2026-07-01T06:00:00Z</time>
        <extensions><gpxtpx:TrackPointExtension><gpxtpx:hr>140</gpxtpx:hr><gpxtpx:cad>82</gpxtpx:cad></gpxtpx:TrackPointExtension></extensions>
      </trkpt>
      <trkpt lat="53.350800" lon="-6.261300">
        <ele>15</ele>
        <time>2026-07-01T06:01:00Z</time>
        <extensions><gpxtpx:TrackPointExtension><gpxtpx:hr>150</gpxtpx:hr></gpxtpx:TrackPointExtension></extensions>
      </trkpt>
    </trkseg>
  </trk>
</gpx>`)

	activity, err := GPXParser{}.Parse(context.Background(), "morning.gpx", data)
	if err != nil {
		t.Fatal(err)
	}
	if activity.Name != "Morning Run" {
		t.Fatalf("name = %q", activity.Name)
	}
	if len(activity.Samples) != 2 {
		t.Fatalf("samples = %d", len(activity.Samples))
	}
	if activity.DistanceM <= 0 {
		t.Fatalf("distance = %f", activity.DistanceM)
	}
	if activity.ElevationGainM != 5 {
		t.Fatalf("elevation gain = %f", activity.ElevationGainM)
	}
	if activity.AvgHeartRate == nil || *activity.AvgHeartRate != 145 {
		t.Fatalf("avg heart rate = %#v", activity.AvgHeartRate)
	}
}

func TestTCXParser(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<TrainingCenterDatabase>
  <Activities>
    <Activity Sport="Running">
      <Id>2026-07-01T06:00:00Z</Id>
      <Lap StartTime="2026-07-01T06:00:00Z">
        <TotalTimeSeconds>60</TotalTimeSeconds>
        <DistanceMeters>200</DistanceMeters>
        <Calories>64</Calories>
        <Track>
          <Trackpoint>
            <Time>2026-07-01T06:00:00Z</Time>
            <Position><LatitudeDegrees>53.3498</LatitudeDegrees><LongitudeDegrees>-6.2603</LongitudeDegrees></Position>
            <AltitudeMeters>10</AltitudeMeters>
            <DistanceMeters>0</DistanceMeters>
            <HeartRateBpm><Value>130</Value></HeartRateBpm>
          </Trackpoint>
          <Trackpoint>
            <Time>2026-07-01T06:01:00Z</Time>
            <Position><LatitudeDegrees>53.3508</LatitudeDegrees><LongitudeDegrees>-6.2613</LongitudeDegrees></Position>
            <AltitudeMeters>13</AltitudeMeters>
            <DistanceMeters>200</DistanceMeters>
            <HeartRateBpm><Value>150</Value></HeartRateBpm>
          </Trackpoint>
        </Track>
      </Lap>
      <Lap StartTime="2026-07-01T06:01:00Z">
        <TotalTimeSeconds>30</TotalTimeSeconds>
        <DistanceMeters>50</DistanceMeters>
        <Calories>36</Calories>
      </Lap>
    </Activity>
  </Activities>
</TrainingCenterDatabase>`)

	activity, err := TCXParser{}.Parse(context.Background(), "sample.tcx", data)
	if err != nil {
		t.Fatal(err)
	}
	if activity.SportType != "Run" {
		t.Fatalf("sport = %q", activity.SportType)
	}
	if activity.DistanceM != 200 {
		t.Fatalf("distance = %f", activity.DistanceM)
	}
	if len(activity.Laps) != 2 {
		t.Fatalf("laps = %d", len(activity.Laps))
	}
	if activity.CaloriesKcal == nil || *activity.CaloriesKcal != 100 {
		t.Fatalf("calories = %#v", activity.CaloriesKcal)
	}
	if activity.MaxHeartRate == nil || *activity.MaxHeartRate != 150 {
		t.Fatalf("max heart rate = %#v", activity.MaxHeartRate)
	}
}

func TestFITSessionCaloriesKcal(t *testing.T) {
	valid := fitSessionCaloriesKcal(&fit.SessionMsg{TotalCalories: 321})
	if valid == nil || *valid != 321 {
		t.Fatalf("valid calories = %#v", valid)
	}
	if invalid := fitSessionCaloriesKcal(&fit.SessionMsg{TotalCalories: 0xFFFF}); invalid != nil {
		t.Fatalf("invalid calories = %#v", invalid)
	}
}

func TestNormalizeSport(t *testing.T) {
	cases := map[string]string{
		"Run":               "Run",
		"Treadmill_running": "Treadmill Run",
		"Lap_swimming":      "Swimming",
		"Kayaking_v2":       "Kayaking",
		"Ride":              "Cycling",
		"Walk":              "Walk",
		"Strength_training": "Strength Training",
		"Weight training":   "Strength Training",
		"WeightTraining":    "Strength Training",
		"strength training": "Strength Training",
		"virtualride":       "Cycling",
		"StrengthTraining":  "Strength Training",
		"weightlifting":     "Strength Training",
		"Biking":            "Cycling",
		"Hike":              "Hike",
	}
	for value, expect := range cases {
		if got := normalizeSport(value); got != expect {
			t.Fatalf("normalizeSport(%q) = %q, want %q", value, got, expect)
		}
	}
}

func TestNormalizeCadence(t *testing.T) {
	if got, want := normalizeCadence(88, "Run"), 176; got != want {
		t.Fatalf("normalizeCadence(88, \"Run\") = %d, want %d", got, want)
	}
	if got, want := normalizeCadence(178, "Run"), 356; got != want {
		t.Fatalf("normalizeCadence(178, \"Run\") = %d, want %d", got, want)
	}
	if got, want := normalizeCadence(90, "Treadmill Run"), 180; got != want {
		t.Fatalf("normalizeCadence(90, \"Treadmill Run\") = %d, want %d", got, want)
	}
	if got, want := normalizeCadence(88, "Cycling"), 88; got != want {
		t.Fatalf("normalizeCadence(88, \"Cycling\") = %d, want %d", got, want)
	}
}

func TestIsProviderSyncedSource(t *testing.T) {
	if !isProviderSyncedSource("garmin", "123", true) {
		t.Fatal("garmin source should be provider synced")
	}
	if isProviderSyncedSource("file", "file:hash", false) {
		t.Fatal("file source should be manual")
	}
	if isProviderSyncedSource("garmin", "123", false) {
		t.Fatal("provider source with a source file should not be treated as synced")
	}
	if isProviderSyncedSource("garmin", "", true) {
		t.Fatal("empty source id should not be treated as synced")
	}
}
