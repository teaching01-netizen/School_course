-- name: AbsenceCreate :one
INSERT INTO student_absences (wcode, course_id, date_from, date_to, reason, sit_in_course_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, wcode, course_id, date_from, date_to, reason, sit_in_course_id, created_at;

-- name: AbsenceList :many
SELECT sa.id, sa.wcode, sa.course_id, sa.date_from, sa.date_to, sa.reason, sa.sit_in_course_id, sa.created_at,
       c.code AS course_code, c.name AS course_name,
       sc.code AS sit_in_course_code, sc.name AS sit_in_course_name
FROM student_absences sa
JOIN courses c ON c.id = sa.course_id
LEFT JOIN courses sc ON sc.id = sa.sit_in_course_id
ORDER BY sa.created_at DESC;
