-- +goose Up

ALTER TABLE student_absences
  ADD COLUMN IF NOT EXISTS sit_in_method text NULL CHECK (sit_in_method IN ('physical', 'zoom')),
  ADD COLUMN IF NOT EXISTS subject_id uuid NULL REFERENCES subjects(id);

CREATE TABLE IF NOT EXISTS absence_sit_ins (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  absence_id  uuid NOT NULL REFERENCES student_absences(id) ON DELETE CASCADE,
  session_id  uuid NOT NULL REFERENCES sessions(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE(absence_id, session_id)
);

ALTER TABLE app_settings
  ADD COLUMN IF NOT EXISTS absence_policies jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_absence_sit_ins_absence ON absence_sit_ins(absence_id);
CREATE INDEX IF NOT EXISTS idx_absence_sit_ins_session ON absence_sit_ins(session_id);

-- +goose Down

ALTER TABLE app_settings
  DROP COLUMN IF EXISTS absence_policies;

DROP TABLE IF EXISTS absence_sit_ins;

ALTER TABLE student_absences
  DROP COLUMN IF EXISTS subject_id,
  DROP COLUMN IF EXISTS sit_in_method;
