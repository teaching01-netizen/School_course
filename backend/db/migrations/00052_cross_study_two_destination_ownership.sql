-- +goose Up
ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS dest_course_a_enrollment_created boolean NOT NULL DEFAULT false;

ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS dest_course_b_enrollment_created boolean NOT NULL DEFAULT false;

UPDATE crm_cross_study_assignments
SET dest_course_a_enrollment_created = assigned_course_enrollment_created
WHERE assigned_course_id = dest_course_a_id
  AND dest_course_a_enrollment_created = false;

UPDATE crm_cross_study_assignments
SET dest_course_b_enrollment_created = assigned_course_enrollment_created
WHERE assigned_course_id = dest_course_b_id
  AND dest_course_b_enrollment_created = false;

-- +goose Down
ALTER TABLE IF EXISTS crm_cross_study_assignments
  DROP COLUMN IF EXISTS dest_course_b_enrollment_created;

ALTER TABLE IF EXISTS crm_cross_study_assignments
  DROP COLUMN IF EXISTS dest_course_a_enrollment_created;
