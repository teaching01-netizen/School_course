-- +goose Up

CREATE TABLE IF NOT EXISTS student_self_service_lookup_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  token_hash bytea NOT NULL UNIQUE,
  wcode text NOT NULL REFERENCES students(wcode) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  last_used_at timestamptz NULL,
  revoked_at timestamptz NULL
);

CREATE INDEX IF NOT EXISTS student_self_service_lookup_tokens_wcode_idx
  ON student_self_service_lookup_tokens (wcode, created_at DESC);
CREATE INDEX IF NOT EXISTS student_self_service_lookup_tokens_expiry_idx
  ON student_self_service_lookup_tokens (expires_at)
  WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS student_self_service_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  token_hash bytea NOT NULL UNIQUE,
  wcode text NOT NULL REFERENCES students(wcode) ON DELETE CASCADE,
  verification_session_id uuid NOT NULL REFERENCES student_parent_verification_sessions(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  absolute_expires_at timestamptz NOT NULL,
  revoked_at timestamptz NULL
);

CREATE INDEX IF NOT EXISTS student_self_service_sessions_wcode_idx
  ON student_self_service_sessions (wcode, created_at DESC);
CREATE INDEX IF NOT EXISTS student_self_service_sessions_expiry_idx
  ON student_self_service_sessions (expires_at, absolute_expires_at)
  WHERE revoked_at IS NULL;

-- +goose Down

DROP TABLE IF EXISTS student_self_service_sessions;
DROP TABLE IF EXISTS student_self_service_lookup_tokens;
