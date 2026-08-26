package scheduleconflictshttp

import (
	"fmt"
	"strings"
)

// allow: SIZE_OK — these SQL constants form one declarative query pipeline and
// are kept together so page and summary conflict semantics cannot drift.
const activeSessionsSQL = `
WITH active_sessions AS NOT MATERIALIZED (
  SELECT s.id, s.course_id, s.room_id, s.teacher_id, s.start_at, s.end_at,
         s.time_range, s.created_at, s.conflict_override, s.legacy_conflict_override
  FROM sessions s
  WHERE s.deleted_at IS NULL
    AND NOT (
      s.source_kind = 'legacy'
      AND EXISTS (
        SELECT 1
        FROM sessions native_session
        WHERE native_session.deleted_at IS NULL
          AND native_session.source_kind = 'native'
          AND native_session.course_id = s.course_id
          AND native_session.teacher_id = s.teacher_id
          AND native_session.room_id IS NOT DISTINCT FROM s.room_id
          AND native_session.start_at = s.start_at
          AND native_session.end_at = s.end_at
      )
    )
), pair_rows AS (
  SELECT 'room_overlap'::text AS conflict_type,
         LEAST(s1.id, s2.id) AS primary_id,
         GREATEST(s1.id, s2.id) AS conflicting_id,
         s1.room_id AS resource_id,
         CASE WHEN s1.id < s2.id THEN s1.start_at ELSE s2.start_at END AS sort_at
  FROM active_sessions s1
  JOIN active_sessions s2
    ON s1.id <> s2.id
   AND s1.room_id = s2.room_id
   AND s1.time_range && s2.time_range
  WHERE (s1.conflict_override OR s1.legacy_conflict_override)
    AND (NOT (s2.conflict_override OR s2.legacy_conflict_override) OR s1.id < s2.id)
    %s
  UNION ALL
  SELECT 'teacher_overlap', LEAST(s1.id, s2.id), GREATEST(s1.id, s2.id), s1.teacher_id,
         CASE WHEN s1.id < s2.id THEN s1.start_at ELSE s2.start_at END
  FROM active_sessions s1
  JOIN active_sessions s2
    ON s1.id <> s2.id
   AND s1.teacher_id = s2.teacher_id
   AND s1.time_range && s2.time_range
  WHERE (s1.conflict_override OR s1.legacy_conflict_override)
    AND (NOT (s2.conflict_override OR s2.legacy_conflict_override) OR s1.id < s2.id)
    %s
  UNION ALL
  SELECT 'student_overlap', LEAST(b1.session_id, b2.session_id), GREATEST(b1.session_id, b2.session_id), b1.student_id,
         CASE WHEN b1.session_id < b2.session_id THEN s1.start_at ELSE s2.start_at END
  FROM student_busy_ranges b1
  JOIN active_sessions s1 ON s1.id = b1.session_id
  JOIN student_busy_ranges b2
    ON b1.session_id <> b2.session_id
   AND b1.student_id = b2.student_id
   AND b1.time_range && b2.time_range
  JOIN active_sessions s2 ON s2.id = b2.session_id
  WHERE b1.deleted_at IS NULL
    AND b2.deleted_at IS NULL
    AND b1.conflict_override
    AND (NOT b2.conflict_override OR b1.session_id < b2.session_id)
    %s
), pairs AS (
  SELECT conflict_type, primary_id, conflicting_id,
         (array_agg(resource_id ORDER BY resource_id))[1] AS resource_id,
         min(sort_at) AS sort_at
  FROM pair_rows
  GROUP BY conflict_type, primary_id, conflicting_id
)
`

