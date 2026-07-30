package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	workoutRepeatPrefixPattern = regexp.MustCompile(`(?i)^\s*(\d+)\s*[x×]\s*`)
	workoutSetsPattern         = regexp.MustCompile(`(?i)^\s*(\d+)\s*sets?\s+of\s*:\s*(.+)$`)
	workoutDurationPattern     = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(hours?|hrs?|hr|h|minutes?|mins?|min|seconds?|secs?|sec|s)\b`)
	workoutDistancePattern     = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(kilometres?|kilometers?|kms?|km|metres?|meters?|m)\b`)
	workoutPacePattern         = regexp.MustCompile(`(?i)@(\d+):(\d{2})(?:\s*-\s*(?:(\d+):)?(\d{2}))?`)
	workoutPaceAliasPattern    = regexp.MustCompile(`(?i)(\d+\s*[x×]\s*\d+(?:\.\d+)?\s*(?:minutes?|mins?|min|seconds?|secs?|sec))\s*:(\d+:\d{2})`)
)

func isStructuredWorkoutPrescription(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(normalized, "//") || strings.Contains(normalized, "@") ||
		strings.Contains(normalized, "continuous") || strings.Contains(normalized, "sets of") ||
		workoutRepeatPrefixPattern.MatchString(normalized)
}

func parseWorkoutPrescription(source string, table *trainingSheetWorkoutTable) WorkoutParseResult {
	result := WorkoutParseResult{
		Definition: WorkoutDefinition{Version: 1, SportType: "Run", Steps: []WorkoutStep{}},
		Status:     workoutParseReady,
		Messages:   []WorkoutParseMessage{},
	}
	text := strings.TrimSpace(source)
	if colon := strings.Index(text, ":"); colon > 0 && len(parseDayScope(text[:colon])) > 0 {
		text = strings.TrimSpace(text[colon+1:])
	}
	if text == "" {
		return workoutParseFailure(result, "Workout prescription is empty", source)
	}
	if alias := workoutPaceAliasPattern.FindStringSubmatch(text); len(alias) == 3 {
		text = workoutPaceAliasPattern.ReplaceAllString(text, `${1}@${2}`)
		result.Messages = append(result.Messages, WorkoutParseMessage{Level: "warning", Message: "Interpreted ':' before a pace as '@'", Source: alias[0]})
	}

	blocks := splitWorkoutTopLevel(text, "//")
	continuous := strings.Contains(strings.ToLower(text), "continuous")
	for _, block := range blocks {
		steps, messages, err := parseWorkoutBlock(block)
		result.Messages = append(result.Messages, messages...)
		if err != nil {
			return workoutParseFailure(result, err.Error(), block)
		}
		result.Definition.Steps = append(result.Definition.Steps, steps...)
	}
	if len(result.Definition.Steps) == 0 {
		return workoutParseFailure(result, "No workout steps could be parsed", source)
	}

	if continuous {
		var err error
		result.Definition.Steps, err = applyContinuousWorkoutRanges(result.Definition.Steps, table)
		if err != nil {
			return workoutParseFailure(result, err.Error(), "continuous analysis ranges")
		}
	}
	applyFinalRecoveryDefault(result.Definition.Steps)
	numberWorkoutSteps(result.Definition.Steps)
	result.Definition.EstimatedDurationS = estimateWorkoutDuration(result.Definition.Steps)
	if len(result.Messages) > 0 {
		result.Status = workoutParseWarning
	}
	return result
}

func parseWorkoutBlock(raw string) ([]WorkoutStep, []WorkoutParseMessage, error) {
	block := strings.TrimSpace(raw)
	block = strings.TrimSpace(strings.TrimSuffix(block, "(continuous)"))
	if block == "" {
		return nil, nil, nil
	}
	lower := strings.ToLower(block)
	if strings.Contains(lower, "warm up") || strings.Contains(lower, "warmup") {
		step, err := parseExecutableWorkoutStep(block, workoutStepWarmup)
		return []WorkoutStep{step}, nil, err
	}
	if strings.Contains(lower, "cool down") || strings.Contains(lower, "cooldown") {
		step, err := parseExecutableWorkoutStep(block, workoutStepCooldown)
		return []WorkoutStep{step}, nil, err
	}
	if match := workoutSetsPattern.FindStringSubmatch(block); len(match) == 3 {
		count, _ := strconv.Atoi(match[1])
		children, messages, err := parseWorkoutSequence(match[2])
		if err != nil {
			return nil, messages, err
		}
		return []WorkoutStep{{Kind: workoutStepRepeat, RepeatCount: count, Description: block, Target: noWorkoutTarget(), Children: children}}, messages, nil
	}
	return parseWorkoutSequence(block)
}

