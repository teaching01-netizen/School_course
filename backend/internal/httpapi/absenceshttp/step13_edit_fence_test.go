package absenceshttp

// Step-13 item 5: session-edit-vs-submission fence (Step-9 G2 proof).
//
// The G2 fix put SessionsLockOrdered (FOR UPDATE, immutable-ID order) inside
// every snapshot insert; the session editor (EditOccurrenceTimeTx) locks the
// same session row via schedulelock. The two writers must therefore
// serialize: an edit either precedes the submission snapshot read (the
// submission then snapshots or version-checks the NEW state) or follows the
// submission commit (the submission snapshots the OLD state, the edit bumps
// the version after). An edit can never land invisibly between the
// submission snapshot read and its insert. Both directions are exercised:
//
// A. Edit commits first: submission with ExpectedVersion of the OLD version
//    deterministically fails with SessionVersionConflictError.
// B. Submission holds the fence (session row locked) while the editor-side
//    lock attempt on the same row blocks until the submission commits
//    (proves the editor cannot slip its write into the fenced gap).
//
// The edit side uses raw SQL (version-guarded UPDATE, same predicate shape
// as the production editor re-check) to keep this package free of the
// scheduling service dependency. Production takes a strictly stronger lock
// set (course + student + teacher + room + session), so serializing here
// implies serializing there.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// Direction A: a committed edit is visible to the submission version check.
// The submission passes ExpectedVersion read BEFORE the edit; after the edit
// commits (version N -> N+1), the fenced snapshot insert must reject with
// SessionVersionConflictError carrying both versions. This is the exact
// stale-version path the staff/public/batch writers map to 409.
func TestSessionEdit_SubmissionSeesCommittedEdit(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	q := sqldb.New(dbpool)
	var teacherID, courseID, sessionID pgtype.UUID
	if err := dbpool.QueryRow(ctx, "INSERT INTO users (username, role, password_hash) VALUES ($1, $2, $3) RETURNING id", "t-edf-"+suffix, "Teacher", "x").Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, "INSERT INTO courses (code, name, level) VALUES ($1, $2, 2) RETURNING id", "EDF-"+suffix, "Edit fence").Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	var versionBefore int32
	if err := dbpool.QueryRow(ctx, "INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1, $2, $3, $4) RETURNING id, version", courseID, teacherID, start, start.Add(time.Hour)).Scan(&sessionID, &versionBefore); err != nil {
		t.Fatal(err)
	}

	// Client read the session (version N) into its form state.
	staleVersion := versionBefore

	// Editor commits a time change with the version guard (production shape:
	// UPDATE ... WHERE id=$1 AND version=$N, bump on success).
	newStart := start.Add(30 * time.Minute)
	tag, err := dbpool.Exec(ctx, "UPDATE sessions SET start_at = $2, end_at = $3, updated_at = now(), version = version + 1 WHERE id = $1 AND version = $4", sessionID, newStart, newStart.Add(time.Hour), staleVersion)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatal("editor update affected 0 rows, want 1")
	}

	// Submission arrives with the STALE expected version: the fenced insert
	// must reject deterministically (proves the edit precedes validation, it
	// cannot hide in an unchecked gap).
	var studentID, absenceID pgtype.UUID
	if err := dbpool.QueryRow(ctx, "INSERT INTO students (wcode, full_name) VALUES ($1, $2) RETURNING id", "wedf-"+suffix, "Edit fence").Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, $3)", courseID, studentID, "enrolled"); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, "INSERT INTO student_absences (wcode, course_id, date_from, date_to, reason) VALUES ((SELECT wcode FROM students WHERE id=$1), $2, $3, $3, $4) RETURNING id", studentID, courseID, "2026-11-05", "race").Scan(&absenceID); err != nil {
		t.Fatal(err)
	}
	expected := staleVersion
	err = q.AbsenceSitInsCreateWithSnapshot(ctx, absenceID, []sqldb.SitInSnapshotInput{{SessionID: sessionID, ExpectedVersion: &expected}}, "Asia/Bangkok", BuildSnapshotFromSessionRow)
	if err == nil {
		t.Fatal("stale-version submission succeeded, want SessionVersionConflictError")
	}
	var versionErr *sqldb.SessionVersionConflictError
	if !errors.As(err, &versionErr) {
		t.Fatalf("want SessionVersionConflictError, got %T: %v", err, err)
	}
	if versionErr.ExpectedVersion != int(staleVersion) || versionErr.ActualVersion != int(staleVersion+1) {
		t.Fatalf("conflict versions = expected %d actual %d, want %d/%d", versionErr.ExpectedVersion, versionErr.ActualVersion, staleVersion, staleVersion+1)
	}

	// The same submission WITHOUT a stale expectation snapshots the NEW
	// (post-edit) state: snapshot version must equal the bumped version.
	if err := q.AbsenceSitInsCreateWithSnapshot(ctx, absenceID, []sqldb.SitInSnapshotInput{{SessionID: sessionID}}, "Asia/Bangkok", BuildSnapshotFromSessionRow); err != nil {
		t.Fatal(err)
	}
	var snapVersion int32
	if err := dbpool.QueryRow(ctx, "SELECT session_version_at_assignment FROM absence_sit_ins WHERE absence_id = $1 AND session_id = $2", absenceID, sessionID).Scan(&snapVersion); err != nil {
		t.Fatal(err)
	}
	if snapVersion != staleVersion+1 {
		t.Fatalf("snapshot version = %d, want post-edit %d", snapVersion, staleVersion+1)
	}
}

// Direction B: an in-flight submission fence blocks the editor-side row lock.
// tx1 runs the submission fence (SessionsLockOrdered, the exact shared call)
// and holds it open; a concurrent editor-side lock attempt on the same row
// must block until tx1 commits. The 500ms probe below is a liveness probe
// only (see the merge-serialization test for the rationale).
func TestSessionEdit_EditorBlocksOnSubmissionFence(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	q := sqldb.New(dbpool)
	var teacherID, courseID, sessionID pgtype.UUID
	if err := dbpool.QueryRow(ctx, "INSERT INTO users (username, role, password_hash) VALUES ($1, $2, $3) RETURNING id", "t-edb-"+suffix, "Teacher", "x").Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, "INSERT INTO courses (code, name, level) VALUES ($1, $2, 2) RETURNING id", "EDB-"+suffix, "Edit block").Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	if err := dbpool.QueryRow(ctx, "INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1, $2, $3, $4) RETURNING id", courseID, teacherID, start, start.Add(time.Hour)).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}

	tx1, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback(ctx)
	if _, err := q.WithTx(tx1).SessionsLockOrdered(ctx, []pgtype.UUID{sessionID}); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		tx2, err := dbpool.Begin(cctx)
		if err != nil {
			done <- err
			return
		}
		defer tx2.Rollback(cctx)
		if _, err := q.WithTx(tx2).SessionsLockOrdered(cctx, []pgtype.UUID{sessionID}); err != nil {
			done <- err
			return
		}
		done <- tx2.Commit(cctx)
	}()

	select {
	case err := <-done:
		t.Fatalf("editor-side lock finished while submission holds the fence: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("editor-side lock failed after fence release: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("editor-side lock did not finish after fence release")
	}
}
