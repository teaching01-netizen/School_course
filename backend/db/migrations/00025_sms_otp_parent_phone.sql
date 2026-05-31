-- +goose Up

ALTER TABLE students
  ADD COLUMN IF NOT EXISTS parent_phone text NULL;

UPDATE students s
SET parent_phone = c.parent_phone
FROM (
  SELECT DISTINCT ON (wcode)
    wcode,
    parent_phone
  FROM crm_rows
  WHERE parent_phone IS NOT NULL
    AND parent_phone <> ''
  ORDER BY wcode, imported_at DESC NULLS LAST
) c
WHERE s.wcode = c.wcode
  AND (s.parent_phone IS NULL OR s.parent_phone = '');

ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS student_absences_pending_otp_excl;

-- One-time cleanup for any legacy pending_otp rows created before this rewrite.
UPDATE student_absences
SET status = 'pending',
    updated_at = now()
WHERE status = 'pending_otp';

ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS student_absences_status_check;

ALTER TABLE student_absences
  ADD CONSTRAINT student_absences_status_check
  CHECK (status IN ('pending', 'reviewed', 'actioned', 'cancelled'));

ALTER TABLE student_absences
  DROP COLUMN IF EXISTS otp_code_hash,
  DROP COLUMN IF EXISTS otp_attempt_count,
  DROP COLUMN IF EXISTS otp_locked_until,
  DROP COLUMN IF EXISTS otp_last_sent_at,
  DROP COLUMN IF EXISTS otp_code_expires_at;

ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS chk_absence_dates;

ALTER TABLE student_absences
  ADD CONSTRAINT chk_absence_dates
  CHECK (date_from <= date_to);

CREATE TABLE IF NOT EXISTS student_parent_verification_sessions (
  id uuid PRIMARY KEY,
  wcode text NOT NULL,
  parent_phone text NOT NULL,
  status text NOT NULL CHECK (status IN ('pending', 'verified', 'consumed', 'cancelled')),
  otp_code_hash text NULL,
  otp_attempt_count integer NOT NULL DEFAULT 0,
  otp_locked_until timestamptz NULL,
  otp_last_sent_at timestamptz NULL,
  otp_code_expires_at timestamptz NULL,
  verified_at timestamptz NULL,
  consumed_at timestamptz NULL,
  consumed_absence_id uuid NULL REFERENCES student_absences(id) ON DELETE SET NULL,
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS student_parent_verification_sessions_wcode_idx
  ON student_parent_verification_sessions (wcode, created_at DESC);

CREATE INDEX IF NOT EXISTS student_parent_verification_sessions_status_idx
  ON student_parent_verification_sessions (status, created_at DESC);

CREATE TABLE IF NOT EXISTS student_otp_lockouts (
  wcode text PRIMARY KEY,
  locked_until timestamptz NOT NULL,
  failure_count integer NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS http_rate_limit_events (
  key text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS http_rate_limit_events_key_created_at_idx
  ON http_rate_limit_events (key, created_at DESC);

CREATE TABLE IF NOT EXISTS sms_circuit_breaker_state (
  provider text PRIMARY KEY,
  failure_count integer NOT NULL DEFAULT 0,
  window_started_at timestamptz NOT NULL DEFAULT now(),
  open_until timestamptz NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

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
      'otp_sent',
      'otp_verified',
      'otp_failed',
      'otp_locked'
    )
  );

-- +goose Down

UPDATE student_absences
SET status = 'pending_otp',
    updated_at = now()
WHERE status = 'pending';

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

DROP INDEX IF EXISTS http_rate_limit_events_key_created_at_idx;
DROP TABLE IF EXISTS http_rate_limit_events;
DROP TABLE IF EXISTS sms_circuit_breaker_state;
DROP TABLE IF EXISTS student_otp_lockouts;

DROP INDEX IF EXISTS student_parent_verification_sessions_status_idx;
DROP INDEX IF EXISTS student_parent_verification_sessions_wcode_idx;
DROP TABLE IF EXISTS student_parent_verification_sessions;

ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS chk_absence_dates;

ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS student_absences_status_check;

ALTER TABLE student_absences
  ADD CONSTRAINT student_absences_status_check
  CHECK (status IN ('pending_otp', 'pending', 'reviewed', 'actioned', 'cancelled'));

ALTER TABLE student_absences
  ADD COLUMN IF NOT EXISTS otp_code_hash text NULL,
  ADD COLUMN IF NOT EXISTS otp_attempt_count integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS otp_locked_until timestamptz NULL,
  ADD COLUMN IF NOT EXISTS otp_last_sent_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS otp_code_expires_at timestamptz NULL;

ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS student_absences_pending_otp_excl;

ALTER TABLE student_absences
  ADD CONSTRAINT student_absences_pending_otp_excl
  EXCLUDE USING gist (
    wcode WITH =,
    daterange(date_from, date_to, '[]') WITH &&
  )
  WHERE (status = 'pending_otp');

ALTER TABLE students
  DROP COLUMN IF EXISTS parent_phone;
