package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	trainingSheetProvider               = "training_sheet"
	defaultTrainingSheetCheckEveryHours = 24
)

type PlannedTrainingSheetService struct {
	store  *Store
	logger *slog.Logger
}

func NewPlannedTrainingSheetService(store *Store, logger *slog.Logger) *PlannedTrainingSheetService {
	return &PlannedTrainingSheetService{store: store, logger: logger}
}

func (s *PlannedTrainingSheetService) Sync(ctx context.Context, cfg TrainingSheetConfig, progress func(map[string]any)) (map[string]any, error) {
	if strings.TrimSpace(cfg.SheetURL) == "" {
		return nil, fmt.Errorf("sheet URL is not configured")
	}
	deployment, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	auth := NewGoogleSheetsAuthService(s.store, deployment)
	if !auth.Configured() {
		return nil, fmt.Errorf("Google OAuth is not configured")
	}
	planYear, err := s.store.GetTrainingSheetPlanYear(ctx)
	if err != nil {
		return nil, err
	}
	sheetID, tabs, err := auth.ReadWorkbook(ctx, cfg.SheetURL)
	if err != nil {
		return nil, err
	}
	if progress != nil {
		progress(map[string]any{"provider": trainingSheetProvider, "kind": "sync", "stage": "Reading worksheets", "sheets": len(tabs)})
	}
	processed, saved, skipped := 0, 0, 0
	warnings := make([]string, 0)
	for _, tab := range tabs {
		weekEnd, ok := parseWeeklyTabDate(tab.Title, planYear)
		if !ok {
			continue
		}
		for _, candidate := range plannedActivitiesFromTab(sheetID, tab, weekEnd) {
			processed++
			if err := s.store.UpsertPlannedActivity(ctx, candidate); err != nil {
				skipped++
				warnings = append(warnings, fmt.Sprintf("%s %s: %v", tab.Title, candidate.PlanCell, err))
				continue
			}
			saved++
			if progress != nil {
				progress(map[string]any{"provider": trainingSheetProvider, "kind": "sync", "stage": "Saving planned activities", "processed": processed, "saved": saved, "skipped": skipped, "currentName": candidate.Name, "worksheetName": tab.Title})
			}
		}
	}
	payload := map[string]any{"provider": trainingSheetProvider, "kind": "sync", "stage": "Completed", "activities": processed, "processed": processed, "saved": saved, "skipped": skipped, "sheets": len(tabs)}
	if len(warnings) > 0 {
		payload["warnings"] = warnings
		payload["stage"] = "Completed with warnings"
	}
	if processed == 0 {
		return payload, fmt.Errorf("no planned activities found in the configured workbook")
	}
	return payload, nil
}

func parseTrainingSheetID(rawURL string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", fmt.Errorf("invalid sheet URL: %w", err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	sheetID := ""
	for index := range parts {
		if parts[index] == "d" && index+1 < len(parts) {
			sheetID = parts[index+1]
			break
		}
	}
	if sheetID == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("sheet URL must be a Google Sheets URL")
	}
	gid := parsed.Query().Get("gid")
	if gid == "" {
		fragment := strings.TrimPrefix(parsed.Fragment, "gid=")
		if fragment != parsed.Fragment {
			gid = fragment
		}
	}
	return sheetID, gid, nil
}

func parseWeeklyTabDate(title string, year int) (time.Time, bool) {
	var day, month int
	if _, err := fmt.Sscanf(strings.TrimSpace(title), "%d-%d", &day, &month); err != nil || day < 1 || day > 31 || month < 1 || month > 12 {
		return time.Time{}, false
	}
	date := time.Date(year, time.Month(month), day, 12, 0, 0, 0, time.UTC)
	return date, date.Day() == day && date.Month() == time.Month(month)
}

func plannedActivitiesFromTab(workbookID string, tab googleSheetTab, weekEnd time.Time) []PlannedActivity {
	if len(tab.Values) < 2 {
		return nil
	}
	details := scopedTabDetails(tab.Values)
	planRow := tab.Values[1]
	activities := make([]PlannedActivity, 0, 7)
	for column := 1; column <= 7; column++ {
		name := strings.TrimSpace(cellValue(planRow, column))
		if name == "" {
			continue
		}
		day := column - 1
		plannedDate := weekEnd.AddDate(0, 0, day-6)
		cell := fmt.Sprintf("%s2", spreadsheetColumn(column+1))
		notes := strings.TrimSpace(strings.Join(details[day], "\n\n"))
		raw := map[string]any{"sheetTitle": tab.Title, "sheetId": tab.ID, "weekEnding": weekEnd.Format("2006-01-02"), "planCell": cell, "planName": name, "notes": notes, "values": tab.Values}
		activities = append(activities, PlannedActivity{Source: trainingSheetProvider, SourceID: fmt.Sprintf("%s:%s:%s", workbookID, tab.ID, cell), WorkbookID: workbookID, SheetID: tab.ID, SheetTitle: tab.Title, PlanCell: cell, PlannedDate: plannedDate, Name: name, SportType: "Run", Notes: notes, Status: "pending", SourceURL: fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit#gid=%s", workbookID, tab.ID), Raw: raw})
	}
	return activities
}

var weekdayPattern = regexp.MustCompile(`(?i)\b(monday|mon|tuesday|tues|tue|wednesday|wed|thursday|thurs|thu|friday|fri|saturday|sat|sunday|sun)\b`)

func scopedTabDetails(values [][]string) map[int][]string {
	details := make(map[int][]string)
	for rowIndex := 2; rowIndex < len(values); rowIndex++ {
		text := strings.TrimSpace(cellValue(values[rowIndex], 0))
		if text == "" || strings.EqualFold(text, "How did it feel/go?") {
			continue
		}
		colon := strings.Index(text, ":")
		if colon <= 0 || colon == len(text)-1 {
			continue
		}
		scope, note := strings.TrimSpace(text[:colon]), strings.TrimSpace(text[colon+1:])
		if note == "" {
			continue
		}
		for _, day := range parseDayScope(scope) {
			details[day] = append(details[day], note)
		}
	}
	return details
}

func parseDayScope(scope string) []int {
	matches := weekdayPattern.FindAllString(scope, -1)
	if len(matches) == 0 {
		return nil
	}
	if strings.Contains(scope, "-") && len(matches) >= 2 {
		start, end := weekdayIndex(matches[0]), weekdayIndex(matches[1])
		if start >= 0 && end >= start {
			out := make([]int, 0, end-start+1)
			for day := start; day <= end; day++ {
				out = append(out, day)
			}
			return out
		}
	}
	out := make([]int, 0, len(matches))
	for _, match := range matches {
		if day := weekdayIndex(match); day >= 0 {
			out = append(out, day)
		}
	}
	return out
}

func weekdayIndex(value string) int {
	switch strings.ToLower(value) {
	case "monday", "mon":
		return 0
	case "tuesday", "tues", "tue":
		return 1
	case "wednesday", "wed":
		return 2
	case "thursday", "thurs", "thu":
		return 3
	case "friday", "fri":
		return 4
	case "saturday", "sat":
		return 5
	case "sunday", "sun":
		return 6
	default:
		return -1
	}
}

func cellValue(row []string, column int) string {
	if column < 0 || column >= len(row) {
		return ""
	}
	return row[column]
}

func spreadsheetColumn(column int) string {
	if column <= 0 {
		return ""
	}
	if column <= 26 {
		return string(rune('A' + column - 1))
	}
	return string(rune('A'+(column-1)/26-1)) + string(rune('A'+(column-1)%26))
}
