package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type ScheduleImpactProcessingRow struct {
	ID          pgtype.UUID
	CourseCode  string
	CourseName  string
	SubjectName string
	CreatedAt   pgtype.Timestamptz
	Status      string
	LastError   pgtype.Text
}

func (q *Queries) ScheduleImpactProcessing(ctx context.Context, limit int32) ([]ScheduleImpactProcessingRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT sc.id, COALESCE(c.code, ''), COALESCE(c.name, ''), COALESCE(subj.name, ''),
		       sc.created_at, run.status, run.last_error
		FROM session_change_impact_runs run
		JOIN session_changes sc ON sc.id = run.session_change_id
		LEFT JOIN courses c ON c.id = sc.new_course_id
		LEFT JOIN subjects subj ON subj.id = c.subject_id
		WHERE run.status IN ('pending', 'processing', 'failed', 'delayed_by_batch')
		ORDER BY sc.created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ScheduleImpactProcessingRow, 0)
	for rows.Next() {
		var item ScheduleImpactProcessingRow
		if err := rows.Scan(&item.ID, &item.CourseCode, &item.CourseName, &item.SubjectName, &item.CreatedAt, &item.Status, &item.LastError); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
