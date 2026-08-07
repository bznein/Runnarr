package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

const (
	maxCourseLoopVariation         = 1_000_000
	maxCourseLoopCandidates        = 12
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

type CourseLoopGenerationRequest struct {
	SportType       CourseSport    `json:"sportType"`
	Start           CourseWaypoint `json:"start"`
	TargetDistanceM float64        `json:"targetDistanceM"`
	Variation       int            `json:"variation"`
}

type CourseLoopCandidate struct {
	ID string `json:"id"`
	CourseRoutingResponse
	Waypoints            []CourseWaypoint `json:"waypoints"`
	DistanceDeviationPct float64          `json:"distanceDeviationPct"`
	Warning              string           `json:"warning,omitempty"`
	retraceRatio         float64
	geometryCells        map[courseLoopCell]struct{}
}

type CourseLoopGenerationResponse struct {
	TargetDistanceM float64               `json:"targetDistanceM"`
	Variation       int                   `json:"variation"`
	Candidates      []CourseLoopCandidate `json:"candidates"`
}

type valhallaIsochroneResponse struct {
	Features []struct {
		Geometry struct {
			Type        string          `json:"type"`
			Coordinates json.RawMessage `json:"coordinates"`
		} `json:"geometry"`
		Properties struct {
			Contour float64 `json:"contour"`
		} `json:"properties"`
	} `json:"features"`
}

type courseLoopContour struct {
	distanceKM float64
	points     []CoursePoint
}

type courseLoopSpec struct {
	waypoints []CourseWaypoint
}

type courseLoopCell struct {
	latitude  int
	longitude int
}

func (service *CourseRoutingService) GenerateLoops(ctx context.Context, input CourseLoopGenerationRequest) (CourseLoopGenerationResponse, error) {
	if !service.enabled {
		return CourseLoopGenerationResponse{}, ErrCourseRoutingDisabled
	}
	if err := validateCourseLoopRequest(input); err != nil {
		return CourseLoopGenerationResponse{}, err
	}

	routingContext, cancel := context.WithTimeout(ctx, maxCourseLoopDuration)
	defer cancel()
	contours, err := service.courseLoopContours(routingContext, input)
	if err != nil {
		return CourseLoopGenerationResponse{}, err
	}
	specs := courseLoopSpecs(input, contours)
	if len(specs) == 0 {
		return CourseLoopGenerationResponse{}, ErrCourseLoopNotFound
	}

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
			case <-routingContext.Done():
				results <- candidateResult{err: routingContext.Err()}
				return
			}
			candidate, routeErr := service.routeCourseLoop(routingContext, input.SportType, input.TargetDistanceM, spec.waypoints)
			results <- candidateResult{candidate: candidate, err: routeErr}
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
	if err := routingContext.Err(); err != nil && len(candidates) == 0 {
		return CourseLoopGenerationResponse{}, err
	}
	if len(candidates) == 0 && upstreamErr != nil {
		return CourseLoopGenerationResponse{}, upstreamErr
	}
	selected := selectCourseLoopCandidates(candidates)
	if len(selected) == 0 {
		return CourseLoopGenerationResponse{}, ErrCourseLoopNotFound
	}
	for index := range selected {
		selected[index].ID = fmt.Sprintf("route-%d", index+1)
		service.enrichCourseLoopElevation(routingContext, input.SportType, &selected[index])
	}
	return CourseLoopGenerationResponse{TargetDistanceM: input.TargetDistanceM, Variation: input.Variation, Candidates: selected}, nil
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
	return nil
}

func (service *CourseRoutingService) courseLoopContours(ctx context.Context, input CourseLoopGenerationRequest) ([]courseLoopContour, error) {
	targetKM := input.TargetDistanceM / 1000
	distances := []float64{targetKM * 0.25, targetKM * 0.30, targetKM * 0.35}
	contourInputs := make([]map[string]float64, len(distances))
	for index, distance := range distances {
		contourInputs[index] = map[string]float64{"distance": distance}
	}
	generalizeM := math.Max(25, math.Min(250, input.TargetDistanceM/1000))
	body, err := json.Marshal(map[string]any{
		"locations":  []map[string]float64{{"lat": input.Start.Latitude, "lon": input.Start.Longitude}},
		"costing":    courseRoutingCosting(input.SportType),
		"contours":   contourInputs,
		"polygons":   false,
		"denoise":    0.5,
		"generalize": generalizeM,
	})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, service.baseURL+"/isochrone", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Runnarr course generator")
	response, err := service.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request routing isodistance: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("routing isodistance returned status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCourseRoutingResponse+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCourseRoutingResponse {
		return nil, errors.New("routing isodistance response exceeded the size limit")
	}
	var payload valhallaIsochroneResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode routing isodistance: %w", err)
	}
	contours := make([]courseLoopContour, 0, len(payload.Features))
	for _, feature := range payload.Features {
		points, decodeErr := decodeCourseLoopContour(feature.Geometry.Type, feature.Geometry.Coordinates)
		if decodeErr != nil || len(points) < 2 {
			continue
		}
		contours = append(contours, courseLoopContour{distanceKM: feature.Properties.Contour, points: points})
	}
	if len(contours) == 0 {
		return nil, ErrCourseLoopNotFound
	}
	sort.SliceStable(contours, func(left, right int) bool { return contours[left].distanceKM < contours[right].distanceKM })
	return contours, nil
}

