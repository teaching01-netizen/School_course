-- +goose Up

CREATE TABLE IF NOT EXISTS sit_in_priorities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    root_course_group_id uuid NOT NULL REFERENCES root_course_groups(id) ON DELETE CASCADE,
    sit_in_rule_id uuid NOT NULL REFERENCES sit_in_rules(id),
    priority_level smallint NOT NULL CHECK (priority_level BETWEEN 1 AND 3),
    label text NOT NULL,
    target_rank smallint,
    target_section smallint,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(root_course_group_id, priority_level)
);

CREATE INDEX IF NOT EXISTS idx_sit_in_priorities_root_group ON sit_in_priorities(root_course_group_id);
CREATE INDEX IF NOT EXISTS idx_sit_in_priorities_rule ON sit_in_priorities(sit_in_rule_id);

-- +goose Down

DROP TABLE IF EXISTS sit_in_priorities;
