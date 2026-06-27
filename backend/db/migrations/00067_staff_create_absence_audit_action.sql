-- +goose Up

-- Add 'created_by_staff' to the allowed absence_audit_log action check.
-- This is needed because the staff-side absence creation endpoint
-- (handleStaffCreateAbsence) records the action as 'created_by_staff'.

ALTER TABLE absence_audit_log
  DROP CONSTRAINT IF EXISTS absence_audit_log_action_check;

ALTER TABLE absence_audit_log
  ADD CONSTRAINT absence_audit_log_action_check
  CHECK (
    action IN (
      'submitted',
      'reviewed',
      'reopened',
      'actioned',
      'cancelled',
      'sit_in_overridden',
      'note_added',
      'created_by_staff'
    )
  );

-- +goose Down

ALTER TABLE absence_audit_log
  DROP CONSTRAINT IF EXISTS absence_audit_log_action_check;

ALTER TABLE absence_audit_log
  ADD CONSTRAINT absence_audit_log_action_check
  CHECK (
    action IN (
      'submitted',
      'reviewed',
      'reopened',
      'actioned',
      'cancelled',
      'sit_in_overridden',
      'note_added'
    )
  );
