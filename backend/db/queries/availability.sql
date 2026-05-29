-- Teacher availability

-- name: ListTeacherAvailability :many
SELECT id, teacher_id, start_at, end_at, created_at, updated_at
FROM teacher_availability
WHERE teacher_id = $1
  AND deleted_at IS NULL
ORDER BY start_at;

-- name: ListTeacherAvailabilityByRange :many
SELECT id, teacher_id, start_at, end_at, created_at, updated_at
FROM teacher_availability
WHERE teacher_id = $1
  AND deleted_at IS NULL
  AND time_range && tstzrange($2, $3, '[)')
ORDER BY start_at;

-- name: CreateTeacherAvailability :one
INSERT INTO teacher_availability (teacher_id, start_at, end_at)
VALUES ($1, $2, $3)
RETURNING id, teacher_id, start_at, end_at, created_at, updated_at;

-- name: SoftDeleteTeacherAvailability :exec
UPDATE teacher_availability
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND teacher_id = $2 AND deleted_at IS NULL;

-- Room availability

-- name: ListRoomAvailability :many
SELECT id, room_id, start_at, end_at, created_at, updated_at
FROM room_availability
WHERE room_id = $1
  AND deleted_at IS NULL
ORDER BY start_at;

-- name: ListRoomAvailabilityByRange :many
SELECT id, room_id, start_at, end_at, created_at, updated_at
FROM room_availability
WHERE room_id = $1
  AND deleted_at IS NULL
  AND time_range && tstzrange($2, $3, '[)')
ORDER BY start_at;

-- name: CreateRoomAvailability :one
INSERT INTO room_availability (room_id, start_at, end_at)
VALUES ($1, $2, $3)
RETURNING id, room_id, start_at, end_at, created_at, updated_at;

-- name: SoftDeleteRoomAvailability :exec
UPDATE room_availability
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND room_id = $2 AND deleted_at IS NULL;

-- name: CheckTeacherAvailability :one
SELECT EXISTS (
    SELECT 1 FROM teacher_availability a
    WHERE a.teacher_id = $1 AND a.deleted_at IS NULL
) AS has_windows,
EXISTS (
    SELECT 1 FROM teacher_availability a
    WHERE a.teacher_id = $1
      AND a.deleted_at IS NULL
      AND a.time_range @> tstzrange($2::timestamptz, $3::timestamptz, '[)')
) AS is_available;

-- name: CheckRoomAvailability :one
SELECT EXISTS (
    SELECT 1 FROM room_availability a
    WHERE a.room_id = $1 AND a.deleted_at IS NULL
) AS has_windows,
EXISTS (
    SELECT 1 FROM room_availability a
    WHERE a.room_id = $1
      AND a.deleted_at IS NULL
      AND a.time_range @> tstzrange($2::timestamptz, $3::timestamptz, '[)')
) AS is_available;

