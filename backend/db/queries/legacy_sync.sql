-- name: ExternalRefGet :one
SELECT * FROM external_refs
WHERE source = $1 AND entity_type = $2 AND external_id = $3;

-- name: ExternalRefUpsert :one
INSERT INTO external_refs (source, entity_type, external_id, internal_id, source_hash, state)
VALUES ($1, $2, $3, $4, $5, 'active')
ON CONFLICT (source, entity_type, external_id)
DO UPDATE SET
    internal_id = EXCLUDED.internal_id,
    source_hash = EXCLUDED.source_hash,
    last_seen_at = now(),
    -- Reapplying a tombstoned reference reactivates it: the source row is
    -- back, so the mapping must not stay in a missing/tombstoned state.
    state = 'active'
RETURNING *;

-- name: ExternalRefListByInternalID :many
SELECT * FROM external_refs
WHERE internal_id = $1
ORDER BY source, entity_type, external_id;

-- name: ExternalRefTouchSeen :exec
UPDATE external_refs
SET last_seen_at = now()
WHERE source = $1 AND entity_type = $2 AND external_id = $3;

-- name: ExternalRefSetState :exec
UPDATE external_refs
SET state = $4
WHERE source = $1 AND entity_type = $2 AND external_id = $3;

-- name: ChangeEventInsert :one
INSERT INTO legacy_change_events (source_event_key, detector, entity_type, external_id, action, observed_at, raw_payload)
VALUES (sqlc.arg(source_event_key), sqlc.arg(detector), sqlc.arg(entity_type), sqlc.arg(external_id), sqlc.arg(action), sqlc.arg(observed_at), sqlc.arg(raw_payload)::text::jsonb)
RETURNING *;

-- name: SnapshotUpsert :one
INSERT INTO legacy_entity_snapshots
    (source, entity_type, external_id, canonical_data, source_hash, parser_version, observed_at, applied_at, quality)
VALUES (sqlc.arg(source), sqlc.arg(entity_type), sqlc.arg(external_id), sqlc.arg(canonical_data)::text::jsonb, sqlc.arg(source_hash), sqlc.arg(parser_version), sqlc.arg(observed_at), sqlc.arg(applied_at), sqlc.arg(quality))
ON CONFLICT (source, entity_type, external_id)
DO UPDATE SET
    canonical_data = EXCLUDED.canonical_data,
    source_hash    = EXCLUDED.source_hash,
    parser_version = EXCLUDED.parser_version,
    observed_at    = EXCLUDED.observed_at,
    applied_at     = EXCLUDED.applied_at,
    quality        = EXCLUDED.quality
RETURNING *;

-- name: SnapshotGet :one
SELECT * FROM legacy_entity_snapshots
WHERE source = $1 AND entity_type = $2 AND external_id = $3;

-- name: SyncRunCreate :one
INSERT INTO legacy_sync_runs (mode)
VALUES ($1)
RETURNING *;

-- name: SyncRunComplete :exec
UPDATE legacy_sync_runs
SET status = $2,
    completed_at = now(),
    pages_requested = $3,
    entities_parsed = $4,
    entities_changed = $5,
    entities_applied = $6,
    parse_failures = $7,
    reconciliation_mismatches = $8,
    source_latency_ms = $9,
    last_error = $10
WHERE id = $1;

-- name: SyncRunListRecent :many
SELECT * FROM legacy_sync_runs
ORDER BY started_at DESC
LIMIT $1;

-- name: ConflictInsert :one
INSERT INTO legacy_sync_conflicts (entity_type, external_id, conflict_type, category, source_payload, local_payload, message)
SELECT sqlc.arg(entity_type), sqlc.arg(external_id), sqlc.arg(conflict_type), sqlc.arg(category), sqlc.arg(source_payload)::text::jsonb, sqlc.arg(local_payload)::text::jsonb, sqlc.arg(message)
WHERE NOT EXISTS (
    SELECT 1 FROM legacy_sync_conflicts
    WHERE entity_type = sqlc.arg(entity_type) AND external_id = sqlc.arg(external_id)
      AND conflict_type = sqlc.arg(conflict_type) AND status = 'open'
)
RETURNING *;

-- name: ConflictCountOpen :one
SELECT count(*)::int FROM legacy_sync_conflicts
WHERE status = 'open';

-- name: ConflictListOpen :many
SELECT * FROM legacy_sync_conflicts
WHERE status = 'open'
ORDER BY created_at DESC;

-- name: ConflictListOpenPaginated :many
SELECT id, entity_type, external_id, conflict_type, category, message, status, created_at, resolved_at
FROM legacy_sync_conflicts
WHERE status = 'open'
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ConflictGet :one
SELECT * FROM legacy_sync_conflicts
WHERE id = $1;

-- name: SyncRunGetLatest :one
SELECT * FROM legacy_sync_runs
ORDER BY started_at DESC
LIMIT 1;

-- name: ConflictSetStatus :one
UPDATE legacy_sync_conflicts
SET status = $2, resolved_at = now()
WHERE id = $1 AND status = 'open'
RETURNING *;

