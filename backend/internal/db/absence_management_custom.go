package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AbsenceFilter struct {
	Query     string
	IDs       []pgtype.UUID
	SubjectID pgtype.UUID
	Status    string
	DateFrom  pgtype.Date
	DateTo    pgtype.Date
	Limit     int32
	Offset    int32
}

type ManagedAbsenceRow struct {
	ID                  pgtype.UUID
	Wcode               string
	StudentName         pgtype.Text
	StudentEmail        pgtype.Text
	StudentNickname     pgtype.Text
	StudentPhone        pgtype.Text
	ParentPhone         pgtype.Text
	CourseID            pgtype.UUID
	CourseCode          string
	CourseName          string
	SubjectID           pgtype.UUID
	SubjectCode         pgtype.Text
	SubjectName         pgtype.Text
	DateFrom            pgtype.Date
	DateTo              pgtype.Date
	ReasonCategory      pgtype.Text
	Reason              pgtype.Text
	SitInMethod         pgtype.Text
	SitInCourseID       pgtype.UUID
	SitInCourseCode     pgtype.Text
	SitInCourseName     pgtype.Text
	SitInSubjectName    pgtype.Text
	Status              string
	AdminNotes          pgtype.Text
	ReviewedBy          pgtype.UUID
	ReviewedAt          pgtype.Timestamptz
	SitInOverridden     bool
	SitInOverriddenBy   pgtype.UUID
	SitInOverrideReason pgtype.Text
	Version             int32
	CreatedAt           pgtype.Timestamptz
	UpdatedAt           pgtype.Timestamptz
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
	return p
}

const absenceStudentNicknameExprPlaceholder = "__STUDENT_NICKNAME_EXPR__"

const managedAbsenceListQueryTemplate = `
		SELECT sa.id, sa.wcode, COALESCE(sa.student_name, st.full_name),
		       COALESCE(sa.student_email, st.email), __STUDENT_NICKNAME_EXPR__,
		       sa.student_phone,
		       st.parent_phone,
		       sa.course_id, c.code, c.name, sa.subject_id, sub.code, sub.name,
		       sa.date_from, sa.date_to, sa.reason_category, sa.reason, sa.sit_in_method,
		       sa.sit_in_course_id, sc.code, sc.name, sit_sub.name, sa.status, sa.admin_notes,
		       sa.reviewed_by, sa.reviewed_at, sa.sit_in_overridden, sa.sit_in_overridden_by,
		       sa.sit_in_override_reason, sa.version, sa.created_at, sa.updated_at,
		       count(*) OVER()
		FROM student_absences sa
		JOIN courses c ON c.id = sa.course_id
		LEFT JOIN students st ON st.wcode = sa.wcode
		LEFT JOIN subjects sub ON sub.id = sa.subject_id
		LEFT JOIN courses sc ON sc.id = sa.sit_in_course_id
		LEFT JOIN subjects sit_sub ON sit_sub.id = sc.subject_id
		WHERE ($1 = '' OR sa.wcode ILIKE '%' || $1 || '%' OR COALESCE(sa.student_name, st.full_name, '') ILIKE '%' || $1 || '%')
		  AND ($2::uuid IS NULL OR sa.subject_id = $2)
		  AND ($3 = '' OR sa.status = $3)
		  AND ($4::date IS NULL OR sa.date_to >= $4)
		  AND ($5::date IS NULL OR sa.date_from <= $5)
		  AND (cardinality($8::uuid[]) = 0 OR sa.id = ANY($8::uuid[]))
		ORDER BY sa.created_at DESC, sa.id DESC
		LIMIT $6 OFFSET $7
`

