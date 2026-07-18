-- +goose Up

-- CRM exports use an uppercase W prefix, while the import pipeline stores the
-- canonical lowercase form. Repair legacy reconnect assignments where doing so
-- cannot collide with another historical row for the same source course.
UPDATE crm_cross_study_assignments AS assignment
SET wcode = LOWER(BTRIM(assignment.wcode)),
    updated_at = now()
WHERE assignment.wcode <> LOWER(BTRIM(assignment.wcode))
  AND NOT EXISTS (
    SELECT 1
    FROM crm_cross_study_assignments AS duplicate
    WHERE duplicate.id <> assignment.id
      AND duplicate.source_course_id = assignment.source_course_id
      AND LOWER(BTRIM(duplicate.wcode)) = LOWER(BTRIM(assignment.wcode))
  );

-- Keep reconnect lookups efficient even if a legacy case-colliding historical
-- row could not be rewritten safely.
CREATE INDEX IF NOT EXISTS crm_cross_study_assignments_wcode_lower_idx
  ON crm_cross_study_assignments (LOWER(BTRIM(wcode)), source_course_id);

-- +goose Down
DROP INDEX IF EXISTS crm_cross_study_assignments_wcode_lower_idx;
