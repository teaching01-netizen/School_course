-- +goose Up

ALTER TABLE students
  ADD COLUMN IF NOT EXISTS school text NULL;

-- +goose Down

ALTER TABLE students
  DROP COLUMN IF EXISTS school;
