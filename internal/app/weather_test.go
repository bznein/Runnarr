package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type openMeteoRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn openMeteoRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestOpenMeteoFetchUsesRoundedCoordinatesAndNearestHour(t *testing.T) {
	start := time.Date(2026, 8, 24, 6, 24, 0, 0, time.UTC)
	service := &OpenMeteoWeatherService{
		forecastURL: "https://weather.test/v1/forecast",
		archiveURL:  "https://archive.test/v1/archive",
		minInterval: 0,
		now:         func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) },
		reserveLimit: func(context.Context, time.Time) (time.Duration, error) {
			return 0, nil
		},
	}
	service.client = &http.Client{Transport: openMeteoRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "weather.test" {
			t.Fatalf("host = %q, want recent forecast endpoint", request.URL.Host)
		}
		if request.URL.Query().Get("latitude") != "53.123" || request.URL.Query().Get("longitude") != "-7.988" {
			t.Fatalf("coordinates were not rounded: %s", request.URL.RawQuery)
		}
		if request.URL.Query().Get("start_date") != "2026-08-24" || request.URL.Query().Get("timezone") != "UTC" {
			t.Fatalf("unexpected time query: %s", request.URL.RawQuery)
		}
		body := `{
			"latitude":53.125,"longitude":-8.0,"timezone":"GMT",
			"hourly":{
				"time":["2026-08-24T05:00","2026-08-24T06:00","2026-08-24T07:00"],
				"temperature_2m":[12.1,13.4,14.2],"apparent_temperature":[11.0,12.8,13.6],
				"dew_point_2m":[8.0,8.4,9.0],"relative_humidity_2m":[76,73,70],
				"weather_code":[3,2,1],"wind_speed_10m":[8,9.5,11],
				"wind_gusts_10m":[14,16,18],"wind_direction_10m":[180,225,270]
			}
		}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}

	weather, err := service.Fetch(context.Background(), start, 53.12349, -7.98765)
	if err != nil {
		t.Fatal(err)
	}
	if weather.Provider != openMeteoProvider || weather.Condition != "Partly cloudy" || weather.WindDirection != "SW" {
		t.Fatalf("weather labels = %#v", weather)
	}
	if weather.TemperatureC == nil || math.Abs(*weather.TemperatureC-13.4) > 0.0001 {
		t.Fatalf("temperature = %#v", weather.TemperatureC)
	}
	if weather.ObservedAt == nil || !weather.ObservedAt.Equal(time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("observed at = %#v", weather.ObservedAt)
	}
	if weather.Raw["hourly"] == nil {
		t.Fatal("raw provider payload was not retained")
	}
	encoded, err := json.Marshal(Activity{Weather: weather})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "53.125") || strings.Contains(string(encoded), "longitude") {
		t.Fatalf("public weather leaked coordinates: %s", encoded)
	}
}

func TestOpenMeteoFetchUsesArchiveForOlderActivities(t *testing.T) {
	requested := make(chan *url.URL, 1)
	service := &OpenMeteoWeatherService{
		client: &http.Client{Transport: openMeteoRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requested <- request.URL
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
		})},
		forecastURL: "https://forecast.test/v1/forecast",
		archiveURL:  "https://archive.test/v1/archive",
		minInterval: 0,
		now:         func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) },
		reserveLimit: func(context.Context, time.Time) (time.Duration, error) {
			return 0, nil
		},
	}

	_, _ = service.Fetch(context.Background(), time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC), 53, -7)
	if got := <-requested; got.Host != "archive.test" {
		t.Fatalf("host = %q, want archive endpoint", got.Host)
	}
}

func TestOpenMeteoFetchStopsBeforeHTTPWhenQuotaIsExhausted(t *testing.T) {
	called := false
	service := &OpenMeteoWeatherService{
		client: &http.Client{Transport: openMeteoRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("unexpected request")
		})},
		forecastURL: "https://forecast.test/v1/forecast",
		archiveURL:  "https://archive.test/v1/archive",
		minInterval: 0,
		now:         func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) },
		reserveLimit: func(context.Context, time.Time) (time.Duration, error) {
			return time.Hour, ErrOpenMeteoRateLimited
		},
	}

	_, err := service.Fetch(context.Background(), time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC), 53, -7)
	if !errors.Is(err, ErrOpenMeteoRateLimited) || called {
		t.Fatalf("error = %v, HTTP called = %v", err, called)
	}
}

func TestOpenMeteoRateLimitWindowsKeepSafetyMargin(t *testing.T) {
	windows := openMeteoRateLimitWindows(time.Date(2026, 8, 24, 6, 24, 31, 0, time.FixedZone("test", 3600)))
	want := map[string]int{"minute": 500, "hour": 4500, "day": 9000, "month": 270000}
	for _, window := range windows {
		if window.Limit != want[window.Kind] {
			t.Fatalf("%s limit = %d, want %d", window.Kind, window.Limit, want[window.Kind])
		}
		if !window.Next.After(window.Start) {
			t.Fatalf("invalid %s window: %#v", window.Kind, window)
		}
	}
}

func TestOpenMeteoWeatherCodeAndCompassLabels(t *testing.T) {
	if got := openMeteoWeatherCodeLabel(95); got != "Thunderstorm" {
		t.Fatalf("weather label = %q", got)
	}
	if got := weatherCompassDirection(359); got != "N" {
		t.Fatalf("compass = %q", got)
	}
}
