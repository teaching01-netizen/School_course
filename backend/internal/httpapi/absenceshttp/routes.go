package absenceshttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/idempotency"
	"warwick-institute/internal/studentauth"
)

type server struct {
	deps                         httpdeps.Deps
	a                            httpadapter.Adapter
	batchNotificationLimiter     chan struct{}
	batchNotificationLimiterOnce sync.Once
}

type sessionRow struct {
	ID          string
	StartAt     string
	EndAt       string
	CourseID    string
	CourseCode  string
	CourseName  string
	SubjectID   string
	SubjectCode string
	SubjectName string
	TeacherName string
}

// sessionsInRangeSelectSQL lists the student's bookable sessions. When a
// subject has an active course configured (subject_active_courses) and the
// student is enrolled in it, only the active course and its same-cycle
// sibling courses are bookable — stale enrollments from earlier cycles are
// hidden. Students not enrolled in the active course keep seeing all their
// enrolled courses so the form never goes empty. Courses flagged
// absence_form_visible = false are hidden from the form entirely. Courses are
// hard-deleted (migration 00032), so no course soft-delete filter is needed.
func sessionsInRangeSelectSQL() string {
	return `
		SELECT sess.id, sess.start_at, sess.end_at,
		       c.id, c.code, c.name,
		       sub.id, sub.code, sub.name,
		       COALESCE(NULLIF(u.full_name, ''), u.username, '') AS teacher_name
		FROM sessions sess
		JOIN courses c ON c.id = sess.course_id
		JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN users u ON u.id = c.teacher_id
		JOIN course_students cs ON cs.course_id = c.id AND cs.status = 'enrolled'
		JOIN students st ON st.id = cs.student_id
		LEFT JOIN subject_active_courses sac ON sac.subject_id = sub.id
		LEFT JOIN courses ac ON ac.id = sac.course_id
		WHERE st.wcode = $1
		  AND sess.start_at >= $2
		  AND sess.start_at < $3
		  AND sess.deleted_at IS NULL
		  AND c.absence_form_visible
		  AND (
			sac.course_id IS NULL
			OR c.id = sac.course_id
			OR (ac.cycle_id IS NOT NULL AND c.cycle_id = ac.cycle_id)
			OR NOT EXISTS (
				SELECT 1 FROM course_students cs2
				WHERE cs2.course_id = sac.course_id
				  AND cs2.status = 'enrolled'
				  AND cs2.student_id = st.id
			)
		  )
		ORDER BY sub.code, sess.start_at
	`
}

// sessionsInRangeStaffSelectSQL is the staff-facing view of a student's
// sessions. The active-course restriction only governs which course students
// can book from the self-service form; staff reviewing a student's absence
// options must see every enrolled course, so no active-course predicate is
// applied. The result shape matches sessionsInRangeSelectSQL.
func sessionsInRangeStaffSelectSQL() string {
	return `
		SELECT sess.id, sess.start_at, sess.end_at,
		       c.id, c.code, c.name,
		       sub.id, sub.code, sub.name,
		       COALESCE(NULLIF(u.full_name, ''), u.username, '') AS teacher_name
		FROM sessions sess
		JOIN courses c ON c.id = sess.course_id
		JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN users u ON u.id = c.teacher_id
		JOIN course_students cs ON cs.course_id = c.id AND cs.status = 'enrolled'
		JOIN students st ON st.id = cs.student_id
		WHERE st.wcode = $1
		  AND sess.start_at >= $2
		  AND sess.start_at < $3
		  AND sess.deleted_at IS NULL
		ORDER BY sub.code, sess.start_at
	`
}

func sessionsInRangeAllSubjectsSelectSQL() string {
	return `
		SELECT sess.id, sess.start_at, sess.end_at,
		       c.id, c.code, c.name,
		       sub.id, sub.code, sub.name,
		       COALESCE(NULLIF(u.full_name, ''), u.username, '') AS teacher_name
		FROM sessions sess
		JOIN courses c ON c.id = sess.course_id
		JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN users u ON u.id = c.teacher_id
		WHERE sub.id::text = ANY(string_to_array($1, ','))
		  AND sess.start_at >= $2
		  AND sess.start_at < $3
		  AND sess.deleted_at IS NULL
		ORDER BY sub.code, c.code, sess.start_at
	`
}

func maxSessionsLookupRangeDays(settings absenceFormSettings) int {
	lookbackDays := 0
	if settings.MaxHoursAfterSession > 0 {
		lookbackDays = (settings.MaxHoursAfterSession + 23) / 24
	}
	return settings.MaxDateRangeDays + lookbackDays
}

func isAdminRequest(v httpadapter.SessionValidator, r *http.Request) bool {
	if v == nil {
		return false
	}
	user, err := v.RequireUser(r.Context(), r)
	return err == nil && user.Role == "Admin"
}

func parseSubjectIDFilter(adapter httpadapter.Adapter, raw string) ([]string, error) {
	var ids []string
	seen := map[string]bool{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		id, err := adapter.ParseUUID(value)
		if err != nil {
			return nil, err
		}
		subjectID, err := adapter.UUIDString(id)
		if err != nil {
			return nil, err
		}
		if seen[subjectID] {
			continue
		}
		seen[subjectID] = true
		ids = append(ids, subjectID)
	}
	return ids, nil
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/courses/public", s.handleCoursesPublic)

	mux.HandleFunc("GET /api/v1/absence-form-config", s.handleFormConfigGet)
	mux.HandleFunc("/api/v1/absences", s.handleAbsencesDispatch)
	mux.HandleFunc("/api/v1/absences/", s.handleAbsencesDispatch)

	mux.HandleFunc("POST /api/v1/absence-self-service/lookup", s.handleStudentLookup)
	mux.HandleFunc("GET /api/v1/admin/absences/student-lookup", s.handleStaffStudentLookup)
	mux.HandleFunc("GET /api/v1/absence-self-service/me", s.handleStudentProfile)
	mux.HandleFunc("GET /api/v1/absence-self-service/sessions", s.handleStudentSessions)
	mux.HandleFunc("GET /api/v1/absence-self-service/absences", s.handleStudentAbsenceHistory)
	mux.HandleFunc("POST /api/v1/absence-self-service/absences/{id}/cancel", s.handleStudentAbsenceCancel)
	mux.HandleFunc("GET /api/v1/absences/sessions-in-range", s.handleSessionsInRange)
	mux.HandleFunc("GET /api/v1/absences/sit-in-options", s.handleSitInOptions)

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
}

func parseDate(s string) pgtype.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
}

func instituteLocation(instituteTZ string) (*time.Location, error) {
	zone := strings.TrimSpace(instituteTZ)
	if zone == "" {
		zone = "Asia/Bangkok"
	}
	return time.LoadLocation(zone)
}

