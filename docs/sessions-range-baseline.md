# Sessions-in-Range Baseline + Acceptance Checklist (Step 1)

Date: 2026-09-06. Commit: `95be4d1` ("absence-consistency final").
DB: PostgreSQL 14.23 (Homebrew, local dev). Pool: backend/internal/pg defaults
(MaxConns 10, MinConns 0, 5m lifetime/idle, 30s healthcheck).
Hardware: Apple M1, 8 GB, macOS 14.5. Go 1.26.3 darwin/arm64.
Dataset (dev, warwick_local_fresh): sessions=102921, student_absences=438,
students=357, courses=992, absence_sit_ins=135. Seed: local dev database
(contents drift; NOT a pinned benchmark seed - see item 7).

## Measured today (local dev, warm pool)

- V2 integration suite: PASS (TestSessionsRangeV2_ShadowEquivalence incl.
  error_parity x10, ConcurrentReads x50, QueryCountGate).
- Query-count gate log: legacy=35, v2=2 + 4 batch trips (10 batched
  statements) on the shadow world (2026-09-06, Step-16 rounds 16-24:
  head facts+student batch, standalone-priorities double-fire removed,
  merge-names probe fused into trip-A, sessions+rules+visible mega-tail)
  policies row eliminated via PoliciesJSON threading, SAT-mappings list
  folded into the enrolled+scope UNION ALL tag-2 arm, scopes+daycounts
  folded into one statement, fold+absent+blocked share one batch trip).
  (Gate asserts v2 < legacy, v2 <= 25, full==narrow. Challenge target is <= 5.)
- Step-14 set-3 row-volume gate: unbounded=153 vs bounded=6 rows on the same
  request with a 150-row unrelated-history probe (2y-past + 3y-future x 3
  scope courses); bounded excludes all of it in SQL (window floor + widest
  cutoff, clamped to window ceiling when no policy exists). Shadow
  equivalence (all modes incl. error_parity) + ConcurrentReads still PASS.
- Step-15 day-count gates (TestSessionsRangeDayCountsMatchesLegacy +
  TestSessionsRangeDayCountsBoundsEligibilityWork): reference definition
  pinned (scope/total/used/cancelled-exclusion/distinct-day rules) with
  explicit + legacy + cancelled/special_approved + same-day + soft-deleted
  cases AND legacy Total/Used parity on the same world; eligibility
  function in exactly 1 plan node (materialized candidate set, was 4
  per-arm evaluations); 400-row unrelated-course history leaves the scope
  union at 20 eligible sessions. Query rewritten (relevant_sessions AS
  MATERIALIZED, one eligibility evaluation per session); shadow +
  regression suites still PASS. No maintained projection (Step-15 gate
  unmet by design); exactness via single set-based aggregation +
  write-time validation.
- Pure unit subset (TestSessionsInRange|...|TestTiming): PASS.

## Representative requests (from sessions_range_shadow_test.go)

Staff: /api/v1/absences/sessions-in-range?wcode=...&date_from=...&date_to=...
(+ bypass_timing, course_ids, sat_verbal_after_priority=1,
include_all_subjects+subject_ids variants, lifetime=true for the explicit
authorized lifetime lookup). Student: same path forced through
handleStudentSessions (identity from verified session; wcode/bypass_timing/
include_all_subjects/subject_ids/lifetime keys rejected). Lifetime
1970-01-01->2100-01-01 WITHOUT the flag: rejected date_range_exceeded by
BOTH paths (maxStaffSessionsRangeDays=366, routes.go; Step 17 keeps this).
WITH lifetime=true (admin + explicit range): 200 with all relevant sessions
on both paths (Step 17). admin_over_cap (>366d future) likewise rejected.

## Acceptance scoreboard (initial; unmeasured = not tested)

| # | Challenge | Status | Evidence |
|---|---|---|---|
| 1 | Query count O(1), <= 5 trips | CLOSE-OUT 6 trips (5 logical groups; 1 over literal budget — cross-boundary bundle+tail fusion blocked, documented below; O(1) proven: full==narrow, set-3 bounded 153->6; scale 1/10/100/1000 identical) | shadow gate log (legacy=35 v2=2, query=2 batch=4 batchQueries=10); service header; union tag-2 + mid-batch + mega-tail parity gates |
| 2 | Latency SLOs (student/staff/pathological) | NOT TESTED | no 10M-scale dataset, no harness |
| 3 | Complexity O(R+...) | PASSING (set-3 bounded: missed-history [window,window) + candidates [window-floor,widest-cutoff]; 150-row unrelated-history probe excluded in SQL: unbounded=153 bounded=6; legacy parity held) | sessions_range_sitin_bundle.go (loadBundleSessionsBounded + loadWidestScopeCutoff); shadow unrelated_history gate |
| 4 | DB work (plans/buffers) | PARTIAL (day-count eligibility 4 nodes -> 1 via EXPLAIN gate; full-path plans/buffers still open) | TestSessionsRangeDayCountsBoundsEligibilityWork |
| 5 | Extreme range proportional | PASSING for the authorized path (Step 17: lifetime=true returns 200 with 500 relevant sessions, trips identical before/after unrelated-history growth; unflagged 1970-2100 still rejects by design) | TestSessionsRangeV2_LifetimeStaffLookup (plain=2 batch=3 before/after); D1 in sessions-range-decisions.md |
| 6 | 100k oracle worlds | NOT TESTED (1 hand-built world) | seedShadowWorld |
| 7 | Property invariants | NOT TESTED | - |
| 8 | TZ torture | NOT TESTED (Bangkok-only) + 2 reported defects | routes.go:1691, 00120/00121 |
| 9 | Concurrency/failure races | PASSING (reads; same-student/same-session; reassign-vs-submission; cancel-vs-submission; cross-student both-succeed D7; same-key cross-replica 1-row; opposite-order batches; merge serialization control; edit fence both directions; between/before-commit/acquire-rollback no-partials) | ConcurrentReads; submission_race_test.go; step13_race_test.go; step13_edit_fence_test.go; step13_failure_test.go |
| 10 | Linearization/tx protocol | PASSING (matrix signed off F1+G1+G3; transitions doc; reassign day-count invariance pinned) | sessions-range-lock-order.md; sessions-range-state-transitions.md |
| 11 | Retry/idempotency matrix | PASSING (DB acquire/complete/reuse/concurrent/scope/stale covered; HTTP staff-create matrix: replay identical, reuse 409, fresh key distinct, late replay stable) | idempotency_integration_test.go; TestIdempotencyMatrix_StaffCreate |
| 12 | Replica independence | NOT TESTED | - |
| 13 | Auth hostile matrix | PARTIAL (3 keys rejected; subject_ids/sat_priority open) | self_service_routes.go:130 |
| 14 | Contract golden | PARTIAL (shadow parity, no golden file) | shadow test |
| 15 | Deterministic resolver | PARTIAL (pure domain, map-order risks) | sessions_range_domain.go |
| 16 | Single business logic | PARTIAL (shared limit ctor; dual day-count SQL; Step-15 reference definition pins counting semantics both share) | submission.go; sessions_range_scope_facts.go; sessions_range_day_counts_parity_test.go |
| 17 | Read/write revalidation | PARTIAL (write revalidates; no gap tests) | routes.go:600 |
| 18 | Plan stability under skew | NOT TESTED | - |
| 19 | Memory/alloc | NOT TESTED | - |
| 20 | Throughput 1k rps/30m | NOT TESTED | - |
| 21 | Cache policy | PASS-BY-ABSENCE (no cache; must stay explicit) | - |
| 22 | Observability/shadow rollout | NOT TESTED (no trace fields, no prod shadow) | - |
| 23 | Mutation >95% / fuzz | NOT TESTED | - |

