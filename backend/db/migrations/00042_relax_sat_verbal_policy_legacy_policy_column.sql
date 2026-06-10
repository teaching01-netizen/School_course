-- +goose Up

-- Older production tables may still contain an obsolete `policy` column.
-- Current runtime uses `policy_rule`; the legacy column must not block inserts.

-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'policy'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ALTER COLUMN policy SET DEFAULT '{}',
      ALTER COLUMN policy DROP NOT NULL;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down

-- Down migration is intentionally a no-op: this only relaxes an obsolete
-- denormalized column that current application code does not require.
