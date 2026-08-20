package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestResilientLoginRateLimiterFallsBackAfterPrimaryFailure(t *testing.T) {
	primaryErr := errors.New("database unavailable")
	fallback := &fixedLoginRateLimiter{result: RateLimitResult{Allowed: false, Remaining: 0}}
	limiter := NewResilientLoginRateLimiter(&errorLoginRateLimiter{err: primaryErr}, fallback)

	result, err := limiter.Allow(context.Background(), "admin", "198.51.100.7")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if result.Allowed {
		t.Fatal("fallback allowed request after primary failure")
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallback.calls)
	}
}

func TestResilientLoginRateLimiterDoesNotFallbackOnPrimaryDecision(t *testing.T) {
	fallback := &fixedLoginRateLimiter{result: RateLimitResult{Allowed: true}}
	limiter := NewResilientLoginRateLimiter(
		&fixedLoginRateLimiter{result: RateLimitResult{Allowed: false}},
		fallback,
	)

	result, err := limiter.Allow(context.Background(), "admin", "198.51.100.7")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if result.Allowed {
		t.Fatal("primary denial was overridden by fallback")
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls = %d, want 0", fallback.calls)
	}
}

func TestResilientLoginRateLimiterFailsWhenBothStoresFail(t *testing.T) {
	primaryErr := errors.New("database unavailable")
	fallbackErr := errors.New("memory limiter unavailable")
	limiter := NewResilientLoginRateLimiter(
		&errorLoginRateLimiter{err: primaryErr},
		&errorLoginRateLimiter{err: fallbackErr},
	)

	_, err := limiter.Allow(context.Background(), "admin", "198.51.100.7")
	if !errors.Is(err, ErrRateLimiterUnavailable) {
		t.Fatalf("Allow error = %v, want ErrRateLimiterUnavailable", err)
	}
	if !errors.Is(err, primaryErr) || !errors.Is(err, fallbackErr) {
		t.Fatalf("Allow error = %v, want both underlying errors", err)
	}
}

func TestDBLoginRateLimiterFailsClosedWithoutStore(t *testing.T) {
	_, err := NewDBLoginRateLimiter(nil).Allow(context.Background(), "admin", "198.51.100.7")
	if !errors.Is(err, ErrRateLimiterUnavailable) {
		t.Fatalf("Allow error = %v, want ErrRateLimiterUnavailable", err)
	}
}

func TestLoginFailsClosedWhenLimiterUnavailable(t *testing.T) {
	svc := NewService(ServiceOptions{
		Hasher:       fakePasswordHasher{verifyOK: true},
		Sessions:     fakeSessionStore{},
		Limiter:      &errorLoginRateLimiter{err: errors.New("all limiters unavailable")},
		Users:        fakeUserLookup{byUsername: User{Username: "admin"}},
		Log:          nil,
		CookieSecure: true,
	})

	_, _, err := svc.Login(context.Background(), "admin", "secret", "198.51.100.7")
	if !errors.Is(err, ErrRateLimiterUnavailable) {
		t.Fatalf("Login error = %v, want ErrRateLimiterUnavailable", err)
	}
}

func TestResilientLimiterBlocksRepeatedInvalidLoginsDuringPrimaryOutage(t *testing.T) {
	fallback := NewInMemoryLoginRateLimiter()
	limiter := NewResilientLoginRateLimiter(
		&errorLoginRateLimiter{err: errors.New("database unavailable")},
		fallback,
	)
	svc := NewService(ServiceOptions{
		Hasher:   fakePasswordHasher{verifyOK: false},
		Sessions: fakeSessionStore{},
		Limiter:  limiter,
		Users: fakeUserLookup{byUsername: User{
			ID:           newTestUserID(),
			Username:     "admin",
			PasswordHash: "hash",
		}},
		Log:          nil,
		CookieSecure: true,
	})

	_, _, firstErr := svc.Login(context.Background(), "admin", "wrong", "198.51.100.7")
	if !errors.Is(firstErr, ErrInvalidCredentials) {
		t.Fatalf("first Login error = %v, want invalid credentials", firstErr)
	}
	_, _, secondErr := svc.Login(context.Background(), "admin", "wrong", "198.51.100.7")
	if !errors.Is(secondErr, ErrTooManyRequests) {
		t.Fatalf("second Login error = %v, want too many requests", secondErr)
	}
}

func newTestUserID() uuid.UUID {
	return uuid.MustParse("00000000-0000-0000-0000-000000000001")
}

type errorLoginRateLimiter struct {
	err error
}

func (l *errorLoginRateLimiter) Allow(context.Context, string, string) (RateLimitResult, error) {
	return RateLimitResult{}, l.err
}

type fixedLoginRateLimiter struct {
	result RateLimitResult
	calls  int
}

func (l *fixedLoginRateLimiter) Allow(context.Context, string, string) (RateLimitResult, error) {
	l.calls++
	return l.result, nil
}
