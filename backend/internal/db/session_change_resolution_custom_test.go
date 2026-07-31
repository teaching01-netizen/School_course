package db

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestScheduleIssueIsResolvable_allowsOpenAndNeedsReview(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "open", status: "open", want: true},
		{name: "needs review", status: "needs_review", want: true},
		{name: "resolved", status: "resolved", want: false},
		{name: "superseded", status: "superseded", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			got := scheduleIssueIsResolvable(test.status)

			// Then
			if got != test.want {
				t.Fatalf("scheduleIssueIsResolvable(%q) = %t, want %t", test.status, got, test.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Notification contract tests (NOTIF-xxx)
//
// These are DB integration tests. Set TEST_DATABASE_URL to run them; without
// it they skip. They cover the confirmed notification contract:
//   - keep/reassign/cancel queue student notifications (when configured and the
//     student has contact information)
//   - mark_for_review/dismiss never notify
//   - resolution + notification queueing is transactional
// ---------------------------------------------------------------------------

type notifFixture struct {
	absenceID        pgtype.UUID
	sessionID        pgtype.UUID
	sessionVersion   int32
	candidateID      pgtype.UUID
	candidateVersion int32
	changeID         pgtype.UUID
	issueID          pgtype.UUID
	issueVersion     int32
	assignmentID     pgtype.UUID
	actorID          pgtype.UUID
	studentEmail     string
	studentPhone     string
}

// newNotificationFixture builds an open schedule issue with an active sit-in
// assignment, a replacement candidate session, and a session change row.
// email/phone may be empty to model a student without contact information.
func newNotificationFixture(t *testing.T, q *Queries, pool *pgxpool.Pool, email, phone string) notifFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	base := newImpactFixture(t, q)
	suffix := base.absenceID.String()

	candidateRoom, err := q.RoomCreate(ctx, RoomCreateParams{Name: "notif-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := q.SessionCreate(ctx, SessionCreateParams{
		CourseID:  base.courseID,
		RoomID:    candidateRoom.ID,
		TeacherID: base.teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2030, 1, 2, 10, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Staff fill in contact details when they record the absence.
	if _, err := pool.Exec(ctx, `
		UPDATE student_absences
		SET student_name = 'Notification Student', student_email = NULLIF($2, ''), student_phone = NULLIF($3, '')
		WHERE id = $1
	`, base.absenceID, email, phone); err != nil {
		t.Fatal(err)
	}

	var assignmentID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO absence_sit_ins (absence_id, session_id)
		VALUES ($1, $2)
		RETURNING id
	`, base.absenceID, base.sessionID).Scan(&assignmentID); err != nil {
		t.Fatal(err)
	}

	var changeID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO session_changes (
			session_id, session_version, changed_fields, before_snapshot, after_snapshot,
			old_start_at, old_end_at, new_start_at, new_end_at,
			old_course_id, new_course_id, old_teacher_id, new_teacher_id
		)
		SELECT id, version + 1, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
		       start_at, end_at, start_at, end_at, course_id, course_id, teacher_id, teacher_id
		FROM sessions WHERE id = $1
		RETURNING id
	`, base.sessionID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}

	var issueID pgtype.UUID
	var issueVersion int32
	if err := pool.QueryRow(ctx, `
		INSERT INTO absence_schedule_issues (
			absence_id, issue_type, severity, status, source_session_id, sit_in_session_id,
			first_session_change_id, latest_session_change_id, fingerprint
		)
		VALUES ($1, 'sit_in_session_changed', 'critical', 'open', $2, $2, $3, $3, $4)
		RETURNING id, issue_version
	`, base.absenceID, base.sessionID, changeID, "notif-"+suffix).Scan(&issueID, &issueVersion); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		// Keep the notification_outbox queue clean: rows must not linger for
		// the (global FIFO) delivery claim tests to pick up.
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM notification_outbox WHERE absence_id = $1`, base.absenceID); err != nil {
			t.Logf("cleanup outbox for absence %s: %v", base.absenceID.String(), err)
		}
		// Absence rows are best-effort: once a resolution writes assignment
		// events, the append-only trigger on absence_sit_in_assignment_events
		// blocks the cascade delete, and the history stays by design.
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, base.absenceID); err != nil {
			t.Logf("cleanup absence %s: %v", base.absenceID.String(), err)
		}
	})

	return notifFixture{
		absenceID:        base.absenceID,
		sessionID:        base.sessionID,
		sessionVersion:   base.version,
		candidateID:      candidate.ID,
		candidateVersion: candidate.Version,
		changeID:         changeID,
		issueID:          issueID,
		issueVersion:     issueVersion,
		assignmentID:     assignmentID,
		actorID:          base.teacherID,
		studentEmail:     email,
		studentPhone:     phone,
	}
}

type channelSettings struct {
	smsEnabled   bool
	smsTemplate  string
	emailEnabled bool
	emailSubject string
	emailBody    string
}

// withChannelSettings replaces the global sit-in change notification settings
// for the duration of the test and restores them afterwards.
func withChannelSettings(t *testing.T, pool *pgxpool.Pool, settings channelSettings) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var previous channelSettings
	if err := pool.QueryRow(ctx, `
		SELECT sit_in_change_sms_enabled, sit_in_change_sms_template,
		       sit_in_change_email_enabled, sit_in_change_email_subject, sit_in_change_email_body
		FROM app_settings WHERE id = true
	`).Scan(&previous.smsEnabled, &previous.smsTemplate, &previous.emailEnabled, &previous.emailSubject, &previous.emailBody); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE app_settings
		SET sit_in_change_sms_enabled = $1, sit_in_change_sms_template = $2,
		    sit_in_change_email_enabled = $3, sit_in_change_email_subject = $4,
		    sit_in_change_email_body = $5
		WHERE id = true
	`, settings.smsEnabled, settings.smsTemplate, settings.emailEnabled, settings.emailSubject, settings.emailBody); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `
			UPDATE app_settings
			SET sit_in_change_sms_enabled = $1, sit_in_change_sms_template = $2,
			    sit_in_change_email_enabled = $3, sit_in_change_email_subject = $4,
			    sit_in_change_email_body = $5
			WHERE id = true
		`, previous.smsEnabled, previous.smsTemplate, previous.emailEnabled, previous.emailSubject, previous.emailBody); err != nil {
			t.Logf("restore channel settings: %v", err)
		}
	})
}

