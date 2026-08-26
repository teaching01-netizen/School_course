package scheduleconflictshttp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const conflictQuery = `
WITH active_sessions AS NOT MATERIALIZED (
  SELECT s.id, s.course_id, s.room_id, s.teacher_id, s.start_at, s.end_at, s.time_range, s.created_at, s.conflict_override, s.legacy_conflict_override
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
  SELECT 'room_overlap'::text AS conflict_type, LEAST(s1.id, s2.id) AS primary_id, GREATEST(s1.id, s2.id) AS conflicting_id,
         s1.room_id AS resource_id, r.name AS resource_name,
         NULL::uuid AS student_id, NULL::text AS student_wcode, NULL::text AS student_name
  FROM active_sessions s1
  JOIN active_sessions s2
    ON s1.id <> s2.id
   AND s1.room_id = s2.room_id
   AND s1.time_range && s2.time_range
  JOIN rooms r ON r.id = s1.room_id
  WHERE (s1.conflict_override OR s1.legacy_conflict_override)
    AND ($1 = '' OR $1 = 'room_overlap')
    AND $4 = ''
    AND ($3 = '' OR s1.teacher_id::text = $3 OR s2.teacher_id::text = $3)
    AND ($5 = '' OR (CASE WHEN s1.id < s2.id THEN s1.start_at ELSE s2.start_at END) >= $5::date)
    AND ($6 = '' OR (CASE WHEN s1.id < s2.id THEN s1.start_at ELSE s2.start_at END) < ($6::date + interval '1 day'))
  UNION ALL
  SELECT 'teacher_overlap', LEAST(s1.id, s2.id), GREATEST(s1.id, s2.id), s1.teacher_id, COALESCE(u.full_name, u.username),
         NULL::uuid, NULL::text, NULL::text
  FROM active_sessions s1
  JOIN active_sessions s2
    ON s1.id <> s2.id
   AND s1.teacher_id = s2.teacher_id
   AND s1.time_range && s2.time_range
  JOIN users u ON u.id = s1.teacher_id
  WHERE (s1.conflict_override OR s1.legacy_conflict_override)
    AND ($1 = '' OR $1 = 'teacher_overlap')
    AND $4 = ''
    AND ($3 = '' OR s1.teacher_id::text = $3 OR s2.teacher_id::text = $3)
    AND ($5 = '' OR (CASE WHEN s1.id < s2.id THEN s1.start_at ELSE s2.start_at END) >= $5::date)
    AND ($6 = '' OR (CASE WHEN s1.id < s2.id THEN s1.start_at ELSE s2.start_at END) < ($6::date + interval '1 day'))
  UNION ALL
  SELECT 'student_overlap', LEAST(b1.session_id, b2.session_id), GREATEST(b1.session_id, b2.session_id), b1.student_id, st.full_name,
         st.id, st.wcode, st.full_name
  FROM student_busy_ranges b1
  JOIN active_sessions s1 ON s1.id = b1.session_id
  JOIN student_busy_ranges b2
    ON b1.session_id <> b2.session_id
   AND b1.student_id = b2.student_id
   AND b1.time_range && b2.time_range
  JOIN active_sessions s2 ON s2.id = b2.session_id
  JOIN students st ON st.id = b1.student_id
  WHERE b1.deleted_at IS NULL
    AND b2.deleted_at IS NULL
    AND b1.conflict_override
    AND ($1 = '' OR $1 = 'student_overlap')
    AND ($4 = '' OR b1.student_id::text = $4)
    AND ($3 = '' OR s1.teacher_id::text = $3 OR s2.teacher_id::text = $3)
    AND ($5 = '' OR (CASE WHEN b1.session_id < b2.session_id THEN s1.start_at ELSE s2.start_at END) >= $5::date)
    AND ($6 = '' OR (CASE WHEN b1.session_id < b2.session_id THEN s1.start_at ELSE s2.start_at END) < ($6::date + interval '1 day'))
), deduped_pair_rows AS (
  SELECT DISTINCT * FROM pair_rows
), pairs AS (
  SELECT conflict_type, primary_id, conflicting_id,
         (array_agg(resource_id ORDER BY resource_id))[1] AS resource_id,
         (array_agg(resource_name ORDER BY resource_id))[1] AS resource_name,
         COALESCE(
           jsonb_agg(
             jsonb_build_object('student_id', student_id::text, 'wcode', student_wcode, 'full_name', student_name)
             ORDER BY student_name, student_id
           ) FILTER (WHERE student_id IS NOT NULL),
           '[]'::jsonb
         ) AS affected_students,
         bool_or(
           COALESCE(student_wcode, '') ILIKE '%' || $7 || '%'
           OR COALESCE(student_name, '') ILIKE '%' || $7 || '%'
         ) AS student_query_match
  FROM deduped_pair_rows
  GROUP BY conflict_type, primary_id, conflicting_id
), enriched AS (
  SELECT p.*,
    p1.course_id AS p_course_id, pc.code AS p_course_code, pc.name AS p_course_name,
    COALESCE(ps.id::text, '') AS p_subject_id, COALESCE(ps.name, pc.name) AS p_subject_name,
    p1.teacher_id AS p_teacher_id, COALESCE(pu.full_name, pu.username) AS p_teacher_name,
    p1.room_id AS p_room_id, pr.name AS p_room_name, p1.start_at AS p_start_at, p1.end_at AS p_end_at,
    p2.course_id AS c_course_id, cc.code AS c_course_code, cc.name AS c_course_name,
    COALESCE(cs.id::text, '') AS c_subject_id, COALESCE(cs.name, cc.name) AS c_subject_name,
    p2.teacher_id AS c_teacher_id, COALESCE(cu.full_name, cu.username) AS c_teacher_name,
    p2.room_id AS c_room_id, cr.name AS c_room_name, p2.start_at AS c_start_at, p2.end_at AS c_end_at,
    GREATEST(p1.created_at, p2.created_at) AS detected_at
  FROM pairs p
  JOIN sessions p1 ON p1.id = p.primary_id
  JOIN sessions p2 ON p2.id = p.conflicting_id
  JOIN courses pc ON pc.id = p1.course_id
  JOIN courses cc ON cc.id = p2.course_id
  LEFT JOIN subjects ps ON ps.id = pc.subject_id
  LEFT JOIN subjects cs ON cs.id = cc.subject_id
  JOIN users pu ON pu.id = p1.teacher_id
  JOIN users cu ON cu.id = p2.teacher_id
  LEFT JOIN rooms pr ON pr.id = p1.room_id
  LEFT JOIN rooms cr ON cr.id = p2.room_id
  WHERE ($2 = '' OR COALESCE(ps.id::text, '') = $2 OR COALESCE(cs.id::text, '') = $2)
    AND ($7 = '' OR pc.code ILIKE '%' || $7 || '%' OR pc.name ILIKE '%' || $7 || '%'
         OR COALESCE(ps.name, pc.name) ILIKE '%' || $7 || '%' OR COALESCE(pu.full_name, pu.username) ILIKE '%' || $7 || '%'
         OR cc.code ILIKE '%' || $7 || '%' OR cc.name ILIKE '%' || $7 || '%'
         OR COALESCE(cs.name, cc.name) ILIKE '%' || $7 || '%' OR COALESCE(cu.full_name, cu.username) ILIKE '%' || $7 || '%'
         OR p.student_query_match)
), numbered AS MATERIALIZED (
  SELECT e.*,
         count(*) OVER ()::int AS total_count,
         count(*) FILTER (WHERE conflict_type = 'room_overlap') OVER ()::int AS room_overlaps,
         count(*) FILTER (WHERE conflict_type = 'teacher_overlap') OVER ()::int AS teacher_overlaps,
         count(*) FILTER (WHERE conflict_type = 'student_overlap') OVER ()::int AS student_overlaps
  FROM enriched e
), page AS (
  SELECT *
  FROM numbered
  ORDER BY p_start_at DESC, conflict_type, primary_id, conflicting_id
  LIMIT $8 OFFSET $9
), metadata AS (
  SELECT COALESCE(max(total_count), 0)::int AS total_count,
         COALESCE(max(room_overlaps), 0)::int AS room_overlaps,
         COALESCE(max(teacher_overlaps), 0)::int AS teacher_overlaps,
         COALESCE(max(student_overlaps), 0)::int AS student_overlaps
  FROM numbered
)
SELECT CASE WHEN page.primary_id IS NULL THEN NULL ELSE jsonb_build_object(
         'id', page.conflict_type || ':' || page.primary_id::text || ':' || page.conflicting_id::text,
         'conflict_type', page.conflict_type,
         'primary_session', jsonb_build_object(
           'session_id', page.primary_id::text,
           'course_id', page.p_course_id::text,
           'course_code', page.p_course_code,
           'course_name', page.p_course_name,
           'subject_id', page.p_subject_id,
           'subject_name', page.p_subject_name,
           'teacher_id', page.p_teacher_id::text,
           'teacher_name', page.p_teacher_name,
           'room_id', page.p_room_id::text,
           'room_name', page.p_room_name,
           'start_at', page.p_start_at,
           'end_at', page.p_end_at
         ),
         'conflicting_sessions', jsonb_build_array(jsonb_build_object(
           'session_id', page.conflicting_id::text,
           'course_id', page.c_course_id::text,
           'course_code', page.c_course_code,
           'course_name', page.c_course_name,
           'subject_id', page.c_subject_id,
           'subject_name', page.c_subject_name,
           'teacher_id', page.c_teacher_id::text,
           'teacher_name', page.c_teacher_name,
           'room_id', page.c_room_id::text,
           'room_name', page.c_room_name,
           'start_at', page.c_start_at,
           'end_at', page.c_end_at
         )),
         'affected_students', page.affected_students,
         'shared_resource', jsonb_build_object(
           'type', CASE page.conflict_type WHEN 'room_overlap' THEN 'room' WHEN 'teacher_overlap' THEN 'teacher' ELSE 'student' END,
           'id', page.resource_id::text,
           'name', page.resource_name
         ),
         'detected_at', page.detected_at
       ) END AS item,
       metadata.total_count, metadata.room_overlaps, metadata.teacher_overlaps, metadata.student_overlaps
FROM metadata
LEFT JOIN page ON true
ORDER BY page.p_start_at DESC NULLS LAST, page.conflict_type, page.primary_id, page.conflicting_id`

