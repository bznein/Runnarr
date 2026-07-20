package app

import (
	"fmt"
	"math"
)

type toolsVDOTRequest struct {
	DistanceKm string `json:"distanceKm"`
	Time       string `json:"time"`
}

type toolsVDOTEquivalent struct {
	Race          string  `json:"race"`
	DistanceKm    float64 `json:"distanceKm"`
	DistanceLabel string  `json:"distanceLabel"`
	TimeSeconds   float64 `json:"timeSeconds"`
	TimeLabel     string  `json:"timeLabel"`
}

type toolsVDOTResponse struct {
	DistanceKm    float64               `json:"distanceKm"`
	TimeSeconds   float64               `json:"timeSeconds"`
	Vdot          float64               `json:"vdot"`
	VdotLabel     string                `json:"vdotLabel"`
	DistanceLabel string                `json:"distanceLabel"`
	TimeLabel     string                `json:"timeLabel"`
	Equivalents   []toolsVDOTEquivalent `json:"equivalents"`
}

type vdotRace struct {
	race string
	dM   float64
}

var toolsVDOTEquivalentRaces = []vdotRace{
	{"5K", 5000},
	{"10K", 10000},
	{"Half marathon", 21097.5},
	{"Marathon", 42195},
}

func calculateToolsVDOT(request toolsVDOTRequest) (toolsVDOTResponse, error) {
	distanceKm, hasDistance, err := parseToolsPaceDistance(request.DistanceKm)
	if err != nil {
		return toolsVDOTResponse{}, err
	}
	if !hasDistance {
		return toolsVDOTResponse{}, fmt.Errorf("distance is required")
	}

	timeSeconds, hasTime, err := parseToolsPaceDuration(request.Time)
	if err != nil {
		return toolsVDOTResponse{}, err
	}
	if !hasTime {
		return toolsVDOTResponse{}, fmt.Errorf("time is required")
	}

	vdot, err := toolsVDOTFromDistanceAndTime(distanceKm*1000, timeSeconds)
	if err != nil {
		return toolsVDOTResponse{}, err
	}

	equivalents, err := toolsVDOTEquivalents(vdot)
	if err != nil {
		return toolsVDOTResponse{}, err
	}

	return toolsVDOTResponse{
		DistanceKm:    distanceKm,
		TimeSeconds:   timeSeconds,
		Vdot:          vdot,
		VdotLabel:     formatToolsVDOT(vdot),
		DistanceLabel: formatToolsPaceDistance(distanceKm),
		TimeLabel:     formatToolsPaceDuration(timeSeconds),
		Equivalents:   equivalents,
	}, nil
}

func toolsVDOTFromDistanceAndTime(distanceMeters float64, timeSeconds float64) (float64, error) {
	if !isPositiveFloat(distanceMeters) || !isPositiveFloat(timeSeconds) {
		return 0, fmt.Errorf("distance and time must be greater than 0")
	}

	timeMinutes := timeSeconds / 60
	if !isPositiveFloat(timeMinutes) {
		return 0, fmt.Errorf("invalid time")
	}

	v := distanceMeters / timeMinutes
	if !isPositiveFloat(v) {
		return 0, fmt.Errorf("invalid distance/time combination")
	}

	vo2 := -4.6 + 0.182258*v + 0.000104*v*v
	efficiency := 0.8 + 0.1894393*math.Exp(-0.012778*timeMinutes) + 0.2989558*math.Exp(-0.1932605*timeMinutes)
	if !isPositiveFloat(efficiency) {
		return 0, fmt.Errorf("invalid effort")
	}

	vdot := vo2 / efficiency
	if !isPositiveFloat(vdot) {
		return 0, fmt.Errorf("could not compute VDOT")
	}
	return vdot, nil
}

func toolsVDOTEquivalents(vdot float64) ([]toolsVDOTEquivalent, error) {
	result := make([]toolsVDOTEquivalent, 0, len(toolsVDOTEquivalentRaces))
	for _, race := range toolsVDOTEquivalentRaces {
		timeSeconds, err := toolsVDOTTimeForDistance(race.dM, vdot)
		if err != nil {
			return nil, err
		}
		result = append(result, toolsVDOTEquivalent{
			Race:          race.race,
			DistanceKm:    race.dM / 1000,
			DistanceLabel: formatToolsPaceDistance(race.dM / 1000),
			TimeSeconds:   timeSeconds,
			TimeLabel:     formatToolsPaceDuration(timeSeconds),
		})
	}
	return result, nil
}

func toolsVDOTTimeForDistance(distanceMeters float64, targetVDOT float64) (float64, error) {
	if !isPositiveFloat(distanceMeters) {
		return 0, fmt.Errorf("distance must be greater than 0")
	}
	if !isPositiveFloat(targetVDOT) {
		return 0, fmt.Errorf("vdot must be greater than 0")
	}

	lowMinutes := 1.0
	highMinutes := 1.0
	highVdot, err := toolsVDOTFromDistanceAndTime(distanceMeters, highMinutes*60)
	if err != nil {
		return 0, err
	}
	if targetVDOT > highVdot {
		return 0, fmt.Errorf("VDOT is too high for this distance")
	}

	for attempts := 0; attempts < 64 && highVdot > targetVDOT; attempts++ {
		lowMinutes = highMinutes
		highMinutes *= 2
		highVdot, err = toolsVDOTFromDistanceAndTime(distanceMeters, highMinutes*60)
		if err != nil {
			return 0, err
		}
		if highMinutes > 24*60 {
			return 0, fmt.Errorf("could not compute equivalent time")
		}
	}

	for attempts := 0; attempts < 200; attempts++ {
		midMinutes := (lowMinutes + highMinutes) / 2
		midVDOT, err := toolsVDOTFromDistanceAndTime(distanceMeters, midMinutes*60)
		if err != nil {
			return 0, err
		}
		if midVDOT > targetVDOT {
			lowMinutes = midMinutes
		} else {
			highMinutes = midMinutes
		}
	}

	return highMinutes * 60, nil
}

func formatToolsVDOT(vdot float64) string {
	return fmt.Sprintf("%0.2f", vdot)
}
