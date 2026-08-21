-- +goose Up
CREATE INDEX IF NOT EXISTS idx_subjects_code_id ON subjects (code, id);
CREATE INDEX IF NOT EXISTS idx_courses_subject_code_id ON courses (subject_id, code, id);
CREATE INDEX IF NOT EXISTS idx_courses_subject_cycle_level_code_id ON courses (subject_id, cycle_id, level, code, id);

-- +goose Down
DROP INDEX IF EXISTS idx_courses_subject_cycle_level_code_id;
DROP INDEX IF EXISTS idx_courses_subject_code_id;
DROP INDEX IF EXISTS idx_subjects_code_id;
