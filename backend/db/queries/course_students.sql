-- name: CourseStudentAdd :exec
INSERT INTO course_students (course_id, student_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: CourseStudentRemove :exec
DELETE FROM course_students
WHERE course_id = $1 AND student_id = $2;

-- name: CourseStudentsList :many
SELECT student_id, created_at
FROM course_students
WHERE course_id = $1
ORDER BY created_at ASC;

-- name: CourseStudentsListDetailed :many
SELECT s.id, s.wcode, s.full_name, s.notes, s.created_at, s.updated_at
FROM course_students cs
JOIN students s ON s.id = cs.student_id
WHERE cs.course_id = $1
ORDER BY s.wcode ASC;

-- name: CourseStudentAddDraft :exec
INSERT INTO course_students (course_id, student_id, status)
VALUES ($1, $2, 'draft')
ON CONFLICT (course_id, student_id) DO NOTHING;

-- name: CourseStudentUpdateStatus :exec
UPDATE course_students
SET status = $3
WHERE course_id = $1 AND student_id = $2;

-- name: CourseStudentUpdateStatusRow :execrows
UPDATE course_students
SET status = @new_status
WHERE course_id = $1 AND student_id = $2 AND status = @old_status;

-- name: CourseStudentsListDetailedWithStatus :many
SELECT s.id, s.wcode, s.full_name, s.notes, s.created_at, s.updated_at,
       cs.status
FROM course_students cs
JOIN students s ON s.id = cs.student_id
WHERE cs.course_id = $1
ORDER BY s.wcode ASC;

-- name: CourseStudentRemoveDraftIfStatus :execrows
DELETE FROM course_students
WHERE course_id = $1 AND student_id = $2 AND status = 'draft';

-- name: CourseStudentRemoveDraftsByIDs :execrows
DELETE FROM course_students
WHERE course_id = $1 AND student_id = ANY($2::uuid[]) AND status = 'draft';
