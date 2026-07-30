package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var errGarminWorkoutOwnership = errors.New("Garmin workout is not owned by Runnarr")

type compiledGarminWorkout struct {
	DefinitionHash  string
	Name            string
	OwnershipMarker string
	Payload         map[string]any
}

func compileGarminWorkout(definition WorkoutDefinition, paceToleranceSeconds int, ownerToken string) (compiledGarminWorkout, error) {
	if err := validateWorkoutDefinition(definition); err != nil {
		return compiledGarminWorkout{}, err
	}
	if paceToleranceSeconds < 0 || paceToleranceSeconds > 60 {
		return compiledGarminWorkout{}, errors.New("pace tolerance must be between 0 and 60 seconds")
	}
	ownerToken = strings.TrimSpace(ownerToken)
	if ownerToken == "" {
		return compiledGarminWorkout{}, errors.New("Garmin workout owner token is required")
	}

	steps := make([]any, 0, len(definition.Steps))
	for _, step := range definition.Steps {
		compiled, err := compileGarminWorkoutStep(step, paceToleranceSeconds)
		if err != nil {
			return compiledGarminWorkout{}, err
		}
		steps = append(steps, compiled)
	}
	content := map[string]any{
		"sportType":               map[string]any{"sportTypeId": 1, "sportTypeKey": "running", "displayOrder": 1},
		"estimatedDurationInSecs": definition.EstimatedDurationS,
		"workoutSegments": []any{map[string]any{
			"segmentOrder": 1,
			"sportType":    map[string]any{"sportTypeId": 1, "sportTypeKey": "running", "displayOrder": 1},
			"workoutSteps": steps,
		}},
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return compiledGarminWorkout{}, err
	}
	sum := sha256.Sum256(encoded)
	hash := hex.EncodeToString(sum[:])
	marker := fmt.Sprintf("runnarr:%s:%s", ownerToken, hash)
	name := fmt.Sprintf("Runnarr %s %s", workoutDefinitionSummary(definition), hash[:8])
	if len(name) > 80 {
		name = name[:80]
	}
	payload := cloneWorkoutMap(content)
	payload["workoutName"] = name
	payload["description"] = marker
	return compiledGarminWorkout{DefinitionHash: hash, Name: name, OwnershipMarker: marker, Payload: payload}, nil
}

func compileGarminWorkoutStep(step WorkoutStep, tolerance int) (map[string]any, error) {
	if step.Kind == workoutStepRepeat {
		children := make([]any, 0, len(step.Children))
		for _, child := range step.Children {
			compiled, err := compileGarminWorkoutStep(child, tolerance)
			if err != nil {
				return nil, err
			}
			children = append(children, compiled)
		}
		return map[string]any{
			"type":               "RepeatGroupDTO",
			"stepOrder":          step.Order,
			"stepType":           garminWorkoutStepType(workoutStepRepeat),
			"numberOfIterations": step.RepeatCount,
			"workoutSteps":       children,
			"endCondition":       map[string]any{"conditionTypeId": 7, "conditionTypeKey": "iterations", "displayOrder": 7, "displayable": false},
			"endConditionValue":  float64(step.RepeatCount),
			"smartRepeat":        false,
			"skipLastRestStep":   step.SkipLastRecovery,
		}, nil
	}
	if step.EndCondition == nil {
		return nil, fmt.Errorf("workout step %d has no end condition", step.Order)
	}
	compiled := map[string]any{
		"type":         "ExecutableStepDTO",
		"stepOrder":    step.Order,
		"stepType":     garminWorkoutStepType(step.Kind),
		"endCondition": garminWorkoutEndCondition(step.EndCondition.Type),
		"targetType":   map[string]any{"workoutTargetTypeId": 1, "workoutTargetTypeKey": "no.target", "displayOrder": 1},
	}
	if step.Description != "" {
		compiled["description"] = step.Description
	}
	if step.EndCondition.Type != workoutEndLapButton {
		compiled["endConditionValue"] = step.EndCondition.Value
	}
	if step.Target.Type == workoutTargetPace {
		fast, slow, err := effectiveWorkoutPaceRange(step.Target, tolerance)
		if err != nil {
			return nil, fmt.Errorf("workout step %d: %w", step.Order, err)
		}
		compiled["targetType"] = map[string]any{"workoutTargetTypeId": 6, "workoutTargetTypeKey": "pace.zone", "displayOrder": 6}
		compiled["targetValueOne"] = 1000.0 / float64(slow)
		compiled["targetValueTwo"] = 1000.0 / float64(fast)
	}
	return compiled, nil
}

