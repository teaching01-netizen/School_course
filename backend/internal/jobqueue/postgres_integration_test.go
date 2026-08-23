package jobqueue

import (
	"context"
	"database/sql"
	"fmt"
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

var (
	queueMigrationsOnce sync.Once
	queueMigrationsErr  error
)

func queueTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL queue integration tests")
	}
	queueMigrationsOnce.Do(func() {
		migrationURL := databaseURL
		if strings.Contains(migrationURL, "?") {
			migrationURL += "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			migrationURL += "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", migrationURL)
		if err != nil {
			queueMigrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			queueMigrationsErr = err
			return
		}
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			queueMigrationsErr = fmt.Errorf("locate queue migration directory")
			return
		}
		queueMigrationsErr = goose.UpContext(context.Background(), db, filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
	})
	if queueMigrationsErr != nil {
		t.Fatal(queueMigrationsErr)
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func queueUniqueKey(t *testing.T) string {
	t.Helper()
	return "test:legacy:" + strings.ReplaceAll(t.Name(), "/", ":") + ":" + fmt.Sprint(time.Now().UnixNano())
}

func cleanupQueueTestJobs(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `DELETE FROM legacy_sync_jobs`); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStore_ConcurrentClaimsHaveOneOwner(t *testing.T) {
	pool := queueTestPool(t)
	cleanupQueueTestJobs(t, pool)
	store := NewPostgresStore(sqldb.New(pool))
	ctx := t.Context()
	uniqueKey := queueUniqueKey(t)
	if _, err := store.Enqueue(ctx, EnqueueRequest{JobType: "test_queue_concurrent", EntityType: "course", ExternalID: "7306", UniqueKey: uniqueKey, RunAfter: time.Now().UTC(), MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 10)
	for worker := range 10 {
		worker := worker
		go func() {
			<-start
			_, err := store.Claim(ctx, fmt.Sprintf("queue-worker-%d", worker), time.Now().UTC(), time.Minute)
			results <- err
		}()
	}
	close(start)
	claimed := 0
	for range 10 {
		err := <-results
		if err == nil {
			claimed++
			continue
		}
		if err != nil && err != ErrNoJobs {
			t.Fatalf("claim error = %v, want ErrNoJobs for losers", err)
		}
	}
	if claimed != 1 {
		t.Fatalf("successful claims = %d, want 1", claimed)
	}
}

func TestPostgresStore_ClaimsAbsenceFormActiveCourseFirst(t *testing.T) {
	pool := queueTestPool(t)
	cleanupQueueTestJobs(t, pool)
	ctx := t.Context()
	suffix := fmt.Sprint(time.Now().UnixNano())
	activeLegacyID := "queue-active-" + suffix
	otherLegacyID := "queue-other-" + suffix

	var subjectID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO subjects (code, name)
		VALUES ($1, $2)
		RETURNING id
	`, "QUEUE-SUBJECT-"+suffix, "Queue subject").Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	var activeCourseID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO courses (code, name, subject_id, legacy_course_id, source_kind, absence_form_visible)
		VALUES ($1, $2, $3, $4, 'legacy', true)
		RETURNING id
	`, "QUEUE-ACTIVE-"+suffix, "Active queue course", subjectID, activeLegacyID).Scan(&activeCourseID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2)`, subjectID, activeCourseID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM subject_active_courses WHERE subject_id = $1`, subjectID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM courses WHERE id = $1`, activeCourseID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM subjects WHERE id = $1`, subjectID)
	})

	store := NewPostgresStore(sqldb.New(pool))
	queuedAt := time.Now().UTC().Add(-time.Second)
	if _, err := store.Enqueue(ctx, EnqueueRequest{
		JobType: "legacy_refresh_course", EntityType: "course", ExternalID: otherLegacyID,
		UniqueKey: queueUniqueKey(t) + ":other", Priority: 0, RunAfter: queuedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue(ctx, EnqueueRequest{
		JobType: "legacy_refresh_course", EntityType: "course", ExternalID: activeLegacyID,
		UniqueKey: queueUniqueKey(t) + ":active", Priority: 100, RunAfter: queuedAt,
	}); err != nil {
		t.Fatal(err)
	}

	claimed, err := store.Claim(ctx, "queue-worker-active-first", time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ExternalID != activeLegacyID {
		t.Fatalf("claimed legacy course %q, want absence-form active course %q", claimed.ExternalID, activeLegacyID)
	}
}

func TestPostgresStore_ExpiredLeaseCanBeReclaimedAndWrongOwnerRejected(t *testing.T) {
	pool := queueTestPool(t)
	cleanupQueueTestJobs(t, pool)
	queries := sqldb.New(pool)
	store := NewPostgresStore(queries)
	ctx := t.Context()
	uniqueKey := queueUniqueKey(t)
	enqueued, err := store.Enqueue(ctx, EnqueueRequest{JobType: "test_queue_reclaim", UniqueKey: uniqueKey, RunAfter: time.Now().UTC(), MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(ctx, "queue-worker-a", time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Heartbeat(ctx, claimed.ID, "queue-worker-b", time.Now().UTC(), time.Minute); err == nil {
		t.Fatal("wrong worker heartbeat unexpectedly succeeded")
	}
	if err := store.Complete(ctx, claimed.ID, "queue-worker-b"); err == nil {
		t.Fatal("wrong worker completion unexpectedly succeeded")
	}
	if _, err := pool.Exec(ctx, `UPDATE legacy_sync_jobs SET locked_until=now()-interval '1 second' WHERE id=$1`, enqueued.ID); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := store.Claim(ctx, "queue-worker-b", time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed.ID != enqueued.ID || reclaimed.Attempt != 2 {
		t.Fatalf("reclaimed job = %+v, want id %s at attempt 2", reclaimed, enqueued.ID)
	}
}

func TestPostgresStore_SubsecondLeaseHasMinimumDuration(t *testing.T) {
	pool := queueTestPool(t)
	cleanupQueueTestJobs(t, pool)
	store := NewPostgresStore(sqldb.New(pool))
	ctx := t.Context()
	enqueued, err := store.Enqueue(ctx, EnqueueRequest{JobType: "test_queue_subsecond_lease", UniqueKey: queueUniqueKey(t), RunAfter: time.Now().UTC(), MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(ctx, "queue-worker", time.Now().UTC(), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	var lockedUntil time.Time
	if err := pool.QueryRow(ctx, `SELECT locked_until FROM legacy_sync_jobs WHERE id=$1`, claimed.ID).Scan(&lockedUntil); err != nil {
		t.Fatal(err)
	}
	if !lockedUntil.After(time.Now().UTC()) {
		t.Fatalf("locked_until = %s, want future lease", lockedUntil)
	}
	if claimed.ID != enqueued.ID {
		t.Fatalf("claimed job = %s, want %s", claimed.ID, enqueued.ID)
	}
}

func TestPostgresStore_MaxAttemptsDeadLetterJob(t *testing.T) {
	pool := queueTestPool(t)
	cleanupQueueTestJobs(t, pool)
	store := NewPostgresStore(sqldb.New(pool))
	ctx := t.Context()
	uniqueKey := queueUniqueKey(t)
	if _, err := store.Enqueue(ctx, EnqueueRequest{JobType: "test_queue_dead", UniqueKey: uniqueKey, RunAfter: time.Now().UTC(), MaxAttempts: 1}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(ctx, "queue-worker", time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(ctx, claimed.ID, "queue-worker", time.Now().UTC(), fmt.Errorf("source unavailable")); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM legacy_sync_jobs WHERE id=$1`, claimed.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "dead" {
		t.Fatalf("job status = %q, want dead", status)
	}
}

// TestPostgresStore_RetryDeadJobCreatesDeadLetter pins CB-08: a job that
// exhausts its attempts must land in legacy_sync_dead_letters so the failure
// is visible in the admin health view, not only as status='dead' on the job
// row. Retryable failures must not create dead letters.
func TestPostgresStore_RetryDeadJobCreatesDeadLetter(t *testing.T) {
	pool := queueTestPool(t)
	cleanupQueueTestJobs(t, pool)
	store := NewPostgresStore(sqldb.New(pool))
	ctx := t.Context()

	// Retryable job: attempt 1 of 3 must not dead-letter.
	retryableKey := queueUniqueKey(t)
	if _, err := store.Enqueue(ctx, EnqueueRequest{JobType: "test_queue_dl", EntityType: "course", ExternalID: "dl-retryable", UniqueKey: retryableKey, RunAfter: time.Now().UTC(), MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	first, err := store.Claim(ctx, "queue-worker", time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(ctx, first.ID, "queue-worker", time.Now().UTC(), fmt.Errorf("transient")); err != nil {
		t.Fatal(err)
	}

	// Dead job: exhausting attempts must create a dead letter.
	deadKey := queueUniqueKey(t)
	if _, err := store.Enqueue(ctx, EnqueueRequest{JobType: "test_queue_dl", EntityType: "course", ExternalID: "dl-final", UniqueKey: deadKey, RunAfter: time.Now().UTC(), MaxAttempts: 1}); err != nil {
		t.Fatal(err)
	}
	second, err := store.Claim(ctx, "queue-worker", time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(ctx, second.ID, "queue-worker", time.Now().UTC(), fmt.Errorf("course code collision")); err != nil {
		t.Fatal(err)
	}

	var retryableLetters, deadLetters int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_dead_letters WHERE unique_key=$1`, retryableKey).Scan(&retryableLetters); err != nil {
		t.Fatal(err)
	}
	if retryableLetters != 0 {
		t.Fatalf("retryable failure created %d dead letters, want 0", retryableLetters)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_dead_letters WHERE unique_key=$1 AND external_id='dl-final' AND last_error='course code collision'`, deadKey).Scan(&deadLetters); err != nil {
		t.Fatal(err)
	}
	if deadLetters != 1 {
		t.Fatalf("dead job produced %d dead letters, want 1", deadLetters)
	}
}

func TestPostgresStore_RetryRejectsWrongOwner(t *testing.T) {
	pool := queueTestPool(t)
	cleanupQueueTestJobs(t, pool)
	store := NewPostgresStore(sqldb.New(pool))
	ctx := t.Context()
	if _, err := store.Enqueue(ctx, EnqueueRequest{JobType: "test_queue_owner", UniqueKey: queueUniqueKey(t), RunAfter: time.Now().UTC(), MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(ctx, "queue-worker-a", time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(ctx, claimed.ID, "queue-worker-b", time.Now().UTC(), fmt.Errorf("boom")); err == nil {
		t.Fatal("wrong worker retry unexpectedly succeeded")
	}
	var status, lockedBy string
	if err := pool.QueryRow(ctx, `SELECT status, COALESCE(locked_by, '') FROM legacy_sync_jobs WHERE id=$1`, claimed.ID).Scan(&status, &lockedBy); err != nil {
		t.Fatal(err)
	}
	if status != "running" || lockedBy != "queue-worker-a" {
		t.Fatalf("job after wrong-worker retry = status %q locked_by %q, want running/queue-worker-a", status, lockedBy)
	}
	if err := store.Retry(ctx, claimed.ID, "queue-worker-a", time.Now().UTC(), fmt.Errorf("transient")); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status, COALESCE(locked_by, '') FROM legacy_sync_jobs WHERE id=$1`, claimed.ID).Scan(&status, &lockedBy); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || lockedBy != "" {
		t.Fatalf("job after owner retry = status %q locked_by %q, want queued and unlocked", status, lockedBy)
	}
}

func TestPostgresStore_RetryRejectsStaleLeaseAfterReclaim(t *testing.T) {
	pool := queueTestPool(t)
	cleanupQueueTestJobs(t, pool)
	store := NewPostgresStore(sqldb.New(pool))
	ctx := t.Context()
	if _, err := store.Enqueue(ctx, EnqueueRequest{JobType: "test_queue_stale", UniqueKey: queueUniqueKey(t), RunAfter: time.Now().UTC(), MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(ctx, "queue-worker-a", time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE legacy_sync_jobs SET locked_until=now()-interval '1 second' WHERE id=$1`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := store.Claim(ctx, "queue-worker-b", time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed.ID != claimed.ID {
		t.Fatalf("reclaimed job = %s, want %s", reclaimed.ID, claimed.ID)
	}
	if err := store.Retry(ctx, claimed.ID, "queue-worker-a", time.Now().UTC(), fmt.Errorf("stale")); err == nil {
		t.Fatal("stale worker retry unexpectedly succeeded after reclaim")
	}
	if err := store.Retry(ctx, reclaimed.ID, "queue-worker-b", time.Now().UTC(), fmt.Errorf("transient")); err != nil {
		t.Fatal(err)
	}
}
