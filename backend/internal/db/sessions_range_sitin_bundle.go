package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SessionsRangeSitInBundleV2 loads every sit-in rule input in a FIXED number
// of round trips independent of course count (Step 16: statements == trips
// is now literal — pgx.Batch shares one trip for the tail pair, and the
// countingTracer only implements QueryTracer so batches do not inflate the
// gate; see the snapshot-strategy note in sessions_range_service.go):
//
//  1. bundleEnrolled+Scope: enrollments AND scope courses, UNION ALL (1 trip)
//  2. bundlePriorities: priorities for distinct root groups via ANY (1 trip)
//  3. bundleSatData: active SAT mappings + missing merge names (<= 2 trips)
//  4. merge members: group members for scope + mapped groups (1 trip)
//  5. SAT members: full course rows for out-of-scope mapped members (1 trip)
//  6. widest cutoff: derived in Go from PoliciesJSON (0 trips; falls back
//     to one app_settings row only when PoliciesJSON is nil, i.e. legacy
//     test callers that never fetched settings)
//  7. bundleSessions: missed-history + candidates, UNION ALL (1 trip)
//  8. rules+visible: sit-in rules UNION ALL + visibility probe (1 trip,
//     pgx.Batch — two statements, one network round trip)
//
// Worst case: 8 trips, constant in courses. Combined with facts (1),
// absent (1), scopes (1), day counts (1), student row (1), and the handler
// settings row (1), the enrolled endpoint stays at a small constant
// (measured v2=12; legacy is 35 on the same world). The invariant under
// test is O(1) in courses, asserted by the query-count gate (full==narrow).
//
// Session-set responsibilities (Step 14 — adding unrelated history must not
// grow a request):
//
//  1. Requested display sessions: SessionsRangeFacts (window-bounded, the
//     rendered rows). NOT loaded here.
//  2. Relevant merged display siblings: derived from the fact window
//     (bundleWindowSiblings) or MergeSiblingsInRange (all-subjects mode).
//     NOT loaded here.
//  3. Eligible sit-in candidate discovery: THIS bundle. Missed-history
//     sessions (MissedCount inputs, occurrence slots) load window-bounded
//     [WindowFromUTC, WindowToExclUTC); candidate sessions load
//     start_at >= WindowFromUTC AND (no cutoff OR start_at <= CutoffUTC),
//     where CutoffUTC = now + widest sit_in_window_weeks in the request.
//     Rule scope comes from the actual scope universe (root/merge sibling
//     courses + missed courses + out-of-scope SAT mapped members — never a
//     guessed lookback, never a LIMIT). Dedupe happens before fetch via the
//     shared course-ID list (one ANY array, overlapping scopes fetched once).
//  4. Historical counters: SessionsRangeDayCounts (all-history by
//     definition). NOT loaded here.
//  5. Scope/rule metadata: sets 1-5 above (rule pools, mappings, members,
//     visibility). Window-independent catalog facts, fetched by ID.
type SitInBundleV2Params struct {
	StudentID       pgtype.UUID
	MissedCourseIDs []pgtype.UUID
	// PoliciesJSON carries the request's app_settings absence_policies
	// bytes (already fetched by the caller for validation). The loader
	// derives the set-3 widest scope cutoff from these SAME bytes the
	// resolvers use — no second app_settings read. Nil preserves the old
	// behavior: the loader falls back to one policies-row read.
	PoliciesJSON []byte
	// Discovery bounds set-3 candidate sessions to the request window +
	// widest scope cutoff. Zero value preserves the legacy unbounded load
	// (non-range callers, degraded paths); range callers pass the window.
	// Step 18 clock: the cutoff is now + widest sit_in_window_weeks, so
	// callers that need request-clock determinism pass NowUTC (captured
	// once at dispatch). Zero NowUTC keeps the old time.Now behavior
	// (non-range callers that never set Discovery bounds).
	Discovery SitInDiscoveryBounds
	NowUTC    time.Time
}

// SitInBundleV2 is the preloaded rule-input universe.
//
// ResolveFailed mirrors the old per-course error swallow: every query inside
// the old resolveSitInForCourse degraded to a nil sit-in for the course, and
// a transport-level failure fails all courses equally. So any bundle loader
// failure (except priorities, which the old path degrades to single-rule
// resolution) sets ResolveFailed and the service returns nil sit-ins with a
// 200 instead of failing the request. Priorities failures degrade silently
// with no flag, exactly like `if pErr == nil && len(priorities) > 0`.
type SitInBundleV2 struct {
	ResolveFailed bool
	Enrolled      []BundleEnrolledCourse
	ScopeCourses  []SubjectCourseV2
	Priorities    []SitInPriorityWithRule
	RulesByID     map[string]*SitInRule
	RulesByRoot   map[string]*SitInRule
	SatMappings   []SatVerbalPolicyCourseMapping

	SatMapByCourse map[string]*SatVerbalPolicyCourseMapping
	// SatMemberCourses holds full course rows for mapped merge-group members
	// that fall outside the student scope universe (legacy resolves them via
	// CourseSubjectByID regardless of enrollment). Never merged into
	// ScopeCourses: scope membership gates rule pools, mapping rows must not.
	SatMemberCourses []SubjectCourseV2
	MergeNames       map[string]string
	MergeMembers     map[string][]pgtype.UUID
	Visible          map[string]struct{}
	Sessions         map[string][]SessionInRange
}

