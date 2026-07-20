package app

import (
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	trainingSheetProvider               = "training_sheet"
	defaultTrainingSheetCheckEveryHours = 24
)

type TrainingSheetService struct {
	store  *Store
	logger *slog.Logger
	client *http.Client
}

type trainingSheetWorksheet struct {
	Name string
	GID  string
}

type trainingSheetParsedActivity struct {
	Date     time.Time
	RawName  string
	SourceID string
	Activity ImportedActivity
}

type trainingSheetColumnIndex struct {
	Name     int
	Date     int
	Distance int
	Duration int
	Pace     int
	Note     int
	Sport    int
	SourceID int
}

func NewTrainingSheetService(store *Store, logger *slog.Logger) *TrainingSheetService {
	return &TrainingSheetService{
		store:  store,
		logger: logger,
		client: &http.Client{Timeout: 45 * time.Second},
	}
}

func (s *TrainingSheetService) Sync(ctx context.Context, cfg TrainingSheetConfig, progress func(map[string]any)) (map[string]any, error) {
	sheetURL := strings.TrimSpace(cfg.SheetURL)
	sheetID, requestedGID, err := parseTrainingSheetID(sheetURL)
	if err != nil {
		return nil, err
	}

	if progress != nil {
		progress(map[string]any{
			"provider": trainingSheetProvider,
			"kind":     "sync",
			"stage":    "Discovering worksheets",
			"sheetId":  sheetID,
			"url":      sheetURL,
		})
	}

	worksheets, err := s.discoverWorksheets(ctx, sheetID)
	if err != nil || len(worksheets) == 0 {
		if strings.TrimSpace(requestedGID) == "" {
			if err == nil {
				err = fmt.Errorf("could not discover worksheets")
			}
			return nil, err
		}
		worksheets = []trainingSheetWorksheet{{Name: "Selected", GID: requestedGID}}
	} else {
		worksheets = prioritizeWorksheet(worksheets, requestedGID)
	}

	warnings := make([]string, 0)
	totalSaved := 0
	totalFailed := 0
	totalSkipped := 0
	totalProcessed := 0
	for _, worksheet := range worksheets {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if progress != nil {
			progress(map[string]any{
				"provider":      trainingSheetProvider,
				"kind":          "sync",
				"stage":         "Reading worksheet",
				"worksheetName": worksheet.Name,
				"worksheetGID":  worksheet.GID,
				"sheets":        len(worksheets),
				"processed":     totalProcessed,
			})
		}

		activities, err := s.activitiesFromWorksheet(ctx, sheetID, worksheet)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", worksheet.Name, err))
			continue
		}
		if len(activities) == 0 {
			continue
		}
		for _, candidate := range activities {
			totalProcessed++
			sourceID := strings.TrimSpace(candidate.SourceID)
			if sourceID == "" {
				sourceID = trainingSheetActivitySourceID(sheetID, worksheet.GID, candidate.Date, candidate.Activity.Name)
			}
			if sourceID == "" {
				totalSkipped++
				continue
			}
			if err := validateTrainingSheetActivity(candidate.Activity); err != nil {
				totalSkipped++
				warnings = append(warnings, fmt.Sprintf("%s: %v", candidate.Activity.Name, err))
				if progress != nil {
					progress(map[string]any{
						"provider":      trainingSheetProvider,
						"kind":          "sync",
						"stage":         "Saving activities",
						"processed":     totalProcessed,
						"saved":         totalSaved,
						"failed":        totalFailed,
						"skipped":       totalSkipped,
						"worksheetName": worksheet.Name,
						"currentName":   candidate.Activity.Name,
					})
				}
				continue
			}
			if _, err := s.store.SaveImportedActivity(ctx, trainingSheetProvider, sourceID, nil, candidate.Activity); err != nil {
				totalFailed++
				warnings = append(warnings, fmt.Sprintf("%s: %v", candidate.Activity.Name, err))
				if progress != nil {
					progress(map[string]any{
						"provider":      trainingSheetProvider,
						"kind":          "sync",
						"stage":         "Saving activities",
						"processed":     totalProcessed,
						"saved":         totalSaved,
						"failed":        totalFailed,
						"skipped":       totalSkipped,
						"worksheetName": worksheet.Name,
						"currentName":   candidate.Activity.Name,
						"currentError":  err.Error(),
					})
				}
				continue
			}
			totalSaved++
			if progress != nil {
				progress(map[string]any{
					"provider":      trainingSheetProvider,
					"kind":          "sync",
					"stage":         "Saving activities",
					"activities":    totalProcessed,
					"processed":     totalProcessed,
					"saved":         totalSaved,
					"failed":        totalFailed,
					"skipped":       totalSkipped,
					"sheets":        len(worksheets),
					"worksheetName": worksheet.Name,
					"currentName":   candidate.Activity.Name,
				})
			}
		}
	}

	payload := map[string]any{
		"provider":   trainingSheetProvider,
		"kind":       "sync",
		"stage":      "Completed",
		"activities": totalProcessed,
		"processed":  totalProcessed,
		"saved":      totalSaved,
		"failed":     totalFailed,
		"skipped":    totalSkipped,
		"sheets":     len(worksheets),
	}
	if len(warnings) > 0 {
		payload["warnings"] = warnings
		payload["stage"] = "Completed with warnings"
	}
	if totalSaved == 0 && totalFailed > 0 {
		return payload, fmt.Errorf("could not save any training activities")
	}
	return payload, nil
}

