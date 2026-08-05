package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

func shortEventHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:12])
}

func (s *Store) publishWorkoutChangeNotification(ctx context.Context, workout Workout, change string) {
	severity := "info"
	kind := "workout_" + change
	title := "Workout generated: " + workout.Name
	body := "A workout was generated from the training plan."
	if change == "updated" {
		title = "Workout updated: " + workout.Name
		body = "The workout prescription or planned date changed."
	} else if change == "archived" {
		title = "Workout removed: " + workout.Name
		body = "The training plan no longer contains a structured workout for this activity."
	}
	if workout.ParseStatus == "warning" {
		severity = "warning"
		body = "The workout was generated with details that need review."
	} else if workout.ParseStatus == "error" {
		severity = "error"
		body = "Runnarr could not fully parse this workout. Review it before scheduling."
	}
	_, _ = s.PublishNotification(ctx, NotificationInput{
		ThreadKey: "workout:" + workout.ID,
		EventKey:  fmt.Sprintf("%s:%s", kind, shortEventHash(workout.Name, workout.SourceText, workout.SourceHash, workout.ParseStatus, workout.ScheduledDate, fmt.Sprint(workout.ArchivedAt))),
		Category:  notificationCategoryWorkoutChanges,
		Kind:      kind, Severity: severity, Title: title, Body: body,
		ActionPath: "/workouts/" + workout.ID,
	})
}

func (s *Server) publishGarminWorkoutNotifications(ctx context.Context, result WorkoutReconcileResult, reconcileErr error) {
	if reconcileErr != nil {
		_, _ = s.store.PublishNotification(ctx, NotificationInput{
			ThreadKey: "garmin-workout-reconcile", EventKey: "failed:" + shortEventHash(reconcileErr.Error()),
			Category: notificationCategoryGarminCalendar, Kind: "garmin_reconcile_failed", Severity: "error",
			Title: "Garmin workout sync failed", Body: safeNotificationError(reconcileErr.Error()), ActionPath: "/settings?section=workouts",
		})
		return
	}
	type actionGroup struct {
		actions []WorkoutReconcileAction
	}
	groups := make(map[string]*actionGroup)
	order := make([]string, 0)
	for _, action := range result.Actions {
		if action.WorkoutID == "" || action.Action == "delete_template" {
			continue
		}
		if groups[action.WorkoutID] == nil {
			groups[action.WorkoutID] = &actionGroup{}
			order = append(order, action.WorkoutID)
		}
		groups[action.WorkoutID].actions = append(groups[action.WorkoutID].actions, action)
	}
	for _, workoutID := range order {
		workout, err := s.store.GetWorkout(ctx, workoutID)
		if err != nil {
			continue
		}
		actions := groups[workoutID].actions
		var failed *WorkoutReconcileAction
		for index := range actions {
			if actions[index].Status == "error" || actions[index].Status == "conflict" {
				failed = &actions[index]
			}
		}
		if failed != nil {
			_, _ = s.store.PublishNotification(ctx, NotificationInput{
				ThreadKey: "workout:" + workoutID,
				EventKey:  "garmin_failure:" + shortEventHash(failed.Action, failed.Date, failed.Status, failed.Message),
				Category:  notificationCategoryGarminCalendar, Kind: "garmin_" + failed.Status, Severity: "error",
				Title: "Garmin needs attention: " + workout.Name, Body: safeNotificationError(failed.Message),
				ActionPath: "/workouts/" + workoutID + "?section=garmin",
			})
			continue
		}
		last := actions[len(actions)-1]
		if last.Action == "none" {
			continue
		}
		kind := ""
		title := ""
		body := ""
		switch {
		case last.Action == "recover" && last.Status == "scheduled":
			kind, title = "garmin_recovered", "Garmin scheduling recovered: "+workout.Name
			body = "The workout is now verified on the Garmin calendar for " + last.Date + "."
		case last.Action == "schedule" && last.Status == "scheduled":
			kind, title = "garmin_scheduled", "Scheduled on Garmin: "+workout.Name
			body = "Scheduled for " + last.Date + "."
			if len(actions) > 1 {
				kind, title = "garmin_rescheduled", "Updated on Garmin: "+workout.Name
				body = "The Garmin calendar workout was updated for " + last.Date + "."
			}
		case last.Action == "unschedule" && (last.Status == "removed" || last.Status == "deleted"):
			kind, title = "garmin_removed", "Removed from Garmin: "+workout.Name
			body = "The Runnarr-managed calendar entry was removed from Garmin."
		}
		if kind == "" {
			continue
		}
		_, _ = s.store.PublishNotification(ctx, NotificationInput{
			ThreadKey: "workout:" + workoutID, EventKey: kind + ":" + shortEventHash(last.Date, fmt.Sprint(workout.Revision)),
			Category: notificationCategoryGarminCalendar, Kind: kind, Severity: "success",
			Title: title, Body: body, ActionPath: "/workouts/" + workoutID + "?section=garmin",
		})
	}
}

