-- +goose Up

CREATE TABLE IF NOT EXISTS absence_missed_sessions (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  absence_id  uuid NOT NULL REFERENCES student_absences(id) ON DELETE CASCADE,
  session_id  uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE(absence_id, session_id)
);

CREATE INDEX IF NOT EXISTS idx_absence_missed_sessions_absence ON absence_missed_sessions(absence_id);
CREATE INDEX IF NOT EXISTS idx_absence_missed_sessions_session ON absence_missed_sessions(session_id);

-- +goose Down

DROP TABLE IF EXISTS absence_missed_sessions;
