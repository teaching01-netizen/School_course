-- +goose Up
-- The legacy sync knows each teacher's display name (e.g. "AJ. REINA
-- (NATIVE)") but users had nowhere to store it, so UIs could only show the
-- synthetic "legacy:<id>" username. Store the display name on the user row.

ALTER TABLE users ADD COLUMN full_name text;

-- +goose Down

ALTER TABLE users DROP COLUMN IF EXISTS full_name;
