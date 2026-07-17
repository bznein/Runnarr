package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const garminHealthDefaultBackfillDays = 90
const garminHealthScheduledRefreshDays = 14

type GarminHealthSyncOptions struct {
	From time.Time
	To   time.Time
}

func (s *GarminService) SyncHealth(ctx context.Context, opts GarminHealthSyncOptions, progress GarminSyncProgress) (map[string]any, error) {
	if progress == nil {
		progress = func(map[string]any) {}
	}
	if _, connected, err := s.Status(ctx); err != nil {
		return nil, err
	} else if !connected {
		return nil, errors.New("Garmin is not connected")
	}
	if err := os.MkdirAll(s.tokenDir, 0o700); err != nil {
		return nil, fmt.Errorf("could not prepare Garmin token storage: %w", err)
	}

	from, to, err := garminHealthSyncRange(opts, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	days := daysInclusive(from, to)
	saved := 0
	failed := 0
	firstErrors := make([]string, 0, 5)
	progress(map[string]any{"provider": garminProvider, "kind": "health", "stage": "Fetching Garmin health", "days": days, "processed": 0, "saved": 0, "failed": 0, "from": from.Format("2006-01-02"), "to": to.Format("2006-01-02")})

	processed := 0
	for current := from; !current.After(to); current = current.AddDate(0, 0, 1) {
		currentDate := current.Format("2006-01-02")
		progress(map[string]any{"provider": garminProvider, "kind": "health", "stage": "Fetching Garmin health", "days": days, "processed": processed, "saved": saved, "failed": failed, "currentDate": currentDate, "from": from.Format("2006-01-02"), "to": to.Format("2006-01-02")})

		day, err := s.bridge.FetchHealthDay(ctx, s.tokenDir, currentDate)
		if err != nil {
			failed++
			firstErrors = appendGarminHealthSyncError(firstErrors, currentDate, err)
			processed++
			progress(map[string]any{"provider": garminProvider, "kind": "health", "stage": "Fetching Garmin health", "days": days, "processed": processed, "saved": saved, "failed": failed, "currentDate": currentDate, "firstErrors": firstErrors, "from": from.Format("2006-01-02"), "to": to.Format("2006-01-02")})
			continue
		}
		if strings.TrimSpace(day.Date) == "" {
			day.Date = currentDate
		}
		metric := normalizeGarminHealthDay(day)
		if _, err := s.store.UpsertDailyHealthMetric(ctx, metric); err != nil {
			failed++
			firstErrors = appendGarminHealthSyncError(firstErrors, currentDate, err)
			processed++
			progress(map[string]any{"provider": garminProvider, "kind": "health", "stage": "Saving Garmin health", "days": days, "processed": processed, "saved": saved, "failed": failed, "currentDate": currentDate, "firstErrors": firstErrors, "from": from.Format("2006-01-02"), "to": to.Format("2006-01-02")})
			continue
		}
		saved++
		processed++
		progress(map[string]any{"provider": garminProvider, "kind": "health", "stage": "Saving Garmin health", "days": days, "processed": processed, "saved": saved, "failed": failed, "currentDate": currentDate, "firstErrors": firstErrors, "from": from.Format("2006-01-02"), "to": to.Format("2006-01-02")})
	}

	return map[string]any{
		"provider":    garminProvider,
		"kind":        "health",
		"stage":       "Completed",
		"days":        days,
		"processed":   processed,
		"saved":       saved,
		"failed":      failed,
		"firstErrors": firstErrors,
		"from":        from.Format("2006-01-02"),
		"to":          to.Format("2006-01-02"),
	}, nil
}

func garminHealthSyncRange(opts GarminHealthSyncOptions, now time.Time) (time.Time, time.Time, error) {
	to := dateOnly(opts.To)
	if to.IsZero() {
		to = dateOnly(now)
	}
	from := dateOnly(opts.From)
	if from.IsZero() {
		from = to.AddDate(0, 0, -(garminHealthDefaultBackfillDays - 1))
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, errors.New("from must be before or equal to to")
	}
	return from, to, nil
}

func dateOnly(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func daysInclusive(from, to time.Time) int {
	if from.After(to) {
		return 0
	}
	return int(to.Sub(from).Hours()/24) + 1
}

func appendGarminHealthSyncError(firstErrors []string, cdate string, err error) []string {
	if len(firstErrors) >= 5 {
		return firstErrors
	}
	return append(firstErrors, cdate+": "+err.Error())
}

func normalizeGarminHealthDay(day GarminBridgeHealthDay) DailyHealthMetric {
	raw := day.Raw
	if raw == nil {
		raw = map[string]any{}
	}
	date := strings.TrimSpace(day.Date)
	if date == "" {
		if rawDate, ok := raw["date"].(string); ok {
			date = strings.TrimSpace(rawDate)
		}
	}

	metric := DailyHealthMetric{
		Provider: garminProvider,
		Date:     date,
		Raw:      raw,
	}
	metric.Steps = firstInt(raw, "totalSteps", "totalStepCount", "stepCount", "steps")
	metric.TotalCaloriesKcal = firstInt(raw, "totalKilocalories", "totalCalories", "totalCalorieCount")
	metric.ActiveCaloriesKcal = firstInt(raw, "activeKilocalories", "activeCalories", "activeCalorieCount")
	metric.RestingHeartRateBPM = firstFloat(raw, "restingHeartRate", "restingHR", "restingHeartRateBpm")
	if metric.RestingHeartRateBPM == nil {
		metric.RestingHeartRateBPM = firstFloat(rawValue(raw, "restingHeartRate"), "value")
	}
	metric.AvgHeartRateBPM = firstFloat(raw, "averageHeartRate", "avgHeartRate", "averageHR")
	metric.MaxHeartRateBPM = firstFloat(raw, "maxHeartRate", "maximumHeartRate")

	metric.SleepDurationS = firstDurationSeconds(raw, "sleepTimeSeconds", "sleepDurationSeconds", "totalSleepSeconds", "totalSleepDuration", "sleepDuration")
	metric.DeepSleepS = firstDurationSeconds(raw, "deepSleepSeconds", "deepSleepDuration")
	metric.LightSleepS = firstDurationSeconds(raw, "lightSleepSeconds", "lightSleepDuration")
	metric.REMSleepS = firstDurationSeconds(raw, "remSleepSeconds", "remSleepDuration")
	metric.AwakeSleepS = firstDurationSeconds(raw, "awakeSleepSeconds", "awakeSleepDuration")
	metric.SleepScore = firstFloat(raw, "sleepScore", "overallSleepScore")
	if metric.SleepScore == nil {
		metric.SleepScore = sleepScoreFromRaw(raw)
	}

	metric.StressAvg = firstFloat(raw, "avgStressLevel", "averageStressLevel", "stressAvg")
	metric.StressMax = firstFloat(raw, "maxStressLevel", "stressMax")

	bodyBatteryValues := boundedSeriesValues(raw, 0, 100, "bodyBatteryValuesArray", "bodyBatteryValues")
	metric.BodyBatteryAvg = firstFloat(raw, "averageBodyBattery", "avgBodyBattery", "bodyBatteryAvg")
	metric.BodyBatteryMin = firstFloat(raw, "minBodyBattery", "bodyBatteryMin", "lowestBodyBattery")
	metric.BodyBatteryMax = firstFloat(raw, "maxBodyBattery", "bodyBatteryMax", "highestBodyBattery")
	if len(bodyBatteryValues) > 0 {
		summary := numberSummary(bodyBatteryValues)
		start := bodyBatteryValues[0]
		end := bodyBatteryValues[len(bodyBatteryValues)-1]
		metric.BodyBatteryStart = &start
		metric.BodyBatteryEnd = &end
		if metric.BodyBatteryAvg == nil {
			metric.BodyBatteryAvg = &summary.avg
		}
		if metric.BodyBatteryMin == nil {
			metric.BodyBatteryMin = &summary.min
		}
		if metric.BodyBatteryMax == nil {
			metric.BodyBatteryMax = &summary.max
		}
	}

	hrvRaw := rawValue(raw, "hrv")
	metric.HRVAvgMS = firstFloat(hrvRaw, "lastNightAvg", "weeklyAvg", "averageHrv", "avgHrv", "hrvAvg", "hrvValue")
	metric.HRVStatus = firstString(hrvRaw, "hrvStatus", "status", "feedbackPhrase")
	metric.WeightKG = firstFloat(raw, "weightKg", "bodyWeightKg", "weight", "bodyWeight")
	metric.BodyFatPct = firstFloat(raw, "bodyFatPct", "bodyFatPercentage", "bodyFat", "percentFat")
	return metric
}

func rawValue(root any, key string) any {
	target := normalizeMetricKey(key)
	var search func(any) any
	search = func(value any) any {
		switch typed := value.(type) {
		case map[string]any:
			for itemKey, itemValue := range typed {
				if normalizeMetricKey(itemKey) == target {
					return itemValue
				}
			}
			for _, itemValue := range typed {
				if found := search(itemValue); found != nil {
					return found
				}
			}
		case []any:
			for _, item := range typed {
				if found := search(item); found != nil {
					return found
				}
			}
		}
		return nil
	}
	return search(root)
}

func firstInt(root any, keys ...string) *int {
	value := firstFloat(root, keys...)
	if value == nil {
		return nil
	}
	rounded := int(math.Round(*value))
	return &rounded
}

func firstDurationSeconds(root any, keys ...string) *int {
	value := firstFloat(root, keys...)
	if value == nil {
		return nil
	}
	seconds := *value
	if seconds > 172800 {
		seconds = seconds / 1000
	}
	rounded := int(math.Round(seconds))
	return &rounded
}

func firstFloat(root any, keys ...string) *float64 {
	for _, key := range keys {
		if value := findNumberByKey(root, key); value != nil {
			return value
		}
	}
	return nil
}

func firstString(root any, keys ...string) string {
	for _, key := range keys {
		if value := findStringByKey(root, key); value != "" {
			return value
		}
	}
	return ""
}

func findNumberByKey(root any, key string) *float64 {
	target := normalizeMetricKey(key)
	var search func(any) *float64
	search = func(value any) *float64 {
		switch typed := value.(type) {
		case map[string]any:
			for itemKey, itemValue := range typed {
				if normalizeMetricKey(itemKey) == target {
					if number, ok := numberValue(itemValue); ok {
						return &number
					}
				}
			}
			for _, itemValue := range typed {
				if number := search(itemValue); number != nil {
					return number
				}
			}
		case []any:
			for _, item := range typed {
				if number := search(item); number != nil {
					return number
				}
			}
		}
		return nil
	}
	return search(root)
}

func findStringByKey(root any, key string) string {
	target := normalizeMetricKey(key)
	var search func(any) string
	search = func(value any) string {
		switch typed := value.(type) {
		case map[string]any:
			for itemKey, itemValue := range typed {
				if normalizeMetricKey(itemKey) == target {
					if text, ok := itemValue.(string); ok {
						return strings.TrimSpace(text)
					}
				}
			}
			for _, itemValue := range typed {
				if text := search(itemValue); text != "" {
					return text
				}
			}
		case []any:
			for _, item := range typed {
				if text := search(item); text != "" {
					return text
				}
			}
		}
		return ""
	}
	return search(root)
}

func sleepScoreFromRaw(root any) *float64 {
	sleepScores := rawValue(root, "sleepScores")
	overall := rawValue(sleepScores, "overall")
	return firstFloat(overall, "value")
}

func boundedSeriesValues(root any, minValue, maxValue float64, keys ...string) []float64 {
	values := make([]float64, 0)
	for _, key := range keys {
		for _, value := range seriesValuesByKey(root, key) {
			if value >= minValue && value <= maxValue {
				values = append(values, value)
			}
		}
	}
	return values
}

func seriesValuesByKey(root any, key string) []float64 {
	target := normalizeMetricKey(key)
	out := make([]float64, 0)
	var search func(any)
	search = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for itemKey, itemValue := range typed {
				if normalizeMetricKey(itemKey) == target {
					out = append(out, extractSeriesNumbers(itemValue)...)
				}
			}
			for _, itemValue := range typed {
				search(itemValue)
			}
		case []any:
			for _, item := range typed {
				search(item)
			}
		}
	}
	search(root)
	return out
}

