-- +goose Up

ALTER TABLE courses
  ADD COLUMN IF NOT EXISTS cycle_id text REFERENCES crm_cycles(id),
  ADD COLUMN IF NOT EXISTS level smallint;

DROP INDEX IF EXISTS idx_courses_level_order_per_subject;

ALTER TABLE courses
  DROP COLUMN IF EXISTS course_level,
  DROP COLUMN IF EXISTS level_order;

CREATE UNIQUE INDEX IF NOT EXISTS idx_courses_subj_cycle_level
  ON courses(subject_id, cycle_id, level)
  WHERE subject_id IS NOT NULL AND cycle_id IS NOT NULL AND level IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_courses_subj_cycle_level;

ALTER TABLE courses
  ADD COLUMN IF NOT EXISTS course_level text NULL CHECK (course_level IN ('beginner', 'intermediate', 'advanced')),
  ADD COLUMN IF NOT EXISTS level_order smallint NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_courses_level_order_per_subject
  ON courses(subject_id, level_order)
  WHERE level_order IS NOT NULL AND subject_id IS NOT NULL;

ALTER TABLE courses
  DROP COLUMN IF EXISTS level,
  DROP COLUMN IF EXISTS cycle_id;
