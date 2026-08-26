-- +goose NO TRANSACTION
-- +goose Up

SET lock_timeout = '5s';

CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_native_occurrence_idx
    ON sessions (course_id, teacher_id, start_at, end_at, room_id)
    WHERE deleted_at IS NULL AND source_kind = 'native';

CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_conflict_override_idx
    ON sessions (id)
    WHERE deleted_at IS NULL AND (conflict_override OR legacy_conflict_override);

CREATE INDEX CONCURRENTLY IF NOT EXISTS student_busy_ranges_active_student_range_idx
    ON student_busy_ranges USING GIST (student_id, time_range)
    WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS student_busy_ranges_active_conflict_override_idx
    ON student_busy_ranges (session_id, student_id)
    WHERE deleted_at IS NULL AND conflict_override;

CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_start_idx
    ON sessions (start_at DESC, id)
    WHERE deleted_at IS NULL;

-- +goose Down
SET lock_timeout = '5s';

DROP INDEX CONCURRENTLY IF EXISTS sessions_active_start_idx;
DROP INDEX CONCURRENTLY IF EXISTS student_busy_ranges_active_conflict_override_idx;
DROP INDEX CONCURRENTLY IF EXISTS student_busy_ranges_active_student_range_idx;
DROP INDEX CONCURRENTLY IF EXISTS sessions_active_conflict_override_idx;
DROP INDEX CONCURRENTLY IF EXISTS sessions_active_native_occurrence_idx;
