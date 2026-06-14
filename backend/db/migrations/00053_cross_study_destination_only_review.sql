-- +goose Up
ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS crm_course_name_snapshot text NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS crm_row_hash_snapshot text NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS crm_xlsx_row_number_snapshot integer NULL;

-- +goose Down
ALTER TABLE IF EXISTS crm_cross_study_assignments
  DROP COLUMN IF EXISTS crm_xlsx_row_number_snapshot;

ALTER TABLE IF EXISTS crm_cross_study_assignments
  DROP COLUMN IF EXISTS crm_row_hash_snapshot;

ALTER TABLE IF EXISTS crm_cross_study_assignments
  DROP COLUMN IF EXISTS crm_course_name_snapshot;
