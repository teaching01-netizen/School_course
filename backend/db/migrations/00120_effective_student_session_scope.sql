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
    (
      p_course_id = a.dest_course_a_id
      OR EXISTS (
        SELECT 1
        FROM course_merge_group_members selected_member
        JOIN course_merge_group_members related_member
          ON related_member.group_id = selected_member.group_id
        WHERE selected_member.course_id = a.dest_course_a_id
          AND related_member.course_id = p_course_id
      )
    ) AS matches_destination_a,
    (
      p_course_id = a.dest_course_b_id
      OR EXISTS (
        SELECT 1
        FROM course_merge_group_members selected_member
        JOIN course_merge_group_members related_member
          ON related_member.group_id = selected_member.group_id
        WHERE selected_member.course_id = a.dest_course_b_id
          AND related_member.course_id = p_course_id
      )
    ) AS matches_destination_b
  FROM students st
  JOIN crm_cross_study_assignments a
    ON lower(a.wcode) = lower(st.wcode)
   AND a.deleted_at IS NULL
  WHERE (
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
CREATE OR REPLACE FUNCTION refresh_student_busy_ranges_for_session(p_session_id uuid)
RETURNS void AS $$
DECLARE
  s_course_id uuid;
  s_start timestamptz;
  s_end timestamptz;
  s_deleted_at timestamptz;
  s_conflict_override boolean;
  s_legacy_conflict_override boolean;
BEGIN
  SELECT course_id, start_at, end_at, deleted_at, conflict_override, legacy_conflict_override
  INTO s_course_id, s_start, s_end, s_deleted_at, s_conflict_override, s_legacy_conflict_override
  FROM sessions
  WHERE id = p_session_id;

  IF NOT FOUND THEN
    RETURN;
  END IF;

  IF s_deleted_at IS NOT NULL THEN
    UPDATE student_busy_ranges
    SET deleted_at = s_deleted_at
    WHERE session_id = p_session_id AND deleted_at IS NULL;
    RETURN;
  END IF;

  DELETE FROM student_busy_ranges WHERE session_id = p_session_id;

  INSERT INTO student_busy_ranges (
    student_id, session_id, start_at, end_at, deleted_at, conflict_override
  )
  SELECT roster.student_id,
         p_session_id,
         s_start,
         s_end,
         NULL,
         (s_conflict_override OR s_legacy_conflict_override)
  FROM (
    SELECT cs.student_id
    FROM course_students cs
    WHERE cs.course_id = s_course_id
    UNION
    SELECT sa.student_id
    FROM session_attendance sa
    WHERE sa.session_id = p_session_id AND sa.status = 'included'
    UNION
    SELECT st.id
    FROM students st
    JOIN crm_cross_study_assignments a
      ON lower(a.wcode) = lower(st.wcode)
     AND a.deleted_at IS NULL
    WHERE (
      s_course_id = a.dest_course_a_id
      OR s_course_id = a.dest_course_b_id
      OR EXISTS (
        SELECT 1
        FROM course_merge_group_members selected_member
        JOIN course_merge_group_members related_member
          ON related_member.group_id = selected_member.group_id
        WHERE selected_member.course_id IN (a.dest_course_a_id, a.dest_course_b_id)
          AND related_member.course_id = s_course_id
      )
    )
  ) roster
  WHERE student_is_expected_at_session(roster.student_id, p_session_id);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_course_students_insert_busy_ranges()
RETURNS trigger AS $$
DECLARE
  session_count bigint;
  guardrail_limit constant bigint := 20000;
BEGIN
  SELECT count(*) INTO session_count
  FROM sessions
  WHERE course_id = NEW.course_id AND deleted_at IS NULL;

  IF session_count > guardrail_limit THEN
    RAISE WARNING 'course_students_insert: course % has % active sessions (>% guardrail), busy range maintenance may be slow',
      NEW.course_id, session_count, guardrail_limit;
  END IF;

  INSERT INTO student_busy_ranges (
    student_id, session_id, start_at, end_at, conflict_override
  )
  SELECT NEW.student_id,
         s.id,
         s.start_at,
         s.end_at,
         (s.conflict_override OR s.legacy_conflict_override)
  FROM sessions s
  WHERE s.course_id = NEW.course_id
    AND s.deleted_at IS NULL
    AND student_is_expected_at_session(NEW.student_id, s.id)
    AND NOT EXISTS (
      SELECT 1
      FROM student_busy_ranges sbr
      WHERE sbr.session_id = s.id
        AND sbr.student_id = NEW.student_id
        AND sbr.deleted_at IS NULL
    );

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_course_students_delete_busy_ranges()
RETURNS trigger AS $$
DECLARE
  session_count bigint;
  guardrail_limit constant bigint := 20000;
BEGIN
  SELECT count(*) INTO session_count
  FROM sessions
  WHERE course_id = OLD.course_id AND deleted_at IS NULL;

  IF session_count > guardrail_limit THEN
    RAISE WARNING 'course_students_delete: course % has % active sessions (>% guardrail), busy range maintenance may be slow',
      OLD.course_id, session_count, guardrail_limit;
  END IF;

  UPDATE student_busy_ranges sbr
  SET deleted_at = now()
  WHERE sbr.student_id = OLD.student_id
    AND sbr.deleted_at IS NULL
    AND sbr.session_id IN (
      SELECT s.id
      FROM sessions s
      WHERE s.course_id = OLD.course_id AND s.deleted_at IS NULL
    )
    AND NOT student_is_expected_at_session(OLD.student_id, sbr.session_id);

  RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_course_students_insert_busy_ranges ON course_students;
CREATE TRIGGER trg_course_students_insert_busy_ranges
AFTER INSERT ON course_students
FOR EACH ROW EXECUTE FUNCTION trg_course_students_insert_busy_ranges();

DROP TRIGGER IF EXISTS trg_course_students_delete_busy_ranges ON course_students;
CREATE TRIGGER trg_course_students_delete_busy_ranges
AFTER DELETE ON course_students
FOR EACH ROW EXECUTE FUNCTION trg_course_students_delete_busy_ranges();

-- +goose StatementBegin
DO $$
DECLARE
  session_to_rebuild uuid;
BEGIN
  CREATE TEMP TABLE effective_student_busy_range_conflict_overrides ON COMMIT DROP AS
  SELECT student_id, session_id
  FROM student_busy_ranges
  WHERE deleted_at IS NULL
    AND conflict_override
  GROUP BY student_id, session_id;

  DELETE FROM student_busy_ranges WHERE deleted_at IS NULL;
  FOR session_to_rebuild IN SELECT s.id FROM sessions s LOOP
    PERFORM refresh_student_busy_ranges_for_session(session_to_rebuild);
  END LOOP;

  UPDATE student_busy_ranges sbr
  SET conflict_override = true
  FROM effective_student_busy_range_conflict_overrides preserved
  WHERE sbr.student_id = preserved.student_id
    AND sbr.session_id = preserved.session_id
    AND sbr.deleted_at IS NULL;
END;
$$;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION refresh_student_busy_ranges_for_session(p_session_id uuid)
RETURNS void AS $$
DECLARE
  s_course_id uuid;
  s_start timestamptz;
  s_end timestamptz;
  s_deleted_at timestamptz;
  s_conflict_override boolean;
  s_legacy_conflict_override boolean;
BEGIN
  SELECT course_id, start_at, end_at, deleted_at, conflict_override, legacy_conflict_override
  INTO s_course_id, s_start, s_end, s_deleted_at, s_conflict_override, s_legacy_conflict_override
  FROM sessions
  WHERE id = p_session_id;

  IF NOT FOUND THEN
    RETURN;
  END IF;

  IF s_deleted_at IS NOT NULL THEN
    UPDATE student_busy_ranges
    SET deleted_at = s_deleted_at
    WHERE session_id = p_session_id AND deleted_at IS NULL;
    RETURN;
  END IF;

  DELETE FROM student_busy_ranges WHERE session_id = p_session_id;
  INSERT INTO student_busy_ranges (
    student_id, session_id, start_at, end_at, deleted_at, conflict_override
  )
  SELECT roster.student_id, p_session_id, s_start, s_end, NULL,
         (s_conflict_override OR s_legacy_conflict_override)
  FROM (
    SELECT cs.student_id
    FROM course_students cs
    WHERE cs.course_id = s_course_id
    UNION
    SELECT sa.student_id
    FROM session_attendance sa
    WHERE sa.session_id = p_session_id AND sa.status = 'included'
  ) roster
  WHERE NOT EXISTS (
    SELECT 1
    FROM session_attendance sa
    WHERE sa.session_id = p_session_id
      AND sa.student_id = roster.student_id
      AND sa.status = 'excluded'
  );
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_course_students_insert_busy_ranges()
RETURNS trigger AS $$
BEGIN
  INSERT INTO student_busy_ranges (
    student_id, session_id, start_at, end_at, conflict_override
  )
  SELECT NEW.student_id, s.id, s.start_at, s.end_at,
         (s.conflict_override OR s.legacy_conflict_override)
  FROM sessions s
  WHERE s.course_id = NEW.course_id
    AND s.deleted_at IS NULL
    AND NOT EXISTS (
      SELECT 1
      FROM session_attendance sa
      WHERE sa.session_id = s.id
        AND sa.student_id = NEW.student_id
        AND sa.status = 'excluded'
    )
    AND NOT EXISTS (
      SELECT 1
      FROM student_busy_ranges sbr
      WHERE sbr.session_id = s.id
        AND sbr.student_id = NEW.student_id
        AND sbr.deleted_at IS NULL
    );
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_course_students_delete_busy_ranges()
RETURNS trigger AS $$
BEGIN
  UPDATE student_busy_ranges
  SET deleted_at = now()
  WHERE student_id = OLD.student_id
    AND deleted_at IS NULL
    AND session_id IN (
      SELECT s.id
      FROM sessions s
      WHERE s.course_id = OLD.course_id AND s.deleted_at IS NULL
    )
    AND NOT EXISTS (
      SELECT 1
      FROM session_attendance sa
      WHERE sa.session_id = student_busy_ranges.session_id
        AND sa.student_id = OLD.student_id
        AND sa.status = 'included'
    );
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
  session_id uuid;
BEGIN
  DELETE FROM student_busy_ranges WHERE deleted_at IS NULL;
  FOR session_id IN SELECT id FROM sessions LOOP
    PERFORM refresh_student_busy_ranges_for_session(session_id);
  END LOOP;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_course_students_insert_busy_ranges ON course_students;
CREATE TRIGGER trg_course_students_insert_busy_ranges
AFTER INSERT ON course_students
FOR EACH ROW EXECUTE FUNCTION trg_course_students_insert_busy_ranges();

DROP TRIGGER IF EXISTS trg_course_students_delete_busy_ranges ON course_students;
CREATE TRIGGER trg_course_students_delete_busy_ranges
AFTER DELETE ON course_students
FOR EACH ROW EXECUTE FUNCTION trg_course_students_delete_busy_ranges();

DROP FUNCTION IF EXISTS student_is_expected_at_session(uuid, uuid);
DROP FUNCTION IF EXISTS student_is_expected_at_course_time(uuid, uuid, timestamptz, uuid, boolean);
