package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"testing"
)

type courseRoutingRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn courseRoutingRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestCourseRoutingUsesGraphHopperProfileElevationAndExactWaypoints(t *testing.T) {
	client := &http.Client{Transport: courseRoutingRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/route" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body graphHopperRouteRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Profile != "bike" || body.PointsEncoded || !body.Elevation || body.Instructions || len(body.Points) != 2 {
			t.Fatalf("routing body = %#v", body)
		}
		if body.Points[0][0] != -6 || body.Points[0][1] != 53 {
			t.Fatalf("GraphHopper coordinate order = %#v", body.Points)
		}
		return testGraphHopperResponse(http.StatusOK, [][]any{
			{-6.0001, 53.0001, 10.2}, {-6.005, 53.005, 18.4}, {-6.0099, 53.0099, 12.6},
		}), nil
	})}
	service := &CourseRoutingService{enabled: true, baseURL: "http://routing.test", client: client}
	result, err := service.Route(context.Background(), CourseRoutingRequest{SportType: CourseSportCycling, Waypoints: []CourseWaypoint{
		{Latitude: 53, Longitude: -6}, {Latitude: 53.01, Longitude: -6.01},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Legs) != 1 || result.Legs[0].Mode != CourseLegRouted || result.Legs[0].Warning != "" {
		t.Fatalf("result = %#v", result)
	}
	points := result.Legs[0].Points
	if points[0].Latitude != 53 || points[0].Longitude != -6 || points[len(points)-1].Latitude != 53.01 || points[len(points)-1].Longitude != -6.01 {
		t.Fatalf("routed boundaries = %#v", points)
	}
	if len(result.Legs[0].ElevationsM) != 3 || result.Legs[0].ElevationsM[1] == nil || *result.Legs[0].ElevationsM[1] != 18.4 {
		t.Fatalf("routed elevations = %#v", result.Legs[0].ElevationsM)
	}
	if len(result.Profile) != 3 || result.ElevationCoverage != 1 || result.ElevationGainM == nil || result.ElevationLossM == nil {
		t.Fatalf("routing preview = %#v", result)
	}
	if math.Abs(*result.ElevationGainM-8.2) > 0.001 || math.Abs(*result.ElevationLossM-5.8) > 0.001 {
		t.Fatalf("routing preview gain/loss = %v/%v", *result.ElevationGainM, *result.ElevationLossM)
	}
}

func TestCourseRoutingKeepsGeometryWhenGraphHopperOmitsElevation(t *testing.T) {
	client := &http.Client{Transport: courseRoutingRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return testGraphHopperResponse(http.StatusOK, [][]any{{-6.0, 53.0}, {-6.01, 53.01}}), nil
	})}
	service := &CourseRoutingService{enabled: true, baseURL: "http://routing.test", client: client}
	result, err := service.Route(context.Background(), CourseRoutingRequest{SportType: CourseSportRun, Waypoints: []CourseWaypoint{
		{Latitude: 53, Longitude: -6}, {Latitude: 53.01, Longitude: -6.01},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Legs[0].Mode != CourseLegRouted || result.Legs[0].Warning == "" || result.Legs[0].ElevationsM[0] != nil {
		t.Fatalf("result = %#v", result.Legs[0])
	}
	if result.ElevationCoverage != 0 || result.ElevationGainM != nil || result.ElevationLossM != nil {
		t.Fatalf("routing preview without elevation = %#v", result)
	}
}

func TestCourseRoutingFallsBackPerLeg(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: courseRoutingRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(bytes.NewBufferString("no path")), Header: make(http.Header)}, nil
	})}
	service := &CourseRoutingService{enabled: true, baseURL: "http://routing.test", client: client}
	result, err := service.Route(context.Background(), CourseRoutingRequest{SportType: CourseSportHike, Waypoints: []CourseWaypoint{
		{Latitude: 53, Longitude: -6}, {Latitude: 53.01, Longitude: -6.01}, {Latitude: 53.02, Longitude: -6.02},
	}, DirectLegIndexes: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(result.Legs) != 2 || result.Legs[0].Mode != CourseLegDirect || result.Legs[0].Warning == "" || result.Legs[1].Warning != "" {
		t.Fatalf("fallback result = %#v, calls = %d", result, calls)
	}
}

func TestCourseRoutingDisabledReturnsInspectableDirectLegs(t *testing.T) {
	service := &CourseRoutingService{}
	result, err := service.Route(context.Background(), CourseRoutingRequest{SportType: CourseSportRun, Waypoints: []CourseWaypoint{
		{Latitude: 53, Longitude: -6}, {Latitude: 53.01, Longitude: -6.01},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.RoutingEnabled || result.Legs[0].Mode != CourseLegDirect || result.Legs[0].Warning == "" {
		t.Fatalf("disabled result = %#v", result)
	}
}

func testGraphHopperResponse(status int, coordinates [][]any) *http.Response {
	payload := map[string]any{"paths": []map[string]any{{
		"distance": 1000,
		"ascend":   10,
		"descend":  10,
		"points":   map[string]any{"type": "LineString", "coordinates": coordinates},
	}}}
	data, _ := json.Marshal(payload)
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}
}
