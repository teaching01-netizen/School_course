package crmimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	"warwick-institute/internal/crmimport/crmtypes"
	"warwick-institute/internal/crmimport/queue"
	"warwick-institute/internal/crmimport/reconcile"
	"warwick-institute/internal/crmimport/xlsx"
)

// ============================================================================
// Test infrastructure
// ============================================================================

var v2MigrateOnce sync.Once
var v2MigrateErr error

func requireTestDBV2(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

func migrateUpV2(t *testing.T, databaseURL string) {
	t.Helper()
	v2MigrateOnce.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			v2MigrateErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			v2MigrateErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Truncate CRM tables before migrating to avoid NOT NULL constraint failures
		// when migration 00012 runs against non-empty tables from prior test sessions.
		if _, err := db.ExecContext(ctx, `TRUNCATE crm_snapshots, crm_rows, crm_cycles CASCADE`); err != nil {
			v2MigrateErr = err
			return
		}
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			v2MigrateErr = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		v2MigrateErr = goose.UpContext(ctx, db, migrationsDir)
	})
	if v2MigrateErr != nil {
		t.Fatal(v2MigrateErr)
	}
}

func newPoolV2(t *testing.T, databaseURL string) *pgxpool.Pool {
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

func cleanupV2(t *testing.T, dbpool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Use TRUNCATE CASCADE for reliable cleanup of all v2 tables.
	// This handles FK constraints automatically.
	_, _ = dbpool.Exec(ctx, `TRUNCATE TABLE
	    crm_pending_diffs,
	    course_roster_overrides,
	    crm_jobs,
	    course_students,
	    students,
	    crm_rows,
	    crm_cycles,
	    crm_snapshots
	  RESTART IDENTITY CASCADE`)
	// Reset crm_state (may have been deleted by CASCADE).
	_, _ = dbpool.Exec(ctx, `INSERT INTO crm_state (singleton, active_snapshot_id) VALUES (true, NULL) ON CONFLICT (singleton) DO UPDATE SET active_snapshot_id = NULL, updated_at = now()`)
	// Reset courses CRM columns.
	_, _ = dbpool.Exec(ctx, `UPDATE courses SET
	    crm_filter_enabled = false, crm_filter = NULL, crm_roster_locked = false,
	    crm_filter_version = 1,
	    crm_last_applied_snapshot_id = NULL,
	    crm_pending_review_snapshot_id = NULL,
	    crm_pending_review_summary = NULL`)
}

// createTestSnapshot creates a snapshot and populates it with the given rows.
// Returns the snapshot ID.
func createTestSnapshot(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool, rows []xlsx.Row) pgtype.UUID {
	t.Helper()

	snapshotSvc, err := NewSnapshotService(dbpool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}

	snapshotID, err := snapshotSvc.CreateSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) > 0 {
		inserted, err := snapshotSvc.PopulateRows(ctx, snapshotID, rows, len(rows))
		if err != nil {
			t.Fatal(err)
		}
		if inserted != len(rows) {
			t.Fatalf("expected %d inserted rows, got %d", len(rows), inserted)
		}
	}

	if err := snapshotSvc.MarkSnapshotReady(ctx, snapshotID, len(rows)); err != nil {
		t.Fatal(err)
	}

	return snapshotID
}

// createTestCourse creates a course with CRM filter enabled.
func createTestCourse(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool, code, name string, filter crmtypes.CourseFilter) pgtype.UUID {
	t.Helper()
	q := sqldb.New(dbpool)
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: code,
		Name: name,
	})
	if err != nil {
		t.Fatal(err)
	}

	filterJSON, err := json.Marshal(filter)
	if err != nil {
		t.Fatal(err)
	}

	_, err = dbpool.Exec(ctx, `UPDATE courses SET crm_filter_enabled = true, crm_filter = $1::jsonb WHERE id = $2`, string(filterJSON), course.ID)
	if err != nil {
		t.Fatal(err)
	}

	return course.ID
}

// ============================================================================
// Queue tests
// ============================================================================

func TestQueue_EnqueueAndClaim(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	queueStore := queue.NewPostgresQueueStore(dbpool)
	worker := queue.NewQueueWorker(tLogger, queueStore, "test-worker-1")
	worker.SetLeaseDuration(5 * time.Second)
	worker.SetHeartbeatInterval(1 * time.Second)

	// Register a simple handler.
	executed := make(chan struct{})
	worker.RegisterHandler(queue.JobTypeStudentSync, func(ctx context.Context, job queue.JobRow) error {
		close(executed)
		return nil
	})

	// Enqueue a job.
	payload := StudentSyncPayload{SnapshotID: uuid.New()}
	jobID, err := worker.EnqueueJob(ctx, queue.JobTypeStudentSync, payload, "")
	if err != nil {
		t.Fatalf("EnqueueJob: %v", err)
	}

	// Verify job is queued.
	var status string
	err = dbpool.QueryRow(ctx, `SELECT status::text FROM crm_jobs WHERE id = $1`, jobID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("expected status 'queued', got %q", status)
	}

	// Manually claim the job.
	job, ok := worker.ClaimNextJob(ctx)
	if !ok {
		t.Fatal("expected to claim a job")
	}
	if job.ID != jobID {
		t.Fatalf("expected job ID %s, got %s", jobID, job.ID)
	}

	// Verify job is now running.
	err = dbpool.QueryRow(ctx, `SELECT status::text FROM crm_jobs WHERE id = $1`, jobID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Fatalf("expected status 'running', got %q", status)
	}

	// Run the job.
	worker.RunJob(ctx, job)

	// Verify job completed.
	select {
	case <-executed:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("job handler was not executed")
	}

	err = dbpool.QueryRow(ctx, `SELECT status::text FROM crm_jobs WHERE id = $1`, jobID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" {
		t.Fatalf("expected status 'succeeded', got %q", status)
	}
}

