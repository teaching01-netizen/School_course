package httpadapter

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/idempotency"
)

func TestDecodeJSON_RestoresExactOriginalBody(t *testing.T) {
	original := []byte("{\n  \"course_id\": \"course-1\"\n}")
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader(original))
	w := httptest.NewRecorder()
	var body map[string]any
	if err := (Adapter{}).DecodeJSON(w, r, &body); err != nil {
		t.Fatal(err)
	}
	restored, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, original) {
		t.Fatalf("got=%q want=%q", restored, original)
	}
}

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

// ---------------------------------------------------------------------------
// isSerializationFailure tests
// ---------------------------------------------------------------------------

func TestIsSerializationFailure_PgErr40001_ReturnsTrue(t *testing.T) {
	err := &pgconn.PgError{Code: "40001"}
	if !isSerializationFailure(err) {
		t.Fatal("expected 40001 to be a serialization failure")
	}
}

func TestIsSerializationFailure_PgErr40P01_ReturnsTrue(t *testing.T) {
	err := &pgconn.PgError{Code: "40P01"}
	if !isSerializationFailure(err) {
		t.Fatal("expected 40P01 to be a serialization failure")
	}
}

func TestIsSerializationFailure_PgErr23505_ReturnsFalse(t *testing.T) {
	err := &pgconn.PgError{Code: "23505"}
	if isSerializationFailure(err) {
		t.Fatal("expected 23505 (unique violation) to NOT be a serialization failure")
	}
}

func TestIsSerializationFailure_DeadlineExceeded_ReturnsFalse(t *testing.T) {
	err := context.DeadlineExceeded
	if isSerializationFailure(err) {
		t.Fatal("expected DeadlineExceeded to NOT be a serialization failure")
	}
}

func TestIsSerializationFailure_GenericError_ReturnsFalse(t *testing.T) {
	err := errors.New("something else")
	if isSerializationFailure(err) {
		t.Fatal("expected generic error to NOT be a serialization failure")
	}
}

// ---------------------------------------------------------------------------
// retryDelay tests
// ---------------------------------------------------------------------------

func TestRetryDelay_Attempt0_ReturnsZero(t *testing.T) {
	d := retryDelay(0)
	if d != 0 {
		t.Fatalf("expected 0 delay, got %v", d)
	}
}

