-- +goose Up
-- course_cohorts groups co-teaching courses that share the same student cohort.
-- Admin creates cohorts self-service via the course edit UI; the name is
-- free-text (e.g. "Rank 1", "Rank 3 S2").
CREATE TABLE course_cohorts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE courses ADD COLUMN cohort_id uuid REFERENCES course_cohorts(id);
CREATE INDEX idx_courses_cohort_id ON courses(cohort_id);

-- +goose Down
DROP INDEX IF EXISTS idx_courses_cohort_id;
ALTER TABLE courses DROP COLUMN IF EXISTS cohort_id;
DROP TABLE IF EXISTS course_cohorts;
