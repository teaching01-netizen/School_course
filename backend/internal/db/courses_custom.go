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
	LegacyCourseID     pgtype.Text        `json:"legacy_course_id"`
	LegacyLastSyncedAt pgtype.Timestamptz `json:"legacy_last_synced_at"`
	TeacherID          pgtype.UUID        `json:"teacher_id"`
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
		       c.version, c.cycle_id, COALESCE(cy.display_name, cy.label, ''), c.expiry_days,
		       (SELECT MAX(sess.end_at) FROM sessions sess WHERE sess.course_id = c.id AND sess.deleted_at IS NULL),
		       c.absence_form_visible
		FROM courses c
		LEFT JOIN users u ON u.id = c.teacher_id
		LEFT JOIN subjects s ON s.id = c.subject_id
		LEFT JOIN crm_cycles cy ON cy.id = c.cycle_id
		WHERE c.id = $1
	`, courseID).Scan(
		&row.ID, &row.CourseNo, &row.Code, &row.Name, &row.Year,
		&row.TeacherID, &row.TeacherName, &row.SubjectID, &row.SubjectCode, &row.SubjectName,
		&row.Hour, &row.StudentCount, &row.CourseType,
		&row.CreatedAt, &row.UpdatedAt,
		&row.LegacyCourseID, &row.LegacyLastSyncedAt,
		&row.Version, &row.CycleID, &row.CycleLabel, &row.ExpiryDays, &row.LastSessionAt,
		&row.AbsenceFormVisible,
	)
	return row, err
}

// CourseAbsenceFormVisibleUpdate applies the optional absence-form visibility
// flag: nil leaves the current value untouched (single-property PATCHes omit
// it), non-nil sets it. Mirrors CourseLifecycleUpdate's set-flag pattern.
func (q *Queries) CourseAbsenceFormVisibleUpdate(ctx context.Context, courseID pgtype.UUID, visible *bool) error {
	_, err := q.db.Exec(ctx, `
		UPDATE courses
		SET absence_form_visible = CASE WHEN $2 THEN $3 ELSE absence_form_visible END,
		    updated_at = now()
		WHERE id = $1
	`, courseID, visible != nil, visible)
	return err
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
