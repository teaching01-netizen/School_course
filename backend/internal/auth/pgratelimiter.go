package auth

import (
	"context"
	"fmt"
	"time"
)

// DBLoginRateLimiter implements LoginRateLimiter using a DB-backed sliding window.
// It wraps a RateLimitStore (satisfied by ratelimit.Store) for persistence across
// instances and supports progressive backoff.
type DBLoginRateLimiter struct {
	store    RateLimitStore
	username int
	ip       int
}

func NewDBLoginRateLimiter(store RateLimitStore) *DBLoginRateLimiter {
	return &DBLoginRateLimiter{
		store:    store,
		username: 5,
		ip:       5,
	}
}

func (l *DBLoginRateLimiter) Allow(ctx context.Context, username, ip string) (RateLimitResult, error) {
	if l.store == nil {
		return RateLimitResult{Allowed: true}, nil
	}

	result, err := l.store.Allow(ctx, "auth:ip:"+ip, l.ip, window60s)
	if err != nil {
		return RateLimitResult{}, fmt.Errorf("rate limit ip: %w", err)
	}
	if !result.Allowed {
		return result, nil
	}

	result, err = l.store.Allow(ctx, "auth:user:"+username, l.username, window60s)
	if err != nil {
		return RateLimitResult{}, fmt.Errorf("rate limit user: %w", err)
	}
	return result, nil
}

var window60s = 60 * time.Second
