package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DefaultLeaseDuration = 60 * time.Second
	HeartbeatInterval    = 10 * time.Second
	PollInterval         = 1 * time.Second
	DefaultMaxAttempts   = 3
)

// JobType categorises the kind of work a job represents.
type JobType string

const (
	JobTypeImportSnapshot       JobType = "import_snapshot"
	JobTypeStudentSync          JobType = "student_sync"
	JobTypeCourseReconcileApply JobType = "course_reconcile_apply"
	JobTypeCourseReconcileDiff  JobType = "course_reconcile_diff"
)

// JobStatus represents the lifecycle state of a queue job.
type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusRetry     JobStatus = "retry"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
)

// JobRow represents a single row from the crm_jobs table.
type JobRow struct {
	ID          uuid.UUID  `json:"id"`
	JobType     string     `json:"job_type"`
	Status      string     `json:"status"`
	Payload     []byte     `json:"payload"`
	UniqueKey   *string    `json:"unique_key,omitempty"`
	Result      []byte     `json:"result,omitempty"`
	LockedBy    *string    `json:"locked_by,omitempty"`
	LockedUntil *time.Time `json:"locked_until,omitempty"`
	HeartbeatAt *time.Time `json:"heartbeat_at,omitempty"`
	Attempt     int        `json:"attempt"`
	MaxAttempts int        `json:"max_attempts"`
	RunAfter    time.Time  `json:"run_after"`
	LastError   *string    `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// JobHandler is the function signature for processing a claimed job.
type JobHandler func(ctx context.Context, job JobRow) error

type nonRetryable interface {
	NonRetryable() bool
}

type nonRetryableError struct {
	err error
}

func (e nonRetryableError) Error() string {
	return e.err.Error()
}

func (e nonRetryableError) Unwrap() error {
	return e.err
}

func (e nonRetryableError) NonRetryable() bool {
	return true
}

// MarkNonRetryable marks deterministic business failures that should not be retried.
func MarkNonRetryable(err error) error {
	if err == nil {
		return nil
	}
	return nonRetryableError{err: err}
}

func isNonRetryable(err error) bool {
	var marker nonRetryable
	return errors.As(err, &marker) && marker.NonRetryable()
}

// QueueStore defines the storage operations the queue worker needs.
type QueueStore interface {
	ClaimJob(ctx context.Context, workerID string, leaseDur time.Duration) (JobRow, bool)
	CompleteJob(ctx context.Context, jobID uuid.UUID, status, errMsg string) error
	RetryJob(ctx context.Context, jobID uuid.UUID, errMsg string, backoff time.Duration) error
	Heartbeat(ctx context.Context, jobID uuid.UUID, leaseDur time.Duration) error
	EnqueueJob(ctx context.Context, jt JobType, payload any, uniqueKey string) (uuid.UUID, error)
}

// JobNotifier provides a channel that signals when new jobs are available.
type JobNotifier interface {
	Listen(ctx context.Context, ch chan<- struct{})
}

// PostgresQueueStore is the production implementation of QueueStore.
type PostgresQueueStore struct {
	db *pgxpool.Pool
}

// NewPostgresQueueStore creates a new PostgresQueueStore.
func NewPostgresQueueStore(db *pgxpool.Pool) *PostgresQueueStore {
	return &PostgresQueueStore{db: db}
}

// ClaimJob atomically claims one eligible job using FOR UPDATE SKIP LOCKED.
func (s *PostgresQueueStore) ClaimJob(ctx context.Context, workerID string, leaseDur time.Duration) (JobRow, bool) {
	var job JobRow
	err := s.db.QueryRow(ctx, `
		WITH eligible AS (
			SELECT id FROM crm_jobs
			WHERE (status IN ('queued', 'retry') AND run_after <= now())
			   OR (status = 'running' AND locked_until < now())
			ORDER BY created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE crm_jobs j
		SET status = 'running',
		    locked_by = $1,
		    locked_until = now() + $2::interval,
		    heartbeat_at = now(),
		    attempt = CASE WHEN j.status IN ('running', 'retry') THEN j.attempt + 1 ELSE j.attempt END,
		    updated_at = now()
		FROM eligible e
		WHERE j.id = e.id
		RETURNING j.id, j.job_type::text, j.status::text, j.payload, j.unique_key,
		          j.result, j.locked_by, j.locked_until, j.heartbeat_at,
		          j.attempt, j.max_attempts, j.run_after, j.last_error, j.created_at
	`, workerID, fmt.Sprintf("%d seconds", int(leaseDur.Seconds()))).Scan(
		&job.ID, &job.JobType, &job.Status, &job.Payload, &job.UniqueKey,
		&job.Result, &job.LockedBy, &job.LockedUntil, &job.HeartbeatAt,
		&job.Attempt, &job.MaxAttempts, &job.RunAfter, &job.LastError, &job.CreatedAt,
	)
	if err != nil {
		return JobRow{}, false
	}
	return job, true
}

// CompleteJob marks a job as succeeded or failed.
func (s *PostgresQueueStore) CompleteJob(ctx context.Context, jobID uuid.UUID, status, errMsg string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE crm_jobs
		 SET status = $1::crm_job_status,
		     locked_by = NULL,
		     locked_until = NULL,
		     heartbeat_at = NULL,
		     last_error = NULLIF($2, ''),
		     updated_at = now()
		 WHERE id = $3`,
		status, errMsg, jobID,
	)
	return err
}

