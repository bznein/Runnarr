package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

const (
	maxCourseLoopVariation         = 1_000_000
	primaryCourseLoopCandidates    = 8
	correctedCourseLoopCandidates  = 4
	maxCourseLoopResults           = 3
	maxCourseLoopConcurrency       = 4
	maxCourseLoopDuration          = 30 * time.Second
	preferredCourseLoopDeviation   = 10.0
	maximumCourseLoopDeviation     = 20.0
	preferredCourseLoopRetracing   = 0.35
	maximumCourseLoopResultOverlap = 0.70
	courseLoopSampleSpacingM       = 25.0
)

var (
	ErrCourseRoutingDisabled = errors.New("course routing is not enabled")
	ErrCourseLoopNotFound    = errors.New("no generated course matched the requested distance")
	errCourseLoopUpstream    = errors.New("course loop routing service failed")
)

type CourseLoopHilliness string

const (
	CourseLoopHillinessFlat     CourseLoopHilliness = "flat"
	CourseLoopHillinessBalanced CourseLoopHilliness = "balanced"
	CourseLoopHillinessHilly    CourseLoopHilliness = "hilly"
)

type CourseLoopGenerationRequest struct {
	SportType       CourseSport         `json:"sportType"`
	Start           CourseWaypoint      `json:"start"`
	TargetDistanceM float64             `json:"targetDistanceM"`
	Variation       int                 `json:"variation"`
	Hilliness       CourseLoopHilliness `json:"hilliness,omitempty"`
}

type CourseLoopCandidate struct {
	ID string `json:"id"`
	CourseRoutingResponse
	Waypoints            []CourseWaypoint `json:"waypoints"`
	DistanceDeviationPct float64          `json:"distanceDeviationPct"`
	Warning              string           `json:"warning,omitempty"`
	seed                 int64
	retraceRatio         float64
	geometryCells        map[courseLoopCell]struct{}
}

type CourseLoopGenerationResponse struct {
	TargetDistanceM float64               `json:"targetDistanceM"`
	Variation       int                   `json:"variation"`
	Hilliness       CourseLoopHilliness   `json:"hilliness"`
	Candidates      []CourseLoopCandidate `json:"candidates"`
}

type courseLoopRequestSpec struct {
	seed      int64
	distanceM float64
}

type courseLoopCell struct {
	latitude  int
	longitude int
}

func (service *CourseRoutingService) GenerateLoops(ctx context.Context, input CourseLoopGenerationRequest) (CourseLoopGenerationResponse, error) {
	if !service.enabled {
		return CourseLoopGenerationResponse{}, ErrCourseRoutingDisabled
	}
	if input.Hilliness == "" {
		input.Hilliness = CourseLoopHillinessBalanced
	}
	if err := validateCourseLoopRequest(input); err != nil {
		return CourseLoopGenerationResponse{}, err
	}

	routingContext, cancel := context.WithTimeout(ctx, maxCourseLoopDuration)
	defer cancel()
	primary := make([]courseLoopRequestSpec, primaryCourseLoopCandidates)
	for index := range primary {
		primary[index] = courseLoopRequestSpec{
			seed:      int64(input.Variation)*1_000_003 + int64(index)*104_729 + 17,
			distanceM: input.TargetDistanceM,
		}
	}
	candidates, upstreamErr := service.generateCourseLoopBatch(routingContext, input, primary)

	preferred := 0
	for _, candidate := range candidates {
		if math.Abs(candidate.DistanceDeviationPct) <= preferredCourseLoopDeviation {
			preferred++
		}
	}
	if preferred < maxCourseLoopResults && len(candidates) > 0 {
		sort.SliceStable(candidates, func(left, right int) bool {
			leftDeviation := math.Abs(candidates[left].DistanceDeviationPct)
			rightDeviation := math.Abs(candidates[right].DistanceDeviationPct)
			if leftDeviation != rightDeviation {
				return leftDeviation < rightDeviation
			}
			return candidates[left].seed < candidates[right].seed
		})
		count := min(correctedCourseLoopCandidates, len(candidates))
		corrections := make([]courseLoopRequestSpec, 0, count)
		for index := 0; index < count; index++ {
			actualDistance := candidates[index].DistanceM
			if actualDistance <= 0 {
				continue
			}
			factor := math.Max(0.5, math.Min(1.5, input.TargetDistanceM/actualDistance))
			corrections = append(corrections, courseLoopRequestSpec{
				seed:      candidates[index].seed,
				distanceM: input.TargetDistanceM * factor,
			})
		}
		corrected, correctionErr := service.generateCourseLoopBatch(routingContext, input, corrections)
		candidates = append(candidates, corrected...)
		if upstreamErr == nil {
			upstreamErr = correctionErr
		}
	}
	if err := routingContext.Err(); err != nil && len(candidates) == 0 {
		return CourseLoopGenerationResponse{}, err
	}
	if len(candidates) == 0 && upstreamErr != nil {
		return CourseLoopGenerationResponse{}, upstreamErr
	}
	selected := selectCourseLoopCandidates(candidates, input.Hilliness)
	if len(selected) == 0 {
		return CourseLoopGenerationResponse{}, ErrCourseLoopNotFound
	}
	for index := range selected {
		selected[index].ID = fmt.Sprintf("route-%d", index+1)
	}
	return CourseLoopGenerationResponse{
		TargetDistanceM: input.TargetDistanceM,
		Variation:       input.Variation,
		Hilliness:       input.Hilliness,
		Candidates:      selected,
	}, nil
}

