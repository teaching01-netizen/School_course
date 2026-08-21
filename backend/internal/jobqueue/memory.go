package jobqueue

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

type MemoryStore struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{jobs: make(map[string]*Job)} }

func (s *MemoryStore) Enqueue(ctx context.Context, request EnqueueRequest) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, job := range s.jobs {
		if job.UniqueKey == request.UniqueKey && (job.Status == "queued" || job.Status == "running") {
			return copyJob(*job), nil
		}
	}
	maxAttempts := request.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	runAfter := request.RunAfter
	if runAfter.IsZero() {
		runAfter = time.Now().UTC()
	}
	job := &Job{ID: newID(), JobType: request.JobType, EntityType: request.EntityType, ExternalID: request.ExternalID, Payload: append([]byte(nil), request.Payload...), UniqueKey: request.UniqueKey, Priority: request.Priority, MaxAttempts: maxAttempts, RunAfter: runAfter, Status: "queued", CreatedAt: time.Now().UTC()}
	s.jobs[job.ID] = job
	return copyJob(*job), nil
}

func (s *MemoryStore) Claim(ctx context.Context, workerID string, now time.Time, lease time.Duration) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		if job.Status == "queued" && !job.RunAfter.After(now) || job.Status == "running" && !job.LockedUntil.After(now) {
			items = append(items, job)
		}
	}
	if len(items) == 0 {
		return Job{}, ErrNoJobs
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority < items[j].Priority
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	job := items[0]
	job.Status = "running"
	job.LockedBy = workerID
	job.LockedUntil = now.Add(lease)
	job.HeartbeatAt = now
	job.Attempt++
	return copyJob(*job), nil
}

func (s *MemoryStore) Heartbeat(ctx context.Context, id, workerID string, now time.Time, lease time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok || job.Status != "running" || job.LockedBy != workerID {
		return errors.New("job queue: lease is not owned by worker")
	}
	job.LockedUntil = now.Add(lease)
	job.HeartbeatAt = now
	return nil
}

func (s *MemoryStore) Complete(ctx context.Context, id, workerID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok || job.Status != "running" || job.LockedBy != workerID {
		return errors.New("job queue: lease is not owned by worker")
	}
	job.Status = "completed"
	job.LockedBy = ""
	return nil
}

func (s *MemoryStore) Retry(ctx context.Context, id, workerID string, now time.Time, cause error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok || job.Status != "running" || job.LockedBy != workerID {
		return errors.New("job queue: lease is not owned by worker")
	}
	job.LastError = cause.Error()
	job.LockedBy = ""
	if job.Attempt >= job.MaxAttempts {
		job.Status = "dead"
		return nil
	}
	job.Status = "queued"
	delay := retryBackoff(job.Attempt, cause)
	job.RunAfter = now.Add(delay)
	return nil
}

func copyJob(job Job) Job {
	job.Payload = append([]byte(nil), job.Payload...)
	return job
}

func newID() string {
	var value [16]byte
	if _, err := crand.Read(value[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().String()))
	}
	return hex.EncodeToString(value[:])
}
