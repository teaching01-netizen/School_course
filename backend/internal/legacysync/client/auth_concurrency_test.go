package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_ConcurrentReauthFailureIsBounded(t *testing.T) {
	var loginPosts atomic.Int32
	var courseRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Account/Login":
			if r.Method == http.MethodGet {
				http.SetCookie(w, &http.Cookie{Name: "__RequestVerificationToken", Value: "cookie", Path: "/"})
				_, _ = w.Write([]byte(`<form action="/Account/Login"><input name="__RequestVerificationToken" value="token"></form>`))
				return
			}
			if loginPosts.Add(1) == 1 {
				http.SetCookie(w, &http.Cookie{Name: "Identity", Value: "ok", Path: "/"})
				_, _ = w.Write([]byte(`<a href="/Account/Logout">logout</a>`))
				return
			}
			_, _ = w.Write([]byte(`<form action="/Account/Login">invalid login</form>`))
		case "/Admin/Courses":
			courseRequests.Add(1)
			_, _ = w.Write([]byte(`<form action="/Account/Login"></form>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := New(Config{
		BaseURL:            server.URL,
		Username:           "user",
		Password:           "pass",
		MaxConcurrent:      10,
		MinRequestInterval: time.Nanosecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	const workers = 10
	start := make(chan struct{})
	errs := make(chan error, workers)
	for range workers {
		go func() {
			<-start
			_, requestErr := client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
			errs <- requestErr
		}()
	}
	close(start)
	for range workers {
		if err := <-errs; !errors.Is(err, ErrAuthentication) {
			t.Fatalf("worker error = %v, want ErrAuthentication", err)
		}
	}
	if loginPosts.Load() != 2 {
		t.Fatalf("login POST count = %d, want initial login plus one bounded re-login", loginPosts.Load())
	}
	if courseRequests.Load() == 0 {
		t.Fatal("no course request reached the expired-session path")
	}
}
