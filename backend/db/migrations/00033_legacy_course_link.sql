-- +goose Up

ALTER TABLE courses
  ADD COLUMN IF NOT EXISTS legacy_course_id text NULL,
  ADD COLUMN IF NOT EXISTS legacy_last_synced_at timestamptz NULL;

-- +goose Down

ALTER TABLE courses
  DROP COLUMN IF EXISTS legacy_last_synced_at,
  DROP COLUMN IF EXISTS legacy_course_id;
