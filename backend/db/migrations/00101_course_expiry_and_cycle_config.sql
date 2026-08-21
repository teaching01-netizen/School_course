-- +goose Up

ALTER TABLE courses
  ADD COLUMN IF NOT EXISTS expiry_days integer NULL
    CHECK (expiry_days IS NULL OR expiry_days >= 0);

ALTER TABLE crm_cycles
  ADD COLUMN IF NOT EXISTS source_kind text NOT NULL DEFAULT 'imported'
    CHECK (source_kind IN ('imported', 'manual')),
  ADD COLUMN IF NOT EXISTS import_key text NULL,
  ADD COLUMN IF NOT EXISTS display_name text NULL,
  ADD COLUMN IF NOT EXISTS start_date date NULL,
  ADD COLUMN IF NOT EXISTS end_date date NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'crm_cycles_date_range_check'
      AND conrelid = 'crm_cycles'::regclass
  ) THEN
    ALTER TABLE crm_cycles
      ADD CONSTRAINT crm_cycles_date_range_check
      CHECK (start_date IS NULL OR end_date IS NULL OR start_date <= end_date);
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS crm_cycles_import_key_uniq
  ON crm_cycles(import_key)
  WHERE import_key IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS crm_cycles_import_key_uniq;

ALTER TABLE crm_cycles
  DROP CONSTRAINT IF EXISTS crm_cycles_date_range_check,
  DROP COLUMN IF EXISTS end_date,
  DROP COLUMN IF EXISTS start_date,
  DROP COLUMN IF EXISTS display_name,
  DROP COLUMN IF EXISTS import_key,
  DROP COLUMN IF EXISTS source_kind;

ALTER TABLE courses
  DROP COLUMN IF EXISTS expiry_days;
