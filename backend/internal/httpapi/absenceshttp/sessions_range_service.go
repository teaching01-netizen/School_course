package absenceshttp

// O(1) service orchestrator for sessions-in-range (challenge core).
//
// Query-count contract (Step 16 — statements and round trips, per mode):
//
// Enrolled modes (staff/student), measured v2=2 + 4 batch trips on the
// shadow world (legacy=35; gate asserts v2 < legacy, v2 <= 25,
// full==narrow):
//
// statements (none in a per-course/per-session loop): settings QueryRow
// (1) + bundle (head UNION ALL 2: enrolled+scope+SAT-mappings tag-2 arm,
// satMappingsLoaded flag skips the backfill even at zero rows; trip-A
// mid-batch trip 3: priorities + merge members + SAT members + merge
// names, queued conditionally, drained in order — the standalone
// priorities call was REMOVED (it fired the identical SELECT twice AND
// appended 2N rows; parity pinned by
// TestSessionsRangeBundleMidBatchMatchesStandalone); bundle tail batch
// trip 4: sessions (bounded UNION ALL arm, cutoff pure Go from the
// dispatch settings bytes — no row; DB fallback only when
// PoliciesJSON==nil, never on this path) + rules + visible) +
// scopes+absent+blocked tail batch trip 5 (fold + absent + blocked,
// 3 statements). Skipped when empty/degraded: mid-batch statements (no
// root/merge groups — shadow world queues 3 of 4: priorities skips,
// seed scope carries no root group), SAT members/sessions inputs,
// merge-names probe (no out-of-scope mapped groups), rules (no rule
// IDs — shadow world skips), blocked probe (no courses, missing
// student, or degraded — tail carries 2 then). Shadow world fires 2
// plain queries + 4 batch trips (10 batched statements): head
// facts+student (2), trip-A (3), bundle tail (2), service tail (3).
// The folds remove statements AND trips with identical work: scopes
// and day counts read the same membership rows in one statement (one
// snapshot) instead of two; sessions+rules+visible share one trip;
// facts+student share one trip.
// Batching shares transport only (pgx.Batch, narrow shapes kept — no
// UNION ALL padding): head pair + trip-A quad + bundle-tail triple +
// service-tail triple. Sessions rides the bundle tail (its SAT-member
// inputs come from the EARLIER trip-A batch — sequential batches, not
// one giant join, so no independent-collection multiplication).
// Merging fact groups into a giant join would multiply independent
// collections (forbidden); batching only removes trips.
//
// All-subjects mode: settings (1) + head facts+student batch trip (2
// statements) + siblings (1) + scopes+absent+blocked tail batch trip
// (3 statements).
//
// Failure-mode contract (mirrors legacy exactly):
//   request-fatal (ClassifyDBErr / 500): settings, facts, merged siblings,
//     absent, scopes, day counts, blocked/conflicts (all-subjects only).
//   per-course swallowed (log + nil sit-in, request stays 200): everything
//     inside bundle resolve, via SitInBundleV2.ResolveFailed or per-course
//     resolve errors.

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences"
	sqldb "warwick-institute/internal/db"
)

// sessionsRangeUseV2 gates the O(1) path. Default ON; set
// WARWICK_SESSIONS_RANGE_V2=0 to roll back to the legacy per-course path.
// Both paths preserve identical response/error semantics.
//
// Step 24 (staged rollout): WARWICK_SESSIONS_RANGE_V2 also accepts a
// percentage ("1".."100"): that share of requests (stable per student +
// window, FNV-1a hash) takes V2, the rest takes the CORRECTED legacy path.
// Rollback returns to the corrected legacy path — shared fixes (Steps
// 4/5/7: institute-day, tz-aware eligibility, version-conflict mapping)
// live in code both paths execute, so rollback never restores the
// timezone defects or unsafe writes. "off"/"false"/"0" = 0%, any other
// non-numeric value = 100% (previous default-ON behavior preserved).
func sessionsRangeUseV2() bool {
	return sessionsRangeUseV2For("")
}

