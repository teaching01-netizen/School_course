package reconcile

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync/normalize"
	"warwick-institute/internal/schedulelock"
	"warwick-institute/internal/schedulepolicy"
)

// wcodeRe validates roster wcodes: the old site's "W" prefix followed by
// digits, e.g. "W250025".
var wcodeRe = regexp.MustCompile(`^W\d+$`)

// applyRoster imports one course's attendee roster, add-only: students are
// created when their wcode is unknown (CRM-created students are never
// modified), enrollments are inserted when absent, student_count mirrors
// the roster size, and newly created students get a legacy external ref.
// Runs outside the link transaction so roster failures never leave a course
// half-linked.
func (r *FullReconciler) applyRoster(ctx context.Context, course normalize.LegacyCourse, courseID pgtype.UUID, stats *FullReconcileStats) error {
	if len(course.Attendees) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin roster transaction: %w", err)
	}
	defer tx.Rollback(context.Background()) // no-op on committed tx
	qtx := r.q.WithTx(tx)
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{CourseIDs: []pgtype.UUID{courseID}}); err != nil {
		return fmt.Errorf("lock roster course: %w", err)
	}
	policy, err := schedulepolicy.NewDBReader().Load(ctx, tx)
	if err != nil {
		return fmt.Errorf("load roster conflict policy: %w", err)
	}
	allowConflicts := !policy.Enforced(schedulepolicy.ScopeLegacySync)
	for _, entry := range course.Attendees {
		wcode, name, nickname, err := splitRosterEntry(entry)
		if err != nil {
			return err
		}
		var nick pgtype.Text
		if nickname != "" {
			nick = pgtype.Text{String: nickname, Valid: true}
		}
		student, err := qtx.StudentImportByWCode(ctx, sqldb.StudentImportByWCodeParams{Wcode: wcode, FullName: name, Nickname: nick})
		var studentID pgtype.UUID
		switch {
		case err == nil:
			stats.RosterStudents++
			studentID = student.ID
			if _, err := tx.Exec(ctx, `
				INSERT INTO external_refs (source, entity_type, external_id, internal_id, state)
				VALUES ($1, 'student', $2, $3, 'active')
				ON CONFLICT (source, entity_type, external_id) DO NOTHING
			`, r.source, wcode, student.ID); err != nil {
				return fmt.Errorf("record student external ref %s: %w", wcode, err)
			}
		case errors.Is(err, pgx.ErrNoRows):
			// Student already exists (typically CRM-created, possibly with a
			// different wcode case); resolve case-insensitively and reuse it.
			if err := tx.QueryRow(ctx, `SELECT id FROM students WHERE lower(wcode) = lower($1)`, wcode).Scan(&studentID); err != nil {
				return fmt.Errorf("resolve roster student %s: %w", wcode, err)
			}
		default:
			return fmt.Errorf("upsert roster student %s: %w", wcode, err)
		}
		if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
			CourseIDs:  []pgtype.UUID{courseID},
			StudentIDs: []pgtype.UUID{studentID},
		}); err != nil {
			return fmt.Errorf("lock roster student %s: %w", wcode, err)
		}
		var alreadyEnrolled bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM course_students WHERE course_id=$1 AND student_id=$2)`, courseID, studentID).Scan(&alreadyEnrolled); err != nil {
			return fmt.Errorf("check roster student %s: %w", wcode, err)
		}
		if alreadyEnrolled {
			continue
		}
		conflict, err := legacyRosterConflict(ctx, tx, courseID, studentID)
		if err != nil {
			return fmt.Errorf("check roster conflict %s: %w", wcode, err)
		}
		if conflict && !allowConflicts {
			return fmt.Errorf("legacy roster student %s: %w", wcode, &pgconn.PgError{Code: "23P01", ConstraintName: "student_busy_ranges_no_overlap", Message: "student schedule overlap"})
		}
		if conflict {
			if _, err := tx.Exec(ctx, `UPDATE sessions SET legacy_conflict_override=true, updated_at=now() WHERE course_id=$1 AND deleted_at IS NULL`, courseID); err != nil {
				return fmt.Errorf("mark allowed roster conflict %s: %w", wcode, err)
			}
			if _, err := tx.Exec(ctx, `UPDATE student_busy_ranges SET conflict_override=true WHERE session_id IN (SELECT id FROM sessions WHERE course_id=$1 AND deleted_at IS NULL)`, courseID); err != nil {
				return fmt.Errorf("mark roster busy ranges %s: %w", wcode, err)
			}
			stats.ConflictWarnings++
		}
		res, err := tx.Exec(ctx, `
			INSERT INTO course_students (course_id, student_id, status)
			VALUES ($1, $2, 'enrolled')
			ON CONFLICT (course_id, student_id) DO NOTHING
		`, courseID, studentID)
		if err != nil {
			return fmt.Errorf("enroll roster student %s: %w", wcode, err)
		}
		if res.RowsAffected() == 1 {
			stats.RosterEnrollments++
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE courses SET student_count = $1 WHERE id = $2`, len(course.Attendees), courseID); err != nil {
		return fmt.Errorf("mirror roster count: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit roster: %w", err)
	}
	return nil
}

func legacyRosterConflict(ctx context.Context, tx pgx.Tx, courseID, studentID pgtype.UUID) (bool, error) {
	var conflict bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM sessions target
			JOIN student_busy_ranges busy ON busy.student_id = $2
			WHERE target.course_id = $1
			  AND target.deleted_at IS NULL
			  AND busy.deleted_at IS NULL
			  AND target.time_range && busy.time_range
		) OR EXISTS (
			SELECT 1
			FROM sessions first_session
			JOIN sessions second_session ON first_session.id < second_session.id
			WHERE first_session.course_id = $1
			  AND second_session.course_id = $1
			  AND first_session.deleted_at IS NULL
			  AND second_session.deleted_at IS NULL
			  AND first_session.time_range && second_session.time_range
		)`, courseID, studentID).Scan(&conflict)
	return conflict, err
}

// splitRosterEntry splits one parsed attendee ("W250025 Nutnicha (Nicha)")
// into the wcode, the full name, and the parenthesized nickname ("" when
// absent). The parser guarantees the shape; anything else here is a
// parser/normalize regression and fails the reconcile loudly.
func splitRosterEntry(entry string) (string, string, string, error) {
	idx := strings.IndexByte(entry, ' ')
	if idx <= 0 {
		return "", "", "", fmt.Errorf("malformed roster entry %q", entry)
	}
	wcode := entry[:idx]
	name := strings.TrimSpace(entry[idx+1:])
	if !wcodeRe.MatchString(wcode) {
		return "", "", "", fmt.Errorf("malformed roster wcode %q", wcode)
	}
	nickname := ""
	const closingParen = ')'
	if i := strings.LastIndexByte(name, closingParen); i == len(name)-1 {
		if j := strings.LastIndexByte(name[:i], '('); j > 0 && !strings.ContainsRune(name[j+1:i], '(') {
			pick := strings.TrimSpace(name[j+1 : i])
			if pick != "" && !strings.HasPrefix(pick, "-") {
				nickname = pick
				name = strings.TrimSpace(name[:j])
			}
		}
	}
	return wcode, name, nickname, nil
}