// SitInDiscoveryBounds carries the instant bounds for candidate discovery
// (set 3 below). Window instants are the half-open request window
// [FromUTC, ToExclusiveUTC); Cutoff bounds the future sit-in search per the
// scope window-weeks policy. Zero CutoffUTC at load time means "no policy
// anywhere": the loader clamps to WindowToExclUTC (nothing beyond the
// request window is ever offered without a make-up window — verified
// against the resolver filter chain, see loadBundleSessionsBounded).
type SitInDiscoveryBounds struct {
	WindowFromUTC    time.Time
	WindowToExclUTC  time.Time
	CutoffUTC        time.Time
	IncludeUnbounded bool
}

// SessionsRangeSitInBundleV2 loads the universe. See contract above.
// loadBundleSessionsRulesVisibleTail runs the post-sessions tail:
// sessions (bounded by Discovery; needs SAT-member IDs) + rules + visible
// (need enrolled+scope+SAT-member IDs) in ONE trip via pgx.Batch, followed
// by the pure-Go cutoff derivation ONLY when the sessions arm actually
// needs it. Queue order is sessions, rules, visibility; the drain scans in
// that order (pgx poison semantics: each statement fully scanned before
// the next is touched). Failure contract mirrors the standalone sequence
// exactly: a sessions statement/scan failure sets ResolveFailed (like the
// legacy per-course swallow failing the whole resolve); the cutoff stays
// zero-clamped and the rules+visible arms still drain (poison discipline)
// before returning; a rules failure sets ResolveFailed (request stays 200
// with nil sit-ins); a visibility failure degrades to the all-visible
// default; a batch-transport/Close failure marks ResolveFailed and keeps
// the all-visible default. Conditional queueing: sessions skips at zero
// courses, rules skips at zero rule/root IDs, visibility skips at zero
// probe IDs — the drain tracks flags, same pattern as the tail batch's
// queueBlocked.
func (q *Queries) loadBundleSessionsRulesVisibleTail(ctx context.Context, out *SitInBundleV2, discovery SitInDiscoveryBounds) error {
	ids := bundleSessionCourseIDs(out)
	sessQueued := len(ids) > 0
	var sessSQL string
	var sessArgs []any
	if sessQueued {
		if discovery.IncludeUnbounded || discovery.WindowFromUTC.IsZero() || discovery.WindowToExclUTC.IsZero() {
			sessSQL = bundleSessionsSelectSQL()
			sessArgs = []any{ids}
		} else {
			cutoff := pgtype.Timestamptz{}
			if !discovery.CutoffUTC.IsZero() {
				cutoff = pgtype.Timestamptz{Time: discovery.CutoffUTC, Valid: true}
			}
			sessSQL = `SELECT 0 AS arm, id, course_id, room_id, start_at, end_at
		FROM sessions
		WHERE course_id = ANY($1::uuid[])
		  AND deleted_at IS NULL
		  AND start_at >= $2::timestamptz
		  AND start_at < $3::timestamptz
		UNION ALL
		SELECT 1 AS arm, id, course_id, room_id, start_at, end_at
		FROM sessions
		WHERE course_id = ANY($4::uuid[])
		  AND deleted_at IS NULL
		  AND start_at >= $2::timestamptz
		  AND ($5::timestamptz IS NULL OR start_at <= $5::timestamptz)
		ORDER BY course_id, start_at ASC`
			sessArgs = []any{ids, pgtype.Timestamptz{Time: discovery.WindowFromUTC, Valid: true}, pgtype.Timestamptz{Time: discovery.WindowToExclUTC, Valid: true}, ids, cutoff}
		}
	}
	var b pgx.Batch
	if sessQueued {
		b.Queue(sessSQL, sessArgs...)
	}
	_, _, rulesQueued := q.loadBundleRulesQuery(ctx, out, &b)
	visIDs := bundleVisibleCourseIDs(out.ScopeCourses, out.SatMemberCourses)
	visQueued := len(visIDs) > 0
	if visQueued {
		b.Queue(bundleVisibleSelectSQL(), visIDs)
	}
	if !sessQueued && !rulesQueued && !visQueued {
		return nil
	}
	br := q.db.(interface {
		SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
	}).SendBatch(ctx, &b)
	defer br.Close()
	var firstErr error
	if sessQueued {
		rows, err := br.Query()
		if err != nil {
			firstErr = err
		} else if err := scanBundleSessionsResult(rows, out); err != nil {
			firstErr = err
		}
	}
	if rulesQueued {
		rows, err := br.Query()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else if err := scanBundleRulesResult(rows, out); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if visQueued {
		rows, err := br.Query()
		if err != nil {
			// Visibility degrades to the all-visible default set above.
		} else {
			out.Visible = scanBundleVisibleResult(rows, visIDs)
		}
	}
	if cerr := br.Close(); cerr != nil && firstErr == nil {
		firstErr = cerr
	}
	return firstErr
}

// scanBundleSessionsResult drains one sessions result set into out.
// The bounded shape carries an arm tag column; the unbounded shape does
// not — the tag is detected by field count via pgx.Rows.FieldDescriptions.
func scanBundleSessionsResult(rows pgx.Rows, out *SitInBundleV2) error {
	defer rows.Close()
	armed := len(rows.FieldDescriptions()) == 6
	for rows.Next() {
		if armed {
			var arm int
			var r SessionInRange
			if err := rows.Scan(&arm, &r.ID, &r.CourseID, &r.RoomID, &r.StartAt, &r.EndAt); err != nil {
				return err
			}
			out.Sessions[uuidBytesString(r.CourseID)] = append(out.Sessions[uuidBytesString(r.CourseID)], r)
		} else {
			var r SessionInRange
			if err := rows.Scan(&r.ID, &r.CourseID, &r.RoomID, &r.StartAt, &r.EndAt); err != nil {
				return err
			}
			out.Sessions[uuidBytesString(r.CourseID)] = append(out.Sessions[uuidBytesString(r.CourseID)], r)
		}
	}
	return rows.Err()
}

func (q *Queries) SessionsRangeSitInBundleV2(ctx context.Context, arg SitInBundleV2Params) (*SitInBundleV2, error) {
	out := &SitInBundleV2{
		RulesByID:      make(map[string]*SitInRule),
		RulesByRoot:    make(map[string]*SitInRule),
		SatMapByCourse: make(map[string]*SatVerbalPolicyCourseMapping),
		MergeNames:     make(map[string]string),
		MergeMembers:   make(map[string][]pgtype.UUID),
		Visible:        make(map[string]struct{}),
		Sessions:       make(map[string][]SessionInRange),
	}
	bundle := &SitInBundleFacts{
		MergeNames:       make(map[string]string),
		VisibleCourseIDs: make(map[string]struct{}),
		SessionsByCourse: make(map[string][]SessionInRange),
	}
	// Reuse the tested single-purpose loaders; each is count-constant.
	// Any failure (except priorities) marks ResolveFailed and continues
	// with whatever loaded, mirroring the old per-course swallow.
	// Enrolled + scope courses load in ONE round trip (UNION ALL).
	if err := q.loadBundleEnrolledAndScope(ctx, SitInBundleFactsParams{StudentID: arg.StudentID, MissedCourseIDs: arg.MissedCourseIDs}, bundle); err != nil {
		out.ResolveFailed = true
	}
	// Priorities load ONLY via the trip-A mid-batch below (same SQL,
	// derivation, and scan as the standalone loader — see
	// TestSessionsRangeBundleMidBatchMatchesStandalone). A standalone
	// call here would fire the identical SELECT twice per request AND
	// append 2N rows (both drains append); the mid-batch priorities-only
	// failure degrades to nil identically (legacy single-rule fallback).
	if err := q.loadBundleSatMappings(ctx, bundle); err != nil {
		out.ResolveFailed = true
	}
	out.Enrolled = bundle.Enrolled
	out.ScopeCourses = bundle.ScopeCourses
	out.Priorities = bundle.Priorities
	out.SatMappings = bundle.SatMappings
	out.MergeNames = bundle.MergeNames
	out.Visible = bundle.VisibleCourseIDs
	out.Sessions = bundle.SessionsByCourse
	for i := range out.SatMappings {
		m := &out.SatMappings[i]
		out.SatMapByCourse[uuidBytesString(m.CourseID)] = m
	}
	// Step-16 trip-A mid-batch: priorities + merge members + SAT members
	// + merge names share ONE round trip via pgx.Batch. All four are
	// independent given the head UNION ALL output (root groups from
	// ScopeCourses; merge groups from scope+SatMappings; mapping groups +
	// scope have-set; missing merge-name IDs from scope MergeNames +
	// SatMappings) — none reads another's rows. Sessions canNOT join
	// this batch: it needs SAT-member IDs produced by the SAT-members
	// query. Rules+visible stays last (needs enrolled+scope+SAT-member
	// IDs).
	//
	// Drain discipline (pgx poison semantics: a failed statement poisons
	// LATER drains, so each statement is fully scanned before the next is
	// touched, in queue order): priorities failure degrades to nil WITHOUT
	// ResolveFailed (legacy single-rule fallback); merge/SAT-member
	// failures set ResolveFailed (legacy per-course swallow failed the
	// whole resolve). Conditional queueing: each loader skips on empty
	// input, so the drain tracks which statements were queued.
	// loadBundleMidBatch error contract: priorities-only failure is
	// SWALLOWED inside (PrioritiesFailed set, Priorities nilled, nil
	// error — legacy single-rule fallback); merge/SAT-member statement
	// failures AND any transport failure return a non-nil error so the
	// caller sets ResolveFailed. The PrioritiesFailed re-check below is
	// belt-and-braces for a both-failed batch (mid-batch error + stale
	// priorities rows must not leak into the resolve).
	if err := q.loadBundleMidBatch(ctx, bundle, out); err != nil {
		out.ResolveFailed = true
	}
	if bundle.PrioritiesFailed {
		bundle.Priorities = nil
	}
	out.Priorities = bundle.Priorities
	// One sessions round trip for scope courses AND out-of-scope SAT mapped
	// members (legacy SessionsByCourse per target, unbounded). Bounded when
	// the caller passes Discovery window bounds (set-3 proportional to the
	// request window + widest scope cutoff, derived below from the SAME
	// policy rows the resolvers use); unbounded legacy shape otherwise.
	// The cutoff needs the scope universe (loaded above) + policies; the
	// policies row is the one extra round trip the bounded shape costs.
	// Failure here degrades exactly like the legacy per-course resolve
	// (ResolveFailed -> nil sit-ins, request stays 200), never a 500: the
	// legacy path also swallowed settings failures per course.
	// Mega-tail (Step 16): sessions + rules + visibility share ONE
	// trip (see loadBundleSessionsRulesVisibleTail). The cutoff is pure
	// Go from the dispatch policies bytes (zero-trip); the DB fallback
	// fires ONLY when PoliciesJSON==nil (never on the range path — the
	// service always threads the dispatch row bytes).
	discovery := arg.Discovery
	if !discovery.IncludeUnbounded && !discovery.WindowFromUTC.IsZero() && !discovery.WindowToExclUTC.IsZero() && discovery.CutoffUTC.IsZero() {
		discovery.CutoffUTC = widestScopeCutoffFromPoliciesAt(arg.PoliciesJSON, out, arg.NowUTC)
		// No policy anywhere in this request: zero cutoff would leave
		// candidates unbounded-future (legacy shape, history leaks back
		// in). Clamp to the request ceiling instead — the resolvers use
		// zero cutoff to mean "no future filtering", but nothing beyond
		// the request window is ever offered without a make-up window,
		// and missed-history never extends past it either. Explicit,
		// documented, and shadow-pinned (not a guessed LIMIT).
		if discovery.CutoffUTC.IsZero() {
			if arg.PoliciesJSON == nil {
				discovery.CutoffUTC = q.loadWidestScopeCutoff(ctx, out)
			}
			if discovery.CutoffUTC.IsZero() {
				discovery.CutoffUTC = discovery.WindowToExclUTC
			}
		}
	}
	out.Visible = arrayToVisibleSet(bundleVisibleCourseIDs(out.ScopeCourses, out.SatMemberCourses))
	if err := q.loadBundleSessionsRulesVisibleTail(ctx, out, discovery); err != nil {
		out.ResolveFailed = true
	}
	return out, nil
}

// arrayToVisibleSet builds the all-visible default (and the empty-universe
// vacuous set) from the probe ID list.
func arrayToVisibleSet(ids []string) map[string]struct{} {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

// loadBundleRulesAndVisible drains the rules + visibility batch.
func (q *Queries) loadBundleRulesAndVisible(ctx context.Context, out *SitInBundleV2) error {
	var b pgx.Batch
	_, _, rulesQueued := q.loadBundleRulesQuery(ctx, out, &b)
	visIDs := bundleVisibleCourseIDs(out.ScopeCourses, out.SatMemberCourses)
	visQueued := len(visIDs) > 0
	if visQueued {
		b.Queue(bundleVisibleSelectSQL(), visIDs)
	}
	if !rulesQueued && !visQueued {
		return nil
	}
	br := q.db.(interface {
		SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
	}).SendBatch(ctx, &b)
	defer br.Close()
	if rulesQueued {
		rows, err := br.Query()
		if err != nil {
			return err
		}
		if err := scanBundleRulesResult(rows, out); err != nil {
			return err
		}
	}
	if visQueued {
		rows, err := br.Query()
		if err != nil {
			// Visibility degrades to the all-visible default set above.
			return nil
		}
		out.Visible = scanBundleVisibleResult(rows, visIDs)
	}
	return nil
}

// loadBundleMidBatch runs the trip-A mid-batch: priorities + merge members
// + SAT members in ONE round trip via pgx.Batch. Inputs are derived from
// the head UNION ALL output (scope courses + SAT mappings) exactly like
// the standalone loaders; queue order is priorities, merge members, SAT
// members, and the drain scans in that order (pgx poison semantics: each
// statement fully scanned before the next is touched).
//
// Failure contract (mirrors the standalone calls exactly): a
// priorities-only statement/scan failure nils bundle.Priorities, sets
// bundle.PrioritiesFailed, and DRAINS AND RETURNS the remaining queued
// statements' errors (never swallowed — pgx poison semantics mean a
// skipped br.Query() would surface as a LATER statement's transport
// error, misattributing the failure; every queued statement is drained
// in order, exactly like the tail batch). Merge/SAT statement, scan,
// transport, and Close failures return non-nil (ResolveFailed).
// Conditional queueing: empty-input statements are not queued (same skip
// as the standalone loaders); the drain tracks flags.
func (q *Queries) loadBundleMidBatch(ctx context.Context, bundle *SitInBundleFacts, out *SitInBundleV2) error {
	var b pgx.Batch
	priGroups := bundlePriorityRootGroups(bundle.ScopeCourses)
	priQueued := len(priGroups) > 0
	if priQueued {
		b.Queue(bundlePrioritiesSQL(), priGroups)
	}
	mergeGroups := bundleMergeMemberGroups(out.ScopeCourses, out.SatMappings)
	mergeQueued := len(mergeGroups) > 0
	if mergeQueued {
		b.Queue(bundleMergeMembersSQL(), mergeGroups)
	}
	satGroups, satHave := bundleSatMemberGroups(out.ScopeCourses, out.SatMappings)
	satQueued := len(satGroups) > 0
	if satQueued {
		b.Queue(loadBundleSatMembersSQL(), satGroups)
	}
	// Merge-names probe (fused from loadBundleSatMappings): names for
	// SAT-mapped merge groups missing from the scope universe. Inputs
	// need only scope+SatMappings — available pre-batch like the other
	// arms. Queued LAST so the established pri/merge/sat drain order is
	// untouched.
	nameIDs := bundleMissingMergeNameIDs(out)
	namesQueued := len(nameIDs) > 0
	if namesQueued {
		b.Queue(bundleMergeNamesSQL(), nameIDs)
	}
	if !priQueued && !mergeQueued && !satQueued && !namesQueued {
		return nil
	}
	br := q.db.(interface {
		SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
	}).SendBatch(ctx, &b)
	defer br.Close()
	// Every queued statement is drained IN ORDER even after a
	// priorities failure: skipping a drain would poison later drains
	// (pgx br.err stickiness) and misattribute the failure. A
	// priorities-only failure is swallowed at the end (legacy
	// single-rule fallback); any merge/SAT/transport/Close failure is
	// returned (ResolveFailed). PrioritiesFailed stays set alongside a
	// returned error so the orchestrator nils stale rows regardless.
	if priQueued {
		rows, err := br.Query()
		if err != nil {
			bundle.Priorities = nil
			bundle.PrioritiesFailed = true
		} else if err := scanBundlePriorities(rows, bundle); err != nil {
			bundle.Priorities = nil
			bundle.PrioritiesFailed = true
		}
	}
	if mergeQueued {
		rows, err := br.Query()
		if err != nil {
			return err
		}
		if err := scanBundleMergeMembers(rows, out); err != nil {
			return err
		}
	}
	if satQueued {
		rows, err := br.Query()
		if err != nil {
			return err
		}
		if err := scanBundleSatMembers(rows, out, satHave); err != nil {
			return err
		}
	}
	if namesQueued {
		rows, err := br.Query()
		if err != nil {
			return err
		}
		if err := scanBundleMergeNames(rows, out); err != nil {
			return err
		}
	}
	if err := br.Close(); err != nil {
		return err
	}
	return nil
}

// bundlePrioritiesSQL is the priorities SELECT shared by the standalone
// loader and the trip-A mid-batch (same text, same params, same order).
func bundlePrioritiesSQL() string {
	return `
		SELECT
			p.id, p.root_course_group_id, p.sit_in_rule_id, p.priority_level, p.label, p.target_rank, p.target_section, p.created_at,
			r.name AS rule_name, r.type AS rule_type, r.predicate AS rule_predicate
		FROM sit_in_priorities p
		JOIN sit_in_rules r ON r.id = p.sit_in_rule_id
		WHERE p.root_course_group_id = ANY($1::uuid[])
		ORDER BY p.root_course_group_id, p.priority_level ASC
	`
}

// bundlePriorityRootGroups derives the distinct root-group input for the
// priorities query from scope courses (same derivation as the standalone
// loader: skip invalid, dedupe by bytes).
func bundlePriorityRootGroups(scopeCourses []SubjectCourseV2) []pgtype.UUID {
	groups := make([]pgtype.UUID, 0, 4)
	seen := make(map[string]struct{})
	for _, c := range scopeCourses {
		if !c.RootCourseGroupID.Valid {
			continue
		}
		key := uuidBytesString(c.RootCourseGroupID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		groups = append(groups, c.RootCourseGroupID)
	}
	return groups
}

// scanBundlePriorities drains one priorities result set into bundle.
func scanBundlePriorities(rows pgx.Rows, bundle *SitInBundleFacts) error {
	defer rows.Close()
	for rows.Next() {
		var p SitInPriorityWithRule
		if err := rows.Scan(
			&p.ID, &p.RootCourseGroupID, &p.SitInRuleID, &p.PriorityLevel, &p.Label, &p.TargetRank, &p.TargetSection, &p.CreatedAt,
			&p.RuleName, &p.RuleType, &p.RulePredicate,
		); err != nil {
			return err
		}
		bundle.Priorities = append(bundle.Priorities, p)
	}
	return rows.Err()
}

// bundleMergeMembersSQL is the merge-members SELECT shared by the
// standalone loader and the trip-A mid-batch.
func bundleMergeMembersSQL() string {
	return "SELECT group_id, course_id FROM course_merge_group_members WHERE group_id = ANY($1::uuid[]) ORDER BY group_id, position ASC"
}

// bundleMergeMemberGroups derives the distinct merge-group input from
// scope courses + SAT mappings (same derivation as the standalone loader).
func bundleMergeMemberGroups(scopeCourses []SubjectCourseV2, satMappings []SatVerbalPolicyCourseMapping) []pgtype.UUID {
	groups := make([]pgtype.UUID, 0)
	seen := make(map[string]struct{})
	add := func(id pgtype.UUID) {
		if !id.Valid {
			return
		}
		k := uuidBytesString(id)
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		groups = append(groups, id)
	}
	for _, c := range scopeCourses {
		add(c.MergeGroupID)
	}
	for i := range satMappings {
		add(satMappings[i].MergeGroupID)
	}
	return groups
}

// scanBundleMergeMembers drains one merge-members result set into out.
func scanBundleMergeMembers(rows pgx.Rows, out *SitInBundleV2) error {
	defer rows.Close()
	for rows.Next() {
		var gid, cid pgtype.UUID
		if err := rows.Scan(&gid, &cid); err != nil {
			return err
		}
		k := uuidBytesString(gid)
		out.MergeMembers[k] = append(out.MergeMembers[k], cid)
	}
	return rows.Err()
}

// bundleSatMemberGroups derives the SAT-members input: distinct mapping
// merge groups plus the scope have-set for in-universe filtering (same
// derivation as the standalone loader).
func bundleSatMemberGroups(scopeCourses []SubjectCourseV2, satMappings []SatVerbalPolicyCourseMapping) ([]pgtype.UUID, map[string]struct{}) {
	groups := make([]pgtype.UUID, 0)
	seen := make(map[string]struct{})
	have := make(map[string]struct{}, len(scopeCourses))
	for _, c := range scopeCourses {
		have[uuidBytesString(c.ID)] = struct{}{}
	}
	for i := range satMappings {
		m := &satMappings[i]
		if !m.MergeGroupID.Valid {
			continue
		}
		k := uuidBytesString(m.MergeGroupID)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		groups = append(groups, m.MergeGroupID)
	}
	return groups, have
}

// bundleMergeNamesSQL is the merge-names SELECT shared by the
// loadBundleSatMappings probe and the trip-A mid-batch (same text,
// same params, same order).
func bundleMergeNamesSQL() string {
	return `SELECT id, name FROM course_merge_groups WHERE id = ANY($1::uuid[])`
}

// bundleMissingMergeNameIDs derives the merge-names probe input:
// SAT-mapped merge groups absent from the scope-universe MergeNames
// (same derivation as loadBundleSatMappings).
func bundleMissingMergeNameIDs(out *SitInBundleV2) []pgtype.UUID {
	missing := make([]pgtype.UUID, 0)
	for _, m := range out.SatMappings {
		if m.MergeGroupID.Valid {
			if _, ok := out.MergeNames[uuidBytesString(m.MergeGroupID)]; !ok {
				missing = append(missing, m.MergeGroupID)
			}
		}
	}
	return missing
}

// scanBundleMergeNames drains one merge-names result set into out.
func scanBundleMergeNames(rows pgx.Rows, out *SitInBundleV2) error {
	defer rows.Close()
	for rows.Next() {
		var id pgtype.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		out.MergeNames[uuidBytesString(id)] = name
	}
	return rows.Err()
}

// scanBundleSatMembers drains one SAT-members result set into out,
// merging only out-of-scope rows (same filter as the standalone loader).
func scanBundleSatMembers(rows pgx.Rows, out *SitInBundleV2, have map[string]struct{}) error {
	defer rows.Close()
	for rows.Next() {
		var r SubjectCourseV2
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.SubjectID, &r.SubjectCode, &r.SubjectName, &r.CycleID, &r.Level, &r.RootCourseGroupID, &r.SitInRuleID, &r.MergeGroupID); err != nil {
			return err
		}
		if _, ok := have[uuidBytesString(r.ID)]; ok {
			continue
		}
		have[uuidBytesString(r.ID)] = struct{}{}
		out.SatMemberCourses = append(out.SatMemberCourses, r)
	}
	return rows.Err()
}

func (q *Queries) loadBundleMergeMembers(ctx context.Context, out *SitInBundleV2) error {
	// Standalone path shares SQL text, input derivation, and row scan with
	// the trip-A mid-batch so both execute identical work.
	groups := bundleMergeMemberGroups(out.ScopeCourses, out.SatMappings)
	if len(groups) == 0 {
		return nil
	}
	rows, err := q.db.Query(ctx, bundleMergeMembersSQL(), groups)
	if err != nil {
		return err
	}
	return scanBundleMergeMembers(rows, out)
}

// loadBundleSatMembers batch-loads full course rows for mapped merge-group
// members outside the student scope universe. Legacy satVerbalMappedCourses
// resolves members via CourseSubjectByID with no enrollment gate, so every
// mapped member must resolve even when the student never enrolled near it.
// One query for all mapped groups; rows merge into SatMemberCourses only.
func (q *Queries) loadBundleSatMembers(ctx context.Context, out *SitInBundleV2) error {
	// Standalone path shares input derivation and row scan with the
	// trip-A mid-batch so both execute identical work.
	groups, have := bundleSatMemberGroups(out.ScopeCourses, out.SatMappings)
	if len(groups) == 0 {
		return nil
	}
	rows, err := q.db.Query(ctx, loadBundleSatMembersSQL(), groups)
	if err != nil {
		return err
	}
	return scanBundleSatMembers(rows, out, have)
}

const bundleSessionsSelectSQLText = `SELECT id, course_id, room_id, start_at, end_at FROM sessions WHERE course_id = ANY($1::uuid[]) AND deleted_at IS NULL ORDER BY course_id, start_at ASC`

func bundleSessionsSelectSQL() string {
	return bundleSessionsSelectSQLText
}

func loadBundleSatMembersSQL() string {
	return `SELECT DISTINCT c.id, c.code, c.name, c.subject_id, COALESCE(sub.code, ''), COALESCE(sub.name, ''), ` +
		`c.cycle_id, COALESCE(mgg.level, c.level), c.root_course_group_id, ` +
		`COALESCE(mgg.sit_in_rule_id, rcg.sit_in_rule_id), mgm.group_id ` +
		`FROM courses c ` +
		`LEFT JOIN subjects sub ON sub.id = c.subject_id ` +
		`LEFT JOIN root_course_groups rcg ON rcg.id = c.root_course_group_id ` +
		`LEFT JOIN course_merge_group_members mgm ON mgm.course_id = c.id ` +
		`LEFT JOIN course_merge_groups mgg ON mgg.id = mgm.group_id ` +
		`JOIN course_merge_group_members f ON f.course_id = c.id AND f.group_id = ANY($1::uuid[])`
}

// loadBundleSessionsAll is the legacy unbounded entry point, kept for
// callers without a request window (tests, non-range paths). Range callers
// go through loadBundleSessionsBounded with explicit Discovery bounds.
// Do not add LIMIT (silent truncation breaks sit-in options and final-day
// derivation); bound by course or by the window+cutoff predicate instead.
func (q *Queries) loadBundleSessionsAll(ctx context.Context, out *SitInBundleV2) error {
	return q.loadBundleSessionsUnbounded(ctx, out, nil)
}

// loadBundleSessionsBounded implements set 3: missed-history sessions
// (start_at in [WindowFromUTC, WindowToExclUTC), the legacy
// SessionsByCourseInRange predicate, so MissedCount matches the old
// per-course lookup exactly) UNION ALL candidate sessions (start_at >=
// WindowFromUTC AND (no CutoffUTC OR start_at <= CutoffUTC) — the upper
// display bound is dropped because candidates beyond the window are
// legitimate inside the make-up window; the lower bound holds because
// nothing starting before the window opens is ever offered; the cutoff
// holds because nothing past the make-up window is offered). One round
// trip over the deduped course universe. Zero/empty bounds fall back to
// the unbounded shape (legacy callers, degraded paths).
func (q *Queries) loadBundleSessionsBounded(ctx context.Context, out *SitInBundleV2, bounds SitInDiscoveryBounds) error {
	ids := bundleSessionCourseIDs(out)
	if len(ids) == 0 {
		return nil
	}
	if bounds.IncludeUnbounded || bounds.WindowFromUTC.IsZero() || bounds.WindowToExclUTC.IsZero() {
		return q.loadBundleSessionsUnbounded(ctx, out, ids)
	}
	// Set-1 (missed history) covers the same course universe as set-3: the
	// missed course for any resolve below is always a scope/SAT-member
	// course, and legacy loaded its history through the identical
	// SessionsByCourseInRange predicate. One shared ID list, one trip.
	missedIDs := ids
	cutoff := pgtype.Timestamptz{}
	if !bounds.CutoffUTC.IsZero() {
		cutoff = pgtype.Timestamptz{Time: bounds.CutoffUTC, Valid: true}
	}
	rows, err := q.db.Query(ctx, `
		SELECT 0 AS arm, id, course_id, room_id, start_at, end_at
		FROM sessions
		WHERE course_id = ANY($1::uuid[])
		  AND deleted_at IS NULL
		  AND start_at >= $2::timestamptz
		  AND start_at < $3::timestamptz
		UNION ALL
		SELECT 1 AS arm, id, course_id, room_id, start_at, end_at
		FROM sessions
		WHERE course_id = ANY($4::uuid[])
		  AND deleted_at IS NULL
		  AND start_at >= $2::timestamptz
		  AND ($5::timestamptz IS NULL OR start_at <= $5::timestamptz)
		ORDER BY course_id, start_at ASC
	`, missedIDs, pgtype.Timestamptz{Time: bounds.WindowFromUTC, Valid: true}, pgtype.Timestamptz{Time: bounds.WindowToExclUTC, Valid: true}, ids, cutoff)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var arm int
		var r SessionInRange
		if err := rows.Scan(&arm, &r.ID, &r.CourseID, &r.RoomID, &r.StartAt, &r.EndAt); err != nil {
			return err
		}
		out.Sessions[uuidBytesString(r.CourseID)] = append(out.Sessions[uuidBytesString(r.CourseID)], r)
	}
	return rows.Err()
}

// loadWidestScopeCutoff returns now + the widest sit_in_window_weeks across
// every scope universe the resolvers below consult: merge-group policies for
// every distinct merge group in the scope universe, root-group policies for
// every distinct root group, and subject-level policies for every distinct
// SAT-mapped subject (SAT windows are subject-scoped with merge override).
// Zero time = no policy anywhere: candidates stay unbounded-future, exactly
// like the legacy per-target lookup. A settings failure returns zero time
// (the resolvers degrade the same way: win=0 -> zero cutoff -> no filtering).
func (q *Queries) loadWidestScopeCutoff(ctx context.Context, out *SitInBundleV2) time.Time {
	settings, err := q.AppSettingsGetWithPolicies(ctx)
	if err != nil {
		return time.Time{}
	}
	return widestScopeCutoffFromPolicies(settings.AbsencePolicies, out)
}

// widestScopeCutoffFromPolicies is the pure derivation behind
// loadWidestScopeCutoff: now + the widest sit_in_window_weeks in the given
// policy bytes. Nil/empty/unparseable bytes mean "no policy anywhere"
// (zero time) exactly like a failed settings read did. Step 18: the
// At-variant takes the request clock; the bare wrapper keeps time.Now for
// non-range callers that never bind a request clock.
func widestScopeCutoffFromPolicies(policiesJSON []byte, out *SitInBundleV2) time.Time {
	return widestScopeCutoffFromPoliciesAt(policiesJSON, out, time.Time{})
}

func widestScopeCutoffFromPoliciesAt(policiesJSON []byte, out *SitInBundleV2, now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now()
	}
	if len(policiesJSON) == 0 {
		return time.Time{}
	}
	var p AbsencePolicies
	if err := json.Unmarshal(policiesJSON, &p); err != nil {
		return time.Time{}
	}
	widest := 0
	seenMerge := make(map[string]struct{})
	seenRoot := make(map[string]struct{})
	for _, c := range out.ScopeCourses {
		if c.MergeGroupID.Valid {
			k := uuidBytesString(c.MergeGroupID)
			if _, ok := seenMerge[k]; !ok {
				seenMerge[k] = struct{}{}
				if w, ok := p.MergeGroups[k]; ok && w.SitInWindowWeeks > widest {
					widest = w.SitInWindowWeeks
				}
			}
		}
		if c.RootCourseGroupID.Valid {
			k := uuidBytesString(c.RootCourseGroupID)
			if _, ok := seenRoot[k]; !ok {
				seenRoot[k] = struct{}{}
				if w, ok := p.RootCourseGroups[k]; ok && w.SitInWindowWeeks > widest {
					widest = w.SitInWindowWeeks
				}
			}
		}
	}
	seenSubj := make(map[string]struct{})
	for i := range out.SatMappings {
		m := &out.SatMappings[i]
		if !m.MergeGroupID.Valid {
			continue
		}
		k := uuidBytesString(m.MergeGroupID)
		if _, ok := seenSubj[k]; ok {
			continue
		}
		seenSubj[k] = struct{}{}
		if w, ok := p.MergeGroups[k]; ok && w.SitInWindowWeeks > widest {
			widest = w.SitInWindowWeeks
		}
	}
	if widest <= 0 {
		return time.Time{}
	}
	return now.Add(time.Duration(widest) * 7 * 24 * time.Hour)
}