func TestRetryDelay_Attempt1_Is10to30ms(t *testing.T) {
	const iterations = 100
	min, max := time.Hour, time.Duration(0)
	for range iterations {
		d := retryDelay(1)
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	if min < 10*time.Millisecond {
		t.Fatalf("attempt 1 min delay %v below 10ms range", min)
	}
	if max > 30*time.Millisecond {
		t.Fatalf("attempt 1 max delay %v above 30ms range", max)
	}
	if min == max {
		t.Fatal("attempt 1 delay is deterministic, expected jitter")
	}
}

func TestRetryDelay_Attempt2_Is30to70ms(t *testing.T) {
	min, max := time.Hour, time.Duration(0)
	for range 100 {
		d := retryDelay(2)
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	if min < 30*time.Millisecond {
		t.Fatalf("attempt 2 min delay %v below 30ms range", min)
	}
	if max > 70*time.Millisecond {
		t.Fatalf("attempt 2 max delay %v above 70ms range", max)
	}
	if min == max {
		t.Fatal("attempt 2 delay is deterministic, expected jitter")
	}
}

func TestRetryDelay_Attempt3_Is70ms(t *testing.T) {
	d := retryDelay(3)
	if d != 70*time.Millisecond {
		t.Fatalf("expected 70ms for attempt 3, got %v", d)
	}
}

func TestRetryDelay_Attempt4_Is70ms(t *testing.T) {
	d := retryDelay(4)
	if d != 70*time.Millisecond {
		t.Fatalf("expected 70ms for attempt 4+, got %v", d)
	}
}

// ---------------------------------------------------------------------------
// Retry behavior tests — use mock pool and mock transactions
// ---------------------------------------------------------------------------

// mockRow implements pgx.Row with a configurable scan result.
type mockRow struct {
	scanErr error
}

func (r *mockRow) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	for _, d := range dest {
		switch ptr := d.(type) {
		case *bool:
			*ptr = true // is_new = true
		case **int32:
			val := int32(http.StatusCreated)
			*ptr = &val // status_code = 201
		case *[]byte:
			*ptr = []byte(`{"id":"sess-1"}`)
		case *string:
			*ptr = "testhash"
		}
	}
	return nil
}

// mockTx implements pgx.Tx with configurable commit/rollback behavior.
// It delegates QueryRow and Exec to succeed for idempotency operations.
type mockTx struct {
	commitErr     atomic.Value // stores error
	rollbackCount atomic.Int32
}

func (tx *mockTx) Begin(ctx context.Context) (pgx.Tx, error) {
	panic("unexpected call to Begin")
}
func (tx *mockTx) BeginFunc(ctx context.Context, f func(pgx.Tx) error) error {
	panic("unexpected call to BeginFunc")
}
func (tx *mockTx) Commit(ctx context.Context) error {
	if err, ok := tx.commitErr.Load().(error); ok && err != nil {
		return err
	}
	return nil
}
func (tx *mockTx) Rollback(ctx context.Context) error {
	tx.rollbackCount.Add(1)
	return nil
}
func (tx *mockTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	panic("unexpected call to CopyFrom")
}
func (tx *mockTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	panic("unexpected call to SendBatch")
}
func (tx *mockTx) LargeObjects() pgx.LargeObjects {
	panic("unexpected call to LargeObjects")
}
func (tx *mockTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	panic("unexpected call to Prepare")
}
func (tx *mockTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (tx *mockTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	panic("unexpected call to Query")
}
func (tx *mockTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return &mockRow{}
}
func (tx *mockTx) Conn() *pgx.Conn {
	panic("unexpected call to Conn")
}

// mockPool implements txBeginner, returning pre-configured transactions.
type mockPool struct {
	makeTx    func(int) *mockTx
	callCount int
}

func (p *mockPool) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	tx := p.makeTx(p.callCount)
	p.callCount++
	return tx, nil
}

func testAdapter() Adapter {
	return Adapter{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestRetryBehavior_FnSerializationFailure_RetriesAndSucceeds(t *testing.T) {
	var fnCalls int
	pool := &mockPool{
		makeTx: func(int) *mockTx {
			return &mockTx{}
		},
	}
	a := testAdapter()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader([]byte(`{}`)))
	r.Header.Set("Idempotency-Key", "test-key-123456789012345")

	ok := a.WithSerializableIdempotentTx(w, r, uuidZero, "sessions", pool, nilQ, func(tx pgx.Tx) (int, any, error) {
		fnCalls++
		if fnCalls == 1 {
			return 0, nil, &pgconn.PgError{Code: "40001"}
		}
		return http.StatusCreated, map[string]any{"id": "sess-1"}, nil
	})
	if !ok {
		t.Fatal("expected WithSerializableIdempotentTx to succeed on retry")
	}
	if fnCalls != 2 {
		t.Fatalf("expected fn to be called 2 times, got %d", fnCalls)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 response, got %d", w.Code)
	}
}

func TestRetryBehavior_CommitSerializationFailure_RetriesAndSucceeds(t *testing.T) {
	var fnCalls int
	var txes []*mockTx
	pool := &mockPool{
		makeTx: func(i int) *mockTx {
			tx := &mockTx{}
			if i == 0 {
				tx.commitErr.Store(&pgconn.PgError{Code: "40001"})
			}
			txes = append(txes, tx)
			return tx
		},
	}
	a := testAdapter()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader([]byte(`{}`)))
	r.Header.Set("Idempotency-Key", "test-key-123456789012345")

	ok := a.WithSerializableIdempotentTx(w, r, uuidZero, "sessions", pool, nilQ, func(tx pgx.Tx) (int, any, error) {
		fnCalls++
		return http.StatusCreated, map[string]any{"id": "sess-1"}, nil
	})
	if !ok {
		t.Fatal("expected WithSerializableIdempotentTx to succeed after commit retry")
	}
	if fnCalls != 2 {
		t.Fatalf("expected fn to be called 2 times after commit retry, got %d", fnCalls)
	}
	if len(txes) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txes))
	}
	if txes[0].rollbackCount.Load() != 1 {
		t.Fatal("expected first transaction to be rolled back")
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 response, got %d", w.Code)
	}
}

