package app

import (
	"fmt"
	"math"
)

const defaultClimbDetectionPreset = "balanced"
const defaultClimbDetectionSensitivity = 50

func defaultClimbDetectionSettings() ClimbDetectionSettings {
	return ClimbDetectionSettings{
		ClimbSmoothingRadiusM:       75,
		MinClimbDistanceM:           300,
		MinClimbElevationGainM:      15,
		MinClimbAverageGradePct:     2.5,
		MaxClimbMergeDipDistanceM:   150,
		MaxClimbMergeElevationLossM: 8,
		ClimbStartGainM:             3,
	}
}

type climbDetectionSensitivityProfile struct {
	sensitivity int
	settings    ClimbDetectionSettings
}

var climbDetectionSensitivityProfiles = []climbDetectionSensitivityProfile{
	{
		sensitivity: 0,
		settings: ClimbDetectionSettings{
			ClimbSmoothingRadiusM:       100,
			MinClimbDistanceM:           450,
			MinClimbElevationGainM:      24,
			MinClimbAverageGradePct:     3.2,
			MaxClimbMergeDipDistanceM:   120,
			MaxClimbMergeElevationLossM:  6,
			ClimbStartGainM:             4,
		},
	},
	{
		sensitivity: 50,
		settings: ClimbDetectionSettings{
			ClimbSmoothingRadiusM:       75,
			MinClimbDistanceM:           300,
			MinClimbElevationGainM:      15,
			MinClimbAverageGradePct:     2.5,
			MaxClimbMergeDipDistanceM:   150,
			MaxClimbMergeElevationLossM:  8,
			ClimbStartGainM:             3,
		},
	},
	{
		sensitivity: 100,
		settings: ClimbDetectionSettings{
			ClimbSmoothingRadiusM:       55,
			MinClimbDistanceM:           170,
			MinClimbElevationGainM:      10,
			MinClimbAverageGradePct:     1.8,
			MaxClimbMergeDipDistanceM:   190,
			MaxClimbMergeElevationLossM:  10,
			ClimbStartGainM:             2,
		},
	},
}

func clampClimbDetectionSensitivity(sensitivity int) int {
	if sensitivity < 0 {
		return 0
	}
	if sensitivity > 100 {
		return 100
	}
	return sensitivity
}

func climbDetectionSettingsForSensitivity(sensitivity int) ClimbDetectionSettings {
	value := clampClimbDetectionSensitivity(sensitivity)
	previousProfile := climbDetectionSensitivityProfiles[0]
	for _, nextProfile := range climbDetectionSensitivityProfiles[1:] {
		if value <= nextProfile.sensitivity {
			return interpolateClimbDetectionSettings(previousProfile, nextProfile, value)
		}
		previousProfile = nextProfile
	}
	return climbDetectionSensitivityProfiles[len(climbDetectionSensitivityProfiles)-1].settings
}

func interpolateClimbDetectionSettings(a, b climbDetectionSensitivityProfile, target int) ClimbDetectionSettings {
	rangeSize := float64(b.sensitivity - a.sensitivity)
	if rangeSize <= 0 {
		return b.settings
	}
	blend := float64(target-a.sensitivity) / rangeSize
	return ClimbDetectionSettings{
		ClimbSmoothingRadiusM:       roundClimbSensitivityValue(a.settings.ClimbSmoothingRadiusM + (b.settings.ClimbSmoothingRadiusM-a.settings.ClimbSmoothingRadiusM)*blend),
		MinClimbDistanceM:           roundClimbSensitivityValue(a.settings.MinClimbDistanceM + (b.settings.MinClimbDistanceM-a.settings.MinClimbDistanceM)*blend),
		MinClimbElevationGainM:      roundClimbSensitivityValue(a.settings.MinClimbElevationGainM + (b.settings.MinClimbElevationGainM-a.settings.MinClimbElevationGainM)*blend),
		MinClimbAverageGradePct:     roundClimbSensitivityValue(a.settings.MinClimbAverageGradePct + (b.settings.MinClimbAverageGradePct-a.settings.MinClimbAverageGradePct)*blend),
		MaxClimbMergeDipDistanceM:   roundClimbSensitivityValue(a.settings.MaxClimbMergeDipDistanceM + (b.settings.MaxClimbMergeDipDistanceM-a.settings.MaxClimbMergeDipDistanceM)*blend),
		MaxClimbMergeElevationLossM:  roundClimbSensitivityValue(a.settings.MaxClimbMergeElevationLossM + (b.settings.MaxClimbMergeElevationLossM-a.settings.MaxClimbMergeElevationLossM)*blend),
		ClimbStartGainM:             roundClimbSensitivityValue(a.settings.ClimbStartGainM + (b.settings.ClimbStartGainM-a.settings.ClimbStartGainM)*blend),
	}
}

func roundClimbSensitivityValue(value float64) float64 {
	return math.Round(value*10) / 10
}

