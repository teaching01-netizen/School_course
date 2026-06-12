package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type inMemoryJob struct {
	row JobRow
}

// InMemoryQueueStore is an in-memory QueueStore implementation for tests.
type InMemoryQueueStore struct {
	mu       sync.Mutex
	jobs     map[uuid.UUID]*inMemoryJob
	notifyCh chan struct{}
	seq      int64
}

// NewInMemoryQueueStore creates a new InMemoryQueueStore.
func NewInMemoryQueueStore() *InMemoryQueueStore {
	return &InMemoryQueueStore{
		jobs:     make(map[uuid.UUID]*inMemoryJob),
		notifyCh: make(chan struct{}, 1),
	}
}

// ClaimJob finds the first eligible (queued, retry, or expired-running) job and claims it.
func (s *InMemoryQueueStore) ClaimJob(ctx context.Context, workerID string, leaseDur time.Duration) (JobRow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var earliestID uuid.UUID
	var earliestTime time.Time

	for id, j := range s.jobs {
		switch j.row.Status {
		case "queued", "retry":
			if !j.row.RunAfter.After(now) {
				if earliestID == uuid.Nil || j.row.CreatedAt.Before(earliestTime) {
					earliestID = id
					earliestTime = j.row.CreatedAt
				}
			}
		case "running":
			if j.row.LockedUntil != nil && j.row.LockedUntil.Before(now) {
				if earliestID == uuid.Nil || j.row.CreatedAt.Before(earliestTime) {
					earliestID = id
					earliestTime = j.row.CreatedAt
				}
			}
		}
	}

	if earliestID == uuid.Nil {
		return JobRow{}, false
	}

	j := s.jobs[earliestID]
	lockedUntil := time.Now().Add(leaseDur)
	now2 := time.Now()

	attemptIncrement := 0
	if j.row.Status == "running" || j.row.Status == "retry" {
		attemptIncrement = 1
	}

	j.row.Status = "running"
	j.row.LockedBy = &workerID
	j.row.LockedUntil = &lockedUntil
	j.row.HeartbeatAt = &now2
	j.row.Attempt += attemptIncrement

	return j.row, true
}

// CompleteJob marks a job as succeeded or failed.
func (s *InMemoryQueueStore) CompleteJob(ctx context.Context, jobID uuid.UUID, status, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}

	j.row.Status = status
	j.row.LockedBy = nil
	j.row.LockedUntil = nil
	j.row.HeartbeatAt = nil
	if errMsg != "" {
		j.row.LastError = &errMsg
	} else {
		j.row.LastError = nil
	}
	return nil
}

// RetryJob marks a job for retry with backoff.
func (s *InMemoryQueueStore) RetryJob(ctx context.Context, jobID uuid.UUID, errMsg string, backoff time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}

	j.row.Status = "retry"
	j.row.LockedBy = nil
	j.row.LockedUntil = nil
	j.row.HeartbeatAt = nil
	j.row.LastError = &errMsg
	j.row.RunAfter = time.Now().Add(backoff)
	return nil
}

// Heartbeat refreshes heartbeat_at and extends locked_until.
func (s *InMemoryQueueStore) Heartbeat(ctx context.Context, jobID uuid.UUID, leaseDur time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}

	if j.row.Status != "running" {
		return nil
	}

	now := time.Now()
	lockedUntil := now.Add(leaseDur)
	j.row.HeartbeatAt = &now
	j.row.LockedUntil = &lockedUntil
	return nil
}

// EnqueueJob inserts a new job with optional deduplication by unique_key.
func (s *InMemoryQueueStore) EnqueueJob(ctx context.Context, jt JobType, payload any, uniqueKey string) (uuid.UUID, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("marshal payload: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if uniqueKey != "" {
		for _, j := range s.jobs {
			if j.row.UniqueKey != nil && *j.row.UniqueKey == uniqueKey &&
				j.row.Status != "succeeded" && j.row.Status != "failed" {
				j.row.RunAfter = time.Now()
				select {
				case s.notifyCh <- struct{}{}:
				default:
				}
				return j.row.ID, nil
			}
		}
	}

	s.seq++
	now := time.Now()
	jobID := uuid.New()
	uk := uniqueKey
	status := "queued"

	j := &inMemoryJob{
		row: JobRow{
			ID:          jobID,
			JobType:     string(jt),
			Status:      status,
			Payload:     payloadJSON,
			UniqueKey:   &uk,
			Attempt:     0,
			MaxAttempts: DefaultMaxAttempts,
			RunAfter:    now,
			CreatedAt:   now,
		},
	}
	if uniqueKey == "" {
		j.row.UniqueKey = nil
	}
	s.jobs[jobID] = j

	select {
	case s.notifyCh <- struct{}{}:
	default:
	}

	return jobID, nil
}

// Listen starts listening for job notifications and sends to ch when jobs are enqueued.
func (s *InMemoryQueueStore) Listen(ctx context.Context, ch chan<- struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.notifyCh:
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
}
