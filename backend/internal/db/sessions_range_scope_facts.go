package db

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SessionsRangeScopeFacts extends SessionsRangeFacts with the per-request
// batched derivations that the old implementation fetched once per course:
//
//   - merge-group membership + names for every course in the fact set
//   - blocked (already-assigned) sit-in session IDs for the student
//   - absence-day counters for every absence scope in ONE query
//     (replaces N x AbsenceDayCountsForCourse/ForMergeGroup)
//
// Query-count contract: exactly two Query calls here (scopes one call, day
// counts one call) plus one blocked-sit-ins call. Conflict details ride on
// the blocked-sit-ins result, so no extra round trip is needed.
//
// NOTE: the handler still loads settings (1 QueryRow) via readAbsenceSettings
// and resolves the student row (1 QueryRow) for enrollment lookups, so the
// full endpoint stays at <= 5 round trips independent of course count.
type AbsenceScopeKey struct {
	// MergeGroup is true when the scope is a merge group, false for a course.
	MergeGroup bool
	// MergeGroupID is valid when MergeGroup is true.
	MergeGroupID pgtype.UUID
	// CourseID is valid when MergeGroup is false.
	CourseID pgtype.UUID
}

// ScopeKey returns the canonical key used to group day counts.
func (k AbsenceScopeKey) String() string {
	if k.MergeGroup {
		return "merge:" + uuidBytesString(k.MergeGroupID)
	}
	return "course:" + uuidBytesString(k.CourseID)
}

// SessionsRangeScopeFactsRow carries one absence scope touched by the fact
// set: its courses and its human-readable merge-group name ("" when none).
type SessionsRangeScopeFactsRow struct {
	Key            AbsenceScopeKey
	CourseIDs      []pgtype.UUID
	MergeGroupName string
}

// scopeMembersCTE lists (course_id, group_id, group_name) for the request
// course universe in one scan. SessionsRangeScopeFacts scans it alone;
// SessionsRangeScopesAndDayCounts piggybacks the day-count aggregation on
// the same statement (Step 16: scopes and day counts share one trip).
const scopeMembersCTE = `
		scope_input AS (
			SELECT unnest($1::uuid[]) AS course_id
		),
		scope_members AS (
			SELECT i.course_id AS course_id, g.id AS group_id, g.name AS group_name
			FROM scope_input i
			LEFT JOIN course_merge_group_members m ON m.course_id = i.course_id
			LEFT JOIN course_merge_groups g ON g.id = m.group_id
		)
	`

