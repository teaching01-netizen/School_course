package absenceshttp

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
)

var (
	migrationsOnceMgmt sync.Once
	migrationsErrMgmt  error
)

func requireTestDBMgmt(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

func migrateUpOnceMgmt(t *testing.T, databaseURL string) {
	t.Helper()
	migrationsOnceMgmt.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErrMgmt = err
			return
		}
		defer db.Close()
		_, _ = db.Exec(`DELETE FROM crm_rows`)
		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErrMgmt = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErrMgmt = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		migrationsErrMgmt = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErrMgmt != nil {
		t.Fatal(migrationsErrMgmt)
	}
}

func newPoolMgmt(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

// seedAbsenceDeleteTestData inserts a student, subject, course, enrolment,
// a session, and a student_absence row. Returns the absence ID and student wcode.
func seedAbsenceDeleteTestData(t *testing.T, dbpool *pgxpool.Pool, q *sqldb.Queries, suffix string) (absenceID pgtype.UUID, wcode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wcode = "w" + suffix

	_, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, $2)`,
		wcode, "Test Student "+suffix)
	if err != nil {
		t.Fatal(err)
	}

	subject, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{
		Code: "SUBJ-" + suffix, Name: "Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	var courseID pgtype.UUID
	err = dbpool.QueryRow(ctx,
		`INSERT INTO courses (code, name, subject_id) VALUES ($1, $2, $3) RETURNING id`,
		"COURSE-"+suffix, "Course "+suffix, subject.ID,
	).Scan(&courseID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = dbpool.Exec(ctx,
		`INSERT INTO course_students (course_id, student_id, status)
		 SELECT $1, s.id, 'enrolled' FROM students s WHERE s.wcode = $2`,
		courseID, wcode)
	if err != nil {
		t.Fatal(err)
	}

	var teacherID pgtype.UUID
	err = dbpool.QueryRow(ctx,
		`INSERT INTO users (username, role, password_hash)
		 VALUES ($1, $2, $3) RETURNING id`,
		"teacher-"+suffix, "Teacher", "x").Scan(&teacherID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = dbpool.Exec(ctx,
		`INSERT INTO sessions (course_id, teacher_id, start_at, end_at)
		 VALUES ($1, $2, now(), now() + interval '1 hour')`,
		courseID, teacherID)
	if err != nil {
		t.Fatal(err)
	}

	err = dbpool.QueryRow(ctx, `
		INSERT INTO student_absences (wcode, course_id, date_from, date_to, status, reason)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, wcode, courseID, "2026-06-01", "2026-06-01", "cancelled", "Test absence").Scan(&absenceID)
	if err != nil {
		t.Fatal(err)
	}

	return absenceID, wcode
}

func TestAbsenceHardDelete_CancelledAbsenceDeletedWithStaleVersion(t *testing.T) {
	databaseURL := requireTestDBMgmt(t)
	migrateUpOnceMgmt(t, databaseURL)
	dbpool := newPoolMgmt(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.New().String()[:8]

	absenceID, _ := seedAbsenceDeleteTestData(t, dbpool, q, suffix)

	// Fetch the current version from the DB
	ctx := context.Background()
	var dbVersion int32
	err := dbpool.QueryRow(ctx, `SELECT version FROM student_absences WHERE id = $1`, absenceID).Scan(&dbVersion)
	if err != nil {
		t.Fatal(err)
	}

	// Attempt to hard-delete with a stale version (dbVersion + 1) — should succeed
	// because the absence is already cancelled (our fix allows this).
	rows, err := q.AbsenceHardDelete(ctx, absenceID, dbVersion+999)
	if err != nil {
		t.Fatalf("AbsenceHardDelete with stale version on cancelled absence should succeed, got: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 row deleted, got %d", rows)
	}

	// Verify the row is actually gone
	var count int32
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*)::int4 FROM student_absences WHERE id = $1`, absenceID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("absence row was not actually deleted")
	}
}

func TestAbsenceHardDelete_NonCancelledAbsenceRejectsStaleVersion(t *testing.T) {
	databaseURL := requireTestDBMgmt(t)
	migrateUpOnceMgmt(t, databaseURL)
	dbpool := newPoolMgmt(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.New().String()[:8]

	// Seed with a non-cancelled absence (pending status)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wcode := "w" + suffix
	_, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, $2)`, wcode, "Student "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{Code: "SUBJ-" + suffix, Name: "Subject"})
	if err != nil {
		t.Fatal(err)
	}
	var courseID pgtype.UUID
	err = dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, subject_id) VALUES ($1,$2,$3) RETURNING id`,
		"CRS-"+suffix, "Course", subject.ID).Scan(&courseID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = dbpool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) SELECT $1, s.id, 'enrolled' FROM students s WHERE s.wcode=$2`,
		courseID, wcode)
	if err != nil {
		t.Fatal(err)
	}
	var absenceID pgtype.UUID
	err = dbpool.QueryRow(ctx, `
		INSERT INTO student_absences (wcode, course_id, date_from, date_to, status, reason)
		VALUES ($1,$2,$3,$4,'pending','Test') RETURNING id
	`, wcode, courseID, "2026-06-01", "2026-06-01").Scan(&absenceID)
	if err != nil {
		t.Fatal(err)
	}

	var dbVersion int32
	err = dbpool.QueryRow(ctx, `SELECT version FROM student_absences WHERE id = $1`, absenceID).Scan(&dbVersion)
	if err != nil {
		t.Fatal(err)
	}

	// Attempting to hard-delete with a stale version should fail (pgx.ErrNoRows)
	_, err = q.AbsenceHardDelete(ctx, absenceID, dbVersion+999)
	if !sqldb.IsNoRows(err) {
		t.Fatalf("AbsenceHardDelete with stale version on pending absence should return ErrNoRows, got: %v", err)
	}

	// Verify the row still exists
	var count int32
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*)::int4 FROM student_absences WHERE id = $1`, absenceID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("absence row should still exist after rejected delete")
	}
}

func TestAbsenceHardDelete_DeletedWithCorrectVersion(t *testing.T) {
	databaseURL := requireTestDBMgmt(t)
	migrateUpOnceMgmt(t, databaseURL)
	dbpool := newPoolMgmt(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.New().String()[:8]

	absenceID, _ := seedAbsenceDeleteTestData(t, dbpool, q, suffix)

	ctx := context.Background()
	var dbVersion int32
	err := dbpool.QueryRow(ctx, `SELECT version FROM student_absences WHERE id = $1`, absenceID).Scan(&dbVersion)
	if err != nil {
		t.Fatal(err)
	}

	// Delete with the correct version — should succeed
	rows, err := q.AbsenceHardDelete(ctx, absenceID, dbVersion)
	if err != nil {
		t.Fatalf("AbsenceHardDelete with correct version should succeed, got: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 row deleted, got %d", rows)
	}

	var count int32
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*)::int4 FROM student_absences WHERE id = $1`, absenceID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("absence row was not actually deleted")
	}
}
