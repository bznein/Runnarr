package app

import "time"

type User struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"displayName"`
	Role        string     `json:"role"`
	Disabled    bool       `json:"disabled"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type UserPreference struct {
	ThemePreference      string   `json:"themePreference"`
	ActivityTableColumns []string `json:"activityTableColumns,omitempty"`
	GearSortBy           string   `json:"gearSortBy"`
}

type SessionUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type Activity struct {
	ID                       string             `json:"id"`
	Source                   string             `json:"source"`
	SourceID                 string             `json:"sourceId"`
	Name                     string             `json:"name"`
	SourceName               string             `json:"sourceName"`
	LocalName                string             `json:"localName,omitempty"`
	Notes                    string             `json:"notes,omitempty"`
	Feedback                 string             `json:"feedback,omitempty"`
	RPE                      *int               `json:"rpe,omitempty"`
	SportType                string             `json:"sportType"`
	StartTime                time.Time          `json:"startTime"`
	DistanceM                float64            `json:"distanceM"`
	MovingTimeS              int                `json:"movingTimeS"`
	ElapsedTimeS             int                `json:"elapsedTimeS"`
	ElevationGainM           float64            `json:"elevationGainM"`
	AvgHeartRate             *float64           `json:"avgHeartRate,omitempty"`
	MaxHeartRate             *float64           `json:"maxHeartRate,omitempty"`
	AvgPaceSPKM              *float64           `json:"avgPaceSPKM,omitempty"`
	AvgGradeAdjustedPaceSPKM *float64           `json:"avgGradeAdjustedPaceSPKM,omitempty"`
	CaloriesKcal             *int               `json:"caloriesKcal,omitempty"`
	OriginalProviderURL      string             `json:"originalProviderUrl,omitempty"`
	SummaryPolyline          string             `json:"summaryPolyline,omitempty"`
	Gear                     []GearSummary      `json:"gear,omitempty"`
	Samples                  []ActivitySample   `json:"samples,omitempty"`
	Laps                     []ActivityLap      `json:"laps,omitempty"`
	Workout                  *ActivityWorkout   `json:"workout,omitempty"`
	Intervals                []ActivityInterval `json:"intervals,omitempty"`
	Climbs                   []ActivityClimb    `json:"climbs,omitempty"`
	Media                    []ActivityMedia    `json:"media,omitempty"`
	CreatedAt                time.Time          `json:"createdAt"`
}

type ClimbDetectionSettings struct {
	ClimbSmoothingRadiusM       float64 `json:"climbSmoothingRadiusM"`
	MinClimbDistanceM           float64 `json:"minClimbDistanceM"`
	MinClimbElevationGainM      float64 `json:"minClimbElevationGainM"`
	MinClimbAverageGradePct     float64 `json:"minClimbAverageGradePct"`
	MaxClimbMergeDipDistanceM   float64 `json:"maxClimbMergeDipDistanceM"`
	MaxClimbMergeElevationLossM float64 `json:"maxClimbMergeElevationLossM"`
	ClimbStartGainM             float64 `json:"climbStartGainM"`
}

type ClimbDetectionConfig struct {
	Settings    ClimbDetectionSettings `json:"settings"`
	Preset      string                 `json:"preset"`
	Sensitivity int                    `json:"sensitivity"`
}

type DeleteActivityResult struct {
	Deleted              bool   `json:"deleted"`
	ExcludedFromSync     bool   `json:"excludedFromSync"`
	SyncExclusionMessage string `json:"syncExclusionMessage,omitempty"`
}

type ActivityListPage struct {
	Activities []Activity `json:"activities"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	NextOffset int        `json:"nextOffset,omitempty"`
	HasMore    bool       `json:"hasMore"`
}

// ActivitySeries is the bounded display representation of an activity's
// samples. Full samples remain available to server-side exports and analysis,
// but clients should use this response for charts and maps.
type ActivitySeries struct {
	Samples      []ActivitySample      `json:"samples"`
	Points       []ActivitySeriesPoint `json:"points"`
	TotalSamples int                   `json:"totalSamples"`
	Sampled      bool                  `json:"sampled"`
}

type ActivitySeriesPoint struct {
	Index       int      `json:"index"`
	Label       string   `json:"label"`
	DistanceM   *float64 `json:"distanceM,omitempty"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	ElevationM  *float64 `json:"elevationM,omitempty"`
	HeartRate   *int     `json:"heartRate,omitempty"`
	PaceSPKM    *float64 `json:"paceSPKM,omitempty"`
	RawPaceSPKM *float64 `json:"rawPaceSPKM,omitempty"`
	Power       *int     `json:"power,omitempty"`
	Cadence     *int     `json:"cadence,omitempty"`
}

