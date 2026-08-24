package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	openMeteoProvider            = "open-meteo"
	openMeteoForecastURL         = "https://api.open-meteo.com/v1/forecast"
	openMeteoArchiveURL          = "https://archive-api.open-meteo.com/v1/archive"
	openMeteoMaxResponseBytes    = 2 << 20
	openMeteoRecentHistoryDays   = 92
	openMeteoRequestInterval     = 850 * time.Millisecond
	openMeteoMinuteRequestLimit  = 500
	openMeteoHourlyRequestLimit  = 4500
	openMeteoDailyRequestLimit   = 9000
	openMeteoMonthlyRequestLimit = 270000
	openMeteoCoordinatePrecision = 1000
)

var (
	ErrOpenMeteoRateLimited = errors.New("Open-Meteo fallback rate limit reached")
	errOpenMeteoNoWeather   = errors.New("Open-Meteo returned no weather near the activity start")
)

type WeatherConfig struct {
	OpenMeteoFallbackEnabled bool `json:"openMeteoFallbackEnabled"`
}

type OpenMeteoWeatherService struct {
	client       *http.Client
	forecastURL  string
	archiveURL   string
	minInterval  time.Duration
	now          func() time.Time
	requestMu    sync.Mutex
	nextRequest  time.Time
	reserveLimit func(context.Context, time.Time) (time.Duration, error)
}

type openMeteoResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
	Hourly    struct {
		Time                []string   `json:"time"`
		Temperature         []*float64 `json:"temperature_2m"`
		ApparentTemperature []*float64 `json:"apparent_temperature"`
		DewPoint            []*float64 `json:"dew_point_2m"`
		RelativeHumidity    []*float64 `json:"relative_humidity_2m"`
		WeatherCode         []*int     `json:"weather_code"`
		WindSpeed           []*float64 `json:"wind_speed_10m"`
		WindGusts           []*float64 `json:"wind_gusts_10m"`
		WindDirection       []*float64 `json:"wind_direction_10m"`
	} `json:"hourly"`
}

type externalAPIRateLimitWindow struct {
	Kind  string
	Start time.Time
	Next  time.Time
	Limit int
}

func NewOpenMeteoWeatherService(store *Store) *OpenMeteoWeatherService {
	service := &OpenMeteoWeatherService{
		client:      &http.Client{Timeout: 10 * time.Second},
		forecastURL: openMeteoForecastURL,
		archiveURL:  openMeteoArchiveURL,
		minInterval: openMeteoRequestInterval,
		now:         time.Now,
	}
	service.reserveLimit = store.ReserveOpenMeteoRequest
	return service
}

func (s *Store) GetWeatherConfig(ctx context.Context) (WeatherConfig, error) {
	var config WeatherConfig
	err := s.db.QueryRow(ctx, `
		select open_meteo_weather_enabled
		from user_settings where user_id = $1
	`, scopedUserID(ctx)).Scan(&config.OpenMeteoFallbackEnabled)
	return config, err
}

func (s *Store) SetWeatherConfig(ctx context.Context, config WeatherConfig) error {
	_, err := s.db.Exec(ctx, `
		update user_settings
		set open_meteo_weather_enabled = $2, updated_at = now()
		where user_id = $1
	`, scopedUserID(ctx), config.OpenMeteoFallbackEnabled)
	return err
}

