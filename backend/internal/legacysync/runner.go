package legacysync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"warwick-institute/internal/jobqueue"
)

type RunnerConfig struct {
	Store       jobqueue.Store
	WorkerID    string
	SweepEvery  time.Duration
	Lease       time.Duration
	Concurrency int // parallel job processors draining the queue; 1 = sequential
	Logger      *slog.Logger
	ListCourses func(context.Context) ([]string, error)
	SyncCourse  func(context.Context, string) error
	Detect      func(context.Context) ([]jobqueue.EnqueueRequest, error)
	ProcessJob  func(context.Context, jobqueue.Job) error
	Leader      func(context.Context) (bool, error)
	Controls    func(context.Context) (RunnerControls, error)
}

type RunnerControls struct {
	DetectionEnabled bool
	FetchEnabled     bool
}

// Runner keeps the normalized RunnerConfig as the single bundle of its
// dependencies; nothing is cloned field-by-field.
type Runner struct {
	cfg RunnerConfig
}

func NewRunner(config RunnerConfig) (*Runner, error) {
	if config.Store == nil || config.ListCourses == nil || config.SyncCourse == nil {
		return nil, errors.New("legacy runner: store, course listing, and sync function are required")
	}
	if config.WorkerID == "" {
		return nil, errors.New("legacy runner: worker ID is required")
	}
	if config.SweepEvery <= 0 {
		config.SweepEvery = 30 * time.Second
	}
	if config.Lease <= 0 {
		config.Lease = 30 * time.Second
	}
	if config.Concurrency < 1 {
		config.Concurrency = 1
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.Controls == nil {
		config.Controls = func(context.Context) (RunnerControls, error) {
			return RunnerControls{DetectionEnabled: true, FetchEnabled: true}, nil
		}
	}
	return &Runner{cfg: config}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	if err := r.cycle(ctx); err != nil && !errors.Is(err, context.Canceled) {
		r.cfg.Logger.Error("legacy sync cycle failed", "error", err)
	}
	ticker := time.NewTicker(r.cfg.SweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.cycle(ctx); err != nil && !errors.Is(err, context.Canceled) {
				r.cfg.Logger.Error("legacy sync cycle failed", "error", err)
			}
		}
	}
}

func (r *Runner) cycle(ctx context.Context) error {
	controls, err := r.cfg.Controls(ctx)
	if err != nil {
		return fmt.Errorf("read sync controls: %w", err)
	}
	if controls.DetectionEnabled {
		if r.cfg.Leader != nil {
			leader, err := r.cfg.Leader(ctx)
			if err != nil {
				return fmt.Errorf("check legacy detector leadership: %w", err)
			}
			if !leader {
				return r.processAvailable(ctx, controls)
			}
		}
		if err := r.enqueueLinkedCourses(ctx); err != nil {
			return err
		}
		if r.cfg.Detect != nil {
			requests, err := r.cfg.Detect(ctx)
			if err != nil {
				return fmt.Errorf("detect legacy changes: %w", err)
			}
			for _, request := range requests {
				if _, err := r.cfg.Store.Enqueue(ctx, request); err != nil {
					return fmt.Errorf("enqueue detected legacy change: %w", err)
				}
			}
		}
	}
	if !controls.FetchEnabled {
		return nil
	}
	return r.processAvailable(ctx, controls)
}

func (r *Runner) processAvailable(ctx context.Context, controls RunnerControls) error {
	if !controls.FetchEnabled {
		return nil
	}
	// Drain the queue with Concurrency processors. Each processor claims its
	// own jobs (SKIP LOCKED / memory lock), so different jobs run in parallel;
	// a job failure is retried or dead-lettered by the processor that claimed
	// it, exactly like the sequential path. The first hard error cancels the
	// remaining processors (in-flight jobs finish, heartbeats stop cleanly)
	// and is returned once every processor has exited.
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wg       sync.WaitGroup
		errOnce  sync.Once
		firstErr error
	)
	for i := 0; i < r.cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.drainJobs(workCtx, controls); err != nil {
				errOnce.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}()
	}
	wg.Wait()
	return firstErr
}

// drainJobs claims and processes jobs until the queue is empty, the context
// is done, or a hard store error occurs.
func (r *Runner) drainJobs(ctx context.Context, controls RunnerControls) error {
	for {
		job, claimErr := r.cfg.Store.Claim(ctx, r.cfg.WorkerID, time.Now().UTC(), r.cfg.Lease)
		if errors.Is(claimErr, jobqueue.ErrNoJobs) {
			return nil
		}
		if claimErr != nil {
			return fmt.Errorf("claim legacy job: %w", claimErr)
		}
		if err := r.process(ctx, job); err != nil {
			r.cfg.Logger.Error("legacy job failed", "job_id", job.ID, "job_type", job.JobType, "external_id", job.ExternalID, "error", err)
			if retryErr := r.cfg.Store.Retry(ctx, job.ID, r.cfg.WorkerID, time.Now().UTC(), err); retryErr != nil {
				return fmt.Errorf("retry legacy job %s: %w", job.ID, retryErr)
			}
			continue
		}
		if err := r.cfg.Store.Complete(ctx, job.ID, r.cfg.WorkerID); err != nil {
			return fmt.Errorf("complete legacy job %s: %w", job.ID, err)
		}
	}
}

func (r *Runner) enqueueLinkedCourses(ctx context.Context) error {
	courses, err := r.cfg.ListCourses(ctx)
	if err != nil {
		return fmt.Errorf("list linked legacy courses: %w", err)
	}
	now := time.Now().UTC()
	for _, courseID := range courses {
		if courseID == "" {
			continue
		}
		if _, err := r.cfg.Store.Enqueue(ctx, jobqueue.EnqueueRequest{JobType: "legacy_refresh_course", EntityType: "course", ExternalID: courseID, UniqueKey: "legacy:course:" + courseID, Priority: 2, RunAfter: now, MaxAttempts: 5}); err != nil {
			return fmt.Errorf("enqueue legacy course %s: %w", courseID, err)
		}
	}
	return nil
}

func (r *Runner) process(ctx context.Context, job jobqueue.Job) error {
	var work func(context.Context) error
	if job.JobType == "legacy_refresh_course" && job.ExternalID != "" {
		work = func(ctx context.Context) error { return r.cfg.SyncCourse(ctx, job.ExternalID) }
	} else if r.cfg.ProcessJob != nil {
		work = func(ctx context.Context) error { return r.cfg.ProcessJob(ctx, job) }
	} else {
		return fmt.Errorf("unsupported legacy job %q", job.JobType)
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	heartbeatErrors := make(chan error, 1)
	go r.heartbeat(heartbeatCtx, job.ID, heartbeatErrors)
	err := work(ctx)
	cancel()
	if err != nil {
		return err
	}
	select {
	case heartbeatErr := <-heartbeatErrors:
		if heartbeatErr != nil {
			return heartbeatErr
		}
	default:
	}
	return nil
}

func (r *Runner) heartbeat(ctx context.Context, jobID string, errorsOut chan<- error) {
	interval := r.cfg.Lease / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.cfg.Store.Heartbeat(ctx, jobID, r.cfg.WorkerID, time.Now().UTC(), r.cfg.Lease); err != nil {
				errorsOut <- fmt.Errorf("heartbeat legacy job %s: %w", jobID, err)
				return
			}
		}
	}
}
