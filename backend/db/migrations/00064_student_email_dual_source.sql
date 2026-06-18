-- +goose Up

-- Add CRM- and system-sourced email columns alongside the existing email.
ALTER TABLE students
  ADD COLUMN IF NOT EXISTS email_crm text NULL,
  ADD COLUMN IF NOT EXISTS email_system text NULL;

-- The existing email column was always populated from CRM import.
-- Backfill email_crm so we don't lose existing data.
UPDATE students SET email_crm = email WHERE email IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_students_email_crm ON students(email_crm);
CREATE INDEX IF NOT EXISTS idx_students_email_system ON students(email_system);

-- +goose Down

DROP INDEX IF EXISTS idx_students_email_system;
DROP INDEX IF EXISTS idx_students_email_crm;

ALTER TABLE students
  DROP COLUMN IF EXISTS email_system,
  DROP COLUMN IF EXISTS email_crm;