func (s *Store) ReserveOpenMeteoRequest(ctx context.Context, now time.Time) (time.Duration, error) {
	transaction, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	for _, window := range openMeteoRateLimitWindows(now) {
		var count int
		err = transaction.QueryRow(ctx, `
			insert into external_api_rate_limit_usage(provider, window_kind, window_start, request_count)
			values($1, $2, $3, 1)
			on conflict(provider, window_kind, window_start) do update set
				request_count = external_api_rate_limit_usage.request_count + 1,
				updated_at = now()
			where external_api_rate_limit_usage.request_count < $4
			returning request_count
		`, openMeteoProvider, window.Kind, window.Start, window.Limit).Scan(&count)
		if errors.Is(err, pgx.ErrNoRows) {
			return max(window.Next.Sub(now), time.Second), ErrOpenMeteoRateLimited
		}
		if err != nil {
			return 0, err
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return 0, err
	}
	return 0, nil
}

func openMeteoRateLimitWindows(now time.Time) []externalAPIRateLimitWindow {
	now = now.UTC()
	minute := now.Truncate(time.Minute)
	hour := now.Truncate(time.Hour)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return []externalAPIRateLimitWindow{
		{Kind: "minute", Start: minute, Next: minute.Add(time.Minute), Limit: openMeteoMinuteRequestLimit},
		{Kind: "hour", Start: hour, Next: hour.Add(time.Hour), Limit: openMeteoHourlyRequestLimit},
		{Kind: "day", Start: day, Next: day.AddDate(0, 0, 1), Limit: openMeteoDailyRequestLimit},
		{Kind: "month", Start: month, Next: month.AddDate(0, 1, 0), Limit: openMeteoMonthlyRequestLimit},
	}
}

func (service *OpenMeteoWeatherService) Fetch(ctx context.Context, activityStart time.Time, latitude, longitude float64) (*ActivityWeather, error) {
	if service == nil || service.client == nil || service.reserveLimit == nil {
		return nil, errors.New("Open-Meteo fallback is unavailable")
	}
	if activityStart.IsZero() || !validWeatherCoordinate(latitude, longitude) {
		return nil, errors.New("activity start time and coordinates are required for weather fallback")
	}
	if err := service.waitForRequestSlot(ctx); err != nil {
		return nil, err
	}
	now := service.now().UTC()
	if retryAfter, err := service.reserveLimit(ctx, now); err != nil {
		if errors.Is(err, ErrOpenMeteoRateLimited) {
			return nil, fmt.Errorf("%w; retry after %s", ErrOpenMeteoRateLimited, now.Add(retryAfter).Format(time.RFC3339))
		}
		return nil, fmt.Errorf("reserve Open-Meteo request: %w", err)
	}

	endpoint := service.forecastURL
	if activityStart.UTC().Before(now.AddDate(0, 0, -openMeteoRecentHistoryDays)) {
		endpoint = service.archiveURL
	}
	requestURL, err := openMeteoRequestURL(endpoint, activityStart, latitude, longitude)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build Open-Meteo request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Runnarr activity weather (https://github.com/bznein/Runnarr)")
	response, err := service.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch Open-Meteo weather: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Open-Meteo returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, openMeteoMaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Open-Meteo response: %w", err)
	}
	if len(body) > openMeteoMaxResponseBytes {
		return nil, errors.New("Open-Meteo response is too large")
	}
	var payload openMeteoResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Open-Meteo response: %w", err)
	}
	var raw map[string]any
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("retain Open-Meteo response: %w", err)
	}
	return activityWeatherFromOpenMeteo(payload, raw, activityStart)
}

func (service *OpenMeteoWeatherService) waitForRequestSlot(ctx context.Context) error {
	for {
		service.requestMu.Lock()
		now := time.Now()
		wait := service.nextRequest.Sub(now)
		if wait <= 0 {
			service.nextRequest = now.Add(service.minInterval)
			service.requestMu.Unlock()
			return nil
		}
		service.requestMu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func openMeteoRequestURL(endpoint string, activityStart time.Time, latitude, longitude float64) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse Open-Meteo endpoint: %w", err)
	}
	date := activityStart.UTC().Format("2006-01-02")
	params := parsed.Query()
	params.Set("latitude", strconv.FormatFloat(roundWeatherCoordinate(latitude), 'f', 3, 64))
	params.Set("longitude", strconv.FormatFloat(roundWeatherCoordinate(longitude), 'f', 3, 64))
	params.Set("start_date", date)
	params.Set("end_date", date)
	params.Set("timezone", "UTC")
	params.Set("hourly", strings.Join([]string{
		"temperature_2m", "apparent_temperature", "dew_point_2m", "relative_humidity_2m",
		"weather_code", "wind_speed_10m", "wind_gusts_10m", "wind_direction_10m",
	}, ","))
	parsed.RawQuery = params.Encode()
	return parsed.String(), nil
}

