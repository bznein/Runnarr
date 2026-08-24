package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const garminProvider = "garmin"
const garminActivityPageLimit = 100
const garminGearActivityPageLimit = 1000
const maxGarminActivityBytes = 100 << 20
const maxGarminBridgeOutputBytes = 140 << 20
const maxGarminSyncActivities = 10_000
const maxGarminGearActivities = 100_000

var ErrGarminBridgeOutputTooLarge = errors.New("Garmin bridge response is too large")
var ErrGarminNotFound = errors.New("Garmin resource not found")

type garminBridgeError struct {
	Code    string
	Message string
}

func (e *garminBridgeError) Error() string {
	return e.Message
}

func (e *garminBridgeError) Is(target error) bool {
	return target == ErrGarminNotFound && garminBridgeErrorIsNotFound(e.Code, e.Message)
}

func garminBridgeErrorIsNotFound(code, message string) bool {
	code = strings.ToLower(strings.TrimSpace(code))
	if strings.Contains(code, "notfound") || strings.Contains(code, "not_found") {
		return true
	}
	message = strings.ToLower(message)
	return strings.Contains(message, "client error (404)") ||
		strings.Contains(message, "api error 404") ||
		strings.Contains(message, "notfoundexception")
}

type GarminService struct {
	store           *Store
	bridge          GarminBridge
	weatherFallback *OpenMeteoWeatherService
	tokenDir        string
	legacyUserID    string
}

type GarminBridge interface {
	Connect(ctx context.Context, tokenStore, email, password, mfaCode string) (GarminBridgeProfile, error)
	ListActivities(ctx context.Context, tokenStore string, start, limit int) ([]GarminBridgeActivity, error)
	ListActivitySplits(ctx context.Context, tokenStore, activityID string) ([]GarminBridgeLap, error)
	GetActivityWorkout(ctx context.Context, tokenStore, activityID string) (GarminBridgeActivityWorkout, error)
	DownloadActivity(ctx context.Context, tokenStore, activityID string) ([]byte, error)
	FetchHealthDay(ctx context.Context, tokenStore, date string) (GarminBridgeHealthDay, error)
	ListGear(ctx context.Context, tokenStore string) (GarminBridgeGearResponse, error)
	ListGearActivities(ctx context.Context, tokenStore, gearID string, start, limit int) ([]GarminBridgeGearActivity, error)
	ListWorkouts(ctx context.Context, tokenStore string, start, limit int) ([]GarminBridgeWorkout, error)
	GetWorkout(ctx context.Context, tokenStore, workoutID string) (GarminBridgeWorkout, error)
	UploadWorkout(ctx context.Context, tokenStore string, payload map[string]any) (GarminBridgeWorkout, error)
	UploadCourse(ctx context.Context, tokenStore, filename string, content []byte, name string, sport CourseSport, description string) (GarminBridgeCourse, error)
	GetCourse(ctx context.Context, tokenStore, courseID string) (GarminBridgeCourse, error)
	DeleteWorkout(ctx context.Context, tokenStore, workoutID string) error
	ListScheduledWorkouts(ctx context.Context, tokenStore string, year, month int) ([]GarminBridgeScheduledWorkout, error)
	GetScheduledWorkout(ctx context.Context, tokenStore, scheduledWorkoutID string) (GarminBridgeScheduledWorkout, error)
	ScheduleWorkout(ctx context.Context, tokenStore, workoutID, date string) (GarminBridgeScheduledWorkout, error)
	UnscheduleWorkout(ctx context.Context, tokenStore, scheduledWorkoutID string) error
}

type GarminBridgeProfile struct {
	AccountID     string `json:"accountId"`
	DisplayName   string `json:"displayName"`
	FullName      string `json:"fullName"`
	UnitSystem    string `json:"unitSystem"`
	UserProfilePK string `json:"userProfilePk"`
}

type GarminBridgeActivity struct {
	ID                       string    `json:"id"`
	Name                     string    `json:"name"`
	SportType                string    `json:"sportType"`
	StartTime                time.Time `json:"startTime"`
	AvgGradeAdjustedSpeedMPS *float64  `json:"avgGradeAdjustedSpeed,omitempty"`
}

type GarminBridgeLap struct {
	Index                    int            `json:"index"`
	StartTime                *time.Time     `json:"startTime,omitempty"`
	ElapsedTimeS             int            `json:"elapsedTimeS"`
	MovingTimeS              int            `json:"movingTimeS"`
	DistanceM                float64        `json:"distanceM"`
	AvgPaceSPKM              *float64       `json:"avgPaceSPKM,omitempty"`
	AvgGradeAdjustedSpeedMPS *float64       `json:"avgGradeAdjustedSpeed,omitempty"`
	AvgGradeAdjustedPaceSPKM *float64       `json:"avgGradeAdjustedPaceSPKM,omitempty"`
	ElevationGainM           *float64       `json:"elevationGainM,omitempty"`
	ElevationLossM           *float64       `json:"elevationLossM,omitempty"`
	AvgHeartRate             *float64       `json:"avgHeartRate,omitempty"`
	MaxHeartRate             *float64       `json:"maxHeartRate,omitempty"`
	AvgPower                 *float64       `json:"avgPower,omitempty"`
	MaxPower                 *float64       `json:"maxPower,omitempty"`
	NormalizedPower          *float64       `json:"normalizedPower,omitempty"`
	AvgRunCadence            *float64       `json:"avgRunCadence,omitempty"`
	AvgGroundContactTimeMS   *float64       `json:"avgGroundContactTimeMS,omitempty"`
	AvgRespirationRate       *float64       `json:"avgRespirationRate,omitempty"`
	AvgTemperatureC          *float64       `json:"avgTemperatureC,omitempty"`
	IntensityType            string         `json:"intensityType,omitempty"`
	WorkoutStepIndex         *int           `json:"workoutStepIndex,omitempty"`
	Raw                      map[string]any `json:"raw,omitempty"`
}

