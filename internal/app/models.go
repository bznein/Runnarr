package app

import "time"

type Activity struct {
	ID                  string           `json:"id"`
	Source              string           `json:"source"`
	SourceID            string           `json:"sourceId"`
	Name                string           `json:"name"`
	SourceName          string           `json:"sourceName"`
	LocalName           string           `json:"localName,omitempty"`
	SportType           string           `json:"sportType"`
	StartTime           time.Time        `json:"startTime"`
	DistanceM           float64          `json:"distanceM"`
	MovingTimeS         int              `json:"movingTimeS"`
	ElapsedTimeS        int              `json:"elapsedTimeS"`
	ElevationGainM      float64          `json:"elevationGainM"`
	AvgHeartRate        *float64         `json:"avgHeartRate,omitempty"`
	MaxHeartRate        *float64         `json:"maxHeartRate,omitempty"`
	AvgPaceSPKM         *float64         `json:"avgPaceSPKM,omitempty"`
	CaloriesKcal        *int             `json:"caloriesKcal,omitempty"`
	OriginalProviderURL string           `json:"originalProviderUrl,omitempty"`
	SummaryPolyline     string           `json:"summaryPolyline,omitempty"`
	Samples             []ActivitySample `json:"samples,omitempty"`
	Laps                []ActivityLap    `json:"laps,omitempty"`
	Climbs              []ActivityClimb  `json:"climbs,omitempty"`
	Media               []ActivityMedia  `json:"media,omitempty"`
	CreatedAt           time.Time        `json:"createdAt"`
}

type DeleteActivityResult struct {
	Deleted              bool   `json:"deleted"`
	ExcludedFromSync     bool   `json:"excludedFromSync"`
	SyncExclusionMessage string `json:"syncExclusionMessage,omitempty"`
}

type DeleteActivityMediaResult struct {
	Deleted bool `json:"deleted"`
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
	Index          int        `json:"index"`
	StartTime      *time.Time `json:"startTime,omitempty"`
	ElapsedTimeS   int        `json:"elapsedTimeS"`
	DistanceM      float64    `json:"distanceM"`
	ElevationGainM *float64   `json:"elevationGainM,omitempty"`
	ElevationLossM *float64   `json:"elevationLossM,omitempty"`
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
	Name                string           `json:"name"`
	SportType           string           `json:"sportType"`
	StartTime           time.Time        `json:"startTime"`
	DistanceM           float64          `json:"distanceM"`
	MovingTimeS         int              `json:"movingTimeS"`
	ElapsedTimeS        int              `json:"elapsedTimeS"`
	ElevationGainM      float64          `json:"elevationGainM"`
	AvgHeartRate        *float64         `json:"avgHeartRate,omitempty"`
	MaxHeartRate        *float64         `json:"maxHeartRate,omitempty"`
	CaloriesKcal        *int             `json:"caloriesKcal,omitempty"`
	OriginalProviderURL string           `json:"originalProviderUrl,omitempty"`
	SummaryPolyline     string           `json:"summaryPolyline,omitempty"`
	Samples             []ActivitySample `json:"samples,omitempty"`
	Laps                []ActivityLap    `json:"laps,omitempty"`
	Raw                 map[string]any   `json:"raw,omitempty"`
}

type ActivityFilters struct {
	SportTypes         []string
	ExcludedSportTypes []string
	Search             string
	DateFrom           time.Time
	DateTo             time.Time
	SortBy             string
	SortOrder          string
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

type StoredProviderConnection struct {
	ProviderConnection
	AccessTokenCiphertext  []byte
	RefreshTokenCiphertext []byte
}

type SyncJob struct {
	ID         string         `json:"id"`
	Provider   string         `json:"provider"`
	Kind       string         `json:"kind"`
	Status     string         `json:"status"`
	Payload    map[string]any `json:"payload,omitempty"`
	Error      string         `json:"error,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
	StartedAt  *time.Time     `json:"startedAt,omitempty"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
}

type SummaryStats struct {
	ActivityCount  int            `json:"activityCount"`
	DistanceM      float64        `json:"distanceM"`
	MovingTimeS    int            `json:"movingTimeS"`
	ElevationGainM float64        `json:"elevationGainM"`
	Recent         []Activity     `json:"recent"`
	WeeklyDistance []WeeklyBucket `json:"weeklyDistance"`
}

type WeeklyBucket struct {
	WeekStart time.Time `json:"weekStart"`
	DistanceM float64   `json:"distanceM"`
}
