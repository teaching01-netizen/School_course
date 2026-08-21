package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/snapshot"
)

type AbsenceFilter struct {
	Query              string
	IDs                []pgtype.UUID
	SubjectID          pgtype.UUID
	Status             string
	Bucket             string
	ScheduleImpactOnly bool
	DateFrom           pgtype.Date
	DateTo             pgtype.Date
	Limit              int32
	Offset             int32
}

type ManagedAbsenceRow struct {
	ID                     pgtype.UUID
	Wcode                  string
	StudentName            pgtype.Text
	StudentEmail           pgtype.Text
	StudentNickname        pgtype.Text
	StudentPhone           pgtype.Text
	ParentPhone            pgtype.Text
	CourseID               pgtype.UUID
	CourseCode             string
	CourseName             string
	SubjectID              pgtype.UUID
	SubjectCode            pgtype.Text
	SubjectName            pgtype.Text
	DateFrom               pgtype.Date
	DateTo                 pgtype.Date
	ReasonCategory         pgtype.Text
	Reason                 pgtype.Text
	SitInMethod            pgtype.Text
	SitInCourseID          pgtype.UUID
	SitInCourseCode        pgtype.Text
	SitInCourseName        pgtype.Text
	SitInSubjectName       pgtype.Text
	Status                 string
	AdminNotes             pgtype.Text
	ReviewedBy             pgtype.UUID
	ReviewedAt             pgtype.Timestamptz
	SitInOverridden        bool
	SitInOverriddenBy      pgtype.UUID
	SitInOverrideReason    pgtype.Text
	Version                int32
	CreatedAt              pgtype.Timestamptz
	UpdatedAt              pgtype.Timestamptz
	OpenScheduleIssues     int32
	CriticalScheduleIssues int32
	LatestSessionChangeID  pgtype.UUID
}

