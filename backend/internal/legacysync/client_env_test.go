package legacysync

import (
	"testing"
	"time"
)

// The throughput env knobs must default to sane egress-safe settings (32 concurrent,
// no global politeness pacing, 720 req/min, 200 MiB/min) and fall back to those
// defaults on junk input, so an operator typo can never silently change the
// scraper's load profile.
func TestMaxConcurrentFromEnv(t *testing.T) {
	t.Setenv("LEGACY_SYNC_MAX_CONCURRENT", "")
	if got := maxConcurrentFromEnv(); got != 32 {
		t.Fatalf("unset LEGACY_SYNC_MAX_CONCURRENT = %d, want default 32", got)
	}
	t.Setenv("LEGACY_SYNC_MAX_CONCURRENT", "32")
	if got := maxConcurrentFromEnv(); got != 32 {
		t.Fatalf("LEGACY_SYNC_MAX_CONCURRENT=32 parsed as %d", got)
	}
	t.Setenv("LEGACY_SYNC_MAX_CONCURRENT", "junk")
	if got := maxConcurrentFromEnv(); got != 32 {
		t.Fatalf("invalid LEGACY_SYNC_MAX_CONCURRENT = %d, want fallback 32", got)
	}
}

func TestMinRequestIntervalFromEnv(t *testing.T) {
	t.Setenv("LEGACY_SYNC_MIN_REQUEST_INTERVAL", "")
	if got := minRequestIntervalFromEnv(); got != 0 {
		t.Fatalf("unset LEGACY_SYNC_MIN_REQUEST_INTERVAL = %v, want default 0 (disabled)", got)
	}
	t.Setenv("LEGACY_SYNC_MIN_REQUEST_INTERVAL", "500ms")
	if got := minRequestIntervalFromEnv(); got != 500*time.Millisecond {
		t.Fatalf("LEGACY_SYNC_MIN_REQUEST_INTERVAL=500ms parsed as %v", got)
	}
	t.Setenv("LEGACY_SYNC_MIN_REQUEST_INTERVAL", "junk")
	if got := minRequestIntervalFromEnv(); got != 0 {
		t.Fatalf("invalid LEGACY_SYNC_MIN_REQUEST_INTERVAL = %v, want fallback 0 (disabled)", got)
	}
}

func TestNewClientClampsMaxConcurrent(t *testing.T) {
	t.Setenv("LEGACY_SYNC_MAX_CONCURRENT", "1000")
	client, err := NewClient("https://example.invalid", "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	if got := client.MaxConcurrent(); got != 128 {
		t.Fatalf("MaxConcurrent = %d, want clamp 128", got)
	}
	t.Setenv("LEGACY_SYNC_MAX_CONCURRENT", "")
	client, err = NewClient("https://example.invalid", "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	if got := client.MaxConcurrent(); got != 32 {
		t.Fatalf("MaxConcurrent = %d, want default 32", got)
	}
}

func TestMaxRequestsPerMinuteFromEnv(t *testing.T) {
	t.Setenv("LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE", "")
	if got := maxRequestsPerMinuteFromEnv(); got != 720 {
		t.Fatalf("unset LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE = %d, want default 720", got)
	}
	t.Setenv("LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE", "720")
	if got := maxRequestsPerMinuteFromEnv(); got != 720 {
		t.Fatalf("LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE=720 parsed as %d", got)
	}
	t.Setenv("LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE", "junk")
	if got := maxRequestsPerMinuteFromEnv(); got != 720 {
		t.Fatalf("invalid LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE = %d, want fallback 720", got)
	}
}

func TestMaxEgressBytesPerMinuteFromEnv(t *testing.T) {
	t.Setenv("LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE", "")
	if got := maxEgressBytesPerMinuteFromEnv(); got != 200<<20 {
		t.Fatalf("unset LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE = %d, want default 200 MiB", got)
	}
	t.Setenv("LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE", "200000000")
	if got := maxEgressBytesPerMinuteFromEnv(); got != 200000000 {
		t.Fatalf("LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE=200000000 parsed as %d", got)
	}
	t.Setenv("LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE", "junk")
	if got := maxEgressBytesPerMinuteFromEnv(); got != 200<<20 {
		t.Fatalf("invalid LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE = %d, want fallback 200 MiB", got)
	}
}
