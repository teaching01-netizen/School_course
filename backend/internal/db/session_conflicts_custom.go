package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const sessionConflictsByCourseSQL = `
SELECT s.id,
       'room_overlap'::text AS kind,
       other.id,
       other.course_id,
       other_course.code,
       other_course.name,
       other.start_at,
       other.end_at
FROM sessions s
JOIN sessions other ON other.id <> s.id
  AND other.deleted_at IS NULL
  AND s.room_id IS NOT NULL
  AND s.room_id = other.room_id
  AND s.time_range && other.time_range
JOIN courses other_course ON other_course.id = other.course_id
WHERE s.course_id = $1
  AND s.deleted_at IS NULL
UNION ALL
SELECT s.id,
       'teacher_overlap'::text AS kind,
       other.id,
       other.course_id,
       other_course.code,
       other_course.name,
       other.start_at,
       other.end_at
FROM sessions s
JOIN sessions other ON other.id <> s.id
  AND other.deleted_at IS NULL
  AND s.teacher_id = other.teacher_id
  AND s.time_range && other.time_range
JOIN courses other_course ON other_course.id = other.course_id
WHERE s.course_id = $1
  AND s.deleted_at IS NULL
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
