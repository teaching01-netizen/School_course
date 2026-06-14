package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type CourseOverviewRow struct {
	ID                 pgtype.UUID        `json:"id"`
	CourseNo           int64              `json:"course_no"`
	Code               string             `json:"code"`
	Name               string             `json:"name"`
	Year               pgtype.Int2        `json:"year"`
	TeacherID          pgtype.UUID        `json:"teacher_id"`
	TeacherName        string             `json:"teacher_name"`
	SubjectID          pgtype.UUID        `json:"subject_id"`
	SubjectCode        string             `json:"subject_code"`
	SubjectName        string             `json:"subject_name"`
	Hour               pgtype.Int4        `json:"hour"`
	StudentCount       pgtype.Int4        `json:"student_count"`
	CourseType         pgtype.Text        `json:"course_type"`
	CreatedAt          pgtype.Timestamptz `json:"created_at"`
	UpdatedAt          pgtype.Timestamptz `json:"updated_at"`
	LegacyCourseID     pgtype.Text        `json:"legacy_course_id"`
	LegacyLastSyncedAt pgtype.Timestamptz `json:"legacy_last_synced_at"`
	CohortID           pgtype.UUID        `json:"cohort_id"`
	CohortName         string             `json:"cohort_name"`
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
		RETURNING id, course_no, code, name, year, teacher_id, subject_id, hour, student_count, course_type, created_at, updated_at, cohort_id
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
		&row.CohortID,
	)
	if err != nil {
		return CourseOverviewRow{}, err
	}

	// Hydrate teacher + subject labels for the response (single extra query each; small lists).
	_ = q.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, row.TeacherID).Scan(&row.TeacherName)
	_ = q.db.QueryRow(ctx, `SELECT code, name FROM subjects WHERE id = $1`, row.SubjectID).Scan(&row.SubjectCode, &row.SubjectName)
	_ = q.db.QueryRow(ctx, `SELECT COALESCE(ch.name, '') FROM course_cohorts ch WHERE ch.id = $1`, row.CohortID).Scan(&row.CohortName)

	return row, nil
}

type CourseOverviewParams struct {
	IncludeArchived bool
}

func (q *Queries) StudentCoursesList(ctx context.Context, studentID pgtype.UUID) ([]CourseOverviewRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.course_no, c.code, c.name, c.year, c.teacher_id, COALESCE(u.username, ''), c.subject_id, COALESCE(s.code, ''), COALESCE(s.name, ''),
		       c.hour, c.student_count, c.course_type, c.created_at, c.updated_at,
		       c.cohort_id, COALESCE(ch.name, '')
		FROM course_students cs
		JOIN courses c ON c.id = cs.course_id
		LEFT JOIN users u ON u.id = c.teacher_id
		LEFT JOIN subjects s ON s.id = c.subject_id
		LEFT JOIN course_cohorts ch ON ch.id = c.cohort_id
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
			&r.CohortID, &r.CohortName,
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
	query := `
		SELECT c.id, c.course_no, c.code, c.name, c.year, c.teacher_id, COALESCE(u.username, ''), c.subject_id, COALESCE(s.code, ''), COALESCE(s.name, ''),
		       c.hour, COALESCE(roster.student_count, 0)::int4, c.course_type, c.created_at, c.updated_at,
		       c.legacy_course_id, c.legacy_last_synced_at,
		       c.cohort_id, COALESCE(ch.name, '')
		FROM courses c
		LEFT JOIN users u ON u.id = c.teacher_id
		LEFT JOIN subjects s ON s.id = c.subject_id
		LEFT JOIN (
			SELECT course_id, COUNT(*) FILTER (WHERE status = 'enrolled') AS student_count
			FROM course_students
			GROUP BY course_id
		) roster ON roster.course_id = c.id
		LEFT JOIN course_cohorts ch ON ch.id = c.cohort_id
		ORDER BY c.course_no DESC
	`
	if p.IncludeArchived {
		rows, err = q.db.Query(ctx, query)
	} else {
		rows, err = q.db.Query(ctx, query)
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
			&r.LegacyCourseID, &r.LegacyLastSyncedAt,
			&r.CohortID, &r.CohortName,
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
