package db

import (
	"context"
	"time"

	"github.com/google/uuid"
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

func (q *Queries) TeacherSessionsInRange(ctx context.Context, teacherID uuid.UUID, rangeStart, rangeEnd time.Time) ([]CalendarSessionRow, error) {
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
		  AND sess.teacher_id = $3
		  AND sess.start_at < $2
		  AND sess.end_at > $1
		ORDER BY sess.start_at ASC
	`, rangeStart, rangeEnd, teacherID)
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

type PendingAbsenceRequestRow struct {
	ID             pgtype.UUID
	Wcode          string
	StudentName    pgtype.Text
	Nickname       pgtype.Text
	CourseCode     string
	CourseName     string
	SubjectName    pgtype.Text
	DateFrom       pgtype.Date
	DateTo         pgtype.Date
	Reason         pgtype.Text
	ReasonCategory pgtype.Text
	CreatedAt      pgtype.Timestamptz
}

const teacherPendingAbsenceRequestsQueryTemplate = `
		SELECT sa.id, sa.wcode,
		       COALESCE(st.full_name, sa.student_name) AS student_name,
		       __STUDENT_NICKNAME_EXPR__ AS nickname,
		       c.code, c.name,
		       sub.name,
		       sa.date_from, sa.date_to,
		       sa.reason, sa.reason_category,
		       sa.created_at
		FROM student_absences sa
		JOIN courses c ON c.id = sa.course_id
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN students st ON st.wcode = sa.wcode
		WHERE sa.status = 'pending'
		  AND EXISTS (
		    SELECT 1 FROM sessions sess
		    WHERE sess.course_id = sa.course_id
		      AND sess.teacher_id = $1
		      AND sess.deleted_at IS NULL
		  )
		ORDER BY sa.created_at DESC
`

func (q *Queries) TeacherPendingAbsenceRequests(ctx context.Context, teacherID uuid.UUID) ([]PendingAbsenceRequestRow, error) {
	hasStudentNicknameColumn, err := q.absenceStudentNicknameColumnExists(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.db.Query(ctx, teacherAbsenceQuerySQL(teacherPendingAbsenceRequestsQueryTemplate, hasStudentNicknameColumn), teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingAbsenceRequestRow
	for rows.Next() {
		var item PendingAbsenceRequestRow
		if err := rows.Scan(
			&item.ID, &item.Wcode,
			&item.StudentName, &item.Nickname,
			&item.CourseCode, &item.CourseName,
			&item.SubjectName,
			&item.DateFrom, &item.DateTo,
			&item.Reason, &item.ReasonCategory,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