func safeNotificationError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "Open Runnarr for details."
	}
	runes := []rune(message)
	if len(runes) > 240 {
		message = string(runes[:239]) + "…"
	}
	return message
}

func activityMatchThreadKey(activityID, plannedID string) string {
	return "activity-match:" + activityID + ":" + plannedID
}

func activityAutoMatchNotificationInput(activityID string, planned PlannedActivity) NotificationInput {
	body := "Add your RPE while the run is still fresh."
	if feedbackCellForPlanned(planned) != "" {
		body = "Add your RPE and feedback while the run is still fresh."
	}
	return NotificationInput{
		ThreadKey: activityMatchThreadKey(activityID, planned.ID), EventKey: "auto_matched",
		Category: notificationCategoryActivityMatching, Kind: "activity_auto_matched", Severity: "info",
		Title:      "Activity matched: " + planned.Name,
		Body:       body,
		ActionPath: "/activities/" + activityID + "#check-in",
	}
}

func (s *Store) publishActivityAutoMatchNotification(ctx context.Context, activityID string, planned PlannedActivity) {
	_, _ = s.PublishNotification(ctx, activityAutoMatchNotificationInput(activityID, planned))
}

func (s *Store) notificationThreadHasWritebackFailure(ctx context.Context, threadKey string) bool {
	var exists bool
	err := s.db.QueryRow(ctx, `
		select exists(
			select 1 from notification_events event
			join notification_threads thread on thread.id = event.thread_id
			where thread.user_id = $1 and thread.thread_key = $2
				and event.kind in ('sheet_writeback_failed', 'sheet_writeback_partial')
		)
	`, scopedUserID(ctx), threadKey).Scan(&exists)
	return err == nil && exists
}

func (s *Server) publishSheetWritebackNotification(ctx context.Context, plannedID, activityID string, payload map[string]any, writeErr error) {
	if writeErr != nil && errors.Is(writeErr, context.Canceled) {
		return
	}
	planned, err := s.store.GetPlannedActivity(ctx, plannedID)
	if err != nil {
		return
	}
	threadKey := activityMatchThreadKey(activityID, plannedID)
	statuses := make([]string, 0, 3)
	for _, key := range []string{"summaryStatus", "intervalsStatus", "feedbackStatus"} {
		if status, ok := payload[key].(string); ok && status != "" {
			statuses = append(statuses, status)
		}
	}
	joinedStatus := strings.Join(statuses, ",")
	if writeErr != nil {
		_, _ = s.store.PublishNotification(ctx, NotificationInput{
			ThreadKey: threadKey, EventKey: "writeback_failed:" + shortEventHash(writeErr.Error(), joinedStatus),
			Category: notificationCategorySheetWriteback, Kind: "sheet_writeback_failed", Severity: "error",
			Title: "Training sheet writeback failed: " + planned.Name,
			Body:  safeNotificationError(writeErr.Error()), ActionPath: "/activities/" + activityID + "?section=writeback",
		})
		return
	}
	partial := false
	for _, status := range statuses {
		if status == "failed" || status == "skipped" {
			partial = true
			break
		}
	}
	if partial {
		_, _ = s.store.PublishNotification(ctx, NotificationInput{
			ThreadKey: threadKey, EventKey: "writeback_partial:" + shortEventHash(joinedStatus),
			Category: notificationCategorySheetWriteback, Kind: "sheet_writeback_partial", Severity: "warning",
			Title: "Training sheet writeback needs review: " + planned.Name,
			Body:  "Some training-sheet sections could not be written.", ActionPath: "/activities/" + activityID + "?section=writeback",
		})
		return
	}
	if s.store.notificationThreadHasWritebackFailure(ctx, threadKey) {
		_, _ = s.store.PublishNotification(ctx, NotificationInput{
			ThreadKey: threadKey, EventKey: "writeback_recovered:" + shortEventHash(joinedStatus),
			Category: notificationCategorySheetWriteback, Kind: "sheet_writeback_recovered", Severity: "success",
			Title: "Training sheet writeback recovered: " + planned.Name,
			Body:  "The activity was written to the training sheet successfully.", ActionPath: "/activities/" + activityID + "?section=writeback",
		})
	}
}