// sessionsRangeUseV2ForBucket resolves the staged flag for a dispatch
// bucket key (empty key = tooling default-ON arm).
func sessionsRangeUseV2ForBucket(key string) bool {
	return sessionsRangeUseV2For(key)
}

// requestRolloutKey extracts a stable rollout bucket key for the current
// request. Nil request (flag-only checks) maps to the V2 arm so tooling
// probes see the default-ON behavior.
func requestRolloutKey(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(r.URL.Query().Get("wcode"))) +
		"|" + strings.TrimSpace(r.URL.Query().Get("date_from")) +
		"|" + strings.TrimSpace(r.URL.Query().Get("date_to"))
}

// sessionsRangeUseV2For resolves the flag for one bucket key. Empty key
// always takes V2 (tooling default-ON); otherwise the configured
// percentage of the FNV-1a key space takes V2.
func sessionsRangeUseV2For(key string) bool {
	v, ok := os.LookupEnv("WARWICK_SESSIONS_RANGE_V2")
	if !ok {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "off":
		return false
	case "", "1", "true", "on":
		return true
	}
	pct, err := parseRolloutPercent(v)
	if err != nil {
		return true
	}
	if pct >= 100 {
		return true
	}
	if pct <= 0 || key == "" {
		return pct > 0
	}
	return fnvBucket(key) < uint64(pct)
}

// parseRolloutPercent accepts "N" or "N%" for 0..100.
func parseRolloutPercent(raw string) (int, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "%")
	s = strings.TrimSpace(s)
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 100 {
		return -1, fmt.Errorf("bad rollout percent %q", raw)
	}
	return n, nil
}

// fnvBucket maps a key to 0..99 via FNV-1a-64 (stdlib only, stable
// across processes — every replica routes the same key identically).
func fnvBucket(key string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return h.Sum64() % 100
}

