-- name: StudentCreate :one
INSERT INTO students (wcode, full_name, notes)
VALUES ($1, $2, $3)
ON CONFLICT (wcode) DO UPDATE
SET full_name = EXCLUDED.full_name,
    notes = CASE WHEN EXCLUDED.notes = '' THEN students.notes ELSE EXCLUDED.notes END,
    updated_at = now()
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
  AND ($2 = (SELECT s2.wcode FROM students s2 WHERE s2.id = $1)
       OR NOT EXISTS (SELECT 1 FROM students s3 WHERE s3.wcode = $2 AND s3.id <> $1))
RETURNING id, wcode, full_name, notes, created_at, updated_at;

-- name: StudentUpsertNameByWCode :one
INSERT INTO students (wcode, full_name, notes)
VALUES ($1, $2, '')
ON CONFLICT (wcode) DO UPDATE
SET full_name = EXCLUDED.full_name,
    updated_at = now()
RETURNING id, wcode, full_name, notes, created_at, updated_at;

-- name: StudentFindDuplicates :many
SELECT wcode, COUNT(*) AS cnt
FROM students
GROUP BY wcode
HAVING COUNT(*) > 1
ORDER BY cnt DESC;

