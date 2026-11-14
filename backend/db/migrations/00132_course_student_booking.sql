-- +goose Up
ALTER TABLE courses
  ADD COLUMN student_booking_enabled boolean NOT NULL DEFAULT false,
  ADD CONSTRAINT courses_student_booking_requires_subject
    CHECK (NOT student_booking_enabled OR subject_id IS NOT NULL);

-- The cutover runs with legacy writers drained. Preserve the old effective
-- availability, including hidden courses and continuation inheritance.
CREATE TEMP TABLE previous_course_booking ON COMMIT DROP AS
SELECT course_id, rule_course_id, absence_form_active FROM course_rule_configs;

UPDATE courses c
SET student_booking_enabled = true
FROM previous_course_booking old
WHERE old.course_id = c.id AND old.rule_course_id = c.id AND old.absence_form_active;

CREATE OR REPLACE VIEW course_rule_configs AS
SELECT
  c.id AS course_id,
  m.group_id AS merge_group_id,
  rc.id AS rule_course_id,
  rc.cycle_id,
  COALESCE(g.level, rc.level) AS level,
  rc.root_course_group_id,
  COALESCE(g.sit_in_rule_id, rcg.sit_in_rule_id) AS sit_in_rule_id,
  rc.student_booking_enabled AS absence_form_visible,
  rc.student_booking_enabled AS absence_form_active
FROM courses c
LEFT JOIN course_merge_group_members m ON m.course_id = c.id
LEFT JOIN course_merge_groups g ON g.id = m.group_id
JOIN courses rc ON rc.id = COALESCE(g.rule_source_course_id, c.id)
LEFT JOIN root_course_groups rcg ON rcg.id = rc.root_course_group_id;

-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM previous_course_booking old
    FULL JOIN course_rule_configs cfg USING (course_id)
    WHERE old.absence_form_active IS DISTINCT FROM cfg.absence_form_active
  ) THEN
    RAISE EXCEPTION 'Course booking availability changed during migration';
  END IF;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- Use only before reopening writes or after reconciling legacy state.
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
ALTER TABLE courses DROP COLUMN student_booking_enabled;
