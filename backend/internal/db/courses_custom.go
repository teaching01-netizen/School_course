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

func (q *Queries) CourseUpdateLegacyLink(ctx context.Context, courseID pgtype.UUID, legacyCourseID pgtype.Text) error {
	_, err := q.db.Exec(ctx, `
		UPDATE courses SET legacy_course_id = $1, updated_at = NOW() WHERE id = $2
	`, legacyCourseID, courseID)
	return err
}
