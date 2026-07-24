package app

import (
	"math"
	"sort"
)

func enrichActivityClimbPerformance(climbs []ActivityClimb, samples []ActivitySample, laps []ActivityLap) []ActivityClimb {
	for index := range climbs {
		climbs[index].PaceSPKM = climbPaceForSamples(samples, climbs[index])
		climbs[index].GapSPKM = climbGapForLaps(laps, climbs[index])
	}
	return climbs
}

func climbPaceForSamples(samples []ActivitySample, climb ActivityClimb) *float64 {
	climbSamples := make([]ActivitySample, 0)
	for _, sample := range samples {
		if sample.Index >= climb.StartSampleIndex && sample.Index <= climb.EndSampleIndex {
			climbSamples = append(climbSamples, sample)
		}
	}
	sort.SliceStable(climbSamples, func(left, right int) bool {
		return climbSamples[left].Index < climbSamples[right].Index
	})
	if len(climbSamples) < 2 {
		return nil
	}

	var distanceM, movingTimeS float64
	for index := 1; index < len(climbSamples); index++ {
		previous := climbSamples[index-1]
		current := climbSamples[index]
		if previous.DistanceM == nil || current.DistanceM == nil || !finiteClimbValue(*previous.DistanceM) || !finiteClimbValue(*current.DistanceM) || *previous.DistanceM < 0 || *current.DistanceM < 0 {
			continue
		}
		distanceDeltaM := *current.DistanceM - *previous.DistanceM
		if distanceDeltaM <= 0 {
			continue
		}

		durationS := climbSampleDurationSeconds(previous, current, distanceDeltaM)
		if durationS == nil || *durationS <= 0 {
			continue
		}
		distanceM += distanceDeltaM
		movingTimeS += *durationS
	}
	if distanceM <= 0 || movingTimeS <= 0 {
		return nil
	}
	paceSPKM := movingTimeS / distanceM * 1000
	return &paceSPKM
}

func climbGapForLaps(laps []ActivityLap, climb ActivityClimb) *float64 {
	startDistanceM := math.Min(climb.StartDistanceM, climb.EndDistanceM)
	endDistanceM := math.Max(climb.StartDistanceM, climb.EndDistanceM)
	if !finiteClimbValue(startDistanceM) || !finiteClimbValue(endDistanceM) {
		return nil
	}

	sortedLaps := append([]ActivityLap(nil), laps...)
	sort.SliceStable(sortedLaps, func(left, right int) bool {
		return sortedLaps[left].Index < sortedLaps[right].Index
	})
	var lapStartDistanceM, weightedGap, weightedDistance float64
	for _, lap := range sortedLaps {
		lapDistanceM := lap.DistanceM
		if !finiteClimbValue(lapDistanceM) || lapDistanceM < 0 {
			lapDistanceM = 0
		}
		lapEndDistanceM := lapStartDistanceM + lapDistanceM
		overlapDistanceM := math.Max(0, math.Min(endDistanceM, lapEndDistanceM)-math.Max(startDistanceM, lapStartDistanceM))
		if overlapDistanceM > 0 && validClimbPace(lap.AvgGradeAdjustedPaceSPKM) {
			weightedGap += *lap.AvgGradeAdjustedPaceSPKM * overlapDistanceM
			weightedDistance += overlapDistanceM
		}
		lapStartDistanceM = lapEndDistanceM
	}
	if weightedDistance <= 0 {
		return nil
	}
	gapSPKM := weightedGap / weightedDistance
	return &gapSPKM
}

func climbSampleDurationSeconds(previous, current ActivitySample, distanceDeltaM float64) *float64 {
	if elapsedS := positiveClimbDifference(previous.ElapsedS, current.ElapsedS); elapsedS != nil {
		return elapsedS
	}
	if previous.Timestamp != nil && current.Timestamp != nil {
		timestampDeltaS := current.Timestamp.Sub(*previous.Timestamp).Seconds()
		if timestampDeltaS > 0 && finiteClimbValue(timestampDeltaS) {
			return &timestampDeltaS
		}
	}
	for _, speedMPS := range []*float64{current.SpeedMPS, previous.SpeedMPS} {
		if speedMPS != nil && finiteClimbValue(*speedMPS) && *speedMPS > 0 {
			durationS := distanceDeltaM / *speedMPS
			return &durationS
		}
	}
	return nil
}

func positiveClimbDifference(previous, current *int) *float64 {
	if previous == nil || current == nil {
		return nil
	}
	difference := float64(*current - *previous)
	if difference <= 0 {
		return nil
	}
	return &difference
}

func validClimbPace(value *float64) bool {
	return value != nil && finiteClimbValue(*value) && *value > 0
}

func finiteClimbValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