type GarminBridgeActivityWorkout struct {
	Available bool                 `json:"available"`
	Workout   *ActivityWorkout     `json:"workout,omitempty"`
	Intervals []ActivityInterval   `json:"intervals,omitempty"`
	Laps      []GarminBridgeLap    `json:"laps,omitempty"`
	Weather   *GarminBridgeWeather `json:"weather,omitempty"`
	Raw       map[string]any       `json:"raw,omitempty"`
	Errors    map[string]string    `json:"errors,omitempty"`
}

type GarminBridgeWeather struct {
	ObservedAt           *time.Time     `json:"observedAt,omitempty"`
	Condition            string         `json:"condition,omitempty"`
	TemperatureF         *float64       `json:"temperatureF,omitempty"`
	ApparentTemperatureF *float64       `json:"apparentTemperatureF,omitempty"`
	DewPointF            *float64       `json:"dewPointF,omitempty"`
	RelativeHumidityPct  *float64       `json:"relativeHumidityPct,omitempty"`
	WindSpeedMPH         *float64       `json:"windSpeedMPH,omitempty"`
	WindGustMPH          *float64       `json:"windGustMPH,omitempty"`
	WindDirectionDeg     *float64       `json:"windDirectionDeg,omitempty"`
	WindDirection        string         `json:"windDirection,omitempty"`
	Latitude             *float64       `json:"latitude,omitempty"`
	Longitude            *float64       `json:"longitude,omitempty"`
	StationID            string         `json:"stationId,omitempty"`
	StationName          string         `json:"stationName,omitempty"`
	StationTimezone      string         `json:"stationTimezone,omitempty"`
	Raw                  map[string]any `json:"raw,omitempty"`
}

type GarminBridgeHealthDay struct {
	Date string         `json:"date"`
	Raw  map[string]any `json:"raw"`
}

type GarminBridgeGearResponse struct {
	UserProfilePK string             `json:"userProfilePk"`
	Gear          []GarminBridgeGear `json:"gear"`
	RawDefaults   any                `json:"rawDefaults"`
}

type GarminBridgeGear struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	GearType             string         `json:"gearType"`
	Brand                string         `json:"brand"`
	Model                string         `json:"model"`
	Retired              bool           `json:"retired"`
	TotalDistanceM       *float64       `json:"totalDistanceM"`
	MaxDistanceM         *float64       `json:"maxDistanceM"`
	FirstUsedAt          *time.Time     `json:"firstUsedAt"`
	LastUsedAt           *time.Time     `json:"lastUsedAt"`
	DefaultActivityTypes []string       `json:"defaultActivityTypes"`
	Raw                  map[string]any `json:"raw"`
	StatsRaw             map[string]any `json:"statsRaw"`
}

type GarminBridgeGearActivity struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	StartTime *time.Time     `json:"startTime"`
	Raw       map[string]any `json:"raw"`
}

type GarminBridgeWorkout struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Raw         map[string]any `json:"raw"`
}

type GarminBridgeCourse struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	URL         string         `json:"url"`
	Raw         map[string]any `json:"raw"`
}

type GarminBridgeScheduledWorkout struct {
	ID        string         `json:"id"`
	WorkoutID string         `json:"workoutId"`
	Date      string         `json:"date"`
	Raw       map[string]any `json:"raw"`
}

type GarminSyncOptions struct {
	Oldest  time.Time
	AllData bool
}

type GarminSyncProgress func(map[string]any)

func NewGarminService(cfg Config, store *Store) *GarminService {
	return &GarminService{
		store:    store,
		bridge:   PythonGarminBridge{Python: cfg.GarminBridgePython, Script: cfg.GarminBridgeScript},
		tokenDir: cfg.GarminTokenDir,
	}
}

func (s *GarminService) tokenStore(ctx context.Context) string {
	userID := scopedUserID(ctx)
	if userID == "" {
		return s.tokenDir
	}
	scoped := filepath.Join(s.tokenDir, userID)
	if userID == s.legacyUserID {
		if _, err := os.Stat(scoped); os.IsNotExist(err) {
			if _, legacyErr := os.Stat(s.tokenDir); legacyErr == nil {
				return s.tokenDir
			}
		}
	}
	return scoped
}

func (s *GarminService) Status(ctx context.Context) (ProviderConnection, bool, error) {
	conn, err := s.store.GetProviderConnection(ctx, garminProvider)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderConnection{Provider: garminProvider}, false, nil
	}
	if err != nil {
		return ProviderConnection{}, false, err
	}
	return conn.ProviderConnection, true, nil
}

func (s *GarminService) Connect(ctx context.Context, email, password, mfaCode string) (ProviderConnection, error) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return ProviderConnection{}, errors.New("Garmin email and password are required")
	}
	tokenStore := s.tokenStore(ctx)
	if err := os.MkdirAll(tokenStore, 0o700); err != nil {
		return ProviderConnection{}, fmt.Errorf("could not prepare Garmin token storage: %w", err)
	}
	profile, err := s.bridge.Connect(ctx, tokenStore, email, password, strings.TrimSpace(mfaCode))
	if err != nil {
		return ProviderConnection{}, err
	}
	displayName := strings.TrimSpace(profile.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(profile.FullName)
	}
	if displayName == "" {
		displayName = "Garmin Connect"
	}
	accountID := strings.TrimSpace(profile.AccountID)
	if accountID == "" {
		accountID = displayName
	}
	if err := s.store.UpsertProviderConnection(ctx, StoredProviderConnection{
		ProviderConnection: ProviderConnection{
			Provider:          garminProvider,
			ProviderAccountID: accountID,
			DisplayName:       displayName,
			Scopes:            []string{"garmin-connect"},
		},
	}); err != nil {
		return ProviderConnection{}, err
	}
	conn, _, err := s.Status(ctx)
	return conn, err
}

