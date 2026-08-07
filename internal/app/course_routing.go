package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"time"
)

const (
	maxCourseRoutingWaypoints = 100
	maxCourseRoutingResponse  = 4 << 20
	maxCourseRoutingDuration  = 20 * time.Second
)

type CourseRoutingService struct {
	enabled bool
	baseURL string
	client  *http.Client
	logger  *slog.Logger
}

type CourseRoutingRequest struct {
	SportType        CourseSport      `json:"sportType"`
	Waypoints        []CourseWaypoint `json:"waypoints"`
	DirectLegIndexes []int            `json:"directLegIndexes,omitempty"`
}

type CourseRoutingLeg struct {
	CourseLeg
	Warning string `json:"warning,omitempty"`
}

type CourseRoutingResponse struct {
	RoutingEnabled    bool                 `json:"routingEnabled"`
	Legs              []CourseRoutingLeg   `json:"legs"`
	DistanceM         float64              `json:"distanceM"`
	ElevationGainM    *float64             `json:"elevationGainM,omitempty"`
	ElevationLossM    *float64             `json:"elevationLossM,omitempty"`
	ElevationCoverage float64              `json:"elevationCoverage"`
	Profile           []CourseProfilePoint `json:"profile"`
}

type valhallaRouteResponse struct {
	Trip struct {
		Legs []struct {
			Shape string `json:"shape"`
		} `json:"legs"`
	} `json:"trip"`
}

type valhallaHeightResponse struct {
	Height []*float64 `json:"height"`
}

func NewCourseRoutingService(cfg Config, logger *slog.Logger) *CourseRoutingService {
	return &CourseRoutingService{
		enabled: cfg.RoutingEnabled,
		baseURL: cfg.RoutingURL,
		client:  &http.Client{Timeout: 8 * time.Second},
		logger:  logger,
	}
}

func (service *CourseRoutingService) Route(ctx context.Context, input CourseRoutingRequest) (CourseRoutingResponse, error) {
	if !validCourseSport(input.SportType) {
		return CourseRoutingResponse{}, fmt.Errorf("%w: sport must be Run, Walk, Hike, or Cycling", ErrCourseInvalid)
	}
	if len(input.Waypoints) < 2 || len(input.Waypoints) > maxCourseRoutingWaypoints {
		return CourseRoutingResponse{}, fmt.Errorf("%w: planner requires between 2 and %d waypoints", ErrCourseInvalid, maxCourseRoutingWaypoints)
	}
	for _, waypoint := range input.Waypoints {
		if !validCourseCoordinate(waypoint.Latitude, waypoint.Longitude) {
			return CourseRoutingResponse{}, fmt.Errorf("%w: waypoints must contain finite latitude/longitude values", ErrCourseInvalid)
		}
	}
	for _, index := range input.DirectLegIndexes {
		if index < 0 || index >= len(input.Waypoints)-1 {
			return CourseRoutingResponse{}, fmt.Errorf("%w: direct leg index is out of range", ErrCourseInvalid)
		}
	}

	response := CourseRoutingResponse{RoutingEnabled: service.enabled, Legs: make([]CourseRoutingLeg, 0, len(input.Waypoints)-1)}
	routingContext, cancel := context.WithTimeout(ctx, maxCourseRoutingDuration)
	defer cancel()
	totalPoints := 0
	for index := 0; index < len(input.Waypoints)-1; index++ {
		start, end := input.Waypoints[index], input.Waypoints[index+1]
		if slices.Contains(input.DirectLegIndexes, index) {
			response.Legs = append(response.Legs, directCourseRoutingLeg(index, start, end, ""))
			totalPoints += plannerLegPointContribution(index, 2)
			continue
		}
		if !service.enabled {
			response.Legs = append(response.Legs, directCourseRoutingLeg(index, start, end, "Routing is not configured; this leg is direct."))
			totalPoints += plannerLegPointContribution(index, 2)
			continue
		}
		leg, err := service.routeLeg(routingContext, index, input.SportType, start, end)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return CourseRoutingResponse{}, err
			}
			if service.logger != nil {
				service.logger.Warn("course routing leg fell back to direct", "leg_index", index, "sport", input.SportType, "error", err)
			}
			response.Legs = append(response.Legs, directCourseRoutingLeg(index, start, end, "No routed path was found; this leg is direct."))
			totalPoints += plannerLegPointContribution(index, 2)
			continue
		}
		projected := totalPoints + plannerLegPointContribution(index, leg.PointCount)
		remainingMinimum := len(input.Waypoints) - index - 2
		if projected+remainingMinimum > maxCoursePoints {
			response.Legs = append(response.Legs, directCourseRoutingLeg(index, start, end, "Routed geometry exceeded the course point limit; this leg is direct."))
			totalPoints += plannerLegPointContribution(index, 2)
			continue
		}
		response.Legs = append(response.Legs, leg)
		totalPoints = projected
	}
	addCourseRoutingPreview(&response)
	return response, nil
}

func addCourseRoutingPreview(response *CourseRoutingResponse) {
	points := make([]CoursePoint, 0)
	for index, leg := range response.Legs {
		if index == 0 {
			points = append(points, leg.Points...)
			continue
		}
		points = append(points, leg.Points[1:]...)
	}
	if len(points) < 2 {
		return
	}
	distance, coverage, gain, loss, profile := courseGeometryMetrics(points)
	response.DistanceM = distance
	response.ElevationCoverage = coverage
	response.ElevationGainM = gain
	response.ElevationLossM = loss
	response.Profile = boundedCourseProfile(profile, maxCourseProfilePoints)
}

