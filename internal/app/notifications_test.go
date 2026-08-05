package app

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultNotificationModes(t *testing.T) {
	for _, category := range notificationCategories {
		want := notificationModeInAppPush
		if category == notificationCategoryActivityMatching {
			want = notificationModeInApp
		}
		if got := defaultNotificationMode(category); got != want {
			t.Fatalf("defaultNotificationMode(%q) = %q, want %q", category, got, want)
		}
	}
}

func TestNormalizeNotificationInput(t *testing.T) {
	input, err := normalizeNotificationInput(NotificationInput{
		ThreadKey: " workout:1 ", EventKey: " generated:1 ", Category: notificationCategoryWorkoutChanges,
		Kind: " workout_generated ", Severity: " success ", Title: " Workout generated ", Body: " Ready ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.ThreadKey != "workout:1" || input.EventKey != "generated:1" || input.Title != "Workout generated" {
		t.Fatalf("input was not normalized: %#v", input)
	}
	if input.ActionPath != "/notifications" {
		t.Fatalf("default action path = %q", input.ActionPath)
	}
	longInput := input
	longInput.Title = strings.Repeat("🏃", 200)
	longInput.Body = strings.Repeat("é", 1200)
	longInput, err = normalizeNotificationInput(longInput)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(longInput.Title)) != 180 || len([]rune(longInput.Body)) != 1000 || !strings.HasSuffix(longInput.Title, "…") {
		t.Fatalf("long notification text was not safely bounded")
	}
}

func TestNormalizeNotificationInputRejectsUnsafeValues(t *testing.T) {
	valid := NotificationInput{
		ThreadKey: "workout:1", EventKey: "generated:1", Category: notificationCategoryWorkoutChanges,
		Kind: "workout_generated", Severity: "success", Title: "Workout generated", ActionPath: "/workouts/1",
	}
	tests := []struct {
		name   string
		mutate func(*NotificationInput)
	}{
		{name: "category", mutate: func(input *NotificationInput) { input.Category = "unknown" }},
		{name: "severity", mutate: func(input *NotificationInput) { input.Severity = "critical" }},
		{name: "external URL", mutate: func(input *NotificationInput) { input.ActionPath = "https://example.com" }},
		{name: "protocol relative URL", mutate: func(input *NotificationInput) { input.ActionPath = "//example.com" }},
		{name: "missing title", mutate: func(input *NotificationInput) { input.Title = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			if _, err := normalizeNotificationInput(input); !errors.Is(err, ErrInvalidNotification) {
				t.Fatalf("error = %v, want ErrInvalidNotification", err)
			}
		})
	}
}

func TestNotificationCursorRoundTrip(t *testing.T) {
	want := notificationCursor{Time: time.Date(2026, time.July, 30, 12, 30, 0, 0, time.UTC), ID: "64a73021-05e8-4f6a-986e-b348a70d3d0f"}
	got, err := decodeNotificationCursor(encodeNotificationCursor(want))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Time.Equal(want.Time) || got.ID != want.ID {
		t.Fatalf("decoded cursor = %#v, want %#v", got, want)
	}
	if _, err := decodeNotificationCursor("not-a-cursor"); !errors.Is(err, ErrInvalidNotification) {
		t.Fatalf("invalid cursor error = %v", err)
	}
	if _, err := decodeNotificationCursor(encodeNotificationCursor(notificationCursor{Time: want.Time, ID: "not-a-uuid"})); !errors.Is(err, ErrInvalidNotification) {
		t.Fatalf("invalid cursor ID error = %v", err)
	}
}

func TestSafeNotificationError(t *testing.T) {
	if got := safeNotificationError("  "); got != "Open Runnarr for details." {
		t.Fatalf("empty error = %q", got)
	}
	got := safeNotificationError(strings.Repeat("x", 300))
	if len([]rune(got)) != 240 || !strings.HasSuffix(got, "…") {
		t.Fatalf("bounded error has %d runes and suffix %q", len([]rune(got)), got[len(got)-3:])
	}
}