func (s *GarminService) Sync(ctx context.Context, opts GarminSyncOptions, progress GarminSyncProgress) (map[string]any, error) {
	if progress == nil {
		progress = func(map[string]any) {}
	}
	if _, connected, err := s.Status(ctx); err != nil {
		return nil, err
	} else if !connected {
		return nil, errors.New("Garmin is not connected")
	}
	tokenStore := s.tokenStore(ctx)
	if err := os.MkdirAll(tokenStore, 0o700); err != nil {
		return nil, fmt.Errorf("could not prepare Garmin token storage: %w", err)
	}

	oldest := garminSyncOldest(opts, time.Now().UTC())
	report := func(payload map[string]any) {
		payload["allData"] = opts.AllData
		progress(payload)
	}

	report(map[string]any{"provider": garminProvider, "stage": "Listing Garmin activities", "activities": 0, "processed": 0, "imported": 0, "failed": 0, "oldest": oldest.Format("2006-01-02")})
	activities, err := s.listActivitiesSince(ctx, oldest, report)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	weatherConfig, err := s.store.GetWeatherConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load weather settings: %w", err)
	}

	imported := 0
	failed := 0
	weatherFallbackImported := 0
	weatherFallbackFailed := 0
	weatherFallbackRateLimited := 0
	weatherFallbackNoLocation := 0
	weatherFallbackErrors := make([]string, 0, 5)
	skippedExcluded := 0
	autoMatches := make([]map[string]string, 0)
	firstErrors := make([]string, 0, 5)
	for index, source := range activities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		processed := index + 1
		payload := map[string]any{
			"provider":            garminProvider,
			"stage":               "Importing Garmin activities",
			"activities":          len(activities),
			"processed":           index,
			"imported":            imported,
			"failed":              failed,
			"skippedExcluded":     skippedExcluded,
			"currentActivityName": source.Name,
			"oldest":              oldest.Format("2006-01-02"),
		}
		report(payload)

		excluded, err := s.store.IsActivitySyncExcluded(ctx, garminProvider, source.ID)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		if excluded {
			skippedExcluded++
			report(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "oldest": oldest.Format("2006-01-02")})
			continue
		}

		data, err := s.bridge.DownloadActivity(ctx, tokenStore, source.ID)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			failed++
			firstErrors = appendGarminSyncError(firstErrors, source, err)
			report(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "firstErrors": firstErrors, "oldest": oldest.Format("2006-01-02")})
			continue
		}
		importedActivity, err := parseGarminActivityDownload(ctx, source.ID, data)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			failed++
			firstErrors = appendGarminSyncError(firstErrors, source, err)
			report(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "firstErrors": firstErrors, "oldest": oldest.Format("2006-01-02")})
			continue
		}
		applyGarminMetadata(&importedActivity, source)
		if workout, err := s.bridge.GetActivityWorkout(ctx, tokenStore, source.ID); err == nil {
			applyGarminWorkoutMetadata(&importedActivity, workout)
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !activityWeatherHasDisplayData(importedActivity.Weather) {
			latitude, longitude, hasLocation := activityWeatherLocation(importedActivity)
			importedActivity.Weather = nil
			if weatherConfig.OpenMeteoFallbackEnabled {
				if !hasLocation {
					weatherFallbackNoLocation++
				} else if s.weatherFallback != nil {
					fallback, fallbackErr := s.weatherFallback.Fetch(ctx, importedActivity.StartTime, latitude, longitude)
					switch {
					case fallbackErr == nil && fallback != nil:
						importedActivity.Weather = fallback
						weatherFallbackImported++
					case errors.Is(fallbackErr, ErrOpenMeteoRateLimited):
						weatherFallbackRateLimited++
						weatherFallbackErrors = appendWeatherFallbackError(weatherFallbackErrors, source, fallbackErr)
					case fallbackErr != nil:
						weatherFallbackFailed++
						weatherFallbackErrors = appendWeatherFallbackError(weatherFallbackErrors, source, fallbackErr)
					}
				}
			}
		}
		activityID, err := s.store.SaveImportedActivity(ctx, garminProvider, source.ID, nil, importedActivity)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if errors.Is(err, ErrActivitySyncExcluded) {
				skippedExcluded++
				report(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "oldest": oldest.Format("2006-01-02")})
				continue
			}
			failed++
			firstErrors = appendGarminSyncError(firstErrors, source, err)
			report(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "firstErrors": firstErrors, "oldest": oldest.Format("2006-01-02")})
			continue
		}
		if importedActivity.Workout != nil && importedActivity.Workout.ProviderWorkoutID != "" {
			planned, matched, matchErr := s.store.AutoMatchManagedWorkout(ctx, activityID, importedActivity.Workout.ProviderWorkoutID)
			if matchErr != nil {
				firstErrors = appendGarminSyncError(firstErrors, source, fmt.Errorf("auto-match workout: %w", matchErr))
			} else if matched {
				s.store.publishActivityAutoMatchNotification(ctx, activityID, planned)
				autoMatches = append(autoMatches, map[string]string{
					"activityId":        activityID,
					"plannedActivityId": planned.ID,
					"source":            planned.Source,
				})
			}
		}
		imported++
		report(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "firstErrors": firstErrors, "oldest": oldest.Format("2006-01-02")})
	}

	return map[string]any{
		"provider":                   garminProvider,
		"stage":                      "Completed",
		"activities":                 len(activities),
		"processed":                  len(activities),
		"imported":                   imported,
		"failed":                     failed,
		"skippedExcluded":            skippedExcluded,
		"firstErrors":                firstErrors,
		"oldest":                     oldest.Format("2006-01-02"),
		"allData":                    opts.AllData,
		"autoMatches":                autoMatches,
		"weatherFallbackImported":    weatherFallbackImported,
		"weatherFallbackFailed":      weatherFallbackFailed,
		"weatherFallbackRateLimited": weatherFallbackRateLimited,
		"weatherFallbackNoLocation":  weatherFallbackNoLocation,
		"weatherFallbackErrors":      weatherFallbackErrors,
	}, nil
}

