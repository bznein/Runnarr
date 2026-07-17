package app

import (
	"encoding/xml"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExportActivityGPXOmitsSensorsByDefault(t *testing.T) {
	activity := testGPXActivity()
	data, err := exportActivityGPX(activity, false)
	if err != nil {
		t.Fatal(err)
	}
	assertValidXML(t, data)

	out := string(data)
	for _, unexpected := range []string{"<extensions>", "gpxtpx:hr", "gpxtpx:cad", "runnarr:power", "runnarr:speedMPS"} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("GPX without sensors contains %q:\n%s", unexpected, out)
		}
	}
	for _, expected := range []string{`lat="53.1"`, `lon="-6.1"`, "<ele>42.5</ele>", "<time>2026-07-17T10:00:00Z</time>"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("GPX missing %q:\n%s", expected, out)
		}
	}
}

func TestExportActivityGPXIncludesSensorsWhenRequested(t *testing.T) {
	activity := testGPXActivity()
	data, err := exportActivityGPX(activity, true)
	if err != nil {
		t.Fatal(err)
	}
	assertValidXML(t, data)

	out := string(data)
	for _, expected := range []string{
		"<extensions>",
		"<gpxtpx:hr>142</gpxtpx:hr>",
		"<gpxtpx:cad>86</gpxtpx:cad>",
		"<runnarr:power>255</runnarr:power>",
		"<runnarr:speedMPS>3.4</runnarr:speedMPS>",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("GPX missing %q:\n%s", expected, out)
		}
	}
	if strings.Contains(out, "<gpxtpx:hr></gpxtpx:hr>") || strings.Contains(out, "<runnarr:power></runnarr:power>") {
		t.Fatalf("GPX contains empty sensor tags:\n%s", out)
	}
}

func TestExportActivityGPXRequiresRoute(t *testing.T) {
	activity := testGPXActivity()
	activity.Samples = activity.Samples[:1]
	if _, err := exportActivityGPX(activity, false); !errors.Is(err, ErrActivityGPXNoRoute) {
		t.Fatalf("exportActivityGPX error = %v, want ErrActivityGPXNoRoute", err)
	}
}

func TestActivityGPXFilename(t *testing.T) {
	name := activityGPXFilename(Activity{Name: " Morning Run / Tempo? "})
	if name != "Morning-Run-Tempo.gpx" {
		t.Fatalf("filename = %q, want sanitized GPX filename", name)
	}
}

func testGPXActivity() Activity {
	start := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	next := start.Add(30 * time.Second)
	lat1, lon1, ele1 := 53.1, -6.1, 42.5
	lat2, lon2, ele2 := 53.1005, -6.1008, 44.2
	hr, cad, power, speed := 142, 86, 255, 3.4
	return Activity{
		Name:      "Morning Run",
		SportType: "Run",
		StartTime: start,
		Samples: []ActivitySample{
			{
				Index:      0,
				Timestamp:  &start,
				Latitude:   &lat1,
				Longitude:  &lon1,
				ElevationM: &ele1,
				HeartRate:  &hr,
				Cadence:    &cad,
				Power:      &power,
				SpeedMPS:   &speed,
			},
			{
				Index:      1,
				Timestamp:  &next,
				Latitude:   &lat2,
				Longitude:  &lon2,
				ElevationM: &ele2,
			},
		},
	}
}

func assertValidXML(t *testing.T, data []byte) {
	t.Helper()
	var parsed any
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid XML: %v\n%s", err, data)
	}
}
