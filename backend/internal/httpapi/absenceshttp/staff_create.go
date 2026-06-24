package absenceshttp

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/idempotency"
)

type staffCreateAbsenceRequest struct {
	Wcode            string   `json:"wcode"`
	SubjectID        *string  `json:"subject_id"`
	CourseID         *string  `json:"course_id"`
	DateFrom         string   `json:"date_from"`
	DateTo           string   `json:"date_to"`
	MissedSessionIDs []string `json:"missed_session_ids"`
	SitInMethod      *string  `json:"sit_in_method"`
	SitInCourseID    *string  `json:"sit_in_course_id"`
	SitInSessionIDs  []string `json:"sit_in_session_ids"`
	Reason           *string  `json:"reason"`
	ReasonCategory   *string  `json:"reason_category"`
}

func (s *server) handleStaffCreateAbsence(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	var createdID string
	if !s.a.WithIdempotentTx(w, r, idempotency.SystemActorUUID, "absences-staff", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)

		var body staffCreateAbsenceRequest
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
		if len(body.MissedSessionIDs) == 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_missed_sessions", "At least one missed session is required")
			return 0, nil, fmt.Errorf("missed sessions required")
		}
		if body.SitInMethod == nil || strings.TrimSpace(*body.SitInMethod) == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_method", "sit_in_method is required")
			return 0, nil, fmt.Errorf("sit-in method required")
		}

		settings, err := s.readAbsenceSettings(r)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
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

		student, subjectID, course, err := s.resolveAbsenceSelection(r.Context(), qtx, tx, body.Wcode, body.SubjectID, body.CourseID)
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

		dateFrom := parseDate(body.DateFrom)
		dateTo := parseDate(body.DateTo)
		if !dateFrom.Valid || !dateTo.Valid {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "Invalid date format, use YYYY-MM-DD")
			return 0, nil, fmt.Errorf("invalid date")
		}
		if dateTo.Time.Before(dateFrom.Time) {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "date_to must be on or after date_from")
			return 0, nil, fmt.Errorf("date_to before date_from")
		}
		days := int(dateTo.Time.Sub(dateFrom.Time).Hours() / 24)
		if days > settings.Form.MaxDateRangeDays {
			s.a.WriteErr(w, http.StatusBadRequest, "date_range_exceeded", fmt.Sprintf("Date range must be %d days or less", settings.Form.MaxDateRangeDays))
			return 0, nil, fmt.Errorf("date range exceeded")
		}

		sitInMethod, err := normalizeSubmissionSitInMethod(body.SitInMethod)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_method", "Invalid sit-in method")
			return 0, nil, err
		}

		if len(body.SitInSessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
			s.a.WriteErr(w, http.StatusBadRequest, "too_many_sessions", "Selected sit-in sessions exceed the configured maximum")
			return 0, nil, fmt.Errorf("too many sit-in sessions")
		}
		if len(body.MissedSessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
			s.a.WriteErr(w, http.StatusBadRequest, "too_many_missed_sessions", "Selected missed sessions exceed the configured maximum")
			return 0, nil, fmt.Errorf("too many missed sessions")
		}
		if sitInMethod.String == "physical" && len(body.SitInSessionIDs) == 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "sit_in_sessions_required", "Physical sit-in requires at least one sit-in session")
			return 0, nil, fmt.Errorf("physical sit-in requires sessions")
		}
		if sitInMethod.String == "physical" && (body.SitInCourseID == nil || strings.TrimSpace(*body.SitInCourseID) == "") {
			s.a.WriteErr(w, http.StatusBadRequest, "sit_in_course_required", "Physical sit-in requires a sit-in course")
			return 0, nil, fmt.Errorf("physical sit-in requires course")
		}

		var sitInCourseID pgtype.UUID
		if body.SitInCourseID != nil && strings.TrimSpace(*body.SitInCourseID) != "" {
			parsed, err := s.a.ParseUUID(strings.TrimSpace(*body.SitInCourseID))
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_course_id", "Invalid sit-in course")
				return 0, nil, err
			}
			sitInCourseID = parsed
		}

		totalSessions, totalErr := qtx.CourseSessionCount(r.Context(), course.CourseID)
		if totalErr != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error checking course sessions")
			return 0, nil, totalErr
		}
		existingAbsences, absErr := qtx.StudentAbsenceCountForCourse(r.Context(), body.Wcode, course.CourseID)
		if absErr != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error checking absence count")
			return 0, nil, absErr
		}
		if projectedAbsenceRecordLimitExceeded(totalSessions, existingAbsences, 1) {
			s.a.WriteErr(w, http.StatusForbidden, "absence_limit_exceeded", "You have reached the maximum number of absences allowed for this course")
			return 0, nil, fmt.Errorf("absence limit exceeded")
		}

		row, err := qtx.AbsenceCreate(r.Context(), sqldb.AbsenceCreateParams{
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

		if err := qtx.AbsenceSetSubmissionMetadata(r.Context(), row.ID, subjectID, sitInMethod, student.FullName, pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, reasonCategory, sitInCourseID); err != nil {
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
				return 0, nil, fmt.Errorf("non-physical sit-in with sessions")
			}
			count, err := qtx.ValidSitInSessionCount(r.Context(), row.ID, sitInCourseID, sessionUUIDs)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			if count != len(sessionUUIDs) {
				s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Sit-in sessions must be in the selected course and must not overlap the missed class")
				return 0, nil, fmt.Errorf("invalid sit-in sessions")
			}
			if err := qtx.AbsenceSitInsCreate(r.Context(), row.ID, sessionUUIDs); err != nil {
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
			count, err := qtx.ValidMissedSessionCount(r.Context(), row.ID, missedUUIDs)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			if count != len(missedUUIDs) {
				s.a.WriteErr(w, http.StatusBadRequest, "invalid_missed_sessions", "Missed sessions must be in the selected class and absence dates")
				return 0, nil, fmt.Errorf("invalid missed sessions")
			}
			timingRows, err := qtx.ValidMissedSessionTiming(r.Context(), row.ID, missedUUIDs)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			if timingErr := validateSessionTiming(settings.Form, time.Now(), sessionTimingInfos(timingRows)); timingErr != nil {
				s.a.WriteErr(w, http.StatusBadRequest, timingErr.code, timingErr.message)
				return 0, nil, timingErr
			}
			if err := qtx.AbsenceMissedSessionsCreate(r.Context(), row.ID, missedUUIDs); err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
		}

		if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{
			AbsenceID: row.ID,
			Action:    "created_by_staff",
			ActorID:   actorID(user.ID),
			ActorRole: "admin",
			Details:   map[string]any{"staff_created": true, "wcode": body.Wcode},
		}); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write absence timeline")
			return 0, nil, err
		}

		managed, err := qtx.ManagedAbsenceGet(r.Context(), row.ID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}

		dto := s.managedAbsenceDTO(managed)
		dto.Status = "pending"
		if id, err := sUUIDString(row.ID); err == nil {
			createdID = id
		}

		if settings.Notifications.SmsSuccessTemplate != "" {
			if contactRows, contactErr := qtx.StudentSubjectByWCode(r.Context(), body.Wcode); contactErr == nil && len(contactRows) > 0 {
				phones := successSMSPhones(contactRows[0].ParentPhone, contactRows[0].StudentPhone)
				if len(phones) > 0 {
					sess, _ := qtx.ManagedAbsenceSessions(r.Context(), row.ID)
					mis, _ := qtx.ManagedAbsenceMissedSessions(r.Context(), row.ID)
					loc, _ := time.LoadLocation(s.deps.InstituteTZ)
					if loc == nil {
						loc = time.UTC
					}
					rendered := renderSuccessSMSTemplate(settings.Notifications.SmsSuccessTemplate, managed, sess, mis, loc)
					dto.SmsPreview = &smsPreviewDTO{Phones: phones, Message: rendered}
				}
			}
		}

		return http.StatusCreated, dto, nil
	}) {
		return
	}
	if createdID != "" {
		s.publishAbsenceChanges([]string{createdID})
	}
}

