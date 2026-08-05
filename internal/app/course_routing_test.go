package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

type courseRoutingRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn courseRoutingRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestCourseRoutingUsesSportCostingAndExactWaypoints(t *testing.T) {
	wantPoints := []CoursePoint{{Latitude: 53.0001, Longitude: -6.0001}, {Latitude: 53.005, Longitude: -6.005}, {Latitude: 53.0099, Longitude: -6.0099}}
	client := &http.Client{Transport: courseRoutingRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/route" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Costing     string `json:"costing"`
			ShapeFormat string `json:"shape_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Costing != "bicycle" || body.ShapeFormat != "polyline6" {
			t.Fatalf("routing body = %#v", body)
		}
		data, err := json.Marshal(map[string]any{"trip": map[string]any{"legs": []map[string]string{{"shape": encodeCoursePolyline(wantPoints, 6)}}}})
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
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
	points, err := decodeCoursePolyline(result.Legs[0].EncodedPolyline, 6)
	if err != nil {
		t.Fatal(err)
	}
	if points[0].Latitude != 53 || points[len(points)-1].Latitude != 53.01 {
		t.Fatalf("routed boundaries = %#v", points)
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