type DeleteActivityMediaResult struct {
	Deleted bool `json:"deleted"`
}

type Gear struct {
	ID                   string         `json:"id,omitempty"`
	Provider             string         `json:"provider"`
	ProviderGearID       string         `json:"providerGearId"`
	Name                 string         `json:"name"`
	GearType             string         `json:"gearType,omitempty"`
	Brand                string         `json:"brand,omitempty"`
	Model                string         `json:"model,omitempty"`
	Retired              bool           `json:"retired"`
	TotalDistanceM       *float64       `json:"totalDistanceM,omitempty"`
	MaxDistanceM         *float64       `json:"maxDistanceM,omitempty"`
	FirstUsedAt          *time.Time     `json:"firstUsedAt,omitempty"`
	LastUsedAt           *time.Time     `json:"lastUsedAt,omitempty"`
	ActivityCount        int            `json:"activityCount,omitempty"`
	DefaultActivityTypes []string       `json:"defaultActivityTypes,omitempty"`
	Raw                  map[string]any `json:"raw,omitempty"`
	StatsRaw             map[string]any `json:"statsRaw,omitempty"`
	CreatedAt            time.Time      `json:"createdAt,omitempty"`
	UpdatedAt            time.Time      `json:"updatedAt,omitempty"`
}

type GearSummary struct {
	ID                   string     `json:"id"`
	ProviderGearID       string     `json:"providerGearId"`
	Name                 string     `json:"name"`
	GearType             string     `json:"gearType,omitempty"`
	Brand                string     `json:"brand,omitempty"`
	Model                string     `json:"model,omitempty"`
	Retired              bool       `json:"retired"`
	TotalDistanceM       *float64   `json:"totalDistanceM,omitempty"`
	MaxDistanceM         *float64   `json:"maxDistanceM,omitempty"`
	DefaultActivityTypes []string   `json:"defaultActivityTypes,omitempty"`
	LastUsedAt           *time.Time `json:"lastUsedAt,omitempty"`
}

type ActivitySample struct {
	Index      int        `json:"index"`
	Timestamp  *time.Time `json:"timestamp,omitempty"`
	ElapsedS   *int       `json:"elapsedS,omitempty"`
	DistanceM  *float64   `json:"distanceM,omitempty"`
	Latitude   *float64   `json:"latitude,omitempty"`
	Longitude  *float64   `json:"longitude,omitempty"`
	ElevationM *float64   `json:"elevationM,omitempty"`
	HeartRate  *int       `json:"heartRate,omitempty"`
	Cadence    *int       `json:"cadence,omitempty"`
	Power      *int       `json:"power,omitempty"`
	SpeedMPS   *float64   `json:"speedMPS,omitempty"`
}

type ActivityLap struct {
	Index                    int            `json:"index"`
	StartTime                *time.Time     `json:"startTime,omitempty"`
	ElapsedTimeS             int            `json:"elapsedTimeS"`
	MovingTimeS              int            `json:"movingTimeS"`
	DistanceM                float64        `json:"distanceM"`
	AvgPaceSPKM              *float64       `json:"avgPaceSPKM,omitempty"`
	ElevationGainM           *float64       `json:"elevationGainM,omitempty"`
	ElevationLossM           *float64       `json:"elevationLossM,omitempty"`
	AvgGradeAdjustedPaceSPKM *float64       `json:"avgGradeAdjustedPaceSPKM,omitempty"`
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
	WorkoutRepeatIndex       *int           `json:"workoutRepeatIndex,omitempty"`
	Raw                      map[string]any `json:"raw,omitempty"`
}

type ActivityWorkout struct {
	Provider          string                `json:"provider"`
	ProviderWorkoutID string                `json:"providerWorkoutId,omitempty"`
	Name              string                `json:"name,omitempty"`
	SportType         string                `json:"sportType,omitempty"`
	Steps             []ActivityWorkoutStep `json:"steps,omitempty"`
	Raw               map[string]any        `json:"raw,omitempty"`
}

