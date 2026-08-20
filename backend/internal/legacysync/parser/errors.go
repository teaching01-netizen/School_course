// Package parser implements strict, fail-closed parsing of the legacy
// system's HTML pages. Any deviation from the expected page contract is
// a DriftError and NOTHING from the page may be applied: parsers never
// return partial data.
package parser

import (
	"errors"
	"fmt"
)

// loginPageReason is the DriftError.Reason used when a login page or
// session redirect is detected. DriftError.Unwrap maps exactly this
// reason to ErrLoginPage so errors.Is(err, ErrLoginPage) succeeds.
const loginPageReason = "login page or session redirect"

// DriftError is a fail-closed parse failure: the page did not match the
// expected contract, so NOTHING from it may be applied.
type DriftError struct {
	PageType      string
	ParserVersion int
	Reason        string
}

func (e *DriftError) Error() string {
	return fmt.Sprintf("parser: %s v%d drift: %s", e.PageType, e.ParserVersion, e.Reason)
}

// Unwrap makes a login-page drift errors.Is-compatible with ErrLoginPage.
func (e *DriftError) Unwrap() error {
	if e.Reason == loginPageReason {
		return ErrLoginPage
	}
	return nil
}

// ErrLoginPage is wrapped (errors.Is-compatible) inside DriftError when
// the page looks like a login/redirect page instead of the expected
// content.
var ErrLoginPage = errors.New("parser: login page or session redirect detected")

// AsDrift reports whether err is (or wraps) a *DriftError.
func AsDrift(err error) (*DriftError, bool) {
	var d *DriftError
	if errors.As(err, &d) {
		return d, true
	}
	return nil, false
}

// drift builds a *DriftError for a contract violation.
func drift(c PageContract, reason string) error {
	return &DriftError{PageType: c.PageType, ParserVersion: c.ParserVersion, Reason: reason}
}