func TestRetryBehavior_ThreeSerializationFailures_Returns500(t *testing.T) {
	var fnCalls int
	pool := &mockPool{
		makeTx: func(int) *mockTx {
			tx := &mockTx{}
			tx.commitErr.Store(&pgconn.PgError{Code: "40001"})
			return tx
		},
	}
	a := testAdapter()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader([]byte(`{}`)))
	r.Header.Set("Idempotency-Key", "test-key-123456789012345")

	ok := a.WithSerializableIdempotentTx(w, r, uuidZero, "sessions", pool, nilQ, func(tx pgx.Tx) (int, any, error) {
		fnCalls++
		return http.StatusCreated, map[string]any{"id": "sess-1"}, nil
	})
	if ok {
		t.Fatal("expected WithSerializableIdempotentTx to fail after 3 retries")
	}
	if fnCalls != 3 {
		t.Fatalf("expected fn to be called 3 times, got %d", fnCalls)
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 response, got %d", w.Code)
	}
}

func TestRetryBehavior_NonSerializationFnErr_NoRetry(t *testing.T) {
	var fnCalls int
	pool := &mockPool{
		makeTx: func(int) *mockTx {
			return &mockTx{}
		},
	}
	a := testAdapter()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader([]byte(`{}`)))
	r.Header.Set("Idempotency-Key", "test-key-123456789012345")

	ok := a.WithSerializableIdempotentTx(w, r, uuidZero, "sessions", pool, nilQ, func(tx pgx.Tx) (int, any, error) {
		fnCalls++
		return 0, nil, errors.New("business logic error")
	})
	if ok {
		t.Fatal("expected WithSerializableIdempotentTx to return false on business logic error")
	}
	if fnCalls != 1 {
		t.Fatalf("expected fn to be called 1 time, got %d", fnCalls)
	}
}

func TestRetryBehavior_NonSerializationCommitErr_NoRetry(t *testing.T) {
	var fnCalls int
	pool := &mockPool{
		makeTx: func(i int) *mockTx {
			tx := &mockTx{}
			tx.commitErr.Store(&pgconn.PgError{Code: "23505"})
			return tx
		},
	}
	a := testAdapter()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader([]byte(`{}`)))
	r.Header.Set("Idempotency-Key", "test-key-123456789012345")

	ok := a.WithSerializableIdempotentTx(w, r, uuidZero, "sessions", pool, nilQ, func(tx pgx.Tx) (int, any, error) {
		fnCalls++
		return http.StatusCreated, map[string]any{"id": "sess-1"}, nil
	})
	if ok {
		t.Fatal("expected WithSerializableIdempotentTx to return false on non-retryable commit error")
	}
	if fnCalls != 1 {
		t.Fatalf("expected fn to be called 1 time, got %d", fnCalls)
	}
}

// uuidZero is a uuid.UUID with all zeros.
var uuidZero = [16]byte{}

// nilQ is a *sqldb.Queries that is never directly queried —
// WithSerializableIdempotentTx only calls q.WithTx(tx) which creates a fresh
// Queries backed by the mock tx.
var nilQ = &sqldb.Queries{}
