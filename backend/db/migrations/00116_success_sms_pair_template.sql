-- +goose Up

UPDATE app_settings
SET absence_policies = jsonb_set(
        COALESCE(absence_policies, '{}'::jsonb),
        '{notifications}',
        jsonb_set(
            COALESCE(absence_policies->'notifications', '{}'::jsonb),
            '{sms_success_template}',
            to_jsonb((
                'Warwick Institute: {{nickname}} ได้แจ้งลาเรียน' || chr(10) || chr(10) ||
                '{{absence_pair_summary}}' || chr(10) || chr(10) ||
                'ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ'
            )::text),
            true
        ),
        true
    ),
    updated_at = now()
WHERE id = true
  AND absence_policies #>> '{notifications,sms_success_template}' IN (
      'Warwick Institute: {{nickname}} ได้แจ้งลาเรียน {{absence_summary}} และมีกำหนดเข้าเรียนชดเชย {{sit_in_summary}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ',
      'Warwick Institute: {{nickname}} ได้แจ้งลาเรียนคลาส {{class_name}} ในวันที่ {{absence_date}} และมีกำหนดเข้าเรียนชดเชย คลาส {{sit_in_class}} ในวันที่ {{sit_in_date_time}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ'
  );

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
      'Warwick Institute: {{nickname}} จะมีเรียนชดเชย {{absence_summary}} และมีกำหนดเข้าเรียน {{sit_in_summary}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ';

-- +goose Down

UPDATE app_settings
SET absence_policies = jsonb_set(
        COALESCE(absence_policies, '{}'::jsonb),
        '{notifications}',
        jsonb_set(
            COALESCE(absence_policies->'notifications', '{}'::jsonb),
            '{sms_success_template}',
            to_jsonb('Warwick Institute: {{nickname}} ได้แจ้งลาเรียน {{absence_summary}} และมีกำหนดเข้าเรียนชดเชย {{sit_in_summary}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ'::text),
            true
        ),
        true
    ),
    updated_at = now()
WHERE id = true
  AND absence_policies #>> '{notifications,sms_success_template}' =
      'Warwick Institute: {{nickname}} ได้แจ้งลาเรียน' || chr(10) || chr(10) || '{{absence_pair_summary}}' || chr(10) || chr(10) || 'ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ';

UPDATE app_settings
SET absence_policies = jsonb_set(
        COALESCE(absence_policies, '{}'::jsonb),
        '{notifications}',
        jsonb_set(
            COALESCE(absence_policies->'notifications', '{}'::jsonb),
            '{sms_special_approved_template}',
            to_jsonb('Warwick Institute: {{nickname}} จะมีเรียนชดเชย {{absence_summary}} และมีกำหนดเข้าเรียน {{sit_in_summary}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ'::text),
            true
        ),
        true
    ),
    updated_at = now()
WHERE id = true
  AND absence_policies #>> '{notifications,sms_special_approved_template}' =
      'Warwick Institute: {{nickname}} จะมีเรียนชดเชย' || chr(10) || chr(10) || '{{absence_pair_summary}}' || chr(10) || chr(10) || 'ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ';