type ActivityWorkoutStep struct {
	Index             int                   `json:"index"`
	Order             int                   `json:"order"`
	Type              string                `json:"type,omitempty"`
	Description       string                `json:"description,omitempty"`
	RepeatCount       *int                  `json:"repeatCount,omitempty"`
	EndCondition      string                `json:"endCondition,omitempty"`
	EndConditionValue *float64              `json:"endConditionValue,omitempty"`
	TargetType        string                `json:"targetType,omitempty"`
	TargetValueOne    *float64              `json:"targetValueOne,omitempty"`
	TargetValueTwo    *float64              `json:"targetValueTwo,omitempty"`
	TargetValueUnit   string                `json:"targetValueUnit,omitempty"`
	ZoneNumber        *int                  `json:"zoneNumber,omitempty"`
	Children          []ActivityWorkoutStep `json:"children,omitempty"`
}

type ActivityInterval struct {
	Index                    int            `json:"index"`
	Category                 string         `json:"category"`
	ProviderType             string         `json:"providerType,omitempty"`
	WorkoutStepIndex         *int           `json:"workoutStepIndex,omitempty"`
	WorkoutRepeatIndex       *int           `json:"workoutRepeatIndex,omitempty"`
	StartTime                *time.Time     `json:"startTime,omitempty"`
	EndTime                  *time.Time     `json:"endTime,omitempty"`
	ElapsedTimeS             int            `json:"elapsedTimeS"`
	MovingTimeS              int            `json:"movingTimeS"`
	DistanceM                float64        `json:"distanceM"`
	AvgPaceSPKM              *float64       `json:"avgPaceSPKM,omitempty"`
	AvgGradeAdjustedPaceSPKM *float64       `json:"avgGradeAdjustedPaceSPKM,omitempty"`
	AvgHeartRate             *float64       `json:"avgHeartRate,omitempty"`
	MaxHeartRate             *float64       `json:"maxHeartRate,omitempty"`
	AvgPower                 *float64       `json:"avgPower,omitempty"`
	MaxPower                 *float64       `json:"maxPower,omitempty"`
	NormalizedPower          *float64       `json:"normalizedPower,omitempty"`
	AvgRunCadence            *float64       `json:"avgRunCadence,omitempty"`
	AvgGroundContactTimeMS   *float64       `json:"avgGroundContactTimeMS,omitempty"`
	AvgRespirationRate       *float64       `json:"avgRespirationRate,omitempty"`
	AvgTemperatureC          *float64       `json:"avgTemperatureC,omitempty"`
	ElevationGainM           *float64       `json:"elevationGainM,omitempty"`
	ElevationLossM           *float64       `json:"elevationLossM,omitempty"`
	CaloriesKcal             *int           `json:"caloriesKcal,omitempty"`
	LapIndexes               []int          `json:"lapIndexes,omitempty"`
	Raw                      map[string]any `json:"raw,omitempty"`
}

type ActivityClimb struct {
	Index            int     `json:"index"`
	Difficulty       string  `json:"difficulty"`
	StartSampleIndex int     `json:"startSampleIndex"`
	EndSampleIndex   int     `json:"endSampleIndex"`
	StartDistanceM   float64 `json:"startDistanceM"`
	EndDistanceM     float64 `json:"endDistanceM"`
	DistanceM        float64 `json:"distanceM"`
	ElevationGainM   float64 `json:"elevationGainM"`
	AvgGradePct      float64 `json:"avgGradePct"`
	StartElevationM  float64 `json:"startElevationM"`
	EndElevationM    float64 `json:"endElevationM"`
}