func (s *TrainingSheetService) discoverWorksheets(ctx context.Context, sheetID string) ([]trainingSheetWorksheet, error) {
	feedURL := fmt.Sprintf("https://spreadsheets.google.com/feeds/worksheets/%s/public/basic?alt=json", sheetID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("worksheet discovery failed with status %d", response.StatusCode)
	}

	var feed trainingSheetFeed
	if err := json.NewDecoder(response.Body).Decode(&feed); err != nil {
		return nil, err
	}

	out := make([]trainingSheetWorksheet, 0, len(feed.Feed.Entry))
	for _, entry := range feed.Feed.Entry {
		gid := findWorksheetGIDFromLinks(entry.Link)
		if gid == "" {
			continue
		}
		out = append(out, trainingSheetWorksheet{Name: strings.TrimSpace(entry.Title.Text), GID: gid})
	}
	return out, nil
}

func (s *TrainingSheetService) activitiesFromWorksheet(ctx context.Context, sheetID string, worksheet trainingSheetWorksheet) ([]trainingSheetParsedActivity, error) {
	csvURL := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv&gid=%s", sheetID, worksheet.GID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, csvURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("worksheet download failed with status %d", response.StatusCode)
	}

	return parseTrainingSheetCSV(response.Body, sheetID, worksheet)
}

