-- name: CrmCycleUpsert :one
INSERT INTO crm_cycles (id, label)
VALUES ($1, $2)
ON CONFLICT (id) DO UPDATE
SET label = EXCLUDED.label,
    updated_at = now()
RETURNING id, label, last_imported_at, created_at, updated_at;

-- name: CrmCyclesList :many
SELECT id, label, last_imported_at, created_at, updated_at
FROM crm_cycles
ORDER BY label ASC;

-- name: CrmCyclesUpsertFromUpload :exec
-- Upsert all distinct cycle values from uploaded rows; update last_imported_at.
WITH distinct_cycles AS (
  SELECT DISTINCT cycle_label AS label FROM crm_rows
),
upserted AS (
  INSERT INTO crm_cycles (id, label)
  SELECT label, label FROM distinct_cycles
  ON CONFLICT (id) DO UPDATE
  SET label = EXCLUDED.label,
      updated_at = now()
  RETURNING id
)
UPDATE crm_cycles SET last_imported_at = now()
WHERE id IN (SELECT id FROM upserted);

-- name: CrmCyclesDeleteUnused :exec
DELETE FROM crm_cycles
WHERE id NOT IN (SELECT DISTINCT cycle_label FROM crm_rows);
