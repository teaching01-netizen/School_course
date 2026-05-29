package idempotency

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRequireKey_Valid_ReturnsKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"uuid", "550e8400-e29b-41d4-a716-446655440000"},
		{"alphanumeric", "a1b2c3d4e5f6g7h8i9j0"},
		{"with_colons_and_dots", "key:1.0.0-test-long"},
		{"min_length", strings.Repeat("a", 16)},
		{"max_length", strings.Repeat("b", 128)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptestNewRequest("POST", "/", nil)
			req.Header.Set("Idempotency-Key", tt.key)
			got, err := RequireKey(req)
			if err != nil {
				t.Fatalf("RequireKey() err = %v, want nil", err)
			}
			if got != tt.key {
				t.Fatalf("RequireKey() = %q, want %q", got, tt.key)
			}
		})
	}
}

func TestRequireKey_Invalid_ReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr string
	}{
		{"missing", "", "missing required"},
		{"too_short", "short", "too short"},
		{"too_long", strings.Repeat("c", 129), "too long"},
		{"invalid_chars_space", "key with spaces in it 123456", "invalid character"},
		{"invalid_chars_special", "key!@#$-1234567890000000", "invalid character"},
		{"invalid_chars_unicode", "këy-12345678900000000", "invalid character"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptestNewRequest("POST", "/", nil)
			if tt.key != "" {
				req.Header.Set("Idempotency-Key", tt.key)
			}
			_, err := RequireKey(req)
			if err == nil {
				t.Fatal("RequireKey() err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("RequireKey() err = %q, want substr %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNewRequestFingerprint_Deterministic(t *testing.T) {
	u, _ := url.Parse("/api/v1/courses")
	body := []byte(`{"code":"MATH101"}`)

	hash1 := NewRequestFingerprint("POST", u, body)
	hash2 := NewRequestFingerprint("POST", u, body)

	if hash1 != hash2 {
		t.Fatalf("expected deterministic hashes, got %q vs %q", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Fatalf("expected 64-char SHA256 hex, got %d chars", len(hash1))
	}
}

func TestNewRequestFingerprint_ChangesOnMethod(t *testing.T) {
	u, _ := url.Parse("/api/v1/courses")
	body := []byte(`{}`)

	postHash := NewRequestFingerprint("POST", u, body)
	putHash := NewRequestFingerprint("PUT", u, body)

	if postHash == putHash {
		t.Fatal("expected different hashes for different methods")
	}
}

func TestNewRequestFingerprint_ChangesOnQueryString(t *testing.T) {
	u1, _ := url.Parse("/api/v1/sessions?start=2026-01-01&end=2026-12-31")
	u2, _ := url.Parse("/api/v1/sessions?start=2026-06-01&end=2026-12-31")
	body := []byte{}

	h1 := NewRequestFingerprint("GET", u1, body)
	h2 := NewRequestFingerprint("GET", u2, body)

	if h1 == h2 {
		t.Fatal("expected different hashes for different query strings")
	}
}

func TestNewRequestFingerprint_ChangesOnBody(t *testing.T) {
	u, _ := url.Parse("/api/v1/courses")

	h1 := NewRequestFingerprint("POST", u, []byte(`{"code":"A"}`))
	h2 := NewRequestFingerprint("POST", u, []byte(`{"code":"B"}`))

	if h1 == h2 {
		t.Fatal("expected different hashes for different bodies")
	}
}

func TestNewRequestFingerprint_EmptyBody(t *testing.T) {
	u, _ := url.Parse("/api/v1/sessions/123/delete")
	body := []byte{}

	hash := NewRequestFingerprint("DELETE", u, body)
	if len(hash) != 64 {
		t.Fatalf("expected 64-char hash for empty body, got %d chars", len(hash))
	}
}

// httptestNewRequest is a lightweight http.NewRequest replacement for tests.
func httptestNewRequest(method, target string, body []byte) *http.Request {
	var r io.Reader
	if len(body) > 0 {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, target, r)
	if err != nil {
		panic(err)
	}
	return req
}
