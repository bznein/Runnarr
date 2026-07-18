package app

import "math"

const (
	climbSmoothingRadiusM       = 75
	maxClimbMergeDipDistanceM   = 150
	maxClimbMergeElevationLossM = 8
)

var defaultClimbProfile = climbActivityProfile{
	minClimbDistanceM:       300,
	minClimbElevationGainM:  15,
	minClimbAverageGradePct: 2.5,
	climbStartGainM:         3,
	difficultyScale:         1,
}

var bikeClimbProfile = climbActivityProfile{
	minClimbDistanceM:       300,
	minClimbElevationGainM:  15,
	minClimbAverageGradePct: 2.0,
	climbStartGainM:         3,
	difficultyScale:         1.8,
}

type climbActivityProfile struct {
	minClimbDistanceM       float64
	minClimbElevationGainM  float64
	minClimbAverageGradePct float64
	climbStartGainM         float64
	difficultyScale         float64
}

type climbPoint struct {
	sampleIndex int
	distanceM   float64
	elevationM  float64
}

func detectActivityClimbs(activityType string, samples []ActivitySample) []ActivityClimb {
	profile := climbProfileForActivityType(activityType)
	points := climbPointsFromSamples(samples)
	if len(points) < 2 {
		return nil
	}
	return detectClimbsFromPoints(smoothClimbPoints(points), profile)
}

func climbProfileForActivityType(activityType string) climbActivityProfile {
	switch normalizeSport(activityType) {
	case "Ride":
		return bikeClimbProfile
	default:
		return defaultClimbProfile
	}
}

func climbPointsFromSamples(samples []ActivitySample) []climbPoint {
	points := make([]climbPoint, 0, len(samples))
	previousDistance := math.Inf(-1)
	for _, sample := range samples {
		if sample.DistanceM == nil || sample.ElevationM == nil {
			continue
		}
		distance := *sample.DistanceM
		elevation := *sample.ElevationM
		if !finite(distance) || !finite(elevation) {
			continue
		}
		if distance < previousDistance {
			return nil
		}
		points = append(points, climbPoint{
			sampleIndex: sample.Index,
			distanceM:   distance,
			elevationM:  elevation,
		})
		previousDistance = distance
	}
	return points
}

func smoothClimbPoints(points []climbPoint) []climbPoint {
	smoothed := make([]climbPoint, len(points))
	left := 0
	right := 0
	sum := 0.0
	count := 0
	for index, point := range points {
		for right < len(points) && points[right].distanceM <= point.distanceM+climbSmoothingRadiusM {
			sum += points[right].elevationM
			count++
			right++
		}
		for left < len(points) && points[left].distanceM < point.distanceM-climbSmoothingRadiusM {
			sum -= points[left].elevationM
			count--
			left++
		}
		smoothed[index] = point
		if count > 0 {
			smoothed[index].elevationM = sum / float64(count)
		}
	}
	return smoothed
}

func detectClimbsFromPoints(points []climbPoint, profile climbActivityProfile) []ActivityClimb {
	climbs := make([]ActivityClimb, 0)
	lowIndex := 0
	inClimb := false
	startIndex := 0
	peakIndex := 0
	dipLowIndex := 0

	for index := 1; index < len(points); index++ {
		current := points[index]

		if !inClimb {
		if current.elevationM <= points[index-1].elevationM {
			lowIndex = index
			continue
		}
		gain := current.elevationM - points[lowIndex].elevationM
		distance := current.distanceM - points[lowIndex].distanceM
		if gain >= profile.climbStartGainM && averageGradePct(gain, distance) >= profile.minClimbAverageGradePct {
			inClimb = true
			startIndex = lowIndex
			peakIndex = index
				dipLowIndex = index
			}
			continue
		}

		if current.elevationM >= points[peakIndex].elevationM {
			peakIndex = index
			dipLowIndex = index
			continue
		}
		if current.elevationM < points[dipLowIndex].elevationM {
			dipLowIndex = index
		}
		dipDistance := current.distanceM - points[peakIndex].distanceM
		dipLoss := points[peakIndex].elevationM - current.elevationM
		if dipDistance > maxClimbMergeDipDistanceM || dipLoss > maxClimbMergeElevationLossM {
			climbs = appendValidClimb(climbs, points, startIndex, peakIndex, profile)
			inClimb = false
			lowIndex = dipLowIndex
		}
	}

	if inClimb {
		climbs = appendValidClimb(climbs, points, startIndex, peakIndex, profile)
	}
	return climbs
}

func appendValidClimb(climbs []ActivityClimb, points []climbPoint, startIndex int, endIndex int, profile climbActivityProfile) []ActivityClimb {
	if startIndex < 0 || endIndex <= startIndex || endIndex >= len(points) {
		return climbs
	}
	start := points[startIndex]
	end := points[endIndex]
	distance := end.distanceM - start.distanceM
	gain := end.elevationM - start.elevationM
	grade := averageGradePct(gain, distance)
	if distance < profile.minClimbDistanceM || gain < profile.minClimbElevationGainM || grade < profile.minClimbAverageGradePct {
		return climbs
	}
	return append(climbs, ActivityClimb{
		Index:            len(climbs),
		Difficulty:       classifyClimb(gain, grade, profile),
		StartSampleIndex: start.sampleIndex,
		EndSampleIndex:   end.sampleIndex,
		StartDistanceM:   start.distanceM,
		EndDistanceM:     end.distanceM,
		DistanceM:        distance,
		ElevationGainM:   gain,
		AvgGradePct:      grade,
		StartElevationM:  start.elevationM,
		EndElevationM:    end.elevationM,
	})
}

func classifyClimb(gainM float64, gradePct float64, profile climbActivityProfile) string {
	scale := profile.difficultyScale
	if scale <= 0 {
		scale = 1
	}
	score := gainM * gradePct / scale
	switch {
	case score >= 1600:
		return "Epic"
	case score >= 900:
		return "Very Hard"
	case score >= 400:
		return "Hard"
	case score >= 150:
		return "Moderate"
	default:
		return "Easy"
	}
}

func averageGradePct(gainM float64, distanceM float64) float64 {
	if distanceM <= 0 {
		return 0
	}
	return gainM / distanceM * 100
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