func climbDetectionSensitivityFromSettings(settings ClimbDetectionSettings) int {
	if err := validateClimbDetectionSettings(settings); err != nil {
		return defaultClimbDetectionSensitivity
	}
	best := defaultClimbDetectionSensitivity
	bestDiff := math.Inf(1)
	for value := 0; value <= 100; value++ {
		diff := climbDetectionSettingsDiff(settings, climbDetectionSettingsForSensitivity(value))
		if diff < bestDiff {
			best = value
			bestDiff = diff
		}
	}
	return best
}

func climbDetectionSettingsDiff(left ClimbDetectionSettings, right ClimbDetectionSettings) float64 {
	return math.Abs(left.ClimbSmoothingRadiusM-right.ClimbSmoothingRadiusM)/120 +
		math.Abs(left.MinClimbDistanceM-right.MinClimbDistanceM)/450 +
		math.Abs(left.MinClimbElevationGainM-right.MinClimbElevationGainM)/25 +
		math.Abs(left.MinClimbAverageGradePct-right.MinClimbAverageGradePct)/4 +
		math.Abs(left.MaxClimbMergeDipDistanceM-right.MaxClimbMergeDipDistanceM)/220 +
		math.Abs(left.MaxClimbMergeElevationLossM-right.MaxClimbMergeElevationLossM)/12 +
		math.Abs(left.ClimbStartGainM-right.ClimbStartGainM)/8
}

type climbPoint struct {
	sampleIndex int
	distanceM   float64
	elevationM  float64
}

func detectActivityClimbs(samples []ActivitySample) []ActivityClimb {
	return detectActivityClimbsWithSettings(samples, defaultClimbDetectionSettings())
}

func detectActivityClimbsWithSettings(samples []ActivitySample, settings ClimbDetectionSettings) []ActivityClimb {
	if err := validateClimbDetectionSettings(settings); err != nil {
		settings = defaultClimbDetectionSettings()
	}

	points := climbPointsFromSamples(samples)
	if len(points) < 2 {
		return nil
	}
	return detectClimbsFromPoints(smoothClimbPoints(points, settings.ClimbSmoothingRadiusM), settings)
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

func smoothClimbPoints(points []climbPoint, radiusM float64) []climbPoint {
	smoothed := make([]climbPoint, len(points))
	left := 0
	right := 0
	sum := 0.0
	count := 0
	for index, point := range points {
		for right < len(points) && points[right].distanceM <= point.distanceM+radiusM {
			sum += points[right].elevationM
			count++
			right++
		}
		for left < len(points) && points[left].distanceM < point.distanceM-radiusM {
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

func detectClimbsFromPoints(points []climbPoint, settings ClimbDetectionSettings) []ActivityClimb {
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
			if gain >= settings.ClimbStartGainM && averageGradePct(gain, distance) >= settings.MinClimbAverageGradePct {
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
		if dipDistance > settings.MaxClimbMergeDipDistanceM || dipLoss > settings.MaxClimbMergeElevationLossM {
			climbs = appendValidClimb(climbs, points, startIndex, peakIndex, settings)
			inClimb = false
			lowIndex = dipLowIndex
		}
	}

	if inClimb {
		climbs = appendValidClimb(climbs, points, startIndex, peakIndex, settings)
	}
	return climbs
}

func appendValidClimb(climbs []ActivityClimb, points []climbPoint, startIndex int, endIndex int, settings ClimbDetectionSettings) []ActivityClimb {
	if startIndex < 0 || endIndex <= startIndex || endIndex >= len(points) {
		return climbs
	}
	start := points[startIndex]
	end := points[endIndex]
	distance := end.distanceM - start.distanceM
	gain := end.elevationM - start.elevationM
	grade := averageGradePct(gain, distance)
	if distance < settings.MinClimbDistanceM || gain < settings.MinClimbElevationGainM || grade < settings.MinClimbAverageGradePct {
		return climbs
	}
	return append(climbs, ActivityClimb{
		Index:            len(climbs),
		Difficulty:       classifyClimb(gain, grade),
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

func classifyClimb(gainM float64, gradePct float64) string {
	score := gainM * gradePct
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

func validateClimbDetectionSettings(settings ClimbDetectionSettings) error {
	if !isPositiveFloat(settings.ClimbSmoothingRadiusM) {
		return fmt.Errorf("climbSmoothingRadiusM must be greater than 0")
	}
	if !isPositiveFloat(settings.MinClimbDistanceM) {
		return fmt.Errorf("minClimbDistanceM must be greater than 0")
	}
	if !isPositiveFloat(settings.MinClimbElevationGainM) {
		return fmt.Errorf("minClimbElevationGainM must be greater than 0")
	}
	if !isPositiveFloat(settings.MinClimbAverageGradePct) {
		return fmt.Errorf("minClimbAverageGradePct must be greater than 0")
	}
	if !isPositiveFloat(settings.MaxClimbMergeDipDistanceM) {
		return fmt.Errorf("maxClimbMergeDipDistanceM must be greater than 0")
	}
	if !isPositiveFloat(settings.MaxClimbMergeElevationLossM) {
		return fmt.Errorf("maxClimbMergeElevationLossM must be greater than 0")
	}
	if !isPositiveFloat(settings.ClimbStartGainM) {
		return fmt.Errorf("climbStartGainM must be greater than 0")
	}
	return nil
}
