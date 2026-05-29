-- name: SubjectCreate :one
INSERT INTO subjects (code, name)
VALUES ($1, $2)
RETURNING id, code, name, deleted_at, created_at, updated_at;

-- name: SubjectGetByID :one
SELECT id, code, name, deleted_at, created_at, updated_at
FROM subjects
WHERE id = $1;

-- name: SubjectListActive :many
SELECT id, code, name, deleted_at, created_at, updated_at
FROM subjects
WHERE deleted_at IS NULL
ORDER BY code ASC;

-- name: SubjectSoftDelete :exec
UPDATE subjects
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

