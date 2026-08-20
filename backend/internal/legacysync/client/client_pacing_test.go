package client

import (
	"context"
	"testing"
	"time"
)

func TestMinRequestIntervalZeroDisablesPacing(t *testing.T) {
	c, err := New(Config{BaseURL: "https://example.com", Username: "user", Password: "pass", MinRequestInterval: 0})
	if err != nil {
		t.Fatal(err)
	}
	if c.minRequestInterval != 0 {
		t.Fatalf("minRequestInterval = %v, want 0 (pacing disabled)", c.minRequestInterval)
	}
	// Two consecutive slot requests must return immediately: with the old
	// default, the second one would have blocked for ~500ms.
	start := time.Now()
	for i := 0; i < 2; i++ {
		if err := c.waitForRequestSlot(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("two paced slot requests took %v, want immediate with interval 0", elapsed)
	}
}

func TestMinRequestIntervalNegativeSelectsDefault(t *testing.T) {
	c, err := New(Config{BaseURL: "https://example.com", Username: "user", Password: "pass", MinRequestInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	if c.minRequestInterval != defaultMinRequestInterval {
		t.Fatalf("minRequestInterval = %v, want the politeness default %v", c.minRequestInterval, defaultMinRequestInterval)
	}
}

func TestMaxConcurrentGetter(t *testing.T) {
	c, err := New(Config{BaseURL: "https://example.com", Username: "user", Password: "pass", MaxConcurrent: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.MaxConcurrent(); got != 5 {
		t.Fatalf("MaxConcurrent() = %d, want 5", got)
	}
}
