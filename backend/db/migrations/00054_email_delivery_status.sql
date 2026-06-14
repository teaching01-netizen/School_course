-- +goose Up

ALTER TABLE email_delivery_claims
  ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'accepted',
  ADD COLUMN IF NOT EXISTS attempt_count int NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS sending_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS accepted_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS failed_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS last_error text NULL,
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

UPDATE email_delivery_claims
SET status = 'accepted',
    accepted_at = COALESCE(accepted_at, claimed_at),
    updated_at = COALESCE(updated_at, claimed_at)
WHERE status = 'accepted'
  AND accepted_at IS NULL;

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'email_delivery_claims_status_check'
  ) THEN
    ALTER TABLE email_delivery_claims
      ADD CONSTRAINT email_delivery_claims_status_check
      CHECK (status IN ('pending', 'sending', 'accepted', 'failed'));
  END IF;
END $$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_email_delivery_claims_status
  ON email_delivery_claims (status, sending_at);

-- +goose Down

DROP INDEX IF EXISTS idx_email_delivery_claims_status;

ALTER TABLE email_delivery_claims
  DROP CONSTRAINT IF EXISTS email_delivery_claims_status_check,
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS last_error,
  DROP COLUMN IF EXISTS failed_at,
  DROP COLUMN IF EXISTS accepted_at,
  DROP COLUMN IF EXISTS sending_at,
  DROP COLUMN IF EXISTS attempt_count,
  DROP COLUMN IF EXISTS status;
