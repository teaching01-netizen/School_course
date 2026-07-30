package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type impactFixture struct {
	absenceID pgtype.UUID
	sessionID pgtype.UUID
	version   int32
	courseID  pgtype.UUID
	teacherID pgtype.UUID
}

func newImpactFixture(t *testing.T, q *Queries) impactFixture {
	t.Helper()
	ctx := context.Background()
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "impact-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "impact-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "IMPACT-" + suffix, Name: "Impact " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	startAt := pgtype.Timestamptz{Time: time.Date(2030, 1, 1, 9, 0, 0, 0, time.UTC), Valid: true}
	endAt := pgtype.Timestamptz{Time: time.Date(2030, 1, 1, 10, 0, 0, 0, time.UTC), Valid: true}
	session, err := q.SessionCreate(ctx, SessionCreateParams{CourseID: course.ID, RoomID: room.ID, TeacherID: teacherID, StartAt: startAt, EndAt: endAt})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode: "IMPACT-" + suffix, CourseID: course.ID,
		DateFrom: pgtype.Date{Time: startAt.Time, Valid: true}, DateTo: pgtype.Date{Time: startAt.Time, Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return impactFixture{absenceID: absence.ID, sessionID: session.ID, version: session.Version, courseID: course.ID, teacherID: teacherID}
}

func TestScheduleImpact_DeleteSessionCapturesImpactedAssignment(t *testing.T) {
	// Given
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)
	fixture := newImpactFixture(t, q)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := dbpool.Exec(ctx, `INSERT INTO absence_sit_ins (absence_id, session_id) VALUES ($1, $2)`, fixture.absenceID, fixture.sessionID); err != nil {
		t.Fatal(err)
	}

	// When
	if _, err := q.SessionHardDelete(ctx, SessionHardDeleteParams{ID: fixture.sessionID, Version: fixture.version}); err != nil {
		t.Fatal(err)
	}

	// Then
	var targets int
	if err := dbpool.QueryRow(ctx, `
		SELECT count(*)
		FROM session_change_impact_targets target
		JOIN session_changes change ON change.id = target.session_change_id
		WHERE change.session_id = $1 AND target.absence_id = $2 AND target.relation_type = 'sit_in'
	`, fixture.sessionID, fixture.absenceID).Scan(&targets); err != nil {
		t.Fatal(err)
	}
	if targets != 1 {
		t.Fatalf("captured targets = %d, want 1", targets)
	}
}

func TestScheduleIssueResolve_dismissesNeedsReviewIssue(t *testing.T) {
	// Given
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)
	fixture := newImpactFixture(t, q)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var changeID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO session_changes (
			session_id, session_version, changed_fields, before_snapshot, after_snapshot,
			old_start_at, old_end_at, new_start_at, new_end_at,
			old_course_id, new_course_id, old_teacher_id, new_teacher_id
		)
		SELECT id, version + 1, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
		       start_at, end_at, start_at, end_at, course_id, course_id, teacher_id, teacher_id
		FROM sessions WHERE id = $1
		RETURNING id
	`, fixture.sessionID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}
	var issueID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO absence_schedule_issues (
			absence_id, issue_type, severity, status, first_session_change_id,
			latest_session_change_id, fingerprint, assigned_to, review_reason
		)
		VALUES ($1, 'sit_in_session_changed', 'critical', 'needs_review', $2, $2, $3, $4, 'needs manager review')
		RETURNING id
	`, fixture.absenceID, changeID, "needs-review-"+fixture.absenceID.String(), fixture.teacherID).Scan(&issueID); err != nil {
		t.Fatal(err)
	}
	tx, err := dbpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// When
	if _, err := q.WithTx(tx).ResolveScheduleIssue(ctx, issueID, pgtype.UUID{}, fixture.teacherID, 1, 0, "dismiss", "approved exception"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Then
	var status string
	if err := dbpool.QueryRow(ctx, `SELECT status FROM absence_schedule_issues WHERE id = $1`, issueID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "dismissed" {
		t.Fatalf("issue status = %q, want dismissed", status)
	}
}

func TestScheduleIssueResolve_rejectsFullCandidate(t *testing.T) {
	// Given
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)
	fixture := newImpactFixture(t, q)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	candidateRoom, err := q.RoomCreate(ctx, RoomCreateParams{Name: "candidate-room-" + fixture.absenceID.String(), Capacity: pgtype.Int4{Int32: 1, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	candidateCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "CANDIDATE-" + fixture.absenceID.String(), Name: "Candidate"})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := q.SessionCreate(ctx, SessionCreateParams{
		CourseID: candidateCourse.ID, RoomID: candidateRoom.ID, TeacherID: fixture.teacherID,
		StartAt: pgtype.Timestamptz{Time: time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2030, 1, 2, 10, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	occupiedAbsence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode: "OCCUPIED-" + fixture.absenceID.String(), CourseID: candidateCourse.ID,
		DateFrom: pgtype.Date{Time: candidate.StartAt.Time, Valid: true}, DateTo: pgtype.Date{Time: candidate.StartAt.Time, Valid: true},
		SitInCourseID: candidateCourse.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO absence_sit_ins (absence_id, session_id)
		VALUES ($1, $2), ($3, $4)
	`, fixture.absenceID, fixture.sessionID, occupiedAbsence.ID, candidate.ID); err != nil {
		t.Fatal(err)
	}
	var changeID, issueID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO session_changes (
			session_id, session_version, changed_fields, before_snapshot, after_snapshot,
			old_start_at, old_end_at, new_start_at, new_end_at,
			old_course_id, new_course_id, old_teacher_id, new_teacher_id
		)
		SELECT id, version + 1, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
		       start_at, end_at, start_at, end_at, course_id, course_id, teacher_id, teacher_id
		FROM sessions WHERE id = $1
		RETURNING id
	`, fixture.sessionID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO absence_schedule_issues (
			absence_id, issue_type, severity, source_session_id, sit_in_session_id,
			first_session_change_id, latest_session_change_id, fingerprint
		)
		VALUES ($1, 'sit_in_session_changed', 'critical', $2, $2, $3, $3, $4)
		RETURNING id
	`, fixture.absenceID, fixture.sessionID, changeID, "full-candidate-"+fixture.absenceID.String()).Scan(&issueID); err != nil {
		t.Fatal(err)
	}
	tx, err := dbpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// When
	_, err = q.WithTx(tx).ResolveScheduleIssue(ctx, issueID, candidate.ID, fixture.teacherID, 1, candidate.Version, "reassign", "")

	// Then
	if err == nil || !strings.Contains(err.Error(), "no remaining capacity") {
		t.Fatalf("expected full candidate error, got %v", err)
	}
}
