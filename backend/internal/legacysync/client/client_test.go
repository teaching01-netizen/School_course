package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_RejectsMutatingSourceRouteBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	t.Cleanup(server.Close)

	client, err := New(Config{BaseURL: server.URL, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(context.Background(), Request{Method: http.MethodPost, Path: "/Admin/Courses/CheckIn", Form: url.Values{"handler": {"checkin"}}})
	if !errors.Is(err, ErrUnsafeEndpoint) {
		t.Fatalf("Do error = %v, want ErrUnsafeEndpoint", err)
	}
	if requests.Load() != 0 {
		t.Fatal("unsafe request reached the source")
	}
}

func TestClient_ReauthenticatesOnceAfterSessionExpiry(t *testing.T) {
	var loginCount atomic.Int32
	var pageCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Account/Login":
			if r.Method == http.MethodGet {
				http.SetCookie(w, &http.Cookie{Name: "__RequestVerificationToken", Value: "cookie", Path: "/"})
				_, _ = w.Write([]byte(`<form action="/Account/Login"><input name="__RequestVerificationToken" value="token"></form>`))
				return
			}
			loginCount.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "Identity", Value: "ok", Path: "/"})
			_, _ = w.Write([]byte(`<html>authenticated</html>`))
		case "/Admin/Courses":
			if r.Method != http.MethodGet {
				t.Fatal("expected GET course list")
			}
			if pageCount.Add(1) == 1 {
				_, _ = w.Write([]byte(`<html><form action="/Account/Login"></form></html>`))
				return
			}
			_, _ = w.Write([]byte(`<html>course list</html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := New(Config{BaseURL: server.URL, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if err != nil {
		t.Fatal(err)
	}
	if string(response.Body) != `<html>course list</html>` {
		t.Fatalf("response body = %q", response.Body)
	}
	if loginCount.Load() != 2 {
		t.Fatalf("login count = %d, want one initial login and one re-login", loginCount.Load())
	}
}

func TestClient_ConcurrentExpiryTriggersSingleRelogin(t *testing.T) {
	var loginCount atomic.Int32
	var pageCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Account/Login":
			if r.Method == http.MethodGet {
				http.SetCookie(w, &http.Cookie{Name: "__RequestVerificationToken", Value: "cookie", Path: "/"})
				_, _ = w.Write([]byte(`<form action="/Account/Login"><input name="__RequestVerificationToken" value="token"></form>`))
				return
			}
			loginCount.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "Identity", Value: "ok", Path: "/"})
			_, _ = w.Write([]byte("ok"))
		case "/Admin/Courses":
			if pageCount.Add(1) <= 2 {
				_, _ = w.Write([]byte(`<form action="/Account/Login"></form>`))
				return
			}
			_, _ = w.Write([]byte("course list"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := New(Config{BaseURL: server.URL, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errCh := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, requestErr := client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
			errCh <- requestErr
		}()
	}
	close(start)
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
	if loginCount.Load() != 2 {
		t.Fatalf("login count = %d, want one initial login and one synchronized re-login", loginCount.Load())
	}
}

func TestClient_ClassifiesSourceFailuresAndLimitsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("case") {
		case "rate":
			w.WriteHeader(http.StatusTooManyRequests)
		case "unavailable":
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = w.Write([]byte(strings.Repeat("x", 32)))
		}
	}))
	t.Cleanup(server.Close)

	client, err := New(Config{BaseURL: server.URL, Username: "user", Password: "pass", MaxBodyBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	client.authenticated = true
	for _, test := range []struct {
		name  string
		query string
		want  error
	}{
		{name: "rate limited", query: "rate", want: ErrRateLimited},
		{name: "source unavailable", query: "unavailable", want: ErrSourceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses", Query: url.Values{"case": {test.query}}})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
	_, err = client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("large response error = %v, want ErrResponseTooLarge", err)
	}
	if strings.Contains(err.Error(), "xxxxxxxx") {
		t.Fatal("response body leaked in error")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	_, err = client.Do(ctx, Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if err == nil {
		t.Fatal("expected canceled request")
	}
}

func TestClient_CircuitBreakerStopsRepeatedSourceFailures(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	client, err := New(Config{BaseURL: server.URL, Username: "user", Password: "pass", CircuitBreakerFailures: 2, CircuitBreakerCooldown: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	client.authenticated = true
	for range 2 {
		_, err = client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
		if !errors.Is(err, ErrSourceUnavailable) {
			t.Fatalf("error = %v, want ErrSourceUnavailable", err)
		}
	}
	_, err = client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("error = %v, want ErrCircuitOpen", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("source requests = %d, want 2", requests.Load())
	}
}
