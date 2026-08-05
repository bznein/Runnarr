package app

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
)

func TestPreviewCourseGPXSeparatesTrackSegmentsAndRoutes(t *testing.T) {
	data := []byte(`<?xml version="1.0"?>
<gpx version="1.1">
  <wpt lat="53" lon="-6"><name>Aid</name></wpt>
  <trk><name>Canal</name><type>running</type>
    <trkseg>
      <trkpt lat="53.0000" lon="-6.0000"><ele>10</ele><extensions><x:test xmlns:x="urn:test">1</x:test></extensions></trkpt>
      <trkpt lat="53.0010" lon="-6.0010"><ele>12</ele></trkpt>
    </trkseg>
    <trkseg>
      <trkpt lat="53.0100" lon="-6.0100"/><trkpt lat="53.0110" lon="-6.0110"/>
    </trkseg>
  </trk>
  <rte><name>Shortcut</name><rtept lat="53.02" lon="-6.02"/><rtept lat="53.03" lon="-6.03"/></rte>
</gpx>`)
	preview, err := previewCourseGPX("routes.gpx", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Candidates) != 3 {
		t.Fatalf("candidates = %d", len(preview.Candidates))
	}
	if len(preview.Diagnostics) != 1 || preview.Diagnostics[0].Count != 1 {
		t.Fatalf("file diagnostics = %#v", preview.Diagnostics)
	}
	if preview.Candidates[0].Kind != "track" || preview.Candidates[0].SportType != CourseSportRun || preview.Candidates[0].course.Legs[0].Mode != CourseLegPreserved {
		t.Fatalf("track candidate = %#v", preview.Candidates[0])
	}
	if len(preview.Candidates[0].Profile) != 2 || preview.Candidates[0].Profile[0].ElevationM == nil {
		t.Fatalf("track preview profile = %#v", preview.Candidates[0].Profile)
	}
	if preview.Candidates[2].Kind != "route" || !preview.Candidates[2].RequiresSport || preview.Candidates[2].course.Legs[0].Mode != CourseLegDirect {
		t.Fatalf("route candidate = %#v", preview.Candidates[2])
	}
	if len(preview.Candidates[0].Diagnostics) != 1 || preview.Candidates[0].Diagnostics[0].Code != "extensions_ignored" {
		t.Fatalf("candidate diagnostics = %#v", preview.Candidates[0].Diagnostics)
	}
}

func TestPreviewCourseGPXKeepsOtherCandidatesWhenOneCoordinateIsBad(t *testing.T) {
	data := []byte(`<gpx><trk><trkseg><trkpt lat="bad" lon="-6"/><trkpt lat="53" lon="-6"/></trkseg><trkseg><trkpt lat="53" lon="-6"/><trkpt lat="53.01" lon="-6.01"/></trkseg></trk></gpx>`)
	preview, err := previewCourseGPX("mixed.gpx", data)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Candidates[0].Valid || preview.Candidates[0].Error == "" {
		t.Fatalf("bad candidate = %#v", preview.Candidates[0])
	}
	if !preview.Candidates[1].Valid {
		t.Fatalf("valid candidate = %#v", preview.Candidates[1])
	}
}

func TestSelectedImportedCourseRequiresValidSportAndDetails(t *testing.T) {
	preview, err := previewCourseGPX("route.gpx", []byte(`<gpx><rte><rtept lat="53" lon="-6"/><rtept lat="53.01" lon="-6.01"/></rte></gpx>`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = selectedImportedCourse(preview.Candidates[0], CourseImportSelection{Key: preview.Candidates[0].Key, Name: "Route", SportType: "Swim"})
	if !errorsIsCourseInvalid(err) {
		t.Fatalf("invalid sport error = %v", err)
	}
	course, err := selectedImportedCourse(preview.Candidates[0], CourseImportSelection{Key: preview.Candidates[0].Key, Name: "Route", SportType: CourseSportHike, Notes: "private"})
	if err != nil {
		t.Fatal(err)
	}
	if course.SportType != CourseSportHike || course.Notes != "private" {
		t.Fatalf("selected course = %#v", course.CourseSummary)
	}
}

func TestCourseGPXExportContainsTrackGeometryButNoPrivateMetadata(t *testing.T) {
	course, err := directCourseFromPoints("Private route", CourseSportCycling, "secret note", []CoursePoint{
		{Latitude: 53, Longitude: -6, ElevationM: courseFloat(10)},
		{Latitude: 53.01, Longitude: -6.01},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := exportCourseGPX(course)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "secret note") || strings.Contains(text, "<time>") || strings.Contains(text, "<rte>") {
		t.Fatalf("export leaked unsupported content: %s", text)
	}
	if !strings.Contains(text, "<trk>") || !strings.Contains(text, "<type>Cycling</type>") || !strings.Contains(text, "<ele>10</ele>") {
		t.Fatalf("export missing track fields: %s", text)
	}
	var parsed courseGPXDocument
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Tracks) != 1 || len(parsed.Tracks[0].Segments[0].Points) != 2 {
		t.Fatalf("parsed export = %#v", parsed)
	}
}

func TestPreviewCourseGPXEnforcesFileSize(t *testing.T) {
	_, err := previewCourseGPX("large.gpx", bytes.Repeat([]byte("x"), maxCourseImportBytes+1))
	if err == nil || !strings.Contains(err.Error(), "MiB") {
		t.Fatalf("size error = %v", err)
	}
}
