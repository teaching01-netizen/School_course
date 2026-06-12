-- +goose Up

ALTER TABLE users ADD COLUMN IF NOT EXISTS email text NULL;

ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS sit_in_email_config jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down

ALTER TABLE users DROP COLUMN IF EXISTS email;
ALTER TABLE app_settings DROP COLUMN IF EXISTS sit_in_email_config;
