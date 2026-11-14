-- +goose Up
-- A regular merge inherits booking from either member, including pairs
-- configured before the switches were linked.
CREATE OR REPLACE VIEW course_rule_configs AS
SELECT
  c.id AS course_id,
  m.group_id AS merge_group_id,
  rc.id AS rule_course_id,
  rc.cycle_id,
  COALESCE(g.level, rc.level) AS level,
  rc.root_course_group_id,
  COALESCE(g.sit_in_rule_id, rcg.sit_in_rule_id) AS sit_in_rule_id,
  booking.enabled AS absence_form_visible,
  booking.enabled AS absence_form_active
FROM courses c
LEFT JOIN course_merge_group_members m ON m.course_id = c.id
LEFT JOIN course_merge_groups g ON g.id = m.group_id
JOIN courses rc ON rc.id = COALESCE(g.rule_source_course_id, c.id)
LEFT JOIN root_course_groups rcg ON rcg.id = rc.root_course_group_id
CROSS JOIN LATERAL (
  SELECT rc.student_booking_enabled OR (
    c.subject_id IS NOT NULL
    AND m.group_id IS NOT NULL
    AND g.rule_source_course_id IS NULL
    AND EXISTS (
      SELECT 1
      FROM course_merge_group_members member
      JOIN courses peer ON peer.id = member.course_id
      WHERE member.group_id = m.group_id AND peer.student_booking_enabled
    )
  ) AS enabled
) booking;

-- +goose Down
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
