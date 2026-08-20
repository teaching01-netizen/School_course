package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_RejectsRedirectToUnexpectedHost(t *testing.T) {
	var redirectedRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedRequests.Add(1)
		_, _ = w.Write([]byte("unexpected host reached"))
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/Admin/Courses", http.StatusFound)
	}))
	t.Cleanup(source.Close)

	client, err := New(Config{BaseURL: source.URL, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	client.authenticated = true
	_, err = client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if !errors.Is(err, ErrUnsafeEndpoint) {
		t.Fatalf("Do error = %v, want ErrUnsafeEndpoint", err)
	}
	if redirectedRequests.Load() != 0 {
		t.Fatal("unexpected redirect host received a request")
	}
}

type failingRoundTripper struct {
	err error
}

func (t failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}

func TestClient_ClassifiesNetworkFailureAsSourceUnavailable(t *testing.T) {
	transportErr := errors.New("dial failed with credential=secret")
	client, err := New(Config{
		BaseURL:    "https://legacy.example.test",
		Username:   "user",
		Password:   "password",
		HTTPClient: &http.Client{Transport: failingRoundTripper{err: transportErr}},
	})
	if err != nil {
		t.Fatal(err)
	}
	client.authenticated = true
	_, err = client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("Do error = %v, want ErrSourceUnavailable", err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("network error incorrectly classified as context cancellation: %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("network error leaked credentials: %v", err)
	}
}

func TestClient_RejectsUnexpectedContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"courses":[]}`))
	}))
	t.Cleanup(server.Close)

	client, err := New(Config{BaseURL: server.URL, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	client.authenticated = true
	_, err = client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if !errors.Is(err, ErrUnexpectedContentType) {
		t.Fatalf("Do error = %v, want ErrUnexpectedContentType", err)
	}
}

type failingBody struct {
	read bool
}

func (b *failingBody) Read(p []byte) (int, error) {
	if b.read {
		return 0, errors.New("body read failed with secret")
	}
	b.read = true
	copy(p, "partial")
	return len("partial"), errors.New("body read failed with secret")
}

func (b *failingBody) Close() error { return nil }

type responseRoundTripper struct {
	body io.ReadCloser
}

func (t responseRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       t.body,
		Request:    request,
	}, nil
}

func TestClient_RejectsResponseBodyReadFailure(t *testing.T) {
	client, err := New(Config{
		BaseURL:    "https://legacy.example.test",
		HTTPClient: &http.Client{Transport: responseRoundTripper{body: &failingBody{}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	client.authenticated = true
	_, err = client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("Do error = %v, want ErrSourceUnavailable", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("body read error leaked response content: %v", err)
	}
}
func TestClient_SourceFailureStormRespectsConcurrencyBound(t *testing.T) {
	var requests atomic.Int32
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	ready := make(chan struct{})
	release := make(chan struct{})
	var readyOnce atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		current := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			previous := maxInFlight.Load()
			if current <= previous || maxInFlight.CompareAndSwap(previous, current) {
				break
			}
		}
		if current >= 4 && readyOnce.CompareAndSwap(false, true) {
			close(ready)
		}
		<-release
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	client, err := New(Config{
		BaseURL:                server.URL,
		Username:               "user",
		Password:               "pass",
		MaxConcurrent:          4,
		MinRequestInterval:     time.Nanosecond,
		CircuitBreakerFailures: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.authenticated = true
	const workers = 20
	errs := make(chan error, workers)
	start := make(chan struct{})
	for range workers {
		go func() {
			<-start
			_, requestErr := client.Do(t.Context(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
			errs <- requestErr
		}()
	}
	close(start)
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("source failure storm did not reach the configured concurrency")
	}
	close(release)
	for range workers {
		if err := <-errs; !errors.Is(err, ErrSourceUnavailable) {
			t.Fatalf("worker error = %v, want source unavailable", err)
		}
	}
	if got := maxInFlight.Load(); got > 4 {
		t.Fatalf("max in-flight requests = %d, want <= 4", got)
	}
	if got := requests.Load(); got != workers {
		t.Fatalf("source requests = %d, want %d bounded requests", got, workers)
	}
}
