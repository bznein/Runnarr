package app

import (
	"fmt"
	"math"
	"sort"
)

// defaultActivitySeriesPoints keeps mobile and browser detail views bounded
// without making ordinary activities look sparse.
const (
	defaultActivitySeriesPoints = 1200
	minActivitySeriesPoints     = 2
	maxActivitySeriesPoints     = 5000

	seriesPaceWindowS             = 15.0
	seriesSensorWindowS           = 5.0
	seriesElevationDistanceRadius = 150.0
	seriesElevationWindowS        = 15.0
	seriesGapS                    = 30.0
)

type activitySeriesBucket struct {
	start int
	end   int
}

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
	points := activitySeriesPoints(samples)
	if len(samples) <= maxPoints {
		return ActivitySeries{
			Samples:      samples,
			Points:       points,
			TotalSamples: len(samples),
			Sampled:      false,
		}
	}

	buckets := activitySeriesBuckets(samples, maxPoints)
	selectedSamples := make([]ActivitySample, 0, len(buckets))
	selectedPoints := make([]ActivitySeriesPoint, 0, len(buckets))
	for _, bucket := range buckets {
		selectedSamples = append(selectedSamples, aggregateActivitySamples(samples, bucket))
		selectedPoints = append(selectedPoints, aggregateActivitySeriesPoints(points, bucket))
	}
	return ActivitySeries{
		Samples:      selectedSamples,
		Points:       selectedPoints,
		TotalSamples: len(samples),
		Sampled:      true,
	}
}

func activitySeriesPoints(samples []ActivitySample) []ActivitySeriesPoint {
	points := make([]ActivitySeriesPoint, len(samples))
	for index, sample := range samples {
		rawPace := seriesPaceForSpeed(sample.SpeedMPS)
		points[index] = ActivitySeriesPoint{
			Index:         sample.Index,
			Label:         seriesPointLabel(sample, index),
			DistanceM:     finiteSeriesFloat(sample.DistanceM),
			Latitude:      finiteSeriesFloat(sample.Latitude),
			Longitude:     finiteSeriesFloat(sample.Longitude),
			RawElevationM: finiteSeriesFloat(sample.ElevationM),
			RawHeartRate:  sample.HeartRate,
			RawPaceSPKM:   rawPace,
			RawPower:      sample.Power,
			RawCadence:    sample.Cadence,
		}
	}

	times := activitySeriesTimes(samples)
	breaks := activitySeriesBreaks(samples, times)
	pace := make([]*float64, len(points))
	elevation := make([]*float64, len(points))
	heartRate := make([]*int, len(points))
	power := make([]*int, len(points))
	cadence := make([]*int, len(points))
	for index := range points {
		pace[index] = points[index].RawPaceSPKM
		elevation[index] = points[index].RawElevationM
		heartRate[index] = points[index].RawHeartRate
		power[index] = points[index].RawPower
		cadence[index] = points[index].RawCadence
	}

	smoothedPace := smoothSeriesFloatMedian(pace, times, seriesPaceWindowS/2, breaks)
	smoothedElevation := smoothSeriesElevation(elevation, samples, times, breaks)
	smoothedHeartRate := smoothSeriesIntMedian(heartRate, times, seriesSensorWindowS/2, breaks)
	smoothedPower := smoothSeriesIntMedian(power, times, seriesSensorWindowS/2, breaks)
	smoothedCadence := smoothSeriesIntMedian(cadence, times, seriesSensorWindowS/2, breaks)
	for index := range points {
		points[index].PaceSPKM = smoothedPace[index]
		points[index].ElevationM = smoothedElevation[index]
		points[index].HeartRate = smoothedHeartRate[index]
		points[index].Power = smoothedPower[index]
		points[index].Cadence = smoothedCadence[index]
	}
	return points
}

func seriesPaceForSpeed(speed *float64) *float64 {
	if speed == nil || !isFiniteSeriesValue(*speed) || *speed <= 0 {
		return nil
	}
	return seriesFloatPtr(1000 / *speed)
}

func finiteSeriesFloat(value *float64) *float64 {
	if value == nil || !isFiniteSeriesValue(*value) {
		return nil
	}
	return seriesFloatPtr(*value)
}

func isFiniteSeriesValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func activitySeriesTimes(samples []ActivitySample) []float64 {
	times := make([]float64, len(samples))
	validElapsed := len(samples) > 0
	for index, sample := range samples {
		if sample.ElapsedS == nil || (index > 0 && *sample.ElapsedS < *samples[index-1].ElapsedS) {
			validElapsed = false
			break
		}
		times[index] = float64(*sample.ElapsedS)
	}
	if validElapsed && (len(times) <= 1 || times[len(times)-1] > times[0]) {
		return times
	}

	validTimestamps := len(samples) > 0 && samples[0].Timestamp != nil
	if validTimestamps {
		first := *samples[0].Timestamp
		for index, sample := range samples {
			if sample.Timestamp == nil || (index > 0 && sample.Timestamp.Before(*samples[index-1].Timestamp)) {
				validTimestamps = false
				break
			}
			times[index] = sample.Timestamp.Sub(first).Seconds()
		}
	}
	if validTimestamps && (len(times) <= 1 || times[len(times)-1] > times[0]) {
		return times
	}

	for index := range samples {
		times[index] = float64(index)
	}
	return times
}

