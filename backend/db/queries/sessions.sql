-- name: SessionCreate :one
INSERT INTO sessions (series_id, course_id, room_id, teacher_id, start_at, end_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, series_id, course_id, room_id, teacher_id, start_at, end_at, version, deleted_at, created_at, updated_at;

-- name: SessionGetByID :one
SELECT id, series_id, course_id, room_id, teacher_id, start_at, end_at, version, deleted_at, created_at, updated_at
FROM sessions
WHERE id = $1;

-- name: SessionListByRange :many
SELECT id, series_id, course_id, room_id, teacher_id, start_at, end_at, version, deleted_at, created_at, updated_at
FROM sessions
WHERE start_at < @range_end AND end_at > @range_start
ORDER BY start_at ASC;

-- name: SessionListActiveByRange :many
SELECT id, series_id, course_id, room_id, teacher_id, start_at, end_at, version, deleted_at, created_at, updated_at
FROM sessions
WHERE deleted_at IS NULL
  AND start_at < @range_end
  AND end_at > @range_start
ORDER BY start_at ASC;

-- name: SessionSoftDelete :one
UPDATE sessions
SET deleted_at = now(), updated_at = now(), version = version + 1
WHERE id = $1 AND deleted_at IS NULL AND version = $2
RETURNING 1;

-- name: SessionUpdateTime :one
UPDATE sessions
SET start_at = $2, end_at = $3, updated_at = now(), version = version + 1
WHERE id = $1 AND version = $4
RETURNING id, series_id, course_id, room_id, teacher_id, start_at, end_at, version, deleted_at, created_at, updated_at;

-- name: SessionUpdateOccurrence :one
UPDATE sessions
SET course_id = $2,
    room_id = $3,
    teacher_id = $4,
    start_at = $5,
    end_at = $6,
    updated_at = now(),
    version = version + 1
WHERE id = $1 AND version = $7
RETURNING id, series_id, course_id, room_id, teacher_id, start_at, end_at, version, deleted_at, created_at, updated_at;

-- name: SessionAttendanceDeleteNotInCourse :exec
DELETE FROM session_attendance sa
WHERE sa.session_id = $1
  AND NOT EXISTS (
    SELECT 1
    FROM course_students cs
    WHERE cs.course_id = $2
      AND cs.student_id = sa.student_id
  );

-- name: SessionSoftDeleteFutureBySeries :exec
UPDATE sessions
SET deleted_at = now(), updated_at = now(), version = version + 1
WHERE series_id = $1
  AND deleted_at IS NULL
  AND start_at >= $2;

-- name: SessionSoftDeleteFutureBySeriesCount :one
WITH upd AS (
  UPDATE sessions
  SET deleted_at = now(), updated_at = now(), version = version + 1
  WHERE series_id = $1
    AND deleted_at IS NULL
    AND start_at >= $2
  RETURNING 1
)
SELECT count(*)::int4 AS canceled
FROM upd;


-- name: SessionAttendanceUpsert :exec
INSERT INTO session_attendance (session_id, student_id, status)
VALUES ($1, $2, $3)
ON CONFLICT (session_id, student_id) DO UPDATE SET status = EXCLUDED.status;

-- name: SessionAttendanceDelete :exec
DELETE FROM session_attendance
WHERE session_id = $1 AND student_id = $2;

-- name: SessionAttendanceList :many
SELECT session_id, student_id, status, created_at
FROM session_attendance
WHERE session_id = $1
ORDER BY created_at ASC;

-- name: SessionLockOverlappingForInsert :many
SELECT id FROM sessions
WHERE deleted_at IS NULL
  AND ((teacher_id = $1 AND time_range && tstzrange($2, $3, '[)'))
       OR (room_id = $4 AND time_range && tstzrange($2, $3, '[)')))
FOR UPDATE;

-- name: StudentBusyRangesLockOverlapping :many
SELECT id FROM student_busy_ranges
WHERE deleted_at IS NULL
  AND student_id = ANY($1::uuid[])
  AND time_range && tstzrange($2, $3, '[)')
FOR UPDATE;
