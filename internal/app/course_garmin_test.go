package app

import (
	"context"
	"strings"
	"testing"
)

func TestGarminCourseOwnershipRequiresProviderIDAndMarker(t *testing.T) {
	course := Course{CourseSummary: CourseSummary{GeometryHash: strings.Repeat("a", 64)}}
	marker, err := garminCourseOwnershipMarker("owner", course)
	if err != nil {
		t.Fatal(err)
	}
	if marker != "runnarr-course:owner:"+strings.Repeat("a", 64) {
		t.Fatalf("marker = %q", marker)
	}
	if err := verifyGarminCourseUpload(GarminBridgeCourse{ID: "123", Description: marker}, marker); err != nil {
		t.Fatalf("matching upload rejected: %v", err)
	}
	for _, remote := range []GarminBridgeCourse{
		{Description: marker},
		{ID: "123"},
		{ID: "123", Description: "Created outside Runnarr"},
	} {
		if err := verifyGarminCourseUpload(remote, marker); err == nil {
			t.Fatalf("unverifiable upload accepted: %#v", remote)
		}
	}
}

func TestGarminServiceSendCourseExportsGPXAndBuildsProviderURL(t *testing.T) {
	course, err := directCourseFromPoints("Canal route", CourseSportRun, "private notes", []CoursePoint{
		{Latitude: 53.3, Longitude: -6.2},
		{Latitude: 53.4, Longitude: -6.1},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bridge := stubGarminBridge{uploadedCourse: GarminBridgeCourse{
		ID:          "321",
		Description: "runnarr-course:owner:hash",
		Raw:         map[string]any{"courseId": "321", "fixture": true},
	}}
	service := GarminService{bridge: bridge, tokenDir: "tokens"}
	remote, err := service.SendCourse(context.Background(), course, "runnarr-course:owner:hash")
	if err != nil {
		t.Fatal(err)
	}
	if remote.URL != "https://connect.garmin.com/modern/course/321" {
		t.Fatalf("provider URL = %q", remote.URL)
	}
	if remote.Raw["fixture"] != true {
		t.Fatalf("raw provider payload not preserved: %#v", remote.Raw)
	}
}

func TestOfflineGarminBridgeUploadsCourse(t *testing.T) {
	bridge := PythonGarminBridge{Python: "python3", Script: "garmin_bridge_testbed.py"}
	tokenStore := t.TempDir()
	remote, err := bridge.UploadCourse(
		context.Background(),
		tokenStore,
		"canal.gpx",
		[]byte(`<?xml version="1.0"?><gpx><trk><trkseg><trkpt lat="53" lon="-6"/><trkpt lat="53.1" lon="-6.1"/></trkseg></trk></gpx>`),
		"Canal route",
		CourseSportRun,
		"runnarr-course:owner:hash",
	)
	if err != nil {
		t.Fatal(err)
	}
	if remote.ID == "" || remote.Name != "Canal route" || remote.Description != "runnarr-course:owner:hash" {
		t.Fatalf("offline course = %#v", remote)
	}
	if remote.Raw["fixture"] != "testbed" {
		t.Fatalf("offline raw response = %#v", remote.Raw)
	}
	checked, err := bridge.GetCourse(context.Background(), tokenStore, remote.ID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.ID != remote.ID || checked.Description != remote.Description {
		t.Fatalf("offline course verification = %#v", checked)
	}
}
