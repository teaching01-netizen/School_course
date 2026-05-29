-- +goose Up

-- Assign "Level Ladder" rule to all existing root course groups that have no rule
UPDATE root_course_groups
SET sit_in_rule_id = 'a0000000-0000-0000-0000-000000000001'
WHERE sit_in_rule_id IS NULL;

-- +goose Down

-- Undo backfill (set back to NULL)
UPDATE root_course_groups
SET sit_in_rule_id = NULL
WHERE sit_in_rule_id = 'a0000000-0000-0000-0000-000000000001';
