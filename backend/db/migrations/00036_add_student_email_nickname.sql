-- +goose Up

ALTER TABLE students ADD COLUMN IF NOT EXISTS email text NULL;
ALTER TABLE students ADD COLUMN IF NOT EXISTS nickname text NULL;

-- +goose Down

ALTER TABLE students DROP COLUMN IF EXISTS email;
ALTER TABLE students DROP COLUMN IF EXISTS nickname;
