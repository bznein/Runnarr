package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	ErrCourseGPXTooLarge = fmt.Errorf("course GPX must be %d MiB or smaller", maxCourseImportBytes>>20)
	ErrCourseGPXNoRoutes = errors.New("GPX contains no course tracks or routes")
)

type courseGPXDocument struct {
	Tracks    []courseGPXTrack `xml:"trk"`
	Routes    []courseGPXRoute `xml:"rte"`
	Waypoints []courseGPXPoint `xml:"wpt"`
}

type courseGPXTrack struct {
	Name     string             `xml:"name"`
	Type     string             `xml:"type"`
	Segments []courseGPXSegment `xml:"trkseg"`
}

type courseGPXRoute struct {
	Name   string           `xml:"name"`
	Type   string           `xml:"type"`
	Points []courseGPXPoint `xml:"rtept"`
}

type courseGPXSegment struct {
	Points []courseGPXPoint `xml:"trkpt"`
}

type courseGPXPoint struct {
	Latitude   string              `xml:"lat,attr"`
	Longitude  string              `xml:"lon,attr"`
	Elevation  string              `xml:"ele"`
	Time       string              `xml:"time"`
	Extensions courseGPXExtensions `xml:"extensions"`
}

type courseGPXExtensions struct {
	InnerXML string `xml:",innerxml"`
}

func previewCourseGPX(filename string, data []byte) (CourseImportPreview, error) {
	if len(data) > maxCourseImportBytes {
		return CourseImportPreview{}, ErrCourseGPXTooLarge
	}
	var document courseGPXDocument
	if err := xml.Unmarshal(data, &document); err != nil {
		return CourseImportPreview{}, fmt.Errorf("invalid GPX: %w", err)
	}
	hash := sha256.Sum256(data)
	preview := CourseImportPreview{Filename: filepath.Base(filename), FileSHA256: hex.EncodeToString(hash[:])}
	if len(document.Waypoints) > 0 {
		preview.Diagnostics = append(preview.Diagnostics, CourseImportDiagnostic{Code: "standalone_waypoints_ignored", Message: "Standalone GPX waypoints are not course points and will be discarded.", Count: len(document.Waypoints)})
	}
	totalSegments := 0
	totalPoints := 0
	for _, track := range document.Tracks {
		totalSegments += len(track.Segments)
		for _, segment := range track.Segments {
			totalPoints += len(segment.Points)
		}
	}
	for _, route := range document.Routes {
		totalSegments++
		totalPoints += len(route.Points)
	}
	if totalSegments == 0 {
		return CourseImportPreview{}, ErrCourseGPXNoRoutes
	}
	if totalSegments > maxCourseImportSegments {
		return CourseImportPreview{}, fmt.Errorf("GPX contains %d segments; at most %d are supported", totalSegments, maxCourseImportSegments)
	}
	if totalPoints > maxCoursePoints {
		return CourseImportPreview{}, fmt.Errorf("GPX contains %d points; at most %d are supported", totalPoints, maxCoursePoints)
	}
	fallback := strings.TrimSpace(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	if fallback == "" {
		fallback = "Imported course"
	}
	candidateNumber := 0
	for trackIndex, track := range document.Tracks {
		for segmentIndex, segment := range track.Segments {
			candidateNumber++
			name := strings.TrimSpace(track.Name)
			if name == "" {
				name = fallback
			}
			if totalSegments > 1 {
				name = fmt.Sprintf("%s %d", name, candidateNumber)
			}
			preview.Candidates = append(preview.Candidates, buildCourseGPXCandidate(
				fmt.Sprintf("track:%d:segment:%d", trackIndex, segmentIndex), "track", name, track.Type, segment.Points,
			))
		}
	}
	for routeIndex, route := range document.Routes {
		candidateNumber++
		name := strings.TrimSpace(route.Name)
		if name == "" {
			name = fallback
		}
		if totalSegments > 1 {
			name = fmt.Sprintf("%s %d", name, candidateNumber)
		}
		preview.Candidates = append(preview.Candidates, buildCourseGPXCandidate(
			fmt.Sprintf("route:%d", routeIndex), "route", name, route.Type, route.Points,
		))
	}
	return preview, nil
}

func buildCourseGPXCandidate(keyPrefix, kind, name, rawSport string, source []courseGPXPoint) CourseImportCandidate {
	candidate := CourseImportCandidate{Key: keyPrefix, Kind: kind, Name: name, Valid: false}
	timed := make([]timedCoursePoint, 0, len(source))
	invalidElevation, invalidTime, extensionCount := 0, 0, 0
	for index, sourcePoint := range source {
		latitude, latErr := strconv.ParseFloat(strings.TrimSpace(sourcePoint.Latitude), 64)
		longitude, lonErr := strconv.ParseFloat(strings.TrimSpace(sourcePoint.Longitude), 64)
		if latErr != nil || lonErr != nil || !validCourseCoordinate(latitude, longitude) {
			candidate.Error = fmt.Sprintf("Point %d has an invalid latitude or longitude.", index+1)
			return candidate
		}
		point := timedCoursePoint{CoursePoint: CoursePoint{Latitude: latitude, Longitude: longitude}}
		if raw := strings.TrimSpace(sourcePoint.Elevation); raw != "" {
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				invalidElevation++
			} else {
				point.ElevationM = &value
			}
		}
		if raw := strings.TrimSpace(sourcePoint.Time); raw != "" {
			value, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				invalidTime++
			} else {
				point.Timestamp = &value
			}
		}
		if strings.TrimSpace(sourcePoint.Extensions.InnerXML) != "" {
			extensionCount++
		}
		timed = append(timed, point)
	}
	if len(timed) < 2 {
		candidate.Error = "Segment contains fewer than two coordinate points."
		return candidate
	}
	sport := normalizeCourseSport(rawSport)
	jumpSport := sport
	if jumpSport == "" {
		jumpSport = CourseSportCycling
	}
	if kind == "track" {
		if index, invalid := courseHasImplausibleJump(timed, jumpSport); invalid {
			candidate.Error = fmt.Sprintf("Implausible GPS jump detected before point %d.", index+1)
			return candidate
		}
	}
	points := make([]CoursePoint, len(timed))
	for index := range timed {
		points[index] = timed[index].CoursePoint
	}
	if sport == "" {
		candidate.RequiresSport = true
		sport = CourseSportRun
	}
	diagnostics := make([]CourseImportDiagnostic, 0)
	if invalidElevation > 0 {
		diagnostics = append(diagnostics, CourseImportDiagnostic{Code: "invalid_elevation_ignored", Message: "Invalid elevation values will be stored as missing.", Count: invalidElevation})
	}
	if invalidTime > 0 {
		diagnostics = append(diagnostics, CourseImportDiagnostic{Code: "invalid_time_ignored", Message: "Invalid timestamps will be discarded.", Count: invalidTime})
	}
	if extensionCount > 0 {
		diagnostics = append(diagnostics, CourseImportDiagnostic{Code: "extensions_ignored", Message: "Unsupported GPX extensions will be discarded.", Count: extensionCount})
	}
	diagnosticMap := map[string]any{"importWarnings": diagnostics}
	var course Course
	var err error
	if kind == "route" {
		course, err = directCourseFromPoints(name, sport, "", points, diagnosticMap)
	} else {
		course, err = preservedCourseFromPoints(name, sport, "", points, diagnosticMap)
	}
	if err != nil {
		candidate.Error = strings.TrimPrefix(err.Error(), ErrCourseInvalid.Error()+": ")
		return candidate
	}
	candidate.Key = keyPrefix + ":" + course.GeometryHash[:16]
	candidate.Valid = true
	candidate.SportType = sport
	if candidate.RequiresSport {
		candidate.SportType = ""
	}
	candidate.EncodedPolyline = encodeCoursePolyline(boundedCoursePoints(flattenCoursePoints(course.Legs), maxCourseMapPreview), 6)
	candidate.DistanceM = course.DistanceM
	candidate.ElevationGainM = cloneFloat(course.ElevationGainM)
	candidate.ElevationLossM = cloneFloat(course.ElevationLossM)
	candidate.ElevationCoverage = course.ElevationCoverage
	candidate.PointCount = course.PointCount
	candidate.WaypointCount = len(course.Waypoints)
	candidate.Profile = course.Profile
	candidate.Diagnostics = diagnostics
	candidate.course = course
	candidate.timed = timed
	return candidate
}

