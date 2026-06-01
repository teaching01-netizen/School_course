package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type CalendarSessionRow struct {
	ID          pgtype.UUID
	CourseID    pgtype.UUID
	CourseCode  string
	CourseName  string
	SubjectName pgtype.Text
	StartAt     pgtype.Timestamptz
	EndAt       pgtype.Timestamptz
	RoomName    pgtype.Text
	TeacherName pgtype.Text
}

func (q *Queries) CalendarSessionsInRange(ctx context.Context, rangeStart, rangeEnd time.Time) ([]CalendarSessionRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT sess.id, sess.course_id,
		       c.code, c.name, sub.name,
		       sess.start_at, sess.end_at,
		       room.name, u.username
		FROM sessions sess
		JOIN courses c ON c.id = sess.course_id
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN rooms room ON room.id = sess.room_id
		LEFT JOIN users u ON u.id = sess.teacher_id
		WHERE sess.deleted_at IS NULL
		  AND sess.start_at < $2
		  AND sess.end_at > $1
		ORDER BY sess.start_at ASC
	`, rangeStart, rangeEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CalendarSessionRow
	for rows.Next() {
		var item CalendarSessionRow
		if err := rows.Scan(&item.ID, &item.CourseID, &item.CourseCode, &item.CourseName, &item.SubjectName, &item.StartAt, &item.EndAt, &item.RoomName, &item.TeacherName); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