func parseInstituteLocalDate(s string, instituteTZ string) (time.Time, error) {
	loc, err := instituteLocation(instituteTZ)
	if err != nil {
		return time.Time{}, err
	}
	t, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func sessionDateKey(utcISO string, instituteTZ string) string {
	start, err := time.Parse(time.RFC3339Nano, utcISO)
	if err != nil {
		if len(utcISO) >= 10 {
			return utcISO[:10]
		}
		return utcISO
	}
	loc, err := instituteLocation(instituteTZ)
	if err != nil {
		return start.UTC().Format("2006-01-02")
	}
	return start.In(loc).Format("2006-01-02")
}

func managedAbsenceResponse(row sqldb.ManagedAbsenceRow) map[string]any {
	response := map[string]any{
		"status":      row.Status,
		"version":     row.Version,
		"created_at":  row.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
		"updated_at":  row.UpdatedAt.Time.UTC().Format(time.RFC3339Nano),
		"wcode":       row.Wcode,
		"course_code": row.CourseCode,
		"course_name": row.CourseName,
		"date_from":   row.DateFrom.Time.Format("2006-01-02"),
		"date_to":     row.DateTo.Time.Format("2006-01-02"),
	}
	if id, err := sUUIDString(row.ID); err == nil {
		response["id"] = id
	}
	if id, err := sUUIDString(row.CourseID); err == nil {
		response["course_id"] = id
	}
	if row.StudentName.Valid {
		response["student_name"] = row.StudentName.String
	}
	if row.StudentEmail.Valid {
		response["student_email"] = row.StudentEmail.String
	}
	if row.StudentNickname.Valid {
		response["student_nickname"] = row.StudentNickname.String
	}
	if row.StudentPhone.Valid {
		response["student_phone"] = row.StudentPhone.String
	}
	if row.SubjectID.Valid {
		if id, err := sUUIDString(row.SubjectID); err == nil {
			response["subject_id"] = id
		}
	}
	if row.SubjectCode.Valid {
		response["subject_code"] = row.SubjectCode.String
	}
	if row.SubjectName.Valid {
		response["subject_name"] = row.SubjectName.String
	}
	if row.ReasonCategory.Valid {
		response["reason_category"] = row.ReasonCategory.String
	}
	if row.Reason.Valid {
		response["reason"] = row.Reason.String
	}
	if row.SitInMethod.Valid {
		response["sit_in_method"] = row.SitInMethod.String
	}
	if row.SitInCourseID.Valid {
		if id, err := sUUIDString(row.SitInCourseID); err == nil {
			response["sit_in_course_id"] = id
		}
	}
	if row.SitInCourseCode.Valid {
		response["sit_in_course_code"] = row.SitInCourseCode.String
	}
	if row.SitInCourseName.Valid {
		response["sit_in_course_name"] = row.SitInCourseName.String
	}
	if row.SitInSubjectName.Valid {
		response["sit_in_subject_name"] = row.SitInSubjectName.String
	}
	if row.AdminNotes.Valid {
		response["admin_notes"] = row.AdminNotes.String
	}
	return response
}
func studentAbsenceResponse(row sqldb.ManagedAbsenceRow) map[string]any {
	response := map[string]any{
		"status":      row.Status,
		"version":     row.Version,
		"created_at":  row.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
		"updated_at":  row.UpdatedAt.Time.UTC().Format(time.RFC3339Nano),
		"course_code": row.CourseCode,
		"course_name": row.CourseName,
		"date_from":   row.DateFrom.Time.Format("2006-01-02"),
		"date_to":     row.DateTo.Time.Format("2006-01-02"),
	}
	if id, err := sUUIDString(row.ID); err == nil {
		response["id"] = id
	}
	if id, err := sUUIDString(row.CourseID); err == nil {
		response["course_id"] = id
	}
	if row.SubjectCode.Valid {
		response["subject_code"] = row.SubjectCode.String
	}
	if row.SubjectName.Valid {
		response["subject_name"] = row.SubjectName.String
	}
	if row.ReasonCategory.Valid {
		response["reason_category"] = row.ReasonCategory.String
	}
	if row.Reason.Valid {
		response["reason"] = row.Reason.String
	}
	if row.SitInMethod.Valid {
		response["sit_in_method"] = row.SitInMethod.String
	}
	if id, err := sUUIDString(row.SitInCourseID); err == nil {
		response["sit_in_course_id"] = id
	}
	if row.SitInCourseCode.Valid {
		response["sit_in_course_code"] = row.SitInCourseCode.String
	}
	if row.SitInCourseName.Valid {
		response["sit_in_course_name"] = row.SitInCourseName.String
	}
	if row.SitInSubjectName.Valid {
		response["sit_in_subject_name"] = row.SitInSubjectName.String
	}
	return response
}

func sUUIDString(u pgtype.UUID) (string, error) {
	if !u.Valid {
		return "", fmt.Errorf("invalid uuid")
	}
	id, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func (s *server) handleAbsenceCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requestOriginAllowed(w, r) {
		return
	}
	adminRequest := isAdminRequest(s.deps.Auth, r)
	studentSession := studentauth.Session{}
	actorID := idempotency.SystemActorUUID
	actorRole := "admin"
	if !adminRequest {
		var ok bool
		studentSession, ok = s.requireStudentSession(w, r)
		if !ok {
			return
		}
		actorID = studentIdempotencyActor(studentSession.Wcode)
		actorRole = "student"
	}
	var createdID string
	if !s.a.WithIdempotentTx(w, r, actorID, "absences-public", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)

		var body struct {
			Wcode             string         `json:"wcode"`
			Email             *string        `json:"email"`
			SubjectID         string         `json:"subject_id"`
			CourseID          string         `json:"course_id"`
			DateFrom          string         `json:"date_from"`
			DateTo            string         `json:"date_to"`
			ReasonCategory    *string        `json:"reason_category"`
			Reason            *string        `json:"reason"`
			SitInMethod       *string        `json:"sit_in_method"`
			SitInCourseID     *string        `json:"sit_in_course_id"`
			MissedSessionIDs  []string       `json:"missed_session_ids"`
			SitInSessionIDs   []string       `json:"sit_in_session_ids"`
			VerificationToken *string        `json:"verification_token"`
			SessionVersions   map[string]int `json:"session_versions"`
		}
		if err := s.a.DecodeJSON(w, r, &body); err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
			return 0, nil, err
		}
		if adminRequest {
			body.Wcode = normalizeWCode(body.Wcode)
			if body.Wcode == "" {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "wcode is required")
				return 0, nil, fmt.Errorf("wcode is required")
			}
		} else {
			if strings.TrimSpace(body.Wcode) != "" {
				s.a.WriteErr(w, http.StatusBadRequest, "identity_parameter_not_allowed", "wcode is derived from the verified student session")
				return 0, nil, fmt.Errorf("client supplied wcode")
			}
			if body.VerificationToken != nil && strings.TrimSpace(*body.VerificationToken) != "" {
				s.a.WriteErr(w, http.StatusBadRequest, "verification_token_not_allowed", "verification_token is no longer accepted")
				return 0, nil, fmt.Errorf("client supplied verification token")
			}
			body.Wcode = normalizeWCode(studentSession.Wcode)
		}
		if body.DateFrom == "" || body.DateTo == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "date_from and date_to are required")
			return 0, nil, fmt.Errorf("date required")
		}
		dateFrom := parseDate(body.DateFrom)
		dateTo := parseDate(body.DateTo)
		if !dateFrom.Valid || !dateTo.Valid {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "Invalid date format, use YYYY-MM-DD")
			return 0, nil, fmt.Errorf("bad date")
		}
		if dateTo.Time.Before(dateFrom.Time) {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "date_to must be on or after date_from")
			return 0, nil, fmt.Errorf("bad date")
		}

		settings, err := s.readAbsenceSettings(r)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		days := int(dateTo.Time.Sub(dateFrom.Time).Hours() / 24)
		if days > settings.Form.MaxDateRangeDays {
			s.a.WriteErr(w, http.StatusBadRequest, "date_range_exceeded", fmt.Sprintf("Date range must be %d days or less", settings.Form.MaxDateRangeDays))
			return 0, nil, fmt.Errorf("date range exceeded")
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
					return 0, nil, fmt.Errorf("bad reason category")
				}
				reasonCategory = pgtype.Text{String: value, Valid: true}
			}
		}
		if settings.Form.RequireReason && !reasonCategory.Valid {
			s.a.WriteErr(w, http.StatusBadRequest, "reason_required", "Select a reason category")
			return 0, nil, fmt.Errorf("reason required")
		}
		if !settings.Form.AllowFreeTextReason && reason.Valid {
			s.a.WriteErr(w, http.StatusBadRequest, "free_text_not_allowed", "Free-text reason is disabled")
			return 0, nil, fmt.Errorf("free text disabled")
		}

		sitInMethod, err := normalizeSubmissionSitInMethod(body.SitInMethod)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_method", "Invalid sit-in method")
			return 0, nil, err
		}
		if len(body.SitInSessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
			s.a.WriteErr(w, http.StatusBadRequest, "too_many_sessions", "Selected sit-in sessions exceed the configured maximum")
			return 0, nil, fmt.Errorf("too many sessions")
		}
		student, subjectID, course, err := s.resolveAbsenceSelection(r.Context(), qtx, tx, body.Wcode, &body.SubjectID, &body.CourseID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			if status == http.StatusInternalServerError {
				status = http.StatusBadRequest
				code = "bad_selection"
				msg = err.Error()
			}
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		// Hard gate: a course hidden from the absence form cannot be booked by
		// students even via a hand-crafted request. Staff submitting on a
		// student's behalf keep full access.
		if !adminRequest && !course.AbsenceFormVisible {
			s.a.WriteErr(w, http.StatusForbidden, "course_not_available", "This class is not available in the absence form")
			return 0, nil, fmt.Errorf("course %s is hidden from the absence form", course.CourseID)
		}
		missedUUIDs, err := parseUUIDStrings(body.MissedSessionIDs)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_missed_session_id", "Invalid missed session ID")
			return 0, nil, err
		}
		limitStats, candidateAbsenceDays, err := projectedAbsenceDayStats(
			r.Context(), qtx, body.Wcode, course.CourseID, missedUUIDs, dateFrom, dateTo, s.deps.InstituteTZ,
		)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error checking absence days")
			return 0, nil, err
		}
		if candidateAbsenceDays > int32(settings.SitIn.MaxSessionsPerAbsence) {
			s.a.WriteErr(w, http.StatusBadRequest, "too_many_missed_sessions", "Selected absence days exceed the configured maximum")
			return 0, nil, fmt.Errorf("too many absence days")
		}
		if limitStats.ProjectedLimitExceeded {
			s.a.WriteErr(w, http.StatusForbidden, "absence_limit_exceeded", "You have reached the maximum number of absence days allowed for this course")
			return 0, nil, fmt.Errorf("absence day limit exceeded for course %s", course.CourseID)
		}

		var sitInCourseID pgtype.UUID
		if body.SitInCourseID != nil && strings.TrimSpace(*body.SitInCourseID) != "" {
			sitInCourseID, err = s.a.ParseUUID(strings.TrimSpace(*body.SitInCourseID))
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_course_id", "Invalid sit-in course")
				return 0, nil, err
			}
		} else if sitInMethod.String == "physical" {
			sitInCourseID = course.CourseID
		}

		var studentPhone pgtype.Text
		var studentEmail pgtype.Text
		var studentEmailCRM pgtype.Text
		var studentEmailSystem pgtype.Text
		var studentNickname pgtype.Text
		studentEmailSourceLoaded := false
		var successSMSRecipients []string
		if contactRows, contactErr := qtx.StudentSubjectByWCode(r.Context(), body.Wcode); contactErr == nil && len(contactRows) > 0 {
			studentPhone = contactRows[0].StudentPhone
			studentEmail = contactRows[0].Email
			studentEmailCRM = contactRows[0].EmailCRM
			studentEmailSystem = contactRows[0].EmailSystem
			studentNickname = contactRows[0].Nickname
			studentEmailSourceLoaded = true
			successSMSRecipients = successSMSPhones(contactRows[0].ParentPhone, contactRows[0].StudentPhone)
		} else if contactErr != nil && s.deps.Log != nil {
			s.deps.Log.Error("failed to load absence contact phones", "wcode", body.Wcode, "error", contactErr)
		}

		if clientStudentEmailProvided(body.Email) && !studentEmailSourceLoaded {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_email", "Could not verify student email status")
			return 0, nil, fmt.Errorf("student email source unavailable")
		}
		// If the form provided an email and the student has no stored email,
		// use it for this absence and persist it as the system email.
		if resolvedEmail, shouldPersist, emailErr := resolveClientStudentEmail(body.Email, studentEmailCRM, studentEmailSystem); emailErr != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_email", "Enter a valid email address")
			return 0, nil, emailErr
		} else if shouldPersist {
			studentEmail = resolvedEmail
			if err := qtx.StudentSetSystemEmail(r.Context(), body.Wcode, resolvedEmail.String); err != nil && s.deps.Log != nil {
				s.deps.Log.Error("failed to persist system email", "wcode", body.Wcode, "error", err)
			}
		}

		item, err := qtx.AbsenceCreate(r.Context(), sqldb.AbsenceCreateParams{
			Wcode:         body.Wcode,
			CourseID:      course.CourseID,
			DateFrom:      dateFrom,
			DateTo:        dateTo,
			Reason:        reason,
			SitInCourseID: sitInCourseID,
		})
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		if err := setAbsenceMergeGroupForCourse(r.Context(), qtx, item.ID, course.CourseID); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		if err := qtx.AbsenceSetSubmissionMetadata(r.Context(), item.ID, subjectID, sitInMethod, student.FullName, studentEmail, studentNickname, studentPhone, reasonCategory, sitInCourseID); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}

		if len(body.SitInSessionIDs) > 0 {
			var sessionUUIDs []pgtype.UUID
			for _, sid := range body.SitInSessionIDs {
				uid, err := s.a.ParseUUID(sid)
				if err != nil {
					s.a.WriteErr(w, http.StatusBadRequest, "bad_session_id", "Invalid sit-in session ID")
					return 0, nil, err
				}
				sessionUUIDs = append(sessionUUIDs, uid)
			}
			if sitInMethod.String != "physical" {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_sessions", "Only physical sit-ins may select sessions")
				return 0, nil, fmt.Errorf("bad sessions")
			}
			excludeFinal, err := satVerbalCourseFinalClassExcluded(r.Context(), qtx, course.CourseID)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			count, err := qtx.ValidSitInSessionOverlap(r.Context(), item.ID, sessionUUIDs, s.deps.InstituteTZ, excludeFinal)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			if count != len(sessionUUIDs) {
				s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Sit-in sessions must be in the selected course and must not overlap the missed class")
				return 0, nil, fmt.Errorf("invalid sessions")
			}
			if err := s.validateSitInCandidates(r.Context(), qtx, item.ID, sessionUUIDs); err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", err.Error())
				return 0, nil, err
			}
			sitInInputs := make([]sqldb.SitInSnapshotInput, 0, len(sessionUUIDs))
			for _, sid := range sessionUUIDs {
				input := sqldb.SitInSnapshotInput{SessionID: sid}
				if body.SessionVersions != nil {
					if v, ok := body.SessionVersions[sid.String()]; ok {
						version := int32(v)
						input.ExpectedVersion = &version
					}
				}
				sitInInputs = append(sitInInputs, input)
			}
			if err := qtx.AbsenceSitInsCreateWithSnapshot(r.Context(), item.ID, sitInInputs, s.deps.InstituteTZ, BuildSnapshotFromSessionRow); err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
		}
		if len(body.MissedSessionIDs) > 0 {
			count, err := qtx.ValidMissedSessionCount(r.Context(), item.ID, missedUUIDs, s.deps.InstituteTZ)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			if count != len(missedUUIDs) {
				s.a.WriteErr(w, http.StatusBadRequest, "invalid_missed_sessions", "Missed sessions must be in the selected class and absence dates")
				return 0, nil, fmt.Errorf("invalid missed sessions")
			}
			timingRows, err := qtx.ValidMissedSessionTiming(r.Context(), item.ID, missedUUIDs, s.deps.InstituteTZ)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			if timingErr := validateSessionTiming(settings.Form, time.Now(), sessionTimingInfos(timingRows)); timingErr != nil {
				s.a.WriteErr(w, http.StatusBadRequest, timingErr.code, timingErr.message)
				return 0, nil, timingErr
			}
			snapshotInputs := make([]sqldb.MissedSessionSnapshotInput, 0, len(missedUUIDs))
			for _, sid := range missedUUIDs {
				input := sqldb.MissedSessionSnapshotInput{SessionID: sid}
				if body.SessionVersions != nil {
					if v, ok := body.SessionVersions[sid.String()]; ok {
						version := int32(v)
						input.ExpectedVersion = &version
					}
				}
				snapshotInputs = append(snapshotInputs, input)
			}
			if _, err := qtx.AbsenceMissedSessionsCreateWithSnapshot(r.Context(), item.ID, snapshotInputs, s.deps.InstituteTZ, BuildSnapshotFromSessionRow); err != nil {
				var versionErr *sqldb.SessionVersionConflictError
				if errors.As(err, &versionErr) {
					s.a.WriteErr(w, http.StatusConflict, "session_version_conflict", "Session has been modified since you last loaded it. Please reload and try again.")
					return 0, nil, err
				}
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
		}

		if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{
			AbsenceID: item.ID,
			Action:    "submitted",
			ActorRole: actorRole,
			Details:   map[string]any{"wcode": body.Wcode},
		}); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write absence timeline")
			return 0, nil, err
		}

		managed, err := qtx.ManagedAbsenceGet(r.Context(), item.ID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}

		// Send success SMS after submission (non-critical; errors are logged only).
		if settings.Notifications.SmsSuccessTemplate != "" {
			if len(successSMSRecipients) > 0 {
				sessions, sesErr := qtx.ManagedAbsenceSessions(r.Context(), item.ID)
				if sesErr == nil {
					missed, missedErr := qtx.ManagedAbsenceMissedSessions(r.Context(), item.ID)
					if missedErr == nil {
						sendSuccessSMS(r.Context(), s.deps.SMS, s.deps.Log, settings.Notifications.SmsSuccessTemplate, managed, sessions, missed, successSMSRecipients, s.deps.InstituteTZ)
					} else {
						if s.deps.Log != nil {
							s.deps.Log.Error("failed to load missed sessions for sms", "absence_id", item.ID, "error", missedErr)
						}
						sendSuccessSMS(r.Context(), s.deps.SMS, s.deps.Log, settings.Notifications.SmsSuccessTemplate, managed, sessions, nil, successSMSRecipients, s.deps.InstituteTZ)
					}
				} else if s.deps.Log != nil {
					s.deps.Log.Error("failed to load absence sessions for sms", "absence_id", item.ID, "error", sesErr)
				}
			}
		}

		// Send success email after submission (non-critical; errors are logged only).
		if s.deps.EmailService != nil {
			emailCfg := settings.emailSuccessConfig()
			if emailCfg.Enabled {
				sessions, sesErr := qtx.ManagedAbsenceSessions(r.Context(), item.ID)
				if sesErr == nil {
					missed, missedErr := qtx.ManagedAbsenceMissedSessions(r.Context(), item.ID)
					if missedErr == nil {
						sendSuccessEmailWithConfig(r.Context(), s.deps.EmailService, s.deps.Log, managed, sessions, missed, emailCfg, s.deps.InstituteName, s.deps.InstituteTZ)
					} else {
						sendSuccessEmailWithConfig(r.Context(), s.deps.EmailService, s.deps.Log, managed, sessions, nil, emailCfg, s.deps.InstituteName, s.deps.InstituteTZ)
					}
				} else if s.deps.Log != nil {
					s.deps.Log.Error("failed to load absence sessions for email", "absence_id", item.ID, "error", sesErr)
				}
			}
		}

		// Burn the verification: one OTP + session per submission.
		if !adminRequest && item.ID.Valid {
			firstAbsenceID, idErr := uuid.FromBytes(item.ID.Bytes[:])
			if idErr != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return 0, nil, idErr
			}
			if err := s.consumeAndRevokeStudentVerificationTx(r.Context(), tx, studentSession, firstAbsenceID); err != nil {
				if s.deps.Log != nil {
					s.deps.Log.Error("failed to burn student verification on submit", "wcode", body.Wcode, "error", err)
				}
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return 0, nil, err
			}
		}

		resp := managedAbsenceResponse(managed)
		resp["status"] = "pending"
		if id, ok := resp["id"].(string); ok {
			createdID = id
		}
		return http.StatusCreated, resp, nil
	}) {
		return
	}
	s.publishAbsenceChanged(createdID)
}

