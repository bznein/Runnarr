package app

import (
	"testing"
	"time"
)

func TestPlannedActivitiesFromTabKeepsWorkbookScopedSourceIDs(t *testing.T) {
	tab := googleSheetTab{
		ID:    "week-tab",
		Title: "5-7",
		Values: [][]string{
			{"", "Mon", "Tue"},
			{"", "Easy run", "Intervals"},
		},
	}
	weekEnd := time.Date(2026, time.July, 5, 12, 0, 0, 0, time.UTC)

	oldWorkbook := plannedActivitiesFromTab("old-workbook", tab, weekEnd)
	newWorkbook := plannedActivitiesFromTab("new-workbook", tab, weekEnd)
	if len(oldWorkbook) != 2 || len(newWorkbook) != 2 {
		t.Fatalf("planned activities = %d/%d, want two activities per workbook", len(oldWorkbook), len(newWorkbook))
	}
	if oldWorkbook[0].SourceID == newWorkbook[0].SourceID {
		t.Fatalf("source ID = %q, want workbook-specific IDs to remain distinct", oldWorkbook[0].SourceID)
	}
	if oldWorkbook[0].WorkbookID != "old-workbook" || newWorkbook[0].WorkbookID != "new-workbook" {
		t.Fatalf("workbook IDs = %q/%q, want old/new workbook IDs", oldWorkbook[0].WorkbookID, newWorkbook[0].WorkbookID)
	}
}

func TestTrainingSheetPlanYearBoundsAreScopedToOneYear(t *testing.T) {
	start, end := trainingSheetPlanYearBounds(2026)

	if !start.Equal(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("start = %v, want start of 2026", start)
	}
	if !end.Equal(time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("end = %v, want start of 2027", end)
	}
}
