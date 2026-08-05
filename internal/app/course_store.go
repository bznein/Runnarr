package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const courseSummarySelect = `
	select courses.id::text, courses.name, courses.sport_type, courses.notes, courses.favorite, courses.revision, courses.geometry_hash,
		courses.distance_m, courses.elevation_gain_m, courses.elevation_loss_m, courses.elevation_coverage,
		courses.point_count, courses.leg_count, courses.direct_leg_count, courses.diagnostics, courses.created_at, courses.updated_at
	from courses
`

type courseScanner interface {
	Scan(dest ...any) error
}

func scanCourseSummary(row courseScanner) (CourseSummary, error) {
	var course CourseSummary
	var sport string
	var gain, loss sql.NullFloat64
	var diagnostics []byte
	err := row.Scan(
		&course.ID, &course.Name, &sport, &course.Notes, &course.Favorite, &course.Revision, &course.GeometryHash,
		&course.DistanceM, &gain, &loss, &course.ElevationCoverage,
		&course.PointCount, &course.LegCount, &course.DirectLegCount, &diagnostics, &course.CreatedAt, &course.UpdatedAt,
	)
	if err != nil {
		return CourseSummary{}, err
	}
	course.SportType = CourseSport(sport)
	if gain.Valid {
		course.ElevationGainM = cloneFloat(&gain.Float64)
	}
	if loss.Valid {
		course.ElevationLossM = cloneFloat(&loss.Float64)
	}
	if len(diagnostics) > 0 {
		_ = json.Unmarshal(diagnostics, &course.Diagnostics)
	}
	if course.Diagnostics == nil {
		course.Diagnostics = map[string]any{}
	}
	return course, nil
}

func (s *Store) ListCourses(ctx context.Context, options CourseListOptions) (CourseListPage, error) {
	if options.Limit <= 0 || options.Limit > 100 {
		options.Limit = 100
	}
	if options.Offset < 0 {
		options.Offset = 0
	}
	conditions := []string{"user_id = $1"}
	args := []any{scopedUserID(ctx)}
	add := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}
	if query := strings.TrimSpace(options.Query); query != "" {
		add("(name ilike '%%' || $%[1]d || '%%' or notes ilike '%%' || $%[1]d || '%%')", query)
	}
	if options.Sport != "" {
		if !validCourseSport(CourseSport(options.Sport)) {
			return CourseListPage{}, fmt.Errorf("%w: invalid sport filter", ErrCourseInvalid)
		}
		add("sport_type = $%d", options.Sport)
	}
	if options.Favorite != nil {
		add("favorite = $%d", *options.Favorite)
	}
	sortExpression := "updated_at"
	switch options.Sort {
	case "", "updated":
	case "name":
		sortExpression = "lower(name)"
	case "distance":
		sortExpression = "distance_m"
	case "elevation_gain":
		sortExpression = "elevation_gain_m"
	default:
		return CourseListPage{}, fmt.Errorf("%w: invalid sort", ErrCourseInvalid)
	}
	direction := "desc"
	if options.Order == "asc" {
		direction = "asc"
	} else if options.Order != "" && options.Order != "desc" {
		return CourseListPage{}, fmt.Errorf("%w: invalid sort order", ErrCourseInvalid)
	}
	args = append(args, options.Limit+1, options.Offset)
	query := courseSummarySelect + " where " + strings.Join(conditions, " and ") +
		fmt.Sprintf(" order by %s %s nulls last, id limit $%d offset $%d", sortExpression, direction, len(args)-1, len(args))
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return CourseListPage{}, err
	}
	defer rows.Close()
	items := make([]CourseSummary, 0, options.Limit+1)
	for rows.Next() {
		item, scanErr := scanCourseSummary(rows)
		if scanErr != nil {
			return CourseListPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return CourseListPage{}, err
	}
	hasMore := len(items) > options.Limit
	if hasMore {
		items = items[:options.Limit]
	}
	page := CourseListPage{Courses: items, Limit: options.Limit, Offset: options.Offset, HasMore: hasMore}
	if hasMore {
		page.NextOffset = options.Offset + options.Limit
	}
	return page, nil
}

