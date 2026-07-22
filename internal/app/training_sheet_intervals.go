package app

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	trainingSheetMetricAvgPace  = "avgPace"
	trainingSheetMetricAvgHeart = "avgHeartRate"
	trainingSheetMetricMaxHeart = "maxHeartRate"
	trainingSheetMetricRepeatNo = "repeatNumber"
	trainingSheetRowExact       = "exact"
	trainingSheetRowAverage     = "average"
	trainingSheetRowFastest     = "fastest"
	trainingSheetRowSlowest     = "slowest"
)

type trainingSheetWorkoutTable struct {
	HeaderRow int                            `json:"headerRow"`
	Columns   map[string]string              `json:"columns"`
	Rows      []trainingSheetWorkoutTableRow `json:"rows"`
}

type trainingSheetWorkoutTableRow struct {
	Row     int    `json:"row"`
	Label   string `json:"label"`
	Kind    string `json:"kind"`
	Group   string `json:"group,omitempty"`
	Ordinal int    `json:"ordinal,omitempty"`
}

type trainingSheetSection struct {
	Start int
	End   int
	Days  []int
}

var (
	trainingSheetDurationPattern  = regexp.MustCompile(`(?i)\b(\d+)\s*(?:min|mins|minute|minutes)\b`)
	trainingSheetSecondsPattern   = regexp.MustCompile(`(?i)\b(\d+)\s*(?:s|sec|secs|second|seconds)\b`)
	trainingSheetAtPattern        = regexp.MustCompile(`^\s*(\d+)\s*@`)
	trainingSheetRangePattern     = regexp.MustCompile(`^\s*(\d+)\s*-\s*(\d+)\s*$`)
	trainingSheetDateRangePattern = regexp.MustCompile(`^\s*(?:\d{4}[-/]\s*)?(\d{1,2})[-/]\s*(\d{1,2})\s*$`)
	trainingSheetUSDatePattern    = regexp.MustCompile(`^\s*(\d{1,2})[-/](\d{1,2})[-/]\d{4}\s*$`)
	trainingSheetOrdinalPattern   = regexp.MustCompile(`(?i)\b(?:rep|set)\s*(\d+)\b`)
)

func workoutTableForDay(values [][]string, day int) *trainingSheetWorkoutTable {
	sections := trainingSheetSections(values)
	for _, section := range sections {
		if !containsInt(section.Days, day) {
			continue
		}
		if table := workoutTableInSection(values, section); table != nil {
			return table
		}
	}
	return nil
}

func trainingSheetSections(values [][]string) []trainingSheetSection {
	starts := make([]int, 0)
	for rowIndex := 2; rowIndex < len(values); rowIndex++ {
		text := strings.TrimSpace(cellValue(values[rowIndex], 0))
		colon := strings.Index(text, ":")
		if colon <= 0 || len(parseDayScope(strings.TrimSpace(text[:colon]))) == 0 {
			continue
		}
		starts = append(starts, rowIndex)
	}
	sections := make([]trainingSheetSection, 0, len(starts))
	for index, start := range starts {
		end := len(values)
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		text := strings.TrimSpace(cellValue(values[start], 0))
		colon := strings.Index(text, ":")
		sections = append(sections, trainingSheetSection{Start: start, End: end, Days: parseDayScope(strings.TrimSpace(text[:colon]))})
	}
	return sections
}

func workoutTableInSection(values [][]string, section trainingSheetSection) *trainingSheetWorkoutTable {
	for rowIndex := section.Start + 1; rowIndex < section.End; rowIndex++ {
		if strings.EqualFold(strings.TrimSpace(cellValue(values[rowIndex], 0)), "How did it feel/go?") {
			break
		}
		columns := workoutTableColumns(values[rowIndex])
		if len(columns) == 0 {
			continue
		}
		rows := make([]trainingSheetWorkoutTableRow, 0)
		currentGroup := ""
		for dataRow := rowIndex + 1; dataRow < section.End; dataRow++ {
			label := strings.TrimSpace(cellValue(values[dataRow], 0))
			if strings.EqualFold(label, "How did it feel/go?") {
				break
			}
			if label == "" {
				continue
			}
			kind := workoutTableRowKind(label)
			if kind == "" {
				continue
			}
			group := workoutTableRowGroup(label)
			if group == "" && (kind == trainingSheetRowFastest || kind == trainingSheetRowSlowest) {
				group = currentGroup
			}
			if group != "" {
				currentGroup = group
			}
			ordinal := 0
			if match := trainingSheetOrdinalPattern.FindStringSubmatch(label); len(match) == 2 {
				ordinal, _ = strconv.Atoi(match[1])
			}
			rows = append(rows, trainingSheetWorkoutTableRow{Row: dataRow + 1, Label: label, Kind: kind, Group: group, Ordinal: ordinal})
		}
		if len(rows) == 0 {
			continue
		}
		return &trainingSheetWorkoutTable{HeaderRow: rowIndex + 1, Columns: columns, Rows: rows}
	}
	return nil
}