// RetryJob marks a job for retry with backoff.
func (s *PostgresQueueStore) RetryJob(ctx context.Context, jobID uuid.UUID, errMsg string, backoff time.Duration) error {
	_, err := s.db.Exec(ctx,
		`UPDATE crm_jobs
		 SET status = 'retry',
		     locked_by = NULL,
		     locked_until = NULL,
		     heartbeat_at = NULL,
		     last_error = $2,
		     run_after = now() + $3::interval,
		     updated_at = now()
		 WHERE id = $1`,
		jobID, errMsg, fmt.Sprintf("%d microseconds", backoff.Microseconds()),
	)
	return err
}

// Heartbeat refreshes heartbeat_at and extends locked_until.
func (s *PostgresQueueStore) Heartbeat(ctx context.Context, jobID uuid.UUID, leaseDur time.Duration) error {
	_, err := s.db.Exec(ctx,
		`UPDATE crm_jobs
		 SET heartbeat_at = now(),
		     locked_until = now() + $1::interval,
		     updated_at = now()
		 WHERE id = $2 AND status = 'running'`,
		fmt.Sprintf("%d seconds", int(leaseDur.Seconds())),
		jobID,
	)
	return err
}

// EnqueueJob inserts a new job into the queue and notifies workers.
func (s *PostgresQueueStore) EnqueueJob(ctx context.Context, jt JobType, payload any, uniqueKey string) (uuid.UUID, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("marshal payload: %w", err)
	}

	var uniqueKeyPtr *string
	if uniqueKey != "" {
		uniqueKeyPtr = &uniqueKey
	}

	var jobID uuid.UUID
	err = s.db.QueryRow(ctx, `
		INSERT INTO crm_jobs (job_type, payload, unique_key)
		VALUES ($1::crm_job_type, $2::jsonb, $3)
		ON CONFLICT (unique_key) WHERE unique_key IS NOT NULL
		  AND status NOT IN ('succeeded', 'failed')
		DO UPDATE SET run_after = now(), updated_at = now()
		RETURNING id
	`, string(jt), string(payloadJSON), uniqueKeyPtr).Scan(&jobID)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("enqueue job: %w", err)
	}

	_, _ = s.db.Exec(ctx, "NOTIFY crm_jobs, 'new'")

	return jobID, nil
}

// Listen starts a PostgreSQL LISTEN loop that sends to ch when jobs are enqueued.
func (s *PostgresQueueStore) Listen(ctx context.Context, ch chan<- struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := s.db.Acquire(ctx)
			if err != nil {
				time.Sleep(PollInterval)
				continue
			}

			_, err = conn.Exec(ctx, "LISTEN crm_jobs")
			if err != nil {
				conn.Release()
				time.Sleep(PollInterval)
				continue
			}

			for {
				notification, err := conn.Conn().WaitForNotification(ctx)
				if err != nil {
					conn.Release()
					break
				}
				if notification != nil && notification.Channel == "crm_jobs" {
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			}
		}
	}
}

// QueueWorker manages the in-process queue loop with lease + heartbeat safety.
type QueueWorker struct {
	log      *slog.Logger
	store    QueueStore
	notifier JobNotifier
	workerID string
	handlers map[JobType]JobHandler

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	wg      sync.WaitGroup

	leaseDur   time.Duration
	hbInterval time.Duration
	pollInt    time.Duration

	ctx    context.Context
	cancel context.CancelFunc
}

// NewQueueWorker creates a new queue worker with the given store.
func NewQueueWorker(log *slog.Logger, store QueueStore, workerID string) *QueueWorker {
	w := &QueueWorker{
		log:        log,
		store:      store,
		workerID:   workerID,
		handlers:   make(map[JobType]JobHandler),
		leaseDur:   DefaultLeaseDuration,
		hbInterval: HeartbeatInterval,
		pollInt:    PollInterval,
	}
	if n, ok := store.(JobNotifier); ok {
		w.notifier = n
	}
	return w
}

// RegisterHandler registers a handler for a job type.
func (w *QueueWorker) RegisterHandler(jt JobType, handler JobHandler) {
	w.handlers[jt] = handler
}

