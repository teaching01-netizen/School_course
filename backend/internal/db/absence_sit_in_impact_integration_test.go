package db

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"warwick-institute/internal/snapshot"
)

func TestAbsenceSitInImpactsByAbsenceIDs(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	defer pool.Close()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	q := New(tx)
	suffix := time.Now().UTC().Format("150405.000000000")
	teacher, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "impact-inbox-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "impact-inbox-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "impact-inbox-" + suffix, Name: "Original class"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 6, 3, 3, 0, 0, 0, time.UTC)
	session := createTestSession(t, ctx, q, course.ID, teacher, room.ID, start, start.Add(time.Hour))
	createAbsence := func(code string) pgtype.UUID {
		t.Helper()
		row, err := q.AbsenceCreate(ctx, AbsenceCreateParams{Wcode: code + suffix, CourseID: course.ID,
			DateFrom: pgtype.Date{Time: start, Valid: true}, DateTo: pgtype.Date{Time: start, Valid: true}, SitInCourseID: course.ID})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	abs1, abs2 := createAbsence("INBOX1"), createAbsence("INBOX2")
	original := snapshot.BuildSessionSnapshotV1(snapshot.AssignmentSession{
		ID: uuid.UUID(session.Bytes), CourseID: uuid.UUID(course.ID.Bytes), TeacherID: uuid.UUID(teacher.Bytes),
		CourseCode: course.Code, CourseName: course.Name, StartAt: start, EndAt: start.Add(time.Hour), Version: 1,
	}, time.Now().UTC(), "Asia/Bangkok")
	evidence, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO absence_sit_ins(absence_id,session_id,session_snapshot_at_assignment,
 snapshot_quality,snapshot_schema_version,snapshot_captured_at,snapshot_source)
 VALUES($1,$2,$3::jsonb,'exact',1,now(),'assignment')`, abs1, session, string(evidence)); err != nil {
		t.Fatal(err)
	}
	var change pgtype.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO session_changes(session_id,session_version,change_source,changed_fields,before_snapshot,after_snapshot,
 old_start_at,old_end_at,new_start_at,new_end_at,old_course_id,new_course_id,old_teacher_id,new_teacher_id)
 VALUES($1,2,'single_edit','["start_at"]','{}','{}',$2,$3,$2,$3,$4,$4,$5,$5) RETURNING id`, session, start, start.Add(time.Hour), course.ID, teacher).Scan(&change); err != nil {
		t.Fatal(err)
	}
	insertIssue := func(absence pgtype.UUID, fingerprint string, sitIn, missed pgtype.UUID) pgtype.UUID {
		t.Helper()
		id, err := q.AbsenceScheduleIssueUpsert(ctx, AbsenceScheduleIssueUpsertParams{AbsenceID: absence, IssueType: "time_changed", Severity: "warning",
			SitInSessionID: sitIn, MissedSessionID: missed, SourceSessionID: session, SessionChangeID: change, DetailsJson: "{}", SuggestedResolutionJson: "[]",
			Fingerprint: fingerprint + suffix, SnapshotJson: string(evidence), SnapshotQuality: "exact", SnapshotSource: pgtype.Text{String: "assignment", Valid: true}})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	insertIssue(abs1, "open", session, pgtype.UUID{})
	insertIssue(abs1, "duplicate", session, pgtype.UUID{})
	insertIssue(abs2, "missed-only", pgtype.UUID{}, session)
	resolved := insertIssue(abs2, "resolved", session, pgtype.UUID{})
	if _, err := tx.Exec(ctx, `UPDATE absence_schedule_issues SET status='resolved' WHERE id=$1`, resolved); err != nil {
		t.Fatal(err)
	}
	moved := start.Add(48 * time.Hour)
	if _, err := tx.Exec(ctx, `UPDATE sessions SET start_at=$2,end_at=$3,version=version+1 WHERE id=$1`, session, moved, moved.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	rows, err := q.AbsenceSitInImpactsByAbsenceIDs(ctx, []pgtype.UUID{abs1, abs2})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].AbsenceID != abs1 {
		t.Fatalf("unexpected contexts: %+v", rows)
	}
	if !rows[0].Current.StartAt.Time.Equal(moved) {
		t.Fatal("current time did not move")
	}
	saved, err := snapshot.DecodeSessionSnapshotV1(rows[0].AssignmentSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.StartAt.Equal(original.StartAt) {
		t.Fatal("original time changed")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM absence_sit_ins WHERE absence_id=$1`, abs1); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET deleted_at=now(),version=version+1 WHERE id=$1`, session); err != nil {
		t.Fatal(err)
	}
	// Deleted-session issues intentionally clear their live foreign keys.
	if _, err := tx.Exec(ctx, `UPDATE absence_schedule_issues SET issue_type='sit_in_session_deleted',
 sit_in_session_id=NULL,source_session_id=NULL WHERE absence_id=$1`, abs1); err != nil {
		t.Fatal(err)
	}
	rows, err = q.AbsenceSitInImpactsByAbsenceIDs(ctx, []pgtype.UUID{abs1})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Current.SessionID.Valid || len(rows[0].AssignmentSnapshot) != 0 || len(rows[0].IssueSnapshot) == 0 {
		t.Fatalf("missing preserved context after removal: %+v", rows)
	}
}
