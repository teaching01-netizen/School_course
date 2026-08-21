package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// LegacyJobAttemptGet returns the current attempt counter of a job so
// callers can schedule exponential retry backoff outside the database.
func (q *Queries) LegacyJobAttemptGet(ctx context.Context, id pgtype.UUID) (int32, error) {
	var attempt int32
	err := q.db.QueryRow(ctx, `SELECT attempt FROM legacy_sync_jobs WHERE id = $1`, id).Scan(&attempt)
	return attempt, err
}

func (q *Queries) LegacyJobHeartbeatOwned(ctx context.Context, id pgtype.UUID, workerID pgtype.Text, leaseSeconds int64) (bool, error) {
	tag, err := q.db.Exec(ctx, `
UPDATE legacy_sync_jobs
SET locked_until = now() + ($1 * interval '1 second'),
    heartbeat_at = now(),
    updated_at = now()
WHERE id = $2 AND status = 'running' AND locked_by = $3
`, leaseSeconds, id, workerID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (q *Queries) LegacyJobCompleteOwned(ctx context.Context, id pgtype.UUID, workerID pgtype.Text) (bool, error) {
	tag, err := q.db.Exec(ctx, `
UPDATE legacy_sync_jobs
SET status = 'completed',
    locked_by = NULL,
    locked_until = NULL,
    updated_at = now()
WHERE id = $1 AND status = 'running' AND locked_by = $2
`, id, workerID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// LegacyJobRetryOwnedWithRunAfter re-queues (or dead-letters) a running job,
// but only when the caller still holds the lease. runAfter carries the
// caller-computed retry delay (exponential backoff + Retry-After + circuit
// floor). The UPDATE row count is the ownership signal, so the dead-letter
// insert shares the statement: an exhausted job is mirrored into
// legacy_sync_dead_letters atomically with the status change.
func (q *Queries) LegacyJobRetryOwnedWithRunAfter(ctx context.Context, id pgtype.UUID, workerID pgtype.Text, lastError string, runAfter pgtype.Timestamptz) (bool, error) {
	var retried int64
	err := q.db.QueryRow(ctx, `
WITH retried AS (
    UPDATE legacy_sync_jobs
    SET status = CASE WHEN attempt >= max_attempts THEN 'dead' ELSE 'queued' END,
        locked_by = NULL,
        locked_until = NULL,
        run_after = $4,
        last_error = $3,
        updated_at = now()
    WHERE id = $1 AND status = 'running' AND locked_by = $2
    RETURNING *
),
letters AS (
    INSERT INTO legacy_sync_dead_letters (job_type, unique_key, entity_type, external_id, payload, last_error, attempts)
    SELECT job_type, unique_key, entity_type, external_id, payload, last_error, attempt
    FROM retried
    WHERE status = 'dead'
)
SELECT count(*) FROM retried
`, id, workerID, lastError, runAfter).Scan(&retried)
	if err != nil {
		return false, err
	}
	return retried == 1, nil
}
