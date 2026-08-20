package auth

import (
	"context"
	"errors"
	"fmt"
)

// ResilientLoginRateLimiter keeps the distributed limiter authoritative and
// uses a bounded in-process limiter only while the primary store is failing.
// A primary denial is never overridden by the fallback.
type ResilientLoginRateLimiter struct {
	primary  LoginRateLimiter
	fallback LoginRateLimiter
}

func NewResilientLoginRateLimiter(primary, fallback LoginRateLimiter) *ResilientLoginRateLimiter {
	return &ResilientLoginRateLimiter{primary: primary, fallback: fallback}
}

func (l *ResilientLoginRateLimiter) Allow(ctx context.Context, username, ip string) (RateLimitResult, error) {
	if l == nil || l.primary == nil {
		return RateLimitResult{}, fmt.Errorf("%w: primary limiter is not configured", ErrRateLimiterUnavailable)
	}

	result, err := l.primary.Allow(ctx, username, ip)
	if err == nil {
		return result, nil
	}
	primaryErr := err

	if l.fallback == nil {
		return RateLimitResult{}, fmt.Errorf("%w: primary limiter failed: %v", ErrRateLimiterUnavailable, primaryErr)
	}
	result, err = l.fallback.Allow(ctx, username, ip)
	if err == nil {
		return result, nil
	}

	return RateLimitResult{}, fmt.Errorf("%w: %w", ErrRateLimiterUnavailable, errors.Join(primaryErr, err))
}
