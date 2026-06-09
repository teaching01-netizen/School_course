-- +goose Up

-- The sat_verbal_policy_mappings table exists in production but is missing
-- the course_id column (migration 00038 was not fully applied).
-- This migration adds the column and its constraints/index idempotently.

DO $$
BEGIN
  -- Add course_id column if missing
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'course_id'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD COLUMN course_id uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE;

    ALTER TABLE sat_verbal_policy_mappings
      ADD CONSTRAINT sat_verbal_policy_mappings_course_id_unique UNIQUE (course_id);

    CREATE INDEX IF NOT EXISTS idx_sat_verbal_policy_mappings_course_active
      ON sat_verbal_policy_mappings(course_id)
      WHERE active;
  END IF;
END $$;

-- +goose Down

-- Down migration is intentionally a no-op: we never want to drop the
-- course_id column that the application requires.
