package app

import (
	"fmt"
	"sort"
)

// defaultActivitySeriesPoints keeps mobile and browser detail views bounded
// without making ordinary activities look sparse.
const (
	defaultActivitySeriesPoints = 1200
	minActivitySeriesPoints     = 2
	maxActivitySeriesPoints     = 5000
)

func normalizeActivitySeriesPoints(value int) int {
	if value <= 0 {
		return defaultActivitySeriesPoints
	}
	if value < minActivitySeriesPoints {
		return minActivitySeriesPoints
	}
	if value > maxActivitySeriesPoints {
		return maxActivitySeriesPoints
	}
	return value
}

func boundedActivitySeries(samples []ActivitySample, maxPoints int) ActivitySeries {
	maxPoints = normalizeActivitySeriesPoints(maxPoints)
	if len(samples) <= maxPoints {
		return ActivitySeries{
			Samples:      samples,
			Points:       activitySeriesPoints(samples),
			TotalSamples: len(samples),
			Sampled:      false,
		}
	}

	selected := make([]ActivitySample, 0, maxPoints)
	lastIndex := -1
	for point := 0; point < maxPoints; point++ {
		position := point * (len(samples) - 1) / (maxPoints - 1)
		if position == lastIndex {
			continue
		}
		selected = append(selected, samples[position])
		lastIndex = position
	}
	return ActivitySeries{
		Samples:      selected,
		Points:       activitySeriesPoints(selected),
		TotalSamples: len(samples),
		Sampled:      true,
	}
}

func activitySeriesPoints(samples []ActivitySample) []ActivitySeriesPoint {
	paceScale := paceScaleForSeries(samples)
	points := make([]ActivitySeriesPoint, 0, len(samples))
	for index, sample := range samples {
		var rawPace *float64
		if sample.SpeedMPS != nil && *sample.SpeedMPS > 0 {
			pace := 1000 / *sample.SpeedMPS
			rawPace = &pace
		}
		point := ActivitySeriesPoint{
			Index:       sample.Index,
			Label:       seriesPointLabel(sample, index),
			DistanceM:   sample.DistanceM,
			Latitude:    sample.Latitude,
			Longitude:   sample.Longitude,
			ElevationM:  sample.ElevationM,
			HeartRate:   sample.HeartRate,
			RawPaceSPKM: rawPace,
			Power:       sample.Power,
			Cadence:     sample.Cadence,
		}
		if rawPace != nil {
			pace := clampSeriesPace(*rawPace, paceScale)
			point.PaceSPKM = &pace
		}
		points = append(points, point)
	}
	return smoothSeriesElevation(points)
}

type seriesPaceScale struct {
	min float64
	max float64
}

func paceScaleForSeries(samples []ActivitySample) *seriesPaceScale {
	values := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if sample.SpeedMPS != nil && *sample.SpeedMPS > 0 {
			values = append(values, 1000 / *sample.SpeedMPS)
		}
	}
	if len(values) == 0 {
		return nil
	}
	sort.Float64s(values)
	min, max := values[quantileIndex(len(values), 0.05)], values[quantileIndex(len(values), 0.95)]
	if max-min < 1 && values[len(values)-1]-values[0] >= 1 {
		min, max = values[0], values[len(values)-1]
	}
	return &seriesPaceScale{min: min, max: max}
}

func quantileIndex(length int, quantile float64) int {
	if length <= 1 {
		return 0
	}
	index := int(quantile * float64(length-1))
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func clampSeriesPace(value float64, scale *seriesPaceScale) float64 {
	if scale == nil {
		return value
	}
	if value < scale.min {
		return scale.min
	}
	if value > scale.max {
		return scale.max
	}
	return value
}

func seriesPointLabel(sample ActivitySample, index int) string {
	if sample.DistanceM != nil {
		return fmt.Sprintf("%.1f km", *sample.DistanceM/1000)
	}
	return fmt.Sprintf("%d", index+1)
}

func smoothSeriesElevation(points []ActivitySeriesPoint) []ActivitySeriesPoint {
	if len(points) < 3 {
		return points
	}
	for index, point := range points {
		if point.ElevationM == nil {
			continue
		}
		var total float64
		count := 0
		start := index - 36
		if start < 0 {
			start = 0
		}
		end := index + 36
		if end >= len(points) {
			end = len(points) - 1
		}
		for neighbor := start; neighbor <= end; neighbor++ {
			if points[neighbor].ElevationM != nil {
				total += *points[neighbor].ElevationM
				count++
			}
		}
		if count > 0 {
			smoothed := total / float64(count)
			points[index].ElevationM = &smoothed
		}
	}
	return points
}
