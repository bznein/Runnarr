package app

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

type TrainingSheetWritebackService struct {
	store *Store
	auth  *GoogleSheetsAuthService
}

func NewTrainingSheetWritebackService(store *Store, auth *GoogleSheetsAuthService) *TrainingSheetWritebackService {
	return &TrainingSheetWritebackService{store: store, auth: auth}
}

func (s *Store) GetPlannedActivity(ctx context.Context, id string) (PlannedActivity, error) {
	var planned PlannedActivity
	err := scanPlannedActivity(s.db.QueryRow(ctx, `select `+plannedActivityColumns+` from planned_activities where id = $1`, id), &planned)
	return planned, err
}

func (s *Store) GetMatchedPlannedActivity(ctx context.Context, activityID string) (PlannedActivity, error) {
	var planned PlannedActivity
	err := scanPlannedActivity(s.db.QueryRow(ctx, `select `+plannedActivityColumns+` from planned_activities where matched_activity_id = $1`, activityID), &planned)
	return planned, err
}

func (s *Store) EnsureTrainingSheetWriteback(ctx context.Context, plannedID, activityID string) error {
	_, err := s.db.Exec(ctx, `
		insert into training_sheet_writebacks(planned_activity_id, activity_id)
		values($1, $2)
		on conflict(planned_activity_id) do update set activity_id = excluded.activity_id, updated_at = now()
	`, plannedID, activityID)
	return err
}

func (s *Store) CreateTrainingSheetWritebackJob(ctx context.Context, plannedID, activityID string) (string, error) {
	if err := s.EnsureTrainingSheetWriteback(ctx, plannedID, activityID); err != nil {
		return "", err
	}
	return s.CreateSyncJob(ctx, trainingSheetProvider, "writeback")
}

func (s *Store) GetTrainingSheetWriteback(ctx context.Context, plannedID string) (*TrainingSheetWritebackStatus, error) {
	status := &TrainingSheetWritebackStatus{}
	var summaryWritten, feedbackWritten, lastAttempt sql.NullTime
	err := s.db.QueryRow(ctx, `
		select planned_activity_id::text, activity_id::text, summary_status, summary_error, summary_written_at,
			feedback_status, feedback_error, feedback_written_at, last_attempt_at
		from training_sheet_writebacks where planned_activity_id = $1
	`, plannedID).Scan(&status.PlannedActivityID, &status.ActivityID, &status.SummaryStatus, &status.SummaryError, &summaryWritten,
		&status.FeedbackStatus, &status.FeedbackError, &feedbackWritten, &lastAttempt)
	if err != nil {
		return nil, err
	}
	if summaryWritten.Valid {
		status.SummaryWrittenAt = &summaryWritten.Time
	}
	if feedbackWritten.Valid {
		status.FeedbackWrittenAt = &feedbackWritten.Time
	}
	if lastAttempt.Valid {
		status.LastAttemptAt = &lastAttempt.Time
	}
	return status, nil
}

func (s *Store) updateTrainingSheetWritebackSection(ctx context.Context, plannedID, section, status, message string) error {
	if section == "summary" {
		_, err := s.db.Exec(ctx, `update training_sheet_writebacks set summary_status = $2, summary_error = $3, summary_written_at = case when $2 = 'completed' then now() else summary_written_at end, updated_at = now() where planned_activity_id = $1`, plannedID, status, message)
		return err
	}
	_, err := s.db.Exec(ctx, `update training_sheet_writebacks set feedback_status = $2, feedback_error = $3, feedback_written_at = case when $2 = 'completed' then now() else feedback_written_at end, updated_at = now() where planned_activity_id = $1`, plannedID, status, message)
	return err
}

func (s *Store) markTrainingSheetWritebackAttempt(ctx context.Context, plannedID string) error {
	_, err := s.db.Exec(ctx, `update training_sheet_writebacks set last_attempt_at = now(), updated_at = now() where planned_activity_id = $1`, plannedID)
	return err
}