func validateCourseLoopRequest(input CourseLoopGenerationRequest) error {
	if !validCourseSport(input.SportType) {
		return fmt.Errorf("%w: sport must be Run, Walk, Hike, or Cycling", ErrCourseInvalid)
	}
	if !validCourseCoordinate(input.Start.Latitude, input.Start.Longitude) {
		return fmt.Errorf("%w: start must contain finite latitude/longitude values", ErrCourseInvalid)
	}
	minimum, maximum := 1000.0, 100000.0
	if input.SportType == CourseSportCycling {
		minimum, maximum = 5000, 300000
	}
	if math.IsNaN(input.TargetDistanceM) || math.IsInf(input.TargetDistanceM, 0) || input.TargetDistanceM < minimum || input.TargetDistanceM > maximum {
		return fmt.Errorf("%w: target distance for %s must be between %.0f and %.0f km", ErrCourseInvalid, input.SportType, minimum/1000, maximum/1000)
	}
	if input.Variation < 0 || input.Variation > maxCourseLoopVariation {
		return fmt.Errorf("%w: variation must be between 0 and %d", ErrCourseInvalid, maxCourseLoopVariation)
	}
	if input.Hilliness != CourseLoopHillinessFlat && input.Hilliness != CourseLoopHillinessBalanced && input.Hilliness != CourseLoopHillinessHilly {
		return fmt.Errorf("%w: hilliness must be flat, balanced, or hilly", ErrCourseInvalid)
	}
	return nil
}

func (service *CourseRoutingService) generateCourseLoopBatch(ctx context.Context, input CourseLoopGenerationRequest, specs []courseLoopRequestSpec) ([]CourseLoopCandidate, error) {
	type candidateResult struct {
		candidate CourseLoopCandidate
		err       error
	}
	results := make(chan candidateResult, len(specs))
	semaphore := make(chan struct{}, maxCourseLoopConcurrency)
	var wait sync.WaitGroup
	for _, spec := range specs {
		spec := spec
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results <- candidateResult{err: ctx.Err()}
				return
			}
			candidate, err := service.routeCourseLoop(ctx, input, spec)
			results <- candidateResult{candidate: candidate, err: err}
		}()
	}
	wait.Wait()
	close(results)
	candidates := make([]CourseLoopCandidate, 0, len(specs))
	var upstreamErr error
	for result := range results {
		if result.err == nil {
			candidates = append(candidates, result.candidate)
		} else if errors.Is(result.err, errCourseLoopUpstream) {
			upstreamErr = result.err
		}
	}
	return candidates, upstreamErr
}

