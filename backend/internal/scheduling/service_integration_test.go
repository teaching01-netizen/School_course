package scheduling

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/series"
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
		// This file lives at backend/internal/scheduling/*.go; migrations live at backend/db/migrations.
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

func newTestService(t *testing.T, dbpool *pgxpool.Pool) *Service {
	t.Helper()
	seriesSvc, err := series.NewService(dbpool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(dbpool, "Asia/Bangkok", seriesSvc)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestCreateSession_RoomOverlap_ReturnsExplainableDetails(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-overlap-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-overlap-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-overlap-" + suffix, Name: "Course overlap"})
	if err != nil {
		t.Fatal(err)
	}

	start1 := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true}
	end1 := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true}
	created1, err := svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   start1,
		EndAt:     end1,
	})
	if err != nil {
		t.Fatal(err)
	}

	start2 := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC), Valid: true}
	end2 := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 30, 0, 0, time.UTC), Valid: true}
	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   start2,
		EndAt:     end2,
	})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	var se *Err
	if !errors.As(err, &se) {
		t.Fatalf("expected *scheduling.Err, got %T (%v)", err, err)
	}
	if se.Code != "schedule_conflict" {
		t.Fatalf("expected code schedule_conflict, got %q", se.Code)
	}
	if se.Details.Kind != ConflictKindRoomOverlap {
		t.Fatalf("expected kind %q, got %q", ConflictKindRoomOverlap, se.Details.Kind)
	}
	if len(se.Details.Conflicts) == 0 {
		t.Fatal("expected conflicts, got none")
	}
	if se.Details.Conflicts[0].SessionID == "" {
		t.Fatal("expected conflict session_id")
	}
	if se.Details.Conflicts[0].SessionID == "" || se.Details.Conflicts[0].CourseID == "" {
		t.Fatal("expected populated conflict session fields")
	}
	if created1.SessionID.Valid {
		// Ensure the first session is among conflicts (typically the only one).
		want, err := uuidString(created1.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, c := range se.Details.Conflicts {
			if c.SessionID == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected conflicts to include first session %s", want)
		}
	}
}

func TestCreateSession_TeacherAvailabilityViolation_ReturnsExplainableDetails(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-avail-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-avail-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-avail-" + suffix, Name: "Course avail"})
	if err != nil {
		t.Fatal(err)
	}

	// Create a narrow teacher availability window that does NOT cover the requested session.
	_, err = q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	start := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true}
	end := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true}
	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   start,
		EndAt:     end,
	})
	if err == nil {
		t.Fatal("expected availability violation error, got nil")
	}
	var se *Err
	if !errors.As(err, &se) {
		t.Fatalf("expected *scheduling.Err, got %T (%v)", err, err)
	}
	if se.Code != "availability_violation" {
		t.Fatalf("expected code availability_violation, got %q", se.Code)
	}
	if se.Details.Kind != ConflictKindTeacherAvailability {
		t.Fatalf("expected kind %q, got %q", ConflictKindTeacherAvailability, se.Details.Kind)
	}
}

func TestExplainFromDBErrByRepreflight_OnExclusion_ReturnsExplainableDetails(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-explain-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-explain-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-explain-" + suffix, Name: "Course explain"})
	if err != nil {
		t.Fatal(err)
	}

	// Create an existing session to overlap with the candidate slot.
	start1 := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true}
	end1 := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true}
	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   start1,
		EndAt:     end1,
	})
	if err != nil {
		t.Fatal(err)
	}

	courseIDStr, err := uuidString(course.ID)
	if err != nil {
		t.Fatal(err)
	}
	roomIDStr, err := uuidString(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	teacherIDStr, err := uuidString(teacherID)
	if err != nil {
		t.Fatal(err)
	}

	candidate := preflightInput{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartUTC:  time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC),
		EndUTC:    time.Date(2026, 5, 20, 11, 30, 0, 0, time.UTC),
		Requested: ConflictRequested{
			StartAt:   time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC).Format(time.RFC3339Nano),
			EndAt:     time.Date(2026, 5, 20, 11, 30, 0, 0, time.UTC).Format(time.RFC3339Nano),
			CourseID:  courseIDStr,
			RoomID:    &roomIDStr,
			TeacherID: teacherIDStr,
			SeriesID:  nil,
		},
	}

	// Simulate a DB exclusion constraint violation; repreflight should recover explainable details.
	se := svc.explainFromDBErrByRepreflight(ctx, &pgconn.PgError{Code: "23P01"}, []preflightInput{candidate})
	if se == nil {
		t.Fatal("expected explainable error, got nil")
	}
	if se.Code != "schedule_conflict" {
		t.Fatalf("expected code schedule_conflict, got %q", se.Code)
	}
	if se.Details.Kind != ConflictKindRoomOverlap {
		t.Fatalf("expected kind %q, got %q", ConflictKindRoomOverlap, se.Details.Kind)
	}
	if len(se.Details.Conflicts) == 0 {
		t.Fatal("expected conflicts, got none")
	}
}

