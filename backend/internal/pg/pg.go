package pg

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	return newPool(ctx, databaseURL, 10, true)
}

// NewRealtimePool creates the small dedicated pool used by PostgreSQL
// LISTEN/NOTIFY. One connection can remain reserved by LISTEN while the other
// publishes notifications.
func NewRealtimePool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	return newPool(ctx, databaseURL, 2, false)
}

func newPool(ctx context.Context, databaseURL string, defaultMaxConns int32, honorPoolMaxConns bool) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	// PgBouncer in transaction pooling mode is incompatible with prepared statements.
	// Supabase "pooler" URLs (often :6543) are PgBouncer.
	if os.Getenv("PGBOUNCER") != "" || IsTransactionPoolerURL(databaseURL) {
		cfg.ConnConfig.StatementCacheCapacity = 0
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	cfg.MaxConns = defaultMaxConns
	if v := os.Getenv("POOL_MAX_CONNS"); honorPoolMaxConns && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxConns = int32(n)
		}
	}
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 5 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool new: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return pool, nil
}

func IsTransactionPoolerURL(databaseURL string) bool {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	return err == nil && cfg.ConnConfig.Port == 6543
}

// ResolveRealtimeDatabaseURL selects a session-capable endpoint for
// LISTEN/NOTIFY. Transaction pooling cannot preserve LISTEN session state.
func ResolveRealtimeDatabaseURL(primaryURL, overrideURL string, primaryIsTransactionPooler bool) (string, error) {
	primaryURL = strings.TrimSpace(primaryURL)
	overrideURL = strings.TrimSpace(overrideURL)

	if overrideURL != "" {
		if IsTransactionPoolerURL(overrideURL) {
			return "", fmt.Errorf("REALTIME_DATABASE_URL must use a direct or session-capable PostgreSQL endpoint")
		}
		return overrideURL, nil
	}

	if primaryIsTransactionPooler || IsTransactionPoolerURL(primaryURL) {
		return "", fmt.Errorf("DATABASE_URL uses transaction pooling; set REALTIME_DATABASE_URL to a direct or session-capable PostgreSQL endpoint")
	}
	return primaryURL, nil
}