func (service *CourseRoutingService) routeCourseLoop(ctx context.Context, input CourseLoopGenerationRequest, spec courseLoopRequestSpec) (CourseLoopCandidate, error) {
	roundTrip := &graphHopperRoundTrip{DistanceM: spec.distanceM, Seed: spec.seed}
	points, _, err := service.graphHopperRoute(ctx, graphHopperRouteRequest{
		Points:        [][]float64{{input.Start.Longitude, input.Start.Latitude}},
		Profile:       courseRoutingProfile(input.SportType),
		PointsEncoded: false,
		Elevation:     true,
		Instructions:  false,
		Algorithm:     "round_trip",
		RoundTrip:     roundTrip,
		CHDisabled:    true,
		CustomModel:   courseLoopCustomModel(input.Hilliness),
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return CourseLoopCandidate{}, err
		}
		var statusErr graphHopperHTTPError
		if errors.As(err, &statusErr) && statusErr.status < 500 {
			return CourseLoopCandidate{}, err
		}
		return CourseLoopCandidate{}, fmt.Errorf("%w: %v", errCourseLoopUpstream, err)
	}
	candidate, err := buildCourseLoopCandidate(points, input.Start, input.TargetDistanceM)
	candidate.seed = spec.seed
	return candidate, err
}

func courseLoopCustomModel(hilliness CourseLoopHilliness) *graphHopperCustomModel {
	switch hilliness {
	case CourseLoopHillinessFlat:
		return &graphHopperCustomModel{Priority: []graphHopperRule{
			{If: "average_slope >= 6 || average_slope <= -6", MultiplyBy: 0.15},
			{ElseIf: "average_slope >= 3 || average_slope <= -3", MultiplyBy: 0.45},
		}}
	case CourseLoopHillinessHilly:
		return &graphHopperCustomModel{Priority: []graphHopperRule{
			{If: "average_slope > -2 && average_slope < 2", MultiplyBy: 0.25},
			{ElseIf: "average_slope > -4 && average_slope < 4", MultiplyBy: 0.60},
		}}
	default:
		return nil
	}
}

func buildCourseLoopCandidate(points []CoursePoint, start CourseWaypoint, targetDistanceM float64) (CourseLoopCandidate, error) {
	if len(points) < 4 {
		return CourseLoopCandidate{}, errors.New("generated route contained too few points")
	}
	points[0].Latitude, points[0].Longitude = start.Latitude, start.Longitude
	last := len(points) - 1
	points[last].Latitude, points[last].Longitude = start.Latitude, start.Longitude
	points, err := normalizeCoursePoints(points)
	if err != nil || len(points) < 4 {
		return CourseLoopCandidate{}, errors.New("generated route contained invalid geometry")
	}
	firstSplit, secondSplit := courseLoopSplitIndexes(points)
	if firstSplit <= 0 || secondSplit <= firstSplit || secondSplit >= len(points)-1 {
		return CourseLoopCandidate{}, errors.New("generated route could not be made editable")
	}
	waypoints := []CourseWaypoint{
		{Index: 0, Latitude: start.Latitude, Longitude: start.Longitude},
		{Index: 1, Latitude: points[firstSplit].Latitude, Longitude: points[firstSplit].Longitude},
		{Index: 2, Latitude: points[secondSplit].Latitude, Longitude: points[secondSplit].Longitude},
		{Index: 3, Latitude: start.Latitude, Longitude: start.Longitude},
	}
	segments := [][]CoursePoint{points[:firstSplit+1], points[firstSplit : secondSplit+1], points[secondSplit:]}
	legs := make([]CourseRoutingLeg, 0, len(segments))
	for index, segment := range segments {
		legPoints := append([]CoursePoint(nil), segment...)
		legs = append(legs, CourseRoutingLeg{CourseLeg: CourseLeg{
			Index:           index,
			Mode:            CourseLegRouted,
			Points:          legPoints,
			PointCount:      len(legPoints),
			EncodedPolyline: encodeCoursePolyline(legPoints, 6),
			ElevationsM:     courseElevations(legPoints),
		}})
	}
	routing := CourseRoutingResponse{RoutingEnabled: true, Legs: legs}
	addCourseRoutingPreview(&routing)
	warning := ""
	if routing.ElevationCoverage == 0 {
		warning = "Elevation data is unavailable for this route."
	} else if routing.ElevationCoverage < 1 {
		warning = "Elevation data is incomplete for this route."
	}
	cells, retracing := courseLoopGeometryCells(legs)
	return CourseLoopCandidate{
		CourseRoutingResponse: routing,
		Waypoints:             waypoints,
		DistanceDeviationPct:  (routing.DistanceM - targetDistanceM) / targetDistanceM * 100,
		Warning:               warning,
		retraceRatio:          retracing,
		geometryCells:         cells,
	}, nil
}

