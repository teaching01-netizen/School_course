package activecourseshttp

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/admin/active-courses", s.handleList)
	mux.HandleFunc("PUT /api/v1/admin/active-courses", s.handleSet)
	mux.HandleFunc("PUT /api/v1/admin/active-courses/set-active", s.handleSetActive)
	mux.HandleFunc("PUT /api/v1/admin/active-courses/set-active/bulk", s.handleSetActiveBulk)
}

type courseDTO struct {
	CourseID           string  `json:"course_id"`
	CourseCode         string  `json:"course_code"`
	CourseName         string  `json:"course_name"`
	CycleID            string  `json:"cycle_id"`
	CycleLabel         string  `json:"cycle_label"`
	IsActive           bool    `json:"is_active"`
	AbsenceFormVisible bool    `json:"absence_form_visible"`
	MergeGroupName     *string `json:"merge_group_name,omitempty"`
}

type subjectDTO struct {
	SubjectID   string      `json:"subject_id"`
	SubjectCode string      `json:"subject_code"`
	SubjectName string      `json:"subject_name"`
	Courses     []courseDTO `json:"courses"`
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	limit, offset, paginated, err := parsePagination(r.URL.Query())
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_pagination", err.Error())
		return
	}
	search, statusFilter, err := parseListFilters(r.URL.Query())
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_filter", err.Error())
		return
	}
	if paginated {
		subjects, coursesBySubject, totalSubjects, totalCourses, err := s.deps.Q.ActiveCoursesListPaginated(r.Context(), sqldb.ActiveCoursesListParams{
			Limit: limit, Offset: offset, Search: search, Status: statusFilter,
		})
		if err != nil {
			s.err(w, err)
			return
		}
		stats, err := s.deps.Q.ActiveCoursesStats(r.Context())
		if err != nil {
			s.err(w, err)
			return
		}
		out, err := outSubjectDTOs(s, subjects, coursesBySubject)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		s.a.WriteJSON(w, http.StatusOK, map[string]any{
			"subjects": out, "total_subjects": totalSubjects,
			"total_courses": totalCourses, "limit": limit, "offset": offset,
			"stats": map[string]int64{
				"total_subjects": stats.Total,
				"missing_active": stats.MissingActive,
				"hidden_active":  stats.HiddenActive,
			},
		})
		return
	}

	subjects, coursesBySubject, err := s.deps.Q.ActiveCoursesList(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	out, err := outSubjectDTOs(s, subjects, coursesBySubject)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"subjects": out})
}

func outSubjectDTOs(s *server, subjects []sqldb.ActiveCourseSubjectRow, coursesBySubject [][]sqldb.ActiveCourseCourseRow) ([]subjectDTO, error) {
	out := make([]subjectDTO, 0, len(subjects))
	for i, subj := range subjects {
		subjID, err := s.a.UUIDString(subj.SubjectID)
		if err != nil {
			return nil, err
		}
		courses := coursesBySubject[i]
		courseDTOs := make([]courseDTO, 0, len(courses))
		for _, c := range courses {
			cID, err := s.a.UUIDString(c.CourseID)
			if err != nil {
				return nil, err
			}
			cycleID := ""
			if c.CycleID.Valid {
				cycleID = c.CycleID.String
			}
			var mergeGroupName *string
			if c.MergeGroupName.Valid {
				name := c.MergeGroupName.String
				mergeGroupName = &name
			}
			courseDTOs = append(courseDTOs, courseDTO{
				CourseID:           cID,
				CourseCode:         c.CourseCode,
				CourseName:         c.CourseName,
				CycleID:            cycleID,
				CycleLabel:         c.CycleLabel,
				IsActive:           c.IsActive,
				AbsenceFormVisible: c.AbsenceFormVisible,
				MergeGroupName:     mergeGroupName,
			})
		}
		out = append(out, subjectDTO{
			SubjectID:   subjID,
			SubjectCode: subj.SubjectCode,
			SubjectName: subj.SubjectName,
			Courses:     courseDTOs,
		})
	}

	return out, nil
}

func parsePagination(query url.Values) (int, int, bool, error) {
	_, hasLimit := query["limit"]
	_, hasOffset := query["offset"]
	if !hasLimit && !hasOffset {
		return 0, 0, false, nil
	}
	limit := 50
	if hasLimit {
		value, err := strconv.Atoi(query.Get("limit"))
		if err != nil || value < 1 || value > 200 {
			return 0, 0, true, fmt.Errorf("limit must be between 1 and 200")
		}
		limit = value
	}
	offset := 0
	if hasOffset {
		value, err := strconv.Atoi(query.Get("offset"))
		if err != nil || value < 0 {
			return 0, 0, true, fmt.Errorf("offset must be non-negative")
		}
		offset = value
	}
	return limit, offset, true, nil
}

func (s *server) err(w http.ResponseWriter, dbErr error) {
	status, code, msg := s.a.ClassifyDBErr(dbErr)
	s.a.WriteErr(w, status, code, msg)
}

// parseListFilters extracts the optional subject search text and active-course
// status filter shared by the operations filter chips.
func parseListFilters(query url.Values) (string, string, error) {
	search := strings.TrimSpace(query.Get("search"))
	if len(search) > 100 {
		search = search[:100]
	}
	status := strings.TrimSpace(query.Get("status"))
	switch status {
	case "", sqldb.ActiveCourseStatusAll, sqldb.ActiveCourseStatusConfigured,
		sqldb.ActiveCourseStatusHiddenActive, sqldb.ActiveCourseStatusMissingActive:
		return search, status, nil
	default:
		return "", "", fmt.Errorf("status must be one of all, configured, hidden_active, missing_active")
	}
}

