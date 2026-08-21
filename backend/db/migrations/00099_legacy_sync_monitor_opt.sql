-- +goose Up
-- +goose NO TRANSACTION

-- Optimal read path for the legacy sync monitor when thousands of open
-- conflicts and dead letters pile up: health must count without shipping JSONB,
-- list pages must paginate via index-only scans.

CREATE INDEX CONCURRENTLY IF NOT EXISTS legacy_sync_conflicts_open_created_idx
    ON legacy_sync_conflicts (created_at DESC) WHERE status = 'open';

CREATE INDEX CONCURRENTLY IF NOT EXISTS legacy_sync_dead_letters_course_created_idx
    ON legacy_sync_dead_letters (created_at DESC) WHERE entity_type = 'course';

CREATE INDEX CONCURRENTLY IF NOT EXISTS legacy_sync_conflicts_schedule_id_idx
    ON legacy_sync_conflicts ((source_payload->>'legacy_schedule_id'))
    WHERE source_payload->>'legacy_schedule_id' IS NOT NULL;

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS legacy_sync_conflicts_schedule_id_idx;
DROP INDEX CONCURRENTLY IF EXISTS legacy_sync_dead_letters_course_created_idx;
DROP INDEX CONCURRENTLY IF EXISTS legacy_sync_conflicts_open_created_idx;
