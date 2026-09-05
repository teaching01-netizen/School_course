package absenceshttp

// Pure domain layer for sessions-range assembly (challenge sections 19/20).
//
// Rule: database access gathers facts (the bundle); domain logic decides.
// Every function in this file is pure: no Queries, no clock, no HTTP.
// Complexity notes are inline; nothing here may issue a query.

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences"
	sqldb "warwick-institute/internal/db"
)

// sessionFact is the normalized in-memory row: fact + derived institute day.
type sessionFact struct {
	row       sqldb.SessionsRangeFactRow
	id        string
	courseID  string
	subjectID string
	startAt   time.Time
	endAt     time.Time
	day       string
}

// normalizeSessionFacts converts fact rows to sessionFacts, computing the
// institute-local day ONCE per session: O(R). Rows with invalid timestamps
// or unformattable UUIDs are skipped (the old code failed the request on
// scan errors; the facts query guarantees valid rows, so a skip here only
// covers corrupt data).
func normalizeSessionFacts(rows []sqldb.SessionsRangeFactRow, instituteTZ string) []sessionFact {
	loc, err := instituteLocation(instituteTZ)
	if err != nil {
		loc = time.UTC
	}
	out := make([]sessionFact, 0, len(rows))
	for _, row := range rows {
		if !row.StartAt.Valid || !row.EndAt.Valid {
			continue
		}
		id, err := uuidString(row.SessionID)
		if err != nil {
			continue
		}
		courseID, err := uuidString(row.CourseID)
		if err != nil {
			continue
		}
		subjectID, err := uuidString(row.SubjectID)
		if err != nil {
			continue
		}
		out = append(out, sessionFact{
			row:       row,
			id:        id,
			courseID:  courseID,
			subjectID: subjectID,
			startAt:   row.StartAt.Time,
			endAt:     row.EndAt.Time,
			day:       row.StartAt.Time.In(loc).Format("2006-01-02"),
		})
	}
	return out
}

// applyTimingAndCourseFilter mirrors the old in-Go filters: timing policy
// (unless bypassed) and the course_ids allow-list. O(R).
func applyTimingAndCourseFilter(
	facts []sessionFact,
	lookup sessionsRangeLookup,
	settings absenceSettings,
	now time.Time,
) []sessionFact {
	allowed := lookup.courseAllowList()
	bypass := lookup.bypassTiming()
	out := facts[:0]
	for _, f := range facts {
		if !bypass && !sessionAllowedByTimingPolicy(settings.Form, now, sessionTimingInfo{StartAt: pgtype.Timestamptz{Time: f.startAt, Valid: true}, EndAt: pgtype.Timestamptz{Time: f.endAt, Valid: true}}) {
			continue
		}
		if len(allowed) > 0 && !allowed[f.courseID] {
			continue
		}
		out = append(out, f)
	}
	return out
}

func distinctFactCourseIDs(facts []sessionFact) []pgtype.UUID {
	seen := make(map[string]struct{}, len(facts))
	out := make([]pgtype.UUID, 0, len(facts))
	for _, f := range facts {
		if _, ok := seen[f.courseID]; ok {
			continue
		}
		seen[f.courseID] = struct{}{}
		out = append(out, f.row.CourseID)
	}
	return out
}

// mergedRangesFromFacts derives merged display ranges WITHOUT a per-course
// query: for each session in a merge group, the range is MIN(start)/MAX(end)
// over sibling sessions of the same group on the same institute day,
// restricted to the loaded fact window. This matches mergedSessionRangesSQL
// semantics for siblings inside [from, to): the old query filtered BOTH
// source and sibling start_at to the window, so siblings outside the window
// never contributed. Complexity O(R).
//
// siblings carries window-wide UNFILTERED sibling sessions (no timing,
// course, enrollment, or visibility gates): the old sibling join applied
// none of those gates, so deriving only from filtered facts would drop
// contributions from e.g. timing-excluded siblings. Callers must derive
// ranges BEFORE applyTimingAndCourseFilter, passing the same pre-filter
// facts as both arguments when no extra siblings were loaded.
//
// Day-partition note: the old query partitioned by SIBLING day and collapsed
// rows per source with an unordered map overwrite, which is nondeterministic
// when one group spans several institute days in the window. This
// implementation groups by SOURCE session day (deterministic, identical
// whenever siblings share the source day). Documented hardening, not a
// silent change.
func mergedRangesFromFacts(facts []sessionFact, instituteTZ string) map[string][2]string {
	return mergedRangesFromSiblings(facts, nil, instituteTZ)
}