func (s *TrainingSheetWritebackService) Write(ctx context.Context, plannedID, activityID string) (map[string]any, error) {
	planned, err := s.store.GetPlannedActivity(ctx, plannedID)
	if err != nil {
		return nil, err
	}
	activity, err := s.store.GetActivity(ctx, activityID)
	if err != nil {
		return nil, err
	}
	if planned.MatchedActivityID != activityID {
		return nil, fmt.Errorf("planned activity is no longer matched to this activity")
	}
	if err := s.store.EnsureTrainingSheetWriteback(ctx, plannedID, activityID); err != nil {
		return nil, err
	}
	if err := s.store.markTrainingSheetWritebackAttempt(ctx, plannedID); err != nil {
		return nil, err
	}

	result := map[string]any{"plannedActivityId": plannedID, "activityId": activityID, "intervals": "not_implemented"}
	var failures []string
	googleStatus, err := s.auth.Status(ctx)
	if err != nil {
		failures = append(failures, err.Error())
	} else if !googleStatus.WriteReady {
		failures = append(failures, "Google Sheets write access requires reconnecting the Google account")
	}

	if len(failures) == 0 {
		if err := s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "summary", "running", ""); err != nil {
			return nil, err
		}
		summaryStatus, summaryError := s.writeSummary(ctx, planned, activity)
		if summaryError != nil && summaryStatus == "failed" {
			failures = append(failures, summaryError.Error())
		}
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "summary", summaryStatus, summaryErrorString(summaryError))
		result["summaryStatus"] = summaryStatus
	} else {
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "summary", "failed", failures[0])
		result["summaryStatus"] = "failed"
	}

	feedbackCell := feedbackCellForPlanned(planned)
	if feedbackCell == "" {
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "feedback", "not_applicable", "")
		result["feedbackStatus"] = "not_applicable"
	} else if strings.TrimSpace(activity.Feedback) == "" {
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "feedback", "waiting_for_feedback", "")
		result["feedbackStatus"] = "waiting_for_feedback"
	} else if len(failures) == 0 {
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "feedback", "running", "")
		feedbackStatus, feedbackError := s.writeFeedback(ctx, planned, activity)
		if feedbackError != nil && feedbackStatus == "failed" {
			failures = append(failures, feedbackError.Error())
		}
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "feedback", feedbackStatus, summaryErrorString(feedbackError))
		result["feedbackStatus"] = feedbackStatus
	} else {
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "feedback", "failed", failures[0])
		result["feedbackStatus"] = "failed"
	}

	if len(failures) > 0 {
		return result, fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return result, nil
}

func (s *TrainingSheetWritebackService) writeSummary(ctx context.Context, planned PlannedActivity, activity Activity) (string, error) {
	column := strings.TrimRight(planned.PlanCell, "0123456789")
	updates := make([]googleValueRangeUpdate, 0, 6)
	add := func(row string, value any) {
		updates = append(updates, googleValueRangeUpdate{Range: sheetCellRange(planned.SheetTitle, column+row), Values: [][]any{{value}}})
	}
	add("3", math.Round(activity.DistanceM/10)/100)
	duration := activity.MovingTimeS
	if duration <= 0 {
		duration = activity.ElapsedTimeS
	}
	if duration > 0 {
		add("4", "'"+sheetDurationText(duration))
	}
	if activity.AvgPaceSPKM != nil {
		add("5", "'"+sheetPaceText(*activity.AvgPaceSPKM))
	}
	if activity.AvgHeartRate != nil {
		add("6", math.Round(*activity.AvgHeartRate))
	}
	if activity.MaxHeartRate != nil {
		add("7", math.Round(*activity.MaxHeartRate))
	}
	if activity.RPE != nil {
		add("8", *activity.RPE)
	}
	return s.writeUpdatesPreservingExisting(ctx, planned.WorkbookID, updates)
}

func sheetDurationText(totalSeconds int) string {
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
}

func sheetPaceText(secondsPerKm float64) string {
	if math.IsNaN(secondsPerKm) || math.IsInf(secondsPerKm, 0) || secondsPerKm <= 0 {
		return ""
	}
	totalSeconds := int(math.Round(secondsPerKm))
	return fmt.Sprintf("%d:%02d", totalSeconds/60, totalSeconds%60)
}