func activitySeriesBreaks(samples []ActivitySample, times []float64) []bool {
	breaks := make([]bool, len(samples))
	for index, sample := range samples {
		if sample.SpeedMPS != nil && (!isFiniteSeriesValue(*sample.SpeedMPS) || *sample.SpeedMPS <= 0) {
			breaks[index] = true
		}
		if index > 0 && times[index]-times[index-1] > seriesGapS {
			breaks[index] = true
		}
	}
	return breaks
}

func smoothSeriesFloatMedian(values []*float64, axes []float64, radius float64, breaks []bool) []*float64 {
	result := make([]*float64, len(values))
	forEachSeriesFloatSegment(values, breaks, func(start, end int) {
		left, right := start, start
		for index := start; index < end; index++ {
			for left < end && axes[left] < axes[index]-radius {
				left++
			}
			if right < index {
				right = index
			}
			for right < end && axes[right] <= axes[index]+radius {
				right++
			}
			window := make([]float64, 0, right-left)
			for neighbor := left; neighbor < right; neighbor++ {
				window = append(window, *values[neighbor])
			}
			sort.Float64s(window)
			result[index] = seriesFloatPtr(seriesMedianFloat(window))
		}
	})
	return result
}

func smoothSeriesIntMedian(values []*int, axes []float64, radius float64, breaks []bool) []*int {
	result := make([]*int, len(values))
	forEachSeriesIntSegment(values, breaks, func(start, end int) {
		left, right := start, start
		for index := start; index < end; index++ {
			for left < end && axes[left] < axes[index]-radius {
				left++
			}
			if right < index {
				right = index
			}
			for right < end && axes[right] <= axes[index]+radius {
				right++
			}
			window := make([]int, 0, right-left)
			for neighbor := left; neighbor < right; neighbor++ {
				window = append(window, *values[neighbor])
			}
			sort.Ints(window)
			result[index] = seriesIntPtr(window[len(window)/2])
		}
	})
	return result
}

func smoothSeriesElevation(values []*float64, samples []ActivitySample, times []float64, breaks []bool) []*float64 {
	result := make([]*float64, len(values))
	forEachSeriesFloatSegment(values, breaks, func(start, end int) {
		axes := times[start:end]
		radius := seriesElevationWindowS / 2
		if distances := monotonicSeriesDistances(samples, start, end); distances != nil {
			axes = distances
			radius = seriesElevationDistanceRadius
		}
		smoothed := smoothSeriesFloatMeanSegment(values[start:end], axes, radius)
		copy(result[start:end], smoothed)
	})
	return result
}

func monotonicSeriesDistances(samples []ActivitySample, start, end int) []float64 {
	distances := make([]float64, 0, end-start)
	for index := start; index < end; index++ {
		if samples[index].DistanceM == nil || !isFiniteSeriesValue(*samples[index].DistanceM) {
			return nil
		}
		distance := *samples[index].DistanceM
		if len(distances) > 0 && distance < distances[len(distances)-1] {
			return nil
		}
		distances = append(distances, distance)
	}
	return distances
}

func smoothSeriesFloatMeanSegment(values []*float64, axes []float64, radius float64) []*float64 {
	result := make([]*float64, len(values))
	left, right := 0, 0
	var sum float64
	for index := range values {
		for left < len(values) && axes[left] < axes[index]-radius {
			sum -= *values[left]
			left++
		}
		if right < index {
			right = index
		}
		for right < len(values) && axes[right] <= axes[index]+radius {
			sum += *values[right]
			right++
		}
		result[index] = seriesFloatPtr(sum / float64(right-left))
	}
	return result
}

func forEachSeriesFloatSegment(values []*float64, breaks []bool, visit func(start, end int)) {
	for start := 0; start < len(values); {
		if breaks[start] || values[start] == nil {
			start++
			continue
		}
		end := start + 1
		for end < len(values) && !breaks[end] && values[end] != nil {
			end++
		}
		visit(start, end)
		start = end
	}
}

func forEachSeriesIntSegment(values []*int, breaks []bool, visit func(start, end int)) {
	for start := 0; start < len(values); {
		if breaks[start] || values[start] == nil {
			start++
			continue
		}
		end := start + 1
		for end < len(values) && !breaks[end] && values[end] != nil {
			end++
		}
		visit(start, end)
		start = end
	}
}

