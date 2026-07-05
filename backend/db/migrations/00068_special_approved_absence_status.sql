-- +goose Up

ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS student_absences_status_check,
  ADD CONSTRAINT student_absences_status_check
    CHECK (status IN ('pending', 'reviewed', 'actioned', 'cancelled', 'special_approved'));

ALTER TABLE absence_audit_log
  DROP CONSTRAINT IF EXISTS absence_audit_log_action_check,
  ADD CONSTRAINT absence_audit_log_action_check
    CHECK (action IN ('submitted', 'reviewed', 'reopened', 'actioned', 'cancelled', 'sit_in_overridden', 'note_added', 'created_by_staff', 'special_approved'));

-- +goose Down

ALTER TABLE absence_audit_log
  DROP CONSTRAINT IF EXISTS absence_audit_log_action_check,
  ADD CONSTRAINT absence_audit_log_action_check
    CHECK (action IN ('submitted', 'reviewed', 'reopened', 'actioned', 'cancelled', 'sit_in_overridden', 'note_added', 'created_by_staff'));

ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS student_absences_status_check,
  ADD CONSTRAINT student_absences_status_check
    CHECK (status IN ('pending', 'reviewed', 'actioned', 'cancelled'));
