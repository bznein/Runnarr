package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type RaceForecastHour struct {
	Time                 time.Time `json:"time"`
	TemperatureC         *float64  `json:"temperatureC,omitempty"`
	ApparentTemperatureC *float64  `json:"apparentTemperatureC,omitempty"`
	Humidity             *float64  `json:"humidity,omitempty"`
	RainProbability      *float64  `json:"rainProbability,omitempty"`
	WindKPH              *float64  `json:"windKph,omitempty"`
	WindDirection        *float64  `json:"windDirection,omitempty"`
}
type RaceForecast struct {
	Hours       []RaceForecastHour `json:"hours"`
	FetchedAt   *time.Time         `json:"fetchedAt,omitempty"`
	AttemptedAt *time.Time         `json:"attemptedAt,omitempty"`
	Error       string             `json:"error,omitempty"`
	Reason      string             `json:"reason,omitempty"`
	Stale       bool               `json:"stale"`
	Saved       bool               `json:"saved"`
	Attribution string             `json:"attribution"`
}

func raceForecastTarget(r Race) string {
	if r.Latitude == nil || r.Longitude == nil {
		return ""
	}
	return fmt.Sprintf("%s|%s|%s|%.3f|%.3f", r.Date, r.StartTime, r.Timezone, roundWeatherCoordinate(*r.Latitude), roundWeatherCoordinate(*r.Longitude))
}
func (s *Store) RaceForecast(ctx context.Context, r Race) (RaceForecast, error) {
	result := RaceForecast{Hours: []RaceForecastHour{}, Attribution: "Weather forecast by Open-Meteo (CC BY 4.0)"}
	var data []byte
	err := s.db.QueryRow(ctx, `select fetched_at,attempted_at,error,data from race_forecasts where race_id=$1 and user_id=$2 and target_key=$3`, r.ID, scopedUserID(ctx), raceForecastTarget(r)).Scan(&result.FetchedAt, &result.AttemptedAt, &result.Error, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if len(data) > 0 {
		if err = json.Unmarshal(data, &result.Hours); err != nil {
			return result, err
		}
	}
	cutoff, err := raceCutoff(r)
	if err == nil && result.FetchedAt != nil {
		result.Saved = !time.Now().Before(cutoff) && result.FetchedAt.Before(cutoff)
		result.Stale = !result.Saved && (time.Since(*result.FetchedAt) > 6*time.Hour || result.Error != "")
	}
	return result, nil
}
func (service *OpenMeteoWeatherService) fetchRaceForecast(ctx context.Context, r Race) ([]RaceForecastHour, json.RawMessage, error) {
	if service == nil || service.client == nil || service.reserveLimit == nil {
		return nil, nil, errors.New("forecast service is unavailable")
	}
	if err := service.waitForRequestSlot(ctx); err != nil {
		return nil, nil, err
	}
	if _, err := service.reserveLimit(ctx, service.now().UTC(), 1); err != nil {
		return nil, nil, err
	}
	endpoint, err := url.Parse(service.forecastURL)
	if err != nil {
		return nil, nil, err
	}
	q := endpoint.Query()
	q.Set("latitude", strconv.FormatFloat(roundWeatherCoordinate(*r.Latitude), 'f', 3, 64))
	q.Set("longitude", strconv.FormatFloat(roundWeatherCoordinate(*r.Longitude), 'f', 3, 64))
	q.Set("start_date", r.Date)
	q.Set("end_date", r.Date)
	q.Set("timezone", r.Timezone)
	q.Set("timeformat", "unixtime")
	q.Set("hourly", "temperature_2m,apparent_temperature,relative_humidity_2m,precipitation_probability,wind_speed_10m,wind_direction_10m")
	endpoint.RawQuery = q.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("User-Agent", "Runnarr race forecast (https://github.com/bznein/Runnarr)")
	response, err := service.client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("Open-Meteo returned status %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, openMeteoMaxResponseBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > openMeteoMaxResponseBytes {
		return nil, nil, errors.New("forecast response is too large")
	}
	hours, err := normalizeRaceForecast(raw, r)
	return hours, raw, err
}
func normalizeRaceForecast(raw []byte, r Race) ([]RaceForecastHour, error) {
	var payload struct {
		Hourly map[string]json.RawMessage `json:"hourly"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	var times []int64
	if err := json.Unmarshal(payload.Hourly["time"], &times); err != nil {
		return nil, errors.New("forecast has no hourly timestamps")
	}
	series := map[string][]*float64{}
	for _, key := range []string{"temperature_2m", "apparent_temperature", "relative_humidity_2m", "precipitation_probability", "wind_speed_10m", "wind_direction_10m"} {
		var values []*float64
		if b, ok := payload.Hourly[key]; ok {
			if err := json.Unmarshal(b, &values); err != nil {
				return nil, err
			}
		}
		series[key] = values
	}
	value := func(key string, index int) *float64 {
		if index < len(series[key]) {
			return series[key][index]
		}
		return nil
	}
	zone, err := time.LoadLocation(r.Timezone)
	if err != nil {
		return nil, err
	}
	result := []RaceForecastHour{}
	for i, timestamp := range times {
		at := time.Unix(timestamp, 0).UTC()
		if at.In(zone).Format("2006-01-02") != r.Date {
			continue
		}
		h := RaceForecastHour{Time: at, TemperatureC: value("temperature_2m", i), ApparentTemperatureC: value("apparent_temperature", i), Humidity: value("relative_humidity_2m", i), RainProbability: value("precipitation_probability", i), WindKPH: value("wind_speed_10m", i), WindDirection: value("wind_direction_10m", i)}
		if h.TemperatureC != nil || h.ApparentTemperatureC != nil || h.Humidity != nil || h.RainProbability != nil || h.WindKPH != nil {
			result = append(result, h)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("no usable forecast hours for race day")
	}
	return result, nil
}
func (s *Server) refreshRaceForecast(ctx context.Context, r Race) (RaceForecast, error) {
	current, err := s.store.RaceForecast(ctx, r)
	if err != nil {
		return current, err
	}
	settings, err := s.store.RaceSettings(ctx)
	if err != nil {
		return current, err
	}
	if !settings.WeatherEnabled {
		current.Reason = "Enable race weather to request a forecast"
		return current, nil
	}
	if r.Status == "cancelled" || r.Status == "finished" || r.Status == "dns" || r.Status == "dnf" || r.Status == "disqualified" {
		current.Reason = "Forecast refresh is available for upcoming races"
		return current, nil
	}
	cutoff, err := raceCutoff(r)
	if err != nil {
		current.Reason = "Set the race date before requesting weather"
		return current, nil
	}
	now := time.Now().UTC()
	if !now.Before(cutoff) {
		current.Reason = "Only a forecast fetched before race start can be saved"
		return current, nil
	}
	if cutoff.Sub(now) > 16*24*time.Hour {
		current.Reason = "Race day is outside the 16-day forecast window"
		return current, nil
	}
	if r.Latitude == nil || r.Longitude == nil {
		current.Reason = "Choose the forecast location first"
		return current, nil
	}
	if current.AttemptedAt != nil && now.Sub(*current.AttemptedAt) < 6*time.Hour {
		return current, nil
	}
	target := raceForecastTarget(r)
	tag, err := s.store.db.Exec(ctx, `insert into race_forecasts(race_id,user_id,target_key,attempted_at) select id,user_id,$3,$4 from races where id=$1 and user_id=$2 and revision=$5
        on conflict(race_id) do update set target_key=excluded.target_key,attempted_at=excluded.attempted_at,error='',
        fetched_at=case when race_forecasts.target_key=excluded.target_key then race_forecasts.fetched_at else null end,
        data=case when race_forecasts.target_key=excluded.target_key then race_forecasts.data else null end,
        raw=case when race_forecasts.target_key=excluded.target_key then race_forecasts.raw else null end
        where race_forecasts.target_key<>excluded.target_key or race_forecasts.attempted_at < excluded.attempted_at-interval '6 hours'`, r.ID, scopedUserID(ctx), target, now, r.Revision)
	if err != nil {
		return current, err
	}
	if tag.RowsAffected() == 0 {
		return s.store.RaceForecast(ctx, r)
	}
	hours, raw, fetchErr := s.garmin.weatherFallback.fetchRaceForecast(ctx, r)
	finished := time.Now().UTC()
	if fetchErr == nil && !finished.Before(cutoff) {
		fetchErr = errors.New("forecast arrived after the race start; pre-race snapshot was retained")
	}
	if fetchErr != nil {
		_, err = s.store.db.Exec(ctx, `update race_forecasts set error=$4 where race_id=$1 and user_id=$2 and target_key=$3 and attempted_at=$5`, r.ID, scopedUserID(ctx), target, fetchErr.Error(), now)
	} else {
		data, marshalErr := json.Marshal(hours)
		if marshalErr != nil {
			return current, marshalErr
		}
		_, err = s.store.db.Exec(ctx, `update race_forecasts set fetched_at=$4,error='',data=$5,raw=$6 where race_id=$1 and user_id=$2 and target_key=$3 and attempted_at=$7`, r.ID, scopedUserID(ctx), target, finished, data, []byte(raw), now)
	}
	if err != nil {
		return current, err
	}
	return s.store.RaceForecast(ctx, r)
}