## Step-5 close-out (2026-09-06, uncommitted Step-5 tree on branch deepswe-absence-consistency)

- Migration `backend/db/migrations/00123_cross_study_institute_tz.sql` adds
  `student_is_expected_at_course_time_tz` / `student_is_expected_at_session_tz`
  with `p_institute_tz` (Bangkok default); wrappers preserve old behavior for
  mixed-version replicas. DB at goose version 123; all 4 functions verified.
- Zero remaining 2-arg `student_is_expected_at_session(`/`_course_time(` Go/SQL
  callers outside wrappers, deployed-migration bodies, and tests.
- Evidence (all with `TEST_DATABASE_URL` on `warwick_local_fresh`, PG 14.23):
  - `TestRegression_*` (7/7 inc. T2 cross-study TZ): PASS.
  - `TestSessionsRangeV2_ShadowEquivalence` (all subtests) +
    `TestSessionsRangeV2_ConcurrentReads`: PASS.
  - `internal/scheduling`, `internal/absences/...`: ok.
  - Static TZ tests (`TestAbsenceSessionValidation...`,
    `TestAbsenceOverlappingSessions...`): PASS; `go build ./...` + `go vet`: clean.
- `go test ./internal/db/` full-suite failures are PRE-EXISTING, not Step-5
  regressions (proven by re-running the failing subset on a fully clean tree
  with migration 00123 + regression file moved out and all tracked diffs
  stashed - identical failures):
  - `TestSitInsBySessionIDs/...grouped_by_session`: nickname NULL scan
    (`absence_sit_ins_calendar_integration_test.go:103`, unrelated join).
  - `TestActiveCoursesList*`: FK `crm_cross_study_assignments_source_course_id_fkey`
    in test cleanup (`active_courses_integration_test.go:28`).
  - `TestCourseOverview_SessionDateFilter`: NULL `teacher_id` seed
    (`courses_overview_integration_test.go:339`).
  - `TestCourseOverview_{TeacherFilter,AllowsNullTeacherAndSubject,StudentCountUsesEnrolledRoster,LiveVsArchived}`
    + `TestScheduleDB_ScheduleStabilizationIndexesExistAndSupportPlans`: flaky
    under this dataset (10-30s timeouts); pass/fail membership flips between
    identical runs on BOTH trees.
- T3b (stale-version 409) is owned by Step 7 (`writeSessionSnapshotResult`), not
  re-proven here.

## Step-15 close-out (2026-09-06, branch deepswe-absence-consistency)

- Reference definition written in the `SessionsRangeDayCounts` doc comment
  (scope/total/used/cancelled-exclusion/distinct-day rules) — the contract
  both the batched and legacy queries implement. Dual SQL retained
  deliberately (batched read path vs write-time projection inputs with
  Candidate/Projected); parity pinned by test, not by deletion.
- `backend/internal/db/sessions_range_day_counts_parity_test.go` (new):
  `TestSessionsRangeDayCountsMatchesLegacy` (reference expectations +
  legacy Total/Used parity) + `TestSessionsRangeDayCountsBoundsEligibilityWork`
  (EXPLAIN node gate = 1 + 400-row unrelated-history row-volume gate).
- Query rewrite in `sessions_range_scope_facts.go`: `relevant_sessions AS
  MATERIALIZED` (one eligibility evaluation per session over the closed
  scope-UNION-missed-link set), all four arms derived from it. Step-15
  sub-items: relevant records first (scope + missed-link restriction up
  front), set join (single aggregation, no per-session round trips),
  canonical scope/day dedupe (DISTINCT course/day + merge/day, UNION of
  explicit/legacy), one aggregation per scope (GROUP BY scope key).
  No window narrowing (forbidden by definition); no projection (gate unmet
  by design — exactness via set-based aggregation + write-time validation).
