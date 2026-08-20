package client

import "errors"

var (
	ErrUnsafeEndpoint        = errors.New("legacy client: endpoint is not allowlisted")
	ErrSessionExpired        = errors.New("legacy client: authenticated session expired")
	ErrAuthentication        = errors.New("legacy client: authentication failed")
	ErrRateLimited           = errors.New("legacy client: source rate limited")
	ErrSourceUnavailable     = errors.New("legacy client: source unavailable")
	ErrResponseTooLarge      = errors.New("legacy client: response exceeds configured limit")
	ErrUnexpectedContentType = errors.New("legacy client: unexpected content type")
	ErrCircuitOpen           = errors.New("legacy client: source circuit is open")
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
