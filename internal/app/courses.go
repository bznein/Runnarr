package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxCourseNameRunes        = 160
	maxCourseNotesRunes       = 5000
	maxCoursePoints           = 100000
	maxCourseWaypoints        = 500
	maxCourseImportBytes      = 10 << 20
	maxCoursePlanRequestBytes = 16 << 20
	maxCourseImportSegments   = 100
	maxCourseMapPreview       = 5000
	maxCourseProfilePoints    = 1200
	maxPreservedCourseAnchors = 25
	courseElevationRadiusM    = 150.0
	courseElevationMaxGapM    = 500.0
	courseElevationMinCover   = 0.90
	courseFootJumpMinM        = 250.0
	courseFootJumpSpeedMPS    = 25.0
	courseBikeJumpMinM        = 500.0
	courseBikeJumpSpeedMPS    = 60.0
	courseUntimedMaxJumpM     = 5000.0
)

var (
	ErrCourseInvalid   = errors.New("invalid course")
	ErrCourseConflict  = errors.New("course changed since it was loaded")
	ErrCourseDuplicate = errors.New("course geometry already exists")
)

type CourseSport string

const (
	CourseSportRun     CourseSport = "Run"
	CourseSportWalk    CourseSport = "Walk"
	CourseSportHike    CourseSport = "Hike"
	CourseSportCycling CourseSport = "Cycling"
)

type CourseLegMode string

const (
	CourseLegPreserved CourseLegMode = "preserved"
	CourseLegRouted    CourseLegMode = "routed"
	CourseLegDirect    CourseLegMode = "direct"
)

type CoursePoint struct {
	Latitude   float64  `json:"latitude"`
	Longitude  float64  `json:"longitude"`
	ElevationM *float64 `json:"elevationM,omitempty"`
}

type CourseWaypoint struct {
	ID        string  `json:"id,omitempty"`
	Index     int     `json:"index"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type CourseLeg struct {
	ID              string        `json:"id,omitempty"`
	Index           int           `json:"index"`
	Mode            CourseLegMode `json:"mode"`
	EncodedPolyline string        `json:"encodedPolyline"`
	ElevationsM     []*float64    `json:"elevationsM"`
	PointCount      int           `json:"pointCount"`
	Points          []CoursePoint `json:"-"`
}

type CourseSummary struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	SportType         CourseSport    `json:"sportType"`
	Notes             string         `json:"notes,omitempty"`
	Favorite          bool           `json:"favorite"`
	Revision          int            `json:"revision"`
	DistanceM         float64        `json:"distanceM"`
	ElevationGainM    *float64       `json:"elevationGainM,omitempty"`
	ElevationLossM    *float64       `json:"elevationLossM,omitempty"`
	ElevationCoverage float64        `json:"elevationCoverage"`
	PointCount        int            `json:"pointCount"`
	LegCount          int            `json:"legCount"`
	DirectLegCount    int            `json:"directLegCount"`
	Diagnostics       map[string]any `json:"diagnostics,omitempty"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
	GeometryHash      string         `json:"-"`
}

type Course struct {
	CourseSummary
	Waypoints []CourseWaypoint     `json:"waypoints"`
	Legs      []CourseLeg          `json:"legs"`
	Profile   []CourseProfilePoint `json:"profile"`
	Bounds    *CourseBounds        `json:"bounds,omitempty"`
}

type CourseBounds struct {
	South float64 `json:"south"`
	West  float64 `json:"west"`
	North float64 `json:"north"`
	East  float64 `json:"east"`
}

type CourseProfilePoint struct {
	DistanceM  float64  `json:"distanceM"`
	ElevationM *float64 `json:"elevationM,omitempty"`
	Latitude   float64  `json:"latitude"`
	Longitude  float64  `json:"longitude"`
}

type CourseListPage struct {
	Courses    []CourseSummary `json:"courses"`
	Limit      int             `json:"limit"`
	Offset     int             `json:"offset"`
	NextOffset int             `json:"nextOffset,omitempty"`
	HasMore    bool            `json:"hasMore"`
}