func garminSyncOldest(opts GarminSyncOptions, now time.Time) time.Time {
	if opts.AllData {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if !opts.Oldest.IsZero() {
		return opts.Oldest
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *GarminService) SyncGear(ctx context.Context, progress GarminSyncProgress) (map[string]any, error) {
	if progress == nil {
		progress = func(map[string]any) {}
	}
	if _, connected, err := s.Status(ctx); err != nil {
		return nil, err
	} else if !connected {
		return nil, errors.New("Garmin is not connected")
	}
	tokenStore := s.tokenStore(ctx)
	if err := os.MkdirAll(tokenStore, 0o700); err != nil {
		return nil, fmt.Errorf("could not prepare Garmin token storage: %w", err)
	}

	progress(map[string]any{"provider": garminProvider, "stage": "Listing Garmin gear", "gear": 0, "processed": 0, "saved": 0, "assignments": 0, "localAssignments": 0})
	response, err := s.bridge.ListGear(ctx, tokenStore)
	if err != nil {
		return nil, err
	}

	saved := 0
	assignments := 0
	localAssignments := 0
	warnings := make([]string, 0)
	totalGear := len(response.Gear)
	for index, source := range response.Gear {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		processed := index + 1
		progress(map[string]any{"provider": garminProvider, "stage": "Importing Garmin gear", "gear": totalGear, "processed": index, "saved": saved, "assignments": assignments, "localAssignments": localAssignments, "currentGearName": source.Name, "warnings": warnings})

		providerGearID := strings.TrimSpace(source.ID)
		if providerGearID == "" {
			warnings = append(warnings, "Skipped Garmin gear without an ID")
			progress(map[string]any{"provider": garminProvider, "stage": "Importing Garmin gear", "gear": totalGear, "processed": processed, "saved": saved, "assignments": assignments, "localAssignments": localAssignments, "currentGearName": source.Name, "warnings": warnings})
			continue
		}

		gear, err := s.store.UpsertGear(ctx, Gear{
			Provider:             garminProvider,
			ProviderGearID:       providerGearID,
			Name:                 strings.TrimSpace(source.Name),
			GearType:             strings.TrimSpace(source.GearType),
			Brand:                strings.TrimSpace(source.Brand),
			Model:                strings.TrimSpace(source.Model),
			Retired:              source.Retired,
			TotalDistanceM:       source.TotalDistanceM,
			MaxDistanceM:         source.MaxDistanceM,
			FirstUsedAt:          source.FirstUsedAt,
			LastUsedAt:           source.LastUsedAt,
			DefaultActivityTypes: compactStrings(source.DefaultActivityTypes),
			Raw:                  source.Raw,
			StatsRaw:             source.StatsRaw,
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		saved++

		sourceActivityIDs, fetched, err := s.gearActivitySourceIDs(ctx, providerGearID)
		assignments += fetched
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			warnings = appendGarminGearSyncWarning(warnings, source.Name, err)
		} else {
			assigned, err := s.store.ReplaceGearAssignmentsForGear(ctx, gear.ID, garminProvider, sourceActivityIDs)
			if err != nil {
				return nil, err
			}
			localAssignments += assigned
		}

		progress(map[string]any{"provider": garminProvider, "stage": "Importing Garmin gear", "gear": totalGear, "processed": processed, "saved": saved, "assignments": assignments, "localAssignments": localAssignments, "currentGearName": source.Name, "warnings": warnings})
	}

	return map[string]any{
		"provider":         garminProvider,
		"stage":            "Completed",
		"gear":             totalGear,
		"processed":        totalGear,
		"saved":            saved,
		"assignments":      assignments,
		"localAssignments": localAssignments,
		"warnings":         warnings,
	}, nil
}

func (s *GarminService) gearActivitySourceIDs(ctx context.Context, gearID string) ([]string, int, error) {
	sourceIDs := make([]string, 0)
	fetched := 0
	for start := 0; ; {
		if err := ctx.Err(); err != nil {
			return sourceIDs, fetched, err
		}
		page, err := s.bridge.ListGearActivities(ctx, s.tokenStore(ctx), gearID, start, garminGearActivityPageLimit)
		if err != nil {
			return sourceIDs, fetched, err
		}
		if len(page) == 0 {
			break
		}
		fetched += len(page)
		if fetched > maxGarminGearActivities {
			return sourceIDs, fetched, fmt.Errorf("Garmin gear activity history is limited to %d records per gear", maxGarminGearActivities)
		}
		for _, activity := range page {
			if strings.TrimSpace(activity.ID) != "" {
				sourceIDs = append(sourceIDs, activity.ID)
			}
		}
		if len(page) < garminGearActivityPageLimit {
			break
		}
		start += len(page)
	}
	return compactStrings(sourceIDs), fetched, nil
}

func appendGarminGearSyncWarning(warnings []string, gearName string, err error) []string {
	if len(warnings) >= 5 {
		return warnings
	}
	gearName = strings.TrimSpace(gearName)
	if gearName == "" {
		gearName = "gear"
	}
	return append(warnings, gearName+": "+err.Error())
}

func (s *GarminService) listActivitiesSince(ctx context.Context, oldest time.Time, progress GarminSyncProgress) ([]GarminBridgeActivity, error) {
	out := make([]GarminBridgeActivity, 0)
	tokenStore := s.tokenStore(ctx)
	for start := 0; ; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := s.bridge.ListActivities(ctx, tokenStore, start, garminActivityPageLimit)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		reachedOldest := false
		for _, activity := range page {
			if strings.TrimSpace(activity.ID) == "" {
				continue
			}
			if !activity.StartTime.IsZero() && activity.StartTime.Before(oldest) {
				reachedOldest = true
				continue
			}
			out = append(out, activity)
			if len(out) >= maxGarminSyncActivities {
				return nil, fmt.Errorf("Garmin sync is limited to %d activities per run", maxGarminSyncActivities)
			}
		}
		progress(map[string]any{"provider": garminProvider, "stage": "Listing Garmin activities", "activities": len(out), "processed": len(out), "fetchedPages": (start / garminActivityPageLimit) + 1})
		if reachedOldest || len(page) < garminActivityPageLimit {
			break
		}
		start += len(page)
	}
	return out, nil
}

func appendGarminSyncError(firstErrors []string, source GarminBridgeActivity, err error) []string {
	if len(firstErrors) >= 5 {
		return firstErrors
	}
	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = source.ID
	}
	return append(firstErrors, name+": "+err.Error())
}

func parseGarminActivityDownload(ctx context.Context, sourceID string, data []byte) (ImportedActivity, error) {
	if len(data) > maxGarminActivityBytes {
		return ImportedActivity{}, errors.New("Garmin activity download is too large")
	}
	filename := sourceID + ".fit"
	if zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err == nil {
		foundActivityFile := false
		for _, file := range zipReader.File {
			ext := strings.ToLower(filepath.Ext(file.Name))
			if ext != ".fit" && ext != ".tcx" && ext != ".gpx" {
				continue
			}
			reader, err := file.Open()
			if err != nil {
				return ImportedActivity{}, err
			}
			fileData, readErr := io.ReadAll(io.LimitReader(reader, maxGarminActivityBytes+1))
			closeErr := reader.Close()
			if readErr != nil {
				return ImportedActivity{}, readErr
			}
			if len(fileData) > maxGarminActivityBytes {
				return ImportedActivity{}, errors.New("Garmin activity file in archive is too large")
			}
			if closeErr != nil {
				return ImportedActivity{}, closeErr
			}
			filename = filepath.Base(file.Name)
			data = fileData
			foundActivityFile = true
			break
		}
		if !foundActivityFile {
			return ImportedActivity{}, errors.New("Garmin archive did not contain a supported FIT, TCX, or GPX activity file")
		}
	}

	var parser ActivityParser
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".fit", "":
		parser = FITParser{}
	case ".tcx":
		parser = TCXParser{}
	case ".gpx":
		parser = GPXParser{}
	default:
		return ImportedActivity{}, fmt.Errorf("unsupported Garmin activity file %q", filename)
	}
	activity, err := parser.Parse(ctx, filename, data)
	if err != nil {
		return ImportedActivity{}, err
	}
	normalizeImported(&activity)
	return activity, nil
}

