package db

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestIdempotencyAcquire_SameKeySamePayload_ReplaysCachedResponse(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	actorID := pgUUIDOrNew(t)
	scope := "test-scope"
	key := "test-key-replay-" + uuid.New().String()
	requestHash := "hash-same-payload"
	expiry := pgtype.Timestamptz{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true}
	respBody := `{"id": "abc-123", "status": "created"}`

	// First acquire — should be new.
	row, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    requestHash,
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !row.IsNew {
		t.Fatal("expected is_new=true on first acquire")
	}

	// Complete the record.
	if err := q.IdempotencyComplete(ctx, IdempotencyCompleteParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		StatusCode:     201,
		ResponseBody:   respBody,
		ExpiresAt:      expiry,
	}); err != nil {
		t.Fatal(err)
	}

	// Second acquire with same payload — should replay cached response.
	row2, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    requestHash,
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row2.IsNew {
		t.Fatal("expected is_new=false on replay")
	}
	if row2.StatusCode == nil || *row2.StatusCode != 201 {
		t.Fatalf("expected cached status 201, got %v", row2.StatusCode)
	}
	if len(row2.ResponseBody) == 0 {
		t.Fatal("expected non-empty cached response body")
	}
	if string(row2.ResponseBody) != respBody {
		t.Fatalf("cached body mismatch:\n  got:  %s\n  want: %s", row2.ResponseBody, respBody)
	}
	if row2.RequestHash != requestHash {
		t.Fatalf("cached request_hash mismatch: got %q, want %q", row2.RequestHash, requestHash)
	}
}

func TestIdempotencyAcquire_SameKeyDifferentPayload_ReturnsReuse(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	actorID := pgUUIDOrNew(t)
	scope := "test-scope"
	key := "test-key-reuse-" + uuid.New().String()
	hash1 := "hash-first-payload"
	hash2 := "hash-second-payload-different"
	expiry := pgtype.Timestamptz{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true}

	// First acquire with hash1.
	row1, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    hash1,
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !row1.IsNew {
		t.Fatal("expected is_new=true on first acquire")
	}

	// Second acquire with different hash2 — should detect reuse via request_hash mismatch.
	row2, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    hash2,
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The query itself does not return an error; it returns the existing row with the original hash.
	// The caller (idempotency.Acquire wrapper) detects the mismatch. We verify the raw row has
	// the original hash (not the new one), signaling reuse.
	if row2.RequestHash == hash2 {
		t.Fatalf("expected request_hash to remain %q (original), got %q (new)", hash1, row2.RequestHash)
	}
	if row2.RequestHash != hash1 {
		t.Fatalf("expected request_hash to be %q (original), got %q", hash1, row2.RequestHash)
	}
}

func TestIdempotencyAcquire_ConcurrentSameKey_OneNewInsert(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	actorID := pgUUIDOrNew(t)
	scope := "test-scope"
	key := "test-key-concurrent-" + uuid.New().String()
	requestHash := "hash-concurrent"
	expiry := pgtype.Timestamptz{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true}

	const numGoroutines = 10
	type result struct {
		isNew bool
		err   error
	}
	ch := make(chan result, numGoroutines)
	var wg sync.WaitGroup
	ready := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			row, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
				ActorUserID:    actorID,
				Scope:          scope,
				IdempotencyKey: key,
				RequestHash:    requestHash,
				ExpiresAt:      expiry,
			})
			if err != nil {
				ch <- result{err: err}
				return
			}
			ch <- result{isNew: row.IsNew}
		}()
	}
	close(ready)
	wg.Wait()
	close(ch)

	newCount := 0
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		if r.isNew {
			newCount++
		}
	}

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if newCount != 1 {
		t.Fatalf("expected exactly 1 goroutine to get is_new=true, got %d (total %d goroutines)", newCount, numGoroutines)
	}
}

func TestIdempotencyAcquire_DifferentScopes_IndependentKeys(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	actorID := pgUUIDOrNew(t)
	key := "shared-key-across-scopes"
	expiry := pgtype.Timestamptz{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true}

	// Acquire in scope "scope-a".
	rowA, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          "scope-a",
		IdempotencyKey: key,
		RequestHash:    "hash-a",
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rowA.IsNew {
		t.Fatal("expected is_new=true for scope-a")
	}

	// Acquire same key in scope "scope-b" — should be independent.
	rowB, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          "scope-b",
		IdempotencyKey: key,
		RequestHash:    "hash-b",
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rowB.IsNew {
		t.Fatal("expected is_new=true for scope-b (independent key)")
	}
}