type CourseListOptions struct {
	Query    string
	Sport    string
	Favorite *bool
	Sort     string
	Order    string
	Limit    int
	Offset   int
}

type CourseDetailsInput struct {
	Revision  int         `json:"revision"`
	Name      string      `json:"name"`
	SportType CourseSport `json:"sportType"`
	Notes     string      `json:"notes"`
}

type CourseLegInput struct {
	ID              string        `json:"id,omitempty"`
	Mode            CourseLegMode `json:"mode"`
	EncodedPolyline string        `json:"encodedPolyline,omitempty"`
	ElevationsM     []*float64    `json:"elevationsM,omitempty"`
	Points          []CoursePoint `json:"points,omitempty"`
}

type CoursePlanInput struct {
	Revision  int              `json:"revision,omitempty"`
	Name      string           `json:"name"`
	SportType CourseSport      `json:"sportType"`
	Notes     string           `json:"notes"`
	Legs      []CourseLegInput `json:"legs"`
}

type CourseDuplicateInput struct {
	Revision int    `json:"revision"`
	Name     string `json:"name"`
	Notes    string `json:"notes"`
}

type CourseImportDiagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Count   int    `json:"count,omitempty"`
}

type CourseImportCandidate struct {
	Key               string                   `json:"key"`
	Kind              string                   `json:"kind"`
	Name              string                   `json:"name"`
	SportType         CourseSport              `json:"sportType,omitempty"`
	RequiresSport     bool                     `json:"requiresSport"`
	Valid             bool                     `json:"valid"`
	Error             string                   `json:"error,omitempty"`
	EncodedPolyline   string                   `json:"encodedPolyline,omitempty"`
	DistanceM         float64                  `json:"distanceM,omitempty"`
	ElevationGainM    *float64                 `json:"elevationGainM,omitempty"`
	ElevationLossM    *float64                 `json:"elevationLossM,omitempty"`
	ElevationCoverage float64                  `json:"elevationCoverage,omitempty"`
	PointCount        int                      `json:"pointCount,omitempty"`
	WaypointCount     int                      `json:"waypointCount,omitempty"`
	Profile           []CourseProfilePoint     `json:"profile,omitempty"`
	DuplicateCourse   *CourseSummary           `json:"duplicateCourse,omitempty"`
	Diagnostics       []CourseImportDiagnostic `json:"diagnostics,omitempty"`
	course            Course
	timed             []timedCoursePoint
}

type CourseImportPreview struct {
	Filename    string                   `json:"filename"`
	FileSHA256  string                   `json:"fileSHA256"`
	Candidates  []CourseImportCandidate  `json:"candidates"`
	Diagnostics []CourseImportDiagnostic `json:"diagnostics,omitempty"`
}

type CourseImportSelection struct {
	Key       string      `json:"key"`
	Name      string      `json:"name"`
	SportType CourseSport `json:"sportType"`
	Notes     string      `json:"notes"`
}

type CourseImportCommitInput struct {
	FileSHA256 string                  `json:"fileSHA256"`
	Selections []CourseImportSelection `json:"selections"`
}

type CourseImportResult struct {
	ImportID    string                   `json:"importId"`
	Filename    string                   `json:"filename"`
	FileSHA256  string                   `json:"fileSHA256"`
	Diagnostics []CourseImportDiagnostic `json:"diagnostics,omitempty"`
	Created     []CourseSummary          `json:"created"`
}

func validCourseSport(sport CourseSport) bool {
	switch sport {
	case CourseSportRun, CourseSportWalk, CourseSportHike, CourseSportCycling:
		return true
	default:
		return false
	}
}

func validCourseLegMode(mode CourseLegMode) bool {
	return mode == CourseLegPreserved || mode == CourseLegRouted || mode == CourseLegDirect
}

