-- Tombstone / no-op: kept for sequence continuity (00001..max contiguous).
-- The absence_policies jsonb column already existed before this number was
-- allocated. The intended change was purely logical (stop using
-- level_action_map in application code) and required no DDL. This file
-- intentionally carries no structural change so the chain stays contiguous.
-- NOTE (C2 trust repair): See GOVERNANCE.md — Phantom-history / trust repair (C2).
-- Future migrations must not reuse this number.

-- +goose Up
-- No structural change. The absence_policies jsonb column already exists.
-- The change is purely logical — we stop using level_action_map within the application.
-- This migration is a placeholder for documentation purposes. Intentionally no DDL.

-- +goose Down
-- No-op. Tombstone has no state to roll back.
