package absenceshttp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/studentauth"
)

type batchAbsenceCreateItem struct {
	SubjectID        string         `json:"subject_id"`
	CourseID         string         `json:"course_id"`
	DateFrom         string         `json:"date_from"`
	DateTo           string         `json:"date_to"`
	SitInMethod      *string        `json:"sit_in_method"`
	SitInCourseID    *string        `json:"sit_in_course_id"`
	MissedSessionIDs []string       `json:"missed_session_ids"`
	SitInSessionIDs  []string       `json:"sit_in_session_ids"`
	SessionVersions  map[string]int `json:"session_versions"`
}

type batchAbsenceCreateRequest struct {
	Wcode string  `json:"wcode"`
	Email *string `json:"email"`
	// Nickname is optional and only fills a student record that has none.
	Nickname          *string                  `json:"nickname,omitempty"`
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

// batchNotificationData carries notification context out of the DB transaction
// so that external side-effects (SMS, email) can be sent after commit.
type batchNotificationData struct {
	smsRecipients []string
	smsTemplate   string
	created       []createdAbsenceRecord
	emailCfg      emailSuccessConfig
}

const batchNotificationTimeout = 60 * time.Second
const batchNotificationConcurrencyLimit = 8

func safeBatchNotificationLog(log *slog.Logger, level slog.Level, message string, args ...any) {
	if log == nil {
		return
	}
	defer func() { _ = recover() }()
	log.Log(context.Background(), level, message, args...)
}

func (s *server) batchNotificationSlots() chan struct{} {
	s.batchNotificationLimiterOnce.Do(func() {
		if s.batchNotificationLimiter == nil {
			s.batchNotificationLimiter = make(chan struct{}, batchNotificationConcurrencyLimit)
		}
	})
	return s.batchNotificationLimiter
}

// consumeAndRevokeStudentVerificationTx burns the parent OTP verification
// session and revokes the student's session inside the submission transaction.
// One verification therefore cannot drive an unlimited number of submissions:
// the next attempt needs a fresh OTP and re-verification.
func (s *server) consumeAndRevokeStudentVerificationTx(ctx context.Context, tx pgx.Tx, session studentauth.Session, absenceID uuid.UUID) error {
	if s.deps.OTP != nil && session.VerificationSessionID != uuid.Nil {
		if err := s.deps.OTP.ConsumeSessionByIDTx(ctx, tx, session.VerificationSessionID, absenceID); err != nil {
			return fmt.Errorf("consume verification session: %w", err)
		}
	}
	if session.ID != uuid.Nil {
		if _, err := tx.Exec(ctx, `
			UPDATE student_self_service_sessions
			SET revoked_at = now()
			WHERE id = $1 AND revoked_at IS NULL
		`, session.ID); err != nil {
			return fmt.Errorf("revoke student session: %w", err)
		}
	}
	return nil
}

func (s *server) handleAbsenceBatchCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requestOriginAllowed(w, r) {
		return
	}
	studentSession, ok := s.requireStudentSession(w, r)
	if !ok {
		return
	}
	createdIDs := []string{}
	var notifyData *batchNotificationData
	if !s.a.WithIdempotentTx(w, r, studentIdempotencyActor(studentSession.Wcode), "absences-public", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)

		var body batchAbsenceCreateRequest
		if err := s.a.DecodeJSON(w, r, &body); err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
			return 0, nil, err
		}
		if strings.TrimSpace(body.Wcode) != "" {
			s.a.WriteErr(w, http.StatusBadRequest, "identity_parameter_not_allowed", "wcode is derived from the verified student session")
			return 0, nil, fmt.Errorf("client supplied wcode")
		}
		if body.VerificationToken != nil && strings.TrimSpace(*body.VerificationToken) != "" {
			s.a.WriteErr(w, http.StatusBadRequest, "verification_token_not_allowed", "verification_token is no longer accepted")
			return 0, nil, fmt.Errorf("client supplied verification token")
		}
		body.Wcode = normalizeWCode(studentSession.Wcode)
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
		// Same policy for the nickname: a form-provided value may only fill
		// an empty record, and the resolved value rides along as the
		// student_nickname snapshot for staff views.
		if resolvedNickname, shouldPersist, nicknameErr := absences.ResolveClientNickname(body.Nickname, studentNickname); nicknameErr != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_nickname", "A nickname is already saved for this student")
			return 0, nil, nicknameErr
		} else if shouldPersist {
			studentNickname = resolvedNickname
			if err := qtx.StudentSetNicknameIfEmpty(r.Context(), body.Wcode, resolvedNickname.String); err != nil && s.deps.Log != nil {
				s.deps.Log.Error("failed to persist student nickname", "wcode", body.Wcode, "error", err)
			}
		}

		created := make([]createdAbsenceRecord, 0, len(body.Items))
		for _, item := range body.Items {
			record, ok := s.createAbsenceRecordTx(w, r, qtx, tx, settings, body.Wcode, reasonCategory, reason, studentEmail, studentNickname, studentPhone, item)
			if !ok {
				return 0, nil, fmt.Errorf("failed to create absence item")
			}
			created = append(created, record)
		}

		// Burn the verification: one OTP + session per submission.
		if len(created) > 0 && created[0].row.ID.Valid {
			firstAbsenceID, err := uuid.FromBytes(created[0].row.ID.Bytes[:])
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return 0, nil, err
			}
			if err := s.consumeAndRevokeStudentVerificationTx(r.Context(), tx, studentSession, firstAbsenceID); err != nil {
				if s.deps.Log != nil {
					s.deps.Log.Error("failed to burn student verification on submit", "wcode", body.Wcode, "error", err)
				}
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
				return 0, nil, err
			}
		}

		// Collect notification data for post-commit delivery.
		// SMS and email are external side-effects that must not hold the DB transaction open.
		if len(created) > 0 && (settings.Notifications.SmsSuccessTemplate != "" && len(successSMSRecipients) > 0) || (s.deps.EmailService != nil && settings.emailSuccessConfig().Enabled) {
			notifyData = &batchNotificationData{
				smsRecipients: successSMSRecipients,
				smsTemplate:   settings.Notifications.SmsSuccessTemplate,
				created:       created,
				emailCfg:      settings.emailSuccessConfig(),
			}
		}

		out := make([]managedAbsenceDTO, 0, len(created))
		for _, record := range created {
			dto := s.managedAbsenceDTO(record.row)
			dto.Status = "pending"
			dto.MissedSessions = s.sessionDTO(record.missed)
			dto.SitIns = s.sessionDTO(record.sessions)
			out = append(out, dto)
			createdIDs = append(createdIDs, dto.ID)
		}

		return http.StatusCreated, batchAbsenceCreateResponse{Items: out}, nil
	}) {
		return
	}

	s.publishAbsenceChanges(createdIDs)
	// Schedule notifications AFTER the idempotent transaction has committed.
	// Delivery is detached so external APIs cannot delay the HTTP response.
	s.sendBatchNotifications(notifyData)
}