func courseLoopSplitIndexes(points []CoursePoint) (int, int) {
	cumulative := make([]float64, len(points))
	for index := 1; index < len(points); index++ {
		cumulative[index] = cumulative[index-1] + haversine(points[index-1].Latitude, points[index-1].Longitude, points[index].Latitude, points[index].Longitude)
	}
	total := cumulative[len(cumulative)-1]
	nearest := func(target float64, minimum, maximum int) int {
		best, difference := minimum, math.Inf(1)
		for index := minimum; index <= maximum; index++ {
			if value := math.Abs(cumulative[index] - target); value < difference {
				best, difference = index, value
			}
		}
		return best
	}
	first := nearest(total/3, 1, len(points)-3)
	second := nearest(total*2/3, first+1, len(points)-2)
	return first, second
}

func courseLoopGeometryCells(legs []CourseRoutingLeg) (map[courseLoopCell]struct{}, float64) {
	points := make([]CoursePoint, 0)
	for index, leg := range legs {
		if index == 0 {
			points = append(points, leg.Points...)
		} else if len(leg.Points) > 1 {
			points = append(points, leg.Points[1:]...)
		}
	}
	cells := make(map[courseLoopCell]struct{})
	lastSeen := make(map[courseLoopCell]int)
	sampleIndex, repeated := 0, 0
	var previous *courseLoopCell
	for index := 1; index < len(points); index++ {
		start, end := points[index-1], points[index]
		steps := int(math.Max(1, math.Ceil(haversine(start.Latitude, start.Longitude, end.Latitude, end.Longitude)/courseLoopSampleSpacingM)))
		for step := 0; step <= steps; step++ {
			ratio := float64(step) / float64(steps)
			cell := courseLoopCellFor(start.Latitude+(end.Latitude-start.Latitude)*ratio, start.Longitude+(end.Longitude-start.Longitude)*ratio)
			if previous != nil && *previous == cell {
				continue
			}
			if seenAt, exists := lastSeen[cell]; exists && sampleIndex-seenAt > 2 {
				repeated++
			}
			cells[cell] = struct{}{}
			lastSeen[cell] = sampleIndex
			copy := cell
			previous = &copy
			sampleIndex++
		}
	}
	if sampleIndex == 0 {
		return cells, 1
	}
	return cells, float64(repeated) / float64(sampleIndex)
}

func courseLoopCellFor(latitude, longitude float64) courseLoopCell {
	latitudeCellM := courseLoopSampleSpacingM / 111320
	longitudeScale := math.Max(0.1, math.Cos(latitude*math.Pi/180))
	longitudeCellM := courseLoopSampleSpacingM / (111320 * longitudeScale)
	return courseLoopCell{latitude: int(math.Round(latitude / latitudeCellM)), longitude: int(math.Round(longitude / longitudeCellM))}
}

