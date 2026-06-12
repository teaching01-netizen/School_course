-- +goose Up

ALTER TABLE email_templates ADD COLUMN built_in boolean NOT NULL DEFAULT false;

UPDATE email_templates SET built_in = true WHERE name = 'Sit-in Day Reminder';

-- +goose Down

ALTER TABLE email_templates DROP COLUMN built_in;
