-- +goose Up

ALTER TABLE sat_verbal_policy_mappings
  ALTER COLUMN course_id DROP NOT NULL,
  ADD COLUMN IF NOT EXISTS merge_group_id uuid REFERENCES course_merge_groups(id) ON DELETE CASCADE;

ALTER TABLE sat_verbal_policy_mappings
  DROP CONSTRAINT IF EXISTS sat_verbal_policy_mappings_target_check;

ALTER TABLE sat_verbal_policy_mappings
  ADD CONSTRAINT sat_verbal_policy_mappings_target_check
  CHECK ((course_id IS NOT NULL) <> (merge_group_id IS NOT NULL));

CREATE UNIQUE INDEX IF NOT EXISTS sat_verbal_policy_mappings_merge_group_unique
  ON sat_verbal_policy_mappings (merge_group_id)
  WHERE merge_group_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS sat_verbal_policy_mappings_merge_group_active
  ON sat_verbal_policy_mappings (merge_group_id)
  WHERE active AND merge_group_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS sat_verbal_policy_mappings_merge_group_active;
DROP INDEX IF EXISTS sat_verbal_policy_mappings_merge_group_unique;

ALTER TABLE sat_verbal_policy_mappings
  DROP CONSTRAINT IF EXISTS sat_verbal_policy_mappings_target_check,
  DROP COLUMN IF EXISTS merge_group_id;
