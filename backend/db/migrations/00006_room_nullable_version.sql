-- +goose Up

-- Allow sessions/series without an assigned room (Provisional scheduling).
ALTER TABLE sessions
  ALTER COLUMN room_id DROP NOT NULL;

ALTER TABLE session_series
  ALTER COLUMN room_id DROP NOT NULL;

-- Optimistic concurrency versioning.
ALTER TABLE sessions
  ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'sessions_version_positive'
  ) THEN
    ALTER TABLE sessions
      ADD CONSTRAINT sessions_version_positive CHECK (version > 0);
  END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE session_series
  ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'session_series_version_positive'
  ) THEN
    ALTER TABLE session_series
      ADD CONSTRAINT session_series_version_positive CHECK (version > 0);
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down

ALTER TABLE session_series
  DROP CONSTRAINT IF EXISTS session_series_version_positive;
ALTER TABLE sessions
  DROP CONSTRAINT IF EXISTS sessions_version_positive;

ALTER TABLE session_series
  DROP COLUMN IF EXISTS version;
ALTER TABLE sessions
  DROP COLUMN IF EXISTS version;

ALTER TABLE session_series
  ALTER COLUMN room_id SET NOT NULL;
ALTER TABLE sessions
  ALTER COLUMN room_id SET NOT NULL;
