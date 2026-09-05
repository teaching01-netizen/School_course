package absenceshttp

// O(1) service orchestrator for sessions-in-range (challenge core).
//
// Query-count contract: every Query call below is listed; none sits in a
// per-course/per-session loop. Bundle internals add a fixed ~8 (see
// sessions_range_sitin_bundle.go). Totals: enrolled modes ~13 round trips,
// all-subjects ~7, independent of course/session counts. The legacy path
// issued 4-8 queries PER COURSE plus unbounded session scans per target.
//
// Failure-mode contract (mirrors legacy exactly):
//   request-fatal (ClassifyDBErr / 500): settings, facts, merged siblings,
//     absent, scopes, day counts, blocked/conflicts (all-subjects only).
//   per-course swallowed (log + nil sit-in, request stays 200): everything
//     inside bundle resolve, via SitInBundleV2.ResolveFailed or per-course
//     resolve errors.

import (
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences"
	sqldb "warwick-institute/internal/db"
)

// sessionsRangeUseV2 gates the O(1) path. Default ON; set
// WARWICK_SESSIONS_RANGE_V2=0 to roll back to the legacy per-course path.
// Both paths preserve identical response/error semantics.
func sessionsRangeUseV2() bool {
	if v, ok := os.LookupEnv("WARWICK_SESSIONS_RANGE_V2"); ok && (v == "0" || v == "false" || v == "off") {
		return false
	}
	return true
}

// serveSessionsRangeV2 runs the O(1) sessions-in-range pipeline.
func (s *server) serveSessionsRangeV2(w http.ResponseWriter, r *http.Request, forcedWCode string, requireAdmin bool) {
	pre, ok := parseSessionsRangePrelim(s, w, r, forcedWCode, requireAdmin)
	if !ok {
		return
	}
	settings, err := s.readAbsenceSettings(r)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	lookup, ok := finalizeSessionsRangeLookup(s, w, r, pre, settings)
	if !ok {
		return
	}
	ctx := r.Context()
	now := time.Now()
	wcode := lookup.studentWCode()
	window := pre.window

	var factRows []sqldb.SessionsRangeFactRow
	if lookup.isAllSubjects() {
		allSubj := lookup.(StaffAllSubjectsLookup)
		factRows, err = s.deps.Q.SessionsRangeFacts(ctx, sqldb.SessionsRangeFactsParams{SubjectIDs: allSubj.SubjectIDs, FromUTC: window.from, ToExclusiveUTC: window.toExclusive, Mode: sqldb.SessionsRangeFactsAllSubjects})
	} else if lookup.isStudent() {
		factRows, err = s.deps.Q.SessionsRangeFacts(ctx, sqldb.SessionsRangeFactsParams{Wcode: wcode, FromUTC: window.from, ToExclusiveUTC: window.toExclusive, Mode: sqldb.SessionsRangeFactsStudent})
	} else {
		factRows, err = s.deps.Q.SessionsRangeFacts(ctx, sqldb.SessionsRangeFactsParams{Wcode: wcode, FromUTC: window.from, ToExclusiveUTC: window.toExclusive, Mode: sqldb.SessionsRangeFactsStaff})
	}
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	preFilter := normalizeSessionFacts(factRows, s.deps.InstituteTZ)
	if len(preFilter) != len(factRows) {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading sessions")
		return
	}
	facts := applyTimingAndCourseFilter(preFilter, lookup, settings, now)

	if lookup.isAllSubjects() {
		s.serveSessionsRangeV2AllSubjects(w, r, lookup.(StaffAllSubjectsLookup), settings, preFilter, facts, window)
		return
	}
	s.serveSessionsRangeV2Enrolled(w, r, lookup, settings, pre, preFilter, facts)
}

