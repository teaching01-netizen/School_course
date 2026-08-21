package main

import (
	"errors"
	"fmt"
	"testing"
	"time"

	sourceclient "warwick-institute/internal/legacysync/client"
)

// TestIsSystemicProfileError pins the abort-vs-skip classification for the
// student profile phase. Errors are wrapped so a classification that matches
// only the bare sentinel (or only the unwrapped type) cannot pass: the worker
// sees whatever the client returned, wrapped in "search students:".
func TestIsSystemicProfileError(t *testing.T) {
	abort := []struct {
		name string
		err  error
	}{
		{"rate limited", fmt.Errorf("search students: %w", &sourceclient.RateLimitedError{StatusCode: 429})},
		{"egress budget exceeded", fmt.Errorf("search students: %w", &sourceclient.EgressBudgetError{ResetAt: time.Now().Add(time.Minute)})},
		{"circuit open", fmt.Errorf("search students: %w", sourceclient.ErrCircuitOpen)},
		{"authentication failed", fmt.Errorf("search students: %w", sourceclient.ErrAuthentication)},
	}
	for _, tc := range abort {
		t.Run(tc.name, func(t *testing.T) {
			if !isSystemicProfileError(tc.err) {
				t.Fatalf("isSystemicProfileError(%v) = false, want true", tc.err)
			}
		})
	}

	skip := []struct {
		name string
		err  error
	}{
		{"source unavailable (5xx storm)", fmt.Errorf("search students: %w", sourceclient.ErrSourceUnavailable)},
		{"unexpected status", fmt.Errorf("search students: %w", &sourceclient.StatusError{StatusCode: 400, Path: "/x"})},
		{"unparsable page", fmt.Errorf("search students: parse: %w", errors.New("no form found"))},
		{"unrelated error", fmt.Errorf("search students: %w", errors.New("boom"))},
		{"nil", nil},
	}
	for _, tc := range skip {
		t.Run(tc.name, func(t *testing.T) {
			if isSystemicProfileError(tc.err) {
				t.Fatalf("isSystemicProfileError(%v) = true, want false", tc.err)
			}
		})
	}
}
