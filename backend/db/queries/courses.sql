-- name: CourseCreate :one
INSERT INTO courses (code, name)
VALUES ($1, $2)
RETURNING id, code, name, created_at, updated_at;

-- name: CourseGetByID :one
SELECT id, code, name, created_at, updated_at
FROM courses
WHERE id = $1;

-- name: CourseGetCoreByID :one
SELECT id, version, code, name, teacher_id
FROM courses
WHERE id = $1;

-- name: CourseListActive :many
SELECT c.id, c.code, c.name, COALESCE(s.name, '') AS subject_name, c.created_at, c.updated_at
FROM courses c
LEFT JOIN subjects s ON s.id = c.subject_id
ORDER BY c.code ASC;

-- name: CourseDelete :exec
DELETE FROM courses
WHERE id = $1;
