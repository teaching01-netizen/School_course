-- +goose Up

-- Some production environments had an older sat_verbal_policy_mappings shape
-- with denormalized report columns. Runtime code derives subject/root data from
-- the mapped course, so those legacy columns must not block inserts.

-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'subject_id'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ALTER COLUMN subject_id DROP NOT NULL;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'root_course_group_id'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ALTER COLUMN root_course_group_id DROP NOT NULL;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'warnings'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ALTER COLUMN warnings SET DEFAULT '[]',
      ALTER COLUMN warnings DROP NOT NULL;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'matched_courses'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ALTER COLUMN matched_courses SET DEFAULT '[]',
      ALTER COLUMN matched_courses DROP NOT NULL;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'unmatched_policy_rows'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ALTER COLUMN unmatched_policy_rows SET DEFAULT '[]',
      ALTER COLUMN unmatched_policy_rows DROP NOT NULL;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'unmatched_courses'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ALTER COLUMN unmatched_courses SET DEFAULT '[]',
      ALTER COLUMN unmatched_courses DROP NOT NULL;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down

-- Down migration is intentionally a no-op: this only relaxes obsolete
-- denormalized columns that current application code does not require.