func selectCourseLoopCandidates(candidates []CourseLoopCandidate, hilliness CourseLoopHilliness) []CourseLoopCandidate {
	preferred := make([]CourseLoopCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if math.Abs(candidate.DistanceDeviationPct) <= preferredCourseLoopDeviation {
			preferred = append(preferred, candidate)
		}
	}
	if len(preferred) == 0 {
		sort.SliceStable(candidates, func(left, right int) bool {
			leftDeviation := math.Abs(candidates[left].DistanceDeviationPct)
			rightDeviation := math.Abs(candidates[right].DistanceDeviationPct)
			if leftDeviation != rightDeviation {
				return leftDeviation < rightDeviation
			}
			return candidates[left].seed < candidates[right].seed
		})
		if len(candidates) == 0 || math.Abs(candidates[0].DistanceDeviationPct) > maximumCourseLoopDeviation {
			return nil
		}
		fallback := candidates[0]
		direction := "longer"
		if fallback.DistanceDeviationPct < 0 {
			direction = "shorter"
		}
		fallback.Warning = fmt.Sprintf("Closest generated route is %.0f%% %s than requested.", math.Abs(fallback.DistanceDeviationPct), direction)
		return []CourseLoopCandidate{fallback}
	}
	sort.SliceStable(preferred, func(left, right int) bool {
		leftLoop := preferred[left].retraceRatio < preferredCourseLoopRetracing
		rightLoop := preferred[right].retraceRatio < preferredCourseLoopRetracing
		if leftLoop != rightLoop {
			return leftLoop
		}
		leftIntensity, leftKnown := courseLoopElevationIntensity(preferred[left])
		rightIntensity, rightKnown := courseLoopElevationIntensity(preferred[right])
		if hilliness != CourseLoopHillinessBalanced && leftKnown != rightKnown {
			return leftKnown
		}
		if leftKnown && rightKnown && leftIntensity != rightIntensity {
			if hilliness == CourseLoopHillinessFlat {
				return leftIntensity < rightIntensity
			}
			if hilliness == CourseLoopHillinessHilly {
				return leftIntensity > rightIntensity
			}
		}
		leftDeviation := math.Abs(preferred[left].DistanceDeviationPct)
		rightDeviation := math.Abs(preferred[right].DistanceDeviationPct)
		if leftDeviation != rightDeviation {
			return leftDeviation < rightDeviation
		}
		if preferred[left].retraceRatio != preferred[right].retraceRatio {
			return preferred[left].retraceRatio < preferred[right].retraceRatio
		}
		return preferred[left].seed < preferred[right].seed
	})
	selected := make([]CourseLoopCandidate, 0, maxCourseLoopResults)
	for _, candidate := range preferred {
		if courseLoopOverlapsSelected(candidate, selected) {
			continue
		}
		selected = append(selected, candidate)
		if len(selected) == maxCourseLoopResults {
			break
		}
	}
	return selected
}

func courseLoopElevationIntensity(candidate CourseLoopCandidate) (float64, bool) {
	if candidate.ElevationGainM == nil || candidate.DistanceM <= 0 || candidate.ElevationCoverage < 0.95 {
		return 0, false
	}
	return *candidate.ElevationGainM / (candidate.DistanceM / 1000), true
}

func courseLoopOverlapsSelected(candidate CourseLoopCandidate, selected []CourseLoopCandidate) bool {
	for _, other := range selected {
		minimum := min(len(candidate.geometryCells), len(other.geometryCells))
		if minimum == 0 {
			continue
		}
		shared := 0
		for cell := range candidate.geometryCells {
			if _, exists := other.geometryCells[cell]; exists {
				shared++
			}
		}
		if float64(shared)/float64(minimum) > maximumCourseLoopResultOverlap {
			return true
		}
	}
	return false
}

func (s *Server) handleGenerateCourseLoops(w http.ResponseWriter, r *http.Request) {
	var input CourseLoopGenerationRequest
	if err := decodeJSONBody(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	response, err := s.courseRouting.GenerateLoops(r.Context(), input)
	switch {
	case errors.Is(err, ErrCourseInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrCourseRoutingDisabled):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ErrCourseLoopNotFound):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case err != nil:
		s.logger.Error("generate course loops", "error", err)
		writeError(w, http.StatusBadGateway, "could not generate course loops")
	default:
		writeJSON(w, http.StatusOK, response)
	}
}
