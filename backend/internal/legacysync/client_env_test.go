package legacysync

import (
	"testing"
	"time"
)

// The throughput env knobs must default to the fast settings (16 concurrent,
// no pacing) and fall back to those defaults on junk input, so an operator
// typo can never silently slow the scraper to a crawl.
func TestMaxConcurrentFromEnv(t *testing.T) {
	t.Setenv("LEGACY_SYNC_MAX_CONCURRENT", "")
	if got := maxConcurrentFromEnv(); got != 16 {
		t.Fatalf("unset LEGACY_SYNC_MAX_CONCURRENT = %d, want default 16", got)
	}
	t.Setenv("LEGACY_SYNC_MAX_CONCURRENT", "32")
	if got := maxConcurrentFromEnv(); got != 32 {
		t.Fatalf("LEGACY_SYNC_MAX_CONCURRENT=32 parsed as %d", got)
	}
	t.Setenv("LEGACY_SYNC_MAX_CONCURRENT", "junk")
	if got := maxConcurrentFromEnv(); got != 16 {
		t.Fatalf("invalid LEGACY_SYNC_MAX_CONCURRENT = %d, want fallback 16", got)
	}
}

func TestMinRequestIntervalFromEnv(t *testing.T) {
	t.Setenv("LEGACY_SYNC_MIN_REQUEST_INTERVAL", "")
	if got := minRequestIntervalFromEnv(); got != 0 {
		t.Fatalf("unset LEGACY_SYNC_MIN_REQUEST_INTERVAL = %v, want 0 (pacing disabled)", got)
	}
	t.Setenv("LEGACY_SYNC_MIN_REQUEST_INTERVAL", "500ms")
	if got := minRequestIntervalFromEnv(); got != 500*time.Millisecond {
		t.Fatalf("LEGACY_SYNC_MIN_REQUEST_INTERVAL=500ms parsed as %v", got)
	}
	t.Setenv("LEGACY_SYNC_MIN_REQUEST_INTERVAL", "junk")
	if got := minRequestIntervalFromEnv(); got != 0 {
		t.Fatalf("invalid LEGACY_SYNC_MIN_REQUEST_INTERVAL = %v, want fallback 0", got)
	}
}
