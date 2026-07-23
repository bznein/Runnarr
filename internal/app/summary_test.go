package app

import "testing"

func TestNormalizeSummaryPeriod(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "weekly default", input: "", want: "weekly"},
		{name: "weekly alias", input: "week", want: "weekly"},
		{name: "monthly", input: "month", want: "monthly"},
		{name: "monthly canonical", input: "monthly", want: "monthly"},
		{name: "yearly", input: "year", want: "yearly"},
		{name: "yearly canonical", input: "YEARLY", want: "yearly"},
		{name: "unknown", input: "quarterly", want: "weekly"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeSummaryPeriod(test.input); got != test.want {
				t.Fatalf("normalizeSummaryPeriod(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestSummaryBucketSQLUsesOnlySupportedPeriods(t *testing.T) {
	if got, window := summaryBucketSQL("monthly"); got != "month" || window != "interval '12 months'" {
		t.Fatalf("monthly bucket = (%q, %q)", got, window)
	}
	if got, window := summaryBucketSQL("yearly"); got != "year" || window != "interval '5 years'" {
		t.Fatalf("yearly bucket = (%q, %q)", got, window)
	}
	if got, window := summaryBucketSQL("unexpected"); got != "week" || window != "interval '12 weeks'" {
		t.Fatalf("default bucket = (%q, %q)", got, window)
	}
}
