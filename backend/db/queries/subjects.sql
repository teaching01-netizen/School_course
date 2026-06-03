-- name: SubjectCreate :one
INSERT INTO subjects (code, name)
VALUES ($1, $2)
RETURNING id, code, name, created_at, updated_at;

-- name: SubjectGetByID :one
SELECT id, code, name, created_at, updated_at
FROM subjects
WHERE id = $1;

-- name: SubjectListActive :many
SELECT id, code, name, created_at, updated_at
FROM subjects
ORDER BY code ASC;

-- name: SubjectDelete :exec
DELETE FROM subjects
WHERE id = $1;
