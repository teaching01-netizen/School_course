-- Course teacher membership queries for the courseadmin domain service.
-- The entire teacher set of a course is replaced atomically inside one
-- transaction (delete-all + reinsert), so reads here are either row locks
-- or snapshots taken inside the same transaction.

-- name: CourseTeacherExists :one
SELECT EXISTS (
    SELECT 1
    FROM course_teachers
    WHERE course_id = $1
      AND teacher_id = $2
);

-- name: CourseTeacherSetExists :one
-- Reports whether the course has any assigned teachers at all. Used by the
-- advisory preflight to stay lenient toward legacy courses whose teacher set
-- was never populated (the transactional check remains authoritative and
-- rejects any teacher for an empty set).
SELECT EXISTS (
    SELECT 1
    FROM course_teachers
    WHERE course_id = $1
);

-- name: CourseLockForTeacherUpdate :one
SELECT
    id,
    version,
    code,
    name,
    legacy_course_id,
    year,
    subject_id,
    hour,
    student_count,
    course_type
FROM courses
WHERE id = $1
FOR UPDATE;

-- name: UsersListForTeacherValidation :many
SELECT
    id,
    username,
    role,
    deleted_at
FROM users
WHERE id = ANY($1::uuid[]);

-- name: CourseTeachersList :many
SELECT
    ct.course_id,
    ct.teacher_id,
    ct.is_primary,
    u.username
FROM course_teachers ct
JOIN users u ON u.id = ct.teacher_id
WHERE ct.course_id = $1
ORDER BY
    ct.is_primary DESC,
    u.username,
    ct.teacher_id;

-- name: CourseTeachersListForCourses :many
-- Batch teacher read for the course list endpoint: returns every assigned
-- teacher (with the is_primary flag) for a set of course ids in one query,
-- ordered by username so the list response is deterministic.
SELECT
    ct.course_id,
    ct.teacher_id,
    ct.is_primary,
    u.username,
    u.full_name
FROM course_teachers ct
JOIN users u ON u.id = ct.teacher_id
WHERE ct.course_id = ANY($1::uuid[])
ORDER BY u.username;

-- name: CourseFutureSessionUsageByTeachers :many
SELECT
    s.teacher_id,
    count(*) AS session_count,
    min(s.start_at)::timestamptz AS earliest_start_at,
    ((array_agg(s.id ORDER BY s.start_at, s.id))[:10])::uuid[] AS sample_session_ids,
    array_remove(array_agg(DISTINCT s.series_id), NULL)::uuid[] AS series_ids
FROM sessions s
WHERE s.course_id = @course_id
  AND s.teacher_id = ANY(@teacher_ids::uuid[])
  AND s.start_at > @start_at
  AND s.deleted_at IS NULL
GROUP BY s.teacher_id;

-- name: CourseTeachersDeleteAll :exec
DELETE FROM course_teachers
WHERE course_id = $1;

-- name: CourseTeacherMembershipGet :one
-- Returns the complete membership state: course existence, whether the course
-- has any assigned teachers, and whether the given teacher is assigned.
SELECT
    EXISTS (SELECT 1 FROM courses WHERE courses.id = sqlc.arg(course_id)) AS course_exists,
    EXISTS (SELECT 1 FROM course_teachers WHERE course_teachers.course_id = sqlc.arg(course_id)) AS has_teachers,
    EXISTS (SELECT 1 FROM course_teachers WHERE course_teachers.course_id = sqlc.arg(course_id) AND course_teachers.teacher_id = sqlc.arg(teacher_id)) AS assigned;

-- name: CourseTeacherInsert :exec
INSERT INTO course_teachers (
    course_id,
    teacher_id,
    is_primary
)
VALUES ($1, $2, $3);

-- name: CourseUpdateAggregate :one
UPDATE courses
SET
    code = $2,
    name = $3,
    legacy_course_id = $4,
    teacher_id = $5,
    year = $6,
    subject_id = $7,
    hour = $8,
    student_count = $9,
    course_type = $10,
    version = version + 1,
    updated_at = now()
WHERE id = $1
RETURNING id, version;

-- name: CourseUpdateCore :one
-- Metadata-only course update for the legacy PUT path (no teacher fields in
-- the request): bumps the version but leaves the teacher set — both the
-- course_teachers rows and the courses.teacher_id compat projection —
-- completely untouched.
UPDATE courses
SET
    code = $2,
    name = $3,
    legacy_course_id = $4,
    year = $5,
    subject_id = $6,
    hour = $7,
    student_count = $8,
    course_type = $9,
    version = version + 1,
    updated_at = now()
WHERE id = $1
RETURNING id, version;

-- name: CourseCreateAggregate :one
-- Mirror of CourseUpdateAggregate for freshly created courses: sets the
-- compat primary projection (courses.teacher_id) and code/name/legacy link
-- WITHOUT bumping version, so a new course starts at version 1 and the first
-- edit is the first optimistic-concurrency bump.
UPDATE courses
SET
    code = $2,
    name = $3,
    legacy_course_id = $4,
    teacher_id = $5,
    updated_at = now()
WHERE id = $1
RETURNING id, version;
