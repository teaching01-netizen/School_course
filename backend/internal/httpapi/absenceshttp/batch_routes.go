package absenceshttp

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/idempotency"
	"warwick-institute/internal/otp"
)

type batchAbsenceCreateItem struct {
	SubjectID        string   `json:"subject_id"`
	CourseID         string   `json:"course_id"`
	DateFrom         string   `json:"date_from"`
	DateTo           string   `json:"date_to"`
	SitInMethod      *string  `json:"sit_in_method"`
	SitInCourseID    *string  `json:"sit_in_course_id"`
	MissedSessionIDs []string `json:"missed_session_ids"`
	SitInSessionIDs  []string `json:"sit_in_session_ids"`
}

type batchAbsenceCreateRequest struct {
	Wcode             string                   `json:"wcode"`
	ReasonCategory    *string                  `json:"reason_category"`
	Reason            *string                  `json:"reason"`
	VerificationToken *string                  `json:"verification_token"`
	Items             []batchAbsenceCreateItem `json:"items"`
}

type batchAbsenceCreateResponse struct {
	Items []managedAbsenceDTO `json:"items"`
}

type createdAbsenceRecord struct {
	row      sqldb.ManagedAbsenceRow
	sessions []sqldb.ManagedAbsenceSession
	missed   []sqldb.ManagedAbsenceSession
}

func (s *server) handleAbsenceBatchCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requestOriginAllowed(w, r) {
		return
	}
	if !s.a.WithIdempotentTx(w, r, idempotency.SystemActorUUID, "absences-public", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)

		var body batchAbsenceCreateRequest
		if err := s.a.DecodeJSON(w, r, &body); err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
			return 0, nil, err
		}
		if strings.TrimSpace(body.Wcode) == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "wcode is required")
			return 0, nil, fmt.Errorf("wcode is required")
		}
		if len(body.Items) == 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_items", "At least one class must be selected")
			return 0, nil, fmt.Errorf("no items")
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
			if session.Status != "verified" {
				s.a.WriteErr(w, http.StatusConflict, "verification_required", "Parent verification is not complete")
				return 0, nil, fmt.Errorf("verification not complete")
			}
		}

		var studentPhone pgtype.Text
		var studentEmail pgtype.Text
		var studentNickname pgtype.Text
		var successSMSRecipients []string
		if contactRows, contactErr := qtx.StudentSubjectByWCode(r.Context(), body.Wcode); contactErr == nil && len(contactRows) > 0 {
			studentPhone = contactRows[0].StudentPhone
			studentEmail = contactRows[0].Email
			studentNickname = contactRows[0].Nickname
			successSMSRecipients = successSMSPhones(contactRows[0].ParentPhone, contactRows[0].StudentPhone)
		} else if contactErr != nil && s.deps.Log != nil {
			s.deps.Log.Error("failed to load absence contact phones", "wcode", body.Wcode, "error", contactErr)
		}

		created := make([]createdAbsenceRecord, 0, len(body.Items))
		for _, item := range body.Items {
			record, ok := s.createAbsenceRecordTx(w, r, qtx, tx, settings, body.Wcode, reasonCategory, reason, studentEmail, studentNickname, studentPhone, item)
			if !ok {
				return 0, nil, fmt.Errorf("failed to create absence item")
			}
			created = append(created, record)
		}

		if verificationToken != "" && len(created) > 0 {
			absenceIDStr, err := sUUIDString(created[0].row.ID)
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

		if settings.Notifications.SmsSuccessTemplate != "" && len(successSMSRecipients) > 0 {
			smsItems := make([]successSMSItem, 0, len(created))
			for _, record := range created {
				smsItems = append(smsItems, successSMSItem{row: record.row, sessions: record.sessions, missed: record.missed})
			}
			sendBatchSuccessSMS(s.deps.SMS, s.deps.Log, settings.Notifications.SmsSuccessTemplate, smsItems, successSMSRecipients, s.deps.InstituteTZ)
		}

		out := make([]managedAbsenceDTO, 0, len(created))
		for _, record := range created {
			dto := s.managedAbsenceDTO(record.row)
			dto.Status = "pending"
			dto.MissedSessions = s.sessionDTO(record.missed)
			dto.SitIns = s.sessionDTO(record.sessions)
			out = append(out, dto)
		}

		return http.StatusCreated, batchAbsenceCreateResponse{Items: out}, nil
	}) {
		return
	}
}

