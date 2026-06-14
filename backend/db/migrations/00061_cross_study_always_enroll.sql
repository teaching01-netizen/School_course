-- Cross-study weekday-scoped assignments now always create
-- course_roster_overrides and course_students entries (not just
-- session_attendance). This backfill retroactively creates missing
-- overrides and enrollments for existing weekday-scoped assignments
-- that were saved before the code fix.
--
-- See also: store.go SaveAssignment() — the `usesFullCourseEnrollment`
-- guard was removed from override creation and IncludeStudent calls.

DO $$
DECLARE
  rec RECORD;
  v_student_id uuid;
  v_admin_user_id uuid;
BEGIN

  -- Pick the most recently created admin as the audit user for backfilled overrides.
  SELECT id INTO v_admin_user_id
    FROM users
   WHERE role = 'Admin'
   ORDER BY created_at DESC
   LIMIT 1;

  IF v_admin_user_id IS NULL THEN
    RAISE EXCEPTION 'no admin user found for backfill attribution';
  END IF;

  FOR rec IN
    SELECT a.id AS assignment_id,
           a.wcode,
           a.dest_course_a_id,
           a.dest_course_b_id,
           a.dest_course_a_enrollment_created,
           a.dest_course_b_enrollment_created
      FROM crm_cross_study_assignments a
     WHERE a.deleted_at IS NULL
       AND (a.dest_course_a_enrollment_created = false OR a.dest_course_b_enrollment_created = false)
  LOOP
    -- Resolve student id
    SELECT id INTO v_student_id FROM students WHERE wcode = rec.wcode;
    IF v_student_id IS NULL THEN
      RAISE WARNING 'student % not found, skipping assignment %', rec.wcode, rec.assignment_id;
      CONTINUE;
    END IF;

    -- Destination A
    IF NOT rec.dest_course_a_enrollment_created THEN
      INSERT INTO course_roster_overrides
        (course_id, student_id, action, created_by_user_id, override_source, cross_study_assignment_id)
      VALUES
        (rec.dest_course_a_id, v_student_id, 'include', v_admin_user_id, 'cross_study', rec.assignment_id)
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

      INSERT INTO course_students (course_id, student_id)
      VALUES (rec.dest_course_a_id, v_student_id)
      ON CONFLICT DO NOTHING;
    END IF;

    -- Destination B (only if different from A)
    IF rec.dest_course_b_id <> rec.dest_course_a_id AND NOT rec.dest_course_b_enrollment_created THEN
      INSERT INTO course_roster_overrides
        (course_id, student_id, action, created_by_user_id, override_source, cross_study_assignment_id)
      VALUES
        (rec.dest_course_b_id, v_student_id, 'include', v_admin_user_id, 'cross_study', rec.assignment_id)
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

      INSERT INTO course_students (course_id, student_id)
      VALUES (rec.dest_course_b_id, v_student_id)
      ON CONFLICT DO NOTHING;
    END IF;

    -- Mark enrollment as created so future code knows this assignment owns the roster entry
    UPDATE crm_cross_study_assignments
       SET dest_course_a_enrollment_created = true,
           dest_course_b_enrollment_created = true,
           assigned_course_enrollment_created = true,
           updated_at = now()
     WHERE id = rec.assignment_id;
  END LOOP;
END $$;

