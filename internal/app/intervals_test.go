package app

import (
	"testing"
	"time"
)

func TestLapsFromIntervals(t *testing.T) {
	start := time.Date(2026, 7, 15, 6, 0, 0, 0, time.UTC)
	laps := lapsFromIntervals(start, []intervalsInterval{
		{StartIndex: 2, StartTime: 600, Distance: 900, MovingTime: 240},
		{StartIndex: 1, StartTime: 0, Distance: 1000, ElapsedTime: 300},
		{StartIndex: 3},
	})

	if len(laps) != 2 {
		t.Fatalf("laps = %d", len(laps))
	}
	if laps[0].DistanceM != 1000 || laps[0].ElapsedTimeS != 300 {
		t.Fatalf("first lap = %#v", laps[0])
	}
	if laps[1].DistanceM != 900 || laps[1].ElapsedTimeS != 240 {
		t.Fatalf("second lap = %#v", laps[1])
	}
	if laps[1].StartTime == nil || !laps[1].StartTime.Equal(start.Add(10*time.Minute)) {
		t.Fatalf("second lap start = %#v", laps[1].StartTime)
	}
}