type ActivityMedia struct {
	ID               string     `json:"id"`
	ActivityID       string     `json:"activityId"`
	OriginalFilename string     `json:"originalFilename"`
	ContentType      string     `json:"contentType"`
	SizeBytes        int64      `json:"sizeBytes"`
	SHA256           string     `json:"sha256"`
	OriginalPath     string     `json:"-"`
	ThumbnailPath    string     `json:"-"`
	Width            int        `json:"width"`
	Height           int        `json:"height"`
	CaptureTime      *time.Time `json:"captureTime,omitempty"`
	Latitude         *float64   `json:"latitude,omitempty"`
	Longitude        *float64   `json:"longitude,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
}

type ImportedActivity struct {
	Name                     string             `json:"name"`
	SportType                string             `json:"sportType"`
	LocalNotes               string             `json:"localNotes,omitempty"`
	StartTime                time.Time          `json:"startTime"`
	DistanceM                float64            `json:"distanceM"`
	MovingTimeS              int                `json:"movingTimeS"`
	ElapsedTimeS             int                `json:"elapsedTimeS"`
	ElevationGainM           float64            `json:"elevationGainM"`
	AvgHeartRate             *float64           `json:"avgHeartRate,omitempty"`
	MaxHeartRate             *float64           `json:"maxHeartRate,omitempty"`
	AvgPaceSPKM              *float64           `json:"avgPaceSPKM,omitempty"`
	AvgGradeAdjustedPaceSPKM *float64           `json:"avgGradeAdjustedPaceSPKM,omitempty"`
	CaloriesKcal             *int               `json:"caloriesKcal,omitempty"`
	OriginalProviderURL      string             `json:"originalProviderUrl,omitempty"`
	SummaryPolyline          string             `json:"summaryPolyline,omitempty"`
	Samples                  []ActivitySample   `json:"samples,omitempty"`
	Laps                     []ActivityLap      `json:"laps,omitempty"`
	Workout                  *ActivityWorkout   `json:"workout,omitempty"`
	Intervals                []ActivityInterval `json:"intervals,omitempty"`
	ReplaceWorkoutMetadata   bool               `json:"-"`
	Raw                      map[string]any     `json:"raw,omitempty"`
}

type ActivityFilters struct {
	SportTypes           []string
	ExcludedSportTypes   []string
	Search               string
	DateFrom             time.Time
	DateTo               time.Time
	SortBy               string
	SortOrder            string
	SummaryPeriod        string
	IncludeTrainingSheet bool
}

type ImportFile struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	SHA256      string    `json:"sha256"`
	SizeBytes   int64     `json:"sizeBytes"`
	Parser      string    `json:"parser"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type ProviderConnection struct {
	ID                string    `json:"id"`
	Provider          string    `json:"provider"`
	ProviderAccountID string    `json:"providerAccountId"`
	DisplayName       string    `json:"displayName"`
	Scopes            []string  `json:"scopes"`
	ConnectedAt       time.Time `json:"connectedAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	TokenExpiresAt    time.Time `json:"tokenExpiresAt"`
}

type TrainingSheetWritebackStatus struct {
	PlannedActivityID  string     `json:"plannedActivityId"`
	ActivityID         string     `json:"activityId"`
	JobID              string     `json:"jobId,omitempty"`
	JobStatus          string     `json:"jobStatus,omitempty"`
	CancelRequestedAt  *time.Time `json:"cancelRequestedAt,omitempty"`
	SummaryStatus      string     `json:"summaryStatus"`
	SummaryError       string     `json:"summaryError,omitempty"`
	SummaryWrittenAt   *time.Time `json:"summaryWrittenAt,omitempty"`
	IntervalsStatus    string     `json:"intervalsStatus"`
	IntervalsError     string     `json:"intervalsError,omitempty"`
	IntervalsWrittenAt *time.Time `json:"intervalsWrittenAt,omitempty"`
	FeedbackStatus     string     `json:"feedbackStatus"`
	FeedbackError      string     `json:"feedbackError,omitempty"`
	FeedbackWrittenAt  *time.Time `json:"feedbackWrittenAt,omitempty"`
	LastAttemptAt      *time.Time `json:"lastAttemptAt,omitempty"`
}

type StoredProviderConnection struct {
	ProviderConnection
	AccessTokenCiphertext  []byte
	RefreshTokenCiphertext []byte
}

type SyncJob struct {
	ID                string         `json:"id"`
	Provider          string         `json:"provider"`
	Kind              string         `json:"kind"`
	Status            string         `json:"status"`
	Payload           map[string]any `json:"payload,omitempty"`
	Error             string         `json:"error,omitempty"`
	CreatedAt         time.Time      `json:"createdAt"`
	StartedAt         *time.Time     `json:"startedAt,omitempty"`
	FinishedAt        *time.Time     `json:"finishedAt,omitempty"`
	CancelRequestedAt *time.Time     `json:"cancelRequestedAt,omitempty"`
}

type DailyHealthMetric struct {
	ID                  string         `json:"id,omitempty"`
	Provider            string         `json:"provider"`
	Date                string         `json:"date"`
	Steps               *int           `json:"steps,omitempty"`
	TotalCaloriesKcal   *int           `json:"totalCaloriesKcal,omitempty"`
	ActiveCaloriesKcal  *int           `json:"activeCaloriesKcal,omitempty"`
	RestingHeartRateBPM *float64       `json:"restingHeartRateBpm,omitempty"`
	AvgHeartRateBPM     *float64       `json:"avgHeartRateBpm,omitempty"`
	MaxHeartRateBPM     *float64       `json:"maxHeartRateBpm,omitempty"`
	SleepDurationS      *int           `json:"sleepDurationS,omitempty"`
	DeepSleepS          *int           `json:"deepSleepS,omitempty"`
	LightSleepS         *int           `json:"lightSleepS,omitempty"`
	REMSleepS           *int           `json:"remSleepS,omitempty"`
	AwakeSleepS         *int           `json:"awakeSleepS,omitempty"`
	SleepScore          *float64       `json:"sleepScore,omitempty"`
	StressAvg           *float64       `json:"stressAvg,omitempty"`
	StressMax           *float64       `json:"stressMax,omitempty"`
	BodyBatteryAvg      *float64       `json:"bodyBatteryAvg,omitempty"`
	BodyBatteryMin      *float64       `json:"bodyBatteryMin,omitempty"`
	BodyBatteryMax      *float64       `json:"bodyBatteryMax,omitempty"`
	BodyBatteryStart    *float64       `json:"bodyBatteryStart,omitempty"`
	BodyBatteryEnd      *float64       `json:"bodyBatteryEnd,omitempty"`
	BodyBatteryGained   *float64       `json:"bodyBatteryGained,omitempty"`
	BodyBatteryDrained  *float64       `json:"bodyBatteryDrained,omitempty"`
	HRVAvgMS            *float64       `json:"hrvAvgMs,omitempty"`
	HRVStatus           string         `json:"hrvStatus,omitempty"`
	WeightKG            *float64       `json:"weightKg,omitempty"`
	BodyFatPct          *float64       `json:"bodyFatPct,omitempty"`
	Raw                 map[string]any `json:"raw,omitempty"`
	CreatedAt           time.Time      `json:"createdAt,omitempty"`
	UpdatedAt           time.Time      `json:"updatedAt,omitempty"`
}

type HealthChartPoint struct {
	Date                   string   `json:"date"`
	Steps                  *float64 `json:"steps,omitempty"`
	TotalCalories          *float64 `json:"totalCalories,omitempty"`
	ActiveCalories         *float64 `json:"activeCalories,omitempty"`
	RemainingCalories      *float64 `json:"remainingCalories,omitempty"`
	SleepHours             *float64 `json:"sleepHours,omitempty"`
	SleepScore             *float64 `json:"sleepScore,omitempty"`
	RestingHeartRate       *float64 `json:"restingHeartRate,omitempty"`
	Stress                 *float64 `json:"stress,omitempty"`
	BodyBatteryGained      *float64 `json:"bodyBatteryGained,omitempty"`
	BodyBatteryDrained     *float64 `json:"bodyBatteryDrained,omitempty"`
	BodyBatteryDrainedLoss *float64 `json:"bodyBatteryDrainedLoss,omitempty"`
	BodyBatteryHighest     *float64 `json:"bodyBatteryHighest,omitempty"`
	HRV                    *float64 `json:"hrv,omitempty"`
	Weight                 *float64 `json:"weight,omitempty"`
}

type SummaryStats struct {
	ActivityCount   int             `json:"activityCount"`
	DistanceM       float64         `json:"distanceM"`
	MovingTimeS     int             `json:"movingTimeS"`
	ElevationGainM  float64         `json:"elevationGainM"`
	Recent          []Activity      `json:"recent"`
	WeeklyDistance  []WeeklyBucket  `json:"weeklyDistance"`
	DistanceBuckets []SummaryBucket `json:"distanceBuckets"`
	SummaryPeriod   string          `json:"summaryPeriod"`
}

type SummaryBucket struct {
	Start     time.Time `json:"start"`
	DistanceM float64   `json:"distanceM"`
}

type WeeklyBucket struct {
	WeekStart time.Time `json:"weekStart"`
	DistanceM float64   `json:"distanceM"`
}

type CalendarActivity struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	Name        string    `json:"name"`
	StartTime   time.Time `json:"startTime"`
	SportType   string    `json:"sportType"`
	DistanceM   float64   `json:"distanceM"`
	MovingTimeS int       `json:"movingTimeS"`
}

type CalendarDay struct {
	Date          string             `json:"date"`
	ActivityCount int                `json:"activityCount"`
	HasHealthData bool               `json:"hasHealthData"`
	Activities    []CalendarActivity `json:"activities"`
}

type ActivityCalendar struct {
	MonthStart string        `json:"monthStart"`
	MonthEnd   string        `json:"monthEnd"`
	Days       []CalendarDay `json:"days"`
}

type CalendarDayView struct {
	Date       string             `json:"date"`
	Health     *DailyHealthMetric `json:"health,omitempty"`
	Activities []CalendarActivity `json:"activities"`
}
