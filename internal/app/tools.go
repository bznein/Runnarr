package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type toolsPaceRequest struct {
	DistanceKm string `json:"distanceKm"`
	Time       string `json:"time"`
	Pace       string `json:"pace"`
}

type toolsPaceResponse struct {
	DistanceKm       float64 `json:"distanceKm"`
	TimeSeconds      float64 `json:"timeSeconds"`
	PaceSecondsPerKm float64 `json:"paceSecondsPerKm"`
	Computed         string  `json:"computed"`
	DistanceLabel    string  `json:"distanceLabel"`
	TimeLabel        string  `json:"timeLabel"`
	PaceLabel        string  `json:"paceLabel"`
}

func calculateToolsPace(request toolsPaceRequest) (toolsPaceResponse, error) {
	distance, hasDistance, err := parseToolsPaceDistance(request.DistanceKm)
	if err != nil {
		return toolsPaceResponse{}, err
	}
	timeSeconds, hasTime, err := parseToolsPaceDuration(request.Time)
	if err != nil {
		return toolsPaceResponse{}, err
	}
	paceSecondsPerKm, hasPace, err := parseToolsPacePace(request.Pace)
	if err != nil {
		return toolsPaceResponse{}, err
	}

	provided := boolToInt(hasDistance) + boolToInt(hasTime) + boolToInt(hasPace)
	if provided != 2 {
		return toolsPaceResponse{}, fmt.Errorf("exactly two values are required")
	}

	if hasDistance && hasTime {
		tempo := timeSeconds / distance
		return formatToolsPaceResponse(distance, timeSeconds, tempo, "pace"), nil
	}
	if hasDistance && hasPace {
		totalTime := distance * paceSecondsPerKm
		return formatToolsPaceResponse(distance, totalTime, paceSecondsPerKm, "time"), nil
	}
	if hasTime && hasPace {
		distance = timeSeconds / paceSecondsPerKm
		return formatToolsPaceResponse(distance, timeSeconds, paceSecondsPerKm, "distance"), nil
	}

	return toolsPaceResponse{}, fmt.Errorf("invalid calculation combination")
}

func parseToolsPaceDistance(value string) (float64, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false, nil
	}

	distance, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false, fmt.Errorf("distance must be a number")
	}
	if !isPositiveFloat(distance) {
		return 0, false, fmt.Errorf("distance must be greater than 0")
	}
	return distance, true, nil
}

func parseToolsPacePace(value string) (float64, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false, nil
	}

	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, false, fmt.Errorf("pace must use mm:ss format")
	}
	minutes, err := parseUnsignedInt(parts[0])
	if err != nil {
		return 0, false, fmt.Errorf("pace must use mm:ss format")
	}
	seconds, err := parseUnsignedInt(parts[1])
	if err != nil || seconds >= 60 {
		return 0, false, fmt.Errorf("pace must use mm:ss format")
	}
	totalSeconds := minutes*60 + seconds
	if !isPositiveFloat(float64(totalSeconds)) {
		return 0, false, fmt.Errorf("pace must be greater than 0")
	}
	return float64(totalSeconds), true, nil
}

func parseToolsPaceDuration(value string) (float64, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false, nil
	}

	parts := strings.Split(value, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return 0, false, fmt.Errorf("time must use MM:SS or HH:MM:SS format")
	}

	if len(parts) == 2 {
		minutes, err := parseUnsignedInt(parts[0])
		if err != nil {
			return 0, false, fmt.Errorf("time must use MM:SS or HH:MM:SS format")
		}
		seconds, err := parseUnsignedInt(parts[1])
		if err != nil || seconds >= 60 {
			return 0, false, fmt.Errorf("time must use MM:SS or HH:MM:SS format")
		}
		return float64(minutes*60 + seconds), true, nil
	}

	hours, err := parseUnsignedInt(parts[0])
	if err != nil {
		return 0, false, fmt.Errorf("time must use MM:SS or HH:MM:SS format")
	}
	minutes, err := parseUnsignedInt(parts[1])
	if err != nil {
		return 0, false, fmt.Errorf("time must use MM:SS or HH:MM:SS format")
	}
	seconds, err := parseUnsignedInt(parts[2])
	if err != nil || seconds >= 60 {
		return 0, false, fmt.Errorf("time must use MM:SS or HH:MM:SS format")
	}
	return float64(hours*3600 + minutes*60 + seconds), true, nil
}

func parseUnsignedInt(value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid integer")
	}
	return parsed, nil
}

func formatToolsPaceResponse(distance float64, timeSeconds float64, paceSeconds float64, computed string) toolsPaceResponse {
	return toolsPaceResponse{
		DistanceKm:       distance,
		TimeSeconds:      timeSeconds,
		PaceSecondsPerKm: paceSeconds,
		Computed:         computed,
		DistanceLabel:    formatToolsPaceDistance(distance),
		TimeLabel:        formatToolsPaceDuration(timeSeconds),
		PaceLabel:        formatToolsPacePace(paceSeconds),
	}
}

func formatToolsPaceDistance(distance float64) string {
	return fmt.Sprintf("%0.3f km", distance)
}

func formatToolsPaceDuration(totalSeconds float64) string {
	seconds := int64(math.Round(totalSeconds))
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	remaining := seconds % 60
	return fmt.Sprintf("%d:%02d:%02d", hours, minutes, remaining)
}

func formatToolsPacePace(secondsPerKm float64) string {
	seconds := int64(math.Round(secondsPerKm))
	if seconds < 0 {
		seconds = 0
	}
	minutes := seconds / 60
	remaining := seconds % 60
	if remaining < 0 {
		remaining = 0
	}
	return fmt.Sprintf("%d:%02d /km", minutes, remaining)
}

func isPositiveFloat(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