func activitySeriesBuckets(samples []ActivitySample, maxPoints int) []activitySeriesBucket {
	if len(samples) <= maxPoints {
		buckets := make([]activitySeriesBucket, len(samples))
		for index := range samples {
			buckets[index] = activitySeriesBucket{start: index, end: index + 1}
		}
		return buckets
	}

	if maxPoints == 2 {
		middle := len(samples) / 2
		return []activitySeriesBucket{{start: 0, end: middle}, {start: middle, end: len(samples)}}
	}

	axes := activitySeriesTimes(samples)
	if axes[len(axes)-1] <= axes[0] {
		for index := range axes {
			axes[index] = float64(index)
		}
	}
	interiorBuckets := maxPoints - 2
	groups := make([]activitySeriesBucket, interiorBuckets)
	used := make([]bool, interiorBuckets)
	span := axes[len(axes)-1] - axes[0]
	for index := 1; index < len(samples)-1; index++ {
		bucketIndex := int((axes[index] - axes[0]) / span * float64(interiorBuckets))
		if bucketIndex >= interiorBuckets {
			bucketIndex = interiorBuckets - 1
		}
		if !used[bucketIndex] {
			groups[bucketIndex] = activitySeriesBucket{start: index, end: index + 1}
			used[bucketIndex] = true
		} else {
			groups[bucketIndex].end = index + 1
		}
	}

	buckets := make([]activitySeriesBucket, 0, maxPoints)
	buckets = append(buckets, activitySeriesBucket{start: 0, end: 1})
	for index, group := range groups {
		if used[index] {
			buckets = append(buckets, group)
		}
	}
	buckets = append(buckets, activitySeriesBucket{start: len(samples) - 1, end: len(samples)})
	return buckets
}

func aggregateActivitySamples(samples []ActivitySample, bucket activitySeriesBucket) ActivitySample {
	if bucket.end-bucket.start == 1 {
		return samples[bucket.start]
	}
	anchor := bucket.start + (bucket.end-bucket.start)/2
	if bucket.start == 0 {
		anchor = 0
	} else if bucket.end == len(samples) {
		anchor = len(samples) - 1
	}
	result := samples[anchor]
	result.ElapsedS = seriesMeanSampleInt(samples, bucket, func(sample ActivitySample) *int { return sample.ElapsedS })
	result.DistanceM = seriesMeanSampleFloat(samples, bucket, func(sample ActivitySample) *float64 { return sample.DistanceM })
	result.Latitude = seriesMeanSampleFloat(samples, bucket, func(sample ActivitySample) *float64 { return sample.Latitude })
	result.Longitude = seriesMeanSampleFloat(samples, bucket, func(sample ActivitySample) *float64 { return sample.Longitude })
	result.ElevationM = seriesMeanSampleFloat(samples, bucket, func(sample ActivitySample) *float64 { return sample.ElevationM })
	result.HeartRate = seriesMedianSampleInt(samples, bucket, func(sample ActivitySample) *int { return sample.HeartRate })
	result.Cadence = seriesMedianSampleInt(samples, bucket, func(sample ActivitySample) *int { return sample.Cadence })
	result.Power = seriesMedianSampleInt(samples, bucket, func(sample ActivitySample) *int { return sample.Power })
	result.SpeedMPS = seriesMeanSampleFloat(samples, bucket, func(sample ActivitySample) *float64 { return sample.SpeedMPS })
	return result
}

func aggregateActivitySeriesPoints(points []ActivitySeriesPoint, bucket activitySeriesBucket) ActivitySeriesPoint {
	if bucket.end-bucket.start == 1 {
		return points[bucket.start]
	}
	anchor := bucket.start + (bucket.end-bucket.start)/2
	if bucket.start == 0 {
		anchor = 0
	} else if bucket.end == len(points) {
		anchor = len(points) - 1
	}
	result := points[anchor]
	result.DistanceM = seriesMeanPointFloat(points, bucket, func(point ActivitySeriesPoint) *float64 { return point.DistanceM }, false)
	result.Latitude = seriesMeanPointFloat(points, bucket, func(point ActivitySeriesPoint) *float64 { return point.Latitude }, false)
	result.Longitude = seriesMeanPointFloat(points, bucket, func(point ActivitySeriesPoint) *float64 { return point.Longitude }, false)
	result.RawElevationM = seriesMeanPointFloat(points, bucket, func(point ActivitySeriesPoint) *float64 { return point.RawElevationM }, false)
	result.ElevationM = seriesMeanPointFloat(points, bucket, func(point ActivitySeriesPoint) *float64 { return point.ElevationM }, true)
	result.RawPaceSPKM = seriesMedianPointFloat(points, bucket, func(point ActivitySeriesPoint) *float64 { return point.RawPaceSPKM }, false)
	result.PaceSPKM = seriesMedianPointFloat(points, bucket, func(point ActivitySeriesPoint) *float64 { return point.PaceSPKM }, true)
	result.RawHeartRate = seriesMedianPointInt(points, bucket, func(point ActivitySeriesPoint) *int { return point.RawHeartRate }, false)
	result.HeartRate = seriesMedianPointInt(points, bucket, func(point ActivitySeriesPoint) *int { return point.HeartRate }, true)
	result.RawCadence = seriesMedianPointInt(points, bucket, func(point ActivitySeriesPoint) *int { return point.RawCadence }, false)
	result.Cadence = seriesMedianPointInt(points, bucket, func(point ActivitySeriesPoint) *int { return point.Cadence }, true)
	result.RawPower = seriesMedianPointInt(points, bucket, func(point ActivitySeriesPoint) *int { return point.RawPower }, false)
	result.Power = seriesMedianPointInt(points, bucket, func(point ActivitySeriesPoint) *int { return point.Power }, true)
	if result.DistanceM != nil {
		result.Label = fmt.Sprintf("%.1f km", *result.DistanceM/1000)
	}
	return result
}

