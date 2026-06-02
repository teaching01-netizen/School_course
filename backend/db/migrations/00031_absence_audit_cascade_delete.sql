-- +goose Up

-- The absence_audit_log append-only trigger blocks ALL deletes, including
-- ON DELETE CASCADE from student_absences. Update the trigger to allow
-- cascade deletes (pg_trigger_depth() > 1 means we are inside a cascade).

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION reject_absence_audit_mutation()
RETURNS trigger AS $$
BEGIN
  IF pg_trigger_depth() > 1 THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION 'absence audit timeline is append-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION reject_absence_audit_mutation()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'absence audit timeline is append-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
