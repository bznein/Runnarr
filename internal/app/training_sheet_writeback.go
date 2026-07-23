package app

import (
	"context"
	"database/sql"
	"encoding/json"
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
	err := scanPlannedActivity(s.db.QueryRow(ctx, `select `+plannedActivityColumns+` from planned_activities where id = $1 and user_id = $2`, id, scopedUserID(ctx)), &planned)
	return planned, err
}

func (s *Store) GetMatchedPlannedActivity(ctx context.Context, activityID string) (PlannedActivity, error) {
	var planned PlannedActivity
	err := scanPlannedActivity(s.db.QueryRow(ctx, `select `+plannedActivityColumns+` from planned_activities where matched_activity_id = $1 and user_id = $2`, activityID, scopedUserID(ctx)), &planned)
	return planned, err
}

func (s *Store) EnsureTrainingSheetWriteback(ctx context.Context, plannedID, activityID string) error {
	_, err := s.db.Exec(ctx, `
		insert into training_sheet_writebacks(planned_activity_id, activity_id)
		select $1, $2
		where exists (select 1 from planned_activities where id = $1 and user_id = $3)
			and exists (select 1 from activities where id = $2 and user_id = $3)
		on conflict(planned_activity_id) do update set activity_id = excluded.activity_id, updated_at = now()
	`, plannedID, activityID, scopedUserID(ctx))
	return err
}

func (s *Store) GetTrainingSheetWritebackOverrides(ctx context.Context, plannedID string) (map[string]string, error) {
	var raw []byte
	err := s.db.QueryRow(ctx, `
		select manual_overrides
		from training_sheet_writebacks
		where planned_activity_id = $1::uuid
			and exists (select 1 from planned_activities where id = $1::uuid and user_id = $2)
	`, plannedID, scopedUserID(ctx)).Scan(&raw)
	if err != nil {
		return nil, err
	}
	overrides := make(map[string]string)
	if len(raw) == 0 {
		return overrides, nil
	}
	if err := json.Unmarshal(raw, &overrides); err != nil {
		return nil, err
	}
	return overrides, nil
}

func (s *Store) CreateTrainingSheetWritebackJob(ctx context.Context, plannedID, activityID string) (string, error) {
	if err := s.EnsureTrainingSheetWriteback(ctx, plannedID, activityID); err != nil {
		return "", err
	}
	return s.CreateSyncJobWithPayload(ctx, trainingSheetProvider, "writeback", map[string]any{
		"plannedActivityId": plannedID,
		"activityId":        activityID,
	})
}

