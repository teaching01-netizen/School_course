package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const studentConflictsByCourseSQL = `
SELECT current_roster.student_id,
       current_session.id,
       current_session.start_at,
       current_session.end_at,
       other_session.id,
       other_course.id,
       other_course.code,
       other_course.name,
       other_session.start_at,
       other_session.end_at
FROM course_students current_roster
JOIN sessions current_session ON current_session.course_id = current_roster.course_id
  AND current_session.deleted_at IS NULL
JOIN course_students other_roster ON other_roster.student_id = current_roster.student_id
  AND other_roster.status = 'enrolled'
JOIN sessions other_session ON other_session.course_id = other_roster.course_id
  AND other_session.deleted_at IS NULL
  AND other_session.id <> current_session.id
  AND other_session.course_id <> current_session.course_id
  AND current_session.time_range && other_session.time_range
JOIN courses other_course ON other_course.id = other_session.course_id
WHERE current_roster.course_id = $1
  AND current_roster.status = 'enrolled'
  AND NOT EXISTS (
    SELECT 1 FROM session_attendance excluded
    WHERE excluded.session_id = current_session.id
      AND excluded.student_id = current_roster.student_id
      AND excluded.status = 'excluded'
  )
  AND NOT EXISTS (
    SELECT 1 FROM session_attendance excluded
    WHERE excluded.session_id = other_session.id
      AND excluded.student_id = current_roster.student_id
      AND excluded.status = 'excluded'
  )
ORDER BY current_roster.student_id, current_session.start_at, other_session.start_at
`

type StudentConflictRow struct {
	StudentID             pgtype.UUID
	CurrentSessionID      pgtype.UUID
	CurrentStartAt        pgtype.Timestamptz
	CurrentEndAt          pgtype.Timestamptz
	ConflictingSessionID  pgtype.UUID
	ConflictingCourseID   pgtype.UUID
	ConflictingCourseCode string
	ConflictingCourseName string
	ConflictingStartAt    pgtype.Timestamptz
	ConflictingEndAt      pgtype.Timestamptz
}

func (q *Queries) StudentConflictsByCourse(ctx context.Context, courseID pgtype.UUID) ([]StudentConflictRow, error) {
	rows, err := q.db.Query(ctx, studentConflictsByCourseSQL, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []StudentConflictRow
	for rows.Next() {
		var item StudentConflictRow
		if err := rows.Scan(
			&item.StudentID,
			&item.CurrentSessionID,
			&item.CurrentStartAt,
			&item.CurrentEndAt,
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
