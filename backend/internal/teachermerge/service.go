// Package teachermerge collapses a duplicate teacher identity (typically a
// legacy-sync shell created by legacysync/apply when no mapping existed) into
// the real account the teacher logs in with.
//
// The durable "same teacher" record is external_refs: after the merge, every
// future legacy sync resolves the old-site teacher to the canonical account,
// and all domain rows (sessions, series, courses, course_teachers,
// availability) carry the canonical user id so teacher-scoped views need no
// aliasing anywhere.
package teachermerge

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
)

// legacyUsernamePrefix is set only by legacysync/apply's INSERT, which makes
// it a reliable discriminator between sync-owned shells and native accounts.
const legacyUsernamePrefix = "legacy:"

type Service struct {
	pool *pgxpool.Pool
	q    *sqldb.Queries
}

func New(pool *pgxpool.Pool, q *sqldb.Queries) *Service {
	return &Service{pool: pool, q: q}
}

type Account struct {
	ID        uuid.UUID
	Username  string
	FullName  string
	Email     string
	Role      string
	Deleted   bool
	CreatedAt time.Time
	IsLegacy  bool
}

type Impact struct {
	SessionsLive        int64
	SessionsDeleted     int64
	Courses             int64
	Series              int64
	CourseTeacherRows   int64
	AvailabilityBlocks  int64
	ExternalRefMappings int64
	ConflictSessions    int64
}

type Preview struct {
	Duplicate Account
	Canonical Account
	Impact    Impact
}

type Result struct {
	Impact    Impact
	Canonical Account
}

func (a Account) legacy() bool {
	return strings.HasPrefix(a.Username, legacyUsernamePrefix)
}

