package app

import (
	"encoding/json"
	"testing"
)

func TestNormalizeThemePreference(t *testing.T) {
	tests := map[string]string{
		"system":    "system",
		" runnarr ": "runnarr",
		"ocean":     "ocean",
		"sunset":    "sunset",
		"midnight":  "midnight",
		"light":     "runnarr",
		"dark":      "midnight",
		"unknown":   "system",
		"":          "system",
	}
	for input, want := range tests {
		if got := normalizeThemePreference(input); got != want {
			t.Errorf("normalizeThemePreference(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeDefaultExperience(t *testing.T) {
	for input, want := range map[string]string{
		"simple":   "simple",
		" SIMPLE ": "simple",
		"full":     "full",
		"":         "full",
		"unknown":  "full",
	} {
		if got := normalizeDefaultExperience(input); got != want {
			t.Errorf("normalizeDefaultExperience(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestApplyUserPreferenceUpdateCourseStartLocation(t *testing.T) {
	preferences := UserPreference{ThemePreference: "system", GearSortBy: "distance_percent", DefaultExperience: "full"}
	update := userPreferenceUpdate{CourseStartLocation: json.RawMessage(`{"latitude":53.3498,"longitude":-6.2603}`)}
	if err := applyUserPreferenceUpdate(&preferences, update); err != nil {
		t.Fatal(err)
	}
	if preferences.CourseStartLocation == nil || preferences.CourseStartLocation.Latitude != 53.3498 || preferences.CourseStartLocation.Longitude != -6.2603 {
		t.Fatalf("course start location = %#v", preferences.CourseStartLocation)
	}

	if err := applyUserPreferenceUpdate(&preferences, userPreferenceUpdate{CourseStartLocation: json.RawMessage(`null`)}); err != nil {
		t.Fatal(err)
	}
	if preferences.CourseStartLocation != nil {
		t.Fatalf("course start location was not cleared: %#v", preferences.CourseStartLocation)
	}
}

func TestApplyUserPreferenceUpdatePreservesCourseStartLocationWhenOmitted(t *testing.T) {
	location := &CourseStartLocation{Latitude: 53.3498, Longitude: -6.2603}
	preferences := UserPreference{CourseStartLocation: location}
	theme := "ocean"
	if err := applyUserPreferenceUpdate(&preferences, userPreferenceUpdate{ThemePreference: &theme}); err != nil {
		t.Fatal(err)
	}
	if preferences.CourseStartLocation != location {
		t.Fatalf("course start location changed: %#v", preferences.CourseStartLocation)
	}
}

func TestApplyUserPreferenceUpdateRejectsInvalidCourseStartLocation(t *testing.T) {
	for _, payload := range []string{
		`{"latitude":91,"longitude":-6.2603}`,
		`{"latitude":53.3498,"longitude":-181}`,
		`{"latitude":53.3498}`,
		`"Dublin"`,
	} {
		preferences := UserPreference{}
		err := applyUserPreferenceUpdate(&preferences, userPreferenceUpdate{CourseStartLocation: json.RawMessage(payload)})
		if err == nil {
			t.Fatalf("payload %s was accepted", payload)
		}
		if preferences.CourseStartLocation != nil {
			t.Fatalf("payload %s changed preferences: %#v", payload, preferences.CourseStartLocation)
		}
	}
}
