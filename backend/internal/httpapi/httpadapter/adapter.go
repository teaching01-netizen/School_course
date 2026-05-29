package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/idempotency"
	"warwick-institute/internal/scheduling"
)

type AuthService interface {
	RequireUser(ctx context.Context, r *http.Request) (auth.User, error)
	HandleLogin(w http.ResponseWriter, r *http.Request) error
	HandleLogout(w http.ResponseWriter, r *http.Request) error
}

type Adapter struct {
	auth AuthService
	log  *slog.Logger
}

func New(authSvc AuthService, log *slog.Logger) Adapter {
	return Adapter{auth: authSvc, log: log}
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (Adapter) WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (a Adapter) WriteErr(w http.ResponseWriter, status int, code string, message string) {
	a.WriteJSON(w, status, apiError{Code: code, Message: message})
}

func (a Adapter) WriteErrDetails(w http.ResponseWriter, status int, code string, message string, details any) {
	a.WriteJSON(w, status, apiError{Code: code, Message: message, Details: details})
}

func (a Adapter) MustUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if a.auth == nil {
		a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return auth.User{}, false
	}
	u, err := a.auth.RequireUser(r.Context(), r)
	if err != nil {
		a.WriteErr(w, http.StatusUnauthorized, "unauthorized", "Not signed in")
		return auth.User{}, false
	}
	return u, true
}

func (a Adapter) MustAdmin(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	u, ok := a.MustUser(w, r)
	if !ok {
		return auth.User{}, false
	}
	if u.Role != "Admin" {
		a.WriteErr(w, http.StatusForbidden, "forbidden", "Admin only")
		return auth.User{}, false
	}
	return u, true
}

func (Adapter) UUIDString(u pgtype.UUID) (string, error) {
	if !u.Valid {
		return "", fmt.Errorf("invalid uuid")
	}
	id, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func (Adapter) TimeString(t pgtype.Timestamptz) (string, bool) {
	if !t.Valid {
		return "", false
	}
	return t.Time.UTC().Format(time.RFC3339Nano), true
}

func (Adapter) Int32Ptr(i pgtype.Int4) *int32 {
	if !i.Valid {
		return nil
	}
	v := i.Int32
	return &v
}

func (Adapter) ParseUUID(s string) (pgtype.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}

func (Adapter) ParseTimestamptz(s string) (pgtype.Timestamptz, error) {
	if s == "" {
		return pgtype.Timestamptz{}, fmt.Errorf("missing timestamp")
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// RFC3339 is a subset; keep error stable.
		t2, err2 := time.Parse(time.RFC3339, s)
		if err2 != nil {
			return pgtype.Timestamptz{}, err
		}
		t = t2
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}, nil
}

func (Adapter) ParseLocalDateYYYYMMDD(s string) (scheduling.LocalDate, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return scheduling.LocalDate{}, err
	}
	return scheduling.LocalDate{Year: t.Year(), Month: t.Month(), Day: t.Day()}, nil
}

func (Adapter) ParseClockHHMM(s string) (scheduling.Clock, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return scheduling.Clock{}, err
	}
	return scheduling.Clock{Hour: t.Hour(), Minute: t.Minute()}, nil
}

func (Adapter) ClockFromPgTime(t pgtype.Time) (scheduling.Clock, bool) {
	if !t.Valid {
		return scheduling.Clock{}, false
	}
	// Microseconds since midnight.
	us := t.Microseconds
	h := int(us / (60 * 60 * 1_000_000))
	us -= int64(h) * 60 * 60 * 1_000_000
	m := int(us / (60 * 1_000_000))
	return scheduling.Clock{Hour: h, Minute: m}, true
}

func (Adapter) DecodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
	return json.NewDecoder(r.Body).Decode(v)
}

// IdempotencyScope returns a short scope string derived from the URL path.
func IdempotencyScope(path string) string {
	return idempotency.IdempotencyScope(path)
}

// RequireIdempotencyKey validates and returns the Idempotency-Key header value.
// Writes a 400 error and returns false if the key is missing or invalid.
func (a Adapter) RequireIdempotencyKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key, err := idempotency.RequireKey(r)
	if err != nil {
		a.WriteErr(w, http.StatusBadRequest, "bad_idempotency_key", err.Error())
		return "", false
	}
	return key, true
}

// ReadBodyBytes reads the full request body and replaces r.Body with a fresh reader.
func (Adapter) ReadBodyBytes(r *http.Request) ([]byte, error) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bodyBytes, nil
}

// NewRequestFingerprint computes a SHA256 fingerprint of method + path + query + body.
func (Adapter) NewRequestFingerprint(method string, url *url.URL, bodyBytes []byte) string {
	return idempotency.NewRequestFingerprint(method, url, bodyBytes)
}

// HandleIdempotencyErr writes an appropriate error response for idempotency errors.
// Returns true if the error was handled (no further action needed).
func (a Adapter) HandleIdempotencyErr(w http.ResponseWriter, err error) bool {
	var reuse *idempotency.ErrIdempotencyKeyReuse
	if errors.As(err, &reuse) {
		a.WriteErr(w, http.StatusConflict, "idempotency_key_reuse", reuse.Error())
		return true
	}
	var stale *idempotency.ErrStaleIdempotencyRecord
	if errors.As(err, &stale) {
		a.WriteErr(w, http.StatusConflict, "stale_idempotency_record", stale.Error())
		return true
	}
	return false
}

