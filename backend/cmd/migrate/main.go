package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func withPgBouncerSafeSettings(databaseURL string) string {
	// PgBouncer in transaction pooling mode is incompatible with prepared statements.
	// Supabase "pooler" URLs (often :6543) are PgBouncer.
	if os.Getenv("PGBOUNCER") != "" || strings.Contains(databaseURL, "pooler.supabase.com") || strings.Contains(databaseURL, ":6543/") {
		u, err := url.Parse(databaseURL)
		if err != nil {
			return databaseURL
		}
		q := u.Query()
		if q.Get("statement_cache_capacity") == "" {
			q.Set("statement_cache_capacity", "0")
		}
		// pgx default is cache_statement; force simple protocol when statement cache is disabled.
		if q.Get("default_query_exec_mode") == "" {
			q.Set("default_query_exec_mode", "simple_protocol")
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	return databaseURL
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("missing DATABASE_URL")
		os.Exit(2)
	}
	databaseURL = withPgBouncerSafeSettings(databaseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Error("open db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		log.Error("ping db", "err", err)
		os.Exit(1)
	}

	// Acquire advisory lock to prevent concurrent migrations (e.g. parallel Railway deploys).
	// Use pg_try_advisory_lock in a poll loop so context cancellation works properly.
	// pg_advisory_lock is avoided because it blocks inside the Postgres session even when
	// the client disconnects — connection poolers (Supavisor, PgBouncer) keep the session
	// alive, causing the lock to persist across client reconnects.
	lockCtx, lockCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer lockCancel()
	var locked bool
	for {
		if err := db.QueryRowContext(lockCtx, `SELECT pg_try_advisory_lock(12345)`).Scan(&locked); err != nil {
			log.Error("acquire advisory lock", "err", err)
			os.Exit(1)
		}
		if locked {
			break
		}
		select {
		case <-lockCtx.Done():
			log.Error("acquire advisory lock", "err", "timeout: could not acquire lock within 60s")
			os.Exit(1)
		case <-time.After(500 * time.Millisecond):
		}
	}
	defer db.ExecContext(context.Background(), `SELECT pg_advisory_unlock(12345)`)

	if err := goose.SetDialect("postgres"); err != nil {
		log.Error("set goose dialect", "err", err)
		os.Exit(1)
	}

	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "db/migrations"
	}

	cmd := "up"
	if len(os.Args) >= 2 {
		cmd = os.Args[1]
	}

	var runErr error
	switch cmd {
	case "up":
		runErr = goose.UpContext(ctx, db, migrationsDir)
	case "down":
		runErr = goose.DownContext(ctx, db, migrationsDir)
	case "status":
		runErr = goose.StatusContext(ctx, db, migrationsDir)
	default:
		runErr = fmt.Errorf("unknown command %q (expected up|down|status)", cmd)
	}

	// Some PgBouncer / cloud Postgres setups can race creating the goose version table.
	// If we hit that transient error, retry once.
	if runErr != nil && strings.Contains(runErr.Error(), "relation \"goose_db_version\" already exists") {
		time.Sleep(200 * time.Millisecond)
		switch cmd {
		case "up":
			runErr = goose.UpContext(ctx, db, migrationsDir)
		case "down":
			runErr = goose.DownContext(ctx, db, migrationsDir)
		case "status":
			runErr = goose.StatusContext(ctx, db, migrationsDir)
		}
	}

	if runErr != nil {
		if errors.Is(runErr, context.DeadlineExceeded) {
			log.Error("migrations timed out", "err", runErr)
			os.Exit(1)
		}
		log.Error("migrations failed", "err", runErr)
		os.Exit(1)
	}
}
