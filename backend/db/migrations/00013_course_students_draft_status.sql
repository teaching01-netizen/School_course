-- +goose Up

ALTER TABLE course_students
  ADD COLUMN status text NOT NULL DEFAULT 'enrolled'
  CHECK (status IN ('draft', 'enrolled'));

COMMENT ON COLUMN course_students.status IS 'enrolled: confirmed, paid student (default). draft: tentative prospect, blocks busy ranges.';

-- +goose Down

ALTER TABLE course_students DROP COLUMN IF EXISTS status;
