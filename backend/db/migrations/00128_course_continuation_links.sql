-- +goose Up
-- Continuation links: two course IDs for one real course (split by
-- inconsistent CRM input). A merge group with rule_source_course_id reads
-- level, family, cycle, sit-in rule and form visibility live from that source
-- course, and enrollment in either member counts for both. Nothing is copied.

ALTER TABLE course_merge_groups
  ADD COLUMN IF NOT EXISTS rule_source_course_id uuid;

-- The source must be a member; this also blocks deleting the source course
-- while linked (its member row cannot cascade away under the reference).
ALTER TABLE course_merge_groups
  DROP CONSTRAINT IF EXISTS course_merge_groups_rule_source_member_fk;
ALTER TABLE course_merge_groups
  ADD CONSTRAINT course_merge_groups_rule_source_member_fk
  FOREIGN KEY (id, rule_source_course_id)
  REFERENCES course_merge_group_members (group_id, course_id);

CREATE OR REPLACE VIEW course_rule_configs AS
SELECT
  c.id AS course_id,
  m.group_id AS merge_group_id,
  rc.id AS rule_course_id,
  rc.cycle_id,
  COALESCE(g.level, rc.level) AS level,
  rc.root_course_group_id,
  COALESCE(g.sit_in_rule_id, rcg.sit_in_rule_id) AS sit_in_rule_id,
  rc.absence_form_visible,
  rc.absence_form_visible AND EXISTS (
    SELECT 1 FROM subject_active_courses sac
    WHERE sac.subject_id = rc.subject_id AND sac.course_id = rc.id
  ) AS absence_form_active
FROM courses c
LEFT JOIN course_merge_group_members m ON m.course_id = c.id
LEFT JOIN course_merge_groups g ON g.id = m.group_id
JOIN courses rc ON rc.id = COALESCE(g.rule_source_course_id, c.id)
LEFT JOIN root_course_groups rcg ON rcg.id = rc.root_course_group_id;

-- Direct enrollment plus enrollment carried over from a continuation sibling.
-- A direct row always wins, so each (course, student) appears once. Shaped as
-- one course_students scan plus a LATERAL expansion (not a top-level UNION) so
-- student-filtered callers stay index-driven.
CREATE OR REPLACE VIEW effective_course_students AS
SELECT x.course_id, cs.student_id, cs.status
FROM course_students cs
CROSS JOIN LATERAL (
  SELECT cs.course_id
  UNION ALL
  SELECT sib.course_id
  FROM course_merge_group_members m
  JOIN course_merge_groups g ON g.id = m.group_id AND g.rule_source_course_id IS NOT NULL
  JOIN course_merge_group_members sib ON sib.group_id = m.group_id AND sib.course_id <> m.course_id
  WHERE m.course_id = cs.course_id
    AND NOT EXISTS (
      SELECT 1 FROM course_students d
      WHERE d.course_id = sib.course_id AND d.student_id = cs.student_id
    )
) x(course_id);

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
    FROM effective_course_students cs
    WHERE cs.course_id = p_course_id
      AND cs.student_id = p_student_id
  ) OR p_assume_course_membership
END
FROM attendance_scope
CROSS JOIN assignment_scope;
$$;
-- +goose StatementEnd

-- +goose Down

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

DROP VIEW IF EXISTS effective_course_students;
DROP VIEW IF EXISTS course_rule_configs;

ALTER TABLE course_merge_groups
  DROP CONSTRAINT IF EXISTS course_merge_groups_rule_source_member_fk,
  DROP COLUMN IF EXISTS rule_source_course_id;