func plannerLegPointContribution(index, pointCount int) int {
	if index == 0 {
		return pointCount
	}
	return pointCount - 1
}

func (service *CourseRoutingService) routeLeg(ctx context.Context, index int, sport CourseSport, start, end CourseWaypoint) (CourseRoutingLeg, error) {
	body, err := json.Marshal(map[string]any{
		"locations": []map[string]float64{
			{"lat": start.Latitude, "lon": start.Longitude},
			{"lat": end.Latitude, "lon": end.Longitude},
		},
		"costing":      courseRoutingCosting(sport),
		"shape_format": "polyline6",
		"units":        "kilometers",
	})
	if err != nil {
		return CourseRoutingLeg{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, service.baseURL+"/route", bytes.NewReader(body))
	if err != nil {
		return CourseRoutingLeg{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Runnarr course planner")
	response, err := service.client.Do(request)
	if err != nil {
		return CourseRoutingLeg{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return CourseRoutingLeg{}, fmt.Errorf("routing service returned status %d", response.StatusCode)
	}
	var payload valhallaRouteResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxCourseRoutingResponse+1))
	if err := decoder.Decode(&payload); err != nil {
		return CourseRoutingLeg{}, fmt.Errorf("decode routing response: %w", err)
	}
	if len(payload.Trip.Legs) != 1 || payload.Trip.Legs[0].Shape == "" {
		return CourseRoutingLeg{}, errors.New("routing response did not contain one leg")
	}
	points, err := decodeCoursePolyline(payload.Trip.Legs[0].Shape, 6)
	if err != nil || len(points) > maxCoursePoints {
		return CourseRoutingLeg{}, errors.New("routing response contained invalid geometry")
	}
	// Valhalla snaps locations to its graph. Keep the user's exact waypoint at
	// each boundary so independently routed legs remain continuous and editable.
	points[0].Latitude, points[0].Longitude = start.Latitude, start.Longitude
	last := len(points) - 1
	points[last].Latitude, points[last].Longitude = end.Latitude, end.Longitude
	points, err = normalizeCoursePoints(points)
	if err != nil {
		return CourseRoutingLeg{}, err
	}
	warning := ""
	knownElevations, elevationErr := service.addElevations(ctx, points)
	if elevationErr != nil {
		warning = "Elevation data is unavailable for this leg."
		if service.logger != nil {
			service.logger.Warn("course routing elevation unavailable", "leg_index", index, "sport", sport, "error", elevationErr)
		}
	} else if knownElevations < len(points) {
		warning = "Elevation data is incomplete for this leg."
	}
	leg := CourseLeg{
		Index:           index,
		Mode:            CourseLegRouted,
		Points:          points,
		PointCount:      len(points),
		EncodedPolyline: encodeCoursePolyline(points, 6),
		ElevationsM:     courseElevations(points),
	}
	return CourseRoutingLeg{CourseLeg: leg, Warning: warning}, nil
}

func (service *CourseRoutingService) addElevations(ctx context.Context, points []CoursePoint) (int, error) {
	body, err := json.Marshal(map[string]any{
		"encoded_polyline": encodeCoursePolyline(points, 6),
		"shape_format":     "polyline6",
		"height_precision": 1,
	})
	if err != nil {
		return 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, service.baseURL+"/height", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Runnarr course planner")
	response, err := service.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return 0, fmt.Errorf("elevation service returned status %d", response.StatusCode)
	}
	var payload valhallaHeightResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxCourseRoutingResponse+1))
	if err := decoder.Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode elevation response: %w", err)
	}
	if len(payload.Height) != len(points) {
		return 0, errors.New("elevation response did not align with routed geometry")
	}
	known := 0
	for index, elevation := range payload.Height {
		if elevation == nil || math.IsNaN(*elevation) || math.IsInf(*elevation, 0) {
			continue
		}
		points[index].ElevationM = cloneFloat(elevation)
		known++
	}
	if known == 0 {
		return 0, errors.New("elevation response contained no usable heights")
	}
	return known, nil
}

func directCourseRoutingLeg(index int, start, end CourseWaypoint, warning string) CourseRoutingLeg {
	points := []CoursePoint{{Latitude: start.Latitude, Longitude: start.Longitude}, {Latitude: end.Latitude, Longitude: end.Longitude}}
	return CourseRoutingLeg{CourseLeg: CourseLeg{
		Index:           index,
		Mode:            CourseLegDirect,
		EncodedPolyline: encodeCoursePolyline(points, 6),
		ElevationsM:     courseElevations(points),
		PointCount:      len(points),
		Points:          points,
	}, Warning: warning}
}

func (s *Server) handleRouteCourseLegs(w http.ResponseWriter, r *http.Request) {
	var input CourseRoutingRequest
	if err := decodeJSONBody(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	response, err := s.courseRouting.Route(r.Context(), input)
	if errors.Is(err, ErrCourseInvalid) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		s.logger.Error("route course legs", "error", err)
		writeError(w, http.StatusBadGateway, "could not route course legs")
		return
	}
	writeJSON(w, http.StatusOK, response)
}