func parseWorkoutSequence(value string) ([]WorkoutStep, []WorkoutParseMessage, error) {
	parts := splitWorkoutSequence(value)
	steps := make([]WorkoutStep, 0, len(parts))
	messages := make([]WorkoutParseMessage, 0)
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "then "))
		if part == "" {
			continue
		}
		parsed, err := parseWorkoutSequenceItem(part)
		if err != nil {
			return nil, messages, err
		}
		steps = append(steps, parsed...)
	}
	return steps, messages, nil
}

func parseWorkoutSequenceItem(value string) ([]WorkoutStep, error) {
	main, recovery := splitTrailingWorkoutParenthesis(value)
	repeatCount := 0
	if match := workoutRepeatPrefixPattern.FindStringSubmatch(main); len(match) == 2 {
		repeatCount, _ = strconv.Atoi(match[1])
		main = strings.TrimSpace(main[len(match[0]):])
	}
	kind := workoutStepWork
	lower := strings.ToLower(main)
	if strings.Contains(lower, "jog") || strings.Contains(lower, "recovery") || strings.Contains(lower, "rest") {
		kind = workoutStepRecovery
	}
	step, err := parseExecutableWorkoutStep(main, kind)
	if err != nil {
		return nil, err
	}
	if repeatCount <= 0 {
		steps := []WorkoutStep{step}
		if strings.TrimSpace(recovery) != "" {
			recoveryStep, recoveryErr := parseRecoveryWorkoutStep(recovery)
			if recoveryErr != nil {
				return nil, recoveryErr
			}
			steps = append(steps, recoveryStep)
		}
		return steps, nil
	}
	children := []WorkoutStep{step}
	if strings.TrimSpace(recovery) != "" {
		recoveryStep, recoveryErr := parseRecoveryWorkoutStep(recovery)
		if recoveryErr != nil {
			return nil, recoveryErr
		}
		children = append(children, recoveryStep)
	}
	return []WorkoutStep{{Kind: workoutStepRepeat, RepeatCount: repeatCount, Description: value, Target: noWorkoutTarget(), Children: children}}, nil
}

func parseExecutableWorkoutStep(value, kind string) (WorkoutStep, error) {
	step := WorkoutStep{Kind: kind, Description: strings.TrimSpace(value), Target: noWorkoutTarget()}
	condition, ok := parseWorkoutEndCondition(value)
	if !ok {
		return step, fmt.Errorf("Could not determine a duration or distance for %q", strings.TrimSpace(value))
	}
	step.EndCondition = &condition
	target, err := parseWorkoutPaceTarget(value)
	if err != nil {
		return step, err
	}
	step.Target = target
	return step, nil
}

func parseRecoveryWorkoutStep(value string) (WorkoutStep, error) {
	lower := strings.ToLower(strings.TrimSpace(value))
	step := WorkoutStep{Kind: workoutStepRecovery, Description: strings.TrimSpace(value), Target: noWorkoutTarget()}
	if strings.Contains(lower, "jog down") || strings.Contains(lower, "to start") || strings.Contains(lower, "lap button") {
		step.EndCondition = &WorkoutEndCondition{Type: workoutEndLapButton}
		return step, nil
	}
	condition, ok := parseWorkoutEndCondition(value)
	if !ok {
		return step, fmt.Errorf("Could not determine recovery duration for %q", strings.TrimSpace(value))
	}
	step.EndCondition = &condition
	return step, nil
}