func TestIdempotencyCleanup_RemovesExpiredRows(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	actorID := pgUUIDOrNew(t)
	scope := "test-cleanup"
	now := time.Now().UTC()

	// Insert an expired key.
	expiredKey := "test-expired-" + uuid.New().String()
	alreadyExpired := pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}
	_, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: expiredKey,
		RequestHash:    "hash-expired",
		ExpiresAt:      alreadyExpired,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Insert a non-expired key.
	activeKey := "test-active-" + uuid.New().String()
	futureExpiry := pgtype.Timestamptz{Time: now.Add(24 * time.Hour), Valid: true}
	_, err = q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: activeKey,
		RequestHash:    "hash-active",
		ExpiresAt:      futureExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cleanup expired rows.
	deleted, err := q.IdempotencyCleanup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if deleted <= 0 {
		t.Fatalf("expected at least 1 deleted row, got %d", deleted)
	}

	// Verify active key still exists.
	row, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: activeKey,
		RequestHash:    "hash-active",
		ExpiresAt:      futureExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.RequestHash != "hash-active" {
		t.Fatalf("expected active key to remain, got request_hash=%q", row.RequestHash)
	}

	// Verify expired key is gone (should insert as new).
	rowExpired, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: expiredKey,
		RequestHash:    "hash-new-after-cleanup",
		ExpiresAt:      futureExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rowExpired.IsNew {
		t.Fatal("expected expired key to be insertable as new after cleanup")
	}
}

// pgUUIDOrNew creates a random UUID wrapped in pgtype.UUID.
func pgUUIDOrNew(t *testing.T) pgtype.UUID {
	t.Helper()
	u := uuid.New()
	return pgtype.UUID{Bytes: u, Valid: true}
}

func TestIdempotencyDeleteStale_RemovesStaleRecord(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	actorID := pgUUIDOrNew(t)
	scope := "test-stale"
	key := "test-stale-key-" + uuid.New().String()
	expiry := pgtype.Timestamptz{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true}

	// Acquire a key (status_code=NULL → stale record scenario).
	_, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    "hash-stale",
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the record exists with status_code=NULL.
	row, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    "hash-stale",
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.IsNew {
		t.Fatal("expected stale record to already exist")
	}
	if row.StatusCode != nil {
		t.Fatalf("expected status_code=NULL for stale record, got %v", *row.StatusCode)
	}

	// Delete the stale record.
	deleted, err := q.IdempotencyDeleteStale(ctx, actorID, scope, key)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected stale record to be deleted")
	}

	// Verify the record is gone (next acquire should be new).
	row2, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    "hash-after-delete",
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !row2.IsNew {
		t.Fatal("expected key to be re-usable after stale delete")
	}
}

func TestIdempotencyDeleteStale_NoOpOnCompletedRecord(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	actorID := pgUUIDOrNew(t)
	scope := "test-stale-completed"
	key := "test-stale-completed-key-" + uuid.New().String()
	expiry := pgtype.Timestamptz{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true}

	// Acquire and complete (non-stale).
	_, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    "hash-completed",
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.IdempotencyComplete(ctx, IdempotencyCompleteParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		StatusCode:     200,
		ResponseBody:   `{"ok":true}`,
		ExpiresAt:      expiry,
	}); err != nil {
		t.Fatal(err)
	}

	// DeleteStale should NOT delete a completed record.
	deleted, err := q.IdempotencyDeleteStale(ctx, actorID, scope, key)
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("expected DeleteStale to no-op on completed record")
	}

	// Verify the record still replays.
	row, err := q.IdempotencyAcquire(ctx, IdempotencyAcquireParams{
		ActorUserID:    actorID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    "hash-completed",
		ExpiresAt:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.IsNew {
		t.Fatal("expected completed record to still exist")
	}
	if row.StatusCode == nil || *row.StatusCode != 200 {
		t.Fatalf("expected status 200, got %v", row.StatusCode)
	}
}