// handleSet is the CourseLevels single-picker endpoint: the chosen course
// becomes the subject's only active class, replacing any previously active
// ones. For additive multi-active control the operations console uses
// PUT /set-active and /set-active/bulk instead.
func (s *server) handleSet(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	var body struct {
		SubjectID string `json:"subject_id"`
		CourseID  string `json:"course_id"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	subjectID, err := s.a.ParseUUID(body.SubjectID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
		return
	}
	courseID, err := s.a.ParseUUID(body.CourseID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", "Invalid course_id")
		return
	}

	s.a.WithIdempotentTx(w, r, user.ID, "active-courses", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		courseSubjectID, found, err := qtx.CourseSubjectID(r.Context(), courseID)
		if err != nil {
			s.err(w, err)
			return 0, nil, err
		} else if !found {
			s.a.WriteErr(w, http.StatusNotFound, "not_found", "Course not found")
			return 0, nil, fmt.Errorf("set active course: course %s not found", courseID.String())
		} else if courseSubjectID.Bytes != subjectID.Bytes {
			s.a.WriteErr(w, http.StatusBadRequest, "course_subject_mismatch",
				"Course does not belong to this subject")
			return 0, nil, fmt.Errorf("set active course: course %s belongs to a different subject", courseID.String())
		}
		// Single-picker semantics for the CourseLevels panel: the chosen course
		// becomes the subject's only active class, replacing any others.
		if err := qtx.ActiveCourseClearBySubject(r.Context(), subjectID); err != nil {
			s.err(w, err)
			return 0, nil, err
		}
		if _, err := qtx.ActiveCourseSetBulk(r.Context(), []string{courseID.String()}); err != nil {
			s.err(w, err)
			return 0, nil, err
		}
		return http.StatusOK, map[string]string{"status": "ok"}, nil
	})
}

const bulkActiveMaxCourses = 200

// handleSetActive is the operations console's switch: active means the class is
// visible in the student absence form and eligible for sit-ins; inactive means
// hidden and no sit-ins. A subject may run several active classes at once —
// toggling one class never changes its siblings. Staff booking is not affected.
func (s *server) handleSetActive(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	var body struct {
		CourseID string `json:"course_id"`
		Active   *bool  `json:"active"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	courseID, err := s.a.ParseUUID(body.CourseID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", "Invalid course_id")
		return
	}
	if body.Active == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_active", "active must be true or false")
		return
	}

	s.a.WithIdempotentTx(w, r, user.ID, "active-courses-set-active", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		if _, found, err := qtx.CourseSubjectID(r.Context(), courseID); err != nil {
			s.err(w, err)
			return 0, nil, err
		} else if !found {
			s.a.WriteErr(w, http.StatusNotFound, "not_found", "Course not found")
			return 0, nil, fmt.Errorf("set-active: course %s not found", courseID.String())
		}
		ids := []string{courseID.String()}
		var err error
		if *body.Active {
			_, err = qtx.ActiveCourseSetBulk(r.Context(), ids)
		} else {
			_, err = qtx.ActiveCourseClearBulk(r.Context(), ids)
		}
		if err != nil {
			s.err(w, err)
			return 0, nil, err
		}
		return http.StatusOK, map[string]string{"status": "ok"}, nil
	})
}

// handleSetActiveBulk applies the same switch to many courses at once:
// activating turns every selected class on (subjects keep as many actives as
// selected), deactivating turns every selected class off. The response reports
// how many courses had their state re-derived, siblings included.
func (s *server) handleSetActiveBulk(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	var body struct {
		CourseIDs []string `json:"course_ids"`
		Active    *bool    `json:"active"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.Active == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_active", "active must be true or false")
		return
	}
	if len(body.CourseIDs) == 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_course_ids", "course_ids must contain at least one course")
		return
	}
	if len(body.CourseIDs) > bulkActiveMaxCourses {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_course_ids",
			fmt.Sprintf("course_ids is limited to %d courses per bulk action", bulkActiveMaxCourses))
		return
	}
	seen := make(map[string]struct{}, len(body.CourseIDs))
	ids := make([]string, 0, len(body.CourseIDs))
	for _, raw := range body.CourseIDs {
		if _, err := s.a.ParseUUID(raw); err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", fmt.Sprintf("Invalid course_id: %s", raw))
			return
		}
		if _, dup := seen[raw]; dup {
			continue
		}
		seen[raw] = struct{}{}
		ids = append(ids, raw)
	}

	s.a.WithIdempotentTx(w, r, user.ID, "active-courses-set-active-bulk", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		var updated int64
		var err error
		if *body.Active {
			updated, err = qtx.ActiveCourseSetBulk(r.Context(), ids)
		} else {
			updated, err = qtx.ActiveCourseClearBulk(r.Context(), ids)
		}
		if err != nil {
			s.err(w, err)
			return 0, nil, err
		}
		if updated == 0 {
			s.a.WriteErr(w, http.StatusNotFound, "not_found", "No matching courses")
			return 0, nil, fmt.Errorf("set-active bulk: no courses matched %d ids", len(ids))
		}
		return http.StatusOK, map[string]int64{"updated": updated}, nil
	})
}