func applyGarminMetadata(activity *ImportedActivity, source GarminBridgeActivity) {
	if strings.TrimSpace(source.Name) != "" {
		activity.Name = strings.TrimSpace(source.Name)
	}
	if strings.TrimSpace(source.SportType) != "" {
		activity.SportType = normalizeSport(source.SportType)
	}
	if activity.Raw == nil {
		activity.Raw = map[string]any{}
	}
	activity.Raw["provider"] = garminProvider
	activity.Raw["garmin_id"] = source.ID
	activity.OriginalProviderURL = garminActivityURL(source.ID)
	if !source.StartTime.IsZero() {
		activity.Raw["garmin_start_time"] = source.StartTime.Format(time.RFC3339)
	}
	if gap := gradeAdjustedPaceFromSpeedMPS(source.AvgGradeAdjustedSpeedMPS); gap != nil {
		activity.AvgGradeAdjustedPaceSPKM = gap
		activity.Raw["garmin_avg_grade_adjusted_speed_mps"] = *source.AvgGradeAdjustedSpeedMPS
	}
}

func applyGarminWorkoutMetadata(activity *ImportedActivity, source GarminBridgeActivityWorkout) {
	if source.Weather != nil {
		activity.Weather = activityWeatherFromGarmin(*source.Weather)
	}
	if source.Raw != nil {
		if activity.Raw == nil {
			activity.Raw = map[string]any{}
		}
		activity.Raw["garmin_workout"] = source.Raw
	}
	if !source.Available {
		return
	}
	activity.ReplaceWorkoutMetadata = true
	activity.Workout = source.Workout
	activity.Intervals = append([]ActivityInterval(nil), source.Intervals...)
	applyGarminLapMetadata(&activity.Laps, source.Laps)
	for _, interval := range source.Intervals {
		for _, lapIndex := range interval.LapIndexes {
			if lapIndex < 0 || lapIndex >= len(activity.Laps) {
				continue
			}
			lap := &activity.Laps[lapIndex]
			if lap.IntensityType == "" {
				lap.IntensityType = strings.ToUpper(interval.Category)
			}
			lap.WorkoutStepIndex = interval.WorkoutStepIndex
			lap.WorkoutRepeatIndex = interval.WorkoutRepeatIndex
		}
	}
}

