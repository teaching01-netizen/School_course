package jobqueue

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestRetryBackoffExponentialSchedule(t *testing.T) {
	cases := []struct {
		attempt int
		wantMin time.Duration
		wantMax time.Duration // exclusive upper bound
	}{
		{1, time.Second, time.Second + time.Nanosecond},  // attempt 1: exact 1s, no jitter
		{2, 2 * time.Second, 2500 * time.Millisecond},    // 2s + jitter <500ms
		{3, 4 * time.Second, 4500 * time.Millisecond},    // 4s + jitter
		{4, 8 * time.Second, 8500 * time.Millisecond},    // 8s + jitter
		{8, 64 * time.Second, 64500 * time.Millisecond},  // capped at 64s
		{20, 64 * time.Second, 64500 * time.Millisecond}, // deep attempts stay capped
	}
	for _, tc := range cases {
		delay := retryBackoff(tc.attempt, nil)
		if delay < tc.wantMin || delay >= tc.wantMax {
			t.Fatalf("retryBackoff(%d) = %v, want [%v, %v)", tc.attempt, delay, tc.wantMin, tc.wantMax)
		}
	}
}

// hintError is a stand-in for the client's typed RateLimitedError / budget
// errors: it exposes RetryAfterDuration but is not an error type jobqueue
// can import.
type hintError struct{ hint time.Duration }

func (e hintError) Error() string { return "source rate limited" }

func (e hintError) RetryAfterDuration() time.Duration { return e.hint }

func TestRetryBackoffRetryAfterHintWins(t *testing.T) {
	// A 90s Retry-After must beat the 64s base cap.
	delay := retryBackoff(20, hintError{hint: 90 * time.Second})
	if delay < 90*time.Second {
		t.Fatalf("retryBackoff with Retry-After 90s = %v, want >= 90s", delay)
	}
	// A short hint does not extend the computed backoff.
	delay = retryBackoff(1, hintError{hint: 100 * time.Millisecond})
	if delay != time.Second {
		t.Fatalf("retryBackoff(1, hint 100ms) = %v, want 1s", delay)
	}
	// The hint is honored through wrapping.
	delay = retryBackoff(3, fmt.Errorf("wrapped: %w", hintError{hint: 45 * time.Second}))
	if delay < 45*time.Second {
		t.Fatalf("retryBackoff with wrapped hint = %v, want >= 45s", delay)
	}
}

func TestRetryBackoffCircuitFloor(t *testing.T) {
	circuitCauses := []error{
		errors.New("legacy client: circuit is open"),
		errors.New("legacy client: egress budget exceeded"),
		errors.New("legacy client: authentication failed"),
		fmt.Errorf("wrapped: %w", errors.New("circuit is open")),
	}
	for _, cause := range circuitCauses {
		delay := retryBackoff(1, cause)
		if delay < 10*time.Second {
			t.Fatalf("retryBackoff(1, %q) = %v, want >= 10s floor", cause, delay)
		}
	}
	// A typed hint short-circuits the string fallback: a 429 with a small
	// Retry-After is not a circuit error, so no floor applies.
	delay := retryBackoff(1, hintError{hint: 2 * time.Second})
	if delay != 2*time.Second {
		t.Fatalf("retryBackoff with typed 2s hint = %v, want exactly 2s (no circuit floor)", delay)
	}
	// Ordinary failures keep the plain exponential schedule.
	if delay := retryBackoff(1, errors.New("boom")); delay != time.Second {
		t.Fatalf("retryBackoff(1, plain error) = %v, want 1s", delay)
	}
}

// TestMemoryStoreRetryUsesAttemptBackoff pins that the memory store's Retry
// schedules by the job's attempt count: attempt 1 reclaims at exactly +1s,
// attempt 2 somewhere in (2s, 2.5s].
func TestMemoryStoreRetryUsesAttemptBackoff(t *testing.T) {
	store := NewMemoryStore()
	// Enqueue stamps RunAfter with the real clock; anchor all retry maths to a
	// fixed point after that so claim/retry timing is deterministic.
	base := time.Now().UTC().Add(time.Minute)
	if _, err := store.Enqueue(context.Background(), EnqueueRequest{JobType: "t", EntityType: "course", ExternalID: "1", UniqueKey: "k1"}); err != nil {
		t.Fatal(err)
	}
	claimAt := func(at time.Time) (Job, bool) {
		job, err := store.Claim(context.Background(), "w", at, time.Minute)
		if errors.Is(err, ErrNoJobs) {
			return Job{}, false
		}
		if err != nil {
			t.Fatal(err)
		}
		return job, true
	}

	first, ok := claimAt(base)
	if !ok {
		t.Fatal("no job to claim at base time")
	}
	if err := store.Retry(context.Background(), first.ID, "w", base, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	// Attempt 1: exact 1s delay — not reclaimable just before +1s.
	if _, ok := claimAt(base.Add(time.Second - time.Millisecond)); ok {
		t.Fatal("attempt-1 retry was claimed before +1s")
	}
	second, ok := claimAt(base.Add(time.Second + time.Millisecond))
	if !ok {
		t.Fatal("attempt-1 retry not reclaimable at +1s")
	}
	if second.Attempt != 2 {
		t.Fatalf("claimed attempt = %d, want 2", second.Attempt)
	}
	if err := store.Retry(context.Background(), second.ID, "w", base.Add(time.Second), errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	// Attempt 2: delayed 2s+jitter, so not reclaimable before +2s.
	if _, ok := claimAt(base.Add(2*time.Second - time.Millisecond)); ok {
		t.Fatal("attempt-2 retry was claimed before +2s")
	}
}
