-- +goose Up
ALTER TABLE sit_in_rules DROP CONSTRAINT IF EXISTS sit_in_rules_type_check;
ALTER TABLE sit_in_rules ADD CONSTRAINT sit_in_rules_type_check 
  CHECK (type IN (
    'level_ladder', 'cross_section', 'any_day_except_last', 
    'rank_chain', 'teacher_case_by_case', 'sat_verbal_priority'
  ));

-- Seed a default SAT Verbal Priority rule for the SAT Verbal root course group.
-- The actual root course group ID will vary per deployment.
-- This inserts a template that can be edited via the admin UI.
INSERT INTO sit_in_rules (name, type, predicate, description)
VALUES (
  'SAT Verbal Priority',
  'sat_verbal_priority',
  '{
    "priority_rows": [
      {"missed_rank": 1, "priority": 1, "rule_type": "cross_section", "occurrence_match": "same", "day_match": "any", "last_class_excluded": true, "label": "Same occurrence cross-section"},
      {"missed_rank": 1, "priority": 2, "rule_type": "cross_section", "occurrence_match": "any", "day_match": "any", "last_class_excluded": true, "label": "Any occurrence cross-section"},
      {"missed_rank": 1, "priority": 3, "rule_type": "any_day_except_last", "occurrence_match": "any", "day_match": "any", "last_class_excluded": true, "label": "Any class except final"},
      {"missed_rank": 2, "priority": 1, "rule_type": "rank_chain", "target_rank": 1, "occurrence_match": "same", "day_match": "any", "last_class_excluded": true, "label": "Rank 2 → Rank 1 same occurrence"},
      {"missed_rank": 2, "priority": 2, "rule_type": "rank_chain", "target_rank": 3, "occurrence_match": "any", "day_match": "any", "last_class_excluded": true, "label": "Rank 2 → Rank 3"},
      {"missed_rank": 3, "priority": 1, "rule_type": "cross_section", "occurrence_match": "same", "day_match": "any", "last_class_excluded": true, "label": "Same occurrence cross-section"},
      {"missed_rank": 3, "priority": 2, "rule_type": "cross_section", "occurrence_match": "any", "day_match": "any", "last_class_excluded": true, "label": "Any occurrence cross-section"},
      {"missed_rank": 4, "priority": 1, "rule_type": "rank_chain", "target_rank": 5, "occurrence_match": "any", "day_match": "any", "last_class_excluded": true, "label": "Rank 4 → Rank 5"},
      {"missed_rank": 4, "priority": 2, "rule_type": "any_day_except_last", "occurrence_match": "any", "day_match": "any", "last_class_excluded": true, "label": "Any class except final"},
      {"missed_rank": 5, "priority": 1, "rule_type": "rank_chain", "target_rank": 4, "occurrence_match": "any", "day_match": "any", "last_class_excluded": true, "label": "Rank 5 → Rank 4"},
      {"missed_rank": 5, "priority": 2, "rule_type": "any_day_except_last", "occurrence_match": "any", "day_match": "any", "last_class_excluded": true, "label": "Any class except final"}
    ]
  }'::jsonb,
  'SAT Verbal priority-based sit-in policy with 3-tier priority for Beginner/Rank 1-3 and 2-tier for Rank 4-5'
)
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DELETE FROM sit_in_rules WHERE name = 'SAT Verbal Priority';
ALTER TABLE sit_in_rules DROP CONSTRAINT IF EXISTS sit_in_rules_type_check;
ALTER TABLE sit_in_rules ADD CONSTRAINT sit_in_rules_type_check 
  CHECK (type IN (
    'level_ladder', 'cross_section', 'any_day_except_last', 
    'rank_chain', 'teacher_case_by_case'
  ));
