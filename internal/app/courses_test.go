package app

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestCoursePlanFinalizesGeometryAndMetrics(t *testing.T) {
	points := []CoursePoint{
		{Latitude: 53.3400, Longitude: -6.2600, ElevationM: courseFloat(10)},
		{Latitude: 53.3410, Longitude: -6.2600, ElevationM: courseFloat(20)},
		{Latitude: 53.3420, Longitude: -6.2600, ElevationM: courseFloat(15)},
	}
	course, err := courseFromPlan(CoursePlanInput{
		Name: " River run ", SportType: CourseSportRun, Notes: "private",
		Waypoints: []CourseWaypointInput{{Index: 0, Name: " Home "}, {Index: 1, Name: "Turnaround"}},
		Legs:      []CourseLegInput{{Mode: CourseLegDirect, Points: points}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if course.Name != "River run" || course.PointCount != 3 || course.LegCount != 1 || course.DirectLegCount != 1 {
		t.Fatalf("unexpected course summary: %#v", course.CourseSummary)
	}
	if course.DistanceM < 220 || course.DistanceM > 225 {
		t.Fatalf("distance = %f", course.DistanceM)
	}
	if course.ElevationCoverage != 1 || course.ElevationGainM == nil || course.ElevationLossM == nil {
		t.Fatalf("unexpected elevation metrics: coverage=%f gain=%v loss=%v", course.ElevationCoverage, course.ElevationGainM, course.ElevationLossM)
	}
	if len(course.Profile) != 3 || course.Profile[1].ElevationM == nil {
		t.Fatalf("profile = %#v", course.Profile)
	}
	if course.Waypoints[0].Latitude != points[0].Latitude || course.Waypoints[1].Latitude != points[2].Latitude {
		t.Fatalf("waypoints = %#v", course.Waypoints)
	}
	if course.Waypoints[0].Name != "Home" || course.Waypoints[1].Name != "Turnaround" {
		t.Fatalf("waypoint names = %#v", course.Waypoints)
	}
	course.Waypoints[0].ID = "waypoint-1"
	if err := finalizeCourse(&course); err != nil {
		t.Fatal(err)
	}
	if course.Waypoints[0].ID != "waypoint-1" || course.Waypoints[0].Name != "Home" {
		t.Fatalf("waypoint metadata was not retained: %#v", course.Waypoints[0])
	}
}

func TestCourseGeometryHashIgnoresElevationButKeepsDirection(t *testing.T) {
	forward := []CoursePoint{{Latitude: 1, Longitude: 2, ElevationM: courseFloat(3)}, {Latitude: 1.1, Longitude: 2.1, ElevationM: courseFloat(4)}}
	sameRounded := []CoursePoint{{Latitude: 1.00000004, Longitude: 2.00000004}, {Latitude: 1.10000004, Longitude: 2.10000004}}
	reverse := []CoursePoint{forward[1], forward[0]}
	if courseGeometryHash(forward) != courseGeometryHash(sameRounded) {
		t.Fatal("expected canonical coordinate hash to ignore sub-precision differences and elevation")
	}
	if courseGeometryHash(forward) == courseGeometryHash(reverse) {
		t.Fatal("reverse direction must remain distinct")
	}
}

func TestCoursePolyline6RoundTrip(t *testing.T) {
	want := []CoursePoint{{Latitude: 53.349805, Longitude: -6.26031}, {Latitude: 53.350991, Longitude: -6.259117}}
	encoded := encodeCoursePolyline(want, 6)
	got, err := decodeCoursePolyline(encoded, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("decoded points = %d", len(got))
	}
	for index := range want {
		if math.Abs(got[index].Latitude-want[index].Latitude) > 0.000001 || math.Abs(got[index].Longitude-want[index].Longitude) > 0.000001 {
			t.Fatalf("point %d = %#v, want %#v", index, got[index], want[index])
		}
	}
}

func TestPreservedCourseAnchorsKeepExactGeometry(t *testing.T) {
	points := make([]CoursePoint, 250)
	for index := range points {
		points[index] = CoursePoint{Latitude: 53 + float64(index)*0.0001, Longitude: -6 + math.Sin(float64(index)/10)*0.001}
	}
	course, err := preservedCourseFromPoints("Dense", CourseSportHike, "", points, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(course.Waypoints) > maxPreservedCourseAnchors {
		t.Fatalf("waypoints = %d", len(course.Waypoints))
	}
	flattened := flattenCoursePoints(course.Legs)
	if len(flattened) != len(points) {
		t.Fatalf("flattened points = %d, want %d", len(flattened), len(points))
	}
	for index := range points {
		if !coursePointsEqual2D(points[index], flattened[index]) {
			t.Fatalf("point %d changed: %#v vs %#v", index, points[index], flattened[index])
		}
	}
}

func TestCourseElevationInterpolatesSmallGapAndRejectsSparseCoverage(t *testing.T) {
	full := []CoursePoint{
		{Latitude: 53, Longitude: -6, ElevationM: courseFloat(10)},
		{Latitude: 53.001, Longitude: -6},
		{Latitude: 53.002, Longitude: -6, ElevationM: courseFloat(20)},
	}
	_, coverage, gain, _, profile := courseGeometryMetrics(full)
	if coverage != 1 || gain == nil || profile[1].ElevationM == nil {
		t.Fatalf("interpolated coverage=%f gain=%v profile=%#v", coverage, gain, profile)
	}
	sparse := []CoursePoint{
		{Latitude: 53, Longitude: -6, ElevationM: courseFloat(10)},
		{Latitude: 53.01, Longitude: -6},
		{Latitude: 53.02, Longitude: -6, ElevationM: courseFloat(20)},
	}
	_, coverage, gain, loss, _ := courseGeometryMetrics(sparse)
	if coverage >= courseElevationMinCover || gain != nil || loss != nil {
		t.Fatalf("sparse coverage=%f gain=%v loss=%v", coverage, gain, loss)
	}
}

func TestCourseJumpDetectionUsesSportAndUntimedFallback(t *testing.T) {
	start := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	end := start.Add(20 * time.Second)
	points := []timedCoursePoint{
		{CoursePoint: CoursePoint{Latitude: 53, Longitude: -6}, Timestamp: &start},
		{CoursePoint: CoursePoint{Latitude: 53.01, Longitude: -6}, Timestamp: &end},
	}
	if _, invalid := courseHasImplausibleJump(points, CourseSportRun); !invalid {
		t.Fatal("expected fast foot jump to be rejected")
	}
	if _, invalid := courseHasImplausibleJump(points, CourseSportCycling); invalid {
		t.Fatal("cycling threshold should allow this test span")
	}
	points[0].Timestamp, points[1].Timestamp = nil, nil
	points[1].Latitude = 53.1
	if _, invalid := courseHasImplausibleJump(points, CourseSportCycling); !invalid {
		t.Fatal("expected untimed 5km+ jump to be rejected")
	}
}

func TestCourseValidationLimitsMetadata(t *testing.T) {
	_, _, _, err := normalizeCourseDetails(strings.Repeat("n", maxCourseNameRunes+1), CourseSportRun, "")
	if !errorsIsCourseInvalid(err) {
		t.Fatalf("long name error = %v", err)
	}
	_, _, _, err = normalizeCourseDetails("ok", "Swim", "")
	if !errorsIsCourseInvalid(err) {
		t.Fatalf("sport error = %v", err)
	}
}

func TestCourseWaypointNamesMustAlignWithGeometry(t *testing.T) {
	leg := CourseLegInput{Mode: CourseLegDirect, Points: []CoursePoint{{Latitude: 53, Longitude: -6}, {Latitude: 53.01, Longitude: -6}}}
	_, err := courseFromPlan(CoursePlanInput{
		Name: "Named route", SportType: CourseSportRun,
		Waypoints: []CourseWaypointInput{{Index: 0, Name: "Only one"}},
		Legs:      []CourseLegInput{leg},
	}, nil)
	if !errorsIsCourseInvalid(err) {
		t.Fatalf("misaligned waypoint error = %v", err)
	}
	_, err = courseFromPlan(CoursePlanInput{
		Name: "Named route", SportType: CourseSportRun,
		Waypoints: []CourseWaypointInput{{Index: 0}, {Index: 1, Name: strings.Repeat("n", maxCourseWaypointNameRunes+1)}},
		Legs:      []CourseLegInput{leg},
	}, nil)
	if !errorsIsCourseInvalid(err) {
		t.Fatalf("long waypoint name error = %v", err)
	}
}

func courseFloat(value float64) *float64 { return &value }

func errorsIsCourseInvalid(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrCourseInvalid.Error())
}
