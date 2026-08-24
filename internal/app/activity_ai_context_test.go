package app

import (
	"testing"
	"time"
)

func TestActivityAIContextWindowUsesCalendarDatesAcrossDST(t *testing.T) {
	activityStart := time.Date(2026, 4, 2, 6, 30, 0, 0, time.UTC)
	windowStart, windowEnd, err := activityAIContextWindow(activityStart, "Europe/Dublin")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := windowStart.Format("2006-01-02"), "2026-03-27"; got != want {
		t.Fatalf("window start = %q, want %q", got, want)
	}
	if got, want := windowEnd.Format("2006-01-02"), "2026-04-02"; got != want {
		t.Fatalf("window end = %q, want %q", got, want)
	}
	if got := windowEnd.Sub(windowStart); got == 6*24*time.Hour {
		t.Fatalf("window used a fixed-hour duration across DST: %v", got)
	}
}

func TestActivityAIContextWindowRejectsInvalidTimezone(t *testing.T) {
	if _, _, err := activityAIContextWindow(time.Now(), "Not/AZone"); err == nil {
		t.Fatal("expected invalid timezone error")
	}
}
