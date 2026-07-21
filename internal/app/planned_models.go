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
	ID                string         `json:"id"`
	Source            string         `json:"source"`
	SourceID          string         `json:"sourceId"`
	WorkbookID        string         `json:"workbookId"`
	SheetID           string         `json:"sheetId"`
	SheetTitle        string         `json:"sheetTitle"`
	PlanCell          string         `json:"planCell"`
	FeedbackCell      string         `json:"feedbackCell,omitempty"`
	PlannedDate       time.Time      `json:"plannedDate"`
	Name              string         `json:"name"`
	SportType         string         `json:"sportType"`
	Notes             string         `json:"notes,omitempty"`
	Status            string         `json:"status"`
	SourceURL         string         `json:"sourceUrl,omitempty"`
	MatchedActivityID string         `json:"matchedActivityId,omitempty"`
	MatchedAt         *time.Time     `json:"matchedAt,omitempty"`
	Raw               map[string]any `json:"raw,omitempty"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
}

type PlannedActivityMatchResponse struct {
	Candidates  []PlannedActivity             `json:"candidates"`
	SuggestedID string                        `json:"suggestedId,omitempty"`
	HasMore     bool                          `json:"hasMore"`
	Matched     *PlannedActivity              `json:"matched,omitempty"`
	Writeback   *TrainingSheetWritebackStatus `json:"writeback,omitempty"`
}

type GoogleSheetsStatus struct {
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	WriteReady bool   `json:"writeReady"`
	Provider   string `json:"provider"`
}
