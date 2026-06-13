-- +goose Up
ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS assigned_course_enrollment_created boolean NOT NULL DEFAULT false;

ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS source_course_enrollment_removed boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE IF EXISTS crm_cross_study_assignments
  DROP COLUMN IF EXISTS source_course_enrollment_removed;

ALTER TABLE IF EXISTS crm_cross_study_assignments
  DROP COLUMN IF EXISTS assigned_course_enrollment_created;
