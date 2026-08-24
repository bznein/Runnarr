package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const trainingSheetReconciliationPageSize = 20

var errTrainingSheetReconciliationChanged = errors.New("the training sheet changed since this discrepancy was reviewed")

type trainingSheetReconciliationCandidate struct {
	PlannedActivityID string
	ActivityID        string
	ActivityName      string
	StartTime         time.Time
}

type TrainingSheetReconciliationChange struct {
	Range         string `json:"range"`
	Label         string `json:"label"`
	CurrentValue  string `json:"currentValue"`
	ProposedValue string `json:"proposedValue"`
}

type TrainingSheetReconciliationItem struct {
	ActivityID        string                              `json:"activityId"`
	ActivityName      string                              `json:"activityName"`
	ActivityStartTime time.Time                           `json:"activityStartTime"`
	PlannedActivityID string                              `json:"plannedActivityId"`
	PlannedName       string                              `json:"plannedName"`
	SheetTitle        string                              `json:"sheetTitle"`
	SheetURL          string                              `json:"sheetUrl"`
	Fingerprint       string                              `json:"fingerprint"`
	Changes           []TrainingSheetReconciliationChange `json:"changes"`
	updates           []googleValueRangeUpdate
}

type TrainingSheetReconciliationResult struct {
	Item       *TrainingSheetReconciliationItem `json:"item,omitempty"`
	NextOffset int                              `json:"nextOffset"`
	Scanned    int                              `json:"scanned"`
	Skipped    int                              `json:"skipped"`
	Done       bool                             `json:"done"`
}

