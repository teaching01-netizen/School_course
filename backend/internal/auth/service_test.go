package auth

import (
	"bytes"
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
	svc := NewService(ServiceOptions{
		Hasher:       fakePasswordHasher{},
		Sessions:     fakeSessionStore{},
		Limiter:      limiter,
		Users:        fakeUserLookup{},
		Log:          nil,
		CookieSecure: true,
	})

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

func TestHandleLoginSetsCookieToAbsoluteSessionExpiry(t *testing.T) {
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
	svc := NewService(ServiceOptions{
		Hasher:   fakePasswordHasher{verifyOK: true},
		Sessions: sessions,
		Limiter:  &recordingLoginLimiter{allowed: true},
		Users: fakeUserLookup{byUsername: User{
			ID:              userID,
			Username:        "admin",
			Role:            "Admin",
			PasswordHash:    "hash",
			PasswordVersion: 1,
		}},
		Log:          nil,
		CookieSecure: true,
	})

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
	// The cookie must live as long as the session's absolute cap; idle
	// timeout, revocation, and password-version checks are enforced server-side
	// on every request, so a shorter browser expiry would log out active users.
	if !cookies[0].Expires.Equal(sessions.createSession.ExpiresAt) {
		t.Fatalf("cookie expiry = %s, want absolute session expiry %s", cookies[0].Expires, sessions.createSession.ExpiresAt)
	}
	absSeconds := int((7 * 24 * time.Hour).Seconds())
	if cookies[0].MaxAge > absSeconds || cookies[0].MaxAge < absSeconds-2 {
		t.Fatalf("cookie max-age = %d, want within 2s of %d", cookies[0].MaxAge, absSeconds)
	}
	if !cookies[0].Secure {
		t.Fatal("cookie Secure = false, want true by default")
	}
	if cookies[0].Name != "__Host-warwick_session" {
		t.Fatalf("cookie name = %q, want __Host-warwick_session", cookies[0].Name)
	}
}
func TestHandleLoginUsesOpaqueCookieTokenInsteadOfSessionID(t *testing.T) {
	userID := uuid.New()
	sessionID := uuid.New()
	svc := NewService(ServiceOptions{
		Hasher: fakePasswordHasher{verifyOK: true},
		Sessions: fakeSessionStore{createSession: Session{
			ID:              sessionID,
			Token:           "opaque-cookie-token",
			UserID:          userID,
			CreatedAt:       time.Now().UTC(),
			LastSeenAt:      time.Now().UTC(),
			ExpiresAt:       time.Now().UTC().Add(sessionAbsoluteTimeout),
			PasswordVersion: 1,
		}},
		Limiter: &recordingLoginLimiter{allowed: true},
		Users: fakeUserLookup{byUsername: User{
			ID: userID, Username: "admin", Role: "Admin", PasswordHash: "hash", PasswordVersion: 1,
		}},
		Log:          nil,
		CookieSecure: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	if err := svc.HandleLogin(w, req); err != nil {
		t.Fatalf("HandleLogin: %v", err)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if cookies[0].Value == sessionID.String() {
		t.Fatalf("cookie reused database session ID %q", cookies[0].Value)
	}
	if cookies[0].Value != "opaque-cookie-token" {
		t.Fatalf("cookie value = %q, want opaque session token", cookies[0].Value)
	}
}
func TestRequireUserAcceptsOpaqueSessionToken(t *testing.T) {
	userID := uuid.New()
	sessionID := uuid.New()
	session := Session{
		ID:              sessionID,
		Token:           "opaque-session-token",
		UserID:          userID,
		CreatedAt:       time.Now().UTC(),
		LastSeenAt:      time.Now().UTC(),
		ExpiresAt:       time.Now().UTC().Add(time.Hour),
		PasswordVersion: 1,
	}
	svc := NewService(ServiceOptions{
		Hasher:   fakePasswordHasher{verifyOK: true},
		Sessions: fakeSessionStore{tokenSession: session},
		Limiter:  &recordingLoginLimiter{allowed: true},
		Users: fakeUserLookup{byUsername: User{
			ID: userID, Username: "admin", Role: "Admin", PasswordVersion: 1,
		}},
		Log:          nil,
		CookieSecure: true,
	})
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: "__Host-warwick_session", Value: session.Token})

	user, err := svc.RequireUser(context.Background(), req)
	if err != nil {
		t.Fatalf("RequireUser: %v", err)
	}
	if user.ID != userID {
		t.Fatalf("user ID = %s, want %s", user.ID, userID)
	}
}

func TestHandleLoginCanDisableSecureCookieForLocalHTTP(t *testing.T) {
	userID := uuid.New()
	svc := NewService(ServiceOptions{
		Hasher:   fakePasswordHasher{verifyOK: true},
		Sessions: fakeSessionStore{},
		Limiter:  &recordingLoginLimiter{allowed: true},
		Users: fakeUserLookup{byUsername: User{
			ID:              userID,
			Username:        "admin",
			Role:            "Admin",
			PasswordHash:    "hash",
			PasswordVersion: 1,
		}},
		Log:          nil,
		CookieSecure: false,
	})

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
	if cookies[0].Secure {
		t.Fatal("cookie Secure = true, want false for local HTTP override")
	}
	if cookies[0].Name != "warwick_session" {
		t.Fatalf("cookie name = %q, want warwick_session for local HTTP override", cookies[0].Name)
	}
}
func TestHandleLoginUsesConfiguredClientIPResolver(t *testing.T) {
	userID := uuid.New()
	limiter := &recordingLoginLimiter{}
	svc := NewService(ServiceOptions{
		Hasher:   fakePasswordHasher{verifyOK: true},
		Sessions: fakeSessionStore{},
		Limiter:  limiter,
		Users: fakeUserLookup{byUsername: User{
			ID:           userID,
			Username:     "admin",
			Role:         "Admin",
			PasswordHash: "hash",
		}},
		Log:          nil,
		CookieSecure: true,
		IPResolver:   fixedClientIPResolver{ip: "198.51.100.7"},
	})

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.8:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	if err := svc.HandleLogin(httptest.NewRecorder(), req); err != nil {
		t.Fatalf("HandleLogin: %v", err)
	}
	if limiter.lastIP != "198.51.100.7" {
		t.Fatalf("limiter IP = %q, want configured resolver result", limiter.lastIP)
	}
}
func TestSessionTokensAreRandomAndStoredAsOneWayHashes(t *testing.T) {
	tokenA, hashA, err := newSessionToken()
	if err != nil {
		t.Fatalf("newSessionToken A: %v", err)
	}
	tokenB, hashB, err := newSessionToken()
	if err != nil {
		t.Fatalf("newSessionToken B: %v", err)
	}
	if tokenA == tokenB {
		t.Fatal("session token generator returned the same bearer token twice")
	}
	if len(hashA) != 32 || len(hashB) != 32 {
		t.Fatalf("token hash lengths = %d and %d, want 32", len(hashA), len(hashB))
	}
	if bytes.Equal(hashA, []byte(tokenA)) || bytes.Equal(hashB, []byte(tokenB)) {
		t.Fatal("raw bearer token was used as its persisted value")
	}
	if bytes.Equal(hashA, hashB) {
		t.Fatal("different bearer tokens produced the same hash")
	}
}

func TestValidateSessionTokenPreservesLegacyUUIDCookieDuringMigration(t *testing.T) {
	userID := uuid.New()
	sessionID := uuid.New()
	now := time.Now().UTC()
	svc := NewService(ServiceOptions{
		Hasher: fakePasswordHasher{verifyOK: true},
		Sessions: fakeSessionStore{legacySession: Session{
			ID:              sessionID,
			UserID:          userID,
			CreatedAt:       now,
			LastSeenAt:      now,
			ExpiresAt:       now.Add(time.Hour),
			PasswordVersion: 1,
		}},
		Limiter: &recordingLoginLimiter{allowed: true},
		Users: fakeUserLookup{byUsername: User{
			ID: userID, Username: "admin", Role: "Admin", PasswordVersion: 1,
		}},
		Log:          nil,
		CookieSecure: true,
	})

	user, err := svc.ValidateSessionToken(context.Background(), sessionID.String())
	if err != nil {
		t.Fatalf("ValidateSessionToken: %v", err)
	}
	if user.ID != userID {
		t.Fatalf("user ID = %s, want %s", user.ID, userID)
	}
}

type fixedClientIPResolver struct {
	ip string
}

func (r fixedClientIPResolver) Resolve(*http.Request) string {
	return r.ip
}

type recordingLoginLimiter struct {
	calls        int
	allowed      bool
	lastUsername string
	lastIP       string
}

func (l *recordingLoginLimiter) Allow(_ context.Context, username, ip string) (RateLimitResult, error) {
	l.calls++
	l.lastUsername = username
	l.lastIP = ip
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
	tokenSession  Session
	legacySession Session
}

func (s fakeSessionStore) Create(_ context.Context, userID uuid.UUID, passwordVersion int32, _, _ time.Duration) (Session, error) {
	if s.createSession.ID != uuid.Nil {
		session := s.createSession
		if session.Token == "" {
			session.Token = "opaque-test-session-token"
		}
		return session, nil
	}
	now := time.Now().UTC()
	return Session{
		ID:              uuid.New(),
		Token:           "opaque-test-session-token",
		UserID:          userID,
		CreatedAt:       now,
		LastSeenAt:      now,
		ExpiresAt:       now.Add(sessionAbsoluteTimeout),
		PasswordVersion: passwordVersion,
	}, nil
}

func (s fakeSessionStore) ByToken(_ context.Context, _ string) (Session, error) {
	if s.tokenSession.ID != uuid.Nil {
		return s.tokenSession, nil
	}
	return Session{}, errors.New("not implemented")
}
func (s fakeSessionStore) ByID(_ context.Context, _ uuid.UUID) (Session, error) {
	if s.legacySession.ID != uuid.Nil {
		return s.legacySession, nil
	}
	return Session{}, errors.New("not implemented")
}

func (fakeSessionStore) RevokeByToken(_ context.Context, _ string) error {
	return nil
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
