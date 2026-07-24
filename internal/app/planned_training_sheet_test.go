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

func TestPlannedActivityStatusAfterUnmatchPreservesSupersededWorkbook(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		workbookID string
		currentID  string
		wantStatus string
	}{
		{name: "replaced training workbook", source: trainingSheetProvider, workbookID: "old", currentID: "new", wantStatus: plannedActivityStatusSuperseded},
		{name: "current training workbook", source: trainingSheetProvider, workbookID: "current", currentID: "current", wantStatus: plannedActivityStatusPending},
		{name: "unknown current workbook", source: trainingSheetProvider, workbookID: "old", currentID: "", wantStatus: plannedActivityStatusPending},
		{name: "other planned source", source: "other", workbookID: "old", currentID: "new", wantStatus: plannedActivityStatusPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := plannedActivityStatusAfterUnmatch(tt.source, tt.workbookID, tt.currentID); got != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got, tt.wantStatus)
			}
		})
	}
}