func activityWeatherFromGarmin(source GarminBridgeWeather) *ActivityWeather {
	return &ActivityWeather{
		Provider:             garminProvider,
		ObservedAt:           source.ObservedAt,
		Condition:            strings.TrimSpace(source.Condition),
		TemperatureC:         fahrenheitToCelsius(source.TemperatureF),
		ApparentTemperatureC: fahrenheitToCelsius(source.ApparentTemperatureF),
		DewPointC:            fahrenheitToCelsius(source.DewPointF),
		RelativeHumidityPct:  source.RelativeHumidityPct,
		WindSpeedKPH:         milesPerHourToKPH(source.WindSpeedMPH),
		WindGustKPH:          milesPerHourToKPH(source.WindGustMPH),
		WindDirectionDeg:     source.WindDirectionDeg,
		WindDirection:        strings.ToUpper(strings.TrimSpace(source.WindDirection)),
		Latitude:             source.Latitude,
		Longitude:            source.Longitude,
		StationID:            strings.TrimSpace(source.StationID),
		StationName:          strings.TrimSpace(source.StationName),
		StationTimezone:      strings.TrimSpace(source.StationTimezone),
		Raw:                  source.Raw,
	}
}

func activityWeatherHasDisplayData(weather *ActivityWeather) bool {
	return weather != nil && (strings.TrimSpace(weather.Condition) != "" ||
		weather.TemperatureC != nil || weather.ApparentTemperatureC != nil ||
		weather.RelativeHumidityPct != nil || weather.WindSpeedKPH != nil ||
		strings.TrimSpace(weather.WindDirection) != "")
}

func activityWeatherLocation(activity ImportedActivity) (float64, float64, bool) {
	if activity.Weather != nil && activity.Weather.Latitude != nil && activity.Weather.Longitude != nil &&
		validWeatherCoordinate(*activity.Weather.Latitude, *activity.Weather.Longitude) {
		return *activity.Weather.Latitude, *activity.Weather.Longitude, true
	}
	for _, sample := range activity.Samples {
		if sample.Latitude != nil && sample.Longitude != nil && validWeatherCoordinate(*sample.Latitude, *sample.Longitude) {
			return *sample.Latitude, *sample.Longitude, true
		}
	}
	return 0, 0, false
}

func appendWeatherFallbackError(current []string, activity GarminBridgeActivity, err error) []string {
	if err == nil {
		return current
	}
	return appendGarminSyncError(current, activity, err)
}

func fahrenheitToCelsius(value *float64) *float64 {
	if value == nil {
		return nil
	}
	converted := (*value - 32) * 5 / 9
	return &converted
}

func milesPerHourToKPH(value *float64) *float64 {
	if value == nil {
		return nil
	}
	converted := *value * 1.609344
	return &converted
}

func applyGarminLapMetadata(activityLaps *[]ActivityLap, sourceLaps []GarminBridgeLap) {
	if activityLaps == nil || len(*activityLaps) == 0 || len(sourceLaps) == 0 {
		return
	}
	for _, sourceLap := range sourceLaps {
		if sourceLap.Index < 0 || sourceLap.Index >= len(*activityLaps) {
			continue
		}
		lap := &(*activityLaps)[sourceLap.Index]
		if sourceLap.StartTime != nil {
			lap.StartTime = sourceLap.StartTime
		}
		if sourceLap.ElapsedTimeS > 0 {
			lap.ElapsedTimeS = sourceLap.ElapsedTimeS
		}
		if sourceLap.MovingTimeS > 0 {
			lap.MovingTimeS = sourceLap.MovingTimeS
		}
		if sourceLap.DistanceM > 0 {
			lap.DistanceM = sourceLap.DistanceM
		}
		if sourceLap.AvgPaceSPKM != nil {
			lap.AvgPaceSPKM = sourceLap.AvgPaceSPKM
		}
		if sourceLap.AvgGradeAdjustedPaceSPKM != nil {
			lap.AvgGradeAdjustedPaceSPKM = sourceLap.AvgGradeAdjustedPaceSPKM
		} else if gap := gradeAdjustedPaceFromSpeedMPS(sourceLap.AvgGradeAdjustedSpeedMPS); gap != nil {
			lap.AvgGradeAdjustedPaceSPKM = gap
		}
		if sourceLap.ElevationGainM != nil {
			lap.ElevationGainM = sourceLap.ElevationGainM
		}
		if sourceLap.ElevationLossM != nil {
			lap.ElevationLossM = sourceLap.ElevationLossM
		}
		if sourceLap.AvgHeartRate != nil {
			lap.AvgHeartRate = sourceLap.AvgHeartRate
		}
		if sourceLap.MaxHeartRate != nil {
			lap.MaxHeartRate = sourceLap.MaxHeartRate
		}
		if sourceLap.AvgPower != nil {
			lap.AvgPower = sourceLap.AvgPower
		}
		if sourceLap.MaxPower != nil {
			lap.MaxPower = sourceLap.MaxPower
		}
		if sourceLap.NormalizedPower != nil {
			lap.NormalizedPower = sourceLap.NormalizedPower
		}
		if sourceLap.AvgRunCadence != nil {
			lap.AvgRunCadence = sourceLap.AvgRunCadence
		}
		if sourceLap.AvgGroundContactTimeMS != nil {
			lap.AvgGroundContactTimeMS = sourceLap.AvgGroundContactTimeMS
		}
		if sourceLap.AvgRespirationRate != nil {
			lap.AvgRespirationRate = sourceLap.AvgRespirationRate
		}
		if sourceLap.AvgTemperatureC != nil {
			lap.AvgTemperatureC = sourceLap.AvgTemperatureC
		}
		if sourceLap.IntensityType != "" {
			lap.IntensityType = sourceLap.IntensityType
		}
		if sourceLap.WorkoutStepIndex != nil {
			lap.WorkoutStepIndex = sourceLap.WorkoutStepIndex
		}
		if sourceLap.Raw != nil {
			lap.Raw = sourceLap.Raw
		}
	}
}