func workoutTableColumns(row []string) map[string]string {
	columns := make(map[string]string)
	for index, value := range row {
		switch normalizeSheetHeader(value) {
		case "avg pace":
			columns[trainingSheetMetricAvgPace] = spreadsheetColumn(index + 1)
		case "avg hr":
			columns[trainingSheetMetricAvgHeart] = spreadsheetColumn(index + 1)
		case "hr max":
			columns[trainingSheetMetricMaxHeart] = spreadsheetColumn(index + 1)
		case "rep no", "repeat no", "repetition no":
			columns[trainingSheetMetricRepeatNo] = spreadsheetColumn(index + 1)
		}
	}
	if columns[trainingSheetMetricAvgPace] == "" && columns[trainingSheetMetricAvgHeart] == "" && columns[trainingSheetMetricMaxHeart] == "" {
		return nil
	}
	return columns
}

func normalizeSheetHeader(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func workoutTableRowKind(label string) string {
	normalized := strings.ToLower(strings.TrimSpace(label))
	switch {
	case strings.Contains(normalized, "fastest") || strings.Contains(normalized, "fasest"):
		return trainingSheetRowFastest
	case strings.Contains(normalized, "slowest"):
		return trainingSheetRowSlowest
	case strings.Contains(normalized, "avg") && workoutTableRowGroup(label) != "":
		return trainingSheetRowAverage
	case workoutTableRowGroup(label) != "":
		return trainingSheetRowExact
	default:
		return ""
	}
}

func workoutTableRowGroup(label string) string {
	trimmed := strings.TrimSpace(label)
	if match := trainingSheetRangePattern.FindStringSubmatch(trimmed); len(match) == 3 {
		return fmt.Sprintf("range:%s-%s", match[1], match[2])
	}
	if match := trainingSheetUSDatePattern.FindStringSubmatch(trimmed); len(match) == 3 {
		start, startErr := strconv.Atoi(match[1])
		end, endErr := strconv.Atoi(match[2])
		if startErr == nil && endErr == nil {
			return fmt.Sprintf("range:%d-%d", start, end)
		}
	}
	if match := trainingSheetDateRangePattern.FindStringSubmatch(trimmed); len(match) == 3 && strings.ContainsAny(trimmed, "-/") {
		start, startErr := strconv.Atoi(match[1])
		end, endErr := strconv.Atoi(match[2])
		if startErr == nil && endErr == nil {
			return fmt.Sprintf("range:%d-%d", start, end)
		}
	}
	if match := trainingSheetDurationPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		return "duration:" + match[1] + "m"
	}
	if match := trainingSheetSecondsPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		return "duration:" + match[1] + "s"
	}
	if match := trainingSheetAtPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		return "duration:" + match[1] + "m"
	}
	return ""
}

func trainingSheetGroupSeconds(group string) (int, bool) {
	if strings.HasPrefix(group, "duration:") {
		value := strings.TrimPrefix(group, "duration:")
		if strings.HasSuffix(value, "m") {
			seconds, err := strconv.Atoi(strings.TrimSuffix(value, "m"))
			return seconds * 60, err == nil && seconds > 0
		}
		if strings.HasSuffix(value, "s") {
			seconds, err := strconv.Atoi(strings.TrimSuffix(value, "s"))
			return seconds, err == nil && seconds > 0
		}
	}
	if strings.HasPrefix(group, "range:") {
		parts := strings.Split(strings.TrimPrefix(group, "range:"), "-")
		if len(parts) == 2 {
			start, startErr := strconv.Atoi(parts[0])
			end, endErr := strconv.Atoi(parts[1])
			if startErr == nil && endErr == nil && end > start {
				return (end - start) * 60, true
			}
		}
	}
	return 0, false
}