func (s *Store) TrainingSheetReconciliationCandidates(ctx context.Context, notBefore time.Time, offset, limit int) ([]trainingSheetReconciliationCandidate, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > trainingSheetReconciliationPageSize {
		limit = trainingSheetReconciliationPageSize
	}
	rows, err := s.db.Query(ctx, `
		select planned.id::text, activity.id::text, activity.name, activity.start_time
		from planned_activities planned
		join activities activity on activity.id = planned.matched_activity_id and activity.user_id = planned.user_id
		where planned.user_id = $1 and planned.source = $2
			and planned.matched_activity_id is not null
			and activity.source = $3
			and activity.start_time >= $4
			and exists (
				select 1 from activity_intervals ai
				where ai.activity_id = activity.id
					and lower(ai.category) = 'active'
					and ai.raw ? 'averageSpeed'
			)
		order by activity.start_time desc, activity.id desc
		limit $5 offset $6
	`, scopedUserID(ctx), trainingSheetProvider, garminProvider, notBefore, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := make([]trainingSheetReconciliationCandidate, 0, limit)
	for rows.Next() {
		var candidate trainingSheetReconciliationCandidate
		if err := rows.Scan(&candidate.PlannedActivityID, &candidate.ActivityID, &candidate.ActivityName, &candidate.StartTime); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (s *TrainingSheetWritebackService) NextReconciliation(ctx context.Context, notBefore time.Time, offset int) (TrainingSheetReconciliationResult, error) {
	if err := s.requireTrainingSheetWriteAccess(ctx); err != nil {
		return TrainingSheetReconciliationResult{}, err
	}
	candidates, err := s.store.TrainingSheetReconciliationCandidates(ctx, notBefore, offset, trainingSheetReconciliationPageSize)
	if err != nil {
		return TrainingSheetReconciliationResult{}, err
	}
	result := TrainingSheetReconciliationResult{NextOffset: offset, Done: len(candidates) < trainingSheetReconciliationPageSize}
	for index, candidate := range candidates {
		result.Scanned++
		result.NextOffset = offset + index + 1
		item, err := s.reconciliationItem(ctx, candidate.PlannedActivityID, candidate.ActivityID)
		if errors.Is(err, errTrainingSheetReconciliationNotApplicable) {
			result.Skipped++
			continue
		}
		if err != nil {
			return TrainingSheetReconciliationResult{}, err
		}
		if item != nil {
			item.ActivityName = candidate.ActivityName
			item.ActivityStartTime = candidate.StartTime
			result.Item = item
			result.Done = false
			return result, nil
		}
	}
	return result, nil
}

var errTrainingSheetReconciliationNotApplicable = errors.New("activity has no reconcilable Garmin interval pace")

func (s *TrainingSheetWritebackService) reconciliationItem(ctx context.Context, plannedID, activityID string) (*TrainingSheetReconciliationItem, error) {
	planned, err := s.store.GetPlannedActivity(ctx, plannedID)
	if err != nil {
		return nil, err
	}
	activity, err := s.store.GetActivity(ctx, activityID)
	if err != nil {
		return nil, err
	}
	if planned.Source != trainingSheetProvider || planned.MatchedActivityID != activityID {
		return nil, errTrainingSheetReconciliationNotApplicable
	}
	if !applyGarminDisplayedIntervalPaces(&activity) {
		return nil, errTrainingSheetReconciliationNotApplicable
	}
	table := workoutTableFromPlanned(planned)
	if table == nil || table.Columns[trainingSheetMetricAvgPace] == "" {
		return nil, errTrainingSheetReconciliationNotApplicable
	}
	plan, err := intervalUpdatesForPlannedActivity(planned, activity)
	if err != nil {
		return nil, errTrainingSheetReconciliationNotApplicable
	}
	paceColumn := table.Columns[trainingSheetMetricAvgPace]
	updates := make([]googleValueRangeUpdate, 0)
	labels := make([]string, 0)
	for _, update := range plan.Updates {
		if sheetRangeColumn(update.Range) != paceColumn {
			continue
		}
		updates = append(updates, update)
		labels = append(labels, intervalLabelForRange(table, update.Range))
	}
	if len(updates) == 0 {
		return nil, errTrainingSheetReconciliationNotApplicable
	}
	var existing [][][]string
	if err := retryGoogle(ctx, func() error {
		var readErr error
		existing, readErr = s.auth.ReadRanges(ctx, planned.WorkbookID, rangesForUpdates(updates))
		return readErr
	}); err != nil {
		return nil, err
	}
	changes, writes := trainingSheetReconciliationDiff(updates, labels, existing)
	if len(changes) == 0 {
		return nil, nil
	}
	return &TrainingSheetReconciliationItem{
		ActivityID: activityID, PlannedActivityID: plannedID, PlannedName: planned.Name,
		SheetTitle: planned.SheetTitle, SheetURL: planned.SourceURL,
		Fingerprint: trainingSheetReconciliationFingerprint(plannedID, activityID, changes),
		Changes:     changes, updates: writes,
	}, nil
}

func trainingSheetReconciliationDiff(updates []googleValueRangeUpdate, labels []string, existing [][][]string) ([]TrainingSheetReconciliationChange, []googleValueRangeUpdate) {
	changes := make([]TrainingSheetReconciliationChange, 0, len(updates))
	writes := make([]googleValueRangeUpdate, 0, len(updates))
	for index, update := range updates {
		current := normalizedTrainingSheetValue(trainingSheetPreviewCurrentValue(existing, index))
		proposed := normalizedTrainingSheetValue(previewValueText(update.Values))
		if current == proposed {
			continue
		}
		label := "Structured interval"
		if index < len(labels) {
			label = labels[index]
		}
		changes = append(changes, TrainingSheetReconciliationChange{Range: update.Range, Label: label, CurrentValue: current, ProposedValue: proposed})
		writes = append(writes, update)
	}
	return changes, writes
}

func (s *TrainingSheetWritebackService) ApplyReconciliation(ctx context.Context, plannedID, activityID, fingerprint string) (int, error) {
	if err := s.requireTrainingSheetWriteAccess(ctx); err != nil {
		return 0, err
	}
	item, err := s.reconciliationItem(ctx, plannedID, activityID)
	if err != nil {
		return 0, err
	}
	if item == nil || item.Fingerprint != strings.TrimSpace(fingerprint) {
		return 0, errTrainingSheetReconciliationChanged
	}
	planned, err := s.store.GetPlannedActivity(ctx, plannedID)
	if err != nil {
		return 0, err
	}
	if err := retryGoogle(ctx, func() error { return s.auth.WriteRanges(ctx, planned.WorkbookID, item.updates) }); err != nil {
		return 0, err
	}
	return len(item.updates), nil
}

func (s *TrainingSheetWritebackService) requireTrainingSheetWriteAccess(ctx context.Context) error {
	status, err := s.auth.Status(ctx)
	if err != nil {
		return err
	}
	if !status.WriteReady {
		return fmt.Errorf("Google Sheets write access requires reconnecting the Google account")
	}
	return nil
}

func applyGarminDisplayedIntervalPaces(activity *Activity) bool {
	if activity == nil || len(activity.Intervals) == 0 {
		return false
	}
	applied := false
	for index := range activity.Intervals {
		interval := &activity.Intervals[index]
		if !strings.EqualFold(strings.TrimSpace(interval.Category), "active") {
			continue
		}
		speed, ok := garminRawNumber(interval.Raw, "averageSpeed")
		if !ok || speed <= 0 {
			continue
		}
		pace := 1000 / speed
		interval.AvgPaceSPKM = &pace
		applied = true
	}
	return applied
}

func garminRawNumber(raw map[string]any, key string) (float64, bool) {
	if raw == nil {
		return 0, false
	}
	value, ok := raw[key]
	if !ok || value == nil {
		return 0, false
	}
	var parsed float64
	switch typed := value.(type) {
	case float64:
		parsed = typed
	case float32:
		parsed = float64(typed)
	case int:
		parsed = float64(typed)
	case int64:
		parsed = float64(typed)
	case json.Number:
		var err error
		parsed, err = typed.Float64()
		if err != nil {
			return 0, false
		}
	case string:
		var err error
		parsed, err = strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return parsed, !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}

func normalizedTrainingSheetValue(value string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "'"))
}

func trainingSheetReconciliationFingerprint(plannedID, activityID string, changes []TrainingSheetReconciliationChange) string {
	payload, _ := json.Marshal(map[string]any{
		"plannedActivityId": plannedID,
		"activityId":        activityID,
		"changes":           changes,
	})
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func reconciliationCandidateExists(ctx context.Context, store *Store, plannedID, activityID string) error {
	planned, err := store.GetPlannedActivity(ctx, plannedID)
	if err != nil {
		return err
	}
	if planned.Source != trainingSheetProvider || planned.MatchedActivityID != activityID {
		return pgx.ErrNoRows
	}
	return nil
}
