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
		if r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var response any
		switch r.URL.Path {
		case "/route":
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
			response = map[string]any{"trip": map[string]any{"legs": []map[string]string{{"shape": encodeCoursePolyline(wantPoints, 6)}}}}
		case "/height":
			var body struct {
				EncodedPolyline string `json:"encoded_polyline"`
				ShapeFormat     string `json:"shape_format"`
				HeightPrecision int    `json:"height_precision"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			points, err := decodeCoursePolyline(body.EncodedPolyline, 6)
			if err != nil || len(points) != len(wantPoints) || body.ShapeFormat != "polyline6" || body.HeightPrecision != 1 {
				t.Fatalf("elevation body = %#v, points = %#v, err = %v", body, points, err)
			}
			response = map[string]any{"height": []float64{10.2, 18.4, 12.6}}
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		data, err := json.Marshal(response)
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
	if len(result.Legs[0].ElevationsM) != 3 || result.Legs[0].ElevationsM[1] == nil || *result.Legs[0].ElevationsM[1] != 18.4 {
		t.Fatalf("routed elevations = %#v", result.Legs[0].ElevationsM)
	}
}

func TestCourseRoutingKeepsGeometryWhenElevationUnavailable(t *testing.T) {
	points := []CoursePoint{{Latitude: 53, Longitude: -6}, {Latitude: 53.01, Longitude: -6.01}}
	client := &http.Client{Transport: courseRoutingRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/height" {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(bytes.NewBufferString("no elevation")), Header: make(http.Header)}, nil
		}
		data, err := json.Marshal(map[string]any{"trip": map[string]any{"legs": []map[string]string{{"shape": encodeCoursePolyline(points, 6)}}}})
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
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
