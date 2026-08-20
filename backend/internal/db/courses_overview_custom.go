package db

import (
	"context"

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
	Version            pgtype.Int4        `json:"version"`
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

// CourseOverviewParams filters and paginates the courses list. Archived toggles
// the live/archived bucket (legacy_archived); CourseType is "" (all), "private"
// (Private), or "general" (General from the legacy site plus native Group);
// TeacherID is "" (all), "none" (no primary teacher), or a user uuid; Q is a
// substring search over code, name, subject, teacher, and roster membership.
// Limit 0 means no limit (the bare-array response path); Offset is only
// meaningful with a limit.
type CourseOverviewParams struct {
	Archived   bool
	CourseType string
	TeacherID  string
	Q          string
	Limit      int32
	Offset     int32
}

// courseOverviewWhere is the shared filter for the list and count queries.
// Placeholders $1-$4 are archived, course type, teacher, and search text; the
// sentinel-style conditions keep every branch constant so a single query shape
// serves all filter combinations.
const courseOverviewWhere = `
		WHERE c.legacy_archived = $1
		  AND ($2 = ''
		       OR ($2 = 'private' AND c.course_type = 'Private')
		       OR ($2 = 'general' AND c.course_type IN ('General', 'Group')))
		  AND ($3 = ''
		       OR ($3 = 'none' AND c.teacher_id IS NULL)
		       OR c.teacher_id::text = $3)
		  AND ($4 = ''
		       OR c.course_no::text ILIKE '%' || $4 || '%'
		       OR c.id::text ILIKE '%' || $4 || '%'
		       OR c.code ILIKE '%' || $4 || '%'
		       OR c.name ILIKE '%' || $4 || '%'
		       OR s.code ILIKE '%' || $4 || '%'
		       OR s.name ILIKE '%' || $4 || '%'
		       OR u.full_name ILIKE '%' || $4 || '%'
		       OR u.username ILIKE '%' || $4 || '%'
		       OR EXISTS (
		           SELECT 1 FROM course_teachers ct
		           JOIN users ctu ON ctu.id = ct.teacher_id
		           WHERE ct.course_id = c.id
		             AND (ctu.full_name ILIKE '%' || $4 || '%' OR ctu.username ILIKE '%' || $4 || '%')
		       ))`

func courseOverviewFilterArgs(p CourseOverviewParams) []any {
	return []any{p.Archived, p.CourseType, p.TeacherID, p.Q}
}

func (q *Queries) StudentCoursesList(ctx context.Context, studentID pgtype.UUID) ([]CourseOverviewRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.course_no, c.code, c.name, c.year, c.teacher_id, COALESCE(NULLIF(u.full_name, ''), u.username, ''), c.subject_id, COALESCE(s.code, ''), COALESCE(s.name, ''),
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
	rows, err := q.db.Query(ctx, `
		SELECT c.id, c.course_no, c.code, c.name, c.year, c.teacher_id, COALESCE(NULLIF(u.full_name, ''), u.username, ''), c.subject_id, COALESCE(s.code, ''), COALESCE(s.name, ''),
		       c.hour, COALESCE(roster.student_count, 0)::int4, c.course_type, c.created_at, c.updated_at,
		       c.legacy_course_id, c.legacy_last_synced_at
		FROM courses c
		LEFT JOIN users u ON u.id = c.teacher_id
		LEFT JOIN subjects s ON s.id = c.subject_id
		LEFT JOIN (
			SELECT course_id, COUNT(*) FILTER (WHERE status = 'enrolled') AS student_count
			FROM course_students
			GROUP BY course_id
		) roster ON roster.course_id = c.id
		`+courseOverviewWhere+`
		ORDER BY c.course_no DESC
		LIMIT NULLIF($5, 0) OFFSET $6
	`, append(courseOverviewFilterArgs(p), p.Limit, p.Offset)...)
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

// CourseOverviewCount returns the number of courses matching the same filters
// as CourseOverview (users/subjects joins are 1:1, so COUNT over the same FROM
// is exact). It backs the paginated envelope's total_count.
func (q *Queries) CourseOverviewCount(ctx context.Context, p CourseOverviewParams) (int64, error) {
	var total int64
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM courses c
		LEFT JOIN users u ON u.id = c.teacher_id
		LEFT JOIN subjects s ON s.id = c.subject_id
		`+courseOverviewWhere,
		courseOverviewFilterArgs(p)...,
	).Scan(&total)
	return total, err
}