func (s *server) createAbsenceRecordTx(
	w http.ResponseWriter,
	r *http.Request,
	qtx *sqldb.Queries,
	tx pgx.Tx,
	settings absenceSettings,
	wcode string,
	reasonCategory pgtype.Text,
	reason pgtype.Text,
	studentEmail pgtype.Text,
	studentNickname pgtype.Text,
	studentPhone pgtype.Text,
	item batchAbsenceCreateItem,
) (createdAbsenceRecord, bool) {
	if strings.TrimSpace(item.SubjectID) == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "subject_id is required")
		return createdAbsenceRecord{}, false
	}
	if strings.TrimSpace(item.CourseID) == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", "course_id is required")
		return createdAbsenceRecord{}, false
	}
	if item.DateFrom == "" || item.DateTo == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "date_from and date_to are required")
		return createdAbsenceRecord{}, false
	}
	dateFrom := parseDate(item.DateFrom)
	dateTo := parseDate(item.DateTo)
	if !dateFrom.Valid || !dateTo.Valid {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "Invalid date format, use YYYY-MM-DD")
		return createdAbsenceRecord{}, false
	}
	if dateTo.Time.Before(dateFrom.Time) {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "date_to must be on or after date_from")
		return createdAbsenceRecord{}, false
	}
	days := int(dateTo.Time.Sub(dateFrom.Time).Hours() / 24)
	if days > settings.Form.MaxDateRangeDays {
		s.a.WriteErr(w, http.StatusBadRequest, "date_range_exceeded", fmt.Sprintf("Date range must be %d days or less", settings.Form.MaxDateRangeDays))
		return createdAbsenceRecord{}, false
	}
	sitInMethod, err := normalizeSubmissionSitInMethod(item.SitInMethod)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_method", "Invalid sit-in method")
		return createdAbsenceRecord{}, false
	}
	if len(item.SitInSessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
		s.a.WriteErr(w, http.StatusBadRequest, "too_many_sessions", "Selected sit-in sessions exceed the configured maximum")
		return createdAbsenceRecord{}, false
	}
	if len(item.MissedSessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
		s.a.WriteErr(w, http.StatusBadRequest, "too_many_missed_sessions", "Selected missed sessions exceed the configured maximum")
		return createdAbsenceRecord{}, false
	}
	var sitInCourseID pgtype.UUID
	if item.SitInCourseID != nil && strings.TrimSpace(*item.SitInCourseID) != "" {
		parsed, err := s.a.ParseUUID(strings.TrimSpace(*item.SitInCourseID))
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_course_id", "Invalid sit-in course")
			return createdAbsenceRecord{}, false
		}
		sitInCourseID = parsed
	}

	student, subjectID, course, err := s.resolveAbsenceSelection(r.Context(), qtx, tx, wcode, &item.SubjectID, &item.CourseID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		if status == http.StatusInternalServerError {
			status = http.StatusBadRequest
			code = "bad_selection"
			msg = err.Error()
		}
		s.a.WriteErr(w, status, code, msg)
		return createdAbsenceRecord{}, false
	}

	row, err := qtx.AbsenceCreate(r.Context(), sqldb.AbsenceCreateParams{
		Wcode:         wcode,
		CourseID:      course.CourseID,
		DateFrom:      dateFrom,
		DateTo:        dateTo,
		Reason:        reason,
		SitInCourseID: sitInCourseID,
	})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return createdAbsenceRecord{}, false
	}
	if err := qtx.AbsenceSetSubmissionMetadata(r.Context(), row.ID, subjectID, sitInMethod, student.FullName, studentEmail, studentNickname, studentPhone, reasonCategory, sitInCourseID); err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return createdAbsenceRecord{}, false
	}

	if len(item.SitInSessionIDs) > 0 {
		var sessionUUIDs []pgtype.UUID
		for _, sid := range item.SitInSessionIDs {
			uid, err := s.a.ParseUUID(sid)
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_session_id", "Invalid sit-in session ID")
				return createdAbsenceRecord{}, false
			}
			sessionUUIDs = append(sessionUUIDs, uid)
		}
		if sitInMethod.String != "physical" {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sessions", "Only physical sit-ins may select sessions")
			return createdAbsenceRecord{}, false
		}
		count, err := qtx.ValidSitInSessionCount(r.Context(), row.ID, sitInCourseID, sessionUUIDs)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return createdAbsenceRecord{}, false
		}
		if count != len(sessionUUIDs) {
			s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Sit-in sessions must be in the selected course and must not overlap the missed class")
			return createdAbsenceRecord{}, false
		}
		if err := qtx.AbsenceSitInsCreate(r.Context(), row.ID, sessionUUIDs); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return createdAbsenceRecord{}, false
		}
	}

	if len(item.MissedSessionIDs) > 0 {
		var missedUUIDs []pgtype.UUID
		for _, sid := range item.MissedSessionIDs {
			uid, err := s.a.ParseUUID(sid)
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_missed_session_id", "Invalid missed session ID")
				return createdAbsenceRecord{}, false
			}
			missedUUIDs = append(missedUUIDs, uid)
		}
		count, err := qtx.ValidMissedSessionCount(r.Context(), row.ID, missedUUIDs)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return createdAbsenceRecord{}, false
		}
		if count != len(missedUUIDs) {
			s.a.WriteErr(w, http.StatusBadRequest, "invalid_missed_sessions", "Missed sessions must be in the selected class and absence dates")
			return createdAbsenceRecord{}, false
		}
		if err := qtx.AbsenceMissedSessionsCreate(r.Context(), row.ID, missedUUIDs); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return createdAbsenceRecord{}, false
		}
	}

	if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{
		AbsenceID: row.ID,
		Action:    "submitted",
		ActorRole: "student",
		Details:   map[string]any{"wcode": wcode},
	}); err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write absence timeline")
		return createdAbsenceRecord{}, false
	}

	managed, err := qtx.ManagedAbsenceGet(r.Context(), row.ID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return createdAbsenceRecord{}, false
	}
	sessions, err := qtx.ManagedAbsenceSessions(r.Context(), row.ID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return createdAbsenceRecord{}, false
	}
	missed, err := qtx.ManagedAbsenceMissedSessions(r.Context(), row.ID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return createdAbsenceRecord{}, false
	}

	managed.Status = "pending"
	return createdAbsenceRecord{row: managed, sessions: sessions, missed: missed}, true
}
