-- +goose Up

-- Normalize existing data: lowercase all wcodes to prevent case-sensitive duplicates.
UPDATE students SET wcode = LOWER(TRIM(wcode)) WHERE wcode <> LOWER(TRIM(wcode));
UPDATE crm_rows SET wcode = LOWER(TRIM(wcode)) WHERE wcode <> LOWER(TRIM(wcode));

-- Add a case-insensitive unique index as a durability safety net.
-- The application layer normalizes wcode before insert, but this catches
-- any code path that slips through.
CREATE UNIQUE INDEX IF NOT EXISTS idx_students_wcode_lower_unique
  ON students (LOWER(wcode));

-- +goose Down
DROP INDEX IF EXISTS idx_students_wcode_lower_unique;
