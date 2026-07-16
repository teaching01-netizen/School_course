-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_series_start_idx ON sessions(series_id, start_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_course_start_idx ON sessions(course_id, start_at) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS student_busy_ranges_session_idx ON student_busy_ranges(session_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_time_range_idx ON sessions USING gist(time_range) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS teacher_availability_active_range_idx ON teacher_availability USING gist(teacher_id, time_range) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS room_availability_active_range_idx ON room_availability USING gist(room_id, time_range) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS room_availability_active_range_idx;
DROP INDEX CONCURRENTLY IF EXISTS teacher_availability_active_range_idx;
DROP INDEX CONCURRENTLY IF EXISTS sessions_active_time_range_idx;
DROP INDEX CONCURRENTLY IF EXISTS student_busy_ranges_session_idx;
DROP INDEX CONCURRENTLY IF EXISTS sessions_active_course_start_idx;
DROP INDEX CONCURRENTLY IF EXISTS sessions_series_start_idx;
