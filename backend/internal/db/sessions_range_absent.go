package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// SessionsRangeAbsentParams binds the batched already-absent lookup.
// The window is the half-open instant range [FromUTC, ToExclusiveUTC),
// converted by the caller from inclusive institute days.
//
// already_absent(session) = true iff a non-cancelled absence covers the
// session institute day under merge-group equivalence. Cancelled absences
// never mark sessions. This preserves sessionsAlreadyAbsentSelectSQL
// exactly (which excludes only status = cancelled).
type SessionsRangeAbsentParams struct {
	Wcode          string
	InstituteTZ    string
	FromUTC        time.Time
	ToExclusiveUTC time.Time
}

// SessionsRangeAlreadyAbsent returns the set of session-ID strings covered
// by non-cancelled absences in the window. Exactly one Query call.
func (q *Queries) SessionsRangeAlreadyAbsent(ctx context.Context, arg SessionsRangeAbsentParams) (map[string]bool, error) {
	rows, err := q.db.Query(ctx, `
		SELECT DISTINCT sess.id
		FROM sessions sess
		JOIN student_absences sa ON (
			sa.course_id = sess.course_id
			OR EXISTS (
				SELECT 1
				FROM course_merge_group_members merge_member
				WHERE merge_member.group_id = sa.merge_group_id
				  AND merge_member.course_id = sess.course_id
			)
		)
		WHERE sa.wcode = $1
		  AND sa.status <> 'cancelled'
		  AND sess.start_at >= $3
		  AND sess.start_at < $4
		  AND (sess.start_at AT TIME ZONE $2)::date BETWEEN sa.date_from AND sa.date_to
		  AND sess.deleted_at IS NULL
	`, arg.Wcode, arg.InstituteTZ, arg.FromUTC, arg.ToExclusiveUTC)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[uuidBytesString(id)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
