-- +goose Up

-- Extend the existing verification-session cron function to clean the opaque
-- discovery/session rows introduced by student self-service. The cleanup is
-- deliberately idempotent and runs in dependency order.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION cleanup_stale_parent_verification_sessions()
RETURNS integer
LANGUAGE plpgsql
AS $$
DECLARE
  deleted_count integer := 0;
  rows_deleted integer;
BEGIN
  DELETE FROM student_self_service_sessions
  WHERE revoked_at IS NOT NULL
     OR expires_at <= now()
     OR absolute_expires_at <= now();

  GET DIAGNOSTICS rows_deleted = ROW_COUNT;
  deleted_count := deleted_count + rows_deleted;

  DELETE FROM student_self_service_lookup_tokens
  WHERE revoked_at IS NOT NULL
     OR expires_at <= now();

  GET DIAGNOSTICS rows_deleted = ROW_COUNT;
  deleted_count := deleted_count + rows_deleted;

  DELETE FROM student_parent_verification_sessions
  WHERE consumed_at IS NULL
    AND created_at < now() - interval '24 hours';

  GET DIAGNOSTICS rows_deleted = ROW_COUNT;
  deleted_count := deleted_count + rows_deleted;

  RETURN deleted_count;
END;
$$;
-- +goose StatementEnd

-- +goose Down

-- Restore the pre-student-self-service cleanup contract.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION cleanup_stale_parent_verification_sessions()
RETURNS integer
LANGUAGE plpgsql
AS $$
DECLARE
  deleted_count integer;
BEGIN
  DELETE FROM student_parent_verification_sessions
  WHERE consumed_at IS NULL
    AND created_at < now() - interval '24 hours';

  GET DIAGNOSTICS deleted_count = ROW_COUNT;
  RETURN deleted_count;
END;
$$;
-- +goose StatementEnd