func normalizeCourseDetails(name string, sport CourseSport, notes string) (string, CourseSport, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxCourseNameRunes {
		return "", "", "", fmt.Errorf("%w: name is required and must be at most %d characters", ErrCourseInvalid, maxCourseNameRunes)
	}
	if !validCourseSport(sport) {
		return "", "", "", fmt.Errorf("%w: sport must be Run, Walk, Hike, or Cycling", ErrCourseInvalid)
	}
	if utf8.RuneCountInString(notes) > maxCourseNotesRunes {
		return "", "", "", fmt.Errorf("%w: notes must be at most %d characters", ErrCourseInvalid, maxCourseNotesRunes)
	}
	return name, sport, notes, nil
}

func courseFromPlan(input CoursePlanInput, diagnostics map[string]any) (Course, error) {
	name, sport, notes, err := normalizeCourseDetails(input.Name, input.SportType, input.Notes)
	if err != nil {
		return Course{}, err
	}
	if len(input.Legs) == 0 || len(input.Legs) > maxCourseWaypoints-1 {
		return Course{}, fmt.Errorf("%w: course must contain between 1 and %d legs", ErrCourseInvalid, maxCourseWaypoints-1)
	}
	legs := make([]CourseLeg, 0, len(input.Legs))
	for index, source := range input.Legs {
		if !validCourseLegMode(source.Mode) {
			return Course{}, fmt.Errorf("%w: invalid leg mode", ErrCourseInvalid)
		}
		points := append([]CoursePoint(nil), source.Points...)
		if len(points) == 0 && source.EncodedPolyline != "" {
			decoded, decodeErr := decodeCoursePolyline(source.EncodedPolyline, 6)
			if decodeErr != nil {
				return Course{}, decodeErr
			}
			points = decoded
		}
		if len(source.ElevationsM) > 0 {
			if len(source.ElevationsM) != len(points) {
				return Course{}, fmt.Errorf("%w: elevations must align with leg geometry", ErrCourseInvalid)
			}
			for pointIndex := range points {
				points[pointIndex].ElevationM = cloneFloat(source.ElevationsM[pointIndex])
			}
		}
		points, err = normalizeCoursePoints(points)
		if err != nil {
			return Course{}, err
		}
		legs = append(legs, CourseLeg{ID: source.ID, Index: index, Mode: source.Mode, Points: points})
	}
	course := Course{CourseSummary: CourseSummary{Name: name, SportType: sport, Notes: notes, Diagnostics: cloneCourseDiagnostics(diagnostics)}, Legs: legs}
	if err := finalizeCourse(&course); err != nil {
		return Course{}, err
	}
	return course, nil
}

func normalizeCoursePoints(points []CoursePoint) ([]CoursePoint, error) {
	if len(points) < 2 {
		return nil, fmt.Errorf("%w: each leg must contain at least two points", ErrCourseInvalid)
	}
	normalized := make([]CoursePoint, 0, len(points))
	for _, point := range points {
		if !validCourseCoordinate(point.Latitude, point.Longitude) {
			return nil, fmt.Errorf("%w: coordinates must be finite latitude/longitude values", ErrCourseInvalid)
		}
		if point.ElevationM != nil && (math.IsNaN(*point.ElevationM) || math.IsInf(*point.ElevationM, 0)) {
			point.ElevationM = nil
		}
		if len(normalized) > 0 && coursePointsEqual2D(normalized[len(normalized)-1], point) {
			if normalized[len(normalized)-1].ElevationM == nil && point.ElevationM != nil {
				normalized[len(normalized)-1].ElevationM = cloneFloat(point.ElevationM)
			}
			continue
		}
		normalized = append(normalized, point)
	}
	if len(normalized) < 2 {
		return nil, fmt.Errorf("%w: each leg must contain two distinct points", ErrCourseInvalid)
	}
	return normalized, nil
}