func (s *Store) GetTrainingSheetWriteback(ctx context.Context, plannedID string) (*TrainingSheetWritebackStatus, error) {
	status := &TrainingSheetWritebackStatus{}
	var summaryWritten, intervalsWritten, feedbackWritten, lastAttempt, cancelRequestedAt sql.NullTime
	var jobID, jobStatus sql.NullString
	err := s.db.QueryRow(ctx, `
		select planned_activity_id::text, activity_id::text, summary_status, summary_error, summary_written_at,
			interval_status, interval_error, interval_written_at,
			feedback_status, feedback_error, feedback_written_at, last_attempt_at,
			writeback_job.id, writeback_job.status, writeback_job.cancel_requested_at
		from training_sheet_writebacks
		left join lateral (
			select id::text, status, cancel_requested_at
			from sync_jobs
			where user_id = $2 and provider = $3 and kind = 'writeback'
				and payload->>'plannedActivityId' = ($1::uuid)::text
			order by created_at desc
			limit 1
		) as writeback_job on true
		where planned_activity_id = $1::uuid and exists (select 1 from planned_activities where id = $1::uuid and user_id = $2)
	`, plannedID, scopedUserID(ctx), trainingSheetProvider).Scan(&status.PlannedActivityID, &status.ActivityID, &status.SummaryStatus, &status.SummaryError, &summaryWritten,
		&status.IntervalsStatus, &status.IntervalsError, &intervalsWritten,
		&status.FeedbackStatus, &status.FeedbackError, &feedbackWritten, &lastAttempt, &jobID, &jobStatus, &cancelRequestedAt)
	if err != nil {
		return nil, err
	}
	if jobID.Valid {
		status.JobID = jobID.String
	}
	if jobStatus.Valid {
		status.JobStatus = jobStatus.String
	}
	if cancelRequestedAt.Valid {
		status.CancelRequestedAt = &cancelRequestedAt.Time
	}
	if summaryWritten.Valid {
		status.SummaryWrittenAt = &summaryWritten.Time
	}
	if intervalsWritten.Valid {
		status.IntervalsWrittenAt = &intervalsWritten.Time
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
		_, err := s.db.Exec(ctx, `update training_sheet_writebacks set summary_status = $2, summary_error = $3, summary_written_at = case when $2 = 'completed' then now() else summary_written_at end, updated_at = now() where planned_activity_id = $1 and exists (select 1 from planned_activities where id = $1 and user_id = $4)`, plannedID, status, message, scopedUserID(ctx))
		return err
	}
	if section == "intervals" {
		_, err := s.db.Exec(ctx, `update training_sheet_writebacks set interval_status = $2, interval_error = $3, interval_written_at = case when $2 = 'completed' then now() else interval_written_at end, updated_at = now() where planned_activity_id = $1 and exists (select 1 from planned_activities where id = $1 and user_id = $4)`, plannedID, status, message, scopedUserID(ctx))
		return err
	}
	_, err := s.db.Exec(ctx, `update training_sheet_writebacks set feedback_status = $2, feedback_error = $3, feedback_written_at = case when $2 = 'completed' then now() else feedback_written_at end, updated_at = now() where planned_activity_id = $1 and exists (select 1 from planned_activities where id = $1 and user_id = $4)`, plannedID, status, message, scopedUserID(ctx))
	return err
}

