package auth

import "errors"

var (
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrTooManyRequests        = errors.New("too many requests")
	ErrRateLimiterUnavailable = errors.New("rate limiter unavailable")
	ErrCredentialsTooLong     = errors.New("credentials exceed maximum length")
	ErrUserNotFound           = errors.New("user not found")
)
