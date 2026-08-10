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

type graphHopperRouteRequest struct {
	Points        [][]float64             `json:"points"`
	Profile       string                  `json:"profile"`
	PointsEncoded bool                    `json:"points_encoded"`
	Elevation     bool                    `json:"elevation"`
	Instructions  bool                    `json:"instructions"`
	Algorithm     string                  `json:"algorithm,omitempty"`
	RoundTrip     *graphHopperRoundTrip   `json:"round_trip,omitempty"`
	CHDisabled    bool                    `json:"ch.disable,omitempty"`
	CustomModel   *graphHopperCustomModel `json:"custom_model,omitempty"`
}

type graphHopperRoundTrip struct {
	DistanceM float64 `json:"distance"`
	Seed      int64   `json:"seed"`
}

type graphHopperCustomModel struct {
	Priority []graphHopperRule `json:"priority,omitempty"`
}

type graphHopperRule struct {
	If         string  `json:"if,omitempty"`
	ElseIf     string  `json:"else_if,omitempty"`
	MultiplyBy float64 `json:"multiply_by"`
}

type graphHopperRouteResponse struct {
	Paths []struct {
		DistanceM float64 `json:"distance"`
		AscendM   float64 `json:"ascend"`
		DescendM  float64 `json:"descend"`
		Points    struct {
			Type        string            `json:"type"`
			Coordinates []json.RawMessage `json:"coordinates"`
		} `json:"points"`
	} `json:"paths"`
}

type graphHopperHTTPError struct {
	status int
}

func (err graphHopperHTTPError) Error() string {
	return fmt.Sprintf("routing service returned status %d", err.status)
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
	points, knownElevations, err := service.graphHopperRoute(ctx, graphHopperRouteRequest{
		Points:        [][]float64{{start.Longitude, start.Latitude}, {end.Longitude, end.Latitude}},
		Profile:       courseRoutingProfile(sport),
		PointsEncoded: false,
		Elevation:     true,
		Instructions:  false,
	})
	if err != nil {
		return CourseRoutingLeg{}, err
	}
	// GraphHopper snaps locations to its graph. Keep the user's exact waypoint at
	// each boundary so independently routed legs remain continuous and editable.
	points[0].Latitude, points[0].Longitude = start.Latitude, start.Longitude
	last := len(points) - 1
	points[last].Latitude, points[last].Longitude = end.Latitude, end.Longitude
	points, err = normalizeCoursePoints(points)
	if err != nil {
		return CourseRoutingLeg{}, err
	}
	warning := ""
	if knownElevations == 0 {
		warning = "Elevation data is unavailable for this leg."
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

func (service *CourseRoutingService) graphHopperRoute(ctx context.Context, input graphHopperRouteRequest) ([]CoursePoint, int, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, service.baseURL+"/route", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Runnarr course planner")
	response, err := service.client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, 0, graphHopperHTTPError{status: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCourseRoutingResponse+1))
	if err != nil {
		return nil, 0, err
	}
	if len(data) > maxCourseRoutingResponse {
		return nil, 0, errors.New("routing response exceeded the size limit")
	}
	var payload graphHopperRouteResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, 0, fmt.Errorf("decode routing response: %w", err)
	}
	if len(payload.Paths) == 0 || payload.Paths[0].Points.Type != "LineString" {
		return nil, 0, errors.New("routing response did not contain a path")
	}
	coordinates := payload.Paths[0].Points.Coordinates
	if len(coordinates) < 2 || len(coordinates) > maxCoursePoints {
		return nil, 0, errors.New("routing response contained invalid geometry")
	}
	points := make([]CoursePoint, 0, len(coordinates))
	known := 0
	for _, raw := range coordinates {
		var coordinate []*float64
		if err := json.Unmarshal(raw, &coordinate); err != nil || len(coordinate) < 2 || coordinate[0] == nil || coordinate[1] == nil || !validCourseCoordinate(*coordinate[1], *coordinate[0]) {
			return nil, 0, errors.New("routing response contained invalid coordinates")
		}
		point := CoursePoint{Latitude: *coordinate[1], Longitude: *coordinate[0]}
		if len(coordinate) >= 3 && coordinate[2] != nil && !math.IsNaN(*coordinate[2]) && !math.IsInf(*coordinate[2], 0) {
			point.ElevationM = cloneFloat(coordinate[2])
			known++
		}
		points = append(points, point)
	}
	points, err = normalizeCoursePoints(points)
	if err != nil {
		return nil, 0, err
	}
	return points, known, nil
}

func courseRoutingProfile(sport CourseSport) string {
	if sport == CourseSportCycling {
		return "bike"
	}
	return "foot"
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
