-- +goose Up

-- Subjects
CREATE TABLE IF NOT EXISTS subjects (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code        text NOT NULL UNIQUE,
  name        text NOT NULL,
  deleted_at  timestamptz NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Seed subjects from existing courses (historical data used "courses" as subjects).
INSERT INTO subjects (code, name, deleted_at, created_at, updated_at)
SELECT c.code, c.name, c.deleted_at, c.created_at, c.updated_at
FROM courses c
ON CONFLICT (code) DO NOTHING;

-- Course fields (nullable for backward compatibility; fill over time).
CREATE SEQUENCE IF NOT EXISTS course_no_seq;

ALTER TABLE courses
  ADD COLUMN IF NOT EXISTS course_no bigint NULL,
  ADD COLUMN IF NOT EXISTS year smallint NULL CHECK (year IS NULL OR (year BETWEEN 0 AND 99)),
  ADD COLUMN IF NOT EXISTS teacher_id uuid NULL REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS subject_id uuid NULL REFERENCES subjects(id),
  ADD COLUMN IF NOT EXISTS hour integer NULL CHECK (hour IS NULL OR hour >= 0),
  ADD COLUMN IF NOT EXISTS student_count integer NULL CHECK (student_count IS NULL OR student_count >= 0),
  ADD COLUMN IF NOT EXISTS course_type text NULL CHECK (course_type IS NULL OR course_type IN ('Private', 'Group'));

-- Backfill course_no for existing rows.
WITH to_fill AS (
  SELECT id
  FROM courses
  WHERE course_no IS NULL
  ORDER BY created_at ASC
)
UPDATE courses c
SET course_no = nextval('course_no_seq')
FROM to_fill f
WHERE c.id = f.id;

ALTER TABLE courses
  ALTER COLUMN course_no SET DEFAULT nextval('course_no_seq'),
  ALTER COLUMN course_no SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS courses_course_no_uniq ON courses(course_no);

-- +goose Down

DROP INDEX IF EXISTS courses_course_no_uniq;

ALTER TABLE courses
  DROP COLUMN IF EXISTS course_type,
  DROP COLUMN IF EXISTS student_count,
  DROP COLUMN IF EXISTS hour,
  DROP COLUMN IF EXISTS subject_id,
  DROP COLUMN IF EXISTS teacher_id,
  DROP COLUMN IF EXISTS year,
  DROP COLUMN IF EXISTS course_no;

DROP SEQUENCE IF EXISTS course_no_seq;

DROP TABLE IF EXISTS subjects;

