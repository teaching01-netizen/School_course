package sessionchangeimpact

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// RED (Hole B): a harmless time move with no overlap must not fan out into
// critical sit_in_ineligible rows from session_version_changed alone, must not
// emit the always-on sit_in_session_changed warning when validation passes,
// and must not duplicate short_notice rows alongside an invalid assignment.
// Time-only scope: analyse/show/queue impact only when the student's sit-in
// session start/end effectively changed AND the student cannot safely attend.
func TestAnalyze_harmlessTimeMoveCreatesNoCriticalOrNoiseWarning(t *testing.T) {
	databaseURL := autoTestDB(t)
	autoMigrateUpOnce(t, databaseURL)
	pool := autoPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "quiet-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "quiet-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "QUIET-" + suffix, Name: "Quiet " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	// Far-future session: +1h move stays far outside the 24h warning window,
	// so any warning/critical row is pure noise, not a timing concern.
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: time.Date(2031, 5, 4, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2031, 5, 4, 10, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode: "QUIET-" + suffix, CourseID: course.ID,
		DateFrom:      pgtype.Date{Time: session.StartAt.Time, Valid: true},
		DateTo:        pgtype.Date{Time: session.StartAt.Time, Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO absence_sit_ins (
			absence_id, session_id, session_version_at_assignment,
			session_snapshot_at_assignment, snapshot_schema_version,
			snapshot_captured_at, snapshot_quality, snapshot_source
		)
		VALUES ($1, $2, $3,
		        '{"schema_version":1}'::jsonb, 1, now(), 'exact', 'captured_at_assignment')
	`, absence.ID, session.ID, session.Version); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, absence.ID); err != nil {
			t.Logf("cleanup absence: %v", err)
		}
	})

	var changeID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO session_changes (
			session_id, session_version, changed_fields, before_snapshot, after_snapshot,
			old_start_at, old_end_at, new_start_at, new_end_at,
			old_course_id, new_course_id, old_teacher_id, new_teacher_id
		)
		SELECT id, version + 1,
		       '{"start_at":true,"end_at":true}'::jsonb, '{}'::jsonb, '{}'::jsonb,
		       start_at, end_at, start_at + interval '1 hour', end_at + interval '1 hour',
		       course_id, course_id, teacher_id, teacher_id
		FROM sessions WHERE id = $1
		RETURNING id
	`, session.ID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, q, "Asia/Bangkok", nil, slog.Default())
	if err := service.Analyze(ctx, changeID); err != nil {
		t.Fatal(err)
	}

	var critical int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM absence_schedule_issues
		WHERE latest_session_change_id = $1 AND status IN ('open','needs_review') AND severity = 'critical'
	`, changeID).Scan(&critical); err != nil {
		t.Fatal(err)
	}
	if critical != 0 {
		t.Errorf("harmless +1h move created %d critical issue(s), want 0 (no overlap, far future)", critical)
	}
	var noiseWarnings int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM absence_schedule_issues
		WHERE latest_session_change_id = $1 AND status IN ('open','needs_review')
		  AND issue_type IN ('sit_in_session_changed','sit_in_ineligible')
	`, changeID).Scan(&noiseWarnings); err != nil {
		t.Fatal(err)
	}
	if noiseWarnings != 0 {
		t.Errorf("harmless +1h move created %d version-only warning(s), want 0", noiseWarnings)
	}
}

