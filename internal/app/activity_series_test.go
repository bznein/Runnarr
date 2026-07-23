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
}
