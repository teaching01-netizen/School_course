-- name: SessionCreate :one
INSERT INTO sessions (series_id, course_id, room_id, teacher_id, start_at, end_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, series_id, course_id, room_id, teacher_id, start_at, end_at, version, deleted_at, created_at, updated_at;

-- name: SessionGetByID :one
SELECT id, series_id, course_id, room_id, teacher_id, start_at, end_at, version, deleted_at, created_at, updated_at
FROM sessions
WHERE id = $1;

-- name: SessionGetByIDForSnapshot :one
SELECT s.id, s.series_id, s.course_id, s.room_id, s.teacher_id,
       s.start_at, s.end_at, s.version,
       COALESCE(c.code, '') AS course_code,
       COALESCE(c.name, '') AS course_name,
       COALESCE(subj.name, '') AS subject_name,
       COALESCE(NULLIF(u.full_name, ''), u.username, '') AS teacher_name,
       r.name AS room_name
FROM sessions s
JOIN courses c ON c.id = s.course_id
LEFT JOIN subjects subj ON subj.id = c.subject_id
JOIN users u ON u.id = s.teacher_id
LEFT JOIN rooms r ON r.id = s.room_id
WHERE s.id = $1;

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

-- name: SessionListActiveByCourse :many
SELECT id, series_id, course_id, room_id, teacher_id, start_at, end_at, version, deleted_at, created_at, updated_at
FROM sessions
WHERE deleted_at IS NULL
  AND course_id = $1
ORDER BY start_at ASC;

-- name: SessionHardDelete :one
DELETE FROM sessions
WHERE id = $1 AND version = $2
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

-- name: SessionHardDeleteFutureBySeries :exec
DELETE FROM sessions
WHERE series_id = $1
  AND start_at >= $2;

-- name: SessionHardDeleteFutureBySeriesCount :one
WITH del AS (
  DELETE FROM sessions
  WHERE series_id = $1
    AND start_at >= $2
  RETURNING 1
)
SELECT count(*)::int4 AS canceled
FROM del;

-- name: SessionSoftDeleteFutureBySeriesCount :one
WITH soft_deleted AS (
  UPDATE sessions
  SET deleted_at = COALESCE(deleted_at, now()),
      updated_at = now(),
      version = version + 1
  WHERE series_id = $1
    AND start_at >= $2
    AND deleted_at IS NULL
  RETURNING 1
)
SELECT count(*)::int4 AS canceled
FROM soft_deleted;

-- name: SessionReparentFutureBySeries :one
WITH moved AS (
  UPDATE sessions
  SET series_id = sqlc.arg(new_series_id),
      updated_at = now(),
      version = version + 1
  WHERE series_id = sqlc.arg(old_series_id)
    AND start_at >= sqlc.arg(start_at)
  RETURNING 1
)
SELECT count(*)::int4 AS moved
FROM moved;

-- name: SessionCountActiveBeforeSeriesPivot :one
SELECT count(*)::int4
FROM sessions
WHERE series_id = $1
  AND deleted_at IS NULL
  AND start_at < $2;

-- name: SessionFindActiveSeriesPivot :one
SELECT s.id, s.series_id, s.course_id, s.room_id, s.teacher_id,
       s.start_at, s.end_at, s.version, s.deleted_at, s.created_at, s.updated_at,
       s.start_at > transaction_timestamp() AS is_future
FROM sessions s
JOIN session_series ss ON ss.id = s.series_id
WHERE s.series_id = $1
  AND s.deleted_at IS NULL
  AND (s.start_at AT TIME ZONE ss.institute_tz)::date = $2::date
ORDER BY s.start_at, s.id
LIMIT 1;

-- name: SessionListActiveIDsForSeriesFrom :many
SELECT id
FROM sessions
WHERE series_id = $1
  AND deleted_at IS NULL
  AND start_at >= $2
ORDER BY id;


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
