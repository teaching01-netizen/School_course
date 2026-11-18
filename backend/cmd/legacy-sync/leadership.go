package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// detectorLeadership owns the session-scoped lock for the runner's sweep.
// The runner checks it serially, so the connection never has concurrent users.
type detectorLeadership struct {
	pool *pgxpool.Pool
	conn *pgxpool.Conn
}

func (l *detectorLeadership) isLeader(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if l.conn != nil {
		// Taking the same advisory lock again would stack an acquisition.
		if err := l.conn.Ping(ctx); err != nil {
			l.close()
			return false, err
		}
		return true, nil
	}

	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	l.conn = conn
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('legacy-sync-detector', 0))`).Scan(&locked); err != nil {
		l.close()
		return false, err
	}
	if !locked {
		l.conn = nil
		conn.Release()
	}
	return locked, nil
}

func (l *detectorLeadership) close() {
	if l.conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// End the session even when the lock query failed after reaching Postgres.
	_ = l.conn.Hijack().Close(ctx)
	l.conn = nil
}