func parseTrainingSheetCSV(body io.Reader, sheetID string, worksheet trainingSheetWorksheet) ([]trainingSheetParsedActivity, error) {
	reader := csv.NewReader(body)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, nil
	}

	indices := discoverTrainingSheetColumns(records[0])
	if indices.Name < 0 {
		return nil, fmt.Errorf("no activity-name column found")
	}

	defaultDate, _ := parseDateFromText(worksheet.Name)
	parsed := make([]trainingSheetParsedActivity, 0, len(records)-1)
	for _, row := range records[1:] {
		if len(row) == 0 {
			continue
		}
		name := strings.TrimSpace(valueFromRow(row, indices.Name))
		if name == "" {
			continue
		}

		activityDate, hasDate := parseRowDate(row, indices.Date, defaultDate)
		if !hasDate {
			activityDate = defaultDate
		}
		if activityDate.IsZero() {
			continue
		}

		sourceID := strings.TrimSpace(valueFromRow(row, indices.SourceID))
		distanceM, _ := parseDistanceToMeters(valueFromRow(row, indices.Distance))
		durationS, _ := parseDurationToSeconds(valueFromRow(row, indices.Duration))
		if durationS == 0 && distanceM > 0 {
			if paceS := parsePaceToSeconds(valueFromRow(row, indices.Pace)); paceS > 0 {
				distanceKm := distanceM / 1000
				durationS = int(distanceKm * float64(paceS))
			}
		}
		sport := parseSportFromText(valueFromRow(row, indices.Sport))
		notes := buildTrainingSheetNotes(row, indices)
		if notes == "" {
			notes = strings.TrimSpace(valueFromRow(row, indices.Note))
		}
		activity := ImportedActivity{
			Name:                name,
			LocalName:           "",
			SportType:           sport,
			LocalNotes:          "",
			StartTime:           activityDate,
			DistanceM:           distanceM,
			MovingTimeS:         durationS,
			ElapsedTimeS:        durationS,
			ElevationGainM:      0,
			OriginalProviderURL: fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit#gid=%s", sheetID, worksheet.GID),
			Raw: map[string]any{
				"sheet":     worksheet.Name,
				"gid":       worksheet.GID,
				"raw_name":  name,
				"distance":  distanceM,
				"duration":  durationS,
				"pace":      valueFromRow(row, indices.Pace),
				"sheetDate": activityDate.Format("2006-01-02"),
				"notes":     notes,
			},
		}

		normalizeImported(&activity)
		activity.LocalNotes = strings.TrimSpace(notes)
		if sourceID == "" {
			sourceID = trainingSheetActivitySourceID(sheetID, worksheet.GID, activityDate, activity.Name)
		}
		parsed = append(parsed, trainingSheetParsedActivity{
			Date:     activityDate,
			RawName:  name,
			SourceID: sourceID,
			Activity: activity,
		})
	}

	return parsed, nil
}

func validateTrainingSheetActivity(activity ImportedActivity) error {
	name := strings.TrimSpace(activity.Name)
	if name == "" {
		return fmt.Errorf("missing activity name")
	}
	if activity.StartTime.IsZero() {
		return fmt.Errorf("missing activity date")
	}
	if strings.TrimSpace(activity.SportType) == "" {
		return fmt.Errorf("missing sport type")
	}
	return nil
}

func discoverTrainingSheetColumns(headers []string) trainingSheetColumnIndex {
	indices := trainingSheetColumnIndex{
		Name:     -1,
		Date:     -1,
		Distance: -1,
		Duration: -1,
		Pace:     -1,
		Note:     -1,
		Sport:    -1,
		SourceID: -1,
	}
	for index, header := range headers {
		norm := normalizeHeaderForSheet(header)
		if norm == "" {
			continue
		}
		if indices.Name < 0 && containsAnyInText(
			norm,
			"activity",
			"workout",
			"name",
			"session",
			"description",
			"title",
		) {
			indices.Name = index
			continue
		}
		if indices.Date < 0 && containsAnyInText(norm, "date", "day", "when") {
			indices.Date = index
		}
		if indices.Distance < 0 && containsAnyInText(norm, "distance", "km", "mile", "miles", "meter") {
			indices.Distance = index
		}
		if indices.Duration < 0 && containsAnyInText(norm, "duration", "time", "length", "durationmin") {
			indices.Duration = index
		}
		if indices.Pace < 0 && containsAnyInText(norm, "pace", "targetpace", "speed") {
			indices.Pace = index
		}
		if indices.Note < 0 && containsAnyInText(norm, "note", "comment", "details", "commentary") {
			indices.Note = index
		}
		if indices.Sport < 0 && containsAnyInText(
			norm,
			"type",
			"sport",
			"activitytype",
			"activity_type",
			"mode",
		) {
			indices.Sport = index
		}
		if indices.SourceID < 0 && containsAnyInText(norm, "sourceid", "source_id", "sheetid", "id") {
			indices.SourceID = index
		}
	}
	return indices
}

type trainingSheetFeed struct {
	Feed struct {
		Entry []struct {
			Title struct {
				Text string `json:"$t"`
			} `json:"title"`
			Link []struct {
				Rel  string `json:"rel"`
				Href string `json:"href"`
			} `json:"link"`
		} `json:"entry"`
	} `json:"feed"`
}

