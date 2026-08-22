-- +goose Up

UPDATE app_settings
SET absence_policies = jsonb_set(
        COALESCE(absence_policies, '{}'::jsonb),
        '{student_self_service}',
        jsonb_set(
            COALESCE(absence_policies->'student_self_service', '{}'::jsonb),
            '{can_view_own}',
            'true'::jsonb,
            true
        ),
        true
    ),
    updated_at = now()
WHERE id = true;

-- +goose Down

UPDATE app_settings
SET absence_policies = jsonb_set(
        COALESCE(absence_policies, '{}'::jsonb),
        '{student_self_service}',
        jsonb_set(
            COALESCE(absence_policies->'student_self_service', '{}'::jsonb),
            '{can_view_own}',
            'false'::jsonb,
            true
        ),
        true
    ),
    updated_at = now()
WHERE id = true;
