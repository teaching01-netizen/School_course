-- +goose Up

-- Backfill student_phone for rows where it's still NULL/empty
-- but crm_rows has mobile_phone data (new reconcile-created students after 00030).
UPDATE students s
SET student_phone = c.mobile_phone,
    updated_at = now()
FROM (
  SELECT DISTINCT ON (wcode)
    wcode,
    mobile_phone
  FROM crm_rows
  WHERE mobile_phone IS NOT NULL
    AND btrim(mobile_phone) <> ''
  ORDER BY wcode, order_quote_updated_at DESC NULLS LAST, imported_at DESC NULLS LAST, xlsx_row_number ASC
) c
WHERE s.wcode = c.wcode
  AND (s.student_phone IS NULL OR btrim(s.student_phone) = '');

-- Backfill parent_phone for rows where it's still NULL/empty
-- but crm_rows has parent_phone data (new reconcile-created students after 00025).
UPDATE students s
SET parent_phone = c.parent_phone,
    updated_at = now()
FROM (
  SELECT DISTINCT ON (wcode)
    wcode,
    parent_phone
  FROM crm_rows
  WHERE parent_phone IS NOT NULL
    AND parent_phone <> ''
  ORDER BY wcode, order_quote_updated_at DESC NULLS LAST, imported_at DESC NULLS LAST, xlsx_row_number ASC
) c
WHERE s.wcode = c.wcode
  AND (s.parent_phone IS NULL OR btrim(s.parent_phone) = '');

-- +goose Down

-- No structural changes to reverse.
-- Reverting data would destroy any new phone data that subsequent imports may have populated.
-- If a rollback is needed, restore from backup.