func findWorksheetGIDFromLinks(links []struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}) string {
	for _, link := range links {
		if link.Rel != "alternate" && link.Rel != "https://schemas.google.com/spreadsheets/2006#tq" {
			continue
		}
		if parsed, err := url.Parse(link.Href); err == nil {
			if gid := strings.TrimSpace(parsed.Query().Get("gid")); gid != "" {
				return gid
			}
			fragment := strings.TrimPrefix(strings.TrimPrefix(parsed.Fragment, "#"), "!")
			if strings.HasPrefix(fragment, "gid=") {
				return strings.TrimPrefix(fragment, "gid=")
			}
		}
	}
	return ""
}

func prioritizeWorksheet(worksheets []trainingSheetWorksheet, preferredGID string) []trainingSheetWorksheet {
	if strings.TrimSpace(preferredGID) == "" {
		return worksheets
	}

	seen := make(map[string]struct{}, len(worksheets)+1)
	unique := make([]trainingSheetWorksheet, 0, len(worksheets)+1)
	for _, worksheet := range worksheets {
		if _, exists := seen[worksheet.GID]; exists {
			continue
		}
		seen[worksheet.GID] = struct{}{}
		unique = append(unique, worksheet)
	}

	for i, worksheet := range unique {
		if worksheet.GID == preferredGID {
			return append([]trainingSheetWorksheet{worksheet}, append(unique[:i], unique[i+1:]...)...)
		}
	}
	return append([]trainingSheetWorksheet{{Name: "Selected", GID: preferredGID}}, unique...)
}

func parseDateFromText(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, false
	}

	formats := []string{
		"2006-01-02",
		"02/01/2006",
		"01/02/2006",
		"02-01-2006",
		"02 Jan 2006",
		"Jan 02, 2006",
		"2 Jan 2006",
		"January 2, 2006",
	}
	for _, layout := range formats {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed, true
		}
	}

	if found := trainingSheetNumericDatePattern.FindStringSubmatch(trimmed); len(found) == 4 {
		year, err1 := strconv.Atoi(found[1])
		month, err2 := strconv.Atoi(found[2])
		day, err3 := strconv.Atoi(found[3])
		if err1 == nil && err2 == nil && err3 == nil {
			if year < 100 {
				year += 2000
			}
			if month >= 1 && month <= 12 && day >= 1 && day <= 31 {
				return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), true
			}
		}
	}

	if found := trainingSheetMonthDatePattern.FindStringSubmatch(trimmed); len(found) == 4 {
		day, err1 := strconv.Atoi(found[1])
		month := trainingSheetMonthToNumber(found[2])
		year, err2 := strconv.Atoi(found[3])
		if err1 == nil && err2 == nil && month > 0 {
			if year < 100 {
				year += 2000
			}
			return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), true
		}
	}

	return time.Time{}, false
}

var trainingSheetNumericDatePattern = regexp.MustCompile(`(?i)(\d{4})[-./](\d{1,2})[-./](\d{1,2})`)
var trainingSheetMonthDatePattern = regexp.MustCompile(`(?i)(\d{1,2})\s+([a-z]+)\s+(\d{2,4})`)

func parseRowDate(row []string, dateColumn int, fallback time.Time) (time.Time, bool) {
	if dateColumn >= 0 && dateColumn < len(row) {
		dateText := strings.TrimSpace(row[dateColumn])
		if date, ok := parseDateFromText(dateText); ok {
			return date, true
		}
	}
	if !fallback.IsZero() {
		return fallback, true
	}
	return time.Time{}, false
}