func decodeCourseLoopContour(kind string, raw json.RawMessage) ([]CoursePoint, error) {
	coordinates := make([][]float64, 0)
	switch kind {
	case "LineString":
		if err := json.Unmarshal(raw, &coordinates); err != nil {
			return nil, err
		}
	case "MultiLineString":
		var lines [][][]float64
		if err := json.Unmarshal(raw, &lines); err != nil {
			return nil, err
		}
		for _, line := range lines {
			coordinates = append(coordinates, line...)
		}
	default:
		return nil, fmt.Errorf("unsupported isodistance geometry %q", kind)
	}
	points := make([]CoursePoint, 0, len(coordinates))
	for _, coordinate := range coordinates {
		if len(coordinate) < 2 || !validCourseCoordinate(coordinate[1], coordinate[0]) {
			continue
		}
		points = append(points, CoursePoint{Latitude: coordinate[1], Longitude: coordinate[0]})
	}
	return points, nil
}

func courseLoopSpecs(input CourseLoopGenerationRequest, contours []courseLoopContour) []courseLoopSpec {
	offset := math.Mod(float64(input.Variation)*137.50776405, 360)
	specs := make([]courseLoopSpec, 0, maxCourseLoopCandidates)
	seen := make(map[string]struct{})
	for contourIndex, contour := range contours {
		for headingIndex := 0; headingIndex < 4 && len(specs) < maxCourseLoopCandidates; headingIndex++ {
			heading := math.Mod(offset+float64(headingIndex)*90, 360)
			turn := 105.0
			if (contourIndex+headingIndex)%2 == 1 {
				turn = -turn
			}
			first, firstOK := courseLoopPointAtBearing(input.Start, contour.points, heading)
			second, secondOK := courseLoopPointAtBearing(input.Start, contour.points, math.Mod(heading+turn+360, 360))
			if !firstOK || !secondOK || haversine(first.Latitude, first.Longitude, second.Latitude, second.Longitude) < 100 {
				continue
			}
			key := fmt.Sprintf("%.5f,%.5f;%.5f,%.5f", first.Latitude, first.Longitude, second.Latitude, second.Longitude)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			start := CourseWaypoint{Index: 0, Latitude: input.Start.Latitude, Longitude: input.Start.Longitude}
			specs = append(specs, courseLoopSpec{waypoints: []CourseWaypoint{
				start,
				{Index: 1, Latitude: first.Latitude, Longitude: first.Longitude},
				{Index: 2, Latitude: second.Latitude, Longitude: second.Longitude},
				{Index: 3, Latitude: start.Latitude, Longitude: start.Longitude},
			}})
		}
	}
	return specs
}

func courseLoopPointAtBearing(start CourseWaypoint, points []CoursePoint, bearing float64) (CoursePoint, bool) {
	bestDifference := math.Inf(1)
	var best CoursePoint
	for _, point := range points {
		if haversine(start.Latitude, start.Longitude, point.Latitude, point.Longitude) < 100 {
			continue
		}
		difference := courseLoopBearingDifference(courseLoopBearing(start.Latitude, start.Longitude, point.Latitude, point.Longitude), bearing)
		if difference < bestDifference {
			bestDifference, best = difference, point
		}
	}
	return best, !math.IsInf(bestDifference, 1)
}

func courseLoopBearing(startLatitude, startLongitude, endLatitude, endLongitude float64) float64 {
	lat1, lat2 := startLatitude*math.Pi/180, endLatitude*math.Pi/180
	deltaLongitude := (endLongitude - startLongitude) * math.Pi / 180
	y := math.Sin(deltaLongitude) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(deltaLongitude)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}

func courseLoopBearingDifference(left, right float64) float64 {
	difference := math.Abs(left - right)
	return math.Min(difference, 360-difference)
}

