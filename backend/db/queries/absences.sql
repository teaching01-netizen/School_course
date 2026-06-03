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

-- name: SessionsByStudentInRange :many
SELECT
  sess.id,
  sess.start_at,
  sess.end_at,
  c.id AS course_id,
  c.code AS course_code,
  c.name AS course_name,
  sub.id AS subject_id,
  sub.code AS subject_code,
  sub.name AS subject_name
FROM sessions sess
JOIN courses c ON c.id = sess.course_id
JOIN subjects sub ON sub.id = c.subject_id
JOIN course_students cs ON cs.course_id = c.id AND cs.status = 'enrolled'
JOIN students st ON st.id = cs.student_id
WHERE st.wcode = $1
  AND sess.start_at >= $2
  AND sess.start_at < ($3::date + interval '1 day')
  AND sess.deleted_at IS NULL
ORDER BY sub.code, sess.start_at;

-- name: AbsenceOverlappingSessions :many
SELECT DISTINCT sess.id AS session_id
FROM sessions sess
JOIN student_absences sa ON sa.course_id = sess.course_id
WHERE sa.wcode = $1
  AND sess.start_at >= sa.date_from
  AND sess.start_at < (sa.date_to + interval '1 day')
  AND sess.start_at >= $2
  AND sess.start_at < ($3::date + interval '1 day')
  AND sess.deleted_at IS NULL;
