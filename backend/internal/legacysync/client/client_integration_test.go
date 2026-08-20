package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClient_RejectsLoginWithoutIdentityCookie(t *testing.T) {
	var loginPosts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Account/Login":
			if r.Method == http.MethodGet {
				http.SetCookie(w, &http.Cookie{Name: "__RequestVerificationToken", Value: "cookie", Path: "/"})
				_, _ = w.Write([]byte(`<form action="/Account/Login"><input name="__RequestVerificationToken" value="token"></form>`))
				return
			}
			loginPosts.Add(1)
			_, _ = w.Write([]byte(`<a href="/Account/Logout">logout</a>`))
		case "/Admin/Courses":
			_, _ = w.Write([]byte("should not be fetched"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := New(Config{BaseURL: server.URL, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("Do error = %v, want ErrAuthentication", err)
	}
	if loginPosts.Load() != 1 {
		t.Fatalf("login POST count = %d, want 1", loginPosts.Load())
	}
}

func TestClient_RejectsLoginWithoutAntiforgeryCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Account/Login":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`<form action="/Account/Login"><input name="__RequestVerificationToken" value="token"></form>`))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "Identity", Value: "ok", Path: "/"})
			_, _ = w.Write([]byte(`<a href="/Account/Logout">logout</a>`))
		case "/Admin/Courses":
			_, _ = w.Write([]byte("should not be fetched"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := New(Config{BaseURL: server.URL, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(context.Background(), Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("Do error = %v, want ErrAuthentication", err)
	}
}