// resolveIssueInTx mirrors the HTTP handler: the resolution runs inside a
// transaction and commits only when it fully succeeds.
func resolveIssueInTx(t *testing.T, pool *pgxpool.Pool, q *Queries, f notifFixture, action string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	candidateID := pgtype.UUID{}
	expectedSessionVersion := int32(0)
	if action == "reassign" {
		candidateID = f.candidateID
		expectedSessionVersion = f.candidateVersion
	}
	status, err := q.WithTx(tx).ResolveScheduleIssueWithSnapshot(ctx, f.issueID, candidateID, f.actorID, f.issueVersion, expectedSessionVersion, action, "", "Asia/Bangkok", DefaultSnapshotBuilder)
	if err != nil {
		return status, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return status, nil
}

type outboxRow struct {
	Channel        string
	Status         string
	MessageType    string
	Recipient      string
	SessionVersion int32
	AssignmentID   pgtype.UUID
	Payload        map[string]string
}

func loadOutboxRows(t *testing.T, pool *pgxpool.Pool, absenceID pgtype.UUID) []outboxRow {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT channel, status, message_type, recipient, session_version, assignment_id, payload
		FROM notification_outbox
		WHERE absence_id = $1
		ORDER BY channel
	`, absenceID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	result := make([]outboxRow, 0)
	for rows.Next() {
		var row outboxRow
		var rawPayload []byte
		if err := rows.Scan(&row.Channel, &row.Status, &row.MessageType, &row.Recipient, &row.SessionVersion, &row.AssignmentID, &rawPayload); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(rawPayload, &row.Payload); err != nil {
			t.Fatal(err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func issueStatus(t *testing.T, pool *pgxpool.Pool, issueID pgtype.UUID) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM absence_schedule_issues WHERE id = $1`, issueID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func assignmentCount(t *testing.T, pool *pgxpool.Pool, absenceID pgtype.UUID) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM absence_sit_ins WHERE absence_id = $1`, absenceID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// NOTIF-001: keep and notify; SMS enabled; valid template and student phone.
func TestResolveKeepNotify_queuesSingleSMS(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		smsEnabled: true, smsTemplate: "Sit-in moved: {{student_name}}",
	})
	f := newNotificationFixture(t, q, pool, "", "+66812345678")

	status, err := resolveIssueInTx(t, pool, q, f, "keep")
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("notification status = %q, want queued", status)
	}
	if got := issueStatus(t, pool, f.issueID); got != "resolved" {
		t.Fatalf("issue status = %q, want resolved", got)
	}
	if got := assignmentCount(t, pool, f.absenceID); got != 1 {
		t.Fatalf("sit-in assignment count = %d, want unchanged (1)", got)
	}

	rows := loadOutboxRows(t, pool, f.absenceID)
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want exactly 1", len(rows))
	}
	row := rows[0]
	if row.Channel != "sms" {
		t.Errorf("channel = %q, want sms", row.Channel)
	}
	if row.Status != "queued" {
		t.Errorf("status = %q, want queued", row.Status)
	}
	if row.Recipient != f.studentPhone {
		t.Errorf("recipient = %q, want student phone %q", row.Recipient, f.studentPhone)
	}
	if row.MessageType != "sit_in_session_moved" {
		t.Errorf("message type = %q, want sit_in_session_moved", row.MessageType)
	}
	if row.Payload["action"] != "keep" {
		t.Errorf("payload action = %q, want keep", row.Payload["action"])
	}
}

// NOTIF-002: keep and notify; email enabled; valid template and student email.
func TestResolveKeepNotify_queuesSingleEmail(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		emailEnabled: true, emailSubject: "Sit-in moved", emailBody: "Hello {{student_name}}",
	})
	f := newNotificationFixture(t, q, pool, "student@example.edu", "")

	status, err := resolveIssueInTx(t, pool, q, f, "keep")
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("notification status = %q, want queued", status)
	}

	rows := loadOutboxRows(t, pool, f.absenceID)
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want exactly 1", len(rows))
	}
	row := rows[0]
	if row.Channel != "email" {
		t.Errorf("channel = %q, want email", row.Channel)
	}
	if row.Status != "queued" {
		t.Errorf("status = %q, want queued", row.Status)
	}
	if row.Recipient != f.studentEmail {
		t.Errorf("recipient = %q, want student email %q", row.Recipient, f.studentEmail)
	}
}

// NOTIF-003: both channels enabled and configured.
func TestResolveKeepNotify_queuesSMSAndEmail(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		smsEnabled:   true,
		smsTemplate:  "Sit-in moved: {{student_name}}",
		emailEnabled: true, emailSubject: "Sit-in moved", emailBody: "Hello {{student_name}}",
	})
	f := newNotificationFixture(t, q, pool, "student@example.edu", "+66812345678")

	status, err := resolveIssueInTx(t, pool, q, f, "keep")
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("notification status = %q, want queued", status)
	}

	rows := loadOutboxRows(t, pool, f.absenceID)
	if len(rows) != 2 {
		t.Fatalf("outbox rows = %d, want exactly 2 (one SMS and one email)", len(rows))
	}
	channels := map[string]string{}
	for _, row := range rows {
		if row.Status != "queued" {
			t.Errorf("%s status = %q, want queued", row.Channel, row.Status)
		}
		channels[row.Channel] = row.Recipient
	}
	if channels["sms"] != f.studentPhone {
		t.Errorf("sms recipient = %q, want %q", channels["sms"], f.studentPhone)
	}
	if channels["email"] != f.studentEmail {
		t.Errorf("email recipient = %q, want %q", channels["email"], f.studentEmail)
	}
}

// NOTIF-004: reassign removes the old assignment, creates the new one, writes
// an audit event, and queues a notification carrying the new session.
func TestResolveReassign_replacesAssignmentAndNotifies(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		smsEnabled: true, smsTemplate: "Sit-in moved: {{student_name}}",
	})
	f := newNotificationFixture(t, q, pool, "", "+66812345678")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	status, err := resolveIssueInTx(t, pool, q, f, "reassign")
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("notification status = %q, want queued", status)
	}
	if got := issueStatus(t, pool, f.issueID); got != "resolved" {
		t.Fatalf("issue status = %q, want resolved", got)
	}

	// Old assignment removed, exactly one new assignment on the candidate session.
	var newAssignmentID pgtype.UUID
	var newSessionID pgtype.UUID
	var source string
	if err := pool.QueryRow(ctx, `
		SELECT id, session_id, assignment_source FROM absence_sit_ins WHERE absence_id = $1
	`, f.absenceID).Scan(&newAssignmentID, &newSessionID, &source); err != nil {
		t.Fatal(err)
	}
	if newSessionID != f.candidateID {
		t.Errorf("new assignment session = %s, want candidate %s", newSessionID.String(), f.candidateID.String())
	}
	if newAssignmentID == f.assignmentID {
		t.Errorf("assignment id was reused; want a fresh assignment row")
	}
	if source != "impact_resolution" {
		t.Errorf("assignment source = %q, want impact_resolution", source)
	}

	// Audit event written.
	var eventAction string
	var previousSessionID, newEventSessionID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		SELECT action, previous_session_id, new_session_id
		FROM absence_sit_in_assignment_events
		WHERE absence_id = $1 ORDER BY created_at DESC LIMIT 1
	`, f.absenceID).Scan(&eventAction, &previousSessionID, &newEventSessionID); err != nil {
		t.Fatal(err)
	}
	if eventAction != "reassigned" {
		t.Errorf("event action = %q, want reassigned", eventAction)
	}
	if previousSessionID != f.sessionID {
		t.Errorf("event previous session = %s, want %s", previousSessionID.String(), f.sessionID.String())
	}
	if newEventSessionID != f.candidateID {
		t.Errorf("event new session = %s, want %s", newEventSessionID.String(), f.candidateID.String())
	}

	// Notification references the new assignment and its session version.
	rows := loadOutboxRows(t, pool, f.absenceID)
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want exactly 1", len(rows))
	}
	row := rows[0]
	if row.MessageType != "sit_in_reassign" {
		t.Errorf("message type = %q, want sit_in_reassign", row.MessageType)
	}
	if row.AssignmentID != newAssignmentID {
		t.Errorf("notification assignment = %s, want new assignment %s", row.AssignmentID.String(), newAssignmentID.String())
	}
	if row.SessionVersion != f.candidateVersion {
		t.Errorf("notification session_version = %d, want candidate version %d", row.SessionVersion, f.candidateVersion)
	}
	if row.Payload["action"] != "reassign" {
		t.Errorf("payload action = %q, want reassign", row.Payload["action"])
	}
}

