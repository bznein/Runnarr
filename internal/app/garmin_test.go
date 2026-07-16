package app

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseGarminActivityDownloadRejectsArchiveWithoutActivityFile(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	file, err := zw.Create("readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("not an activity")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = parseGarminActivityDownload(context.Background(), "123", buf.Bytes())
	if err == nil || !strings.Contains(err.Error(), "supported FIT, TCX, or GPX") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyGarminMetadata(t *testing.T) {
	start := time.Date(2026, 7, 15, 8, 30, 0, 0, time.UTC)
	activity := ImportedActivity{
		Name:      "123",
		SportType: "Run",
		Raw:       map[string]any{"format": "fit"},
	}
	applyGarminMetadata(&activity, GarminBridgeActivity{
		ID:        "123",
		Name:      "Morning Run",
		SportType: "running",
		StartTime: start,
	})

	if activity.Name != "Morning Run" {
		t.Fatalf("name = %q", activity.Name)
	}
	if activity.SportType != "Run" {
		t.Fatalf("sport = %q", activity.SportType)
	}
	if activity.Raw["provider"] != garminProvider || activity.Raw["garmin_id"] != "123" {
		t.Fatalf("raw metadata = %#v", activity.Raw)
	}
}
