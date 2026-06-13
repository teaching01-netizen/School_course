package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestInMemoryLoginRateLimiter_TTLEviction(t *testing.T) {
	l := NewInMemoryLoginRateLimiter()

	l.mu.Lock()
	l.userLimit = make(map[string]*memLimiterEntry)
	l.limitSizes = nil
	now := time.Now()
	for i := 0; i < 5; i++ {
		key := "user:old" + string(rune('0'+i))
		l.userLimit[key] = &memLimiterEntry{
			limiter:   nil,
			createdAt: now.Add(-20 * time.Minute),
		}
		l.limitSizes = append(l.limitSizes, key)
	}
	l.mu.Unlock()

	result, err := l.Allow(nil, "newuser", "127.0.0.1")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.userLimit) != 2 {
		t.Errorf("expected 2 entries after TTL eviction (1 ip + 1 user), got %d", len(l.userLimit))
	}
}

func TestInMemoryLoginRateLimiter_CapEviction(t *testing.T) {
	l := NewInMemoryLoginRateLimiter()

	const maxEntries = 10_000

	l.mu.Lock()
	l.userLimit = make(map[string]*memLimiterEntry)
	l.limitSizes = nil
	now := time.Now()
	for i := 0; i < maxEntries; i++ {
		key := "user:" + string(rune(i/256)) + string(rune(i%256))
		l.userLimit[key] = &memLimiterEntry{
			limiter:   nil,
			createdAt: now,
		}
		l.limitSizes = append(l.limitSizes, key)
	}
	l.mu.Unlock()

	result, err := l.Allow(nil, "overflowuser", "10.0.0.1")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.userLimit) > maxEntries+2 {
		t.Errorf("expected at most %d entries, got %d", maxEntries+2, len(l.userLimit))
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

func TestLoginRejectsOversizedCredentialsBeforeRateLimit(t *testing.T) {
	limiter := &recordingLoginLimiter{}
	svc := NewService(
		fakePasswordHasher{},
		fakeSessionStore{},
		limiter,
		fakeUserLookup{},
		nil,
	)

	_, _, err := svc.Login(context.Background(), strings.Repeat("a", maxUsernameLen+1), "password", "127.0.0.1")
	if !errors.Is(err, ErrCredentialsTooLong) {
		t.Fatalf("Login error = %v, want %v", err, ErrCredentialsTooLong)
	}
	if limiter.calls != 0 {
		t.Fatalf("limiter called %d times, want 0", limiter.calls)
	}
}

func TestDBLoginRateLimiterChecksIPBeforeUsername(t *testing.T) {
	store := &recordingRateLimitStore{
		results: map[string]RateLimitResult{
			"auth:ip:127.0.0.1": {Allowed: false},
		},
	}
	limiter := NewDBLoginRateLimiter(store)

	result, err := limiter.Allow(context.Background(), "admin", "127.0.0.1")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if result.Allowed {
		t.Fatal("expected request to be rate limited")
	}

	want := []string{"auth:ip:127.0.0.1"}
	if strings.Join(store.keys, ",") != strings.Join(want, ",") {
		t.Fatalf("rate limit keys = %v, want %v", store.keys, want)
	}
}

func TestHandleLoginSetsCookieToIdleTimeout(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	userID := uuid.New()
	sessionID := uuid.New()
	sessions := fakeSessionStore{
		createSession: Session{
			ID:              sessionID,
			UserID:          userID,
			CreatedAt:       now,
			LastSeenAt:      now,
			ExpiresAt:       now.Add(sessionAbsoluteTimeout),
			PasswordVersion: 1,
		},
	}
	svc := NewService(
		fakePasswordHasher{verifyOK: true},
		sessions,
		&recordingLoginLimiter{allowed: true},
		fakeUserLookup{byUsername: User{
			ID:              userID,
			Username:        "admin",
			Role:            "Admin",
			PasswordHash:    "hash",
			PasswordVersion: 1,
		}},
		nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	if err := svc.HandleLogin(w, req); err != nil {
		t.Fatalf("HandleLogin: %v", err)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	wantExpires := now.Add(sessionIdleTimeout)
	if !cookies[0].Expires.Equal(wantExpires) {
		t.Fatalf("cookie expiry = %s, want idle expiry %s", cookies[0].Expires, wantExpires)
	}
	if cookies[0].Expires.Equal(sessions.createSession.ExpiresAt) {
		t.Fatalf("cookie expiry unexpectedly matched absolute session expiry %s", sessions.createSession.ExpiresAt)
	}
}

type recordingLoginLimiter struct {
	calls   int
	allowed bool
}

func (l *recordingLoginLimiter) Allow(_ context.Context, _, _ string) (RateLimitResult, error) {
	l.calls++
	if !l.allowed {
		return RateLimitResult{Allowed: true}, nil
	}
	return RateLimitResult{Allowed: true}, nil
}

type recordingRateLimitStore struct {
	keys    []string
	results map[string]RateLimitResult
}

func (s *recordingRateLimitStore) Allow(_ context.Context, key string, _ int, _ time.Duration) (RateLimitResult, error) {
	s.keys = append(s.keys, key)
	if result, ok := s.results[key]; ok {
		return result, nil
	}
	return RateLimitResult{Allowed: true}, nil
}

type fakePasswordHasher struct {
	verifyOK bool
}

func (h fakePasswordHasher) HashPassword(password string) (string, error) {
	return "hash:" + password, nil
}

func (h fakePasswordHasher) VerifyPassword(_, _ string) (bool, error) {
	return h.verifyOK, nil
}

type fakeSessionStore struct {
	createSession Session
}

func (s fakeSessionStore) Create(_ context.Context, userID uuid.UUID, passwordVersion int32, _, _ time.Duration) (Session, error) {
	if s.createSession.ID != uuid.Nil {
		return s.createSession, nil
	}
	now := time.Now().UTC()
	return Session{
		ID:              uuid.New(),
		UserID:          userID,
		CreatedAt:       now,
		LastSeenAt:      now,
		ExpiresAt:       now.Add(sessionAbsoluteTimeout),
		PasswordVersion: passwordVersion,
	}, nil
}

func (fakeSessionStore) ByID(_ context.Context, _ uuid.UUID) (Session, error) {
	return Session{}, errors.New("not implemented")
}

func (fakeSessionStore) Revoke(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (fakeSessionStore) RevokeAllForUser(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}

func (fakeSessionStore) ListForUser(_ context.Context, _ uuid.UUID) ([]Session, error) {
	return nil, nil
}

func (fakeSessionStore) DeleteExpired(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (fakeSessionStore) TouchLastSeen(_ context.Context, _ uuid.UUID) {}

type fakeUserLookup struct {
	byUsername User
}

func (l fakeUserLookup) ByUsername(_ context.Context, _ string) (User, error) {
	if l.byUsername.ID == uuid.Nil {
		return User{}, ErrUserNotFound
	}
	return l.byUsername, nil
}

func (l fakeUserLookup) ByID(_ context.Context, userID uuid.UUID) (User, error) {
	if l.byUsername.ID == userID {
		return l.byUsername, nil
	}
	return User{}, ErrUserNotFound
}
