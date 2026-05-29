-- +goose Up
CREATE TABLE root_course_groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE courses ADD COLUMN root_course_group_id uuid REFERENCES root_course_groups(id);
CREATE INDEX idx_courses_root_course_group ON courses(root_course_group_id);

-- +goose Down
DROP INDEX IF EXISTS idx_courses_root_course_group;
ALTER TABLE courses DROP COLUMN IF EXISTS root_course_group_id;
DROP TABLE IF EXISTS root_course_groups;
