package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

func (q *Queries) AutoArchiveExpiredSitIns(ctx context.Context, instituteTZ string, actorID pgtype.UUID) ([]pgtype.UUID, error) {
	timezone := strings.TrimSpace(instituteTZ)
	if timezone == "" {
		timezone = "Asia/Bangkok"
	}

	rows, err := q.db.Query(ctx, `
		UPDATE student_absences sa
		SET status = 'actioned',
		    reviewed_by = $2,
		    reviewed_at = COALESCE(sa.reviewed_at, CURRENT_TIMESTAMP),
		    updated_at = CURRENT_TIMESTAMP,
		    version = sa.version + 1
		WHERE sa.status IN ('pending', 'reviewed')
		  AND sa.sit_in_method = 'physical'
		  AND (
			SELECT MAX((sess.start_at AT TIME ZONE $1)::date)
			FROM absence_sit_ins asi
			JOIN sessions sess ON sess.id = asi.session_id
			WHERE asi.absence_id = sa.id
		  ) < (CURRENT_TIMESTAMP AT TIME ZONE $1)::date
		RETURNING sa.id
	`, timezone, actorID)
	if err != nil {
		return nil, fmt.Errorf("auto-archive expired sit-ins: %w", err)
	}
	defer rows.Close()

	archived := make([]pgtype.UUID, 0)
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan auto-archived absence: %w", err)
		}
		archived = append(archived, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read auto-archived absences: %w", err)
	}
	return archived, nil
}