func (s *Store) markTrainingSheetWritebackAttempt(ctx context.Context, plannedID string) error {
	_, err := s.db.Exec(ctx, `update training_sheet_writebacks set last_attempt_at = now(), updated_at = now() where planned_activity_id = $1 and exists (select 1 from planned_activities where id = $1 and user_id = $2)`, plannedID, scopedUserID(ctx))
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
	manualOverrides, err := s.store.GetTrainingSheetWritebackOverrides(ctx, plannedID)
	if err != nil {
		return nil, err
	}
	previewPlan, err := trainingSheetPreviewUpdatePlanForActivity(planned, activity, manualOverrides)
	if err != nil {
		return nil, err
	}
	if err := s.store.markTrainingSheetWritebackAttempt(ctx, plannedID); err != nil {
		return nil, err
	}

	result := map[string]any{"plannedActivityId": plannedID, "activityId": activityID, "intervalsStatus": "not_applicable"}
	if ctx.Err() != nil {
		return s.canceledWritebackResult(ctx, planned, result)
	}
	var failures []string
	googleStatus, err := s.auth.Status(ctx)
	if ctx.Err() != nil {
		return s.canceledWritebackResult(ctx, planned, result)
	}
	if err != nil {
		failures = append(failures, err.Error())
	} else if !googleStatus.WriteReady {
		failures = append(failures, "Google Sheets write access requires reconnecting the Google account")
	}

	if len(failures) == 0 {
		if ctx.Err() != nil {
			return s.canceledWritebackResult(ctx, planned, result)
		}
		if err := s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "summary", "running", ""); err != nil {
			return nil, err
		}
		summaryStatus, summaryError := s.writeSummary(ctx, planned, previewPlan.Updates)
		if ctx.Err() != nil {
			return s.canceledWritebackResult(ctx, planned, result)
		}
		if summaryError != nil && summaryStatus == "failed" {
			failures = append(failures, summaryError.Error())
		}
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "summary", summaryStatus, summaryErrorString(summaryError))
		result["summaryStatus"] = summaryStatus

		if ctx.Err() != nil {
			return s.canceledWritebackResult(ctx, planned, result)
		}
		if err := s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "intervals", "running", ""); err != nil {
			return nil, err
		}
		result["intervalsStatus"] = "running"
		intervalsStatus, intervalsError := s.writeIntervals(ctx, planned, previewPlan.Updates, previewPlan.IntervalError, previewPlan.IntervalWarning)
		if ctx.Err() != nil {
			return s.canceledWritebackResult(ctx, planned, result)
		}
		if intervalsError != nil && intervalsStatus == "failed" {
			failures = append(failures, intervalsError.Error())
		}
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "intervals", intervalsStatus, summaryErrorString(intervalsError))
		result["intervalsStatus"] = intervalsStatus
	} else {
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "summary", "failed", failures[0])
		result["summaryStatus"] = "failed"
		if workoutTableFromPlanned(planned) == nil {
			_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "intervals", "not_applicable", "")
			result["intervalsStatus"] = "not_applicable"
		} else {
			_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "intervals", "failed", failures[0])
			result["intervalsStatus"] = "failed"
		}
	}

	// Feedback can be entered immediately after matching, while this job is
	// already writing the summary and interval table. Reload it here so the
	// write-back uses the latest saved reflection rather than the snapshot
	// taken when the job started.
	if latestActivity, latestErr := s.store.GetActivity(ctx, activityID); latestErr == nil {
		activity = latestActivity
	}
	feedbackCell := feedbackCellForPlanned(planned)
	if ctx.Err() != nil {
		return s.canceledWritebackResult(ctx, planned, result)
	}
	if feedbackCell == "" {
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "feedback", "not_applicable", "")
		result["feedbackStatus"] = "not_applicable"
	} else if strings.TrimSpace(activity.Feedback) == "" {
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "feedback", "not_provided", "")
		result["feedbackStatus"] = "not_provided"
	} else if len(failures) == 0 {
		_ = s.store.updateTrainingSheetWritebackSection(ctx, plannedID, "feedback", "running", "")
		feedbackStatus, feedbackError := s.writeFeedback(ctx, planned, previewPlan.Updates)
		if ctx.Err() != nil {
			return s.canceledWritebackResult(ctx, planned, result)
		}
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

func (s *TrainingSheetWritebackService) canceledWritebackResult(ctx context.Context, planned PlannedActivity, result map[string]any) (map[string]any, error) {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	if status, _ := result["summaryStatus"].(string); status == "" || status == "running" {
		_ = s.store.updateTrainingSheetWritebackSection(persistCtx, planned.ID, "summary", "canceled", "Canceled by user")
		result["summaryStatus"] = "canceled"
	}
	if status, _ := result["intervalsStatus"].(string); status == "running" {
		_ = s.store.updateTrainingSheetWritebackSection(persistCtx, planned.ID, "intervals", "canceled", "Canceled by user")
		result["intervalsStatus"] = "canceled"
	}
	feedbackCell := feedbackCellForPlanned(planned)
	if feedbackCell == "" {
		_ = s.store.updateTrainingSheetWritebackSection(persistCtx, planned.ID, "feedback", "not_applicable", "")
		result["feedbackStatus"] = "not_applicable"
	} else if status, _ := result["feedbackStatus"].(string); status == "" || status == "running" {
		_ = s.store.updateTrainingSheetWritebackSection(persistCtx, planned.ID, "feedback", "canceled", "Canceled by user")
		result["feedbackStatus"] = "canceled"
	}
	return result, context.Canceled
}

func (s *TrainingSheetWritebackService) writeIntervals(ctx context.Context, planned PlannedActivity, previewUpdates []trainingSheetPreviewUpdate, mappingErr error, mappingWarning string) (string, error) {
	table := workoutTableFromPlanned(planned)
	if table == nil {
		return "not_applicable", nil
	}
	if mappingErr != nil {
		return "skipped", mappingErr
	}
	updates, manualRanges := writebackUpdatesForSection(previewUpdates, "intervals")
	heartColumns := map[string]bool{
		table.Columns[trainingSheetMetricAvgHeart]: true,
		table.Columns[trainingSheetMetricMaxHeart]: true,
	}
	status, err := s.writeUpdatesPreservingExistingWith(ctx, planned.WorkbookID, updates, func(update googleValueRangeUpdate, existing [][][]string, index int) bool {
		if manualRanges[update.Range] {
			return true
		}
		if !heartColumns[sheetRangeColumn(update.Range)] || index >= len(existing) {
			return false
		}
		return rangeHasZeroClockValue(existing[index])
	})
	if mappingWarning == "" || err != nil && status == "failed" {
		return status, err
	}
	if status == "completed" {
		return "completed_with_warnings", fmt.Errorf("%s", mappingWarning)
	}
	if err != nil {
		return status, fmt.Errorf("%s; %w", mappingWarning, err)
	}
	return status, fmt.Errorf("%s", mappingWarning)
}

func (s *TrainingSheetWritebackService) writeSummary(ctx context.Context, planned PlannedActivity, previewUpdates []trainingSheetPreviewUpdate) (string, error) {
	updates, manualRanges := writebackUpdatesForSection(previewUpdates, "summary")
	return s.writeUpdatesPreservingExistingWith(ctx, planned.WorkbookID, updates, func(update googleValueRangeUpdate, _ [][][]string, _ int) bool {
		return manualRanges[update.Range]
	})
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

func (s *TrainingSheetWritebackService) writeFeedback(ctx context.Context, planned PlannedActivity, previewUpdates []trainingSheetPreviewUpdate) (string, error) {
	updates, _ := writebackUpdatesForSection(previewUpdates, "feedback")
	if len(updates) == 0 {
		return "not_applicable", nil
	}
	if err := retryGoogle(ctx, func() error { return s.auth.WriteRanges(ctx, planned.WorkbookID, updates) }); err != nil {
		return "failed", err
	}
	return "completed", nil
}

func writebackUpdatesForSection(previewUpdates []trainingSheetPreviewUpdate, section string) ([]googleValueRangeUpdate, map[string]bool) {
	updates := make([]googleValueRangeUpdate, 0)
	manualRanges := make(map[string]bool)
	for _, item := range previewUpdates {
		if item.Section != section {
			continue
		}
		updates = append(updates, item.Update)
		if item.ManualOverride {
			manualRanges[item.Update.Range] = true
		}
	}
	return updates, manualRanges
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
	return s.writeUpdatesPreservingExistingWith(ctx, workbookID, updates, nil)
}

func (s *TrainingSheetWritebackService) writeUpdatesPreservingExistingWith(ctx context.Context, workbookID string, updates []googleValueRangeUpdate, replaceExisting func(googleValueRangeUpdate, [][][]string, int) bool) (string, error) {
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
		if index < len(existing) && rangeHasValue(existing[index]) && (replaceExisting == nil || !replaceExisting(update, existing, index)) {
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

func sheetRangeColumn(rangeName string) string {
	cell := rangeName
	if separator := strings.LastIndex(cell, "!"); separator >= 0 {
		cell = cell[separator+1:]
	}
	return strings.ToUpper(strings.TrimRight(cell, "0123456789"))
}

func rangeHasZeroClockValue(values [][]string) bool {
	if len(values) == 0 || len(values[0]) == 0 {
		return false
	}
	for _, row := range values {
		for _, value := range row {
			if strings.TrimSpace(value) != "0:00" && strings.TrimSpace(value) != "0:00:00" {
				return false
			}
		}
	}
	return true
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
