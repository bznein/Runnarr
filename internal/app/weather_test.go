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

func TestOpenMeteoFetchUsesMidpoint15MinuteDataAndMedianTemperatureModel(t *testing.T) {
	start := time.Date(2026, 8, 24, 6, 24, 0, 0, time.UTC)
	reservedUnits := 0
	service := &OpenMeteoWeatherService{
		forecastURL: "https://weather.test/v1/forecast",
		archiveURL:  "https://archive.test/v1/archive",
		minInterval: 0,
		now:         func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) },
		reserveLimit: func(_ context.Context, _ time.Time, units int) (time.Duration, error) {
			reservedUnits = units
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
		if request.URL.Query().Get("models") != "ukmo_seamless,icon_seamless,ecmwf_ifs025" || request.URL.Query().Get("minutely_15") == "" || request.URL.Query().Has("hourly") {
			t.Fatalf("unexpected consensus query: %s", request.URL.RawQuery)
		}
		body := `{
			"latitude":53.125,"longitude":-8.0,"timezone":"GMT",
			"minutely_15":{
				"time":["2026-08-24T06:15","2026-08-24T06:30","2026-08-24T06:45"],
				"temperature_2m_ukmo_seamless":[5.0,3.2,7.0],
				"temperature_2m_icon_seamless":[6.0,8.4,9.0],
				"temperature_2m_ecmwf_ifs025":[6.0,6.5,8.0],
				"apparent_temperature_ecmwf_ifs025":[5.0,5.8,7.1],
				"dew_point_2m_ecmwf_ifs025":[4.0,4.5,5.0],
				"relative_humidity_2m_ecmwf_ifs025":[90,87,84],
				"weather_code_ecmwf_ifs025":[3,2,1],
				"wind_speed_10m_ecmwf_ifs025":[8,9.5,11],
				"wind_gusts_10m_ecmwf_ifs025":[14,16,18],
				"wind_direction_10m_ecmwf_ifs025":[180,225,270]
			}
		}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}

	weather, err := service.Fetch(context.Background(), start, 53.12349, -7.98765)
	if err != nil {
		t.Fatal(err)
	}
	if weather.Provider != openMeteoProvider || weather.SelectionMethod != openMeteoMethodMultiModel15Min || weather.Model != "ECMWF IFS" || weather.Condition != "Partly cloudy" || weather.WindDirection != "SW" {
		t.Fatalf("weather labels = %#v", weather)
	}
	if weather.TemperatureC == nil || math.Abs(*weather.TemperatureC-6.5) > 0.0001 {
		t.Fatalf("temperature = %#v", weather.TemperatureC)
	}
	if weather.ObservedAt == nil || !weather.ObservedAt.Equal(time.Date(2026, 8, 24, 6, 30, 0, 0, time.UTC)) {
		t.Fatalf("observed at = %#v", weather.ObservedAt)
	}
	if reservedUnits != openMeteoRecentRequestUnits {
		t.Fatalf("reserved units = %d, want %d", reservedUnits, openMeteoRecentRequestUnits)
	}
	if weather.Raw["minutely_15"] == nil {
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
	reservedUnits := 0
	service := &OpenMeteoWeatherService{
		client: &http.Client{Transport: openMeteoRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requested <- request.URL
			body := `{
				"latitude":53.0,"longitude":-7.0,"timezone":"GMT",
				"hourly":{
					"time":["2025-01-01T08:00","2025-01-01T09:00","2025-01-01T10:00"],
					"temperature_2m":[4.1,4.8,5.2],"weather_code":[3,2,1]
				}
			}`
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})},
		forecastURL: "https://forecast.test/v1/forecast",
		archiveURL:  "https://archive.test/v1/archive",
		minInterval: 0,
		now:         func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) },
		reserveLimit: func(_ context.Context, _ time.Time, units int) (time.Duration, error) {
			reservedUnits = units
			return 0, nil
		},
	}

	weather, err := service.Fetch(context.Background(), time.Date(2025, 1, 1, 9, 10, 0, 0, time.UTC), 53, -7)
	if err != nil {
		t.Fatal(err)
	}
	if got := <-requested; got.Host != "archive.test" || got.Query().Get("hourly") == "" || got.Query().Has("models") || got.Query().Has("minutely_15") {
		t.Fatalf("host = %q, want archive endpoint", got.Host)
	}
	if reservedUnits != 1 {
		t.Fatalf("reserved units = %d, want 1", reservedUnits)
	}
	if weather.SelectionMethod != openMeteoMethodArchiveHourly || weather.Model != "" || weather.TemperatureC == nil || *weather.TemperatureC != 4.8 || weather.ObservedAt == nil || !weather.ObservedAt.Equal(time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("archive weather = %#v", weather)
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
		reserveLimit: func(context.Context, time.Time, int) (time.Duration, error) {
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
