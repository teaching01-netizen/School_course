-- +goose Up

-- Legacy roster import (students from the old site's course attendee rows)
-- writes into students/course_students and is opt-in: it must be enabled
-- explicitly from the Legacy Sync admin UI.
ALTER TABLE legacy_sync_controls
    ADD COLUMN student_enabled boolean NOT NULL DEFAULT false;

-- +goose Down

ALTER TABLE legacy_sync_controls
    DROP COLUMN IF EXISTS student_enabled;