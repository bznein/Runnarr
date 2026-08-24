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
	"sort"
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
	openMeteoRecentRequestUnits  = 3

	openMeteoMethodMultiModel15Min = "midpoint-15-minute-multi-model"
	openMeteoMethodArchiveHourly   = "midpoint-nearest-hour-archive"
)

var (
	ErrOpenMeteoRateLimited = errors.New("Open-Meteo fallback rate limit reached")
	errOpenMeteoNoWeather   = errors.New("Open-Meteo returned no weather near the activity midpoint")
)

type openMeteoModel struct {
	ID    string
	Label string
}

var openMeteoConsensusModels = []openMeteoModel{
	{ID: "ukmo_seamless", Label: "UKMO Seamless"},
	{ID: "icon_seamless", Label: "ICON Seamless"},
	{ID: "ecmwf_ifs025", Label: "ECMWF IFS"},
}

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
	reserveLimit func(context.Context, time.Time, int) (time.Duration, error)
}

type openMeteoSeries map[string]json.RawMessage

type openMeteoResponse struct {
	Latitude   float64         `json:"latitude"`
	Longitude  float64         `json:"longitude"`
	Timezone   string          `json:"timezone"`
	Minutely15 openMeteoSeries `json:"minutely_15"`
	Hourly     openMeteoSeries `json:"hourly"`
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

func (s *Store) ReserveOpenMeteoRequest(ctx context.Context, now time.Time, units int) (time.Duration, error) {
	if units < 1 {
		units = 1
	}
	transaction, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	for _, window := range openMeteoRateLimitWindows(now) {
		var count int
		err = transaction.QueryRow(ctx, `
			insert into external_api_rate_limit_usage(provider, window_kind, window_start, request_count)
			values($1, $2, $3, $4)
			on conflict(provider, window_kind, window_start) do update set
				request_count = external_api_rate_limit_usage.request_count + excluded.request_count,
				updated_at = now()
			where external_api_rate_limit_usage.request_count + excluded.request_count <= $5
			returning request_count
		`, openMeteoProvider, window.Kind, window.Start, units, window.Limit).Scan(&count)
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
		return nil, errors.New("activity midpoint time and coordinates are required for weather fallback")
	}
	if err := service.waitForRequestSlot(ctx); err != nil {
		return nil, err
	}
	now := service.now().UTC()
	recent := !activityStart.UTC().Before(now.AddDate(0, 0, -openMeteoRecentHistoryDays))
	requestUnits := 1
	if recent {
		requestUnits = openMeteoRecentRequestUnits
	}
	if retryAfter, err := service.reserveLimit(ctx, now, requestUnits); err != nil {
		if errors.Is(err, ErrOpenMeteoRateLimited) {
			return nil, fmt.Errorf("%w; retry after %s", ErrOpenMeteoRateLimited, now.Add(retryAfter).Format(time.RFC3339))
		}
		return nil, fmt.Errorf("reserve Open-Meteo request: %w", err)
	}

	endpoint := service.forecastURL
	if !recent {
		endpoint = service.archiveURL
	}
	requestURL, err := openMeteoRequestURL(endpoint, activityStart, latitude, longitude, recent)
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
	return activityWeatherFromOpenMeteo(payload, raw, activityStart, recent)
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

func openMeteoRequestURL(endpoint string, activityStart time.Time, latitude, longitude float64, recent bool) (string, error) {
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
	variables := strings.Join([]string{
		"temperature_2m", "apparent_temperature", "dew_point_2m", "relative_humidity_2m",
		"weather_code", "wind_speed_10m", "wind_gusts_10m", "wind_direction_10m",
	}, ",")
	if recent {
		models := make([]string, 0, len(openMeteoConsensusModels))
		for _, model := range openMeteoConsensusModels {
			models = append(models, model.ID)
		}
		params.Set("minutely_15", variables)
		params.Set("models", strings.Join(models, ","))
	} else {
		params.Set("hourly", variables)
	}
	parsed.RawQuery = params.Encode()
	return parsed.String(), nil
}

func activityWeatherFromOpenMeteo(payload openMeteoResponse, raw map[string]any, activityStart time.Time, recent bool) (*ActivityWeather, error) {
	if !recent {
		index, observedAt, err := nearestOpenMeteoTime(payload.Hourly, activityStart, 90*time.Minute)
		if err != nil {
			return nil, err
		}
		return openMeteoWeatherAt(payload, raw, payload.Hourly, "", index, observedAt, openMeteoMethodArchiveHourly, "")
	}

	index, observedAt, err := nearestOpenMeteoTime(payload.Minutely15, activityStart, 20*time.Minute)
	if err != nil {
		return nil, err
	}
	type modelCandidate struct {
		temperature float64
		weather     *ActivityWeather
	}
	candidates := make([]modelCandidate, 0, len(openMeteoConsensusModels))
	for _, model := range openMeteoConsensusModels {
		weather, candidateErr := openMeteoWeatherAt(payload, raw, payload.Minutely15, "_"+model.ID, index, observedAt, openMeteoMethodMultiModel15Min, model.Label)
		if candidateErr != nil || weather.TemperatureC == nil {
			continue
		}
		candidates = append(candidates, modelCandidate{temperature: *weather.TemperatureC, weather: weather})
	}
	if len(candidates) == 0 {
		return nil, errOpenMeteoNoWeather
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].temperature < candidates[j].temperature
	})
	return candidates[len(candidates)/2].weather, nil
}

