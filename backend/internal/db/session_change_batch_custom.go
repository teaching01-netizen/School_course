package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (q *Queries) SessionChangeBatchCreate(ctx context.Context, requestedCount int32, requestedBy pgtype.UUID, idempotencyKey string) (pgtype.UUID, string, error) {
	var id pgtype.UUID
	var status string
	err := q.db.QueryRow(ctx, `
		INSERT INTO session_change_batches (requested_count, requested_by, idempotency_key)
		VALUES ($1, $2, NULLIF($3, ''))
		ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL
		DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key
		RETURNING id, status
	`, requestedCount, requestedBy, idempotencyKey).Scan(&id, &status)
	return id, status, err
}

func (q *Queries) SessionChangeBatchComplete(ctx context.Context, id pgtype.UUID, succeededCount, failedCount int32) error {
	status := "completed"
	if failedCount > 0 {
		status = "failed"
	}
	_, err := q.db.Exec(ctx, `
		UPDATE session_change_batches
		SET succeeded_count = $2, failed_count = $3, status = $4, completed_at = now()
		WHERE id = $1
	`, id, succeededCount, failedCount, status)
	return err
}

func (q *Queries) SessionChangeBatchStatus(ctx context.Context, id pgtype.UUID) (string, error) {
	var status string
	err := q.db.QueryRow(ctx, `SELECT status FROM session_change_batches WHERE id = $1`, id).Scan(&status)
	if err == pgx.ErrNoRows {
		return "", err
	}
	return status, err
}
