package legacysync

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
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

	sqldb "warwick-institute/internal/db"
)

func randString(n int) string {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)[:n]
}

var (
	syncMigrationsOnce sync.Once
	syncMigrationsErr  error
)

func requireTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

func migrateUpOnce(t *testing.T, databaseURL string) {
	t.Helper()
	syncMigrationsOnce.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			syncMigrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			syncMigrationsErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			syncMigrationsErr = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		syncMigrationsErr = goose.UpContext(ctx, db, migrationsDir)
	})
	if syncMigrationsErr != nil {
		t.Fatal(syncMigrationsErr)
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
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestSyncer_CreatesSessions(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	ctx := context.Background()

	// Create a teacher user
	teacherUsername := "legacy-teacher-" + randString(6)
	var teacherID pgtype.UUID
	err := pool.QueryRow(ctx, `INSERT INTO users (id, username, role, password_hash) 
		VALUES (gen_random_uuid(), $1, 'Teacher', 'hash') RETURNING id`, teacherUsername).Scan(&teacherID)
	if err != nil {
		t.Fatal(err)
	}

	// Create a course
	courseCode := "LEGACY-01-" + randString(6)
	var courseID pgtype.UUID
	err = pool.QueryRow(ctx, `INSERT INTO courses (code, name, teacher_id)
		VALUES ($1, 'Legacy Test', $2) RETURNING id`, courseCode, teacherID).Scan(&courseID)
	if err != nil {
		t.Fatal(err)
	}

	// Create a room
	roomName := "Auditorium-Test-" + randString(6)
	var roomID pgtype.UUID
	err = pool.QueryRow(ctx, `INSERT INTO rooms (id, name, capacity)
		VALUES (gen_random_uuid(), $1, 100) RETURNING id`, roomName).Scan(&roomID)
	if err != nil {
		t.Fatal(err)
	}

	loc, _ := time.LoadLocation("Asia/Bangkok")
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	syncer := NewSyncer(pool, q, log, loc)

	rows := []ParsedRow{
		{Date: time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), Begin: "13:00", End: "16:20", Classroom: "[120204] 12A: " + roomName},
		{Date: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC), Begin: "09:00", End: "11:30", Classroom: "[NOT SET]"},
	}

	rooms := []Room{
		{ID: uuidString(t, roomID), Name: roomName},
	}

	result, err := syncer.SyncCourse(ctx, courseID, rows, rooms)
	if err != nil {
		t.Fatalf("SyncCourse failed: %v", err)
	}
	if result.SessionsCreated != 2 {
		t.Errorf("expected 2 sessions created, got %d", result.SessionsCreated)
	}
	if result.SyncedAt.IsZero() {
		t.Error("expected synced_at to be set")
	}

	// Verify sessions in DB
	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE course_id = $1 AND deleted_at IS NULL`, courseID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 sessions in DB, got %d", count)
	}

	// Verify last synced time
	var syncedAt pgtype.Timestamptz
	err = pool.QueryRow(ctx, `SELECT legacy_last_synced_at FROM courses WHERE id = $1`, courseID).Scan(&syncedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !syncedAt.Valid {
		t.Error("expected legacy_last_synced_at to be set")
	}
}

func TestSyncer_ConcurrentSyncNoDuplicateSessions(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	ctx := context.Background()

	teacherUsername := "legacy-concurdup-" + randString(6)
	var teacherID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (id, username, role, password_hash) 
		VALUES (gen_random_uuid(), $1, 'Teacher', 'hash') RETURNING id`, teacherUsername).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}

	courseCode := "LEGACY-CONCURDUP-" + randString(6)
	var courseID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO courses (code, name, teacher_id)
		VALUES ($1, 'Concur Dup Test', $2) RETURNING id`, courseCode, teacherID).Scan(&courseID); err != nil {
		t.Fatal(err)
	}

	loc, _ := time.LoadLocation("Asia/Bangkok")
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	syncer := NewSyncer(pool, q, log, loc)

	// Use non-overlapping times so exclusion constraint won't prevent duplicates
	rows := []ParsedRow{
		{Date: time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), Begin: "13:00", End: "14:00", Classroom: "[NOT SET]"},
		{Date: time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), Begin: "14:00", End: "15:00", Classroom: "[NOT SET]"},
	}

	// Run 10 concurrent syncs
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	ready := make(chan struct{})

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			_, err := syncer.SyncCourse(ctx, courseID, rows, nil)
			errs <- err
		}()
	}

	close(ready)
	wg.Wait()
	close(errs)

	var succeedCount int
	for err := range errs {
		if err == nil {
			succeedCount++
		}
	}

	t.Logf("succeeded syncs: %d, failed: %d", succeedCount, 10-succeedCount)

	// FOR UPDATE serializes concurrent syncs so they never fail (no deadlocks/exclusion violations)
	// Without FOR UPDATE: concurrent syncs collide → some fail
	// With FOR UPDATE: all syncs complete cleanly
	if succeedCount != 10 {
		t.Errorf("expected all 10 concurrent syncs to succeed with FOR UPDATE, got %d succeeded, %d failed", succeedCount, 10-succeedCount)
	}

	// Final state should be exactly 2 sessions (last sync wins via soft-delete)
	var activeCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE course_id = $1 AND deleted_at IS NULL`, courseID).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if activeCount != 2 {
		t.Errorf("expected exactly 2 active sessions, got %d", activeCount)
	}
}

