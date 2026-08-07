package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCourseLoopGenerationValidatesSportDistanceAndAvailability(t *testing.T) {
	valid := CourseLoopGenerationRequest{
		SportType:       CourseSportRun,
		Start:           CourseWaypoint{Latitude: 53.3498, Longitude: -6.2603},
		TargetDistanceM: 10000,
	}
	if _, err := (&CourseRoutingService{}).GenerateLoops(context.Background(), valid); !errors.Is(err, ErrCourseRoutingDisabled) {
		t.Fatalf("disabled error = %v", err)
	}
	service := &CourseRoutingService{enabled: true}
	invalid := valid
	invalid.TargetDistanceM = 999
	if _, err := service.GenerateLoops(context.Background(), invalid); !errors.Is(err, ErrCourseInvalid) {
		t.Fatalf("short run error = %v", err)
	}
	invalid = valid
	invalid.SportType = CourseSportCycling
	invalid.TargetDistanceM = 4999
	if _, err := service.GenerateLoops(context.Background(), invalid); !errors.Is(err, ErrCourseInvalid) {
		t.Fatalf("short ride error = %v", err)
	}
	invalid = valid
	invalid.Variation = maxCourseLoopVariation + 1
	if _, err := service.GenerateLoops(context.Background(), invalid); !errors.Is(err, ErrCourseInvalid) {
		t.Fatalf("variation error = %v", err)
	}
}