func normalizedAbsencePaging(p AbsenceFilter) AbsenceFilter {
	if p.Limit <= 0 {
		p.Limit = 25
	}
	if p.Limit > 10000 {
		p.Limit = 10000
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	// A nil slice binds as SQL NULL, making cardinality($8::uuid[]) = 0
	// evaluate to NULL and silently filter out every row.
	if p.IDs == nil {
		p.IDs = []pgtype.UUID{}
	}
	return p
}

const absenceStudentNicknameExprPlaceholder = "__STUDENT_NICKNAME_EXPR__"

const managedAbsenceListQueryTemplate = `
		SELECT sa.id, sa.wcode, COALESCE(sa.student_name, st.full_name),
		       COALESCE(sa.student_email, COALESCE(st.email_crm, st.email_system)), __STUDENT_NICKNAME_EXPR__,
		       sa.student_phone,
		       st.parent_phone,
		       sa.course_id, c.code, c.name, sa.subject_id, sub.code, sub.name,
		       sa.date_from, sa.date_to, sa.reason_category, sa.reason, sa.sit_in_method,
		       sa.sit_in_course_id, sc.code, sc.name, COALESCE(sit_sub.name, sit_sc_sub.name), sa.status, sa.admin_notes,
		       sa.reviewed_by, sa.reviewed_at, sa.sit_in_overridden, sa.sit_in_overridden_by,
		       sa.sit_in_override_reason, sa.version, sa.created_at, sa.updated_at,
		       COALESCE(impact.open_issue_count, 0),
		       COALESCE(impact.critical_issue_count, 0),
		       impact.latest_session_change_id,
		       count(*) OVER()
		FROM student_absences sa
		JOIN courses c ON c.id = sa.course_id
		LEFT JOIN students st ON LOWER(st.wcode) = LOWER(sa.wcode)
		LEFT JOIN subjects sub ON sub.id = sa.subject_id
		LEFT JOIN courses sc ON sc.id = sa.sit_in_course_id
		LEFT JOIN subjects sit_sc_sub ON sit_sc_sub.id = sc.subject_id
		LEFT JOIN (
		  SELECT asi.absence_id, string_agg(DISTINCT sit_sub_inner.name, ', ' ORDER BY sit_sub_inner.name) AS name
		  FROM absence_sit_ins asi
		  JOIN sessions s ON s.id = asi.session_id
		  JOIN courses sit_courses ON sit_courses.id = s.course_id
		  JOIN subjects sit_sub_inner ON sit_sub_inner.id = sit_courses.subject_id
		  GROUP BY asi.absence_id
		) sit_sub ON sit_sub.absence_id = sa.id
		LEFT JOIN LATERAL (
		  SELECT count(*)::int4 AS open_issue_count,
		         count(*) FILTER (WHERE severity = 'critical')::int4 AS critical_issue_count,
		         (array_agg(latest_session_change_id ORDER BY updated_at DESC))[1] AS latest_session_change_id
		  FROM absence_schedule_issues
		  WHERE absence_id = sa.id AND status = 'open'
		) impact ON true
		WHERE ($1 = '' OR sa.wcode ILIKE '%' || $1 || '%' OR COALESCE(sa.student_name, st.full_name, '') ILIKE '%' || $1 || '%' OR COALESCE(__STUDENT_NICKNAME_EXPR__, '') ILIKE '%' || $1 || '%')
		  AND ($2::uuid IS NULL OR sa.subject_id = $2)
		  AND ($3 = '' OR sa.status = $3)
		  AND ($4::date IS NULL OR sa.date_to >= $4)
		  AND ($5::date IS NULL OR sa.date_from <= $5)
		  AND (cardinality($8::uuid[]) = 0 OR sa.id = ANY($8::uuid[]))
		  -- keep status buckets in sync with internal/absences/status.go (AllStatuses); active=pending+reviewed, archived=actioned+cancelled+special_approved
		  AND ($10::boolean OR $9 = '' OR $3 <> '' OR ($9 = 'active' AND sa.status IN ('pending', 'reviewed')) OR ($9 = 'archived' AND sa.status IN ('actioned', 'cancelled', 'special_approved')))
		  AND (NOT $10::boolean OR COALESCE(impact.open_issue_count, 0) > 0)
		ORDER BY (COALESCE(impact.open_issue_count, 0) > 0) DESC,
		         (COALESCE(impact.critical_issue_count, 0) > 0) DESC,
		         sa.created_at DESC, sa.id DESC
		LIMIT $6 OFFSET $7
`

const managedAbsenceGetQueryTemplate = `
		SELECT sa.id, sa.wcode, COALESCE(sa.student_name, st.full_name),
		       COALESCE(sa.student_email, COALESCE(st.email_crm, st.email_system)), __STUDENT_NICKNAME_EXPR__,
		       sa.student_phone,
		       st.parent_phone,
		       sa.course_id, c.code, c.name, sa.subject_id, sub.code, sub.name,
		       sa.date_from, sa.date_to, sa.reason_category, sa.reason, sa.sit_in_method,
		       sa.sit_in_course_id, sc.code, sc.name, COALESCE(sit_sub.name, sit_sc_sub.name), sa.status, sa.admin_notes,
		       sa.reviewed_by, sa.reviewed_at, sa.sit_in_overridden, sa.sit_in_overridden_by,
		       sa.sit_in_override_reason, sa.version, sa.created_at, sa.updated_at
		FROM student_absences sa
		JOIN courses c ON c.id = sa.course_id
		LEFT JOIN students st ON LOWER(st.wcode) = LOWER(sa.wcode)
		LEFT JOIN subjects sub ON sub.id = sa.subject_id
		LEFT JOIN courses sc ON sc.id = sa.sit_in_course_id
		LEFT JOIN subjects sit_sc_sub ON sit_sc_sub.id = sc.subject_id
		LEFT JOIN (
		  SELECT asi.absence_id, string_agg(DISTINCT sit_sub_inner.name, ', ' ORDER BY sit_sub_inner.name) AS name
		  FROM absence_sit_ins asi
		  JOIN sessions s ON s.id = asi.session_id
		  JOIN courses sit_courses ON sit_courses.id = s.course_id
		  JOIN subjects sit_sub_inner ON sit_sub_inner.id = sit_courses.subject_id
		  GROUP BY asi.absence_id
		) sit_sub ON sit_sub.absence_id = sa.id
		WHERE sa.id = $1
`

func managedAbsenceQuerySQL(template string, hasStudentNicknameColumn bool) string {
	studentNicknameExpr := "st.nickname"
	if hasStudentNicknameColumn {
		studentNicknameExpr = "COALESCE(sa.student_nickname, st.nickname)"
	}
	return strings.ReplaceAll(template, absenceStudentNicknameExprPlaceholder, studentNicknameExpr)
}

func (q *Queries) absenceStudentNicknameColumnExists(ctx context.Context) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_attribute
			WHERE attrelid = 'public.student_absences'::regclass
			  AND attname = 'student_nickname'
			  AND NOT attisdropped
		)
	`).Scan(&exists)
	return exists, err
}

func (q *Queries) ManagedAbsenceList(ctx context.Context, p AbsenceFilter) ([]ManagedAbsenceRow, int64, error) {
	p = normalizedAbsencePaging(p)
	hasStudentNicknameColumn, err := q.absenceStudentNicknameColumnExists(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.db.Query(ctx, managedAbsenceQuerySQL(managedAbsenceListQueryTemplate, hasStudentNicknameColumn), p.Query, p.SubjectID, p.Status, p.DateFrom, p.DateTo, p.Limit, p.Offset, p.IDs, p.Bucket, p.ScheduleImpactOnly)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]ManagedAbsenceRow, 0)
	var total int64
	for rows.Next() {
		var item ManagedAbsenceRow
		if err := rows.Scan(
			&item.ID, &item.Wcode, &item.StudentName, &item.StudentEmail, &item.StudentNickname, &item.StudentPhone,
			&item.ParentPhone,
			&item.CourseID, &item.CourseCode, &item.CourseName, &item.SubjectID, &item.SubjectCode, &item.SubjectName,
			&item.DateFrom, &item.DateTo, &item.ReasonCategory, &item.Reason, &item.SitInMethod,
			&item.SitInCourseID, &item.SitInCourseCode, &item.SitInCourseName, &item.SitInSubjectName, &item.Status, &item.AdminNotes,
			&item.ReviewedBy, &item.ReviewedAt, &item.SitInOverridden, &item.SitInOverriddenBy,
			&item.SitInOverrideReason, &item.Version, &item.CreatedAt, &item.UpdatedAt,
			&item.OpenScheduleIssues, &item.CriticalScheduleIssues, &item.LatestSessionChangeID,
			&total,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (q *Queries) OpenAbsenceScheduleIssueSummary(ctx context.Context) (int64, int64, error) {
	var affectedAbsences int64
	var criticalIssues int64
	err := q.db.QueryRow(ctx, `
		SELECT count(DISTINCT absence_id),
		       count(*) FILTER (WHERE severity = 'critical')
		FROM absence_schedule_issues
		WHERE status = 'open'
	`).Scan(&affectedAbsences, &criticalIssues)
	return affectedAbsences, criticalIssues, err
}

func (q *Queries) ManagedAbsenceGet(ctx context.Context, id pgtype.UUID) (ManagedAbsenceRow, error) {
	var item ManagedAbsenceRow
	hasStudentNicknameColumn, err := q.absenceStudentNicknameColumnExists(ctx)
	if err != nil {
		return item, err
	}
	err = q.db.QueryRow(ctx, managedAbsenceQuerySQL(managedAbsenceGetQueryTemplate, hasStudentNicknameColumn), id).Scan(
		&item.ID, &item.Wcode, &item.StudentName, &item.StudentEmail, &item.StudentNickname, &item.StudentPhone,
		&item.ParentPhone,
		&item.CourseID, &item.CourseCode, &item.CourseName, &item.SubjectID, &item.SubjectCode, &item.SubjectName,
		&item.DateFrom, &item.DateTo, &item.ReasonCategory, &item.Reason, &item.SitInMethod,
		&item.SitInCourseID, &item.SitInCourseCode, &item.SitInCourseName, &item.SitInSubjectName, &item.Status, &item.AdminNotes,
		&item.ReviewedBy, &item.ReviewedAt, &item.SitInOverridden, &item.SitInOverriddenBy,
		&item.SitInOverrideReason, &item.Version, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

type ManagedAbsenceSession struct {
	AbsenceID   pgtype.UUID
	ID          pgtype.UUID
	SessionID   pgtype.UUID
	CourseID    pgtype.UUID
	CourseCode  string
	CourseName  string
	SubjectName pgtype.Text
	RoomName    pgtype.Text
	StartAt     pgtype.Timestamptz
	EndAt       pgtype.Timestamptz
}

func (q *Queries) ManagedAbsenceMissedSessions(ctx context.Context, absenceID pgtype.UUID) ([]ManagedAbsenceSession, error) {
	rows, err := q.db.Query(ctx, `
		SELECT ams.absence_id, ams.id, sess.id, sess.course_id, c.code, c.name, subj.name, room.name, sess.start_at, sess.end_at
		FROM absence_missed_sessions ams
		JOIN sessions sess ON sess.id = ams.session_id AND sess.deleted_at IS NULL
		JOIN courses c ON c.id = sess.course_id
		LEFT JOIN subjects subj ON subj.id = c.subject_id
		LEFT JOIN rooms room ON room.id = sess.room_id
		WHERE ams.absence_id = $1
		ORDER BY sess.start_at ASC
	`, absenceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManagedAbsenceSession
	for rows.Next() {
		var session ManagedAbsenceSession
		if err := rows.Scan(&session.AbsenceID, &session.ID, &session.SessionID, &session.CourseID, &session.CourseCode, &session.CourseName, &session.SubjectName, &session.RoomName, &session.StartAt, &session.EndAt); err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func (q *Queries) ManagedAbsenceMissedSessionsByAbsenceIDs(ctx context.Context, absenceIDs []pgtype.UUID) ([]ManagedAbsenceSession, error) {
	if len(absenceIDs) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx, `
		SELECT ams.absence_id, ams.id, sess.id, sess.course_id, c.code, c.name, subj.name, room.name, sess.start_at, sess.end_at
		FROM absence_missed_sessions ams
		JOIN sessions sess ON sess.id = ams.session_id AND sess.deleted_at IS NULL
		JOIN courses c ON c.id = sess.course_id
		LEFT JOIN subjects subj ON subj.id = c.subject_id
		LEFT JOIN rooms room ON room.id = sess.room_id
		WHERE ams.absence_id = ANY($1::uuid[])
		ORDER BY ams.absence_id, sess.start_at ASC, ams.id ASC
	`, absenceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManagedAbsenceSession
	for rows.Next() {
		var session ManagedAbsenceSession
		if err := rows.Scan(&session.AbsenceID, &session.ID, &session.SessionID, &session.CourseID, &session.CourseCode, &session.CourseName, &session.SubjectName, &session.RoomName, &session.StartAt, &session.EndAt); err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func (q *Queries) ManagedAbsenceSessions(ctx context.Context, absenceID pgtype.UUID) ([]ManagedAbsenceSession, error) {
	rows, err := q.db.Query(ctx, `
		SELECT asi.absence_id, asi.id, sess.id, sess.course_id, c.code, c.name, subj.name, room.name, sess.start_at, sess.end_at
		FROM absence_sit_ins asi
		JOIN sessions sess ON sess.id = asi.session_id AND sess.deleted_at IS NULL
		JOIN courses c ON c.id = sess.course_id
		LEFT JOIN subjects subj ON subj.id = c.subject_id
		LEFT JOIN rooms room ON room.id = sess.room_id
		WHERE asi.absence_id = $1
		ORDER BY sess.start_at ASC
	`, absenceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManagedAbsenceSession
	for rows.Next() {
		var session ManagedAbsenceSession
		if err := rows.Scan(&session.AbsenceID, &session.ID, &session.SessionID, &session.CourseID, &session.CourseCode, &session.CourseName, &session.SubjectName, &session.RoomName, &session.StartAt, &session.EndAt); err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func (q *Queries) ManagedAbsenceSessionsByAbsenceIDs(ctx context.Context, absenceIDs []pgtype.UUID) ([]ManagedAbsenceSession, error) {
	if len(absenceIDs) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx, `
		SELECT asi.absence_id, asi.id, sess.id, sess.course_id, c.code, c.name, subj.name, room.name, sess.start_at, sess.end_at
		FROM absence_sit_ins asi
		JOIN sessions sess ON sess.id = asi.session_id AND sess.deleted_at IS NULL
		JOIN courses c ON c.id = sess.course_id
		LEFT JOIN subjects subj ON subj.id = c.subject_id
		LEFT JOIN rooms room ON room.id = sess.room_id
		WHERE asi.absence_id = ANY($1::uuid[])
		ORDER BY asi.absence_id, sess.start_at ASC, asi.id ASC
	`, absenceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManagedAbsenceSession
	for rows.Next() {
		var session ManagedAbsenceSession
		if err := rows.Scan(&session.AbsenceID, &session.ID, &session.SessionID, &session.CourseID, &session.CourseCode, &session.CourseName, &session.SubjectName, &session.RoomName, &session.StartAt, &session.EndAt); err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

type SitInStudentRow struct {
	AbsenceID               pgtype.UUID
	SessionID               pgtype.UUID
	Wcode                   string
	Nickname                pgtype.Text
	StudentName             pgtype.Text
	FromCourseCode          string
	FromCourseName          pgtype.Text
	FromSubjectName         pgtype.Text
	SessionStartAt          pgtype.Timestamptz
	SessionEndAt            pgtype.Timestamptz
	AbsentCourseSubjectName pgtype.Text
	AbsenceDateFrom         pgtype.Date
}

type AbsentStudentRow struct {
	SessionID   pgtype.UUID
	Wcode       string
	Nickname    pgtype.Text
	StudentName pgtype.Text
	AbsenceID   pgtype.UUID
	CreatedAt   pgtype.Timestamptz
}

type AbsenceCountRow struct {
	SessionID pgtype.UUID
	Count     int32
}

func (q *Queries) AbsentStudentsBySessionIDs(ctx context.Context, sessionIDs []pgtype.UUID) ([]AbsentStudentRow, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx, `
		SELECT ams.session_id, sa.wcode,
		       st.nickname,
		       COALESCE(st.full_name, '') AS student_name,
		       sa.id AS absence_id,
		       sa.created_at
		FROM absence_missed_sessions ams
		JOIN student_absences sa ON sa.id = ams.absence_id AND sa.status <> 'cancelled'
		LEFT JOIN students st ON LOWER(st.wcode) = LOWER(sa.wcode)
		WHERE ams.session_id = ANY($1::uuid[])
		ORDER BY ams.session_id, st.full_name
	`, sessionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AbsentStudentRow
	for rows.Next() {
		var r AbsentStudentRow
		if err := rows.Scan(&r.SessionID, &r.Wcode, &r.Nickname, &r.StudentName, &r.AbsenceID, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (q *Queries) AbsenceCountBySessionIDs(ctx context.Context, sessionIDs []pgtype.UUID) ([]AbsenceCountRow, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx, `
		SELECT ams.session_id, COUNT(DISTINCT ams.absence_id)::int4
		FROM absence_missed_sessions ams
		JOIN student_absences sa ON sa.id = ams.absence_id AND sa.status <> 'cancelled'
		WHERE ams.session_id = ANY($1::uuid[])
		GROUP BY ams.session_id
	`, sessionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AbsenceCountRow
	for rows.Next() {
		var r AbsenceCountRow
		if err := rows.Scan(&r.SessionID, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (q *Queries) SitInsBySessionIDs(ctx context.Context, sessionIDs []pgtype.UUID) ([]SitInStudentRow, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx, `
		SELECT asi.absence_id, asi.session_id, sa.wcode,
		       st.nickname,
		       COALESCE(st.full_name, '') AS student_name,
		       c.code AS from_course_code,
		       COALESCE(c.name, '') AS from_course_name,
		       COALESCE(sub.name, '') AS from_subject_name,
		       sess.start_at, sess.end_at,
		       COALESCE(absent_sub.name, '') AS absent_subject_name,
		       sa.date_from
		FROM absence_sit_ins asi
		JOIN student_absences sa ON sa.id = asi.absence_id
		LEFT JOIN students st ON LOWER(st.wcode) = LOWER(sa.wcode)
		LEFT JOIN courses c ON c.id = sa.sit_in_course_id
		LEFT JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN sessions sess ON sess.id = asi.session_id AND sess.deleted_at IS NULL
		LEFT JOIN courses absent_c ON absent_c.id = sa.course_id
		LEFT JOIN subjects absent_sub ON absent_sub.id = absent_c.subject_id
		WHERE asi.session_id = ANY($1::uuid[])
		ORDER BY asi.session_id, sa.wcode
	`, sessionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SitInStudentRow
	for rows.Next() {
		var r SitInStudentRow
		if err := rows.Scan(&r.AbsenceID, &r.SessionID, &r.Wcode, &r.Nickname, &r.StudentName, &r.FromCourseCode, &r.FromCourseName, &r.FromSubjectName, &r.SessionStartAt, &r.SessionEndAt, &r.AbsentCourseSubjectName, &r.AbsenceDateFrom); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type AbsenceAuditInsertParams struct {
	AbsenceID pgtype.UUID
	Action    string
	ActorID   pgtype.UUID
	ActorRole string
	Details   any
}

type AbsenceAuditEntry struct {
	ID        pgtype.UUID
	Action    string
	ActorID   pgtype.UUID
	ActorName pgtype.Text
	ActorRole string
	Details   []byte
	CreatedAt pgtype.Timestamptz
}

func (q *Queries) AbsenceAuditInsert(ctx context.Context, p AbsenceAuditInsertParams) error {
	raw, err := json.Marshal(p.Details)
	if err != nil {
		return fmt.Errorf("marshal absence audit details: %w", err)
	}
	role := p.ActorRole
	if role == "" {
		role = "admin"
	}
	_, err = q.db.Exec(ctx, `
		INSERT INTO absence_audit_log (absence_id, action, actor_id, actor_role, details)
		VALUES ($1, $2, $3, $4, $5::jsonb)
	`, p.AbsenceID, p.Action, p.ActorID, role, string(raw))
	return err
}

func (q *Queries) AbsenceAuditList(ctx context.Context, absenceID pgtype.UUID) ([]AbsenceAuditEntry, error) {
	rows, err := q.db.Query(ctx, `
		SELECT al.id, al.action, al.actor_id, u.username, al.actor_role, al.details, al.created_at
		FROM absence_audit_log al
		LEFT JOIN users u ON u.id = al.actor_id
		WHERE al.absence_id = $1
		ORDER BY al.created_at DESC, al.id DESC
	`, absenceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AbsenceAuditEntry
	for rows.Next() {
		var item AbsenceAuditEntry
		if err := rows.Scan(&item.ID, &item.Action, &item.ActorID, &item.ActorName, &item.ActorRole, &item.Details, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (q *Queries) AbsenceSetSubmissionMetadata(ctx context.Context, id, subjectID pgtype.UUID, method pgtype.Text, studentName string, studentEmail pgtype.Text, studentNickname pgtype.Text, studentPhone pgtype.Text, reasonCategory pgtype.Text, sitInCourseID pgtype.UUID) error {
	hasStudentNicknameColumn, err := q.absenceStudentNicknameColumnExists(ctx)
	if err != nil {
		return err
	}
	if hasStudentNicknameColumn {
		_, err = q.db.Exec(ctx, `
			UPDATE student_absences
			SET subject_id = $2, sit_in_method = $3, student_name = $4, student_email = $5, student_nickname = $6, student_phone = $7, reason_category = $8, sit_in_course_id = $9, updated_at = now()
			WHERE id = $1
		`, id, subjectID, method, studentName, studentEmail, studentNickname, studentPhone, reasonCategory, sitInCourseID)
		return err
	}
	_, err = q.db.Exec(ctx, `
		UPDATE student_absences
		SET subject_id = $2, sit_in_method = $3, student_name = $4, student_email = $5, student_phone = $6, reason_category = $7, sit_in_course_id = $8, updated_at = now()
		WHERE id = $1
	`, id, subjectID, method, studentName, studentEmail, studentPhone, reasonCategory, sitInCourseID)
	return err
}

func (q *Queries) AbsenceStatusUpdate(ctx context.Context, id pgtype.UUID, status string, actorID pgtype.UUID, expectedVersion int32) (int32, error) {
	var version int32
	err := q.db.QueryRow(ctx, `
		UPDATE student_absences
		SET status = $2,
		    reviewed_by = CASE WHEN $2 IN ('reviewed', 'actioned') THEN $3 ELSE reviewed_by END,
		    reviewed_at = CASE WHEN $2 IN ('reviewed', 'actioned') AND reviewed_at IS NULL THEN now() ELSE reviewed_at END,
		    updated_at = now(),
		    version = version + 1
		WHERE id = $1 AND version = $4
		RETURNING version
	`, id, status, actorID, expectedVersion).Scan(&version)
	return version, err
}

func (q *Queries) AbsenceNotesUpdate(ctx context.Context, id pgtype.UUID, notes string, expectedVersion int32) (int32, error) {
	var version int32
	err := q.db.QueryRow(ctx, `
		UPDATE student_absences
		SET admin_notes = NULLIF($2, ''), updated_at = now(), version = version + 1
		WHERE id = $1 AND version = $3
		RETURNING version
	`, id, notes, expectedVersion).Scan(&version)
	return version, err
}

func (q *Queries) AbsenceSitInUpdate(ctx context.Context, id pgtype.UUID, method string, courseID pgtype.UUID, actorID pgtype.UUID, reason string, expectedVersion int32) (int32, error) {
	var version int32
	err := q.db.QueryRow(ctx, `
		UPDATE student_absences
		SET sit_in_method = $2, sit_in_course_id = $3, sit_in_overridden = true,
		    sit_in_overridden_by = $4, sit_in_override_reason = $5,
		    updated_at = now(), version = version + 1
		WHERE id = $1 AND version = $6
		RETURNING version
	`, id, method, courseID, actorID, reason, expectedVersion).Scan(&version)
	return version, err
}

func (q *Queries) AbsenceHardDelete(ctx context.Context, id pgtype.UUID, expectedVersion int32) (int32, error) {
	var one int32
	err := q.db.QueryRow(ctx, `
		DELETE FROM student_absences
		WHERE id = $1
		  AND (version = $2 OR status IN ('cancelled', 'actioned', 'special_approved'))
		RETURNING 1
	`, id, expectedVersion).Scan(&one)
	return one, err
}

// AbsenceSitInsReplace replaces all sit-in assignments for an absence.
//
// Deprecated: Use AbsenceSitInsReplaceWithSnapshot instead. This method does not
// populate snapshot columns on new assignments, which violates the domain invariant
// that all new sit-in assignments must carry a session snapshot.
func (q *Queries) AbsenceSitInsReplace(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID) error {
	type beginner interface {
		Begin(context.Context) (pgx.Tx, error)
	}
	work := q
	var tx pgx.Tx
	if db, ok := q.db.(beginner); ok {
		var err error
		tx, err = db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback(ctx)
		work = New(tx)
	}
	rows, err := work.db.Query(ctx, `SELECT session_id FROM absence_sit_ins WHERE absence_id = $1 FOR UPDATE`, absenceID)
	if err != nil {
		return fmt.Errorf("lock sit-ins: %w", err)
	}
	var previous []pgtype.UUID
	for rows.Next() {
		var sessionID pgtype.UUID
		if err := rows.Scan(&sessionID); err != nil {
			rows.Close()
			return fmt.Errorf("scan sit-in: %w", err)
		}
		previous = append(previous, sessionID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if _, err := work.db.Exec(ctx, `DELETE FROM absence_sit_ins WHERE absence_id = $1`, absenceID); err != nil {
		return fmt.Errorf("delete sit-ins: %w", err)
	}

	for _, sid := range sessionIDs {
		if _, err := work.db.Exec(ctx, `
			INSERT INTO absence_sit_ins (absence_id, session_id, session_version_at_assignment, assigned_at, assignment_source)
			SELECT $1, id, version, now(), 'absence_override'
			FROM sessions
			WHERE id = $2
			ON CONFLICT DO NOTHING
		`, absenceID, sid); err != nil {
			return fmt.Errorf("insert sit-in: %w", err)
		}
	}
	for _, sid := range previous {
		if _, err := work.db.Exec(ctx, `
			INSERT INTO absence_sit_in_assignment_events (absence_id, previous_session_id, action, reason)
			VALUES ($1, $2, 'cancelled', 'absence_override')
		`, absenceID, sid); err != nil {
			return fmt.Errorf("record cancelled sit-in: %w", err)
		}
	}
	for _, sid := range sessionIDs {
		if _, err := work.db.Exec(ctx, `
			INSERT INTO absence_sit_in_assignment_events (absence_id, new_session_id, action, reason)
			VALUES ($1, $2, 'assigned', 'absence_override')
		`, absenceID, sid); err != nil {
			return fmt.Errorf("record assigned sit-in: %w", err)
		}
	}
	if tx != nil {
		return tx.Commit(ctx)
	}
	return nil
}

// AbsenceSitInsReplaceWithSnapshot replaces all sit-in assignments for an absence,
// building session snapshots for each new assignment.
//
// Steps:
//  1. Lock existing assignment rows
//  2. Delete old assignments
//  3. For each new session, load it, build a snapshot, and insert with snapshot metadata
//  4. Write cancelled events for old assignments
//  5. Write assigned events for new assignments
func (q *Queries) AbsenceSitInsReplaceWithSnapshot(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID, timezone string, snapshotFunc SnapshotBuilderFunc) error {
	type beginner interface {
		Begin(context.Context) (pgx.Tx, error)
	}
	work := q
	var tx pgx.Tx
	if db, ok := q.db.(beginner); ok {
		var err error
		tx, err = db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback(ctx)
		work = New(tx)
	}

	// 1. Lock existing assignment rows.
	rows, err := work.db.Query(ctx, `SELECT session_id FROM absence_sit_ins WHERE absence_id = $1 FOR UPDATE`, absenceID)
	if err != nil {
		return fmt.Errorf("lock sit-ins: %w", err)
	}
	var previous []pgtype.UUID
	for rows.Next() {
		var sessionID pgtype.UUID
		if err := rows.Scan(&sessionID); err != nil {
			rows.Close()
			return fmt.Errorf("scan sit-in: %w", err)
		}
		previous = append(previous, sessionID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// 2. Delete old assignments.
	if _, err := work.db.Exec(ctx, `DELETE FROM absence_sit_ins WHERE absence_id = $1`, absenceID); err != nil {
		return fmt.Errorf("delete sit-ins: %w", err)
	}

	// 3. For each new session, build snapshot and insert with snapshot metadata.
	capturedAt := time.Now().UTC()
	capturedAtPg := pgtype.Timestamptz{Time: capturedAt, Valid: true}

	for _, sid := range sessionIDs {
		row, err := work.SessionGetByIDForSnapshot(ctx, sid)
		if err != nil {
			return fmt.Errorf("load session for snapshot %s: %w", sid.String(), err)
		}

		var roomName *string
		if row.RoomName.Valid {
			roomName = &row.RoomName.String
		}

		snapshotJSON, schemaVersion, err := snapshotFunc(
			row.CourseCode, row.CourseName, row.TeacherName, roomName,
			sid, row.SeriesID, row.CourseID, row.RoomID, row.TeacherID,
			row.StartAt, row.EndAt, row.Version, capturedAt, timezone,
		)
		if err != nil {
			return fmt.Errorf("build snapshot for session %s: %w", sid.String(), err)
		}

		if _, err := work.db.Exec(ctx, `
			INSERT INTO absence_sit_ins (
				absence_id, session_id, session_version_at_assignment,
				assigned_at, assignment_source,
				session_snapshot_at_assignment, snapshot_schema_version,
				snapshot_captured_at, snapshot_quality, snapshot_source
			)
			SELECT $1, id, version, now(), 'absence_override',
			       $3, $4, $5, 'exact', 'captured_at_assignment'
			FROM sessions
			WHERE id = $2
			ON CONFLICT DO NOTHING
		`, absenceID, sid, snapshotJSON, schemaVersion, capturedAtPg); err != nil {
			return fmt.Errorf("insert sit-in with snapshot: %w", err)
		}
	}

	// 4. Write cancelled events for old assignments.
	for _, sid := range previous {
		if _, err := work.db.Exec(ctx, `
			INSERT INTO absence_sit_in_assignment_events (absence_id, previous_session_id, action, reason)
			VALUES ($1, $2, 'cancelled', 'absence_override')
		`, absenceID, sid); err != nil {
			return fmt.Errorf("record cancelled sit-in: %w", err)
		}
	}

	// 5. Write assigned events for new assignments.
	for _, sid := range sessionIDs {
		if _, err := work.db.Exec(ctx, `
			INSERT INTO absence_sit_in_assignment_events (absence_id, new_session_id, action, reason)
			VALUES ($1, $2, 'assigned', 'absence_override')
		`, absenceID, sid); err != nil {
			return fmt.Errorf("record assigned sit-in: %w", err)
		}
	}

	if tx != nil {
		return tx.Commit(ctx)
	}
	return nil
}

func (q *Queries) AbsenceMissedSessionsCreate(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID) error {
	inputs := make([]MissedSessionSnapshotInput, len(sessionIDs))
	for i, sid := range sessionIDs {
		inputs[i] = MissedSessionSnapshotInput{SessionID: sid}
	}
	_, err := q.AbsenceMissedSessionsCreateWithSnapshot(ctx, absenceID, inputs, "Europe/London", DefaultSnapshotBuilder)
	return err
}

// DefaultSnapshotBuilder builds a SessionSnapshotV1 from session data retrieved
// during absence submission. It is the canonical snapshot builder for the db
// layer and is used by AbsenceMissedSessionsCreate.
func DefaultSnapshotBuilder(
	courseCode, courseName, teacherName string,
	roomName *string,
	sessionID pgtype.UUID,
	seriesID pgtype.UUID,
	courseID pgtype.UUID,
	roomID pgtype.UUID,
	teacherID pgtype.UUID,
	startAt, endAt pgtype.Timestamptz,
	version int32,
	capturedAt time.Time,
	timezone string,
) ([]byte, int16, error) {
	s := snapshot.AssignmentSession{
		ID:         uuidFromPgtypeDB(sessionID),
		SeriesID:   ptrUUIDDB(uuidFromPgtypeDB(seriesID)),
		CourseID:   uuidFromPgtypeDB(courseID),
		RoomID:     ptrUUIDDB(uuidFromPgtypeDB(roomID)),
		TeacherID:  uuidFromPgtypeDB(teacherID),
		StartAt:    startAt.Time.UTC(),
		EndAt:      endAt.Time.UTC(),
		Version:    version,
		CourseCode: courseCode,
		CourseName: courseName,
		TeacherName: teacherName,
		RoomName:   roomName,
	}

	if s.RoomID != nil && *s.RoomID == uuid.Nil {
		s.RoomID = nil
	}
	if s.SeriesID != nil && *s.SeriesID == uuid.Nil {
		s.SeriesID = nil
	}

	snap := snapshot.BuildSessionSnapshotV1(s, capturedAt, timezone)

	data, err := json.Marshal(snap)
	if err != nil {
		return nil, 0, err
	}

	return data, int16(snap.SchemaVersion), nil
}

func uuidFromPgtypeDB(u pgtype.UUID) uuid.UUID {
	if !u.Valid {
		return uuid.Nil
	}
	parsed, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return uuid.Nil
	}
	return parsed
}

func ptrUUIDDB(u uuid.UUID) *uuid.UUID {
	if u == uuid.Nil {
		return nil
	}
	return &u
}

// SessionVersionConflictError is returned when a session's version has changed
// since the client last loaded it, indicating a concurrent modification.
type SessionVersionConflictError struct {
	SessionID      string
	ExpectedVersion int
	ActualVersion   int
}

func (e *SessionVersionConflictError) Error() string {
	return fmt.Sprintf("session %s version conflict: expected %d, got %d", e.SessionID, e.ExpectedVersion, e.ActualVersion)
}

// MissedSessionSnapshotInput contains the data needed to build and store a
// snapshot for a single missed session at absence submission time.
type MissedSessionSnapshotInput struct {
	SessionID       pgtype.UUID
	ExpectedVersion *int32 // nil means no version check (backward compatibility)
}

// MissedSessionSnapshotData holds the snapshot JSON and metadata for a single
// missed session row in absence_missed_sessions.
type MissedSessionSnapshotData struct {
	SessionID       pgtype.UUID
	SnapshotJSON    []byte
	SchemaVersion   int16
	CapturedAt      pgtype.Timestamptz
	Quality         string
	Source          string
	Version         int32
	OriginalStartAt pgtype.Timestamptz
	OriginalEndAt   pgtype.Timestamptz
}

// AbsenceMissedSessionsCreateWithSnapshot creates missed session records with
// attached snapshots. For each session:
//  1. Loads the session with display labels (course, teacher, room)
//  2. Optionally verifies the session version matches expectedVersion
//  3. Inserts the absence_missed_sessions row with snapshot metadata
//
// If expectedVersion is non-nil and doesn't match the current session version,
// returns a *SessionVersionConflictError.
func (q *Queries) AbsenceMissedSessionsCreateWithSnapshot(
	ctx context.Context,
	absenceID pgtype.UUID,
	inputs []MissedSessionSnapshotInput,
	timezone string,
	snapshotFunc func(courseCode, courseName, teacherName string, roomName *string, sessionID pgtype.UUID, seriesID pgtype.UUID, courseID pgtype.UUID, roomID pgtype.UUID, teacherID pgtype.UUID, startAt, endAt pgtype.Timestamptz, version int32, capturedAt time.Time, tz string) ([]byte, int16, error),
) ([]MissedSessionSnapshotData, error) {
	capturedAt := time.Now().UTC()
	results := make([]MissedSessionSnapshotData, 0, len(inputs))

	for _, input := range inputs {
		row, err := q.SessionGetByIDForSnapshot(ctx, input.SessionID)
		if err != nil {
			return nil, fmt.Errorf("load session for snapshot: %w", err)
		}

		if input.ExpectedVersion != nil && row.Version != *input.ExpectedVersion {
			sessionIDStr := input.SessionID.String()
			return nil, &SessionVersionConflictError{
				SessionID:       sessionIDStr,
				ExpectedVersion: int(*input.ExpectedVersion),
				ActualVersion:   int(row.Version),
			}
		}

		var roomName *string
		if row.RoomName.Valid {
			roomName = &row.RoomName.String
		}

		snapshotJSON, schemaVersion, err := snapshotFunc(
			row.CourseCode, row.CourseName, row.TeacherName, roomName,
			input.SessionID, row.SeriesID, row.CourseID, row.RoomID, row.TeacherID,
			row.StartAt, row.EndAt, row.Version, capturedAt, timezone,
		)
		if err != nil {
			return nil, fmt.Errorf("build snapshot for session %s: %w", input.SessionID.String(), err)
		}

		_, err = q.db.Exec(ctx, `
			INSERT INTO absence_missed_sessions (
				absence_id, session_id,
				session_version_at_submission, original_start_at, original_end_at,
				session_snapshot_at_submission, snapshot_schema_version,
				snapshot_captured_at, snapshot_quality, snapshot_source
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (absence_id, session_id) DO NOTHING
		`,
			absenceID, input.SessionID,
			row.Version, row.StartAt, row.EndAt,
			string(snapshotJSON), schemaVersion,
			capturedAt, "exact", "captured_at_submission",
		)
		if err != nil {
			return nil, fmt.Errorf("insert missed session with snapshot: %w", err)
		}

		capturedAtPg := pgtype.Timestamptz{Time: capturedAt, Valid: true}
		results = append(results, MissedSessionSnapshotData{
			SessionID:       input.SessionID,
			SnapshotJSON:    snapshotJSON,
			SchemaVersion:   schemaVersion,
			CapturedAt:      capturedAtPg,
			Quality:         "exact",
			Source:          "captured_at_submission",
			Version:         row.Version,
			OriginalStartAt: row.StartAt,
			OriginalEndAt:   row.EndAt,
		})
	}

	return results, nil
}

func (q *Queries) ValidMissedSessionCount(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID, instituteTZ string) (int, error) {
	var count int
	err := q.db.QueryRow(ctx, `
		SELECT count(*)
		FROM sessions sess
		JOIN student_absences sa ON sa.id = $1
		WHERE sess.id = ANY($2::uuid[])
		  AND sess.course_id = sa.course_id
		  AND sess.deleted_at IS NULL
		  AND (sess.start_at AT TIME ZONE $3)::date BETWEEN sa.date_from AND sa.date_to
	`, absenceID, sessionIDs, instituteTZ).Scan(&count)
	return count, err
}

type MissedSessionTimingRow struct {
	ID      pgtype.UUID
	StartAt pgtype.Timestamptz
	EndAt   pgtype.Timestamptz
}

func (q *Queries) ValidMissedSessionTiming(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID, instituteTZ string) ([]MissedSessionTimingRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT sess.id, sess.start_at, sess.end_at
		FROM sessions sess
		JOIN student_absences sa ON sa.id = $1
		WHERE sess.id = ANY($2::uuid[])
		  AND sess.course_id = sa.course_id
		  AND sess.deleted_at IS NULL
		  AND (sess.start_at AT TIME ZONE $3)::date BETWEEN sa.date_from AND sa.date_to
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

func (q *Queries) ValidSitInSessionCount(ctx context.Context, absenceID, courseID pgtype.UUID, sessionIDs []pgtype.UUID, instituteTZ string) (int, error) {
	var count int
	err := q.db.QueryRow(ctx, `
		SELECT count(*)
		FROM sessions sess
		JOIN student_absences sa ON sa.id = $1
		WHERE sess.id = ANY($3::uuid[])
		  AND sess.course_id = $2
		  AND sess.deleted_at IS NULL
		  AND EXISTS (
		    SELECT 1
		    FROM sessions later
		    WHERE later.course_id = sess.course_id
		      AND later.deleted_at IS NULL
		      AND (later.start_at AT TIME ZONE $4)::date >
		          (sess.start_at AT TIME ZONE $4)::date
		  )
		  AND NOT EXISTS (
		    SELECT 1
		    FROM sessions missed
		    WHERE missed.course_id = sa.course_id
		      AND missed.deleted_at IS NULL
		      AND (missed.start_at AT TIME ZONE $4)::date BETWEEN sa.date_from AND sa.date_to
		      AND sess.start_at < missed.end_at
		      AND sess.end_at > missed.start_at
		  )
	`, absenceID, courseID, sessionIDs, instituteTZ).Scan(&count)
	return count, err
}

func (q *Queries) ValidSitInSessionOverlap(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID, instituteTZ string, excludeFinal bool) (int, error) {
	var count int
	err := q.db.QueryRow(ctx, `
		SELECT count(*)
		FROM sessions sess
		WHERE sess.id = ANY($2::uuid[])
		  AND sess.deleted_at IS NULL
		  AND (NOT $4 OR EXISTS (
		    SELECT 1
		    FROM sessions later
		    WHERE later.course_id = sess.course_id
		      AND later.deleted_at IS NULL
		      AND (later.start_at AT TIME ZONE $3)::date >
		          (sess.start_at AT TIME ZONE $3)::date
		  ))
		  AND NOT EXISTS (
		    SELECT 1
		    FROM student_absences sa
		    JOIN sessions missed ON missed.course_id = sa.course_id
		    WHERE sa.id = $1
		      AND missed.deleted_at IS NULL
		      AND (missed.start_at AT TIME ZONE $3)::date BETWEEN sa.date_from AND sa.date_to
		      AND sess.start_at < missed.end_at
		      AND sess.end_at > missed.start_at
		  )
	`, absenceID, sessionIDs, instituteTZ, excludeFinal).Scan(&count)
	return count, err
}

type SitInCandidateSession struct {
	ID           pgtype.UUID
	CourseID     pgtype.UUID
	RoomID       pgtype.UUID
	StartAt      pgtype.Timestamptz
	EndAt        pgtype.Timestamptz
	RoomName     pgtype.Text
	RoomCapacity pgtype.Int4
	Occupancy    int64
}

func (q *Queries) SitInCandidateSessions(ctx context.Context, absenceID, courseID pgtype.UUID, instituteTZ string) ([]SitInCandidateSession, error) {
	rows, err := q.db.Query(ctx, `
		SELECT sess.id, sess.course_id, sess.room_id, sess.start_at, sess.end_at,
		       room.name, room.capacity,
		       (SELECT count(*) FROM course_students cs WHERE cs.course_id = sess.course_id) +
		       (SELECT count(*) FROM absence_sit_ins asi WHERE asi.session_id = sess.id AND asi.absence_id <> $1)
		FROM sessions sess
		JOIN student_absences sa ON sa.id = $1
		LEFT JOIN rooms room ON room.id = sess.room_id
		WHERE sess.course_id = $2
		  AND sess.deleted_at IS NULL
		  AND EXISTS (
		    SELECT 1
		    FROM sessions later
		    WHERE later.course_id = sess.course_id
		      AND later.deleted_at IS NULL
		      AND (later.start_at AT TIME ZONE $3)::date >
		          (sess.start_at AT TIME ZONE $3)::date
		  )
		  AND NOT EXISTS (
		    SELECT 1
		    FROM sessions missed
		    WHERE missed.course_id = sa.course_id
		      AND missed.deleted_at IS NULL
		      AND (missed.start_at AT TIME ZONE $3)::date BETWEEN sa.date_from AND sa.date_to
		      AND sess.start_at < missed.end_at
		      AND sess.end_at > missed.start_at
		  )
		ORDER BY sess.start_at ASC
	`, absenceID, courseID, instituteTZ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SitInCandidateSession
	for rows.Next() {
		var item SitInCandidateSession
		if err := rows.Scan(&item.ID, &item.CourseID, &item.RoomID, &item.StartAt, &item.EndAt, &item.RoomName, &item.RoomCapacity, &item.Occupancy); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type AbsenceStats struct {
	TotalCount           int64 `json:"total_count"`
	PendingCount         int64 `json:"pending_count"`
	ReviewedCount        int64 `json:"reviewed_count"`
	ActionedCount        int64 `json:"actioned_count"`
	CancelledCount       int64 `json:"cancelled_count"`
	SpecialApprovedCount int64 `json:"special_approved_count"`
	TodayCount           int64 `json:"today_count"`
}

func (q *Queries) AbsenceStatsGet(ctx context.Context) (AbsenceStats, error) {
	var stats AbsenceStats
	err := q.db.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'pending'),
		       count(*) FILTER (WHERE status = 'reviewed'),
		       count(*) FILTER (WHERE status = 'actioned'),
		       count(*) FILTER (WHERE status = 'cancelled'),
		       count(*) FILTER (WHERE status = 'special_approved'),
		       count(*) FILTER (WHERE created_at::date = CURRENT_DATE)
		FROM student_absences
		-- Status values must match absences.Status* constants
	`).Scan(&stats.TotalCount, &stats.PendingCount, &stats.ReviewedCount, &stats.ActionedCount, &stats.CancelledCount, &stats.SpecialApprovedCount, &stats.TodayCount)
	return stats, err
}