func parseWorkoutEndCondition(value string) (WorkoutEndCondition, bool) {
	if match := workoutDurationPattern.FindStringSubmatch(value); len(match) == 3 {
		amount, err := strconv.ParseFloat(match[1], 64)
		if err != nil || amount <= 0 {
			return WorkoutEndCondition{}, false
		}
		unit := strings.ToLower(match[2])
		seconds := amount
		switch {
		case strings.HasPrefix(unit, "h"):
			seconds *= 3600
		case strings.HasPrefix(unit, "m"):
			seconds *= 60
		}
		return WorkoutEndCondition{Type: workoutEndTime, Value: seconds, Unit: "seconds"}, true
	}
	if match := workoutDistancePattern.FindStringSubmatch(value); len(match) == 3 {
		amount, err := strconv.ParseFloat(match[1], 64)
		if err != nil || amount <= 0 {
			return WorkoutEndCondition{}, false
		}
		if strings.HasPrefix(strings.ToLower(match[2]), "k") {
			amount *= 1000
		}
		return WorkoutEndCondition{Type: workoutEndDistance, Value: amount, Unit: "metres"}, true
	}
	return WorkoutEndCondition{}, false
}

func parseWorkoutPaceTarget(value string) (WorkoutTarget, error) {
	match := workoutPacePattern.FindStringSubmatch(value)
	if len(match) == 0 {
		return noWorkoutTarget(), nil
	}
	minutes, _ := strconv.Atoi(match[1])
	seconds, _ := strconv.Atoi(match[2])
	if seconds >= 60 {
		return WorkoutTarget{}, fmt.Errorf("Invalid pace target %q", match[0])
	}
	first := minutes*60 + seconds
	if match[4] == "" {
		return WorkoutTarget{Type: workoutTargetPace, PaceSecondsPerKM: intWorkoutPtr(first)}, nil
	}
	secondMinutes := minutes
	if match[3] != "" {
		secondMinutes, _ = strconv.Atoi(match[3])
	}
	secondSeconds, _ := strconv.Atoi(match[4])
	if secondSeconds >= 60 {
		return WorkoutTarget{}, fmt.Errorf("Invalid pace target %q", match[0])
	}
	second := secondMinutes*60 + secondSeconds
	fast, slow := first, second
	if fast > slow {
		fast, slow = slow, fast
	}
	return WorkoutTarget{Type: workoutTargetPace, PaceFastSecondsKM: intWorkoutPtr(fast), PaceSlowSecondsKM: intWorkoutPtr(slow)}, nil
}

func applyContinuousWorkoutRanges(steps []WorkoutStep, table *trainingSheetWorkoutTable) ([]WorkoutStep, error) {
	workIndexes := make([]int, 0)
	for index, step := range steps {
		if step.Kind == workoutStepWork {
			workIndexes = append(workIndexes, index)
		}
	}
	if len(workIndexes) != 1 || table == nil {
		return steps, nil
	}
	workIndex := workIndexes[0]
	work := steps[workIndex]
	if work.EndCondition == nil || work.EndCondition.Type != workoutEndTime {
		return steps, nil
	}
	type interval struct{ start, end int }
	ranges := make([]interval, 0)
	for _, row := range table.Rows {
		if row.Kind != trainingSheetRowExact || !strings.HasPrefix(row.Group, "range:") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(row.Group, "range:"), "-")
		if len(parts) != 2 {
			continue
		}
		start, startErr := strconv.Atoi(parts[0])
		end, endErr := strconv.Atoi(parts[1])
		if startErr != nil || endErr != nil || end <= start {
			return nil, fmt.Errorf("Continuous analysis ranges are invalid")
		}
		ranges = append(ranges, interval{start: start, end: end})
	}
	if len(ranges) == 0 {
		return steps, nil
	}
	sort.SliceStable(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	expectedStart := 0
	for _, item := range ranges {
		if item.start != expectedStart {
			return nil, fmt.Errorf("Continuous analysis ranges must be contiguous from 0 minutes")
		}
		expectedStart = item.end
	}
	prescribedMinutes := int(work.EndCondition.Value) / 60
	if float64(prescribedMinutes*60) != work.EndCondition.Value || expectedStart != prescribedMinutes {
		return nil, fmt.Errorf("Continuous analysis ranges must cover the prescribed duration")
	}
	replacement := make([]WorkoutStep, 0, len(ranges))
	for _, item := range ranges {
		child := work
		condition := *work.EndCondition
		condition.Value = float64((item.end - item.start) * 60)
		child.EndCondition = &condition
		child.Description = fmt.Sprintf("%s (%d-%d min)", work.Description, item.start, item.end)
		replacement = append(replacement, child)
	}
	result := append([]WorkoutStep(nil), steps[:workIndex]...)
	result = append(result, replacement...)
	result = append(result, steps[workIndex+1:]...)
	return result, nil
}

