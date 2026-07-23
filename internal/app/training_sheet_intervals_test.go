package app

import (
	"testing"
	"time"
)

func TestWorkoutTableForDayParsesExactAndAggregateRows(t *testing.T) {
	values := [][]string{
		{" ", "Mon", "Tues", "Wed"},
		{"PLAN", "40mins", "", "5x3mins"},
		{"Wednesday:15mins easy warm up//5x3mins@3:30", "", "", ""},
		{"", "Avg Pace", "Avg HR", "HR MAX", "Rep No"},
		{"3min rep avg", "", "", "", ""},
		{"Fasest", "", "", "", ""},
		{"Slowest", "", "", "", ""},
		{"How did it feel/go?", "", "", "", ""},
	}

	table := workoutTableForDay(values, 2)
	if table == nil {
		t.Fatal("workout table = nil")
	}
	if table.HeaderRow != 4 {
		t.Fatalf("header row = %d, want 4", table.HeaderRow)
	}
	if table.Columns[trainingSheetMetricAvgPace] != "B" || table.Columns[trainingSheetMetricAvgHeart] != "C" || table.Columns[trainingSheetMetricMaxHeart] != "D" || table.Columns[trainingSheetMetricRepeatNo] != "E" {
		t.Fatalf("columns = %#v", table.Columns)
	}
	if len(table.Rows) != 3 || table.Rows[0].Kind != trainingSheetRowAverage || table.Rows[1].Kind != trainingSheetRowFastest || table.Rows[2].Kind != trainingSheetRowSlowest {
		t.Fatalf("rows = %#v", table.Rows)
	}
	if table.Rows[0].Group != "duration:3m" || table.Rows[1].Group != table.Rows[0].Group || table.Rows[2].Group != table.Rows[0].Group {
		t.Fatalf("row groups = %#v", table.Rows)
	}
}

