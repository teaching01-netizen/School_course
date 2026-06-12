package queue

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

type testNonRetryableError struct {
	err error
}

func (e testNonRetryableError) Error() string {
	return e.err.Error()
}

func (e testNonRetryableError) Unwrap() error {
	return e.err
}

func (e testNonRetryableError) NonRetryable() bool {
	return true
}

func TestQueueWorkerRunJob_CompletesNonRetryableErrorsAsFailed(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryQueueStore()
	worker := NewQueueWorker(slog.Default(), store, "test-worker")
	worker.SetHeartbeatInterval(time.Hour)

	jobID, err := store.EnqueueJob(ctx, JobTypeStudentSync, map[string]string{"id": "job-1"}, "")
	if err != nil {
		t.Fatalf("EnqueueJob: %v", err)
	}

	job, ok := worker.ClaimNextJob(ctx)
	if !ok {
		t.Fatal("expected to claim queued job")
	}

	worker.RegisterHandler(JobTypeStudentSync, func(context.Context, JobRow) error {
		return testNonRetryableError{err: errors.New("deterministic conflict")}
	})
	worker.RunJob(ctx, job)

	stored := store.jobs[jobID].row
	if stored.Status != "failed" {
		t.Fatalf("expected non-retryable error to fail job immediately, got %q", stored.Status)
	}
	if stored.LastError == nil || *stored.LastError != "deterministic conflict" {
		t.Fatalf("expected last_error to capture non-retryable error, got %#v", stored.LastError)
	}
}

func TestQueueStoreCompleteJob_ClearsStaleLastErrorOnSuccess(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryQueueStore()
	jobID, err := store.EnqueueJob(ctx, JobTypeStudentSync, map[string]string{"id": "job-1"}, "")
	if err != nil {
		t.Fatalf("EnqueueJob: %v", err)
	}

	if err := store.CompleteJob(ctx, jobID, "failed", "previous failure"); err != nil {
		t.Fatalf("CompleteJob failed status: %v", err)
	}
	if err := store.CompleteJob(ctx, jobID, "succeeded", ""); err != nil {
		t.Fatalf("CompleteJob succeeded status: %v", err)
	}

	stored := store.jobs[jobID].row
	if stored.LastError != nil {
		t.Fatalf("expected successful completion to clear stale last_error, got %q", *stored.LastError)
	}
}