func nearestOpenMeteoTime(series openMeteoSeries, target time.Time, maximumDistance time.Duration) (int, time.Time, error) {
	times := stringValues(series, "time")
	index := -1
	closest := maximumDistance + time.Second
	var observedAt time.Time
	for candidate, value := range times {
		parsed, err := time.ParseInLocation("2006-01-02T15:04", value, time.UTC)
		if err != nil {
			continue
		}
		distance := parsed.Sub(target.UTC()).Abs()
		if distance < closest {
			index, closest, observedAt = candidate, distance, parsed
		}
	}
	if index < 0 || closest > maximumDistance {
		return -1, time.Time{}, errOpenMeteoNoWeather
	}
	return index, observedAt, nil
}

func openMeteoWeatherAt(payload openMeteoResponse, raw map[string]any, series openMeteoSeries, suffix string, index int, observedAt time.Time, method, model string) (*ActivityWeather, error) {
	weather := &ActivityWeather{
		Provider:             openMeteoProvider,
		SelectionMethod:      method,
		Model:                model,
		ObservedAt:           &observedAt,
		TemperatureC:         floatAt(floatValues(series, "temperature_2m"+suffix), index),
		ApparentTemperatureC: floatAt(floatValues(series, "apparent_temperature"+suffix), index),
		DewPointC:            floatAt(floatValues(series, "dew_point_2m"+suffix), index),
		RelativeHumidityPct:  floatAt(floatValues(series, "relative_humidity_2m"+suffix), index),
		WindSpeedKPH:         floatAt(floatValues(series, "wind_speed_10m"+suffix), index),
		WindGustKPH:          floatAt(floatValues(series, "wind_gusts_10m"+suffix), index),
		WindDirectionDeg:     floatAt(floatValues(series, "wind_direction_10m"+suffix), index),
		Latitude:             &payload.Latitude,
		Longitude:            &payload.Longitude,
		StationTimezone:      strings.TrimSpace(payload.Timezone),
		Raw:                  raw,
	}
	if code := intAt(intValues(series, "weather_code"+suffix), index); code != nil {
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

func stringValues(series openMeteoSeries, key string) []string {
	var values []string
	if raw := series[key]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &values)
	}
	return values
}

func floatValues(series openMeteoSeries, key string) []*float64 {
	var values []*float64
	if raw := series[key]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &values)
	}
	return values
}

func intValues(series openMeteoSeries, key string) []*int {
	var values []*int
	if raw := series[key]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &values)
	}
	return values
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
