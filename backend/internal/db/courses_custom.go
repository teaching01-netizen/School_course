package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

func (q *Queries) CourseList(ctx context.Context) ([]Course, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, code, name, created_at, updated_at
		FROM courses
		ORDER BY code ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Course
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

type LegacyCourseFields struct {
	LegacyCourseID    pgtype.Text        `json:"legacy_course_id"`
	LegacyLastSyncedAt pgtype.Timestamptz `json:"legacy_last_synced_at"`
	TeacherID         pgtype.UUID        `json:"teacher_id"`
}

func (q *Queries) CourseGetLegacyFields(ctx context.Context, courseID pgtype.UUID) (LegacyCourseFields, error) {
	var out LegacyCourseFields
	err := q.db.QueryRow(ctx, `
		SELECT legacy_course_id, legacy_last_synced_at, teacher_id
		FROM courses
		WHERE id = $1
	`, courseID).Scan(&out.LegacyCourseID, &out.LegacyLastSyncedAt, &out.TeacherID)
	return out, err
}

func (q *Queries) CourseGetFull(ctx context.Context, courseID pgtype.UUID) (CourseOverviewRow, error) {
	var row CourseOverviewRow
	err := q.db.QueryRow(ctx, `
		SELECT c.id, c.course_no, c.code, c.name, c.year,
		       c.teacher_id, COALESCE(u.username, ''),
		       c.subject_id, COALESCE(s.code, ''), COALESCE(s.name, ''),
		       c.hour, c.student_count, c.course_type,
		       c.created_at, c.updated_at,
		       c.legacy_course_id, c.legacy_last_synced_at,
		       c.cohort_id, COALESCE(ch.name, '')
		FROM courses c
		LEFT JOIN users u ON u.id = c.teacher_id
		LEFT JOIN subjects s ON s.id = c.subject_id
		LEFT JOIN course_cohorts ch ON ch.id = c.cohort_id
		WHERE c.id = $1
	`, courseID).Scan(
		&row.ID, &row.CourseNo, &row.Code, &row.Name, &row.Year,
		&row.TeacherID, &row.TeacherName, &row.SubjectID, &row.SubjectCode, &row.SubjectName,
		&row.Hour, &row.StudentCount, &row.CourseType,
		&row.CreatedAt, &row.UpdatedAt,
		&row.LegacyCourseID, &row.LegacyLastSyncedAt,
		&row.CohortID, &row.CohortName,
	)
	return row, err
}

type CourseUpdateFullParams struct {
	ID        pgtype.UUID
	Code      string
	Name      string
	TeacherID pgtype.UUID
}

func (q *Queries) CourseUpdateFull(ctx context.Context, p CourseUpdateFullParams) (CourseOverviewRow, error) {
	var row CourseOverviewRow
	err := q.db.QueryRow(ctx, `
		UPDATE courses
		SET code = $2, name = $3, teacher_id = $4, updated_at = now()
		WHERE id = $1
		RETURNING id, course_no, code, name, year, teacher_id, hour, student_count, course_type, created_at, updated_at, cohort_id
	`, p.ID, p.Code, p.Name, p.TeacherID).Scan(
		&row.ID, &row.CourseNo, &row.Code, &row.Name, &row.Year,
		&row.TeacherID, &row.Hour, &row.StudentCount, &row.CourseType,
		&row.CreatedAt, &row.UpdatedAt,
		&row.CohortID,
	)
	if err != nil {
		return CourseOverviewRow{}, err
	}
	// Hydrate teacher + subject labels for the response.
	_ = q.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, row.TeacherID).Scan(&row.TeacherName)
	_ = q.db.QueryRow(ctx, `SELECT COALESCE(code, ''), COALESCE(name, '') FROM subjects WHERE id = $1`, row.SubjectID).Scan(&row.SubjectCode, &row.SubjectName)
	// Legacy fields: read back separately.
	_ = q.db.QueryRow(ctx, `SELECT legacy_course_id, legacy_last_synced_at FROM courses WHERE id = $1`, p.ID).Scan(&row.LegacyCourseID, &row.LegacyLastSyncedAt)
	// Cohort name: read back separately.
	_ = q.db.QueryRow(ctx, `SELECT COALESCE(ch.name, '') FROM course_cohorts ch WHERE ch.id = $1`, row.CohortID).Scan(&row.CohortName)
	return row, err
}

func (q *Queries) CourseUpdateLegacyLink(ctx context.Context, courseID pgtype.UUID, legacyCourseID pgtype.Text) error {
	_, err := q.db.Exec(ctx, `
		UPDATE courses SET legacy_course_id = $1, updated_at = NOW() WHERE id = $2
	`, legacyCourseID, courseID)
	return err
}

type CourseCohort struct {
	ID        pgtype.UUID        `json:"id"`
	Name      string             `json:"name"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

func (q *Queries) CourseCohortFindOrCreate(ctx context.Context, name string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := q.db.QueryRow(ctx, `
		INSERT INTO course_cohorts (name)
		VALUES ($1)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, name).Scan(&id)
	return id, err
}

func (q *Queries) CourseCohortList(ctx context.Context) ([]CourseCohort, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, name, created_at
		FROM course_cohorts
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CourseCohort
	for rows.Next() {
		var c CourseCohort
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type CourseBatchDeleteResult struct {
	ID      pgtype.UUID
	Success bool
	Error   string
}

func (q *Queries) CourseBatchDelete(ctx context.Context, ids []pgtype.UUID) []CourseBatchDeleteResult {
	results := make([]CourseBatchDeleteResult, 0, len(ids))
	for _, id := range ids {
		tag, err := q.db.Exec(ctx, `DELETE FROM courses WHERE id = $1`, id)
		if err != nil {
			results = append(results, CourseBatchDeleteResult{ID: id, Success: false, Error: err.Error()})
			continue
		}
		if tag.RowsAffected() == 0 {
			results = append(results, CourseBatchDeleteResult{ID: id, Success: false, Error: "not found"})
			continue
		}
		results = append(results, CourseBatchDeleteResult{ID: id, Success: true})
	}
	return results
}
