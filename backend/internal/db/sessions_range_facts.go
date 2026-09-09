package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SessionsRangeFacts is the consolidated O(1)-round-trip read model backing
// GET /api/v1/absences/sessions-in-range and
// GET /api/v1/absence-self-service/sessions.
//
// Design contract:
//   - Exactly ONE database round trip for session facts, regardless of how
//     many courses the student is enrolled in. Merged display ranges,
//     already-absent flags, absence-day limits, and sit-in candidates are all
//     derived by the caller from batched result sets, never via per-course
//     queries.
//   - Work scales with R, the number of sessions relevant to this
//     student, never total sessions in the database and never R x courses.
//     (Step 18 honesty: per-function linear scans are O(R), but sorting
//     and the scope/counter/bundle inputs add O(R log R) + metadata terms
//     — see the domain-layer header. No endpoint-wide O(R) claim here.)
//   - Every returned row belongs to the lookup student: the enrollment
//     predicate lives inside the SQL, so a facts query for one wcode cannot
//     return another student sessions.
//
// Preserved semantics:
//   - Institute-local days: the caller converts inclusive date_from/date_to
//     into a half-open instant range [FromUTC, ToExclusiveUTC) using the
//     institute timezone. The SQL filters absolute instants only.
//   - Enrollment eligibility: enrolled modes require
//     course_students(status=enrolled) AND student_is_expected_at_session.
//   - Student mode additionally requires absence_form_visible AND direct
//     subject_active_courses membership (single-switch model, no fallback).
//   - Staff mode applies no visibility or active-course predicate.
//   - All-subjects (special sit-in) mode lists by subject with no enrollment
//     predicate.
//   - Soft-deleted sessions are never returned.
type SessionsRangeFactsMode int

const (
	// SessionsRangeFactsStaff lists every enrolled session of the student.
	// Staff-only: no visibility or active-course predicate.
	SessionsRangeFactsStaff SessionsRangeFactsMode = iota
	// SessionsRangeFactsStudent lists only bookable sessions: visible AND
	// directly active courses. The only mode the student endpoint may use.
	SessionsRangeFactsStudent
	// SessionsRangeFactsAllSubjects lists sessions for explicit subject IDs
	// without an enrollment predicate. Staff-only (special sit-in lookup).
	SessionsRangeFactsAllSubjects
)

// SessionsRangeFactsParams binds one lookup. SubjectIDs is required exactly
// when Mode == SessionsRangeFactsAllSubjects.
type SessionsRangeFactsParams struct {
	// Wcode is the normalized student identifier. Ignored in AllSubjects mode.
	Wcode string
	// SubjectIDs filters AllSubjects mode to explicit subjects.
	SubjectIDs []string
	// FromUTC is the inclusive institute-day lower bound as an instant.
	FromUTC time.Time
	// ToExclusiveUTC is the exclusive upper bound (inclusive date_to + 1 day).
	ToExclusiveUTC time.Time
	Mode           SessionsRangeFactsMode
	// InstituteTZ selects the cross-study weekday interpretation
	// (student_is_expected_at_session_tz). Empty defaults to Asia/Bangkok,
	// matching the server default; callers pass the configured institute zone.
	InstituteTZ string
	// Lifetime relaxes the set-1/2 display predicate for the explicit
	// authorized lifetime staff lookup (Step 17: admin + lifetime=true +
	// explicit range). The enrollment-membership join is unchanged; only
	// the expectation gate admits administratively-excluded sessions
	// (attendance override), which are still scope-relevant history the
	// lifetime view must show. Cross-study weekday selection still
	// applies. False preserves the legacy/V2 predicate exactly.
	Lifetime bool
}

// SessionsRangeFactRow is one session plus the course/subject/teacher labels
// the response needs and the merge-group membership required to derive merged
// display ranges and absence scopes without further queries. MergeGroupID is
// the zero UUID when the course belongs to no merge group.
type SessionsRangeFactRow struct {
	SessionID    pgtype.UUID
	StartAt      pgtype.Timestamptz
	EndAt        pgtype.Timestamptz
	CourseID     pgtype.UUID
	CourseCode   string
	CourseName   string
	SubjectID    pgtype.UUID
	SubjectCode  string
	SubjectName  string
	TeacherName  string
	MergeGroupID pgtype.UUID
}

