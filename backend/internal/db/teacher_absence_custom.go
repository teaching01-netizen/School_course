package db

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// TeacherAbsenceRow is the deliberately limited absence projection available
// to teaching staff. Contact details, admin notes, audit data, and workflow
// version fields are intentionally absent.
type TeacherAbsenceRow struct {
	ID              pgtype.UUID
	Wcode           string
	StudentName     pgtype.Text
	StudentNickname pgtype.Text
	CourseCode      string
	CourseName      string
	SubjectName     pgtype.Text
	DateFrom        pgtype.Date
	DateTo          pgtype.Date
	ReasonCategory  pgtype.Text
	Reason          pgtype.Text
	Status          string
}

const teacherAbsenceSelectTemplate = `
	SELECT sa.id, sa.wcode,
	       COALESCE(st.full_name, sa.student_name),
	       __STUDENT_NICKNAME_EXPR__,
	       c.code, c.name, sub.name,
	       sa.date_from, sa.date_to, sa.reason_category, sa.reason, sa.status
	FROM student_absences sa
	JOIN courses c ON c.id = sa.course_id
	LEFT JOIN subjects sub ON sub.id = c.subject_id
	LEFT JOIN students st ON st.wcode = sa.wcode
`

func teacherAbsenceQuerySQL(template string, hasStudentNicknameColumn bool) string {
	studentNicknameExpr := "st.nickname"
	if hasStudentNicknameColumn {
		studentNicknameExpr = "COALESCE(st.nickname, sa.student_nickname)"
	}
	return strings.ReplaceAll(template, absenceStudentNicknameExprPlaceholder, studentNicknameExpr)
}

func scanTeacherAbsence(row interface{ Scan(...any) error }) (TeacherAbsenceRow, error) {
	var item TeacherAbsenceRow
	err := row.Scan(
		&item.ID, &item.Wcode, &item.StudentName, &item.StudentNickname,
		&item.CourseCode, &item.CourseName, &item.SubjectName,
		&item.DateFrom, &item.DateTo, &item.ReasonCategory, &item.Reason, &item.Status,
	)
	return item, err
}

func (q *Queries) TeacherAbsenceGet(ctx context.Context, absenceID, teacherID pgtype.UUID) (TeacherAbsenceRow, error) {
	hasStudentNicknameColumn, err := q.absenceStudentNicknameColumnExists(ctx)
	if err != nil {
		return TeacherAbsenceRow{}, err
	}
	return scanTeacherAbsence(q.db.QueryRow(ctx, teacherAbsenceQuerySQL(teacherAbsenceSelectTemplate+`
		WHERE sa.id = $1
		  AND EXISTS (
		    SELECT 1 FROM sessions sess
		    WHERE sess.course_id = sa.course_id
		      AND sess.teacher_id = $2
		      AND sess.deleted_at IS NULL
		  )
	`, hasStudentNicknameColumn), absenceID, teacherID))
}

func (q *Queries) TeacherAbsenceGetAdmin(ctx context.Context, absenceID pgtype.UUID) (TeacherAbsenceRow, error) {
	hasStudentNicknameColumn, err := q.absenceStudentNicknameColumnExists(ctx)
	if err != nil {
		return TeacherAbsenceRow{}, err
	}
	return scanTeacherAbsence(q.db.QueryRow(ctx, teacherAbsenceQuerySQL(teacherAbsenceSelectTemplate+` WHERE sa.id = $1`, hasStudentNicknameColumn), absenceID))
}

func (q *Queries) TeacherAbsenceMissedSessions(ctx context.Context, absenceID, teacherID pgtype.UUID) ([]ManagedAbsenceSession, error) {
	return q.teacherAbsenceSessions(ctx, `absence_missed_sessions`, absenceID, teacherID)
}

func (q *Queries) TeacherAbsenceSitInSessions(ctx context.Context, absenceID, teacherID pgtype.UUID) ([]ManagedAbsenceSession, error) {
	return q.teacherAbsenceSessions(ctx, `absence_sit_ins`, absenceID, teacherID)
}

func (q *Queries) teacherAbsenceSessions(ctx context.Context, relation string, absenceID, teacherID pgtype.UUID) ([]ManagedAbsenceSession, error) {
	// relation is selected exclusively by the two callers above, never from request input.
	rows, err := q.db.Query(ctx, `
		SELECT rel.absence_id, rel.id, sess.id, sess.course_id,
		       c.code, c.name, subj.name, room.name, sess.start_at, sess.end_at
		FROM `+relation+` rel
		JOIN sessions sess ON sess.id = rel.session_id
		  AND sess.deleted_at IS NULL
		  AND sess.teacher_id = $2
		JOIN courses c ON c.id = sess.course_id
		LEFT JOIN subjects subj ON subj.id = c.subject_id
		LEFT JOIN rooms room ON room.id = sess.room_id
		WHERE rel.absence_id = $1
		ORDER BY sess.start_at ASC
	`, absenceID, teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ManagedAbsenceSession, 0)
	for rows.Next() {
		var session ManagedAbsenceSession
		if err := rows.Scan(&session.AbsenceID, &session.ID, &session.SessionID, &session.CourseID, &session.CourseCode, &session.CourseName, &session.SubjectName, &session.RoomName, &session.StartAt, &session.EndAt); err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}
