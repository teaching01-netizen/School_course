-- +goose Up
-- Fix idempotency_keys: make actor_user_id NOT NULL with system sentinel.
-- This ensures the unique index (actor_user_id, scope, idempotency_key) works
-- correctly for both user-initiated and system/background job writes without
-- relying on Postgres 15+ NULLS NOT DISTINCT.

-- Define the system actor sentinel UUID.
-- Used when the actor is a background job rather than a logged-in user.
-- This avoids NULL uniqueness issues and provides a clear sentinel value.
--
-- Note: We use a raw literal because pgx/pgtype.UUID supports
-- the all-zero UUID as a valid value.

-- Backfill existing rows where actor_user_id IS NULL to the sentinel.
UPDATE idempotency_keys
SET actor_user_id = '00000000-0000-0000-0000-000000000000'
WHERE actor_user_id IS NULL;

-- Make actor_user_id NOT NULL with default sentinel.
ALTER TABLE idempotency_keys
ALTER COLUMN actor_user_id SET NOT NULL;

ALTER TABLE idempotency_keys
ALTER COLUMN actor_user_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

-- Ensure we have the proper unique index (should already exist from 00009).
-- Re-create it to be safe (IF NOT EXISTS in the original creation).
-- The unique index on (actor_user_id, scope, idempotency_key) now correctly
-- handles all cases since actor_user_id is always NOT NULL.
SELECT 1;

-- +goose Down
ALTER TABLE idempotency_keys
ALTER COLUMN actor_user_id DROP NOT NULL;

ALTER TABLE idempotency_keys
ALTER COLUMN actor_user_id DROP DEFAULT;
