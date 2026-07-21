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

type GarminService struct {
	store        *Store
	bridge       GarminBridge
	tokenDir     string
	legacyUserID string
}

type GarminBridge interface {
	Connect(ctx context.Context, tokenStore, email, password, mfaCode string) (GarminBridgeProfile, error)
	ListActivities(ctx context.Context, tokenStore string, start, limit int) ([]GarminBridgeActivity, error)
	ListActivitySplits(ctx context.Context, tokenStore, activityID string) ([]GarminBridgeLap, error)
	DownloadActivity(ctx context.Context, tokenStore, activityID string) ([]byte, error)
	FetchHealthDay(ctx context.Context, tokenStore, date string) (GarminBridgeHealthDay, error)
	ListGear(ctx context.Context, tokenStore string) (GarminBridgeGearResponse, error)
	ListGearActivities(ctx context.Context, tokenStore, gearID string, start, limit int) ([]GarminBridgeGearActivity, error)
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
	Index                    int      `json:"index"`
	AvgGradeAdjustedSpeedMPS *float64 `json:"avgGradeAdjustedSpeed,omitempty"`
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

type GarminSyncOptions struct {
	Oldest time.Time
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

	oldest := opts.Oldest
	if oldest.IsZero() {
		oldest = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	progress(map[string]any{"provider": garminProvider, "stage": "Listing Garmin activities", "activities": 0, "processed": 0, "imported": 0, "failed": 0, "oldest": oldest.Format("2006-01-02")})
	activities, err := s.listActivitiesSince(ctx, oldest, progress)
	if err != nil {
		return nil, err
	}

	imported := 0
	failed := 0
	skippedExcluded := 0
	firstErrors := make([]string, 0, 5)
	for index, source := range activities {
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
		progress(payload)

		excluded, err := s.store.IsActivitySyncExcluded(ctx, garminProvider, source.ID)
		if err != nil {
			return nil, err
		}
		if excluded {
			skippedExcluded++
			progress(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "oldest": oldest.Format("2006-01-02")})
			continue
		}

		data, err := s.bridge.DownloadActivity(ctx, tokenStore, source.ID)
		if err != nil {
			failed++
			firstErrors = appendGarminSyncError(firstErrors, source, err)
			progress(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "firstErrors": firstErrors, "oldest": oldest.Format("2006-01-02")})
			continue
		}
		importedActivity, err := parseGarminActivityDownload(ctx, source.ID, data)
		if err != nil {
			failed++
			firstErrors = appendGarminSyncError(firstErrors, source, err)
			progress(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "firstErrors": firstErrors, "oldest": oldest.Format("2006-01-02")})
			continue
		}
		applyGarminMetadata(&importedActivity, source)
		if len(importedActivity.Laps) > 0 && source.AvgGradeAdjustedSpeedMPS != nil {
			if laps, err := s.bridge.ListActivitySplits(ctx, tokenStore, source.ID); err == nil {
				applyGarminLapMetadata(&importedActivity, laps)
			}
		}
		if _, err := s.store.SaveImportedActivity(ctx, garminProvider, source.ID, nil, importedActivity); err != nil {
			if errors.Is(err, ErrActivitySyncExcluded) {
				skippedExcluded++
				progress(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "oldest": oldest.Format("2006-01-02")})
				continue
			}
			failed++
			firstErrors = appendGarminSyncError(firstErrors, source, err)
			progress(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "firstErrors": firstErrors, "oldest": oldest.Format("2006-01-02")})
			continue
		}
		imported++
		progress(map[string]any{"provider": garminProvider, "stage": "Importing Garmin activities", "activities": len(activities), "processed": processed, "imported": imported, "failed": failed, "skippedExcluded": skippedExcluded, "firstErrors": firstErrors, "oldest": oldest.Format("2006-01-02")})
	}

	return map[string]any{
		"provider":        garminProvider,
		"stage":           "Completed",
		"activities":      len(activities),
		"processed":       len(activities),
		"imported":        imported,
		"failed":          failed,
		"skippedExcluded": skippedExcluded,
		"firstErrors":     firstErrors,
		"oldest":          oldest.Format("2006-01-02"),
	}, nil
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
			return nil, err
		}
		saved++

		sourceActivityIDs, fetched, err := s.gearActivitySourceIDs(ctx, providerGearID)
		assignments += fetched
		if err != nil {
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
		page, err := s.bridge.ListGearActivities(ctx, s.tokenStore(ctx), gearID, start, garminGearActivityPageLimit)
		if err != nil {
			return sourceIDs, fetched, err
		}
		if len(page) == 0 {
			break
		}
		fetched += len(page)
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
			fileData, readErr := io.ReadAll(io.LimitReader(reader, 100<<20))
			closeErr := reader.Close()
			if readErr != nil {
				return ImportedActivity{}, readErr
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

func applyGarminLapMetadata(activity *ImportedActivity, sourceLaps []GarminBridgeLap) {
	if len(activity.Laps) == 0 || len(sourceLaps) == 0 {
		return
	}

	gapByIndex := make(map[int]*float64, len(sourceLaps))
	for _, sourceLap := range sourceLaps {
		if sourceLap.Index < 0 {
			continue
		}
		if pace := gradeAdjustedPaceFromSpeedMPS(sourceLap.AvgGradeAdjustedSpeedMPS); pace != nil {
			gapByIndex[sourceLap.Index] = pace
		}
	}

	for index := range activity.Laps {
		if pace, ok := gapByIndex[activity.Laps[index].Index]; ok {
			activity.Laps[index].AvgGradeAdjustedPaceSPKM = pace
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
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var bridgeErr struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		if json.Unmarshal(stdout.Bytes(), &bridgeErr) == nil && bridgeErr.Error != "" {
			return errors.New(bridgeErr.Error)
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
