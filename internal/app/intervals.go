package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const intervalsProvider = "intervals"
const intervalsActivityWindowLimit = 100
const intervalsActivitySplitThreshold = 40

type SyncProgressFunc func(map[string]any)

type IntervalsService struct {
	store   *Store
	cipher  *TokenCipher
	client  *http.Client
	baseURL string
}

type IntervalsSyncOptions struct {
	Oldest time.Time
	Newest time.Time
}

func NewIntervalsService(store *Store, cipher *TokenCipher) *IntervalsService {
	return &IntervalsService{
		store:   store,
		cipher:  cipher,
		client:  &http.Client{Timeout: 60 * time.Second},
		baseURL: "https://intervals.icu",
	}
}

func (s *IntervalsService) Status(ctx context.Context) (ProviderConnection, bool, error) {
	conn, err := s.store.GetProviderConnection(ctx, intervalsProvider)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderConnection{Provider: intervalsProvider}, false, nil
	}
	if err != nil {
		return ProviderConnection{}, false, err
	}
	return conn.ProviderConnection, true, nil
}

func (s *IntervalsService) Connect(ctx context.Context, apiKey string) (ProviderConnection, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ProviderConnection{}, errors.New("Intervals.icu API key is required")
	}

	activities, err := s.listActivities(ctx, apiKey, time.Now().AddDate(0, 0, -30), time.Now(), 1)
	if err != nil {
		return ProviderConnection{}, err
	}

	accountID := "0"
	if len(activities) > 0 && activities[0].IcuAthleteID != "" {
		accountID = activities[0].IcuAthleteID
	}
	tokenCiphertext, err := s.cipher.EncryptString(apiKey)
	if err != nil {
		return ProviderConnection{}, err
	}
	conn := StoredProviderConnection{
		ProviderConnection: ProviderConnection{
			Provider:          intervalsProvider,
			ProviderAccountID: accountID,
			DisplayName:       "Intervals.icu",
			Scopes:            []string{"api_key"},
		},
		AccessTokenCiphertext: tokenCiphertext,
	}
	if err := s.store.UpsertProviderConnection(ctx, conn); err != nil {
		return ProviderConnection{}, err
	}
	saved, err := s.store.GetProviderConnection(ctx, intervalsProvider)
	if err != nil {
		return ProviderConnection{}, err
	}
	return saved.ProviderConnection, nil
}

