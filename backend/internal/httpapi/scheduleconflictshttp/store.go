package scheduleconflictshttp

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const conflictQuery = `
WITH pair_conflicts AS (
  SELECT 'room_overlap'::text AS conflict_type, s1.id AS primary_id, s2.id AS conflicting_id,
         s1.room_id AS resource_id, r.name AS resource_name,
         NULL::uuid AS student_id, NULL::text AS student_wcode, NULL::text AS student_name
  FROM sessions s1
  JOIN sessions s2 ON s1.id < s2.id AND s1.room_id = s2.room_id AND s1.time_range && s2.time_range
  JOIN rooms r ON r.id = s1.room_id
  WHERE s1.deleted_at IS NULL AND s2.deleted_at IS NULL
  UNION ALL
  SELECT 'teacher_overlap', s1.id, s2.id, s1.teacher_id, COALESCE(u.full_name, u.username),
         NULL::uuid, NULL::text, NULL::text
  FROM sessions s1
  JOIN sessions s2 ON s1.id < s2.id AND s1.teacher_id = s2.teacher_id AND s1.time_range && s2.time_range
  JOIN users u ON u.id = s1.teacher_id
  WHERE s1.deleted_at IS NULL AND s2.deleted_at IS NULL
  UNION ALL
  SELECT 'student_overlap', b1.session_id, b2.session_id, b1.student_id, st.full_name,
         st.id, st.wcode, st.full_name
  FROM student_busy_ranges b1
  JOIN student_busy_ranges b2 ON b1.session_id < b2.session_id AND b1.student_id = b2.student_id AND b1.time_range && b2.time_range
  JOIN students st ON st.id = b1.student_id
  WHERE b1.deleted_at IS NULL AND b2.deleted_at IS NULL
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
  FROM pair_conflicts p
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
)
SELECT conflict_type, primary_id::text, conflicting_id::text, resource_id::text, resource_name,
       student_id::text, student_wcode, student_name,
       p_course_id::text, p_course_code, p_course_name, p_subject_id, p_subject_name,
       p_teacher_id::text, p_teacher_name, p_room_id::text, p_room_name, p_start_at, p_end_at,
       c_course_id::text, c_course_code, c_course_name, c_subject_id, c_subject_name,
       c_teacher_id::text, c_teacher_name, c_room_id::text, c_room_name, c_start_at, c_end_at, detected_at
FROM enriched
WHERE ($1 = '' OR conflict_type = $1)
  AND ($2 = '' OR p_subject_id = $2 OR c_subject_id = $2)
  AND ($3 = '' OR p_teacher_id::text = $3 OR c_teacher_id::text = $3)
  AND ($4 = '' OR student_id::text = $4)
  AND ($5 = '' OR p_start_at >= $5::date)
  AND ($6 = '' OR p_start_at < ($6::date + interval '1 day'))
  AND ($7 = '' OR p_course_code ILIKE '%' || $7 || '%' OR p_course_name ILIKE '%' || $7 || '%'
       OR p_subject_name ILIKE '%' || $7 || '%' OR p_teacher_name ILIKE '%' || $7 || '%'
       OR c_course_code ILIKE '%' || $7 || '%' OR c_course_name ILIKE '%' || $7 || '%'
       OR c_subject_name ILIKE '%' || $7 || '%' OR c_teacher_name ILIKE '%' || $7 || '%'
       OR COALESCE(student_wcode, '') ILIKE '%' || $7 || '%' OR COALESCE(student_name, '') ILIKE '%' || $7 || '%')
ORDER BY p_start_at DESC, conflict_type, primary_id, conflicting_id, student_name`

type conflictStore struct {
	db *pgxpool.Pool
}

