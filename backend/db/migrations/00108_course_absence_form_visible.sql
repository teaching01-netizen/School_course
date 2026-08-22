-- +goose Up
-- Per-course visibility switch for the student absence form. Default true
-- keeps every existing course selectable; setting it to false hides the
-- course from the student self-service listing AND hard-blocks student
-- submissions for it (staff flows are not affected).

ALTER TABLE courses
    ADD COLUMN IF NOT EXISTS absence_form_visible boolean NOT NULL DEFAULT true;

-- +goose Down

ALTER TABLE courses
    DROP COLUMN IF EXISTS absence_form_visible;
