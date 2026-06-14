-- name: StudentCreate :one
INSERT INTO students (wcode, full_name, notes)
VALUES ($1, $2, $3)
RETURNING id, wcode, full_name, notes, created_at, updated_at;

-- name: StudentGetByID :one
SELECT id, wcode, full_name, notes, created_at, updated_at
FROM students
WHERE id = $1;

-- name: StudentGetByWCode :one
SELECT id, wcode, full_name, notes, created_at, updated_at
FROM students
WHERE wcode = $1;

-- name: StudentList :many
SELECT id, wcode, full_name, notes, created_at, updated_at
FROM students
WHERE (wcode ILIKE '%' || $3 || '%' OR full_name ILIKE '%' || $3 || '%' OR $3 = '')
ORDER BY wcode ASC
LIMIT $1 OFFSET $2;

-- name: StudentListCount :one
SELECT count(*) FROM students
WHERE (wcode ILIKE '%' || $1 || '%' OR full_name ILIKE '%' || $1 || '%' OR $1 = '');

-- name: StudentUpdate :one
UPDATE students
SET wcode = $2, full_name = $3, notes = $4, updated_at = now()
WHERE id = $1
RETURNING id, wcode, full_name, notes, created_at, updated_at;

-- name: StudentUpsertNameByWCode :one
INSERT INTO students (wcode, full_name, notes)
VALUES ($1, $2, '')
ON CONFLICT (wcode) DO UPDATE
SET full_name = EXCLUDED.full_name,
    updated_at = now()
RETURNING id, wcode, full_name, notes, created_at, updated_at;

