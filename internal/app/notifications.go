package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	notificationCategoryWorkoutChanges   = "workout_changes"
	notificationCategoryGarminCalendar   = "garmin_calendar"
	notificationCategoryActivityMatching = "activity_matching"
	notificationCategorySheetWriteback   = "sheet_writeback"

	notificationModeOff       = "off"
	notificationModeInApp     = "in_app"
	notificationModeInAppPush = "in_app_push"
)

var notificationCategories = []string{
	notificationCategoryWorkoutChanges,
	notificationCategoryGarminCalendar,
	notificationCategoryActivityMatching,
	notificationCategorySheetWriteback,
}

var ErrInvalidNotification = errors.New("invalid notification")

type NotificationInput struct {
	ThreadKey  string
	EventKey   string
	Category   string
	Kind       string
	Severity   string
	Title      string
	Body       string
	ActionPath string
}

type NotificationEvent struct {
	ID         string    `json:"id"`
	Category   string    `json:"category"`
	Kind       string    `json:"kind"`
	Severity   string    `json:"severity"`
	Title      string    `json:"title"`
	Body       string    `json:"body,omitempty"`
	ActionPath string    `json:"actionPath"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Notification struct {
	ID          string              `json:"id"`
	Category    string              `json:"category"`
	Kind        string              `json:"kind"`
	Severity    string              `json:"severity"`
	Title       string              `json:"title"`
	Body        string              `json:"body,omitempty"`
	ActionPath  string              `json:"actionPath"`
	ReadAt      *time.Time          `json:"readAt,omitempty"`
	CreatedAt   time.Time           `json:"createdAt"`
	LastEventAt time.Time           `json:"lastEventAt"`
	EventCount  int                 `json:"eventCount"`
	Events      []NotificationEvent `json:"events,omitempty"`
}

type NotificationPage struct {
	Notifications []Notification `json:"notifications"`
	UnreadCount   int            `json:"unreadCount"`
	NextCursor    string         `json:"nextCursor,omitempty"`
}

type NotificationSettings struct {
	Categories     map[string]string `json:"categories"`
	VAPIDPublicKey string            `json:"vapidPublicKey,omitempty"`
}

type WebPushSubscription struct {
	ID            string     `json:"id"`
	DeviceName    string     `json:"deviceName"`
	UserAgent     string     `json:"userAgent,omitempty"`
	LastSeenAt    time.Time  `json:"lastSeenAt"`
	LastSuccessAt *time.Time `json:"lastSuccessAt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type webPushOutboxItem struct {
	ID                     string
	SubscriptionID         string
	SubscriptionCiphertext []byte
	Payload                []byte
	Attempts               int
}

type notificationCursor struct {
	Time time.Time `json:"t"`
	ID   string    `json:"id"`
}

func defaultNotificationMode(category string) string {
	if category == notificationCategoryActivityMatching {
		return notificationModeInApp
	}
	return notificationModeInAppPush
}

func validNotificationCategory(category string) bool {
	for _, candidate := range notificationCategories {
		if category == candidate {
			return true
		}
	}
	return false
}

func validNotificationMode(mode string) bool {
	return mode == notificationModeOff || mode == notificationModeInApp || mode == notificationModeInAppPush
}

func validNotificationSeverity(severity string) bool {
	return severity == "info" || severity == "success" || severity == "warning" || severity == "error"
}

func validNotificationActionPath(path string) bool {
	parsed, err := url.Parse(path)
	return err == nil && strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && parsed.IsAbs() == false && parsed.Host == ""
}

func normalizeNotificationInput(input NotificationInput) (NotificationInput, error) {
	input.ThreadKey = strings.TrimSpace(input.ThreadKey)
	input.EventKey = strings.TrimSpace(input.EventKey)
	input.Category = strings.TrimSpace(input.Category)
	input.Kind = strings.TrimSpace(input.Kind)
	input.Severity = strings.TrimSpace(input.Severity)
	input.Title = truncateNotificationText(strings.TrimSpace(input.Title), 180)
	input.Body = truncateNotificationText(strings.TrimSpace(input.Body), 1000)
	input.ActionPath = strings.TrimSpace(input.ActionPath)
	if input.ActionPath == "" {
		input.ActionPath = "/notifications"
	}
	if input.ThreadKey == "" || len(input.ThreadKey) > 240 || input.EventKey == "" || len(input.EventKey) > 240 ||
		!validNotificationCategory(input.Category) || input.Kind == "" || len(input.Kind) > 80 ||
		!validNotificationSeverity(input.Severity) || input.Title == "" || !validNotificationActionPath(input.ActionPath) {
		return NotificationInput{}, ErrInvalidNotification
	}
	return input, nil
}