// Acceptance A1: a room-only change (identical start/end) creates nothing.
// The student can still attend; any queue row would be an unnecessary warning.
func TestAnalyze_roomOnlyChangeCreatesNoIssues(t *testing.T) {
	databaseURL := autoTestDB(t)
	autoMigrateUpOnce(t, databaseURL)
	pool := autoPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "room-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "room-old-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "ROOM-" + suffix, Name: "Room " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: time.Date(2031, 5, 4, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2031, 5, 4, 10, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode: "ROOM-" + suffix, CourseID: course.ID,
		DateFrom:      pgtype.Date{Time: session.StartAt.Time, Valid: true},
		DateTo:        pgtype.Date{Time: session.StartAt.Time, Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO absence_sit_ins (absence_id, session_id, session_version_at_assignment, session_snapshot_at_assignment, snapshot_schema_version, snapshot_captured_at, snapshot_quality, snapshot_source) VALUES ($1, $2, $3, '{"schema_version":1}'::jsonb, 1, now(), 'exact', 'captured_at_assignment')`, absence.ID, session.ID, session.Version); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, absence.ID); err != nil {
			t.Logf("cleanup absence: %v", err)
		}
	})

	// Room-only change: identical times, so the sit-in time did not change.
	var changeID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO session_changes (session_id, session_version, changed_fields, before_snapshot, after_snapshot, old_start_at, old_end_at, new_start_at, new_end_at, old_course_id, new_course_id, old_teacher_id, new_teacher_id) SELECT id, version + 1, '{"room_id":true}'::jsonb, '{}'::jsonb, '{}'::jsonb, start_at, end_at, start_at, end_at, course_id, course_id, teacher_id, teacher_id FROM sessions WHERE id = $1 RETURNING id`, session.ID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, q, "Asia/Bangkok", nil, slog.Default())
	if err := service.Analyze(ctx, changeID); err != nil {
		t.Fatal(err)
	}

	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review')`, changeID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("room-only change created %d issue(s), want 0", total)
	}
	var runStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM session_change_impact_runs WHERE session_change_id = $1`, changeID).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus != "completed" {
		t.Errorf("impact run status = %q, want completed", runStatus)
	}
}

// Acceptance A2: deleted session still creates exactly 1 critical row.
// Deletion is the exception to time-only scope: unchanged timestamps still
// require action because the arrangement no longer exists.
func TestAnalyze_deletedSessionCreatesSingleCritical(t *testing.T) {
	databaseURL := autoTestDB(t)
	autoMigrateUpOnce(t, databaseURL)
	pool := autoPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "del-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "del-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "DEL-" + suffix, Name: "Del " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: time.Date(2031, 5, 4, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2031, 5, 4, 10, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode: "DEL-" + suffix, CourseID: course.ID,
		DateFrom:      pgtype.Date{Time: session.StartAt.Time, Valid: true},
		DateTo:        pgtype.Date{Time: session.StartAt.Time, Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO absence_sit_ins (absence_id, session_id, session_version_at_assignment, session_snapshot_at_assignment, snapshot_schema_version, snapshot_captured_at, snapshot_quality, snapshot_source) VALUES ($1, $2, $3, '{"schema_version":1}'::jsonb, 1, now(), 'exact', 'captured_at_assignment')`, absence.ID, session.ID, session.Version); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, absence.ID); err != nil {
			t.Logf("cleanup absence: %v", err)
		}
	})

	// Deletion change with unchanged timestamps + impact target row.
	var changeID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO session_changes (session_id, session_version, changed_fields, before_snapshot, after_snapshot, old_start_at, old_end_at, new_start_at, new_end_at, old_course_id, new_course_id, old_teacher_id, new_teacher_id, change_source) SELECT id, version + 1, '{"deleted":true}'::jsonb, '{}'::jsonb, '{}'::jsonb, start_at, end_at, start_at, end_at, course_id, course_id, teacher_id, teacher_id, 'session_delete' FROM sessions WHERE id = $1 RETURNING id`, session.ID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO session_change_impact_targets (session_change_id, absence_id, relation_type) VALUES ($1, $2, 'sit_in')`, changeID, absence.ID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, q, "Asia/Bangkok", nil, slog.Default())
	if err := service.Analyze(ctx, changeID); err != nil {
		t.Fatal(err)
	}

	var total, critical int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE severity='critical') FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review')`, changeID).Scan(&total, &critical); err != nil {
		t.Fatal(err)
	}
	if total != 1 || critical != 1 {
		t.Errorf("deleted session created total=%d critical=%d, want 1/1", total, critical)
	}
	var issueType string
	if err := pool.QueryRow(ctx, `SELECT issue_type FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review') LIMIT 1`, changeID).Scan(&issueType); err != nil {
		t.Fatal(err)
	}
	if issueType != "sit_in_session_deleted" {
		t.Errorf("issue_type=%q, want sit_in_session_deleted", issueType)
	}
}