// bundleSessionCourseIDs dedupes the scope + SAT-member course universe.
func bundleSessionCourseIDs(out *SitInBundleV2) []pgtype.UUID {
	ids := make([]pgtype.UUID, 0, len(out.ScopeCourses)+len(out.SatMemberCourses))
	have := make(map[string]struct{}, len(out.ScopeCourses)+len(out.SatMemberCourses))
	for _, c := range out.ScopeCourses {
		k := uuidBytesString(c.ID)
		if _, ok := have[k]; ok {
			continue
		}
		have[k] = struct{}{}
		ids = append(ids, c.ID)
	}
	for _, c := range out.SatMemberCourses {
		k := uuidBytesString(c.ID)
		if _, ok := have[k]; ok {
			continue
		}
		have[k] = struct{}{}
		ids = append(ids, c.ID)
	}
	return ids
}

func (q *Queries) loadBundleSessionsUnbounded(ctx context.Context, out *SitInBundleV2, ids []pgtype.UUID) error {
	if ids == nil {
		ids = bundleSessionCourseIDs(out)
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.db.Query(ctx, bundleSessionsSelectSQL(), ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r SessionInRange
		if err := rows.Scan(&r.ID, &r.CourseID, &r.RoomID, &r.StartAt, &r.EndAt); err != nil {
			return err
		}
		out.Sessions[uuidBytesString(r.CourseID)] = append(out.Sessions[uuidBytesString(r.CourseID)], r)
	}
	return rows.Err()
}

// loadBundleRulesQuery queues the distinct sit-in rules fetch (by ID and by
// root group, UNION ALL with a tag column) onto a batch with the visibility
// probe. Both are pure catalog lookups over request-scoped ID lists, so they
// share one round trip via pgx.Batch (see loadBundleRulesAndVisible).
func (q *Queries) loadBundleRulesQuery(ctx context.Context, out *SitInBundleV2, b *pgx.Batch) (byID, byRoot []pgtype.UUID, ok bool) {
	byID = make([]pgtype.UUID, 0)
	seenID := make(map[string]struct{})
	byRoot = make([]pgtype.UUID, 0)
	seenRoot := make(map[string]struct{})
	collect := func(id pgtype.UUID, root pgtype.UUID) {
		if id.Valid {
			k := uuidBytesString(id)
			if _, ok := seenID[k]; !ok {
				seenID[k] = struct{}{}
				byID = append(byID, id)
			}
		}
		if root.Valid {
			k := uuidBytesString(root)
			if _, ok := seenRoot[k]; !ok {
				seenRoot[k] = struct{}{}
				byRoot = append(byRoot, root)
			}
		}
	}
	for _, e := range out.Enrolled {
		collect(e.SitInRuleID, e.RootCourseGroupID)
	}
	for _, c := range out.ScopeCourses {
		collect(c.SitInRuleID, c.RootCourseGroupID)
	}
	for _, c := range out.SatMemberCourses {
		collect(c.SitInRuleID, c.RootCourseGroupID)
	}
	if len(byID) == 0 && len(byRoot) == 0 {
		return nil, nil, false
	}
	// Queued, not executed: the caller drains the batch and scans results
	// in queue order (rules first, visibility second).
	b.Queue(`
		SELECT 0 AS tag, id, name, type, predicate, description, created_at, updated_at, NULL::uuid AS root_id
		FROM sit_in_rules WHERE id = ANY($1::uuid[])
		UNION ALL
		SELECT 1 AS tag, sir.id, sir.name, sir.type, sir.predicate, sir.description, sir.created_at, sir.updated_at, rcg.id
		FROM sit_in_rules sir JOIN root_course_groups rcg ON rcg.sit_in_rule_id = sir.id WHERE rcg.id = ANY($2::uuid[])
	`, byID, byRoot)
	return byID, byRoot, true
}

// scanBundleRulesResult drains one queued rules result set in batch order.
func scanBundleRulesResult(rows pgx.Rows, out *SitInBundleV2) error {
	defer rows.Close()
	for rows.Next() {
		var r SitInRule
		var root pgtype.UUID
		var tag int
		if err := rows.Scan(&tag, &r.ID, &r.Name, &r.Type, &r.Predicate, &r.Description, &r.CreatedAt, &r.UpdatedAt, &root); err != nil {
			return err
		}
		cp := r
		if tag == 0 {
			out.RulesByID[uuidBytesString(r.ID)] = &cp
		} else {
			out.RulesByRoot[uuidBytesString(root)] = &cp
		}
	}
	return rows.Err()
}
