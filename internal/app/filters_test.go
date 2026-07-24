package app

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestActivityFiltersFromQuerySearch(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/activities?search=morning+run", nil)

	filters := activityFiltersFromQuery(request)

	if filters.Search != "morning run" {
		t.Fatalf("search = %q", filters.Search)
	}
}

func TestActivityFilterConditionsSearch(t *testing.T) {
	conditions, args := activityFilterConditions(ActivityFilters{Search: " morning "}, 2)

	wantConditions := []string{"source <> 'training_sheet'", "coalesce(nullif(local_name, ''), name) ilike $2"}
	wantArgs := []any{"%morning%"}
	if !reflect.DeepEqual(conditions, wantConditions) {
		t.Fatalf("conditions = %#v, want %#v", conditions, wantConditions)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestActivityFiltersFromQueryDates(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/activities?dateFrom=2026-01-01&dateTo=2026-12-31", nil)

	filters := activityFiltersFromQuery(request)

	wantFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	if !filters.DateFrom.Equal(wantFrom) {
		t.Fatalf("date from = %v, want %v", filters.DateFrom, wantFrom)
	}
	if !filters.DateTo.Equal(wantTo) {
		t.Fatalf("date to = %v, want %v", filters.DateTo, wantTo)
	}
}

func TestActivityFiltersFromQuerySort(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/activities?sortBy=duration&sortOrder=asc", nil)

	filters := activityFiltersFromQuery(request)

	if filters.SortBy != "duration" {
		t.Fatalf("sort by = %q", filters.SortBy)
	}
	if filters.SortOrder != "asc" {
		t.Fatalf("sort order = %q", filters.SortOrder)
	}
}

func TestNormalizeActivityPage(t *testing.T) {
	limit, offset := normalizeActivityPage(0, -10)
	if limit != 50 || offset != 0 {
		t.Fatalf("default page = %d/%d, want 50/0", limit, offset)
	}

	limit, offset = normalizeActivityPage(101, 20)
	if limit != 50 || offset != 20 {
		t.Fatalf("capped page = %d/%d, want 50/20", limit, offset)
	}

	limit, offset = normalizeActivityPage(100, 30)
	if limit != 100 || offset != 30 {
		t.Fatalf("valid page = %d/%d, want 100/30", limit, offset)
	}
}

func TestActivityFilterConditionsDateRange(t *testing.T) {
	dateFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dateTo := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	conditions, args := activityFilterConditions(ActivityFilters{DateFrom: dateFrom, DateTo: dateTo}, 1)

	wantConditions := []string{"source <> 'training_sheet'", "start_time >= $1", "start_time < $2"}
	wantArgs := []any{dateFrom, dateTo.AddDate(0, 0, 1)}
	if !reflect.DeepEqual(conditions, wantConditions) {
		t.Fatalf("conditions = %#v, want %#v", conditions, wantConditions)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestCalendarDateRangeUsesRequestedTimezone(t *testing.T) {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	gotFrom, gotTo, err := calendarDateRangeInTimezone(from, to, "Europe/Dublin")
	if err != nil {
		t.Fatal(err)
	}
	wantFrom := time.Date(2026, 7, 1, 0, 0, 0, 0, time.FixedZone("IST", 3600))
	wantTo := time.Date(2026, 7, 1, 0, 0, 0, 0, time.FixedZone("IST", 3600))
	if !gotFrom.Equal(wantFrom) || gotFrom.Location().String() != "Europe/Dublin" {
		t.Fatalf("from = %v (%s), want %v (Europe/Dublin)", gotFrom, gotFrom.Location(), wantFrom)
	}
	if !gotTo.Equal(wantTo) || gotTo.Location().String() != "Europe/Dublin" {
		t.Fatalf("to = %v (%s), want %v (Europe/Dublin)", gotTo, gotTo.Location(), wantTo)
	}
}

func TestCalendarTimezoneFromQueryValidatesIANAZone(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/stats/calendar/day?date=2026-07-01&timezone=Europe%2FDublin", nil)
	if got, err := calendarTimezoneFromQuery(request); err != nil || got != "Europe/Dublin" {
		t.Fatalf("timezone = %q, error = %v", got, err)
	}

	request = httptest.NewRequest("GET", "/api/stats/calendar/day?date=2026-07-01&timezone=not-a-zone", nil)
	if _, err := calendarTimezoneFromQuery(request); err == nil {
		t.Fatal("expected invalid timezone error")
	}
}

func TestCalendarTimezoneFilterUsesRequestedZoneForDates(t *testing.T) {
	conditions, args := activityFilterConditionsForUser(ActivityFilters{
		CalendarTimezone:     "Europe/Dublin",
		IncludeTrainingSheet: true,
	}, 1, "user-1")
	if len(args) != 2 || args[0] != "user-1" || args[1] != "Europe/Dublin" {
		t.Fatalf("args = %#v, want user and timezone", args)
	}
	want := "(source <> 'training_sheet' or (case when source = 'training_sheet' then date(start_time) else date(start_time at time zone $2) end >= date(now() at time zone $2) and not exists ("
	if len(conditions) < 2 || !strings.HasPrefix(conditions[1], want) {
		t.Fatalf("planned conditions = %#v, want prefix %q", conditions, want)
	}
}

func TestCalendarTimezoneFilterUsesCalendarDatesForActivityBounds(t *testing.T) {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.FixedZone("PDT", -7*60*60))
	to := time.Date(2026, 7, 2, 0, 0, 0, 0, time.FixedZone("PDT", -7*60*60))
	conditions, args := activityFilterConditionsForUser(ActivityFilters{
		CalendarTimezone:     "America/Los_Angeles",
		DateFrom:             from,
		DateTo:               to,
		IncludeTrainingSheet: true,
	}, 1, "user-1")
	dateExpression := "case when source = 'training_sheet' then date(start_time) else date(start_time at time zone $2) end"
	if !strings.Contains(conditions[2], dateExpression+" >= $3::date") {
		t.Fatalf("date-from condition = %q, want calendar date expression", conditions[2])
	}
	if !strings.Contains(conditions[3], dateExpression+" <= $4::date") {
		t.Fatalf("date-to condition = %q, want calendar date expression", conditions[3])
	}
	wantArgs := []any{"user-1", "America/Los_Angeles", "2026-07-01", "2026-07-02"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestActivityOrderBy(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		want      string
	}{
		{
			name: "default",
			want: "order by start_time desc, id desc",
		},
		{
			name:      "distance asc",
			sortBy:    "distance",
			sortOrder: "asc",
			want:      "order by distance_m asc, start_time desc, id desc",
		},
		{
			name:      "average pace desc",
			sortBy:    "avg_pace",
			sortOrder: "desc",
			want:      "order by coalesce(avg_pace_s_per_km, 0) desc, start_time desc, id desc",
		},
		{
			name:      "calories desc",
			sortBy:    "calories",
			sortOrder: "desc",
			want:      "order by coalesce(calories_kcal, 0) desc, start_time desc, id desc",
		},
		{
			name:      "invalid values",
			sortBy:    "drop table",
			sortOrder: "sideways",
			want:      "order by start_time desc, id desc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := activityOrderBy(tt.sortBy, tt.sortOrder); got != tt.want {
				t.Fatalf("order by = %q, want %q", got, tt.want)
			}
		})
	}
}