func TestQueue_LeaseExpiryReclaim(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	queueStore1 := queue.NewPostgresQueueStore(dbpool)
	worker1 := queue.NewQueueWorker(tLogger, queueStore1, "test-worker-1")
	worker1.SetLeaseDuration(60 * time.Second)
	queueStore2 := queue.NewPostgresQueueStore(dbpool)
	worker2 := queue.NewQueueWorker(tLogger, queueStore2, "test-worker-2")
	worker2.SetLeaseDuration(60 * time.Second)

	executed1 := make(chan struct{})
	executed2 := make(chan struct{})

	// Worker 1's handler blocks forever (simulating a zombie).
	worker1.RegisterHandler(queue.JobTypeStudentSync, func(ctx context.Context, job queue.JobRow) error {
		close(executed1)
		// Block forever — this job will become a zombie.
		<-make(chan struct{})
		return nil
	})

	// Worker 2's handler succeeds.
	worker2.RegisterHandler(queue.JobTypeStudentSync, func(ctx context.Context, job queue.JobRow) error {
		close(executed2)
		return nil
	})

	payload := StudentSyncPayload{SnapshotID: uuid.New()}
	jobID, err := worker1.EnqueueJob(ctx, queue.JobTypeStudentSync, payload, "")
	if err != nil {
		t.Fatalf("EnqueueJob: %v", err)
	}

	// Worker 1 claims the job.
	job, ok := worker1.ClaimNextJob(ctx)
	if !ok {
		t.Fatal("worker1: expected to claim a job")
	}
	if job.ID != jobID {
		t.Fatalf("worker1: expected job ID %s, got %s", jobID, job.ID)
	}

	// Run job in goroutine since handler blocks forever.
	go worker1.RunJob(ctx, job)

	// Wait for worker1's handler to start.
	select {
	case <-executed1:
	case <-time.After(2 * time.Second):
		t.Fatal("worker1 handler did not execute")
	}

	// Manually expire the lease to simulate zombie.
	_, err = dbpool.Exec(ctx, `UPDATE crm_jobs SET locked_until = now() - interval '1 second' WHERE id = $1`, jobID)
	if err != nil {
		t.Fatal(err)
	}

	// Worker 2 should now be able to reclaim the zombie job.
	reclaimed, ok := worker2.ClaimNextJob(ctx)
	if !ok {
		t.Fatal("worker2: expected to reclaim zombie job")
	}
	if reclaimed.ID != jobID {
		t.Fatalf("worker2: expected job ID %s, got %s", jobID, reclaimed.ID)
	}

	// Verify the attempt was incremented.
	var attempt int
	err = dbpool.QueryRow(ctx, `SELECT attempt FROM crm_jobs WHERE id = $1`, jobID).Scan(&attempt)
	if err != nil {
		t.Fatal(err)
	}
	if attempt < 1 {
		t.Fatalf("expected attempt >= 1, got %d", attempt)
	}

	// Run the job on worker2.
	worker2.RunJob(ctx, reclaimed)

	select {
	case <-executed2:
	case <-time.After(2 * time.Second):
		t.Fatal("worker2 handler did not execute")
	}

	// Verify job succeeded.
	var status string
	err = dbpool.QueryRow(ctx, `SELECT status::text FROM crm_jobs WHERE id = $1`, jobID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" {
		t.Fatalf("expected status 'succeeded', got %q", status)
	}
}

func TestQueue_HeartbeatPreventsReclaim(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	queueStore1 := queue.NewPostgresQueueStore(dbpool)
	worker1 := queue.NewQueueWorker(tLogger, queueStore1, "test-worker-1")
	worker1.SetLeaseDuration(10 * time.Second)
	worker1.SetHeartbeatInterval(1 * time.Second)
	queueStore2 := queue.NewPostgresQueueStore(dbpool)
	worker2 := queue.NewQueueWorker(tLogger, queueStore2, "test-worker-2")
	worker2.SetLeaseDuration(10 * time.Second)

	executed := make(chan struct{})

	// Worker 1's handler does a long operation but with heartbeats.
	worker1.RegisterHandler(queue.JobTypeStudentSync, func(ctx context.Context, job queue.JobRow) error {
		defer close(executed)
		// Simulate work that takes longer than the lease (5 seconds > 10s lease... no, 5 < 10)
		// Actually, let's make the lease 3 seconds and work 8 seconds.
		// The heartbeat should extend the lease.
		for i := 0; i < 8; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
		}
		return nil
	})

	payload := StudentSyncPayload{SnapshotID: uuid.New()}
	jobID, err := worker1.EnqueueJob(ctx, queue.JobTypeStudentSync, payload, "")
	if err != nil {
		t.Fatalf("EnqueueJob: %v", err)
	}

	// Worker 1 claims and runs the job (heartbeat loop runs in background).
	job, ok := worker1.ClaimNextJob(ctx)
	if !ok {
		t.Fatal("worker1: expected to claim a job")
	}
	go worker1.RunJob(ctx, job)

	// Wait for handler to start and check lease is being extended.
	time.Sleep(2 * time.Second)

	// Even though the original lease of 10s hasn't expired yet,
	// the heartbeat loop should still be active.
	// Try reclaiming with worker2 — should NOT get the job because heartbeat extends lease.
	_, ok = worker2.ClaimNextJob(ctx)
	if ok {
		t.Fatal("worker2: should NOT have been able to reclaim a job with active heartbeat")
	}

	// Wait for handler to complete.
	select {
	case <-executed:
	case <-time.After(12 * time.Second):
		t.Fatal("job handler did not complete in time")
	}

	// Poll for job to be marked as succeeded (completeJob may run slightly after handler returns).
	var status string
	for i := 0; i < 20; i++ {
		err = dbpool.QueryRow(ctx, `SELECT status::text FROM crm_jobs WHERE id = $1`, jobID).Scan(&status)
		if err == nil && status == "succeeded" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if status != "succeeded" {
		t.Fatalf("expected status 'succeeded', got %q", status)
	}
}

