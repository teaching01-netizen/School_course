-- +goose Up

CREATE TABLE sat_verbal_policy_mappings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    policy jsonb NOT NULL,
    policy_hash text NOT NULL,
    warnings jsonb NOT NULL DEFAULT '[]'::jsonb,
    matched_courses jsonb NOT NULL DEFAULT '[]'::jsonb,
    unmatched_policy_rows jsonb NOT NULL DEFAULT '[]'::jsonb,
    unmatched_courses jsonb NOT NULL DEFAULT '[]'::jsonb,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(subject_id)
);

CREATE INDEX idx_sat_verbal_policy_mappings_subject_active
    ON sat_verbal_policy_mappings(subject_id)
    WHERE active;

-- +goose Down

DROP INDEX IF EXISTS idx_sat_verbal_policy_mappings_subject_active;
DROP TABLE IF EXISTS sat_verbal_policy_mappings;