func activityWeatherFromOpenMeteo(payload openMeteoResponse, raw map[string]any, activityStart time.Time) (*ActivityWeather, error) {
	index := -1
	closest := 2 * time.Hour
	var observedAt time.Time
	for candidate, value := range payload.Hourly.Time {
		parsed, err := time.ParseInLocation("2006-01-02T15:04", value, time.UTC)
		if err != nil {
			continue
		}
		distance := parsed.Sub(activityStart.UTC()).Abs()
		if distance < closest {
			index, closest, observedAt = candidate, distance, parsed
		}
	}
	if index < 0 || closest > 90*time.Minute {
		return nil, errOpenMeteoNoWeather
	}
	weather := &ActivityWeather{
		Provider:             openMeteoProvider,
		ObservedAt:           &observedAt,
		TemperatureC:         floatAt(payload.Hourly.Temperature, index),
		ApparentTemperatureC: floatAt(payload.Hourly.ApparentTemperature, index),
		DewPointC:            floatAt(payload.Hourly.DewPoint, index),
		RelativeHumidityPct:  floatAt(payload.Hourly.RelativeHumidity, index),
		WindSpeedKPH:         floatAt(payload.Hourly.WindSpeed, index),
		WindGustKPH:          floatAt(payload.Hourly.WindGusts, index),
		WindDirectionDeg:     floatAt(payload.Hourly.WindDirection, index),
		Latitude:             &payload.Latitude,
		Longitude:            &payload.Longitude,
		StationTimezone:      strings.TrimSpace(payload.Timezone),
		Raw:                  raw,
	}
	if code := intAt(payload.Hourly.WeatherCode, index); code != nil {
		weather.Condition = openMeteoWeatherCodeLabel(*code)
	}
	if weather.WindDirectionDeg != nil {
		weather.WindDirection = weatherCompassDirection(*weather.WindDirectionDeg)
	}
	if !activityWeatherHasDisplayData(weather) {
		return nil, errOpenMeteoNoWeather
	}
	return weather, nil
}

func floatAt(values []*float64, index int) *float64 {
	if index < 0 || index >= len(values) || values[index] == nil || math.IsNaN(*values[index]) || math.IsInf(*values[index], 0) {
		return nil
	}
	value := *values[index]
	return &value
}

func intAt(values []*int, index int) *int {
	if index < 0 || index >= len(values) || values[index] == nil {
		return nil
	}
	value := *values[index]
	return &value
}

func validWeatherCoordinate(latitude, longitude float64) bool {
	return !math.IsNaN(latitude) && !math.IsNaN(longitude) && !math.IsInf(latitude, 0) && !math.IsInf(longitude, 0) &&
		latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180 && !(latitude == 0 && longitude == 0)
}

func roundWeatherCoordinate(value float64) float64 {
	return math.Round(value*openMeteoCoordinatePrecision) / openMeteoCoordinatePrecision
}

func weatherCompassDirection(degrees float64) string {
	directions := [...]string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	normalized := math.Mod(degrees, 360)
	if normalized < 0 {
		normalized += 360
	}
	return directions[int(math.Floor((normalized+11.25)/22.5))%len(directions)]
}

func openMeteoWeatherCodeLabel(code int) string {
	switch code {
	case 0:
		return "Clear sky"
	case 1:
		return "Mainly clear"
	case 2:
		return "Partly cloudy"
	case 3:
		return "Overcast"
	case 45, 48:
		return "Fog"
	case 51, 53, 55:
		return "Drizzle"
	case 56, 57:
		return "Freezing drizzle"
	case 61, 63, 65:
		return "Rain"
	case 66, 67:
		return "Freezing rain"
	case 71, 73, 75:
		return "Snow"
	case 77:
		return "Snow grains"
	case 80, 81, 82:
		return "Rain showers"
	case 85, 86:
		return "Snow showers"
	case 95:
		return "Thunderstorm"
	case 96, 99:
		return "Thunderstorm with hail"
	default:
		return ""
	}
}

func (s *Server) handleWeatherConfig(w http.ResponseWriter, r *http.Request) {
	config, err := s.store.GetWeatherConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load weather settings")
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) handleUpdateWeatherConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OpenMeteoFallbackEnabled *bool `json:"openMeteoFallbackEnabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.OpenMeteoFallbackEnabled == nil {
		writeError(w, http.StatusBadRequest, "openMeteoFallbackEnabled is required")
		return
	}
	config := WeatherConfig{OpenMeteoFallbackEnabled: *body.OpenMeteoFallbackEnabled}
	if err := s.store.SetWeatherConfig(r.Context(), config); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save weather settings")
		return
	}
	writeJSON(w, http.StatusOK, config)
}