func extractSeriesNumbers(value any) []float64 {
	switch typed := value.(type) {
	case []any:
		if len(typed) >= 2 {
			if number, ok := numberValue(typed[1]); ok {
				return []float64{number}
			}
		}
		out := make([]float64, 0)
		for _, item := range typed {
			out = append(out, extractSeriesNumbers(item)...)
		}
		return out
	case map[string]any:
		for _, key := range []string{"value", "bodyBattery", "bodyBatteryValue"} {
			if number := findNumberByKey(typed, key); number != nil {
				return []float64{*number}
			}
		}
	}
	if number, ok := numberValue(value); ok {
		return []float64{number}
	}
	return nil
}

type numericSummary struct {
	min float64
	max float64
	avg float64
}

func numberSummary(values []float64) numericSummary {
	if len(values) == 0 {
		return numericSummary{}
	}
	summary := numericSummary{min: values[0], max: values[0]}
	total := 0.0
	for _, value := range values {
		if value < summary.min {
			summary.min = value
		}
		if value > summary.max {
			summary.max = value
		}
		total += value
	}
	summary.avg = total / float64(len(values))
	return summary
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return finiteNumber(typed)
	case float32:
		return finiteNumber(float64(typed))
	case int:
		return finiteNumber(float64(typed))
	case int64:
		return finiteNumber(float64(typed))
	case int32:
		return finiteNumber(float64(typed))
	case uint64:
		return finiteNumber(float64(typed))
	case uint:
		return finiteNumber(float64(typed))
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
		return finiteNumber(parsed)
	default:
		return 0, false
	}
}

func finiteNumber(value float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

func normalizeMetricKey(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
