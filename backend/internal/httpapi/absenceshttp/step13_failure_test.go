package absenceshttp

// Step-13 failure injection: failure-before-commit leaves no partial domain
// records (plan items: failures before writes, between writes, before
// commit).
//
// Every absence writer runs inside ONE WithIdempotentTx transaction and
// returns fnErr on any write failure, which rolls the tx back. These tests
// drive the same query functions the writers call inside an explicit tx,
// inject a failure at each point, roll back, and assert the committed
// database holds NEITHER the absence row NOR any assignment row:
//
//  1. TestFailure_BetweenWrites_NoPartials: absence row inserted, then the
//     sit-in insert fails (garbage session id violates the FK) -> rollback.
//  2. TestFailure_BeforeCommit_NoPartials: all writes succeed but the tx is
//     never committed (rollback before commit) -> nothing durable.
//  3. TestFailure_IdempotencyAcquireRollback_NoKeyHeld: idempotency Acquire
//     inside a rolled-back tx holds no key (a retry acquires as new).
//
// All assertions read committed state AFTER rollback, never inside the
// aborted tx.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
)

func seedFailureWorld(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool, courseID pgtype.UUID) (studentID, sessionID pgtype.UUID, wcode string) {
	t.Helper()
	suffix := uuid.NewString()[:8]
	wcode = "wfail-" + suffix
	if err := dbpool.QueryRow(ctx, "INSERT INTO students (wcode, full_name) VALUES ($1, $2) RETURNING id", wcode, "Fail "+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, $3)", courseID, studentID, "enrolled"); err != nil {
		t.Fatal(err)
	}
	var teacherID pgtype.UUID
	if err := dbpool.QueryRow(ctx, "INSERT INTO users (username, role, password_hash) VALUES ($1, $2, $3) RETURNING id", "t-fail-"+suffix, "Teacher", "x").Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	if err := dbpool.QueryRow(ctx, "INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1, $2, $3, $4) RETURNING id", courseID, teacherID, start, start.Add(time.Hour)).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	return studentID, sessionID, wcode
}

func mkFailureCourse(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool) pgtype.UUID {
	t.Helper()
	suffix := uuid.NewString()[:8]
	var courseID pgtype.UUID
	if err := dbpool.QueryRow(ctx, "INSERT INTO courses (code, name, level) VALUES ($1, $2, 2) RETURNING id", "FAIL-"+suffix, "Fail course").Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	return courseID
}

func countFailureRows(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool, wcode string, absenceID pgtype.UUID) (absences, sitIns int) {
	t.Helper()
	if err := dbpool.QueryRow(ctx, "SELECT count(*) FROM student_absences WHERE wcode = $1", wcode).Scan(&absences); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, "SELECT count(*) FROM absence_sit_ins WHERE absence_id = $1", absenceID).Scan(&sitIns); err != nil {
		t.Fatal(err)
	}
	return absences, sitIns
}

// Failure BETWEEN the absence insert and the assignment insert.
func TestFailure_BetweenWrites_NoPartials(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	q := sqldb.New(dbpool)
	courseID := mkFailureCourse(t, ctx, dbpool)
	_, _, wcode := seedFailureWorld(t, ctx, dbpool, courseID)

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	qtx := q.WithTx(tx)

	row, err := qtx.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode:    wcode,
		CourseID: courseID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Injected failure: garbage session id violates the sit-in FK.
	garbage := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	injectErr := qtx.AbsenceSitInsCreateWithSnapshot(ctx, row.ID, []sqldb.SitInSnapshotInput{{SessionID: garbage}}, "Asia/Bangkok", BuildSnapshotFromSessionRow)
	if injectErr == nil {
		t.Fatal("injected garbage-session insert succeeded, want FK failure")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	absences, sitIns := countFailureRows(t, ctx, dbpool, wcode, row.ID)
	if absences != 0 || sitIns != 0 {
		t.Fatalf("between-writes failure left partials: absences=%d sit_ins=%d, want 0/0", absences, sitIns)
	}
}

// All writes succeed but the tx never commits.
func TestFailure_BeforeCommit_NoPartials(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	q := sqldb.New(dbpool)
	courseID := mkFailureCourse(t, ctx, dbpool)
	_, sessionID, wcode := seedFailureWorld(t, ctx, dbpool, courseID)

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	qtx := q.WithTx(tx)

	row, err := qtx.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode:    wcode,
		CourseID: courseID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := qtx.AbsenceSitInsCreateWithSnapshot(ctx, row.ID, []sqldb.SitInSnapshotInput{{SessionID: sessionID}}, "Asia/Bangkok", BuildSnapshotFromSessionRow); err != nil {
		t.Fatal(err)
	}
	// Never commit: roll back a fully-written tx.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	absences, sitIns := countFailureRows(t, ctx, dbpool, wcode, row.ID)
	if absences != 0 || sitIns != 0 {
		t.Fatalf("before-commit rollback left partials: absences=%d sit_ins=%d, want 0/0", absences, sitIns)
	}
}

// An idempotency Acquire inside a rolled-back tx must not hold the key:
// the wrapper begins-then-rolls-back on fnErr, so a client retry with the
// same key must acquire as NEW (no stale-record conflict, no phantom
// ownership). Failure-before-Complete leaves no key row at all because
// Acquire ran inside the aborted tx.
func TestFailure_IdempotencyAcquireRollback_NoKeyHeld(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	q := sqldb.New(dbpool)
	actorID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	scope := "test-failure-scope"
	key := "test-failure-key-" + uuid.NewString()
	expiry := pgtype.Timestamptz{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true}

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := q.WithTx(tx).IdempotencyAcquire(ctx, sqldb.IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    "hash-failure",
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !acquired.IsNew {
		t.Fatal("first acquire in tx must be new")
	}
	// fnErr path: roll back before Complete.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	retry, err := q.IdempotencyAcquire(ctx, sqldb.IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    "hash-failure",
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !retry.IsNew {
		t.Fatal("retry after rollback must acquire as new (no key held)")
	}
	if retry.StatusCode != nil {
		t.Fatalf("retry status = %v, want nil (no phantom completion)", retry.StatusCode)
	}
}
