package db

// Step-16 tag-2 union-arm parity gate.
//
// The enrolled+scope UNION ALL carries the active SAT verbal mappings list
// (tag 2) instead of a standalone SatVerbalPolicyMappingsList query. This
// test seeds real mappings (course-targeted + merge-group-targeted,
// inserted in reverse rule_id order, plus one inactive row that must be
// excluded) and asserts the union arm returns identical structs in the
// list order contract (ORDER BY rule_id ASC), and marks satMappingsLoaded
// so the backfill does not re-query.
import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSessionsRangeBundleUnionSatMappingsParity(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	q := New(pool)
	suffix := strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")

	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{
		Username:     "teacher-unionpar-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = teacherID
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "UNIONPAR-" + suffix, Name: "UnionPar " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	// UNIQUE(course_id) holds regardless of active: the inactive probe row
	// needs its own course.
	courseInactive, err := q.CourseCreate(ctx, CourseCreateParams{Code: "UNIONPAR-INACT-" + suffix, Name: "UnionPar Inact " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	student, err := q.StudentCreate(ctx, StudentCreateParams{Wcode: "wunionpar-" + suffix, FullName: "Union Parity Student"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: course.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}
	var mergeID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO course_merge_groups (name) VALUES ($1) RETURNING id`, "UnionPar Merge "+suffix).Scan(&mergeID); err != nil {
		t.Fatal(err)
	}

	// rule_ids chosen so insert order (zz first, aa second) is the
	// REVERSE of the ORDER BY rule_id contract - the scan must re-sort.
	courseRule := "zz-course-" + suffix
	mergeRule := "aa-merge-" + suffix
	inactiveRule := "mm-inactive-" + suffix
	mkUUID := func(v pgtype.UUID) any {
		return v
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sat_verbal_policy_mappings (rule_id, course_id, merge_group_id, policy_rule, policy_hash, active) VALUES ($1, $2, NULL, '{}', 'h1', true)`, courseRule, mkUUID(course.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sat_verbal_policy_mappings (rule_id, course_id, merge_group_id, policy_rule, policy_hash, active) VALUES ($1, NULL, $2, '{"w":2}', 'h2', true)`, mergeRule, mkUUID(mergeID)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sat_verbal_policy_mappings (rule_id, course_id, merge_group_id, policy_rule, policy_hash, active) VALUES ($1, $2, NULL, '{}', 'h3', false)`, inactiveRule, mkUUID(courseInactive.ID)); err != nil {
		t.Fatal(err)
	}
	// Cleanup deletes by the seeded IDs: rule_ids embed a timestamp
	// suffix that could theoretically collide across runs, but IDs are
	// unique per insert — capture them for an exact delete.
	var seededIDs []pgtype.UUID
	for _, rid := range []string{courseRule, mergeRule, inactiveRule} {
		var id pgtype.UUID
		if err := pool.QueryRow(ctx, `SELECT id FROM sat_verbal_policy_mappings WHERE rule_id = $1`, rid).Scan(&id); err != nil {
			t.Fatal(err)
		}
		seededIDs = append(seededIDs, id)
	}
	t.Cleanup(func() {
		// Cleanup over a FRESH pool with a plain (non-simple-protocol)
		// connection: the test pool uses QueryExecModeSimpleProtocol,
		// which cannot encode a []pgtype.UUID array argument (unknown
		// type OID 0) — the delete silently affected 0 rows and every
		// run leaked its 3 seeded mappings into the shared database.
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		cpool, cerr := pgxpool.New(cctx, databaseURL)
		if cerr != nil {
			return
		}
		defer cpool.Close()
		_, _ = cpool.Exec(cctx, `DELETE FROM sat_verbal_policy_mappings WHERE id = ANY($1::uuid[])`, seededIDs)
	})

	list, err := q.SatVerbalPolicyMappingsList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var want []SatVerbalPolicyCourseMapping
	for _, m := range list {
		if strings.HasSuffix(m.RuleID, suffix) {
			want = append(want, m)
		}
	}
	if len(want) != 2 {
		t.Fatalf("expected 2 active seeded mappings in list, got %d", len(want))
	}
	if want[0].RuleID != mergeRule || want[1].RuleID != courseRule {
		t.Fatalf("list order broken: got %q, %q", want[0].RuleID, want[1].RuleID)
	}

	out := &SitInBundleFacts{
		MergeNames:       make(map[string]string),
		VisibleCourseIDs: make(map[string]struct{}),
		SessionsByCourse: make(map[string][]SessionInRange),
	}
	if err := q.loadBundleEnrolledAndScopeUnion(ctx, SitInBundleFactsParams{StudentID: student.ID}, out); err != nil {
		t.Fatal(err)
	}
	if !out.satMappingsLoaded {
		t.Fatal("union did not mark satMappingsLoaded")
	}
	var got []SatVerbalPolicyCourseMapping
	for _, m := range out.SatMappings {
		if strings.HasSuffix(m.RuleID, suffix) {
			got = append(got, m)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("union arm returned %d seeded mappings, list has %d", len(got), len(want))
	}
	sameUUID := func(a, b pgtype.UUID) bool { return a.Valid == b.Valid && (!a.Valid || a.Bytes == b.Bytes) }
	for i := range want {
		a, b := want[i], got[i]
		if a.RuleID != b.RuleID || !sameUUID(a.ID, b.ID) || !sameUUID(a.CourseID, b.CourseID) ||
			!sameUUID(a.MergeGroupID, b.MergeGroupID) || a.CourseCode != b.CourseCode ||
			a.CourseName != b.CourseName || !sameUUID(a.SubjectID, b.SubjectID) ||
			a.SubjectCode != b.SubjectCode || a.SubjectName != b.SubjectName ||
			a.CycleID.String != b.CycleID.String || a.Level.Int16 != b.Level.Int16 ||
			!sameUUID(a.RootCourseGroupID, b.RootCourseGroupID) || !sameUUID(a.SitInRuleID, b.SitInRuleID) ||
			string(a.PolicyRule) != string(b.PolicyRule) || a.PolicyHash != b.PolicyHash ||
			a.Active != b.Active || !a.CreatedAt.Time.Equal(b.CreatedAt.Time) ||
			!a.UpdatedAt.Time.Equal(b.UpdatedAt.Time) {
			t.Fatalf("row %d diverged: list=%+v union=%+v", i, a, b)
		}
	}
	// Merge-group-targeted row has no course join: course columns empty.
	if got[0].CourseCode != "" || got[0].CourseName != "" || got[0].CourseID.Valid {
		t.Fatalf("merge-targeted row should have empty course columns: %+v", got[0])
	}
	if got[1].CourseCode == "" || !got[1].CourseID.Valid {
		t.Fatalf("course-targeted row should carry the joined course: %+v", got[1])
	}
	// Backfill must be a no-op now: no duplicate rows behind the flag.
	before := len(out.SatMappings)
	if err := q.loadBundleSatMappings(ctx, out); err != nil {
		t.Fatal(err)
	}
	if len(out.SatMappings) != before {
		t.Fatalf("backfill duplicated mappings: %d -> %d", before, len(out.SatMappings))
	}
}

// Step-16 trip-A mid-batch parity gate: the batched drain
// (loadBundleMidBatch) must produce IDENTICAL bundle state to the three
// standalone loaders (loadBundlePriorities + loadBundleMergeMembers +
// loadBundleSatMembers) on the same world — same priorities rows, same
// merge-member map, same out-of-scope SAT members. The mid-batch shares
// SQL text, input derivation, and row scans with the standalone paths
// (bundlePrioritiesSQL / bundleMergeMembersSQL / loadBundleSatMembersSQL
// + bundle*Groups + scan* helpers), so this gate pins the wiring
// (queue order, conditional queueing, drain order), not the SQL.
//
// The production orchestrator (SessionsRangeSitInBundleV2) loads
// priorities ONLY via the mid-batch (no standalone call — that would
// fire the identical SELECT twice AND append 2N rows). The standalone
// arm below exists purely as the parity reference for this gate.
//
// Merge-names arm: the world seeds an out-of-scope SAT-mapped merge
// group (mergeID2, name set), so loadBundleSatMappingNamesStandalone
// fills the missing name on the standalone side and the fused names arm
// must produce the identical MergeNames entry on the batch side.
func TestSessionsRangeBundleMidBatchMatchesStandalone(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	q := New(pool)
	suffix := strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")

	courseA, err := q.CourseCreate(ctx, CourseCreateParams{Code: "MIDB-A-" + suffix, Name: "MidB A " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	courseB, err := q.CourseCreate(ctx, CourseCreateParams{Code: "MIDB-B-" + suffix, Name: "MidB B " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	student, err := q.StudentCreate(ctx, StudentCreateParams{Wcode: "wmidbatch-" + suffix, FullName: "MidBatch Student"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []CourseCreateRow{courseA, courseB} {
		if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: c.ID, StudentID: student.ID}); err != nil {
			t.Fatal(err)
		}
	}
	// Merge group over A+B (member rows the mid-batch must return).
	var mergeID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO course_merge_groups (name) VALUES ($1) RETURNING id`, "MidB Merge "+suffix).Scan(&mergeID); err != nil {
		t.Fatal(err)
	}
	for i, c := range []CourseCreateRow{courseA, courseB} {
		if _, err := pool.Exec(ctx, `INSERT INTO course_merge_group_members (group_id, course_id, position) VALUES ($1, $2, $3)`, mergeID, c.ID, i+1); err != nil {
			t.Fatal(err)
		}
	}
	// Third course OUTSIDE the student universe, pulled in only via a
	// merge-group-targeted mapping (exercises the SAT-members arm).
	courseC, err := q.CourseCreate(ctx, CourseCreateParams{Code: "MIDB-C-" + suffix, Name: "MidB C " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	var mergeID2 pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO course_merge_groups (name) VALUES ($1) RETURNING id`, "MidB Merge2 "+suffix).Scan(&mergeID2); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO course_merge_group_members (group_id, course_id, position) VALUES ($1, $2, 1)`, mergeID2, courseC.ID); err != nil {
		t.Fatal(err)
	}
	// Root group + rule + priority (exercises the priorities arm).
	var rootID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO root_course_groups (name) VALUES ($1) RETURNING id`, "MidB Root "+suffix).Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	var ruleID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO sit_in_rules (name, type, predicate) VALUES ($1, 'level_ladder', '{"level_1_action": "zoom", "non_max_direction": "sit_higher", "max_direction": "sit_lower", "min_level_for_sit_lower": 2}'::jsonb) RETURNING id`, "MidB Rule "+suffix).Scan(&ruleID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE root_course_groups SET sit_in_rule_id = $1 WHERE id = $2`, ruleID, rootID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE courses SET root_course_group_id = $1 WHERE id = $2`, rootID, courseA.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sit_in_priorities (root_course_group_id, sit_in_rule_id, priority_level, label) VALUES ($1, $2, 1, 'p1')`, rootID, ruleID); err != nil {
		t.Fatal(err)
	}
	// Merge-group-targeted mapping over mergeID2 (SAT-members arm).
	mapRule := "midb-map-" + suffix
	if _, err := pool.Exec(ctx, `INSERT INTO sat_verbal_policy_mappings (rule_id, course_id, merge_group_id, policy_rule, policy_hash, active) VALUES ($1, NULL, $2, '{}', 'hm', true)`, mapRule, mergeID2); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		cpool, cerr := pgxpool.New(cctx, databaseURL)
		if cerr != nil {
			return
		}
		defer cpool.Close()
		_, _ = cpool.Exec(cctx, `DELETE FROM sat_verbal_policy_mappings WHERE rule_id = $1`, mapRule)
		_, _ = cpool.Exec(cctx, `DELETE FROM sit_in_priorities WHERE root_course_group_id = $1`, rootID)
		_, _ = cpool.Exec(cctx, `DELETE FROM sit_in_rules WHERE id = $1`, ruleID)
		_, _ = cpool.Exec(cctx, `DELETE FROM course_merge_group_members WHERE group_id IN ($1, $2)`, mergeID, mergeID2)
		_, _ = cpool.Exec(cctx, `DELETE FROM course_merge_groups WHERE id IN ($1, $2)`, mergeID, mergeID2)
	})

	mkOut := func() (*SitInBundleFacts, *SitInBundleV2) {
		bundle := &SitInBundleFacts{
			MergeNames:       make(map[string]string),
			VisibleCourseIDs: make(map[string]struct{}),
			SessionsByCourse: make(map[string][]SessionInRange),
		}
		out := &SitInBundleV2{
			RulesByID:      make(map[string]*SitInRule),
			RulesByRoot:    make(map[string]*SitInRule),
			SatMapByCourse: make(map[string]*SatVerbalPolicyCourseMapping),
			MergeNames:     make(map[string]string),
			MergeMembers:   make(map[string][]pgtype.UUID),
			Visible:        make(map[string]struct{}),
			Sessions:       make(map[string][]SessionInRange),
		}
		return bundle, out
	}
	loadHead := func(bundle *SitInBundleFacts, out *SitInBundleV2) {
		t.Helper()
		if err := q.loadBundleEnrolledAndScopeUnion(ctx, SitInBundleFactsParams{StudentID: student.ID}, bundle); err != nil {
			t.Fatal(err)
		}
		out.Enrolled = bundle.Enrolled
		out.ScopeCourses = bundle.ScopeCourses
		out.SatMappings = bundle.SatMappings
		out.MergeNames = bundle.MergeNames
	}
	// Standalone path.
	bundleS, outS := mkOut()
	loadHead(bundleS, outS)
	if err := q.loadBundlePriorities(ctx, bundleS); err != nil {
		t.Fatal(err)
	}
	if err := q.loadBundleMergeMembers(ctx, outS); err != nil {
		t.Fatal(err)
	}
	if err := q.loadBundleSatMembers(ctx, outS); err != nil {
		t.Fatal(err)
	}
	// Mid-batch path (trip-A + mega-tail, mirroring the production
	// orchestrator: mid-batch, then cutoff derivation, then the
	// sessions+rules+visible tail — but with the SAME bounded Discovery
	// the service passes, so the sessions arm loads the bounded shape).
	bundleB, outB := mkOut()
	loadHead(bundleB, outB)
	if err := q.loadBundleMidBatch(ctx, bundleB, outB); err != nil {
		t.Fatal(err)
	}
	if bundleB.PrioritiesFailed {
		bundleB.Priorities = nil
	}
	outB.Priorities = bundleB.Priorities
	// Standalone reference path for the tail: bounded sessions +
	// rules+visible via the pre-mega-tail loaders.
	bundleS2, outS2 := mkOut()
	loadHead(bundleS2, outS2)
	if err := q.loadBundlePriorities(ctx, bundleS2); err != nil {
		t.Fatal(err)
	}
	if err := q.loadBundleMergeMembers(ctx, outS2); err != nil {
		t.Fatal(err)
	}
	if err := q.loadBundleSatMembers(ctx, outS2); err != nil {
		t.Fatal(err)
	}
	if err := loadBundleSatMappingNamesStandalone(ctx, pool, bundleS2); err != nil {
		t.Fatal(err)
	}
	outS2.Priorities = bundleS2.Priorities
	outS2.ScopeCourses = bundleS2.ScopeCourses
	outS2.SatMappings = bundleS2.SatMappings
	outS2.MergeNames = bundleS2.MergeNames
	outS2.SatMemberCourses = outS.SatMemberCourses
	wf, _ := time.Parse("2006-01-02", "2026-01-05")
	wt, _ := time.Parse("2006-01-02", "2026-01-12")
	disc := SitInDiscoveryBounds{WindowFromUTC: wf, WindowToExclUTC: wt}
	if err := q.loadBundleSessionsBounded(ctx, outS2, disc); err != nil {
		t.Fatal(err)
	}
	outS2.Visible = arrayToVisibleSet(bundleVisibleCourseIDs(outS2.ScopeCourses, outS2.SatMemberCourses))
	if err := q.loadBundleRulesAndVisible(ctx, outS2); err != nil {
		t.Fatal(err)
	}
	// Mega-tail path: same head + mid-batch outputs, one fused trip.
	outB.ScopeCourses = bundleB.ScopeCourses
	outB.SatMappings = bundleB.SatMappings
	outB.Enrolled = bundleB.Enrolled
	for k, v := range bundleB.MergeNames {
		outB.MergeNames[k] = v
	}
	outB.Visible = arrayToVisibleSet(bundleVisibleCourseIDs(outB.ScopeCourses, outB.SatMemberCourses))
	if err := q.loadBundleSessionsRulesVisibleTail(ctx, outB, disc); err != nil {
		t.Fatal(err)
	}
	// Tail parity: sessions universe, rules maps, visible set identical.
	if len(outB.Sessions) != len(outS2.Sessions) {
		t.Fatalf("tail sessions courses diverged: standalone=%d batch=%d", len(outS2.Sessions), len(outB.Sessions))
	}
	for k, vs := range outS2.Sessions {
		vb, ok := outB.Sessions[k]
		if !ok || len(vb) != len(vs) {
			t.Fatalf("tail sessions course %s diverged: standalone=%d batch=%d", k, len(vs), len(vb))
		}
		for i := range vs {
			if vs[i].ID != vb[i].ID || vs[i].StartAt != vb[i].StartAt {
				t.Fatalf("tail session %d course %s diverged", i, k)
			}
		}
	}
	if len(outB.RulesByID) != len(outS2.RulesByID) || len(outB.RulesByRoot) != len(outS2.RulesByRoot) {
		t.Fatalf("tail rules diverged: standalone id=%d/root=%d batch id=%d/root=%d", len(outS2.RulesByID), len(outS2.RulesByRoot), len(outB.RulesByID), len(outB.RulesByRoot))
	}
	for k := range outS2.RulesByID {
		if _, ok := outB.RulesByID[k]; !ok {
			t.Fatalf("tail rule id %s missing in batch", k)
		}
	}
	for k := range outS2.RulesByRoot {
		if _, ok := outB.RulesByRoot[k]; !ok {
			t.Fatalf("tail rule root %s missing in batch", k)
		}
	}
	for k := range outS2.Visible {
		if _, ok := outB.Visible[k]; !ok {
			t.Fatalf("tail visible %s missing in batch", k)
		}
	}
	if len(outB.Visible) != len(outS2.Visible) {
		t.Fatalf("tail visible count diverged: standalone=%d batch=%d", len(outS2.Visible), len(outB.Visible))
	}
	if bundleB.PrioritiesFailed {
		t.Fatal("mid-batch flagged PrioritiesFailed on a healthy world")
	}
	if len(bundleB.Priorities) == 0 {
		t.Fatal("mid-batch priorities arm returned nothing; seed a priority")
	}
	if len(bundleB.Priorities) != len(bundleS.Priorities) {
		t.Fatalf("priorities diverged: standalone=%d batch=%d", len(bundleS.Priorities), len(bundleB.Priorities))
	}
	for i := range bundleS.Priorities {
		a, b := bundleS.Priorities[i], bundleB.Priorities[i]
		if a.ID != b.ID || a.RootCourseGroupID != b.RootCourseGroupID || a.SitInRuleID != b.SitInRuleID || a.PriorityLevel != b.PriorityLevel {
			t.Fatalf("priority row %d diverged", i)
		}
	}
	if len(outB.MergeMembers) != len(outS.MergeMembers) {
		t.Fatalf("merge members diverged: standalone=%d groups batch=%d groups", len(outS.MergeMembers), len(outB.MergeMembers))
	}
	for k, vs := range outS.MergeMembers {
		vb, ok := outB.MergeMembers[k]
		if !ok || len(vb) != len(vs) {
			t.Fatalf("merge group %s diverged", k)
		}
		for i := range vs {
			if vs[i] != vb[i] {
				t.Fatalf("merge group %s member %d diverged", k, i)
			}
		}
	}
	if len(outB.SatMemberCourses) != len(outS.SatMemberCourses) {
		t.Fatalf("SAT members diverged: standalone=%d batch=%d", len(outS.SatMemberCourses), len(outB.SatMemberCourses))
	}
	if len(outB.SatMemberCourses) == 0 {
		t.Fatal("mid-batch SAT-members arm returned nothing; seed an out-of-scope mapped member")
	}
	for i := range outS.SatMemberCourses {
		a, b := outS.SatMemberCourses[i], outB.SatMemberCourses[i]
		if a.ID != b.ID || a.Code != b.Code {
			t.Fatalf("SAT member %d diverged", i)
		}
	}
	// Merge-names parity: standalone probe (V1 helper) vs fused trip-A
	// 4th arm. mergeID2's group sits outside the scope universe, so its
	// name arrives ONLY via the probe/arm — both sides must agree.
	if err := loadBundleSatMappingNamesStandalone(ctx, pool, bundleS); err != nil {
		t.Fatal(err)
	}
	// NOTE: standalone names land in bundleS.MergeNames (bundle-space)
	// while the fused arm drains into outB.MergeNames (out-space, same
	// map the orchestrator copies at line ~149). loadHead seeded both
	// from the same union rows, so the SETS must agree exactly.
	for k, want := range bundleS.MergeNames {
		if got, ok := outB.MergeNames[k]; !ok || got != want {
			t.Fatalf("merge name %s diverged: standalone=%q batch=%q", k, want, got)
		}
	}
	if len(outB.MergeNames) != len(bundleS.MergeNames) {
		t.Fatalf("merge names count diverged: standalone=%d batch=%d", len(bundleS.MergeNames), len(outB.MergeNames))
	}
	if len(outB.MergeNames) == 0 {
		t.Fatal("expected merge names on both sides; seed a named merge group")
	}
}
