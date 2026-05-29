-- name: CourseCreate :one
INSERT INTO courses (code, name)
VALUES ($1, $2)
RETURNING id, code, name, deleted_at, created_at, updated_at;

-- name: CourseGetByID :one
SELECT id, code, name, deleted_at, created_at, updated_at
FROM courses
WHERE id = $1;

-- name: CourseListActive :many
SELECT id, code, name, deleted_at, created_at, updated_at
FROM courses
WHERE deleted_at IS NULL
ORDER BY code ASC;

-- name: CourseUpdate :one
UPDATE courses
SET code = $2, name = $3, updated_at = now()
WHERE id = $1
RETURNING id, code, name, deleted_at, created_at, updated_at;

-- name: CourseSoftDelete :exec
UPDATE courses
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: CourseRestore :exec
UPDATE courses
SET deleted_at = NULL, updated_at = now()
WHERE id = $1 AND deleted_at IS NOT NULL;