type conflictStore struct {
	db *pgxpool.Pool
}

func (s conflictStore) list(ctx context.Context, filters listFilters) (listResponse, error) {
	rows, err := s.db.Query(ctx, conflictQuery,
		filters.ConflictType,
		filters.SubjectID,
		filters.TeacherID,
		filters.StudentID,
		filters.DateFrom,
		filters.DateTo,
		filters.Query,
		filters.Limit,
		filters.Offset,
	)
	if err != nil {
		return listResponse{}, fmt.Errorf("query schedule conflicts: %w", err)
	}
	defer rows.Close()

	items := make([]conflictDTO, 0, filters.Limit)
	summary := summaryDTO{}
	totalCount := 0
	for rows.Next() {
		var rawItem []byte
		if err := rows.Scan(
			&rawItem,
			&totalCount,
			&summary.RoomOverlaps,
			&summary.TeacherOverlaps,
			&summary.StudentOverlaps,
		); err != nil {
			return listResponse{}, fmt.Errorf("scan schedule conflict: %w", err)
		}
		if len(rawItem) == 0 {
			continue
		}
		var item conflictDTO
		if err := json.Unmarshal(rawItem, &item); err != nil {
			return listResponse{}, fmt.Errorf("decode schedule conflict: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return listResponse{}, fmt.Errorf("iterate schedule conflicts: %w", err)
	}

	summary.TotalConflicts = totalCount
	return listResponse{
		Items:      items,
		TotalCount: totalCount,
		Offset:     filters.Offset,
		Limit:      filters.Limit,
		Summary:    summary,
	}, nil
}
