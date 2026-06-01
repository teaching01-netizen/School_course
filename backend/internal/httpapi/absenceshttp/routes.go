package absenceshttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/idempotency"
	"warwick-institute/internal/otp"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("GET /api/v1/courses/public", s.handleCoursesPublic)

	mux.HandleFunc("GET /api/v1/absence-form-config", s.handleFormConfigGet)
	mux.HandleFunc("/api/v1/absences", s.handleAbsencesDispatch)
	mux.HandleFunc("/api/v1/absences/", s.handleAbsencesDispatch)

	// Public sub-routes — register explicitly so literal segments beat {id} wildcard.
	// parent-verification/{token} cannot be registered here because it conflicts
	// with {id}/timeline — handled via the dispatch prefix pattern.
	mux.HandleFunc("GET /api/v1/absences/student-lookup", s.handleStudentLookup)
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
	if !s.a.WithIdempotentTx(w, r, idempotency.SystemActorUUID, "absences-public", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)

		var body struct {
			Wcode             string   `json:"wcode"`
			SubjectID         string   `json:"subject_id"`
			DateFrom          string   `json:"date_from"`
			DateTo            string   `json:"date_to"`
			ReasonCategory    *string  `json:"reason_category"`
			Reason            *string  `json:"reason"`
			SitInMethod       *string  `json:"sit_in_method"`
			SitInCourseID     *string  `json:"sit_in_course_id"`
			MissedSessionIDs  []string `json:"missed_session_ids"`
			SitInSessionIDs   []string `json:"sit_in_session_ids"`
			VerificationToken *string  `json:"verification_token"`
		}
		if err := s.a.DecodeJSON(w, r, &body); err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
			return 0, nil, err
		}
		if strings.TrimSpace(body.Wcode) == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "wcode is required")
			return 0, nil, fmt.Errorf("wcode is required")
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

		var sitInMethod pgtype.Text
		if body.SitInMethod != nil {
			if *body.SitInMethod != "zoom" && *body.SitInMethod != "physical" {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_method", "Invalid sit-in method")
				return 0, nil, fmt.Errorf("bad sit in method")
			}
			sitInMethod = pgtype.Text{String: *body.SitInMethod, Valid: true}
		}
		if len(body.SitInSessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
			s.a.WriteErr(w, http.StatusBadRequest, "too_many_sessions", "Selected sit-in sessions exceed the configured maximum")
			return 0, nil, fmt.Errorf("too many sessions")
		}
		if len(body.MissedSessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
			s.a.WriteErr(w, http.StatusBadRequest, "too_many_missed_sessions", "Selected missed sessions exceed the configured maximum")
			return 0, nil, fmt.Errorf("too many missed sessions")
		}
		var sitInCourseID pgtype.UUID
		if body.SitInCourseID != nil && strings.TrimSpace(*body.SitInCourseID) != "" {
			sitInCourseID, err = s.a.ParseUUID(strings.TrimSpace(*body.SitInCourseID))
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_course_id", "Invalid sit-in course")
				return 0, nil, err
			}
		}

		student, subjectID, course, err := s.resolveAbsenceSelection(r.Context(), qtx, tx, body.Wcode, &body.SubjectID, nil)
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

		requireVerification := settings.Notifications.SmsParentEnabled && !settings.Notifications.AllowSubmitWithoutOtp
		if !settings.Notifications.SmsParentEnabled && !settings.Notifications.AllowSubmitWithoutOtp {
			s.a.WriteErr(w, http.StatusForbidden, "feature_disabled", "Parent verification codes are currently disabled")
			return 0, nil, fmt.Errorf("verification disabled")
		}

		var verificationToken string
		if body.VerificationToken != nil {
			verificationToken = strings.TrimSpace(*body.VerificationToken)
		}
		if verificationToken == "" && requireVerification {
			s.a.WriteErr(w, http.StatusBadRequest, "verification_required", "Parent verification is required before submitting this absence")
			return 0, nil, fmt.Errorf("verification required")
		}

		if verificationToken != "" {
			session, sessionErr := s.deps.OTP.LoadSessionTx(r.Context(), tx, verificationToken)
			switch {
			case sessionErr == nil:
			case errors.Is(sessionErr, otp.ErrExpired):
				s.a.WriteErr(w, http.StatusGone, "otp_expired", "Verification token expired")
				return 0, nil, sessionErr
			case errors.Is(sessionErr, otp.ErrTampered):
				s.a.WriteErr(w, http.StatusBadRequest, "bad_token", "Verification token is invalid")
				return 0, nil, sessionErr
			default:
				status, code, msg := s.a.ClassifyDBErr(sessionErr)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, sessionErr
			}
			if session.Wcode != body.Wcode {
				s.a.WriteErr(w, http.StatusConflict, "token_mismatch", "Verification token does not match this student")
				return 0, nil, fmt.Errorf("token mismatch")
			}
			if session.Status == "consumed" {
				if session.ConsumedAbsence.Valid {
					existingAbsenceID := session.ConsumedAbsence
					existing, err := qtx.ManagedAbsenceGet(r.Context(), existingAbsenceID)
					if err != nil {
						status, code, msg := s.a.ClassifyDBErr(err)
						s.a.WriteErr(w, status, code, msg)
						return 0, nil, err
					}
					resp := managedAbsenceResponse(existing)
					return http.StatusOK, resp, nil
				}
				s.a.WriteErr(w, http.StatusConflict, "already_submitted", "This verification token has already been used")
				return 0, nil, fmt.Errorf("token consumed")
			}
			if session.Status != "verified" {
				s.a.WriteErr(w, http.StatusConflict, "verification_required", "Parent verification is not complete")
				return 0, nil, fmt.Errorf("verification not complete")
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
		if err := qtx.AbsenceSetSubmissionMetadata(r.Context(), item.ID, subjectID, sitInMethod, student.FullName, reasonCategory, sitInCourseID); err != nil {
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
			count, err := qtx.ValidSitInSessionCount(r.Context(), item.ID, sitInCourseID, sessionUUIDs)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			if count != len(sessionUUIDs) {
				s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Sit-in sessions must be in the selected course and must not overlap the missed class")
				return 0, nil, fmt.Errorf("invalid sessions")
			}
			if err := qtx.AbsenceSitInsCreate(r.Context(), item.ID, sessionUUIDs); err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
		}
		if len(body.MissedSessionIDs) > 0 {
			var missedUUIDs []pgtype.UUID
			for _, sid := range body.MissedSessionIDs {
				uid, err := s.a.ParseUUID(sid)
				if err != nil {
					s.a.WriteErr(w, http.StatusBadRequest, "bad_missed_session_id", "Invalid missed session ID")
					return 0, nil, err
				}
				missedUUIDs = append(missedUUIDs, uid)
			}
			count, err := qtx.ValidMissedSessionCount(r.Context(), item.ID, missedUUIDs)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			if count != len(missedUUIDs) {
				s.a.WriteErr(w, http.StatusBadRequest, "invalid_missed_sessions", "Missed sessions must be in the selected class and absence dates")
				return 0, nil, fmt.Errorf("invalid missed sessions")
			}
			if err := qtx.AbsenceMissedSessionsCreate(r.Context(), item.ID, missedUUIDs); err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
		}

		if verificationToken != "" {
			absenceIDStr, err := sUUIDString(item.ID)
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return 0, nil, err
			}
			absenceUUID, err := uuid.Parse(absenceIDStr)
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return 0, nil, err
			}
			if err := s.deps.OTP.ConsumeSessionTx(r.Context(), tx, verificationToken, absenceUUID); err != nil {
				if errors.Is(err, otp.ErrAlreadyVerified) {
					s.a.WriteErr(w, http.StatusConflict, "already_submitted", "This verification token has already been used")
				} else if errors.Is(err, otp.ErrTampered) {
					s.a.WriteErr(w, http.StatusBadRequest, "bad_token", "Verification token is invalid")
				} else {
					status, code, msg := s.a.ClassifyDBErr(err)
					s.a.WriteErr(w, status, code, msg)
				}
				return 0, nil, err
			}
		}

		if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{
			AbsenceID: item.ID,
			Action:    "submitted",
			ActorRole: "student",
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
			phone := resolveParentPhone(r.Context(), qtx, body.Wcode)
			if phone != "" {
				sessions, sesErr := qtx.ManagedAbsenceSessions(r.Context(), item.ID)
				if sesErr == nil {
					missed, missedErr := qtx.ManagedAbsenceMissedSessions(r.Context(), item.ID)
					if missedErr == nil {
						sendSuccessSMS(s.deps.SMS, s.deps.Log, settings.Notifications.SmsSuccessTemplate, managed, sessions, missed, phone, s.deps.InstituteTZ)
					} else {
						if s.deps.Log != nil {
							s.deps.Log.Error("failed to load missed sessions for sms", "absence_id", item.ID, "error", missedErr)
						}
						sendSuccessSMS(s.deps.SMS, s.deps.Log, settings.Notifications.SmsSuccessTemplate, managed, sessions, nil, phone, s.deps.InstituteTZ)
					}
				} else if s.deps.Log != nil {
					s.deps.Log.Error("failed to load absence sessions for sms", "absence_id", item.ID, "error", sesErr)
				}
			}
		}

		resp := managedAbsenceResponse(managed)
		resp["status"] = "pending"
		return http.StatusCreated, resp, nil
	}) {
		return
	}
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
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if len(rows) == 0 {
		s.a.WriteErr(w, http.StatusNotFound, "no_subjects", "No enrolled subjects found for this student")
		return
	}

	type subjectDTO struct {
		ID             string  `json:"id"`
		Code           string  `json:"code"`
		Name           string  `json:"name"`
		ActiveCourseID *string `json:"active_course_id,omitempty"`
	}

	studentID, _ := s.a.UUIDString(rows[0].StudentID)
	subjects := make([]subjectDTO, 0, len(rows))
	for _, r := range rows {
		sid, _ := s.a.UUIDString(r.SubjectID)
		dto := subjectDTO{ID: sid, Code: r.SubjectCode, Name: r.SubjectName}
		if cid, err := s.a.UUIDString(r.ActiveCourseID); err == nil {
			dto.ActiveCourseID = &cid
		}
		subjects = append(subjects, dto)
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"student_id":   studentID,
		"wcode":        rows[0].Wcode,
		"full_name":    rows[0].FullName,
		"parent_phone": stringPtrIfValid(rows[0].ParentPhone),
		"subjects":     subjects,
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

// Public: return all sessions for a student across enrolled subjects in a date range, with absence flagging
func (s *server) handleSessionsInRange(w http.ResponseWriter, r *http.Request) {
	wcode := r.URL.Query().Get("wcode")
	dateFromStr := r.URL.Query().Get("date_from")
	dateToStr := r.URL.Query().Get("date_to")

	if wcode == "" || dateFromStr == "" || dateToStr == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_params", "wcode, date_from, date_to are required")
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

	settings, err := s.readAbsenceSettings(r)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	days := int(dateTo.Sub(dateFrom).Hours() / 24)
	if days > settings.Form.MaxDateRangeDays {
		s.a.WriteErr(w, http.StatusBadRequest, "date_range_exceeded",
			fmt.Sprintf("Date range must be %d days or less", settings.Form.MaxDateRangeDays))
		return
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

	// Query sessions in range for all enrolled subjects.
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
	}

	rows, err := s.deps.DB.Query(r.Context(), `
		SELECT sess.id, sess.start_at, sess.end_at,
		       c.id, c.code, c.name,
		       sub.id, sub.code, sub.name
		FROM sessions sess
		JOIN courses c ON c.id = sess.course_id AND c.deleted_at IS NULL
		JOIN subjects sub ON sub.id = c.subject_id AND sub.deleted_at IS NULL
		JOIN course_students cs ON cs.course_id = c.id AND cs.status = 'enrolled'
		JOIN students st ON st.id = cs.student_id
		WHERE st.wcode = $1
		  AND sess.start_at >= $2
		  AND sess.start_at < ($3::date + interval '1 day')
		  AND sess.deleted_at IS NULL
		ORDER BY sub.code, sess.start_at
	`, wcode, dateFrom, dateTo)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	defer rows.Close()

	var sessions []sessionRow
	for rows.Next() {
		var dbRow sessionDBRow
		if err := rows.Scan(&dbRow.ID, &dbRow.StartAt, &dbRow.EndAt,
			&dbRow.CourseID, &dbRow.CourseCode, &dbRow.CourseName,
			&dbRow.SubjectID, &dbRow.SubjectCode, &dbRow.SubjectName); err != nil {
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
		JOIN student_absences sa ON sa.course_id = sess.course_id
		WHERE sa.wcode = $1
		  AND sa.status <> 'cancelled'
		  AND sess.start_at >= sa.date_from
		  AND sess.start_at < (sa.date_to + interval '1 day')
		  AND sess.start_at >= $2
		  AND sess.start_at < ($3::date + interval '1 day')
		  AND sess.deleted_at IS NULL
	`, wcode, dateFrom, dateTo)
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
		RuleName          string           `json:"rule_name,omitempty"`
		RuleType          string           `json:"rule_type,omitempty"`
		SitInMethod       string           `json:"sit_in_method"`
		SitInCourse       *SitInCourseInfo `json:"sit_in_course,omitempty"`
		AvailableSessions []sessionBrief   `json:"available_sessions,omitempty"`
		MissedSessions    []sessionBrief   `json:"missed_sessions,omitempty"`
	}
	type courseResponse struct {
		SubjectID   string               `json:"subject_id"`
		SubjectCode string               `json:"subject_code"`
		SubjectName string               `json:"subject_name"`
		CourseID    string               `json:"course_id"`
		CourseCode  string               `json:"course_code"`
		CourseName  string               `json:"course_name"`
		Sessions    []sessionResponse    `json:"sessions"`
		SitIn       *courseSitInResponse `json:"sit_in,omitempty"`
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
				Date:          sess.StartAt[:10],
				AlreadyAbsent: absentSet[sess.ID],
			})
		}

		var sitIn *courseSitInResponse
		// Resolve sit-in using the student's enrolled course ID for this block
		courseID, cErr := s.a.ParseUUID(g.CourseID)
		if cErr == nil {
			subjectID, sErr := s.a.ParseUUID(g.SubjectID)
			if sErr == nil {
				result, resolveErr := resolveSitInForCourse(r.Context(), s.deps.Q, wcode, courseID, subjectID, dateFrom, dateTo)
				if resolveErr != nil {
					s.deps.Log.Error("sit-in resolution failed", "course_id", g.CourseID, "error", resolveErr)
				} else if result != nil && result.SitInMethod != SitInMethodNone {
					sitIn = &courseSitInResponse{
						RuleName:    result.RuleName,
						RuleType:    result.RuleType,
						SitInMethod: result.SitInMethod,
						SitInCourse: result.SitInCourse,
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

		courses = append(courses, courseResponse{
			SubjectID:   g.SubjectID,
			SubjectCode: g.SubjectCode,
			SubjectName: g.SubjectName,
			CourseID:    g.CourseID,
			CourseCode:  g.CourseCode,
			CourseName:  g.CourseName,
			Sessions:    sessionsResp,
			SitIn:       sitIn,
		})
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{"subjects": courses})
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
