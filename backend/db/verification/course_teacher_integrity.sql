-- Course teacher integrity — data audit queries.
--
-- These are MANUAL / PERIODIC audits, not part of the migration. Run them
-- against the database before and after enabling strict enforcement to confirm
-- the course_teachers snapshot (is_primary) is consistent with the legacy
-- courses.teacher_id field and with session assignments. All are expected to
-- return zero rows / zero counts.
--
-- Active-entity convention: soft-deleted rows are excluded via
-- `deleted_at IS NULL`, matching the predicate used throughout the codebase.

-- 1. Legacy primary (courses.teacher_id) missing from the course's teacher set.
SELECT count(*)
FROM courses c
LEFT JOIN course_teachers ct
  ON ct.course_id = c.id
 AND ct.teacher_id = c.teacher_id
WHERE c.teacher_id IS NOT NULL
  AND ct.teacher_id IS NULL;

-- 2. Courses with more than one primary teacher.
SELECT course_id
FROM course_teachers
WHERE is_primary
GROUP BY course_id
HAVING count(*) > 1;

-- 3. Future sessions whose teacher is not assigned to the course at all.
SELECT
    s.id,
    s.course_id,
    s.teacher_id
FROM sessions s
LEFT JOIN course_teachers ct
  ON ct.course_id = s.course_id
 AND ct.teacher_id = s.teacher_id
WHERE s.start_at > now()
  AND s.deleted_at IS NULL
  AND ct.teacher_id IS NULL;
