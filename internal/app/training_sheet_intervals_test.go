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

func TestWorkoutTableColumnsRecognizesElevationColumns(t *testing.T) {
	columns := workoutTableColumns([]string{"", "Avg Pace", "Avg HR", "HR MAX", "Elev Gain", "Elevation Loss"})
	if columns[trainingSheetMetricElevationGain] != "E" || columns[trainingSheetMetricElevationLoss] != "F" {
		t.Fatalf("elevation columns = %#v, want gain E and loss F", columns)
	}

	combined := workoutTableColumns([]string{"", "Avg Pace", "Elevation Gain/Loss"})
	if combined[trainingSheetMetricElevation] != "C" {
		t.Fatalf("combined elevation column = %#v, want C", combined)
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
			trainingSheetMetricAvgPace:       "B",
			trainingSheetMetricAvgHeart:      "C",
			trainingSheetMetricMaxHeart:      "D",
			trainingSheetMetricRepeatNo:      "E",
			trainingSheetMetricElevationGain: "F",
			trainingSheetMetricElevationLoss: "G",
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
	gainOne, gainTwo := 10.0, 5.0
	lossOne, lossTwo := 4.0, 6.0
	records := []trainingSheetWorkoutRecord{
		{DurationS: 180, MovingTimeS: 180, DistanceM: 1000, AvgPaceSPKM: &paceOne, AvgHeartRate: &heartOne, MaxHeartRate: &maxOne, ElevationGainM: &gainOne, ElevationLossM: &lossOne, RepeatNumber: 1},
		{DurationS: 180, MovingTimeS: 180, DistanceM: 900, AvgPaceSPKM: &paceTwo, AvgHeartRate: &heartTwo, MaxHeartRate: &maxTwo, ElevationGainM: &gainTwo, ElevationLossM: &lossTwo, RepeatNumber: 2},
	}

	updates, err := intervalUpdatesForRecords("Week", table, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 17 {
		t.Fatalf("update count = %d, want 17", len(updates))
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
	if value := updateValue(updates, "'Week'!F5"); value != "'15" {
		t.Fatalf("aggregate elevation gain = %#v", value)
	}
	if value := updateValue(updates, "'Week'!G5"); value != "'10" {
		t.Fatalf("aggregate elevation loss = %#v", value)
	}
	if value := updateValue(updates, "'Week'!F6"); value != "'10" || updateValue(updates, "'Week'!G6") != "'4" {
		t.Fatalf("fastest elevation = %#v / %#v", value, updateValue(updates, "'Week'!G6"))
	}
	if value := updateValue(updates, "'Week'!F7"); value != "'5" || updateValue(updates, "'Week'!G7") != "'6" {
		t.Fatalf("slowest elevation = %#v / %#v", value, updateValue(updates, "'Week'!G7"))
	}
	if value := updateValue(updates, "'Week'!E6"); value != 1 {
		t.Fatalf("fastest repetition = %#v", value)
	}
	if value := updateValue(updates, "'Week'!E7"); value != 2 {
		t.Fatalf("slowest repetition = %#v", value)
	}
}

func TestStructuredWorkoutRecordsFallsBackToLapElevation(t *testing.T) {
	gainOne, gainTwo := 4.0, 6.0
	lossOne, lossTwo := 2.0, 3.0
	activity := Activity{
		Workout:   &ActivityWorkout{Provider: garminProvider},
		Intervals: []ActivityInterval{{Category: "active", LapIndexes: []int{0, 1}}},
		Laps: []ActivityLap{
			{ElevationGainM: &gainOne, ElevationLossM: &lossOne},
			{ElevationGainM: &gainTwo, ElevationLossM: &lossTwo},
		},
	}

	records := structuredWorkoutRecords(activity, false)
	if len(records) != 1 || records[0].ElevationGainM == nil || *records[0].ElevationGainM != 10 || records[0].ElevationLossM == nil || *records[0].ElevationLossM != 5 {
		t.Fatalf("records = %#v, want one record with 10m gain and 5m loss", records)
	}
}

func TestIntervalUpdatesForRecordsWritesCombinedElevationColumn(t *testing.T) {
	table := &trainingSheetWorkoutTable{
		Columns: map[string]string{trainingSheetMetricElevation: "E"},
		Rows:    []trainingSheetWorkoutTableRow{{Row: 3, Label: "5min rep 1", Kind: trainingSheetRowExact, Group: "duration:5m"}},
	}
	gain, loss := 12.4, 3.6
	updates, err := intervalUpdatesForRecords("Week", table, []trainingSheetWorkoutRecord{{DurationS: 300, ElevationGainM: &gain, ElevationLossM: &loss}})
	if err != nil {
		t.Fatal(err)
	}
	if value := updateValue(updates, "'Week'!E3"); value != "'+12/-4" {
		t.Fatalf("combined elevation = %#v, want '+12/-4", value)
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