// serveSessionsRangeV2Enrolled serves staff/student enrolled modes.
func (s *server) serveSessionsRangeV2Enrolled(w http.ResponseWriter, r *http.Request, lookup sessionsRangeLookup, settings absenceSettings, pre sessionsRangePrelim, preFilter, facts []sessionFact) {
	ctx := r.Context()
	wcode := lookup.studentWCode()
	window := pre.window
	adminRequest := pre.adminRequest

	// Legacy resolves the student per course and swallows lookup failures
	// into nil sit-ins (request stays 200 with day counts). A missing student
	// therefore degrades resolution, never the request: with zero facts the
	// legacy loop also yields an empty 200.
	student, studentErr := s.deps.Q.StudentGetByWCode(ctx, wcode)
	studentMissing := studentErr != nil
	if studentMissing && s.deps.Log != nil {
		s.deps.Log.Error("sessions-range student lookup failed", "error", studentErr)
	}

	// A missing student degrades resolution, never the request. Skip the
	// bundle round trips outright: a zero student ID would match nothing
	// anyway, and every downstream consumer treats ResolveFailed as nil
	// sit-in with a 200 response.
	var bundle *sqldb.SitInBundleV2
	if studentMissing {
		bundle = &sqldb.SitInBundleV2{ResolveFailed: true}
	} else {
		var err error
		bundle, err = s.deps.Q.SessionsRangeSitInBundleV2(ctx, sqldb.SitInBundleV2Params{StudentID: student.ID, MissedCourseIDs: distinctFactCourseIDs(facts)})
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}
	}
	if bundle.ResolveFailed && s.deps.Log != nil {
		s.deps.Log.Error("sit-in bundle degraded", "wcode", wcode)
	}

	merged := mergedRangesFromSiblings(facts, append(preFilter, bundleWindowSiblings(bundle, facts, window)...), s.deps.InstituteTZ)

	absent, err := s.deps.Q.SessionsRangeAlreadyAbsent(ctx, sqldb.SessionsRangeAbsentParams{Wcode: wcode, InstituteTZ: s.deps.InstituteTZ, FromUTC: window.from, ToExclusiveUTC: window.toExclusive})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	order := groupFactsByCourse(facts)
	scopes, err := s.deps.Q.SessionsRangeScopeFacts(ctx, distinctFactCourseIDs(facts))
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	scopeByCourse := scopeRefMap(scopes)
	counts, err := s.deps.Q.SessionsRangeDayCounts(ctx, sqldb.SessionsRangeDayCountsParams{Wcode: wcode, Scopes: scopes, InstituteTZ: s.deps.InstituteTZ})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	// Policies + blocked sit-ins feed per-course resolution only: legacy
	// swallows their failures into nil sit-ins (200), so degrade here.
	// Skipped entirely with no courses (legacy issues zero resolve queries).
	var policies []byte
	blocked := make(map[string]struct{})
	conflicts := make(map[string]*sitInSessionConflictInfo)
	if len(order) > 0 && !bundle.ResolveFailed {
		policiesRow, policiesErr := s.deps.Q.AppSettingsGetWithPolicies(ctx)
		blockedRows, blockedErr := s.deps.Q.SessionsRangeBlockedSitIns(ctx, student.ID)
		if policiesErr != nil || blockedErr != nil {
			if s.deps.Log != nil {
				s.deps.Log.Error("sessions-range resolve inputs degraded")
			}
			bundle.ResolveFailed = true
		} else {
			policies = policiesRow.AbsencePolicies
			blocked = make(map[string]struct{}, len(blockedRows))
			conflicts = make(map[string]*sitInSessionConflictInfo, len(blockedRows))
			for _, row := range blockedRows {
				k := uuidStringOrZero(row.SessionID)
				blocked[k] = struct{}{}
				conflicts[k] = sitInConflictInfo(row)
			}
		}
	}
	studentFacing := !adminRequest
	sitInForCourse := func(g *courseGroupView) *courseSitInJSON {
		return s.resolveEnrolledCourseSitInV2(g, bundle, policies, blocked, conflicts, studentFacing, pre, lookup, wcode, student.ID)
	}
	courses := assembleCourseResponses(order, scopeByCourse, counts, merged, absent, s.deps.InstituteTZ, sitInForCourse)
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"subjects": courses})
}