func TestPlannedActivitiesFromTabStoresWorkoutTableMetadata(t *testing.T) {
	tab := googleSheetTab{ID: "123", Title: "05-07", Values: [][]string{
		{"", "Mon", "Tues", "Wed"},
		{"PLAN", "", "", "5x8mins"},
		{"Wednesday:10mins easy warm up//5x8mins@3:45", "", "", ""},
		{"", "Avg Pace", "Avg HR", "HR MAX"},
		{"8min rep 1", "", "", ""},
		{"How did it feel/go?", "", "", ""},
	}}
	activities := plannedActivitiesFromTab("workbook", tab, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if len(activities) != 1 {
		t.Fatalf("activities = %#v", activities)
	}
	if _, ok := activities[0].Raw["workoutTable"]; !ok {
		t.Fatalf("raw metadata = %#v", activities[0].Raw)
	}
}

func TestWorkoutTableRowGroupParsesDateFormattedRanges(t *testing.T) {
	for label, want := range map[string]string{
		"0-5":        "range:0-5",
		"2026-05-10": "range:5-10",
		"2026/10/15": "range:10-15",
		"5/10/2026":  "range:5-10",
	} {
		if got := workoutTableRowGroup(label); got != want {
			t.Fatalf("workoutTableRowGroup(%q) = %q, want %q", label, got, want)
		}
	}
}

func TestIntervalUpdatesForRecordsUsesWeightedAggregatesAndRepNumbers(t *testing.T) {
	table := &trainingSheetWorkoutTable{
		Columns: map[string]string{
			trainingSheetMetricAvgPace:  "B",
			trainingSheetMetricAvgHeart: "C",
			trainingSheetMetricMaxHeart: "D",
			trainingSheetMetricRepeatNo: "E",
		},
		Rows: []trainingSheetWorkoutTableRow{
			{Row: 5, Label: "3min rep avg", Kind: trainingSheetRowAverage, Group: "duration:3m"},
			{Row: 6, Label: "Fastest", Kind: trainingSheetRowFastest, Group: "duration:3m"},
			{Row: 7, Label: "Slowest", Kind: trainingSheetRowSlowest, Group: "duration:3m"},
		},
	}
	paceOne, paceTwo := 180.0, 200.0
	heartOne, heartTwo := 150.0, 160.0
	maxOne, maxTwo := 170.0, 175.0
	records := []trainingSheetWorkoutRecord{
		{DurationS: 180, MovingTimeS: 180, DistanceM: 1000, AvgPaceSPKM: &paceOne, AvgHeartRate: &heartOne, MaxHeartRate: &maxOne, RepeatNumber: 1},
		{DurationS: 180, MovingTimeS: 180, DistanceM: 900, AvgPaceSPKM: &paceTwo, AvgHeartRate: &heartTwo, MaxHeartRate: &maxTwo, RepeatNumber: 2},
	}

	updates, err := intervalUpdatesForRecords("Week", table, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 11 {
		t.Fatalf("update count = %d, want 11", len(updates))
	}
	if value := updateValue(updates, "'Week'!B5"); value != "'3:09" {
		t.Fatalf("weighted pace = %#v", value)
	}
	if value := updateValue(updates, "'Week'!C5"); value != "'155" {
		t.Fatalf("weighted heart rate = %#v", value)
	}
	if value := updateValue(updates, "'Week'!D5"); value != "'175" {
		t.Fatalf("aggregate max heart rate = %#v", value)
	}
	if value := updateValue(updates, "'Week'!E6"); value != 1 {
		t.Fatalf("fastest repetition = %#v", value)
	}
	if value := updateValue(updates, "'Week'!E7"); value != 2 {
		t.Fatalf("slowest repetition = %#v", value)
	}
}

func TestIntervalUpdatesForRecordsSplitsRepeatedSetsAtExactAnchors(t *testing.T) {
	table := &trainingSheetWorkoutTable{
		Columns: map[string]string{trainingSheetMetricAvgPace: "B"},
		Rows: []trainingSheetWorkoutTableRow{
			{Row: 1, Label: "5min rep 1", Kind: trainingSheetRowExact, Group: "duration:5m", Ordinal: 1},
			{Row: 2, Label: "1min set 1 avg", Kind: trainingSheetRowAverage, Group: "duration:1m", Ordinal: 1},
			{Row: 3, Label: "5min rep 2", Kind: trainingSheetRowExact, Group: "duration:5m", Ordinal: 2},
			{Row: 4, Label: "1min set 2 avg", Kind: trainingSheetRowAverage, Group: "duration:1m", Ordinal: 2},
			{Row: 5, Label: "5min rep 3", Kind: trainingSheetRowExact, Group: "duration:5m", Ordinal: 3},
		},
	}
	records := make([]trainingSheetWorkoutRecord, 0, 11)
	for _, pace := range []float64{300, 60, 61, 62, 63, 300, 64, 65, 66, 67, 300} {
		value := pace
		records = append(records, trainingSheetWorkoutRecord{DurationS: durationForPace(pace), MovingTimeS: durationForPace(pace), DistanceM: 1000, AvgPaceSPKM: &value})
	}

	updates, err := intervalUpdatesForRecords("Week", table, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 5 {
		t.Fatalf("update count = %d, want 5", len(updates))
	}
}

func TestIntervalUpdatesForContinuousTableExpandsStructuredLaps(t *testing.T) {
	table := &trainingSheetWorkoutTable{
		Columns: map[string]string{trainingSheetMetricAvgPace: "B"},
		Rows: []trainingSheetWorkoutTableRow{
			{Row: 3, Label: "0-5", Kind: trainingSheetRowExact, Group: "range:0-5"},
			{Row: 4, Label: "5-10", Kind: trainingSheetRowExact, Group: "range:5-10"},
		},
	}
	paceOne, paceTwo := 300.0, 290.0
	activity := Activity{
		Workout:   &ActivityWorkout{Provider: garminProvider},
		Intervals: []ActivityInterval{{Category: "active", MovingTimeS: 600, DistanceM: 2000, LapIndexes: []int{0, 1}}},
		Laps: []ActivityLap{
			{MovingTimeS: 300, DistanceM: 1000, AvgPaceSPKM: &paceOne},
			{MovingTimeS: 300, DistanceM: 1000, AvgPaceSPKM: &paceTwo},
		},
	}

	updates, err := intervalUpdatesForRecords("Week", table, structuredWorkoutRecords(activity, true))
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 || updateValue(updates, "'Week'!B4") != "'4:50" {
		t.Fatalf("updates = %#v", updates)
	}
}

func TestIntervalUpdatesForRecordsRejectsUnmappedRows(t *testing.T) {
	table := &trainingSheetWorkoutTable{
		Columns: map[string]string{trainingSheetMetricAvgPace: "B"},
		Rows:    []trainingSheetWorkoutTableRow{{Row: 1, Label: "5min rep 1", Kind: trainingSheetRowExact, Group: "duration:5m"}},
	}
	pace := 300.0
	_, err := intervalUpdatesForRecords("Week", table, []trainingSheetWorkoutRecord{{DurationS: 180, MovingTimeS: 180, DistanceM: 600, AvgPaceSPKM: &pace}})
	if err == nil {
		t.Fatal("expected unmapped row error")
	}
}

func TestIntervalUpdatesForRecordsReportsUnmatchedActiveIntervals(t *testing.T) {
	table := &trainingSheetWorkoutTable{
		Columns: map[string]string{trainingSheetMetricAvgPace: "B"},
		Rows: []trainingSheetWorkoutTableRow{
			{Row: 1, Label: "5min rep 1", Kind: trainingSheetRowExact, Group: "duration:5m"},
			{Row: 2, Label: "5min rep 2", Kind: trainingSheetRowExact, Group: "duration:5m"},
			{Row: 3, Label: "5min rep 3", Kind: trainingSheetRowExact, Group: "duration:5m"},
		},
	}
	records := make([]trainingSheetWorkoutRecord, 0, 11)
	for _, duration := range []int{300, 300, 300, 159, 40, 40, 40, 40, 40, 40, 40} {
		pace := 200.0
		records = append(records, trainingSheetWorkoutRecord{DurationS: duration, MovingTimeS: duration, DistanceM: 1000, AvgPaceSPKM: &pace})
	}

	plan, err := intervalUpdatesForRecordsWithWarning("Week", table, records)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SkippedRecord != 8 {
		t.Fatalf("skipped records = %d, want 8", plan.SkippedRecord)
	}
	if len(plan.Updates) != 3 {
		t.Fatalf("updates = %d, want 3", len(plan.Updates))
	}
	if _, err := intervalUpdatesForRecords("Week", table, records); err == nil {
		t.Fatal("strict mapping should reject unmatched active intervals")
	}
}

func updateValue(updates []googleValueRangeUpdate, rangeName string) any {
	for _, update := range updates {
		if update.Range != rangeName || len(update.Values) == 0 || len(update.Values[0]) == 0 {
			continue
		}
		value := update.Values[0][0]
		return value
	}
	return nil
}

func durationForPace(pace float64) int {
	if pace >= 300 {
		return 300
	}
	return 60
}