func TestCourseLoopGenerationReturnsDistinctEditableCandidates(t *testing.T) {
	var isochroneCalls atomic.Int32
	var routeCalls atomic.Int32
	var heightCalls atomic.Int32
	client := &http.Client{Transport: courseRoutingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var response any
		switch request.URL.Path {
		case "/isochrone":
			isochroneCalls.Add(1)
			var body struct {
				Costing  string `json:"costing"`
				Contours []struct {
					Distance float64 `json:"distance"`
				} `json:"contours"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Costing != "pedestrian" || len(body.Contours) != 3 {
				t.Fatalf("isodistance body = %#v", body)
			}
			features := make([]map[string]any, 0, len(body.Contours))
			for _, contour := range body.Contours {
				coordinates := make([][]float64, 0, 24)
				for bearing := 0.0; bearing < 360; bearing += 15 {
					point := testCourseLoopPoint(53.3498, -6.2603, contour.Distance*1000, bearing)
					coordinates = append(coordinates, []float64{point.Longitude, point.Latitude})
				}
				features = append(features, map[string]any{
					"type":       "Feature",
					"properties": map[string]any{"contour": contour.Distance},
					"geometry":   map[string]any{"type": "LineString", "coordinates": coordinates},
				})
			}
			response = map[string]any{"type": "FeatureCollection", "features": features}
		case "/route":
			routeCalls.Add(1)
			var body struct {
				Locations []struct {
					Latitude  float64 `json:"lat"`
					Longitude float64 `json:"lon"`
				} `json:"locations"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body.Locations) != 4 {
				t.Fatalf("route locations = %#v", body.Locations)
			}
			legs := make([]map[string]string, 0, 3)
			for index := 0; index < 3; index++ {
				legs = append(legs, map[string]string{"shape": encodeCoursePolyline([]CoursePoint{
					{Latitude: body.Locations[index].Latitude, Longitude: body.Locations[index].Longitude},
					{Latitude: body.Locations[index+1].Latitude, Longitude: body.Locations[index+1].Longitude},
				}, 6)})
			}
			response = map[string]any{"trip": map[string]any{"legs": legs}}
		case "/height":
			heightCalls.Add(1)
			var body struct {
				Polyline string `json:"encoded_polyline"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			points, err := decodeCoursePolyline(body.Polyline, 6)
			if err != nil {
				t.Fatal(err)
			}
			heights := make([]float64, len(points))
			for index := range heights {
				heights[index] = float64(index)
			}
			response = map[string]any{"height": heights}
		default:
			t.Fatalf("request = %s", request.URL.Path)
		}
		data, err := json.Marshal(response)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	})}
	service := &CourseRoutingService{enabled: true, baseURL: "http://routing.test", client: client}
	request := CourseLoopGenerationRequest{
		SportType:       CourseSportRun,
		Start:           CourseWaypoint{Latitude: 53.3498, Longitude: -6.2603},
		TargetDistanceM: 10000,
		Variation:       0,
	}
	result, err := service.GenerateLoops(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) < 2 || len(result.Candidates) > 3 {
		t.Fatalf("candidate count = %d", len(result.Candidates))
	}
	if isochroneCalls.Load() != 1 || routeCalls.Load() != maxCourseLoopCandidates || heightCalls.Load() != int32(len(result.Candidates)) {
		t.Fatalf("calls = isodistance %d, route %d, height %d", isochroneCalls.Load(), routeCalls.Load(), heightCalls.Load())
	}
	for index, candidate := range result.Candidates {
		if candidate.ID != "route-"+string(rune('1'+index)) || len(candidate.Waypoints) != 4 || len(candidate.Legs) != 3 {
			t.Fatalf("candidate = %#v", candidate)
		}
		first, last := candidate.Waypoints[0], candidate.Waypoints[len(candidate.Waypoints)-1]
		if first.Latitude != last.Latitude || first.Longitude != last.Longitude {
			t.Fatalf("open candidate = %#v", candidate.Waypoints)
		}
		if math.Abs(candidate.DistanceDeviationPct) > preferredCourseLoopDeviation || candidate.ElevationCoverage != 1 {
			t.Fatalf("candidate metrics = %#v", candidate)
		}
	}
	secondRequest := request
	secondRequest.Variation = 1
	second, err := service.GenerateLoops(context.Background(), secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	if second.Candidates[0].Waypoints[1] == result.Candidates[0].Waypoints[1] {
		t.Fatal("variation did not change generated headings")
	}
}

func TestCourseLoopGenerationHandlerReportsActionableStatuses(t *testing.T) {
	input := `{"sportType":"Run","start":{"index":0,"latitude":53.3498,"longitude":-6.2603},"targetDistanceM":10000,"variation":0}`
	tests := []struct {
		name    string
		service *CourseRoutingService
		body    string
		status  int
	}{
		{name: "disabled", service: &CourseRoutingService{}, body: input, status: http.StatusServiceUnavailable},
		{name: "invalid", service: &CourseRoutingService{enabled: true}, body: strings.Replace(input, "10000", "500", 1), status: http.StatusBadRequest},
		{name: "no contours", service: &CourseRoutingService{enabled: true, baseURL: "http://routing.test", client: &http.Client{Transport: courseRoutingRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return testCourseLoopHTTPResponse(http.StatusOK, map[string]any{"type": "FeatureCollection", "features": []any{}}), nil
		})}}, body: input, status: http.StatusUnprocessableEntity},
		{name: "upstream", service: &CourseRoutingService{enabled: true, baseURL: "http://routing.test", client: &http.Client{Transport: courseRoutingRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return testCourseLoopHTTPResponse(http.StatusServiceUnavailable, map[string]any{"error": "unavailable"}), nil
		})}}, body: input, status: http.StatusBadGateway},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{courseRouting: test.service, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			request := httptest.NewRequest(http.MethodPost, "/api/course-routing/loops", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			server.handleGenerateCourseLoops(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSelectCourseLoopCandidatesUsesToleranceDiversityAndFallback(t *testing.T) {
	cell := func(values ...int) map[courseLoopCell]struct{} {
		result := make(map[courseLoopCell]struct{}, len(values))
		for _, value := range values {
			result[courseLoopCell{latitude: value}] = struct{}{}
		}
		return result
	}
	candidates := []CourseLoopCandidate{
		{DistanceDeviationPct: 1, retraceRatio: 0.8, geometryCells: cell(1, 2, 3, 4)},
		{DistanceDeviationPct: 5, retraceRatio: 0.1, geometryCells: cell(10, 11, 12, 13)},
		{DistanceDeviationPct: 6, retraceRatio: 0.1, geometryCells: cell(10, 11, 12, 14)},
		{DistanceDeviationPct: -8, retraceRatio: 0.2, geometryCells: cell(20, 21, 22, 23)},
		{DistanceDeviationPct: 13, retraceRatio: 0, geometryCells: cell(30)},
	}
	selected := selectCourseLoopCandidates(candidates)
	if len(selected) != 3 || selected[0].DistanceDeviationPct != 5 || selected[1].DistanceDeviationPct != -8 || selected[2].DistanceDeviationPct != 1 {
		t.Fatalf("selected = %#v", selected)
	}
	fallback := selectCourseLoopCandidates([]CourseLoopCandidate{{DistanceDeviationPct: -17}, {DistanceDeviationPct: 25}})
	if len(fallback) != 1 || !strings.Contains(fallback[0].Warning, "17% shorter") {
		t.Fatalf("fallback = %#v", fallback)
	}
	if selected := selectCourseLoopCandidates([]CourseLoopCandidate{{DistanceDeviationPct: 21}}); selected != nil {
		t.Fatalf("outside cutoff = %#v", selected)
	}
}

func testCourseLoopPoint(latitude, longitude, distanceM, bearingDegrees float64) CoursePoint {
	const radiusM = 6371000
	bearing := bearingDegrees * math.Pi / 180
	angularDistance := distanceM / radiusM
	startLatitude := latitude * math.Pi / 180
	startLongitude := longitude * math.Pi / 180
	endLatitude := math.Asin(math.Sin(startLatitude)*math.Cos(angularDistance) + math.Cos(startLatitude)*math.Sin(angularDistance)*math.Cos(bearing))
	endLongitude := startLongitude + math.Atan2(math.Sin(bearing)*math.Sin(angularDistance)*math.Cos(startLatitude), math.Cos(angularDistance)-math.Sin(startLatitude)*math.Sin(endLatitude))
	return CoursePoint{Latitude: endLatitude * 180 / math.Pi, Longitude: endLongitude * 180 / math.Pi}
}

func testCourseLoopHTTPResponse(status int, payload any) *http.Response {
	data, _ := json.Marshal(payload)
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}
}