const enrichedSelectSQL = `
SELECT k.conflict_type, k.primary_id, k.conflicting_id,
       CASE k.conflict_type WHEN 'room_overlap' THEN 'room' WHEN 'teacher_overlap' THEN 'teacher' ELSE 'student' END,
       k.resource_id,
       CASE k.conflict_type
         WHEN 'room_overlap' THEN room_resource.name
         WHEN 'teacher_overlap' THEN COALESCE(teacher_resource.full_name, teacher_resource.username)
         ELSE student_resource.full_name
       END,
       p1.course_id, pc.code, pc.name, COALESCE(ps.id::text, ''), COALESCE(ps.name, pc.name),
       p1.teacher_id, COALESCE(pu.full_name, pu.username), p1.room_id, pr.name, p1.start_at, p1.end_at,
       p2.course_id, cc.code, cc.name, COALESCE(cs.id::text, ''), COALESCE(cs.name, cc.name),
       p2.teacher_id, COALESCE(cu.full_name, cu.username), p2.room_id, cr.name, p2.start_at, p2.end_at,
       affected.student_id::text, affected.wcode, affected.full_name,
       GREATEST(p1.created_at, p2.created_at)
FROM page_keys k
JOIN sessions p1 ON p1.id = k.primary_id
JOIN sessions p2 ON p2.id = k.conflicting_id
JOIN courses pc ON pc.id = p1.course_id
JOIN courses cc ON cc.id = p2.course_id
LEFT JOIN subjects ps ON ps.id = pc.subject_id
LEFT JOIN subjects cs ON cs.id = cc.subject_id
JOIN users pu ON pu.id = p1.teacher_id
JOIN users cu ON cu.id = p2.teacher_id
LEFT JOIN rooms pr ON pr.id = p1.room_id
LEFT JOIN rooms cr ON cr.id = p2.room_id
LEFT JOIN rooms room_resource ON k.conflict_type = 'room_overlap' AND room_resource.id = k.resource_id
LEFT JOIN users teacher_resource ON k.conflict_type = 'teacher_overlap' AND teacher_resource.id = k.resource_id
LEFT JOIN students student_resource ON k.conflict_type = 'student_overlap' AND student_resource.id = k.resource_id
LEFT JOIN LATERAL (
  SELECT students.id AS student_id, students.wcode, students.full_name
  FROM student_busy_ranges sb1
  JOIN student_busy_ranges sb2
    ON sb2.student_id = sb1.student_id
   AND sb2.session_id = k.conflicting_id
   AND sb1.time_range && sb2.time_range
   AND sb2.deleted_at IS NULL
  JOIN students ON students.id = sb1.student_id
  WHERE k.conflict_type = 'student_overlap'
    AND sb1.session_id = k.primary_id
    AND sb1.deleted_at IS NULL
    AND (sb1.conflict_override OR sb2.conflict_override)
  ORDER BY students.full_name, students.id
) affected ON true
`

const defaultKeyFilterSQL = `
SELECT * FROM pairs
%s
%s
LIMIT $%d`

const filteredPairsSQL = `
, searchable_pairs AS (
  SELECT p.*
  FROM pairs p
  JOIN sessions p1 ON p1.id = p.primary_id
  JOIN sessions p2 ON p2.id = p.conflicting_id
  JOIN courses pc ON pc.id = p1.course_id
  JOIN courses cc ON cc.id = p2.course_id
  LEFT JOIN subjects ps ON ps.id = pc.subject_id
  LEFT JOIN subjects cs ON cs.id = cc.subject_id
  JOIN users pu ON pu.id = p1.teacher_id
  JOIN users cu ON cu.id = p2.teacher_id
  WHERE ($2::uuid IS NULL OR ps.id = $2 OR cs.id = $2)
    AND ($7::text IS NULL OR pc.code ILIKE '%%' || $7 || '%%' OR pc.name ILIKE '%%' || $7 || '%%'
      OR COALESCE(ps.name, pc.name) ILIKE '%%' || $7 || '%%' OR COALESCE(pu.full_name, pu.username) ILIKE '%%' || $7 || '%%'
      OR cc.code ILIKE '%%' || $7 || '%%' OR cc.name ILIKE '%%' || $7 || '%%'
      OR COALESCE(cs.name, cc.name) ILIKE '%%' || $7 || '%%' OR COALESCE(cu.full_name, cu.username) ILIKE '%%' || $7 || '%%'
      OR EXISTS (
        SELECT 1 FROM student_busy_ranges qb1
        JOIN student_busy_ranges qb2 ON qb2.student_id = qb1.student_id AND qb2.session_id = p.conflicting_id
        JOIN students qs ON qs.id = qb1.student_id
        WHERE qb1.session_id = p.primary_id AND qb1.deleted_at IS NULL AND qb2.deleted_at IS NULL
          AND (qs.wcode ILIKE '%%' || $7 || '%%' OR qs.full_name ILIKE '%%' || $7 || '%%')
      ))
)
`

