package app

import (
	"errors"
	"time"
)

var (
	ErrWorkoutReadOnly      = errors.New("sheet-derived workouts are read-only")
	ErrWorkoutInvalid       = errors.New("workout is invalid")
	ErrWorkoutPlanCompleted = errors.New("a completed workout cannot be unscheduled")
)

const (
	workoutSourceTrainingSheet = "training_sheet"
	workoutSourceManual        = "manual"

	workoutParseReady   = "ready"
	workoutParseWarning = "warning"
	workoutParseError   = "error"

	workoutStepWarmup   = "warmup"
	workoutStepWork     = "work"
	workoutStepRecovery = "recovery"
	workoutStepCooldown = "cooldown"
	workoutStepRepeat   = "repeat"

	workoutEndTime      = "time"
	workoutEndDistance  = "distance"
	workoutEndLapButton = "lap_button"

	workoutTargetNone = "none"
	workoutTargetPace = "pace"
)

type WorkoutConfig struct {
	SyncEnabled                 bool   `json:"syncEnabled"`
	DefaultPaceToleranceSeconds int    `json:"defaultPaceToleranceSeconds"`
	Timezone                    string `json:"timezone"`
	HorizonDays                 int    `json:"horizonDays"`
}

type Workout struct {
	ID                   string                `json:"id"`
	Source               string                `json:"source"`
	PlannedActivityID    string                `json:"plannedActivityId,omitempty"`
	CopiedFromWorkoutID  string                `json:"copiedFromWorkoutId,omitempty"`
	Name                 string                `json:"name"`
	SportType            string                `json:"sportType"`
	SourceText           string                `json:"sourceText,omitempty"`
	SourceHash           string                `json:"sourceHash,omitempty"`
	Definition           WorkoutDefinition     `json:"definition"`
	ParseStatus          string                `json:"parseStatus"`
	ParseMessages        []WorkoutParseMessage `json:"parseMessages"`
	ScheduledDate        string                `json:"scheduledDate,omitempty"`
	PaceToleranceSeconds *int                  `json:"paceToleranceSeconds,omitempty"`
	GarminExcluded       bool                  `json:"garminExcluded"`
	Revision             int                   `json:"revision"`
	Garmin               WorkoutGarminState    `json:"garmin"`
	GeneratedAt          time.Time             `json:"generatedAt"`
	ArchivedAt           *time.Time            `json:"archivedAt,omitempty"`
	CreatedAt            time.Time             `json:"createdAt"`
	UpdatedAt            time.Time             `json:"updatedAt"`
}

type WorkoutDefinition struct {
	Version            int           `json:"version"`
	SportType          string        `json:"sportType"`
	Steps              []WorkoutStep `json:"steps"`
	EstimatedDurationS int           `json:"estimatedDurationS"`
}

type WorkoutStep struct {
	Order            int                  `json:"order"`
	Kind             string               `json:"kind"`
	Description      string               `json:"description,omitempty"`
	EndCondition     *WorkoutEndCondition `json:"endCondition,omitempty"`
	Target           WorkoutTarget        `json:"target"`
	RepeatCount      int                  `json:"repeatCount,omitempty"`
	SkipLastRecovery bool                 `json:"skipLastRecovery,omitempty"`
	Children         []WorkoutStep        `json:"children,omitempty"`
}

type WorkoutEndCondition struct {
	Type  string  `json:"type"`
	Value float64 `json:"value,omitempty"`
	Unit  string  `json:"unit,omitempty"`
}

type WorkoutTarget struct {
	Type              string `json:"type"`
	PaceSecondsPerKM  *int   `json:"paceSecondsPerKM,omitempty"`
	PaceFastSecondsKM *int   `json:"paceFastSecondsPerKM,omitempty"`
	PaceSlowSecondsKM *int   `json:"paceSlowSecondsPerKM,omitempty"`
}

type WorkoutParseMessage struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	Source  string `json:"source,omitempty"`
}

type WorkoutParseResult struct {
	Definition WorkoutDefinition     `json:"definition"`
	Status     string                `json:"status"`
	Messages   []WorkoutParseMessage `json:"messages"`
}

type WorkoutGarminState struct {
	Status             string     `json:"status,omitempty"`
	Error              string     `json:"error,omitempty"`
	ProviderWorkoutID  string     `json:"providerWorkoutId,omitempty"`
	ProviderScheduleID string     `json:"providerScheduleId,omitempty"`
	ScheduledAt        *time.Time `json:"scheduledAt,omitempty"`
}

type WorkoutList struct {
	Workouts []Workout `json:"workouts"`
}

func noWorkoutTarget() WorkoutTarget {
	return WorkoutTarget{Type: workoutTargetNone}
}
