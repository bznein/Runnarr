package app

import "time"

type TrainingSheetConfig struct {
	Enabled         bool   `json:"enabled"`
	SheetURL        string `json:"sheetURL"`
	CheckEveryHours int    `json:"checkEveryHours"`
	PlanYear        int    `json:"planYear,omitempty"`
	LastSyncedAt    string `json:"lastSyncedAt,omitempty"`
}

type PlannedActivity struct {
	ID          string         `json:"id"`
	Source      string         `json:"source"`
	SourceID    string         `json:"sourceId"`
	WorkbookID  string         `json:"workbookId"`
	SheetID     string         `json:"sheetId"`
	SheetTitle  string         `json:"sheetTitle"`
	PlanCell    string         `json:"planCell"`
	PlannedDate time.Time      `json:"plannedDate"`
	Name        string         `json:"name"`
	SportType   string         `json:"sportType"`
	Notes       string         `json:"notes,omitempty"`
	Status      string         `json:"status"`
	SourceURL   string         `json:"sourceUrl,omitempty"`
	Raw         map[string]any `json:"raw,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type GoogleSheetsStatus struct {
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	Provider   string `json:"provider"`
}
