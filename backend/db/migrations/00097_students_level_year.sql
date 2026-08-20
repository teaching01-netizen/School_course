-- +goose Up

-- Student academic level and admission year, synced from the old site's
-- /Admin/Students page (Level=AcademicLevel, Year=AdmissionYear). Filled in
-- by the legacy student sync (and Level also by CRM import) using
-- fill-in-if-empty semantics, so human edits and CRM values win.
ALTER TABLE students
  ADD COLUMN IF NOT EXISTS level text NULL,
  ADD COLUMN IF NOT EXISTS year text NULL;

-- +goose Down

ALTER TABLE students
  DROP COLUMN IF EXISTS year,
  DROP COLUMN IF EXISTS level;