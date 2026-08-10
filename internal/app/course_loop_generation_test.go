package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestCourseLoopGenerationValidatesDistanceHillinessAndAvailability(t *testing.T) {
	valid := CourseLoopGenerationRequest{SportType: CourseSportRun, Start: CourseWaypoint{Latitude: 53.3498, Longitude: -6.2603}, TargetDistanceM: 10000}
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
	invalid.Hilliness = "mountainous"
	if _, err := service.GenerateLoops(context.Background(), invalid); !errors.Is(err, ErrCourseInvalid) {
		t.Fatalf("hilliness error = %v", err)
	}
}

func TestCourseLoopGenerationUsesNativeRoundTripsAndReturnsEditableCandidates(t *testing.T) {
	var mutex sync.Mutex
	requests := make([]graphHopperRouteRequest, 0)
	client := &http.Client{Transport: courseRoutingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body graphHopperRouteRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		mutex.Lock()
		requests = append(requests, body)
		requestIndex := len(requests)
		mutex.Unlock()
		angle := float64(body.RoundTrip.Seed%360) + float64(requestIndex)*17
		coordinates := testGraphHopperLoop(53.3498, -6.2603, body.RoundTrip.DistanceM, angle, 30+float64(body.RoundTrip.Seed%5)*20)
		return testGraphHopperResponse(http.StatusOK, coordinates), nil
	})}
	service := &CourseRoutingService{enabled: true, baseURL: "http://routing.test", client: client}
	input := CourseLoopGenerationRequest{
		SportType: CourseSportRun, Start: CourseWaypoint{Latitude: 53.3498, Longitude: -6.2603},
		TargetDistanceM: 10000, Variation: 4, Hilliness: CourseLoopHillinessHilly,
	}
	result, err := service.GenerateLoops(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Hilliness != CourseLoopHillinessHilly || len(result.Candidates) == 0 || len(result.Candidates) > 3 {
		t.Fatalf("result = %#v", result)
	}
	if len(requests) != primaryCourseLoopCandidates {
		t.Fatalf("request count = %d", len(requests))
	}
	for _, request := range requests {
		if request.Algorithm != "round_trip" || request.RoundTrip == nil || request.Profile != "foot" || !request.CHDisabled || request.CustomModel == nil || len(request.CustomModel.Priority) != 2 {
			t.Fatalf("GraphHopper request = %#v", request)
		}
	}
	for index, candidate := range result.Candidates {
		if candidate.ID != "route-"+string(rune('1'+index)) || len(candidate.Waypoints) != 4 || len(candidate.Legs) != 3 {
			t.Fatalf("candidate = %#v", candidate)
		}
		first, last := candidate.Waypoints[0], candidate.Waypoints[3]
		if first.Latitude != last.Latitude || first.Longitude != last.Longitude || candidate.ElevationCoverage != 1 || candidate.ElevationGainM == nil {
			t.Fatalf("candidate metrics = %#v", candidate)
		}
	}
}

func TestCourseLoopGenerationDefaultsToBalancedAndCapsCorrectionRequests(t *testing.T) {
	var mutex sync.Mutex
	requests := make([]graphHopperRouteRequest, 0)
	client := &http.Client{Transport: courseRoutingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body graphHopperRouteRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		mutex.Lock()
		requests = append(requests, body)
		mutex.Unlock()
		// Overshoot every request enough to force the bounded correction pass.
		return testGraphHopperResponse(http.StatusOK, testGraphHopperLoop(53.3498, -6.2603, body.RoundTrip.DistanceM*1.3, float64(body.RoundTrip.Seed%360), 50)), nil
	})}
	service := &CourseRoutingService{enabled: true, baseURL: "http://routing.test", client: client}
	result, err := service.GenerateLoops(context.Background(), CourseLoopGenerationRequest{
		SportType: CourseSportRun, Start: CourseWaypoint{Latitude: 53.3498, Longitude: -6.2603}, TargetDistanceM: 10000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Hilliness != CourseLoopHillinessBalanced || len(requests) != primaryCourseLoopCandidates+correctedCourseLoopCandidates {
		t.Fatalf("hilliness/calls = %s/%d", result.Hilliness, len(requests))
	}
	for _, request := range requests {
		if request.CustomModel != nil {
			t.Fatalf("balanced request has custom model: %#v", request.CustomModel)
		}
	}
}

func TestSelectCourseLoopCandidatesRanksHillinessAndPreservesFallback(t *testing.T) {
	gain := func(value float64) *float64 { return &value }
	cell := func(value int) map[courseLoopCell]struct{} { return map[courseLoopCell]struct{}{{latitude: value}: {}} }
	candidates := []CourseLoopCandidate{
		{CourseRoutingResponse: CourseRoutingResponse{DistanceM: 10000, ElevationGainM: gain(100), ElevationCoverage: 1}, DistanceDeviationPct: 1, retraceRatio: .1, geometryCells: cell(1)},
		{CourseRoutingResponse: CourseRoutingResponse{DistanceM: 10000, ElevationGainM: gain(500), ElevationCoverage: 1}, DistanceDeviationPct: 2, retraceRatio: .1, geometryCells: cell(2)},
		{CourseRoutingResponse: CourseRoutingResponse{DistanceM: 10000, ElevationGainM: gain(250), ElevationCoverage: 1}, DistanceDeviationPct: .5, retraceRatio: .1, geometryCells: cell(3)},
	}
	flat := selectCourseLoopCandidates(append([]CourseLoopCandidate(nil), candidates...), CourseLoopHillinessFlat)
	hilly := selectCourseLoopCandidates(append([]CourseLoopCandidate(nil), candidates...), CourseLoopHillinessHilly)
	balanced := selectCourseLoopCandidates(append([]CourseLoopCandidate(nil), candidates...), CourseLoopHillinessBalanced)
	if *flat[0].ElevationGainM != 100 || *hilly[0].ElevationGainM != 500 || balanced[0].DistanceDeviationPct != .5 {
		t.Fatalf("flat/hilly/balanced = %#v / %#v / %#v", flat, hilly, balanced)
	}
	fallback := selectCourseLoopCandidates([]CourseLoopCandidate{{DistanceDeviationPct: -17}, {DistanceDeviationPct: 25}}, CourseLoopHillinessBalanced)
	if len(fallback) != 1 || !strings.Contains(fallback[0].Warning, "17% shorter") {
		t.Fatalf("fallback = %#v", fallback)
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
		{name: "upstream", service: &CourseRoutingService{enabled: true, baseURL: "http://routing.test", client: &http.Client{Transport: courseRoutingRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}, nil
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

func testGraphHopperLoop(latitude, longitude, distanceM, heading, gainM float64) [][]any {
	leg := distanceM / 4
	p0 := CoursePoint{Latitude: latitude, Longitude: longitude}
	p1 := testCourseLoopPoint(p0.Latitude, p0.Longitude, leg, heading)
	p2 := testCourseLoopPoint(p1.Latitude, p1.Longitude, leg, heading+90)
	p3 := testCourseLoopPoint(p2.Latitude, p2.Longitude, leg, heading+180)
	return [][]any{
		{p0.Longitude, p0.Latitude, 10.0},
		{p1.Longitude, p1.Latitude, 10.0 + gainM},
		{p2.Longitude, p2.Latitude, 10.0},
		{p3.Longitude, p3.Latitude, 10.0 + gainM/2},
		{p0.Longitude, p0.Latitude, 10.0},
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
