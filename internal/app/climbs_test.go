package app

import "testing"

func TestDetectActivityClimbsEasyClimb(t *testing.T) {
	samples := climbSamples([][2]float64{
		{0, 100},
		{100, 103},
		{220, 106.5},
		{360, 111},
		{500, 115},
		{620, 118},
	})

	climbs := detectActivityClimbs(samples)
	if len(climbs) != 1 {
		t.Fatalf("climbs = %d, want 1", len(climbs))
	}
	climb := climbs[0]
	if climb.Difficulty != "Easy" {
		t.Fatalf("difficulty = %q, want Easy", climb.Difficulty)
	}
	if climb.DistanceM < 610 || climb.DistanceM > 630 {
		t.Fatalf("distance = %.1f, want about 620", climb.DistanceM)
	}
	if climb.ElevationGainM < 17 || climb.ElevationGainM > 19 {
		t.Fatalf("gain = %.1f, want about 18", climb.ElevationGainM)
	}
	if climb.AvgGradePct < 2.8 || climb.AvgGradePct > 3.1 {
		t.Fatalf("grade = %.2f, want about 3%%", climb.AvgGradePct)
	}
}

func TestDetectActivityClimbsIgnoresFlatNoise(t *testing.T) {
	samples := climbSamples([][2]float64{
		{0, 100},
		{150, 100.6},
		{300, 99.8},
		{450, 100.4},
		{600, 99.7},
		{750, 100.2},
		{900, 100},
	})

	climbs := detectActivityClimbs(samples)
	if len(climbs) != 0 {
		t.Fatalf("climbs = %d, want 0", len(climbs))
	}
}

func TestDetectActivityClimbsMergesShortDips(t *testing.T) {
	samples := climbSamples([][2]float64{
		{0, 100},
		{150, 108},
		{260, 106},
		{360, 116},
		{520, 124},
	})

	climbs := detectActivityClimbs(samples)
	if len(climbs) != 1 {
		t.Fatalf("climbs = %d, want 1", len(climbs))
	}
	if climbs[0].EndDistanceM < 500 {
		t.Fatalf("end distance = %.1f, want merged climb to continue after dip", climbs[0].EndDistanceM)
	}
}

func TestDetectActivityClimbsSplitsSustainedDescents(t *testing.T) {
	samples := climbSamples([][2]float64{
		{0, 100},
		{150, 110},
		{320, 119},
		{500, 109},
		{650, 109},
		{820, 119},
		{980, 128},
	})

	climbs := detectActivityClimbs(samples)
	if len(climbs) != 2 {
		t.Fatalf("climbs = %d, want 2", len(climbs))
	}
	if climbs[0].EndSampleIndex >= climbs[1].StartSampleIndex {
		t.Fatalf("climbs overlap: first end %d, second start %d", climbs[0].EndSampleIndex, climbs[1].StartSampleIndex)
	}
}

func TestDetectActivityClimbsRequiresUsableSamples(t *testing.T) {
	samples := []ActivitySample{
		{Index: 0, ElevationM: floatPtr(100)},
		{Index: 1, DistanceM: floatPtr(100)},
	}

	climbs := detectActivityClimbs(samples)
	if len(climbs) != 0 {
		t.Fatalf("climbs = %d, want 0", len(climbs))
	}
}

func climbSamples(points [][2]float64) []ActivitySample {
	samples := make([]ActivitySample, 0, len(points))
	for index, point := range points {
		samples = append(samples, ActivitySample{
			Index:      index,
			DistanceM:  floatPtr(point[0]),
			ElevationM: floatPtr(point[1]),
		})
	}
	return samples
}

func floatPtr(value float64) *float64 {
	return &value
}
