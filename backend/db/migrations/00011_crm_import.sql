-- +goose Up

-- Cycles config: populated from distinct cycle_label values in uploaded data.
-- Provides dropdown options for course filter.
CREATE TABLE IF NOT EXISTS crm_cycles (
  id               text PRIMARY KEY,
  label            text NOT NULL,
  last_imported_at timestamptz NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

-- Imported CRM rows (single authoritative snapshot; replaced atomically on each upload).
CREATE TABLE IF NOT EXISTS crm_rows (
  row_hash             text NOT NULL,

  cycle_label          text NOT NULL,
  course_name          text NOT NULL,
  wcode                text NOT NULL,

  first_name           text NULL,
  last_name            text NULL,
  nickname             text NULL,
  secondary_school     text NULL,
  academic_level       text NULL,
  mobile_phone         text NULL,
  hours                integer NULL CHECK (hours IS NULL OR hours >= 0),
  teachers_raw         text NULL,
  primary_email        text NULL,
  parent_name          text NULL,
  parent_phone         text NULL,
  parent_email         text NULL,
  order_quote_updated_at timestamptz NULL,

  imported_at          timestamptz NOT NULL DEFAULT now(),

  PRIMARY KEY (row_hash)
);

CREATE INDEX IF NOT EXISTS crm_rows_course_name_idx ON crm_rows(course_name);
CREATE INDEX IF NOT EXISTS crm_rows_wcode_idx ON crm_rows(wcode);
CREATE INDEX IF NOT EXISTS crm_rows_cycle_label_idx ON crm_rows(cycle_label);

-- Course-level CRM filter definition + flags + lock.
ALTER TABLE courses
  ADD COLUMN IF NOT EXISTS crm_filter_enabled boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS crm_filter jsonb NULL,
  ADD COLUMN IF NOT EXISTS crm_filter_updated_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS crm_roster_locked boolean NOT NULL DEFAULT false;

-- +goose Down

ALTER TABLE courses
  DROP COLUMN IF EXISTS crm_roster_locked,
  DROP COLUMN IF EXISTS crm_filter_updated_at,
  DROP COLUMN IF EXISTS crm_filter,
  DROP COLUMN IF EXISTS crm_filter_enabled;

DROP TABLE IF EXISTS crm_rows;
DROP TABLE IF EXISTS crm_cycles;
