-- +goose Up
-- One legacy course id may map to at most one local course (CB-05). Duplicate
-- links made sync routing ambiguous: the runner's lookup picked an arbitrary
-- course and external_refs could be overwritten to point at the wrong one.
-- Pre-existing duplicates are collapsed deterministically before the unique
-- index is created: each legacy id keeps its most recently synced course and
-- the losing links are cleared (a later full reconcile relinks them only if
-- the mapping is genuinely unclaimed).
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY legacy_course_id
               ORDER BY legacy_last_synced_at DESC NULLS LAST,
                        updated_at DESC,
                        id
           ) AS rn
    FROM courses
    WHERE legacy_course_id IS NOT NULL
)
UPDATE courses
SET legacy_course_id = NULL
WHERE id IN (SELECT id FROM ranked WHERE rn > 1);

CREATE UNIQUE INDEX IF NOT EXISTS courses_legacy_course_id_unique
    ON courses (legacy_course_id)
    WHERE legacy_course_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS courses_legacy_course_id_unique;
