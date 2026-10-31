-- +goose Up

-- A hard-deleted session used to leave its legacy-schedule mapping behind in
-- external_refs. The mapping is polymorphic (internal_id has no foreign key),
-- so nothing removed it, and the next legacy refresh failed the whole course
-- with "legacy schedule <id> maps to missing session <uuid>". Drop the rows
-- that are already unusable, then keep external_refs in step with future
-- session hard deletes.
DELETE FROM external_refs ref
WHERE ref.entity_type = 'schedule'
  AND NOT EXISTS (SELECT 1 FROM sessions sess WHERE sess.id = ref.internal_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_sessions_delete_legacy_schedule_refs()
RETURNS trigger AS $$
BEGIN
  DELETE FROM external_refs
  WHERE entity_type = 'schedule' AND internal_id = OLD.id;
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_sessions_delete_legacy_schedule_refs ON sessions;
CREATE TRIGGER trg_sessions_delete_legacy_schedule_refs
AFTER DELETE ON sessions
FOR EACH ROW
EXECUTE FUNCTION trg_sessions_delete_legacy_schedule_refs();

-- +goose Down

DROP TRIGGER IF EXISTS trg_sessions_delete_legacy_schedule_refs ON sessions;
DROP FUNCTION IF EXISTS trg_sessions_delete_legacy_schedule_refs();
