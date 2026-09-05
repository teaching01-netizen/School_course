package db

func loadBundleEnrolledAndScopeSQL() string {
	return `
WITH enrolled AS (
SELECT c.id, c.root_course_group_id
FROM course_students cs
JOIN courses c ON c.id = cs.course_id
WHERE cs.student_id = $1 AND cs.status = 'enrolled'
), missed AS (
SELECT unnest($2::uuid[]) AS id
), root_scopes AS (
SELECT DISTINCT c.root_course_group_id AS id FROM courses c
JOIN enrolled e ON e.root_course_group_id = c.root_course_group_id
WHERE c.root_course_group_id IS NOT NULL
UNION
SELECT DISTINCT c.root_course_group_id FROM courses c
JOIN missed m ON m.id = c.id
WHERE c.root_course_group_id IS NOT NULL
), merge_scopes AS (
SELECT DISTINCT mgm.group_id AS id
FROM course_merge_group_members mgm
JOIN enrolled e ON e.id = mgm.course_id
UNION
SELECT DISTINCT mgm.group_id
FROM course_merge_group_members mgm
JOIN missed m ON m.id = mgm.course_id
)
SELECT 0 AS tag, c.id, c.code, c.name, c.subject_id,
'' , '' ,
c.cycle_id, COALESCE(mgg.level, c.level), c.root_course_group_id,
COALESCE(mgg.sit_in_rule_id, rcg.sit_in_rule_id), mgm.group_id,
'' , c.absence_form_visible
FROM course_students cs
JOIN courses c ON c.id = cs.course_id
LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
LEFT JOIN course_merge_group_members mgm ON mgm.course_id = c.id
LEFT JOIN course_merge_groups mgg ON mgg.id = mgm.group_id
WHERE cs.student_id = $1 AND cs.status = 'enrolled'
UNION ALL
SELECT 1 AS tag, c.id, c.code, c.name, c.subject_id,
COALESCE(sub.code, ''), COALESCE(sub.name, ''),
c.cycle_id, COALESCE(mgg.level, c.level), c.root_course_group_id,
COALESCE(mgg.sit_in_rule_id, rcg.sit_in_rule_id), mgm.group_id,
COALESCE(g.name, ''), false
FROM courses c
LEFT JOIN subjects sub ON sub.id = c.subject_id
LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id
LEFT JOIN course_merge_group_members mgm ON mgm.course_id = c.id
LEFT JOIN course_merge_groups mgg ON mgg.id = mgm.group_id
LEFT JOIN course_merge_groups g ON g.id = mgm.group_id
WHERE (c.root_course_group_id IN (SELECT id FROM root_scopes))
OR (mgm.group_id IN (SELECT id FROM merge_scopes))
OR (c.id IN (SELECT id FROM missed))
ORDER BY tag ASC, code ASC
`
}
