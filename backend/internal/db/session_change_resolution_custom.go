package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (q *Queries) ResolveScheduleIssue(ctx context.Context, issueID, candidateSessionID, actorID pgtype.UUID, expectedIssueVersion, expectedSessionVersion int32, action, reason string) (string, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "keep", "reassign", "cancel", "dismiss", "mark_for_review":
	default:
		return "", fmt.Errorf("unsupported resolution action %q", action)
	}
	if expectedIssueVersion < 1 {
		return "", fmt.Errorf("expected_issue_version is required")
	}

	var absenceID, sitInSessionID, latestChangeID pgtype.UUID
	var status string
	var issueVersion int32
	var studentName, studentEmail, studentPhone pgtype.Text
	if err := q.db.QueryRow(ctx, `
		SELECT i.absence_id, i.sit_in_session_id, i.latest_session_change_id, i.status, i.issue_version,
		       sa.student_name, sa.student_email, sa.student_phone
		FROM absence_schedule_issues i
		JOIN student_absences sa ON sa.id = i.absence_id
		WHERE i.id = $1
		FOR UPDATE
	`, issueID).Scan(&absenceID, &sitInSessionID, &latestChangeID, &status, &issueVersion, &studentName, &studentEmail, &studentPhone); err != nil {
		return "", err
	}
	if !scheduleIssueIsResolvable(status) {
		return "", fmt.Errorf("schedule issue is already %s", status)
	}
	if issueVersion != expectedIssueVersion {
		return "", fmt.Errorf("schedule issue changed while you were reviewing it")
	}
	if action == "dismiss" && strings.TrimSpace(reason) == "" {
		return "", fmt.Errorf("a dismissal reason is required")
	}
	if action == "mark_for_review" {
		if status != "open" {
			return "", fmt.Errorf("schedule issue is already awaiting review")
		}
		if strings.TrimSpace(reason) == "" {
			return "", fmt.Errorf("a review reason is required")
		}
		commandTag, err := q.db.Exec(ctx, `
			UPDATE absence_schedule_issues
			SET status = 'needs_review', assigned_to = $3, review_reason = $4,
			    review_note = NULL, resolved_at = NULL, resolved_by = NULL,
			    resolution_action = 'mark_for_review', issue_version = issue_version + 1,
			    updated_at = now()
			WHERE id = $1 AND status = 'open' AND issue_version = $2
		`, issueID, expectedIssueVersion, actorID, strings.TrimSpace(reason))
		if err != nil {
			return "", err
		}
		if commandTag.RowsAffected() == 0 {
			return "", pgx.ErrNoRows
		}
		return "not_required", nil
	}

	var assignmentID, previousSessionID pgtype.UUID
	var previousSessionVersion int32
	if sitInSessionID.Valid {
		if err := q.db.QueryRow(ctx, `
			SELECT asi.id, asi.session_id, s.version
			FROM absence_sit_ins asi
			JOIN sessions s ON s.id = asi.session_id
			WHERE absence_id = $1 AND session_id = $2
			FOR UPDATE
		`, absenceID, sitInSessionID).Scan(&assignmentID, &previousSessionID, &previousSessionVersion); err != nil && err != pgx.ErrNoRows {
			return "", err
		}
	}

	if action == "reassign" {
		if !assignmentID.Valid {
			return "", fmt.Errorf("schedule issue has no active sit-in assignment")
		}
		if !candidateSessionID.Valid || expectedSessionVersion < 1 {
			return "", fmt.Errorf("candidate session and expected_session_version are required")
		}
		candidateVersion, validationErr := q.validateResolutionCandidate(ctx, resolutionCandidateInput{
			AbsenceID:              absenceID,
			AssignmentID:           assignmentID,
			CurrentSessionID:       previousSessionID,
			CandidateSessionID:     candidateSessionID,
			ExpectedSessionVersion: expectedSessionVersion,
		})
		if validationErr != nil {
			return "", validationErr
		}

		if _, err := q.db.Exec(ctx, `DELETE FROM absence_sit_ins WHERE id = $1`, assignmentID); err != nil {
			return "", err
		}
		if err := q.db.QueryRow(ctx, `
			INSERT INTO absence_sit_ins (
				absence_id, session_id, session_version_at_assignment, assigned_at,
				assigned_by, assignment_source
			)
			VALUES ($1, $2, $3, now(), $4, 'impact_resolution')
			RETURNING id
		`, absenceID, candidateSessionID, candidateVersion, actorID).Scan(&assignmentID); err != nil {
			return "", err
		}
		if _, err := q.db.Exec(ctx, `
			INSERT INTO absence_sit_in_assignment_events (
				absence_id, previous_session_id, new_session_id, action, reason,
				session_change_id, actor_id
			)
			VALUES ($1, $2, $3, 'reassigned', $4, $5, $6)
		`, absenceID, previousSessionID, candidateSessionID, strings.TrimSpace(reason), latestChangeID, actorID); err != nil {
			return "", err
		}
	} else if action == "cancel" && assignmentID.Valid {
		if _, err := q.db.Exec(ctx, `DELETE FROM absence_sit_ins WHERE id = $1`, assignmentID); err != nil {
			return "", err
		}
		if _, err := q.db.Exec(ctx, `
			INSERT INTO absence_sit_in_assignment_events (
				absence_id, previous_session_id, action, reason, session_change_id, actor_id
			)
			VALUES ($1, $2, 'cancelled', $3, $4, $5)
			`, absenceID, previousSessionID, strings.TrimSpace(reason), latestChangeID, actorID); err != nil {
			return "", err
		}
	}
	notificationStatus := "not_required"
	shouldNotify := action == "reassign" || ((action == "keep" || action == "cancel") && assignmentID.Valid)
	if shouldNotify {
		settings, settingsErr := q.AppSettingsGetSessionChangeSettings(ctx)
		if settingsErr != nil {
			return "", settingsErr
		}
		smsReady := settings.SmsEnabled && strings.TrimSpace(settings.SmsTemplate) != ""
		emailReady := settings.EmailEnabled && strings.TrimSpace(settings.EmailSubject) != "" && strings.TrimSpace(settings.EmailBody) != ""
		if !smsReady && !emailReady {
			notificationStatus = "not_configured"
		} else {
			notificationStatus = "no_recipient"
		}
		payload, payloadErr := json.Marshal(map[string]string{
			"absence_id":    absenceID.String(),
			"action":        action,
			"student":       studentName.String,
			"sms_template":  settings.SmsTemplate,
			"email_subject": settings.EmailSubject,
			"email_body":    settings.EmailBody,
		})
		if payloadErr != nil {
			return "", payloadErr
		}
		candidateVersion := previousSessionVersion
		if action == "reassign" {
			candidateVersion = expectedSessionVersion
		}
		messageType := "sit_in_" + action
		if action == "keep" {
			messageType = "sit_in_session_moved"
		}
		if smsReady && studentPhone.Valid && strings.TrimSpace(studentPhone.String) != "" {
			if err := q.NotificationOutboxInsert(ctx, NotificationOutboxInsertParams{
				AbsenceID: absenceID, AssignmentID: assignmentID, SessionVersion: candidateVersion,
				MessageType: messageType, Recipient: studentPhone.String, Channel: "sms",
				Payload: string(payload), IdempotencyKey: notificationKey(issueID, action, "sms"),
			}); err != nil {
				return "", err
			}
			notificationStatus = "queued"
		}
		if emailReady && studentEmail.Valid && strings.TrimSpace(studentEmail.String) != "" {
			if err := q.NotificationOutboxInsert(ctx, NotificationOutboxInsertParams{
				AbsenceID: absenceID, AssignmentID: assignmentID, SessionVersion: candidateVersion,
				MessageType: messageType, Recipient: studentEmail.String, Channel: "email",
				Payload: string(payload), IdempotencyKey: notificationKey(issueID, action, "email"),
			}); err != nil {
				return "", err
			}
			notificationStatus = "queued"
		}
	}

	resolutionStatus := "resolved"
	if action == "dismiss" {
		resolutionStatus = "dismissed"
	}
	resolutionAction := action
	if strings.TrimSpace(reason) != "" {
		resolutionAction += ":" + strings.TrimSpace(reason)
	}
	commandTag, err := q.db.Exec(ctx, `
		UPDATE absence_schedule_issues
		SET status = $2,
		    resolved_at = CASE WHEN $2 = 'open' THEN NULL ELSE now() END,
		    resolved_by = $3,
		    resolution_action = $4,
		    issue_version = issue_version + 1,
		    updated_at = now()
	WHERE id = $1 AND status IN ('open', 'needs_review') AND issue_version = $5
	`, issueID, resolutionStatus, actorID, resolutionAction, expectedIssueVersion)
	if err != nil {
		return "", err
	}
	if commandTag.RowsAffected() == 0 {
		return "", pgx.ErrNoRows
	}
	return notificationStatus, nil
}

func scheduleIssueIsResolvable(status string) bool {
	return status == "open" || status == "needs_review"
}

func notificationKey(issueID pgtype.UUID, action, channel string) string {
	return fmt.Sprintf("%x:%s:%s", issueID.Bytes, action, channel)
}