func TestActivityAutoMatchNotificationPromptsForReflection(t *testing.T) {
	planned := PlannedActivity{ID: "plan-1", Name: "Tempo run", FeedbackCell: "C19"}
	input := activityAutoMatchNotificationInput("activity-1", planned)
	if input.Severity != "info" || input.Kind != "activity_auto_matched" {
		t.Fatalf("notification metadata = %#v", input)
	}
	if input.ActionPath != "/activities/activity-1#check-in" {
		t.Fatalf("action path = %q, want direct reflection prompt", input.ActionPath)
	}
	if !strings.Contains(input.Body, "RPE and feedback") {
		t.Fatalf("body = %q, want RPE and feedback reminder", input.Body)
	}

	input = activityAutoMatchNotificationInput("activity-1", PlannedActivity{ID: "plan-1", Name: "Easy run"})
	if strings.Contains(input.Body, "feedback") || !strings.Contains(input.Body, "RPE") {
		t.Fatalf("body without feedback cell = %q, want RPE-only reminder", input.Body)
	}
}

func TestValidatePushEndpoint(t *testing.T) {
	valid := []string{
		"https://fcm.googleapis.com/fcm/send/example",
		"https://updates.push.services.mozilla.com/wpush/v2/example",
		"https://push.example.com:443/subscription",
	}
	for _, endpoint := range valid {
		if err := validatePushEndpoint(endpoint); err != nil {
			t.Errorf("validatePushEndpoint(%q) = %v", endpoint, err)
		}
	}
	invalid := []string{
		"http://push.example.com/subscription",
		"https://localhost/subscription",
		"https://127.0.0.1/subscription",
		"https://10.1.2.3/subscription",
		"https://192.0.2.1/subscription",
		"https://push.example.com:8443/subscription",
		"https://user:password@push.example.com/subscription",
		"//push.example.com/subscription",
	}
	for _, endpoint := range invalid {
		if err := validatePushEndpoint(endpoint); !errors.Is(err, ErrInvalidNotification) {
			t.Errorf("validatePushEndpoint(%q) = %v, want ErrInvalidNotification", endpoint, err)
		}
	}
}

func TestPublicPushIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{ip: "8.8.8.8", want: true},
		{ip: "2606:4700:4700::1111", want: true},
		{ip: "127.0.0.1", want: false},
		{ip: "100.64.0.1", want: false},
		{ip: "198.51.100.12", want: false},
		{ip: "2001:db8::1", want: false},
		{ip: "::ffff:10.0.0.1", want: false},
	}
	for _, test := range tests {
		if got := publicPushIP(net.ParseIP(test.ip)); got != test.want {
			t.Errorf("publicPushIP(%q) = %t, want %t", test.ip, got, test.want)
		}
	}
}

func TestSupportModeCannotManagePushDevices(t *testing.T) {
	server := &Server{}
	handlers := []struct {
		name    string
		method  string
		handler http.HandlerFunc
	}{
		{name: "list", method: http.MethodGet, handler: server.handleListWebPushSubscriptions},
		{name: "create", method: http.MethodPost, handler: server.handleCreateWebPushSubscription},
		{name: "rename", method: http.MethodPatch, handler: server.handleUpdateWebPushSubscription},
		{name: "delete", method: http.MethodDelete, handler: server.handleDeleteWebPushSubscription},
		{name: "delete current", method: http.MethodDelete, handler: server.handleDeleteCurrentWebPushSubscription},
		{name: "test", method: http.MethodPost, handler: server.handleTestWebPushSubscription},
	}
	for _, test := range handlers {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/api/push-subscriptions/test", nil)
			request = request.WithContext(withPrincipal(request.Context(), UserPrincipal{ID: "target-user", ActorID: "admin-user", SupportTarget: true}))
			response := httptest.NewRecorder()
			test.handler(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
		})
	}
}