func (s *IntervalsService) Sync(ctx context.Context, opts IntervalsSyncOptions, progress SyncProgressFunc) (map[string]any, error) {
	conn, err := s.store.GetProviderConnection(ctx, intervalsProvider)
	if err != nil {
		return nil, err
	}
	apiKey, err := s.cipher.DecryptString(conn.AccessTokenCiphertext)
	if err != nil {
		return nil, err
	}
	if apiKey == "" {
		return nil, errors.New("Intervals.icu API key is missing")
	}

	oldest := opts.Oldest
	if oldest.IsZero() {
		oldest = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	newest := opts.Newest
	if newest.IsZero() {
		newest = time.Now().UTC()
	}
	if oldest.After(newest) {
		return nil, errors.New("oldest sync date must be before newest sync date")
	}

	imported := 0
	failed := 0
	total := 0
	fitDownloads := 0
	summaryOnly := 0
	lapsImported := 0
	windows := 0
	fetchedWindows := 0
	splitWindows := 0
	cappedWindows := 0
	var firstErrors []string
	var warnings []string
	var currentWindowStart time.Time
	var currentWindowEnd time.Time
	currentActivity := ""
	lastProgress := time.Time{}
	seenActivityIDs := make(map[string]bool)

	payload := func(stage string) map[string]any {
		out := map[string]any{
			"stage":           stage,
			"activities":      total,
			"processed":       imported + failed,
			"imported":        imported,
			"failed":          failed,
			"fitDownloads":    fitDownloads,
			"summaryOnly":     summaryOnly,
			"lapsImported":    lapsImported,
			"windows":         windows,
			"fetchedWindows":  fetchedWindows,
			"splitWindows":    splitWindows,
			"cappedWindows":   cappedWindows,
			"oldest":          oldest.Format("2006-01-02"),
			"newest":          newest.Format("2006-01-02"),
			"firstErrors":     append([]string(nil), firstErrors...),
			"warnings":        append([]string(nil), warnings...),
			"source":          intervalsProvider,
			"dedupeKey":       "Intervals.icu activity ID",
			"windowLimit":     intervalsActivityWindowLimit,
			"splitThreshold":  intervalsActivitySplitThreshold,
			"automaticMode":   "scheduled sync uses the last 30 days",
			"completionBasis": "all activity windows fetched from oldest through newest; busy windows are split to avoid limited API responses",
		}
		if !currentWindowStart.IsZero() {
			out["currentWindowStart"] = currentWindowStart.Format("2006-01-02")
		}
		if !currentWindowEnd.IsZero() {
			out["currentWindowEnd"] = currentWindowEnd.Format("2006-01-02")
		}
		if currentActivity != "" {
			out["currentActivity"] = currentActivity
		}
		return out
	}
	report := func(stage string, force bool) {
		if progress == nil {
			return
		}
		now := time.Now()
		if !force && !lastProgress.IsZero() && now.Sub(lastProgress) < time.Second {
			return
		}
		lastProgress = now
		progress(payload(stage))
	}
	report("starting", true)

	fetchWindow := func(windowStart, windowEnd time.Time) ([]intervalsActivity, error) {
		var fetch func(time.Time, time.Time) ([]intervalsActivity, error)
		fetch = func(start, end time.Time) ([]intervalsActivity, error) {
			currentWindowStart = start
			currentWindowEnd = end
			report("fetching", true)
			activities, err := s.listActivities(ctx, apiKey, start, end, intervalsActivityWindowLimit)
			if err != nil {
				return nil, err
			}
			fetchedWindows++
			if len(activities) >= intervalsActivitySplitThreshold && start.Before(end) {
				splitWindows++
				report("splitting", true)
				days := int(end.Sub(start).Hours() / 24)
				mid := start.AddDate(0, 0, days/2)
				left, err := fetch(start, mid)
				if err != nil {
					return nil, err
				}
				right, err := fetch(mid.AddDate(0, 0, 1), end)
				if err != nil {
					return nil, err
				}
				return append(left, right...), nil
			}
			if len(activities) >= intervalsActivitySplitThreshold {
				cappedWindows++
				warnings = append(warnings, fmt.Sprintf("%s returned %d activities; this single-day window may still be capped by Intervals.icu", start.Format("2006-01-02"), len(activities)))
			}
			report("fetched", true)
			return activities, nil
		}
		return fetch(windowStart, windowEnd)
	}

	for windowStart := startOfDay(oldest); !windowStart.After(newest); windowStart = windowStart.AddDate(1, 0, 0) {
		windowEnd := startOfDay(minTime(windowStart.AddDate(1, 0, 0).Add(-time.Second), newest))
		activities, err := fetchWindow(windowStart, windowEnd)
		if err != nil {
			return nil, err
		}
		windows++
		sources := uniqueIntervalsActivities(activities, seenActivityIDs)
		total += len(sources)
		report("importing", true)

		for _, source := range sources {
			if source.ID == "" {
				continue
			}
			currentActivity = source.ID
			importedActivity, usedFit, err := s.importedActivity(ctx, apiKey, source)
			if err != nil {
				failed++
				if len(firstErrors) < 5 {
					firstErrors = append(firstErrors, source.ID+": "+err.Error())
				}
				continue
			}
			if usedFit {
				fitDownloads++
			} else {
				summaryOnly++
			}
			lapsImported += len(importedActivity.Laps)
			normalizeImported(&importedActivity)
			if _, err := s.store.SaveImportedActivity(ctx, intervalsProvider, source.ID, nil, importedActivity); err != nil {
				failed++
				if len(firstErrors) < 5 {
					firstErrors = append(firstErrors, source.ID+": "+err.Error())
				}
				continue
			}
			imported++
			report("importing", false)
		}
		currentActivity = ""
		report("importing", true)
	}

	finalPayload := payload("completed")
	report("completed", true)
	return finalPayload, nil
}

func uniqueIntervalsActivities(activities []intervalsActivity, seen map[string]bool) []intervalsActivity {
	out := make([]intervalsActivity, 0, len(activities))
	for _, activity := range activities {
		if activity.ID == "" {
			out = append(out, activity)
			continue
		}
		if seen[activity.ID] {
			continue
		}
		seen[activity.ID] = true
		out = append(out, activity)
	}
	return out
}

func (s *IntervalsService) importedActivity(ctx context.Context, apiKey string, source intervalsActivity) (ImportedActivity, bool, error) {
	data, err := s.downloadActivityFile(ctx, apiKey, source.ID, "fit-file")
	if err == nil && len(data) > 0 {
		imported, parseErr := FITParser{}.Parse(ctx, source.ID+".fit", data)
		if parseErr == nil {
			applyIntervalsMetadata(&imported, source)
			s.applyIntervalsLaps(ctx, apiKey, source.ID, &imported)
			return imported, true, nil
		}
	}

	imported := importedFromIntervalsSummary(source)
	if imported.StartTime.IsZero() {
		return ImportedActivity{}, false, errors.New("activity has no usable start time")
	}
	s.applyIntervalsLaps(ctx, apiKey, source.ID, &imported)
	return imported, false, nil
}

func (s *IntervalsService) listActivities(ctx context.Context, apiKey string, oldest, newest time.Time, limit int) ([]intervalsActivity, error) {
	endpoint, err := url.Parse(s.baseURL + "/api/v1/athlete/0/activities")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("oldest", oldest.Format("2006-01-02"))
	if !newest.IsZero() {
		query.Set("newest", newest.Format("2006-01-02"))
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	endpoint.RawQuery = query.Encode()

	var activities []intervalsActivity
	if err := s.getJSON(ctx, apiKey, endpoint.String(), &activities); err != nil {
		return nil, err
	}
	return activities, nil
}

func (s *IntervalsService) applyIntervalsLaps(ctx context.Context, apiKey, activityID string, activity *ImportedActivity) {
	if len(activity.Laps) > 0 {
		return
	}
	intervals, err := s.activityIntervals(ctx, apiKey, activityID)
	if err != nil || len(intervals) == 0 {
		return
	}
	activity.Laps = lapsFromIntervals(activity.StartTime, intervals)
	if activity.Raw == nil {
		activity.Raw = map[string]any{}
	}
	activity.Raw["intervals_interval_count"] = len(intervals)
	activity.Raw["intervals_imported_lap_count"] = len(activity.Laps)
}

func (s *IntervalsService) activityIntervals(ctx context.Context, apiKey, activityID string) ([]intervalsInterval, error) {
	endpoint := s.baseURL + "/api/v1/activity/" + url.PathEscape(activityID) + "/intervals"
	var dto intervalsDTO
	if err := s.getJSON(ctx, apiKey, endpoint, &dto); err != nil {
		return nil, err
	}
	return dto.Intervals, nil
}

func (s *IntervalsService) downloadActivityFile(ctx context.Context, apiKey, activityID, kind string) ([]byte, error) {
	endpoint := s.baseURL + "/api/v1/activity/" + url.PathEscape(activityID) + "/" + kind
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("API_KEY", apiKey)
	req.Header.Set("User-Agent", "Runnarr/0.1 (+https://github.com/bznein/Runnarr)")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("Intervals.icu rejected the API key; use the personal API key from Intervals.icu Settings > Developer Settings")
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Intervals.icu file request failed: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 80<<20))
}

