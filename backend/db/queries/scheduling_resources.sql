-- Scheduling resource validation queries for the scheduling domain service.
-- Validates all referenced resources (course, teacher, room) in one round trip.

-- name: SchedulingResourcesGet :one
-- Returns the complete resource validation state: whether the course, teacher,
-- and room exist. Used by the advisory preflight to surface explicit resource
-- errors before availability/overlap checks.
SELECT
    EXISTS (SELECT 1 FROM courses WHERE courses.id = sqlc.arg(course_id)) AS course_exists,
    EXISTS (SELECT 1 FROM users WHERE users.id = sqlc.arg(teacher_id) AND users.deleted_at IS NULL) AS teacher_exists,
    EXISTS (SELECT 1 FROM users WHERE users.id = sqlc.arg(teacher_id) AND users.deleted_at IS NULL AND users.role IN ('Admin', 'Teacher')) AS teacher_active,
    EXISTS (SELECT 1 FROM rooms WHERE rooms.id = sqlc.arg(room_id)) AS room_exists;