// NOTIF-005: cancel removes the assignment, writes a cancellation audit event,
// and still queues the student notification.
func TestResolveCancel_deletesAssignmentAndNotifies(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		smsEnabled: true, smsTemplate: "Sit-in cancelled: {{student_name}}",
	})
	f := newNotificationFixture(t, q, pool, "", "+66812345678")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	status, err := resolveIssueInTx(t, pool, q, f, "cancel")
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("notification status = %q, want queued", status)
	}
	if got := issueStatus(t, pool, f.issueID); got != "resolved" {
		t.Fatalf("issue status = %q, want resolved", got)
	}
	if got := assignmentCount(t, pool, f.absenceID); got != 0 {
		t.Fatalf("sit-in assignment count = %d, want 0 after cancellation", got)
	}

	var eventAction string
	var previousSessionID pgtype.UUID
	var newSessionID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		SELECT action, previous_session_id, new_session_id
		FROM absence_sit_in_assignment_events
		WHERE absence_id = $1 ORDER BY created_at DESC LIMIT 1
	`, f.absenceID).Scan(&eventAction, &previousSessionID, &newSessionID); err != nil {
		t.Fatal(err)
	}
	if eventAction != "cancelled" {
		t.Errorf("event action = %q, want cancelled", eventAction)
	}
	if previousSessionID != f.sessionID {
		t.Errorf("event previous session = %s, want cancelled assignment's session %s", previousSessionID.String(), f.sessionID.String())
	}
	if newSessionID.Valid {
		t.Errorf("cancel event must not reference a new session, got %s", newSessionID.String())
	}

	rows := loadOutboxRows(t, pool, f.absenceID)
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want exactly 1", len(rows))
	}
	row := rows[0]
	if row.Channel != "sms" || row.Status != "queued" {
		t.Errorf("notification = %s/%s, want sms/queued", row.Channel, row.Status)
	}
	if row.MessageType != "sit_in_cancel" {
		t.Errorf("message type = %q, want sit_in_cancel", row.MessageType)
	}
	if row.AssignmentID.Valid {
		t.Errorf("notification assignment_id = %s, want NULL (the assignment was deleted)", row.AssignmentID.String())
	}
	if row.Recipient != f.studentPhone {
		t.Errorf("recipient = %q, want %q", row.Recipient, f.studentPhone)
	}
	if row.Payload["action"] != "cancel" {
		t.Errorf("payload action = %q, want cancel", row.Payload["action"])
	}
}

// NOTIF-006: mark for review changes state but never notifies.
func TestResolveMarkForReview_noNotification(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		smsEnabled:   true,
		smsTemplate:  "Sit-in moved: {{student_name}}",
		emailEnabled: true, emailSubject: "Sit-in moved", emailBody: "Hello {{student_name}}",
	})
	f := newNotificationFixture(t, q, pool, "student@example.edu", "+66812345678")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := q.WithTx(tx).ResolveScheduleIssueWithSnapshot(ctx, f.issueID, pgtype.UUID{}, f.actorID, f.issueVersion, 0, "mark_for_review", "Needs owner review", "Asia/Bangkok", DefaultSnapshotBuilder)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if status != "not_required" {
		t.Fatalf("notification status = %q, want not_required", status)
	}
	if got := issueStatus(t, pool, f.issueID); got != "needs_review" {
		t.Fatalf("issue status = %q, want needs_review", got)
	}
	if rows := loadOutboxRows(t, pool, f.absenceID); len(rows) != 0 {
		t.Fatalf("outbox rows = %d, want 0 for mark_for_review", len(rows))
	}
	if got := assignmentCount(t, pool, f.absenceID); got != 1 {
		t.Fatalf("sit-in assignment count = %d, want unchanged (1)", got)
	}
}

// NOTIF-007: dismiss changes nothing about the arrangement and never notifies.
func TestResolveDismiss_noNotificationNoAssignmentChange(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		smsEnabled:   true,
		smsTemplate:  "Sit-in moved: {{student_name}}",
		emailEnabled: true, emailSubject: "Sit-in moved", emailBody: "Hello {{student_name}}",
	})
	f := newNotificationFixture(t, q, pool, "student@example.edu", "+66812345678")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := q.WithTx(tx).ResolveScheduleIssueWithSnapshot(ctx, f.issueID, pgtype.UUID{}, f.actorID, f.issueVersion, 0, "dismiss", "Duplicate issue", "Asia/Bangkok", DefaultSnapshotBuilder)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if status != "not_required" {
		t.Fatalf("notification status = %q, want not_required", status)
	}
	if got := issueStatus(t, pool, f.issueID); got != "dismissed" {
		t.Fatalf("issue status = %q, want dismissed", got)
	}
	if rows := loadOutboxRows(t, pool, f.absenceID); len(rows) != 0 {
		t.Fatalf("outbox rows = %d, want 0 for dismiss", len(rows))
	}
	if got := assignmentCount(t, pool, f.absenceID); got != 1 {
		t.Fatalf("sit-in assignment count = %d, want unchanged (1)", got)
	}
}

// NOTIF-008: when the surrounding transaction fails and rolls back, nothing is
// persisted — no issue resolution, no assignment change, no notification row.
func TestResolveRollback_leavesNoPartialWrites(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		smsEnabled: true, smsTemplate: "Sit-in moved: {{student_name}}",
	})
	f := newNotificationFixture(t, q, pool, "", "+66812345678")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := q.WithTx(tx).ResolveScheduleIssueWithSnapshot(ctx, f.issueID, pgtype.UUID{}, f.actorID, f.issueVersion, 0, "keep", "", "Asia/Bangkok", DefaultSnapshotBuilder)
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("notification status inside tx = %q, want queued", status)
	}

	// Inside the transaction the notification exists...
	var pendingCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE absence_id = $1`, f.absenceID).Scan(&pendingCount); err != nil {
		t.Fatal(err)
	}
	if pendingCount != 1 {
		t.Fatalf("pending outbox rows inside tx = %d, want 1", pendingCount)
	}

	// ...but a downstream failure forces a rollback of the entire transaction.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	if got := issueStatus(t, pool, f.issueID); got != "open" {
		t.Fatalf("issue status after rollback = %q, want open", got)
	}
	if got := assignmentCount(t, pool, f.absenceID); got != 1 {
		t.Fatalf("sit-in assignment count after rollback = %d, want 1", got)
	}
	if rows := loadOutboxRows(t, pool, f.absenceID); len(rows) != 0 {
		t.Fatalf("outbox rows after rollback = %d, want 0", len(rows))
	}
}