// sendBatchNotifications starts bounded best-effort notification delivery and
// returns immediately. Delivery is independent of request cancellation.
func (s *server) sendBatchNotifications(data *batchNotificationData) {
	if data == nil {
		return
	}
	limiter := s.batchNotificationSlots()
	select {
	case limiter <- struct{}{}:
	default:
		safeBatchNotificationLog(s.deps.Log, slog.LevelWarn, "batch notifications dropped",
			"reason", "saturated",
			"capacity", cap(limiter),
			"absence_count", len(data.created),
		)
		return
	}

	go func() {
		defer func() { <-limiter }()
		defer func() {
			if recovered := recover(); recovered != nil {
				safeBatchNotificationLog(s.deps.Log, slog.LevelError, "batch notifications panicked", "absence_count", len(data.created))
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), batchNotificationTimeout)
		defer cancel()

		started := time.Now()
		safeBatchNotificationLog(s.deps.Log, slog.LevelInfo, "batch notifications started",
			"absence_count", len(data.created),
			"timeout_seconds", int(batchNotificationTimeout/time.Second),
		)

		s.deliverBatchNotifications(ctx, data)
		if ctx.Err() != nil {
			safeBatchNotificationLog(s.deps.Log, slog.LevelWarn, "batch notifications timed out",
				"absence_count", len(data.created),
				"duration_ms", time.Since(started).Milliseconds(),
			)
			return
		}
		safeBatchNotificationLog(s.deps.Log, slog.LevelInfo, "batch notifications completed",
			"absence_count", len(data.created),
			"duration_ms", time.Since(started).Milliseconds(),
		)
	}()
}

