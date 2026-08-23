package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const sessionConflictsByCourseSQL = `
WITH active_sessions AS (
    SELECT s.*
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
)
SELECT s.id,
       'room_overlap'::text AS kind,
       other.id,
       other.course_id,
       other_course.code,
       other_course.name,
       other.start_at,
       other.end_at
FROM active_sessions s
JOIN active_sessions other ON other.id <> s.id
  AND s.room_id IS NOT NULL
  AND s.room_id = other.room_id
  AND s.time_range && other.time_range
JOIN courses other_course ON other_course.id = other.course_id
WHERE s.course_id = $1
UNION ALL
SELECT s.id,
       'teacher_overlap'::text AS kind,
       other.id,
       other.course_id,
       other_course.code,
       other_course.name,
       other.start_at,
       other.end_at
FROM active_sessions s
JOIN active_sessions other ON other.id <> s.id
  AND s.teacher_id = other.teacher_id
  AND s.time_range && other.time_range
JOIN courses other_course ON other_course.id = other.course_id
WHERE s.course_id = $1
ORDER BY 1, 2, 7
`

type SessionConflictRow struct {
	SessionID             pgtype.UUID
	Kind                  string
	ConflictingSessionID  pgtype.UUID
	ConflictingCourseID   pgtype.UUID
	ConflictingCourseCode string
	ConflictingCourseName string
	ConflictingStartAt    pgtype.Timestamptz
	ConflictingEndAt      pgtype.Timestamptz
}

func (q *Queries) SessionConflictsByCourse(ctx context.Context, courseID pgtype.UUID) ([]SessionConflictRow, error) {
	rows, err := q.db.Query(ctx, sessionConflictsByCourseSQL, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SessionConflictRow
	for rows.Next() {
		var item SessionConflictRow
		if err := rows.Scan(
			&item.SessionID,
			&item.Kind,
			&item.ConflictingSessionID,
			&item.ConflictingCourseID,
			&item.ConflictingCourseCode,
			&item.ConflictingCourseName,
			&item.ConflictingStartAt,
			&item.ConflictingEndAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
