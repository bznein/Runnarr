package app

import "testing"

func TestCalculateToolsPaceDistanceTimeComputesPace(t *testing.T) {
	result, err := calculateToolsPace(toolsPaceRequest{
		DistanceKm: "10",
		Time:       "45:00",
	})
	if err != nil {
		t.Fatalf("calculateToolsPace() error = %v", err)
	}
	if result.Computed != "pace" {
		t.Fatalf("computed = %s, want pace", result.Computed)
	}
	if result.PaceSecondsPerKm != 270 {
		t.Fatalf("pace = %f, want 270", result.PaceSecondsPerKm)
	}
	if result.PaceLabel != "4:30 /km" {
		t.Fatalf("paceLabel = %q, want 4:30 /km", result.PaceLabel)
	}
}

func TestCalculateToolsPaceDistancePaceComputesTime(t *testing.T) {
	result, err := calculateToolsPace(toolsPaceRequest{
		DistanceKm: "10",
		Pace:       "5:00",
	})
	if err != nil {
		t.Fatalf("calculateToolsPace() error = %v", err)
	}
	if result.Computed != "time" {
		t.Fatalf("computed = %s, want time", result.Computed)
	}
	if result.TimeSeconds != 3000 {
		t.Fatalf("time = %f, want 3000", result.TimeSeconds)
	}
	if result.TimeLabel != "0:50:00" {
		t.Fatalf("timeLabel = %q, want 0:50:00", result.TimeLabel)
	}
}

func TestCalculateToolsPaceTimePaceComputesDistance(t *testing.T) {
	result, err := calculateToolsPace(toolsPaceRequest{
		Time: "1:00:00",
		Pace: "5:00",
	})
	if err != nil {
		t.Fatalf("calculateToolsPace() error = %v", err)
	}
	if result.Computed != "distance" {
		t.Fatalf("computed = %s, want distance", result.Computed)
	}
	if result.DistanceKm != 12 {
		t.Fatalf("distance = %f, want 12", result.DistanceKm)
	}
	if result.DistanceLabel != "12.000 km" {
		t.Fatalf("distanceLabel = %q, want 12.000 km", result.DistanceLabel)
	}
}

func TestCalculateToolsPaceRequiresExactlyTwoValues(t *testing.T) {
	_, err := calculateToolsPace(toolsPaceRequest{DistanceKm: "10", Time: "40:00", Pace: "4:30"})
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = calculateToolsPace(toolsPaceRequest{DistanceKm: "10"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseToolsPaceDuration(t *testing.T) {
	seconds, ok, err := parseToolsPaceDuration("1:20:30")
	if err != nil {
		t.Fatalf("parseToolsPaceDuration() error = %v", err)
	}
	if !ok {
		t.Fatal("expected value")
	}
	if seconds != 4830 {
		t.Fatalf("seconds = %f, want 4830", seconds)
	}

	seconds, ok, err = parseToolsPaceDuration("25:40")
	if err != nil {
		t.Fatalf("parseToolsPaceDuration() error = %v", err)
	}
	if !ok {
		t.Fatal("expected value")
	}
	if seconds != 1540 {
		t.Fatalf("seconds = %f, want 1540", seconds)
	}
}

func TestParseToolsPaceDurationRejectsInvalidFormat(t *testing.T) {
	if _, _, err := parseToolsPaceDuration("1:2:3:4"); err == nil {
		t.Fatal("expected error")
	}
	if _, _, err := parseToolsPacePace("4"); err == nil {
		t.Fatal("expected error")
	}
}
