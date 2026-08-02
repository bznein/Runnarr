package app

import (
	"fmt"
	"math"
	"testing"
)

func TestBoundedActivitySeriesPreservesEndpointsAndOrder(t *testing.T) {
	samples := make([]ActivitySample, 10)
	for index := range samples {
		samples[index].Index = index
		samples[index].ElapsedS = seriesTestInt(index)
		samples[index].ElevationM = seriesTestFloat(float64(index))
	}

	series := boundedActivitySeries(samples, 5)
	if !series.Sampled || series.TotalSamples != len(samples) || len(series.Samples) != 5 {
		t.Fatalf("unexpected series metadata: %#v", series)
	}
	if series.Samples[0].Index != 0 || series.Samples[len(series.Samples)-1].Index != 9 {
		t.Fatalf("series endpoints = %d, %d", series.Samples[0].Index, series.Samples[len(series.Samples)-1].Index)
	}
	for index := 1; index < len(series.Samples); index++ {
		if series.Samples[index-1].Index >= series.Samples[index].Index {
			t.Fatalf("series is not ordered: %#v", series.Samples)
		}
	}
	if len(series.Points) != len(series.Samples) {
		t.Fatalf("points length = %d, samples length = %d", len(series.Points), len(series.Samples))
	}
	if series.Points[1].RawElevationM == nil || math.Abs(*series.Points[1].RawElevationM-1.5) > 0.000001 {
		t.Fatalf("first elapsed-time bucket did not aggregate every sample: %#v", series.Points[1])
	}
}

func TestNormalizeActivitySeriesPoints(t *testing.T) {
	if got := normalizeActivitySeriesPoints(0); got != defaultActivitySeriesPoints {
		t.Fatalf("default points = %d", got)
	}
	if got := normalizeActivitySeriesPoints(1); got != minActivitySeriesPoints {
		t.Fatalf("minimum points = %d", got)
	}
	if got := normalizeActivitySeriesPoints(maxActivitySeriesPoints + 1); got != maxActivitySeriesPoints {
		t.Fatalf("maximum points = %d", got)
	}
}

func TestActivitySeriesPointsAreChartReady(t *testing.T) {
	distance := 1500.0
	speed := 3.0
	elevation := 100.0
	samples := []ActivitySample{{
		Index:      7,
		DistanceM:  &distance,
		SpeedMPS:   &speed,
		ElevationM: &elevation,
	}}

	points := activitySeriesPoints(samples)
	if len(points) != 1 {
		t.Fatalf("points length = %d", len(points))
	}
	point := points[0]
	if point.Index != 7 || point.Label != "1.5 km" {
		t.Fatalf("unexpected point identity: %#v", point)
	}
	if point.PaceSPKM == nil || point.RawPaceSPKM == nil || math.Abs(*point.RawPaceSPKM-(1000.0/3.0)) > 0.000001 {
		raw := "nil"
		if point.RawPaceSPKM != nil {
			raw = fmt.Sprintf("%f", *point.RawPaceSPKM)
		}
		pace := "nil"
		if point.PaceSPKM != nil {
			pace = fmt.Sprintf("%f", *point.PaceSPKM)
		}
		t.Fatalf("unexpected pace fields: raw=%s pace=%s point=%#v", raw, pace, point)
	}
	if point.RawElevationM == nil || point.ElevationM == nil || *point.RawElevationM != elevation || *point.ElevationM != elevation {
		t.Fatalf("unexpected elevation fields: %#v", point)
	}
}

func TestActivitySeriesPaceTrendRemovesSpikeAndPreservesSustainedEffort(t *testing.T) {
	samples := make([]ActivitySample, 61)
	for index := range samples {
		pace := 300.0
		if index >= 16 && index <= 45 {
			pace = 240
		}
		if index == 8 {
			pace = 900
		}
		samples[index] = ActivitySample{
			Index:    index,
			ElapsedS: seriesTestInt(index),
			SpeedMPS: seriesTestFloat(1000 / pace),
		}
	}

	points := activitySeriesPoints(samples)
	if got := *points[8].RawPaceSPKM; math.Abs(got-900) > 0.000001 {
		t.Fatalf("recorded spike pace = %f", got)
	}
	if got := *points[8].PaceSPKM; math.Abs(got-300) > 0.000001 {
		t.Fatalf("spike trend pace = %f", got)
	}
	if got := *points[30].PaceSPKM; math.Abs(got-240) > 0.000001 {
		t.Fatalf("sustained effort trend pace = %f", got)
	}
}

