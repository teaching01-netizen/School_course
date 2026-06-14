-- +goose Up

CREATE TABLE IF NOT EXISTS sat_verbal_policy_mappings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id text NOT NULL,
    course_id uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    policy_rule jsonb NOT NULL,
    policy_hash text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(rule_id),
    UNIQUE(course_id)
);

CREATE INDEX IF NOT EXISTS idx_sat_verbal_policy_mappings_course_active
    ON sat_verbal_policy_mappings(course_id)
    WHERE active;

-- +goose Down

DROP INDEX IF EXISTS idx_sat_verbal_policy_mappings_course_active;
DROP TABLE IF EXISTS sat_verbal_policy_mappings;
