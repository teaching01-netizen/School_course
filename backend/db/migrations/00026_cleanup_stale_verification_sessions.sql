-- +goose Up
-- Cron/ops entrypoint: SELECT cleanup_stale_parent_verification_sessions();
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

-- +goose Down
DROP FUNCTION IF EXISTS cleanup_stale_parent_verification_sessions();
