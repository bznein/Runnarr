package app

import (
	"encoding/xml"
	"errors"
	"strings"
	"time"
	"unicode"
)

var ErrActivityGPXNoRoute = errors.New("activity has no GPS route to export")

const (
	gpxNamespace        = "http://www.topografix.com/GPX/1/1"
	gpxTPXNamespace     = "http://www.garmin.com/xmlschemas/TrackPointExtension/v1"
	runnarrGPXNamespace = "https://github.com/bznein/Runnarr/gpx/extensions/v1"
)

func exportActivityGPX(activity Activity, includeSensors bool) ([]byte, error) {
	points := make([]gpxExportPoint, 0, len(activity.Samples))
	for _, sample := range activity.Samples {
		if sample.Latitude == nil || sample.Longitude == nil {
			continue
		}
		point := gpxExportPoint{
			Latitude:  *sample.Latitude,
			Longitude: *sample.Longitude,
			Elevation: sample.ElevationM,
		}
		if sample.Timestamp != nil {
			point.Time = sample.Timestamp.UTC().Format(time.RFC3339)
		}
		if includeSensors {
			point.Extensions = gpxExtensionsForSample(sample)
		}
		points = append(points, point)
	}
	if len(points) < 2 {
		return nil, ErrActivityGPXNoRoute
	}

	doc := gpxExportDocument{
		XMLNS:        gpxNamespace,
		XMLNSGPXTPX:  gpxTPXNamespace,
		XMLNSRunnarr: runnarrGPXNamespace,
		Version:      "1.1",
		Creator:      "Runnarr",
		Metadata: gpxExportMetadata{
			Name: strings.TrimSpace(activity.Name),
		},
		Track: gpxExportTrack{
			Name: strings.TrimSpace(activity.Name),
			Type: strings.TrimSpace(activity.SportType),
			Segments: []gpxExportSegment{
				{Points: points},
			},
		},
	}
	if !activity.StartTime.IsZero() {
		doc.Metadata.Time = activity.StartTime.UTC().Format(time.RFC3339)
	}

	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

func gpxExtensionsForSample(sample ActivitySample) *gpxPointExtensions {
	var trackPoint *gpxTrackPointExtension
	if sample.HeartRate != nil || sample.Cadence != nil {
		trackPoint = &gpxTrackPointExtension{
			HeartRate: sample.HeartRate,
			Cadence:   sample.Cadence,
		}
	}
	var sensors *gpxRunnarrSensors
	if sample.Power != nil || sample.SpeedMPS != nil {
		sensors = &gpxRunnarrSensors{
			Power:    sample.Power,
			SpeedMPS: sample.SpeedMPS,
		}
	}
	if trackPoint == nil && sensors == nil {
		return nil
	}
	return &gpxPointExtensions{
		TrackPoint: trackPoint,
		Sensors:    sensors,
	}
}

func activityGPXFilename(activity Activity) string {
	name := strings.TrimSpace(activity.Name)
	if name == "" {
		name = "activity"
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			return r
		case unicode.IsSpace(r), r == '.':
			return '-'
		default:
			return -1
		}
	}, name)
	name = strings.Trim(name, "-_")
	if name == "" {
		name = "activity"
	}
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	if len(name) > 80 {
		name = strings.Trim(name[:80], "-_")
	}
	return name + ".gpx"
}

type gpxExportDocument struct {
	XMLName      xml.Name          `xml:"gpx"`
	XMLNS        string            `xml:"xmlns,attr"`
	XMLNSGPXTPX  string            `xml:"xmlns:gpxtpx,attr"`
	XMLNSRunnarr string            `xml:"xmlns:runnarr,attr"`
	Version      string            `xml:"version,attr"`
	Creator      string            `xml:"creator,attr"`
	Metadata     gpxExportMetadata `xml:"metadata"`
	Track        gpxExportTrack    `xml:"trk"`
}

type gpxExportMetadata struct {
	Name string `xml:"name,omitempty"`
	Time string `xml:"time,omitempty"`
}

type gpxExportTrack struct {
	Name     string             `xml:"name,omitempty"`
	Type     string             `xml:"type,omitempty"`
	Segments []gpxExportSegment `xml:"trkseg"`
}

type gpxExportSegment struct {
	Points []gpxExportPoint `xml:"trkpt"`
}

type gpxExportPoint struct {
	Latitude   float64             `xml:"lat,attr"`
	Longitude  float64             `xml:"lon,attr"`
	Elevation  *float64            `xml:"ele,omitempty"`
	Time       string              `xml:"time,omitempty"`
	Extensions *gpxPointExtensions `xml:"extensions,omitempty"`
}

type gpxPointExtensions struct {
	TrackPoint *gpxTrackPointExtension `xml:"gpxtpx:TrackPointExtension,omitempty"`
	Sensors    *gpxRunnarrSensors      `xml:"runnarr:sensors,omitempty"`
}

type gpxTrackPointExtension struct {
	HeartRate *int `xml:"gpxtpx:hr,omitempty"`
	Cadence   *int `xml:"gpxtpx:cad,omitempty"`
}

type gpxRunnarrSensors struct {
	Power    *int     `xml:"runnarr:power,omitempty"`
	SpeedMPS *float64 `xml:"runnarr:speedMPS,omitempty"`
}
