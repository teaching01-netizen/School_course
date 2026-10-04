package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// AbsenceSitInImpact retains assignment evidence even when the live session is removed.
type AbsenceSitInImpact struct {
	AbsenceID          pgtype.UUID
	SessionID          pgtype.UUID
	AssignmentSnapshot []byte
	AssignmentQuality  string
	IssueSnapshot      []byte
	IssueQuality       string
	Current            ManagedAbsenceSession
}

func (q *Queries) AbsenceSitInImpactsByAbsenceIDs(ctx context.Context, ids []pgtype.UUID) ([]AbsenceSitInImpact, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx, `

 WITH affected AS (
   SELECT i.*, COALESCE(i.sit_in_session_id,
     CASE WHEN i.issue_type = 'sit_in_session_deleted' THEN sc.session_id END) AS affected_session_id
   FROM absence_schedule_issues i
   LEFT JOIN session_changes sc ON sc.id = i.latest_session_change_id
   WHERE i.absence_id = ANY($1::uuid[]) AND i.status IN ('open', 'needs_review')
     AND (i.sit_in_session_id IS NOT NULL OR i.issue_type = 'sit_in_session_deleted')
 )
 SELECT DISTINCT ON (i.absence_id, i.affected_session_id)
   i.absence_id, i.affected_session_id,
   asi.session_snapshot_at_assignment, COALESCE(asi.snapshot_quality, 'unavailable'),
   i.assignment_snapshot_at_detection, i.assignment_snapshot_quality,
   s.id, s.course_id, COALESCE(c.code, ''), COALESCE(c.name, ''), subj.name, room.name, s.start_at, s.end_at
 FROM affected i
 LEFT JOIN absence_sit_ins asi ON asi.absence_id = i.absence_id AND asi.session_id = i.affected_session_id
 LEFT JOIN sessions s ON s.id = i.affected_session_id AND s.deleted_at IS NULL
 LEFT JOIN courses c ON c.id = s.course_id
 LEFT JOIN subjects subj ON subj.id = c.subject_id
 LEFT JOIN rooms room ON room.id = s.room_id
 ORDER BY i.absence_id, i.affected_session_id,
   (i.assignment_snapshot_quality IN ('exact', 'reconstructed')) DESC, i.detected_at ASC, i.id ASC
 `, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AbsenceSitInImpact
	for rows.Next() {
		var item AbsenceSitInImpact
		if err := rows.Scan(&item.AbsenceID, &item.SessionID, &item.AssignmentSnapshot, &item.AssignmentQuality,
			&item.IssueSnapshot, &item.IssueQuality, &item.Current.SessionID, &item.Current.CourseID,
			&item.Current.CourseCode, &item.Current.CourseName, &item.Current.SubjectName, &item.Current.RoomName,
			&item.Current.StartAt, &item.Current.EndAt); err != nil {
			return nil, err
		}
		item.Current.ID = item.Current.SessionID
		out = append(out, item)
	}
	return out, rows.Err()
}