// SessionsRangeFacts loads the complete session fact set in ONE round trip.
//
// Query-count contract: exactly one Query call. Callers must not loop
// per-course queries around it.
//
// Cost statement: row volume is O(window sessions of the student's courses)
// and is fundamental — every returned row is rendered. Trip count is
// constant. Window abuse is bounded above by the handler range cap
// (maxStaffSessionsRangeDays), so worst-case work is ~1 year of one
// student's courses, served via sessions_active_course_start_idx.
func (q *Queries) SessionsRangeFacts(ctx context.Context, arg SessionsRangeFactsParams) ([]SessionsRangeFactRow, error) {
	switch arg.Mode {
	case SessionsRangeFactsStudent:
		return q.sessionsRangeFactsEnrolled(ctx, arg, true)
	case SessionsRangeFactsAllSubjects:
		return q.sessionsRangeFactsAllSubjects(ctx, arg)
	default:
		return q.sessionsRangeFactsEnrolled(ctx, arg, false)
	}
}

// sessionsRangeFactsEnrolledSQLText is the enrolled-modes fact query text
// with a %s hole for the expectation gate followed by a %s hole for the
// student-facing visibility predicate. Extracted verbatim so the Step-16
// head batch queues byte-identical SQL.
//
// Step 17: the expectation gate is rendered by
// sessionsRangeExpectationGate(Lifetime): the default arm is the legacy
// predicate byte-identical; the lifetime arm additionally admits
// administratively-excluded sessions (attendance override
// status='excluded' with a manual override_source), which remain
// scope-relevant history for the authorized lifetime view.
const sessionsRangeFactsEnrolledSQLText = `
		SELECT sess.id, sess.start_at, sess.end_at,
		       c.id, c.code, c.name,
		       sub.id, sub.code, sub.name,
		       COALESCE(NULLIF(u.full_name, ''), u.username, '') AS teacher_name,
		       COALESCE(mgm.group_id, '00000000-0000-0000-0000-000000000000'::uuid) AS merge_group_id
		FROM sessions sess
		JOIN courses c ON c.id = sess.course_id
		JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN users u ON u.id = c.teacher_id
		JOIN course_students cs ON cs.course_id = c.id AND cs.status = 'enrolled'
		JOIN students st ON st.id = cs.student_id
		LEFT JOIN course_merge_group_members mgm ON mgm.course_id = c.id
		WHERE st.wcode = $1
		  AND sess.start_at >= $2
		  AND sess.start_at < $3
		  AND sess.deleted_at IS NULL
		  AND %s%s
		ORDER BY sub.code, sess.start_at, sess.id
	`

// sessionsRangeExpectationGate renders the set-1/2 expectation predicate
// for the enrolled fact query. The default arm is the legacy predicate
// text byte-identical; the lifetime arm admits administratively-excluded
// sessions (manual attendance override) that the default gate drops but
// the authorized lifetime history view must show.
func sessionsRangeExpectationGate(lifetime bool) string {
	if lifetime {
		return `(student_is_expected_at_session_tz(st.id, sess.id, $4) OR EXISTS (` +
			`SELECT 1 FROM session_attendance sa ` +
			`WHERE sa.session_id = sess.id AND sa.student_id = st.id ` +
			`AND sa.status = 'excluded' ` +
			`AND COALESCE(sa.override_source, 'manual') <> 'cross_study'))`
	}
	return `student_is_expected_at_session_tz(st.id, sess.id, $4)`
}