// resolveEnrolledCourseSitInV2 mirrors the legacy per-course resolve block:
// derived resolve window, swallow-and-log errors, none-gate, len-guarded
// session lists.
func (s *server) resolveEnrolledCourseSitInV2(g *courseGroupView, bundle *sqldb.SitInBundleV2, policies []byte, blocked map[string]struct{}, conflicts map[string]*sitInSessionConflictInfo, studentFacing bool, pre sessionsRangePrelim, lookup sessionsRangeLookup, wcode string, studentID pgtype.UUID) *courseSitInJSON {
	if bundle.ResolveFailed {
		return nil
	}
	courseID, cErr := s.a.ParseUUID(g.courseID)
	if cErr != nil {
		return nil
	}
	subjectID, sErr := s.a.ParseUUID(g.subjectID)
	if sErr != nil {
		return nil
	}
	resolveFrom, resolveTo := resolveDateRangeForSessionStartsInZone(sessionFactStartAts(g.sessions), pre.dateFrom, pre.dateTo, s.deps.InstituteTZ)
	result, resolveErr := resolveSitInForCourseFromBundle(bundleSitInInputs{bundle: bundle, policies: policies, blocked: blocked, conflicts: conflicts, instituteTZ: s.deps.InstituteTZ, afterPriority: lookup.satAfterPriority(), studentFacing: studentFacing}, wcode, studentID, courseID, subjectID, resolveFrom, resolveTo.AddDate(0, 0, 1))
	if resolveErr != nil {
		if s.deps.Log != nil {
			s.deps.Log.Error("sit-in resolution failed", "course_id", g.courseID, "error", resolveErr)
		}
		return nil
	}
	if result == nil || result.SitInMethod == SitInMethodNone {
		return nil
	}
	sitIn := &courseSitInJSON{RuleName: result.RuleName, RuleType: result.RuleType, SitInMethod: result.SitInMethod, SitInCourse: result.SitInCourse, Priorities: result.Priorities, CurrentPriorityLevel: result.CurrentPriorityLevel, HasNextPriority: result.HasNextPriority, SitInByMissedSession: result.SitInByMissedSession}
	if len(result.Available) > 0 {
		sitIn.AvailableSessions = result.Available
	}
	if len(result.MissedSession) > 0 {
		sitIn.MissedSessions = result.MissedSession
	}
	if len(result.Unavailable) > 0 {
		sitIn.UnavailableSessions = result.Unavailable
	}
	return sitIn
}