func parseDurationToSeconds(value string) (int, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0, false
	}

	if match := parseDurationPatternMinutesSeconds.FindStringSubmatch(value); len(match) == 3 {
		first, err1 := strconv.Atoi(match[1])
		second, err2 := strconv.Atoi(match[2])
		if err1 != nil || err2 != nil || second >= 60 {
			return 0, false
		}
		return first*60 + second, true
	}
	if match := parseDurationPatternHours.FindStringSubmatch(value); len(match) == 4 {
		hours, err1 := strconv.Atoi(match[1])
		minutes, err2 := strconv.Atoi(match[2])
		seconds, err3 := strconv.Atoi(match[3])
		if err1 != nil || err2 != nil || err3 != nil || minutes >= 60 || seconds >= 60 {
			return 0, false
		}
		return hours*3600 + minutes*60 + seconds, true
	}
	if match := parseDurationPatternWord.FindStringSubmatch(value); len(match) == 4 {
		hours, err1 := strconv.Atoi(match[1])
		minutes, err2 := strconv.Atoi(match[2])
		seconds := 0
		if match[3] != "" {
			v, err := strconv.Atoi(match[3])
			if err != nil || v >= 60 {
				return 0, false
			}
			seconds = v
		}
		if err1 != nil || err2 != nil || minutes >= 60 {
			return 0, false
		}
		return hours*3600 + minutes*60 + seconds, true
	}
	if match := parseDurationPatternMinuteWord.FindStringSubmatch(value); len(match) == 3 {
		minutes, err1 := strconv.Atoi(match[1])
		seconds := 0
		if match[2] != "" {
			v, err := strconv.Atoi(match[2])
			if err != nil || v >= 60 {
				return 0, false
			}
			seconds = v
		}
		if err1 != nil {
			return 0, false
		}
		return minutes*60 + seconds, true
	}

	return 0, false
}

var parseDurationPatternMinutesSeconds = regexp.MustCompile(`^(\d+):(\d{2})$`)
var parseDurationPatternHours = regexp.MustCompile(`^(\d+):(\d{2}):(\d{2})$`)
var parseDurationPatternWord = regexp.MustCompile(`(?i)^(\d+)\s*h(?:\s*(\d+)\s*m)?(?:\s*(\d+)\s*s)?$`)
var parseDurationPatternMinuteWord = regexp.MustCompile(`(?i)^(\d+)\s*m(?:\s*(\d+)\s*s)?$`)

func parsePaceToSeconds(value string) int {
	match := parsePacePattern.FindStringSubmatch(strings.TrimSpace(strings.ToLower(value)))
	if len(match) != 3 {
		return 0
	}
	minutes, err1 := strconv.Atoi(match[1])
	seconds, err2 := strconv.Atoi(match[2])
	if err1 != nil || err2 != nil || seconds >= 60 {
		return 0
	}
	return minutes*60 + seconds
}

var parsePacePattern = regexp.MustCompile(`^(\d+):(\d{2})`)

func parseDistanceToMeters(value string) (float64, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(strings.ReplaceAll(value, ",", ".")))
	if trimmed == "" {
		return 0, false
	}
	match := parseDistancePattern.FindStringSubmatch(trimmed)
	if len(match) == 0 {
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false
		}
		return parsed * 1000, true
	}

	parsed, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", "."), 64)
	if err != nil {
		return 0, false
	}
	unit := match[2]
	if unit == "" || strings.HasPrefix(unit, "km") || strings.HasPrefix(unit, "kil") || unit == "k" {
		return parsed * 1000, true
	}
	if strings.HasPrefix(unit, "mi") || strings.Contains(unit, "mile") {
		return parsed * 1609.34, true
	}
	if unit == "m" || strings.HasPrefix(unit, "meter") || unit == "mts" {
		return parsed, true
	}
	if strings.Contains(unit, "yd") {
		return parsed * 0.9144, true
	}
	return parsed * 1000, true
}

