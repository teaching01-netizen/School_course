-- +goose Up

CREATE TABLE IF NOT EXISTS sms_otp_deliveries (
  id uuid PRIMARY KEY,
  session_id uuid NOT NULL REFERENCES student_parent_verification_sessions(id) ON DELETE CASCADE,
  status text NOT NULL DEFAULT 'queued'
    CHECK (status IN ('queued', 'preparing', 'submitting', 'accepted', 'retryable', 'failed', 'uncertain', 'expired')),
  campaign_id text NOT NULL UNIQUE,
  key_version text NULL,
  payload_nonce bytea NULL,
  encrypted_payload bytea NULL,
  attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  run_after timestamptz NOT NULL DEFAULT now(),
  locked_by text NULL,
  locked_until timestamptz NULL,
  submitting_at timestamptz NULL,
  accepted_at timestamptz NULL,
  failed_at timestamptz NULL,
  uncertain_at timestamptz NULL,
  expires_at timestamptz NOT NULL,
  error_code text NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS sms_otp_deliveries_claim_idx
  ON sms_otp_deliveries (run_after, created_at)
  WHERE status IN ('queued', 'retryable', 'preparing');

CREATE INDEX IF NOT EXISTS sms_otp_deliveries_session_idx
  ON sms_otp_deliveries (session_id, created_at DESC);

CREATE INDEX IF NOT EXISTS sms_otp_deliveries_stale_submitting_idx
  ON sms_otp_deliveries (locked_until)
  WHERE status = 'submitting';

CREATE INDEX IF NOT EXISTS sms_otp_deliveries_expiry_idx
  ON sms_otp_deliveries (expires_at)
  WHERE status IN ('queued', 'preparing', 'retryable', 'uncertain');

CREATE UNIQUE INDEX IF NOT EXISTS sms_otp_deliveries_one_active_per_session_idx
  ON sms_otp_deliveries (session_id)
  WHERE status IN ('queued', 'preparing', 'submitting', 'retryable');

-- +goose Down

DROP TABLE IF EXISTS sms_otp_deliveries;