func (q *Queries) AbsenceStatsForRange(ctx context.Context, from, to time.Time) (AbsenceStats, error) {
	var stats AbsenceStats
	err := q.db.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'pending'),
		       count(*) FILTER (WHERE status = 'reviewed'),
		       count(*) FILTER (WHERE status = 'actioned'),
		       count(*) FILTER (WHERE status = 'cancelled'),
		       count(*) FILTER (WHERE status = 'special_approved'),
		       count(*) FILTER (WHERE created_at::date = CURRENT_DATE)
		FROM student_absences
		WHERE created_at >= $1 AND created_at < $2
		-- Status values must match absences.Status* constants
	`, from, to).Scan(&stats.TotalCount, &stats.PendingCount, &stats.ReviewedCount, &stats.ActionedCount, &stats.CancelledCount, &stats.SpecialApprovedCount, &stats.TodayCount)
	return stats, err
}

type AbsenceChartRow struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

func (q *Queries) AbsenceDashboardBreakdowns(ctx context.Context, from, to time.Time) ([]AbsenceChartRow, []AbsenceChartRow, error) {
	subjectRows, err := q.db.Query(ctx, `
		SELECT COALESCE(sub.code, 'Unassigned'), count(*)
		FROM student_absences sa
		LEFT JOIN subjects sub ON sub.id = sa.subject_id
		WHERE sa.created_at >= $1 AND sa.created_at < $2
		GROUP BY COALESCE(sub.code, 'Unassigned')
		ORDER BY count(*) DESC, COALESCE(sub.code, 'Unassigned')
	`, from, to)
	if err != nil {
		return nil, nil, err
	}
	defer subjectRows.Close()
	var subjects []AbsenceChartRow
	for subjectRows.Next() {
		var row AbsenceChartRow
		if err := subjectRows.Scan(&row.Label, &row.Count); err != nil {
			return nil, nil, err
		}
		subjects = append(subjects, row)
	}
	if err := subjectRows.Err(); err != nil {
		return nil, nil, err
	}

	reasonRows, err := q.db.Query(ctx, `
		SELECT COALESCE(NULLIF(reason_category, ''), 'Other'), count(*)
		FROM student_absences
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY COALESCE(NULLIF(reason_category, ''), 'Other')
		ORDER BY count(*) DESC, COALESCE(NULLIF(reason_category, ''), 'Other')
	`, from, to)
	if err != nil {
		return nil, nil, err
	}
	defer reasonRows.Close()
	var reasons []AbsenceChartRow
	for reasonRows.Next() {
		var row AbsenceChartRow
		if err := reasonRows.Scan(&row.Label, &row.Count); err != nil {
			return nil, nil, err
		}
		reasons = append(reasons, row)
	}
	return subjects, reasons, reasonRows.Err()
}

type AbsenceDayInRangeRow struct {
	ID               pgtype.UUID
	Wcode            string
	StudentName      pgtype.Text
	Status           string
	SubjectCode      pgtype.Text
	SubjectName      pgtype.Text
	DateFrom         pgtype.Date
	DateTo           pgtype.Date
	SitInMethod      pgtype.Text
	SitInCourseCode  pgtype.Text
	SitInCourseName  pgtype.Text
	SitInSubjectName pgtype.Text
}

// AbsenceDaysInRange returns absences whose [date_from, date_to] overlap the
// requested YYYY-MM-DD range. The bounds are passed as date strings so the
// ::date casts never depend on the database session timezone.
func (q *Queries) AbsenceDaysInRange(ctx context.Context, startKey, endKey string) ([]AbsenceDayInRangeRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT
		  sa.id,
		  sa.wcode,
		  COALESCE(sa.student_name, st.full_name),
		  sa.status,
		  sub.code,
		  sub.name,
		  sa.date_from,
		  sa.date_to,
		  sa.sit_in_method,
		  sc.code,
		  sc.name,
		  COALESCE(sit_sub.name, sit_sc_sub.name)
		FROM student_absences sa
		LEFT JOIN students st ON LOWER(st.wcode) = LOWER(sa.wcode)
		LEFT JOIN subjects sub ON sub.id = sa.subject_id
		LEFT JOIN courses sc ON sc.id = sa.sit_in_course_id
		LEFT JOIN subjects sit_sc_sub ON sit_sc_sub.id = sc.subject_id
		LEFT JOIN (
		  SELECT asi.absence_id, string_agg(DISTINCT sit_sub_inner.name, ', ' ORDER BY sit_sub_inner.name) AS name
		  FROM absence_sit_ins asi
		  JOIN sessions s ON s.id = asi.session_id
		  JOIN courses sit_courses ON sit_courses.id = s.course_id
		  JOIN subjects sit_sub_inner ON sit_sub_inner.id = sit_courses.subject_id
		  GROUP BY asi.absence_id
		) sit_sub ON sit_sub.absence_id = sa.id
		WHERE sa.date_from <= $2::date
		  AND sa.date_to >= $1::date
		ORDER BY sa.date_from ASC, sa.id ASC
	`, startKey, endKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AbsenceDayInRangeRow
	for rows.Next() {
		var item AbsenceDayInRangeRow
		if err := rows.Scan(&item.ID, &item.Wcode, &item.StudentName, &item.Status, &item.SubjectCode, &item.SubjectName, &item.DateFrom, &item.DateTo, &item.SitInMethod, &item.SitInCourseCode, &item.SitInCourseName, &item.SitInSubjectName); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func IsNoRows(err error) bool {
	return err == pgx.ErrNoRows
}

type BatchStatusResult struct {
	ID      pgtype.UUID
	Success bool
	Error   string
}

func (q *Queries) AbsenceBatchStatusUpdate(ctx context.Context, ids []pgtype.UUID, status string, actorID pgtype.UUID, expectedVersions map[[16]byte]int32, reason string) []BatchStatusResult {
	results := make([]BatchStatusResult, 0, len(ids))
	for _, id := range ids {
		ver, ok := expectedVersions[id.Bytes]
		if !ok {
			results = append(results, BatchStatusResult{ID: id, Success: false, Error: "missing expected_version"})
			continue
		}
		tag, err := q.db.Exec(ctx, `
			UPDATE student_absences
			SET status = $2,
			    reviewed_by = CASE WHEN $2 IN ('reviewed', 'actioned') THEN $3 ELSE reviewed_by END,
			    reviewed_at = CASE WHEN $2 IN ('reviewed', 'actioned') AND reviewed_at IS NULL THEN now() ELSE reviewed_at END,
			    updated_at = now(),
			    version = version + 1
			WHERE id = $1 AND version = $4
		`, id, status, actorID, ver)
		if err != nil {
			results = append(results, BatchStatusResult{ID: id, Success: false, Error: err.Error()})
		} else if tag.RowsAffected() == 0 {
			results = append(results, BatchStatusResult{ID: id, Success: false, Error: "stale_edit or not found"})
		} else {
			results = append(results, BatchStatusResult{ID: id, Success: true})
		}
	}
	return results
}

// ---------------------------------------------------------------------------
// Sit-in assignment domain service queries
// ---------------------------------------------------------------------------

// SitInAssignmentRow holds the columns returned by sit-in assignment queries.
type SitInAssignmentRow struct {
	ID                          pgtype.UUID
	AbsenceID                   pgtype.UUID
	SessionID                   pgtype.UUID
	CreatedAt                   pgtype.Timestamptz
	SessionVersionAtAssignment  pgtype.Int4
	AssignedAt                  pgtype.Timestamptz
	AssignedBy                  pgtype.UUID
	AssignmentSource            string
	SessionSnapshotAtAssignment []byte
	SnapshotSchemaVersion       pgtype.Int2
	SnapshotCapturedAt          pgtype.Timestamptz
	SnapshotQuality             string
	SnapshotSource              pgtype.Text
}

// SitInAssignmentGetAllByAbsence returns all sit-in assignments for an absence,
// ordered by created_at. Use FOR UPDATE via the caller's transaction to lock rows.
func (q *Queries) SitInAssignmentGetAllByAbsence(ctx context.Context, absenceID pgtype.UUID) ([]SitInAssignmentRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, absence_id, session_id, created_at,
		       session_version_at_assignment, assigned_at, assigned_by, assignment_source,
		       session_snapshot_at_assignment, snapshot_schema_version,
		       snapshot_captured_at, snapshot_quality, snapshot_source
		FROM absence_sit_ins
		WHERE absence_id = $1
		ORDER BY created_at ASC
	`, absenceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SitInAssignmentRow
	for rows.Next() {
		var r SitInAssignmentRow
		if err := rows.Scan(
			&r.ID, &r.AbsenceID, &r.SessionID, &r.CreatedAt,
			&r.SessionVersionAtAssignment, &r.AssignedAt, &r.AssignedBy, &r.AssignmentSource,
			&r.SessionSnapshotAtAssignment, &r.SnapshotSchemaVersion,
			&r.SnapshotCapturedAt, &r.SnapshotQuality, &r.SnapshotSource,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SitInAssignmentLockByAbsence locks all sit-in assignment rows for an absence
// using SELECT ... FOR UPDATE. Must be called within a transaction.
func (q *Queries) SitInAssignmentLockByAbsence(ctx context.Context, absenceID pgtype.UUID) error {
	_, err := q.db.Query(ctx, `
		SELECT id FROM absence_sit_ins WHERE absence_id = $1 FOR UPDATE
	`, absenceID)
	return err
}

// SitInAssignmentGetByAbsenceAndSession returns a single sit-in assignment by
// absence and session. Returns pgx.ErrNoRows if not found.
func (q *Queries) SitInAssignmentGetByAbsenceAndSession(ctx context.Context, absenceID, sessionID pgtype.UUID) (SitInAssignmentRow, error) {
	var r SitInAssignmentRow
	err := q.db.QueryRow(ctx, `
		SELECT id, absence_id, session_id, created_at,
		       session_version_at_assignment, assigned_at, assigned_by, assignment_source,
		       session_snapshot_at_assignment, snapshot_schema_version,
		       snapshot_captured_at, snapshot_quality, snapshot_source
		FROM absence_sit_ins
		WHERE absence_id = $1 AND session_id = $2
	`, absenceID, sessionID).Scan(
		&r.ID, &r.AbsenceID, &r.SessionID, &r.CreatedAt,
		&r.SessionVersionAtAssignment, &r.AssignedAt, &r.AssignedBy, &r.AssignmentSource,
		&r.SessionSnapshotAtAssignment, &r.SnapshotSchemaVersion,
		&r.SnapshotCapturedAt, &r.SnapshotQuality, &r.SnapshotSource,
	)
	return r, err
}

// SitInAssignmentInsertWithSnapshot inserts a sit-in assignment with snapshot
// metadata. The snapshot columns are set atomically with the row insert.
func (q *Queries) SitInAssignmentInsertWithSnapshot(ctx context.Context, arg struct {
	AbsenceID          pgtype.UUID
	SessionID          pgtype.UUID
	SessionVersion     int32
	AssignedBy         pgtype.UUID
	Source             string
	SnapshotJSON       []byte
	SchemaVersion      int16
	SnapshotCapturedAt pgtype.Timestamptz
}) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO absence_sit_ins (
			absence_id, session_id, session_version_at_assignment,
			assigned_at, assigned_by, assignment_source,
			session_snapshot_at_assignment, snapshot_schema_version,
			snapshot_captured_at, snapshot_quality, snapshot_source
		) VALUES ($1, $2, $3, now(), $4, $5, $6, $7, $8, 'exact', 'captured_at_assignment')
		ON CONFLICT DO NOTHING
	`,
		arg.AbsenceID, arg.SessionID, arg.SessionVersion,
		arg.AssignedBy, arg.Source,
		arg.SnapshotJSON, arg.SchemaVersion, arg.SnapshotCapturedAt,
	)
	return err
}