func gradeAdjustedPaceFromSpeedMPS(speed *float64) *float64 {
	if speed == nil || *speed <= 0 {
		return nil
	}
	value := 1000 / *speed
	return &value
}

func garminActivityURL(activityID string) string {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return ""
	}
	return "https://connect.garmin.com/modern/activity/" + activityID
}

type PythonGarminBridge struct {
	Python string
	Script string
}

func (b PythonGarminBridge) Connect(ctx context.Context, tokenStore, email, password, mfaCode string) (GarminBridgeProfile, error) {
	var response GarminBridgeProfile
	err := b.run(ctx, map[string]any{
		"action":     "connect",
		"tokenStore": tokenStore,
		"email":      email,
		"password":   password,
		"mfaCode":    mfaCode,
	}, &response)
	return response, err
}

func (b PythonGarminBridge) ListActivities(ctx context.Context, tokenStore string, start, limit int) ([]GarminBridgeActivity, error) {
	var response struct {
		Activities []GarminBridgeActivity `json:"activities"`
	}
	err := b.run(ctx, map[string]any{
		"action":     "list",
		"tokenStore": tokenStore,
		"start":      start,
		"limit":      limit,
	}, &response)
	return response.Activities, err
}

func (b PythonGarminBridge) ListActivitySplits(ctx context.Context, tokenStore, activityID string) ([]GarminBridgeLap, error) {
	var response struct {
		Laps []GarminBridgeLap `json:"laps"`
	}
	err := b.run(ctx, map[string]any{
		"action":     "splits",
		"tokenStore": tokenStore,
		"activityId": activityID,
	}, &response)
	return response.Laps, err
}

func (b PythonGarminBridge) GetActivityWorkout(ctx context.Context, tokenStore, activityID string) (GarminBridgeActivityWorkout, error) {
	var response GarminBridgeActivityWorkout
	err := b.run(ctx, map[string]any{
		"action":     "activity-workout",
		"tokenStore": tokenStore,
		"activityId": activityID,
	}, &response)
	return response, err
}

func (b PythonGarminBridge) DownloadActivity(ctx context.Context, tokenStore, activityID string) ([]byte, error) {
	var response struct {
		ContentBase64 string `json:"contentBase64"`
	}
	if err := b.run(ctx, map[string]any{
		"action":     "download",
		"tokenStore": tokenStore,
		"activityId": activityID,
	}, &response); err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(response.ContentBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid Garmin bridge download response: %w", err)
	}
	if len(data) > maxGarminActivityBytes {
		return nil, errors.New("Garmin activity download is too large")
	}
	return data, nil
}

func (b PythonGarminBridge) FetchHealthDay(ctx context.Context, tokenStore, date string) (GarminBridgeHealthDay, error) {
	var response map[string]any
	if err := b.run(ctx, map[string]any{
		"action":     "health-day",
		"tokenStore": tokenStore,
		"date":       date,
	}, &response); err != nil {
		return GarminBridgeHealthDay{}, err
	}
	responseDate, _ := response["date"].(string)
	return GarminBridgeHealthDay{
		Date: responseDate,
		Raw:  response,
	}, nil
}

func (b PythonGarminBridge) ListGear(ctx context.Context, tokenStore string) (GarminBridgeGearResponse, error) {
	var response GarminBridgeGearResponse
	err := b.run(ctx, map[string]any{
		"action":     "gear",
		"tokenStore": tokenStore,
	}, &response)
	return response, err
}

func (b PythonGarminBridge) ListGearActivities(ctx context.Context, tokenStore, gearID string, start, limit int) ([]GarminBridgeGearActivity, error) {
	var response struct {
		Activities []GarminBridgeGearActivity `json:"activities"`
	}
	err := b.run(ctx, map[string]any{
		"action":     "gear-activities",
		"tokenStore": tokenStore,
		"gearId":     gearID,
		"start":      start,
		"limit":      limit,
	}, &response)
	return response.Activities, err
}

func (b PythonGarminBridge) ListWorkouts(ctx context.Context, tokenStore string, start, limit int) ([]GarminBridgeWorkout, error) {
	var response struct {
		Workouts []GarminBridgeWorkout `json:"workouts"`
	}
	err := b.run(ctx, map[string]any{
		"action":     "workouts",
		"tokenStore": tokenStore,
		"start":      start,
		"limit":      limit,
	}, &response)
	return response.Workouts, err
}

func (b PythonGarminBridge) GetWorkout(ctx context.Context, tokenStore, workoutID string) (GarminBridgeWorkout, error) {
	var response GarminBridgeWorkout
	err := b.run(ctx, map[string]any{
		"action":     "workout",
		"tokenStore": tokenStore,
		"workoutId":  workoutID,
	}, &response)
	return response, err
}

func (b PythonGarminBridge) UploadWorkout(ctx context.Context, tokenStore string, payload map[string]any) (GarminBridgeWorkout, error) {
	var response GarminBridgeWorkout
	err := b.run(ctx, map[string]any{
		"action":     "upload-workout",
		"tokenStore": tokenStore,
		"workout":    payload,
	}, &response)
	return response, err
}

