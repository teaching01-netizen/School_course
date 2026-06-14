-- +goose NO TRANSACTION
-- +goose Up
ALTER TYPE crm_job_type ADD VALUE IF NOT EXISTS 'cross_study_process';

ALTER TABLE IF EXISTS crm_rows ADD COLUMN IF NOT EXISTS extra_note text NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS crm_cross_study_assignments (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  snapshot_id         uuid NOT NULL REFERENCES crm_snapshots(id),
  wcode               text NOT NULL,
  source_course_id    uuid NOT NULL REFERENCES courses(id),
  dest_course_a_id    uuid NOT NULL REFERENCES courses(id),
  dest_course_b_id    uuid NOT NULL REFERENCES courses(id),
  assigned_course_id  uuid NOT NULL REFERENCES courses(id),
  extra_note_snapshot text NOT NULL DEFAULT '',
  extra_note_hash     text NOT NULL DEFAULT '',
  assigned_course_enrollment_created boolean NOT NULL DEFAULT false,
  dest_course_a_enrollment_created boolean NOT NULL DEFAULT false,
  dest_course_b_enrollment_created boolean NOT NULL DEFAULT false,
  source_course_enrollment_removed boolean NOT NULL DEFAULT false,
  source_valid        boolean NOT NULL DEFAULT true,
  status              text NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active', 'notes_changed', 'orphaned', 'pending')),
  deleted_at          timestamptz,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  UNIQUE (wcode, source_course_id)
);

ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS assigned_course_enrollment_created boolean NOT NULL DEFAULT false;

ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS dest_course_a_enrollment_created boolean NOT NULL DEFAULT false;

ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS dest_course_b_enrollment_created boolean NOT NULL DEFAULT false;

ALTER TABLE IF EXISTS crm_cross_study_assignments
  ADD COLUMN IF NOT EXISTS source_course_enrollment_removed boolean NOT NULL DEFAULT false;

ALTER TABLE IF EXISTS course_roster_overrides
  ADD COLUMN IF NOT EXISTS override_source text NULL
  CHECK (override_source IS NULL OR override_source IN ('manual', 'cross_study'));

ALTER TABLE IF EXISTS course_roster_overrides
  ADD COLUMN IF NOT EXISTS cross_study_assignment_id uuid NULL REFERENCES crm_cross_study_assignments(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS course_roster_overrides_cross_study_assignment_idx
  ON course_roster_overrides(cross_study_assignment_id)
  WHERE cross_study_assignment_id IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS course_roster_overrides_cross_study_assignment_idx;
ALTER TABLE IF EXISTS course_roster_overrides DROP COLUMN IF EXISTS cross_study_assignment_id;
ALTER TABLE IF EXISTS course_roster_overrides DROP COLUMN IF EXISTS override_source;
DROP TABLE IF EXISTS crm_cross_study_assignments;
ALTER TABLE IF EXISTS crm_rows DROP COLUMN IF EXISTS extra_note;
-- Enum value removal requires creating a new type and migrating, omitted for simplicity.
