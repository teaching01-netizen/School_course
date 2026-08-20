package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
	"warwick-institute/internal/ratelimit"
)

const (
	sweepInterval = 1 * time.Hour
	sweepTimeout  = 30 * time.Second
	// Revoked sessions are kept for this long after revocation so recent
	// revocations remain visible to session-list and audit queries.
	revokedSessionsRetention = 30 * 24 * time.Hour
	// Rate-limit events are only consulted inside their sliding window (at
	// most an hour today); a one-day retention is a generous margin against
	// larger future windows.
	rateLimitEventsRetention = 24 * time.Hour
)

// maintenanceSweeper periodically deletes expired auth sessions and stale
// rate-limit events. Both tables only ever accumulate rows — nothing else
// removes them — so without this loop they grow without bound. Each sweep is
// a bounded batch loop and idempotent, so concurrent replicas only do a
// little redundant work.
type maintenanceSweeper struct {
	log       *slog.Logger
	sessions  *auth.PGSessionStore
	rateLimit *ratelimit.Store
	stop      context.CancelFunc
}

func newMaintenanceSweeper(log *slog.Logger, db *pgxpool.Pool) *maintenanceSweeper {
	return &maintenanceSweeper{
		log:       log,
		sessions:  auth.NewPGSessionStore(db, log),
		rateLimit: ratelimit.NewStore(db),
	}
}

// Start runs the sweep loop until ctx is cancelled or Stop is called.
func (m *maintenanceSweeper) Start(ctx context.Context) {
	sweepCtx, cancel := context.WithCancel(ctx)
	m.stop = cancel
	go func() {
		ticker := time.NewTicker(sweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-sweepCtx.Done():
				return
			case <-ticker.C:
				m.runOnce(sweepCtx)
			}
		}
	}()
}

func (m *maintenanceSweeper) Stop() {
	if m.stop != nil {
		m.stop()
	}
}

func (m *maintenanceSweeper) runOnce(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, sweepTimeout)
	defer cancel()

	if n, err := m.sessions.DeleteExpired(ctx, time.Now().UTC().Add(-revokedSessionsRetention)); err != nil {
		m.log.Error("maintenance sweep: auth sessions failed", "error", err)
	} else if n > 0 {
		m.log.Info("maintenance sweep: deleted expired auth sessions", "rows", n)
	}

	if n, err := m.rateLimit.SweepExpired(ctx, time.Now().UTC().Add(-rateLimitEventsRetention), 5000); err != nil {
		m.log.Error("maintenance sweep: rate limit events failed", "error", err)
	} else if n > 0 {
		m.log.Info("maintenance sweep: deleted stale rate limit events", "rows", n)
	}
}