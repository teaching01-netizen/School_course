-- +goose Up

UPDATE email_templates
SET
    subject = 'Sit-in Reminder — {{sit_in_count}} session(s) today ({{today_date}})',
    body = 'Hi {{institute_name}} team,

This is an automated reminder for today''s sit-in sessions ({{sit_in_count}} total).

{{sit_in_table}}

Thank you,
{{institute_name}}',
    updated_at = now()
WHERE name = 'Sit-in Day Reminder';

-- +goose Down

UPDATE email_templates
SET
    subject = 'Sit-in Reminder: {{student_name}} — {{sit_in_course_code}}',
    body = 'Hi {{institute_name}} team,

This is an automated reminder for today''s sit-in sessions.

Student: {{student_name}}
Nickname: {{student_nickname}}
Course visiting: {{sit_in_course_name}} ({{sit_in_course_code}})
Date: {{sit_in_date}}
Time: {{sit_in_time}}

The student is absent from {{course_name}} ({{course_code}}) from {{absence_date_range}}.

Thank you,
{{institute_name}}',
    updated_at = now()
WHERE name = 'Sit-in Day Reminder';
