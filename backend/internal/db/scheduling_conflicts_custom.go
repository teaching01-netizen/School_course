package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

func (q *Queries) SessionSetConflictOverride(ctx context.Context, sessionID pgtype.UUID, enabled bool) error {
	_, err := q.db.Exec(ctx, `
		UPDATE sessions
		SET conflict_override = $2, updated_at = now(), version = version + 1
		WHERE id = $1
	`, sessionID, enabled)
	return err
}

func (q *Queries) SessionBusyRangesSetConflictOverride(ctx context.Context, sessionID pgtype.UUID, enabled bool) error {
	_, err := q.db.Exec(ctx, `
		UPDATE student_busy_ranges
		SET conflict_override = $2
		WHERE session_id = $1 AND deleted_at IS NULL
	`, sessionID, enabled)
	return err
}

func (q *Queries) CourseSessionsSetConflictOverride(ctx context.Context, courseID pgtype.UUID, enabled bool) error {
	_, err := q.db.Exec(ctx, `
		UPDATE sessions
		SET conflict_override = $2, updated_at = now(), version = version + 1
		WHERE course_id = $1 AND deleted_at IS NULL
	`, courseID, enabled)
	return err
}

func (q *Queries) CourseBusyRangesSetConflictOverride(ctx context.Context, courseID pgtype.UUID, enabled bool) error {
	_, err := q.db.Exec(ctx, `
		UPDATE student_busy_ranges br
		SET conflict_override = $2
		FROM sessions s
		WHERE br.session_id = s.id AND s.course_id = $1 AND br.deleted_at IS NULL
	`, courseID, enabled)
	return err
}