func seriesMeanSampleFloat(samples []ActivitySample, bucket activitySeriesBucket, value func(ActivitySample) *float64) *float64 {
	var sum float64
	count := 0
	for index := bucket.start; index < bucket.end; index++ {
		candidate := finiteSeriesFloat(value(samples[index]))
		if candidate != nil {
			sum += *candidate
			count++
		}
	}
	if count == 0 {
		return nil
	}
	return seriesFloatPtr(sum / float64(count))
}

func seriesMeanSampleInt(samples []ActivitySample, bucket activitySeriesBucket, value func(ActivitySample) *int) *int {
	total, count := 0, 0
	for index := bucket.start; index < bucket.end; index++ {
		if candidate := value(samples[index]); candidate != nil {
			total += *candidate
			count++
		}
	}
	if count == 0 {
		return nil
	}
	return seriesIntPtr(int(math.Round(float64(total) / float64(count))))
}

func seriesMedianSampleInt(samples []ActivitySample, bucket activitySeriesBucket, value func(ActivitySample) *int) *int {
	values := make([]int, 0, bucket.end-bucket.start)
	for index := bucket.start; index < bucket.end; index++ {
		if candidate := value(samples[index]); candidate != nil {
			values = append(values, *candidate)
		}
	}
	if len(values) == 0 {
		return nil
	}
	sort.Ints(values)
	return seriesIntPtr(values[len(values)/2])
}

func seriesMeanPointFloat(points []ActivitySeriesPoint, bucket activitySeriesBucket, value func(ActivitySeriesPoint) *float64, requireAll bool) *float64 {
	var sum float64
	count := 0
	for index := bucket.start; index < bucket.end; index++ {
		candidate := value(points[index])
		if candidate == nil {
			if requireAll {
				return nil
			}
			continue
		}
		sum += *candidate
		count++
	}
	if count == 0 {
		return nil
	}
	return seriesFloatPtr(sum / float64(count))
}

func seriesMedianPointFloat(points []ActivitySeriesPoint, bucket activitySeriesBucket, value func(ActivitySeriesPoint) *float64, requireAll bool) *float64 {
	values := make([]float64, 0, bucket.end-bucket.start)
	for index := bucket.start; index < bucket.end; index++ {
		candidate := value(points[index])
		if candidate == nil {
			if requireAll {
				return nil
			}
			continue
		}
		values = append(values, *candidate)
	}
	if len(values) == 0 {
		return nil
	}
	sort.Float64s(values)
	return seriesFloatPtr(seriesMedianFloat(values))
}

func seriesMedianPointInt(points []ActivitySeriesPoint, bucket activitySeriesBucket, value func(ActivitySeriesPoint) *int, requireAll bool) *int {
	values := make([]int, 0, bucket.end-bucket.start)
	for index := bucket.start; index < bucket.end; index++ {
		candidate := value(points[index])
		if candidate == nil {
			if requireAll {
				return nil
			}
			continue
		}
		values = append(values, *candidate)
	}
	if len(values) == 0 {
		return nil
	}
	sort.Ints(values)
	return seriesIntPtr(values[len(values)/2])
}

func seriesMedianFloat(values []float64) float64 {
	middle := len(values) / 2
	if len(values)%2 == 0 {
		return (values[middle-1] + values[middle]) / 2
	}
	return values[middle]
}

func seriesPointLabel(sample ActivitySample, index int) string {
	if sample.DistanceM != nil {
		return fmt.Sprintf("%.1f km", *sample.DistanceM/1000)
	}
	return fmt.Sprintf("%d", index+1)
}

func seriesFloatPtr(value float64) *float64 {
	return &value
}

func seriesIntPtr(value int) *int {
	return &value
}
