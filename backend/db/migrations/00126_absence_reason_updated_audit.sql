-- +goose Up

ALTER TABLE absence_audit_log
  DROP CONSTRAINT IF EXISTS absence_audit_log_action_check,
  ADD CONSTRAINT absence_audit_log_action_check
    CHECK (action IN (
      'submitted',
      'reviewed',
      'reopened',
      'actioned',
      'cancelled',
      'sit_in_overridden',
      'note_added',
      'created_by_staff',
      'special_approved',
      'reason_updated'
    ));

-- +goose Down

ALTER TABLE absence_audit_log
  DROP CONSTRAINT IF EXISTS absence_audit_log_action_check,
  ADD CONSTRAINT absence_audit_log_action_check
    CHECK (action IN (
      'submitted',
      'reviewed',
      'reopened',
      'actioned',
      'cancelled',
      'sit_in_overridden',
      'note_added',
      'created_by_staff',
      'special_approved'
    ));
