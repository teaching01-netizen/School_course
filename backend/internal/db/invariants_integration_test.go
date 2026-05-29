package db

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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func requireTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

var migrationsOnce sync.Once
var migrationsErr error

func migrateUpOnce(t *testing.T, databaseURL string) {
	t.Helper()
	migrationsOnce.Do(func() {
		// Supabase pooler / PgBouncer can break prepared statements; ensure stdlib driver uses simple protocol.
		// See pgx DSN params: default_query_exec_mode=simple_protocol, statement_cache_capacity=0
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErr = context.Canceled
			return
		}
		// This file lives at backend/internal/db/*.go; migrations live at backend/db/migrations.
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		migrationsErr = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErr != nil {
		t.Fatal(migrationsErr)
	}
}

func newPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	// Supabase pooler / PgBouncer can break prepared statements; use simple protocol for tests.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestStudentBusyRanges_IncludeExclude(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher1-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "R1-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "C1-" + suffix, Name: "Course 1"})
	if err != nil {
		t.Fatal(err)
	}
	s1, err := q.StudentCreate(ctx, StudentCreateParams{Wcode: "W0001-" + suffix, FullName: "A", Notes: ""})
	if err != nil {
		t.Fatal(err)
	}
	s2, err := q.StudentCreate(ctx, StudentCreateParams{Wcode: "W0002-" + suffix, FullName: "B", Notes: ""})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: course.ID, StudentID: s1.ID}); err != nil {
		t.Fatal(err)
	}

	start := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true}
	end := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true}
	session, err := q.SessionCreate(ctx, SessionCreateParams{
		SeriesID:  pgtype.UUID{},
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   start,
		EndAt:     end,
	})
	if err != nil {
		t.Fatal(err)
	}

	var count int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM student_busy_ranges WHERE session_id = $1 AND deleted_at IS NULL`, session.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 busy range, got %d", count)
	}

	// Exclude roster student -> busy ranges should drop to 0.
	if err := q.SessionAttendanceUpsert(ctx, SessionAttendanceUpsertParams{SessionID: session.ID, StudentID: s1.ID, Status: "excluded"}); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM student_busy_ranges WHERE session_id = $1 AND deleted_at IS NULL`, session.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 busy ranges after exclusion, got %d", count)
	}

	// Include non-roster student -> busy ranges should become 1 for that student.
	if err := q.SessionAttendanceUpsert(ctx, SessionAttendanceUpsertParams{SessionID: session.ID, StudentID: s2.ID, Status: "included"}); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM student_busy_ranges WHERE session_id = $1 AND deleted_at IS NULL`, session.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 busy range after include override, got %d", count)
	}
}

func TestStudentOverlap_ConstraintBlocksConcurrentWrites(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher2-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "R2-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "C2-" + suffix, Name: "Course 2"})
	if err != nil {
		t.Fatal(err)
	}
	stu, err := q.StudentCreate(ctx, StudentCreateParams{Wcode: "W0100-" + suffix, FullName: "S", Notes: ""})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: course.ID, StudentID: stu.ID}); err != nil {
		t.Fatal(err)
	}

	start1 := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), Valid: true}
	end1 := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 13, 0, 0, 0, time.UTC), Valid: true}
	start2 := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 12, 30, 0, 0, time.UTC), Valid: true}
	end2 := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 13, 30, 0, 0, time.UTC), Valid: true}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, e := q.SessionCreate(ctx, SessionCreateParams{
			SeriesID:  pgtype.UUID{},
			CourseID:  course.ID,
			RoomID:    room.ID,
			TeacherID: teacherID,
			StartAt:   start1,
			EndAt:     end1,
		})
		errs[0] = e
	}()
	go func() {
		defer wg.Done()
		_, e := q.SessionCreate(ctx, SessionCreateParams{
			SeriesID:  pgtype.UUID{},
			CourseID:  course.ID,
			RoomID:    room.ID,
			TeacherID: teacherID,
			StartAt:   start2,
			EndAt:     end2,
		})
		errs[1] = e
	}()
	wg.Wait()

	success := 0
	for _, e := range errs {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("expected exactly 1 successful insert, got %d (errs=%v)", success, errs)
	}
}

func TestCourseStudentRosterEdit_BoundedTime(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher3-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "R3-" + suffix, Capacity: pgtype.Int4{Int32: 50, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "C3-" + suffix, Name: "Large Course"})
	if err != nil {
		t.Fatal(err)
	}
	student, err := q.StudentCreate(ctx, StudentCreateParams{Wcode: "WSEED-" + suffix, FullName: "Seed Student", Notes: ""})
	if err != nil {
		t.Fatal(err)
	}

	// Seed 200 sessions for the course using direct SQL to bypass per-session trigger overhead.
	// Each SessionCreate triggers availability checks + busy range refresh, which adds up
	// across many sequential calls. Bulk INSERT is faster but we use individual INSERTs
	// to exercise the session-level trigger as well.
	baseDate := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC) // Monday
	sessionCount := 200
	for i := 0; i < sessionCount; i++ {
		dayOffset := i / 2 // 2 sessions per day
		day := baseDate.AddDate(0, 0, dayOffset)
		var startAt, endAt time.Time
		if i%2 == 0 {
			startAt = day.Add(9 * time.Hour)
			endAt = day.Add(10*time.Hour + 30*time.Minute)
		} else {
			startAt = day.Add(11 * time.Hour)
			endAt = day.Add(12*time.Hour + 30*time.Minute)
		}
		_, err := dbpool.Exec(ctx, `INSERT INTO sessions (course_id, room_id, teacher_id, start_at, end_at) VALUES ($1, $2, $3, $4, $5)`, course.ID, room.ID, teacherID, startAt, endAt)
		if err != nil {
			t.Fatalf("failed to seed session %d: %v", i, err)
		}
	}

	// Verify the sessions exist.
	var totalSessions int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE course_id = $1 AND deleted_at IS NULL`, course.ID).Scan(&totalSessions); err != nil {
		t.Fatal(err)
	}
	if totalSessions != sessionCount {
		t.Fatalf("expected %d sessions, got %d", sessionCount, totalSessions)
	}

	// ---- Test INSERT (add student to course roster) ----
	startInsert := time.Now()
	if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: course.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}
	elapsedInsert := time.Since(startInsert)

	// Verify correct number of busy ranges were created (one per session).
	var busyCount int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM student_busy_ranges WHERE student_id = $1 AND deleted_at IS NULL`, student.ID).Scan(&busyCount); err != nil {
		t.Fatal(err)
	}
	if busyCount != sessionCount {
		t.Fatalf("expected %d busy ranges after roster add, got %d", sessionCount, busyCount)
	}

	// ---- Test DELETE (remove student from course roster) ----
	startDelete := time.Now()
	if err := q.CourseStudentRemove(ctx, CourseStudentRemoveParams{CourseID: course.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}
	elapsedDelete := time.Since(startDelete)

	// Verify busy ranges were removed (soft-deleted).
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM student_busy_ranges WHERE student_id = $1 AND deleted_at IS NULL`, student.ID).Scan(&busyCount); err != nil {
		t.Fatal(err)
	}
	if busyCount != 0 {
		t.Fatalf("expected 0 busy ranges after roster remove, got %d", busyCount)
	}

	// Verify soft-deleted rows still exist (audit trail).
	var softDeletedCount int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM student_busy_ranges WHERE student_id = $1 AND deleted_at IS NOT NULL`, student.ID).Scan(&softDeletedCount); err != nil {
		t.Fatal(err)
	}
	if softDeletedCount != sessionCount {
		t.Fatalf("expected %d soft-deleted busy ranges, got %d", sessionCount, softDeletedCount)
	}

	// Assert bounded time: each operation on 200 sessions should complete
	// comfortably within 5 seconds (typically <500ms with incremental approach).
	// The old full-refresh approach would take O(N*R) and likely exceed this.
	const maxElapsed = 5 * time.Second
	if elapsedInsert > maxElapsed {
		t.Errorf("roster add (INSERT) took %v, exceeds %v bound", elapsedInsert, maxElapsed)
	}
	if elapsedDelete > maxElapsed {
		t.Errorf("roster remove (DELETE) took %v, exceeds %v bound", elapsedDelete, maxElapsed)
	}

	t.Logf("roster add took %v, roster remove took %v (%d sessions)", elapsedInsert, elapsedDelete, sessionCount)

	// ---- Verify excluded override works with incremental trigger ----
	// Re-add student, then mark one session as explicitly excluded.
	// The incremental INSERT trigger should skip sessions where the student
	// is excluded, but this exclusion was added AFTER the busy ranges were created.
	// The session_attendance trigger handles the removal for that one session.
	if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: course.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	// Get one session to test exclusion on.
	var targetSessionID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `SELECT id FROM sessions WHERE course_id = $1 AND deleted_at IS NULL ORDER BY start_at ASC LIMIT 1`, course.ID).Scan(&targetSessionID); err != nil {
		t.Fatal(err)
	}

	// Exclude the student from that specific session.
	if err := q.SessionAttendanceUpsert(ctx, SessionAttendanceUpsertParams{SessionID: targetSessionID, StudentID: student.ID, Status: "excluded"}); err != nil {
		t.Fatal(err)
	}

	// Verify that session is now excluded (busy range soft-deleted by session_attendance trigger).
	var excludedCount int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM student_busy_ranges WHERE session_id = $1 AND student_id = $2 AND deleted_at IS NULL`, targetSessionID, student.ID).Scan(&excludedCount); err != nil {
		t.Fatal(err)
	}
	if excludedCount != 0 {
		t.Errorf("expected 0 busy ranges after exclusion override, got %d", excludedCount)
	}

	// Now simulate the incremental INSERT trigger firing for a session where
	// the student is excluded: remove and re-add the student. The INSERT trigger
	// should NOT insert a busy range for the excluded session.
	if err := q.CourseStudentRemove(ctx, CourseStudentRemoveParams{CourseID: course.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: course.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	// Verify the excluded session still has no busy range for this student.
	var afterReAdd int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM student_busy_ranges WHERE session_id = $1 AND student_id = $2 AND deleted_at IS NULL`, targetSessionID, student.ID).Scan(&afterReAdd); err != nil {
		t.Fatal(err)
	}
	if afterReAdd != 0 {
		t.Errorf("expected 0 busy ranges after re-add with exclusion in place, got %d", afterReAdd)
	}

	// Verify other sessions got their busy ranges back.
	var otherCount int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM student_busy_ranges WHERE student_id = $1 AND deleted_at IS NULL`, student.ID).Scan(&otherCount); err != nil {
		t.Fatal(err)
	}
	if otherCount != sessionCount-1 {
		t.Errorf("expected %d busy ranges (all sessions except excluded one), got %d", sessionCount-1, otherCount)
	}

	// Cleanup: remove student again.
	if err := q.CourseStudentRemove(ctx, CourseStudentRemoveParams{CourseID: course.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}
}
