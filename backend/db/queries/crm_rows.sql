-- name: CrmRowsDeleteAll :exec
DELETE FROM crm_rows;

-- name: CrmRowsInsert :one
INSERT INTO crm_rows (
  row_hash, cycle_label, course_name, wcode,
  first_name, last_name, nickname, secondary_school, academic_level,
  mobile_phone, hours, teachers_raw, primary_email,
  parent_name, parent_phone, parent_email, order_quote_updated_at
) VALUES (
  $1,$2,$3,$4,
  NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),
  NULLIF($10,''),$11,NULLIF($12,''),NULLIF($13,''),
  NULLIF($14,''),NULLIF($15,''),NULLIF($16,''),$17
) RETURNING row_hash;

-- name: CrmRowsCount :one
SELECT COUNT(*)::bigint FROM crm_rows;

-- name: CrmDistinctOptions :one
SELECT
  COALESCE(jsonb_agg(DISTINCT cycle_label ORDER BY cycle_label) FILTER (WHERE cycle_label IS NOT NULL), '[]'::jsonb) AS cycle_labels,
  COALESCE(jsonb_agg(DISTINCT course_name ORDER BY course_name) FILTER (WHERE course_name IS NOT NULL), '[]'::jsonb) AS course_names,
  COALESCE(jsonb_agg(DISTINCT academic_level ORDER BY academic_level) FILTER (WHERE academic_level IS NOT NULL AND btrim(academic_level) <> ''), '[]'::jsonb) AS academic_levels,
  COALESCE(jsonb_agg(DISTINCT secondary_school ORDER BY secondary_school) FILTER (WHERE secondary_school IS NOT NULL AND btrim(secondary_school) <> ''), '[]'::jsonb) AS secondary_schools
FROM crm_rows
WHERE snapshot_id = $1;