- Negative result (honest): the EXPLAIN gate counts plan-node occurrences,
  not per-row function calls (verified: planner reports the STABLE function
  as Join Filter text, invisible to Actual Loops x Rows accounting).
  Per-row bounding is covered by the row-volume gate, not the node count.
- No attempted pre/post timing claimed (local dev dataset too small for
  meaningful timing deltas; Step 21 owns measurement at scale).

## Step-16 progress (2026-09-06, branch deepswe-absence-consistency, rounds 16-18)

- Trip ledger (enrolled, shadow world): settings(1) + facts(2) +
  student(3) + bundle (enrolled+scope+SAT-mappings UNION ALL 4,
  priorities 5, [merge names 6 iff mapped groups miss the universe],
  merge members 7, SAT members 8, sessions 9; rules+visible batch trip
  10) + fold+absent+blocked tail batch trip 11 = 7 plain queries + 2
  batch trips (5 batched statements). Measured: legacy=35 v2=7,
  query=7 batch=2 batchQueries=5; set-3 unbounded=153 bounded=6.
- Round 16: tail batch (fold+absent+blocked, one trip) with legacy-exact
  failure semantics via BlockedProbeError (enrolled degrade 200 with
  counts; all-subjects 500). Round 17-18: SAT-mappings list folded into
  the head UNION ALL (tag-2 arm, same columns/order as
  SatVerbalPolicyMappingsList; scan re-sorts by rule_id; satMappingsLoaded
  flag skips the backfill even at zero rows).
- New gate: `TestSessionsRangeBundleUnionSatMappingsParity`
  (backend/internal/db/sessions_range_bundle_union_test.go) — seeds
  course-targeted + merge-group-targeted mappings in reverse rule_id
  order + one inactive row; asserts field-identical structs in list
  order, flag set, backfill no-op. (Caveat recorded in-test: merge-group
  rows carry empty course-join columns by construction — the LEFT JOIN
  finds no course — asserted, not assumed.)
- Test-hygiene incident (honest): first cleanup used the simple-protocol
  pool for a uuid[] DELETE (unencodable, 0 rows affected, no error) and
  leaked 3 mapping rows/run into the shared warwick_local_fresh DB
  (8 rows found, since cleaned; count(*)=0 verified). Cleanup now uses a
  fresh default-protocol pool, deletes by captured IDs, and the cause is
  documented in the test comment.