// Acceptance A5: returning the session to its assigned snapshot time retires
// the earlier timing row via Analyze. The queue must not keep warning about a
// change that has been undone.
func TestAnalyze_revertToAssignedTimeRetiresEarlierRow(t *testing.T) {
	databaseURL := autoTestDB(t)
	autoMigrateUpOnce(t, databaseURL)
	pool := autoPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "revert-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "revert-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "REVERT-" + suffix, Name: "Revert " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Second).Add(2 * time.Hour)
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: base, Valid: true},
		EndAt:   pgtype.Timestamptz{Time: base.Add(1 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode: "REVERT-" + suffix, CourseID: course.ID,
		DateFrom:      pgtype.Date{Time: base, Valid: true},
		DateTo:        pgtype.Date{Time: base, Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Snapshot records the assigned time explicitly so the revert comparison is
	// well-defined (rather than relying on the '{}' fallback path).
	snapStart := session.StartAt.Time.UTC().Format(time.RFC3339Nano)
	snapEnd := session.EndAt.Time.UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(ctx, `INSERT INTO absence_sit_ins (absence_id, session_id, session_version_at_assignment, session_snapshot_at_assignment, snapshot_schema_version, snapshot_captured_at, snapshot_quality, snapshot_source) VALUES ($1, $2, $3, jsonb_build_object('schema_version', 1, 'start_at', $4::timestamptz, 'end_at', $5::timestamptz), 1, now(), 'exact', 'captured_at_assignment')`, absence.ID, session.ID, session.Version, snapStart, snapEnd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, absence.ID); err != nil {
			t.Logf("cleanup absence: %v", err)
		}
	})

	service := New(pool, q, "Asia/Bangkok", nil, slog.Default())
	// Move 1: +30min near-future -> exactly 1 short_notice row.
	var change1 pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO session_changes (session_id, session_version, changed_fields, before_snapshot, after_snapshot, old_start_at, old_end_at, new_start_at, new_end_at, old_course_id, new_course_id, old_teacher_id, new_teacher_id) SELECT id, version + 1, '{"start_at":true,"end_at":true}'::jsonb, '{}'::jsonb, '{}'::jsonb, start_at, end_at, start_at + interval '30 minutes', end_at + interval '30 minutes', course_id, course_id, teacher_id, teacher_id FROM sessions WHERE id = $1 RETURNING id`, session.ID).Scan(&change1); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sessions SET start_at = start_at + interval '30 minutes', end_at = end_at + interval '30 minutes', version = version + 1 WHERE id = $1`, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Analyze(ctx, change1); err != nil {
		t.Fatal(err)
	}
	var firstCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review')`, change1).Scan(&firstCount); err != nil {
		t.Fatal(err)
	}
	if firstCount != 1 {
		t.Fatalf("move 1 rows = %d, want 1 (setup)", firstCount)
	}

	// Move 2: revert to the assigned snapshot time. Production order first.
	if _, err := pool.Exec(ctx, `UPDATE sessions SET start_at = start_at - interval '30 minutes', end_at = end_at - interval '30 minutes', version = version + 1 WHERE id = $1`, session.ID); err != nil {
		t.Fatal(err)
	}
	var change2 pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO session_changes (session_id, session_version, changed_fields, before_snapshot, after_snapshot, old_start_at, old_end_at, new_start_at, new_end_at, old_course_id, new_course_id, old_teacher_id, new_teacher_id) SELECT id, version + 1, '{"start_at":true,"end_at":true}'::jsonb, '{}'::jsonb, '{}'::jsonb, start_at + interval '30 minutes', end_at + interval '30 minutes', start_at, end_at, course_id, course_id, teacher_id, teacher_id FROM sessions WHERE id = $1 RETURNING id`, session.ID).Scan(&change2); err != nil {
		t.Fatal(err)
	}
	if err := service.Analyze(ctx, change2); err != nil {
		t.Fatal(err)
	}
	var staleOpen, total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review')`, change1).Scan(&staleOpen); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM absence_schedule_issues WHERE absence_id = $1 AND status IN ('open','needs_review')`, absence.ID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if staleOpen != 0 || total != 0 {
		t.Errorf("revert left stale=%d total=%d open, want 0/0", staleOpen, total)
	}
}

// Adversarial H5: harmful move (new time overlaps a missed session) must still
// yield exactly one critical row (sit_in_overlap) with no timing duplicate.
func TestAnalyze_overlappingMoveKeepsSingleCritical(t *testing.T) {
	databaseURL := autoTestDB(t)
	autoMigrateUpOnce(t, databaseURL)
	pool := autoPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "clash-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "clash-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "CLASH-" + suffix, Name: "Clash " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2031, 7, 6, 9, 0, 0, 0, time.UTC)
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: base, Valid: true},
		EndAt:   pgtype.Timestamptz{Time: base.Add(1 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Missed session overlapping the post-move window (10:00-11:00).
	// Different room: same-room overlapping sessions violate the
	// sessions_no_room_overlap exclusion constraint.
	missedRoom, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "clash-missed-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	missedTeacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "clash-missed-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	missed, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: missedRoom.ID, TeacherID: missedTeacherID,
		StartAt: pgtype.Timestamptz{Time: base.Add(1 * time.Hour), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: base.Add(2 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode: "CLASH-" + suffix, CourseID: course.ID,
		DateFrom:      pgtype.Date{Time: base, Valid: true},
		DateTo:        pgtype.Date{Time: base, Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO absence_sit_ins (absence_id, session_id, session_version_at_assignment, session_snapshot_at_assignment, snapshot_schema_version, snapshot_captured_at, snapshot_quality, snapshot_source) VALUES ($1, $2, $3, '{"schema_version":1}'::jsonb, 1, now(), 'exact', 'captured_at_assignment')`, absence.ID, session.ID, session.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO absence_missed_sessions(absence_id, session_id) VALUES ($1, $2)`, absence.ID, missed.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, absence.ID); err != nil {
			t.Logf("cleanup absence: %v", err)
		}
	})

	// Production updates the session row first, then records the change.
	// ValidateAssignment reads current session time, so mirror that order.
	if _, err := pool.Exec(ctx, `UPDATE sessions SET start_at = start_at + interval '1 hour', end_at = end_at + interval '1 hour', version = version + 1 WHERE id = $1`, session.ID); err != nil {
		t.Fatal(err)
	}
	// Move sit-in 09:00-10:00 -> 10:00-11:00: effective change AND overlaps missed.
	var changeID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO session_changes (
			session_id, session_version, changed_fields, before_snapshot, after_snapshot,
			old_start_at, old_end_at, new_start_at, new_end_at,
			old_course_id, new_course_id, old_teacher_id, new_teacher_id
		)
		SELECT id, version + 1,
		       '{"start_at":true,"end_at":true}'::jsonb, '{}'::jsonb, '{}'::jsonb,
		       start_at, end_at, start_at + interval '1 hour', end_at + interval '1 hour',
		       course_id, course_id, teacher_id, teacher_id
		FROM sessions WHERE id = $1
		RETURNING id
	`, session.ID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, q, "Asia/Bangkok", nil, slog.Default())
	if err := service.Analyze(ctx, changeID); err != nil {
		t.Fatal(err)
	}

	var total, critical int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE severity='critical') FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review')`, changeID).Scan(&total, &critical); err != nil {
		t.Fatal(err)
	}
	if total != 1 || critical != 1 {
		t.Errorf("overlapping move created total=%d critical=%d, want 1/1", total, critical)
	}
	var issueType string
	if err := pool.QueryRow(ctx, `SELECT issue_type FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review') LIMIT 1`, changeID).Scan(&issueType); err != nil {
		t.Fatal(err)
	}
	if issueType != "sit_in_overlap" {
		t.Errorf("issue_type=%q, want sit_in_overlap", issueType)
	}
}

