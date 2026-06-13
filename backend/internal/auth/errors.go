package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrTooManyRequests    = errors.New("too many requests")
	ErrCredentialsTooLong = errors.New("credentials exceed maximum length")
	ErrUserNotFound       = errors.New("user not found")
)
