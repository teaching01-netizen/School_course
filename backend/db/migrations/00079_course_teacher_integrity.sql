-- +goose NO TRANSACTION
-- +goose Up

-- Course teacher integrity (PR1): purely additive schema foundation.
-- 1. courses.version enables optimistic concurrency for teacher edits.
-- 2. course_teachers.is_primary marks the single primary teacher of a course.
-- 3. ux_course_teachers_one_primary enforces at most one primary per course.

ALTER TABLE courses
    ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;

ALTER TABLE course_teachers
    ADD COLUMN IF NOT EXISTS is_primary boolean NOT NULL DEFAULT false;

-- Reset primaries so the backfill below is the single source of truth on re-run.
UPDATE course_teachers
SET is_primary = false
WHERE is_primary = true;

-- Mirror the legacy primary assignment (courses.teacher_id) into course_teachers.
-- Idempotent: ON CONFLICT DO UPDATE re-marks existing rows instead of duplicating.
INSERT INTO course_teachers (
    course_id,
    teacher_id,
    is_primary
)
SELECT
    c.id,
    c.teacher_id,
    true
FROM courses c
WHERE c.teacher_id IS NOT NULL
ON CONFLICT (course_id, teacher_id)
DO UPDATE SET is_primary = true;

-- At most one primary teacher per course; many non-primary teachers allowed.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS
    ux_course_teachers_one_primary
ON course_teachers (course_id)
WHERE is_primary = true;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS ux_course_teachers_one_primary;

ALTER TABLE courses
    DROP COLUMN IF EXISTS version;

ALTER TABLE course_teachers
    DROP COLUMN IF EXISTS is_primary;
