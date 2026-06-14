-- +goose Up
ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS dest_course_a_weekdays smallint[] NOT NULL DEFAULT ARRAY[1,2,3,4,5,6,7]::smallint[];

ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS dest_course_b_weekdays smallint[] NOT NULL DEFAULT ARRAY[1,2,3,4,5,6,7]::smallint[];

ALTER TABLE IF EXISTS session_attendance
  ADD COLUMN IF NOT EXISTS override_source text NULL
  CHECK (override_source IS NULL OR override_source IN ('manual', 'cross_study'));

ALTER TABLE IF EXISTS session_attendance
  ADD COLUMN IF NOT EXISTS cross_study_assignment_id uuid NULL REFERENCES crm_cross_study_assignments(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS session_attendance_cross_study_assignment_idx
  ON session_attendance(cross_study_assignment_id)
  WHERE cross_study_assignment_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS session_attendance_cross_study_assignment_idx;

ALTER TABLE IF EXISTS session_attendance
  DROP COLUMN IF EXISTS cross_study_assignment_id;

ALTER TABLE IF EXISTS session_attendance
  DROP COLUMN IF EXISTS override_source;

ALTER TABLE IF EXISTS crm_cross_study_assignments
  DROP COLUMN IF EXISTS dest_course_b_weekdays;

ALTER TABLE IF EXISTS crm_cross_study_assignments
  DROP COLUMN IF EXISTS dest_course_a_weekdays;
