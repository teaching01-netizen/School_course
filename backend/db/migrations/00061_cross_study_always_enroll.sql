-- +goose Up
-- Cross-study weekday-scoped assignments now always create
-- course_roster_overrides and course_students entries (not just
-- session_attendance). This backfill retroactively creates
-- missing overrides and enrollments for existing weekday-scoped
-- assignments saved before the code fix.
--
-- See also: store.go SaveAssignment() — the `usesFullCourseEnrollment`
-- guard was removed from override creation and IncludeStudent calls.

WITH admin_user AS (
  SELECT id FROM users WHERE role = 'Admin' ORDER BY created_at DESC LIMIT 1
)
INSERT INTO course_roster_overrides
  (course_id, student_id, action, created_by_user_id, override_source, cross_study_assignment_id)
SELECT
  a.dest_course_a_id,
  s.id,
  'include',
  (SELECT id FROM admin_user),
  'cross_study',
  a.id
FROM crm_cross_study_assignments a
JOIN students s ON s.wcode = a.wcode
WHERE a.deleted_at IS NULL
  AND a.dest_course_a_enrollment_created = false
ON CONFLICT (course_id, student_id)
  DO UPDATE SET
    action = EXCLUDED.action,
    updated_by_user_id = EXCLUDED.created_by_user_id,
    updated_at = now(),
    deleted_at = NULL,
    override_source = 'cross_study',
    cross_study_assignment_id = EXCLUDED.cross_study_assignment_id
  WHERE course_roster_overrides.override_source = 'cross_study'
     OR course_roster_overrides.deleted_at IS NOT NULL;

WITH admin_user AS (
  SELECT id FROM users WHERE role = 'Admin' ORDER BY created_at DESC LIMIT 1
)
INSERT INTO course_roster_overrides
  (course_id, student_id, action, created_by_user_id, override_source, cross_study_assignment_id)
SELECT
  a.dest_course_b_id,
  s.id,
  'include',
  (SELECT id FROM admin_user),
  'cross_study',
  a.id
FROM crm_cross_study_assignments a
JOIN students s ON s.wcode = a.wcode
WHERE a.deleted_at IS NULL
  AND a.dest_course_b_enrollment_created = false
  AND a.dest_course_b_id <> a.dest_course_a_id
ON CONFLICT (course_id, student_id)
  DO UPDATE SET
    action = EXCLUDED.action,
    updated_by_user_id = EXCLUDED.created_by_user_id,
    updated_at = now(),
    deleted_at = NULL,
    override_source = 'cross_study',
    cross_study_assignment_id = EXCLUDED.cross_study_assignment_id
  WHERE course_roster_overrides.override_source = 'cross_study'
     OR course_roster_overrides.deleted_at IS NOT NULL;

-- Add student to course_students for destination courses (if not already there)
INSERT INTO course_students (course_id, student_id)
SELECT DISTINCT unnest(ARRAY[a.dest_course_a_id, a.dest_course_b_id]), s.id
FROM crm_cross_study_assignments a
JOIN students s ON s.wcode = a.wcode
WHERE a.deleted_at IS NULL
  AND (a.dest_course_a_enrollment_created = false OR a.dest_course_b_enrollment_created = false)
  AND s.id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Mark enrollment as created so future code knows this assignment owns the roster entry
UPDATE crm_cross_study_assignments
SET dest_course_a_enrollment_created = true,
    dest_course_b_enrollment_created = true,
    assigned_course_enrollment_created = true,
    updated_at = now()
WHERE deleted_at IS NULL
  AND (dest_course_a_enrollment_created = false OR dest_course_b_enrollment_created = false);

-- +goose Down
-- No down migration — the behavioral fix in store.go makes this permanent.