var parseDistancePattern = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*(km|kilometre|kilometer|mile|miles|m|mi|meter|metre|k|yd)?`)

func parseSportFromText(raw string) string {
	sport := strings.TrimSpace(strings.ToLower(raw))
	if sport == "" {
		return "run"
	}
	return sport
}

func buildTrainingSheetNotes(row []string, columns trainingSheetColumnIndex) string {
	parts := make([]string, 0)
	if columns.Pace >= 0 && columns.Pace < len(row) {
		value := strings.TrimSpace(row[columns.Pace])
		if value != "" {
			parts = append(parts, fmt.Sprintf("Pace: %s", value))
		}
	}
	if columns.Duration >= 0 && columns.Duration < len(row) {
		timeText := strings.TrimSpace(row[columns.Duration])
		if timeText != "" {
			parts = append(parts, fmt.Sprintf("Time: %s", timeText))
		}
	}
	if columns.Distance >= 0 && columns.Distance < len(row) {
		distanceText := strings.TrimSpace(row[columns.Distance])
		if distanceText != "" {
			parts = append(parts, fmt.Sprintf("Distance: %s", distanceText))
		}
	}
	if columns.Note >= 0 && columns.Note < len(row) {
		noteText := strings.TrimSpace(row[columns.Note])
		if noteText != "" {
			parts = append(parts, noteText)
		}
	}
	return strings.Join(parts, " · ")
}

func parseTrainingSheetID(sheetURL string) (string, string, error) {
	trimmed := strings.TrimSpace(sheetURL)
	if trimmed == "" {
		return "", "", fmt.Errorf("training sheet URL is required")
	}

	if !strings.Contains(trimmed, "://") {
		if strings.Contains(trimmed, "/") {
			return "", "", fmt.Errorf("invalid training sheet URL")
		}
		return trimmed, "", nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", "", fmt.Errorf("invalid training sheet URL")
	}

	pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, part := range pathParts {
		if part == "d" && i+1 < len(pathParts) {
			reqGID := parsed.Query().Get("gid")
			if reqGID == "" {
				reqGID = parseTrainingSheetGIDFromFragment(parsed.Fragment)
			}
			return pathParts[i+1], reqGID, nil
		}
	}
	if strings.Contains(parsed.Host, "docs.google.com") && len(pathParts) >= 1 {
		reqGID := parsed.Query().Get("gid")
		if reqGID == "" {
			reqGID = parseTrainingSheetGIDFromFragment(parsed.Fragment)
		}
		return pathParts[len(pathParts)-1], reqGID, nil
	}

	return "", "", fmt.Errorf("could not extract spreadsheet id")
}

func parseTrainingSheetGIDFromFragment(fragment string) string {
	fragment = strings.TrimPrefix(strings.TrimSpace(fragment), "#")
	if fragment == "" {
		return ""
	}
	if parsedFragment, err := url.Parse("?" + fragment); err == nil {
		if value := parsedFragment.Query().Get("gid"); value != "" {
			return value
		}
	}
	if idx := strings.Index(fragment, "gid="); idx >= 0 {
		fragment = fragment[idx+4:]
		if amp := strings.Index(fragment, "&"); amp >= 0 {
			fragment = fragment[:amp]
		}
		return fragment
	}
	return ""
}

func normalizeHeaderForSheet(value string) string {
	clean := strings.ToLower(strings.TrimSpace(value))
	clean = strings.ReplaceAll(clean, " ", "")
	clean = strings.ReplaceAll(clean, "_", "")
	clean = strings.ReplaceAll(clean, "-", "")
	return clean
}

func containsAnyInText(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func valueFromRow(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func trainingSheetMonthToNumber(month string) int {
	switch strings.ToLower(month) {
	case "jan", "january":
		return 1
	case "feb", "february":
		return 2
	case "mar", "march":
		return 3
	case "apr", "april":
		return 4
	case "may":
		return 5
	case "jun", "june":
		return 6
	case "jul", "july":
		return 7
	case "aug", "august":
		return 8
	case "sep", "sept", "september":
		return 9
	case "oct", "october":
		return 10
	case "nov", "november":
		return 11
	case "dec", "december":
		return 12
	default:
		return 0
	}
}

func trainingSheetActivitySourceID(sheetID, worksheetGID string, date time.Time, name string) string {
	key := fmt.Sprintf("%s|%s|%s|%s", sheetID, worksheetGID, date.Format("2006-01-02"), strings.TrimSpace(name))
	sum := sha1.Sum([]byte(key))
	return hex.EncodeToString(sum[:])
}