const managedAbsenceGetQueryTemplate = `
		SELECT sa.id, sa.wcode, COALESCE(sa.student_name, st.full_name),
		       COALESCE(sa.student_email, st.email), __STUDENT_NICKNAME_EXPR__,
		       sa.student_phone,
		       st.parent_phone,
		       sa.course_id, c.code, c.name, sa.subject_id, sub.code, sub.name,
		       sa.date_from, sa.date_to, sa.reason_category, sa.reason, sa.sit_in_method,
		       sa.sit_in_course_id, sc.code, sc.name, sit_sub.name, sa.status, sa.admin_notes,
		       sa.reviewed_by, sa.reviewed_at, sa.sit_in_overridden, sa.sit_in_overridden_by,
		       sa.sit_in_override_reason, sa.version, sa.created_at, sa.updated_at
		FROM student_absences sa
		JOIN courses c ON c.id = sa.course_id
		LEFT JOIN students st ON st.wcode = sa.wcode
		LEFT JOIN subjects sub ON sub.id = sa.subject_id
		LEFT JOIN courses sc ON sc.id = sa.sit_in_course_id
		LEFT JOIN subjects sit_sub ON sit_sub.id = sc.subject_id
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
	rows, err := q.db.Query(ctx, managedAbsenceQuerySQL(managedAbsenceListQueryTemplate, hasStudentNicknameColumn), p.Query, p.SubjectID, p.Status, p.DateFrom, p.DateTo, p.Limit, p.Offset, p.IDs)
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
	AbsenceID  pgtype.UUID
	ID         pgtype.UUID
	SessionID  pgtype.UUID
	CourseID   pgtype.UUID
	CourseCode string
	CourseName string
	RoomName   pgtype.Text
	StartAt    pgtype.Timestamptz
	EndAt      pgtype.Timestamptz
}

func (q *Queries) ManagedAbsenceMissedSessions(ctx context.Context, absenceID pgtype.UUID) ([]ManagedAbsenceSession, error) {
	rows, err := q.db.Query(ctx, `
		SELECT ams.absence_id, ams.id, sess.id, sess.course_id, c.code, c.name, room.name, sess.start_at, sess.end_at
		FROM absence_missed_sessions ams
		JOIN sessions sess ON sess.id = ams.session_id AND sess.deleted_at IS NULL
		JOIN courses c ON c.id = sess.course_id
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
		if err := rows.Scan(&session.AbsenceID, &session.ID, &session.SessionID, &session.CourseID, &session.CourseCode, &session.CourseName, &session.RoomName, &session.StartAt, &session.EndAt); err != nil {
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
		SELECT ams.absence_id, ams.id, sess.id, sess.course_id, c.code, c.name, room.name, sess.start_at, sess.end_at
		FROM absence_missed_sessions ams
		JOIN sessions sess ON sess.id = ams.session_id AND sess.deleted_at IS NULL
		JOIN courses c ON c.id = sess.course_id
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
		if err := rows.Scan(&session.AbsenceID, &session.ID, &session.SessionID, &session.CourseID, &session.CourseCode, &session.CourseName, &session.RoomName, &session.StartAt, &session.EndAt); err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func (q *Queries) ManagedAbsenceSessions(ctx context.Context, absenceID pgtype.UUID) ([]ManagedAbsenceSession, error) {
	rows, err := q.db.Query(ctx, `
		SELECT asi.absence_id, asi.id, sess.id, sess.course_id, c.code, c.name, room.name, sess.start_at, sess.end_at
		FROM absence_sit_ins asi
		JOIN sessions sess ON sess.id = asi.session_id AND sess.deleted_at IS NULL
		JOIN courses c ON c.id = sess.course_id
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
		if err := rows.Scan(&session.AbsenceID, &session.ID, &session.SessionID, &session.CourseID, &session.CourseCode, &session.CourseName, &session.RoomName, &session.StartAt, &session.EndAt); err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

type SitInStudentRow struct {
	AbsenceID      pgtype.UUID
	SessionID      pgtype.UUID
	Wcode          string
	Nickname       pgtype.Text
	StudentName    pgtype.Text
	FromCourseCode string
	FromCourseName pgtype.Text
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
		       COALESCE(c.name, '') AS from_course_name
		FROM absence_sit_ins asi
		JOIN student_absences sa ON sa.id = asi.absence_id
		LEFT JOIN students st ON st.wcode = sa.wcode
		LEFT JOIN courses c ON c.id = sa.sit_in_course_id
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
		if err := rows.Scan(&r.AbsenceID, &r.SessionID, &r.Wcode, &r.Nickname, &r.StudentName, &r.FromCourseCode, &r.FromCourseName); err != nil {
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
		WHERE id = $1 AND version = $2
		RETURNING 1
	`, id, expectedVersion).Scan(&one)
	return one, err
}

func (q *Queries) AbsenceSitInsReplace(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID) error {
	type beginner interface {
		Begin(context.Context) (pgx.Tx, error)
	}
	db, ok := q.db.(beginner)
	if !ok {
		return fmt.Errorf("database does not support transactions")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM absence_sit_ins WHERE absence_id = $1`, absenceID); err != nil {
		return fmt.Errorf("delete sit-ins: %w", err)
	}

	for _, sid := range sessionIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO absence_sit_ins (absence_id, session_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, absenceID, sid); err != nil {
			return fmt.Errorf("insert sit-in: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (q *Queries) AbsenceMissedSessionsCreate(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID) error {
	for _, sid := range sessionIDs {
		if _, err := q.db.Exec(ctx, `
			INSERT INTO absence_missed_sessions (absence_id, session_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, absenceID, sid); err != nil {
			return fmt.Errorf("insert missed session: %w", err)
		}
	}
	return nil
}

func (q *Queries) ValidMissedSessionCount(ctx context.Context, absenceID pgtype.UUID, sessionIDs []pgtype.UUID) (int, error) {
	var count int
	err := q.db.QueryRow(ctx, `
		SELECT count(*)
		FROM sessions sess
		JOIN student_absences sa ON sa.id = $1
		WHERE sess.id = ANY($2::uuid[])
		  AND sess.course_id = sa.course_id
		  AND sess.deleted_at IS NULL
		  AND sess.start_at::date BETWEEN sa.date_from AND sa.date_to
	`, absenceID, sessionIDs).Scan(&count)
	return count, err
}

func (q *Queries) ValidSitInSessionCount(ctx context.Context, absenceID, courseID pgtype.UUID, sessionIDs []pgtype.UUID) (int, error) {
	var count int
	err := q.db.QueryRow(ctx, `
		SELECT count(*)
		FROM sessions sess
		JOIN student_absences sa ON sa.id = $1
		WHERE sess.id = ANY($3::uuid[])
		  AND sess.course_id = $2
		  AND sess.deleted_at IS NULL
		  AND NOT EXISTS (
		    SELECT 1
		    FROM sessions missed
		    WHERE missed.course_id = sa.course_id
		      AND missed.deleted_at IS NULL
		      AND missed.start_at::date BETWEEN sa.date_from AND sa.date_to
		      AND sess.start_at < missed.end_at
		      AND sess.end_at > missed.start_at
		  )
	`, absenceID, courseID, sessionIDs).Scan(&count)
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

func (q *Queries) SitInCandidateSessions(ctx context.Context, absenceID, courseID pgtype.UUID) ([]SitInCandidateSession, error) {
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
		  AND NOT EXISTS (
		    SELECT 1
		    FROM sessions missed
		    WHERE missed.course_id = sa.course_id
		      AND missed.deleted_at IS NULL
		      AND missed.start_at::date BETWEEN sa.date_from AND sa.date_to
		      AND sess.start_at < missed.end_at
		      AND sess.end_at > missed.start_at
		  )
		ORDER BY sess.start_at ASC
	`, absenceID, courseID)
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
	TotalCount     int64 `json:"total_count"`
	PendingCount   int64 `json:"pending_count"`
	ReviewedCount  int64 `json:"reviewed_count"`
	ActionedCount  int64 `json:"actioned_count"`
	CancelledCount int64 `json:"cancelled_count"`
	TodayCount     int64 `json:"today_count"`
}

func (q *Queries) AbsenceStatsGet(ctx context.Context) (AbsenceStats, error) {
	var stats AbsenceStats
	err := q.db.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'pending'),
		       count(*) FILTER (WHERE status = 'reviewed'),
		       count(*) FILTER (WHERE status = 'actioned'),
		       count(*) FILTER (WHERE status = 'cancelled'),
		       count(*) FILTER (WHERE created_at::date = CURRENT_DATE)
		FROM student_absences
	`).Scan(&stats.TotalCount, &stats.PendingCount, &stats.ReviewedCount, &stats.ActionedCount, &stats.CancelledCount, &stats.TodayCount)
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
		       count(*) FILTER (WHERE created_at::date = CURRENT_DATE)
		FROM student_absences
		WHERE created_at >= $1 AND created_at < $2
	`, from, to).Scan(&stats.TotalCount, &stats.PendingCount, &stats.ReviewedCount, &stats.ActionedCount, &stats.CancelledCount, &stats.TodayCount)
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

func (q *Queries) AbsenceDaysInRange(ctx context.Context, rangeStart, rangeEnd time.Time) ([]AbsenceDayInRangeRow, error) {
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
		  sit_sub.name
		FROM student_absences sa
		LEFT JOIN students st ON st.wcode = sa.wcode
		LEFT JOIN subjects sub ON sub.id = sa.subject_id
		LEFT JOIN courses sc ON sc.id = sa.sit_in_course_id
		LEFT JOIN subjects sit_sub ON sit_sub.id = sc.subject_id
		WHERE sa.date_from <= $2::date
		  AND sa.date_to >= $1::date
		ORDER BY sa.date_from ASC, sa.id ASC
	`, rangeStart, rangeEnd)
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