func (s *server) handleSendSuccessSMS(w http.ResponseWriter, r *http.Request) {
	_, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid absence ID")
		return
	}

	idempotencyKey := "send-sms-" + r.PathValue("id")
	if !s.a.WithIdempotentTx(w, r, idempotency.SystemActorUUID, idempotencyKey, s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)

		managed, err := qtx.ManagedAbsenceGet(r.Context(), id)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}

		settings, err := s.readAbsenceSettings(r)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}

		if settings.Notifications.SmsSuccessTemplate == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "sms_disabled", "SMS notifications are not configured")
			return 0, nil, fmt.Errorf("sms not configured")
		}

		contactRows, contactErr := qtx.StudentSubjectByWCode(r.Context(), managed.Wcode)
		if contactErr != nil || len(contactRows) == 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "no_contacts", "No contact phone numbers found for this student")
			return 0, nil, fmt.Errorf("no contacts")
		}
		phones := successSMSPhones(contactRows[0].ParentPhone, contactRows[0].StudentPhone)
		if len(phones) == 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "no_phones", "No phone numbers available for this student")
			return 0, nil, fmt.Errorf("no phones")
		}

		sessions, _ := qtx.ManagedAbsenceSessions(r.Context(), id)
		missed, _ := qtx.ManagedAbsenceMissedSessions(r.Context(), id)

		sent := sendSuccessSMS(s.deps.SMS, s.deps.Log, settings.Notifications.SmsSuccessTemplate, managed, sessions, missed, phones, s.deps.InstituteTZ)
		if !sent {
			s.a.WriteErr(w, http.StatusInternalServerError, "sms_send_failed", "Failed to send SMS notification")
			return 0, nil, fmt.Errorf("sms send failed")
		}

		return http.StatusOK, map[string]any{"sent": true, "recipient_count": len(phones)}, nil
	}) {
		return
	}
}
