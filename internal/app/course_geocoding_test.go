package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestCourseGeocodingSearchesNominatimAndFiltersInvalidResults(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: courseRoutingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Method != http.MethodGet || request.URL.Path != "/search" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("q") != "Phoenix Park" || query.Get("format") != "jsonv2" || query.Get("limit") != "5" || query.Get("addressdetails") != "0" {
			t.Fatalf("query = %#v", query)
		}
		if request.Header.Get("User-Agent") != "Runnarr course planner (https://github.com/bznein/Runnarr)" {
			t.Fatalf("user agent = %q", request.Header.Get("User-Agent"))
		}
		body := `[
			{"name":"Phoenix Park","display_name":"Phoenix Park, Dublin, Ireland","lat":"53.356","lon":"-6.329"},
			{"display_name":"Missing coordinates","lat":"not-a-number","lon":"-6.3"},
			{"name":"","display_name":"Galway Cathedral, Galway, Ireland","lat":"53.275","lon":"-9.057"}
		]`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}, nil
	})}
	service := &CourseGeocodingService{enabled: true, baseURL: "http://geocoder.test", client: client}

	results, err := service.Search(context.Background(), "  Phoenix Park  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Name != "Phoenix Park" || results[0].Latitude != 53.356 || results[1].Name != "Galway Cathedral" {
		t.Fatalf("results = %#v", results)
	}
	cached, err := service.Search(context.Background(), "phoenix park")
	if err != nil || len(cached) != 2 || calls != 1 {
		t.Fatalf("cached results = %#v, calls = %d, error = %v", cached, calls, err)
	}
}

func TestCourseGeocodingRejectsDisabledShortAndRapidSearches(t *testing.T) {
	if _, err := (&CourseGeocodingService{}).Search(context.Background(), "Dublin"); err != errCourseGeocodingDisabled {
		t.Fatalf("disabled error = %v", err)
	}
	service := &CourseGeocodingService{enabled: true, baseURL: "http://geocoder.test", client: &http.Client{}, minInterval: time.Second}
	if _, err := service.Search(context.Background(), "D"); !errors.Is(err, errCourseGeocodingInput) {
		t.Fatalf("short query error = %v", err)
	}
	service.nextRequest = time.Now().Add(time.Second)
	if _, err := service.Search(context.Background(), "Dublin"); err != errCourseGeocodingRate {
		t.Fatalf("rate error = %v", err)
	}
}