// NOTIF-009: submitting the same resolution twice (stale version retry) must
// not duplicate the notification.
func TestResolveDuplicateSubmission_noDuplicateNotifications(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		smsEnabled: true, smsTemplate: "Sit-in moved: {{student_name}}",
	})
	f := newNotificationFixture(t, q, pool, "", "+66812345678")

	status, err := resolveIssueInTx(t, pool, q, f, "keep")
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("first notification status = %q, want queued", status)
	}

	// Second submission of the identical request (same expected version).
	if _, err := resolveIssueInTx(t, pool, q, f, "keep"); err == nil {
		t.Fatal("expected duplicate submission to be rejected, got nil error")
	}

	rows := loadOutboxRows(t, pool, f.absenceID)
	if len(rows) != 1 {
		t.Fatalf("outbox rows after duplicate submission = %d, want exactly 1", len(rows))
	}
}

// NOTIF-010: two staff members resolving the same issue concurrently — exactly
// one succeeds, and only the successful action queues notifications.
func TestResolveConcurrent_singleWinner(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		smsEnabled: true, smsTemplate: "Sit-in moved: {{student_name}}",
	})
	f := newNotificationFixture(t, q, pool, "", "+66812345678")

	type outcome struct {
		status string
		err    error
	}
	attempt := func() outcome {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return outcome{err: err}
		}
		defer func() { _ = tx.Rollback(ctx) }()
		status, err := q.WithTx(tx).ResolveScheduleIssueWithSnapshot(ctx, f.issueID, pgtype.UUID{}, f.actorID, f.issueVersion, 0, "keep", "", "Asia/Bangkok", DefaultSnapshotBuilder)
		if err != nil {
			return outcome{err: err}
		}
		if err := tx.Commit(ctx); err != nil {
			return outcome{err: err}
		}
		return outcome{status: status}
	}

	start := make(chan struct{})
	results := make([]outcome, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := range results {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = attempt()
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded := 0
	for _, result := range results {
		if result.err == nil {
			succeeded++
			if result.status != "queued" {
				t.Errorf("winner notification status = %q, want queued", result.status)
			}
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful resolutions = %d, want exactly 1 (results: %+v)", succeeded, results)
	}

	rows := loadOutboxRows(t, pool, f.absenceID)
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want exactly 1 (only the winning action notifies)", len(rows))
	}
	if got := issueStatus(t, pool, f.issueID); got != "resolved" {
		t.Fatalf("issue status = %q, want resolved", got)
	}
}

// NOTIF-011 through NOTIF-019: channel configuration and contact availability
// matrix. Resolution always succeeds; the notification outcome depends on
// which channels are enabled, configured, and have a student recipient.
func TestResolveNotify_configurationMatrix(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)

	tests := []struct {
		name         string
		settings     channelSettings
		email        string
		phone        string
		wantStatus   string
		wantChannels []string
		wantRowCount int
	}{
		{
			name:         "NOTIF-011 sms disabled, email configured",
			settings:     channelSettings{emailEnabled: true, emailSubject: "s", emailBody: "b"},
			email:        "student@example.edu",
			wantStatus:   "queued",
			wantChannels: []string{"email"},
			wantRowCount: 1,
		},
		{
			name:         "NOTIF-012 email disabled, sms configured",
			settings:     channelSettings{smsEnabled: true, smsTemplate: "t"},
			phone:        "+66812345678",
			wantStatus:   "queued",
			wantChannels: []string{"sms"},
			wantRowCount: 1,
		},
		{
			name:         "NOTIF-013 both channels disabled",
			settings:     channelSettings{},
			email:        "student@example.edu",
			phone:        "+66812345678",
			wantStatus:   "not_configured",
			wantRowCount: 0,
		},
		{
			name:         "NOTIF-014 sms enabled but template empty",
			settings:     channelSettings{smsEnabled: true, smsTemplate: "  "},
			phone:        "+66812345678",
			wantStatus:   "not_configured",
			wantRowCount: 0,
		},
		{
			name:         "NOTIF-015 email enabled but subject missing",
			settings:     channelSettings{emailEnabled: true, emailSubject: "", emailBody: "b"},
			email:        "student@example.edu",
			wantStatus:   "not_configured",
			wantRowCount: 0,
		},
		{
			name:         "NOTIF-016 email enabled but body missing",
			settings:     channelSettings{emailEnabled: true, emailSubject: "s", emailBody: ""},
			email:        "student@example.edu",
			wantStatus:   "not_configured",
			wantRowCount: 0,
		},
		{
			name:         "NOTIF-017 sms configured but student has no phone",
			settings:     channelSettings{smsEnabled: true, smsTemplate: "t"},
			wantStatus:   "no_recipient",
			wantRowCount: 0,
		},
		{
			name:         "NOTIF-018 email configured but student has no email",
			settings:     channelSettings{emailEnabled: true, emailSubject: "s", emailBody: "b"},
			wantStatus:   "no_recipient",
			wantRowCount: 0,
		},
		{
			name: "NOTIF-019 sms recipient missing but valid email exists",
			settings: channelSettings{
				smsEnabled:   true,
				smsTemplate:  "t",
				emailEnabled: true, emailSubject: "s", emailBody: "b",
			},
			email:        "student@example.edu",
			wantStatus:   "queued",
			wantChannels: []string{"email"}, // SMS skipped, email still queued
			wantRowCount: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withChannelSettings(t, pool, test.settings)
			f := newNotificationFixture(t, q, pool, test.email, test.phone)

			status, err := resolveIssueInTx(t, pool, q, f, "keep")
			if err != nil {
				t.Fatal(err)
			}
			if status != test.wantStatus {
				t.Errorf("notification status = %q, want %q", status, test.wantStatus)
			}
			if got := issueStatus(t, pool, f.issueID); got != "resolved" {
				t.Errorf("issue status = %q, want resolved (resolution succeeds regardless of notification outcome)", got)
			}

			rows := loadOutboxRows(t, pool, f.absenceID)
			if len(rows) != test.wantRowCount {
				t.Fatalf("outbox rows = %d, want %d", len(rows), test.wantRowCount)
			}
			for i, channel := range test.wantChannels {
				if rows[i].Channel != channel {
					t.Errorf("row %d channel = %q, want %q", i, rows[i].Channel, channel)
				}
				if rows[i].Status != "queued" {
					t.Errorf("row %d status = %q, want queued", i, rows[i].Status)
				}
			}
		})
	}
}

