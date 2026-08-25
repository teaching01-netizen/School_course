package absenceshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/idempotency"
)

type staffCreateAbsenceRequest struct {
	Wcode            string         `json:"wcode"`
	SubjectID        *string        `json:"subject_id"`
	CourseID         *string        `json:"course_id"`
	DateFrom         string         `json:"date_from"`
	DateTo           string         `json:"date_to"`
	MissedSessionIDs []string       `json:"missed_session_ids"`
	SitInMethod      *string        `json:"sit_in_method"`
	SitInCourseID    *string        `json:"sit_in_course_id"`
	SitInSessionIDs  []string       `json:"sit_in_session_ids"`
	Reason           *string        `json:"reason"`
	ReasonCategory   *string        `json:"reason_category"`
	Status           *string        `json:"status"` // optional: "pending" (default) or "special_approved"
	SessionVersions  map[string]int `json:"session_versions"`
}

func shouldValidateStaffSitInSessions(status absences.Status) bool {
	return status != absences.StatusSpecialApproved
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

		body.Wcode = normalizeWCode(body.Wcode)
		if body.Wcode == "" {
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
				return 0, nil, fmt.Errorf("bad status")
			}
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

		student, subjectID, course, err := s.resolveStaffAbsenceSelection(r.Context(), qtx, tx, body.Wcode, body.SubjectID, body.CourseID)
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
		sitInMethod, err := normalizeSubmissionSitInMethod(body.SitInMethod)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_method", "Invalid sit-in method")
			return 0, nil, err
		}
		sessionUUIDs, err := parseUUIDStrings(body.SitInSessionIDs)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_session_id", "Invalid sit-in session ID")
			return 0, nil, err
		}
		if len(sessionUUIDs) > 0 && sitInMethod.String != "physical" {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sessions", "Only physical sit-ins may select sessions")
			return 0, nil, fmt.Errorf("non-physical sit-in with sessions")
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
		if err := qtx.LockStudentForAbsenceSubmission(r.Context(), student.ID); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not lock student absence submission")
			return 0, nil, err
		}
		if err := ensureSitInSessionsAvailable(r.Context(), qtx, student.ID, sessionUUIDs); err != nil {
			if s.writeSitInSessionConflict(w, err) {
				return 0, nil, err
			}
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not check sit-in session availability")
			return 0, nil, err
		}
		var sitInCourseID pgtype.UUID
		if body.SitInCourseID != nil && strings.TrimSpace(*body.SitInCourseID) != "" {
			parsed, err := s.a.ParseUUID(strings.TrimSpace(*body.SitInCourseID))
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_course_id", "Invalid sit-in course")
				return 0, nil, err
			}
			sitInCourseID = parsed
		} else if sitInMethod.String == "physical" {
			sitInCourseID = course.CourseID
		}

		// ponytail: staff bypass the absence-record limit; add a policy config
		// when a non-admin staff-creation path is introduced.

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
		if err := setAbsenceMergeGroupForCourse(r.Context(), qtx, row.ID, course.CourseID); err != nil {
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
			if shouldValidateStaffSitInSessions(requestedStatus) {
				excludeFinal, err := satVerbalCourseFinalClassExcluded(r.Context(), qtx, course.CourseID)
				if err != nil {
					status, code, msg := s.a.ClassifyDBErr(err)
					s.a.WriteErr(w, status, code, msg)
					return 0, nil, err
				}
				count, err := qtx.ValidSitInSessionOverlap(r.Context(), row.ID, sessionUUIDs, s.deps.InstituteTZ, excludeFinal)
				if err != nil {
					status, code, msg := s.a.ClassifyDBErr(err)
					s.a.WriteErr(w, status, code, msg)
					return 0, nil, err
				}
				if count != len(sessionUUIDs) {
					s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Sit-in sessions must be active and must not overlap the missed class")
					return 0, nil, fmt.Errorf("invalid sit-in sessions")
				}
				if err := s.validateSitInCandidates(r.Context(), qtx, row.ID, sessionUUIDs); err != nil {
					s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", err.Error())
					return 0, nil, err
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
			count, err := qtx.ValidMissedSessionCount(r.Context(), row.ID, missedUUIDs, s.deps.InstituteTZ)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			if count != len(missedUUIDs) {
				s.a.WriteErr(w, http.StatusBadRequest, "invalid_missed_sessions", "Missed sessions must be in the selected class and absence dates")
				return 0, nil, fmt.Errorf("invalid missed sessions")
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
					return 0, nil, err
				}
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

		if requestedStatus == absences.StatusSpecialApproved {
			newVersion, err := qtx.AbsenceStatusUpdate(r.Context(), row.ID, string(absences.StatusSpecialApproved), actorID(user.ID), row.Version)
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not set special approved status")
				return 0, nil, err
			}
			if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{
				AbsenceID: row.ID,
				Action:    string(absences.StatusSpecialApproved),
				ActorID:   actorID(user.ID),
				ActorRole: "admin",
				Details:   map[string]any{"from": "pending", "to": "special_approved", "staff_created": true, "wcode": body.Wcode},
			}); err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write absence timeline")
				return 0, nil, err
			}
			row.Version = newVersion
		}

		managed, err := qtx.ManagedAbsenceGet(r.Context(), row.ID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}

		dto := s.managedAbsenceDTO(managed)
		sessions, err := qtx.ManagedAbsenceSessions(r.Context(), row.ID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		missed, err := qtx.ManagedAbsenceMissedSessions(r.Context(), row.ID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
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

		return http.StatusCreated, dto, nil
	}) {
		return
	}
	if createdID != "" {
		s.publishAbsenceChanges([]string{createdID})
	}
}

