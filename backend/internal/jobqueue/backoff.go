package jobqueue

import (
	"errors"
	"math/rand"
	"time"
)

// retryBackoff is the retry schedule for jobs that failed while running:
// attempt 1 retries after ~1s, then ~2s, ~4s ... capped at 64s, each with up
// to 500ms jitter. The source site's Retry-After hint (when present) takes
// precedence, and circuit/budget errors get a 10s floor so a broken source
// is not hammered by eager retries.
func retryBackoff(attempt int, cause error) time.Duration {
	base := time.Duration(attempt) * time.Second
	if attempt > 2 {
		base = time.Duration(1<<min(attempt-1, 6)) * time.Second
	}
	jitter := time.Duration(rand.Intn(500)) * time.Millisecond
	if attempt == 1 {
		// Keep the first retry deterministic: tests and operators expect a
		// reclaim at now+2s to be safe.
		jitter = 0
	}
	delay := base + jitter
	if d := retryAfterHint(cause); d > delay {
		delay = d
	}
	if isCircuitError(cause) && delay < 10*time.Second {
		delay = 10*time.Second + jitter
	}
	return delay
}

// retryAfterHint returns the source-provided Retry-After duration when the
// failure error carries one (client.RateLimitedError / EgressBudgetError
// implement RetryAfterDuration; jobqueue must not import the client package).
func retryAfterHint(cause error) time.Duration {
	var hint interface{ RetryAfterDuration() time.Duration }
	if errors.As(cause, &hint) {
		return hint.RetryAfterDuration()
	}
	return 0
}

// isCircuitError reports whether the failure came from an open circuit, an
// exhausted egress budget, or a blocked login. These all mean the source is
// temporarily unavailable, so eager 1-2s retries are pointless; retryBackoff
// applies a 10s floor. The typed errors from the legacy client carry a
// RetryAfterDuration hint, which wins; the string match is only a fallback
// for wrapped errors.
func isCircuitError(cause error) bool {
	if cause == nil {
		return false
	}
	if d := retryAfterHint(cause); d > 0 {
		return false
	}
	message := cause.Error()
	return containsFold(message, "circuit is open") ||
		containsFold(message, "egress budget exceeded") ||
		containsFold(message, "authentication failed")
}

func containsFold(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	ls := toLowerASCII(s)
	sl := toLowerASCII(sub)
	for i := 0; i <= len(ls)-len(sl); i++ {
		if ls[i:i+len(sl)] == sl {
			return true
		}
	}
	return false
}

func toLowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
