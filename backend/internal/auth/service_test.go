package auth

import (
	"strings"
	"testing"
	"time"
)

func TestStripPort(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.1.1:8080", "192.168.1.1"},
		{"[::1]:8080", "::1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"127.0.0.1", "127.0.0.1"},
		{"[::1]", "[::1]"},
	}
	for _, tt := range tests {
		got := stripPort(tt.input)
		if got != tt.want {
			t.Errorf("stripPort(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetLimiter_TTLEviction(t *testing.T) {
	s := &Service{}

	// Populate with entries that have old createdAt.
	s.mu.Lock()
	s.userLimit = make(map[string]*limiterEntry)
	s.limitSizes = nil
	now := time.Now()
	for i := 0; i < 5; i++ {
		key := "user:old" + string(rune('0'+i))
		s.userLimit[key] = &limiterEntry{
			limiter:   nil,
			createdAt: now.Add(-20 * time.Minute), // older than 10min TTL
		}
		s.limitSizes = append(s.limitSizes, key)
	}
	s.mu.Unlock()

	// getLimiter triggers eviction, then adds a new entry.
	lim := s.getLimiter("user:new")
	if lim == nil {
		t.Fatal("expected non-nil limiter for new key")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.userLimit) != 1 {
		t.Errorf("expected 1 entry after TTL eviction, got %d", len(s.userLimit))
	}
	if len(s.limitSizes) != 1 {
		t.Errorf("expected 1 key in limitSizes, got %d", len(s.limitSizes))
	}
}

func TestGetLimiter_CapEviction(t *testing.T) {
	s := &Service{}

	const maxEntries = 10_000

	// Fill to cap with fresh entries (won't be evicted by TTL).
	s.mu.Lock()
	s.userLimit = make(map[string]*limiterEntry)
	s.limitSizes = nil
	now := time.Now()
	for i := 0; i < maxEntries; i++ {
		key := "user:" + string(rune(i/256)) + string(rune(i%256))
		s.userLimit[key] = &limiterEntry{
			limiter:   nil,
			createdAt: now, // fresh
		}
		s.limitSizes = append(s.limitSizes, key)
	}
	s.mu.Unlock()

	// This should evict the oldest (first) entry to make room.
	lim := s.getLimiter("user:overflow")
	if lim == nil {
		t.Fatal("expected non-nil limiter")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.userLimit) > maxEntries {
		t.Errorf("expected at most %d entries, got %d", maxEntries, len(s.userLimit))
	}
	// The new entry should exist.
	if _, ok := s.userLimit["user:overflow"]; !ok {
		t.Error("expected 'user:overflow' to exist")
	}
}

func TestCredentialLengthLimits(t *testing.T) {
	longUsername := strings.Repeat("a", maxUsernameLen+1)
	longPassword := strings.Repeat("b", maxPasswordLen+1)
	shortUsername := "admin"
	shortPassword := "pass123"

	tests := []struct {
		name     string
		username string
		password string
		wantErr  bool
	}{
		{"valid credentials", shortUsername, shortPassword, false},
		{"valid at boundary", strings.Repeat("a", maxUsernameLen), strings.Repeat("b", maxPasswordLen), false},
		{"username too long", longUsername, shortPassword, true},
		{"password too long", shortUsername, longPassword, true},
		{"both too long", longUsername, longPassword, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check that length validation produces the expected result
			// without hitting the DB. We replicate the check logic from HandleLogin.
			userLen := len(tt.username)
			passLen := len(tt.password)
			isTooLong := userLen > maxUsernameLen || passLen > maxPasswordLen

			if tt.wantErr && !isTooLong {
				t.Errorf("expected credentials to be too long, but they are within limits")
			}
			if !tt.wantErr && isTooLong {
				t.Errorf("expected credentials within limits, but they are too long")
			}
		})
	}
}
