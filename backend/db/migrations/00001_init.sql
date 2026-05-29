-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS app_settings (
  id              boolean PRIMARY KEY DEFAULT true,
  institute_tz    text NOT NULL DEFAULT 'Asia/Bangkok',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT app_settings_singleton CHECK (id = true)
);

INSERT INTO app_settings (id) VALUES (true)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS users (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  username         text NOT NULL UNIQUE,
  role             text NOT NULL CHECK (role IN ('Admin', 'Teacher')),
  password_hash    text NOT NULL,
  password_version integer NOT NULL DEFAULT 1,
  deleted_at       timestamptz NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auth_sessions (
  id               uuid PRIMARY KEY,
  user_id          uuid NOT NULL REFERENCES users(id),
  created_at       timestamptz NOT NULL,
  last_seen_at     timestamptz NOT NULL,
  expires_at       timestamptz NOT NULL,
  revoked_at       timestamptz NULL,
  password_version integer NOT NULL,
  CONSTRAINT auth_sessions_expires_after_created CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS auth_sessions_user_id_idx ON auth_sessions(user_id);
CREATE INDEX IF NOT EXISTS auth_sessions_expires_at_idx ON auth_sessions(expires_at);

CREATE TABLE IF NOT EXISTS audit_log (
  id          bigserial PRIMARY KEY,
  created_at  timestamptz NOT NULL DEFAULT now(),
  actor_user_id uuid NULL REFERENCES users(id),
  action      text NOT NULL,
  payload     jsonb NOT NULL DEFAULT '{}'::jsonb
);

-- +goose Down
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS auth_sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS app_settings;

