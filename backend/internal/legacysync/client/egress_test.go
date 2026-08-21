package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEgress_ContentLengthGateAbortsBeforeBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Account/Login" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><input name="__RequestVerificationToken" value="tok"></html>`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", "9999999")
		w.Write([]byte("x"))
	}))
	defer server.Close()

	// Budgets must be configured for the meter to engage at all.
	client, err := New(Config{BaseURL: server.URL, Username: "u", Password: "p", MaxBodyBytes: 2 << 20, MaxConcurrent: 2, MinRequestInterval: 0, MaxRequestsPerMinute: 120, MaxEgressBytesPerMinute: 50 << 20})
	if err != nil {
		t.Fatal(err)
	}
	// Prime auth to skip login path for the actual fetch
	client.authenticated = true

	_, err = client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge", err)
	}
	// The abort happens before any body byte is read, so the meter must not
	// book the announced 9999999-byte Content-Length against the budget.
	reqs, bytes, _ := client.EgressStats()
	if reqs == 0 {
		t.Fatalf("meter reqs = %d, want the aborted request counted", reqs)
	}
	if bytes != 0 {
		t.Fatalf("meter bytes = %d, want 0 (no body bytes read before abort)", bytes)
	}
}

func TestEgress_RetryAfterParsed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Account/Login" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><input name="__RequestVerificationToken" value="tok"></html>`))
			return
		}
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, Username: "u", Password: "p", MaxConcurrent: 2, MinRequestInterval: 0})
	if err != nil {
		t.Fatal(err)
	}
	client.authenticated = true

	_, err = client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if err == nil {
		t.Fatal("expected rate limited error")
	}
	rl, ok := err.(*RateLimitedError)
	if !ok {
		t.Fatalf("error type = %T, want *RateLimitedError", err)
	}
	if rl.RetryAfter < 59*time.Second || rl.RetryAfter > 61*time.Second {
		t.Fatalf("RetryAfter = %v, want ~60s", rl.RetryAfter)
	}
}

func TestEgress_CircuitState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Account/Login" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><input name="__RequestVerificationToken" value="tok"></html>`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, Username: "u", Password: "p", MaxConcurrent: 2, MinRequestInterval: 0, CircuitBreakerFailures: 2, CircuitBreakerCooldown: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client.authenticated = true
	for i := 0; i < 2; i++ {
		client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	}
	if open, _ := client.CircuitState(); !open {
		t.Fatal("circuit should be open after 2 failures")
	}
}

func TestEgress_MeterCounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><input name="__RequestVerificationToken" value="tok"></html>`))
	}))
	defer server.Close()

	client, _ := New(Config{BaseURL: server.URL, Username: "u", Password: "p", MaxConcurrent: 2, MaxRequestsPerMinute: 120, MaxEgressBytesPerMinute: 50 << 20})
	client.authenticated = true
	client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	reqs, bytes, _ := client.EgressStats()
	if reqs == 0 || bytes == 0 {
		t.Fatalf("meter reqs=%d bytes=%d want >0", reqs, bytes)
	}
}

// TestEgress_BudgetBlocksFifthConcurrentReservation pins the detailed-plan
// acceptance: with a byte budget of 4*avgBody, four concurrent admissions fit
// (4 reservations), and the fifth must be rejected before any body is read.
func TestEgress_BudgetBlocksFifthConcurrentReservation(t *testing.T) {
	const avgBody = int64(64 << 10)
	client, err := New(Config{
		BaseURL: "http://example.test", Username: "u", Password: "p",
		MaxConcurrent: 8, MinRequestInterval: 0,
		MaxRequestsPerMinute:    1000,
		MaxEgressBytesPerMinute: 4 * avgBody,
		EstimatedBodyBytes:      avgBody,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := client.reserveBudget(); err != nil {
			t.Fatalf("reservation %d: %v, want admission within 4*avgBody", i+1, err)
		}
	}
	if err := client.reserveBudget(); !errors.Is(err, ErrEgressBudgetExceeded) {
		t.Fatalf("5th reservation err = %v, want ErrEgressBudgetExceeded (budget 4*avgBody)", err)
	}
	// Once one in-flight request finishes, its estimate is released and a new
	// admission fits again (bytes so far: none recorded, 3 reserved).
	for i := 0; i < 4; i++ {
		client.releaseBudgetEstimate()
	}
	if err := client.reserveBudget(); err != nil {
		t.Fatalf("reservation after release: %v, want admission", err)
	}
}

// TestEgress_PendingEstimateReleasedAfterRequest proves a completed request
// releases its admission reservation: right after a full round trip the
// budget must admit another request whose estimate would not fit while the
// old one was still counted as pending.
func TestEgress_PendingEstimateReleasedAfterRequest(t *testing.T) {
	const avgBody = int64(64 << 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Account/Login" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><input name="__RequestVerificationToken" value="tok"></html>`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html>ok</html>`))
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL, Username: "u", Password: "p",
		MaxConcurrent: 2, MinRequestInterval: 0,
		MaxRequestsPerMinute:    1000,
		MaxEgressBytesPerMinute: 3 * avgBody,
		EstimatedBodyBytes:      avgBody,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.authenticated = true
	if _, err := client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"}); err != nil {
		t.Fatal(err)
	}
	// The request reserved ~avgBody pending plus a tiny recorded body; if the
	// reservation were not released, the next admission (1 pending + 1 new)
	// would already exceed 3*avgBody.
	for i := 0; i < 2; i++ {
		if err := client.reserveBudget(); err != nil {
			t.Fatalf("reservation %d after completed request: %v (pending estimate not released?)", i+1, err)
		}
	}
}

// TestEgress_BudgetExceededExpiresWithWindow pins that the runner's enqueue
// gate cannot stick: once the budget window has rolled, BudgetExceeded
// reports false even when no further request has reset the window (the
// failure mode where exhaustion + an empty queue permanently silenced the
// leader).
func TestEgress_BudgetExceededExpiresWithWindow(t *testing.T) {
	// Default estimate (256 KiB) x4 so the request budget exhausts first.
	const avgBody = int64(256 << 10)
	client, err := New(Config{
		BaseURL: "http://example.test", Username: "u", Password: "p",
		MaxRequestsPerMinute:    2,
		MaxEgressBytesPerMinute: 4 * avgBody,
	})
	if err != nil {
		t.Fatal(err)
	}
	fake := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return fake }
	if err := client.reserveBudget(); err != nil {
		t.Fatal(err)
	}
	if err := client.reserveBudget(); err != nil {
		t.Fatal(err)
	}
	if !client.BudgetExceeded() {
		t.Fatal("BudgetExceeded() = false, want true after request budget exhausted")
	}
	fake = fake.Add(61 * time.Second)
	if client.BudgetExceeded() {
		t.Fatal("BudgetExceeded() = true after window expiry, want false (gate must self-heal)")
	}
}