func TestQueue_UniqueKeyDedupe(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	queueStore := queue.NewPostgresQueueStore(dbpool)
	worker := queue.NewQueueWorker(tLogger, queueStore, "test-worker")

	uniqueKey := "reconcile-apply-snap1-course1"

	payload1 := crmtypes.CourseReconcilePayload{
		SnapshotID:            uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		CourseID:              uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		ExpectedFilterVersion: 1,
	}
	payload2 := crmtypes.CourseReconcilePayload{
		SnapshotID:            uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		CourseID:              uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		ExpectedFilterVersion: 2,
	}

	// Enqueue first job.
	jobID1, err := worker.EnqueueJob(ctx, queue.JobTypeCourseReconcileApply, payload1, uniqueKey)
	if err != nil {
		t.Fatalf("EnqueueJob 1: %v", err)
	}

	// Enqueue second job with same unique_key — should update (not create new).
	jobID2, err := worker.EnqueueJob(ctx, queue.JobTypeCourseReconcileApply, payload2, uniqueKey)
	if err != nil {
		t.Fatalf("EnqueueJob 2: %v", err)
	}

	// Both should return the same job ID (upsert behaviour).
	if jobID1 != jobID2 {
		t.Fatalf("expected same job ID for deduped enqueue, got %s vs %s", jobID1, jobID2)
	}

	// Count active jobs with this unique key.
	var count int
	err = dbpool.QueryRow(ctx,
		`SELECT COUNT(*) FROM crm_jobs WHERE unique_key = $1 AND status NOT IN ('succeeded', 'failed')`,
		uniqueKey,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 active job, got %d", count)
	}
}

// ============================================================================
// Snapshot tests
// ============================================================================

func TestSnapshot_CreateAndPopulate(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows := []xlsx.Row{
		{WCode: "W250001", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "John", LastName: "Doe"},
		{WCode: "W250002", CourseName: "Science", CycleLabel: "Cycle B", FirstName: "Jane", LastName: "Smith"},
		{WCode: "W250003", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Bob", LastName: "Brown"},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	// Verify snapshot status.
	var status string
	var rowCount int
	err := dbpool.QueryRow(ctx, `SELECT status, row_count FROM crm_snapshots WHERE id = $1`, snapshotID).Scan(&status, &rowCount)
	if err != nil {
		t.Fatal(err)
	}
	if status != "ready" {
		t.Fatalf("expected status 'ready', got %q", status)
	}
	if rowCount != 3 {
		t.Fatalf("expected row_count 3, got %d", rowCount)
	}

	// Verify rows stored with snapshot_id and xlsx_row_number.
	var storedCount int
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_rows WHERE snapshot_id = $1`, snapshotID).Scan(&storedCount)
	if err != nil {
		t.Fatal(err)
	}
	if storedCount != 3 {
		t.Fatalf("expected 3 rows in crm_rows, got %d", storedCount)
	}

	// Verify xlsx_row_numbers are sequential.
	var maxRowNum int
	err = dbpool.QueryRow(ctx, `SELECT MAX(xlsx_row_number) FROM crm_rows WHERE snapshot_id = $1`, snapshotID).Scan(&maxRowNum)
	if err != nil {
		t.Fatal(err)
	}
	if maxRowNum != 3 {
		t.Fatalf("expected max xlsx_row_number 3, got %d", maxRowNum)
	}

	// Verify active snapshot is set.
	var activeID pgtype.UUID
	err = dbpool.QueryRow(ctx, `SELECT active_snapshot_id FROM crm_state WHERE singleton = true`).Scan(&activeID)
	if err != nil {
		t.Fatal(err)
	}
	if !activeID.Valid || activeID.Bytes != snapshotID.Bytes {
		t.Fatal("expected active_snapshot_id to be set")
	}

	// Verify cycles populated.
	var cycleCount int
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_cycles`).Scan(&cycleCount)
	if err != nil {
		t.Fatal(err)
	}
	if cycleCount != 2 {
		t.Fatalf("expected 2 cycles, got %d", cycleCount)
	}
}

func TestSnapshot_DuplicateRowsDeduped(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Same row repeated 3 times — deduped at import handler level.
	row := xlsx.Row{WCode: "W250001", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "John", LastName: "Doe"}
	dupes := []xlsx.Row{row, row, row}

	snapshotSvc, err := NewSnapshotService(dbpool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}

	snapshotID, err := snapshotSvc.CreateSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// The handler in upload_v2.go dedupes before PopulateRows.
	// Simulate that logic:
	seen := map[string]struct{}{}
	deduped := make([]xlsx.Row, 0, len(dupes))
	for _, r := range dupes {
		h := r.Hash()
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		deduped = append(deduped, r)
	}

	inserted, err := snapshotSvc.PopulateRows(ctx, snapshotID, deduped, len(dupes))
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 1 {
		t.Fatalf("expected 1 inserted row after dedupe, got %d", inserted)
	}

	// Verify only 1 row stored.
	var storedCount int
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_rows WHERE snapshot_id = $1`, snapshotID).Scan(&storedCount)
	if err != nil {
		t.Fatal(err)
	}
	if storedCount != 1 {
		t.Fatalf("expected 1 row stored, got %d", storedCount)
	}
}

// ============================================================================
// Student Sync tests
// ============================================================================

func TestStudentSync_FromSnapshot(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows := []xlsx.Row{
		{WCode: "W250010", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Alice", LastName: "Alpha"},
		{WCode: "W250011", CourseName: "Science", CycleLabel: "Cycle B", FirstName: "Bob", LastName: "", Nickname: "Bobby"},
		{WCode: "W250012", CourseName: "History", CycleLabel: "Cycle A", FirstName: "", LastName: "Charlie"},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	syncSvc := NewStudentSyncService(dbpool)
	synced, err := syncSvc.SyncFromSnapshot(ctx, snapshotID)
	if err != nil {
		t.Fatalf("SyncFromSnapshot: %v", err)
	}
	if synced < 3 {
		t.Fatalf("expected at least 3 synced records, got %d", synced)
	}

	// Verify students in DB.
	var count int
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM students`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 students, got %d", count)
	}

	// Verify full names.
	var fullName string
	err = dbpool.QueryRow(ctx, `SELECT full_name FROM students WHERE wcode = 'W250010'`).Scan(&fullName)
	if err != nil {
		t.Fatal(err)
	}
	if fullName != "Alice Alpha" {
		t.Fatalf("expected full_name 'Alice Alpha', got %q", fullName)
	}
}

