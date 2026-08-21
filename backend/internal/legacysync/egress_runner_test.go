package legacysync

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"warwick-institute/internal/jobqueue"
)

var jitterSeq int

func TestEgress_RunnerJitterSpreadsRunAfter(t *testing.T) {
	store := jobqueue.NewMemoryStore()
	fixed := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	const sweep = time.Minute
	runner, err := NewRunner(RunnerConfig{
		Store:       store,
		WorkerID:    "test",
		SweepEvery:  sweep,
		Lease:       time.Minute,
		Logger:      slog.New(slog.NewTextHandler(testWriter{t}, nil)),
		ListCourses: func(context.Context) ([]string, error) { return []string{"a", "b", "c", "d"}, nil },
		SyncCourse:  func(context.Context, string) error { return nil },
		Now:         func() time.Time { return fixed },
		Rand: func(n int) int {
			// Deterministic spread across the full 0..SweepEvery window.
			vi := jitterSeq
			jitterSeq++
			return []int{0, 30000, 59999, 15000}[vi%4] % n
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.enqueueLinkedCourses(context.Background()); err != nil {
		t.Fatal(err)
	}
	// All four jobs must exist with distinct (jittered) RunAfter values spread
	// over the sweep window, per the detailed plan's acceptance criterion.
	seen := make(map[time.Time]bool)
	var minRun, maxRun time.Time
	for {
		job, err := store.Claim(context.Background(), "w", fixed.Add(sweep), time.Minute)
		if err != nil {
			if errors.Is(err, jobqueue.ErrNoJobs) {
				break
			}
			t.Fatal(err)
		}
		if seen[job.RunAfter] {
			t.Fatalf("duplicate RunAfter %v (jitter not applied)", job.RunAfter)
		}
		seen[job.RunAfter] = true
		if minRun.IsZero() || job.RunAfter.Before(minRun) {
			minRun = job.RunAfter
		}
		if job.RunAfter.After(maxRun) {
			maxRun = job.RunAfter
		}
	}
	if len(seen) != 4 {
		t.Fatalf("claimed %d jobs, want 4", len(seen))
	}
	if spread := maxRun.Sub(minRun); spread < sweep*2/5 {
		t.Fatalf("RunAfter spread = %v, want >= %v (SweepEvery*0.4)", spread, sweep*2/5)
	}
}

func TestEgress_RunnerCircuitOpenSkipsEnqueue(t *testing.T) {
	store := jobqueue.NewMemoryStore()
	runner, err := NewRunner(RunnerConfig{
		Store:       store,
		WorkerID:    "test",
		SweepEvery:  time.Minute,
		Lease:       time.Minute,
		Logger:      slog.New(slog.NewTextHandler(testWriter{t}, nil)),
		ListCourses: func(context.Context) ([]string, error) { return []string{"a"}, nil },
		SyncCourse:  func(context.Context, string) error { return nil },
		Circuit:     func() (bool, time.Time) { return true, time.Now().Add(time.Minute) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	// No job may exist: the circuit-open gate must have skipped enqueue.
	if _, err := store.Claim(context.Background(), "w", time.Now().Add(time.Minute), time.Minute); !errors.Is(err, jobqueue.ErrNoJobs) {
		t.Fatalf("claim error = %v, want ErrNoJobs (nothing enqueued while circuit open)", err)
	}
}

func TestEgress_BudgetExceededGatesEnqueue(t *testing.T) {
	store := jobqueue.NewMemoryStore()
	runner, err := NewRunner(RunnerConfig{
		Store:          store,
		WorkerID:       "test",
		SweepEvery:     time.Minute,
		Lease:          time.Minute,
		Logger:         slog.New(slog.NewTextHandler(testWriter{t}, nil)),
		ListCourses:    func(context.Context) ([]string, error) { return []string{"a"}, nil },
		SyncCourse:     func(context.Context, string) error { return nil },
		BudgetExceeded: func() bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), "w", time.Now().Add(time.Minute), time.Minute); !errors.Is(err, jobqueue.ErrNoJobs) {
		t.Fatalf("claim error = %v, want ErrNoJobs (nothing enqueued while budget exceeded)", err)
	}
}

// TestEgress_RunnerPacingSecondSweepEnqueuesZero pins the R-001 real
// verification: a second sweep over the same due-filtered course list (empty,
// because every course is within its refresh cooldown) must add no new jobs.
// The two first-sweep admissions stay queued untouched — no re-enqueue
// duplicates — because enqueue jitter spreads their RunAfter into the future.
func TestEgress_RunnerPacingSecondSweepEnqueuesZero(t *testing.T) {
	store := jobqueue.NewMemoryStore()
	calls := 0
	runner, err := NewRunner(RunnerConfig{
		Store:      store,
		WorkerID:   "test",
		SweepEvery: time.Minute,
		Lease:      time.Minute,
		Logger:     slog.New(slog.NewTextHandler(testWriter{t}, nil)),
		ListCourses: func(context.Context) ([]string, error) {
			calls++
			if calls == 1 {
				return []string{"a", "b"}, nil
			}
			// Due filter: nothing is due on the second sweep.
			return nil, nil
		},
		SyncCourse: func(context.Context, string) error {
			t.Fatal("no job may be due before the sweep window elapses")
			return nil
		},
		Rand: func(int) int { return 30000 }, // spread into the future
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runner.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("list course called %d times, want 2 sweeps", calls)
	}
	// Exactly the two first-sweep jobs exist, each once — the second sweep
	// enqueued zero.
	seen := make(map[string]bool)
	for {
		job, err := store.Claim(context.Background(), "w", time.Now().Add(2*time.Minute), time.Minute)
		if err != nil {
			if errors.Is(err, jobqueue.ErrNoJobs) {
				break
			}
			t.Fatal(err)
		}
		if seen[job.UniqueKey] {
			t.Fatalf("duplicate enqueue of %q on the second sweep", job.UniqueKey)
		}
		seen[job.UniqueKey] = true
	}
	if len(seen) != 2 {
		t.Fatalf("queued %d unique jobs after two sweeps, want 2", len(seen))
	}
}
