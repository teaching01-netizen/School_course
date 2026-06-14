-- +goose Up

CREATE TABLE IF NOT EXISTS sit_in_rules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text UNIQUE NOT NULL,
    type text NOT NULL CHECK (type IN ('level_ladder', 'cross_section', 'any_day_except_last', 'rank_chain', 'teacher_case_by_case')),
    predicate jsonb NOT NULL DEFAULT '{}',
    description text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE root_course_groups ADD COLUMN sit_in_rule_id uuid REFERENCES sit_in_rules(id);

CREATE INDEX IF NOT EXISTS idx_root_course_groups_sit_in_rule ON root_course_groups(sit_in_rule_id);
CREATE INDEX IF NOT EXISTS idx_sit_in_rules_type ON sit_in_rules(type);
CREATE INDEX IF NOT EXISTS idx_sit_in_rules_name ON sit_in_rules(name);

INSERT INTO sit_in_rules (id, name, type, predicate, description) VALUES
('a0000000-0000-0000-0000-000000000001', 'Level Ladder', 'level_ladder', '{"level_1_action": "zoom", "non_max_direction": "sit_higher", "max_direction": "sit_lower", "min_level_for_sit_lower": 2}', 'Level 1 students zoom, higher levels sit up or down based on level'),
('a0000000-0000-0000-0000-000000000002', 'Cross-Section Same Occurrence', 'cross_section', '{"section_match": "cross_section", "occurrence_match": "same_occurrence_number", "day_match": "any", "last_class_excluded": true}', 'SAT Verbal Beginner: cross-section, same occurrence number'),
('a0000000-0000-0000-0000-000000000003', 'Any Day Except Last', 'any_day_except_last', '{"day_match": "any_day", "last_class_excluded": true}', 'SAT Verbal Rank 5: any day, not last class'),
('a0000000-0000-0000-0000-000000000004', 'Rank Chain', 'rank_chain', '{"chains": [{"from_rank": 2, "to_rank": 3}, {"from_rank": 1, "to_rank": 2}], "last_class_excluded": true, "day_match": "any_day"}', 'SAT Verbal Rank 2→3, 1→2'),
('a0000000-0000-0000-0000-000000000005', 'Teacher Case by Case', 'teacher_case_by_case', '{"auto_assign": false, "requires_teacher_approval": true}', 'Reading Mastery, Knock Out, etc.'),
('a0000000-0000-0000-0000-000000000006', 'Rank 5 to Rank 4 Chain', 'rank_chain', '{"chains": [{"from_rank": 5, "to_rank": 4}], "last_class_excluded": true, "day_match": "any_day"}', 'SAT Reading/Writing Rank 5 uses Rank 4 schedule'),
('a0000000-0000-0000-0000-000000000007', 'Rank 4 to Rank 5 Chain', 'rank_chain', '{"chains": [{"from_rank": 4, "to_rank": 5}], "last_class_excluded": true, "day_match": "any_day"}', 'SAT Reading/Writing Rank 4 uses Rank 5 schedule'),
('a0000000-0000-0000-0000-000000000008', 'Rank 3 Cross-Section', 'cross_section', '{"section_match": "cross_section", "occurrence_match": "same_occurrence_number", "day_match": "any", "last_class_excluded": true, "schedule_source": "teacher_rank_3_table"}', 'SAT Verbal Rank 3 with teacher schedule');

-- +goose Down

ALTER TABLE root_course_groups DROP COLUMN IF EXISTS sit_in_rule_id;

DROP INDEX IF EXISTS idx_root_course_groups_sit_in_rule;
DROP INDEX IF EXISTS idx_sit_in_rules_type;
DROP INDEX IF EXISTS idx_sit_in_rules_name;

DROP TABLE IF EXISTS sit_in_rules;
