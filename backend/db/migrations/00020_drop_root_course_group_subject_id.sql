-- +goose Up
ALTER TABLE root_course_groups DROP COLUMN IF EXISTS subject_id;
ALTER TABLE root_course_groups DROP COLUMN IF EXISTS code;

-- +goose Down
-- NOTE (C2 trust repair): Up drops subject_id/code added by pre-00019 schema. Down intentionally
-- restores them as nullable — the original NOT NULL cannot be restored on
-- populated tables without data loss. Populate subject_id/code manually if
-- rollback is required, then add NOT NULL in a follow-up migration.
ALTER TABLE root_course_groups ADD COLUMN IF NOT EXISTS code text;
ALTER TABLE root_course_groups ADD COLUMN IF NOT EXISTS subject_id uuid REFERENCES subjects(id);