func (s conflictStore) list(ctx context.Context, filters listFilters) (listResponse, error) {
	rows, err := s.db.Query(ctx, conflictQuery, filters.ConflictType, filters.SubjectID, filters.TeacherID, filters.StudentID, filters.DateFrom, filters.DateTo, filters.Query)
	if err != nil {
		return listResponse{}, fmt.Errorf("query schedule conflicts: %w", err)
	}
	defer rows.Close()

	items := make([]conflictDTO, 0)
	indexByID := make(map[string]int)
	for rows.Next() {
		var conflictType, primaryID, conflictingID, resourceID, resourceName string
		var studentID, studentWCode, studentName, primaryRoomID, primaryRoomName, conflictingRoomID, conflictingRoomName *string
		var primary, conflicting sessionDTO
		var primaryStart, primaryEnd, conflictingStart, conflictingEnd, detectedAt time.Time
		err := rows.Scan(
			&conflictType, &primaryID, &conflictingID, &resourceID, &resourceName,
			&studentID, &studentWCode, &studentName,
			&primary.CourseID, &primary.CourseCode, &primary.CourseName, &primary.SubjectID, &primary.SubjectName,
			&primary.TeacherID, &primary.TeacherName, &primaryRoomID, &primaryRoomName, &primaryStart, &primaryEnd,
			&conflicting.CourseID, &conflicting.CourseCode, &conflicting.CourseName, &conflicting.SubjectID, &conflicting.SubjectName,
			&conflicting.TeacherID, &conflicting.TeacherName, &conflictingRoomID, &conflictingRoomName, &conflictingStart, &conflictingEnd, &detectedAt,
		)
		if err != nil {
			return listResponse{}, fmt.Errorf("scan schedule conflict: %w", err)
		}
		primary.SessionID, primary.RoomID, primary.RoomName = primaryID, primaryRoomID, primaryRoomName
		primary.StartAt, primary.EndAt = primaryStart.UTC().Format(time.RFC3339), primaryEnd.UTC().Format(time.RFC3339)
		conflicting.SessionID, conflicting.RoomID, conflicting.RoomName = conflictingID, conflictingRoomID, conflictingRoomName
		conflicting.StartAt, conflicting.EndAt = conflictingStart.UTC().Format(time.RFC3339), conflictingEnd.UTC().Format(time.RFC3339)
		id := conflictType + ":" + primaryID + ":" + conflictingID
		itemIndex, exists := indexByID[id]
		if !exists {
			resourceType := "student"
			if conflictType == "room_overlap" {
				resourceType = "room"
			} else if conflictType == "teacher_overlap" {
				resourceType = "teacher"
			}
			items = append(items, conflictDTO{ID: id, ConflictType: conflictType, PrimarySession: primary, ConflictingSessions: []sessionDTO{conflicting}, AffectedStudents: []studentDTO{}, SharedResource: resourceDTO{Type: resourceType, ID: resourceID, Name: resourceName}, DetectedAt: detectedAt.UTC().Format(time.RFC3339)})
			itemIndex = len(items) - 1
			indexByID[id] = itemIndex
		}
		if studentID != nil && studentWCode != nil && studentName != nil {
			items[itemIndex].AffectedStudents = append(items[itemIndex].AffectedStudents, studentDTO{StudentID: *studentID, WCode: *studentWCode, FullName: *studentName})
		}
	}
	if err := rows.Err(); err != nil {
		return listResponse{}, fmt.Errorf("iterate schedule conflicts: %w", err)
	}

	summary := summaryDTO{TotalConflicts: len(items)}
	for _, item := range items {
		switch item.ConflictType {
		case "room_overlap":
			summary.RoomOverlaps++
		case "teacher_overlap":
			summary.TeacherOverlaps++
		case "student_overlap":
			summary.StudentOverlaps++
		}
	}
	start := min(filters.Offset, len(items))
	end := min(start+filters.Limit, len(items))
	return listResponse{Items: items[start:end], TotalCount: len(items), Offset: filters.Offset, Limit: filters.Limit, Summary: summary}, nil
}
