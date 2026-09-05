package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// SessionsRangeSitInBundleV2 loads every sit-in rule input in a FIXED number
// of round trips independent of course count:
//
//  1. bundleEnrolled: all enrollments of the student (1 Query)
//  2. bundleScopeCourses: root/merge siblings + missed courses (1 Query)
//  3. bundlePriorities: priorities for distinct root groups via ANY (1 Query)
//  4. bundleRules: sit-in rules for distinct rule IDs + root groups (1 Query)
//  5. bundleSatData: active SAT mappings + missing merge names (<= 2 Queries)
//  6. bundleSessions: all sessions of scope courses via ANY (1 Query)
//  7. bundleVisibility: CourseIDsVisible for the pool (1 Query)
//
// Worst case: 8 round trips, constant in courses. Combined with facts (1),
// absent (1), scopes (1), day counts (1), student row (1, cached from the
// facts path), and the handler settings row (1), the endpoint stays at a
// small constant. The follow-up optimization folds 2-4 into fewer queries;
// the invariant under test is O(1) in courses, asserted by the query-count
// gate.
type SitInBundleV2Params struct {
	StudentID       pgtype.UUID
	MissedCourseIDs []pgtype.UUID
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

// SessionsRangeSitInBundleV2 loads the universe. See contract above.
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
	// Mirror legacy priority handling: a priority-catalog
	// failure degrades to single-rule resolution, never a request error.
	if err := q.loadBundlePriorities(ctx, bundle); err != nil {
		bundle.Priorities = nil
	}
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
	if err := q.loadBundleMergeMembers(ctx, out); err != nil {
		out.ResolveFailed = true
	}
	if err := q.loadBundleSatMembers(ctx, out); err != nil {
		out.ResolveFailed = true
	}
	// One sessions round trip for scope courses AND out-of-scope SAT mapped
	// members (legacy SessionsByCourse per target, unbounded).
	if err := q.loadBundleSessionsAll(ctx, out); err != nil {
		out.ResolveFailed = true
	}
	visible, err := q.LoadBundleVisible(ctx, out.ScopeCourses, out.SatMemberCourses)
	if err != nil {
		out.ResolveFailed = true
	} else {
		out.Visible = visible
	}
	if err := q.loadBundleRules(ctx, out); err != nil {
		out.ResolveFailed = true
	}
	return out, nil
}

func (q *Queries) loadBundleMergeMembers(ctx context.Context, out *SitInBundleV2) error {
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
	for _, c := range out.ScopeCourses {
		add(c.MergeGroupID)
	}
	for i := range out.SatMappings {
		add(out.SatMappings[i].MergeGroupID)
	}
	if len(groups) == 0 {
		return nil
	}
	rows, err := q.db.Query(ctx, "SELECT group_id, course_id FROM course_merge_group_members WHERE group_id = ANY($1::uuid[]) ORDER BY group_id, position ASC", groups)
	if err != nil {
		return err
	}
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

// loadBundleSatMembers batch-loads full course rows for mapped merge-group
// members outside the student scope universe. Legacy satVerbalMappedCourses
// resolves members via CourseSubjectByID with no enrollment gate, so every
// mapped member must resolve even when the student never enrolled near it.
// One query for all mapped groups; rows merge into SatMemberCourses only.
func (q *Queries) loadBundleSatMembers(ctx context.Context, out *SitInBundleV2) error {
	groups := make([]pgtype.UUID, 0)
	seen := make(map[string]struct{})
	have := make(map[string]struct{}, len(out.ScopeCourses))
	for _, c := range out.ScopeCourses {
		have[uuidBytesString(c.ID)] = struct{}{}
	}
	for i := range out.SatMappings {
		m := &out.SatMappings[i]
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
	if len(groups) == 0 {
		return nil
	}
	rows, err := q.db.Query(ctx, loadBundleSatMembersSQL(), groups)
	if err != nil {
		return err
	}
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

// loadBundleSessionsAll loads unbounded sessions for scope courses AND
// out-of-scope SAT mapped members in ONE round trip (legacy SessionsByCourse
// per target, unbounded; the caller slices by instant in Go). Sessions land
// in out.Sessions keyed by course, covering both universes.
//
// offcut: one unbounded query for the student scope universe (same rows the
// legacy path loaded across N per-target queries, via an indexed
// course_id = ANY lookup); bound by enrolled+scope course count, not by
// table size. Do not add LIMIT (silent truncation breaks sit-in options);
// split by course or add a cutoff upper bound if a scope ever exceeds ~50k
// rows.
func (q *Queries) loadBundleSessionsAll(ctx context.Context, out *SitInBundleV2) error {
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

// loadBundleRules fetches distinct sit-in rules by ID and by root group in
// ONE round trip (UNION ALL with a tag column).
func (q *Queries) loadBundleRules(ctx context.Context, out *SitInBundleV2) error {
	byID := make([]pgtype.UUID, 0)
	seenID := make(map[string]struct{})
	byRoot := make([]pgtype.UUID, 0)
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
	if len(byID) > 0 || len(byRoot) > 0 {
		rows, err := q.db.Query(ctx, `
			SELECT 0 AS tag, id, name, type, predicate, description, created_at, updated_at, NULL::uuid AS root_id
			FROM sit_in_rules WHERE id = ANY($1::uuid[])
			UNION ALL
			SELECT 1 AS tag, sir.id, sir.name, sir.type, sir.predicate, sir.description, sir.created_at, sir.updated_at, rcg.id
			FROM sit_in_rules sir JOIN root_course_groups rcg ON rcg.sit_in_rule_id = sir.id WHERE rcg.id = ANY($2::uuid[])
		`, byID, byRoot)
		if err != nil {
			return err
		}
		for rows.Next() {
			var r SitInRule
			var root pgtype.UUID
			var tag int
			if err := rows.Scan(&tag, &r.ID, &r.Name, &r.Type, &r.Predicate, &r.Description, &r.CreatedAt, &r.UpdatedAt, &root); err != nil {
				rows.Close()
				return err
			}
			cp := r
			if tag == 0 {
				out.RulesByID[uuidBytesString(r.ID)] = &cp
			} else {
				out.RulesByRoot[uuidBytesString(root)] = &cp
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}