func TestSyncer_SetsSeriesIDToNull(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	ctx := context.Background()

	teacherUsername := "legacy-series-" + randString(6)
	var teacherID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (id, username, role, password_hash) 
		VALUES (gen_random_uuid(), $1, 'Teacher', 'hash') RETURNING id`, teacherUsername).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}

	courseCode := "LEGACY-SERIES-" + randString(6)
	var courseID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO courses (code, name, teacher_id)
		VALUES ($1, 'Series Test', $2) RETURNING id`, courseCode, teacherID).Scan(&courseID); err != nil {
		t.Fatal(err)
	}

	loc, _ := time.LoadLocation("Asia/Bangkok")
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	syncer := NewSyncer(pool, q, log, loc)

	rows := []ParsedRow{
		{Date: time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), Begin: "13:00", End: "16:20", Classroom: "[NOT SET]"},
	}

	result, err := syncer.SyncCourse(ctx, courseID, rows, nil)
	if err != nil {
		t.Fatalf("SyncCourse failed: %v", err)
	}
	if result.SessionsCreated != 1 {
		t.Fatalf("expected 1 session created, got %d", result.SessionsCreated)
	}

	var nullCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE course_id = $1 AND series_id IS NULL`, courseID).Scan(&nullCount); err != nil {
		t.Fatal(err)
	}
	if nullCount != 1 {
		t.Errorf("expected 1 session with series_id IS NULL, got %d", nullCount)
	}
}

func TestSyncer_ReplacesExistingSessions(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	ctx := context.Background()

	teacherUsername := "legacy-teacher-" + randString(6)
	var teacherID pgtype.UUID
	err := pool.QueryRow(ctx, `INSERT INTO users (id, username, role, password_hash) 
		VALUES (gen_random_uuid(), $1, 'Teacher', 'hash') RETURNING id`, teacherUsername).Scan(&teacherID)
	if err != nil {
		t.Fatal(err)
	}

	courseCode := "LEGACY-02-" + randString(6)
	var courseID pgtype.UUID
	err = pool.QueryRow(ctx, `INSERT INTO courses (code, name, teacher_id)
		VALUES ($1, 'Legacy Test 2', $2) RETURNING id`, courseCode, teacherID).Scan(&courseID)
	if err != nil {
		t.Fatal(err)
	}

	// Create an existing session
	start := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	_, err = q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  courseID,
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: end, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	loc, _ := time.LoadLocation("Asia/Bangkok")
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	syncer := NewSyncer(pool, q, log, loc)

	rows := []ParsedRow{
		{Date: time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC), Begin: "13:00", End: "16:20", Classroom: "[NOT SET]"},
	}

	result, err := syncer.SyncCourse(ctx, courseID, rows, nil)
	if err != nil {
		t.Fatalf("SyncCourse failed: %v", err)
	}
	if result.SessionsCreated != 1 {
		t.Errorf("expected 1 session created, got %d", result.SessionsCreated)
	}

	// Verify only 1 active session (old should be soft-deleted)
	var activeCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE course_id = $1 AND deleted_at IS NULL`, courseID).Scan(&activeCount)
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Errorf("expected 1 active session, got %d", activeCount)
	}

	// Old session should have deleted_at set
	var deletedCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE course_id = $1 AND deleted_at IS NOT NULL`, courseID).Scan(&deletedCount)
	if err != nil {
		t.Fatal(err)
	}
	if deletedCount != 1 {
		t.Errorf("expected 1 soft-deleted session, got %d", deletedCount)
	}
}

func uuidString(t *testing.T, u pgtype.UUID) string {
	t.Helper()
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}