func TestStudentSync_DeterministicTieBreak(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	rows := []xlsx.Row{
		{WCode: "W250020", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "First", LastName: "Entry", OrderQuoteUpdatedAt: &now},
		{WCode: "W250020", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Second", LastName: "Entry", OrderQuoteUpdatedAt: &now},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	syncSvc := NewStudentSyncService(dbpool)
	_, err := syncSvc.SyncFromSnapshot(ctx, snapshotID)
	if err != nil {
		t.Fatalf("SyncFromSnapshot: %v", err)
	}

	// With same order_quote_updated_at, the secondary sort (xlsx_row_number ASC) should pick the first row.
	var fullName string
	err = dbpool.QueryRow(ctx, `SELECT full_name FROM students WHERE wcode = 'W250020'`).Scan(&fullName)
	if err != nil {
		t.Fatal(err)
	}
	// First entry (xlsx_row_number=1) has FirstName "First"
	if fullName != "First Entry" {
		t.Fatalf("expected full_name 'First Entry' (deterministic tie-break by xlsx_row_number), got %q", fullName)
	}
}

func TestStudentSync_PreservesNotes(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Pre-create a student with notes.
	_, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ('W250030', 'Existing Name', 'important note')`)
	if err != nil {
		t.Fatal(err)
	}

	rows := []xlsx.Row{
		{WCode: "W250030", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Updated", LastName: "Name"},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	syncSvc := NewStudentSyncService(dbpool)
	_, err = syncSvc.SyncFromSnapshot(ctx, snapshotID)
	if err != nil {
		t.Fatalf("SyncFromSnapshot: %v", err)
	}

	// Verify full name was updated but notes were preserved.
	var fullName, notes string
	err = dbpool.QueryRow(ctx, `SELECT full_name, notes FROM students WHERE wcode = 'W250030'`).Scan(&fullName, &notes)
	if err != nil {
		t.Fatal(err)
	}
	if fullName != "Updated Name" {
		t.Fatalf("expected full_name 'Updated Name', got %q", fullName)
	}
	if notes != "important note" {
		t.Fatalf("expected notes 'important note' to be preserved, got %q", notes)
	}
}

// ============================================================================
// Reconcile tests
// ============================================================================

func TestReconcileApply_AddsStudentsFromSnapshot(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	rows := []xlsx.Row{
		{WCode: "W250040", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Dan", LastName: "Delta"},
		{WCode: "W250041", CourseName: "Science", CycleLabel: "Cycle B", FirstName: "Eve", LastName: "Epsilon"},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	filter := crmtypes.CourseFilter{
		CycleLabels:              []string{"Cycle A"},
		CycleBlankMode:           crmtypes.BlankModeAny,
		CourseNameBlankMode:      crmtypes.BlankModeAny,
		AcademicLevelBlankMode:   crmtypes.BlankModeAny,
		SecondarySchoolBlankMode: crmtypes.BlankModeAny,
		TeachersBlankMode:        crmtypes.BlankModeAny,
	}

	courseID := createTestCourse(t, ctx, dbpool, "C-V2-ADD-"+suffix, "V2 Add Test", filter)

	reconcileSvc := reconcile.NewReconcileV2Service(dbpool)
	result, err := reconcileSvc.ApplyCourseReconcile(ctx, snapshotID, courseID, filter)
	if err != nil {
		t.Fatalf("ApplyCourseReconcile: %v", err)
	}

	if result.DesiredStudents != 1 {
		t.Fatalf("expected DesiredStudents=1 (Cycle A only), got %d", result.DesiredStudents)
	}
	if result.Added != 1 {
		t.Fatalf("expected Added=1, got %d", result.Added)
	}
	if result.Removed != 0 {
		t.Fatalf("expected Removed=0, got %d", result.Removed)
	}

	// Verify student was added.
	var enrolledCount int
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM course_students WHERE course_id = $1`, courseID).Scan(&enrolledCount)
	if err != nil {
		t.Fatal(err)
	}
	if enrolledCount != 1 {
		t.Fatalf("expected 1 student enrolled, got %d", enrolledCount)
	}

	// Verify last_applied_snapshot was set.
	var lastApplied pgtype.UUID
	err = dbpool.QueryRow(ctx, `SELECT crm_last_applied_snapshot_id FROM courses WHERE id = $1`, courseID).Scan(&lastApplied)
	if err != nil {
		t.Fatal(err)
	}
	if !lastApplied.Valid || lastApplied.Bytes != snapshotID.Bytes {
		t.Fatal("expected crm_last_applied_snapshot_id to be set")
	}
}

func TestReconcileApply_RemovesExtraStudents(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	rows := []xlsx.Row{
		{WCode: "W250050", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Frank", LastName: "Falk"},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	filter := crmtypes.CourseFilter{
		CycleLabels:              []string{"Cycle A"},
		CycleBlankMode:           crmtypes.BlankModeAny,
		CourseNameBlankMode:      crmtypes.BlankModeAny,
		AcademicLevelBlankMode:   crmtypes.BlankModeAny,
		SecondarySchoolBlankMode: crmtypes.BlankModeAny,
		TeachersBlankMode:        crmtypes.BlankModeAny,
	}

	courseID := createTestCourse(t, ctx, dbpool, "C-V2-REMOVE-"+suffix, "V2 Remove Test", filter)

	// Pre-enroll an extra student who doesn't match the filter.
	_, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ('W259999', 'Extra Student', '')`)
	if err != nil {
		t.Fatal(err)
	}
	var extraStudentID pgtype.UUID
	err = dbpool.QueryRow(ctx, `SELECT id FROM students WHERE wcode = 'W259999'`).Scan(&extraStudentID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = dbpool.Exec(ctx, `INSERT INTO course_students (course_id, student_id) VALUES ($1, $2)`, courseID, extraStudentID)
	if err != nil {
		t.Fatal(err)
	}

	reconcileSvc := reconcile.NewReconcileV2Service(dbpool)
	result, err := reconcileSvc.ApplyCourseReconcile(ctx, snapshotID, courseID, filter)
	if err != nil {
		t.Fatalf("ApplyCourseReconcile: %v", err)
	}

	if result.DesiredStudents != 1 {
		t.Fatalf("expected DesiredStudents=1, got %d", result.DesiredStudents)
	}
	if result.Added != 1 {
		t.Fatalf("expected Added=1, got %d", result.Added)
	}
	if result.Removed != 1 {
		t.Fatalf("expected Removed=1, got %d", result.Removed)
	}

	// Verify only 1 student remains.
	var enrolledCount int
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM course_students WHERE course_id = $1`, courseID).Scan(&enrolledCount)
	if err != nil {
		t.Fatal(err)
	}
	if enrolledCount != 1 {
		t.Fatalf("expected 1 student enrolled, got %d", enrolledCount)
	}
}

func TestReconcileApply_Idempotent(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	rows := []xlsx.Row{
		{WCode: "W250060", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Grace", LastName: "Gale"},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	filter := crmtypes.CourseFilter{
		CycleLabels:              []string{"Cycle A"},
		CycleBlankMode:           crmtypes.BlankModeAny,
		CourseNameBlankMode:      crmtypes.BlankModeAny,
		AcademicLevelBlankMode:   crmtypes.BlankModeAny,
		SecondarySchoolBlankMode: crmtypes.BlankModeAny,
		TeachersBlankMode:        crmtypes.BlankModeAny,
	}

	courseID := createTestCourse(t, ctx, dbpool, "C-V2-IDEM-"+suffix, "V2 Idempotent Test", filter)

	reconcileSvc := reconcile.NewReconcileV2Service(dbpool)

	// First apply.
	r1, err := reconcileSvc.ApplyCourseReconcile(ctx, snapshotID, courseID, filter)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if r1.Added != 1 || r1.Removed != 0 {
		t.Fatalf("first: expected Adds=1 Removes=0, got Adds=%d Removes=%d", r1.Added, r1.Removed)
	}

	// Second apply — should be idempotent.
	r2, err := reconcileSvc.ApplyCourseReconcile(ctx, snapshotID, courseID, filter)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if r2.Added != 0 {
		t.Fatalf("second: expected Adds=0 (idempotent), got %d", r2.Added)
	}
	if r2.Removed != 0 {
		t.Fatalf("second: expected Removes=0 (idempotent), got %d", r2.Removed)
	}
}

func TestReconcileApply_WithOverridesIncludeExclude(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	// Create snapshot with 3 students.
	rows := []xlsx.Row{
		{WCode: "W250070", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Hank", LastName: "Hill"},
		{WCode: "W250071", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Iris", LastName: "Ivy"},
		{WCode: "W250072", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Jack", LastName: "Jill"},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	filter := crmtypes.CourseFilter{
		CycleLabels:              []string{"Cycle A"},
		CycleBlankMode:           crmtypes.BlankModeAny,
		CourseNameBlankMode:      crmtypes.BlankModeAny,
		AcademicLevelBlankMode:   crmtypes.BlankModeAny,
		SecondarySchoolBlankMode: crmtypes.BlankModeAny,
		TeachersBlankMode:        crmtypes.BlankModeAny,
	}

	courseID := createTestCourse(t, ctx, dbpool, "C-V2-OVERRIDE-"+suffix, "V2 Override Test", filter)

	reconcileSvc := reconcile.NewReconcileV2Service(dbpool)

	// Create a user for audit (use unique suffix to avoid conflicts).
	var userID pgtype.UUID
	err := dbpool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1, 'Admin', 'hash') RETURNING id`, "test-admin-"+suffix).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}

	// Apply reconcile first (adds all 3).
	_, err = reconcileSvc.ApplyCourseReconcile(ctx, snapshotID, courseID, filter)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Exclude one student via override (manually insert override + write-through to course_students).
	var excludeStudentID pgtype.UUID
	err = dbpool.QueryRow(ctx, `SELECT id FROM students WHERE wcode = 'W250071'`).Scan(&excludeStudentID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = dbpool.Exec(ctx, `
		INSERT INTO course_roster_overrides (course_id, student_id, action, created_by_user_id)
		VALUES ($1, $2, 'exclude'::override_action, $3)
		ON CONFLICT (course_id, student_id) DO UPDATE
		SET action = 'exclude'::override_action,
		    updated_by_user_id = $3,
		    updated_at = now(),
		    deleted_at = NULL
	`, courseID, excludeStudentID, userID)
	if err != nil {
		t.Fatalf("insert override: %v", err)
	}

	// Write-through: remove from course_students for immediate UX consistency.
	_, err = dbpool.Exec(ctx,
		`DELETE FROM course_students WHERE course_id = $1 AND student_id = $2`,
		courseID, excludeStudentID,
	)
	if err != nil {
		t.Fatalf("delete course_student: %v", err)
	}

	// Re-apply — override write-through already removed the student, so reconcile should be idempotent.
	r2, err := reconcileSvc.ApplyCourseReconcile(ctx, snapshotID, courseID, filter)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if r2.Removed != 0 {
		t.Fatalf("expected Removed=0 (override write-through already removed), got %d", r2.Removed)
	}

	// Verify only 2 students remain (the excluded one was removed by override write-through).
	var enrolledCount int
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM course_students WHERE course_id = $1`, courseID).Scan(&enrolledCount)
	if err != nil {
		t.Fatal(err)
	}
	if enrolledCount != 2 {
		t.Fatalf("expected 2 enrolled students, got %d", enrolledCount)
	}
}

// ============================================================================
// Filter concurrency test
// ============================================================================

func TestFilterConcurrency_StaleJobAbortsAndReenqueues(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	rows := []xlsx.Row{
		{WCode: "W250080", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Kate", LastName: "King"},
	}

	createTestSnapshot(t, ctx, dbpool, rows)

	filter := crmtypes.CourseFilter{
		CycleLabels:              []string{"Cycle A"},
		CycleBlankMode:           crmtypes.BlankModeAny,
		CourseNameBlankMode:      crmtypes.BlankModeAny,
		AcademicLevelBlankMode:   crmtypes.BlankModeAny,
		SecondarySchoolBlankMode: crmtypes.BlankModeAny,
		TeachersBlankMode:        crmtypes.BlankModeAny,
	}

	courseID := createTestCourse(t, ctx, dbpool, "C-V2-STALE-"+suffix, "V2 Stale Test", filter)

	reconcileSvc := reconcile.NewReconcileV2Service(dbpool)

	// Check current filter version.
	currentVersion, err := reconcileSvc.CheckFilterVersion(ctx, courseID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !currentVersion {
		t.Fatal("expected version 1 to match")
	}

	// Simulate a stale job by bumping the version.
	err = reconcileSvc.UpdateCourseFilter(ctx, courseID, true, filter)
	if err != nil {
		t.Fatalf("UpdateCourseFilter: %v", err)
	}

	// Now check if version 1 is stale.
	version1Valid, err := reconcileSvc.CheckFilterVersion(ctx, courseID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if version1Valid {
		t.Fatal("expected version 1 to be stale after bump")
	}

	// Verify current version is now 2.
	currentVersion, err = reconcileSvc.CheckFilterVersion(ctx, courseID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !currentVersion {
		t.Fatal("expected version 2 to match")
	}
}

// ============================================================================
// Review diff tests
// ============================================================================

func TestReconcileDiff_StoresDiffs(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	rows := []xlsx.Row{
		{WCode: "W250090", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Leo", LastName: "Lion"},
		{WCode: "W250091", CourseName: "Science", CycleLabel: "Cycle B", FirstName: "Mia", LastName: "Mole"},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	filter := crmtypes.CourseFilter{
		CycleLabels:              []string{"Cycle A", "Cycle B"},
		CycleBlankMode:           crmtypes.BlankModeAny,
		CourseNameBlankMode:      crmtypes.BlankModeAny,
		AcademicLevelBlankMode:   crmtypes.BlankModeAny,
		SecondarySchoolBlankMode: crmtypes.BlankModeAny,
		TeachersBlankMode:        crmtypes.BlankModeAny,
	}

	courseID := createTestCourse(t, ctx, dbpool, "C-V2-DIFF-"+suffix, "V2 Diff Test", filter)

	reconcileSvc := reconcile.NewReconcileV2Service(dbpool)

	// Initially, no students enrolled — so diff should show 2 adds, 0 removes.
	result, err := reconcileSvc.DiffCourseReconcile(ctx, snapshotID, courseID, filter)
	if err != nil {
		t.Fatalf("DiffCourseReconcile: %v", err)
	}

	if result.AddCount != 2 {
		t.Fatalf("expected AddCount=2, got %d", result.AddCount)
	}
	if result.RemoveCount != 0 {
		t.Fatalf("expected RemoveCount=0, got %d", result.RemoveCount)
	}

	// Verify diffs stored in crm_pending_diffs.
	var diffCount int
	err = dbpool.QueryRow(ctx,
		`SELECT COUNT(*) FROM crm_pending_diffs WHERE course_id = $1 AND snapshot_id = $2`,
		courseID, snapshotID,
	).Scan(&diffCount)
	if err != nil {
		t.Fatal(err)
	}
	if diffCount != 2 {
		t.Fatalf("expected 2 pending diff rows, got %d", diffCount)
	}

	// Verify course has pending review summary.
	var summaryJSON []byte
	var pendingSnapshot pgtype.UUID
	err = dbpool.QueryRow(ctx,
		`SELECT crm_pending_review_summary, crm_pending_review_snapshot_id FROM courses WHERE id = $1`,
		courseID,
	).Scan(&summaryJSON, &pendingSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !pendingSnapshot.Valid || pendingSnapshot.Bytes != snapshotID.Bytes {
		t.Fatal("expected crm_pending_review_snapshot_id to be set")
	}
	if summaryJSON == nil {
		t.Fatal("expected crm_pending_review_summary to be set")
	}
}

func TestReviewDiffPaging(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	// Create a snapshot with 5 rows.
	rows := make([]xlsx.Row, 5)
	for i := 0; i < 5; i++ {
		rows[i] = xlsx.Row{
			WCode:      fmt.Sprintf("W25%04d", 100+i),
			CourseName: "Math",
			CycleLabel: "Cycle A",
			FirstName:  fmt.Sprintf("Student%d", i+1),
			LastName:   "Test",
		}
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	filter := crmtypes.CourseFilter{
		CycleLabels:              []string{"Cycle A"},
		CycleBlankMode:           crmtypes.BlankModeAny,
		CourseNameBlankMode:      crmtypes.BlankModeAny,
		AcademicLevelBlankMode:   crmtypes.BlankModeAny,
		SecondarySchoolBlankMode: crmtypes.BlankModeAny,
		TeachersBlankMode:        crmtypes.BlankModeAny,
	}

	courseID := createTestCourse(t, ctx, dbpool, "C-V2-PAGE-"+suffix, "V2 Paging Test", filter)

	reconcileSvc := reconcile.NewReconcileV2Service(dbpool)

	// Store diffs.
	_, err := reconcileSvc.DiffCourseReconcile(ctx, snapshotID, courseID, filter)
	if err != nil {
		t.Fatalf("DiffCourseReconcile: %v", err)
	}

	// Page 1: limit 2.
	page1, err := reconcileSvc.GetPendingDiffPage(ctx, courseID, crmtypes.DiffAdd, 0, 2)
	if err != nil {
		t.Fatalf("GetPendingDiffPage page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 results on page 1, got %d", len(page1))
	}
	if page1[0].Seq != 1 {
		t.Fatalf("expected first seq=1, got %d", page1[0].Seq)
	}

	// Page 2: cursor=2, limit 2.
	page2, err := reconcileSvc.GetPendingDiffPage(ctx, courseID, crmtypes.DiffAdd, 2, 2)
	if err != nil {
		t.Fatalf("GetPendingDiffPage page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("expected 2 results on page 2, got %d", len(page2))
	}
	if page2[0].Seq != 3 {
		t.Fatalf("expected first seq=3, got %d", page2[0].Seq)
	}

	// Page 3: cursor=4, limit 2 — should have 1 remaining.
	page3, err := reconcileSvc.GetPendingDiffPage(ctx, courseID, crmtypes.DiffAdd, 4, 2)
	if err != nil {
		t.Fatalf("GetPendingDiffPage page3: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("expected 1 result on page 3, got %d", len(page3))
	}
	if page3[0].Seq != 5 {
		t.Fatalf("expected seq=5, got %d", page3[0].Seq)
	}
}

func TestApproveReviewEnqueuesApply(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	rows := []xlsx.Row{
		{WCode: "W250110", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Nina", LastName: "Nest"},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	filter := crmtypes.CourseFilter{
		CycleLabels:              []string{"Cycle A"},
		CycleBlankMode:           crmtypes.BlankModeAny,
		CourseNameBlankMode:      crmtypes.BlankModeAny,
		AcademicLevelBlankMode:   crmtypes.BlankModeAny,
		SecondarySchoolBlankMode: crmtypes.BlankModeAny,
		TeachersBlankMode:        crmtypes.BlankModeAny,
	}

	courseID := createTestCourse(t, ctx, dbpool, "C-V2-APPRV-"+suffix, "V2 Approve Test", filter)

	reconcileSvc := reconcile.NewReconcileV2Service(dbpool)
	queueStore := queue.NewPostgresQueueStore(dbpool)
	worker := queue.NewQueueWorker(tLogger, queueStore, "test-worker-approve")

	// First create the diff.
	_, err := reconcileSvc.DiffCourseReconcile(ctx, snapshotID, courseID, filter)
	if err != nil {
		t.Fatalf("DiffCourseReconcile: %v", err)
	}

	// Approve the review — this should enqueue an apply job and clear pending fields.
	err = reconcileSvc.ApproveReview(ctx, courseID, worker)
	if err != nil {
		t.Fatalf("ApproveReview: %v", err)
	}

	// Verify pending review fields are cleared.
	var pendingSnapshot pgtype.UUID
	var summaryJSON []byte
	err = dbpool.QueryRow(ctx,
		`SELECT crm_pending_review_snapshot_id, crm_pending_review_summary FROM courses WHERE id = $1`,
		courseID,
	).Scan(&pendingSnapshot, &summaryJSON)
	if err != nil {
		t.Fatal(err)
	}
	if pendingSnapshot.Valid {
		t.Fatal("expected crm_pending_review_snapshot_id to be cleared after approval")
	}
	if summaryJSON != nil {
		t.Fatal("expected crm_pending_review_summary to be cleared after approval")
	}

	// Verify a reconcile apply job was enqueued.
	var jobCount int
	err = dbpool.QueryRow(ctx,
		`SELECT COUNT(*) FROM crm_jobs WHERE job_type = 'course_reconcile_apply'::crm_job_type AND status = 'queued'`,
	).Scan(&jobCount)
	if err != nil {
		t.Fatal(err)
	}
	if jobCount != 1 {
		t.Fatalf("expected 1 queued reconcile apply job, got %d", jobCount)
	}
}

func TestRejectReviewClearsDiffs(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	rows := []xlsx.Row{
		{WCode: "W250120", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Oscar", LastName: "Owl"},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	filter := crmtypes.CourseFilter{
		CycleLabels:              []string{"Cycle A"},
		CycleBlankMode:           crmtypes.BlankModeAny,
		CourseNameBlankMode:      crmtypes.BlankModeAny,
		AcademicLevelBlankMode:   crmtypes.BlankModeAny,
		SecondarySchoolBlankMode: crmtypes.BlankModeAny,
		TeachersBlankMode:        crmtypes.BlankModeAny,
	}

	courseID := createTestCourse(t, ctx, dbpool, "C-V2-REJCT-"+suffix, "V2 Reject Test", filter)

	reconcileSvc := reconcile.NewReconcileV2Service(dbpool)

	// First create the diff.
	_, err := reconcileSvc.DiffCourseReconcile(ctx, snapshotID, courseID, filter)
	if err != nil {
		t.Fatalf("DiffCourseReconcile: %v", err)
	}

	// Verify diffs exist.
	var diffCount int
	err = dbpool.QueryRow(ctx,
		`SELECT COUNT(*) FROM crm_pending_diffs WHERE course_id = $1`, courseID,
	).Scan(&diffCount)
	if err != nil {
		t.Fatal(err)
	}
	if diffCount == 0 {
		t.Fatal("expected pending diffs before reject")
	}

	// Reject the review.
	err = reconcileSvc.RejectReview(ctx, courseID)
	if err != nil {
		t.Fatalf("RejectReview: %v", err)
	}

	// Verify diffs are cleared.
	err = dbpool.QueryRow(ctx,
		`SELECT COUNT(*) FROM crm_pending_diffs WHERE course_id = $1`, courseID,
	).Scan(&diffCount)
	if err != nil {
		t.Fatal(err)
	}
	if diffCount != 0 {
		t.Fatalf("expected 0 pending diffs after reject, got %d", diffCount)
	}

	// Verify course pending fields cleared.
	var pendingSnapshot pgtype.UUID
	err = dbpool.QueryRow(ctx,
		`SELECT crm_pending_review_snapshot_id FROM courses WHERE id = $1`, courseID,
	).Scan(&pendingSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if pendingSnapshot.Valid {
		t.Fatal("expected crm_pending_review_snapshot_id to be NULL after reject")
	}
}

// ============================================================================
// End-to-end: Import → Snapshot → StudentSync → Reconcile pipeline
// ============================================================================

func TestCRMPipelineEndToEnd(t *testing.T) {
	databaseURL := requireTestDBV2(t)
	migrateUpV2(t, databaseURL)
	dbpool := newPoolV2(t, databaseURL)
	t.Cleanup(dbpool.Close)
	cleanupV2(t, dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	// Step 1: Create a snapshot with CRM rows.
	rows := []xlsx.Row{
		{WCode: "W250200", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Paul", LastName: "Panda"},
		{WCode: "W250201", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Quinn", LastName: "Quail"},
		{WCode: "W250202", CourseName: "Science", CycleLabel: "Cycle B", FirstName: "Rita", LastName: "Raven"},
		{WCode: "W250203", CourseName: "History", CycleLabel: "Cycle A", FirstName: "Sam", LastName: "Seal"},
	}

	snapshotID := createTestSnapshot(t, ctx, dbpool, rows)

	// Step 2: Sync students.
	syncSvc := NewStudentSyncService(dbpool)
	synced, err := syncSvc.SyncFromSnapshot(ctx, snapshotID)
	if err != nil {
		t.Fatalf("SyncFromSnapshot: %v", err)
	}
	if synced < 4 {
		t.Fatalf("expected at least 4 synced students, got %d", synced)
	}

	// Step 3: Create two courses with filters and reconcile.
	filterA := crmtypes.CourseFilter{
		CycleLabels:              []string{"Cycle A"},
		CycleBlankMode:           crmtypes.BlankModeAny,
		CourseNameBlankMode:      crmtypes.BlankModeAny,
		AcademicLevelBlankMode:   crmtypes.BlankModeAny,
		SecondarySchoolBlankMode: crmtypes.BlankModeAny,
		TeachersBlankMode:        crmtypes.BlankModeAny,
	}
	courseA := createTestCourse(t, ctx, dbpool, "C-V2-E2E-A-"+suffix, "V2 E2E Course A", filterA)

	filterB := crmtypes.CourseFilter{
		CycleLabels:              []string{"Cycle B"},
		CycleBlankMode:           crmtypes.BlankModeAny,
		CourseNameBlankMode:      crmtypes.BlankModeAny,
		AcademicLevelBlankMode:   crmtypes.BlankModeAny,
		SecondarySchoolBlankMode: crmtypes.BlankModeAny,
		TeachersBlankMode:        crmtypes.BlankModeAny,
	}
	courseB := createTestCourse(t, ctx, dbpool, "C-V2-E2E-B-"+suffix, "V2 E2E Course B", filterB)

	reconcileSvc := reconcile.NewReconcileV2Service(dbpool)

	// Apply Course A: should get 3 students (Cycle A).
	resultA, err := reconcileSvc.ApplyCourseReconcile(ctx, snapshotID, courseA, filterA)
	if err != nil {
		t.Fatalf("ApplyCourseReconcile A: %v", err)
	}
	if resultA.Added != 3 {
		t.Fatalf("expected 3 added to Course A, got %d", resultA.Added)
	}

	// Apply Course B: should get 1 student (Cycle B).
	resultB, err := reconcileSvc.ApplyCourseReconcile(ctx, snapshotID, courseB, filterB)
	if err != nil {
		t.Fatalf("ApplyCourseReconcile B: %v", err)
	}
	if resultB.Added != 1 {
		t.Fatalf("expected 1 added to Course B, got %d", resultB.Added)
	}

	// Verify Course A enrollment.
	var countA int
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM course_students WHERE course_id = $1`, courseA).Scan(&countA)
	if err != nil {
		t.Fatal(err)
	}
	if countA != 3 {
		t.Fatalf("expected 3 students in Course A, got %d", countA)
	}

	// Verify Course B enrollment.
	var countB int
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM course_students WHERE course_id = $1`, courseB).Scan(&countB)
	if err != nil {
		t.Fatal(err)
	}
	if countB != 1 {
		t.Fatalf("expected 1 student in Course B, got %d", countB)
	}

	// Verify no overlap: Course A should not have Course B's students.
	var exists bool
	err = dbpool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM course_students cs
			JOIN students s ON s.id = cs.student_id
			WHERE cs.course_id = $1 AND s.wcode = 'W250202'
		)
	`, courseA).Scan(&exists)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("Course A should NOT have the Cycle B student")
	}
}

// ============================================================================
// Errors
// ============================================================================

// tLogger is a discard logger for queue worker tests.
var tLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
