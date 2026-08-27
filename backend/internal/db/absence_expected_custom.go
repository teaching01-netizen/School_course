package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

func (q *Queries) ValidExpectedMissedSessionTiming(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID, instituteTZ string) ([]MissedSessionTimingRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT sess.id, sess.start_at, sess.end_at
		FROM sessions sess
		JOIN student_absences sa ON sa.id = $1
		JOIN students st ON lower(st.wcode) = lower(sa.wcode)
		WHERE sess.id = ANY($2::uuid[])
		  AND (
		    sess.course_id = sa.course_id
		    OR EXISTS (
		      SELECT 1
		      FROM course_merge_group_members sess_member
		      JOIN course_merge_group_members absence_member
		        ON absence_member.group_id = sess_member.group_id
		      WHERE sess_member.course_id = sess.course_id
		        AND absence_member.course_id = sa.course_id
		    )
		  )
		  AND sess.deleted_at IS NULL
		  AND (sess.start_at AT TIME ZONE $3)::date BETWEEN sa.date_from AND sa.date_to
		  AND student_is_expected_at_session(st.id, sess.id)
		ORDER BY sess.start_at ASC
	`, absenceID, sessionIDs, instituteTZ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]MissedSessionTimingRow, 0, len(sessionIDs))
	for rows.Next() {
		var item MissedSessionTimingRow
		if err := rows.Scan(&item.ID, &item.StartAt, &item.EndAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
