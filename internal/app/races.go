package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrRaceInvalid = errors.New("invalid race")
var ErrRaceConflict = errors.New("race changed since it was loaded; reload before saving")

type RaceResult struct {
	Confirmed       bool             `json:"confirmed"`
	ChipTimeMS      *int64           `json:"chipTimeMs,omitempty"`
	GunTimeMS       *int64           `json:"gunTimeMs,omitempty"`
	ManualTimeMS    *int64           `json:"manualTimeMs,omitempty"`
	OverallPlace    *int             `json:"overallPlace,omitempty"`
	OverallTotal    *int             `json:"overallTotal,omitempty"`
	Category        string           `json:"category,omitempty"`
	CategoryPlace   *int             `json:"categoryPlace,omitempty"`
	CategoryTotal   *int             `json:"categoryTotal,omitempty"`
	Excluded        bool             `json:"excluded"`
	ExclusionReason string           `json:"exclusionReason,omitempty"`
	SplitBasis      string           `json:"splitBasis,omitempty"`
	Checkpoints     []RaceCheckpoint `json:"checkpoints"`
}
type RaceCheckpoint struct {
	Name      string  `json:"name"`
	DistanceM float64 `json:"distanceM"`
	TimeMS    int64   `json:"timeMs"`
}
type RaceGoal struct {
	Name     string `json:"name"`
	TimeMS   *int64 `json:"timeMs,omitempty"`
	Achieved *bool  `json:"achieved,omitempty"`
}
type RaceChecklistItem struct {
	Label      string `json:"label"`
	Done       bool   `json:"done"`
	DueDate    string `json:"dueDate,omitempty"`
	DaysBefore *int   `json:"daysBefore,omitempty"`
}
type RaceInput struct {
	Revision             int                 `json:"revision"`
	Name                 string              `json:"name"`
	Date                 string              `json:"date,omitempty"`
	StartTime            string              `json:"startTime,omitempty"`
	Timezone             string              `json:"timezone"`
	Status               string              `json:"status"`
	Discipline           string              `json:"discipline"`
	Kind                 string              `json:"kind"`
	DistanceM            *float64            `json:"distanceM,omitempty"`
	Priority             string              `json:"priority,omitempty"`
	GroupID              string              `json:"groupId,omitempty"`
	ActivityID           string              `json:"activityId,omitempty"`
	PlannedActivityID    string              `json:"plannedActivityId,omitempty"`
	CourseID             string              `json:"courseId,omitempty"`
	RefreshCourse        bool                `json:"refreshCourse,omitempty"`
	RemoveCourse         bool                `json:"removeCourse,omitempty"`
	RaceOnly             bool                `json:"raceOnly"`
	Location             string              `json:"location,omitempty"`
	Latitude             *float64            `json:"latitude,omitempty"`
	Longitude            *float64            `json:"longitude,omitempty"`
	EventURL             string              `json:"eventUrl,omitempty"`
	ResultsURL           string              `json:"resultsUrl,omitempty"`
	RegistrationDeadline string              `json:"registrationDeadline,omitempty"`
	RegistrationNotes    string              `json:"registrationNotes,omitempty"`
	Bib                  string              `json:"bib,omitempty"`
	StartDetails         string              `json:"startDetails,omitempty"`
	TravelNotes          string              `json:"travelNotes,omitempty"`
	FuelingNotes         string              `json:"fuelingNotes,omitempty"`
	PacingNotes          string              `json:"pacingNotes,omitempty"`
	KitNotes             string              `json:"kitNotes,omitempty"`
	Notes                string              `json:"notes,omitempty"`
	AgeOverride          *int                `json:"ageOverride,omitempty"`
	TableOverride        string              `json:"tableOverride,omitempty"`
	Result               RaceResult          `json:"result"`
	Goals                []RaceGoal          `json:"goals"`
	Checklist            []RaceChecklistItem `json:"checklist"`
}
type Race struct {
	RaceInput
	ID             string    `json:"id"`
	CourseSnapshot *Course   `json:"courseSnapshot,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	RecordingHash  string    `json:"-"`
}
type RaceSummary struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Date              string   `json:"date,omitempty"`
	Status            string   `json:"status"`
	ActivityID        string   `json:"activityId,omitempty"`
	PlannedActivityID string   `json:"plannedActivityId,omitempty"`
	Priority          string   `json:"priority,omitempty"`
	Timezone          string   `json:"timezone"`
	StartTime         string   `json:"startTime,omitempty"`
	DistanceM         *float64 `json:"distanceM,omitempty"`
}

func (r Race) summary() RaceSummary {
	return RaceSummary{ID: r.ID, Name: r.Name, Date: r.Date, Status: r.Status, ActivityID: r.ActivityID, PlannedActivityID: r.PlannedActivityID, Priority: r.Priority, Timezone: r.Timezone, StartTime: r.StartTime, DistanceM: r.DistanceM}
}

type RaceSettings struct {
	Revision              int        `json:"revision"`
	BirthDate             string     `json:"birthDate,omitempty"`
	GradingTable          string     `json:"gradingTable,omitempty"`
	VDOTReferenceID       string     `json:"vdotReferenceId,omitempty"`
	PredictionsEnabled    bool       `json:"predictionsEnabled"`
	WeatherEnabled        bool       `json:"weatherEnabled"`
	PredictionSyncedAt    *time.Time `json:"predictionSyncedAt,omitempty"`
	PredictionAttemptedAt *time.Time `json:"predictionAttemptedAt,omitempty"`
	PredictionBackfilled  bool       `json:"predictionBackfilled"`
	PredictionError       string     `json:"predictionError,omitempty"`
}
type RaceResource struct {
	ID       string              `json:"id"`
	Revision int                 `json:"revision"`
	Name     string              `json:"name"`
	Items    []RaceChecklistItem `json:"items,omitempty"`
}
type RacePage struct {
	Races      []Race `json:"races"`
	HasMore    bool   `json:"hasMore"`
	NextOffset int    `json:"nextOffset"`
}
type raceListOptions struct {
	View, Query, Discipline, Kind, Status, GroupID, From, To, Sort, Order string
	Limit, Offset                                                         int
}

func validRaceDate(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value && parsed.Year() >= 1900 && parsed.Year() <= 2200
}
func validateRace(input *RaceInput) error {
	bad := func(message string) error { return fmt.Errorf("%w: %s", ErrRaceInvalid, message) }
	input.Name = strings.TrimSpace(input.Name)
	if utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 160 {
		return bad("name must contain 1–160 characters")
	}
	if !validRaceDate(input.Date) || !validRaceDate(input.RegistrationDeadline) {
		return bad("invalid date")
	}
	if input.Timezone == "" {
		input.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil || input.Timezone == "Local" {
		return bad("invalid timezone")
	}
	if input.StartTime != "" {
		if input.Date == "" {
			return bad("a start time requires a date")
		}
		if t, err := time.Parse("15:04", input.StartTime); err != nil || t.Format("15:04") != input.StartTime {
			return bad("start time must use HH:MM")
		}
	}
	if !slices.Contains([]string{"wishlist", "planned", "registered", "finished", "dns", "dnf", "disqualified", "cancelled", "postponed"}, input.Status) {
		return bad("invalid race status")
	}
	if !slices.Contains([]string{"road", "track", "trail", "cross_country"}, input.Discipline) {
		return bad("invalid discipline")
	}
	if !slices.Contains([]string{"race", "parkrun", "virtual", "time_trial"}, input.Kind) {
		return bad("invalid event kind")
	}
	if !slices.Contains([]string{"", "A", "B", "C"}, input.Priority) {
		return bad("invalid priority")
	}
	if input.DistanceM != nil && (!isPositiveFloat(*input.DistanceM) || *input.DistanceM >= 10000000) {
		return bad("distance must be positive and less than 10,000 km")
	}
	if (input.Latitude == nil) != (input.Longitude == nil) {
		return bad("latitude and longitude are required together")
	}
	if input.Latitude != nil && (math.IsNaN(*input.Latitude) || math.IsInf(*input.Latitude, 0) || math.Abs(*input.Latitude) > 90 || math.IsNaN(*input.Longitude) || math.IsInf(*input.Longitude, 0) || math.Abs(*input.Longitude) > 180) {
		return bad("invalid location")
	}
	for _, link := range []string{input.EventURL, input.ResultsURL} {
		if link != "" {
			parsed, err := url.Parse(link)
			if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || len(link) > 2048 {
				return bad("links must be HTTP(S) URLs")
			}
		}
	}
	for _, value := range []string{input.Location, input.Bib, input.StartDetails, input.RegistrationNotes, input.TravelNotes, input.FuelingNotes, input.PacingNotes, input.KitNotes, input.Notes, input.Result.Category, input.Result.ExclusionReason} {
		if utf8.RuneCountInString(value) > 5000 {
			return bad("notes must contain at most 5,000 characters")
		}
	}
	if input.AgeOverride != nil && (*input.AgeOverride < 0 || *input.AgeOverride > 130) {
		return bad("invalid age")
	}
	if !slices.Contains([]string{"", "M", "F"}, input.TableOverride) {
		return bad("invalid reference table")
	}
	for _, ms := range []*int64{input.Result.ChipTimeMS, input.Result.GunTimeMS, input.Result.ManualTimeMS} {
		if ms != nil && (*ms <= 0 || *ms > 365*24*60*60*1000) {
			return bad("finish times must be positive and less than a year")
		}
	}
	if input.Result.ChipTimeMS != nil && input.Result.GunTimeMS != nil && *input.Result.ChipTimeMS > *input.Result.GunTimeMS {
		return bad("chip time cannot exceed gun time")
	}
	for _, position := range []*int{input.Result.OverallPlace, input.Result.OverallTotal, input.Result.CategoryPlace, input.Result.CategoryTotal} {
		if position != nil && (*position < 1 || *position > 10000000) {
			return bad("positions and field sizes must be positive")
		}
	}
	for _, pair := range [][2]*int{{input.Result.OverallPlace, input.Result.OverallTotal}, {input.Result.CategoryPlace, input.Result.CategoryTotal}} {
		if pair[0] != nil && pair[1] != nil && *pair[0] > *pair[1] {
			return bad("placing exceeds field size")
		}
	}
	if !slices.Contains([]string{"", "chip", "gun", "manual"}, input.Result.SplitBasis) {
		return bad("invalid checkpoint timing basis")
	}
	if input.StartTime != "" {
		t, err := raceCutoff(Race{RaceInput: *input})
		if err != nil || t.Format("2006-01-02 15:04") != input.Date+" "+input.StartTime {
			return bad("start time does not exist in the event timezone")
		}
	}
	if input.Result.Confirmed && (input.DistanceM == nil || raceResultTime(input.Result) == nil || input.Date == "") {
		return bad("a confirmed result requires date, distance, and finish time")
	}
	if len(input.Goals) > 30 || len(input.Checklist) > 200 || len(input.Result.Checkpoints) > 200 {
		return bad("too many goals, checklist items, or checkpoints")
	}
	lastDistance, lastTime := 0.0, int64(0)
	for _, point := range input.Result.Checkpoints {
		if strings.TrimSpace(point.Name) == "" || utf8.RuneCountInString(point.Name) > 160 || !isPositiveFloat(point.DistanceM) || point.DistanceM <= lastDistance || point.TimeMS <= lastTime || input.DistanceM == nil || point.DistanceM > *input.DistanceM {
			return bad("checkpoints need a name and increasing distances/times within the race distance")
		}
		if input.Result.SplitBasis == "" {
			return bad("select a timing basis for official checkpoints")
		}
		if total := raceBasisTime(input.Result, input.Result.SplitBasis); total != nil && point.TimeMS > *total {
			return bad("checkpoint time exceeds finish time in the selected timing basis")
		}
		lastDistance, lastTime = point.DistanceM, point.TimeMS
	}
	for _, goal := range input.Goals {
		if strings.TrimSpace(goal.Name) == "" || utf8.RuneCountInString(goal.Name) > 500 || goal.TimeMS != nil && (*goal.TimeMS <= 0 || *goal.TimeMS > 365*24*60*60*1000) {
			return bad("goals require a name and an optional positive time")
		}
	}
	if err := validateRaceChecklist(input.Checklist); err != nil {
		return err
	}
	if input.RaceOnly && input.ActivityID == "" {
		return bad("race-only confirmation requires an activity")
	}
	if input.Result.Checkpoints == nil {
		input.Result.Checkpoints = []RaceCheckpoint{}
	}
	if input.Goals == nil {
		input.Goals = []RaceGoal{}
	}
	if input.Checklist == nil {
		input.Checklist = []RaceChecklistItem{}
	}
	return nil
}
func validateRaceChecklist(items []RaceChecklistItem) error {
	if len(items) > 200 {
		return fmt.Errorf("%w: too many checklist items", ErrRaceInvalid)
	}
	for _, item := range items {
		if strings.TrimSpace(item.Label) == "" || utf8.RuneCountInString(item.Label) > 500 || !validRaceDate(item.DueDate) || item.DaysBefore != nil && (*item.DaysBefore < -365 || *item.DaysBefore > 365) || item.DueDate != "" && item.DaysBefore != nil {
			return fmt.Errorf("%w: checklist items need a label and either an absolute date or days before race", ErrRaceInvalid)
		}
	}
	return nil
}
func raceResultTime(result RaceResult) *int64 {
	if result.ChipTimeMS != nil {
		return result.ChipTimeMS
	}
	if result.GunTimeMS != nil {
		return result.GunTimeMS
	}
	return result.ManualTimeMS
}
func raceResultBasis(result RaceResult) string {
	if result.ChipTimeMS != nil {
		return "chip"
	}
	if result.GunTimeMS != nil {
		return "gun"
	}
	return "manual"
}
func raceBasisTime(result RaceResult, basis string) *int64 {
	switch basis {
	case "chip":
		return result.ChipTimeMS
	case "gun":
		return result.GunTimeMS
	case "manual":
		return result.ManualTimeMS
	}
	return nil
}
func raceCutoff(race Race) (time.Time, error) {
	zone, err := time.LoadLocation(race.Timezone)
	if err != nil {
		return time.Time{}, err
	}
	clock := race.StartTime
	if clock == "" {
		clock = "00:00"
	}
	return time.ParseInLocation("2006-01-02 15:04", race.Date+" "+clock, zone)
}
func raceRecordingHash(start time.Time, distance float64, elapsed int, samples []ActivitySample) string {
	h := sha256.New()
	e := json.NewEncoder(h)
	_ = e.Encode([]any{start.UTC().Truncate(time.Microsecond), distance, elapsed})
	ordered := append([]ActivitySample(nil), samples...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Index < ordered[j].Index })
	for _, s := range ordered {
		var ts *time.Time
		if s.Timestamp != nil {
			v := s.Timestamp.UTC().Truncate(time.Microsecond)
			ts = &v
		}
		_ = e.Encode([]any{s.Index, ts, s.ElapsedS, s.DistanceM})
	}
	return hex.EncodeToString(h.Sum(nil))
}