func TestCancelSeries_ThisAndFuture_CancelsFutureSessionsAndClampsSeries(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-cancel-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-cancel-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-cancel-" + suffix, Name: "Course cancel"})
	if err != nil {
		t.Fatal(err)
	}

	startDate := LocalDate{Year: 2026, Month: 5, Day: 19}
	endDate := LocalDate{Year: 2026, Month: 5, Day: 25}
	createRes, err := svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
		CourseID:        course.ID,
		RoomID:          room.ID,
		TeacherID:       teacherID,
		Weekdays:        []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday},
		StartLocalTime:  Clock{Hour: 10, Minute: 0},
		DurationMinutes: 60,
		StartDate:       startDate,
		EndDate:         &endDate,
	})
	if err != nil {
		t.Fatal(err)
	}

	seriesID := createRes.SeriesID
	ser, err := q.SeriesGetByID(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}

	rangeStart := pgtype.Timestamptz{Time: time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC), Valid: true}
	rangeEnd := pgtype.Timestamptz{Time: time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC), Valid: true}

	before, err := q.SessionListActiveByRange(ctx, sqldb.SessionListActiveByRangeParams{RangeStart: rangeStart, RangeEnd: rangeEnd})
	if err != nil {
		t.Fatal(err)
	}
	beforeCount := 0
	for _, srow := range before {
		if srow.SeriesID.Valid && srow.SeriesID.Bytes == seriesID.Bytes {
			beforeCount++
		}
	}
	if beforeCount != 7 {
		t.Fatalf("expected 7 active sessions before cancel, got %d", beforeCount)
	}

	pivot := LocalDate{Year: 2026, Month: 5, Day: 22}
	cancelRes, err := svc.CancelSeries(ctx, CancelSeriesParams{
		SeriesID:        seriesID,
		Scope:           CancelScopeThisAndFuture,
		PivotDate:       &pivot,
		ExpectedVersion: ser.Version,
		NowUTC:          time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cancelRes.SessionsCanceled != 4 {
		t.Fatalf("expected to cancel 4 sessions, got %d", cancelRes.SessionsCanceled)
	}

	after, err := q.SessionListActiveByRange(ctx, sqldb.SessionListActiveByRangeParams{RangeStart: rangeStart, RangeEnd: rangeEnd})
	if err != nil {
		t.Fatal(err)
	}
	afterCount := 0
	for _, srow := range after {
		if srow.SeriesID.Valid && srow.SeriesID.Bytes == seriesID.Bytes {
			afterCount++
		}
	}
	if afterCount != 3 {
		t.Fatalf("expected 3 active sessions after cancel, got %d", afterCount)
	}
}

func TestCancelSeries_EntireSeriesFutureOnly_CancelsAtLeastOneFutureSession(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-cancel2-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-cancel2-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-cancel2-" + suffix, Name: "Course cancel2"})
	if err != nil {
		t.Fatal(err)
	}

	startDate := LocalDate{Year: 2026, Month: 5, Day: 19}
	endDate := LocalDate{Year: 2026, Month: 5, Day: 25}
	createRes, err := svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
		CourseID:        course.ID,
		RoomID:          room.ID,
		TeacherID:       teacherID,
		Weekdays:        []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday},
		StartLocalTime:  Clock{Hour: 10, Minute: 0},
		DurationMinutes: 60,
		StartDate:       startDate,
		EndDate:         &endDate,
	})
	if err != nil {
		t.Fatal(err)
	}

	seriesID := createRes.SeriesID
	ser, err := q.SeriesGetByID(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}

	nowUTC := time.Date(2026, 5, 21, 4, 0, 0, 0, time.UTC) // 11:00 Bangkok
	cancelRes, err := svc.CancelSeries(ctx, CancelSeriesParams{
		SeriesID:        seriesID,
		Scope:           CancelScopeEntireSeriesFutureOnly,
		ExpectedVersion: ser.Version,
		NowUTC:          nowUTC,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cancelRes.SessionsCanceled == 0 {
		t.Fatal("expected to cancel at least one future session")
	}
}

func TestPreflight_ExplicitEmptyRosterDoesNotFallbackToCourse(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	// Teacher A: for course A being preflighted.
	teacherA, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-empty-roster-A-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// Teacher B: for course B's busy-range session — different teacher to avoid teacher overlap.
	teacherB, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-empty-roster-B-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// Room A: for the course being preflighted.
	roomA, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-empty-A-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	// Room B: for the busy-range session (different room).
	roomB, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-empty-B-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-empty-roster-" + suffix, Name: "Course empty roster"})
	if err != nil {
		t.Fatal(err)
	}

	// Create availability windows for both teachers.
	_, err = q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacherA,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacherB,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a student and add them to the course roster.
	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: "S-EMPTY-" + suffix, FullName: "Student Empty"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: course.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	// Create a session in a DIFFERENT course (courseB) with the same student in its roster.
	// Uses teacher B and room B — no overlap with teacher A's preflight on room A.
	// This creates a busy range for the student at 10-11.
	courseB, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-empty-B-" + suffix, Name: "Course B"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: courseB.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  courseB.ID,
		RoomID:    roomB.ID,
		TeacherID: teacherB,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	courseIDStr, err := uuidString(course.ID)
	if err != nil {
		t.Fatal(err)
	}
	roomAStr, err := uuidString(roomA.ID)
	if err != nil {
		t.Fatal(err)
	}
	teacherAStr, err := uuidString(teacherA)
	if err != nil {
		t.Fatal(err)
	}

	slotStart := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true}
	slotEnd := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true}

	requested := ConflictRequested{
		StartAt:   slotStart.Time.UTC().Format(time.RFC3339Nano),
		EndAt:     slotEnd.Time.UTC().Format(time.RFC3339Nano),
		CourseID:  courseIDStr,
		RoomID:    &roomAStr,
		TeacherID: teacherAStr,
	}

	// Case 1: nil StudentIDs (use course roster) — should detect student_overlap because the course
	// roster student has a busy range at this time (from courseB's session). Room A is free,
	// teacher A is free, so preflight reaches the student overlap check.
	_, se, err := svc.Preflight(ctx, PreflightParams{
		CourseID:   course.ID,
		RoomID:     roomA.ID,
		TeacherID:  teacherA,
		StartAt:    slotStart,
		EndAt:      slotEnd,
		StudentIDs: nil,
		Requested:  requested,
	})
	if err != nil {
		t.Fatal(err)
	}
	if se == nil {
		t.Fatal("expected conflict with course-roster student overlap, got nil")
	}
	if se.Details.Kind != ConflictKindStudentOverlap {
		t.Fatalf("expected student_overlap kind with nil roster, got %q", se.Details.Kind)
	}

	// Case 2: explicit empty StudentIDs — should succeed because empty roster means no student overlap checks.
	explicitEmpty := make([]pgtype.UUID, 0)
	res, se, err := svc.Preflight(ctx, PreflightParams{
		CourseID:   course.ID,
		RoomID:     roomA.ID,
		TeacherID:  teacherA,
		StartAt:    slotStart,
		EndAt:      slotEnd,
		StudentIDs: &explicitEmpty,
		Requested:  requested,
	})
	if err != nil {
		t.Fatal(err)
	}
	if se != nil {
		t.Fatalf("expected no conflict with explicit empty roster, got: %s", se.Message)
	}
	if res.Status != "available" {
		t.Fatalf("expected status available, got %q", res.Status)
	}
}