// serveSessionsRangeV2 runs the O(1) sessions-in-range pipeline.
func (s *server) serveSessionsRangeV2(w http.ResponseWriter, r *http.Request, forcedWCode string, requireAdmin bool) {
	totalStart := time.Now()
	pre, ok := parseSessionsRangePrelim(s, w, r, forcedWCode, requireAdmin)
	if !ok {
		return
	}
	settingsStart := time.Now()
	settingsRow, err := s.deps.Q.AppSettingsGetWithPolicies(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	// Step 16: the raw policies row is fetched once here and threaded
	// through validation (parsed) and the enrolled tail below (raw bytes),
	// so the later per-course resolve no longer re-reads app_settings.
	settingsMS := time.Since(settingsStart).Milliseconds()
	settings := parseAbsenceSettings(settingsRow.AbsencePolicies)
	lookup, ok := finalizeSessionsRangeLookup(s, w, r, pre, settings)
	if !ok {
		return
	}
	ctx := r.Context()
	// Step 18: one clock value per request. Captured here and threaded
	// into every domain decision below (timing filter + sit-in cutoffs +
	// SAT RequestTime); nothing downstream in this pipeline calls
	// time.Now for a decision. Legacy parity note: legacy also reads one
	// now per stage, so sub-millisecond clock skew between the two paths
	// can only flip a session exactly at a timing/cutoff boundary — the
	// shadow suite pins agreement away from boundaries.
	now := time.Now()
	wcode := lookup.studentWCode()
	window := pre.window

	// Step-16 facts+student batch: the two post-validation head reads
	// share ONE trip (settings stays standalone above: finalize needs it
	// for the range cap, and the legacy order is pinned by the
	// admin_over_cap/lifetime_range 400 parity cases). Facts failure =
	// request-fatal 500; student failure coerces INSIDE the batch to
	// headStudentMissing (log + ResolveFailed bundle skip, 200) — the
	// batch never returns a student-arm error. The head student row is
	// threaded into the enrolled + all-subjects tails below; no second
	// StudentGetByWCode read.
	var factRows []sqldb.SessionsRangeFactRow
	var headStudent sqldb.StudentGetByWCodeRow
	var headStudentMissing bool
	factsStart := time.Now()
	if lookup.isAllSubjects() {
		allSubj := lookup.(StaffAllSubjectsLookup)
		head, herr := s.deps.Q.SessionsRangeFactsStudentBatch(ctx, sqldb.SessionsRangeFactsParams{SubjectIDs: allSubj.SubjectIDs, FromUTC: window.from, ToExclusiveUTC: window.toExclusive, Mode: sqldb.SessionsRangeFactsAllSubjects, InstituteTZ: s.deps.InstituteTZ}, wcode)
		if herr != nil {
			status, code, msg := s.a.ClassifyDBErr(herr)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		factRows, headStudent, headStudentMissing = head.Facts, head.Student, head.StudentMissing
	} else if lookup.isStudent() {
		head, herr := s.deps.Q.SessionsRangeFactsStudentBatch(ctx, sqldb.SessionsRangeFactsParams{Wcode: wcode, FromUTC: window.from, ToExclusiveUTC: window.toExclusive, Mode: sqldb.SessionsRangeFactsStudent, InstituteTZ: s.deps.InstituteTZ}, wcode)
		if herr != nil {
			status, code, msg := s.a.ClassifyDBErr(herr)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		factRows, headStudent, headStudentMissing = head.Facts, head.Student, head.StudentMissing
	} else {
		head, herr := s.deps.Q.SessionsRangeFactsStudentBatch(ctx, sqldb.SessionsRangeFactsParams{Wcode: wcode, FromUTC: window.from, ToExclusiveUTC: window.toExclusive, Mode: sqldb.SessionsRangeFactsStaff, InstituteTZ: s.deps.InstituteTZ, Lifetime: lookup.isLifetime()}, wcode)
		if herr != nil {
			status, code, msg := s.a.ClassifyDBErr(herr)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		factRows, headStudent, headStudentMissing = head.Facts, head.Student, head.StudentMissing
	}
	factsMS := time.Since(factsStart).Milliseconds()
	domainStart := time.Now()
	preFilter := normalizeSessionFacts(factRows, s.deps.InstituteTZ)
	if len(preFilter) != len(factRows) {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error reading sessions")
		return
	}
	facts := applyTimingAndCourseFilter(preFilter, lookup, settings, now)

	domainMS := time.Since(domainStart).Milliseconds()
	tele := sessionsRangeTelemetry{
		Mode: sessionsRangeMode(lookup.isAllSubjects(), pre.adminRequest),
		ImplVersion: "v2",
		StaffFacing: pre.adminRequest,
		AllSubjects: lookup.isAllSubjects(),
		Lifetime: lookup.isLifetime(),
		BypassTiming: lookup.bypassTiming(),
		SettingsMS: settingsMS,
		FactsMS: factsMS,
		DomainMS: domainMS,
	}
	if lookup.isAllSubjects() {
		s.serveSessionsRangeV2AllSubjectsHead(w, r, lookup.(StaffAllSubjectsLookup), settings, preFilter, facts, window, headStudent, headStudentMissing, now, tele, totalStart)
		return
	}
	s.serveSessionsRangeV2EnrolledHead(w, r, lookup, settings, settingsRow.AbsencePolicies, pre, preFilter, facts, headStudent, headStudentMissing, now, tele, totalStart)
}

// serveSessionsRangeV2EnrolledHead serves staff/student enrolled modes
// with the head-batch student row already drained. studentMissing coerces
// exactly like the standalone StudentGetByWCode error path (log +
// ResolveFailed bundle skip, request stays 200).
func (s *server) serveSessionsRangeV2EnrolledHead(w http.ResponseWriter, r *http.Request, lookup sessionsRangeLookup, settings absenceSettings, policiesBytes []byte, pre sessionsRangePrelim, preFilter, facts []sessionFact, student sqldb.StudentGetByWCodeRow, studentMissing bool, now time.Time, tele sessionsRangeTelemetry, totalStart time.Time) {
	ctx := r.Context()
	wcode := lookup.studentWCode()
	window := pre.window
	adminRequest := pre.adminRequest

	// Legacy resolves the student per course and swallows lookup failures
	// into nil sit-ins (request stays 200 with day counts). A missing student
	// therefore degrades resolution, never the request: with zero facts the
	// legacy loop also yields an empty 200.
	if studentMissing && s.deps.Log != nil {
		s.deps.Log.Error("sessions-range student lookup failed")
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
		// Step 18: the dispatch clock binds the loader cutoff too, so
		// the same request always derives the same candidate ceiling.
		params := sqldb.SitInBundleV2Params{StudentID: student.ID, MissedCourseIDs: distinctFactCourseIDs(facts), PoliciesJSON: policiesBytes, NowUTC: now}
		// Set-3 discovery bound (Step 14): the candidate predicate needs
		// the request window floor and the widest scope cutoff. Policies
		// load later in this function (legacy order: bundle BEFORE
		// policies), so the bound is derived inside the bundle from the
		// same policy rows the resolvers use — see maxDiscoveryCutoffWeeks
		// applied at load time. The window floor/ceiling come from here.
		params.Discovery.WindowFromUTC = window.from
		params.Discovery.WindowToExclUTC = window.toExclusive
		bundle, err = s.deps.Q.SessionsRangeSitInBundleV2(ctx, params)
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

	order := groupFactsByCourse(facts)
	// Step 16 tail batch: scopes + day counts (fold, statement 1), the
	// already-absent probe (statement 2), and the blocked-sit-ins probe
	// (statement 3, queued only with courses + a resolved student +
	// ResolveFailed=false) share ONE round trip via pgx.Batch. Same
	// inputs, same scans — only the transport is shared. A blocked-probe
	// failure degrades exactly like the legacy separate call (log + nil
	// sit-ins, request stays 200); fold/absent failures stay request-fatal.
	// Policies bytes arrive from the dispatch settings row (fetched once
	// for validation) — no second app_settings read; the bytes feed the
	// resolvers directly.
	policies := policiesBytes
	wantBlocked := len(order) > 0 && !bundle.ResolveFailed && !studentMissing
	scopes, counts, absent, blockedRows, err := s.deps.Q.ScopesCountsAbsentBlockedBatch(ctx, wcode, distinctFactCourseIDs(facts), s.deps.InstituteTZ, window.from, window.toExclusive, student.ID, wantBlocked)
	if err != nil {
		// Blocked-only failure (fold+absent drained fine): legacy
		// degrade rule — log + nil sit-ins, request stays 200 with
		// real counts. The error carries the drained data.
		var blockedErr *sqldb.BlockedProbeError
		if errors.As(err, &blockedErr) {
			if s.deps.Log != nil {
				s.deps.Log.Error("sessions-range resolve inputs degraded")
			}
			bundle.ResolveFailed = true
			scopes, counts, absent = blockedErr.Scopes, blockedErr.Counts, blockedErr.Absent
			blockedRows = nil
		} else {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return
		}
	}
	scopeByCourse := scopeRefMap(scopes)
	blocked := make(map[string]struct{}, len(blockedRows))
	conflicts := make(map[string]*sitInSessionConflictInfo, len(blockedRows))
	for _, row := range blockedRows {
		k := uuidStringOrZero(row.SessionID)
		blocked[k] = struct{}{}
		conflicts[k] = sitInConflictInfo(row)
	}
	studentFacing := !adminRequest
	sitInForCourse := func(g *courseGroupView) *courseSitInJSON {
		return s.resolveEnrolledCourseSitInV2(g, bundle, policies, blocked, conflicts, studentFacing, pre, lookup, wcode, student.ID, now)
	}
	courses := assembleCourseResponses(order, scopeByCourse, counts, merged, absent, s.deps.InstituteTZ, sitInForCourse)
	serializeStart := time.Now()
	payload := map[string]any{"subjects": courses}
	rawPayload, _ := json.Marshal(payload)
	tele.PayloadBytes = len(rawPayload)
	tele.Candidates = countSitInCandidates(courses)
	s.a.WriteJSON(w, http.StatusOK, payload)
	tele.SerializeMS = time.Since(serializeStart).Milliseconds()
	tele.TotalMS = time.Since(totalStart).Milliseconds()
	tele.Subjects = len(courses)
	for _, c := range courses {
		tele.Courses++
		tele.Sessions += len(c.Sessions)
	}
	s.emitSessionsRangeTelemetry(r, tele)
}

// resolveEnrolledCourseSitInV2 mirrors the legacy per-course resolve block:
// derived resolve window, swallow-and-log errors, none-gate, len-guarded
// session lists.
func (s *server) resolveEnrolledCourseSitInV2(g *courseGroupView, bundle *sqldb.SitInBundleV2, policies []byte, blocked map[string]struct{}, conflicts map[string]*sitInSessionConflictInfo, studentFacing bool, pre sessionsRangePrelim, lookup sessionsRangeLookup, wcode string, studentID pgtype.UUID, now time.Time) *courseSitInJSON {
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
	result, resolveErr := resolveSitInForCourseFromBundle(bundleSitInInputs{bundle: bundle, policies: policies, blocked: blocked, conflicts: conflicts, instituteTZ: s.deps.InstituteTZ, afterPriority: lookup.satAfterPriority(), studentFacing: studentFacing, now: now}, wcode, studentID, courseID, subjectID, resolveFrom, resolveTo.AddDate(0, 0, 1))
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
func (s *server) serveSessionsRangeV2AllSubjectsHead(w http.ResponseWriter, r *http.Request, lookup StaffAllSubjectsLookup, settings absenceSettings, preFilter, facts []sessionFact, window sessionsRangeWindow, student sqldb.StudentGetByWCodeRow, studentMissing bool, now time.Time, tele sessionsRangeTelemetry, totalStart time.Time) {
	ctx := r.Context()
	wcode := lookup.WCode
	siblingRows, err := s.deps.Q.MergeSiblingsInRange(ctx, distinctFactGroups(facts), window.from, window.toExclusive)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	merged := mergedRangesFromSiblings(facts, append(preFilter, mergeSiblingFacts(siblingRows)...), s.deps.InstituteTZ)
	if len(facts) > 0 && studentMissing {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error checking sit-in session availability")
		return
	}
	order := groupFactsByCourse(facts)
	// Step 16 tail batch (same as enrolled path): fold + absent + blocked
	// share ONE round trip. All-subjects legacy treats a blocked failure
	// as request-fatal (500 "Error checking sit-in session details"),
	// including a BlockedProbeError — no degrade rule here.
	var studentID pgtype.UUID
	if !studentMissing {
		studentID = student.ID
	}
	scopes, counts, absent, blockedRows, err := s.deps.Q.ScopesCountsAbsentBlockedBatch(ctx, wcode, distinctFactCourseIDs(facts), s.deps.InstituteTZ, window.from, window.toExclusive, studentID, len(facts) > 0 && !studentMissing)
	if err != nil {
		var blockedErr *sqldb.BlockedProbeError
		if errors.As(err, &blockedErr) {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Error checking sit-in session details")
			return
		}
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	scopeByCourse := scopeRefMap(scopes)
	blocked := make(map[string]struct{}, len(blockedRows))
	conflicts := make(map[string]*sitInSessionConflictInfo, len(blockedRows))
	for _, row := range blockedRows {
		blocked[uuidStringOrZero(row.SessionID)] = struct{}{}
		conflicts[uuidStringOrZero(row.SessionID)] = sitInConflictInfo(row)
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
	serializeStart := time.Now()
	payload := map[string]any{"subjects": courses}
	rawPayload, _ := json.Marshal(payload)
	tele.PayloadBytes = len(rawPayload)
	tele.Candidates = countSitInCandidates(courses)
	s.a.WriteJSON(w, http.StatusOK, payload)
	tele.SerializeMS = time.Since(serializeStart).Milliseconds()
	tele.TotalMS = time.Since(totalStart).Milliseconds()
	tele.Subjects = len(courses)
	for _, c := range courses {
		tele.Courses++
		tele.Sessions += len(c.Sessions)
	}
	s.emitSessionsRangeTelemetry(r, tele)
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
