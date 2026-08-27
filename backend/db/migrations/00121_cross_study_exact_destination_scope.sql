-- +goose Up
-- lint:allow-data: rebuild derived student_busy_ranges from the canonical session scope

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION student_is_expected_at_course_time(
  p_student_id uuid,
  p_course_id uuid,
  p_start_at timestamptz,
  p_session_id uuid DEFAULT NULL,
  p_assume_course_membership boolean DEFAULT false
)
RETURNS boolean
LANGUAGE sql
STABLE
AS $$
WITH attendance_scope AS (
  SELECT
    COALESCE(bool_or(sa.status = 'excluded'), false) AS explicitly_excluded,
    COALESCE(bool_or(
      sa.status = 'included'
      AND COALESCE(sa.override_source, 'manual') <> 'cross_study'
    ), false) AS manually_included
  FROM session_attendance sa
  WHERE p_session_id IS NOT NULL
    AND sa.session_id = p_session_id
    AND sa.student_id = p_student_id
), assignment_matches AS (
  SELECT
    a.dest_course_a_weekdays,
    a.dest_course_b_weekdays,
    (p_course_id = a.dest_course_a_id) AS matches_destination_a,
    (p_course_id = a.dest_course_b_id) AS matches_destination_b
  FROM students st
  JOIN crm_cross_study_assignments a
    ON lower(a.wcode) = lower(st.wcode)
   AND a.deleted_at IS NULL
  WHERE st.id = p_student_id
    AND (
      p_course_id = a.dest_course_a_id
      OR p_course_id = a.dest_course_b_id
      OR EXISTS (
        SELECT 1
        FROM course_merge_group_members selected_member
        JOIN course_merge_group_members related_member
          ON related_member.group_id = selected_member.group_id
        WHERE selected_member.course_id IN (a.dest_course_a_id, a.dest_course_b_id)
          AND related_member.course_id = p_course_id
      )
    )
), assignment_scope AS (
  SELECT
    EXISTS (
      SELECT 1
      FROM assignment_matches am
      WHERE (
        am.matches_destination_a
        AND EXTRACT(ISODOW FROM (p_start_at AT TIME ZONE 'Asia/Bangkok'))::smallint = ANY(
          COALESCE(am.dest_course_a_weekdays, ARRAY[1,2,3,4,5,6,7]::smallint[])
        )
      )
      OR (
        am.matches_destination_b
        AND EXTRACT(ISODOW FROM (p_start_at AT TIME ZONE 'Asia/Bangkok'))::smallint = ANY(
          COALESCE(am.dest_course_b_weekdays, ARRAY[1,2,3,4,5,6,7]::smallint[])
        )
      )
    ) AS selected,
    EXISTS (SELECT 1 FROM assignment_matches) AS covered
)
SELECT CASE
  WHEN attendance_scope.explicitly_excluded THEN false
  WHEN attendance_scope.manually_included THEN true
  WHEN assignment_scope.covered THEN assignment_scope.selected
  ELSE EXISTS (
    SELECT 1
    FROM course_students cs
    WHERE cs.course_id = p_course_id
      AND cs.student_id = p_student_id
  ) OR p_assume_course_membership
END
FROM attendance_scope
CROSS JOIN assignment_scope;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
  session_to_rebuild uuid;
BEGIN
  CREATE TEMP TABLE preserved_busy_range_overrides
  ON COMMIT DROP AS
  SELECT student_id, session_id
  FROM student_busy_ranges
  WHERE deleted_at IS NULL
    AND conflict_override
  GROUP BY student_id, session_id;

  DELETE FROM student_busy_ranges
  WHERE deleted_at IS NULL;

  FOR session_to_rebuild IN
    SELECT id
    FROM sessions
    WHERE deleted_at IS NULL
  LOOP
    PERFORM refresh_student_busy_ranges_for_session(session_to_rebuild);
  END LOOP;

  UPDATE student_busy_ranges sbr
  SET conflict_override = true
  FROM preserved_busy_range_overrides preserved
  WHERE sbr.student_id = preserved.student_id
    AND sbr.session_id = preserved.session_id
    AND sbr.deleted_at IS NULL;
END;
$$;
-- +goose StatementEnd

-- +goose Down

