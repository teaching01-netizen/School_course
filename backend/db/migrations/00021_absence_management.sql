-- +goose Up

ALTER TABLE student_absences
  ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'reviewed', 'actioned', 'cancelled')),
  ADD COLUMN IF NOT EXISTS reason_category text NULL,
  ADD COLUMN IF NOT EXISTS admin_notes text NULL,
  ADD COLUMN IF NOT EXISTS student_name text NULL,
  ADD COLUMN IF NOT EXISTS student_email text NULL,
  ADD COLUMN IF NOT EXISTS student_phone text NULL,
  ADD COLUMN IF NOT EXISTS reviewed_by uuid NULL REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS reviewed_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS sit_in_overridden boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS sit_in_overridden_by uuid NULL REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS sit_in_override_reason text NULL,
  ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_student_absences_status_created
  ON student_absences(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_student_absences_subject_dates
  ON student_absences(subject_id, date_from, date_to);

CREATE TABLE IF NOT EXISTS absence_audit_log (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  absence_id  uuid NOT NULL REFERENCES student_absences(id) ON DELETE CASCADE,
  action      text NOT NULL CHECK (
    action IN ('submitted', 'reviewed', 'reopened', 'actioned', 'cancelled', 'sit_in_overridden', 'note_added')
  ),
  actor_id    uuid NULL REFERENCES users(id),
  actor_role  text NOT NULL DEFAULT 'admin' CHECK (actor_role IN ('admin', 'student')),
  details     jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_absence_audit_log_absence_created
  ON absence_audit_log(absence_id, created_at DESC);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION reject_absence_audit_mutation()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'absence audit timeline is append-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS absence_audit_log_append_only ON absence_audit_log;
CREATE TRIGGER absence_audit_log_append_only
  BEFORE UPDATE OR DELETE ON absence_audit_log
  FOR EACH ROW EXECUTE FUNCTION reject_absence_audit_mutation();

-- +goose Down

DROP TRIGGER IF EXISTS absence_audit_log_append_only ON absence_audit_log;
DROP FUNCTION IF EXISTS reject_absence_audit_mutation();
DROP TABLE IF EXISTS absence_audit_log;
DROP INDEX IF EXISTS idx_student_absences_subject_dates;
DROP INDEX IF EXISTS idx_student_absences_status_created;

ALTER TABLE student_absences
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS version,
  DROP COLUMN IF EXISTS sit_in_override_reason,
  DROP COLUMN IF EXISTS sit_in_overridden_by,
  DROP COLUMN IF EXISTS sit_in_overridden,
  DROP COLUMN IF EXISTS reviewed_at,
  DROP COLUMN IF EXISTS reviewed_by,
  DROP COLUMN IF EXISTS student_phone,
  DROP COLUMN IF EXISTS student_email,
  DROP COLUMN IF EXISTS student_name,
  DROP COLUMN IF EXISTS admin_notes,
  DROP COLUMN IF EXISTS reason_category,
  DROP COLUMN IF EXISTS status;

