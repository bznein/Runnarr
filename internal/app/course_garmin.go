package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type CourseGarminExport struct {
	ID               string         `json:"-"`
	CourseID         string         `json:"-"`
	CourseRevision   int            `json:"courseRevision"`
	Status           string         `json:"status"`
	ProviderCourseID string         `json:"providerCourseId,omitempty"`
	ProviderURL      string         `json:"providerUrl,omitempty"`
	Error            string         `json:"error,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	OwnershipMarker  string         `json:"-"`
	ProviderResponse map[string]any `json:"-"`
}

type CourseGarminStatus struct {
	Connected bool `json:"connected"`
	Current   bool `json:"current"`
	CourseGarminExport
}

func (s *Store) GarminCourseOwnerToken(ctx context.Context) (string, error) {
	var token string
	err := s.db.QueryRow(ctx, `select garmin_course_owner_token::text from user_settings where user_id = $1`, scopedUserID(ctx)).Scan(&token)
	return token, err
}

func (s *Store) ReconcileRunningCourseGarminExports(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		update course_garmin_exports set
			status = 'attention',
			error = 'Runnarr stopped before the Garmin response was recorded; the remote outcome is unknown and this course revision will not be retried automatically',
			updated_at = now()
		where status = 'sending'
	`)
	return err
}

func (s *Store) GetLatestCourseGarminExport(ctx context.Context, courseID string) (CourseGarminExport, error) {
	return scanCourseGarminExport(s.db.QueryRow(ctx, `
		select exports.id::text, exports.course_id::text, exports.course_revision, exports.status,
			exports.ownership_marker, exports.provider_course_id, exports.provider_url,
			exports.provider_response, exports.error, exports.created_at, exports.updated_at
		from course_garmin_exports exports
		join courses on courses.id = exports.course_id
		where exports.user_id = $1 and exports.course_id = $2 and courses.user_id = $1
		order by exports.course_revision desc, exports.created_at desc
		limit 1
	`, scopedUserID(ctx), courseID))
}

func (s *Store) CreateCourseGarminExport(ctx context.Context, course Course, ownershipMarker string) (CourseGarminExport, bool, error) {
	row := s.db.QueryRow(ctx, `
		insert into course_garmin_exports(user_id, course_id, course_revision, status, ownership_marker)
		select $1, id, $3, 'sending', $4 from courses where id = $2 and user_id = $1
		on conflict(user_id, course_id, course_revision) do nothing
		returning id::text, course_id::text, course_revision, status, ownership_marker,
			provider_course_id, provider_url, provider_response, error, created_at, updated_at
	`, scopedUserID(ctx), course.ID, course.Revision, ownershipMarker)
	export, err := scanCourseGarminExport(row)
	if err == nil {
		return export, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CourseGarminExport{}, false, err
	}
	export, err = s.GetLatestCourseGarminExport(ctx, course.ID)
	return export, false, err
}

func (s *Store) MarkCourseGarminExportAttention(ctx context.Context, exportID, message string, remote GarminBridgeCourse) error {
	return s.finishCourseGarminExport(ctx, exportID, "attention", remote.ID, remote.URL, message, remote.Raw)
}

func (s *Store) MarkCourseGarminExportSent(ctx context.Context, exportID string, remote GarminBridgeCourse) error {
	return s.finishCourseGarminExport(ctx, exportID, "sent", remote.ID, remote.URL, "", remote.Raw)
}

func (s *Store) finishCourseGarminExport(ctx context.Context, exportID, status, providerCourseID, providerURL, message string, response map[string]any) error {
	if response == nil {
		response = map[string]any{}
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(ctx, `
		update course_garmin_exports set
			status = $3, provider_course_id = $4, provider_url = $5,
			provider_response = $6, error = $7, updated_at = now()
		where id = $1 and user_id = $2 and status = 'sending'
	`, exportID, scopedUserID(ctx), status, providerCourseID, providerURL, raw, message)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("Garmin course send state changed unexpectedly")
	}
	return nil
}

func scanCourseGarminExport(row courseScanner) (CourseGarminExport, error) {
	var export CourseGarminExport
	var raw []byte
	err := row.Scan(&export.ID, &export.CourseID, &export.CourseRevision, &export.Status,
		&export.OwnershipMarker, &export.ProviderCourseID, &export.ProviderURL,
		&raw, &export.Error, &export.CreatedAt, &export.UpdatedAt)
	if err != nil {
		return export, err
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &export.ProviderResponse)
	}
	if export.ProviderResponse == nil {
		export.ProviderResponse = map[string]any{}
	}
	return export, nil
}

func garminCourseOwnershipMarker(ownerToken string, course Course) (string, error) {
	ownerToken = strings.TrimSpace(ownerToken)
	geometryHash := strings.TrimSpace(course.GeometryHash)
	if ownerToken == "" || geometryHash == "" {
		return "", errors.New("Garmin course ownership data is incomplete")
	}
	return fmt.Sprintf("runnarr-course:%s:%s", ownerToken, geometryHash), nil
}

func verifyGarminCourseUpload(remote GarminBridgeCourse, ownershipMarker string) error {
	if strings.TrimSpace(remote.ID) == "" || strings.TrimSpace(remote.Description) != strings.TrimSpace(ownershipMarker) {
		return errors.New("uploaded Garmin course could not be ownership-verified")
	}
	return nil
}

func (s *GarminService) SendCourse(ctx context.Context, course Course, ownershipMarker string) (GarminBridgeCourse, error) {
	data, err := exportCourseGPX(course)
	if err != nil {
		return GarminBridgeCourse{}, err
	}
	remote, err := s.bridge.UploadCourse(ctx, s.tokenStore(ctx), courseGPXFilename(course), data, course.Name, course.SportType, ownershipMarker)
	if err != nil {
		return GarminBridgeCourse{}, err
	}
	if remote.URL == "" && remote.ID != "" {
		remote.URL = "https://connect.garmin.com/modern/course/" + remote.ID
	}
	return remote, nil
}

func (s *GarminService) GetCourse(ctx context.Context, courseID string) (GarminBridgeCourse, error) {
	return s.bridge.GetCourse(ctx, s.tokenStore(ctx), courseID)
}
