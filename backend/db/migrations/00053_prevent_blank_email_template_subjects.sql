-- +goose Up

UPDATE email_templates
SET
    subject = 'Sit-in Reminder - {{sit_in_count}} session(s) today ({{today_date}})',
    updated_at = now()
WHERE btrim(subject) = '';

ALTER TABLE email_templates
    ADD CONSTRAINT email_templates_subject_not_blank CHECK (btrim(subject) <> '');

-- +goose Down

ALTER TABLE email_templates
    DROP CONSTRAINT IF EXISTS email_templates_subject_not_blank;