func (s *server) handleCoursesPublic(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Q.CourseListActive(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	type courseDTO struct {
		ID          string `json:"id"`
		Code        string `json:"code"`
		Name        string `json:"name"`
		SubjectName string `json:"subject_name"`
	}
	out := make([]courseDTO, 0, len(items))
	for _, it := range items {
		id, _ := s.a.UUIDString(it.ID)
		out = append(out, courseDTO{ID: id, Code: it.Code, Name: it.Name, SubjectName: it.SubjectName})
	}
	s.a.WriteJSON(w, http.StatusOK, out)
}

// Public lookup returns only the values needed to start self-service. It must
// never become a student directory: profile data is available only after OTP
// verification has created a student self-service session. The nickname hint
// is the one pre-verification identity cue and is always masked server-side.
func (s *server) handleStudentLookup(w http.ResponseWriter, r *http.Request) {
	if _, hasNickname := r.URL.Query()["nickname"]; hasNickname {
		s.a.WriteErr(w, http.StatusBadRequest, "nickname_not_supported", "nickname lookup is not supported; use wcode")
		return
	}
	var body struct {
		Wcode string `json:"wcode"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	wcode := normalizeWCode(body.Wcode)
	if len(wcode) < 2 || !strings.HasPrefix(wcode, "w") {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "a valid wcode is required")
		return
	}
	// W-Code lookup is intentionally low-friction, but it must not become an
	// unlimited student-existence oracle or a database write amplifier.
	for _, limit := range []struct {
		key   string
		count int
	}{
		{key: "student-lookup:ip:" + s.requestIP(r), count: 50},
		{key: "student-lookup:wcode:" + wcode, count: 20},
	} {
		if retryAfter, err := s.allowPublicRateLimit(r.Context(), limit.key, limit.count, time.Hour); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		} else if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			s.a.WriteErr(w, http.StatusTooManyRequests, "rate_limited", "Too many student lookup requests")
			return
		}
	}

	service := s.studentAuthService()
	if service == nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	// Minting a lookup token is a side effect, so the endpoint follows the
	// idempotency policy: the response is persisted and replayed on retries.
	if !s.a.WithIdempotentTx(w, r, idempotency.SystemActorUUID, "student-lookup", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		result, err := service.LookupTx(r.Context(), tx, wcode)
		if err != nil {
			if errors.Is(err, studentauth.ErrStudentNotFound) {
				s.a.WriteErr(w, http.StatusNotFound, "student_not_found", "Student ID was not found")
				return 0, nil, err
			}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		response := map[string]any{
			"wcode":                         result.Wcode,
			"lookup_token":                  result.LookupToken,
			"email_input_required":          result.EmailInputRequired,
			"parent_verification_available": result.ParentVerificationAvailable,
		}
		// Masked identity cues so the submitter can confirm the W-Code hit
		// the right student before verification; raw values stay behind the
		// OTP.
		if hint := maskNicknameForPublic(result.DisplayName); hint != "" {
			response["nickname_hint"] = hint
		}
		if phoneHint := maskPhoneForPublic(result.ParentPhone); phoneHint != "" {
			response["parent_phone_hint"] = phoneHint
		}
		return http.StatusOK, response, nil
	}) {
		return
	}
}

// handleStaffStudentLookup is the staff-only counterpart to the minimal
// self-service lookup. W-Code is an identifier here, but the authenticated
// admin session is the authorization boundary for returning student details.
func (s *server) handleStaffStudentLookup(w http.ResponseWriter, r *http.Request) {
	if !isAdminRequest(s.deps.Auth, r) {
		s.a.WriteErr(w, http.StatusUnauthorized, "unauthorized", "Staff authorization is required")
		return
	}
	wcode := normalizeWCode(r.URL.Query().Get("wcode"))
	if len(wcode) < 2 || !strings.HasPrefix(wcode, "w") {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "a valid wcode is required")
		return
	}

	rows, err := s.deps.Q.StudentSubjectByWCode(r.Context(), wcode)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if len(rows) == 0 {
		student, studentErr := s.deps.Q.StudentGetByWCode(r.Context(), wcode)
		if errors.Is(studentErr, pgx.ErrNoRows) {
			s.a.WriteErr(w, http.StatusNotFound, "student_not_found", "Student ID was not found")
			return
		}
		if studentErr != nil {
			status, code, msg := s.a.ClassifyDBErr(studentErr)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		studentID, idErr := s.a.UUIDString(student.ID)
		if idErr != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading student")
			return
		}
		s.a.WriteJSON(w, http.StatusOK, map[string]any{
			"student_id":   studentID,
			"wcode":        student.Wcode,
			"full_name":    student.FullName,
			"display_name": student.FullName,
			"subjects":     []any{},
		})
		return
	}

	textValue := func(value pgtype.Text) *string {
		if !value.Valid || strings.TrimSpace(value.String) == "" {
			return nil
		}
		result := value.String
		return &result
	}
	studentID, idErr := s.a.UUIDString(rows[0].StudentID)
	if idErr != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading student")
		return
	}
	displayName := rows[0].FullName
	if nickname := textValue(rows[0].Nickname); nickname != nil {
		displayName = *nickname
	}
	subjects := make([]map[string]any, 0, len(rows))
	seenSubjects := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		subjectID, subjectErr := s.a.UUIDString(row.SubjectID)
		if subjectErr != nil {
			continue
		}
		if _, seen := seenSubjects[subjectID]; seen {
			continue
		}
		seenSubjects[subjectID] = struct{}{}
		activeCourseID, courseErr := s.a.UUIDString(row.ActiveCourseID)
		subject := map[string]any{
			"id":   subjectID,
			"code": row.SubjectCode,
			"name": row.SubjectName,
		}
		if courseErr == nil {
			subject["active_course_id"] = activeCourseID
		}
		if row.ActiveTeacherName != "" {
			subject["teacher_name"] = row.ActiveTeacherName
		}
		subjects = append(subjects, subject)
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"student_id":   studentID,
		"wcode":        rows[0].Wcode,
		"full_name":    rows[0].FullName,
		"display_name": displayName,
		"nickname":     textValue(rows[0].Nickname),
		"school":       textValue(rows[0].School),
		"email":        textValue(rows[0].Email),
		"email_crm":    textValue(rows[0].EmailCRM),
		"email_system": textValue(rows[0].EmailSystem),
		"parent_phone": textValue(rows[0].ParentPhone),
		"subjects":     subjects,
	})
}

// Public: given student wcode + subject + optional dates, return sit-in options
func (s *server) handleSitInOptions(w http.ResponseWriter, r *http.Request) {
	wcode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("wcode")))
	subjectIDStr := r.URL.Query().Get("subject_id")
	dateFromStr := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateToStr := strings.TrimSpace(r.URL.Query().Get("date_to"))

	if wcode == "" || subjectIDStr == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_params", "wcode and subject_id are required")
		return
	}

	subjectID, err := s.a.ParseUUID(subjectIDStr)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
		return
	}

	var dateFrom, dateTo time.Time
	if dateFromStr != "" && dateToStr != "" {
		dateFrom, err = parseInstituteLocalDate(dateFromStr, s.deps.InstituteTZ)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date_from", "Invalid date_from, use YYYY-MM-DD")
			return
		}
		dateTo, err = parseInstituteLocalDate(dateToStr, s.deps.InstituteTZ)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date_to", "Invalid date_to, use YYYY-MM-DD")
			return
		}
	} else {
		now := time.Now()
		dateFrom = now.AddDate(0, 0, -30)
		dateTo = now.AddDate(0, 0, 90)
	}

	if !isAdminRequest(s.deps.Auth, r) {
		s.a.WriteErr(w, http.StatusUnauthorized, "unauthorized", "Staff authorization is required")
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

// handleSessionsInRange serves the staff compatibility endpoint. A student
// W-Code is not an authorization credential, so valid requests require an
// authenticated admin session.
func (s *server) handleSessionsInRange(w http.ResponseWriter, r *http.Request) {
	s.handleSessionsInRangeForWCode(w, r, "", true)
}

func (s *server) handleSessionsInRangeForWCode(w http.ResponseWriter, r *http.Request, forcedWCode string, requireAdmin bool) {
	wcode := normalizeWCode(forcedWCode)
	if wcode == "" {
		wcode = normalizeWCode(r.URL.Query().Get("wcode"))
	}
	dateFromStr := r.URL.Query().Get("date_from")
	dateToStr := r.URL.Query().Get("date_to")

	if wcode == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_params", "wcode is required")
		return
	}

	dateFromProvided := dateFromStr != ""
	dateToProvided := dateToStr != ""
	if dateFromProvided != dateToProvided {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date_range", "date_from and date_to must be provided together")
		return
	}
	dateRangeProvided := dateFromProvided && dateToProvided

	var dateFrom, dateTo time.Time
	var err error
	if dateRangeProvided {
		dateFrom, err = parseInstituteLocalDate(dateFromStr, s.deps.InstituteTZ)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date_from", "Invalid date_from, use YYYY-MM-DD")
			return
		}
		dateTo, err = parseInstituteLocalDate(dateToStr, s.deps.InstituteTZ)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date_to", "Invalid date_to, use YYYY-MM-DD")
			return
		}
		if dateTo.Before(dateFrom) {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date_range", "date_to must be on or after date_from")
			return
		}
	} else {
		now := time.Now()
		dateFrom = now.AddDate(0, 0, -30)
		dateTo = now.AddDate(0, 0, 90)
	}

	adminRequest := isAdminRequest(s.deps.Auth, r)
	if requireAdmin && !adminRequest {
		s.a.WriteErr(w, http.StatusUnauthorized, "unauthorized", "Staff authorization is required")
		return
	}

	settings, err := s.readAbsenceSettings(r)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if dateRangeProvided && !adminRequest {
		days := int(dateTo.Sub(dateFrom).Hours() / 24)
		maxLookupRangeDays := maxSessionsLookupRangeDays(settings.Form)
		if days > maxLookupRangeDays {
			s.a.WriteErr(w, http.StatusBadRequest, "date_range_exceeded",
				fmt.Sprintf("Date range must be %d days or less", maxLookupRangeDays))
			return
		}
	}

	allowedCourseIDs := map[string]bool{}
	if raw := strings.TrimSpace(r.URL.Query().Get("course_ids")); raw != "" {
		for _, value := range strings.Split(raw, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			id, err := s.a.ParseUUID(value)
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_course_ids", "Invalid course_ids filter")
				return
			}
			courseID, _ := s.a.UUIDString(id)
			allowedCourseIDs[courseID] = true
		}
	}
	satVerbalAfterPriority := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("sat_verbal_after_priority")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sat_verbal_after_priority", "sat_verbal_after_priority must be a non-negative integer")
			return
		}
		satVerbalAfterPriority = value
	}
	bypassTiming := strings.TrimSpace(r.URL.Query().Get("bypass_timing")) == "true"
	includeAllSubjects := strings.TrimSpace(r.URL.Query().Get("include_all_subjects")) == "true"
	subjectIDFilter, err := parseSubjectIDFilter(s.a, strings.TrimSpace(r.URL.Query().Get("subject_ids")))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_ids", "Invalid subject_ids filter")
		return
	}
	if includeAllSubjects {
		if !adminRequest {
			s.a.WriteErr(w, http.StatusForbidden, "admin_required", "Only staff can load all subject sessions")
			return
		}
		if len(subjectIDFilter) == 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_ids", "subject_ids are required when loading all subject sessions")
			return
		}
	}

	type sessionDBRow struct {
		ID          pgtype.UUID
		StartAt     pgtype.Timestamptz
		EndAt       pgtype.Timestamptz
		CourseID    pgtype.UUID
		CourseCode  string
		CourseName  string
		SubjectID   pgtype.UUID
		SubjectCode string
		SubjectName string
		TeacherName string
	}

	var rows pgx.Rows
	if includeAllSubjects {
		rows, err = s.deps.DB.Query(r.Context(), sessionsInRangeAllSubjectsSelectSQL(), strings.Join(subjectIDFilter, ","), dateFrom, dateTo.AddDate(0, 0, 1))
	} else if adminRequest {
		rows, err = s.deps.DB.Query(r.Context(), sessionsInRangeStaffSelectSQL(), wcode, dateFrom, dateTo.AddDate(0, 0, 1))
	} else {
		rows, err = s.deps.DB.Query(r.Context(), sessionsInRangeSelectSQL(), wcode, dateFrom, dateTo.AddDate(0, 0, 1))
	}
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	defer rows.Close()

	now := time.Now()
	var sessions []sessionRow
	for rows.Next() {
		var dbRow sessionDBRow
		if err := rows.Scan(&dbRow.ID, &dbRow.StartAt, &dbRow.EndAt,
			&dbRow.CourseID, &dbRow.CourseCode, &dbRow.CourseName,
			&dbRow.SubjectID, &dbRow.SubjectCode, &dbRow.SubjectName, &dbRow.TeacherName); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading sessions")
			return
		}
		sessionID, err := sUUIDString(dbRow.ID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading sessions")
			return
		}
		courseID, err := sUUIDString(dbRow.CourseID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading sessions")
			return
		}
		subjectID, err := sUUIDString(dbRow.SubjectID)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading sessions")
			return
		}
		if !dbRow.StartAt.Valid || !dbRow.EndAt.Valid {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading sessions")
			return
		}
		if !bypassTiming && !sessionAllowedByTimingPolicy(settings.Form, now, sessionTimingInfo{StartAt: dbRow.StartAt, EndAt: dbRow.EndAt}) {
			continue
		}
		row := sessionRow{
			ID:          sessionID,
			StartAt:     dbRow.StartAt.Time.UTC().Format(time.RFC3339Nano),
			EndAt:       dbRow.EndAt.Time.UTC().Format(time.RFC3339Nano),
			CourseID:    courseID,
			CourseCode:  dbRow.CourseCode,
			CourseName:  dbRow.CourseName,
			SubjectID:   subjectID,
			SubjectCode: dbRow.SubjectCode,
			SubjectName: dbRow.SubjectName,
			TeacherName: dbRow.TeacherName,
		}
		if len(allowedCourseIDs) > 0 && !allowedCourseIDs[row.CourseID] {
			continue
		}
		sessions = append(sessions, row)
	}
	if err := rows.Err(); err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading sessions")
		return
	}

	// Query session IDs already covered by existing absences
	absentRows, err := s.deps.DB.Query(r.Context(), `
		SELECT DISTINCT sess.id
		FROM sessions sess
		JOIN student_absences sa ON (
			sa.course_id = sess.course_id
			OR EXISTS (
				SELECT 1
				FROM course_merge_group_members merge_member
				WHERE merge_member.group_id = sa.merge_group_id
				  AND merge_member.course_id = sess.course_id
			)
		)
		WHERE sa.wcode = $1
		  AND sa.status <> 'cancelled'
		  AND (sess.start_at AT TIME ZONE $2)::date BETWEEN sa.date_from AND sa.date_to
		  AND sess.deleted_at IS NULL
	`, wcode, s.deps.InstituteTZ)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	defer absentRows.Close()

	absentSet := map[string]bool{}
	for absentRows.Next() {
		var id pgtype.UUID
		if err := absentRows.Scan(&id); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading absence data")
			return
		}
		idStr, err := sUUIDString(id)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading absence data")
			return
		}
		if len(allowedCourseIDs) > 0 {
			// Course filter is applied after the query so the SQL shape stays stable
			// when only a subset is selected.
		}
		absentSet[idStr] = true
	}
	if err := absentRows.Err(); err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading absence data")
		return
	}

	// Group by course, preserving order
	type courseGroup struct {
		CourseID    string
		CourseCode  string
		CourseName  string
		SubjectID   string
		SubjectCode string
		SubjectName string
		TeacherName string
		Sessions    []sessionRow
	}

	grouped := map[string]*courseGroup{}
	var courseOrder []string
	for _, sess := range sessions {
		key := sess.CourseID
		if grouped[key] == nil {
			grouped[key] = &courseGroup{
				CourseID:    sess.CourseID,
				CourseCode:  sess.CourseCode,
				CourseName:  sess.CourseName,
				SubjectID:   sess.SubjectID,
				SubjectCode: sess.SubjectCode,
				SubjectName: sess.SubjectName,
				TeacherName: sess.TeacherName,
			}
			courseOrder = append(courseOrder, key)
		}
		grouped[key].Sessions = append(grouped[key].Sessions, sess)
	}

	// Build JSON response
	type sessionResponse struct {
		ID            string `json:"id"`
		StartAt       string `json:"start_at"`
		EndAt         string `json:"end_at"`
		Date          string `json:"date"`
		AlreadyAbsent bool   `json:"already_absent"`
	}
	type courseSitInResponse struct {
		RuleName             string                        `json:"rule_name,omitempty"`
		RuleType             string                        `json:"rule_type,omitempty"`
		SitInMethod          string                        `json:"sit_in_method"`
		Priorities           []SitInPriorityResult         `json:"priorities,omitempty"`
		CurrentPriorityLevel int                           `json:"current_priority_level,omitempty"`
		HasNextPriority      bool                          `json:"has_next_priority,omitempty"`
		SitInCourse          *SitInCourseInfo              `json:"sit_in_course,omitempty"`
		AvailableSessions    []sessionBrief                `json:"available_sessions,omitempty"`
		MissedSessions       []sessionBrief                `json:"missed_sessions,omitempty"`
		SitInByMissedSession map[string]SitInSessionResult `json:"sit_in_by_missed_session,omitempty"`
	}
	type courseResponse struct {
		SubjectID            string               `json:"subject_id"`
		SubjectCode          string               `json:"subject_code"`
		SubjectName          string               `json:"subject_name"`
		TeacherName          string               `json:"teacher_name,omitempty"`
		CourseID             string               `json:"course_id"`
		CourseCode           string               `json:"course_code"`
		CourseName           string               `json:"course_name"`
		MergeGroupID         string               `json:"merge_group_id,omitempty"`
		MergeGroupName       string               `json:"merge_group_name,omitempty"`
		Sessions             []sessionResponse    `json:"sessions"`
		SitIn                *courseSitInResponse `json:"sit_in,omitempty"`
		TotalCourseDays      int32                `json:"total_course_days"`
		UsedAbsenceDays      int32                `json:"used_absence_days"`
		MaximumAbsenceDays   int32                `json:"maximum_absence_days"`
		RemainingAbsenceDays int32                `json:"remaining_absence_days"`
		AbsenceLimitReached  bool                 `json:"absence_limit_reached"`
	}

	staffSubjectAvailable := map[string][]sessionBrief{}
	if includeAllSubjects {
		for _, sess := range sessions {
			staffSubjectAvailable[sess.SubjectID] = append(staffSubjectAvailable[sess.SubjectID], sessionBrief{
				ID:          sess.ID,
				StartAt:     sess.StartAt,
				EndAt:       sess.EndAt,
				CourseID:    sess.CourseID,
				ClassName:   sess.CourseName,
				CourseName:  sess.CourseName,
				CourseCode:  sess.CourseCode,
				SubjectCode: sess.SubjectCode,
				SubjectName: sess.SubjectName,
				TeacherName: sess.TeacherName,
			})
		}
	}

	courses := make([]courseResponse, 0, len(courseOrder))
	for _, key := range courseOrder {
		g := grouped[key]
		sessionsResp := make([]sessionResponse, 0, len(g.Sessions))
		for _, sess := range g.Sessions {
			sessionsResp = append(sessionsResp, sessionResponse{
				ID:            sess.ID,
				StartAt:       sess.StartAt,
				EndAt:         sess.EndAt,
				Date:          sessionDateKey(sess.StartAt, s.deps.InstituteTZ),
				AlreadyAbsent: absentSet[sess.ID],
			})
		}

		var sitIn *courseSitInResponse
		courseID, cErr := s.a.ParseUUID(g.CourseID)
		if includeAllSubjects {
			sitIn = &courseSitInResponse{
				SitInMethod:       SitInMethodPhysical,
				AvailableSessions: staffSubjectAvailable[g.SubjectID],
			}
		} else if cErr == nil {
			// Resolve sit-in using the student's enrolled course ID for this block.
			subjectID, sErr := s.a.ParseUUID(g.SubjectID)
			if sErr == nil {
				resolveFrom, resolveTo := resolveDateRangeForSessionStartsInZone(sessionStartAtValues(g.Sessions), dateFrom, dateTo, s.deps.InstituteTZ)
				result, resolveErr := resolveSitInForCourse(r.Context(), s.deps.Q, wcode, courseID, subjectID, resolveFrom, resolveTo, s.deps.InstituteTZ, satVerbalAfterPriority)
				if resolveErr != nil {
					s.deps.Log.Error("sit-in resolution failed", "course_id", g.CourseID, "error", resolveErr)
				} else if result != nil && result.SitInMethod != SitInMethodNone {
					sitIn = &courseSitInResponse{
						RuleName:             result.RuleName,
						RuleType:             result.RuleType,
						SitInMethod:          result.SitInMethod,
						SitInCourse:          result.SitInCourse,
						Priorities:           result.Priorities,
						CurrentPriorityLevel: result.CurrentPriorityLevel,
						HasNextPriority:      result.HasNextPriority,
						SitInByMissedSession: result.SitInByMissedSession,
					}
					if len(result.Available) > 0 {
						sitIn.AvailableSessions = result.Available
					}
					if len(result.MissedSession) > 0 {
						sitIn.MissedSessions = result.MissedSession
					}
				}
			}
		}

		if cErr != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading course absence days")
			return
		}
		mergeScope, hasMergeScope, scopeErr := mergeGroupScopeForCourse(r.Context(), s.deps.Q, courseID)
		if scopeErr != nil {
			if s.deps.Log != nil {
				s.deps.Log.Error("failed to resolve course absence scope", "course_id", g.CourseID, "error", scopeErr)
			}
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error checking absence days")
			return
		}
		var dayCounts sqldb.AbsenceDayCounts
		var dayCountErr error
		if hasMergeScope {
			dayCounts, dayCountErr = s.deps.Q.AbsenceDayCountsForMergeGroup(r.Context(), sqldb.AbsenceDayCountsForMergeGroupParams{
				Wcode:        wcode,
				MergeGroupID: mergeScope.ID,
				InstituteTZ:  s.deps.InstituteTZ,
			})
		} else {
			dayCounts, dayCountErr = s.deps.Q.AbsenceDayCountsForCourse(r.Context(), sqldb.AbsenceDayCountsForCourseParams{
				Wcode:       wcode,
				CourseID:    courseID,
				InstituteTZ: s.deps.InstituteTZ,
			})
		}
		if dayCountErr != nil {
			if s.deps.Log != nil {
				s.deps.Log.Error("failed to calculate course absence days", "course_id", g.CourseID, "error", dayCountErr)
			}
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error checking absence days")
			return
		}
		limitStats := absences.NewAbsenceDayLimitStats(dayCounts.TotalCourseDays, dayCounts.UsedAbsenceDays, dayCounts.UsedAbsenceDays)
		mergeGroupID := ""
		if hasMergeScope {
			mergeGroupID, dayCountErr = sUUIDString(mergeScope.ID)
			if dayCountErr != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading course absence scope")
				return
			}
		}

		courses = append(courses, courseResponse{
			SubjectID:            g.SubjectID,
			SubjectCode:          g.SubjectCode,
			SubjectName:          g.SubjectName,
			TeacherName:          g.TeacherName,
			CourseID:             g.CourseID,
			CourseCode:           g.CourseCode,
			CourseName:           g.CourseName,
			MergeGroupID:         mergeGroupID,
			MergeGroupName:       mergeScope.Name,
			Sessions:             sessionsResp,
			SitIn:                sitIn,
			TotalCourseDays:      limitStats.TotalCourseDays,
			UsedAbsenceDays:      limitStats.UsedAbsenceDays,
			MaximumAbsenceDays:   limitStats.MaximumAbsenceDays,
			RemainingAbsenceDays: limitStats.RemainingAbsenceDays,
			AbsenceLimitReached:  limitStats.LimitReached,
		})
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{"subjects": courses})
}

func sessionStartAtValues(sessions []sessionRow) []string {
	starts := make([]string, 0, len(sessions))
	for _, session := range sessions {
		starts = append(starts, session.StartAt)
	}
	return starts
}

func resolveDateRangeForSessionStarts(starts []string, fallbackFrom time.Time, fallbackTo time.Time) (time.Time, time.Time) {
	return resolveDateRangeForSessionStartsInZone(starts, fallbackFrom, fallbackTo, "Asia/Bangkok")
}

func resolveDateRangeForSessionStartsInZone(starts []string, fallbackFrom time.Time, fallbackTo time.Time, instituteTZ string) (time.Time, time.Time) {
	loc, err := instituteLocation(instituteTZ)
	if err != nil {
		loc = time.UTC
	}
	var from time.Time
	var to time.Time
	for _, raw := range starts {
		start, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			continue
		}
		localStart := start.In(loc)
		date := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, time.UTC)
		if from.IsZero() || date.Before(from) {
			from = date
		}
		if to.IsZero() || date.After(to) {
			to = date
		}
	}
	if from.IsZero() || to.IsZero() {
		return fallbackFrom, fallbackTo
	}
	return from, to
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
