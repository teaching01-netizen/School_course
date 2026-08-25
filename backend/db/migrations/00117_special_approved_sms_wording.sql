-- +goose Up

UPDATE app_settings
SET absence_policies = jsonb_set(
        COALESCE(absence_policies, '{}'::jsonb),
        '{notifications}',
        jsonb_set(
            COALESCE(absence_policies->'notifications', '{}'::jsonb),
            '{sms_special_approved_template}',
            to_jsonb((
                'Warwick Institute: {{nickname}} มีเรียนชดเชย' || chr(10) || chr(10) ||
                '{{absence_pair_summary}}' || chr(10) || chr(10) ||
                'ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ'
            )::text),
            true
        ),
        true
    ),
    updated_at = now()
WHERE id = true
  AND absence_policies #>> '{notifications,sms_special_approved_template}' =
      'Warwick Institute: {{nickname}} จะมีเรียนชดเชย' || chr(10) || chr(10) || '{{absence_pair_summary}}' || chr(10) || chr(10) || 'ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ';

-- +goose Down

UPDATE app_settings
SET absence_policies = jsonb_set(
        COALESCE(absence_policies, '{}'::jsonb),
        '{notifications}',
        jsonb_set(
            COALESCE(absence_policies->'notifications', '{}'::jsonb),
            '{sms_special_approved_template}',
            to_jsonb((
                'Warwick Institute: {{nickname}} จะมีเรียนชดเชย' || chr(10) || chr(10) ||
                '{{absence_pair_summary}}' || chr(10) || chr(10) ||
                'ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ'
            )::text),
            true
        ),
        true
    ),
    updated_at = now()
WHERE id = true
  AND absence_policies #>> '{notifications,sms_special_approved_template}' =
      'Warwick Institute: {{nickname}} มีเรียนชดเชย' || chr(10) || chr(10) || '{{absence_pair_summary}}' || chr(10) || chr(10) || 'ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ';