// serveSessionsRangeV2AllSubjects serves the staff special sit-in lookup.
// Legacy-exact: availability pools from filtered sessions (blocked sessions
// appear in BOTH lists), per-course sit_in pool slices, day counts and
// scopes per course, no rule resolution.
func (s *server) serveSessionsRangeV2AllSubjects(w http.ResponseWriter, r *http.Request, lookup StaffAllSubjectsLookup, settings absenceSettings, preFilter, facts []sessionFact, window sessionsRangeWindow) {
	ctx := r.Context()
	wcode := lookup.WCode
	siblingRows, err := s.deps.Q.MergeSiblingsInRange(ctx, distinctFactGroups(facts), window.from, window.toExclusive)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	merged := mergedRangesFromSiblings(facts, append(preFilter, mergeSiblingFacts(siblingRows)...), s.deps.InstituteTZ)
	student, studentErr := s.deps.Q.StudentGetByWCode(ctx, wcode)
	var blocked map[string]struct{}
	conflicts := map[string]*sitInSessionConflictInfo{}
	if len(facts) > 0 {
		if studentErr != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error checking sit-in session availability")
			return
		}
		blockedRows, conflictErr := s.deps.Q.SessionsRangeBlockedSitIns(ctx, student.ID)
		if conflictErr != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error checking sit-in session details")
			return
		}
		blocked = make(map[string]struct{}, len(blockedRows))
		for _, row := range blockedRows {
			blocked[uuidStringOrZero(row.SessionID)] = struct{}{}
			conflicts[uuidStringOrZero(row.SessionID)] = sitInConflictInfo(row)
		}
	}

	absent, err := s.deps.Q.SessionsRangeAlreadyAbsent(ctx, sqldb.SessionsRangeAbsentParams{Wcode: wcode, InstituteTZ: s.deps.InstituteTZ, FromUTC: window.from, ToExclusiveUTC: window.toExclusive})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	order := groupFactsByCourse(facts)
	scopes, err := s.deps.Q.SessionsRangeScopeFacts(ctx, distinctFactCourseIDs(facts))
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	scopeByCourse := scopeRefMap(scopes)
	counts, err := s.deps.Q.SessionsRangeDayCounts(ctx, sqldb.SessionsRangeDayCountsParams{Wcode: wcode, Scopes: scopes, InstituteTZ: s.deps.InstituteTZ})
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	available := map[string][]sessionBrief{}
	unavailable := map[string][]unavailableSessionBrief{}
	for _, f := range facts {
		brief := sessionBrief{ID: f.id, StartAt: f.startAt.UTC().Format(time.RFC3339Nano), EndAt: f.endAt.UTC().Format(time.RFC3339Nano), CourseID: f.courseID, ClassName: f.row.CourseName, CourseName: f.row.CourseName, CourseCode: f.row.CourseCode, SubjectCode: f.row.SubjectCode, SubjectName: f.row.SubjectName, TeacherName: f.row.TeacherName}
		if m, ok := merged[f.id]; ok {
			brief.MergedStartAt = m[0]
			brief.MergedEndAt = m[1]
		}
		if _, isBlocked := blocked[f.id]; isBlocked {
			briefCopy := brief
			briefCopy.Conflict = conflicts[f.id]
			unavailable[f.subjectID] = append(unavailable[f.subjectID], unavailableSessionBrief{Session: &briefCopy, Reason: "This sit-in session is already assigned to this student's absence.", ReasonCode: "sit_in_session_already_used"})
		}
		available[f.subjectID] = append(available[f.subjectID], brief)
	}

	courses := make([]courseJSON, 0, len(order))
	for _, g := range order {
		sessions := make([]sessionJSON, 0, len(g.sessions))
		for _, f := range g.sessions {
			sessions = append(sessions, sessionJSON{ID: f.id, StartAt: f.startAt.UTC().Format(time.RFC3339Nano), EndAt: f.endAt.UTC().Format(time.RFC3339Nano), Date: f.day, AlreadyAbsent: absent[f.id]})
		}
		scope := scopeByCourse[g.courseID]
		mergeID, mergeName := "", ""
		var dayCounts sqldb.AbsenceDayCounts
		if scope != nil {
			mergeName = scope.MergeGroupName
			if scope.Key.MergeGroup {
				mergeID = uuidStringOrZero(scope.Key.MergeGroupID)
			}
			dayCounts = counts[scope.Key.String()]
		}
		stats := absences.NewAbsenceDayLimitStats(dayCounts.TotalCourseDays, dayCounts.UsedAbsenceDays, dayCounts.UsedAbsenceDays)
		courses = append(courses, courseJSON{SubjectID: g.subjectID, SubjectCode: g.subjectCode, SubjectName: g.subjectName, TeacherName: g.teacherName, CourseID: g.courseID, CourseCode: g.courseCode, CourseName: g.courseName, MergeGroupID: mergeID, MergeGroupName: mergeName, Sessions: sessions, SitIn: &courseSitInJSON{SitInMethod: SitInMethodPhysical, AvailableSessions: available[g.subjectID], UnavailableSessions: unavailable[g.subjectID]}, TotalCourseDays: stats.TotalCourseDays, UsedAbsenceDays: stats.UsedAbsenceDays, MaximumAbsenceDays: stats.MaximumAbsenceDays, RemainingAbsenceDays: stats.RemainingAbsenceDays, AbsenceLimitReached: stats.LimitReached})
	}
	if courses == nil {
		courses = []courseJSON{}
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"subjects": courses})
}