func selectedImportedCourse(candidate CourseImportCandidate, selection CourseImportSelection) (Course, error) {
	if !candidate.Valid {
		return Course{}, fmt.Errorf("%w: selected GPX candidate is invalid", ErrCourseInvalid)
	}
	if candidate.Kind == "track" {
		if index, invalid := courseHasImplausibleJump(candidate.timed, selection.SportType); invalid {
			return Course{}, fmt.Errorf("%w: implausible GPS jump detected before point %d", ErrCourseInvalid, index+1)
		}
	}
	legs := make([]CourseLegInput, len(candidate.course.Legs))
	for index, leg := range candidate.course.Legs {
		legs[index] = CourseLegInput{Mode: leg.Mode, Points: append([]CoursePoint(nil), leg.Points...)}
	}
	return courseFromPlan(CoursePlanInput{Name: selection.Name, SportType: selection.SportType, Notes: selection.Notes, Legs: legs}, candidate.course.Diagnostics)
}

func exportCourseGPX(course Course) ([]byte, error) {
	points := flattenCoursePoints(course.Legs)
	if len(points) < 2 {
		return nil, ErrCourseGPXNoRoutes
	}
	exportPoints := make([]gpxExportPoint, len(points))
	for index, point := range points {
		exportPoints[index] = gpxExportPoint{Latitude: point.Latitude, Longitude: point.Longitude, Elevation: cloneFloat(point.ElevationM)}
	}
	doc := gpxExportDocument{
		XMLNS:        gpxNamespace,
		XMLNSGPXTPX:  gpxTPXNamespace,
		XMLNSRunnarr: runnarrGPXNamespace,
		Version:      "1.1",
		Creator:      "Runnarr",
		Metadata: gpxExportMetadata{
			Name: strings.TrimSpace(course.Name),
		},
		Track: gpxExportTrack{
			Name:     strings.TrimSpace(course.Name),
			Type:     string(course.SportType),
			Segments: []gpxExportSegment{{Points: exportPoints}},
		},
	}
	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

func courseGPXFilename(course Course) string {
	name := strings.TrimSpace(course.Name)
	if name == "" {
		name = "course"
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
		name = "course"
	}
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	if len(name) > 80 {
		name = strings.Trim(name[:80], "-_")
	}
	return name + ".gpx"
}

func flattenCoursePoints(legs []CourseLeg) []CoursePoint {
	result := make([]CoursePoint, 0)
	for index, leg := range legs {
		if index == 0 {
			result = append(result, leg.Points...)
		} else if len(leg.Points) > 1 {
			result = append(result, leg.Points[1:]...)
		}
	}
	return result
}