// SetLeaseDuration overrides the default lease duration for claims.
func (w *QueueWorker) SetLeaseDuration(d time.Duration) {
	w.leaseDur = d
}

// SetHeartbeatInterval overrides the default heartbeat interval.
func (w *QueueWorker) SetHeartbeatInterval(d time.Duration) {
	w.hbInterval = d
}

// SetPollInterval overrides the default poll interval.
func (w *QueueWorker) SetPollInterval(d time.Duration) {
	w.pollInt = d
}

// Start begins the worker loop in a background goroutine.
// The provided context controls the worker's lifetime — cancelling it signals shutdown.
func (w *QueueWorker) Start(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return
	}
	w.running = true
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.stopCh = make(chan struct{})
	w.wg.Add(1)
	go w.loop()
	w.log.Info("queue worker started", "worker_id", w.workerID)
}

// Stop signals the worker to stop and waits for it to finish.
func (w *QueueWorker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.cancel()
	close(w.stopCh)
	w.mu.Unlock()
	w.wg.Wait()
	w.log.Info("queue worker stopped", "worker_id", w.workerID)
}

func (w *QueueWorker) loop() {
	defer w.wg.Done()

	notifyCh := make(chan struct{}, 1)

	if w.notifier != nil {
		go w.notifier.Listen(w.ctx, notifyCh)
	}

	for {
		select {
		case <-w.stopCh:
			return
		case <-w.ctx.Done():
			return
		case <-notifyCh:
			w.tryClaimAndRun()
		case <-time.After(w.pollInt):
			w.tryClaimAndRun()
		}
	}
}

// ClaimNextJob delegates to the store for claiming a job (used by tests and internal loop).
func (w *QueueWorker) ClaimNextJob(ctx context.Context) (JobRow, bool) {
	return w.store.ClaimJob(ctx, w.workerID, w.leaseDur)
}

// tryClaimAndRun attempts to claim a single job and execute it.
func (w *QueueWorker) tryClaimAndRun() {
	if w.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(w.ctx, w.leaseDur)
	defer cancel()

	job, ok := w.ClaimNextJob(ctx)
	if !ok {
		return
	}

	w.RunJob(ctx, job)
}

// RunJob executes the job with heartbeat maintenance.
func (w *QueueWorker) RunJob(ctx context.Context, job JobRow) {
	log := w.log.With("job_id", job.ID, "job_type", job.JobType, "attempt", job.Attempt)

	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	var hbWg sync.WaitGroup
	hbWg.Add(1)
	go w.heartbeatLoop(heartbeatCtx, &hbWg, job.ID)

	handler, ok := w.handlers[JobType(job.JobType)]
	if !ok {
		log.Error("no handler registered for job type")
		_ = w.store.CompleteJob(context.Background(), job.ID, "failed", fmt.Sprintf("no handler for %s", job.JobType))
		heartbeatCancel()
		hbWg.Wait()
		return
	}

	err := handler(ctx, job)

	heartbeatCancel()
	hbWg.Wait()

	if err != nil {
		log.Error("job failed", "error", err)
		if isNonRetryable(err) || job.Attempt >= job.MaxAttempts {
			if err := w.store.CompleteJob(context.Background(), job.ID, "failed", err.Error()); err != nil {
				log.Error("failed to complete job", "job_id", job.ID, "status", "failed", "error", err)
			}
		} else {
			backoff := time.Duration(1<<uint(job.Attempt)) * 10 * time.Second
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
			if err := w.store.RetryJob(context.Background(), job.ID, err.Error(), backoff); err != nil {
				log.Error("failed to retry job", "job_id", job.ID, "error", err)
			}
		}
		return
	}

	log.Info("job succeeded")
	if err := w.store.CompleteJob(context.Background(), job.ID, "succeeded", ""); err != nil {
		log.Error("failed to complete job", "job_id", job.ID, "status", "succeeded", "error", err)
	}
}

// heartbeatLoop refreshes heartbeat_at and extends locked_until every interval.
func (w *QueueWorker) heartbeatLoop(ctx context.Context, wg *sync.WaitGroup, jobID uuid.UUID) {
	defer wg.Done()
	ticker := time.NewTicker(w.hbInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.store.Heartbeat(ctx, jobID, w.leaseDur); err != nil {
				w.log.Error("heartbeat update failed", "job_id", jobID, "error", err)
				return
			}
		}
	}
}

// EnqueueJob inserts a new job into the queue, delegating to the underlying store.
func (w *QueueWorker) EnqueueJob(ctx context.Context, jt JobType, payload any, uniqueKey string) (uuid.UUID, error) {
	return w.store.EnqueueJob(ctx, jt, payload, uniqueKey)
}
