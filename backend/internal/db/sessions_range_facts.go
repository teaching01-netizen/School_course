package db

import (
	"context"
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
//   - Complexity is O(R) where R is the number of sessions relevant to this
//     student, never O(total sessions in the database) and never O(R x courses).
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

func (q *Queries) sessionsRangeFactsEnrolled(ctx context.Context, arg SessionsRangeFactsParams, studentFacing bool) ([]SessionsRangeFactRow, error) {
	visibilityPredicate := ""
	if studentFacing {
		visibilityPredicate = " AND c.absence_form_visible" +
			" AND EXISTS (SELECT 1 FROM subject_active_courses sac" +
			" WHERE sac.subject_id = sub.id AND sac.course_id = c.id)"
	}
	rows, err := q.db.Query(ctx, `
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
		  AND student_is_expected_at_session(st.id, sess.id)`+visibilityPredicate+`
		ORDER BY sub.code, sess.start_at, sess.id
	`, arg.Wcode, arg.FromUTC, arg.ToExclusiveUTC)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessionsRangeFactRows(rows)
}

func (q *Queries) sessionsRangeFactsAllSubjects(ctx context.Context, arg SessionsRangeFactsParams) ([]SessionsRangeFactRow, error) {
	rows, err := q.db.Query(ctx, `
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
	`, strings.Join(arg.SubjectIDs, ","), arg.FromUTC, arg.ToExclusiveUTC)
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
