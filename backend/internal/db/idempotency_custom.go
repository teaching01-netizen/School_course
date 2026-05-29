package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// IdempotencyAcquireParams are the input parameters for acquiring an idempotency key.
type IdempotencyAcquireParams struct {
	ActorUserID    pgtype.UUID
	Scope          string
	IdempotencyKey string
	RequestHash    string
	ExpiresAt      pgtype.Timestamptz
}

// IdempotencyCompleteParams are the input parameters for completing an idempotency key.
type IdempotencyCompleteParams struct {
	ActorUserID    pgtype.UUID
	Scope          string
	IdempotencyKey string
	StatusCode     int32
	ResponseBody   string
	ExpiresAt      pgtype.Timestamptz
}

// IdempotencyAcquireRow is the result of an acquire operation.
type IdempotencyAcquireRow struct {
	StatusCode   *int32
	ResponseBody []byte
	RequestHash  string
	IsNew        bool
}

// IdempotencyAcquire attempts to insert a new idempotency key, or returns
// the existing record if the key already exists.
//
// The function uses INSERT ... ON CONFLICT ... DO UPDATE so it is safe
// under concurrent access. The is_new flag is determined by the PostgreSQL
// xmax system column: 0 for a newly inserted row, non-zero for an existing row.
func (q *Queries) IdempotencyAcquire(ctx context.Context, arg IdempotencyAcquireParams) (IdempotencyAcquireRow, error) {
	var row IdempotencyAcquireRow
	var statusCode *int32

	err := q.db.QueryRow(ctx, `
		INSERT INTO idempotency_keys (actor_user_id, scope, idempotency_key, request_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (actor_user_id, scope, idempotency_key) DO UPDATE SET
			expires_at = GREATEST(idempotency_keys.expires_at, EXCLUDED.expires_at)
		RETURNING
			(xmax = 0) AS is_new,
			status_code,
			response_body,
			COALESCE(request_hash, '') AS request_hash
	`,
		arg.ActorUserID,
		arg.Scope,
		arg.IdempotencyKey,
		arg.RequestHash,
		arg.ExpiresAt,
	).Scan(&row.IsNew, &statusCode, &row.ResponseBody, &row.RequestHash)
	if err != nil {
		return IdempotencyAcquireRow{}, fmt.Errorf("idempotency acquire: %w", err)
	}

	row.StatusCode = statusCode
	return row, nil
}

// IdempotencyComplete stores the response body and status code for an idempotency key.
// This should be called after the mutation has completed (success or error).
// IdempotencyCleanup deletes all expired idempotency keys and returns the count of deleted rows.
func (q *Queries) IdempotencyCleanup(ctx context.Context) (int64, error) {
	res, err := q.db.Exec(ctx, `DELETE FROM idempotency_keys WHERE expires_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("idempotency cleanup: %w", err)
	}
	return res.RowsAffected(), nil
}

// IdempotencyDeleteStale deletes a specific stale idempotency record (one with status_code IS NULL)
// that was left behind by a crash between Acquire and Complete. Returns true if a row was deleted.
func (q *Queries) IdempotencyDeleteStale(ctx context.Context, actorUserID pgtype.UUID, scope, idempotencyKey string) (bool, error) {
	tag, err := q.db.Exec(ctx, `
		DELETE FROM idempotency_keys
		WHERE actor_user_id = $1
		  AND scope = $2
		  AND idempotency_key = $3
		  AND status_code IS NULL
	`, actorUserID, scope, idempotencyKey)
	if err != nil {
		return false, fmt.Errorf("idempotency delete stale: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (q *Queries) IdempotencyComplete(ctx context.Context, arg IdempotencyCompleteParams) error {
	_, err := q.db.Exec(ctx, `
		UPDATE idempotency_keys
		SET status_code = $4,
		    response_body = $5::jsonb,
		    expires_at = GREATEST(expires_at, $6)
		WHERE actor_user_id = $1
		  AND scope = $2
		  AND idempotency_key = $3
	`,
		arg.ActorUserID,
		arg.Scope,
		arg.IdempotencyKey,
		arg.StatusCode,
		arg.ResponseBody,
		arg.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("idempotency complete: %w", err)
	}
	return nil
}
