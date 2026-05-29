-- +goose Up
-- No structural change. The absence_policies jsonb column already exists.
-- The change is purely logical — we stop using level_action_map within the application.
-- This migration is a placeholder for documentation purposes.

-- +goose Down
-- No-op.