func (s *server) resolveStaffAbsenceSelection(ctx context.Context, q *sqldb.Queries, db publicRowExecutor, wcode string, subjectIDRaw, courseIDRaw *string) (sqldb.Student, pgtype.UUID, sqldb.StudentEnrolledCourseV2, error) {
	if courseIDRaw == nil || strings.TrimSpace(*courseIDRaw) == "" {
		return s.resolveAbsenceSelection(ctx, q, db, wcode, subjectIDRaw, courseIDRaw)
	}

	studentRow, err := q.StudentGetByWCode(ctx, wcode)
	if err != nil {
		return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, err
	}
	student := sqldb.Student{
		ID:        studentRow.ID,
		Wcode:     studentRow.Wcode,
		FullName:  studentRow.FullName,
		Notes:     studentRow.Notes,
		CreatedAt: studentRow.CreatedAt,
		UpdatedAt: studentRow.UpdatedAt,
	}

	var requestedSubjectID pgtype.UUID
	if subjectIDRaw != nil && strings.TrimSpace(*subjectIDRaw) != "" {
		requestedSubjectID, err = s.a.ParseUUID(strings.TrimSpace(*subjectIDRaw))
		if err != nil {
			return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, err
		}
	}

	selectedCourseID, err := s.a.ParseUUID(strings.TrimSpace(*courseIDRaw))
	if err != nil {
		return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, err
	}

	var course sqldb.StudentEnrolledCourseV2
	if err := db.QueryRow(ctx, `
		SELECT c.id, c.code, c.name, c.subject_id, c.cycle_id, COALESCE(mgg.level, c.level), c.root_course_group_id,
		       COALESCE(mgg.sit_in_rule_id, rcg.sit_in_rule_id), mgm.group_id
		FROM courses c
		LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
		LEFT JOIN course_merge_group_members mgm ON mgm.course_id = c.id
		LEFT JOIN course_merge_groups mgg ON mgg.id = mgm.group_id
		WHERE c.id = $1
	`, selectedCourseID).Scan(
		&course.CourseID,
		&course.CourseCode,
		&course.CourseName,
		&course.SubjectID,
		&course.CycleID,
		&course.Level,
		&course.RootCourseGroupID,
		&course.SitInRuleID,
		&course.MergeGroupID,
	); err != nil {
		return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, err
	}

	if requestedSubjectID.Valid && course.SubjectID.Valid && requestedSubjectID != course.SubjectID {
		return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, fmt.Errorf("course does not belong to selected subject")
	}
	if !course.SubjectID.Valid {
		return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, fmt.Errorf("selected course has no subject")
	}

	return student, course.SubjectID, course, nil
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

		sessions, _ := qtx.ManagedAbsenceSessions(r.Context(), id)
		missed, _ := qtx.ManagedAbsenceMissedSessions(r.Context(), id)
		smsItems := []successSMSItem{{row: managed, sessions: sessions, missed: missed}}
		smsTemplate := successSMSTemplateForItems(settings, managed.Status, smsItems)

		contactRows, contactErr := qtx.StudentSubjectByWCode(r.Context(), managed.Wcode)
		if contactErr != nil && s.deps.Log != nil {
			s.deps.Log.Error("failed to load contacts for sms/email", "wcode", managed.Wcode, "error", contactErr)
		}

		var phones []string
		if contactErr == nil && len(contactRows) > 0 {
			phones = successSMSPhones(contactRows[0].ParentPhone, contactRows[0].StudentPhone)
		}

		notificationID, queueErr := enqueueSuccessSMS(
			r.Context(), qtx, "absence_success_sms_manual", "manual-success-sms:"+id.String(),
			smsItems, phones, smsTemplate,
			s.deps.InstituteTZ, "absence-"+id.String(),
		)
		if queueErr != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not queue success SMS")
			return 0, nil, queueErr
		}
		queued := notificationID.Valid
		notificationIDString := ""
		if queued {
			notificationIDString, _ = sUUIDString(notificationID)
		}

		emailSent := false
		if s.deps.EmailService != nil {
			emailCfg := settings.emailSuccessConfig()
			if emailCfg.Enabled {
				emailSent = sendSuccessEmailWithConfig(r.Context(), s.deps.EmailService, s.deps.Log, managed, sessions, missed, emailCfg, s.deps.InstituteName, s.deps.InstituteTZ)
			} else if s.deps.Log != nil {
				s.deps.Log.Info("success email skipped: email_success_enabled is false", "absence_id", id)
			}
		} else if s.deps.Log != nil {
			s.deps.Log.Info("success email skipped: email service not configured", "absence_id", id)
		}

		statusCode := http.StatusOK
		if queued {
			statusCode = http.StatusAccepted
		}
		return statusCode, map[string]any{
			"sent":            queued || emailSent,
			"queued":          queued,
			"notification_id": notificationIDString,
			"sms_queued":      queued,
			"sms_sent":        false,
			"email_sent":      emailSent,
			"recipient_count": len(phones),
		}, nil
	}) {
		return
	}
}