func (b PythonGarminBridge) UploadCourse(ctx context.Context, tokenStore, filename string, content []byte, name string, sport CourseSport, description string) (GarminBridgeCourse, error) {
	var response GarminBridgeCourse
	err := b.run(ctx, map[string]any{
		"action":        "upload-course",
		"tokenStore":    tokenStore,
		"filename":      filename,
		"contentBase64": base64.StdEncoding.EncodeToString(content),
		"name":          name,
		"sport":         sport,
		"description":   description,
	}, &response)
	return response, err
}

func (b PythonGarminBridge) GetCourse(ctx context.Context, tokenStore, courseID string) (GarminBridgeCourse, error) {
	var response GarminBridgeCourse
	err := b.run(ctx, map[string]any{
		"action":     "course",
		"tokenStore": tokenStore,
		"courseId":   courseID,
	}, &response)
	return response, err
}

func (b PythonGarminBridge) DeleteWorkout(ctx context.Context, tokenStore, workoutID string) error {
	var response struct {
		OK bool `json:"ok"`
	}
	err := b.run(ctx, map[string]any{
		"action":     "delete-workout",
		"tokenStore": tokenStore,
		"workoutId":  workoutID,
	}, &response)
	if errors.Is(err, ErrGarminNotFound) {
		return nil
	}
	return err
}

func (b PythonGarminBridge) ListScheduledWorkouts(ctx context.Context, tokenStore string, year, month int) ([]GarminBridgeScheduledWorkout, error) {
	var response struct {
		Scheduled []GarminBridgeScheduledWorkout `json:"scheduled"`
	}
	err := b.run(ctx, map[string]any{
		"action":     "scheduled-workouts",
		"tokenStore": tokenStore,
		"year":       year,
		"month":      month,
	}, &response)
	for index := range response.Scheduled {
		response.Scheduled[index] = normalizeGarminScheduledWorkout(response.Scheduled[index])
	}
	return response.Scheduled, err
}

func (b PythonGarminBridge) GetScheduledWorkout(ctx context.Context, tokenStore, scheduledWorkoutID string) (GarminBridgeScheduledWorkout, error) {
	var response GarminBridgeScheduledWorkout
	err := b.run(ctx, map[string]any{
		"action":             "scheduled-workout",
		"tokenStore":         tokenStore,
		"scheduledWorkoutId": scheduledWorkoutID,
	}, &response)
	return normalizeGarminScheduledWorkout(response), err
}

func (b PythonGarminBridge) ScheduleWorkout(ctx context.Context, tokenStore, workoutID, date string) (GarminBridgeScheduledWorkout, error) {
	var response GarminBridgeScheduledWorkout
	err := b.run(ctx, map[string]any{
		"action":     "schedule-workout",
		"tokenStore": tokenStore,
		"workoutId":  workoutID,
		"date":       date,
	}, &response)
	return normalizeGarminScheduledWorkout(response), err
}

func normalizeGarminScheduledWorkout(response GarminBridgeScheduledWorkout) GarminBridgeScheduledWorkout {
	if response.ID == "" {
		response.ID = garminRawWorkoutID(response.Raw["workoutScheduleId"])
		if response.ID == "" {
			response.ID = garminRawWorkoutID(response.Raw["scheduledWorkoutId"])
		}
	}
	if response.WorkoutID == "" {
		response.WorkoutID = garminRawWorkoutID(response.Raw["workoutId"])
		if response.WorkoutID == "" {
			if workout, ok := response.Raw["workout"].(map[string]any); ok {
				response.WorkoutID = garminRawWorkoutID(workout["workoutId"])
				if response.WorkoutID == "" {
					response.WorkoutID = garminRawWorkoutID(workout["id"])
				}
			}
		}
	}
	if response.Date == "" {
		for _, key := range []string{"date", "calendarDate", "startDate"} {
			value := strings.TrimSpace(fmt.Sprint(response.Raw[key]))
			if value != "" && value != "<nil>" {
				if len(value) > 10 {
					value = value[:10]
				}
				response.Date = value
				break
			}
		}
	}
	return response
}

func (b PythonGarminBridge) UnscheduleWorkout(ctx context.Context, tokenStore, scheduledWorkoutID string) error {
	var response struct {
		OK bool `json:"ok"`
	}
	return b.run(ctx, map[string]any{
		"action":             "unschedule-workout",
		"tokenStore":         tokenStore,
		"scheduledWorkoutId": scheduledWorkoutID,
	}, &response)
}

func (b PythonGarminBridge) run(ctx context.Context, request map[string]any, response any) error {
	if strings.TrimSpace(b.Python) == "" || strings.TrimSpace(b.Script) == "" {
		return errors.New("Garmin bridge is not configured")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, b.Python, b.Script)
	cmd.Stdin = bytes.NewReader(body)
	var stdout boundedBuffer
	stdout.max = maxGarminBridgeOutputBytes
	var stderr boundedBuffer
	stderr.max = 1 << 20
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stdout.tooLarge || stderr.tooLarge {
			return ErrGarminBridgeOutputTooLarge
		}
		var bridgeErr struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		if json.Unmarshal(stdout.Bytes(), &bridgeErr) == nil && bridgeErr.Error != "" {
			return &garminBridgeError{Code: bridgeErr.Code, Message: bridgeErr.Error}
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("Garmin bridge failed: %s", message)
	}
	if err := json.Unmarshal(stdout.Bytes(), response); err != nil {
		return fmt.Errorf("invalid Garmin bridge response: %w", err)
	}
	return nil
}

type boundedBuffer struct {
	bytes.Buffer
	max      int
	tooLarge bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		b.tooLarge = true
		return 0, ErrGarminBridgeOutputTooLarge
	}
	return b.Buffer.Write(p)
}
