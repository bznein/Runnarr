package app

import (
	"math"
	"testing"
)

func TestCalculateToolsVDOT(t *testing.T) {
	result, err := calculateToolsVDOT(toolsVDOTRequest{
		DistanceKm: "10",
		Time:       "40:00",
	})
	if err != nil {
		t.Fatalf("calculateToolsVDOT() error = %v", err)
	}

	if !approx(result.Vdot, 51.94, 0.02) {
		t.Fatalf("vdot = %f, want approx 51.94", result.Vdot)
	}

	if len(result.Equivalents) != len(toolsVDOTEquivalentRaces) {
		t.Fatalf("expected %d equivalents, got %d", len(toolsVDOTEquivalentRaces), len(result.Equivalents))
	}
	if result.Equivalents[0].Race != "5K" {
		t.Fatalf("first equivalent = %s, want 5K", result.Equivalents[0].Race)
	}
	if !approx(result.Equivalents[0].TimeSeconds, float64(19*60+18), 1) {
		t.Fatalf("5k time = %f, want 19:18", result.Equivalents[0].TimeSeconds)
	}
	if !approx(result.Equivalents[3].TimeSeconds, float64(3*3600+4*60+38), 5) {
		t.Fatalf("marathon time = %f, want 3:04:38", result.Equivalents[3].TimeSeconds)
	}
}

func TestCalculateToolsVDOTRequiresDistanceAndTime(t *testing.T) {
	if _, err := calculateToolsVDOT(toolsVDOTRequest{DistanceKm: "10", Time: ""}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := calculateToolsVDOT(toolsVDOTRequest{DistanceKm: "", Time: "40:00"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestCalculateToolsVDOTRejectsBadTime(t *testing.T) {
	if _, err := calculateToolsVDOT(toolsVDOTRequest{DistanceKm: "10", Time: "bad"}); err == nil {
		t.Fatal("expected error")
	}
}

func approx(actual float64, expected float64, tolerance float64) bool {
	return math.Abs(actual-expected) <= tolerance
}
