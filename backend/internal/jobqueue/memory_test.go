package jobqueue

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryStore_DeduplicatesActiveUniqueKey(t *testing.T) {
	store := NewMemoryStore()
	request := EnqueueRequest{JobType: "legacy_refresh_course", UniqueKey: "legacy:course:7306", Priority: 1}
	first, err := store.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("deduplicated job IDs differ: %s != %s", first.ID, second.ID)
	}
}

func TestMemoryStore_ClaimsPriorityAndRecoversExpiredLease(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.August, 3, 3, 47, 0, 0, time.UTC)
	if _, err := store.Enqueue(context.Background(), EnqueueRequest{JobType: "background", UniqueKey: "background", Priority: 5, RunAfter: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue(context.Background(), EnqueueRequest{JobType: "hot", UniqueKey: "hot", Priority: 0, RunAfter: now}); err != nil {
		t.Fatal(err)
	}
	hot, err := store.Claim(context.Background(), "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if hot.JobType != "hot" {
		t.Fatalf("claimed %q, want hot", hot.JobType)
	}
	background, err := store.Claim(context.Background(), "worker-b", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if background.JobType != "background" {
		t.Fatalf("claimed %q, want background", background.JobType)
	}
	if _, err := store.Claim(context.Background(), "worker-c", now, time.Minute); !errors.Is(err, ErrNoJobs) {
		t.Fatalf("second claim error = %v, want ErrNoJobs", err)
	}
	expired := now.Add(2 * time.Minute)
	reclaimed, err := store.Claim(context.Background(), "worker-c", expired, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed.JobType != "hot" {
		t.Fatalf("reclaimed %q, want hot", reclaimed.JobType)
	}
}

func TestMemoryStore_RetryThenDeadLetters(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	job, err := store.Enqueue(context.Background(), EnqueueRequest{JobType: "legacy", UniqueKey: "legacy", MaxAttempts: 2, RunAfter: now})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(context.Background(), "worker", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(context.Background(), claimed.ID, "worker", now, errors.New("source unavailable")); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.Claim(context.Background(), "worker", now.Add(2*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(context.Background(), claimed.ID, "worker", now, errors.New("source unavailable")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), "worker", now, time.Minute); !errors.Is(err, ErrNoJobs) {
		t.Fatalf("claim after dead letter = %v, want ErrNoJobs", err)
	}
	if job.ID == "" {
		t.Fatal("enqueue returned empty job ID")
	}
}
func TestMemoryStore_ConcurrentClaimsHaveOneOwner(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.August, 4, 3, 47, 0, 0, time.UTC)
	if _, err := store.Enqueue(context.Background(), EnqueueRequest{JobType: "legacy", UniqueKey: "legacy:course:7306", RunAfter: now}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 10)
	for worker := 0; worker < 10; worker++ {
		worker := worker
		go func() {
			<-start
			_, err := store.Claim(context.Background(), "worker-"+string(rune('a'+worker)), now, time.Minute)
			results <- err
		}()
	}
	close(start)
	claimed := 0
	for range 10 {
		err := <-results
		if err == nil {
			claimed++
			continue
		}
		if !errors.Is(err, ErrNoJobs) {
			t.Fatalf("claim error = %v, want ErrNoJobs for losers", err)
		}
	}
	if claimed != 1 {
		t.Fatalf("successful claims = %d, want 1", claimed)
	}
}

func TestMemoryStore_LeaseOwnershipAndRecovery(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.August, 4, 3, 47, 0, 0, time.UTC)
	job, err := store.Enqueue(context.Background(), EnqueueRequest{JobType: "legacy", UniqueKey: "legacy", RunAfter: now})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(context.Background(), "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Heartbeat(context.Background(), claimed.ID, "worker-b", now, time.Minute); err == nil {
		t.Fatal("wrong worker heartbeat unexpectedly succeeded")
	}
	if err := store.Complete(context.Background(), claimed.ID, "worker-b"); err == nil {
		t.Fatal("wrong worker completion unexpectedly succeeded")
	}
	reclaimed, err := store.Claim(context.Background(), "worker-b", now.Add(2*time.Minute), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed.ID != job.ID || reclaimed.Attempt != 2 {
		t.Fatalf("reclaimed job = %+v, want same job at attempt 2", reclaimed)
	}
}

func TestMemoryStore_RetryRequiresOwnership(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.August, 4, 3, 47, 0, 0, time.UTC)
	if _, err := store.Enqueue(context.Background(), EnqueueRequest{JobType: "legacy", UniqueKey: "legacy:ownership", RunAfter: now}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(context.Background(), "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(context.Background(), claimed.ID, "worker-b", now, errors.New("boom")); err == nil {
		t.Fatal("wrong worker retry unexpectedly succeeded")
	}
	job := store.jobs[claimed.ID]
	if job == nil || job.Status != "running" || job.LockedBy != "worker-a" {
		t.Fatalf("job after wrong-worker retry = %+v, want running under worker-a", job)
	}
	if err := store.Retry(context.Background(), claimed.ID, "worker-a", now, errors.New("transient")); err != nil {
		t.Fatal(err)
	}
	job = store.jobs[claimed.ID]
	if job.Status != "queued" || job.LockedBy != "" {
		t.Fatalf("job after owner retry = %+v, want queued and unlocked", job)
	}
}

func TestMemoryStore_RetryRejectsStaleLeaseAfterReclaim(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.August, 4, 3, 47, 0, 0, time.UTC)
	if _, err := store.Enqueue(context.Background(), EnqueueRequest{JobType: "legacy", UniqueKey: "legacy:stale", RunAfter: now}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(context.Background(), "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// Lease expires and a second worker takes the job over.
	reclaimed, err := store.Claim(context.Background(), "worker-b", now.Add(2*time.Minute), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed.ID != claimed.ID {
		t.Fatalf("reclaimed job = %s, want %s", reclaimed.ID, claimed.ID)
	}
	// The stale owner must not be able to retry the job it no longer holds.
	if err := store.Retry(context.Background(), claimed.ID, "worker-a", now.Add(2*time.Minute), errors.New("stale")); err == nil {
		t.Fatal("stale worker retry unexpectedly succeeded after reclaim")
	}
	// The current owner can retry.
	if err := store.Retry(context.Background(), reclaimed.ID, "worker-b", now.Add(2*time.Minute), errors.New("transient")); err != nil {
		t.Fatal(err)
	}
}
