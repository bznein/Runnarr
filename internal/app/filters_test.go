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

	wantConditions := []string{"name ilike $2"}
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
