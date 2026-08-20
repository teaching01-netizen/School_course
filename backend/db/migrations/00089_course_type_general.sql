-- +goose Up
-- The legacy site now publishes courses with type "General" (visible in the
-- course list and detail pages). Accept it alongside Private/Group so the
-- legacy sync can apply those courses instead of dead-lettering them.

ALTER TABLE courses DROP CONSTRAINT IF EXISTS courses_course_type_check;
ALTER TABLE courses
  ADD CONSTRAINT courses_course_type_check
  CHECK (course_type IS NULL OR course_type IN ('Private', 'Group', 'General'));

-- +goose Down

ALTER TABLE courses DROP CONSTRAINT IF EXISTS courses_course_type_check;
ALTER TABLE courses
  ADD CONSTRAINT courses_course_type_check
  CHECK (course_type IS NULL OR course_type IN ('Private', 'Group'));
UPDATE courses SET course_type = NULL WHERE course_type = 'General';
