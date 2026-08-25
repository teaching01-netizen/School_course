package absenceshttp

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences"
	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
)

func (s *server) createStaffAbsenceTx(
	w http.ResponseWriter,
	r *http.Request,
	tx pgx.Tx,
	qtx *sqldb.Queries,
	user auth.AuthenticatedUser,
	body staffCreateAbsenceRequest,
) (string, any, error) {
	createdID := ""
	body.Wcode = normalizeWCode(body.Wcode)
	if body.Wcode == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "wcode is required")
		return "", nil, fmt.Errorf("wcode is required")
	}
	if body.DateFrom == "" || body.DateTo == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "date_from and date_to are required")
		return "", nil, fmt.Errorf("date required")
	}
	if len(body.MissedSessionIDs) == 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_missed_sessions", "At least one missed session is required")
		return "", nil, fmt.Errorf("missed sessions required")
	}
	requestedStatus := absences.StatusPending
	if body.Status != nil {
		statusVal := strings.TrimSpace(*body.Status)
		switch absences.Status(statusVal) {
		case absences.StatusPending:
			requestedStatus = absences.StatusPending
		case absences.StatusSpecialApproved:
			requestedStatus = absences.StatusSpecialApproved
		default:
			s.a.WriteErr(w, http.StatusBadRequest, "bad_status", "status must be 'pending' or 'special_approved'")
			return "", nil, fmt.Errorf("bad status")
		}
	}

	settings, err := s.readAbsenceSettings(r)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return "", nil, err
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
				return "", nil, fmt.Errorf("bad reason category")
			}
			reasonCategory = pgtype.Text{String: value, Valid: true}
		}
	}
	if settings.Form.RequireReason && !reasonCategory.Valid {
		s.a.WriteErr(w, http.StatusBadRequest, "reason_required", "Select a reason category")
		return "", nil, fmt.Errorf("reason required")
	}
	if !settings.Form.AllowFreeTextReason && reason.Valid {
		s.a.WriteErr(w, http.StatusBadRequest, "free_text_not_allowed", "Free-text reason is disabled")
		return "", nil, fmt.Errorf("free text disabled")
	}

	student, subjectID, course, err := s.resolveStaffAbsenceSelection(r.Context(), qtx, tx, body.Wcode, body.SubjectID, body.CourseID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		if status == http.StatusInternalServerError {
			status = http.StatusBadRequest
			code = "bad_selection"
			msg = err.Error()
		}
		s.a.WriteErr(w, status, code, msg)
		return "", nil, err
	}

	dateFrom := parseDate(body.DateFrom)
	dateTo := parseDate(body.DateTo)
	if !dateFrom.Valid || !dateTo.Valid {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "Invalid date format, use YYYY-MM-DD")
		return "", nil, fmt.Errorf("invalid date")
	}
	if dateTo.Time.Before(dateFrom.Time) {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "date_to must be on or after date_from")
		return "", nil, fmt.Errorf("date_to before date_from")
	}
	sitInMethod, err := normalizeSubmissionSitInMethod(body.SitInMethod)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_method", "Invalid sit-in method")
		return "", nil, err
	}
	sessionUUIDs, err := parseUUIDStrings(body.SitInSessionIDs)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_session_id", "Invalid sit-in session ID")
		return "", nil, err
	}
	if len(sessionUUIDs) > 0 && sitInMethod.String != "physical" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_sessions", "Only physical sit-ins may select sessions")
		return "", nil, fmt.Errorf("non-physical sit-in with sessions")
	}

	if len(body.SitInSessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
		s.a.WriteErr(w, http.StatusBadRequest, "too_many_sessions", "Selected sit-in sessions exceed the configured maximum")
		return "", nil, fmt.Errorf("too many sit-in sessions")
	}
	if len(body.MissedSessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
		s.a.WriteErr(w, http.StatusBadRequest, "too_many_missed_sessions", "Selected missed sessions exceed the configured maximum")
		return "", nil, fmt.Errorf("too many missed sessions")
	}
	if sitInMethod.String == "physical" && len(body.SitInSessionIDs) == 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "sit_in_sessions_required", "Physical sit-in requires at least one sit-in session")
		return "", nil, fmt.Errorf("physical sit-in requires sessions")
	}
	if err := qtx.LockStudentForAbsenceSubmission(r.Context(), student.ID); err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not lock student absence submission")
		return "", nil, err
	}
	if err := ensureSitInSessionsAvailable(r.Context(), qtx, student.ID, sessionUUIDs); err != nil {
		if s.writeSitInSessionConflict(w, err) {
			return "", nil, err
		}
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not check sit-in session availability")
		return "", nil, err
	}
	var sitInCourseID pgtype.UUID
	if sitInMethod.Valid && body.SitInCourseID != nil && strings.TrimSpace(*body.SitInCourseID) != "" {
		parsed, err := s.a.ParseUUID(strings.TrimSpace(*body.SitInCourseID))
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_course_id", "Invalid sit-in course")
			return "", nil, err
		}
		sitInCourseID = parsed
	} else if sitInMethod.String == "physical" {
		sitInCourseID = course.CourseID
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
		return "", nil, err
	}
	if err := setAbsenceMergeGroupForCourse(r.Context(), qtx, row.ID, course.CourseID); err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return "", nil, err
	}

	if err := qtx.AbsenceSetSubmissionMetadata(r.Context(), row.ID, subjectID, sitInMethod, student.FullName, pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, reasonCategory, sitInCourseID); err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return "", nil, err
	}

	if len(body.SitInSessionIDs) > 0 {
		if shouldValidateStaffSitInSessions(requestedStatus) {
			excludeFinal, err := satVerbalCourseFinalClassExcluded(r.Context(), qtx, course.CourseID)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return "", nil, err
			}
			count, err := qtx.ValidSitInSessionOverlap(r.Context(), row.ID, sessionUUIDs, s.deps.InstituteTZ, excludeFinal)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return "", nil, err
			}
			if count != len(sessionUUIDs) {
				s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Sit-in sessions must be active and must not overlap the missed class")
				return "", nil, fmt.Errorf("invalid sit-in sessions")
			}
			if err := s.validateSitInCandidates(r.Context(), qtx, row.ID, sessionUUIDs); err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", err.Error())
				return "", nil, err
			}
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
		if err := qtx.AbsenceSitInsCreateWithSnapshot(r.Context(), row.ID, sitInInputs, s.deps.InstituteTZ, BuildSnapshotFromSessionRow); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return "", nil, err
		}
	}

	if len(body.MissedSessionIDs) > 0 {
		var missedUUIDs []pgtype.UUID
		for _, sid := range body.MissedSessionIDs {
			uid, err := s.a.ParseUUID(sid)
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_missed_session_id", "Invalid missed session ID")
				return "", nil, err
			}
			missedUUIDs = append(missedUUIDs, uid)
		}
		count, err := qtx.ValidMissedSessionCount(r.Context(), row.ID, missedUUIDs, s.deps.InstituteTZ)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return "", nil, err
		}
		if count != len(missedUUIDs) {
			s.a.WriteErr(w, http.StatusBadRequest, "invalid_missed_sessions", "Missed sessions must be in the selected class and absence dates")
			return "", nil, fmt.Errorf("invalid missed sessions")
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
		if _, err := qtx.AbsenceMissedSessionsCreateWithSnapshot(r.Context(), row.ID, snapshotInputs, s.deps.InstituteTZ, BuildSnapshotFromSessionRow); err != nil {
			var versionErr *sqldb.SessionVersionConflictError
			if errors.As(err, &versionErr) {
				s.a.WriteErr(w, http.StatusConflict, "session_version_conflict", "Session has been modified since you last loaded it. Please reload and try again.")
				return "", nil, err
			}
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return "", nil, err
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
		return "", nil, err
	}

	if requestedStatus == absences.StatusSpecialApproved {
		newVersion, err := qtx.AbsenceStatusUpdate(r.Context(), row.ID, string(absences.StatusSpecialApproved), actorID(user.ID), row.Version)
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not set special approved status")
			return "", nil, err
		}
		if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{
			AbsenceID: row.ID,
			Action:    string(absences.StatusSpecialApproved),
			ActorID:   actorID(user.ID),
			ActorRole: "admin",
			Details:   map[string]any{"from": "pending", "to": "special_approved", "staff_created": true, "wcode": body.Wcode},
		}); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write absence timeline")
			return "", nil, err
		}
		row.Version = newVersion
	}

	managed, err := qtx.ManagedAbsenceGet(r.Context(), row.ID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return "", nil, err
	}

	dto := s.managedAbsenceDTO(managed)
	sessions, err := qtx.ManagedAbsenceSessions(r.Context(), row.ID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return "", nil, err
	}
	missed, err := qtx.ManagedAbsenceMissedSessions(r.Context(), row.ID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return "", nil, err
	}
	dto.SitIns = s.sessionDTO(sessions)
	dto.MissedSessions = s.sessionDTO(missed)
	if id, err := sUUIDString(row.ID); err == nil {
		createdID = id
	}

	smsTemplate := successSMSTemplateForItems(settings, managed.Status, []successSMSItem{{row: managed, sessions: sessions, missed: missed}})
	if smsTemplate != "" {
		if contactRows, contactErr := qtx.StudentSubjectByWCode(r.Context(), body.Wcode); contactErr == nil && len(contactRows) > 0 {
			phones := successSMSPhones(contactRows[0].ParentPhone, contactRows[0].StudentPhone)
			if len(phones) > 0 {
				sess, _ := qtx.ManagedAbsenceSessions(r.Context(), row.ID)
				mis, _ := qtx.ManagedAbsenceMissedSessions(r.Context(), row.ID)
				loc, _ := time.LoadLocation(s.deps.InstituteTZ)
				if loc == nil {
					loc = time.UTC
				}
				rendered := renderSuccessSMSTemplate(smsTemplate, managed, sess, mis, loc)
				dto.SmsPreview = &smsPreviewDTO{Phones: phones, Message: rendered}
			}
		}
	}

	return createdID, dto, nil
}
