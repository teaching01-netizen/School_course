-- +goose NO TRANSACTION
-- +goose Up
-- Case-insensitive wcode lookups serve every sessions-in-range read
-- (StudentGetByWCode, scope day counts, blocked sit-ins) via
-- lower(wcode) = lower($1), which cannot use the plain btree indexes.
-- Functional indexes make those lookups index scans at any table size.
-- See sessions_range_scope_facts.go (SessionsRangeDayCounts).
CREATE INDEX CONCURRENTLY IF NOT EXISTS students_lower_wcode_idx ON students (lower(wcode));
CREATE INDEX CONCURRENTLY IF NOT EXISTS student_absences_lower_wcode_idx ON student_absences (lower(wcode));

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS student_absences_lower_wcode_idx;
DROP INDEX CONCURRENTLY IF EXISTS students_lower_wcode_idx;