func truncateNotificationText(value string, limit int) string {
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}

func (s *Store) PublishNotification(ctx context.Context, input NotificationInput) (bool, error) {
	input, err := normalizeNotificationInput(input)
	if err != nil {
		return false, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	mode := defaultNotificationMode(input.Category)
	if err := tx.QueryRow(ctx, `select mode from notification_preferences where user_id = $1 and category = $2`, scopedUserID(ctx), input.Category).Scan(&mode); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if mode == notificationModeOff {
		return false, tx.Commit(ctx)
	}

	var threadID string
	err = tx.QueryRow(ctx, `
		insert into notification_threads(user_id, thread_key, category, kind, severity, title, body, action_path)
		values($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict(user_id, thread_key) do update set thread_key = excluded.thread_key
		returning id::text
	`, scopedUserID(ctx), input.ThreadKey, input.Category, input.Kind, input.Severity, input.Title, input.Body, input.ActionPath).Scan(&threadID)
	if err != nil {
		return false, err
	}

	var eventID string
	err = tx.QueryRow(ctx, `
		insert into notification_events(thread_id, event_key, category, kind, severity, title, body, action_path)
		values($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict(thread_id, event_key) do nothing
		returning id::text
	`, threadID, input.EventKey, input.Category, input.Kind, input.Severity, input.Title, input.Body, input.ActionPath).Scan(&eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		update notification_threads set category = $2, kind = $3, severity = $4, title = $5, body = $6,
			action_path = $7, read_at = null, last_event_at = now(), updated_at = now()
		where id = $1
	`, threadID, input.Category, input.Kind, input.Severity, input.Title, input.Body, input.ActionPath); err != nil {
		return false, err
	}

	if mode == notificationModeInAppPush {
		var unreadCount int
		if err := tx.QueryRow(ctx, `select count(*) from notification_threads where user_id = $1 and read_at is null`, scopedUserID(ctx)).Scan(&unreadCount); err != nil {
			return false, err
		}
		payload, err := json.Marshal(map[string]any{
			"notificationId": threadID,
			"eventId":        eventID,
			"title":          input.Title,
			"body":           input.Body,
			"severity":       input.Severity,
			"actionPath":     input.ActionPath,
			"tag":            "runnarr-" + threadID,
			"unreadCount":    unreadCount,
		})
		if err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, `
			insert into web_push_outbox(thread_id, event_id, subscription_id, payload)
			select $1, $2, id, $3::jsonb from web_push_subscriptions where user_id = $4
			on conflict(thread_id, subscription_id) do update set
				event_id = excluded.event_id, payload = excluded.payload, attempts = 0,
				available_at = now(), locked_at = null, last_error = '', updated_at = now()
		`, threadID, eventID, payload, scopedUserID(ctx)); err != nil {
			return false, err
		}
	}
	return true, tx.Commit(ctx)
}

func encodeNotificationCursor(cursor notificationCursor) string {
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeNotificationCursor(raw string) (notificationCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return notificationCursor{}, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return notificationCursor{}, ErrInvalidNotification
	}
	var cursor notificationCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Time.IsZero() || cursor.ID == "" {
		return notificationCursor{}, ErrInvalidNotification
	}
	var id pgtype.UUID
	if err := id.Scan(cursor.ID); err != nil || !id.Valid {
		return notificationCursor{}, ErrInvalidNotification
	}
	return cursor, nil
}

func (s *Store) ListNotifications(ctx context.Context, limit int, cursorRaw string, unreadOnly bool) (NotificationPage, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	cursor, err := decodeNotificationCursor(cursorRaw)
	if err != nil {
		return NotificationPage{}, err
	}
	rows, err := s.db.Query(ctx, `
		select thread.id::text, thread.category, thread.kind, thread.severity, thread.title, thread.body,
			thread.action_path, thread.read_at, thread.created_at, thread.last_event_at,
			(select count(*) from notification_events event where event.thread_id = thread.id)
		from notification_threads thread
		where thread.user_id = $1
			and (not $2 or thread.read_at is null)
			and ($3::timestamptz is null or (thread.last_event_at, thread.id) < ($3::timestamptz, $4::uuid))
		order by thread.last_event_at desc, thread.id desc
		limit $5
	`, scopedUserID(ctx), unreadOnly, nullableCursorTime(cursor.Time), nullableCursorID(cursor.ID), limit+1)
	if err != nil {
		return NotificationPage{}, err
	}
	defer rows.Close()
	items := make([]Notification, 0, limit+1)
	for rows.Next() {
		var item Notification
		if err := rows.Scan(&item.ID, &item.Category, &item.Kind, &item.Severity, &item.Title, &item.Body,
			&item.ActionPath, &item.ReadAt, &item.CreatedAt, &item.LastEventAt, &item.EventCount); err != nil {
			return NotificationPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return NotificationPage{}, err
	}
	page := NotificationPage{Notifications: items}
	if len(items) > limit {
		last := items[limit-1]
		page.Notifications = items[:limit]
		page.NextCursor = encodeNotificationCursor(notificationCursor{Time: last.LastEventAt, ID: last.ID})
	}
	if err := s.db.QueryRow(ctx, `select count(*) from notification_threads where user_id = $1 and read_at is null`, scopedUserID(ctx)).Scan(&page.UnreadCount); err != nil {
		return NotificationPage{}, err
	}
	return page, nil
}

func nullableCursorTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullableCursorID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) GetNotification(ctx context.Context, id string) (Notification, error) {
	var item Notification
	err := s.db.QueryRow(ctx, `
		select thread.id::text, thread.category, thread.kind, thread.severity, thread.title, thread.body,
			thread.action_path, thread.read_at, thread.created_at, thread.last_event_at,
			(select count(*) from notification_events event where event.thread_id = thread.id)
		from notification_threads thread where thread.id = $1 and thread.user_id = $2
	`, id, scopedUserID(ctx)).Scan(&item.ID, &item.Category, &item.Kind, &item.Severity, &item.Title, &item.Body,
		&item.ActionPath, &item.ReadAt, &item.CreatedAt, &item.LastEventAt, &item.EventCount)
	if err != nil {
		return Notification{}, err
	}
	rows, err := s.db.Query(ctx, `
		select id::text, category, kind, severity, title, body, action_path, created_at
		from notification_events where thread_id = $1 order by created_at, id
	`, id)
	if err != nil {
		return Notification{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var event NotificationEvent
		if err := rows.Scan(&event.ID, &event.Category, &event.Kind, &event.Severity, &event.Title, &event.Body, &event.ActionPath, &event.CreatedAt); err != nil {
			return Notification{}, err
		}
		item.Events = append(item.Events, event)
	}
	return item, rows.Err()
}

func (s *Store) SetNotificationRead(ctx context.Context, id string, read bool) error {
	command, err := s.db.Exec(ctx, `update notification_threads set read_at = case when $3 then now() else null end, updated_at = now() where id = $1 and user_id = $2`, id, scopedUserID(ctx), read)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) MarkAllNotificationsRead(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `update notification_threads set read_at = now(), updated_at = now() where user_id = $1 and read_at is null`, scopedUserID(ctx))
	return err
}

func (s *Store) DeleteNotification(ctx context.Context, id string) error {
	command, err := s.db.Exec(ctx, `delete from notification_threads where id = $1 and user_id = $2`, id, scopedUserID(ctx))
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ClearNotifications(ctx context.Context, readOnly bool) error {
	_, err := s.db.Exec(ctx, `delete from notification_threads where user_id = $1 and (not $2 or read_at is not null)`, scopedUserID(ctx), readOnly)
	return err
}

func (s *Store) GetNotificationSettings(ctx context.Context, publicKey string) (NotificationSettings, error) {
	settings := NotificationSettings{Categories: make(map[string]string), VAPIDPublicKey: publicKey}
	for _, category := range notificationCategories {
		settings.Categories[category] = defaultNotificationMode(category)
	}
	rows, err := s.db.Query(ctx, `select category, mode from notification_preferences where user_id = $1`, scopedUserID(ctx))
	if err != nil {
		return NotificationSettings{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var category, mode string
		if err := rows.Scan(&category, &mode); err != nil {
			return NotificationSettings{}, err
		}
		settings.Categories[category] = mode
	}
	return settings, rows.Err()
}

func (s *Store) UpdateNotificationSettings(ctx context.Context, categories map[string]string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for category, mode := range categories {
		if !validNotificationCategory(category) || !validNotificationMode(mode) {
			return ErrInvalidNotification
		}
		if _, err := tx.Exec(ctx, `
			insert into notification_preferences(user_id, category, mode) values($1, $2, $3)
			on conflict(user_id, category) do update set mode = excluded.mode, updated_at = now()
		`, scopedUserID(ctx), category, mode); err != nil {
			return err
		}
		if mode != notificationModeInAppPush {
			if _, err := tx.Exec(ctx, `
				delete from web_push_outbox outbox using notification_events event
				where outbox.event_id = event.id and event.category = $2
					and exists(select 1 from notification_threads thread where thread.id = outbox.thread_id and thread.user_id = $1)
			`, scopedUserID(ctx), category); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func endpointHash(endpoint string) []byte {
	sum := sha256.Sum256([]byte(endpoint))
	return sum[:]
}

func (s *Store) SaveWebPushSubscription(ctx context.Context, endpoint string, ciphertext []byte, deviceName, userAgent string) (WebPushSubscription, error) {
	deviceName = strings.TrimSpace(deviceName)
	if deviceName == "" || len(deviceName) > 100 || len(userAgent) > 500 {
		return WebPushSubscription{}, ErrInvalidNotification
	}
	userID := scopedUserID(ctx)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return WebPushSubscription{}, err
	}
	defer tx.Rollback(ctx)
	var item WebPushSubscription
	err = tx.QueryRow(ctx, `
		insert into web_push_subscriptions(user_id, endpoint_hash, subscription_ciphertext, device_name, user_agent)
		values($1, $2, $3, $4, $5)
		on conflict(endpoint_hash) do update set user_id = excluded.user_id, subscription_ciphertext = excluded.subscription_ciphertext,
			device_name = case when web_push_subscriptions.user_id = excluded.user_id then web_push_subscriptions.device_name else excluded.device_name end,
			user_agent = excluded.user_agent, last_seen_at = now(), last_error = '', updated_at = now()
		returning id::text, device_name, user_agent, last_seen_at, last_success_at, last_error, created_at
	`, userID, endpointHash(endpoint), ciphertext, deviceName, userAgent).Scan(&item.ID, &item.DeviceName, &item.UserAgent,
		&item.LastSeenAt, &item.LastSuccessAt, &item.LastError, &item.CreatedAt)
	if err != nil {
		return WebPushSubscription{}, err
	}
	if _, err := tx.Exec(ctx, `
		delete from web_push_outbox outbox using notification_threads thread
		where outbox.subscription_id = $1 and outbox.thread_id = thread.id and thread.user_id <> $2
	`, item.ID, userID); err != nil {
		return WebPushSubscription{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return WebPushSubscription{}, err
	}
	return item, nil
}

func (s *Store) ListWebPushSubscriptions(ctx context.Context) ([]WebPushSubscription, error) {
	rows, err := s.db.Query(ctx, `
		select id::text, device_name, user_agent, last_seen_at, last_success_at, last_error, created_at
		from web_push_subscriptions where user_id = $1 order by updated_at desc
	`, scopedUserID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WebPushSubscription, 0)
	for rows.Next() {
		var item WebPushSubscription
		if err := rows.Scan(&item.ID, &item.DeviceName, &item.UserAgent, &item.LastSeenAt, &item.LastSuccessAt, &item.LastError, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) RenameWebPushSubscription(ctx context.Context, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return ErrInvalidNotification
	}
	command, err := s.db.Exec(ctx, `update web_push_subscriptions set device_name = $3, updated_at = now() where id = $1 and user_id = $2`, id, scopedUserID(ctx), name)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteWebPushSubscription(ctx context.Context, id string) error {
	command, err := s.db.Exec(ctx, `delete from web_push_subscriptions where id = $1 and user_id = $2`, id, scopedUserID(ctx))
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteWebPushSubscriptionByEndpoint(ctx context.Context, endpoint string) error {
	_, err := s.db.Exec(ctx, `delete from web_push_subscriptions where endpoint_hash = $1 and user_id = $2`, endpointHash(endpoint), scopedUserID(ctx))
	return err
}

func (s *Store) RecordWebPushSubscriptionSuccess(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `update web_push_subscriptions set last_success_at = now(), last_error = '', updated_at = now() where id = $1 and user_id = $2`, id, scopedUserID(ctx))
	return err
}

func (s *Store) RecordWebPushSubscriptionError(ctx context.Context, id, message string) error {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := s.db.Exec(ctx, `update web_push_subscriptions set last_error = $3, updated_at = now() where id = $1 and user_id = $2`, id, scopedUserID(ctx), message)
	return err
}

func (s *Store) GetWebPushSubscriptionCiphertext(ctx context.Context, id string) ([]byte, error) {
	var ciphertext []byte
	err := s.db.QueryRow(ctx, `select subscription_ciphertext from web_push_subscriptions where id = $1 and user_id = $2`, id, scopedUserID(ctx)).Scan(&ciphertext)
	return ciphertext, err
}

func (s *Store) WebPushKeys(ctx context.Context) ([]byte, string, error) {
	var ciphertext []byte
	var publicKey string
	err := s.db.QueryRow(ctx, `select web_push_private_key_ciphertext, web_push_public_key from app_settings where id = $1`, appSettingsID).Scan(&ciphertext, &publicKey)
	return ciphertext, publicKey, err
}

func (s *Store) SaveWebPushKeys(ctx context.Context, ciphertext []byte, publicKey string) error {
	_, err := s.db.Exec(ctx, `update app_settings set web_push_private_key_ciphertext = $2, web_push_public_key = $3, updated_at = now() where id = $1`, appSettingsID, ciphertext, publicKey)
	return err
}

func (s *Store) ClaimWebPushOutbox(ctx context.Context, limit int) ([]webPushOutboxItem, error) {
	if limit <= 0 {
		limit = 20
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		select outbox.id::text, subscription.id::text, subscription.subscription_ciphertext, outbox.payload, outbox.attempts
		from web_push_outbox outbox
		join web_push_subscriptions subscription on subscription.id = outbox.subscription_id
		join notification_threads thread on thread.id = outbox.thread_id and thread.user_id = subscription.user_id
		where outbox.available_at <= now() and (outbox.locked_at is null or outbox.locked_at < now() - interval '5 minutes')
		order by outbox.available_at, outbox.created_at
		for update of outbox skip locked limit $1
	`, limit)
	if err != nil {
		return nil, err
	}
	items := make([]webPushOutboxItem, 0)
	for rows.Next() {
		var item webPushOutboxItem
		if err := rows.Scan(&item.ID, &item.SubscriptionID, &item.SubscriptionCiphertext, &item.Payload, &item.Attempts); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, item := range items {
		if _, err := tx.Exec(ctx, `update web_push_outbox set locked_at = now(), updated_at = now() where id = $1`, item.ID); err != nil {
			return nil, err
		}
	}
	return items, tx.Commit(ctx)
}

func (s *Store) CompleteWebPushOutbox(ctx context.Context, item webPushOutboxItem) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `delete from web_push_outbox where id = $1`, item.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `update web_push_subscriptions set last_success_at = now(), last_error = '', updated_at = now() where id = $1`, item.SubscriptionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) FailWebPushOutbox(ctx context.Context, item webPushOutboxItem, message string, retryAfter time.Duration) error {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		message = message[:500]
	}
	if retryAfter <= 0 || item.Attempts >= 3 {
		_, err := s.db.Exec(ctx, `
			with deleted as (delete from web_push_outbox where id = $1 returning subscription_id)
			update web_push_subscriptions set last_error = $2, updated_at = now() where id in (select subscription_id from deleted)
		`, item.ID, message)
		return err
	}
	_, err := s.db.Exec(ctx, `
		update web_push_outbox set attempts = attempts + 1, available_at = now() + $2::interval,
			locked_at = null, last_error = $3, updated_at = now() where id = $1
	`, item.ID, fmt.Sprintf("%f seconds", retryAfter.Seconds()), message)
	return err
}

func (s *Store) ExpireWebPushSubscription(ctx context.Context, subscriptionID string) error {
	_, err := s.db.Exec(ctx, `delete from web_push_subscriptions where id = $1`, subscriptionID)
	return err
}

func (s *Store) PruneNotifications(ctx context.Context, before time.Time) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `delete from notification_threads where last_event_at < $1`, before); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		delete from notification_events event where event.created_at < $1
			and exists(select 1 from notification_threads thread where thread.id = event.thread_id and thread.last_event_at >= $1)
	`, before); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