func finalizeCourse(course *Course) error {
	if course == nil || len(course.Legs) == 0 || len(course.Legs) > maxCourseWaypoints-1 {
		return fmt.Errorf("%w: invalid leg count", ErrCourseInvalid)
	}
	flat := make([]CoursePoint, 0)
	waypoints := make([]CourseWaypoint, 0, len(course.Legs)+1)
	directCount := 0
	for index := range course.Legs {
		leg := &course.Legs[index]
		leg.Index = index
		if !validCourseLegMode(leg.Mode) {
			return fmt.Errorf("%w: invalid leg mode", ErrCourseInvalid)
		}
		points, err := normalizeCoursePoints(leg.Points)
		if err != nil {
			return err
		}
		if index > 0 {
			previous := course.Legs[index-1].Points[len(course.Legs[index-1].Points)-1]
			if haversine(previous.Latitude, previous.Longitude, points[0].Latitude, points[0].Longitude) > 0.2 {
				return fmt.Errorf("%w: adjacent legs must share a waypoint", ErrCourseInvalid)
			}
			points[0].Latitude = previous.Latitude
			points[0].Longitude = previous.Longitude
			if points[0].ElevationM == nil {
				points[0].ElevationM = cloneFloat(previous.ElevationM)
			}
		}
		leg.Points = points
		leg.PointCount = len(points)
		leg.EncodedPolyline = encodeCoursePolyline(points, 6)
		leg.ElevationsM = courseElevations(points)
		if leg.Mode == CourseLegDirect {
			directCount++
		}
		if index == 0 {
			flat = append(flat, points...)
			waypoints = append(waypoints, CourseWaypoint{Index: 0, Latitude: points[0].Latitude, Longitude: points[0].Longitude})
		} else {
			flat = append(flat, points[1:]...)
		}
		last := points[len(points)-1]
		waypoints = append(waypoints, CourseWaypoint{Index: index + 1, Latitude: last.Latitude, Longitude: last.Longitude})
	}
	if len(flat) < 2 || len(flat) > maxCoursePoints {
		return fmt.Errorf("%w: course geometry must contain between 2 and %d points", ErrCourseInvalid, maxCoursePoints)
	}
	distance, coverage, gain, loss, profile := courseGeometryMetrics(flat)
	if distance <= 0 {
		return fmt.Errorf("%w: course distance must be greater than zero", ErrCourseInvalid)
	}
	course.DistanceM = distance
	course.ElevationCoverage = coverage
	course.ElevationGainM = gain
	course.ElevationLossM = loss
	course.PointCount = len(flat)
	course.LegCount = len(course.Legs)
	course.DirectLegCount = directCount
	course.GeometryHash = courseGeometryHash(flat)
	course.Waypoints = waypoints
	course.Profile = boundedCourseProfile(profile, maxCourseProfilePoints)
	course.Bounds = courseBounds(flat)
	return nil
}

func courseGeometryMetrics(points []CoursePoint) (float64, float64, *float64, *float64, []CourseProfilePoint) {
	distances := make([]float64, len(points))
	total := 0.0
	for index := 1; index < len(points); index++ {
		step := haversine(points[index-1].Latitude, points[index-1].Longitude, points[index].Latitude, points[index].Longitude)
		total += step
		distances[index] = total
	}
	elevations := make([]*float64, len(points))
	for index := range points {
		elevations[index] = cloneFloat(points[index].ElevationM)
	}
	interpolateCourseElevation(elevations, distances, courseElevationMaxGapM)
	knownDistance := 0.0
	for index := 1; index < len(points); index++ {
		if elevations[index-1] != nil && elevations[index] != nil {
			knownDistance += distances[index] - distances[index-1]
		}
	}
	coverage := 0.0
	if total > 0 {
		coverage = knownDistance / total
	}
	profile := make([]CourseProfilePoint, len(points))
	for index, point := range points {
		profile[index] = CourseProfilePoint{DistanceM: distances[index], ElevationM: cloneFloat(elevations[index]), Latitude: point.Latitude, Longitude: point.Longitude}
	}
	if coverage < courseElevationMinCover {
		return total, coverage, nil, nil, profile
	}
	smoothed := smoothCourseElevation(elevations, distances, courseElevationRadiusM)
	for index := range profile {
		profile[index].ElevationM = cloneFloat(smoothed[index])
	}
	gainValue, lossValue := 0.0, 0.0
	var previous *float64
	for _, value := range smoothed {
		if value == nil {
			previous = nil
			continue
		}
		if previous != nil {
			delta := *value - *previous
			if delta > 0 {
				gainValue += delta
			} else {
				lossValue -= delta
			}
		}
		previous = value
	}
	return total, coverage, &gainValue, &lossValue, profile
}

