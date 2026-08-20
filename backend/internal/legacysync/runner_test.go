package legacysync

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"warwick-institute/internal/jobqueue"
)

func TestRunnerEnqueuesAndProcessesLinkedCourses(t *testing.T) {
	store := jobqueue.NewMemoryStore()
	var mu sync.Mutex
	var synced []string
	runner, err := NewRunner(RunnerConfig{
		Store:      store,
		WorkerID:   "test-worker",
		SweepEvery: time.Hour,
		Lease:      time.Minute,
		Logger:     slog.New(slog.NewTextHandler(testWriter{t}, nil)),
		ListCourses: func(context.Context) ([]string, error) {
			return []string{"7306", "7307"}, nil
		},
		SyncCourse: func(_ context.Context, id string) error {
			mu.Lock()
			synced = append(synced, id)
			mu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(synced) != 2 || synced[0] != "7306" || synced[1] != "7307" {
		t.Fatalf("synced courses = %v", synced)
	}
}

func TestRunnerDoesNotFetchWhenPaused(t *testing.T) {
	store := jobqueue.NewMemoryStore()
	called := false
	runner, err := NewRunner(RunnerConfig{
		Store:       store,
		WorkerID:    "test-worker",
		ListCourses: func(context.Context) ([]string, error) { t.Fatal("list should not run while paused"); return nil, nil },
		SyncCourse:  func(context.Context, string) error { called = true; return nil },
		Controls: func(context.Context) (RunnerControls, error) {
			return RunnerControls{DetectionEnabled: false, FetchEnabled: false}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("sync function called while paused")
	}
}

func TestRunnerRetriesFailedJobs(t *testing.T) {
	store := jobqueue.NewMemoryStore()
	attempts := 0
	runner, err := NewRunner(RunnerConfig{
		Store:       store,
		WorkerID:    "test-worker",
		ListCourses: func(context.Context) ([]string, error) { return []string{"7306"}, nil },
		SyncCourse: func(_ context.Context, _ string) error {
			attempts++
			return errors.New("source unavailable")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want one claimed attempt", attempts)
	}
}

func TestRunnerProcessesDetectedJobsWithCustomHandler(t *testing.T) {
	store := jobqueue.NewMemoryStore()
	processed := false
	runner, err := NewRunner(RunnerConfig{
		Store:       store,
		WorkerID:    "test-worker",
		ListCourses: func(context.Context) ([]string, error) { return nil, nil },
		SyncCourse:  func(context.Context, string) error { return nil },
		Detect: func(context.Context) ([]jobqueue.EnqueueRequest, error) {
			return []jobqueue.EnqueueRequest{{JobType: "legacy_refresh_teacher", EntityType: "teacher", ExternalID: "78", UniqueKey: "legacy:teacher:78"}}, nil
		},
		ProcessJob: func(_ context.Context, job jobqueue.Job) error {
			processed = job.JobType == "legacy_refresh_teacher" && job.ExternalID == "78"
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !processed {
		t.Fatal("custom detected job was not processed")
	}
}

func TestRunnerNonLeaderStillProcessesQueuedJobs(t *testing.T) {
	store := jobqueue.NewMemoryStore()
	if _, err := store.Enqueue(context.Background(), jobqueue.EnqueueRequest{JobType: "legacy_refresh_course", ExternalID: "7306", UniqueKey: "legacy:course:7306"}); err != nil {
		t.Fatal(err)
	}
	processed := false
	runner, err := NewRunner(RunnerConfig{
		Store:       store,
		WorkerID:    "test-worker",
		ListCourses: func(context.Context) ([]string, error) { t.Fatal("non-leader should not detect"); return nil, nil },
		SyncCourse:  func(_ context.Context, id string) error { processed = id == "7306"; return nil },
		Leader:      func(context.Context) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !processed {
		t.Fatal("non-leader did not process queued job")
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(value []byte) (int, error) {
	w.t.Helper()
	return len(value), nil
}

// TestRunnerProcessesJobsConcurrently pins that Concurrency lets multiple
// processors drain the queue at once. Three queued jobs with three
// processors means every processor holds exactly one job, so all three must
// reach the sync barrier simultaneously (which only happens if the drain is
// genuinely parallel), and the queue must be fully drained afterward.
func TestRunnerProcessesJobsConcurrently(t *testing.T) {
	store := jobqueue.NewMemoryStore()
	for i := 0; i < 3; i++ {
		if _, err := store.Enqueue(context.Background(), jobqueue.EnqueueRequest{JobType: "legacy_refresh_course", EntityType: "course", ExternalID: strconv.Itoa(i), UniqueKey: "legacy:course:" + strconv.Itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	var synced atomic.Int32
	runner, err := NewRunner(RunnerConfig{
		Store:       store,
		WorkerID:    "test-worker",
		Concurrency: 3,
		Lease:       time.Minute,
		Logger:      slog.New(slog.NewTextHandler(testWriter{t}, nil)),
		ListCourses: func(context.Context) ([]string, error) { return nil, nil },
		SyncCourse: func(_ context.Context, _ string) error {
			synced.Add(1)
			entered <- struct{}{}
			select {
			case <-release:
				return nil
			case <-time.After(10 * time.Second):
				return errors.New("worker stuck before the release barrier")
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- runner.cycle(context.Background()) }()
	deadline := time.After(10 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case <-entered:
		case <-deadline:
			close(release)
			t.Fatalf("only %d of 3 workers reached the sync barrier concurrently", i)
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if synced.Load() != 3 {
		t.Fatalf("synced = %d, want 3", synced.Load())
	}
	// Nothing may remain queued or running: every job must have completed.
	if _, err := store.Claim(context.Background(), "verify", time.Now().Add(10*time.Second), time.Minute); !errors.Is(err, jobqueue.ErrNoJobs) {
		t.Fatalf("queue was not drained after parallel processing, claim returned %v", err)
	}
}
