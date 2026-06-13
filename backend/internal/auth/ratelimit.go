package auth

import (
	"context"
	"time"
)

// RateLimitResult describes the outcome of a rate limit check.
type RateLimitResult struct {
	Allowed   bool
	Remaining int
}

// LoginRateLimiter abstracts login rate limiting.
type LoginRateLimiter interface {
	Allow(ctx context.Context, username, ip string) (RateLimitResult, error)
}

// RateLimitStore is the minimal interface for a rate limit store.
// Satisfied by ratelimit.Store from the infrastructure layer.
type RateLimitStore interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (RateLimitResult, error)
}