func interpolateCourseElevation(values []*float64, distances []float64, maxGapM float64) {
	for index := 0; index < len(values); {
		if values[index] != nil {
			index++
			continue
		}
		start := index
		for index < len(values) && values[index] == nil {
			index++
		}
		left, right := start-1, index
		if left < 0 || right >= len(values) || distances[right]-distances[left] > maxGapM {
			continue
		}
		span := distances[right] - distances[left]
		for fill := start; fill < right; fill++ {
			ratio := 0.0
			if span > 0 {
				ratio = (distances[fill] - distances[left]) / span
			}
			value := *values[left] + (*values[right]-*values[left])*ratio
			values[fill] = &value
		}
	}
}

func smoothCourseElevation(values []*float64, distances []float64, radiusM float64) []*float64 {
	result := make([]*float64, len(values))
	left, right := 0, 0
	sum := 0.0
	count := 0
	for index := range values {
		for left < len(values) && distances[index]-distances[left] > radiusM {
			if values[left] != nil {
				sum -= *values[left]
				count--
			}
			left++
		}
		if right < index {
			right = index
		}
		for right < len(values) && distances[right]-distances[index] <= radiusM {
			if values[right] != nil {
				sum += *values[right]
				count++
			}
			right++
		}
		if values[index] != nil && count > 0 {
			value := sum / float64(count)
			result[index] = &value
		}
	}
	return result
}

