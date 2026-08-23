package main

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

var (
	retentionMigrationsOnce sync.Once
	retentionMigrationsErr  error
)

func retentionTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run retention integration tests")
	}

	retentionMigrationsOnce.Do(func() {
		migrationURL := databaseURL
		if strings.Contains(migrationURL, "?") {
			migrationURL += "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			migrationURL += "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", migrationURL)
		if err != nil {
			retentionMigrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			retentionMigrationsErr = err
			return
		}
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			retentionMigrationsErr = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "db", "migrations"))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		retentionMigrationsErr = goose.UpContext(ctx, db, migrationsDir)
	})
	if retentionMigrationsErr != nil {
		t.Fatal(retentionMigrationsErr)
	}

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestCleanupLegacySyncOperationalHistoryRetainsLiveAndFunctionalRows(t *testing.T) {
	pool := retentionTestPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	suffix := uuid.New().String()
	old := time.Now().UTC().Add(-48 * time.Hour)
	recent := time.Now().UTC().Add(-2 * time.Hour)
	cutoff := time.Now().UTC().Add(-legacySyncOperationalRetention)

	oldDeadKey := "retention-old-dead-" + suffix
	recentDeadKey := "retention-recent-dead-" + suffix
	oldCompletedJobKey := "retention-old-completed-job-" + suffix
	recentCompletedJobKey := "retention-recent-completed-job-" + suffix
	oldRunningJobKey := "retention-old-running-job-" + suffix
	oldQueuedJobKey := "retention-old-queued-job-" + suffix
	oldRunID := uuid.New()
	recentRunID := uuid.New()
	runningRunID := uuid.New()
	outboxKey := "retention-outbox-" + suffix
	conflictExternalID := "retention-conflict-" + suffix

	if _, err := tx.Exec(ctx, `
INSERT INTO legacy_sync_dead_letters (job_type, unique_key, entity_type, external_id, last_error, created_at)
VALUES
('legacy_refresh_course', $1, 'course', $1, 'old failure', $2),
('legacy_refresh_course', $3, 'course', $3, 'recent failure', $4)
`, oldDeadKey, old, recentDeadKey, recent); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO legacy_sync_jobs (job_type, unique_key, status, created_at, updated_at)
VALUES
('legacy_refresh_course', $1, 'completed', $2, $2),
('legacy_refresh_course', $3, 'completed', $4, $4),
('legacy_refresh_course', $5, 'running', $2, $2),
('legacy_refresh_course', $6, 'queued', $2, $2)
`, oldCompletedJobKey, old, recentCompletedJobKey, recent, oldRunningJobKey, oldQueuedJobKey); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO legacy_sync_runs (id, mode, status, started_at, completed_at)
VALUES
($1, 'full_sweep', 'completed', $4, $4),
($2, 'full_sweep', 'completed', $5, $5),
($3, 'full_sweep', 'running', $4, NULL)
`, oldRunID, recentRunID, runningRunID, old, recent); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO legacy_sync_run_progress (run_id, phase, updated_at)
VALUES ($1, 'completed', $2), ($3, 'running', $2)
`, oldRunID, old, runningRunID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO legacy_sync_outbox (source_event_key, event_type, channel, entity_type, external_id, payload, status, created_at)
VALUES ($1, 'retention.test', 'realtime', 'course', $2, '{}'::jsonb, 'published', $3)
`, outboxKey, suffix, old); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO legacy_sync_conflicts (entity_type, external_id, conflict_type, category, source_payload, status, created_at)
VALUES ('course', $1, 'retention_test', 'internal_bug', '{}'::jsonb, 'open', $2)
`, conflictExternalID, old); err != nil {
		t.Fatal(err)
	}

	deleted, err := cleanupLegacySyncOperationalHistory(ctx, tx, cutoff)
	if err != nil {
		t.Fatalf("cleanupLegacySyncOperationalHistory: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted rows = %d, want 3", deleted)
	}

	assertExists := func(query string, args ...any) bool {
		t.Helper()
		var exists bool
		if err := tx.QueryRow(ctx, query, args...).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		return exists
	}
	if assertExists(`SELECT EXISTS (SELECT 1 FROM legacy_sync_dead_letters WHERE unique_key = $1)`, oldDeadKey) {
		t.Fatal("old dead letter was retained")
	}
	if !assertExists(`SELECT EXISTS (SELECT 1 FROM legacy_sync_dead_letters WHERE unique_key = $1)`, recentDeadKey) {
		t.Fatal("recent dead letter was deleted")
	}
	if assertExists(`SELECT EXISTS (SELECT 1 FROM legacy_sync_jobs WHERE unique_key = $1)`, oldCompletedJobKey) {
		t.Fatal("old completed job was retained")
	}
	for _, key := range []string{recentCompletedJobKey, oldRunningJobKey, oldQueuedJobKey} {
		if !assertExists(`SELECT EXISTS (SELECT 1 FROM legacy_sync_jobs WHERE unique_key = $1)`, key) {
			t.Fatalf("live job %q was deleted", key)
		}
	}
	if assertExists(`SELECT EXISTS (SELECT 1 FROM legacy_sync_runs WHERE id = $1)`, oldRunID) {
		t.Fatal("old completed run was retained")
	}
	for _, id := range []uuid.UUID{recentRunID, runningRunID} {
		if !assertExists(`SELECT EXISTS (SELECT 1 FROM legacy_sync_runs WHERE id = $1)`, id) {
			t.Fatalf("live run %s was deleted", id)
		}
	}
	if assertExists(`SELECT EXISTS (SELECT 1 FROM legacy_sync_run_progress WHERE run_id = $1)`, oldRunID) {
		t.Fatal("progress for old run was not cascaded")
	}
	if !assertExists(`SELECT EXISTS (SELECT 1 FROM legacy_sync_run_progress WHERE run_id = $1)`, runningRunID) {
		t.Fatal("progress for running run was deleted")
	}
	if !assertExists(`SELECT EXISTS (SELECT 1 FROM legacy_sync_outbox WHERE source_event_key = $1)`, outboxKey) {
		t.Fatal("outbox idempotency row was deleted")
	}
	if !assertExists(`SELECT EXISTS (SELECT 1 FROM legacy_sync_conflicts WHERE external_id = $1)`, conflictExternalID) {
		t.Fatal("conflict history row was deleted")
	}
}