func (s *IntervalsService) getJSON(ctx context.Context, apiKey, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth("API_KEY", apiKey)
	req.Header.Set("User-Agent", "Runnarr/0.1 (+https://github.com/bznein/Runnarr)")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return errors.New("Intervals.icu rejected the API key; use the personal API key from Intervals.icu Settings > Developer Settings")
	}
	if resp.StatusCode >= 300 {
		var apiErr map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return fmt.Errorf("Intervals.icu request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func lapsFromIntervals(start time.Time, intervals []intervalsInterval) []ActivityLap {
	filtered := make([]intervalsInterval, 0, len(intervals))
	for _, interval := range intervals {
		if interval.ElapsedTime <= 0 && interval.MovingTime <= 0 && interval.Distance <= 0 {
			continue
		}
		filtered = append(filtered, interval)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].StartTime != filtered[j].StartTime {
			return filtered[i].StartTime < filtered[j].StartTime
		}
		return filtered[i].StartIndex < filtered[j].StartIndex
	})

	laps := make([]ActivityLap, 0, len(filtered))
	for i, interval := range filtered {
		elapsed := interval.ElapsedTime
		if elapsed == 0 {
			elapsed = interval.MovingTime
		}
		var lapStart *time.Time
		if !start.IsZero() && interval.StartTime >= 0 {
			value := start.Add(time.Duration(interval.StartTime) * time.Second)
			lapStart = &value
		}
		laps = append(laps, ActivityLap{
			Index:        i,
			StartTime:    lapStart,
			ElapsedTimeS: elapsed,
			DistanceM:    interval.Distance,
		})
	}
	return laps
}