func workoutTableFromPlanned(planned PlannedActivity) *trainingSheetWorkoutTable {
	if planned.Raw != nil {
		if rawTable, ok := planned.Raw["workoutTable"]; ok {
			encoded, err := json.Marshal(rawTable)
			if err == nil {
				var table trainingSheetWorkoutTable
				if json.Unmarshal(encoded, &table) == nil && len(table.Rows) > 0 {
					return &table
				}
			}
		}
		if values := stringMatrixFromAny(planned.Raw["values"]); len(values) > 0 {
			column := strings.TrimRight(planned.PlanCell, "0123456789")
			if len(column) == 1 && column[0] >= 'B' && column[0] <= 'H' {
				return workoutTableForDay(values, int(column[0]-'B'))
			}
		}
	}
	return nil
}

func stringMatrixFromAny(value any) [][]string {
	if values, ok := value.([][]string); ok {
		return values
	}
	rows, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([][]string, 0, len(rows))
	for _, rawRow := range rows {
		rowValues, ok := rawRow.([]any)
		if !ok {
			continue
		}
		row := make([]string, len(rowValues))
		for index, rawValue := range rowValues {
			if rawValue != nil {
				row[index] = fmt.Sprint(rawValue)
			}
		}
		result = append(result, row)
	}
	return result
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type trainingSheetWorkoutRecord struct {
	DurationS    int
	MovingTimeS  int
	DistanceM    float64
	AvgPaceSPKM  *float64
	AvgHeartRate *float64
	MaxHeartRate *float64
	StepIndex    *int
	RepeatNumber int
}

func structuredWorkoutRecords(activity Activity, expandLaps bool) []trainingSheetWorkoutRecord {
	if activity.Workout == nil || len(activity.Intervals) == 0 {
		return nil
	}
	records := make([]trainingSheetWorkoutRecord, 0, len(activity.Intervals))
	for _, interval := range activity.Intervals {
		if !strings.EqualFold(strings.TrimSpace(interval.Category), "active") {
			continue
		}
		if expandLaps && len(interval.LapIndexes) > 1 {
			for _, lapIndex := range interval.LapIndexes {
				if lapIndex < 0 || lapIndex >= len(activity.Laps) {
					return nil
				}
				lap := activity.Laps[lapIndex]
				repeat := interval.WorkoutRepeatIndex
				if lap.WorkoutRepeatIndex != nil {
					repeat = lap.WorkoutRepeatIndex
				}
				records = append(records, trainingSheetWorkoutRecord{
					DurationS: lapDuration(lap), MovingTimeS: lap.MovingTimeS, DistanceM: lap.DistanceM,
					AvgPaceSPKM: lap.AvgPaceSPKM, AvgHeartRate: lap.AvgHeartRate, MaxHeartRate: lap.MaxHeartRate,
					StepIndex:    interval.WorkoutStepIndex,
					RepeatNumber: intValue(repeat),
				})
			}
			continue
		}
		records = append(records, trainingSheetWorkoutRecord{
			DurationS: intervalDuration(interval), MovingTimeS: interval.MovingTimeS, DistanceM: interval.DistanceM,
			AvgPaceSPKM: interval.AvgPaceSPKM, AvgHeartRate: interval.AvgHeartRate, MaxHeartRate: interval.MaxHeartRate,
			StepIndex:    interval.WorkoutStepIndex,
			RepeatNumber: intValue(interval.WorkoutRepeatIndex),
		})
	}
	return records
}

func intervalDuration(interval ActivityInterval) int {
	if interval.MovingTimeS > 0 {
		return interval.MovingTimeS
	}
	return interval.ElapsedTimeS
}

func lapDuration(lap ActivityLap) int {
	if lap.MovingTimeS > 0 {
		return lap.MovingTimeS
	}
	return lap.ElapsedTimeS
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func durationMatches(group string, duration int) bool {
	expected, ok := trainingSheetGroupSeconds(group)
	if !ok || duration <= 0 {
		return true
	}
	tolerance := int(math.Round(float64(expected) * 0.25))
	if tolerance < 15 {
		tolerance = 15
	}
	return absInt(expected-duration) <= tolerance
}

func intervalUpdatesForPlannedActivity(planned PlannedActivity, activity Activity) ([]googleValueRangeUpdate, error) {
	table := workoutTableFromPlanned(planned)
	if table == nil {
		return nil, nil
	}
	if activity.Workout == nil || len(activity.Intervals) == 0 {
		return nil, fmt.Errorf("the activity has no structured Garmin workout intervals")
	}

	records := structuredWorkoutRecords(activity, false)
	updates, err := intervalUpdatesForRecords(planned.SheetTitle, table, records)
	if err == nil {
		return updates, nil
	}
	if !workoutTableHasRangeRows(table) {
		return nil, err
	}

	expandedRecords := structuredWorkoutRecords(activity, true)
	if len(expandedRecords) <= len(records) {
		return nil, err
	}
	return intervalUpdatesForRecords(planned.SheetTitle, table, expandedRecords)
}

func workoutTableHasRangeRows(table *trainingSheetWorkoutTable) bool {
	for _, row := range table.Rows {
		if strings.HasPrefix(row.Group, "range:") {
			return true
		}
	}
	return false
}

func intervalUpdatesForRecords(sheetTitle string, table *trainingSheetWorkoutTable, records []trainingSheetWorkoutRecord) ([]googleValueRangeUpdate, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("the activity has no active structured workout intervals")
	}
	assignments := make([][]trainingSheetWorkoutRecord, len(table.Rows))
	cursor := 0
	currentGroup := ""
	var currentRecords []trainingSheetWorkoutRecord
	for rowIndex, row := range table.Rows {
		switch row.Kind {
		case trainingSheetRowFastest, trainingSheetRowSlowest:
			if currentGroup == "" || len(currentRecords) == 0 {
				return nil, fmt.Errorf("%s row %q has no preceding interval group", row.Kind, row.Label)
			}
			assignments[rowIndex] = currentRecords
		case trainingSheetRowExact:
			if cursor >= len(records) {
				return nil, fmt.Errorf("worksheet row %q has no matching structured interval", row.Label)
			}
			if row.Group != "" && !durationMatches(row.Group, records[cursor].DurationS) {
				return nil, fmt.Errorf("worksheet row %q does not match the next structured interval", row.Label)
			}
			assignments[rowIndex] = records[cursor : cursor+1]
			currentGroup = row.Group
			currentRecords = assignments[rowIndex]
			cursor++
		case trainingSheetRowAverage:
			group := row.Group
			if group == "" {
				group = currentGroup
			}
			if group == "" {
				return nil, fmt.Errorf("average row %q has no interval group", row.Label)
			}
			start := cursor
			nextGroup := nextConsumedWorkoutGroup(table.Rows, rowIndex, group)
			groupStep := records[start].StepIndex
			for cursor < len(records) {
				matchesCurrent := durationMatches(group, records[cursor].DurationS)
				matchesNext := nextGroup != "" && durationMatches(nextGroup, records[cursor].DurationS)
				stepChanged := cursor > start && groupStep != nil && records[cursor].StepIndex != nil && *groupStep != *records[cursor].StepIndex
				if !matchesCurrent || (cursor > start && (matchesNext || stepChanged)) {
					break
				}
				cursor++
			}
			if cursor == start {
				return nil, fmt.Errorf("average row %q has no matching structured intervals", row.Label)
			}
			currentGroup = group
			currentRecords = records[start:cursor]
			assignments[rowIndex] = currentRecords
		default:
			return nil, fmt.Errorf("unsupported worksheet row kind %q", row.Kind)
		}
	}
	if cursor != len(records) {
		return nil, fmt.Errorf("worksheet table does not account for all active structured intervals")
	}

	updates := make([]googleValueRangeUpdate, 0, len(table.Rows)*len(table.Columns))
	for index, row := range table.Rows {
		selected := assignments[index]
		switch row.Kind {
		case trainingSheetRowExact:
			updates = append(updates, workoutRecordUpdates(sheetTitle, table, row, selected[0])...)
		case trainingSheetRowAverage:
			aggregate, err := aggregateWorkoutRecords(selected)
			if err != nil {
				return nil, fmt.Errorf("%s row %q: %w", trainingSheetRowAverage, row.Label, err)
			}
			updates = append(updates, workoutRecordUpdates(sheetTitle, table, row, aggregate)...)
		case trainingSheetRowFastest, trainingSheetRowSlowest:
			selectedRecord, err := fastestOrSlowestWorkoutRecord(selected, row.Kind == trainingSheetRowFastest)
			if err != nil {
				return nil, fmt.Errorf("%s row %q: %w", row.Kind, row.Label, err)
			}
			updates = append(updates, workoutRecordUpdates(sheetTitle, table, row, *selectedRecord)...)
		}
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("worksheet table has no available interval metrics")
	}
	return updates, nil
}

func nextConsumedWorkoutGroup(rows []trainingSheetWorkoutTableRow, start int, current string) string {
	for index := start + 1; index < len(rows); index++ {
		row := rows[index]
		if row.Kind == trainingSheetRowFastest || row.Kind == trainingSheetRowSlowest {
			continue
		}
		if row.Group != "" && row.Group != current {
			return row.Group
		}
	}
	return ""
}

func workoutRecordUpdates(sheetTitle string, table *trainingSheetWorkoutTable, row trainingSheetWorkoutTableRow, record trainingSheetWorkoutRecord) []googleValueRangeUpdate {
	updates := make([]googleValueRangeUpdate, 0, len(table.Columns))
	add := func(column string, value any) {
		if column == "" {
			return
		}
		updates = append(updates, googleValueRangeUpdate{Range: sheetCellRange(sheetTitle, fmt.Sprintf("%s%d", column, row.Row)), Values: [][]any{{value}}})
	}
	if pace := workoutRecordPace(record); pace != nil {
		add(table.Columns[trainingSheetMetricAvgPace], "'"+sheetPaceText(*pace))
	}
	if record.AvgHeartRate != nil {
		add(table.Columns[trainingSheetMetricAvgHeart], sheetIntegerText(*record.AvgHeartRate))
	}
	if record.MaxHeartRate != nil {
		add(table.Columns[trainingSheetMetricMaxHeart], sheetIntegerText(*record.MaxHeartRate))
	}
	if record.RepeatNumber > 0 && (row.Kind == trainingSheetRowFastest || row.Kind == trainingSheetRowSlowest) {
		add(table.Columns[trainingSheetMetricRepeatNo], record.RepeatNumber)
	}
	return updates
}

func sheetIntegerText(value float64) string {
	return "'" + strconv.Itoa(int(math.Round(value)))
}

func workoutRecordPace(record trainingSheetWorkoutRecord) *float64 {
	if record.AvgPaceSPKM != nil && *record.AvgPaceSPKM > 0 {
		return record.AvgPaceSPKM
	}
	if record.MovingTimeS > 0 && record.DistanceM > 0 {
		pace := float64(record.MovingTimeS) / record.DistanceM * 1000
		return &pace
	}
	return nil
}

func aggregateWorkoutRecords(records []trainingSheetWorkoutRecord) (trainingSheetWorkoutRecord, error) {
	if len(records) == 0 {
		return trainingSheetWorkoutRecord{}, fmt.Errorf("no records")
	}
	result := trainingSheetWorkoutRecord{}
	var paceWeighted, paceWeight, movingTime, distance float64
	var heartWeighted, heartWeight float64
	for _, record := range records {
		weight := record.MovingTimeS
		if weight <= 0 {
			weight = record.DurationS
		}
		if weight <= 0 {
			weight = 1
		}
		movingTime += float64(record.MovingTimeS)
		distance += record.DistanceM
		if pace := workoutRecordPace(record); pace != nil {
			paceWeighted += *pace * float64(weight)
			paceWeight += float64(weight)
		}
		if record.AvgHeartRate != nil {
			heartWeighted += *record.AvgHeartRate * float64(weight)
			heartWeight += float64(weight)
		}
		if record.MaxHeartRate != nil && (result.MaxHeartRate == nil || *record.MaxHeartRate > *result.MaxHeartRate) {
			value := *record.MaxHeartRate
			result.MaxHeartRate = &value
		}
	}
	if movingTime > 0 && distance > 0 {
		pace := movingTime / distance * 1000
		result.AvgPaceSPKM = &pace
	} else if paceWeight > 0 {
		pace := paceWeighted / paceWeight
		result.AvgPaceSPKM = &pace
	}
	if heartWeight > 0 {
		heart := heartWeighted / heartWeight
		result.AvgHeartRate = &heart
	}
	if result.AvgPaceSPKM == nil && result.AvgHeartRate == nil && result.MaxHeartRate == nil {
		return trainingSheetWorkoutRecord{}, fmt.Errorf("no available metrics")
	}
	return result, nil
}

func fastestOrSlowestWorkoutRecord(records []trainingSheetWorkoutRecord, fastest bool) (*trainingSheetWorkoutRecord, error) {
	var selected *trainingSheetWorkoutRecord
	for index := range records {
		pace := workoutRecordPace(records[index])
		if pace == nil {
			continue
		}
		if selected == nil {
			copy := records[index]
			selected = &copy
			continue
		}
		selectedPace := workoutRecordPace(*selected)
		if (fastest && *pace < *selectedPace) || (!fastest && *pace > *selectedPace) {
			copy := records[index]
			selected = &copy
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("no interval pace is available")
	}
	return selected, nil
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
