package absenceshttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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

	mux.HandleFunc("POST /api/v1/absences", s.handleAbsenceCreate)
	mux.HandleFunc("GET /api/v1/absences", s.handleAbsenceInbox)
	mux.HandleFunc("GET /api/v1/courses/public", s.handleCoursesPublic)

	// Public endpoints for absence form
	mux.HandleFunc("GET /api/v1/absences/student-lookup", s.handleStudentLookup)
	mux.HandleFunc("GET /api/v1/absences/sit-in-options", s.handleSitInOptions)
	mux.HandleFunc("GET /api/v1/absence-form-config", s.handleFormConfigGet)

	// Admin endpoints for absence policies (registered here for convenience)
	mux.HandleFunc("GET /api/v1/admin/absence-policies", s.handlePoliciesGet)
	mux.HandleFunc("PUT /api/v1/admin/absence-policies", s.handlePoliciesUpdate)
	mux.HandleFunc("GET /api/v1/admin/absence-settings", s.handleAbsenceSettingsGet)
	mux.HandleFunc("PUT /api/v1/admin/absence-settings", s.handleAbsenceSettingsUpdate)

	// Staff-side operational absence workflow.
	mux.HandleFunc("GET /api/v1/absences/stats", s.handleAbsenceStats)
	mux.HandleFunc("GET /api/v1/absences/dashboard", s.handleAbsenceDashboard)
	mux.HandleFunc("GET /api/v1/absences/export", s.handleAbsenceExport)
	mux.HandleFunc("POST /api/v1/absences/batch-status", s.handleBatchStatus)
	mux.HandleFunc("GET /api/v1/absences/{id}", s.handleAbsenceGet)
	mux.HandleFunc("GET /api/v1/absences/{id}/timeline", s.handleAbsenceTimeline)
	mux.HandleFunc("GET /api/v1/absences/{id}/sit-in-candidates", s.handleSitInCandidates)
	mux.HandleFunc("PUT /api/v1/absences/{id}/status", s.handleAbsenceStatusUpdate)
	mux.HandleFunc("PUT /api/v1/absences/{id}/notes", s.handleAbsenceNotesUpdate)
	mux.HandleFunc("PUT /api/v1/absences/{id}/sit-in", s.handleSitInOverride)

	mux.HandleFunc("GET /api/v1/operations/calendar", s.handleCalendar)
	mux.HandleFunc("GET /api/v1/absences/calendar", s.handleCalendar)
}

func parseDate(s string) pgtype.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
}

