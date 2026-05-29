package scheduling

import (
	"errors"
	"testing"
)

func TestErrStaleEdit_MatchViaErrorsAs(t *testing.T) {
	// Simulate wrapping a stale_edit from series in scheduling.Err.
	wrapped := &Err{Code: "stale_edit", Message: "stale edit"}

	var se *Err
	if !errors.As(wrapped, &se) {
		t.Fatal("errors.As failed to extract *Err")
	}
	if se.Code != "stale_edit" {
		t.Fatalf("Code = %q, want %q", se.Code, "stale_edit")
	}
}

func TestErrOtherCode_NotStaleEdit(t *testing.T) {
	other := &Err{Code: "room_overlap", Message: "room conflict"}

	var se *Err
	if !errors.As(other, &se) {
		t.Fatal("errors.As failed to extract *Err")
	}
	if se.Code == "stale_edit" {
		t.Fatal("expected non-stale_edit code")
	}
}

func TestErrWrapped_StaleEditInChain(t *testing.T) {
	inner := &Err{Code: "stale_edit", Message: "version mismatch"}
	outer := wrappedError{inner: inner}

	var se *Err
	if !errors.As(outer, &se) {
		t.Fatal("errors.As should unwrap through custom wrapper")
	}
	if se.Code != "stale_edit" {
		t.Fatalf("Code = %q, want %q", se.Code, "stale_edit")
	}
}

type wrappedError struct {
	inner error
}

func (w wrappedError) Error() string {
	return "wrapped: " + w.inner.Error()
}

func (w wrappedError) Unwrap() error {
	return w.inner
}
