-- name: SeriesCreate :one
INSERT INTO session_series (
  course_id, room_id, teacher_id,
  institute_tz, weekdays, start_local_time, duration_minutes,
  start_date, end_date, count
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id, course_id, room_id, teacher_id, institute_tz, weekdays, start_local_time, duration_minutes, start_date, end_date, count, version, deleted_at, created_at, updated_at;

-- name: SeriesGetByID :one
SELECT id, course_id, room_id, teacher_id, institute_tz, weekdays, start_local_time, duration_minutes, start_date, end_date, count, version, deleted_at, created_at, updated_at
FROM session_series
WHERE id = $1;

-- name: SeriesGetByIDForUpdate :one
SELECT id, course_id, room_id, teacher_id, institute_tz, weekdays, start_local_time, duration_minutes, start_date, end_date, count, version, deleted_at, created_at, updated_at
FROM session_series
WHERE id = $1
FOR UPDATE;

-- name: SeriesUpdateEndDate :exec
UPDATE session_series
SET end_date = $2, updated_at = now(), version = version + 1
WHERE id = $1 AND version = $3;

-- name: SeriesUpdateCount :exec
UPDATE session_series
SET count = $2, updated_at = now(), version = version + 1
WHERE id = $1 AND version = $3;

-- name: SeriesUpdateFields :one
UPDATE session_series
SET course_id = $2,
    room_id = $3,
    teacher_id = $4,
    weekdays = $5,
    start_local_time = $6,
    duration_minutes = $7,
    end_date = $8,
    count = $9,
    updated_at = now(),
    version = version + 1
WHERE id = $1 AND version = $10
RETURNING id, course_id, room_id, teacher_id, institute_tz, weekdays, start_local_time, duration_minutes, start_date, end_date, count, version, deleted_at, created_at, updated_at;
