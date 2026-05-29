-- +goose Up
ALTER TABLE root_course_groups DROP COLUMN IF EXISTS subject_id;
ALTER TABLE root_course_groups DROP COLUMN IF EXISTS code;

-- +goose Down
ALTER TABLE root_course_groups ADD COLUMN code text NOT NULL;
ALTER TABLE root_course_groups ADD COLUMN subject_id uuid NOT NULL REFERENCES subjects(id);