func TestPreflight_IncludedNonRosterStudentChecked(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	// Teacher A: for course A being preflighted.
	teacherA, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-non-roster-A-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// Teacher B: for course B that creates student B's busy range — different teacher to avoid teacher overlap.
	teacherB, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-non-roster-B-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// Room A: for course A being preflighted.
	roomA, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-non-roster-A-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	// Room B: for course B's session — different room to avoid room overlap.
	roomB, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-non-roster-B-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-non-roster-" + suffix, Name: "Course non-roster"})
	if err != nil {
		t.Fatal(err)
	}

	// Create availability windows for both teachers.
	_, err = q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacherA,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacherB,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Student A: in course A's roster.
	studentA, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: "S-NONROSTER-A-" + suffix, FullName: "Student A"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: course.ID, StudentID: studentA.ID}); err != nil {
		t.Fatal(err)
	}

	// Student B: NOT in course A's roster — will be explicitly included in preflight.
	studentB, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: "S-NONROSTER-B-" + suffix, FullName: "Student B"})
	if err != nil {
		t.Fatal(err)
	}

	// Course B: a separate course with student B in its roster.
	courseB, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-nonroster-B-" + suffix, Name: "Course B"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: courseB.ID, StudentID: studentB.ID}); err != nil {
		t.Fatal(err)
	}

	// Create a session for course B (different teacher, different room).
	// This creates a busy range for student B at 10-11 without overlapping teacherA or roomA.
	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  courseB.ID,
		RoomID:    roomB.ID,
		TeacherID: teacherB,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	courseIDStr, err := uuidString(course.ID)
	if err != nil {
		t.Fatal(err)
	}
	roomAStr, err := uuidString(roomA.ID)
	if err != nil {
		t.Fatal(err)
	}
	teacherAStr, err := uuidString(teacherA)
	if err != nil {
		t.Fatal(err)
	}

	slotStart := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true}
	slotEnd := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true}

	requested := ConflictRequested{
		StartAt:   slotStart.Time.UTC().Format(time.RFC3339Nano),
		EndAt:     slotEnd.Time.UTC().Format(time.RFC3339Nano),
		CourseID:  courseIDStr,
		RoomID:    &roomAStr,
		TeacherID: teacherAStr,
	}

	// Case 1: nil StudentIDs (course roster only) — should NOT conflict because student A (course A's only
	// roster student) has no busy range. Room A is free, teacher A is free.
	_, se, err := svc.Preflight(ctx, PreflightParams{
		CourseID:   course.ID,
		RoomID:     roomA.ID,
		TeacherID:  teacherA,
		StartAt:    slotStart,
		EndAt:      slotEnd,
		StudentIDs: nil,
		Requested:  requested,
	})
	if err != nil {
		t.Fatal(err)
	}
	if se != nil {
		t.Fatalf("expected no conflict with course roster only, got: %s (kind=%s)", se.Message, se.Details.Kind)
	}

	// Case 2: explicit roster including student B — should conflict because B has a busy range at 10-11.
	explicitRoster := []pgtype.UUID{studentA.ID, studentB.ID}
	_, se, err = svc.Preflight(ctx, PreflightParams{
		CourseID:   course.ID,
		RoomID:     roomA.ID,
		TeacherID:  teacherA,
		StartAt:    slotStart,
		EndAt:      slotEnd,
		StudentIDs: &explicitRoster,
		Requested:  requested,
	})
	if err != nil {
		t.Fatal(err)
	}
	if se == nil {
		t.Fatal("expected conflict when including non-roster student B who has a busy range, got nil")
	}
	if se.Details.Kind != ConflictKindStudentOverlap {
		t.Fatalf("expected student_overlap kind, got %q", se.Details.Kind)
	}
}