func importedFromIntervalsSummary(source intervalsActivity) ImportedActivity {
	distance := source.Distance
	if distance == 0 {
		distance = source.IcuDistance
	}
	start := parseIntervalsTime(source.StartDate, source.StartDateLocal)
	activity := ImportedActivity{
		Name:           source.Name,
		SportType:      normalizeSport(source.Type),
		StartTime:      start,
		DistanceM:      distance,
		MovingTimeS:    source.MovingTime,
		ElapsedTimeS:   source.ElapsedTime,
		ElevationGainM: source.TotalElevationGain,
		AvgHeartRate:   source.AverageHeartRate,
		MaxHeartRate:   source.MaxHeartRate,
	}
	applyIntervalsMetadata(&activity, source)
	return activity
}

func applyIntervalsMetadata(activity *ImportedActivity, source intervalsActivity) {
	if source.Name != "" {
		activity.Name = source.Name
	}
	if source.Type != "" {
		activity.SportType = normalizeSport(source.Type)
	}
	if activity.StartTime.IsZero() {
		activity.StartTime = parseIntervalsTime(source.StartDate, source.StartDateLocal)
	}
	if activity.DistanceM == 0 {
		activity.DistanceM = source.Distance
		if activity.DistanceM == 0 {
			activity.DistanceM = source.IcuDistance
		}
	}
	if activity.MovingTimeS == 0 {
		activity.MovingTimeS = source.MovingTime
	}
	if activity.ElapsedTimeS == 0 {
		activity.ElapsedTimeS = source.ElapsedTime
	}
	if activity.ElevationGainM == 0 {
		activity.ElevationGainM = source.TotalElevationGain
	}
	if activity.AvgHeartRate == nil {
		activity.AvgHeartRate = source.AverageHeartRate
	}
	if activity.MaxHeartRate == nil {
		activity.MaxHeartRate = source.MaxHeartRate
	}
	if activity.Raw == nil {
		activity.Raw = map[string]any{}
	}
	activity.Raw["provider"] = intervalsProvider
	activity.Raw["intervals_id"] = source.ID
	activity.Raw["intervals_source"] = source.Source
	activity.Raw["intervals_external_id"] = source.ExternalID
	activity.Raw["intervals_strava_id"] = source.StravaID
	activity.Raw["intervals_file_type"] = source.FileType
}

func parseIntervalsTime(values ...string) time.Time {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, value); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Time{}
}

func startOfDay(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

type intervalsActivity struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Type               string   `json:"type"`
	StartDate          string   `json:"start_date"`
	StartDateLocal     string   `json:"start_date_local"`
	Distance           float64  `json:"distance"`
	IcuDistance        float64  `json:"icu_distance"`
	MovingTime         int      `json:"moving_time"`
	ElapsedTime        int      `json:"elapsed_time"`
	TotalElevationGain float64  `json:"total_elevation_gain"`
	AverageHeartRate   *float64 `json:"average_heartrate"`
	MaxHeartRate       *float64 `json:"max_heartrate"`
	StravaID           string   `json:"strava_id"`
	ExternalID         string   `json:"external_id"`
	Source             string   `json:"source"`
	FileType           string   `json:"file_type"`
	IcuAthleteID       string   `json:"icu_athlete_id"`
}

type intervalsDTO struct {
	ID        string              `json:"id"`
	Intervals []intervalsInterval `json:"icu_intervals"`
}

type intervalsInterval struct {
	ID          int     `json:"id"`
	Type        string  `json:"type"`
	Label       string  `json:"label"`
	StartIndex  int     `json:"start_index"`
	EndIndex    int     `json:"end_index"`
	StartTime   int     `json:"start_time"`
	EndTime     int     `json:"end_time"`
	Distance    float64 `json:"distance"`
	MovingTime  int     `json:"moving_time"`
	ElapsedTime int     `json:"elapsed_time"`
}