func (s *TrainingSheetWritebackService) writeFeedback(ctx context.Context, planned PlannedActivity, activity Activity) (string, error) {
	cell := feedbackCellForPlanned(planned)
	if cell == "" {
		return "not_applicable", nil
	}
	update := googleValueRangeUpdate{Range: sheetCellRange(planned.SheetTitle, cell), Values: [][]any{{strings.TrimSpace(activity.Feedback)}}}
	return s.writeUpdatesPreservingExisting(ctx, planned.WorkbookID, []googleValueRangeUpdate{update})
}

func feedbackCellForPlanned(planned PlannedActivity) string {
	cell := planned.FeedbackCell
	if cell == "" {
		if raw, ok := planned.Raw["feedbackCell"].(string); ok {
			cell = raw
		}
	}
	if cell == "" {
		cell = feedbackCellFromRaw(planned.Raw, planned.PlanCell)
	}
	return cell
}

func feedbackCellFromRaw(raw map[string]any, planCell string) string {
	values, ok := raw["values"].([]any)
	if !ok {
		return ""
	}
	column := strings.TrimRight(planCell, "0123456789")
	if len(column) != 1 || column[0] < 'B' || column[0] > 'H' {
		return ""
	}
	day := int(column[0] - 'B')
	currentDays := []int(nil)
	for rowIndex, rawRow := range values {
		row, ok := rawRow.([]any)
		if !ok || len(row) == 0 {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(row[0]))
		if text == "" {
			continue
		}
		if colon := strings.Index(text, ":"); colon > 0 {
			currentDays = parseDayScope(strings.TrimSpace(text[:colon]))
		}
		if strings.EqualFold(text, "How did it feel/go?") {
			for _, candidate := range currentDays {
				if candidate == day {
					return fmt.Sprintf("C%d", rowIndex+1)
				}
			}
		}
	}
	return ""
}

func (s *TrainingSheetWritebackService) writeUpdatesPreservingExisting(ctx context.Context, workbookID string, updates []googleValueRangeUpdate) (string, error) {
	if len(updates) == 0 {
		return "completed", nil
	}
	var existing [][][]string
	err := retryGoogle(ctx, func() error {
		var readErr error
		existing, readErr = s.auth.ReadRanges(ctx, workbookID, rangesForUpdates(updates))
		return readErr
	})
	if err != nil {
		return "failed", err
	}
	writes := make([]googleValueRangeUpdate, 0, len(updates))
	conflicts := make([]string, 0)
	for index, update := range updates {
		if index < len(existing) && rangeHasValue(existing[index]) {
			conflicts = append(conflicts, update.Range)
			continue
		}
		writes = append(writes, update)
	}
	if err := retryGoogle(ctx, func() error { return s.auth.WriteRanges(ctx, workbookID, writes) }); err != nil {
		return "failed", err
	}
	if len(conflicts) > 0 {
		return "completed_with_conflicts", fmt.Errorf("existing values preserved in %s", strings.Join(conflicts, ", "))
	}
	return "completed", nil
}

func rangesForUpdates(updates []googleValueRangeUpdate) []string {
	ranges := make([]string, len(updates))
	for index := range updates {
		ranges[index] = updates[index].Range
	}
	return ranges
}

func rangeHasValue(values [][]string) bool {
	for _, row := range values {
		for _, value := range row {
			if strings.TrimSpace(value) != "" {
				return true
			}
		}
	}
	return false
}

func retryGoogle(ctx context.Context, operation func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = operation()
		if err == nil || (!strings.Contains(err.Error(), "status 429") && !strings.Contains(err.Error(), "status 500") && !strings.Contains(err.Error(), "status 502") && !strings.Contains(err.Error(), "status 503")) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Second):
		}
	}
	return err
}

func sheetCellRange(title, cell string) string {
	return fmt.Sprintf("'%s'!%s", strings.ReplaceAll(title, "'", "''"), cell)
}

func summaryErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