// Adversarial H6 (recheck): a stale timing row from an earlier analysis must not
// linger open after a later effective move. Near-future move creates
// short_notice; a second effective move to the far future must retire it, so the
// queue never shows a warning whose times no longer match reality.
func TestAnalyze_laterEffectiveMoveRetiresStaleTimingRow(t *testing.T) {
	databaseURL := autoTestDB(t)
	autoMigrateUpOnce(t, databaseURL)
	pool := autoPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "stale-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "stale-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "STALE-" + suffix, Name: "Stale " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Second).Add(2 * time.Hour)
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: base, Valid: true},
		EndAt:   pgtype.Timestamptz{Time: base.Add(1 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode: "STALE-" + suffix, CourseID: course.ID,
		DateFrom:      pgtype.Date{Time: base, Valid: true},
		DateTo:        pgtype.Date{Time: base, Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO absence_sit_ins (absence_id, session_id, session_version_at_assignment, session_snapshot_at_assignment, snapshot_schema_version, snapshot_captured_at, snapshot_quality, snapshot_source) VALUES ($1, $2, $3, '{"schema_version":1}'::jsonb, 1, now(), 'exact', 'captured_at_assignment')`, absence.ID, session.ID, session.Version); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, absence.ID); err != nil {
			t.Logf("cleanup absence: %v", err)
		}
	})

	service := New(pool, q, "Asia/Bangkok", nil, slog.Default())
	// Move 1: +30min, stays near-future -> exactly 1 short_notice row.
	var change1 pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO session_changes (session_id, session_version, changed_fields, before_snapshot, after_snapshot, old_start_at, old_end_at, new_start_at, new_end_at, old_course_id, new_course_id, old_teacher_id, new_teacher_id) SELECT id, version + 1, '{"start_at":true,"end_at":true}'::jsonb, '{}'::jsonb, '{}'::jsonb, start_at, end_at, start_at + interval '30 minutes', end_at + interval '30 minutes', course_id, course_id, teacher_id, teacher_id FROM sessions WHERE id = $1 RETURNING id`, session.ID).Scan(&change1); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sessions SET start_at = start_at + interval '30 minutes', end_at = end_at + interval '30 minutes', version = version + 1 WHERE id = $1`, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Analyze(ctx, change1); err != nil {
		t.Fatal(err)
	}
	var firstCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review') AND issue_type = 'short_notice_change'`, change1).Scan(&firstCount); err != nil {
		t.Fatal(err)
	}
	if firstCount != 1 {
		t.Fatalf("move 1 short_notice rows = %d, want 1 (setup)", firstCount)
	}

	// Move 2: effective move to the far future -> stale short_notice must retire.
	if _, err := pool.Exec(ctx, `UPDATE sessions SET start_at = '2031-01-01T09:00:00Z', end_at = '2031-01-01T10:00:00Z', version = version + 1 WHERE id = $1`, session.ID); err != nil {
		t.Fatal(err)
	}
	var change2 pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO session_changes (session_id, session_version, changed_fields, before_snapshot, after_snapshot, old_start_at, old_end_at, new_start_at, new_end_at, old_course_id, new_course_id, old_teacher_id, new_teacher_id) SELECT id, version + 1, '{"start_at":true,"end_at":true}'::jsonb, '{}'::jsonb, '{}'::jsonb, start_at - interval '30 minutes', end_at - interval '30 minutes', start_at, end_at, course_id, course_id, teacher_id, teacher_id FROM sessions WHERE id = $1 RETURNING id`, session.ID).Scan(&change2); err != nil {
		t.Fatal(err)
	}
	if err := service.Analyze(ctx, change2); err != nil {
		t.Fatal(err)
	}
	var staleOpen int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review')`, change1).Scan(&staleOpen); err != nil {
		t.Fatal(err)
	}
	if staleOpen != 0 {
		t.Errorf("stale timing rows still open after later effective move: %d, want 0", staleOpen)
	}
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM absence_schedule_issues WHERE absence_id = $1 AND status IN ('open','needs_review')`, absence.ID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("open issues after far-future valid move: %d, want 0", total)
	}
}

// Acceptance A3: past-time valid move yields exactly 1 past_time_change.
// A session moved into the past is unactionable as arranged; the student needs
// one row, not a past_time + short_notice pair.
func TestAnalyze_pastTimeMoveKeepsSinglePastRow(t *testing.T) {
	databaseURL := autoTestDB(t)
	autoMigrateUpOnce(t, databaseURL)
	pool := autoPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "past-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "past-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "PAST-" + suffix, Name: "Past " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	// Session yesterday: the move target is unambiguously in the past.
	base := time.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: base, Valid: true},
		EndAt:   pgtype.Timestamptz{Time: base.Add(1 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode: "PAST-" + suffix, CourseID: course.ID,
		DateFrom:      pgtype.Date{Time: base, Valid: true},
		DateTo:        pgtype.Date{Time: base, Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO absence_sit_ins (absence_id, session_id, session_version_at_assignment, session_snapshot_at_assignment, snapshot_schema_version, snapshot_captured_at, snapshot_quality, snapshot_source) VALUES ($1, $2, $3, '{"schema_version":1}'::jsonb, 1, now(), 'exact', 'captured_at_assignment')`, absence.ID, session.ID, session.Version); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, absence.ID); err != nil {
			t.Logf("cleanup absence: %v", err)
		}
	})

	var changeID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO session_changes (session_id, session_version, changed_fields, before_snapshot, after_snapshot, old_start_at, old_end_at, new_start_at, new_end_at, old_course_id, new_course_id, old_teacher_id, new_teacher_id) SELECT id, version + 1, '{"start_at":true,"end_at":true}'::jsonb, '{}'::jsonb, '{}'::jsonb, start_at, end_at, start_at - interval '1 hour', end_at - interval '1 hour', course_id, course_id, teacher_id, teacher_id FROM sessions WHERE id = $1 RETURNING id`, session.ID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sessions SET start_at = start_at - interval '1 hour', end_at = end_at - interval '1 hour', version = version + 1 WHERE id = $1`, session.ID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, q, "Asia/Bangkok", nil, slog.Default())
	if err := service.Analyze(ctx, changeID); err != nil {
		t.Fatal(err)
	}

	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review')`, changeID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("past-time valid move created %d issue(s), want exactly 1 (past_time)", total)
	}
	var pastRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review') AND issue_type = 'past_time_change'`, changeID).Scan(&pastRows); err != nil {
		t.Fatal(err)
	}
	if pastRows != 1 {
		t.Errorf("past_time rows = %d, want 1", pastRows)
	}
}