// bundleWindowSiblings converts in-window bundle sessions of fact-group
// courses to extra merge siblings (legacy sibling join: window + deleted
// gates only, no enrollment/timing/visibility gates). O(scope sessions).
func bundleWindowSiblings(bundle *sqldb.SitInBundleV2, facts []sessionFact, window sessionsRangeWindow) []sessionFact {
	factGroups := make(map[string]struct{}, len(facts))
	for _, f := range facts {
		if f.row.MergeGroupID.Valid {
			factGroups[uuidStringOrZero(f.row.MergeGroupID)] = struct{}{}
		}
	}
	if len(factGroups) == 0 {
		return nil
	}
	courseGroup := make(map[string]pgtype.UUID, len(bundle.ScopeCourses)+len(bundle.SatMemberCourses))
	for _, c := range bundle.ScopeCourses {
		courseGroup[uuidStringOrZero(c.ID)] = c.MergeGroupID
	}
	for _, c := range bundle.SatMemberCourses {
		k := uuidStringOrZero(c.ID)
		if _, ok := courseGroup[k]; !ok {
			courseGroup[k] = c.MergeGroupID
		}
	}
	var extra []sessionFact
	for courseKey, sessions := range bundle.Sessions {
		group, ok := courseGroup[courseKey]
		if !ok || !group.Valid {
			continue
		}
		if _, ok := factGroups[uuidStringOrZero(group)]; !ok {
			continue
		}
		for _, sn := range sessions {
			if !sn.StartAt.Valid || !sn.EndAt.Valid {
				continue
			}
			t := sn.StartAt.Time
			if t.Before(window.from) || !t.Before(window.toExclusive) {
				continue
			}
			extra = append(extra, sessionFact{id: uuidStringOrZero(sn.ID), startAt: t, endAt: sn.EndAt.Time, row: sqldb.SessionsRangeFactRow{MergeGroupID: group}})
		}
	}
	return extra
}

// mergeSiblingFacts converts MergeSiblingsInRange rows to extra siblings.
func mergeSiblingFacts(rows []sqldb.MergeSiblingRow) []sessionFact {
	extra := make([]sessionFact, 0, len(rows))
	for _, r := range rows {
		if !r.StartAt.Valid || !r.EndAt.Valid {
			continue
		}
		extra = append(extra, sessionFact{id: uuidStringOrZero(r.SessionID), startAt: r.StartAt.Time, endAt: r.EndAt.Time, row: sqldb.SessionsRangeFactRow{MergeGroupID: r.MergeGroupID}})
	}
	return extra
}

// distinctFactGroups returns distinct valid merge-group UUIDs in fact order.
func distinctFactGroups(facts []sessionFact) []pgtype.UUID {
	seen := make(map[string]struct{}, len(facts))
	var out []pgtype.UUID
	for _, f := range facts {
		if !f.row.MergeGroupID.Valid {
			continue
		}
		k := uuidStringOrZero(f.row.MergeGroupID)
		if k == "" || k == "00000000-0000-0000-0000-000000000000" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, f.row.MergeGroupID)
	}
	return out
}

// sessionFactStartAts renders listed session starts like the legacy
// sessionRow.StartAt strings feeding the derived resolve window.
func sessionFactStartAts(sessions []sessionFact) []string {
	starts := make([]string, 0, len(sessions))
	for _, f := range sessions {
		starts = append(starts, f.startAt.UTC().Format(time.RFC3339Nano))
	}
	return starts
}
