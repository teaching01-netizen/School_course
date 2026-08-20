package jobqueue

import (
	"context"
	"errors"
	"time"
)

var ErrNoJobs = errors.New("job queue: no eligible jobs")

type Job struct {
	ID          string
	JobType     string
	EntityType  string
	ExternalID  string
	Payload     []byte
	UniqueKey   string
	Priority    int
	Attempt     int
	MaxAttempts int
	RunAfter    time.Time
	LockedBy    string
	LockedUntil time.Time
	HeartbeatAt time.Time
	LastError   string
	Status      string
	CreatedAt   time.Time
}

type EnqueueRequest struct {
	JobType     string
	EntityType  string
	ExternalID  string
	Payload     []byte
	UniqueKey   string
	Priority    int
	RunAfter    time.Time
	MaxAttempts int
}

type Store interface {
	Enqueue(context.Context, EnqueueRequest) (Job, error)
	Claim(context.Context, string, time.Time, time.Duration) (Job, error)
	Heartbeat(context.Context, string, string, time.Time, time.Duration) error
	Complete(context.Context, string, string) error
	Retry(context.Context, string, string, time.Time, error) error
}
