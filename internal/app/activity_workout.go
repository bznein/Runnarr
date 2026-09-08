package app

// Older imports retained skipLastRestStep only in the provider payload. Restore
// it in the response without rewriting stored workouts or requiring a resync.
func restoreActivityWorkoutRepeatMetadata(workout *ActivityWorkout) {
	if workout.Provider != garminProvider {
		return
	}
	var rawSteps []any
	segments, _ := workout.Raw["workoutSegments"].([]any)
	for _, value := range segments {
		segment, _ := value.(map[string]any)
		steps, _ := segment["workoutSteps"].([]any)
		rawSteps = append(rawSteps, steps...)
	}
	var restore func([]ActivityWorkoutStep, []any)
	restore = func(steps []ActivityWorkoutStep, raw []any) {
		for i := range steps {
			if i >= len(raw) {
				break
			}
			item, _ := raw[i].(map[string]any)
			if skip, ok := item["skipLastRestStep"].(bool); ok && steps[i].SkipLastRecovery == nil {
				steps[i].SkipLastRecovery = &skip
			}
			children, _ := item["workoutSteps"].([]any)
			restore(steps[i].Children, children)
		}
	}
	restore(workout.Steps, rawSteps)
}