// validate enforces the merge invariants shared by Preview and Merge.
func validate(duplicate, canonical Account) error {
	if duplicate.ID == canonical.ID {
		return ErrSameAccount
	}
	if duplicate.Role != "Teacher" || canonical.Role != "Teacher" {
		return ErrNotTeacher
	}
	if canonical.Deleted {
		return ErrCanonicalInactive
	}
	if canonical.legacy() {
		return ErrCanonicalLegacy
	}
	if duplicate.Deleted {
		return ErrAlreadyMerged
	}
	return nil
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func loadAccounts(ctx context.Context, q queryer, forUpdate bool, duplicateID, canonicalID uuid.UUID) (Account, Account, error) {
	sql := `SELECT id, username, full_name, email, role, deleted_at, created_at FROM users WHERE id IN ($1, $2)`
	if forUpdate {
		sql += ` ORDER BY id FOR UPDATE`
	}
	rows, err := q.Query(ctx, sql, duplicateID, canonicalID)
	if err != nil {
		return Account{}, Account{}, err
	}
	defer rows.Close()
	byID := make(map[uuid.UUID]Account, 2)
	for rows.Next() {
		var id uuid.UUID
		var username, role string
		var fullName, email pgtype.Text
		var deletedAt, createdAt pgtype.Timestamptz
		if err := rows.Scan(&id, &username, &fullName, &email, &role, &deletedAt, &createdAt); err != nil {
			return Account{}, Account{}, err
		}
		byID[id] = Account{ID: id, Username: username, FullName: fullName.String, Email: email.String,
			Role: role, Deleted: deletedAt.Valid, CreatedAt: createdAt.Time, IsLegacy: strings.HasPrefix(username, legacyUsernamePrefix)}
	}
	if len(byID) != 2 {
		return Account{}, Account{}, ErrAccountNotFound
	}
	return byID[duplicateID], byID[canonicalID], nil
}

// conflictPredicate matches, row for row, what the merge would violate if the
// row kept its flags: the sessions_no_teacher_overlap EXCLUDE constraint
// (live overlap with a canonical session) and the enforce_session_availability
// trigger (canonical teacher has windows that do not cover the session).
// Matching rows are exempted via legacy_conflict_override, the same escape
// hatch migration 00103 defined for legacy reality.
const conflictPredicate = `EXISTS (
		SELECT 1 FROM sessions s2
		WHERE s2.teacher_id = $2 AND s2.deleted_at IS NULL AND s2.id <> sessions.id
		  AND s2.time_range && sessions.time_range
	) OR (
		EXISTS (SELECT 1 FROM teacher_availability ta WHERE ta.teacher_id = $2 AND ta.deleted_at IS NULL)
		AND NOT EXISTS (
			SELECT 1 FROM teacher_availability ta
			WHERE ta.teacher_id = $2 AND ta.deleted_at IS NULL AND ta.time_range @> sessions.time_range
		)
	)`

// Preview reports what Merge would do without doing it. It is advisory:
// Merge re-validates atomically under row locks.
func (s *Service) Preview(ctx context.Context, duplicateID, canonicalID uuid.UUID) (Preview, error) {
	duplicate, canonical, err := loadAccounts(ctx, s.pool, false, duplicateID, canonicalID)
	if err != nil {
		return Preview{}, err
	}
	if err := validate(duplicate, canonical); err != nil {
		return Preview{}, err
	}
	impact, err := countImpact(ctx, s.pool, duplicateID, canonicalID)
	if err != nil {
		return Preview{}, err
	}
	return Preview{Duplicate: duplicate, Canonical: canonical, Impact: impact}, nil
}

func countImpact(ctx context.Context, q queryer, duplicateID, canonicalID uuid.UUID) (Impact, error) {
	var impact Impact
	err := q.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM sessions WHERE teacher_id = $1 AND deleted_at IS NULL),
			(SELECT count(*) FROM sessions WHERE teacher_id = $1 AND deleted_at IS NOT NULL),
			(SELECT count(*) FROM courses WHERE teacher_id = $1),
			(SELECT count(*) FROM session_series WHERE teacher_id = $1),
			(SELECT count(*) FROM course_teachers WHERE teacher_id = $1),
			(SELECT count(*) FROM teacher_availability WHERE teacher_id = $1),
			(SELECT count(*) FROM external_refs WHERE entity_type = 'teacher' AND internal_id = $1 AND state IN ('active','restored')),
			(SELECT count(*) FROM sessions WHERE teacher_id = $1 AND deleted_at IS NULL AND NOT legacy_conflict_override AND (`+conflictPredicate+`))
	`, duplicateID, canonicalID).Scan(
		&impact.SessionsLive, &impact.SessionsDeleted, &impact.Courses, &impact.Series,
		&impact.CourseTeacherRows, &impact.AvailabilityBlocks, &impact.ExternalRefMappings,
		&impact.ConflictSessions)
	return impact, err
}

// Merge re-points every teacher reference from the duplicate to the canonical
// account in one transaction and deactivates the duplicate. Constraints stay
// authoritative afterwards: rows that would violate the canonical teacher's
// overlap or availability invariants are flagged legacy_conflict_override,
// exactly like other imported conflicts.
func (s *Service) Merge(ctx context.Context, actor, duplicateID, canonicalID uuid.UUID) (Result, error) {
	if duplicateID == canonicalID {
		return Result{}, ErrSameAccount
	}
	if s.pool == nil || s.q == nil {
		return Result{}, pgx.ErrNoRows
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "teachermerge:"+duplicateID.String()); err != nil {
		return Result{}, err
	}

	duplicate, canonical, err := loadAccounts(ctx, tx, true, duplicateID, canonicalID)
	if err != nil {
		return Result{}, err
	}
	if err := validate(duplicate, canonical); err != nil {
		return Result{}, err
	}

	impact := Impact{}
	if ct, err := tx.Exec(ctx, `
		UPDATE sessions SET legacy_conflict_override = true, updated_at = now()
		WHERE teacher_id = $1 AND deleted_at IS NULL AND NOT legacy_conflict_override
		  AND (`+conflictPredicate+`)`, duplicateID, canonicalID); err != nil {
		return Result{}, err
	} else {
		impact.ConflictSessions = ct.RowsAffected()
	}

	var live, deleted int64
	rows, err := tx.Query(ctx, `
		UPDATE sessions SET teacher_id = $2, version = version + 1, updated_at = now()
		WHERE teacher_id = $1
		RETURNING (deleted_at IS NOT NULL)`, duplicateID, canonicalID)
	if err != nil {
		return Result{}, err
	}
	for rows.Next() {
		var isDeleted bool
		if err := rows.Scan(&isDeleted); err != nil {
			rows.Close()
			return Result{}, err
		}
		if isDeleted {
			deleted++
		} else {
			live++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	impact.SessionsLive, impact.SessionsDeleted = live, deleted

	if impact.Courses, err = execRows(ctx, tx, `
		UPDATE courses SET teacher_id = $2, version = version + 1, updated_at = now()
		WHERE teacher_id = $1`, duplicateID, canonicalID); err != nil {
		return Result{}, err
	}
	if impact.Series, err = execRows(ctx, tx, `
		UPDATE session_series SET teacher_id = $2, version = version + 1, updated_at = now()
		WHERE teacher_id = $1`, duplicateID, canonicalID); err != nil {
		return Result{}, err
	}
	// course_teachers: ux_course_teachers_one_primary forbids two primary
	// rows per course, so the duplicate's primary flags must drop before the
	// canonical rows appear. Primary-ness is re-derived from courses
	// (teacher_id, already re-pointed above) — the same mirror invariant
	// migration 00079 established. Statements are ordered because
	// data-modifying CTEs cannot see each other's effects.
	if _, err := tx.Exec(ctx, `
		UPDATE course_teachers SET is_primary = false
		WHERE teacher_id = $1 AND is_primary`, duplicateID); err != nil {
		return Result{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO course_teachers (course_id, teacher_id, is_primary)
		SELECT id, $1, true FROM courses WHERE teacher_id = $1
		ON CONFLICT (course_id, teacher_id) DO UPDATE SET is_primary = true`, canonicalID); err != nil {
		return Result{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO course_teachers (course_id, teacher_id, is_primary)
		SELECT course_id, $2, false FROM course_teachers WHERE teacher_id = $1
		ON CONFLICT (course_id, teacher_id) DO NOTHING`, duplicateID, canonicalID); err != nil {
		return Result{}, err
	}
	if impact.CourseTeacherRows, err = execRows(ctx, tx, `
		DELETE FROM course_teachers WHERE teacher_id = $1`, duplicateID); err != nil {
		return Result{}, err
	}
	if impact.AvailabilityBlocks, err = execRows(ctx, tx, `
		UPDATE teacher_availability SET teacher_id = $2, updated_at = now()
		WHERE teacher_id = $1`, duplicateID, canonicalID); err != nil {
		return Result{}, err
	}
	if impact.ExternalRefMappings, err = execRows(ctx, tx, `
		UPDATE external_refs SET internal_id = $2, last_applied_at = now()
		WHERE entity_type = 'teacher' AND internal_id = $1 AND state IN ('active','restored')`, duplicateID, canonicalID); err != nil {
		return Result{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`, duplicateID); err != nil {
		return Result{}, err
	}

	qtx := s.q.WithTx(tx)
	if _, err := qtx.AuditInsert(ctx, sqldb.AuditInsertParams{
		ActorUserID: pgtype.UUID{Bytes: actor, Valid: true},
		Action:      "teacher.merge",
		Payload: map[string]any{
			"duplicate_user_id":  duplicateID.String(),
			"canonical_user_id":  canonicalID.String(),
			"duplicate_username": duplicate.Username,
			"canonical_username": canonical.Username,
			"impact":              impact,
		},
	}); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{Impact: impact, Canonical: canonical}, nil
}

func execRows(ctx context.Context, tx pgx.Tx, sql string, args ...any) (int64, error) {
	ct, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}
