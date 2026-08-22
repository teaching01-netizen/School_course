-- +goose Up

ALTER TABLE student_absences
    ADD COLUMN IF NOT EXISTS merge_group_id uuid REFERENCES course_merge_groups(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS ix_student_absences_wcode_merge_group_id
    ON student_absences(wcode, merge_group_id);

-- +goose Down

DROP INDEX IF EXISTS ix_student_absences_wcode_merge_group_id;

ALTER TABLE student_absences
    DROP COLUMN IF EXISTS merge_group_id;
