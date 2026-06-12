package db

import (
	"context"
	"time"
)

type TodaySitInRow struct {
	StudentName      string
	StudentNickname  string
	CourseCode       string
	CourseName       string
	SitInCourseCode  string
	SitInCourseName  string
	StartAt          time.Time
	EndAt            time.Time
	TeacherName      string
	TeacherEmail     string
	AbsenceDateRange string
}

func (q *Queries) QueryTodaySitIns(ctx context.Context, todayDate, instituteTZ string) ([]TodaySitInRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT
			COALESCE(st.full_name, sa.wcode),
			COALESCE(st.nickname, st.full_name, sa.wcode),
			c.code,
			c.name,
			sit_c.code,
			sit_c.name,
			ses.start_at,
			ses.end_at,
			COALESCE(u.username, ''),
			COALESCE(u.email, ''),
			sa.date_from::text || ' - ' || sa.date_to::text
		FROM student_absences sa
		JOIN absence_sit_ins asi ON asi.absence_id = sa.id
		JOIN sessions ses ON ses.id = asi.session_id
		JOIN courses c ON c.id = sa.course_id
		JOIN courses sit_c ON sit_c.id = COALESCE(sa.sit_in_course_id, ses.course_id)
		LEFT JOIN students st ON st.wcode = sa.wcode
		LEFT JOIN users u ON u.id = ses.teacher_id
		WHERE sa.status NOT IN ('cancelled')
		  AND sa.sit_in_method = 'physical'
		  AND ses.deleted_at IS NULL
		  AND (ses.start_at AT TIME ZONE $2)::date = $1::date
		ORDER BY ses.start_at ASC
	`, todayDate, instituteTZ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TodaySitInRow
	for rows.Next() {
		var d TodaySitInRow
		if err := rows.Scan(
			&d.StudentName,
			&d.StudentNickname,
			&d.CourseCode,
			&d.CourseName,
			&d.SitInCourseCode,
			&d.SitInCourseName,
			&d.StartAt,
			&d.EndAt,
			&d.TeacherName,
			&d.TeacherEmail,
			&d.AbsenceDateRange,
		); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if results == nil {
		results = []TodaySitInRow{}
	}
	return results, nil
}
