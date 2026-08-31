package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type ScheduleImpactQueueFilter struct {
	Status   string
	Severity string
	Query    string
	Limit    int32
	Offset   int32
}

type ScheduleImpactQueueRow struct {
	ID                        pgtype.UUID
	AbsenceID                 pgtype.UUID
	IssueType                 string
	Severity                  string
	Status                    string
	IssueVersion              int32
	SourceSessionID           pgtype.UUID
	SitInSessionID            pgtype.UUID
	MissedSessionID           pgtype.UUID
	WCode                     string
	StudentName               pgtype.Text
	StudentEmail              pgtype.Text
	StudentPhone              pgtype.Text
	CourseCode                string
	CourseName                string
	SubjectName               string
	StartAt                   pgtype.Timestamptz
	EndAt                     pgtype.Timestamptz
	UpdatedAt                 pgtype.Timestamptz
	Details                   []byte
	SuggestedResolutions      []byte
	AssignmentSnapshotJSON    []byte
	AssignmentSnapshotQuality string
	AssignmentSnapshotSource  pgtype.Text
	AssignedAt                pgtype.Timestamptz
	LatestSessionChangeID     pgtype.UUID
	ImpactAnalysisStatus      pgtype.Text
	AssignedToUsername        pgtype.Text
	ReviewReason              pgtype.Text
	ReviewDueAt               pgtype.Timestamptz
	ResolutionAction          pgtype.Text
}

type ScheduleImpactSummary struct {
	OpenCount                int64
	CriticalCount            int64
	WarningCount             int64
	NotificationFailureCount int64
}

func (q *Queries) SessionChangeImpactRunCreate(ctx context.Context, changeID pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO session_change_impact_runs (session_change_id, status)
		VALUES ($1, 'pending')
		ON CONFLICT (session_change_id) DO NOTHING
	`, changeID)
	return err
}

func (q *Queries) SessionChangeImpactRunSetStatus(ctx context.Context, changeID pgtype.UUID, status, lastError string) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO session_change_impact_runs (session_change_id, status, last_error, started_at, completed_at)
		VALUES (
			$1, $2, NULLIF($3, ''),
			CASE WHEN $2 = 'processing' THEN now() ELSE NULL END,
			CASE WHEN $2 = 'completed' THEN now() ELSE NULL END
		)
		ON CONFLICT (session_change_id) DO UPDATE
		SET status = EXCLUDED.status,
		    last_error = EXCLUDED.last_error,
		    started_at = CASE WHEN EXCLUDED.status = 'processing' THEN now() ELSE session_change_impact_runs.started_at END,
		    completed_at = CASE WHEN EXCLUDED.status = 'completed' THEN now() ELSE session_change_impact_runs.completed_at END,
		    processing_attempt = CASE WHEN EXCLUDED.status = 'processing' THEN session_change_impact_runs.processing_attempt + 1 ELSE session_change_impact_runs.processing_attempt END,
		    updated_at = now()
	`, changeID, status, lastError)
	return err
}

// SessionChangeImpactRunSetResult records the processing result of an impact analysis run.
func (q *Queries) SessionChangeImpactRunSetResult(ctx context.Context, changeID pgtype.UUID, resultJSON []byte, issueIDs []pgtype.UUID, errorCategory string, retryable bool) error {
	_, err := q.db.Exec(ctx, `
		UPDATE session_change_impact_runs
		SET analysis_result = convert_from($2, 'UTF8')::jsonb,
		    created_issue_ids = $3,
		    error_category = NULLIF($4, ''),
		    retryable = $5,
		    processed_at = now(),
		    updated_at = now()
		WHERE session_change_id = $1
	`, changeID, resultJSON, issueIDs, errorCategory, retryable)
	return err
}

