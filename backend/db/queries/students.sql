-- name: StudentCreate :one
INSERT INTO students (wcode, full_name, notes, nickname, school, level, year, student_phone, email)
VALUES (LOWER(BTRIM(@wcode)), @full_name, @notes, @nickname, @school, @level, @year, @student_phone, @email)
ON CONFLICT (LOWER(wcode)) DO UPDATE
SET full_name = EXCLUDED.full_name,
    notes = CASE WHEN EXCLUDED.notes = '' THEN students.notes ELSE EXCLUDED.notes END,
    nickname = CASE WHEN btrim(COALESCE(students.nickname, '')) = '' THEN EXCLUDED.nickname ELSE students.nickname END,
    school = CASE WHEN btrim(COALESCE(students.school, '')) = '' THEN EXCLUDED.school ELSE students.school END,
    level = CASE WHEN btrim(COALESCE(students.level, '')) = '' THEN EXCLUDED.level ELSE students.level END,
    year = CASE WHEN btrim(COALESCE(students.year, '')) = '' THEN EXCLUDED.year ELSE students.year END,
    student_phone = CASE WHEN btrim(COALESCE(students.student_phone, '')) = '' THEN EXCLUDED.student_phone ELSE students.student_phone END,
    email = CASE WHEN btrim(COALESCE(students.email, '')) = '' THEN EXCLUDED.email ELSE students.email END,
    updated_at = now()
RETURNING id, wcode, full_name, notes, nickname, email, student_phone, parent_phone, email_crm, email_system, school, level, year, created_at, updated_at;

-- name: StudentImportByWCode :one
INSERT INTO students (wcode, full_name, nickname)
VALUES (LOWER(BTRIM(@wcode)), @full_name, @nickname)
ON CONFLICT DO NOTHING
RETURNING id, wcode;

-- name: StudentGetByID :one
SELECT id, wcode, full_name, notes, nickname, email, student_phone, parent_phone, email_crm, email_system, school, level, year, created_at, updated_at
FROM students
WHERE id = $1;

-- name: StudentGetByWCode :one
SELECT id, wcode, full_name, notes, nickname, email, student_phone, parent_phone, email_crm, email_system, school, level, year, created_at, updated_at
FROM students
WHERE lower(wcode) = lower(@wcode);

-- name: StudentList :many
SELECT id, wcode, full_name, notes, nickname, email, student_phone, parent_phone, email_crm, email_system, school, level, year, created_at, updated_at
FROM students
WHERE (wcode ILIKE '%' || $3 || '%' OR full_name ILIKE '%' || $3 || '%' OR $3 = '')
ORDER BY wcode ASC
LIMIT $1 OFFSET $2;

-- name: StudentListCount :one
SELECT count(*) FROM students
WHERE (wcode ILIKE '%' || $1 || '%' OR full_name ILIKE '%' || $1 || '%' OR $1 = '');

-- name: StudentUpdate :one
UPDATE students
SET wcode = LOWER(BTRIM(@wcode)), full_name = @full_name, notes = @notes,
    nickname = @nickname, email = @email, student_phone = @student_phone, parent_phone = @parent_phone,
    school = @school, level = @level, year = @year, updated_at = now()
WHERE students.id = @id
  AND (LOWER(BTRIM(@wcode)) = (SELECT LOWER(BTRIM(s2.wcode)) FROM students s2 WHERE s2.id = @id)
       OR NOT EXISTS (SELECT 1 FROM students s3 WHERE LOWER(BTRIM(s3.wcode)) = LOWER(BTRIM(@wcode)) AND s3.id <> @id))
RETURNING students.id, students.wcode, students.full_name, students.notes, students.nickname, students.email, students.student_phone, students.parent_phone, students.email_crm, students.email_system, students.school, students.level, students.year, students.created_at, students.updated_at;

-- name: StudentProfileUpsert :one
INSERT INTO students (wcode, full_name, nickname, school, level, year, student_phone, email)
VALUES (LOWER(BTRIM(@wcode)), @full_name, @nickname, @school, @level, @year, @student_phone, @email)
ON CONFLICT (LOWER(wcode)) DO UPDATE
SET full_name = CASE WHEN btrim(COALESCE(students.full_name, '')) = '' THEN EXCLUDED.full_name ELSE students.full_name END,
    nickname = CASE WHEN btrim(COALESCE(students.nickname, '')) = '' THEN EXCLUDED.nickname ELSE students.nickname END,
    school = CASE WHEN btrim(COALESCE(students.school, '')) = '' THEN EXCLUDED.school ELSE students.school END,
    level = CASE WHEN btrim(COALESCE(students.level, '')) = '' THEN EXCLUDED.level ELSE students.level END,
    year = CASE WHEN btrim(COALESCE(students.year, '')) = '' THEN EXCLUDED.year ELSE students.year END,
    student_phone = CASE WHEN btrim(COALESCE(students.student_phone, '')) = '' THEN EXCLUDED.student_phone ELSE students.student_phone END,
    email = CASE WHEN btrim(COALESCE(students.email, '')) = '' THEN EXCLUDED.email ELSE students.email END,
    updated_at = now()
WHERE btrim(COALESCE(students.full_name, '')) = '' AND NULLIF(EXCLUDED.full_name, '') IS NOT NULL
   OR btrim(COALESCE(students.nickname, '')) = '' AND NULLIF(EXCLUDED.nickname, '') IS NOT NULL
   OR btrim(COALESCE(students.school, '')) = '' AND NULLIF(EXCLUDED.school, '') IS NOT NULL
   OR btrim(COALESCE(students.level, '')) = '' AND NULLIF(EXCLUDED.level, '') IS NOT NULL
   OR btrim(COALESCE(students.year, '')) = '' AND NULLIF(EXCLUDED.year, '') IS NOT NULL
   OR btrim(COALESCE(students.student_phone, '')) = '' AND NULLIF(EXCLUDED.student_phone, '') IS NOT NULL
   OR btrim(COALESCE(students.email, '')) = '' AND NULLIF(EXCLUDED.email, '') IS NOT NULL
RETURNING id;

-- name: StudentUpsertNameByWCode :one
INSERT INTO students (wcode, full_name, notes)
VALUES (LOWER(BTRIM(@wcode)), @full_name, '')
ON CONFLICT (LOWER(wcode)) DO UPDATE
SET full_name = EXCLUDED.full_name,
    updated_at = now()
RETURNING id, wcode, full_name, notes, nickname, email, student_phone, parent_phone, email_crm, email_system, school, level, year, created_at, updated_at;

-- name: StudentFindDuplicates :many
SELECT LOWER(BTRIM(wcode)) AS wcode, COUNT(*) AS cnt
FROM students
GROUP BY LOWER(BTRIM(wcode))
HAVING COUNT(*) > 1
ORDER BY cnt DESC;
