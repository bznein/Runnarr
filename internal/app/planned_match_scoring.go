package app

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	plannedMatchLevelStrong   = "strong"
	plannedMatchLevelPossible = "possible"
	plannedMatchLevelWeak     = "weak"
)

var (
	plannedMatchHourMinutePattern = regexp.MustCompile(`(?i)\b(\d+)\s*(?:h|hr|hrs|hour|hours)\s*(\d+)\s*(?:m|min|mins|minute|minutes)\b`)
	plannedMatchHourPattern       = regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?)\s*(?:h|hr|hrs|hour|hours)\b`)
	plannedMatchMinutePattern     = regexp.MustCompile(`(?i)\b(\d+)\s*(?:m|min|mins|minute|minutes)\b`)
	plannedMatchRepeatPattern     = regexp.MustCompile(`(?i)\b\d+\s*[x×]\s*\d+`)
	plannedMatchRepeatPrefix      = regexp.MustCompile(`(?i)\d+\s*[x×]\s*$`)
	plannedMatchIntervalPattern   = regexp.MustCompile(`(?i)\b(?:intervals?|repetitions?|reps?)\b`)
)

func assessPlannedActivityMatch(activityDate time.Time, movingTimeS, elapsedTimeS int, activityStructured bool, planned PlannedActivity) PlannedActivityMatchCandidate {
	activityDurationS := movingTimeS
	if activityDurationS <= 0 {
		activityDurationS = elapsedTimeS
	}

	dateDelta := plannedMatchDateDeltaDays(activityDate, planned.PlannedDate)
	datePoints := 30 * math.Max(0, 1-float64(dateDelta)/7)
	reasons := []string{plannedMatchDateReason(activityDate, planned.PlannedDate, dateDelta)}

	durationPoints := 20.0
	durationBlocked := false
	if plannedDurationS, ok := plannedActivityExpectedDurationS(planned); ok && activityDurationS > 0 {
		ratio := float64(min(activityDurationS, plannedDurationS)) / float64(max(activityDurationS, plannedDurationS))
		durationPoints = 40 * ratio
		durationBlocked = float64(max(activityDurationS, plannedDurationS))/float64(min(activityDurationS, plannedDurationS)) > 1.5
		reasons = append(reasons, fmt.Sprintf("%s activity vs %s plan", formatPlannedMatchDuration(activityDurationS), formatPlannedMatchDuration(plannedDurationS)))
	} else if activityDurationS <= 0 {
		reasons = append(reasons, "Activity duration unavailable")
	} else {
		reasons = append(reasons, "Planned duration unavailable")
	}

	structurePoints := 15.0
	structureBlocked := false
	if plannedStructured, ok := plannedActivityStructured(planned); ok {
		if plannedStructured == activityStructured {
			structurePoints = 30
			if plannedStructured {
				reasons = append(reasons, "Both structured workouts")
			} else {
				reasons = append(reasons, "Both continuous runs")
			}
		} else {
			structurePoints = 0
			structureBlocked = true
			if plannedStructured {
				reasons = append(reasons, "Planned intervals; activity is continuous")
			} else {
				reasons = append(reasons, "Planned run is continuous; activity has intervals")
			}
		}
	} else {
		reasons = append(reasons, "Planned workout structure unavailable")
	}

	score := int(math.Round(datePoints + durationPoints + structurePoints))
	blocked := durationBlocked || structureBlocked
	if blocked && score > 59 {
		score = 59
	}
	level := plannedMatchLevelWeak
	if score >= 80 {
		level = plannedMatchLevelStrong
	} else if score >= 60 {
		level = plannedMatchLevelPossible
	}
	return PlannedActivityMatchCandidate{
		PlannedActivity:   planned,
		MatchScore:        score,
		MatchLevel:        level,
		MatchReasons:      reasons,
		suggestionBlocked: blocked,
	}
}

func suggestedPlannedActivityID(candidates []PlannedActivityMatchCandidate) string {
	if len(candidates) == 0 {
		return ""
	}
	bestIndex := 0
	secondScore := -1
	for index := 1; index < len(candidates); index++ {
		if candidates[index].MatchScore > candidates[bestIndex].MatchScore {
			secondScore = candidates[bestIndex].MatchScore
			bestIndex = index
		} else if candidates[index].MatchScore > secondScore {
			secondScore = candidates[index].MatchScore
		}
	}
	best := candidates[bestIndex]
	if best.suggestionBlocked || best.MatchScore < 80 {
		return ""
	}
	if secondScore >= 0 && best.MatchScore-secondScore < 10 {
		return ""
	}
	return best.ID
}

func plannedActivityExpectedDurationS(planned PlannedActivity) (int, bool) {
	color, _ := planned.Raw["planCellBackgroundColor"].(string)
	if !plannedMatchDurationColorTrusted(color) {
		return 0, false
	}
	return parsePlannedMatchTitleDuration(planned.Name)
}

func plannedActivityStructured(planned PlannedActivity) (bool, bool) {
	if plannedWorkoutTableHasRows(planned.Raw["workoutTable"]) {
		return true, true
	}
	text := strings.TrimSpace(planned.Name + "\n" + planned.Notes)
	if plannedMatchRepeatPattern.MatchString(text) || plannedMatchIntervalPattern.MatchString(text) {
		return true, true
	}
	if text != "" {
		return false, true
	}
	return false, false
}

func plannedWorkoutTableHasRows(value any) bool {
	switch table := value.(type) {
	case trainingSheetWorkoutTable:
		return len(table.Rows) > 0
	case *trainingSheetWorkoutTable:
		return table != nil && len(table.Rows) > 0
	case map[string]any:
		rows, ok := table["rows"].([]any)
		return ok && len(rows) > 0
	default:
		return false
	}
}

func parsePlannedMatchTitleDuration(title string) (int, bool) {
	if match := firstPlannedMatchDuration(title, plannedMatchHourMinutePattern); match != nil {
		hours, _ := strconv.Atoi(match[1])
		minutes, _ := strconv.Atoi(match[2])
		return hours*3600 + minutes*60, hours > 0 || minutes > 0
	}
	if match := firstPlannedMatchDuration(title, plannedMatchHourPattern); match != nil {
		hours, _ := strconv.ParseFloat(match[1], 64)
		seconds := int(math.Round(hours * 3600))
		return seconds, seconds > 0
	}
	if match := firstPlannedMatchDuration(title, plannedMatchMinutePattern); match != nil {
		minutes, _ := strconv.Atoi(match[1])
		return minutes * 60, minutes > 0
	}
	return 0, false
}

func firstPlannedMatchDuration(title string, pattern *regexp.Regexp) []string {
	for _, indexes := range pattern.FindAllStringSubmatchIndex(title, -1) {
		if len(indexes) < 4 || plannedMatchRepeatPrefix.MatchString(title[:indexes[0]]) {
			continue
		}
		matches := make([]string, 0, len(indexes)/2)
		for index := 0; index < len(indexes); index += 2 {
			if indexes[index] < 0 || indexes[index+1] < 0 {
				matches = append(matches, "")
				continue
			}
			matches = append(matches, title[indexes[index]:indexes[index+1]])
		}
		return matches
	}
	return nil
}

func plannedMatchDurationColorTrusted(value string) bool {
	red, green, blue, ok := parsePlannedMatchHexColor(value)
	if !ok {
		return false
	}
	maxValue := math.Max(red, math.Max(green, blue))
	minValue := math.Min(red, math.Min(green, blue))
	lightness := (maxValue + minValue) / 2
	delta := maxValue - minValue
	saturation := 0.0
	if delta > 0 {
		saturation = delta / (1 - math.Abs(2*lightness-1))
	}
	if saturation <= 0.10 && lightness >= 0.90 {
		return true
	}
	if delta == 0 || saturation < 0.10 {
		return false
	}
	hue := 0.0
	switch maxValue {
	case red:
		hue = 60 * math.Mod((green-blue)/delta, 6)
	case green:
		hue = 60 * ((blue-red)/delta + 2)
	default:
		hue = 60 * ((red-green)/delta + 4)
	}
	if hue < 0 {
		hue += 360
	}
	return hue >= 250 && hue <= 330
}

func parsePlannedMatchHexColor(value string) (float64, float64, float64, bool) {
	value = strings.TrimSpace(value)
	if len(value) != 7 || value[0] != '#' {
		return 0, 0, 0, false
	}
	parsed, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return 0, 0, 0, false
	}
	return float64((parsed>>16)&0xff) / 255, float64((parsed>>8)&0xff) / 255, float64(parsed&0xff) / 255, true
}

func plannedMatchDateDeltaDays(activityDate, plannedDate time.Time) int {
	activityDay := time.Date(activityDate.Year(), activityDate.Month(), activityDate.Day(), 0, 0, 0, 0, time.UTC)
	plannedDay := time.Date(plannedDate.Year(), plannedDate.Month(), plannedDate.Day(), 0, 0, 0, 0, time.UTC)
	return int(math.Abs(plannedDay.Sub(activityDay).Hours() / 24))
}

func plannedMatchDateReason(activityDate, plannedDate time.Time, delta int) string {
	if delta == 0 {
		return "Same day"
	}
	direction := "later"
	if plannedDate.Before(activityDate) {
		direction = "earlier"
	}
	unit := "days"
	if delta == 1 {
		unit = "day"
	}
	return fmt.Sprintf("Planned %d %s %s", delta, unit, direction)
}

func formatPlannedMatchDuration(seconds int) string {
	minutes := int(math.Round(float64(seconds) / 60))
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	hours := minutes / 60
	remainingMinutes := minutes % 60
	if remainingMinutes == 0 {
		return fmt.Sprintf("%d hr", hours)
	}
	return fmt.Sprintf("%d hr %d min", hours, remainingMinutes)
}
