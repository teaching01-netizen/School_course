package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type CourseOverviewRow struct {
	ID           pgtype.UUID   `json:"id"`
	CourseNo     int64         `json:"course_no"`
	Code         string        `json:"code"`
	Name         string        `json:"name"`
	Year         pgtype.Int2   `json:"year"`
	TeacherID    pgtype.UUID   `json:"teacher_id"`
	TeacherName  string        `json:"teacher_name"`
	SubjectID    pgtype.UUID   `json:"subject_id"`
	SubjectCode  string        `json:"subject_code"`
	SubjectName  string        `json:"subject_name"`
	Hour         pgtype.Int4   `json:"hour"`
	StudentCount pgtype.Int4   `json:"student_count"`
	CourseType   pgtype.Text   `json:"course_type"`
	CreatedAt    pgtype.Timestamptz `json:"created_at"`
	UpdatedAt    pgtype.Timestamptz `json:"updated_at"`
}

type CourseCreateV2Params struct {
	Year         pgtype.Int2
	TeacherID    pgtype.UUID
	SubjectID    pgtype.UUID
	Hour         pgtype.Int4
	StudentCount pgtype.Int4
	CourseType   string
}

func (q *Queries) CourseCreateV2(ctx context.Context, p CourseCreateV2Params) (CourseOverviewRow, error) {
	var row CourseOverviewRow
	err := q.db.QueryRow(ctx, `
		WITH next AS (SELECT nextval('course_no_seq') AS n)
		INSERT INTO courses (course_no, code, name, year, teacher_id, subject_id, hour, student_count, course_type)
		SELECT next.n,
		       lpad(next.n::text, 10, '0'),
		       '', -- name is derived in UI; keep empty for now
		       $1, $2, $3, $4, $5, $6
		FROM next
		RETURNING id, course_no, code, name, year, teacher_id, subject_id, hour, student_count, course_type, created_at, updated_at
	`, p.Year, p.TeacherID, p.SubjectID, p.Hour, p.StudentCount, p.CourseType).Scan(
		&row.ID,
		&row.CourseNo,
		&row.Code,
		&row.Name,
		&row.Year,
		&row.TeacherID,
		&row.SubjectID,
		&row.Hour,
		&row.StudentCount,
		&row.CourseType,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return CourseOverviewRow{}, err
	}

	// Hydrate teacher + subject labels for the response (single extra query each; small lists).
	_ = q.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, row.TeacherID).Scan(&row.TeacherName)
	_ = q.db.QueryRow(ctx, `SELECT code, name FROM subjects WHERE id = $1`, row.SubjectID).Scan(&row.SubjectCode, &row.SubjectName)

	return row, nil
}

type CourseOverviewParams struct {
	IncludeArchived bool
}

func (q *Queries) StudentCoursesList(ctx context.Context, studentID pgtype.UUID) ([]CourseOverviewRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.course_no, c.code, c.name, c.year, c.teacher_id, COALESCE(u.username, ''), c.subject_id, COALESCE(s.code, ''), COALESCE(s.name, ''),
		       c.hour, c.student_count, c.course_type, c.created_at, c.updated_at
		FROM course_students cs
		JOIN courses c ON c.id = cs.course_id
		LEFT JOIN users u ON u.id = c.teacher_id
		LEFT JOIN subjects s ON s.id = c.subject_id
		WHERE cs.student_id = $1
		ORDER BY c.code ASC
	`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CourseOverviewRow
	for rows.Next() {
		var r CourseOverviewRow
		if err := rows.Scan(
			&r.ID, &r.CourseNo, &r.Code, &r.Name, &r.Year,
			&r.TeacherID, &r.TeacherName, &r.SubjectID, &r.SubjectCode, &r.SubjectName,
			&r.Hour, &r.StudentCount, &r.CourseType,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) CourseOverview(ctx context.Context, p CourseOverviewParams) ([]CourseOverviewRow, error) {
	var rows pgx.Rows
	var err error
	if p.IncludeArchived {
		rows, err = q.db.Query(ctx, `
			SELECT c.id, c.course_no, c.code, c.name, c.year, c.teacher_id, COALESCE(u.username, ''), c.subject_id, COALESCE(s.code, ''), COALESCE(s.name, ''),
			       c.hour, c.student_count, c.course_type, c.created_at, c.updated_at
			FROM courses c
			LEFT JOIN users u ON u.id = c.teacher_id
			LEFT JOIN subjects s ON s.id = c.subject_id
			ORDER BY c.course_no DESC
		`)
	} else {
		rows, err = q.db.Query(ctx, `
			SELECT c.id, c.course_no, c.code, c.name, c.year, c.teacher_id, COALESCE(u.username, ''), c.subject_id, COALESCE(s.code, ''), COALESCE(s.name, ''),
			       c.hour, c.student_count, c.course_type, c.created_at, c.updated_at
			FROM courses c
			LEFT JOIN users u ON u.id = c.teacher_id
			LEFT JOIN subjects s ON s.id = c.subject_id
			ORDER BY c.course_no DESC
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CourseOverviewRow
	for rows.Next() {
		var r CourseOverviewRow
		if err := rows.Scan(
			&r.ID, &r.CourseNo, &r.Code, &r.Name, &r.Year,
			&r.TeacherID, &r.TeacherName, &r.SubjectID, &r.SubjectCode, &r.SubjectName,
			&r.Hour, &r.StudentCount, &r.CourseType,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