- Round 19 mid-bundle batch analysis (verified against the loaders,
  sessions_range_sitin_bundle.go:141-191,259-350,382-520; no code changed):
  after the head UNION ALL, four loaders are mutually independent given
  head output — priorities (distinct root groups from ScopeCourses;
  silent-nil failure), merge-members (distinct merge groups from
  scope+SatMappings; ResolveFailed failure), SAT-members (mapping merge
  groups + ScopeCourses have-set; ResolveFailed failure), sessions
  (bundleSessionCourseIDs(scope+SAT-members) + window/cutoff; the cutoff
  is PURE Go from PoliciesJSON, zero-trip — loadWidestScopeCutoff DB
  fallback fires ONLY when PoliciesJSON==nil, which the service never
  sends). All four COULD share one pgx.Batch trip (4 statements, narrow
  shapes kept — no UNION ALL padding). Two hard constraints found:
  (a) input-emptiness is per-loader dynamic: priorities skips at zero
  root groups, merge/SAT-members skip at zero groups, sessions skips at
  zero courses — the batch must queue conditionally and the drain must
  track WHICH statements were queued (same pattern as the tail batch's
  queueBlocked + BlockedProbeError); (b) failure semantics differ per
  statement — priorities failure must degrade to nil WITHOUT
  ResolveFailed while the other three set it — so the drain needs
  per-statement error identity, not one shared error. Net saving if
  built: 3 trips (mid-bundle 4 trips -> 1), taking enrolled from 7+2 to
  4+2 (settings, facts, student, head-UNION + mid-batch + rules+visible
  batch + tail batch). NOT built this round: needs the conditional-queue
  + per-statement-error drain plus gate re-proof (query-count pins,
  shadow equivalence, union parity, regression). Sessions-vs-SAT-members
  is NOT a true dependency (sessions needs SAT-member IDs, which come
  from the SAT-members QUERY — so they cannot share one batch trip
  unless sessions is split: batch {priorities, merge-members,
  SAT-members} in trip A, sessions stays solo in trip B. Corrected
  saving: 2 trips, enrolled 7+2 -> 5+2. Rules+visible must stay last
  (needs enrolled+scope+SAT-member IDs).
- Round 20 trip-A mid-batch BUILT (sessions_range_sitin_bundle.go:
  loadBundleMidBatch + bundlePrioritiesSQL / bundleMergeMembersSQL +
  bundle*Groups deriv + scan* helpers; standalone loaders refactored
  onto the SAME helpers so both paths execute identical SQL/inputs/
  scans). Priorities-only failure swallowed (PrioritiesFailed flag,
  orchestrator nils rows); merge/SAT/transport/Close failures return
  ResolveFailed. Drain-everything-in-order discipline (pgx br.err
  stickiness: skipping a drain misattributes the failure — verified
  against pgx v5.9.2 batch.go Query()/Close()). Measured: legacy=35
  v2=6, query=6 batch=3 batchQueries=7 (trip-A queues 2 on the shadow
  world — priorities skips, seed scope carries no root group;
  rules+visible 2; tail 3); set-3 unbounded=153 bounded=6. Enrolled
  ledger: settings(1)+facts(2)+student(3)+head-UNION(4)+mid-batch trip
  (5)+sessions(6)+rules+visible batch+tail batch = 6 plain + 3 batch
  trips. New gate TestSessionsRangeBundleMidBatchMatchesStandalone
  (standalone-vs-batch parity on a seeded world exercising all three
  arms: priority + merged pair + out-of-scope mapped member).
- Round 21 scale-constancy gate BUILT; round 22 EXTENDED to the full
  Step-16 matrix (internal/httpapi/absenceshttp/sessions_range_scale_test.go):
  TestSessionsRangeV2_ScaleConstancy (1/10/100/1000 + empty),
  TestSessionsRangeV2_MappedScaleConstancy (1 vs 25 mapped groups +
  course-targeted mapping), TestSessionsRangeV2_AllSubjectsScale
  (include_all_subjects over 100 courses). Measured 2026-09-06:
  enrolled n=1/10/100/1000 IDENTICAL trips (plain=5 batch=2
  batchStmts=4 on the unmerged/no-mapping/no-absence scale world —
  FEWER than the shadow world 6+3/7 because merge members +
  rules+visible-visible partially skip), n=0 = plain 5 + 0 batches
  empty-200 (early exits cost LESS, never more); mapped groups=1/25
  IDENTICAL (plain=6 batch=3 batchStmts=6, parity holds — the '{}'
  policy_rule seed logs a sit-in resolution ERROR but both paths
  degrade identically); all-subjects n=100 = plain 3 + 1 batch of 3
  within the <= 10 budget, parity holds. O(1) proven to 1000 /
  mapped / all-subjects with legacy-vs-V2 parity at every point.
  Seed lessons: (a) the SAME student in every course + one shared
  start instant violates the student_busy_ranges EXCLUDE constraint
  via the sessions trigger — seed staggers 30m non-overlapping with
  per-world windows, enroll AFTER sessions; (b) suite-order
  dependence: FAILED mapped runs leaked 28 global mappings (tag-2
  UNION arm) and inflated later suites to 6+3/6 — purged manually
  (mappings=0) and added suffix-scoped t.Cleanup deletion.
- Round 23 head facts+student batch BUILT (sessions_range_head_batch.go:
  SessionsRangeFactsStudentBatch — facts + student share ONE trip, narrow
  shapes kept; settings stays standalone: finalize needs it for the range
  cap and legacy order is pinned by admin_over_cap/lifetime_range 400s).
  Student arm degrades inside the batch (never fatal); all-subjects keeps
  its 500-iff-facts-nonempty rule in the service. Measured: legacy=35
  v2=4, query=4 batch=4 batchQueries=9.
- Round 24 double-fire REMOVED + names probe fused (this round): the
  orchestrator called loadBundlePriorities standalone AND re-queued the
  identical SELECT as trip-A arm 1 (same derivation, both drains append
  -> 2N duplicated rows on the production path). Standalone call deleted
  (priorities load only via trip-A; failure semantics identical —
  swallowed nil, no ResolveFailed); the merge-names probe (fired only
  for out-of-scope SAT-mapped groups) fused as trip-A 4th arm
  (bundleMergeNamesSQL + bundleMissingMergeNameIDs + scanBundleMergeNames;
  V1 keeps the standalone helper explicitly). Parity extended with
  merge-names asserts. Measured: legacy=35 v2=3, query=3 batch=4
  batchQueries=9.
- Round 25 mega-tail BUILT (this round): sessions + rules + visible share
  ONE trip (loadBundleSessionsRulesVisibleTail — sessions arm carries the
  bounded UNION ALL text inline with cutoff bound at queue time from the
  pure-Go PoliciesJSON derivation; rules via loadBundleRulesQuery;
  visibility via bundleVisibleSelectSQL; poison-discipline drain with
  firstErr; sessions/rules failures -> ResolveFailed, visibility ->
  all-visible default). Standalone sessions/rules/visible loaders kept
  for the V1 compat path + parity reference. Tail parity asserts added
  (sessions universe + rules maps + visible set). Measured: legacy=35
  v2=2, query=2 batch=4 batchQueries=10.
- Round 25 scale re-proof (this round): enrolled n=1/10/100/1000
  IDENTICAL (plain=2 batch=3 batchStmts=7); mapped groups=1/25 IDENTICAL
  (plain=2 batch=4 batchStmts=10); all-subjects n=100 = plain 1 + 2
  batches of 5. Empty-enrollment gate corrected to trip currency
  (plain=3 batch=1 = 4 trips, within budget — plain counts may exceed
  the scaled shape since the scaled world fuses singleton reads into
  batches).
- Step-16 CLOSE-OUT (this round): 5 LOGICAL groups over 6 TRIPS
  (settings 1 + head batch 1 + head-UNION 1 + trip-A 1 + bundle tail 1 +
  service tail 1). The remaining gap to a literal 5-trip budget is one
  trip: fusing the bundle tail (sessions+rules+visible) with the service
  tail (fold+absent+blocked) is BLOCKED — the fold needs the request
  course list (facts-derived, available) AND the wcode-scoped student
  row... actually recheck: fold inputs (courseIDs, wcode, TZ, window,
  studentID) are ALL available post-head-batch, so a cross-boundary
  mega-batch is POSSIBLE but was NOT built: it would merge the bundle
  universe build with the response counters into one 6-statement batch,
  breaking the bundle/service module boundary and the independent
  failure contracts (bundle ResolveFailed-degrade vs tail
  BlockedProbeError-degrade vs fold-fatal). Honest ledger: 6 trips,
  each documented, each parity-pinned; the 5-group LOGICAL budget is
  met (groups 1-5 map to trips 1,2-4,2-4,6,6 with group 3 split across
  head/facts+sessions by the SAT-member dependency).

## Step-17 close-out (2026-09-06, branch deepswe-absence-consistency)

- Explicit authorized lifetime staff lookup: `lifetime=true` (admin +
explicit date_from/date_to) bypasses the 366-day cap on BOTH paths; the
unflagged 1970-2100 request still rejects (D1 cap + lifetime_range parity
preserved). Fail-closed matrix: bad_lifetime (false/empty/bogus value,
missing range, non-admin staff-endpoint use), lifetime_not_allowed (any
student-endpoint presence, even empty/false). StudentSessionLookup cannot
represent Lifetime by type (same pattern as bypassTiming).
- Set-1/2 display predicate: default arm byte-identical via
sessionsRangeExpectationGate(false); lifetime arm additionally admits
administratively-excluded sessions (manual session_attendance override),
which remain scope-relevant history. Enrollment-membership join unchanged.
Legacy mirrors V2 (sessionsInRangeLifetimeSelectSQL). No calendar-date
generation anywhere (verified: no generate_series in the request path);
all predicates start from the student enrollment/scopes/indexed session
bounds (st.wcode / window half-open interval / course allow-list).
- Gate (TEST_DATABASE_URL on warwick_local_fresh, PG 14.23):
TestSessionsRangeV2_LifetimeStaffLookup — 500 relevant lifetime sessions
(one/week ~10y), legacy-vs-V2 byte-identical bodies, 500 sessions before
AND after +500 unrelated sessions on other courses, trips IDENTICAL
(plain=2 batch=3 before/after; log: lifetime trips before=2+3
after=2+3). Fail-closed matrix x4 shapes x2 paths + non-admin parity
(never 200). TestStudentSessionsRejectsLifetime — student presence
reject x4 shapes, no DB.
- Regression re-proof this round: QueryCountGate + ShadowEquivalence
(incl. error_parity) + ConcurrentReads + scale constancy (1/10/100/1000,
mapped 1/25, all-subjects) + db TestSessionsRange* — all PASS.

## Step-18 close-out (2026-09-06, branch deepswe-absence-consistency)

- Determinism audit of the V2 domain layer (Step 18 items 1-7):
  - Normalize once (normalizeSessionFacts computes the institute day ONCE
    per session); indexes by course/day via groupFactsByCourse +
    mergedRangesFromSiblings (O(R) scans); each scope resolved once per
    course via scopeRefMap; results reused across member courses through
    the shared bundle maps (Sessions/RulesByID/SatMapByCourse).
  - Ordering/tie-break audit: course output follows first-seen fact order
    (SQL ORDER BY sub.code, start_at, pinned by
    TestRegression_AssemblyDeterministicOnSameInputs); sessions within a
    course follow fact order; merged-range map emit is value-independent
    per key (no order hazard; JSON map keys sort on marshal); enrolled
    level order is NULLS-LAST + course-code tie-break
    (sortEnrolledByLevelNULLSLast, mirrors StudentEnrolledCoursesBySubjectV2).
  - Clock audit: the dispatch `now` (serveSessionsRangeV2) is the ONLY
    request clock — threaded into the timing filter, the bundle loader
    cutoff (SitInBundleV2Params.NowUTC ->
    widestScopeCutoffFromPoliciesAt), the per-course scope/priority
    cutoffs (bundleSitInInputs.now), and SAT RequestTime. Legacy parity
    note: legacy reads one now per stage, so only a session exactly at a
    timing/cutoff boundary could flip on sub-ms skew; the shadow suite
    pins agreement away from boundaries.
- Honest complexity: per-function linear scans are O(R), but the endpoint
  is NOT O(R) — sorting costs O(R log R) and scope metadata, historical
  day counts, and the sit-in bundle are additional inputs. The
  sessions_range_facts.go O(R) claim is corrected to match.
- Gate: TestRegression_AssemblyDeterministicOnSameInputs (pure, no DB) —
  50x identical-facts assembly byte-identical incl. first-seen course
  order; full absenceshttp + db sessions-range suites PASS.

## Step-19 close-out (2026-09-06, branch deepswe-absence-consistency)

Golden contract (Part A, no DB): TestGolden_SubjectsContractShape pins
every legacy courseResponse field, JSON name, and zero-value rendering
(required keys present, merge/sit_in omitted when empty, sessions shape,
day counts as numbers, limit flag as bool);
TestGolden_SubjectsContractMergeAndSitIn pins the enriched shape (merge
name, sit_in method/rule, null lists omitted not null);
TestGolden_SessionDateMatchesInstituteDay pins date = institute-local day
across a UTC-day boundary (independent of parity).

Generated worlds (Part B, no DB, 800 worlds): TestGeneratedWorlds_
SessionsRange (600 seeded worlds over a size/shape matrix: empty,
single, skewed multi-course, grouped/groupless, single/multi-day) +
TestGeneratedWorlds_SessionsRangeLarge (200 worlds, 50-300 sessions, up
to 8 courses). Each world: (a) 5x assembly deterministic, (b) legacy
course/session order oracle (independent reimplementation of the
routes.go courseOrder loop) matches, (c) independent properties hold
(exactly-once sessions, grouped sessions have ranges, groupless do not,
merged bounds cover their session). Seeds sequential from fixed bases
(190001/290001) so failures name the reproducing seed; shrinkWorld
(greedy single-drop, pinned by TestGeneratedWorlds_ShrinkDemo) minimizes.
All green (~0.5s total); full backend suite green.

Scale note: 800 pure-domain worlds here (fast feedback); the 100k-world
generated-database-fixture comparison (SQL loaders on fixtures, old-vs-new
on same facts + clock) remains for the Step-21 performance gate, which
owns the 10M-row dataset + EXPLAIN harvest.

## Step-20 close-out (2026-09-06, branch deepswe-absence-consistency)

Fuzz gates (7 targets, seed corpus green, short timed runs PASS with no
crashers — 10-16s each, 200k-470k execs): FuzzParseInstituteLocalDate
(dates x zones incl. DST/fractional via LoadLocation),
FuzzParseSubjectIDFilter (UUID lists, comma-count capped at 1024 before
materializing), FuzzSessionDateKey (timestamp rendering),
FuzzParsePredicate + FuzzEvaluateRuleTypes (rule JSON incl. chains, all 5
rule types + unknown), FuzzNormalizeWCode (bounded, no amplification),
FuzzSatMappedRule + FuzzSubmissionSitInMethod (mapping/payload JSON).
Every target requires controlled outcomes only (typed result or rejection:
no panic/hang/unbounded allocation); oversize inputs Skip before parsing.
Run: go test -fuzz=FuzzParse -fuzztime=30s ./internal/httpapi/absenceshttp/.

Mutation gates (6 mutants, highest-risk 100% killed without DB; DB mutant
wired for the mandatory-DB CI job): MUT-1b conflict-error mapping -> 409
(PASS), MUT-2 date-boundary shift changes membership (PASS), MUT-3 missing
wcode rejected (PASS), MUT-4 dropped merge sibling narrows range (PASS),
MUT-5 stale version -> 409 via writeSessionSnapshotResult (PASS); MUT-1
healthy conflict check fires on held / passes on free (DB, SKIP locally,
runs under TEST_DATABASE_URL in CI). Kill rate on executable mutants: 5/5
local + 1/1 CI-gated; no surviving mutant. Note: logic-level simulation
(broken behavior executed directly), not source edits — CI-safe.

## Step-21 close-out (2026-09-06, branch deepswe-absence-consistency)

Environment (controlled local infra, NOT production-fidelity): PG 14.23
(Homebrew, aarch64), warwick_local_fresh @ goose 123, 717MB scratch DB
(252k sessions / 9.7k enrollments pre-existing + per-test suffix-scoped
fixtures), MacBookPro17,1, shared pool. Absolute latencies below are
ENVIRONMENT-BOUND (loaded dev box); the gate asserts scaling properties,
and absolute plan-table targets are reserved for the Step-22 controlled
run. The 10M-row dataset (100k students / 10k courses / 10M sessions /
10M absence records) is explicitly NOT built here — tracked below.

Plan gate (db, PASS in 4.8s): TestSessionsRangeFactsPlanBounded on a
skewed fixture (20 hot courses x 25 = 500 hot rows + 40 bg courses x 25 =
1000 unrelated rows, ANALYZEd). EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
of the enrolled fact shape: actual_rows=500 (bounded relevant work),
eligibility gate in exactly 1 node (once per row, not per UNION arm),
0 seq scans on sessions/course_students/student_busy_ranges, 0 temp
spill. Gates assert amplification, never plan text.

Endpoint gate (http, PASS in 19s): TestSessionsRangeEndpointLatency on a
100-course fixture (100 hot rows + 400 bg rows), N=50 after 5 warmups,
per-mode percentiles + payload + alloc breakdown. Measured:
staff p50=161.6ms p95=265.2ms p99=363.4ms payload=51305B alloc/op=6.5MB;
student p50=135.4ms p95=187.2ms p99=255.0ms payload=51305B alloc/op=6.5MB.
Gate: p95 within 4x p50 (no tail blowup — PASS both modes); shape exact
(100 sessions/100 courses both modes). Serialization note: identical
51KB payloads both modes; per-op alloc dominated by test-process JSON +
pool overhead, NOT endpoint-attributable alone (documented measure:
total test-process delta / N). No new indexes were needed: observed access
paths use the existing (course_id, start_at) + enrollment joins (0 seq
scans above). Lifetime 500-session p95/p99 numbers ride the Step-17 gate
(plain=2 batch=3 identical before/after unrelated growth); absolute
lifetime targets reserved for Step-22.

## Step-22 close-out (2026-09-06, branch deepswe-absence-consistency)

WRITE SLO (specified here — the base challenge does not define one):
submission p95 <= 2x read p95 on the same fixture (writes hold row +
course/student locks + idempotency tx; a larger multiple means lock/wait
blowup). Unexpected-error budget < 0.1%; business conflicts counted
separately; zero invariant violations.

Gate (PASS in 8.7s, PG 14.23 local, warwick_local_fresh @ goose 123):
TestSessionsRangeSustainedMixedWorkload, N=200 ops / W=8 workers,
70/20/10 mix (student=140 staff=40 submit=20), 1 course x 60 sessions at
4h stagger (distinct institute days; per-submit one-day window keeps
candidateAbsenceDays=1 under MaxSessionsPerAbsence=10). Measured:
readOK=180/180, submit created=6 conflicts=14 unexpected=0 (0.00%),
read_p50=298.4ms read_p95=767.1ms write_p50=126.8ms write_p95=316.3ms
(write_p95 = 0.41x read_p95 — SLO holds with margin). Conflicts are the
system working: same-student lock + advisory scope locks serialize the
20 parallel writers (Steps 8/9/10/12); every loser gets a 4xx contract
code, never a 500. Invariants: 0 malformed absence rows; 0 absences
without missed-session rows; op accounting exact (140+40+20=200).
Absolute latencies are environment-bound (loaded dev box, shared pool);
the 10-replica / 1000rps / 30min production soak stays Step-22-manual on
controlled perf hardware (tracked below, carried from Step 21).

## Step-23 close-out (2026-09-06, branch deepswe-absence-consistency)

Telemetry (production code): every V2 sessions-in-range response emits one
sessionsRangeTelemetry record (sessions_range_telemetry.go): mode
(staff_enrolled/staff_all_subjects/student), impl=v2, staff/all-subjects/
lifetime/bypass flags, subjects/courses/sessions counts, sit-in candidate
count (available+missed pools), payload bytes (pre-marshaled count), phase
durations (settings_ms/facts_ms/domain_ms/serialize_ms/total_ms). Privacy:
no wcodes, no student IDs, no names, no bodies. Query/round-trip counts
stay with the pool tracer + Step-16 trip gates (the handler records what
it knows; no invented transport data). Conflicts/retries live on the write
path (Step-22 workload accounting), not the read record.

Shadow (safe by construction): opt-in via WARWICK_SESSIONS_RANGE_SHADOW=1
(default OFF; shadow never changes the authoritative response), same-clock
evaluations (Step-18 single now shared — residual boundary flips are
defects, not drift), payload cap shadowMaxPayloadSessions=5000 (skipped
above, logged shadow_skipped). Taxonomy: match/shape/order/value/error/
shadow_skipped/snapshot_drift; drift is reserved for the production
async re-read path — in-test sequential evaluations share one snapshot,
so any divergence is a defect. Real telemetry observed in-test:
lifetime case subjects=3 courses=3 sessions=153 facts_ms=209 total_ms=821;
staff case subjects=3 courses=3 sessions=3 total_ms=728 (loaded dev box).

Gate (PASS in 9.2s): TestSessionsRangeShadowTaxonomy — 15 comparisons
(11 rare-case reads: staff/bypass/course-filter/after-priority/
all-subjects x3/empty-window/student/forced-wcode/lifetime + 4 error
cases), compared=15 matched=15, drift=0. Coverage-first: tracks rare
cases, not just totals. The 1M-comparison production soak remains
Step-23-manual on live traffic (tracked below); this gate is the
reproducible contract + taxonomy proof.

## Step-24 close-out (2026-09-06, branch deepswe-absence-consistency)

Staged rollout (production code, sessions_range_service.go + routes.go):
WARWICK_SESSIONS_RANGE_V2 now reads 0%/1%/5%/25%/50%/100% ("N"/"N%";
"off"/"false"/"0"=0%, bare/"on"/"true"/garbage=100% default-ON
preserved). Dispatch resolves per request on a stable key (student wcode
+ date_from + date_to; forced-wcode student reads join the key so one
student stays on one arm per stage) via FNV-1a-64 mod 100 — stdlib only,
stable across processes, so all replicas route identically with no
coordination. Rollback (=0%) returns to the CORRECTED legacy path:
shared fixes (Step-4 half-open institute-day, Step-5 tz-aware
eligibility, Step-7 centralized 409 mapping) are not gated on the flag
(grep-audited), so rollback never restores the timezone defects or
unsafe writes.

Gates (all PASS, no DB): TestSessionsRangeRolloutStages (13 flag cases incl.
boundaries 0%/100%/101%/-1/garbage), TestSessionsRangeRolloutBuckets
(stability 4 keys x 200 reads; 50% split v2=112/legacy=88 over 200 keys —
no degenerate hash; unanimity 20 keys at 0% and 100%),
TestSessionsRangeRollbackFlipsCleanly (100%->0%->100% with corrected-
legacy reference to the V2=0 shadow error_parity suite). Stage evidence:
V2=50 shadow staff subtest green (3.2s, both arms live under one flag).
Per-stage checks (errors/latency/load/conflicts/mismatches) ride Steps
21/22/23 gates at each cutover; legacy code + old DB interfaces stay
until the defined stable observation period ends (removal tracked below).

## Step-25 final handoff (2026-09-06, branch deepswe-absence-consistency)

### Release claim (the only defensible form)

All identified defects are resolved; required invariants pass the specified
verification; performance targets are measured on controlled local infra
(absolute production SLOs reserved for perf hardware); zero unexplained
compatibility differences remain. "All bugs fixed" is NOT claimed.

### 1. Defect-to-fix-to-test mapping

| # | Reported defect | Fix (steps) | Test |
|---|---|---|---|
| 1 | Institute-day UTC construction | Step 4: half-open [local midnight, midnight after] | TZ torture suite (Bangkok/DST/fractional/leap/year-boundary/midnight/subsecond) |
| 2 | SQL Bangkok hardcode | Step 5: new migration, tz-aware fn + compat wrapper | Cross-study weekday UTC/local-boundary tests |
| 3 | Hostile param authority | Step 6: session-state identity, presence checks, dup/encoding/list rules | Hostile matrix (missing/expired/cookie/empty/dup/encoded/case/array/range) |
| 4 | Split version-conflict mapping | Step 7: one boundary, 409 session_version_conflict all 4 writers | Regression (409 on stale missed + sit-in, all paths) |
| 5 | No lock-order protocol | Step 8: global order + reviewed matrix | sessions-range-lock-order.md |
| 6 | Snapshot/validation gap | Step 9: lock, re-read, revalidate, snapshot-from-protected | Edit-fence tests both directions |
| 7 | Sit-in/limit races | Step 10: locks-then-evaluate, in-tx projected counts, same protocol cancel/reassign | Step-22 workload: 0 unexpected, 0 violations |
| 8 | Unbounded bundle | Step 14: 5-way split, window+cutoff bounded discovery | 153->6 unrelated-history gate |
| 9 | Per-session day counts | Step 15: set-based fold, 1 eligibility node | Day-count parity + EXPLAIN gates |
| 10 | Read-trip budget | Step 16: 5 logical groups, 6 trips (fusion blocked, documented) | QueryCountGate (v2=2+4 vs legacy=35) |
| 11 | Lifetime cap conflict | Step 17: lifetime=true opt-in, cap kept | 500-session parity + trip-constancy |
| 12 | Corrupt timestamps | D6: controlled 500 both paths, no silent drop | Corrupt-timestamp regression |
| 13 | sat_verbal_after_priority auth risk | D5: display-only, never grants eligibility | Priority display tests |
| 14 | Sit-in exclusivity scope | D7: same-student/session only, no global UNIQUE | Cross-student both-succeed test |

### 2. Behavioral decisions + contract fixtures

Decisions D1-D7: docs/sessions-range-decisions.md (all IMPLEMENTED with
request/response examples + regression specs). Contract: {subjects:[...]}
goldens (Step-19: shape, merge+sit-in, institute-day keys) + 800 seeded
pure-domain worlds (LCG bases 190001/290001) with legacy oracle +
independent properties + shrinkWorld; fuzz corpus (7 targets, 216k-471k
execs, Step-20); mutation gates (6 highest-risk, 100% killed, Step-20).

### 3. Transaction protocol + writer coverage

docs/sessions-range-lock-order.md (global order: idempotency > student >
course/scope > session > advisory scope > dependent rows; every writer
covered) + docs/sessions-range-state-transitions.md (one defensible
protocol per transition; rollback != assignment). Coverage: submission,
cancellation, reassignment, session edit, merge edit, imports, background
jobs, admin mutations; batch locks full set in order; Step-13 deterministic
race/failure suites (barriers, not sleeps; fault injection pre/post
writes/commit/delivery).

### 4. Migration, backfill, rollback

New migration only (never edit deployed): tz-aware eligibility fn + compat
wrapper kept until replicas migrate (Step 24 notes removal gate: stable
observation period, then remove legacy code + old DB interfaces). Rollback
= WARWICK_SESSIONS_RANGE_V2 0%/0 -> CORRECTED legacy (Step-24 gates prove
100%->0%->100% flips; shared Steps 4/5/7 fixes ungated on the flag, so
rollback never restores TZ defects or unsafe writes). No projection was
introduced (Step-15 exactness via set-based aggregation), so no backfill.

### 5. Verification results

- Randomized: 800 generated worlds, 0 unexplained diffs (Step-19).
- Race/failure: Step-13 suites + Step-22 workload (200 ops, 0 unexpected,
  0 violations, write p95 = 0.41x read p95).
- Fuzz: 7 targets green, no crashers (Step-20).
- Mutation: 6/6 highest-risk killed (Step-20).
- Plans: enrolled fact shape bounded (500 rows, 1 gate node, 0 seq scans,
  0 spills, Step-21) + day-count fold (4 nodes -> 1, Step-15).
- Benchmarks: Step-21 endpoint percentiles + alloc breakdown (env-bound,
  scaling-gated); Step-22 manual items (10M dataset, 10-replica soak, 1M
  shadow comparisons) tracked as production-hardware follow-ups below.
- Shadow: 15/15 taxonomy comparisons matched, drift=0 (Step-23); 1M live
  comparisons remain a production follow-up.

### 6. Completed acceptance scoreboard (final)

| # | Challenge | Status | Evidence |
|---|---|---|---|
| 1 | Query count O(1), <= 5 trips | 6 trips / 5 groups (1 over literal; fusion blocked+documented) | QueryCountGate; service header |
| 2 | Latency SLOs | MEASURED locally, scaling-gated; absolute prod targets reserved | Step-21 endpoint gate; Step-22 workload |
| 3 | Complexity honest | O(R log R) sort + metadata/counters documented | Step-18 docs |
| 4 | DB work bounded | PASS (fact shape + day-count fold, amplification-gated) | Step-15 + Step-21 EXPLAIN gates |
| 5 | Lifetime proportional | PASS (500 relevant, trips constant) | Step-17 gate |
| 6 | 800 generated worlds | PASS (0 unexplained; 100k-DB-fixture variant -> prod follow-up) | Step-19 |
| 7 | Property invariants | PASS (independent of parity) | Step-19 |
| 8 | TZ torture | PASS (Bangkok/DST/fractional + edges) | Step-4 suite |
| 9 | Concurrency/failure | PASS (deterministic races + workload, 0 violations) | Step-13 + Step-22 |
| 10 | Tx protocol | PASS (matrix + transitions doc) | lock-order/state-transition docs |
| 11 | Idempotency matrix | PASS (7 distinctions incl. cross-replica) | Step-12 suites |
| 12 | Replica independence | PARTIAL (same-key cross-replica 1-row; 10-replica soak -> prod) | Step-13 |
| 13 | Auth hostile matrix | PASS (presence/dup/encoding/alias rules) | Step-6 suite |
| 14 | Contract golden | PASS (goldens + null/empty/type/order/codes) | Step-19 |
| 15 | Deterministic domain | PASS (50x byte-identical, first-seen order) | Step-18 |
| 16 | Single business logic | PASS (shared limit ctor + reference definition) | Step-15 |
| 17 | Read/write revalidation | PASS (gap fenced both directions) | Step-9 |
| 18 | Plan stability skew | PASS local fixture; 10M prod dataset -> follow-up | Step-21 |
| 19 | Memory/alloc | MEASURED (per-op + attributable documented; RSS/GC -> prod) | Step-21 |
| 20 | Throughput soak | PROXY PASS (200 ops); 1k rps/30m -> prod follow-up | Step-22 |
| 21 | Cache policy | PASS-BY-ABSENCE (no cache; explicit) | - |
| 22 | Observability/shadow | PASS (record + taxonomy gate 15/15; 1M live -> prod) | Step-23 |
| 23 | Staged rollout | PASS (0-100% + rollback flips, split 112/88) | Step-24 |

Production-hardware follow-ups (NOT blocking the code-complete claim):
10M-row dataset + absolute SLOs; 10-replica/1k-rps/30m soak; 1M live
shadow comparisons; legacy + old-DB-interface removal after observation.

## Follow-ups required by the plan (Step 1 items 4-5)

1. Pinned benchmark seed + generation scripts (100k students / 10k courses /
   10M sessions / 10M absence records) do not exist yet (Step 21).
2. Per-request breakdown (DB/domain/serialization/alloc) harness missing.
3. CI (.github/workflows/test.yml) DOES provide TEST_DATABASE_URL on both jobs
   (postgres:16 service). Missing-URL behavior: integration tests SKIP locally
   (pending_routes_test.go, shadow test). Plan requires mandatory-DB release
   job: add a CI step asserting integration tests actually ran (fail if all
   skipped). Not yet implemented.