// TestPreflight_UnknownStudentID_DoesNotConflict verifies that passing a non-existent student
// ID to the service-layer Preflight does not trigger an error — the service layer does not
// validate student existence (that's handled at the HTTP boundary). A non-existent student ID
// simply contributes no busy ranges, so preflight should succeed.
func TestPreflight_UnknownStudentID_DoesNotConflict(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-unknown-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-unknown-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-unknown-" + suffix, Name: "Course unknown"})
	if err != nil {
		t.Fatal(err)
	}

	// Create teacher availability window.
	_, err = q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	courseIDStr, err := uuidString(course.ID)
	if err != nil {
		t.Fatal(err)
	}
	roomIDStr, err := uuidString(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	teacherIDStr, err := uuidString(teacherID)
	if err != nil {
		t.Fatal(err)
	}

	slotStart := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true}
	slotEnd := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true}

	requested := ConflictRequested{
		StartAt:   slotStart.Time.UTC().Format(time.RFC3339Nano),
		EndAt:     slotEnd.Time.UTC().Format(time.RFC3339Nano),
		CourseID:  courseIDStr,
		RoomID:    &roomIDStr,
		TeacherID: teacherIDStr,
	}

	// Use a random UUID that doesn't exist in the students table.
	nonExistentID := pgtype.UUID{Bytes: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Valid: true}
	nonExistentRoster := []pgtype.UUID{nonExistentID}

	// Preflight with an explicit roster containing a non-existent student ID should succeed
	// because the service layer doesn't validate student existence — a non-existent ID simply
	// has no busy ranges, so no conflict is detected.
	res, se, err := svc.Preflight(ctx, PreflightParams{
		CourseID:   course.ID,
		RoomID:     room.ID,
		TeacherID:  teacherID,
		StartAt:    slotStart,
		EndAt:      slotEnd,
		StudentIDs: &nonExistentRoster,
		Requested:  requested,
	})
	if err != nil {
		t.Fatal(err)
	}
	if se != nil {
		t.Fatalf("expected no error for unknown student ID (service layer does not validate existence), got: %s", se.Message)
	}
	if res.Status != "available" {
		t.Fatalf("expected status 'available', got %q", res.Status)
	}
}