func (q *Queries) sessionsRangeFactsEnrolled(ctx context.Context, arg SessionsRangeFactsParams, studentFacing bool) ([]SessionsRangeFactRow, error) {
	visibilityPredicate := ""
	if studentFacing {
		visibilityPredicate = " AND c.absence_form_visible" +
			" AND EXISTS (SELECT 1 FROM subject_active_courses sac" +
			" WHERE sac.subject_id = sub.id AND sac.course_id = c.id)"
	}
	// Step 5: cross-study weekday scope follows the configured institute zone
	// (student_is_expected_at_session_tz), not a hardcoded Bangkok offset.
	instituteTZ := arg.InstituteTZ
	if instituteTZ == "" {
		instituteTZ = "Asia/Bangkok"
	}
	rows, err := q.db.Query(ctx, fmt.Sprintf(sessionsRangeFactsEnrolledSQLText, sessionsRangeExpectationGate(arg.Lifetime), visibilityPredicate), arg.Wcode, arg.FromUTC, arg.ToExclusiveUTC, instituteTZ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessionsRangeFactRows(rows)
}

// sessionsRangeFactsAllSubjectsSQLText is the staff special-lookup fact
// query text. Extracted verbatim so the Step-16 head batch queues
// byte-identical SQL.
const sessionsRangeFactsAllSubjectsSQLText = `
		SELECT sess.id, sess.start_at, sess.end_at,
		       c.id, c.code, c.name,
		       sub.id, sub.code, sub.name,
		       COALESCE(NULLIF(u.full_name, ''), u.username, '') AS teacher_name,
		       COALESCE(mgm.group_id, '00000000-0000-0000-0000-000000000000'::uuid) AS merge_group_id
		FROM sessions sess
		JOIN courses c ON c.id = sess.course_id
		JOIN subjects sub ON sub.id = c.subject_id
		LEFT JOIN users u ON u.id = c.teacher_id
		LEFT JOIN course_merge_group_members mgm ON mgm.course_id = c.id
		WHERE sub.id::text = ANY(string_to_array($1, ','))
		  AND sess.start_at >= $2
		  AND sess.start_at < $3
		  AND sess.deleted_at IS NULL
		ORDER BY sub.code, c.code, sess.start_at, sess.id
	`

func (q *Queries) sessionsRangeFactsAllSubjects(ctx context.Context, arg SessionsRangeFactsParams) ([]SessionsRangeFactRow, error) {
	rows, err := q.db.Query(ctx, sessionsRangeFactsAllSubjectsSQLText, strings.Join(arg.SubjectIDs, ","), arg.FromUTC, arg.ToExclusiveUTC)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessionsRangeFactRows(rows)
}

// MergeSiblingRow is a window-wide raw sibling session for merged display
// ranges. Legacy mergedSessionRangesSQL applies NO enrollment, expectation,
// visibility, or timing gates to siblings — only the window + deleted flag.
// The service feeds these as extraSiblings to mergedRangesFromSiblings so
// timing/course-filtered or unenrolled siblings still contribute.
type MergeSiblingRow struct {
	SessionID    pgtype.UUID
	CourseID     pgtype.UUID
	MergeGroupID pgtype.UUID
	StartAt      pgtype.Timestamptz
	EndAt        pgtype.Timestamptz
}

// MergeSiblingsInRange loads raw sibling sessions for merge groups in ONE
// round trip. Call only for groups outside the bundle scope universe
// (all-subjects mode); enrolled modes derive siblings from bundle sessions.
func (q *Queries) MergeSiblingsInRange(ctx context.Context, groupIDs []pgtype.UUID, fromUTC, toExclusiveUTC time.Time) ([]MergeSiblingRow, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx, `
		SELECT s.id, s.course_id, mgm.group_id, s.start_at, s.end_at
		FROM sessions s
		JOIN course_merge_group_members mgm ON mgm.course_id = s.course_id
		WHERE mgm.group_id = ANY($1::uuid[])
		  AND s.start_at >= $2
		  AND s.start_at < $3
		  AND s.deleted_at IS NULL
	`, groupIDs, fromUTC, toExclusiveUTC)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MergeSiblingRow
	for rows.Next() {
		var r MergeSiblingRow
		if err := rows.Scan(&r.SessionID, &r.CourseID, &r.MergeGroupID, &r.StartAt, &r.EndAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanSessionsRangeFactRows(rows pgx.Rows) ([]SessionsRangeFactRow, error) {
	var out []SessionsRangeFactRow
	for rows.Next() {
		var r SessionsRangeFactRow
		if err := rows.Scan(&r.SessionID, &r.StartAt, &r.EndAt,
			&r.CourseID, &r.CourseCode, &r.CourseName,
			&r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.TeacherName,
			&r.MergeGroupID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