// WithIdempotentTx wraps a mutating handler inside a database transaction, enforcing
// idempotency via the Idempotency-Key header. Use for handlers that directly use Q.
//
// The flow:
//  1. Validate and extract Idempotency-Key header
//  2. Read full body bytes and compute request fingerprint
//  3. Begin a DB transaction
//  4. Acquire idempotency lock (INSERT … ON CONFLICT)
//  5. If replay with cached response → rollback and return cached response
//  6. Execute fn(tx), which receives the transaction handle
//  7. Complete the idempotency record with fn's returned status/response
//  8. Commit the transaction
func (a Adapter) WithIdempotentTx(
	w http.ResponseWriter,
	r *http.Request,
	userID uuid.UUID,
	scope string,
	pool *pgxpool.Pool,
	q *sqldb.Queries,
	fn func(tx pgx.Tx) (int, any, error),
) bool {
	key, ok := a.RequireIdempotencyKey(w, r)
	if !ok {
		return false
	}

	bodyBytes, err := a.ReadBodyBytes(r)
	if err != nil {
		a.WriteErr(w, http.StatusBadRequest, "bad_request", "cannot read request body")
		return false
	}

	fingerprint := idempotency.NewRequestFingerprint(r.Method, r.URL, bodyBytes)
	expiry := idempotency.DefaultUIExpiry()

	tx, err := pool.Begin(r.Context())
	if err != nil {
		a.log.Error("idempotent tx: begin failed", "error", err)
		a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return false
	}
	defer tx.Rollback(context.Background()) // no-op on committed tx; avoid r.Context() which may be cancelled

	qtx := q.WithTx(tx)

	isNew, cached, err := idempotency.Acquire(r.Context(), qtx, userID, scope, key, fingerprint, expiry)
	if err != nil {
		if a.HandleIdempotencyErr(w, err) {
			return false
		}
		a.log.Error("idempotent tx: acquire failed", "error", err)
		a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return false
	}

	if !isNew && cached != nil && cached.StatusCode == nil {
		// Stale record: server crashed between Acquire and Complete.
		// Delete the stale record and tell the client to retry with a new key.
		if _, delErr := qtx.IdempotencyDeleteStale(r.Context(), pgtype.UUID{Bytes: userID, Valid: true}, scope, key); delErr != nil {
			a.log.Error("idempotent tx: delete stale record failed", "error", delErr)
		}
		a.WriteErr(w, http.StatusConflict, "stale_idempotency_record",
			fmt.Sprintf("stale idempotency record for key %q; retry with a new idempotency key", key))
		return true
	}

	if !isNew && cached != nil && cached.StatusCode != nil && len(cached.ResponseBody) > 0 {
		// Replay: return the cached response. Rollback the no-op tx.
		tx.Rollback(context.Background())
		a.writeRawJSON(w, int(*cached.StatusCode), cached.ResponseBody)
		return true
	}

	statusCode, resp, fnErr := fn(tx)
	if fnErr != nil {
		// fnErr is the error from the mutation — the caller should have already
		// written an error response. Just return false.
		return false
	}

	// Complete the idempotency record.
	qtx2 := q.WithTx(tx)
	if err := idempotency.Complete(r.Context(), qtx2, userID, scope, key, statusCode, resp, expiry); err != nil {
		a.log.Error("idempotent tx: complete failed", "error", err)
		a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return false
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.log.Error("idempotent tx: commit failed", "error", err)
		a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return false
	}

	a.WriteJSON(w, statusCode, resp)
	return true
}

// writeRawJSON writes a raw JSON byte slice as the response with the given status code.
func (a Adapter) writeRawJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (a Adapter) ClassifyDBErr(err error) (status int, code string, message string) {
	// Context cancellation/timeout — distinct from internal errors.
	// These occur when the per-request context deadline is exceeded
	// (see httpapi.withRequestTimeout middleware) or the client disconnects.
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, "timeout", "Request timed out"
	}
	if errors.Is(err, context.Canceled) {
		return http.StatusServiceUnavailable, "canceled", "Request cancelled"
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return http.StatusConflict, "conflict", "Already exists"
		case "23P01":
			return http.StatusConflict, "schedule_conflict", "Schedule conflict"
		case "23514":
			// Includes availability violations from triggers.
			if a.log != nil {
				a.log.Error("constraint violation", "error", err, "pg_code", pgErr.Code, "pg_message", pgErr.Message)
			}
			msg := pgErr.Message
			if msg == "" {
				msg = "Constraint failed"
			}
			if strings.Contains(strings.ToLower(msg), "not available") {
				return http.StatusConflict, "availability_violation", msg
			}
			return http.StatusConflict, "constraint_failed", "Constraint failed"
		case "23503":
			return http.StatusBadRequest, "invalid_reference", "Invalid reference"
		default:
			if a.log != nil {
				a.log.Error("database error", "error", err, "pg_code", pgErr.Code)
			}
			return http.StatusInternalServerError, "db_error", "Database error"
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return http.StatusNotFound, "not_found", "Not found"
	}
	if a.log != nil {
		a.log.Error("unhandled error", "error", err)
	}
	return http.StatusInternalServerError, "internal", "Internal error"
}
