-- name: CourseCreate :one
INSERT INTO courses (code, name)
VALUES ($1, $2)
RETURNING id, code, name, created_at, updated_at;

-- name: CourseGetByID :one
SELECT id, code, name, created_at, updated_at
FROM courses
WHERE id = $1;

-- name: CourseListActive :many
SELECT id, code, name, created_at, updated_at
FROM courses
ORDER BY code ASC;

-- name: CourseUpdate :one
UPDATE courses
SET code = $2, name = $3, updated_at = now()
WHERE id = $1
RETURNING id, code, name, created_at, updated_at;

-- name: CourseDelete :exec
DELETE FROM courses
WHERE id = $1;