// deliverBatchNotifications starts enabled channels independently and waits
// for them within the detached dispatcher, bounded by ctx.
func (s *server) deliverBatchNotifications(ctx context.Context, data *batchNotificationData) {
	if data == nil {
		return
	}

	items := make([]successSMSItem, 0, len(data.created))
	for _, record := range data.created {
		items = append(items, successSMSItem{row: record.row, sessions: record.sessions, missed: record.missed})
	}

	type channelResult struct{}
	results := make(chan channelResult, 2)
	launched := 0
	launch := func(channel string, deliver func()) {
		launched++
		go func() {
			defer func() {
				recovered := recover()
				results <- channelResult{}
				if recovered != nil {
					safeBatchNotificationLog(s.deps.Log, slog.LevelError, "batch notification channel panicked", "channel", channel)
				}
			}()
			deliver()
		}()
	}

	if data.smsTemplate != "" && len(data.smsRecipients) > 0 {
		launch("sms", func() {
			sendBatchSuccessSMS(ctx, s.deps.SMS, s.deps.Log, data.smsTemplate, items, data.smsRecipients, s.deps.InstituteTZ)
		})
	}

	if s.deps.EmailService != nil && data.emailCfg.Enabled && len(data.created) > 0 {
		launch("email", func() {
			sendBatchSuccessEmailWithConfig(ctx, s.deps.EmailService, s.deps.Log, items, data.emailCfg, s.deps.InstituteName, s.deps.InstituteTZ)
		})
	}

	for range launched {
		select {
		case <-results:
		case <-ctx.Done():
			return
		}
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
	// Same hard gate as the single-create path: batch submission is a
	// student endpoint, so hidden courses are rejected here too.
	if !course.AbsenceFormVisible {
		s.a.WriteErr(w, http.StatusForbidden, "course_not_available", "This class is not available in the absence form")
		return createdAbsenceRecord{}, false
	}
	if len(item.MissedSessionIDs) == 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_missed_session_id", "At least one missed session must be selected")
		return createdAbsenceRecord{}, false
	}
	missedUUIDs, err := parseUUIDStrings(item.MissedSessionIDs)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_missed_session_id", "Invalid missed session ID")
		return createdAbsenceRecord{}, false
	}
	limitStats, candidateAbsenceDays, err := projectedAbsenceDayStats(
		r.Context(), qtx, wcode, course.CourseID, missedUUIDs, dateFrom, dateTo, s.deps.InstituteTZ,
	)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error checking absence days")
		return createdAbsenceRecord{}, false
	}
	if candidateAbsenceDays > int32(settings.SitIn.MaxSessionsPerAbsence) {
		s.a.WriteErr(w, http.StatusBadRequest, "too_many_missed_sessions", "Selected absence days exceed the configured maximum")
		return createdAbsenceRecord{}, false
	}
	if limitStats.ProjectedLimitExceeded {
		s.a.WriteErr(w, http.StatusForbidden, "absence_limit_exceeded", "You have reached the maximum number of absence days allowed for this course")
		return createdAbsenceRecord{}, false
	}

	var sitInCourseID pgtype.UUID
	if item.SitInCourseID != nil && strings.TrimSpace(*item.SitInCourseID) != "" {
		parsed, err := s.a.ParseUUID(strings.TrimSpace(*item.SitInCourseID))
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_course_id", "Invalid sit-in course")
			return createdAbsenceRecord{}, false
		}
		// A student may only sit in to courses of the subject they are
		// submitting for; anything else is outside the resolved selection.
		sitInCourse, err := qtx.CourseGetFull(r.Context(), parsed)
		if err != nil || !sitInCourse.SubjectID.Valid || sitInCourse.SubjectID != subjectID {
			s.a.WriteErr(w, http.StatusBadRequest, "sit_in_course_outside_selection", "Sit-in course must belong to the selected subject")
			return createdAbsenceRecord{}, false
		}
		sitInCourseID = parsed
	} else if sitInMethod.String == "physical" {
		sitInCourseID = course.CourseID
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
	if err := setAbsenceMergeGroupForCourse(r.Context(), qtx, row.ID, course.CourseID); err != nil {
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
		excludeFinal, err := satVerbalCourseFinalClassExcluded(r.Context(), qtx, course.CourseID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return createdAbsenceRecord{}, false
		}
		count, err := qtx.ValidSitInSessionOverlap(r.Context(), row.ID, sessionUUIDs, s.deps.InstituteTZ, excludeFinal)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return createdAbsenceRecord{}, false
		}
		if count != len(sessionUUIDs) {
			s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Sit-in sessions must be in the selected course and must not overlap the missed class")
			return createdAbsenceRecord{}, false
		}
		if err := s.validateSitInCandidates(r.Context(), qtx, row.ID, sessionUUIDs); err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", err.Error())
			return createdAbsenceRecord{}, false
		}
		sitInInputs := make([]sqldb.SitInSnapshotInput, 0, len(sessionUUIDs))
		for _, sid := range sessionUUIDs {
			input := sqldb.SitInSnapshotInput{SessionID: sid}
			if item.SessionVersions != nil {
				if v, ok := item.SessionVersions[sid.String()]; ok {
					version := int32(v)
					input.ExpectedVersion = &version
				}
			}
			sitInInputs = append(sitInInputs, input)
		}
		if err := qtx.AbsenceSitInsCreateWithSnapshot(r.Context(), row.ID, sitInInputs, s.deps.InstituteTZ, BuildSnapshotFromSessionRow); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return createdAbsenceRecord{}, false
		}
	}

	if len(item.MissedSessionIDs) > 0 {
		count, err := qtx.ValidMissedSessionCount(r.Context(), row.ID, missedUUIDs, s.deps.InstituteTZ)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return createdAbsenceRecord{}, false
		}
		if count != len(missedUUIDs) {
			s.a.WriteErr(w, http.StatusBadRequest, "invalid_missed_sessions", "Missed sessions must be in the selected class and absence dates")
			return createdAbsenceRecord{}, false
		}
		timingRows, err := qtx.ValidMissedSessionTiming(r.Context(), row.ID, missedUUIDs, s.deps.InstituteTZ)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return createdAbsenceRecord{}, false
		}
		if timingErr := validateSessionTiming(settings.Form, time.Now(), sessionTimingInfos(timingRows)); timingErr != nil {
			s.a.WriteErr(w, http.StatusBadRequest, timingErr.code, timingErr.message)
			return createdAbsenceRecord{}, false
		}
		snapshotInputs := make([]sqldb.MissedSessionSnapshotInput, 0, len(missedUUIDs))
		for _, sid := range missedUUIDs {
			input := sqldb.MissedSessionSnapshotInput{SessionID: sid}
			if item.SessionVersions != nil {
				if v, ok := item.SessionVersions[sid.String()]; ok {
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
				return createdAbsenceRecord{}, false
			}
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