func (q *Queries) ScheduleImpactQueue(ctx context.Context, filter ScheduleImpactQueueFilter) ([]ScheduleImpactQueueRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT i.id, i.absence_id, i.issue_type, i.severity, i.status, i.issue_version,
		       i.source_session_id, i.sit_in_session_id, i.missed_session_id,
		       sa.wcode, sa.student_name, sa.student_email, sa.student_phone,
		       COALESCE(c.code, ''), COALESCE(c.name, ''), COALESCE(subj.name, ''),
		       s.start_at, s.end_at, i.updated_at, i.details_json, i.suggested_resolution_json,
		       i.assignment_snapshot_at_detection, i.assignment_snapshot_quality, i.assignment_snapshot_source,
		       asi.assigned_at,
		       i.latest_session_change_id, run.status, assignee.username,
		       i.review_reason, i.review_due_at, i.resolution_action
		FROM absence_schedule_issues i
		JOIN student_absences sa ON sa.id = i.absence_id
		LEFT JOIN sessions s ON s.id = COALESCE(i.sit_in_session_id, i.missed_session_id, i.source_session_id)
		LEFT JOIN courses c ON c.id = s.course_id
		LEFT JOIN subjects subj ON subj.id = c.subject_id
		LEFT JOIN absence_sit_ins asi ON asi.absence_id = i.absence_id AND asi.session_id = i.sit_in_session_id
		LEFT JOIN session_change_impact_runs run ON run.session_change_id = i.latest_session_change_id
		LEFT JOIN users assignee ON assignee.id = i.assigned_to
		WHERE i.status IN ('open', 'needs_review')
		  AND ($1 = '' OR i.status = $1)
		  AND ($2 = '' OR i.severity = $2)
		  AND ($3 = '' OR concat_ws(' ', sa.wcode, sa.student_name, c.code, c.name) ILIKE '%' || $3 || '%')
		ORDER BY CASE WHEN i.severity = 'critical' THEN 0 ELSE 1 END,
		         CASE WHEN i.status = 'open' THEN 0 WHEN i.status = 'needs_review' THEN 1 ELSE 2 END,
		         i.updated_at DESC, i.id DESC
		LIMIT $4 OFFSET $5
	`, filter.Status, filter.Severity, filter.Query, filter.Limit, filter.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ScheduleImpactQueueRow, 0)
	for rows.Next() {
		var item ScheduleImpactQueueRow
		if err := rows.Scan(
			&item.ID, &item.AbsenceID, &item.IssueType, &item.Severity, &item.Status, &item.IssueVersion,
			&item.SourceSessionID, &item.SitInSessionID, &item.MissedSessionID,
			&item.WCode, &item.StudentName, &item.StudentEmail, &item.StudentPhone,
			&item.CourseCode, &item.CourseName, &item.SubjectName,
			&item.StartAt, &item.EndAt, &item.UpdatedAt, &item.Details, &item.SuggestedResolutions,
			&item.AssignmentSnapshotJSON, &item.AssignmentSnapshotQuality, &item.AssignmentSnapshotSource,
			&item.AssignedAt,
			&item.LatestSessionChangeID, &item.ImpactAnalysisStatus, &item.AssignedToUsername,
			&item.ReviewReason, &item.ReviewDueAt, &item.ResolutionAction,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *Queries) ScheduleImpactQueueSummary(ctx context.Context) (ScheduleImpactSummary, error) {
	var summary ScheduleImpactSummary
	err := q.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE i.status IN ('open', 'needs_review')),
		       count(*) FILTER (WHERE i.status IN ('open', 'needs_review') AND i.severity = 'critical'),
		       count(*) FILTER (WHERE i.status IN ('open', 'needs_review') AND i.severity = 'warning'),
		       count(DISTINCT n.id) FILTER (WHERE i.status IN ('open', 'needs_review') AND n.status IN ('failed', 'dead_letter'))
		FROM absence_schedule_issues i
		LEFT JOIN notification_outbox n ON n.absence_id = i.absence_id
	`).Scan(&summary.OpenCount, &summary.CriticalCount, &summary.WarningCount, &summary.NotificationFailureCount)
	return summary, err
}

