package client

import (
	"errors"
	"time"
)

var (
	ErrUnsafeEndpoint        = errors.New("legacy client: endpoint is not allowlisted")
	ErrSessionExpired        = errors.New("legacy client: authenticated session expired")
	ErrAuthentication        = errors.New("legacy client: authentication failed")
	ErrRateLimited           = errors.New("legacy client: source rate limited")
	ErrSourceUnavailable     = errors.New("legacy client: source unavailable")
	ErrResponseTooLarge      = errors.New("legacy client: response exceeds configured limit")
	ErrUnexpectedContentType = errors.New("legacy client: unexpected content type")
	ErrCircuitOpen           = errors.New("legacy client: source circuit is open")
	ErrEgressBudgetExceeded  = errors.New("legacy client: egress budget exceeded")
)

type StatusError struct {
	StatusCode int
	Path       string
}

func (e *StatusError) Error() string {
	return "legacy client: source returned an unexpected HTTP status"
}

type ConfigError struct{ Message string }

func (e *ConfigError) Error() string { return "legacy client: invalid configuration: " + e.Message }

type RateLimitedError struct {
	RetryAfter time.Duration
	StatusCode int
}

func (e *RateLimitedError) Error() string { return ErrRateLimited.Error() }

func (e *RateLimitedError) Is(target error) bool { return errors.Is(target, ErrRateLimited) }

func (e *RateLimitedError) RetryAfterDuration() time.Duration { return e.RetryAfter }

type EgressBudgetError struct {
	ResetAt time.Time
}

func (e *EgressBudgetError) Error() string { return ErrEgressBudgetExceeded.Error() }

func (e *EgressBudgetError) Is(target error) bool { return errors.Is(target, ErrEgressBudgetExceeded) }

func (e *EgressBudgetError) RetryAfterDuration() time.Duration { return time.Until(e.ResetAt) }