-- name: DeadLetterInsert :one
INSERT INTO legacy_sync_dead_letters (job_type, unique_key, entity_type, external_id, payload, error_category, last_error, attempts)
VALUES (sqlc.arg(job_type), sqlc.arg(unique_key), sqlc.arg(entity_type), sqlc.arg(external_id), sqlc.arg(payload)::text::jsonb, sqlc.arg(error_category), sqlc.arg(last_error), sqlc.arg(attempts))
RETURNING *;

-- name: OutboxInsert :one
INSERT INTO legacy_sync_outbox (source_event_key, event_type, channel, entity_type, external_id, payload)
VALUES (sqlc.arg(source_event_key), sqlc.arg(event_type), sqlc.arg(channel), sqlc.arg(entity_type), sqlc.arg(external_id), sqlc.arg(payload)::text::jsonb)
ON CONFLICT (source_event_key)
DO UPDATE SET
    event_type  = EXCLUDED.event_type,
    channel     = EXCLUDED.channel,
    entity_type = EXCLUDED.entity_type,
    external_id = EXCLUDED.external_id,
    payload     = EXCLUDED.payload,
    status      = 'pending',
    published_at = NULL,
    last_error   = NULL,
    claimed_at   = NULL,
    claim_until  = NULL
RETURNING *;

-- name: LegacyJobEnqueue :one
INSERT INTO legacy_sync_jobs
    (job_type, entity_type, external_id, payload, unique_key, priority, deadline_at, max_attempts, run_after)
VALUES (sqlc.arg(job_type), sqlc.arg(entity_type), sqlc.arg(external_id), sqlc.arg(payload)::text::jsonb,
        sqlc.arg(unique_key), sqlc.arg(priority), sqlc.arg(deadline_at), sqlc.arg(max_attempts), sqlc.arg(run_after))
ON CONFLICT (unique_key) WHERE unique_key IS NOT NULL AND status IN ('queued','running')
DO UPDATE SET
    priority = LEAST(legacy_sync_jobs.priority, EXCLUDED.priority),
    run_after = LEAST(legacy_sync_jobs.run_after, EXCLUDED.run_after),
    updated_at = now()
RETURNING *;

-- name: LegacyJobClaim :one
WITH candidate AS (
    SELECT j.id
    FROM legacy_sync_jobs j
    WHERE (j.status = 'queued' AND j.run_after <= now())
       OR (j.status = 'running' AND j.locked_until < now())
    ORDER BY CASE WHEN j.job_type = 'legacy_refresh_course'
                       AND EXISTS (
                           SELECT 1
                           FROM courses c
                           JOIN subject_active_courses sac
                             ON sac.subject_id = c.subject_id AND sac.course_id = c.id
                           WHERE c.legacy_course_id = j.external_id
                             AND c.absence_form_visible
                       )
                  THEN 0 ELSE 1 END,
             j.priority ASC,
             j.created_at ASC
    FOR UPDATE OF j SKIP LOCKED
    LIMIT 1
)
UPDATE legacy_sync_jobs j
SET status = 'running',
    locked_by = sqlc.arg(worker_id),
    locked_until = now() + (sqlc.arg(lease_seconds) * interval '1 second'),
    heartbeat_at = now(),
    attempt = j.attempt + 1,
    updated_at = now()
FROM candidate
WHERE j.id = candidate.id
RETURNING j.*;

-- name: LegacyJobHeartbeat :exec
UPDATE legacy_sync_jobs
SET locked_until = now() + (sqlc.arg(lease_seconds) * interval '1 second'),
    heartbeat_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'running' AND locked_by = sqlc.arg(worker_id);

-- name: LegacyJobComplete :exec
UPDATE legacy_sync_jobs
SET status = 'completed', locked_by = NULL, locked_until = NULL, updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'running' AND locked_by = sqlc.arg(worker_id);

-- name: LegacyJobListRecent :many
SELECT * FROM legacy_sync_jobs
ORDER BY created_at DESC
LIMIT $1;

-- name: LegacyJobListPaginated :many
SELECT * FROM legacy_sync_jobs
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: LegacyJobCounts :one
SELECT
    count(*) FILTER (WHERE status = 'queued')::int AS queued,
    count(*) FILTER (WHERE status = 'running')::int AS running,
    count(*) FILTER (WHERE status = 'completed')::int AS completed,
    count(*) FILTER (WHERE status = 'dead')::int AS dead
FROM legacy_sync_jobs;

-- name: LegacySyncControlGet :one
SELECT * FROM legacy_sync_controls
WHERE id = true;

-- name: LegacySyncControlSet :one
UPDATE legacy_sync_controls
SET detection_enabled = COALESCE(sqlc.narg(detection_enabled), detection_enabled),
    fetch_enabled = COALESCE(sqlc.narg(fetch_enabled), fetch_enabled),
    apply_enabled = COALESCE(sqlc.narg(apply_enabled), apply_enabled),
    student_enabled = COALESCE(sqlc.narg(student_enabled), student_enabled),
    tombstone_enabled = COALESCE(sqlc.narg(tombstone_enabled), tombstone_enabled),
    realtime_enabled = COALESCE(sqlc.narg(realtime_enabled), realtime_enabled),
    shadow_mode = COALESCE(sqlc.narg(shadow_mode), shadow_mode),
    updated_at = now()
WHERE id = true
RETURNING *;
