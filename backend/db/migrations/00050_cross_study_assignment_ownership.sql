-- +goose Up
ALTER TABLE IF EXISTS course_roster_overrides
  ADD COLUMN IF NOT EXISTS cross_study_assignment_id uuid NULL REFERENCES crm_cross_study_assignments(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS course_roster_overrides_cross_study_assignment_idx
  ON course_roster_overrides(cross_study_assignment_id)
  WHERE cross_study_assignment_id IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS course_roster_overrides_cross_study_assignment_idx;
ALTER TABLE IF EXISTS course_roster_overrides DROP COLUMN IF EXISTS cross_study_assignment_id;
