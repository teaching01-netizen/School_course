package idempotency

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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
)

func requireTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run idempotency tests")
	}
	return url
}

var migrationsOnce sync.Once
var migrationsErr error

func migrateUpOnce(t *testing.T, databaseURL string) {
	t.Helper()
	migrationsOnce.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErr = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		migrationsErr = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErr != nil {
		t.Fatal(migrationsErr)
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

func TestErrStaleIdempotencyRecord_Error(t *testing.T) {
	err := &ErrStaleIdempotencyRecord{Key: "test-key-123"}
	if err.Error() != `stale idempotency record for key "test-key-123" (server crashed before completion); retry with a new idempotency key` {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func TestCleanupExpired_DeletesExpiredRecords(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	actorID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	scope := "test-cleanup-expired"
	now := time.Now().UTC()

	// Insert expired keys.
	expiredExpiry := pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}
	for i := 0; i < 5; i++ {
		_, err := q.IdempotencyAcquire(ctx, sqldb.IdempotencyAcquireParams{
			ActorUserID:    actorID,
			Scope:          scope,
			IdempotencyKey: "expired-" + uuid.New().String(),
			RequestHash:    "hash-expired",
			ExpiresAt:      expiredExpiry,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Insert active key.
	activeExpiry := pgtype.Timestamptz{Time: now.Add(24 * time.Hour), Valid: true}
	activeKey := "active-" + uuid.New().String()
	_, err := q.IdempotencyAcquire(ctx, sqldb.IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: activeKey,
		RequestHash:    "hash-active",
		ExpiresAt:      activeExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}

	total, err := CleanupExpired(ctx, dbpool, 3)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Fatalf("expected 5 deleted, got %d", total)
	}

	// Verify active key still exists.
	row, err := q.IdempotencyAcquire(ctx, sqldb.IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: activeKey,
		RequestHash:    "hash-active",
		ExpiresAt:      activeExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.RequestHash != "hash-active" {
		t.Fatalf("expected active key to remain, got request_hash=%q", row.RequestHash)
	}
}

func TestCleanupExpired_DefaultBatchSize(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	actorID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	scope := "test-cleanup-default"
	expiredExpiry := pgtype.Timestamptz{Time: time.Now().UTC().Add(-1 * time.Hour), Valid: true}

	_, err := q.IdempotencyAcquire(ctx, sqldb.IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: "expired-default-" + uuid.New().String(),
		RequestHash:    "hash-default",
		ExpiresAt:      expiredExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}

	// batchSize=0 → defaults to 5000.
	total, err := CleanupExpired(ctx, dbpool, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total < 1 {
		t.Fatalf("expected at least 1 deleted, got %d", total)
	}
}

func TestCleanupExpired_NoExpiredRecords(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, err := CleanupExpired(ctx, dbpool, 100)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("expected 0 deleted, got %d", total)
	}
}
