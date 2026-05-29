package auth

import "errors"

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrTooManyRequests = errors.New("too many requests")
var ErrCredentialsTooLong = errors.New("credentials exceed maximum length")