func applyFinalRecoveryDefault(steps []WorkoutStep) {
	lastMain := -1
	for index := range steps {
		if steps[index].Kind != workoutStepCooldown {
			lastMain = index
		}
	}
	if lastMain < 0 || steps[lastMain].Kind != workoutStepRepeat || len(steps[lastMain].Children) == 0 {
		return
	}
	children := steps[lastMain].Children
	steps[lastMain].SkipLastRecovery = children[len(children)-1].Kind == workoutStepRecovery
}

func numberWorkoutSteps(steps []WorkoutStep) {
	order := 1
	var visit func([]WorkoutStep)
	visit = func(items []WorkoutStep) {
		for index := range items {
			items[index].Order = order
			order++
			visit(items[index].Children)
		}
	}
	visit(steps)
}

func estimateWorkoutDuration(steps []WorkoutStep) int {
	total := 0
	for _, step := range steps {
		if step.Kind == workoutStepRepeat {
			childDuration := estimateWorkoutDuration(step.Children)
			total += childDuration * step.RepeatCount
			if step.SkipLastRecovery && len(step.Children) > 0 {
				last := step.Children[len(step.Children)-1]
				if last.Kind == workoutStepRecovery && last.EndCondition != nil && last.EndCondition.Type == workoutEndTime {
					total -= int(last.EndCondition.Value)
				}
			}
			continue
		}
		if step.EndCondition != nil && step.EndCondition.Type == workoutEndTime {
			total += int(step.EndCondition.Value)
		}
	}
	return total
}

func workoutSourceHash(source string, table *trainingSheetWorkoutTable) string {
	groups := make([]string, 0)
	if table != nil {
		for _, row := range table.Rows {
			if row.Kind == trainingSheetRowExact && row.Group != "" {
				groups = append(groups, row.Group)
			}
		}
	}
	payload, _ := json.Marshal(struct {
		Source string   `json:"source"`
		Groups []string `json:"groups"`
	}{Source: strings.TrimSpace(source), Groups: groups})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func workoutParseFailure(result WorkoutParseResult, message, source string) WorkoutParseResult {
	result.Status = workoutParseError
	result.Messages = append(result.Messages, WorkoutParseMessage{Level: "error", Message: message, Source: source})
	return result
}

func splitWorkoutSequence(value string) []string {
	parts := splitWorkoutTopLevel(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		thenParts := regexp.MustCompile(`(?i)\s+then\s+`).Split(part, -1)
		result = append(result, thenParts...)
	}
	return result
}

func splitWorkoutTopLevel(value, separator string) []string {
	parts := make([]string, 0)
	depth, start := 0, 0
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && strings.HasPrefix(value[index:], separator) {
			parts = append(parts, strings.TrimSpace(value[start:index]))
			index += len(separator) - 1
			start = index + 1
		}
	}
	parts = append(parts, strings.TrimSpace(value[start:]))
	return parts
}

func splitTrailingWorkoutParenthesis(value string) (string, string) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasSuffix(trimmed, ")") {
		return trimmed, ""
	}
	depth := 0
	for index := len(trimmed) - 1; index >= 0; index-- {
		switch trimmed[index] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				inside := strings.TrimSpace(trimmed[index+1 : len(trimmed)-1])
				if strings.EqualFold(inside, "continuous") {
					return strings.TrimSpace(trimmed[:index]), ""
				}
				return strings.TrimSpace(trimmed[:index]), inside
			}
		}
	}
	return trimmed, ""
}

func intWorkoutPtr(value int) *int { return &value }