// SessionsRangeScopeFacts resolves absence scopes for the given courses in
// ONE round trip: merge-group membership for all course IDs at once. Courses
// without a merge group each form their own course scope.
func (q *Queries) SessionsRangeScopeFacts(ctx context.Context, courseIDs []pgtype.UUID) ([]SessionsRangeScopeFactsRow, error) {
	if len(courseIDs) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx, `
		WITH `+scopeMembersCTE+`
		SELECT course_id, group_id, COALESCE(group_name, '') FROM scope_members
	`, courseIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type membership struct {
		groupID   pgtype.UUID
		groupName string
	}
	byCourse := make(map[string]membership, len(courseIDs))
	for rows.Next() {
		var courseID, groupID pgtype.UUID
		var name string
		if err := rows.Scan(&courseID, &groupID, &name); err != nil {
			return nil, err
		}
		if groupID.Valid {
			byCourse[uuidBytesString(courseID)] = membership{groupID: groupID, groupName: name}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	byScope := make(map[string]*SessionsRangeScopeFactsRow)
	order := make([]string, 0, len(courseIDs))
	for _, courseID := range courseIDs {
		key := "course:" + uuidBytesString(courseID)
		name := ""
		var scopeKey AbsenceScopeKey
		if m, ok := byCourse[uuidBytesString(courseID)]; ok {
			key = "merge:" + uuidBytesString(m.groupID)
			name = m.groupName
			scopeKey = AbsenceScopeKey{MergeGroup: true, MergeGroupID: m.groupID}
		} else {
			scopeKey = AbsenceScopeKey{CourseID: courseID}
		}
		row := byScope[key]
		if row == nil {
			row = &SessionsRangeScopeFactsRow{Key: scopeKey, MergeGroupName: name}
			byScope[key] = row
			order = append(order, key)
		}
		row.CourseIDs = append(row.CourseIDs, courseID)
	}
	out := make([]SessionsRangeScopeFactsRow, 0, len(order))
	for _, key := range order {
		out = append(out, *byScope[key])
	}
	return out, nil
}

// ScopeDayCounts maps scope key -> day counters.
type ScopeDayCounts map[string]AbsenceDayCounts

// SessionsRangeDayCountsParams binds the batched day-count query.
type SessionsRangeDayCountsParams struct {
	Wcode       string
	Scopes      []SessionsRangeScopeFactsRow
	InstituteTZ string
}

// SessionsRangeDayCounts computes total/used day counters for EVERY scope in
// ONE round trip. It unions course_days and used_days per scope and returns
// one row per scope. Complexity is O(scopes + relevant sessions + relevant
// absences), never O(courses x history).
//
// Reference definition (Step 15 — the contract this query implements):
//   - Scopes: each unmerged course is its own scope; each merge group is
//     one scope over its member courses (see SessionsRangeScopeFacts).
//   - TotalCourseDays: DISTINCT institute-local days of non-deleted sessions
//     where the student is expected (eligibility function), on the scope
//     course (course scope) or any member course (merge scope).
//   - UsedAbsenceDays: DISTINCT institute-local days from the student's
//     non-cancelled, non-special_approved absences: explicit = missed-session
//     days on scope courses with scope-equivalent absences (absence
//     merge_group_id matches, or absence course is a group member); legacy =
//     scope session days inside [date_from, date_to] of absences WITHOUT
//     missed sessions; used = explicit UNION legacy. Cancelled and
//     special_approved absences contribute nothing; same-day sessions count
//     once. Pinned by TestSessionsRangeDayCountsMatchesLegacy (explicit +
//     legacy + cancelled/special_approved + same-day + soft-deleted cases,
//     plus legacy Total/Used parity on the same world).
//
// Cost statement: the all-history scan per scope is fundamental, not
// accidental — TotalCourseDays/UsedAbsenceDays are defined over all time,
// so no window bound can apply (the plan explicitly forbids narrowing
// all-history counters to the display window). What was accidental (seq
// scans from lower(wcode) predicates) is removed via lower(wcode) functional
// indexes (migration 00122). A cross-request cache would trade freshness for
// speed and is deliberately absent: counts must reflect concurrent
// submissions. No maintained projection exists yet; introducing one requires
// the Step-15 gate (source facts, transactional updates, backfill +
// reconciliation, merge/TZ/eligibility handling, write-time validation) —
// until then, exactness comes from this single set-based aggregation with
// authoritative write-time validation in projectedAbsenceDayStats.
func (q *Queries) SessionsRangeDayCounts(ctx context.Context, arg SessionsRangeDayCountsParams) (ScopeDayCounts, error) {
	out := make(ScopeDayCounts, len(arg.Scopes))
	if len(arg.Scopes) == 0 {
		return out, nil
	}
	timezone := strings.TrimSpace(arg.InstituteTZ)
	if timezone == "" {
		timezone = "Asia/Bangkok"
	}
	mergeGroupIDs := make([]pgtype.UUID, 0)
	courseIDs := make([]pgtype.UUID, 0)
	for _, scope := range arg.Scopes {
		if scope.Key.MergeGroup {
			mergeGroupIDs = append(mergeGroupIDs, scope.Key.MergeGroupID)
		} else {
			courseIDs = append(courseIDs, scope.Key.CourseID)
		}
	}
	// One query, two grouped aggregations: per-course scopes and per-merge
	// scopes. A scope key prefix disambiguates the two namespaces.
	//
	// Step 15: eliminate repeated per-session eligibility work (the pre-Step-15
	// shape evaluated the eligibility function once per (student, session)
	// row in EACH of 4 arms over student_scope x sessions: course_days,
	// merge_days, explicit_days inner, legacy_days inner). The closed set of
	// sessions any arm can need is: scope sessions (course_days + merge_days
	// + legacy_days inner select the same eligible scope-session set, by
	// course or by merge member) UNION missed-link sessions (explicit_days
	// joins absence_missed_sessions -> sessions). Materialize that set ONCE
	// with one eligibility evaluation per session, then derive every arm.
	// Unrelated sessions (other courses, deleted, ineligible, unlinked) are
	// excluded up front, so unrelated-history growth cannot widen the arms.
	// SQL text lives in dayCountsQueryText (shared with the folded entry
	// point below); this path executes it standalone.
	rows, err := q.db.Query(ctx, dayCountsQueryText(), arg.Wcode, courseIDs, mergeGroupIDs, timezone, timezone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	totals := make(map[string]int32)
	used := make(map[string]int32)
	if err := scanDayCountRows(rows, totals, used); err != nil {
		return nil, err
	}
	for _, scope := range arg.Scopes {
		key := scope.Key.String()
		out[key] = AbsenceDayCounts{TotalCourseDays: totals[key], UsedAbsenceDays: used[key]}
	}
	return out, nil
}

// buildScopesFromMembers folds the Go scope-grouping behind
// SessionsRangeScopeFacts so the folded entry point below assembles scopes
// without a second statement. Input order is the request course order, so
// output order matches SessionsRangeScopeFacts exactly.
func buildScopesFromMembers(courseIDs []pgtype.UUID, byCourse map[string]struct {
	groupID   pgtype.UUID
	groupName string
}) []SessionsRangeScopeFactsRow {
	byScope := make(map[string]*SessionsRangeScopeFactsRow)
	order := make([]string, 0, len(courseIDs))
	for _, courseID := range courseIDs {
		key := "course:" + uuidBytesString(courseID)
		name := ""
		var scopeKey AbsenceScopeKey
		if m, ok := byCourse[uuidBytesString(courseID)]; ok {
			key = "merge:" + uuidBytesString(m.groupID)
			name = m.groupName
			scopeKey = AbsenceScopeKey{MergeGroup: true, MergeGroupID: m.groupID}
		} else {
			scopeKey = AbsenceScopeKey{CourseID: courseID}
		}
		row := byScope[key]
		if row == nil {
			row = &SessionsRangeScopeFactsRow{Key: scopeKey, MergeGroupName: name}
			byScope[key] = row
			order = append(order, key)
		}
		row.CourseIDs = append(row.CourseIDs, courseID)
	}
	out := make([]SessionsRangeScopeFactsRow, 0, len(order))
	for _, key := range order {
		out = append(out, *byScope[key])
	}
	return out
}

// SessionsRangeScopesAndDayCounts resolves absence scopes AND day counters
// in ONE statement (Step 16 fold): a scope_members arm carries the
// membership rows, and the Step-15 day-count aggregation rides the same
// statement ($1=request course IDs, $2=wcode, $3/$4=TZ; the course/merge
// universe is derived inside the statement via split/merge_split, never
// from separate ID arrays). Row shape is (text section, text key,
// uuid group_id, text name, int count), so a single scan loop routes
// scope_member rows to the scope builder and total/used rows to the
// counters. Empty courseIDs returns empty results with zero statements,
// exactly like the two separate entry points.
//
// Consistency: one statement is one snapshot — scopes and counters cannot
// skew from a concurrent merge-membership change between two statements.
// The fold changes NOTHING else: scope grouping runs the same code shape as
// SessionsRangeScopeFacts (buildScopesFromMembers), day counts run the same
// SQL (dayCountsQueryText) and scan (scanDayCountRows).
func (q *Queries) SessionsRangeScopesAndDayCounts(ctx context.Context, wcode string, courseIDs []pgtype.UUID, instituteTZ string) ([]SessionsRangeScopeFactsRow, ScopeDayCounts, error) {
	out := make(ScopeDayCounts)
	if len(courseIDs) == 0 {
		return nil, out, nil
	}
	timezone := strings.TrimSpace(instituteTZ)
	if timezone == "" {
		timezone = "Asia/Bangkok"
	}
	members, totals, used, err := q.queryScopesAndDayCounts(ctx, wcode, courseIDs, timezone)
	if err != nil {
		return nil, nil, err
	}
	scopes := buildScopesFromMembers(courseIDs, members)
	for _, scope := range scopes {
		key := scope.Key.String()
		out[key] = AbsenceDayCounts{TotalCourseDays: totals[key], UsedAbsenceDays: used[key]}
	}
	return scopes, out, nil
}

// ScopesCountsAbsentBlockedBatch is the Step-16 Group-4/5 tail batch:
// the scopes+daycounts fold (statement 1), the already-absent coverage
// probe (statement 2), and the blocked-sit-ins probe (statement 3) share
// ONE network round trip via pgx.Batch. Row-shape note: each statement
// keeps its own narrow shape (5-col fold / 1-col absent / 9-col blocked)
// — the rejected UNION ALL alternative would have padded every arm to the
// 10-col conflict width for zero trip savings, since pgx.Batch already
// shares one trip for N statements.
//
// Statement independence: the three SELECTs share only input params —
// neither reads another's rows — so one poisoned statement cannot corrupt
// another's result beyond the shared transport-failure semantics below.
// Draining is statement-safe: each statement is fully scanned before the
// next is touched, so a later-statement error surfaces only after earlier
// rows are consumed.
//
// Failure-mode contract (mirrors the separate calls exactly):
//   - fold-statement error -> returned (caller: ClassifyDBErr / 500).
//   - absent-statement error -> returned (caller: ClassifyDBErr / 500).
//   - blocked-statement error -> returned to the caller, which applies
//     the per-path legacy rule (enrolled: degrade to ResolveFailed/200;
//     all-subjects: request-fatal 500). The batch itself swallows nothing.
//   - batch-transport error on ANY drain -> returned the same way.
//   - br.Close error -> returned (drains unread results; no silent drop).
//
// Empty courseIDs short-circuits to zero statements (nil scopes, empty
// counts) plus the standalone absent probe — identical to the separate
// entry points, which also skip the fold when the universe is empty.
// The blocked probe is queued only when wantBlocked is true AND the
// student ID is valid; otherwise the caller gets nil blocked rows and
// applies its own skip/degrade rule (legacy issues zero resolve queries
// with no courses). Params per statement:
//
//	stmt 1 (fold): $1=request course IDs, $2=wcode, $3/$4=TZ.
//	stmt 2 (absent): $1=wcode, $2=TZ, $3=FromUTC, $4=ToExclusiveUTC.
//	stmt 3 (blocked): $1=student ID.
func (q *Queries) ScopesCountsAbsentBlockedBatch(ctx context.Context, wcode string, courseIDs []pgtype.UUID, instituteTZ string, fromUTC, toExclusiveUTC time.Time, studentID pgtype.UUID, wantBlocked bool) ([]SessionsRangeScopeFactsRow, ScopeDayCounts, map[string]bool, []ActiveSitInSessionConflict, error) {
	out := make(ScopeDayCounts)
	if len(courseIDs) == 0 {
		absent, err := q.SessionsRangeAlreadyAbsent(ctx, SessionsRangeAbsentParams{Wcode: wcode, InstituteTZ: instituteTZ, FromUTC: fromUTC, ToExclusiveUTC: toExclusiveUTC})
		if err != nil {
			return nil, nil, nil, nil, err
		}
		return nil, out, absent, nil, nil
	}
	timezone := strings.TrimSpace(instituteTZ)
	if timezone == "" {
		timezone = "Asia/Bangkok"
	}
	queueBlocked := wantBlocked && studentID.Valid
	var b pgx.Batch
	b.Queue(scopesAndDayCountsSQL(), courseIDs, wcode, timezone, timezone)
	b.Queue(sessionsRangeAbsentSQL(), wcode, timezone, fromUTC, toExclusiveUTC)
	if queueBlocked {
		b.Queue(sessionsRangeBlockedSQL(), studentID)
	}
	br := q.db.(interface {
		SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
	}).SendBatch(ctx, &b)
	defer br.Close()
	foldRows, err := br.Query()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	members, totals, used, err := scanFoldRows(foldRows)
	foldRows.Close()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	absentRows, err := br.Query()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	absent, err := scanAbsentRows(absentRows)
	absentRows.Close()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var blocked []ActiveSitInSessionConflict
	// Build scopes+counts BEFORE the blocked drain so a blocked-only
	// failure can still return them with the error (enrolled degrade
	// path serves counts with nil sit-ins, exactly like legacy).
	preScopes := buildScopesFromMembers(courseIDs, members)
	preOut := make(ScopeDayCounts, len(preScopes))
	for _, scope := range preScopes {
		key := scope.Key.String()
		preOut[key] = AbsenceDayCounts{TotalCourseDays: totals[key], UsedAbsenceDays: used[key]}
	}
	if queueBlocked {
		blockedRows, err := br.Query()
		if err != nil {
			return preScopes, preOut, absent, nil, &BlockedProbeError{Err: err, Scopes: preScopes, Counts: preOut, Absent: absent}
		}
		blocked, err = scanBlockedRows(blockedRows)
		blockedRows.Close()
		if err != nil {
			return preScopes, preOut, absent, nil, &BlockedProbeError{Err: err, Scopes: preScopes, Counts: preOut, Absent: absent}
		}
	}
	if err := br.Close(); err != nil {
		if queueBlocked {
			return preScopes, preOut, absent, nil, &BlockedProbeError{Err: err, Scopes: preScopes, Counts: preOut, Absent: absent}
		}
		return nil, nil, nil, nil, err
	}
	return preScopes, preOut, absent, blocked, nil
}

// BlockedProbeError carries a blocked-probe-only batch failure WITH the
// successfully drained fold+absent data, so the enrolled path can apply
// the legacy degrade rule (log + nil sit-ins, 200) instead of a 500.
// The all-subjects path treats it as request-fatal, matching legacy.
type BlockedProbeError struct {
	Err    error
	Scopes []SessionsRangeScopeFactsRow
	Counts ScopeDayCounts
	Absent map[string]bool
}

func (e *BlockedProbeError) Error() string {
	return "sessions-range: blocked probe failed: " + e.Err.Error()
}
func (e *BlockedProbeError) Unwrap() error { return e.Err }

// ScopesCountsAbsentBatch is the two-statement wrapper (fold + absent,
// no blocked probe) for non-service callers. Service paths use
// ScopesCountsAbsentBlockedBatch.
func (q *Queries) ScopesCountsAbsentBatch(ctx context.Context, wcode string, courseIDs []pgtype.UUID, instituteTZ string, fromUTC, toExclusiveUTC time.Time) ([]SessionsRangeScopeFactsRow, ScopeDayCounts, map[string]bool, error) {
	scopes, counts, absent, _, err := q.ScopesCountsAbsentBlockedBatch(ctx, wcode, courseIDs, instituteTZ, fromUTC, toExclusiveUTC, pgtype.UUID{}, false)
	if err != nil {
		return nil, nil, nil, err
	}
	return scopes, counts, absent, nil
}

// queryScopesAndDayCounts executes the folded statement: scope_members arm
// first (request course order is NOT a SQL guarantee, so Go regroups via
// buildScopesFromMembers), then the day-count aggregation over the
// split/merge_split CTEs derived from the SAME membership rows (a course in
// a merge group counts toward merge_days/used merge arms via merge_split
// and toward nothing course-scoped; an unmerged course counts toward
// course_days/used via split — identical to the separate Scopes split).
// Param order ($1..$4) is fixed by the text below: $1=request course IDs
// (scope_members arm + split/merge_split derivation), $2=wcode, $3/$4=TZ
// (day key + eligibility zone). The day-count arms read their course/merge
// universe from split/merge_split, never from separate ID arrays.
func errUnknownFoldSection(section string) error {
	return &foldSectionError{section: section}
}

type foldSectionError struct{ section string }

func (e *foldSectionError) Error() string { return "sessions-range: unknown fold section " + e.section }

// scopesAndDayCountsSQL returns the fold statement body shared by
// queryScopesAndDayCounts and ScopesCountsAbsentBatch: WITH prefix
// (params $1=request course IDs, $2=wcode, $3/$4=TZ) plus the final
// SELECT arms. Moved verbatim from the previously inline text, so both
// entry points execute byte-identical SQL.
func scopesAndDayCountsSQL() string {
	return `
		WITH ` + scopeMembersCTE + `,
		split AS (
			SELECT course_id AS id FROM scope_members WHERE group_id IS NULL
		), merge_split AS (
			SELECT DISTINCT group_id AS id FROM scope_members WHERE group_id IS NOT NULL
		),
		student_scope AS (
			SELECT id FROM students WHERE lower(wcode) = lower($2)
		), relevant_sessions AS MATERIALIZED (
			SELECT s.id AS session_id, s.course_id AS course_id,
			       (s.start_at AT TIME ZONE $3)::date AS day
			FROM sessions s
			CROSS JOIN student_scope st
			WHERE s.deleted_at IS NULL
			  AND student_is_expected_at_session_tz(st.id, s.id, $4)
			  AND (
				s.course_id IN (SELECT id FROM split)
				OR EXISTS (
					SELECT 1 FROM course_merge_group_members m
					WHERE m.course_id = s.course_id
					  AND m.group_id IN (SELECT id FROM merge_split)
				)
				OR EXISTS (
					SELECT 1 FROM absence_missed_sessions ams
					JOIN student_absences msa ON msa.id = ams.absence_id
					WHERE ams.session_id = s.id
					  AND lower(msa.wcode) = lower($2)
					  AND msa.status NOT IN ('cancelled', 'special_approved')
				)
			  )
		), course_days AS (
			SELECT DISTINCT rs.course_id AS course_id, NULL::uuid AS merge_group_id,
			       rs.day AS day
			FROM relevant_sessions rs
			WHERE rs.course_id IN (SELECT id FROM split)
		), merge_days AS (
			SELECT DISTINCT NULL::uuid AS course_id, m.group_id AS merge_group_id,
			       rs.day AS day
			FROM relevant_sessions rs
			JOIN course_merge_group_members m ON m.course_id = rs.course_id
			WHERE m.group_id IN (SELECT id FROM merge_split)
		), scoped_absences AS (
			SELECT sa.id, sa.course_id, sa.merge_group_id,
			       sa.date_from, sa.date_to,
			       EXISTS (SELECT 1 FROM absence_missed_sessions ams WHERE ams.absence_id = sa.id) AS has_missed
			FROM student_absences sa
			WHERE lower(sa.wcode) = lower($2)
			  AND sa.status NOT IN ('cancelled', 'special_approved')
		), explicit_days AS (
			SELECT DISTINCT
				CASE WHEN mc.group_id IS NULL THEN rs.course_id ELSE NULL END AS course_id,
				mc.group_id AS merge_group_id,
				rs.day AS day
			FROM scoped_absences sa
			JOIN absence_missed_sessions ams ON ams.absence_id = sa.id
			JOIN relevant_sessions rs ON rs.session_id = ams.session_id
			LEFT JOIN course_merge_group_members mc ON mc.course_id = rs.course_id
			WHERE (
				(mc.group_id IS NOT NULL AND mc.group_id IN (SELECT id FROM merge_split))
				OR
				(rs.course_id IN (SELECT id FROM split))
			  )
			  AND (
				sa.merge_group_id IS NOT DISTINCT FROM mc.group_id
				OR (sa.merge_group_id IS NULL AND rs.course_id = sa.course_id)
				OR (sa.merge_group_id IS NULL AND mc.group_id IS NOT NULL AND EXISTS (
					SELECT 1 FROM course_merge_group_members m2
					WHERE m2.group_id = mc.group_id AND m2.course_id = sa.course_id
				))
			  )
		), legacy_days AS (
			SELECT DISTINCT
				CASE WHEN mg.group_id IS NULL THEN rs.course_id ELSE NULL END AS course_id,
				mg.group_id AS merge_group_id,
				rs.day AS day
			FROM scoped_absences sa
			JOIN relevant_sessions rs ON (
				rs.course_id IN (SELECT id FROM split)
				OR EXISTS (
					SELECT 1 FROM course_merge_group_members mmcheck
					WHERE mmcheck.course_id = rs.course_id
					  AND mmcheck.group_id IN (SELECT id FROM merge_split)
				)
			)
			LEFT JOIN course_merge_group_members mg ON mg.course_id = rs.course_id
			WHERE NOT sa.has_missed
			  AND rs.day BETWEEN sa.date_from AND sa.date_to
			  AND (
				(sa.merge_group_id IS NOT NULL AND sa.merge_group_id = mg.group_id)
				OR (sa.merge_group_id IS NULL AND sa.course_id = rs.course_id)
				OR (sa.merge_group_id IS NULL AND mg.group_id IS NOT NULL AND EXISTS (
					SELECT 1 FROM course_merge_group_members m2
					WHERE m2.group_id = mg.group_id AND m2.course_id = sa.course_id
				))
			  )
		), used_days AS (
			SELECT course_id, merge_group_id, day FROM explicit_days
			UNION
			SELECT course_id, merge_group_id, day FROM legacy_days
		)
		SELECT 'scope_member', course_id::text, group_id, COALESCE(group_name, ''), 0 FROM scope_members
		UNION ALL
		SELECT 'total', 'course:' || course_id::text, NULL::uuid, '', count(DISTINCT day) FROM course_days GROUP BY 2
		UNION ALL
		SELECT 'total', 'merge:' || merge_group_id::text, NULL::uuid, '', count(DISTINCT day) FROM merge_days GROUP BY 2
		UNION ALL
		SELECT 'used', COALESCE('merge:' || merge_group_id::text, 'course:' || course_id::text), NULL::uuid, '', count(DISTINCT day) FROM used_days GROUP BY 2
	`
}

func (q *Queries) queryScopesAndDayCounts(ctx context.Context, wcode string, courseIDs []pgtype.UUID, timezone string) (map[string]struct {
	groupID   pgtype.UUID
	groupName string
}, map[string]int32, map[string]int32, error) {
	members := make(map[string]struct {
		groupID   pgtype.UUID
		groupName string
	})
	// No Go pre-query: split/merge_split derive the course/merge universe
	// from scope_members INSIDE the same statement, and the day-count arms
	// read them via IN-subselects instead of separate ID arrays.
	// Group-4 (blocked sit-ins) intentionally does NOT ride this UNION ALL:
	// conflict rows are 10 columns (2 uuid + 2 text + 2 date + 2 text +
	// 2 timestamptz), and padding every scope/count arm to that shape would
	// widen every row for zero trip savings (pgx.Batch already shares one
	// trip for 2+ statements — see ScopesCountsAbsentBlockedBatch).
	rows, err := q.db.Query(ctx, scopesAndDayCountsSQL(), courseIDs, wcode, timezone, timezone)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	// Single scan path: the tail batch drains the identical row shape
	// through scanFoldRows, so inline duplication cannot diverge.
	fresh, totals, used, err := scanFoldRows(rows)
	if err != nil {
		return nil, nil, nil, err
	}
	for k, v := range fresh {
		members[k] = v
	}
	return members, totals, used, nil
}

// scanFoldRows drains a fold result set into fresh maps. Shared by
// queryScopesAndDayCounts and the tail batch (ScopesCountsAbsentBlockedBatch
// notes) so both entry points scan byte-identical rows the same way.
func scanFoldRows(rows pgx.Rows) (map[string]struct {
	groupID   pgtype.UUID
	groupName string
}, map[string]int32, map[string]int32, error) {
	members := make(map[string]struct {
		groupID   pgtype.UUID
		groupName string
	})
	totals := make(map[string]int32)
	used := make(map[string]int32)
	for rows.Next() {
		var section, key string
		var id, groupID pgtype.UUID
		var name string
		var count int32
		if err := rows.Scan(&section, &key, &groupID, &name, &count); err != nil {
			return nil, nil, nil, err
		}
		if section == "scope_member" {
			if err := id.Scan(key); err != nil {
				return nil, nil, nil, err
			}
			if groupID.Valid {
				members[uuidBytesString(id)] = struct {
					groupID   pgtype.UUID
					groupName string
				}{groupID: groupID, groupName: name}
			}
		} else if section == "used" {
			used[key] = count
		} else if section == "total" {
			totals[key] = count
		} else {
			return nil, nil, nil, errUnknownFoldSection(section)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	return members, totals, used, nil
}

// dayCountsQueryText is the set-based aggregation behind
// SessionsRangeDayCounts, shared with SessionsRangeScopesAndDayCounts so the
// two entry points execute byte-identical SQL. $1=wcode, $2=course IDs,
// $3=merge IDs, $4/$5=institute TZ (day key + eligibility zone).
func dayCountsQueryText() string {
	return `
		WITH student_scope AS (
			SELECT id FROM students WHERE lower(wcode) = lower($1)
		), relevant_sessions AS MATERIALIZED (
			SELECT s.id AS session_id, s.course_id AS course_id,
			       (s.start_at AT TIME ZONE $4)::date AS day
			FROM sessions s
			CROSS JOIN student_scope st
			WHERE s.deleted_at IS NULL
			  AND student_is_expected_at_session_tz(st.id, s.id, $5)
			  AND (
				s.course_id = ANY($2::uuid[])
				OR EXISTS (
					SELECT 1 FROM course_merge_group_members m
					WHERE m.course_id = s.course_id
					  AND m.group_id = ANY($3::uuid[])
				)
				OR EXISTS (
					SELECT 1 FROM absence_missed_sessions ams
					JOIN student_absences msa ON msa.id = ams.absence_id
					WHERE ams.session_id = s.id
					  AND lower(msa.wcode) = lower($1)
					  AND msa.status NOT IN ('cancelled', 'special_approved')
				)
			  )
		), course_days AS (
			SELECT DISTINCT rs.course_id AS course_id, NULL::uuid AS merge_group_id,
			       rs.day AS day
			FROM relevant_sessions rs
			WHERE rs.course_id = ANY($2::uuid[])
		), merge_days AS (
			SELECT DISTINCT NULL::uuid AS course_id, m.group_id AS merge_group_id,
			       rs.day AS day
			FROM relevant_sessions rs
			JOIN course_merge_group_members m ON m.course_id = rs.course_id
			WHERE m.group_id = ANY($3::uuid[])
		), scoped_absences AS (
			SELECT sa.id, sa.course_id, sa.merge_group_id,
			       sa.date_from, sa.date_to,
			       EXISTS (SELECT 1 FROM absence_missed_sessions ams WHERE ams.absence_id = sa.id) AS has_missed
			FROM student_absences sa
			WHERE lower(sa.wcode) = lower($1)
			  AND sa.status NOT IN ('cancelled', 'special_approved')
		), explicit_days AS (
			SELECT DISTINCT
				CASE WHEN mc.group_id IS NULL THEN rs.course_id ELSE NULL END AS course_id,
				mc.group_id AS merge_group_id,
				rs.day AS day
			FROM scoped_absences sa
			JOIN absence_missed_sessions ams ON ams.absence_id = sa.id
			JOIN relevant_sessions rs ON rs.session_id = ams.session_id
			LEFT JOIN course_merge_group_members mc ON mc.course_id = rs.course_id
			WHERE (
				(mc.group_id IS NOT NULL AND mc.group_id = ANY($3::uuid[]))
				OR
				(rs.course_id = ANY($2::uuid[]))
			  )
			  AND (
				sa.merge_group_id IS NOT DISTINCT FROM mc.group_id
				OR (sa.merge_group_id IS NULL AND rs.course_id = sa.course_id)
				OR (sa.merge_group_id IS NULL AND mc.group_id IS NOT NULL AND EXISTS (
					SELECT 1 FROM course_merge_group_members m2
					WHERE m2.group_id = mc.group_id AND m2.course_id = sa.course_id
				))
			  )
		), legacy_days AS (
			SELECT DISTINCT
				CASE WHEN mg.group_id IS NULL THEN rs.course_id ELSE NULL END AS course_id,
				mg.group_id AS merge_group_id,
				rs.day AS day
			FROM scoped_absences sa
			JOIN relevant_sessions rs ON (
				rs.course_id = ANY($2::uuid[])
				OR EXISTS (
					SELECT 1 FROM course_merge_group_members mmcheck
					WHERE mmcheck.course_id = rs.course_id
					  AND mmcheck.group_id = ANY($3::uuid[])
				)
			)
			LEFT JOIN course_merge_group_members mg ON mg.course_id = rs.course_id
			WHERE NOT sa.has_missed
			  AND rs.day BETWEEN sa.date_from AND sa.date_to
			  AND (
				(sa.merge_group_id IS NOT NULL AND sa.merge_group_id = mg.group_id)
				OR (sa.merge_group_id IS NULL AND sa.course_id = rs.course_id)
				OR (sa.merge_group_id IS NULL AND mg.group_id IS NOT NULL AND EXISTS (
					SELECT 1 FROM course_merge_group_members m2
					WHERE m2.group_id = mg.group_id AND m2.course_id = sa.course_id
				))
			  )
		), used_days AS (
			SELECT course_id, merge_group_id, day FROM explicit_days
			UNION
			SELECT course_id, merge_group_id, day FROM legacy_days
		)
		SELECT 'total', 'course:' || course_id::text, count(DISTINCT day) FROM course_days GROUP BY 2
		UNION ALL
		SELECT 'total', 'merge:' || merge_group_id::text, count(DISTINCT day) FROM merge_days GROUP BY 2
		UNION ALL
		SELECT 'used', COALESCE('merge:' || merge_group_id::text, 'course:' || course_id::text), count(DISTINCT day) FROM used_days GROUP BY 2
	`
}

// scanDayCountRows drains the day-count aggregation into totals/used maps.
// Shared by SessionsRangeDayCounts and SessionsRangeScopesAndDayCounts so a
// scan-shape change cannot silently diverge the two paths.
func scanDayCountRows(rows pgx.Rows, totals, used map[string]int32) error {
	for rows.Next() {
		var section, key string
		var count int32
		if err := rows.Scan(&section, &key, &count); err != nil {
			return err
		}
		if section == "used" {
			used[key] = count
		} else {
			totals[key] = count
		}
	}
	return rows.Err()
}

// sessionsRangeBlockedSQLText is the blocked-sit-ins SELECT shared by the
// standalone loader and the tail batch. Byte-identical predicate and
// ORDER BY; param $1=student ID.
func sessionsRangeBlockedSQLText() string {
	return `
		SELECT asi.session_id, sa.id,
		       COALESCE(abs_subj.name, ''), sa.date_from, sa.date_to,
		       COALESCE(sit_subj.name, ''), COALESCE(sit_course.name, ''), sit_session.start_at, sit_session.end_at
		FROM absence_sit_ins asi
		JOIN student_absences sa ON sa.id = asi.absence_id
		JOIN students st ON lower(st.wcode) = lower(sa.wcode)
		JOIN sessions sit_session ON sit_session.id = asi.session_id
		JOIN courses sit_course ON sit_course.id = sit_session.course_id
		LEFT JOIN subjects sit_subj ON sit_subj.id = sit_course.subject_id
		LEFT JOIN subjects abs_subj ON abs_subj.id = sa.subject_id
		WHERE st.id = $1
		  AND sa.status <> 'cancelled'
		ORDER BY asi.session_id, sa.created_at DESC
	`
}

// sessionsRangeBlockedSQL queues the same text on a batch.
func sessionsRangeBlockedSQL() string { return sessionsRangeBlockedSQLText() }

// SessionsRangeBlockedSitIns loads the blocked (already-assigned) sit-in
// session IDs plus per-session conflict details for one student in ONE round
// trip. It replaces the old per-course pair
// (ActiveSitInSessionIDsForStudent + ActiveSitInSessionConflictsForStudent
// per resolveSitInForCourse call) with a single student-scoped fetch whose
// cost is O(active sit-ins of this student), independent of course count.
func (q *Queries) SessionsRangeBlockedSitIns(ctx context.Context, studentID pgtype.UUID) ([]ActiveSitInSessionConflict, error) {
	rows, err := q.db.Query(ctx, sessionsRangeBlockedSQLText(), studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBlockedRows(rows)
}

// scanBlockedRows drains a blocked-sit-ins result set (standalone or the
// tail batch's third statement) with first-wins dedup per session: rows
// arrive newest-absence-first (ORDER BY session, created DESC), mirroring
// ActiveSitInSessionConflictsForStudent exactly.
func scanBlockedRows(rows pgx.Rows) ([]ActiveSitInSessionConflict, error) {
	var out []ActiveSitInSessionConflict
	seen := make(map[string]struct{})
	for rows.Next() {
		var item ActiveSitInSessionConflict
		if err := rows.Scan(&item.SessionID, &item.AbsenceID, &item.AbsenceSubjectName,
			&item.AbsenceDateFrom, &item.AbsenceDateTo,
			&item.SitInSubjectName, &item.SitInCourseName,
			&item.SitInStartAt, &item.SitInEndAt); err != nil {
			return nil, err
		}
		key := uuidBytesString(item.SessionID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

var _ = pgx.ErrNoRows

func uuidBytesString(u pgtype.UUID) string {
	if !u.Valid {
		return "00000000-0000-0000-0000-000000000000"
	}
	return sprintfUUID(u.Bytes)
}

func sprintfUUID(b [16]byte) string {
	const hexd = "0123456789abcdef"
	var out [36]byte
	hex := func(v byte) (byte, byte) { return hexd[v>>4], hexd[v&0x0f] }
	out[0], out[1] = hex(b[0])
	out[2], out[3] = hex(b[1])
	out[4], out[5] = hex(b[2])
	out[6], out[7] = hex(b[3])
	out[8] = '-'
	out[9], out[10] = hex(b[4])
	out[11], out[12] = hex(b[5])
	out[13] = '-'
	out[14], out[15] = hex(b[6])
	out[16], out[17] = hex(b[7])
	out[18] = '-'
	out[19], out[20] = hex(b[8])
	out[21], out[22] = hex(b[9])
	out[23] = '-'
	out[24], out[25] = hex(b[10])
	out[26], out[27] = hex(b[11])
	out[28], out[29] = hex(b[12])
	out[30], out[31] = hex(b[13])
	out[32], out[33] = hex(b[14])
	out[34], out[35] = hex(b[15])
	return string(out[:])
}
