-- +goose Up

CREATE TABLE IF NOT EXISTS course_merge_groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE CHECK (btrim(name) <> ''),
    created_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS course_merge_group_members (
    group_id uuid NOT NULL REFERENCES course_merge_groups(id) ON DELETE CASCADE,
    course_id uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    position smallint NOT NULL CHECK (position IN (1, 2)),
    PRIMARY KEY (group_id, course_id),
    UNIQUE (course_id)
);

CREATE INDEX IF NOT EXISTS ix_course_merge_group_members_group_id
    ON course_merge_group_members(group_id, position);

-- +goose Down

DROP TABLE IF EXISTS course_merge_group_members;
DROP TABLE IF EXISTS course_merge_groups;