func (s *Store) GetCourse(ctx context.Context, id string) (Course, error) {
	summary, err := scanCourseSummary(s.db.QueryRow(ctx, courseSummarySelect+` where id = $1 and user_id = $2`, id, scopedUserID(ctx)))
	if err != nil {
		return Course{}, err
	}
	rows, err := s.db.Query(ctx, `
		select id::text, leg_index, mode, st_asgeojson(geometry, 9), elevations
		from course_legs where course_id = $1 order by leg_index
	`, id)
	if err != nil {
		return Course{}, err
	}
	defer rows.Close()
	legs := make([]CourseLeg, 0, summary.LegCount)
	for rows.Next() {
		var leg CourseLeg
		var mode, geometryJSON string
		var elevations []pgtype.Float8
		if err := rows.Scan(&leg.ID, &leg.Index, &mode, &geometryJSON, &elevations); err != nil {
			return Course{}, err
		}
		leg.Mode = CourseLegMode(mode)
		points, err := coursePointsFromGeoJSON(geometryJSON, elevations)
		if err != nil {
			return Course{}, err
		}
		leg.Points = points
		legs = append(legs, leg)
	}
	if err := rows.Err(); err != nil {
		return Course{}, err
	}
	course := Course{CourseSummary: summary, Legs: legs}
	if err := finalizeCourse(&course); err != nil {
		return Course{}, err
	}
	course.ID = summary.ID
	course.Favorite = summary.Favorite
	course.Revision = summary.Revision
	course.CreatedAt = summary.CreatedAt
	course.UpdatedAt = summary.UpdatedAt
	course.Diagnostics = summary.Diagnostics
	boundCourseLegPayload(&course, maxCourseMapPreview)
	return course, nil
}

func (s *Store) FindCourseByGeometryHash(ctx context.Context, hash string) (CourseSummary, error) {
	return scanCourseSummary(s.db.QueryRow(ctx, courseSummarySelect+`
		where user_id = $1 and geometry_hash = $2 order by updated_at desc limit 1
	`, scopedUserID(ctx), hash))
}