func mergedRangesFromSiblings(facts, extraSiblings []sessionFact, instituteTZ string) map[string][2]string {
	loc, err := instituteLocation(instituteTZ)
	if err != nil {
		loc = time.UTC
	}
	type dayKey struct {
		group string
		day   string
	}
	zero := "00000000-0000-0000-0000-000000000000"
	minStart := make(map[dayKey]time.Time)
	maxEnd := make(map[dayKey]time.Time)
	groupOf := make(map[string]dayKey, len(facts))
	sources := make(map[string]struct{}, len(facts))
	for _, f := range facts {
		sources[f.id] = struct{}{}
	}
	universe := make([]sessionFact, 0, len(facts)+len(extraSiblings))
	universe = append(universe, facts...)
	universe = append(universe, extraSiblings...)
	for _, f := range universe {
		if !f.row.MergeGroupID.Valid {
			continue
		}
		g := uuidStringOrZero(f.row.MergeGroupID)
		if g == "" || g == zero {
			continue
		}
		d := f.startAt.In(loc).Format("2006-01-02")
		k := dayKey{group: g, day: d}
		if _, ok := sources[f.id]; ok {
			groupOf[f.id] = k
		}
		if cur, ok := minStart[k]; !ok || f.startAt.Before(cur) {
			minStart[k] = f.startAt
		}
		if cur, ok := maxEnd[k]; !ok || f.endAt.After(cur) {
			maxEnd[k] = f.endAt
		}
	}
	out := make(map[string][2]string, len(groupOf))
	for id, k := range groupOf {
		out[id] = [2]string{
			minStart[k].UTC().Format(time.RFC3339Nano),
			maxEnd[k].UTC().Format(time.RFC3339Nano),
		}
	}
	return out
}

// courseGroupView is the grouped response accumulator. Grouping is O(R);
// insertion order is preserved like the old courseOrder slice.
type courseGroupView struct {
	courseID    string
	courseCode  string
	courseName  string
	subjectID   string
	subjectCode string
	subjectName string
	teacherName string
	sessions    []sessionFact
}

func groupFactsByCourse(facts []sessionFact) []*courseGroupView {
	grouped := make(map[string]*courseGroupView)
	var order []*courseGroupView
	for _, f := range facts {
		g := grouped[f.courseID]
		if g == nil {
			g = &courseGroupView{
				courseID:    f.courseID,
				courseCode:  f.row.CourseCode,
				courseName:  f.row.CourseName,
				subjectID:   f.subjectID,
				subjectCode: f.row.SubjectCode,
				subjectName: f.row.SubjectName,
				teacherName: f.row.TeacherName,
			}
			grouped[f.courseID] = g
			order = append(order, g)
		}
		g.sessions = append(g.sessions, f)
	}
	return order
}

// limitStatsForScope converts batched day counts to the legacy limit stats,
// using the same constructor as the old per-course path, so the
// remaining = max(maximum - used, 0) invariant is shared, not duplicated.
func limitStatsForScope(counts sqldb.AbsenceDayCounts) absences.AbsenceDayLimitStats {
	return absences.NewAbsenceDayLimitStats(counts.TotalCourseDays, counts.UsedAbsenceDays, counts.UsedAbsenceDays)
}

// scopeRefForCourse maps a course to its absence-scope key using the batched
// scope facts (no per-course mergeGroupScopeForCourse query). O(scopes) per
// call; callers build a map once for O(1) amortized lookup.
func scopeRefMap(scopes []sqldb.SessionsRangeScopeFactsRow) map[string]*sqldb.SessionsRangeScopeFactsRow {
	byCourse := make(map[string]*sqldb.SessionsRangeScopeFactsRow)
	for i := range scopes {
		for _, cid := range scopes[i].CourseIDs {
			byCourse[uuidBytesStringLocal(cid)] = &scopes[i]
		}
	}
	return byCourse
}

func uuidBytesStringLocal(u pgtype.UUID) string {
	return uuidStringOrZero(u)
}
