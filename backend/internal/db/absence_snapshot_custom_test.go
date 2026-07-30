package db

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSessionVersionConflictError_Error(t *testing.T) {
	err := &SessionVersionConflictError{
		SessionID:       "abc-123",
		ExpectedVersion: 3,
		ActualVersion:   5,
	}

	msg := err.Error()
	if msg != "session abc-123 version conflict: expected 3, got 5" {
		t.Errorf("unexpected error message: %q", msg)
	}
}

func TestSessionVersionConflictError_ImplementsError(t *testing.T) {
	var err error = &SessionVersionConflictError{}
	_ = err // just verify it satisfies the error interface
}

func TestSessionVersionConflictError_UsableWithErrorsAs(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", &SessionVersionConflictError{
		SessionID:       "xyz-789",
		ExpectedVersion: 1,
		ActualVersion:   2,
	})

	var versionErr *SessionVersionConflictError
	if !errors.As(wrapped, &versionErr) {
		t.Error("expected errors.As to find SessionVersionConflictError in wrapped error")
	}
	if versionErr.SessionID != "xyz-789" {
		t.Errorf("session ID mismatch: got %q, want xyz-789", versionErr.SessionID)
	}
	if versionErr.ExpectedVersion != 1 {
		t.Errorf("expected version mismatch: got %d, want 1", versionErr.ExpectedVersion)
	}
	if versionErr.ActualVersion != 2 {
		t.Errorf("actual version mismatch: got %d, want 2", versionErr.ActualVersion)
	}
}

func TestMissedSessionSnapshotInput_NilVersion(t *testing.T) {
	input := MissedSessionSnapshotInput{
		SessionID:       pgtype.UUID{Valid: true},
		ExpectedVersion: nil,
	}
	if input.ExpectedVersion != nil {
		t.Error("expected nil ExpectedVersion")
	}
}

func TestMissedSessionSnapshotInput_WithVersion(t *testing.T) {
	v := int32(42)
	input := MissedSessionSnapshotInput{
		SessionID:       pgtype.UUID{Valid: true},
		ExpectedVersion: &v,
	}
	if input.ExpectedVersion == nil || *input.ExpectedVersion != 42 {
		t.Errorf("expected ExpectedVersion 42, got %v", input.ExpectedVersion)
	}
}

func TestMissedSessionSnapshotData_Fields(t *testing.T) {
	data := MissedSessionSnapshotData{
		Quality:  "exact",
		Source:   "captured_at_submission",
		Version:  7,
	}
	if data.Quality != "exact" {
		t.Errorf("quality mismatch: got %q, want exact", data.Quality)
	}
	if data.Source != "captured_at_submission" {
		t.Errorf("source mismatch: got %q, want captured_at_submission", data.Source)
	}
	if data.Version != 7 {
		t.Errorf("version mismatch: got %d, want 7", data.Version)
	}
}
