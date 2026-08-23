package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/idempotency"
)

const (
	retentionCleanupInterval       = 6 * time.Hour
	retentionCleanupTimeout        = 30 * time.Second
	legacySyncOperationalRetention = 24 * time.Hour
	legacySyncCleanupBatchSize     = 5000
)

type retentionCleanup struct {
	log  *slog.Logger
	db   *pgxpool.Pool
	stop context.CancelFunc
}

type legacySyncRetentionDB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func newRetentionCleanup(log *slog.Logger, db *pgxpool.Pool) *retentionCleanup {
	return &retentionCleanup{log: log, db: db}
}

func (c *retentionCleanup) Start(ctx context.Context) {
	cleanupCtx, cancel := context.WithCancel(ctx)
	c.stop = cancel
	go func() {
		c.runOnce(cleanupCtx)
		ticker := time.NewTicker(retentionCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-cleanupCtx.Done():
				return
			case <-ticker.C:
				c.runOnce(cleanupCtx)
			}
		}
	}()
}

func (c *retentionCleanup) Stop() {
	if c.stop != nil {
		c.stop()
	}
}

func (c *retentionCleanup) runOnce(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, retentionCleanupTimeout)
	defer cancel()
	cutoff := time.Now().UTC().Add(-legacySyncOperationalRetention)

	if rows, err := idempotency.CleanupExpired(ctx, c.db, 5000); err != nil {
		c.log.Error("retention cleanup: idempotency keys failed", "error", err)
	} else if rows > 0 {
		c.log.Info("retention cleanup: deleted expired idempotency keys", "rows", rows)
	}

	var rows int
	if err := c.db.QueryRow(ctx, `SELECT cleanup_stale_parent_verification_sessions()`).Scan(&rows); err != nil {
		c.log.Error("retention cleanup: student auth rows failed", "error", err)
	} else if rows > 0 {
		c.log.Info("retention cleanup: deleted expired student auth rows", "rows", rows)
	}

	legacyRows, err := cleanupLegacySyncOperationalHistory(ctx, c.db, cutoff)
	if err != nil {
		c.log.Error("retention cleanup: legacy sync history failed", "error", err)
	} else if legacyRows > 0 {
		c.log.Info("retention cleanup: deleted legacy sync operational history", "rows", legacyRows, "retention", legacySyncOperationalRetention.String())
	}
}

func cleanupLegacySyncOperationalHistory(ctx context.Context, db legacySyncRetentionDB, cutoff time.Time) (int64, error) {
	deadLetters, err := deleteLegacySyncRows(ctx, db, `
WITH victims AS (
    SELECT id
    FROM legacy_sync_dead_letters
    WHERE created_at < $1
    ORDER BY created_at, id
    LIMIT $2
)
DELETE FROM legacy_sync_dead_letters AS target
USING victims
WHERE target.id = victims.id
`, cutoff)
	if err != nil {
		return deadLetters, fmt.Errorf("dead letters: %w", err)
	}

	terminalJobs, err := deleteLegacySyncRows(ctx, db, `
WITH victims AS (
    SELECT id
    FROM legacy_sync_jobs
    WHERE status IN ('completed', 'dead')
      AND updated_at < $1
    ORDER BY updated_at, id
    LIMIT $2
)
DELETE FROM legacy_sync_jobs AS target
USING victims
WHERE target.id = victims.id
`, cutoff)
	if err != nil {
		return deadLetters + terminalJobs, fmt.Errorf("terminal jobs: %w", err)
	}

	completedRuns, err := deleteLegacySyncRows(ctx, db, `
WITH victims AS (
    SELECT id
    FROM legacy_sync_runs
    WHERE status IN ('completed', 'failed', 'paused')
      AND COALESCE(completed_at, started_at) < $1
    ORDER BY COALESCE(completed_at, started_at), id
    LIMIT $2
)
DELETE FROM legacy_sync_runs AS target
USING victims
WHERE target.id = victims.id
`, cutoff)
	if err != nil {
		return deadLetters + terminalJobs + completedRuns, fmt.Errorf("completed runs: %w", err)
	}

	return deadLetters + terminalJobs + completedRuns, nil
}

func deleteLegacySyncRows(ctx context.Context, db legacySyncRetentionDB, statement string, cutoff time.Time) (int64, error) {
	var total int64
	for {
		tag, err := db.Exec(ctx, statement, cutoff, legacySyncCleanupBatchSize)
		if err != nil {
			return total, err
		}
		deleted := tag.RowsAffected()
		total += deleted
		if deleted < legacySyncCleanupBatchSize {
			return total, nil
		}
	}
}
