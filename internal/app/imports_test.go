package app

import (
	"context"
	"testing"
)

func TestGPXParser(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test">
  <trk>
    <name>Morning Run</name>
    <trkseg>
      <trkpt lat="53.349800" lon="-6.260300">
        <ele>10</ele>
        <time>2026-07-01T06:00:00Z</time>
        <extensions><gpxtpx:TrackPointExtension><gpxtpx:hr>140</gpxtpx:hr><gpxtpx:cad>82</gpxtpx:cad></gpxtpx:TrackPointExtension></extensions>
      </trkpt>
      <trkpt lat="53.350800" lon="-6.261300">
        <ele>15</ele>
        <time>2026-07-01T06:01:00Z</time>
        <extensions><gpxtpx:TrackPointExtension><gpxtpx:hr>150</gpxtpx:hr></gpxtpx:TrackPointExtension></extensions>
      </trkpt>
    </trkseg>
  </trk>
</gpx>`)

	activity, err := GPXParser{}.Parse(context.Background(), "morning.gpx", data)
	if err != nil {
		t.Fatal(err)
	}
	if activity.Name != "Morning Run" {
		t.Fatalf("name = %q", activity.Name)
	}
	if len(activity.Samples) != 2 {
		t.Fatalf("samples = %d", len(activity.Samples))
	}
	if activity.DistanceM <= 0 {
		t.Fatalf("distance = %f", activity.DistanceM)
	}
	if activity.ElevationGainM != 5 {
		t.Fatalf("elevation gain = %f", activity.ElevationGainM)
	}
	if activity.AvgHeartRate == nil || *activity.AvgHeartRate != 145 {
		t.Fatalf("avg heart rate = %#v", activity.AvgHeartRate)
	}
}

func TestTCXParser(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<TrainingCenterDatabase>
  <Activities>
    <Activity Sport="Running">
      <Id>2026-07-01T06:00:00Z</Id>
      <Lap StartTime="2026-07-01T06:00:00Z">
        <TotalTimeSeconds>60</TotalTimeSeconds>
        <DistanceMeters>200</DistanceMeters>
        <Track>
          <Trackpoint>
            <Time>2026-07-01T06:00:00Z</Time>
            <Position><LatitudeDegrees>53.3498</LatitudeDegrees><LongitudeDegrees>-6.2603</LongitudeDegrees></Position>
            <AltitudeMeters>10</AltitudeMeters>
            <DistanceMeters>0</DistanceMeters>
            <HeartRateBpm><Value>130</Value></HeartRateBpm>
          </Trackpoint>
          <Trackpoint>
            <Time>2026-07-01T06:01:00Z</Time>
            <Position><LatitudeDegrees>53.3508</LatitudeDegrees><LongitudeDegrees>-6.2613</LongitudeDegrees></Position>
            <AltitudeMeters>13</AltitudeMeters>
            <DistanceMeters>200</DistanceMeters>
            <HeartRateBpm><Value>150</Value></HeartRateBpm>
          </Trackpoint>
        </Track>
      </Lap>
    </Activity>
  </Activities>
</TrainingCenterDatabase>`)

	activity, err := TCXParser{}.Parse(context.Background(), "sample.tcx", data)
	if err != nil {
		t.Fatal(err)
	}
	if activity.SportType != "Run" {
		t.Fatalf("sport = %q", activity.SportType)
	}
	if activity.DistanceM != 200 {
		t.Fatalf("distance = %f", activity.DistanceM)
	}
	if len(activity.Laps) != 1 {
		t.Fatalf("laps = %d", len(activity.Laps))
	}
	if activity.MaxHeartRate == nil || *activity.MaxHeartRate != 150 {
		t.Fatalf("max heart rate = %#v", activity.MaxHeartRate)
	}
}