// NOTIF-020: settings changed while the resolution panel is open — the server
// reads transactional settings at resolve time, not frontend-cached values.
func TestResolveNotify_usesCurrentSettings(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)

	// Panel opens while SMS is disabled.
	withChannelSettings(t, pool, channelSettings{})
	f := newNotificationFixture(t, q, pool, "", "+66812345678")

	// Operations enable SMS (fresh template) before the staff member confirms.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		UPDATE app_settings
		SET sit_in_change_sms_enabled = true, sit_in_change_sms_template = 'Fresh: {{student_name}}'
		WHERE id = true
	`); err != nil {
		t.Fatal(err)
	}

	status, err := resolveIssueInTx(t, pool, q, f, "keep")
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("notification status = %q, want queued with current settings", status)
	}
	rows := loadOutboxRows(t, pool, f.absenceID)
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(rows))
	}
	if rows[0].Payload["sms_template"] != "Fresh: {{student_name}}" {
		t.Errorf("payload sms_template = %q, want current transactional template", rows[0].Payload["sms_template"])
	}
}

// PARENT negative contract: guardian notification is not a feature. Only the
// student's own contact information may ever be used — when the student has
// none, the outcome is no_recipient, never an implicit fallback.
func TestResolveNotify_parentContactNeverImplicitFallback(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		smsEnabled:   true,
		smsTemplate:  "t",
		emailEnabled: true, emailSubject: "s", emailBody: "b",
	})
	// Absence recorded without any student contact details.
	f := newNotificationFixture(t, q, pool, "", "")

	status, err := resolveIssueInTx(t, pool, q, f, "keep")
	if err != nil {
		t.Fatal(err)
	}
	if status != "no_recipient" {
		t.Fatalf("notification status = %q, want no_recipient", status)
	}
	if rows := loadOutboxRows(t, pool, f.absenceID); len(rows) != 0 {
		t.Fatalf("outbox rows = %d, want 0 (no parent/guardian fallback may be used)", len(rows))
	}
}

// The resolution pipeline reads contact details exclusively from the student's
// own absence record; no guardian tables or columns participate.
func TestResolveNotify_recipientAlwaysStudentContact(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	withChannelSettings(t, pool, channelSettings{
		smsEnabled:   true,
		smsTemplate:  "t",
		emailEnabled: true, emailSubject: "s", emailBody: "b",
	})
	f := newNotificationFixture(t, q, pool, "student@example.edu", "+66812345678")

	if _, err := resolveIssueInTx(t, pool, q, f, "keep"); err != nil {
		t.Fatal(err)
	}
	rows := loadOutboxRows(t, pool, f.absenceID)
	if len(rows) != 2 {
		t.Fatalf("outbox rows = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if row.Recipient != f.studentEmail && row.Recipient != f.studentPhone {
			t.Errorf("recipient %q is not one of the student's own contacts", row.Recipient)
		}
		if row.Channel == "sms" && strings.Contains(row.Recipient, "@") {
			t.Errorf("sms recipient %q looks like an email address", row.Recipient)
		}
		if row.Channel == "email" && !strings.Contains(row.Recipient, "@") {
			t.Errorf("email recipient %q does not look like an email address", row.Recipient)
		}
	}
}
