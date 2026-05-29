package httpadapter

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"warwick-institute/internal/idempotency"
)

func TestParseTimestamptz_AcceptsRFC3339AndNano(t *testing.T) {
	a := Adapter{}

	tsNano := "2026-05-21T12:34:56.123456789Z"
	gotNano, err := a.ParseTimestamptz(tsNano)
	if err != nil {
		t.Fatalf("ParseTimestamptz(nano) err = %v", err)
	}
	if !gotNano.Valid {
		t.Fatalf("ParseTimestamptz(nano) valid = false")
	}
	if gotNano.Time.UTC().Format(time.RFC3339Nano) != tsNano {
		t.Fatalf("ParseTimestamptz(nano) mismatch: got=%q want=%q", gotNano.Time.UTC().Format(time.RFC3339Nano), tsNano)
	}

	ts := "2026-05-21T12:34:56Z"
	got, err := a.ParseTimestamptz(ts)
	if err != nil {
		t.Fatalf("ParseTimestamptz err = %v", err)
	}
	if !got.Valid {
		t.Fatalf("ParseTimestamptz valid = false")
	}
	if got.Time.UTC().Format(time.RFC3339) != ts {
		t.Fatalf("ParseTimestamptz mismatch: got=%q want=%q", got.Time.UTC().Format(time.RFC3339), ts)
	}
}

func TestClassifyDBErr_NoRows_IsNotFound(t *testing.T) {
	a := Adapter{}
	status, code, msg := a.ClassifyDBErr(pgx.ErrNoRows)
	if status != 404 || code != "not_found" || msg != "Not found" {
		t.Fatalf("ClassifyDBErr(no rows) got (%d,%q,%q)", status, code, msg)
	}
}

func TestHandleIdempotencyErr_Reuse(t *testing.T) {
	a := Adapter{}
	w := httptest.NewRecorder()
	err := &idempotency.ErrIdempotencyKeyReuse{Key: "my-key"}
	handled := a.HandleIdempotencyErr(w, err)
	if !handled {
		t.Fatal("expected HandleIdempotencyErr to handle ErrIdempotencyKeyReuse")
	}
	if w.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", w.Code)
	}
}

func TestHandleIdempotencyErr_StaleRecord(t *testing.T) {
	a := Adapter{}
	w := httptest.NewRecorder()
	err := &idempotency.ErrStaleIdempotencyRecord{Key: "my-key"}
	handled := a.HandleIdempotencyErr(w, err)
	if !handled {
		t.Fatal("expected HandleIdempotencyErr to handle ErrStaleIdempotencyRecord")
	}
	if w.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", w.Code)
	}
}

func TestHandleIdempotencyErr_OtherError(t *testing.T) {
	a := Adapter{}
	w := httptest.NewRecorder()
	err := errors.New("some other error")
	handled := a.HandleIdempotencyErr(w, err)
	if handled {
		t.Fatal("expected HandleIdempotencyErr to NOT handle unrelated errors")
	}
}
