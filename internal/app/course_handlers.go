package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (s *Server) handleListCourses(w http.ResponseWriter, r *http.Request) {
	options := CourseListOptions{
		Query:  r.URL.Query().Get("q"),
		Sport:  r.URL.Query().Get("sport"),
		Sort:   r.URL.Query().Get("sort"),
		Order:  r.URL.Query().Get("order"),
		Limit:  queryInteger(r, "limit"),
		Offset: queryInteger(r, "offset"),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("favorite")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "favorite must be true or false")
			return
		}
		options.Favorite = &value
	}
	page, err := s.store.ListCourses(r.Context(), options)
	if errors.Is(err, ErrCourseInvalid) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		s.logger.Error("list courses", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load courses")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleGetCourse(w http.ResponseWriter, r *http.Request) {
	course, err := s.store.GetCourse(r.Context(), chi.URLParam(r, "id"))
	if writeCourseStoreError(w, err, "could not load course") {
		return
	}
	writeJSON(w, http.StatusOK, course)
}

func (s *Server) handleCreateCourse(w http.ResponseWriter, r *http.Request) {
	var input CoursePlanInput
	if err := decodeJSONBody(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	course, err := courseFromPlan(input, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.store.CreateCourse(r.Context(), course, false)
	if writeCourseStoreError(w, err, "could not create course") {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateCourseDetails(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	current, err := s.store.GetCourse(r.Context(), id)
	if writeCourseStoreError(w, err, "could not load course") {
		return
	}
	var input CourseDetailsInput
	if err := decodeJSONBody(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if input.SportType != current.SportType {
		for _, leg := range current.Legs {
			if leg.Mode == CourseLegRouted {
				writeError(w, http.StatusConflict, "changing sport requires recalculating routed legs in the course planner")
				return
			}
		}
	}
	updated, err := s.store.UpdateCourseDetails(r.Context(), id, input)
	if writeCourseStoreError(w, err, "could not update course") {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleUpdateCoursePlan(w http.ResponseWriter, r *http.Request) {
	var input CoursePlanInput
	if err := decodeJSONBody(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	updated, err := s.store.UpdateCoursePlan(r.Context(), chi.URLParam(r, "id"), input)
	if writeCourseStoreError(w, err, "could not update course") {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleSetCourseFavorite(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Favorite bool `json:"favorite"`
	}
	if err := decodeJSONBody(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	updated, err := s.store.SetCourseFavorite(r.Context(), chi.URLParam(r, "id"), input.Favorite)
	if writeCourseStoreError(w, err, "could not update course favorite") {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDuplicateCourse(w http.ResponseWriter, r *http.Request) {
	var input CourseDuplicateInput
	if err := decodeJSONBody(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	created, err := s.store.DuplicateCourse(r.Context(), chi.URLParam(r, "id"), input)
	if writeCourseStoreError(w, err, "could not duplicate course") {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleDeleteCourse(w http.ResponseWriter, r *http.Request) {
	revision, err := strconv.Atoi(r.URL.Query().Get("revision"))
	if err != nil || revision <= 0 {
		writeError(w, http.StatusBadRequest, "revision must be a positive integer")
		return
	}
	if err := s.store.DeleteCourse(r.Context(), chi.URLParam(r, "id"), revision); writeCourseStoreError(w, err, "could not delete course") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleExportCourseGPX(w http.ResponseWriter, r *http.Request) {
	course, err := s.store.GetCourse(r.Context(), chi.URLParam(r, "id"))
	if writeCourseStoreError(w, err, "could not load course") {
		return
	}
	data, err := exportCourseGPX(course)
	if err != nil {
		s.logger.Error("export course GPX", "course_id", course.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "could not export course GPX")
		return
	}
	w.Header().Set("Content-Type", "application/gpx+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": courseGPXFilename(course)}))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handlePreviewCourseImport(w http.ResponseWriter, r *http.Request) {
	filename, data, err := readCourseGPXUpload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	preview, err := previewCourseGPX(filename, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for index := range preview.Candidates {
		candidate := &preview.Candidates[index]
		if !candidate.Valid {
			continue
		}
		duplicate, err := s.store.FindCourseByGeometryHash(r.Context(), candidate.course.GeometryHash)
		if err == nil {
			candidate.DuplicateCourse = &duplicate
		} else if !errors.Is(err, pgx.ErrNoRows) {
			s.logger.Error("check course import duplicate", "error", err)
			writeError(w, http.StatusInternalServerError, "could not preview course import")
			return
		}
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleCommitCourseImport(w http.ResponseWriter, r *http.Request) {
	filename, data, err := readCourseGPXUpload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var input CourseImportCommitInput
	if err := json.Unmarshal([]byte(r.FormValue("input")), &input); err != nil {
		writeError(w, http.StatusBadRequest, "input must contain valid import selections")
		return
	}
	preview, err := previewCourseGPX(filename, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.FileSHA256 == "" || input.FileSHA256 != preview.FileSHA256 {
		writeError(w, http.StatusConflict, "GPX file changed after preview; preview it again")
		return
	}
	byKey := make(map[string]CourseImportCandidate, len(preview.Candidates))
	for _, candidate := range preview.Candidates {
		byKey[candidate.Key] = candidate
	}
	seen := make(map[string]struct{}, len(input.Selections))
	keys := make([]string, 0, len(input.Selections))
	courses := make([]Course, 0, len(input.Selections))
	for _, selection := range input.Selections {
		if _, duplicate := seen[selection.Key]; duplicate {
			writeError(w, http.StatusBadRequest, "each GPX candidate may be selected only once")
			return
		}
		seen[selection.Key] = struct{}{}
		candidate, ok := byKey[selection.Key]
		if !ok || !candidate.Valid {
			writeError(w, http.StatusBadRequest, "selected GPX candidate is not valid")
			return
		}
		course, err := selectedImportedCourse(candidate, selection)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(preview.Diagnostics) > 0 {
			course.Diagnostics["fileWarnings"] = preview.Diagnostics
		}
		keys = append(keys, selection.Key)
		courses = append(courses, course)
	}
	result, err := s.store.CommitCourseImport(r.Context(), preview.Filename, preview.FileSHA256, preview.Diagnostics, keys, courses)
	if writeCourseStoreError(w, err, "could not import courses") {
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleGetCourseImport(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.GetCourseImport(r.Context(), chi.URLParam(r, "id"))
	if writeCourseStoreError(w, err, "could not load course import") {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleSaveActivityAsCourse(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name      string      `json:"name"`
		SportType CourseSport `json:"sportType"`
		Notes     string      `json:"notes"`
	}
	if err := decodeJSONBody(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	activity, err := s.store.GetActivity(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "activity not found")
		return
	}
	if err != nil {
		s.logger.Error("load activity for course", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load activity")
		return
	}
	timed := make([]timedCoursePoint, 0, len(activity.Samples))
	for _, sample := range activity.Samples {
		if sample.Latitude == nil || sample.Longitude == nil || !validCourseCoordinate(*sample.Latitude, *sample.Longitude) {
			continue
		}
		timed = append(timed, timedCoursePoint{CoursePoint: CoursePoint{
			Latitude: *sample.Latitude, Longitude: *sample.Longitude, ElevationM: cloneFloat(sample.ElevationM),
		}, Timestamp: sample.Timestamp})
	}
	if len(timed) < 2 {
		writeError(w, http.StatusBadRequest, "activity has fewer than two valid GPS points")
		return
	}
	if index, invalid := courseHasImplausibleJump(timed, input.SportType); invalid {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("activity route has an implausible GPS jump before point %d", index+1))
		return
	}
	points := make([]CoursePoint, len(timed))
	for index := range timed {
		points[index] = timed[index].CoursePoint
	}
	course, err := preservedCourseFromPoints(input.Name, input.SportType, input.Notes, points, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.store.CreateCourse(r.Context(), course, true)
	if writeCourseStoreError(w, err, "could not save activity as course") {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func readCourseGPXUpload(r *http.Request) (string, []byte, error) {
	if err := r.ParseMultipartForm(maxCourseImportBytes + 1); err != nil {
		return "", nil, errors.New("invalid multipart GPX upload")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return "", nil, errors.New("GPX file is required")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCourseImportBytes+1))
	if err != nil {
		return "", nil, errors.New("could not read GPX file")
	}
	if len(data) > maxCourseImportBytes {
		return "", nil, ErrCourseGPXTooLarge
	}
	return header.Filename, data, nil
}

func decodeJSONBody(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func queryInteger(r *http.Request, key string) int {
	value, _ := strconv.Atoi(r.URL.Query().Get(key))
	return value
}

func writeCourseStoreError(w http.ResponseWriter, err error, fallback string) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "course not found")
	case errors.Is(err, ErrCourseInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrCourseConflict):
		writeError(w, http.StatusConflict, ErrCourseConflict.Error())
	case errors.Is(err, ErrCourseDuplicate):
		writeError(w, http.StatusConflict, ErrCourseDuplicate.Error())
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
	return true
}
