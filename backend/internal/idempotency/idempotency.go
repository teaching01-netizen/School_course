// Package idempotency provides helpers for enforcing Idempotency-Key on mutating HTTP endpoints.
package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// CleanupExpired deletes expired idempotency records in batches to avoid long-held locks.
// It loops, deleting batchSize rows per iteration, until no more expired rows remain.
// Returns the total number of deleted rows.
func CleanupExpired(ctx context.Context, db sqldb.DBTX, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 5000
	}
	var total int64
	for {
		tag, err := db.Exec(ctx,
			`DELETE FROM idempotency_keys
			 WHERE id IN (
			   SELECT id FROM idempotency_keys WHERE expires_at < now() LIMIT $1
			 )`,
			batchSize,
		)
		if err != nil {
			return total, fmt.Errorf("idempotency cleanup batch: %w", err)
		}
		deleted := tag.RowsAffected()
		total += deleted
		if int64(deleted) < int64(batchSize) {
			break
		}
	}
	return total, nil
}

// ErrStaleIdempotencyRecord is returned when a replay hits a stale record (status_code=NULL)
// left behind by a crash between Acquire and Complete.
type ErrStaleIdempotencyRecord struct {
	Key string
}

func (e *ErrStaleIdempotencyRecord) Error() string {
	return fmt.Sprintf("stale idempotency record for key %q (server crashed before completion); retry with a new idempotency key", e.Key)
}

// SystemActorUUID is the sentinel UUID used for background/system jobs
// that have no logged-in human actor.
var SystemActorUUID = uuid.MustParse("00000000-0000-0000-0000-000000000000")

// ErrIdempotencyKeyReuse is returned when the same key is used with a different payload.
type ErrIdempotencyKeyReuse struct {
	Key string
}

func (e *ErrIdempotencyKeyReuse) Error() string {
	return fmt.Sprintf("idempotency key %q cannot be reused with a different request", e.Key)
}

// maxKeyLength is the maximum allowed length for an Idempotency-Key value.
const maxKeyLength = 128
const minKeyLength = 16

// validKeyChars contains characters allowed in Idempotency-Key values.
const validKeyChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789:._-"

// RequireKey validates the Idempotency-Key header on a request.
// Returns the key value, or an error describing why it's invalid.
func RequireKey(r *http.Request) (string, error) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		return "", fmt.Errorf("missing required Idempotency-Key header")
	}
	if len(key) < minKeyLength {
		return "", fmt.Errorf("Idempotency-Key too short: got %d chars, minimum %d", len(key), minKeyLength)
	}
	if len(key) > maxKeyLength {
		return "", fmt.Errorf("Idempotency-Key too long: got %d chars, maximum %d", len(key), maxKeyLength)
	}
	for _, ch := range key {
		if !isValidKeyChar(ch) {
			return "", fmt.Errorf("Idempotency-Key contains invalid character %q (allowed: A-Za-z0-9:._-)", ch)
		}
	}
	return key, nil
}

func isValidKeyChar(ch rune) bool {
	return strings.ContainsRune(validKeyChars, ch)
}

// NewRequestFingerprint computes a SHA256 fingerprint of the request.
func NewRequestFingerprint(method string, u *url.URL, bodyBytes []byte) string {
	h := sha256.New()
	_, _ = io.WriteString(h, method)
	_, _ = io.WriteString(h, ":")
	_, _ = io.WriteString(h, u.Path)
	if u.RawQuery != "" {
		_, _ = io.WriteString(h, "?")
		_, _ = io.WriteString(h, u.RawQuery)
	}
	_, _ = io.WriteString(h, ":")
	if len(bodyBytes) > 0 {
		_, _ = h.Write(bodyBytes)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// pgUUID converts a go uuid.UUID to pgtype.UUID.
func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// Acquire attempts to acquire the idempotency key for a new request.
//
// It returns (true, nil) if this is a new request.
// It returns (false, nil) if this is a replay with matching hash but no cached response yet.
// It returns (false, row) if this is a replay with a cached response.
// It returns an *ErrIdempotencyKeyReuse if the key exists with a different request_hash.
func Acquire(ctx context.Context, q *sqldb.Queries, actorUserID uuid.UUID, scope, key, requestHash string, expiresAt time.Time) (bool, *sqldb.IdempotencyAcquireRow, error) {
	row, err := q.IdempotencyAcquire(ctx, sqldb.IdempotencyAcquireParams{
		ActorUserID:    pgUUID(actorUserID),
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    requestHash,
		ExpiresAt:      pgtype.Timestamptz{Time: expiresAt.UTC(), Valid: true},
	})
	if err != nil {
		return false, nil, err
	}

	if row.IsNew {
		return true, &row, nil
	}

	if row.RequestHash != requestHash {
		return false, nil, &ErrIdempotencyKeyReuse{Key: key}
	}

	return false, &row, nil
}

// Complete stores the response for an idempotency key after a mutation completes.
func Complete(ctx context.Context, q *sqldb.Queries, actorUserID uuid.UUID, scope, key string, statusCode int, responseBody any, expiresAt time.Time) error {
	var bodyJSON []byte
	switch v := responseBody.(type) {
	case []byte:
		bodyJSON = v
	case string:
		bodyJSON = []byte(v)
	default:
		var err error
		bodyJSON, err = json.Marshal(responseBody)
		if err != nil {
			return fmt.Errorf("marshal response body: %w", err)
		}
	}
	return q.IdempotencyComplete(ctx, sqldb.IdempotencyCompleteParams{
		ActorUserID:    pgUUID(actorUserID),
		Scope:          scope,
		IdempotencyKey: key,
		StatusCode:     int32(statusCode),
		ResponseBody:   string(bodyJSON),
		ExpiresAt:      pgtype.Timestamptz{Time: expiresAt.UTC(), Valid: true},
	})
}

// DefaultUIExpiry returns an expiry time 24 hours from now, suitable for UI mutations.
func DefaultUIExpiry() time.Time {
	return time.Now().UTC().Add(24 * time.Hour)
}

// DefaultJobExpiry returns an expiry time 7 days from now, suitable for background jobs.
func DefaultJobExpiry() time.Time {
	return time.Now().UTC().Add(7 * 24 * time.Hour)
}

// IdempotencyScope returns a short scope string derived from the URL path.
func IdempotencyScope(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/v1/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "unknown"
	}
	return parts[0]
}
