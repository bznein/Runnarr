package app

import "testing"

func TestEnrichActivityClimbPerformanceUsesCompleteSamplesAndOverlappingLaps(t *testing.T) {
	climb := ActivityClimb{
		StartSampleIndex: 0,
		EndSampleIndex:   2,
		StartDistanceM:   0,
		EndDistanceM:     1000,
	}
	samples := []ActivitySample{
		{Index: 0, ElapsedS: intClimbPtr(0), DistanceM: floatClimbPtr(0), SpeedMPS: floatClimbPtr(10.0 / 3)},
		{Index: 1, ElapsedS: intClimbPtr(150), DistanceM: floatClimbPtr(500), SpeedMPS: floatClimbPtr(10.0 / 3)},
		{Index: 2, ElapsedS: intClimbPtr(300), DistanceM: floatClimbPtr(1000), SpeedMPS: floatClimbPtr(10.0 / 3)},
	}
	laps := []ActivityLap{
		{Index: 0, DistanceM: 500, AvgGradeAdjustedPaceSPKM: floatClimbPtr(320)},
		{Index: 1, DistanceM: 500, AvgGradeAdjustedPaceSPKM: floatClimbPtr(280)},
	}

	climbs := enrichActivityClimbPerformance([]ActivityClimb{climb}, samples, laps)
	if len(climbs) != 1 || climbs[0].PaceSPKM == nil || *climbs[0].PaceSPKM < 299.9 || *climbs[0].PaceSPKM > 300.1 {
		t.Fatalf("pace = %#v, want 300 s/km", climbs[0].PaceSPKM)
	}
	if climbs[0].GapSPKM == nil || *climbs[0].GapSPKM < 299.9 || *climbs[0].GapSPKM > 300.1 {
		t.Fatalf("GAP = %#v, want 300 s/km", climbs[0].GapSPKM)
	}
}

func TestEnrichActivityClimbPerformanceOmitsMissingValues(t *testing.T) {
	climb := ActivityClimb{StartSampleIndex: 0, EndSampleIndex: 1, StartDistanceM: 0, EndDistanceM: 500}
	climbs := enrichActivityClimbPerformance([]ActivityClimb{climb}, []ActivitySample{{Index: 0}, {Index: 1}}, nil)
	if climbs[0].PaceSPKM != nil || climbs[0].GapSPKM != nil {
		t.Fatalf("performance = %#v, want no values", climbs[0])
	}
}

func intClimbPtr(value int) *int {
	return &value
}

func floatClimbPtr(value float64) *float64 {
	return &value
}
