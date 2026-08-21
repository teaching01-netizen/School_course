package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/idempotency"
)

const (
	retentionCleanupInterval = 6 * time.Hour
	retentionCleanupTimeout  = 30 * time.Second
)

type retentionCleanup struct {
	log  *slog.Logger
	db   *pgxpool.Pool
	stop context.CancelFunc
}

func newRetentionCleanup(log *slog.Logger, db *pgxpool.Pool) *retentionCleanup {
	return &retentionCleanup{log: log, db: db}
}

func (c *retentionCleanup) Start(ctx context.Context) {
	cleanupCtx, cancel := context.WithCancel(ctx)
	c.stop = cancel
	go func() {
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
}
