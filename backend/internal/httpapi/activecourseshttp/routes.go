package activecourseshttp

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/httpapi/sessiondaterange"
	"warwick-institute/internal/realtime"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func (s *server) publishCourseUpdates(ids []string) {
	if s.deps.Realtime == nil {
		return
	}
	for _, id := range ids {
		if id != "" {
			s.deps.Realtime.Publish("courses:all", realtime.Event{Type: "course.updated", ID: id})
		}
	}
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
	RuleCourseID       string  `json:"rule_course_id"`
	RuleCourseCode     string  `json:"rule_course_code"`
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
	sessionDateFrom, sessionDateTo, err := sessiondaterange.Parse(r.URL.Query())
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_filter", err.Error())
		return
	}
	if !paginated && (sessionDateFrom != "" || sessionDateTo != "") {
		limit, offset, paginated = 50, 0, true
	}
	if paginated {
		ctx := r.Context()
		tx, err := s.deps.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			s.err(w, err)
			return
		}
		defer tx.Rollback(ctx)
		qtx := s.deps.Q.WithTx(tx)
		subjects, coursesBySubject, totalSubjects, totalCourses, err := qtx.ActiveCoursesListPaginated(ctx, sqldb.ActiveCoursesListParams{
			Limit: limit, Offset: offset, Search: search, Status: statusFilter,
			SessionDateFrom: sessionDateFrom, SessionDateTo: sessionDateTo, InstituteTZ: s.deps.InstituteTZ,
		})
		if err != nil {
			s.err(w, err)
			return
		}
		stats, err := qtx.ActiveCoursesStats(ctx)
		if err != nil {
			s.err(w, err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
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
			ruleCourseID, err := s.a.UUIDString(c.RuleCourseID)
			if err != nil {
				return nil, err
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
				RuleCourseID:       ruleCourseID,
				RuleCourseCode:     c.RuleCourseCode.String,
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

const bulkActiveMaxCourses = 200

type setActiveRequest struct {
	CourseID              string            `json:"course_id"`
	CourseIDs             []string          `json:"course_ids"`
	SubjectID             string            `json:"subject_id"`
	Active                *bool             `json:"active"`
	ExpectedRuleCourseIDs map[string]string `json:"expected_rule_course_ids"`
}

type bookingChange struct {
	SourceID string `json:"source_id"`
	Before   bool   `json:"before"`
	After    bool   `json:"after"`
}

func (s *server) handleSet(w http.ResponseWriter, r *http.Request) {
	s.setBooking(w, r, "active-courses", true, false)
}

func (s *server) handleSetActive(w http.ResponseWriter, r *http.Request) {
	s.setBooking(w, r, "active-courses-set-active", false, false)
}

func (s *server) handleSetActiveBulk(w http.ResponseWriter, r *http.Request) {
	s.setBooking(w, r, "active-courses-set-active-bulk", false, true)
}

func (s *server) setBooking(w http.ResponseWriter, r *http.Request, scope string, replace, bulk bool) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body setActiveRequest
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	var subjectID pgtype.UUID
	active := true
	if replace {
		var err error
		subjectID, err = s.a.ParseUUID(body.SubjectID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
			return
		}
	} else if body.Active == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_active", "active must be true or false")
		return
	} else {
		active = *body.Active
	}
	rawIDs := body.CourseIDs
	if !bulk {
		rawIDs = []string{body.CourseID}
	}
	if len(rawIDs) == 0 || len(rawIDs) > bulkActiveMaxCourses {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_course_ids", "Select between 1 and 200 courses")
		return
	}
	ids := make([]pgtype.UUID, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, err := s.a.ParseUUID(raw)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", "Invalid course_id")
			return
		}
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b pgtype.UUID) int { return strings.Compare(a.String(), b.String()) })
	ids = slices.Compact(ids)
	expectedSources := make(map[pgtype.UUID]pgtype.UUID, len(body.ExpectedRuleCourseIDs))
	for course, source := range body.ExpectedRuleCourseIDs {
		courseID, courseErr := s.a.ParseUUID(course)
		sourceID, sourceErr := s.a.ParseUUID(source)
		if courseErr != nil || sourceErr != nil || !slices.Contains(ids, courseID) {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_rule_source_precondition", "Provide a rule source ID for a selected course")
			return
		}
		if prior, exists := expectedSources[courseID]; exists && prior != sourceID {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_rule_source_precondition", "A course has conflicting rule source IDs")
			return
		}
		expectedSources[courseID] = sourceID
	}

	var affected []string
	s.a.WithRetryableIdempotentTx(w, r, user.ID, scope, s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		ctx := r.Context()
		qtx := s.deps.Q.WithTx(tx)
		if err := qtx.LockCourseBookingWrites(ctx); err != nil {
			return 0, nil, err
		}
		targets, err := qtx.CourseBookingTargets(ctx, ids)
		if err != nil {
			return 0, nil, err
		}
		byID := make(map[pgtype.UUID]sqldb.CourseBookingTarget, len(targets))
		for _, target := range targets {
			byID[target.CourseID] = target
		}
		reject := func(status int, code, message string, courseIDs []string) (int, any, error) {
			return status, map[string]any{"code": code, "message": message, "course_ids": courseIDs}, fmt.Errorf("%s", code)
		}
		var missing, subjectless, stale []string
		selectedSources := make([]pgtype.UUID, 0, len(ids))
		for _, id := range ids {
			target, exists := byID[id]
			if !exists {
				missing = append(missing, id.String())
				continue
			}
			if expected, ok := expectedSources[id]; ok && expected != target.SourceID {
				stale = append(stale, id.String())
			}
			if active && !target.SourceSubjectID.Valid {
				subjectless = append(subjectless, id.String())
			}
			if replace && (target.SubjectID != subjectID || target.SourceSubjectID != subjectID) {
				return reject(http.StatusBadRequest, "course_subject_mismatch", "Course and rule source must belong to this subject", []string{id.String()})
			}
			selectedSources = append(selectedSources, target.SourceID)
		}
		if len(missing) > 0 {
			return reject(http.StatusNotFound, "course_not_found", "Course not found", missing)
		}
		if len(stale) > 0 {
			return reject(http.StatusConflict, "course_rule_source_changed", "Course settings changed. Refresh the list and try again.", stale)
		}
		if len(subjectless) > 0 {
			return reject(http.StatusUnprocessableEntity, "course_subject_required", "Assign the rule source to a subject before turning it on", subjectless)
		}
		selected, err := qtx.CourseBookingStates(ctx, selectedSources)
		if err != nil {
			return 0, nil, err
		}
		desired := make(map[pgtype.UUID]bool, len(selected))
		before := make(map[pgtype.UUID]bool, len(selected))
		for _, state := range selected {
			desired[state.ID] = active
			before[state.ID] = state.Enabled
		}

		if replace {
			others, err := qtx.EnabledBookingCoursesBySubject(ctx, subjectID)
			if err != nil {
				return 0, nil, err
			}
			replaced, err := qtx.CourseBookingStates(ctx, others)
			if err != nil {
				return 0, nil, err
			}
			for _, state := range replaced {
				if _, selected := desired[state.ID]; !selected {
					desired[state.ID] = false
					before[state.ID] = state.Enabled
				}
			}
		}
		var enable, disable, sources []pgtype.UUID
		for id, after := range desired {
			sources = append(sources, id)
			if before[id] == after {
				continue
			}
			if after {
				enable = append(enable, id)
			} else {
				disable = append(disable, id)
			}
		}
		if _, err := qtx.CoursesLockOrdered(ctx, sources); err != nil {
			return 0, nil, err
		}
		if err := qtx.SetCourseBookingEnabled(ctx, enable, true); err != nil {
			return 0, nil, err
		}
		if err := qtx.SetCourseBookingEnabled(ctx, disable, false); err != nil {
			return 0, nil, err
		}
		changed := make([]bookingChange, 0, len(enable)+len(disable))
		for _, id := range enable {
			changed = append(changed, bookingChange{id.String(), false, true})
		}
		for _, id := range disable {
			changed = append(changed, bookingChange{id.String(), true, false})
		}
		slices.SortFunc(changed, func(a, b bookingChange) int { return strings.Compare(a.SourceID, b.SourceID) })
		if len(changed) > 0 {
			if _, err := qtx.AuditInsert(ctx, sqldb.AuditInsertParams{
				ActorUserID: pgtype.UUID{Bytes: user.ID, Valid: true},
				Action:      "course.student_booking_updated",
				Payload:     map[string]any{"scope": scope, "changes": changed},
			}); err != nil {
				return 0, nil, err
			}
		}
		affected, err = qtx.CourseBookingAffectedIDs(ctx, sources)
		if err != nil {
			return 0, nil, err
		}
		sourceIDs := make([]string, 0, len(sources))
		for _, id := range sources {
			sourceIDs = append(sourceIDs, id.String())
		}
		slices.Sort(sourceIDs)
		return http.StatusOK, map[string]any{
			"status": "ok", "updated": len(changed), "active": active,
			"source_ids": sourceIDs, "affected_course_ids": affected, "changes": changed,
		}, nil
	}, func() { s.publishCourseUpdates(affected) })
}
