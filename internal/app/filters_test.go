package app

import (
	"net/http/httptest"
	"reflect"
	"testing"
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