func effectiveWorkoutPaceRange(target WorkoutTarget, tolerance int) (int, int, error) {
	if target.PaceFastSecondsKM != nil && target.PaceSlowSecondsKM != nil {
		fast, slow := *target.PaceFastSecondsKM, *target.PaceSlowSecondsKM
		if fast <= 0 || slow < fast {
			return 0, 0, errors.New("pace range is invalid")
		}
		return fast, slow, nil
	}
	if target.PaceSecondsPerKM == nil || *target.PaceSecondsPerKM <= 0 {
		return 0, 0, errors.New("pace target is missing")
	}
	return max(1, *target.PaceSecondsPerKM-tolerance), *target.PaceSecondsPerKM + tolerance, nil
}

func garminWorkoutStepType(kind string) map[string]any {
	id, key, display := 7, "other", 7
	switch kind {
	case workoutStepWarmup:
		id, key, display = 1, "warmup", 1
	case workoutStepCooldown:
		id, key, display = 2, "cooldown", 2
	case workoutStepWork:
		id, key, display = 3, "interval", 3
	case workoutStepRecovery:
		id, key, display = 4, "recovery", 4
	case workoutStepRepeat:
		id, key, display = 6, "repeat", 6
	}
	return map[string]any{"stepTypeId": id, "stepTypeKey": key, "displayOrder": display}
}

func garminWorkoutEndCondition(kind string) map[string]any {
	id, key, display, displayable := 1, "lap.button", 1, true
	switch kind {
	case workoutEndTime:
		id, key, display = 2, "time", 2
	case workoutEndDistance:
		id, key, display = 3, "distance", 3
	}
	return map[string]any{"conditionTypeId": id, "conditionTypeKey": key, "displayOrder": display, "displayable": displayable}
}

func validateWorkoutDefinition(definition WorkoutDefinition) error {
	if len(definition.Steps) == 0 {
		return errors.New("workout must contain at least one step")
	}
	var validate func([]WorkoutStep) error
	validate = func(steps []WorkoutStep) error {
		for _, step := range steps {
			switch step.Kind {
			case workoutStepWarmup, workoutStepWork, workoutStepRecovery, workoutStepCooldown:
				if step.EndCondition == nil {
					return fmt.Errorf("workout step %d has no end condition", step.Order)
				}
				if step.EndCondition.Type != workoutEndLapButton && (!isFiniteWorkoutNumber(step.EndCondition.Value) || step.EndCondition.Value <= 0) {
					return fmt.Errorf("workout step %d has an invalid end condition", step.Order)
				}
			case workoutStepRepeat:
				if step.RepeatCount < 2 || len(step.Children) == 0 {
					return fmt.Errorf("repeat step %d is invalid", step.Order)
				}
				if err := validate(step.Children); err != nil {
					return err
				}
			default:
				return fmt.Errorf("workout step %d has an unknown kind", step.Order)
			}
			if step.Target.Type != workoutTargetNone && step.Target.Type != workoutTargetPace {
				return fmt.Errorf("workout step %d has an unsupported target", step.Order)
			}
		}
		return nil
	}
	return validate(definition.Steps)
}

func workoutDefinitionSummary(definition WorkoutDefinition) string {
	for _, step := range definition.Steps {
		if step.Kind == workoutStepRepeat {
			return fmt.Sprintf("%dx intervals", step.RepeatCount)
		}
	}
	return "running workout"
}

func garminWorkoutRemoteOwned(remote map[string]any, providerWorkoutID, ownershipMarker string) error {
	if strings.TrimSpace(providerWorkoutID) == "" || strings.TrimSpace(ownershipMarker) == "" {
		return errGarminWorkoutOwnership
	}
	remoteID := garminRawWorkoutID(remote["workoutId"])
	description := strings.TrimSpace(fmt.Sprint(remote["description"]))
	if remoteID != providerWorkoutID || description != ownershipMarker {
		return errGarminWorkoutOwnership
	}
	return nil
}

func garminRawWorkoutID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return strconv.FormatInt(integer, 10)
		}
		return ""
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) {
			return ""
		}
		return strconv.FormatFloat(typed, 'f', 0, 64)
	case float32:
		value := float64(typed)
		if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) {
			return ""
		}
		return strconv.FormatFloat(value, 'f', 0, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	default:
		return ""
	}
}

func cloneWorkoutMap(value map[string]any) map[string]any {
	encoded, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(encoded, &result)
	return result
}

func isFiniteWorkoutNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