func TestActivitySeriesSensorTrendsRemoveSpikesAndPreserveFiveSecondEffort(t *testing.T) {
	samples := make([]ActivitySample, 25)
	for index := range samples {
		heartRate := 150
		if index >= 10 && index <= 14 {
			heartRate = 180
		}
		power := 240
		if index == 6 {
			power = 900
		}
		samples[index] = ActivitySample{
			Index:     index,
			ElapsedS:  seriesTestInt(index),
			SpeedMPS:  seriesTestFloat(3),
			HeartRate: seriesTestInt(heartRate),
			Cadence:   seriesTestInt(170 + index%2),
			Power:     seriesTestInt(power),
		}
	}

	points := activitySeriesPoints(samples)
	if got := *points[12].HeartRate; got != 180 {
		t.Fatalf("five-second heart-rate effort trend = %d", got)
	}
	if got := *points[6].RawPower; got != 900 {
		t.Fatalf("recorded power spike = %d", got)
	}
	if got := *points[6].Power; got != 240 {
		t.Fatalf("power spike trend = %d", got)
	}
}

func TestActivitySeriesTrendsBreakAtStopsGapsAndMissingValues(t *testing.T) {
	samples := make([]ActivitySample, 8)
	for index := range samples {
		elapsed := index
		if index >= 4 {
			elapsed += 40
		}
		samples[index] = ActivitySample{
			Index:     index,
			ElapsedS:  seriesTestInt(elapsed),
			SpeedMPS:  seriesTestFloat(3),
			HeartRate: seriesTestInt(150 + index),
			Power:     seriesTestInt(220),
		}
	}
	*samples[2].SpeedMPS = 0
	samples[6].HeartRate = nil

	points := activitySeriesPoints(samples)
	if points[2].PaceSPKM != nil || points[2].HeartRate != nil || points[2].Power != nil {
		t.Fatalf("stop point retained trend values: %#v", points[2])
	}
	if points[2].RawHeartRate == nil {
		t.Fatalf("stop point lost its recorded sensor value: %#v", points[2])
	}
	if points[4].PaceSPKM != nil || points[4].HeartRate != nil {
		t.Fatalf("gap boundary retained trend values: %#v", points[4])
	}
	if points[6].HeartRate != nil || points[6].RawHeartRate != nil {
		t.Fatalf("missing heart rate was bridged: %#v", points[6])
	}
	if points[6].Power == nil {
		t.Fatalf("metric-specific dropout removed an available power trend: %#v", points[6])
	}
}

func TestActivitySeriesElevationUsesDistanceAndTimeWindows(t *testing.T) {
	distanceSamples := make([]ActivitySample, 7)
	timeSamples := make([]ActivitySample, 7)
	for index := range distanceSamples {
		elevation := 100.0
		if index == 3 {
			elevation = 170
		}
		distanceSamples[index] = ActivitySample{
			Index:      index,
			ElapsedS:   seriesTestInt(index * 10),
			DistanceM:  seriesTestFloat(float64(index * 50)),
			ElevationM: seriesTestFloat(elevation),
		}
		timeSamples[index] = ActivitySample{
			Index:      index,
			ElapsedS:   seriesTestInt(index),
			ElevationM: seriesTestFloat(elevation),
		}
	}

	distancePoints := activitySeriesPoints(distanceSamples)
	if got := *distancePoints[3].ElevationM; math.Abs(got-110) > 0.000001 {
		t.Fatalf("distance-smoothed elevation = %f", got)
	}
	timePoints := activitySeriesPoints(timeSamples)
	if got := *timePoints[3].ElevationM; math.Abs(got-110) > 0.000001 {
		t.Fatalf("time-smoothed elevation = %f", got)
	}
}

func TestBoundedActivitySeriesPreservesDropoutsInsideBuckets(t *testing.T) {
	samples := make([]ActivitySample, 20)
	for index := range samples {
		samples[index] = ActivitySample{
			Index:     index,
			ElapsedS:  seriesTestInt(index),
			SpeedMPS:  seriesTestFloat(3),
			HeartRate: seriesTestInt(150),
		}
	}
	samples[3].HeartRate = nil

	series := boundedActivitySeries(samples, 5)
	foundDropout := false
	for _, point := range series.Points {
		if point.HeartRate == nil {
			foundDropout = true
			break
		}
	}
	if !foundDropout {
		t.Fatalf("bounded points bridged a recorded dropout: %#v", series.Points)
	}
}

func seriesTestFloat(value float64) *float64 {
	return &value
}

func seriesTestInt(value int) *int {
	return &value
}
