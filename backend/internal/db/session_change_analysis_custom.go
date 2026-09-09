package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

func (q *Queries) SessionChangeIsLatestForAnalysis(ctx context.Context, changeID pgtype.UUID) (bool, error) {
	var isLatest bool
	err := q.db.QueryRow(ctx, `
		WITH change_row AS (
			SELECT session_id, session_version
			FROM session_changes
			WHERE id = $1
			FOR UPDATE
		)
		SELECT NOT EXISTS (
			SELECT 1
			FROM session_changes newer
			JOIN change_row ON newer.session_id = change_row.session_id
			WHERE newer.session_version > change_row.session_version
              AND (newer.old_start_at IS DISTINCT FROM newer.new_start_at
                OR newer.old_end_at IS DISTINCT FROM newer.new_end_at
                OR newer.changed_fields ?| ARRAY['deleted', 'restored'])
		)
	`, changeID).Scan(&isLatest)
	if err != nil {
		return false, fmt.Errorf("check current session change: %w", err)
	}
	return isLatest, nil
}
