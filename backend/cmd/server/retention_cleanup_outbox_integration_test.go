package main

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCleanupPublishedLegacyOutboxDeletesOnlyExpiredPublishedRows(t *testing.T) {
	pool := retentionTestPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	cutoff := now.Add(-6 * time.Hour)
	suffix := uuid.New().String()
	oldPublishedKey := "retention-old-published-outbox-" + suffix
	recentPublishedKey := "retention-recent-published-outbox-" + suffix
	oldPendingKey := "retention-old-pending-outbox-" + suffix

	if _, err := tx.Exec(ctx, `
INSERT INTO legacy_sync_outbox (source_event_key, event_type, channel, entity_type, external_id, payload, status, published_at, created_at)
VALUES
($1, 'retention.test', 'realtime', 'course', $2, '{}'::jsonb, 'published', $3, $3),
($4, 'retention.test', 'realtime', 'course', $2, '{}'::jsonb, 'published', $5, $5),
($6, 'retention.test', 'realtime', 'course', $2, '{}'::jsonb, 'pending', NULL, $3)
`, oldPublishedKey, suffix, now.Add(-7*time.Hour), recentPublishedKey, now.Add(-5*time.Hour), oldPendingKey); err != nil {
		t.Fatal(err)
	}

	deleted, err := cleanupPublishedLegacyOutbox(ctx, tx, cutoff)
	if err != nil {
		t.Fatalf("cleanupPublishedLegacyOutbox: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted rows = %d, want 1", deleted)
	}

	assertExists := func(sourceEventKey string) bool {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM legacy_sync_outbox WHERE source_event_key = $1)`, sourceEventKey).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		return exists
	}
	if assertExists(oldPublishedKey) {
		t.Fatal("expired published outbox row was retained")
	}
	for _, sourceEventKey := range []string{recentPublishedKey, oldPendingKey} {
		if !assertExists(sourceEventKey) {
			t.Fatalf("live outbox row %q was deleted", sourceEventKey)
		}
	}
}