func TestEditOccurrence_RespectsSessionAttendanceExcludes(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherA, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-edit-excl-A-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	teacherB, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-edit-excl-B-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	roomA, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-edit-excl-A-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	roomB, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-edit-excl-B-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	courseA, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-edit-excl-A-" + suffix, Name: "Course A edit excludes"})
	if err != nil {
		t.Fatal(err)
	}
	courseB, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-edit-excl-B-" + suffix, Name: "Course B edit excludes"})
	if err != nil {
		t.Fatal(err)
	}

	// Availability windows for both teachers.
	_, err = q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacherA,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacherB,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: "S-EDIT-EXCL-" + suffix, FullName: "Student Excluded"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: courseA.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: courseB.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	// Session in course B at 10-11 creates a busy range for the student.
	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  courseB.ID,
		RoomID:    roomB.ID,
		TeacherID: teacherB,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Session in course A initially at 09-10 (within teacher availability), then we exclude the student from attendance.
	createA, err := svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  courseA.ID,
		RoomID:    roomA.ID,
		TeacherID: teacherA,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.SessionAttendanceUpsert(ctx, sqldb.SessionAttendanceUpsertParams{SessionID: createA.SessionID, StudentID: student.ID, Status: "excluded"}); err != nil {
		t.Fatal(err)
	}

	existing, err := q.SessionGetByID(ctx, createA.SessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Edit session A to 10-11. This should succeed because the only roster student is excluded.
	newStart := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true}
	newEnd := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true}

	_, err = svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID:       createA.SessionID,
		StartAt:         &newStart,
		EndAt:           &newEnd,
		ExpectedVersion: existing.Version,
	})
	if err != nil {
		t.Fatalf("expected edit to succeed with excluded student, got: %v", err)
	}
}

