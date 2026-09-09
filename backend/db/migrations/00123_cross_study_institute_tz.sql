-- +goose Up
-- Step 5: timezone-aware cross-study weekday scope (decision D2b).
--
-- student_is_expected_at_course_time hardcoded AT TIME ZONE 'Asia/Bangkok'
-- (migrations 00120/00121). Go callers honor arbitrary IANA zones via
-- instituteLocation, so any DST-bearing or fractional-offset configuration
-- would diverge from SQL scope. This migration adds a tz-aware overload and
-- repoints student_is_expected_at_session at it; the Bangkok behavior is
-- preserved exactly when p_institute_tz = 'Asia/Bangkok'.
--
-- Compatibility: the 5-arg signature is kept as a thin wrapper defaulting to
-- Asia/Bangkok so mixed-version replicas calling the old form keep working.
-- Remove the wrapper only after all replicas run this migration (Step 24).

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION student_is_expected_at_course_time_tz(
  p_student_id uuid,
  p_course_id uuid,
  p_start_at timestamptz,
  p_session_id uuid DEFAULT NULL,
  p_assume_course_membership boolean DEFAULT false,
  p_institute_tz text DEFAULT 'Asia/Bangkok'
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
        AND EXTRACT(ISODOW FROM (p_start_at AT TIME ZONE p_institute_tz))::smallint = ANY(
          COALESCE(am.dest_course_a_weekdays, ARRAY[1,2,3,4,5,6,7]::smallint[])
        )
      )
      OR (
        am.matches_destination_b
        AND EXTRACT(ISODOW FROM (p_start_at AT TIME ZONE p_institute_tz))::smallint = ANY(
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
CREATE OR REPLACE FUNCTION student_is_expected_at_session_tz(
  p_student_id uuid,
  p_session_id uuid,
  p_institute_tz text DEFAULT 'Asia/Bangkok'
)
RETURNS boolean
LANGUAGE sql
STABLE
AS $$
SELECT COALESCE((
  SELECT student_is_expected_at_course_time_tz(
    p_student_id,
    s.course_id,
    s.start_at,
    p_session_id,
    false,
    p_institute_tz
  )
  FROM sessions s
  WHERE s.id = p_session_id
    AND s.deleted_at IS NULL
), false);
$$;
-- +goose StatementEnd

-- Old 5-arg form keeps working (Bangkok default) for mixed-version replicas.
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
SELECT student_is_expected_at_course_time_tz(
  p_student_id, p_course_id, p_start_at, p_session_id,
  p_assume_course_membership, 'Asia/Bangkok'
);
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION student_is_expected_at_session(
  p_student_id uuid,
  p_session_id uuid
)
RETURNS boolean
LANGUAGE sql
STABLE
AS $$
SELECT student_is_expected_at_session_tz(p_student_id, p_session_id, 'Asia/Bangkok');
$$;
-- +goose StatementEnd

-- +goose Down
-- Restore the pre-00123 bodies exactly (from 00121 for course_time,
-- from 00120 for session) and drop the tz-aware overloads.
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
CREATE OR REPLACE FUNCTION student_is_expected_at_session(
  p_student_id uuid,
  p_session_id uuid
)
RETURNS boolean
LANGUAGE sql
STABLE
AS $$
SELECT COALESCE((
  SELECT student_is_expected_at_course_time(
    p_student_id,
    s.course_id,
    s.start_at,
    p_session_id
  )
  FROM sessions s
  WHERE s.id = p_session_id
    AND s.deleted_at IS NULL
), false);
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DROP FUNCTION IF EXISTS student_is_expected_at_session_tz(uuid, uuid, text);
-- +goose StatementEnd

-- +goose StatementBegin
DROP FUNCTION IF EXISTS student_is_expected_at_course_time_tz(uuid, uuid, timestamptz, uuid, boolean, text);
-- +goose StatementEnd
