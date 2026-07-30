package app

import "testing"

func TestParsePlannedMatchWindowDays(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{raw: "", want: 7},
		{raw: "7", want: 7},
		{raw: "30", want: 30},
		{raw: "90", want: 90},
		{raw: "180", want: 180},
	}
	for _, test := range tests {
		got, err := parsePlannedMatchWindowDays(test.raw)
		if err != nil {
			t.Fatalf("parsePlannedMatchWindowDays(%q): %v", test.raw, err)
		}
		if got != test.want {
			t.Fatalf("parsePlannedMatchWindowDays(%q) = %d, want %d", test.raw, got, test.want)
		}
	}
}

func TestParsePlannedMatchWindowDaysRejectsUnboundedWindows(t *testing.T) {
	for _, raw := range []string{"0", "31", "365", "all"} {
		if _, err := parsePlannedMatchWindowDays(raw); err == nil {
			t.Fatalf("parsePlannedMatchWindowDays(%q) succeeded, want bounded-window error", raw)
		}
	}
}
