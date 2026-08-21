-- +goose NO TRANSACTION
-- +goose Up

SET lock_timeout = '5s';

CREATE INDEX CONCURRENTLY IF NOT EXISTS legacy_sync_conflicts_open_entity_external_idx
    ON legacy_sync_conflicts (entity_type, external_id)
    WHERE status = 'open';

CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_room_range_idx
    ON sessions USING GIST (room_id, time_range)
    WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_teacher_range_idx
    ON sessions USING GIST (teacher_id, time_range)
    WHERE deleted_at IS NULL;

-- +goose Down

SET lock_timeout = '5s';

DROP INDEX CONCURRENTLY IF EXISTS sessions_active_teacher_range_idx;
DROP INDEX CONCURRENTLY IF EXISTS sessions_active_room_range_idx;
DROP INDEX CONCURRENTLY IF EXISTS legacy_sync_conflicts_open_entity_external_idx;