// SitInAssignmentDeleteByAbsence removes all sit-in assignments for an absence.
// Must be called within a transaction that holds the row lock.
func (q *Queries) SitInAssignmentDeleteByAbsence(ctx context.Context, absenceID pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `DELETE FROM absence_sit_ins WHERE absence_id = $1`, absenceID)
	return err
}

// SitInAssignmentEventInsert inserts a sit-in assignment audit event.
func (q *Queries) SitInAssignmentEventInsert(ctx context.Context, arg struct {
	AbsenceID         pgtype.UUID
	PreviousSessionID pgtype.UUID
	NewSessionID      pgtype.UUID
	Action            string
	Reason            string
	ActorID           pgtype.UUID
}) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO absence_sit_in_assignment_events
			(absence_id, previous_session_id, new_session_id, action, reason, actor_id)
		VALUES ($1, NULLIF($2, '00000000-0000-0000-0000-000000000000'::uuid),
		        NULLIF($3, '00000000-0000-0000-0000-000000000000'::uuid),
		        $4, $5, NULLIF($6, '00000000-0000-0000-0000-000000000000'::uuid))
	`, arg.AbsenceID, arg.PreviousSessionID, arg.NewSessionID, arg.Action, arg.Reason, arg.ActorID)
	return err
}

// AbsenceScheduleIssueUpdateIssueVersion bumps the issue_version for an
// absence_schedule_issues row. Returns the new version; returns 0 and pgx.ErrNoRows
// if the expected version doesn't match.
func (q *Queries) AbsenceScheduleIssueUpdateIssueVersion(ctx context.Context, issueID pgtype.UUID, expectedVersion int32) (int32, error) {
	var newVersion int32
	err := q.db.QueryRow(ctx, `
		UPDATE absence_schedule_issues
		SET issue_version = issue_version + 1, updated_at = now()
		WHERE id = $1 AND issue_version = $2
		RETURNING issue_version
	`, issueID, expectedVersion).Scan(&newVersion)
	return newVersion, err
}

// AbsenceScheduleIssueGetOpenByAbsence returns the first open or needs_review
// issue for an absence, or pgx.ErrNoRows if none.
func (q *Queries) AbsenceScheduleIssueGetOpenByAbsence(ctx context.Context, absenceID pgtype.UUID) (AbsenceScheduleIssueGetRow, error) {
	var i AbsenceScheduleIssueGetRow
	err := q.db.QueryRow(ctx, `
		SELECT i.id, i.absence_id, i.issue_type, i.severity, i.status,
		       i.source_session_id, i.sit_in_session_id, i.missed_session_id,
		       i.first_session_change_id, i.latest_session_change_id,
		       i.details_json, i.suggested_resolution_json, i.detected_at,
		       i.updated_at, i.resolved_at, i.resolved_by, i.resolution_action,
		       i.fingerprint, sa.wcode, sa.student_name, sa.student_email,
		       sa.student_phone
		FROM absence_schedule_issues i
		JOIN student_absences sa ON sa.id = i.absence_id
		WHERE i.absence_id = $1 AND i.status IN ('open', 'needs_review')
		ORDER BY i.detected_at DESC
		LIMIT 1
	`, absenceID).Scan(
		&i.ID, &i.AbsenceID, &i.IssueType, &i.Severity, &i.Status,
		&i.SourceSessionID, &i.SitInSessionID, &i.MissedSessionID,
		&i.FirstSessionChangeID, &i.LatestSessionChangeID,
		&i.DetailsJson, &i.SuggestedResolutionJson, &i.DetectedAt,
		&i.UpdatedAt, &i.ResolvedAt, &i.ResolvedBy, &i.ResolutionAction,
		&i.Fingerprint, &i.Wcode, &i.StudentName, &i.StudentEmail,
		&i.StudentPhone,
	)
	return i, err
}

// AbsenceScheduleIssueGetWithVersion returns an issue with its issue_version
// for optimistic concurrency checks.
type AbsenceScheduleIssueVersionRow struct {
	ID            pgtype.UUID
	AbsenceID     pgtype.UUID
	Status        string
	IssueVersion  int32
	Fingerprint   string
}

func (q *Queries) AbsenceScheduleIssueGetWithVersion(ctx context.Context, issueID pgtype.UUID) (AbsenceScheduleIssueVersionRow, error) {
	var r AbsenceScheduleIssueVersionRow
	err := q.db.QueryRow(ctx, `
		SELECT id, absence_id, status, issue_version, fingerprint
		FROM absence_schedule_issues
		WHERE id = $1
	`, issueID).Scan(&r.ID, &r.AbsenceID, &r.Status, &r.IssueVersion, &r.Fingerprint)
	return r, err
}

// OutboxInsertAbsenceEvent inserts an outbox event for an absence change.
func (q *Queries) OutboxInsertAbsenceEvent(ctx context.Context, arg OutboxEventInsertParams) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO outbox_events (event_type, aggregate_id, aggregate_version, payload)
		VALUES ($1, $2, $3, $4::text::jsonb)
		ON CONFLICT (event_type, aggregate_id, aggregate_version) DO NOTHING
	`, arg.EventType, arg.AggregateID, arg.AggregateVersion, arg.Payload)
	return err
}

// AbsenceVersionGet returns the current version of an absence row.
func (q *Queries) AbsenceVersionGet(ctx context.Context, absenceID pgtype.UUID) (int32, error) {
	var version int32
	err := q.db.QueryRow(ctx, `SELECT version FROM student_absences WHERE id = $1`, absenceID).Scan(&version)
	return version, err
}