// Acceptance A4: overlapping move into the past yields only the primary row.
// Timing rows add no action when the assignment is already invalid.
func TestAnalyze_overlappingPastMoveKeepsOnlyPrimaryRow(t *testing.T) {
	databaseURL := autoTestDB(t)
	autoMigrateUpOnce(t, databaseURL)
	pool := autoPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "pastclash-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "pastclash-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "PASTCLASH-" + suffix, Name: "PastClash " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	// Sit-in window yesterday; missed session centered on the POST-move window
	// (sit -1h overlaps missed middle). Truncate AFTER subtracting so both
	// fixtures share the exact same base.
	sitBase := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: sitBase, Valid: true},
		EndAt:   pgtype.Timestamptz{Time: sitBase.Add(1 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	missedRoom, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "pastclash-missed-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	missedTeacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "pastclash-missed-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	missed, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: missedRoom.ID, TeacherID: missedTeacherID,
		StartAt: pgtype.Timestamptz{Time: sitBase.Add(-30 * time.Minute), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: sitBase.Add(30 * time.Minute), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode: "PASTCLASH-" + suffix, CourseID: course.ID,
		DateFrom:      pgtype.Date{Time: sitBase, Valid: true},
		DateTo:        pgtype.Date{Time: sitBase, Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO absence_sit_ins (absence_id, session_id, session_version_at_assignment, session_snapshot_at_assignment, snapshot_schema_version, snapshot_captured_at, snapshot_quality, snapshot_source) VALUES ($1, $2, $3, '{"schema_version":1}'::jsonb, 1, now(), 'exact', 'captured_at_assignment')`, absence.ID, session.ID, session.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO absence_missed_sessions(absence_id, session_id) VALUES ($1, $2)`, absence.ID, missed.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, absence.ID); err != nil {
			t.Logf("cleanup absence: %v", err)
		}
	})

	// Effective past move: shift the window 1h earlier (still yesterday, still
	// overlapping the missed session). Production order: update row first.
	if _, err := pool.Exec(ctx, `UPDATE sessions SET start_at = start_at - interval '1 hour', end_at = end_at - interval '1 hour', version = version + 1 WHERE id = $1`, session.ID); err != nil {
		t.Fatal(err)
	}
	var changeID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO session_changes (session_id, session_version, changed_fields, before_snapshot, after_snapshot, old_start_at, old_end_at, new_start_at, new_end_at, old_course_id, new_course_id, old_teacher_id, new_teacher_id) SELECT id, version, '{"start_at":true,"end_at":true}'::jsonb, '{}'::jsonb, '{}'::jsonb, start_at + interval '1 hour', end_at + interval '1 hour', start_at, end_at, course_id, course_id, teacher_id, teacher_id FROM sessions WHERE id = $1 RETURNING id`, session.ID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, q, "Asia/Bangkok", nil, slog.Default())
	if err := service.Analyze(ctx, changeID); err != nil {
		t.Fatal(err)
	}

	var total, timing int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE issue_type IN ('short_notice_change','past_time_change')) FROM absence_schedule_issues WHERE latest_session_change_id = $1 AND status IN ('open','needs_review')`, changeID).Scan(&total, &timing); err != nil {
		t.Fatal(err)
	}
	if total != 1 || timing != 0 {
		t.Errorf("overlapping past move created total=%d timing=%d, want 1/0", total, timing)
	}
}

