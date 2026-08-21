-- +goose NO TRANSACTION
-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_course_end_idx
  ON sessions(course_id, end_at)
  WHERE deleted_at IS NULL;

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS sessions_active_course_end_idx;
