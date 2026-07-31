package app

import "testing"

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
