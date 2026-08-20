-- +goose Up
-- absence_sit_in_assignment_events is append-only (00073): its trigger rejects
-- every UPDATE and DELETE. That makes any FK action that mutates or removes its
-- rows a hard failure:
--   * 00075 declared previous_session_id / new_session_id as ON DELETE SET
--     NULL, and SET NULL is an RI UPDATE on this very table -> "session change
--     history is append-only" on every session delete.
--   * absence_id ON DELETE CASCADE issues an RI DELETE -> same failure on every
--     absence delete. History is meant to survive absence deletion (see the
--     resolution test fixtures, which rely on this).
-- session_changes already keeps soft references without FKs; do the same for
-- the event history table.

ALTER TABLE absence_sit_in_assignment_events
  DROP CONSTRAINT IF EXISTS absence_sit_in_assignment_events_previous_session_id_fkey,
  DROP CONSTRAINT IF EXISTS absence_sit_in_assignment_events_new_session_id_fkey,
  DROP CONSTRAINT IF EXISTS absence_sit_in_assignment_events_absence_id_fkey;

-- +goose Down

ALTER TABLE absence_sit_in_assignment_events
  ADD CONSTRAINT absence_sit_in_assignment_events_previous_session_id_fkey
    FOREIGN KEY (previous_session_id) REFERENCES sessions(id) ON DELETE SET NULL,
  ADD CONSTRAINT absence_sit_in_assignment_events_new_session_id_fkey
    FOREIGN KEY (new_session_id) REFERENCES sessions(id) ON DELETE SET NULL,
  ADD CONSTRAINT absence_sit_in_assignment_events_absence_id_fkey
    FOREIGN KEY (absence_id) REFERENCES student_absences(id) ON DELETE CASCADE;
