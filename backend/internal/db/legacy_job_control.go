package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

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

// LegacyJobRetryOwned re-queues (or dead-letters) a running job, but only when
// the caller still holds the lease. The UPDATE row count is the ownership
// signal, so the dead-letter insert shares the statement: an exhausted job is
// mirrored into legacy_sync_dead_letters atomically with the status change.
func (q *Queries) LegacyJobRetryOwned(ctx context.Context, id pgtype.UUID, workerID pgtype.Text, lastError string) (bool, error) {
	var retried int64
	err := q.db.QueryRow(ctx, `
WITH retried AS (
    UPDATE legacy_sync_jobs
    SET status = CASE WHEN attempt >= max_attempts THEN 'dead' ELSE 'queued' END,
        locked_by = NULL,
        locked_until = NULL,
        run_after = now() + (GREATEST(attempt, 1) * interval '1 second'),
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
`, id, workerID, lastError).Scan(&retried)
	if err != nil {
		return false, err
	}
	return retried == 1, nil
}