func (service *CourseRoutingService) routeCourseLoop(ctx context.Context, sport CourseSport, targetDistanceM float64, waypoints []CourseWaypoint) (CourseLoopCandidate, error) {
	locations := make([]map[string]float64, len(waypoints))
	for index, waypoint := range waypoints {
		locations[index] = map[string]float64{"lat": waypoint.Latitude, "lon": waypoint.Longitude}
	}
	body, err := json.Marshal(map[string]any{
		"locations":    locations,
		"costing":      courseRoutingCosting(sport),
		"shape_format": "polyline6",
		"units":        "kilometers",
	})
	if err != nil {
		return CourseLoopCandidate{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, service.baseURL+"/route", bytes.NewReader(body))
	if err != nil {
		return CourseLoopCandidate{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Runnarr course generator")
	response, err := service.client.Do(request)
	if err != nil {
		return CourseLoopCandidate{}, fmt.Errorf("%w: %v", errCourseLoopUpstream, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode >= 500 {
			return CourseLoopCandidate{}, fmt.Errorf("%w: generated route returned status %d", errCourseLoopUpstream, response.StatusCode)
		}
		return CourseLoopCandidate{}, fmt.Errorf("generated route returned status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCourseRoutingResponse+1))
	if err != nil {
		return CourseLoopCandidate{}, err
	}
	if len(data) > maxCourseRoutingResponse {
		return CourseLoopCandidate{}, errors.New("generated route response exceeded the size limit")
	}
	var payload valhallaRouteResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return CourseLoopCandidate{}, err
	}
	if len(payload.Trip.Legs) != len(waypoints)-1 {
		return CourseLoopCandidate{}, errors.New("generated route did not contain the expected legs")
	}
	legs := make([]CourseRoutingLeg, 0, len(payload.Trip.Legs))
	totalPoints := 0
	for index, routedLeg := range payload.Trip.Legs {
		if routedLeg.Shape == "" {
			return CourseLoopCandidate{}, errors.New("generated route contained empty geometry")
		}
		points, decodeErr := decodeCoursePolyline(routedLeg.Shape, 6)
		if decodeErr != nil || len(points) < 2 {
			return CourseLoopCandidate{}, errors.New("generated route contained invalid geometry")
		}
		points[0].Latitude, points[0].Longitude = waypoints[index].Latitude, waypoints[index].Longitude
		last := len(points) - 1
		points[last].Latitude, points[last].Longitude = waypoints[index+1].Latitude, waypoints[index+1].Longitude
		points, decodeErr = normalizeCoursePoints(points)
		if decodeErr != nil {
			return CourseLoopCandidate{}, decodeErr
		}
		totalPoints += plannerLegPointContribution(index, len(points))
		if totalPoints > maxCoursePoints {
			return CourseLoopCandidate{}, errors.New("generated route exceeded the course point limit")
		}
		legs = append(legs, CourseRoutingLeg{CourseLeg: CourseLeg{
			Index: index, Mode: CourseLegRouted, Points: points, PointCount: len(points),
			EncodedPolyline: encodeCoursePolyline(points, 6), ElevationsM: courseElevations(points),
		}})
	}
	routing := CourseRoutingResponse{RoutingEnabled: true, Legs: legs}
	addCourseRoutingPreview(&routing)
	deviation := (routing.DistanceM - targetDistanceM) / targetDistanceM * 100
	cells, retracing := courseLoopGeometryCells(legs)
	return CourseLoopCandidate{
		CourseRoutingResponse: routing,
		Waypoints:             waypoints,
		DistanceDeviationPct:  deviation,
		retraceRatio:          retracing,
		geometryCells:         cells,
	}, nil
}

func courseRoutingCosting(sport CourseSport) string {
	if sport == CourseSportCycling {
		return "bicycle"
	}
	return "pedestrian"
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
			latitude := start.Latitude + (end.Latitude-start.Latitude)*ratio
			longitude := start.Longitude + (end.Longitude-start.Longitude)*ratio
			cell := courseLoopCellFor(latitude, longitude)
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

func selectCourseLoopCandidates(candidates []CourseLoopCandidate) []CourseLoopCandidate {
	preferred := make([]CourseLoopCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if math.Abs(candidate.DistanceDeviationPct) <= preferredCourseLoopDeviation {
			preferred = append(preferred, candidate)
		}
	}
	if len(preferred) == 0 {
		sort.SliceStable(candidates, func(left, right int) bool {
			return math.Abs(candidates[left].DistanceDeviationPct) < math.Abs(candidates[right].DistanceDeviationPct)
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
		leftDeviation := math.Abs(preferred[left].DistanceDeviationPct)
		rightDeviation := math.Abs(preferred[right].DistanceDeviationPct)
		if leftDeviation != rightDeviation {
			return leftDeviation < rightDeviation
		}
		return preferred[left].retraceRatio < preferred[right].retraceRatio
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

func (service *CourseRoutingService) enrichCourseLoopElevation(ctx context.Context, sport CourseSport, candidate *CourseLoopCandidate) {
	points := make([]CoursePoint, 0)
	for _, leg := range candidate.Legs {
		points = append(points, leg.Points...)
	}
	known, err := service.addElevations(ctx, points)
	warning := ""
	if err != nil {
		warning = "Elevation data is unavailable for this leg."
		if service.logger != nil {
			service.logger.Warn("generated course elevation unavailable", "sport", sport, "error", err)
		}
	} else if known < len(points) {
		warning = "Elevation data is incomplete for this leg."
	}
	offset := 0
	for index := range candidate.Legs {
		count := len(candidate.Legs[index].Points)
		copy(candidate.Legs[index].Points, points[offset:offset+count])
		candidate.Legs[index].ElevationsM = courseElevations(candidate.Legs[index].Points)
		candidate.Legs[index].Warning = warning
		offset += count
	}
	addCourseRoutingPreview(&candidate.CourseRoutingResponse)
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
