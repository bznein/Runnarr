package app

import (
	"net/http/httptest"
	"reflect"
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

	wantConditions := []string{"coalesce(nullif(local_name, ''), name) ilike $2"}
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

func TestActivityFilterConditionsDateRange(t *testing.T) {
	dateFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dateTo := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	conditions, args := activityFilterConditions(ActivityFilters{DateFrom: dateFrom, DateTo: dateTo}, 1)

	wantConditions := []string{"start_time >= $1", "start_time < $2"}
	wantArgs := []any{dateFrom, dateTo.AddDate(0, 0, 1)}
	if !reflect.DeepEqual(conditions, wantConditions) {
		t.Fatalf("conditions = %#v, want %#v", conditions, wantConditions)
	}
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
