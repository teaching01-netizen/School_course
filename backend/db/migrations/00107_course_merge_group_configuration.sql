-- +goose Up

ALTER TABLE course_merge_groups
  ADD COLUMN IF NOT EXISTS level smallint CHECK (level IS NULL OR level >= 1),
  ADD COLUMN IF NOT EXISTS sit_in_rule_id uuid REFERENCES sit_in_rules(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_course_merge_groups_sit_in_rule
  ON course_merge_groups(sit_in_rule_id);

-- +goose Down

DROP INDEX IF EXISTS idx_course_merge_groups_sit_in_rule;

ALTER TABLE course_merge_groups
  DROP COLUMN IF EXISTS sit_in_rule_id,
  DROP COLUMN IF EXISTS level;