// Adversarial H2: timing-only concern must survive the quietening. A valid
// assignment moved to within the warning window must still yield exactly one
// short_notice row (the one actionable item: notify the student).
func TestAnalyze_nearFutureValidMoveKeepsSingleShortNotice(t *testing.T) {
	databaseURL := autoTestDB(t)
	autoMigrateUpOnce(t, databaseURL)
	pool := autoPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "urgent-teacher-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "urgent-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "URGENT-" + suffix, Name: "Urgent " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	// Session 2h in the future: inside the default 24h warning window but not past.
	base := time.Now().UTC().Truncate(time.Second).Add(2 * time.Hour)
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: base, Valid: true},
		EndAt:   pgtype.Timestamptz{Time: base.Add(1 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode: "URGENT-" + suffix, CourseID: course.ID,
		DateFrom:      pgtype.Date{Time: base, Valid: true},
		DateTo:        pgtype.Date{Time: base, Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO absence_sit_ins (
			absence_id, session_id, session_version_at_assignment,
			session_snapshot_at_assignment, snapshot_schema_version,
			snapshot_captured_at, snapshot_quality, snapshot_source
		)
		VALUES ($1, $2, $3,
		        '{"schema_version":1}'::jsonb, 1, now(), 'exact', 'captured_at_assignment')
	`, absence.ID, session.ID, session.Version); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, absence.ID); err != nil {
			t.Logf("cleanup absence: %v", err)
		}
	})

	var changeID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO session_changes (
			session_id, session_version, changed_fields, before_snapshot, after_snapshot,
			old_start_at, old_end_at, new_start_at, new_end_at,
			old_course_id, new_course_id, old_teacher_id, new_teacher_id
		)
		SELECT id, version + 1,
		       '{"start_at":true,"end_at":true}'::jsonb, '{}'::jsonb, '{}'::jsonb,
		       start_at, end_at, start_at + interval '30 minutes', end_at + interval '30 minutes',
		       course_id, course_id, teacher_id, teacher_id
		FROM sessions WHERE id = $1
		RETURNING id
	`, session.ID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, q, "Asia/Bangkok", nil, slog.Default())
	if err := service.Analyze(ctx, changeID); err != nil {
		t.Fatal(err)
	}

	var total int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM absence_schedule_issues
		WHERE latest_session_change_id = $1 AND status IN ('open','needs_review')
	`, changeID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("near-future valid move created %d issue(s), want exactly 1 (short_notice)", total)
	}
	var shortNotice int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM absence_schedule_issues
		WHERE latest_session_change_id = $1 AND status IN ('open','needs_review')
		  AND issue_type = 'short_notice_change'
	`, changeID).Scan(&shortNotice); err != nil {
		t.Fatal(err)
	}
	if shortNotice != 1 {
		t.Errorf("short_notice rows = %d, want 1", shortNotice)
	}
}
