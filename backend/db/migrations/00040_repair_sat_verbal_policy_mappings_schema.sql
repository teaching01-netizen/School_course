-- +goose Up

-- Repair environments where 00039 already ran but the pre-existing
-- sat_verbal_policy_mappings table was still missing other columns required by
-- the application queries.

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'id'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD COLUMN id uuid DEFAULT gen_random_uuid();
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'rule_id'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD COLUMN rule_id text;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'course_id'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD COLUMN course_id uuid REFERENCES courses(id) ON DELETE CASCADE;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'policy_rule'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD COLUMN policy_rule jsonb DEFAULT '{}'::jsonb;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'policy_hash'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD COLUMN policy_hash text DEFAULT '';
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'active'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD COLUMN active boolean DEFAULT true;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'created_at'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD COLUMN created_at timestamptz DEFAULT now();
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'sat_verbal_policy_mappings' AND column_name = 'updated_at'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD COLUMN updated_at timestamptz DEFAULT now();
  END IF;

  UPDATE sat_verbal_policy_mappings
  SET id = gen_random_uuid()
  WHERE id IS NULL;

  UPDATE sat_verbal_policy_mappings
  SET rule_id = 'legacy-' || id::text
  WHERE rule_id IS NULL OR btrim(rule_id) = '';

  UPDATE sat_verbal_policy_mappings
  SET policy_rule = '{}'::jsonb
  WHERE policy_rule IS NULL;

  UPDATE sat_verbal_policy_mappings
  SET policy_hash = ''
  WHERE policy_hash IS NULL;

  UPDATE sat_verbal_policy_mappings
  SET active = true
  WHERE active IS NULL;

  UPDATE sat_verbal_policy_mappings
  SET created_at = now()
  WHERE created_at IS NULL;

  UPDATE sat_verbal_policy_mappings
  SET updated_at = now()
  WHERE updated_at IS NULL;

  -- Existing rows without a course_id cannot be mapped to courses by this
  -- application path. Remove them before restoring the expected constraint.
  DELETE FROM sat_verbal_policy_mappings
  WHERE course_id IS NULL;

  ALTER TABLE sat_verbal_policy_mappings
    ALTER COLUMN id SET DEFAULT gen_random_uuid(),
    ALTER COLUMN id SET NOT NULL,
    ALTER COLUMN rule_id SET NOT NULL,
    ALTER COLUMN course_id SET NOT NULL,
    ALTER COLUMN policy_rule SET DEFAULT '{}'::jsonb,
    ALTER COLUMN policy_rule SET NOT NULL,
    ALTER COLUMN policy_hash SET DEFAULT '',
    ALTER COLUMN policy_hash SET NOT NULL,
    ALTER COLUMN active SET DEFAULT true,
    ALTER COLUMN active SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'sat_verbal_policy_mappings'::regclass
      AND conname = 'sat_verbal_policy_mappings_pkey'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD CONSTRAINT sat_verbal_policy_mappings_pkey PRIMARY KEY (id);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'sat_verbal_policy_mappings'::regclass
      AND contype = 'u'
      AND pg_get_constraintdef(oid) = 'UNIQUE (rule_id)'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD CONSTRAINT sat_verbal_policy_mappings_rule_id_unique UNIQUE (rule_id);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'sat_verbal_policy_mappings'::regclass
      AND contype = 'u'
      AND pg_get_constraintdef(oid) = 'UNIQUE (course_id)'
  ) THEN
    ALTER TABLE sat_verbal_policy_mappings
      ADD CONSTRAINT sat_verbal_policy_mappings_course_id_unique UNIQUE (course_id);
  END IF;

  CREATE INDEX IF NOT EXISTS idx_sat_verbal_policy_mappings_course_active
    ON sat_verbal_policy_mappings(course_id)
    WHERE active;
END $$;
-- +goose StatementEnd

-- +goose Down

-- Down migration is intentionally a no-op: this is a production schema repair.