func (s *Store) CreateCourse(ctx context.Context, course Course, rejectDuplicate bool) (Course, error) {
	if _, _, _, err := normalizeCourseDetails(course.Name, course.SportType, course.Notes); err != nil {
		return Course{}, err
	}
	if err := finalizeCourse(&course); err != nil {
		return Course{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Course{}, err
	}
	defer tx.Rollback(ctx)
	if rejectDuplicate {
		var exists bool
		if err := tx.QueryRow(ctx, `select exists(select 1 from courses where user_id = $1 and geometry_hash = $2)`, scopedUserID(ctx), course.GeometryHash).Scan(&exists); err != nil {
			return Course{}, err
		}
		if exists {
			return Course{}, ErrCourseDuplicate
		}
	}
	id, err := insertCourseTx(ctx, tx, scopedUserID(ctx), course)
	if err != nil {
		return Course{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Course{}, err
	}
	return s.GetCourse(ctx, id)
}

func insertCourseTx(ctx context.Context, tx pgx.Tx, userID string, course Course) (string, error) {
	diagnostics, err := json.Marshal(course.Diagnostics)
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `
		insert into courses(
			user_id, name, sport_type, notes, favorite, revision, geometry_hash,
			distance_m, elevation_gain_m, elevation_loss_m, elevation_coverage,
			point_count, leg_count, direct_leg_count, diagnostics
		) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		returning id::text
	`, userID, course.Name, course.SportType, course.Notes, course.Favorite, max(course.Revision, 1), course.GeometryHash,
		course.DistanceM, course.ElevationGainM, course.ElevationLossM, course.ElevationCoverage,
		course.PointCount, course.LegCount, course.DirectLegCount, diagnostics).Scan(&id)
	if err != nil {
		return "", err
	}
	if err := insertCourseGeometryTx(ctx, tx, id, course); err != nil {
		return "", err
	}
	return id, nil
}

func insertCourseGeometryTx(ctx context.Context, tx pgx.Tx, courseID string, course Course) error {
	for _, waypoint := range course.Waypoints {
		if _, err := tx.Exec(ctx, `
			insert into course_waypoints(course_id, waypoint_index, location)
			values($1, $2, st_setsrid(st_makepoint($3, $4), 4326))
		`, courseID, waypoint.Index, waypoint.Longitude, waypoint.Latitude); err != nil {
			return err
		}
	}
	for _, leg := range course.Legs {
		geometryJSON, err := courseLineGeoJSON(leg.Points)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			insert into course_legs(course_id, leg_index, mode, geometry, elevations)
			values($1, $2, $3, st_setsrid(st_geomfromgeojson($4), 4326), $5)
		`, courseID, leg.Index, leg.Mode, geometryJSON, coursePgElevations(leg.Points)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpdateCourseDetails(ctx context.Context, id string, input CourseDetailsInput) (Course, error) {
	name, sport, notes, err := normalizeCourseDetails(input.Name, input.SportType, input.Notes)
	if err != nil {
		return Course{}, err
	}
	command, err := s.db.Exec(ctx, `
		update courses set name = $1, sport_type = $2, notes = $3,
			revision = revision + 1, updated_at = now()
		where id = $4 and user_id = $5 and revision = $6
	`, name, sport, notes, id, scopedUserID(ctx), input.Revision)
	if err != nil {
		return Course{}, err
	}
	if command.RowsAffected() == 0 {
		return Course{}, s.courseMutationMiss(ctx, id)
	}
	return s.GetCourse(ctx, id)
}

func (s *Store) UpdateCoursePlan(ctx context.Context, id string, input CoursePlanInput) (Course, error) {
	current, err := s.GetCourse(ctx, id)
	if err != nil {
		return Course{}, err
	}
	if current.Revision != input.Revision {
		return Course{}, ErrCourseConflict
	}
	prepared, err := courseFromPlan(input, current.Diagnostics)
	if err != nil {
		return Course{}, err
	}
	diagnostics, err := json.Marshal(prepared.Diagnostics)
	if err != nil {
		return Course{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Course{}, err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `
		update courses set name=$1, sport_type=$2, notes=$3, geometry_hash=$4,
			distance_m=$5, elevation_gain_m=$6, elevation_loss_m=$7, elevation_coverage=$8,
			point_count=$9, leg_count=$10, direct_leg_count=$11, diagnostics=$12,
			revision=revision+1, updated_at=now()
		where id=$13 and user_id=$14 and revision=$15
	`, prepared.Name, prepared.SportType, prepared.Notes, prepared.GeometryHash,
		prepared.DistanceM, prepared.ElevationGainM, prepared.ElevationLossM, prepared.ElevationCoverage,
		prepared.PointCount, prepared.LegCount, prepared.DirectLegCount, diagnostics,
		id, scopedUserID(ctx), input.Revision)
	if err != nil {
		return Course{}, err
	}
	if command.RowsAffected() == 0 {
		return Course{}, ErrCourseConflict
	}
	if _, err := tx.Exec(ctx, `delete from course_waypoints where course_id=$1`, id); err != nil {
		return Course{}, err
	}
	if _, err := tx.Exec(ctx, `delete from course_legs where course_id=$1`, id); err != nil {
		return Course{}, err
	}
	if err := insertCourseGeometryTx(ctx, tx, id, prepared); err != nil {
		return Course{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Course{}, err
	}
	return s.GetCourse(ctx, id)
}

func (s *Store) SetCourseFavorite(ctx context.Context, id string, favorite bool) (Course, error) {
	command, err := s.db.Exec(ctx, `update courses set favorite=$1 where id=$2 and user_id=$3`, favorite, id, scopedUserID(ctx))
	if err != nil {
		return Course{}, err
	}
	if command.RowsAffected() == 0 {
		return Course{}, pgx.ErrNoRows
	}
	return s.GetCourse(ctx, id)
}

func (s *Store) DuplicateCourse(ctx context.Context, id string, input CourseDuplicateInput) (Course, error) {
	current, err := s.GetCourse(ctx, id)
	if err != nil {
		return Course{}, err
	}
	if current.Revision != input.Revision {
		return Course{}, ErrCourseConflict
	}
	name, sport, notes, err := normalizeCourseDetails(input.Name, current.SportType, input.Notes)
	if err != nil {
		return Course{}, err
	}
	current.ID = ""
	current.Name, current.SportType, current.Notes = name, sport, notes
	current.Favorite = false
	current.Revision = 1
	for index := range current.Legs {
		current.Legs[index].ID = ""
	}
	for index := range current.Waypoints {
		current.Waypoints[index].ID = ""
	}
	return s.CreateCourse(ctx, current, false)
}

func (s *Store) DeleteCourse(ctx context.Context, id string, revision int) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `delete from courses where id=$1 and user_id=$2 and revision=$3`, id, scopedUserID(ctx), revision)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return s.courseMutationMiss(ctx, id)
	}
	if _, err := tx.Exec(ctx, `
		delete from course_imports imports
		where imports.user_id=$1
		and not exists(select 1 from course_import_items items where items.import_id=imports.id)
	`, scopedUserID(ctx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CommitCourseImport(ctx context.Context, filename, hash string, diagnostics []CourseImportDiagnostic, keys []string, courses []Course) (CourseImportResult, error) {
	if len(courses) == 0 || len(courses) != len(keys) {
		return CourseImportResult{}, fmt.Errorf("%w: choose at least one valid GPX course", ErrCourseInvalid)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CourseImportResult{}, err
	}
	defer tx.Rollback(ctx)
	for _, course := range courses {
		var exists bool
		if err := tx.QueryRow(ctx, `select exists(select 1 from courses where user_id=$1 and geometry_hash=$2)`, scopedUserID(ctx), course.GeometryHash).Scan(&exists); err != nil {
			return CourseImportResult{}, err
		}
		if exists {
			return CourseImportResult{}, ErrCourseDuplicate
		}
	}
	diagnosticsJSON, err := json.Marshal(diagnostics)
	if err != nil {
		return CourseImportResult{}, err
	}
	var importID string
	if err := tx.QueryRow(ctx, `
		insert into course_imports(user_id, filename, file_sha256, diagnostics)
		values($1,$2,$3,$4) returning id::text
	`, scopedUserID(ctx), filename, hash, diagnosticsJSON).Scan(&importID); err != nil {
		return CourseImportResult{}, err
	}
	ids := make([]string, len(courses))
	for index, course := range courses {
		id, err := insertCourseTx(ctx, tx, scopedUserID(ctx), course)
		if err != nil {
			return CourseImportResult{}, err
		}
		ids[index] = id
		if _, err := tx.Exec(ctx, `insert into course_import_items(import_id, course_id, candidate_key) values($1,$2,$3)`, importID, id, keys[index]); err != nil {
			return CourseImportResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return CourseImportResult{}, err
	}
	result := CourseImportResult{ImportID: importID, Filename: filename, FileSHA256: hash, Diagnostics: diagnostics, Created: make([]CourseSummary, 0, len(ids))}
	for _, id := range ids {
		course, err := s.GetCourse(ctx, id)
		if err != nil {
			return CourseImportResult{}, err
		}
		result.Created = append(result.Created, course.CourseSummary)
	}
	return result, nil
}

func (s *Store) GetCourseImport(ctx context.Context, id string) (CourseImportResult, error) {
	var result CourseImportResult
	var diagnostics []byte
	if err := s.db.QueryRow(ctx, `
		select id::text, filename, file_sha256, diagnostics
		from course_imports where id=$1 and user_id=$2
	`, id, scopedUserID(ctx)).Scan(&result.ImportID, &result.Filename, &result.FileSHA256, &diagnostics); err != nil {
		return CourseImportResult{}, err
	}
	if len(diagnostics) > 0 {
		_ = json.Unmarshal(diagnostics, &result.Diagnostics)
	}
	rows, err := s.db.Query(ctx, courseSummarySelect+`
		join course_import_items items on items.course_id=courses.id
		where items.import_id=$1 and courses.user_id=$2
		order by courses.created_at, courses.id
	`, id, scopedUserID(ctx))
	if err != nil {
		return CourseImportResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		course, scanErr := scanCourseSummary(rows)
		if scanErr != nil {
			return CourseImportResult{}, scanErr
		}
		result.Created = append(result.Created, course)
	}
	return result, rows.Err()
}

func (s *Store) courseMutationMiss(ctx context.Context, id string) error {
	var exists bool
	if err := s.db.QueryRow(ctx, `select exists(select 1 from courses where id=$1 and user_id=$2)`, id, scopedUserID(ctx)).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrCourseConflict
	}
	return pgx.ErrNoRows
}

func courseLineGeoJSON(points []CoursePoint) (string, error) {
	coordinates := make([][2]float64, len(points))
	for index, point := range points {
		coordinates[index] = [2]float64{point.Longitude, point.Latitude}
	}
	data, err := json.Marshal(map[string]any{"type": "LineString", "coordinates": coordinates})
	return string(data), err
}

func coursePointsFromGeoJSON(raw string, elevations []pgtype.Float8) ([]CoursePoint, error) {
	var geometry struct {
		Type        string      `json:"type"`
		Coordinates [][]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(raw), &geometry); err != nil {
		return nil, err
	}
	if geometry.Type != "LineString" || len(geometry.Coordinates) != len(elevations) {
		return nil, fmt.Errorf("%w: stored course geometry is inconsistent", ErrCourseInvalid)
	}
	points := make([]CoursePoint, len(geometry.Coordinates))
	for index, coordinate := range geometry.Coordinates {
		if len(coordinate) < 2 {
			return nil, fmt.Errorf("%w: stored coordinate is invalid", ErrCourseInvalid)
		}
		points[index] = CoursePoint{Latitude: coordinate[1], Longitude: coordinate[0]}
		if elevations[index].Valid {
			points[index].ElevationM = cloneFloat(&elevations[index].Float64)
		}
	}
	return points, nil
}

func coursePgElevations(points []CoursePoint) []pgtype.Float8 {
	result := make([]pgtype.Float8, len(points))
	for index, point := range points {
		if point.ElevationM != nil {
			result[index] = pgtype.Float8{Float64: *point.ElevationM, Valid: true}
		}
	}
	return result
}

func boundCourseLegPayload(course *Course, maxPoints int) {
	if course == nil || course.PointCount <= maxPoints {
		return
	}
	for index := range course.Legs {
		leg := &course.Legs[index]
		allocation := max(2, int(float64(maxPoints)*float64(len(leg.Points))/float64(course.PointCount)))
		bounded := boundedCoursePoints(leg.Points, allocation)
		leg.EncodedPolyline = encodeCoursePolyline(bounded, 6)
		leg.ElevationsM = courseElevations(bounded)
		leg.PointCount = len(leg.Points)
	}
}

func isCourseNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