func courseGeometryHash(points []CoursePoint) string {
	hash := sha256.New()
	for _, point := range points {
		_, _ = hash.Write([]byte(strconv.FormatFloat(math.Round(point.Latitude*1e6)/1e6, 'f', 6, 64)))
		_, _ = hash.Write([]byte{','})
		_, _ = hash.Write([]byte(strconv.FormatFloat(math.Round(point.Longitude*1e6)/1e6, 'f', 6, 64)))
		_, _ = hash.Write([]byte{';'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func encodeCoursePolyline(points []CoursePoint, precision int) string {
	factor := math.Pow10(precision)
	lastLat, lastLon := int64(0), int64(0)
	var result strings.Builder
	for _, point := range points {
		lat := int64(math.Round(point.Latitude * factor))
		lon := int64(math.Round(point.Longitude * factor))
		encodeCoursePolylineDelta(&result, lat-lastLat)
		encodeCoursePolylineDelta(&result, lon-lastLon)
		lastLat, lastLon = lat, lon
	}
	return result.String()
}

func encodeCoursePolylineDelta(result *strings.Builder, delta int64) {
	value := uint64(delta << 1)
	if delta < 0 {
		value = uint64(^(delta << 1))
	}
	for value >= 0x20 {
		result.WriteByte(byte((0x20 | (value & 0x1f)) + 63))
		value >>= 5
	}
	result.WriteByte(byte(value + 63))
}

func decodeCoursePolyline(encoded string, precision int) ([]CoursePoint, error) {
	factor := math.Pow10(precision)
	lat, lon := int64(0), int64(0)
	points := make([]CoursePoint, 0)
	for index := 0; index < len(encoded); {
		deltaLat, next, err := decodeCoursePolylineDelta(encoded, index)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid encoded polyline", ErrCourseInvalid)
		}
		deltaLon, nextAfterLon, err := decodeCoursePolylineDelta(encoded, next)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid encoded polyline", ErrCourseInvalid)
		}
		index = nextAfterLon
		lat += deltaLat
		lon += deltaLon
		points = append(points, CoursePoint{Latitude: float64(lat) / factor, Longitude: float64(lon) / factor})
	}
	if len(points) < 2 {
		return nil, fmt.Errorf("%w: encoded polyline must contain at least two points", ErrCourseInvalid)
	}
	return points, nil
}

func decodeCoursePolylineDelta(encoded string, index int) (int64, int, error) {
	var result uint64
	var shift uint
	for {
		if index >= len(encoded) || shift > 60 {
			return 0, index, errors.New("invalid polyline")
		}
		value := encoded[index] - 63
		index++
		result |= uint64(value&0x1f) << shift
		if value < 0x20 {
			break
		}
		shift += 5
	}
	delta := int64(result >> 1)
	if result&1 != 0 {
		delta = ^delta
	}
	return delta, index, nil
}

func preservedCourseFromPoints(name string, sport CourseSport, notes string, points []CoursePoint, diagnostics map[string]any) (Course, error) {
	points, err := normalizeCoursePoints(points)
	if err != nil {
		return Course{}, err
	}
	indices := adaptiveCourseAnchorIndices(points, maxPreservedCourseAnchors)
	legs := make([]CourseLegInput, 0, len(indices)-1)
	for index := 1; index < len(indices); index++ {
		segment := append([]CoursePoint(nil), points[indices[index-1]:indices[index]+1]...)
		legs = append(legs, CourseLegInput{Mode: CourseLegPreserved, Points: segment})
	}
	return courseFromPlan(CoursePlanInput{Name: name, SportType: sport, Notes: notes, Legs: legs}, diagnostics)
}

func directCourseFromPoints(name string, sport CourseSport, notes string, points []CoursePoint, diagnostics map[string]any) (Course, error) {
	points, err := normalizeCoursePoints(points)
	if err != nil {
		return Course{}, err
	}
	if len(points) > maxCourseWaypoints {
		return Course{}, fmt.Errorf("%w: GPX route contains more than %d control points", ErrCourseInvalid, maxCourseWaypoints)
	}
	legs := make([]CourseLegInput, 0, len(points)-1)
	for index := 1; index < len(points); index++ {
		legs = append(legs, CourseLegInput{Mode: CourseLegDirect, Points: []CoursePoint{points[index-1], points[index]}})
	}
	return courseFromPlan(CoursePlanInput{Name: name, SportType: sport, Notes: notes, Legs: legs}, diagnostics)
}

func adaptiveCourseAnchorIndices(points []CoursePoint, maxAnchors int) []int {
	if len(points) <= 2 {
		return []int{0, len(points) - 1}
	}
	if maxAnchors < 2 {
		maxAnchors = 2
	}
	tolerance := 10.0
	indices := simplifyCourseIndices(points, tolerance)
	for len(indices) > maxAnchors {
		tolerance *= 1.5
		indices = simplifyCourseIndices(points, tolerance)
	}
	return indices
}

func simplifyCourseIndices(points []CoursePoint, toleranceM float64) []int {
	keep := make([]bool, len(points))
	keep[0], keep[len(points)-1] = true, true
	var visit func(int, int)
	visit = func(start, end int) {
		if end <= start+1 {
			return
		}
		maxDistance, maxIndex := 0.0, -1
		for index := start + 1; index < end; index++ {
			distance := coursePointSegmentDistanceM(points[index], points[start], points[end])
			if distance > maxDistance {
				maxDistance, maxIndex = distance, index
			}
		}
		if maxIndex >= 0 && maxDistance > toleranceM {
			keep[maxIndex] = true
			visit(start, maxIndex)
			visit(maxIndex, end)
		}
	}
	visit(0, len(points)-1)
	indices := make([]int, 0)
	for index, selected := range keep {
		if selected {
			indices = append(indices, index)
		}
	}
	return indices
}

func coursePointSegmentDistanceM(point, start, end CoursePoint) float64 {
	referenceLat := (start.Latitude + end.Latitude) * math.Pi / 360
	project := func(value CoursePoint) (float64, float64) {
		return value.Longitude * math.Cos(referenceLat) * 111320, value.Latitude * 110540
	}
	px, py := project(point)
	sx, sy := project(start)
	ex, ey := project(end)
	dx, dy := ex-sx, ey-sy
	if dx == 0 && dy == 0 {
		return math.Hypot(px-sx, py-sy)
	}
	t := ((px-sx)*dx + (py-sy)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(sx+t*dx), py-(sy+t*dy))
}

func boundedCourseProfile(points []CourseProfilePoint, limit int) []CourseProfilePoint {
	if limit <= 0 || len(points) <= limit {
		return points
	}
	result := make([]CourseProfilePoint, 0, limit)
	for index := 0; index < limit; index++ {
		source := int(math.Round(float64(index) * float64(len(points)-1) / float64(limit-1)))
		result = append(result, points[source])
	}
	return result
}

func boundedCoursePoints(points []CoursePoint, limit int) []CoursePoint {
	if limit <= 0 || len(points) <= limit {
		return points
	}
	indices := adaptiveCourseAnchorIndices(points, limit)
	if len(indices) > limit {
		indices = indices[:limit]
		indices[len(indices)-1] = len(points) - 1
	}
	result := make([]CoursePoint, 0, len(indices))
	for _, index := range indices {
		result = append(result, points[index])
	}
	return result
}

func courseBounds(points []CoursePoint) *CourseBounds {
	if len(points) == 0 {
		return nil
	}
	bounds := &CourseBounds{South: points[0].Latitude, North: points[0].Latitude, West: points[0].Longitude, East: points[0].Longitude}
	for _, point := range points[1:] {
		bounds.South = math.Min(bounds.South, point.Latitude)
		bounds.North = math.Max(bounds.North, point.Latitude)
		bounds.West = math.Min(bounds.West, point.Longitude)
		bounds.East = math.Max(bounds.East, point.Longitude)
	}
	return bounds
}

func validCourseCoordinate(latitude, longitude float64) bool {
	return !math.IsNaN(latitude) && !math.IsInf(latitude, 0) && !math.IsNaN(longitude) && !math.IsInf(longitude, 0) && latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180
}

func coursePointsEqual2D(left, right CoursePoint) bool {
	return left.Latitude == right.Latitude && left.Longitude == right.Longitude
}

func courseElevations(points []CoursePoint) []*float64 {
	result := make([]*float64, len(points))
	for index := range points {
		result[index] = cloneFloat(points[index].ElevationM)
	}
	return result
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneCourseDiagnostics(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var result map[string]any
	if json.Unmarshal(data, &result) != nil || result == nil {
		return map[string]any{}
	}
	return result
}

func normalizeCourseSport(value string) CourseSport {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(normalized, "cycl"), strings.Contains(normalized, "bik"):
		return CourseSportCycling
	case strings.Contains(normalized, "hik"):
		return CourseSportHike
	case strings.Contains(normalized, "walk"):
		return CourseSportWalk
	case strings.Contains(normalized, "run"):
		return CourseSportRun
	default:
		return ""
	}
}

func courseHasImplausibleJump(points []timedCoursePoint, sport CourseSport) (int, bool) {
	for index := 1; index < len(points); index++ {
		distance := haversine(points[index-1].Latitude, points[index-1].Longitude, points[index].Latitude, points[index].Longitude)
		if points[index-1].Timestamp != nil && points[index].Timestamp != nil && points[index].Timestamp.After(*points[index-1].Timestamp) {
			seconds := points[index].Timestamp.Sub(*points[index-1].Timestamp).Seconds()
			minimum, maximum := courseFootJumpMinM, courseFootJumpSpeedMPS
			if sport == CourseSportCycling {
				minimum, maximum = courseBikeJumpMinM, courseBikeJumpSpeedMPS
			}
			if distance >= minimum && distance/seconds > maximum {
				return index, true
			}
			continue
		}
		if distance > courseUntimedMaxJumpM {
			return index, true
		}
	}
	return 0, false
}

type timedCoursePoint struct {
	CoursePoint
	Timestamp *time.Time
}

func sortCourseLegs(legs []CourseLeg) {
	sort.Slice(legs, func(i, j int) bool { return legs[i].Index < legs[j].Index })
}
