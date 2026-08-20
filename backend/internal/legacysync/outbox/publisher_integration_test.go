package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"warwick-institute/internal/realtime"
)

var (
	outboxMigrationsOnce sync.Once
	outboxMigrationsErr  error
)

func outboxTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run outbox integration tests")
	}
	outboxMigrationsOnce.Do(func() {
		migrationURL := databaseURL
		if strings.Contains(migrationURL, "?") {
			migrationURL += "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			migrationURL += "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", migrationURL)
		if err != nil {
			outboxMigrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			outboxMigrationsErr = err
			return
		}
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			outboxMigrationsErr = fmt.Errorf("locate outbox migration directory")
			return
		}
		outboxMigrationsErr = goose.UpContext(context.Background(), db, filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
	})
	if outboxMigrationsErr != nil {
		t.Fatal(outboxMigrationsErr)
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

func outboxSourceKey(t *testing.T) string {
	t.Helper()
	return "test-outbox:" + strings.ReplaceAll(t.Name(), "/", ":") + ":" + fmt.Sprint(time.Now().UnixNano())
}

func insertPendingOutbox(t *testing.T, pool *pgxpool.Pool, sourceEventKey, channel string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO legacy_sync_outbox (source_event_key, event_type, channel, entity_type, external_id, payload) VALUES ($1,'test.outbox.updated',$2,'course','7306','{}'::jsonb)`, sourceEventKey, channel); err != nil {
		t.Fatal(err)
	}
}

func cleanupOutboxTestRows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `DELETE FROM legacy_sync_outbox`); err != nil {
		t.Fatal(err)
	}
}

func newOutboxPublisher(t *testing.T, pool *pgxpool.Pool) (*Publisher, *realtime.Client, string) {
	t.Helper()
	hub := realtime.NewHub()
	client := hub.NewClient()
	channel := "course:test-outbox"
	client.Subscribe(channel)
	publisher, err := NewPublisher(pool, hub, time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hub.Close)
	return publisher, client, channel
}

func TestPublisher_DoesNotPublishBeforeClaimCommit(t *testing.T) {
	pool := outboxTestPool(t)
	cleanupOutboxTestRows(t, pool)
	publisher, client, channel := newOutboxPublisher(t, pool)
	sourceEventKey := outboxSourceKey(t)
	insertPendingOutbox(t, pool, sourceEventKey, channel)
	triggerName := "test_outbox_fail_claim"
	functionName := "test_outbox_fail_claim_fn"
	if _, err := pool.Exec(t.Context(), fmt.Sprintf(`CREATE OR REPLACE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'claim failure'; END $$; CREATE TRIGGER %s BEFORE UPDATE OF status ON legacy_sync_outbox FOR EACH ROW WHEN (NEW.status='publishing') EXECUTE FUNCTION %s()`, functionName, triggerName, functionName)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON legacy_sync_outbox; DROP FUNCTION IF EXISTS %s()`, triggerName, functionName))
	})

	if published, err := publisher.publishOne(t.Context()); err == nil || published {
		t.Fatalf("publish result = %v/%v, want claim failure without publication", published, err)
	}
	select {
	case event := <-client.Send():
		t.Fatalf("received event before claim commit: %s", event)
	default:
	}
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM legacy_sync_outbox WHERE source_event_key=$1`, sourceEventKey).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("outbox status after failed claim = %q, want pending", status)
	}
}

func TestPublisher_CommitsClaimPublishesAndMarksPublished(t *testing.T) {
	pool := outboxTestPool(t)
	cleanupOutboxTestRows(t, pool)
	publisher, client, channel := newOutboxPublisher(t, pool)
	sourceEventKey := outboxSourceKey(t)
	insertPendingOutbox(t, pool, sourceEventKey, channel)

	published, err := publisher.publishOne(t.Context())
	if err != nil || !published {
		t.Fatalf("publish result = %v/%v, want published", published, err)
	}
	select {
	case data := <-client.Send():
		var event realtime.Event
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != "test.outbox.updated" || event.ID != "7306" || event.Channel != channel {
			t.Fatalf("event = %+v, want committed outbox event", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for committed outbox event")
	}
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM legacy_sync_outbox WHERE source_event_key=$1`, sourceEventKey).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "published" {
		t.Fatalf("outbox status = %q, want published", status)
	}
}

func TestPublisher_ReclaimsExpiredPublishingEvent(t *testing.T) {
	pool := outboxTestPool(t)
	cleanupOutboxTestRows(t, pool)
	publisher, client, channel := newOutboxPublisher(t, pool)
	sourceEventKey := outboxSourceKey(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO legacy_sync_outbox (source_event_key, event_type, channel, entity_type, external_id, payload, status, claim_until) VALUES ($1,'test.outbox.updated',$2,'course','7306','{}'::jsonb,'publishing',now()-interval '1 second')`, sourceEventKey, channel); err != nil {
		t.Fatal(err)
	}
	published, err := publisher.publishOne(t.Context())
	if err != nil || !published {
		t.Fatalf("reclaim result = %v/%v, want published", published, err)
	}
	select {
	case <-client.Send():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reclaimed event")
	}
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM legacy_sync_outbox WHERE source_event_key=$1`, sourceEventKey).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "published" {
		t.Fatalf("reclaimed outbox status = %q, want published", status)
	}
}