func (s *server) handleBatchSendSuccessSMS(w http.ResponseWriter, r *http.Request) {
	_, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	var body struct {
		IDs    []string `json:"ids"`
		DryRun bool     `json:"dry_run"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid request body")
		return
	}
	if len(body.IDs) == 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "no_ids", "At least one absence ID is required")
		return
	}

	ids := make([]pgtype.UUID, 0, len(body.IDs))
	seen := map[string]bool{}
	for _, raw := range body.IDs {
		if seen[raw] {
			continue
		}
		seen[raw] = true
		id, err := s.a.ParseUUID(raw)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid absence ID: "+raw)
			return
		}
		ids = append(ids, id)
	}

	idempotencyKey := "batch-send-sms-" + strings.Join(body.IDs, ",")
	if !s.a.WithIdempotentTx(w, r, idempotency.SystemActorUUID, idempotencyKey, s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)

		settings, err := s.readAbsenceSettings(r)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}

		items := make([]successSMSItem, 0, len(ids))
		var wcode string
		for _, id := range ids {
			managed, err := qtx.ManagedAbsenceGet(r.Context(), id)
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			if wcode == "" {
				wcode = managed.Wcode
			} else if managed.Wcode != wcode {
				s.a.WriteErr(w, http.StatusBadRequest, "mixed_students", "All absences must belong to the same student")
				return 0, nil, fmt.Errorf("mixed students")
			}
			sess, _ := qtx.ManagedAbsenceSessions(r.Context(), id)
			mis, _ := qtx.ManagedAbsenceMissedSessions(r.Context(), id)
			items = append(items, successSMSItem{row: managed, sessions: sess, missed: mis})
		}

		hasSpecial := false
		hasNormal := false
		for _, item := range items {
			if absences.Status(item.row.Status) == absences.StatusSpecialApproved {
				hasSpecial = true
			} else {
				hasNormal = true
			}
		}
		if hasSpecial && hasNormal && strings.TrimSpace(settings.Notifications.SmsSpecialApprovedTemplate) != "" {
			s.a.WriteErr(w, http.StatusBadRequest, "mixed_status_sms_templates", "Send normal and special-approved SMS notifications separately")
			return 0, nil, fmt.Errorf("mixed status sms templates")
		}

		smsTemplate := settings.Notifications.SmsSuccessTemplate
		if hasSpecial && strings.TrimSpace(settings.Notifications.SmsSpecialApprovedTemplate) != "" {
			smsTemplate = settings.Notifications.SmsSpecialApprovedTemplate
		}
		if len(items) > 0 {
			smsTemplate = successSMSTemplateForItems(settings, items[0].row.Status, items)
		}

		contactRows, contactErr := qtx.StudentSubjectByWCode(r.Context(), wcode)
		if contactErr != nil && s.deps.Log != nil {
			s.deps.Log.Error("failed to load contacts for sms/email", "wcode", wcode, "error", contactErr)
		}

		var phones []string
		if contactErr == nil && len(contactRows) > 0 {
			phones = successSMSPhones(contactRows[0].ParentPhone, contactRows[0].StudentPhone)
		}

		if body.DryRun {
			loc, _ := time.LoadLocation(s.deps.InstituteTZ)
			if loc == nil {
				loc = time.UTC
			}
			rendered := ""
			if strings.TrimSpace(smsTemplate) != "" {
				rendered = renderBatchSuccessSMSTemplate(smsTemplate, items, loc)
			}
			return http.StatusOK, map[string]any{"preview": map[string]any{"phones": phones, "message": rendered}}, nil
		}

		firstID, _ := sUUIDString(ids[0])
		notificationID, queueErr := enqueueSuccessSMS(
			r.Context(), qtx, "absence_success_sms_manual", "manual-batch-success-sms:"+strings.Join(body.IDs, ","),
			items, phones, smsTemplate, s.deps.InstituteTZ, "absence-batch-"+firstID,
		)
		if queueErr != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not queue success SMS")
			return 0, nil, queueErr
		}
		queued := notificationID.Valid
		notificationIDString := ""
		if queued {
			notificationIDString, _ = sUUIDString(notificationID)
		}

		emailSent := false
		if s.deps.EmailService != nil {
			emailCfg := settings.emailSuccessConfig()
			if emailCfg.Enabled {
				emailSent = sendBatchSuccessEmailWithConfig(r.Context(), s.deps.EmailService, s.deps.Log, items, emailCfg, s.deps.InstituteName, s.deps.InstituteTZ)
			} else if s.deps.Log != nil {
				s.deps.Log.Info("success email skipped: email_success_enabled is false", "absence_count", len(ids))
			}
		} else if s.deps.Log != nil {
			s.deps.Log.Info("success email skipped: email service not configured", "absence_count", len(ids))
		}

		statusCode := http.StatusOK
		if queued {
			statusCode = http.StatusAccepted
		}
		return statusCode, map[string]any{
			"sent":            queued || emailSent,
			"queued":          queued,
			"notification_id": notificationIDString,
			"sms_queued":      queued,
			"sms_sent":        false,
			"email_sent":      emailSent,
			"recipient_count": len(phones),
			"absence_count":   len(ids),
		}, nil
	}) {
		return
	}
}

func successSMSTemplateForStatus(settings absenceSettings, status string) string {
	if absences.Status(status) == absences.StatusSpecialApproved && strings.TrimSpace(settings.Notifications.SmsSpecialApprovedTemplate) != "" {
		return settings.Notifications.SmsSpecialApprovedTemplate
	}
	return settings.Notifications.SmsSuccessTemplate
}