func pageQuery(filters listFilters) (string, []any) {
	if filters.defaultRequest() {
		cursorWhere, ordering := cursorSQL(filters.Cursor, 1)
		limitParameter := 1
		args := []any{filters.Limit + 1}
		if filters.Cursor != nil {
			limitParameter = 5
			args = append(cursorValues(filters.Cursor), filters.Limit+1)
		}
		keys := fmt.Sprintf(defaultKeyFilterSQL, cursorWhere, ordering, limitParameter)
		query := fmt.Sprintf(activeSessionsSQL, "", "", "") + ", page_keys AS (" + keys + ")\n" + enrichedSelectSQL + orderingForResult(filters.Cursor)
		return query, args
	}

	common := `AND ($1::text IS NULL OR $1 = '%s')
    AND ($3::uuid IS NULL OR s1.teacher_id = $3 OR s2.teacher_id = $3)
    AND ($5::timestamptz IS NULL OR (CASE WHEN s1.id < s2.id THEN s1.start_at ELSE s2.start_at END) >= $5)
    AND ($6::timestamptz IS NULL OR (CASE WHEN s1.id < s2.id THEN s1.start_at ELSE s2.start_at END) < $6 + interval '1 day')`
	room := fmt.Sprintf(common, "room_overlap") + "\n    AND $4::uuid IS NULL"
	teacher := fmt.Sprintf(common, "teacher_overlap") + "\n    AND $4::uuid IS NULL"
	student := `AND ($1::text IS NULL OR $1 = 'student_overlap')
    AND ($3::uuid IS NULL OR s1.teacher_id = $3 OR s2.teacher_id = $3)
    AND ($4::uuid IS NULL OR b1.student_id = $4)
    AND ($5::timestamptz IS NULL OR (CASE WHEN b1.session_id < b2.session_id THEN s1.start_at ELSE s2.start_at END) >= $5)
    AND ($6::timestamptz IS NULL OR (CASE WHEN b1.session_id < b2.session_id THEN s1.start_at ELSE s2.start_at END) < $6 + interval '1 day')`
	cursorWhere, ordering := cursorSQL(filters.Cursor, 8)
	limitParameter := 8
	args := filterArgs(filters)
	if filters.Cursor != nil {
		limitParameter = 12
		args = append(args, cursorValues(filters.Cursor)...)
	}
	args = append(args, filters.Limit+1)
	query := fmt.Sprintf(activeSessionsSQL, room, teacher, student) + filteredPairsSQL + ", page_keys AS (SELECT * FROM searchable_pairs\n" + cursorWhere + "\n" + ordering + fmt.Sprintf("\nLIMIT $%d)\n", limitParameter) + enrichedSelectSQL + orderingForResult(filters.Cursor)
	return query, args
}

func summaryQuery(filters listFilters) (string, []any) {
	base, args := summaryBase(filters)
	return base + `
SELECT count(*)::int,
       count(*) FILTER (WHERE conflict_type = 'room_overlap')::int,
       count(*) FILTER (WHERE conflict_type = 'teacher_overlap')::int,
       count(*) FILTER (WHERE conflict_type = 'student_overlap')::int
FROM ` + summarySource(filters), args
}

func summaryBase(filters listFilters) (string, []any) {
	if filters.defaultRequest() {
		return fmt.Sprintf(activeSessionsSQL, "", "", ""), nil
	}
	query, args := pageQuery(filters)
	marker := ", page_keys AS ("
	return query[:strings.Index(query, marker)], args[:7]
}

func summarySource(filters listFilters) string {
	if filters.defaultRequest() {
		return "pairs"
	}
	return "searchable_pairs"
}

func (f listFilters) defaultRequest() bool {
	return f.ConflictType == "" && f.SubjectID == nil && f.TeacherID == nil && f.StudentID == nil && f.DateFrom == nil && f.DateTo == nil && f.Query == ""
}

func filterArgs(filters listFilters) []any {
	return []any{emptyToNil(filters.ConflictType), filters.SubjectID, filters.TeacherID, filters.StudentID, filters.DateFrom, filters.DateTo, emptyToNil(filters.Query)}
}

func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func cursorSQL(cursor *conflictCursor, start int) (string, string) {
	if cursor == nil {
		return "", "ORDER BY sort_at DESC, conflict_type, primary_id, conflicting_id"
	}
	operator, order := ">", "ORDER BY sort_at DESC, conflict_type, primary_id, conflicting_id"
	if cursor.Direction == cursorNext {
		operator = ">"
		return fmt.Sprintf("WHERE sort_at < $%d OR (sort_at = $%d AND (conflict_type, primary_id, conflicting_id) %s ($%d, $%d, $%d))", start, start, operator, start+1, start+2, start+3), order
	}
	operator = "<"
	order = "ORDER BY sort_at ASC, conflict_type DESC, primary_id DESC, conflicting_id DESC"
	return fmt.Sprintf("WHERE sort_at > $%d OR (sort_at = $%d AND (conflict_type, primary_id, conflicting_id) %s ($%d, $%d, $%d))", start, start, operator, start+1, start+2, start+3), order
}

func orderingForResult(cursor *conflictCursor) string {
	if cursor != nil && cursor.Direction == cursorPrev {
		return "\nORDER BY k.sort_at ASC, k.conflict_type DESC, k.primary_id DESC, k.conflicting_id DESC, affected.full_name, affected.student_id"
	}
	return "\nORDER BY k.sort_at DESC, k.conflict_type, k.primary_id, k.conflicting_id, affected.full_name, affected.student_id"
}

func cursorValues(cursor *conflictCursor) []any {
	return []any{cursor.StartAt, cursor.ConflictType, cursor.PrimaryID, cursor.ConflictingID}
}