func TestCreateSession_ConcurrentOverlap_RaceCondition(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	q := sqldb.New(dbpool)
	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-concurrent-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-concurrent-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-concurrent-" + suffix, Name: "Course concurrent"})
	if err != nil {
		t.Fatal(err)
	}

	const numRequests = 10
	errCh := make(chan error, numRequests)
	var wg sync.WaitGroup
	ready := make(chan struct{})

	start := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true}
	end := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true}

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			_, err := svc.CreateSession(ctx, CreateSessionParams{
				CourseID:  course.ID,
				RoomID:    room.ID,
				TeacherID: teacherID,
				StartAt:   start,
				EndAt:     end,
			})
			errCh <- err
		}()
	}
	close(ready) // release all goroutines simultaneously
	wg.Wait()
	close(errCh)

	successes := 0
	conflicts := 0
	otherErrors := 0
	for err := range errCh {
		if err == nil {
			successes++
			continue
		}
		var se *Err
		if errors.As(err, &se) && se.Code == "schedule_conflict" {
			conflicts++
			if len(se.Details.Conflicts) == 0 {
				t.Errorf("expected explainable conflict details (kind=%s), got empty Conflicts", se.Details.Kind)
			}
		} else {
			otherErrors++
			t.Errorf("unexpected error type: %T %v", err, err)
		}
	}

	if successes != 1 {
		t.Fatalf("expected exactly 1 success (10 concurrent requests for the same slot), got %d successes, %d conflicts, %d other",
			successes, conflicts, otherErrors)
	}
	if successes+conflicts != numRequests {
		t.Fatalf("expected %d total results (successes+conflicts), got successes=%d conflicts=%d other=%d",
			numRequests, successes, conflicts, otherErrors)
	}
	if otherErrors > 0 {
		t.Fatalf("expected no unexpected errors, got %d", otherErrors)
	}
}

func TestSessionListActiveByRange_OverlapSemantics_ReturnsOnlyOverlapping(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-overlap-regression-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-overlap-regression-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-overlap-regression-" + suffix, Name: "Course overlap regression"})
	if err != nil {
		t.Fatal(err)
	}

	// Session A (overlapping): May 19 10:00-11:00 — should be returned by range [May 18, May 26]
	sessionA, err := svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 19, 11, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Session B (before range): May 17 10:00-11:00 — ends before range start (May 18), should NOT overlap
	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 17, 11, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Session C (after range): May 27 10:00-11:00 — starts after range end (May 26), should NOT overlap
	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 27, 11, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	rangeStart := pgtype.Timestamptz{Time: time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC), Valid: true}
	rangeEnd := pgtype.Timestamptz{Time: time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC), Valid: true}

	sessions, err := q.SessionListActiveByRange(ctx, sqldb.SessionListActiveByRangeParams{RangeStart: rangeStart, RangeEnd: rangeEnd})
	if err != nil {
		t.Fatal(err)
	}

	// Filter sessions belonging to our test course to isolate from other tests.
	var matched []sqldb.SessionListActiveByRangeRow
	for _, s := range sessions {
		if s.CourseID.Bytes == course.ID.Bytes {
			matched = append(matched, s)
		}
	}

	if len(matched) != 1 {
		t.Fatalf("expected exactly 1 overlapping session for our course, got %d", len(matched))
	}
	aIDStr, err := uuidString(sessionA.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	mIDStr, err := uuidString(matched[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if aIDStr != mIDStr {
		t.Fatalf("expected session A (%s) to be returned, got %s", aIDStr, mIDStr)
	}
}

func TestCreateSeries_ConcurrentOverlap_RaceCondition(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	q := sqldb.New(dbpool)
	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-series-concurrent-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-series-concurrent-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-series-concurrent-" + suffix, Name: "Course series concurrent"})
	if err != nil {
		t.Fatal(err)
	}

	startDate := LocalDate{Year: 2026, Month: 5, Day: 18}
	endDate := LocalDate{Year: 2026, Month: 5, Day: 18}
	params := CreateSeriesParams{
		CourseID:        course.ID,
		RoomID:          room.ID,
		TeacherID:       teacherID,
		Weekdays:        []time.Weekday{time.Monday},
		StartLocalTime:  Clock{Hour: 10, Minute: 0},
		DurationMinutes: 60,
		StartDate:       startDate,
		EndDate:         &endDate,
	}

	const numRequests = 10
	errCh := make(chan error, numRequests)
	var wg sync.WaitGroup
	ready := make(chan struct{})

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			_, err := svc.CreateSeriesAndMaterialize(ctx, params)
			errCh <- err
		}()
	}
	close(ready) // release all goroutines simultaneously
	wg.Wait()
	close(errCh)

	successes := 0
	conflicts := 0
	otherErrors := 0
	for err := range errCh {
		if err == nil {
			successes++
			continue
		}
		var se *Err
		if errors.As(err, &se) && se.Code == "schedule_conflict" {
			conflicts++
			if len(se.Details.Conflicts) == 0 {
				t.Errorf("expected explainable conflict details (kind=%s), got empty Conflicts", se.Details.Kind)
			}
		} else {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				otherErrors++
				t.Errorf("unexpected pg error code=%s: %v", pgErr.Code, err)
			} else {
				otherErrors++
				t.Errorf("unexpected error type: %T %v", err, err)
			}
		}
	}

	if successes != 1 {
		t.Fatalf("expected exactly 1 success (10 concurrent series for the same slot), got %d successes, %d conflicts, %d other",
			successes, conflicts, otherErrors)
	}
	if successes+conflicts != numRequests {
		t.Fatalf("expected %d total results (successes+conflicts), got successes=%d conflicts=%d other=%d",
			numRequests, successes, conflicts, otherErrors)
	}
	if otherErrors > 0 {
		t.Fatalf("expected no unexpected errors, got %d", otherErrors)
	}
}

