package scheduleconflictshttp

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
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

var scheduleConflictsMigrationsOnce sync.Once
var scheduleConflictsMigrationsErr error

func migrateScheduleConflictsUpOnce(t *testing.T, databaseURL string) {
	t.Helper()
	scheduleConflictsMigrationsOnce.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL += "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL += "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			scheduleConflictsMigrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			scheduleConflictsMigrationsErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			scheduleConflictsMigrationsErr = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		scheduleConflictsMigrationsErr = goose.UpContext(ctx, db, migrationsDir)
	})
	if scheduleConflictsMigrationsErr != nil {
		t.Fatal(scheduleConflictsMigrationsErr)
	}
}

func newScheduleConflictsPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestConflictStoreExcludesUnselectedCrossStudySessions(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	migrateScheduleConflictsUpOnce(t, databaseURL)
	pool := newScheduleConflictsPool(t, databaseURL)
	t.Cleanup(pool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	suffix := uuid.NewString()
	studentID := uuid.New()
	sourceCourseID := uuid.New()
	targetCourseID := uuid.New()
	destinationBCourseID := uuid.New()
	otherCourseID := uuid.New()
	teacherID := uuid.New()
	otherTeacherID := uuid.New()
	selectedSessionID := uuid.New()
	selectedOtherSessionID := uuid.New()
	unselectedSessionID := uuid.New()
	unselectedOtherSessionID := uuid.New()
	snapshotID := uuid.New()

	if _, err := pool.Exec(ctx, `
		INSERT INTO students (id, wcode, full_name) VALUES ($1, $2, $3)
	`, studentID, "schedule-scope-"+suffix, "Schedule Scope Student"); err != nil {
		t.Fatal(err)
	}
	for _, course := range []struct {
		id   uuid.UUID
		code string
		name string
	}{
		{sourceCourseID, "SCOPE-SOURCE-" + suffix, "Scope Source"},
		{targetCourseID, "SCOPE-TARGET-" + suffix, "Scope Target"},
		{destinationBCourseID, "SCOPE-B-" + suffix, "Scope Destination B"},
		{otherCourseID, "SCOPE-OTHER-" + suffix, "Scope Other"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO courses (id, code, name) VALUES ($1, $2, $3)`, course.id, course.code, course.name); err != nil {
			t.Fatal(err)
		}
	}
	for _, teacher := range []struct {
		id       uuid.UUID
		username string
	}{
		{teacherID, "scope-teacher-" + suffix},
		{otherTeacherID, "scope-other-teacher-" + suffix},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO users (id, username, role, password_hash) VALUES ($1, $2, 'Teacher', 'x')
		`, teacher.id, teacher.username); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO crm_snapshots (id, status) VALUES ($1, 'ready');
		INSERT INTO crm_cross_study_assignments (
			id, snapshot_id, wcode, source_course_id, dest_course_a_id,
			dest_course_b_id, assigned_course_id, dest_course_a_weekdays,
			dest_course_b_weekdays
		) VALUES ($2, $1, $3, $4, $5, $6, $5, ARRAY[1]::smallint[], ARRAY[7]::smallint[])
	`, snapshotID, uuid.New(), "schedule-scope-"+suffix, sourceCourseID, targetCourseID, destinationBCourseID); err != nil {
		t.Fatal(err)
	}
	for _, courseID := range []uuid.UUID{targetCourseID, otherCourseID} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO course_students (course_id, student_id) VALUES ($1, $2)
		`, courseID, studentID); err != nil {
			t.Fatal(err)
		}
	}

	selectedStart := time.Date(2026, 8, 31, 2, 0, 0, 0, time.UTC)
	unselectedStart := time.Date(2026, 9, 1, 2, 0, 0, 0, time.UTC)
	for _, session := range []struct {
		id               uuid.UUID
		courseID         uuid.UUID
		teacherID        uuid.UUID
		startAt          time.Time
		conflictOverride bool
	}{
		{selectedSessionID, targetCourseID, teacherID, selectedStart, true},
		{selectedOtherSessionID, otherCourseID, otherTeacherID, selectedStart, false},
		{unselectedSessionID, targetCourseID, teacherID, unselectedStart, true},
		{unselectedOtherSessionID, otherCourseID, otherTeacherID, unselectedStart, false},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO sessions (id, course_id, teacher_id, start_at, end_at, conflict_override)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, session.id, session.courseID, session.teacherID, session.startAt, session.startAt.Add(time.Hour), session.conflictOverride); err != nil {
			t.Fatal(err)
		}
	}

	result, err := (conflictStore{db: pool}).list(ctx, listFilters{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("student conflicts = %d, want 1: %+v", len(result.Items), result.Items)
	}
	item := result.Items[0]
	if item.PrimarySession.SessionID != selectedSessionID.String() && item.PrimarySession.SessionID != selectedOtherSessionID.String() {
		t.Fatalf("primary session = %s, want selected Monday pair: %+v", item.PrimarySession.SessionID, item)
	}
	if len(item.ConflictingSessions) != 1 {
		t.Fatalf("conflicting sessions = %d, want 1: %+v", len(item.ConflictingSessions), item)
	}
	conflictingIDs := map[string]bool{
		item.PrimarySession.SessionID:         true,
		item.ConflictingSessions[0].SessionID: true,
	}
	if !conflictingIDs[selectedSessionID.String()] || !conflictingIDs[selectedOtherSessionID.String()] {
		t.Fatalf("conflict sessions = %v, want selected Monday pair", conflictingIDs)
	}
	if conflictingIDs[unselectedSessionID.String()] || conflictingIDs[unselectedOtherSessionID.String()] {
		t.Fatalf("unselected Tuesday session reached schedule-conflicts: %v", conflictingIDs)
	}
}