func (q *Queries) ScheduleIssueSummaries(ctx context.Context, sessionIDs []pgtype.UUID) (map[string]map[string]int32, error) {
	rows, err := q.db.Query(ctx, `
		SELECT session_id, count(*)::int4,
		       count(*) FILTER (WHERE severity = 'critical')::int4
		FROM (
			SELECT source_session_id AS session_id, severity FROM absence_schedule_issues WHERE status IN ('open', 'needs_review')
			UNION ALL SELECT sit_in_session_id, severity FROM absence_schedule_issues WHERE status IN ('open', 'needs_review')
			UNION ALL SELECT missed_session_id, severity FROM absence_schedule_issues WHERE status IN ('open', 'needs_review')
		) related
		WHERE session_id = ANY($1::uuid[])
		GROUP BY session_id
	`, sessionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]map[string]int32)
	for rows.Next() {
		var sessionID pgtype.UUID
		var openCount, criticalCount int32
		if err := rows.Scan(&sessionID, &openCount, &criticalCount); err != nil {
			return nil, err
		}
		result[sessionID.String()] = map[string]int32{"open_count": openCount, "critical_count": criticalCount}
	}
	return result, rows.Err()
}

func (q *Queries) CandidateDetails(ctx context.Context, absenceID, sessionID pgtype.UUID) (map[string]any, error) {
	row := q.db.QueryRow(ctx, `
		SELECT s.id, s.version, s.start_at, s.end_at, COALESCE(c.code, ''), COALESCE(c.name, ''),
		       COALESCE(r.name, ''), r.capacity, COALESCE(u.username, ''),
		       count(sa.student_id)::int4
		FROM sessions s
		JOIN courses c ON c.id = s.course_id
		LEFT JOIN rooms r ON r.id = s.room_id
		LEFT JOIN users u ON u.id = s.teacher_id
		LEFT JOIN session_attendance sa ON sa.session_id = s.id AND sa.status <> 'excluded'
		WHERE s.id = $1 AND s.deleted_at IS NULL
		GROUP BY s.id, c.code, c.name, r.name, r.capacity, u.username
	`, sessionID)
	var id pgtype.UUID
	var version int32
	var startAt, endAt pgtype.Timestamptz
	var courseCode, courseName, roomName, teacher string
	var capacity pgtype.Int4
	var occupied int32
	if err := row.Scan(&id, &version, &startAt, &endAt, &courseCode, &courseName, &roomName, &capacity, &teacher, &occupied); err != nil {
		return nil, err
	}
	available := int32(-1)
	if capacity.Valid {
		available = capacity.Int32 - occupied
	}
	return map[string]any{
		"session_id": id.String(), "session_version": version, "start_at": startAt.Time,
		"end_at": endAt.Time, "course_code": courseCode, "course_name": courseName,
		"room_name": roomName, "teacher": teacher, "available_capacity": available,
		"eligible": true, "student_conflicts": false, "generated_at": time.Now().UTC(),
	}, nil
}

func (q *Queries) IssueActivity(ctx context.Context, absenceID pgtype.UUID) ([]map[string]any, error) {
	rows, err := q.db.Query(ctx, `
		SELECT action, reason, created_at
		FROM absence_sit_in_assignment_events
		WHERE absence_id = $1
		ORDER BY created_at DESC
		LIMIT 10
	`, absenceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var action, reason string
		var createdAt time.Time
		if err := rows.Scan(&action, &reason, &createdAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"action": action, "reason": reason, "created_at": createdAt})
	}
	return items, rows.Err()
}

func (q *Queries) ResolveScheduleIssueVersioned(ctx context.Context, issueID pgtype.UUID, expectedVersion int32) error {
	commandTag, err := q.db.Exec(ctx, `
		UPDATE absence_schedule_issues
		SET issue_version = issue_version + 1, updated_at = now()
		WHERE id = $1 AND issue_version = $2 AND status IN ('open', 'needs_review')
	`, issueID, expectedVersion)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
