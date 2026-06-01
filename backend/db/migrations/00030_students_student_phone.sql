-- +goose Up

ALTER TABLE students
  ADD COLUMN IF NOT EXISTS student_phone text NULL;

UPDATE students s
SET student_phone = c.mobile_phone
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

-- +goose Down

ALTER TABLE students
  DROP COLUMN IF EXISTS student_phone;