func (s *server) handleAbsenceCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Wcode           string   `json:"wcode"`
		SubjectID       string   `json:"subject_id"`
		DateFrom        string   `json:"date_from"`
		DateTo          string   `json:"date_to"`
		ReasonCategory  *string  `json:"reason_category"`
		Reason          *string  `json:"reason"`
		SitInMethod     *string  `json:"sit_in_method"`
		SitInCourseID   *string  `json:"sit_in_course_id"`
		SitInSessionIDs []string `json:"sit_in_session_ids"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.Wcode == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "wcode is required")
		return
	}
	subjectID, err := s.a.ParseUUID(body.SubjectID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
		return
	}
	if body.DateFrom == "" || body.DateTo == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "date_from and date_to are required")
		return
	}
	dateFrom := parseDate(body.DateFrom)
	dateTo := parseDate(body.DateTo)
	if !dateFrom.Valid || !dateTo.Valid {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "Invalid date format, use YYYY-MM-DD")
		return
	}
	if dateTo.Time.Before(dateFrom.Time) {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "date_to must be on or after date_from")
		return
	}
	settings, err := s.readAbsenceSettings(r)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	days := int(dateTo.Time.Sub(dateFrom.Time).Hours() / 24)
	if days > settings.Form.MaxDateRangeDays {
		s.a.WriteErr(w, http.StatusBadRequest, "date_range_exceeded", fmt.Sprintf("Date range must be %d days or less", settings.Form.MaxDateRangeDays))
		return
	}
	var reason pgtype.Text
	if body.Reason != nil {
		value := strings.TrimSpace(*body.Reason)
		if value != "" {
			reason = pgtype.Text{String: value, Valid: true}
		}
	}
	var reasonCategory pgtype.Text
	if body.ReasonCategory != nil {
		value := strings.TrimSpace(*body.ReasonCategory)
		if value != "" {
			validCategory := false
			for _, category := range settings.Form.ReasonCategories {
				if category.Value == value {
					validCategory = true
					break
				}
			}
			if !validCategory {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_reason_category", "Select a configured reason category")
				return
			}
			reasonCategory = pgtype.Text{String: value, Valid: true}
		}
	}
	if settings.Form.RequireReason && !reasonCategory.Valid {
		s.a.WriteErr(w, http.StatusBadRequest, "reason_required", "Select a reason category")
		return
	}
	if !settings.Form.AllowFreeTextReason && reason.Valid {
		s.a.WriteErr(w, http.StatusBadRequest, "free_text_not_allowed", "Free-text reason is disabled")
		return
	}
	var sitInMethod pgtype.Text
	if body.SitInMethod != nil {
		if *body.SitInMethod != "zoom" && *body.SitInMethod != "physical" {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_method", "Invalid sit-in method")
			return
		}
		sitInMethod = pgtype.Text{String: *body.SitInMethod, Valid: true}
	}
	if len(body.SitInSessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
		s.a.WriteErr(w, http.StatusBadRequest, "too_many_sessions", "Selected sit-in sessions exceed the configured maximum")
		return
	}
	var sitInCourseID pgtype.UUID
	if sitInMethod.Valid && sitInMethod.String == "physical" {
		if body.SitInCourseID == nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_course_id", "Physical sit-in requires a course")
			return
		}
		sitInCourseID, err = s.a.ParseUUID(*body.SitInCourseID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_course_id", "Invalid sit-in course")
			return
		}
	}

	// Look up student and find their main course for this subject
	student, err := s.deps.Q.StudentGetByWCode(r.Context(), body.Wcode)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "student_not_found", "Student not found")
		return
	}

	enrolled, err := s.deps.Q.StudentEnrolledCoursesBySubjectV2(r.Context(), student.ID, subjectID)
	if err != nil || len(enrolled) == 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "not_enrolled", "Student not enrolled in any course for this subject")
		return
	}

	// Pick main course (highest level)
	main := enrolled[0]
	for _, c := range enrolled {
		if c.Level.Valid && main.Level.Valid && c.Level.Int16 > main.Level.Int16 {
			main = c
		}
	}

	tx, err := s.deps.DB.Begin(r.Context())
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := s.deps.Q.WithTx(tx)

	item, err := qtx.AbsenceCreate(r.Context(), sqldb.AbsenceCreateParams{
		Wcode:         body.Wcode,
		CourseID:      main.CourseID,
		DateFrom:      dateFrom,
		DateTo:        dateTo,
		Reason:        reason,
		SitInCourseID: sitInCourseID,
	})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	if err := qtx.AbsenceSetSubmissionMetadata(r.Context(), item.ID, subjectID, sitInMethod, student.FullName, reasonCategory, sitInCourseID); err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	// Insert sit-in sessions if provided
	if len(body.SitInSessionIDs) > 0 {
		var sessionUUIDs []pgtype.UUID
		for _, sid := range body.SitInSessionIDs {
			uid, err := s.a.ParseUUID(sid)
			if err != nil {
				continue
			}
			sessionUUIDs = append(sessionUUIDs, uid)
		}
		if len(sessionUUIDs) != len(body.SitInSessionIDs) {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_session_id", "Invalid sit-in session ID")
			return
		}
		if sitInMethod.String != "physical" {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sessions", "Only physical sit-ins may select sessions")
			return
		}
		count, err := qtx.ValidSitInSessionCount(r.Context(), item.ID, sitInCourseID, sessionUUIDs)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		if count != len(sessionUUIDs) {
			s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Sit-in sessions must be in the selected course and absence dates")
			return
		}
		if err := qtx.AbsenceSitInsCreate(r.Context(), item.ID, sessionUUIDs); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}
	}
	if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{
		AbsenceID: item.ID,
		Action:    "submitted",
		ActorRole: "student",
		Details:   map[string]any{"wcode": body.Wcode},
	}); err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write absence timeline")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}

	id, _ := s.a.UUIDString(item.ID)
	courseIDStr, _ := s.a.UUIDString(item.CourseID)
	s.a.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":            id,
		"wcode":         item.Wcode,
		"subject_id":    body.SubjectID,
		"course_id":     courseIDStr,
		"date_from":     item.DateFrom.Time.Format("2006-01-02"),
		"date_to":       item.DateTo.Time.Format("2006-01-02"),
		"sit_in_method": body.SitInMethod,
		"status":        "pending",
		"version":       1,
	})
}

func (s *server) handleAbsenceList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	items, err := s.deps.Q.AbsenceListExtended(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	type sitInDTO struct {
		ID        string `json:"id"`
		SessionID string `json:"session_id"`
	}
	type absenceDTO struct {
		ID              string     `json:"id"`
		Wcode           string     `json:"wcode"`
		CourseID        string     `json:"course_id"`
		SubjectID       *string    `json:"subject_id"`
		SubjectCode     *string    `json:"subject_code"`
		SubjectName     *string    `json:"subject_name"`
		DateFrom        string     `json:"date_from"`
		DateTo          string     `json:"date_to"`
		Reason          *string    `json:"reason"`
		SitInMethod     *string    `json:"sit_in_method"`
		SitInCourseID   *string    `json:"sit_in_course_id"`
		CreatedAt       string     `json:"created_at"`
		CourseCode      string     `json:"course_code"`
		CourseName      string     `json:"course_name"`
		SitInCourseCode *string    `json:"sit_in_course_code"`
		SitInCourseName *string    `json:"sit_in_course_name"`
		SitIns          []sitInDTO `json:"sit_ins,omitempty"`
	}
	out := make([]absenceDTO, 0, len(items))
	for _, it := range items {
		id, _ := s.a.UUIDString(it.ID)
		courseID, _ := s.a.UUIDString(it.CourseID)
		dto := absenceDTO{
			ID:         id,
			Wcode:      it.Wcode,
			CourseID:   courseID,
			DateFrom:   it.DateFrom.Time.Format("2006-01-02"),
			DateTo:     it.DateTo.Time.Format("2006-01-02"),
			CreatedAt:  it.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
			CourseCode: it.CourseCode,
			CourseName: it.CourseName,
		}
		if it.Reason.Valid {
			r := it.Reason.String
			dto.Reason = &r
		}
		if it.SitInMethod.Valid {
			m := it.SitInMethod.String
			dto.SitInMethod = &m
		}
		if it.SubjectID.Valid {
			sid, _ := s.a.UUIDString(it.SubjectID)
			dto.SubjectID = &sid
		}
		if it.SubjectCode.Valid {
			c := it.SubjectCode.String
			dto.SubjectCode = &c
		}
		if it.SubjectName.Valid {
			n := it.SubjectName.String
			dto.SubjectName = &n
		}
		if it.SitInCourseID.Valid {
			sid, _ := s.a.UUIDString(it.SitInCourseID)
			dto.SitInCourseID = &sid
		}
		if it.SitInCourseCode.Valid {
			c := it.SitInCourseCode.String
			dto.SitInCourseCode = &c
		}
		if it.SitInCourseName.Valid {
			n := it.SitInCourseName.String
			dto.SitInCourseName = &n
		}

		// Load sit-in sessions for this absence
		sitIns, _ := s.deps.Q.AbsenceSitInsByAbsence(r.Context(), it.ID)
		for _, si := range sitIns {
			siID, _ := s.a.UUIDString(si.ID)
			sessionID, _ := s.a.UUIDString(si.SessionID)
			dto.SitIns = append(dto.SitIns, sitInDTO{ID: siID, SessionID: sessionID})
		}

		out = append(out, dto)
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleCoursesPublic(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Q.CourseListActive(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type courseDTO struct {
		ID   string `json:"id"`
		Code string `json:"code"`
		Name string `json:"name"`
	}
	out := make([]courseDTO, 0, len(items))
	for _, it := range items {
		id, _ := s.a.UUIDString(it.ID)
		out = append(out, courseDTO{ID: id, Code: it.Code, Name: it.Name})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

// Public: lookup student by wcode and return their enrolled subjects
func (s *server) handleStudentLookup(w http.ResponseWriter, r *http.Request) {
	wcode := r.URL.Query().Get("wcode")
	if wcode == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "wcode parameter is required")
		return
	}

	rows, err := s.deps.Q.StudentSubjectByWCode(r.Context(), wcode)
	if err != nil {
		s.a.WriteErr(w, http.StatusNotFound, "student_not_found", "Student not found")
		return
	}
	if len(rows) == 0 {
		s.a.WriteErr(w, http.StatusNotFound, "no_subjects", "No enrolled subjects found for this student")
		return
	}

	type subjectDTO struct {
		ID   string `json:"id"`
		Code string `json:"code"`
		Name string `json:"name"`
	}

	studentID, _ := s.a.UUIDString(rows[0].StudentID)
	seen := map[string]bool{}
	subjects := make([]subjectDTO, 0)
	for _, r := range rows {
		sid, _ := s.a.UUIDString(r.SubjectID)
		if seen[sid] {
			continue
		}
		seen[sid] = true
		subjects = append(subjects, subjectDTO{ID: sid, Code: r.SubjectCode, Name: r.SubjectName})
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"student_id": studentID,
		"wcode":      rows[0].Wcode,
		"full_name":  rows[0].FullName,
		"subjects":   subjects,
	})
}

// Public: given student wcode + subject + dates, return sit-in options
func (s *server) handleSitInOptions(w http.ResponseWriter, r *http.Request) {
	wcode := r.URL.Query().Get("wcode")
	subjectIDStr := r.URL.Query().Get("subject_id")
	dateFromStr := r.URL.Query().Get("date_from")
	dateToStr := r.URL.Query().Get("date_to")

	if wcode == "" || subjectIDStr == "" || dateFromStr == "" || dateToStr == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_params", "wcode, subject_id, date_from, date_to are required")
		return
	}

	subjectID, err := s.a.ParseUUID(subjectIDStr)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
		return
	}

	dateFrom, err := time.Parse("2006-01-02", dateFromStr)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date_from", "Invalid date_from, use YYYY-MM-DD")
		return
	}
	dateTo, err := time.Parse("2006-01-02", dateToStr)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date_to", "Invalid date_to, use YYYY-MM-DD")
		return
	}

	result, err := resolveSitIn(r.Context(), s.deps.Q, wcode, subjectID, dateFrom, dateTo)
	if err != nil {
		s.deps.Log.Error("resolve sit-in failed", "error", err)
		s.a.WriteErr(w, http.StatusBadRequest, "resolve_error", "Could not resolve sit-in")
		return
	}
	if result == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "no_resolution", "No auto-resolution available for this student/subject combination")
		return
	}

	s.a.WriteJSON(w, http.StatusOK, result)
}

// Admin: get absence policies
func (s *server) handlePoliciesGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	settings, err := s.deps.Q.AppSettingsGetWithPolicies(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"absence_policies": json.RawMessage(settings.AbsencePolicies),
	})
}

// Admin: update absence policies (partial merge — single-group toggle only)
func (s *server) handlePoliciesUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		AbsencePolicies json.RawMessage `json:"absence_policies"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.AbsencePolicies == nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_policies", "absence_policies is required")
		return
	}
	adminID := actorID(user.ID)
	s.a.WithIdempotentTx(w, r, user.ID, "absence-policies", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		settings, err := qtx.AppSettingsGetWithPolicies(r.Context())
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		merged := deepMergeAbsencePolicies(settings.AbsencePolicies, body.AbsencePolicies)
		if err := qtx.AppSettingsUpdateAbsencePolicies(r.Context(), merged); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		if _, err := qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: adminID,
			Action:      "absence.policy_updated",
			Payload:     map[string]any{"absence_policies": json.RawMessage(body.AbsencePolicies)},
		}); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write audit log")
			return 0, nil, err
		}
		return http.StatusOK, map[string]string{"status": "ok"}, nil
	})
}

// deepMergeAbsencePolicies recursively merges src map values into dst.
// Both are expected to be JSON objects. Non-map values in src replace dst entirely.
func deepMergeAbsencePolicies(dst, src []byte) []byte {
	var dstMap, srcMap map[string]any
	if json.Unmarshal(dst, &dstMap) != nil || json.Unmarshal(src, &srcMap) != nil {
		return dst
	}
	deepMergeMap(dstMap, srcMap)
	merged, _ := json.Marshal(dstMap)
	return merged
}

func deepMergeMap(dst, src map[string]any) {
	for k, sv := range src {
		dv, exists := dst[k]
		if !exists {
			dst[k] = sv
			continue
		}
		sdst, sdstOK := dv.(map[string]any)
		ssrc, ssrcOK := sv.(map[string]any)
		if sdstOK && ssrcOK {
			deepMergeMap(sdst, ssrc)
		} else {
			dst[k] = sv
		}
	}
}