func TestEditEntireSeriesFutureOnly_UpdatesFutureOccurrencesWithoutTouchingPast(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherOld, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-entire-old-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	teacherNew, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "teacher-entire-new-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-entire-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-entire-" + suffix, Name: "Course entire"})
	if err != nil {
		t.Fatal(err)
	}

	startDate := LocalDate{Year: 2026, Month: 5, Day: 19}
	endDate := LocalDate{Year: 2026, Month: 5, Day: 25}
	createRes, err := svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
		CourseID:        course.ID,
		RoomID:          room.ID,
		TeacherID:       teacherOld,
		Weekdays:        []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday},
		StartLocalTime:  Clock{Hour: 10, Minute: 0},
		DurationMinutes: 60,
		StartDate:       startDate,
		EndDate:         &endDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	seriesID := createRes.SeriesID

	ser, err := q.SeriesGetByID(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}

	nowUTC := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC) // Before Bangkok 10:00 (03:00 UTC) session
	_, err = svc.EditEntireSeriesFutureOnly(ctx, EditEntireSeriesParams{
		SeriesID:        seriesID,
		ExpectedVersion: ser.Version,
		NowUTC:          nowUTC,
		CourseID:        course.ID,
		RoomID:          room.ID,
		TeacherID:       teacherNew,
		Weekdays:        []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday},
		StartLocalTime:  Clock{Hour: 10, Minute: 0},
		DurationMinutes: 60,
		EndDate:         &endDate,
	})
	if err != nil {
		t.Fatal(err)
	}

	rangeStart := pgtype.Timestamptz{Time: time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC), Valid: true}
	rangeEnd := pgtype.Timestamptz{Time: time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC), Valid: true}
	sessions, err := q.SessionListActiveByRange(ctx, sqldb.SessionListActiveByRangeParams{RangeStart: rangeStart, RangeEnd: rangeEnd})
	if err != nil {
		t.Fatal(err)
	}

	pastOld := 0
	futureNew := 0
	for _, srow := range sessions {
		if !srow.SeriesID.Valid || srow.SeriesID.Bytes != seriesID.Bytes {
			continue
		}
		if srow.StartAt.Time.UTC().Before(nowUTC) {
			if srow.TeacherID.Bytes == teacherOld.Bytes {
				pastOld++
			}
		} else {
			if srow.TeacherID.Bytes == teacherNew.Bytes {
				futureNew++
			}
		}
	}
	if pastOld == 0 {
		t.Fatal("expected at least one past session to remain with old teacher")
	}
	if futureNew == 0 {
		t.Fatal("expected at least one future session to use new teacher")
	}
}
