-- +goose Up

CREATE TABLE email_templates (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    subject    text NOT NULL,
    body       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE email_workflows (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                text NOT NULL,
    enabled             boolean NOT NULL DEFAULT false,
    template_id         uuid NOT NULL REFERENCES email_templates(id),
    trigger_description text NOT NULL DEFAULT 'Daily at 08:00 (Asia/Bangkok)',
    recipients          text[] NOT NULL DEFAULT '{}',
    last_sent_at        timestamptz,
    last_sent_count     int NOT NULL DEFAULT 0,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- Seed default template
INSERT INTO email_templates (name, subject, body) VALUES (
    'Sit-in Day Reminder',
    'Sit-in Reminder — {{sit_in_count}} session(s) today ({{today_date}})',
    'Hi {{institute_name}} team,

This is an automated reminder for today''s sit-in sessions ({{sit_in_count}} total).

{{sit_in_table}}

Thank you,
{{institute_name}}'
);

-- Seed default workflow referencing the template
INSERT INTO email_workflows (name, enabled, template_id, recipients)
SELECT 'Sit-in Day Reminder', true, id, ARRAY[]::text[]
FROM email_templates WHERE name = 'Sit-in Day Reminder';

-- +goose Down

DROP TABLE IF EXISTS email_workflows;
DROP TABLE IF EXISTS email_templates;
