# Implementation plan: absence sit-in eligibility and legacy schedule tombstones

## Implementation status — 2026-10-10

The implementation phases for candidate parity, form precheck, parser completeness, and guarded schedule observations are complete in the working tree. The selected removal policy is two distinct complete missing observations with a 24-hour grace period; generations come from a PostgreSQL sequence allocated before source fetch. Local PostgreSQL migration and targeted integration tests pass, as do the frontend checks and the Lean model. The broader `internal/db` suite has unrelated failures on the populated local app database; details and exact results are in [`PROOF_REPORT.md`](../../verification/absence-sit-in-legacy-sync/PROOF_REPORT.md).

Production rollout, verification of current source page variants, and the targeted refresh for schedule `113373` remain pending. No production database write was made.

## 0. Context and baseline behavior

- **Goal:** Make the make-up choices shown to a student consistent with the final server validation, and prevent a legacy schedule row from being soft-deleted on a stale, incomplete, or unconfirmed source observation.
- **Baseline → implemented:** Before this change, the form could offer a make-up that submission later rejected; the legacy importer could deactivate a local session when one fetched aggregate omitted its source ID. Candidate paths now use matching conflict scope, and sync deletion now requires trustworthy, ordered evidence that the source row is gone.
- **Repository:** commit `31733170966de2978484f2e0dbd0d1f5853c22f4`.
- **Verified absence evidence:** `collectAttendingSessions` skips every selected subject in `src/features/absences/domain/submissionPayload.ts`; the SAT-Verbal candidate rule checks the missed sessions passed to it in `backend/internal/httpapi/absenceshttp/sat_verbal_policy.go`; final submit validation checks active candidates against expected sessions in the absence course or its merge group, for every institute-local date from `date_from` through `date_to`, in `backend/internal/db/absence_management_custom.go`. Selecting Oct 10 and Oct 17 makes that window Oct 10–17. The Oct 13 option, 13:00–16:20, overlaps expected sessions from the same merge scope at 13:00–14:40 and 14:40–16:20. The server rejects it inside the create transaction; the generic error combines inactive and overlap cases.
- **Baseline sync evidence:** `syncCourse` fetched and parsed a course detail page before calling both appliers. The old parser accepted an explicit empty schedule page and skipped blank-date rows; the old `deactivateMissingSchedules` path soft-deleted active legacy sessions omitted from one aggregate. The applier used a per-course advisory transaction lock, but the source fetch happened before that lock. The sync runner has configurable concurrency.
- **Database evidence from the earlier read-only check:** legacy schedule ID `113373` (session `9cd1faa8-c62b-44fd-91c1-42bc2080d2bd`, Oct 14 17:00–20:20) was soft-deleted with `change_source='legacy_sync'`; its external mapping is tombstoned and its recorded `last_seen_at` predates the deletion. The user checked the source and reports that the schedule is still available there. This establishes a source/local disagreement. The fetched page from the deletion run is not available in the evidence reviewed, so whether the omission came from a transient/incomplete response, parser behavior, or an out-of-order refresh remains **unknown**.
- **Scope:** student-facing candidate generation, client pre-submit validation, authoritative submit validation/error, and legacy schedule observation/deactivation/recovery.
- **Non-goals:** changing absence eligibility rules, changing the merge-group definition, deleting historical session rows, redesigning legacy sync, or writing to the production database as part of this plan.

## 1. Requirements

### Functional

| ID | Priority | Behavior | Acceptance criterion |
|---|---|---|---|
| FR-1 | P0 | Candidate options are filtered using the same active-session and expected-class conflict scope as submit validation. | For Oct 10 + Oct 17 selected, the Oct 13 13:00–16:20 candidate is unavailable because it overlaps the two expected merge-scope sessions. A non-overlapping candidate remains available. |
| FR-2 | P0 | Submit remains authoritative for stale or forged client selections. | The same conflicting session submitted directly is rejected; the absence and sit-in association do not commit. A deleted candidate is rejected as inactive. |
| FR-3 | P0 | A source schedule is deactivated only after a complete, authenticated, parser-valid, current observation confirms it is missing under the configured confirmation policy. | One incomplete or out-of-order omission leaves the local session active. Two qualifying missing observations past the approved grace period may soft-delete it and tombstone its mapping. |
| FR-4 | P1 | A schedule seen again at source recovers local state. | A newer source-present observation clears missing progress, restores `deleted_at=NULL`, and reactivates its mapping. |
| FR-5 | P1 | The form precheck explains a known schedule conflict. | The client reports a conflict-specific message before submit; the existing authoritative server error shape remains compatible for stale or forged selections. |

### Relevant non-functional requirements

| ID | Constraint | Target / validation |
|---|---|---|
| NFR-1 | Conflict correctness | Candidate list and final validation use one defined scope: sessions the student is expected to attend in the absence course or its merge group, within the inclusive institute-local date range; time ranges use half-open overlap semantics. Verify with Go/SQL integration tests and the Lean abstraction. |
| NFR-2 | Sync ordering and durability | Apply source observations monotonically per course. State transition, session soft-delete, mapping state, and audit event commit atomically. Verify with deterministic concurrent integration tests against PostgreSQL. |
| NFR-3 | Failure safety | Transport/auth/parser/incomplete-generation failures cannot advance a schedule toward deletion. Verify with parser and applier tests. |

No latency or production SLO is proposed; inspect the current bundle/query workload and measure any new query before rollout.

## 2. Core entities and invariants

- **Sit-in candidate:** an active session proposed to one student for an absence scope. The server owns eligibility; browser state is advisory.
- **Conflict scope:** expected sessions from the missed course or any course in the same merge group whose institute-local dates fall between the absence `date_from` and `date_to`.
- **Schedule observation:** a fetched, authenticated, parser-valid snapshot associated with a monotonic per-course generation. The legacy site remains the source of truth; the imported aggregate is only evidence if completeness checks pass.
- **Legacy schedule mapping:** `external_refs` relation from `(source, entity_type='schedule', external_id)` to the preserved local `sessions` row.

Invariants:

- **INV-1:** Every displayed/selectable sit-in candidate is active and has no strict interval overlap with any expected session in its conflict scope.
- **INV-2:** The final submit check rejects any candidate that violates INV-1, even if the browser sends stale state.
- **INV-3:** A failed, incomplete, or older observation cannot newly advance a schedule to soft-deleted/tombstoned.
- **INV-4:** A newer complete observation that contains the schedule clears missing progress and restores the local row and mapping.
- **INV-5:** Soft deletion preserves the session row and historical references; the current audit trigger continues to record the source as `legacy_sync`.

Boundary semantics: dates are computed in the configured institute timezone; date endpoints are inclusive. Time overlap remains `candidate.start < expected.end && candidate.end > expected.start`, so exactly touching sessions do not conflict.

## 3. API / interface changes

- Keep the existing sessions-range response shape where possible; candidate availability/reason fields are internal policy output already represented by available/unavailable session collections.
- Use the same conflict policy for candidate generation, the existing local form precheck, and final submit. Do not weaken the server check.
- The form precheck now returns a conflict-specific message. The server retains its existing `invalid_sessions` response for stale or forged selections so this change does not alter the API contract.
- Add internal schedule-observation metadata to the legacy apply request: monotonic generation and completeness/auth/parser status. Do not let a caller-supplied browser field determine this metadata; it comes from the sync client/parser pipeline.
- The schema change is additive. Keep old application versions safe during rollout by pausing/draining legacy sync writers while code that enforces the observation guard is deployed, unless deployment inspection proves a DB-level guard covers old writers.

## 4. Critical data flow

### Absence flow

`sessions/range → bundle-backed resolver → candidate options → browser selection → payload precheck → transactional absence create → ValidSitInSessionOverlap → sit-in snapshot`

Before this change, candidate resolution checked overlap against the passed missed-session list and the browser precheck excluded the selected subject entirely. The transactional check considered all expected sessions in the absence course/merge group across the date window and rejected the Oct 13 choice. The transaction rollback prevented a partial absence. Candidate resolution and the form precheck now include the same-course/merge-group expected-session conflicts.

### Legacy schedule flow

`job claim → FetchSchedulePageContext → ParseCourseDetail → CourseApplier.Apply → ScheduleApplier.Apply → per-course advisory lock → session upserts/deactivation + mapping state + snapshot → transaction commit`

The lock serializes database application, not the preceding source fetch. Before this change, a slow older fetch could reach apply after a newer fetch without a generation comparison, and one accepted omission could soft-delete and tombstone a row. Sync now assigns a database sequence generation before fetch, rejects older applies under the lock, and requires two missing observations plus a 24-hour grace period. A later source-present aggregate still restores a row only when another refresh runs.

## 5. Recommended design

### A. Use one eligibility rule at the backend boundary

1. Define the blocker set once from the server-owned student/scope/date facts: expected sessions in the missed course or merge group between `date_from` and `date_to`.
2. Feed that blocker set into each active candidate builder, including the bundle-backed SAT-Verbal path and any remaining query-backed path. Before this change, `SitInCandidateSessions` SQL only checked the absence course; it now excludes candidates overlapping expected classes in the same merge group, matching final validation.
3. Keep `ValidSitInSessionOverlap` in the create transaction as the final guard. Prefer extracting/reusing the SQL predicate or materializing the same expected-session set in the existing bundle; avoid one query per candidate.
4. Change the form precheck to evaluate the same-scope sessions rather than dropping all groups whose subject is selected. Preserve the server rejection for stale client state. Return an explicit conflict reason.

Affected owners: `backend/internal/httpapi/absenceshttp/sessions_range_resolve.go`, `sessions_range_mapped.go`, `sat_verbal_policy.go`, `backend/internal/db/absence_management_custom.go`, `src/features/absences/domain/submissionPayload.ts`, and their existing tests. Confirm whether V1, staff-create, and direct candidate-query callers share the same rule before editing; several submit paths already call `ValidSitInSessionOverlap`.

### B. Gate legacy deletion on trustworthy ordered evidence

1. Make parser completeness explicit. Accept an empty schedule set only when the known “No schedules yet” marker and the full table contract are present. Reject unexpected blank-date/colspan rows rather than silently interpreting them as an empty or smaller set. Keep valid non-empty pages all-or-nothing.
2. Allocate a monotonic per-course observation generation before fetching. Carry it with the parsed result. Under the existing per-course transaction lock, reject any apply older than the last successfully applied generation. Do not use the timestamp assigned after fetch as an ordering token.
3. Track missing observations using the existing `suspected_missing` / `confirmed_missing` / `tombstoned` state vocabulary. This implementation persists generation/count/time on `external_refs` with additive migration 00130 rather than delegating schedule completeness to the separate reconcile helper.
4. First qualifying omission marks the mapping suspected-missing but leaves the session active. Tombstone only after two consecutive complete observations and the approved grace period. A source-present observation clears the counter and restores session + mapping. Parser/auth/incomplete results do not count.
5. Keep the session soft-delete, mapping state, missing-observation state, snapshot, and audit changes in one transaction. Preserve the per-course advisory lock and snapshot hash behavior.

Affected owners: `backend/cmd/legacy-sync/syncer.go`, `backend/internal/legacysync/parser/course_detail.go`, `backend/internal/legacysync/apply/course.go`, `backend/internal/legacysync/apply/schedule.go`, `backend/internal/legacysync/reconcile/state.go`, and an additive `backend/db/migrations/` migration if required. The exact generation allocator/storage location is an implementation decision to verify against job queue and schema; the required property is a monotonic token minted before source fetch and checked at apply.

## 6. Production and failure-mode deep dives

| Risk | Consequence | Protection / owner | Verification | Residual risk |
|---|---|---|---|---|
| Candidate resolver and submit use different scopes | Student selects an invalid make-up, then gets a generic submission error. | Shared blocker set and exact merge/date scope; backend resolver plus transactional final validation. | Oct 13 same-subject, merge-group fixture; direct forged submit remains rejected. | Schedule/enrollment changes after range load can stale the UI; final server guard remains required. |
| Candidate is deleted between range load and submit | Stale option reaches submission. | Active check at submit; distinguish inactive from overlap in error. | Delete after range response, then submit. | User must reload/select another candidate. |
| Parser accepts a partial/empty-looking schedule table | A live source schedule may be soft-deleted locally. | Strict parser completeness and repeated confirmed-missing policy; `deactivateMissingSchedules` only receives deletion candidates from verified generations. | Empty marker, malformed rows, omitted-row fixtures, parser/auth failures. | A consistently wrong upstream representation can still pass a syntactic contract; monitor source row counts and reconcile against source. |
| Older source fetch applies after a newer fetch | Newer active schedule can be tombstoned by stale aggregate despite the DB advisory lock. | Monotonic generation minted before fetch and compare-and-apply under the per-course transaction lock. | Deterministic barrier: newer-present apply commits, then older-missing apply is rejected. | Requires generation identity to be shared across all sync workers/instances; deployment ownership is UNKNOWN and must be checked. |
| Failure during DB apply | Session, mapping, and snapshot disagree. | Keep all related writes in the existing transaction; fail/rollback on any query or commit error. | Inject faults before commit and between state writes; verify no partial state. | Commit acknowledgment loss remains an operational ambiguity; retries must be idempotent by generation/hash. |
| Corrected source row reappears | Tombstone may remain indefinitely if refresh is skipped. | Present observation clears missing state, upserts/undeletes row, reactivates mapping; explicit per-course refresh for repair. | Reappearance integration test and post-repair SQL assertions. | Refresh cooldown can delay recovery; inspect the current refresh behavior during rollout. |

## 7. Ordered implementation phases

| Phase | Files / components | Specific change | Requirement / invariant | Done when |
|---|---|---|---|---|
| 1. Pin behavior | Existing absence tests and legacy parser/applier tests | Add failing regressions for Oct 13 same-scope overlap, parser partial/empty response, one missing generation, source reappearance, and stale generation ordering. | FR-1–FR-4, INV-1–INV-4 | Each regression reproduces the current failure or pins the expected contract. |
| 2. Backend candidate policy | `sessions_range_resolve.go`, `sessions_range_mapped.go`, `sat_verbal_policy.go`, `absence_management_custom.go` | Build/use the exact expected-session blocker set in every candidate path; keep final transactional validation. | FR-1–FR-2, INV-1–INV-2 | Both candidate response and direct submit agree on conflict, including merge-member sessions. |
| 3. Form response | `submissionPayload.ts` and `submissionPayload.test.ts` | Stop skipping the selected subject wholesale; compare against same-scope sessions (or rely on explicit server blocker data) and show conflict-specific copy. | FR-1, FR-5 | The form rejects locally before submit, while no-conflict and adjacent-time cases pass. |
| 4. Parser and observation contract | `course_detail.go`, `syncer.go`, `course.go`, `schedule.go`, `reconcile/state.go` | Require explicit completeness, assign generation before fetch, persist missing progress, reject stale apply, and gate soft-delete on confirmation. | FR-3–FR-4, INV-3–INV-5 | No invalid/incomplete/stale observation can tombstone; reappearing source row recovers. |
| 5. PostgreSQL integration and recovery | Existing applier integration tests; additive migration if required | Verify atomic session/mapping/snapshot transitions, advisory-lock interleavings, and repair of schedule `113373` via a successful current source refresh. | NFR-2–NFR-3 | Database state and audit event match each expected outcome. |
| 6. Rollout | legacy-sync deployment and student absence client | Pause/drain legacy writers, apply additive schema, deploy guarded sync code, run shadow/targeted source refresh checks, resume sync, then deploy the UI parity change. | All | No old writer can bypass the guard; error/conflict and tombstone metrics stay explainable. |

## 8. Test matrix

| Case | Setup / stimulus | Expected persisted + external outcome | Level | Coverage |
|---|---|---|---|---|
| TC-1 | Oct 10 and Oct 17 missed; Oct 13 candidate 13:00–16:20; two expected same-merge sessions cover 13:00–16:20. | Candidate is unavailable with conflict reason; it is not selectable. | Go resolver + API/e2e | FR-1, INV-1 |
| TC-2 | Submit TC-1 candidate directly with valid IDs. | HTTP rejection; absence/sit-in rows roll back. | PostgreSQL integration | FR-2, INV-2 |
| TC-3 | Candidate ends exactly when expected class begins, or starts when it ends. | Candidate remains eligible under half-open intervals. | Unit + PostgreSQL integration | NFR-1 |
| TC-4 | Candidate is soft-deleted after range response, before submit. | Submit rejects with the existing `invalid_sessions` response and no partial absence commits. | API integration | FR-2 |
| TC-5 | Candidate conflicts only with another course in the same merge group. | All student-facing candidate builders and final submit reject it. | Resolver + DB integration | FR-1, INV-1 |
| TC-6 | Detail response is an explicitly valid “No schedules yet” page. | Parser returns a complete empty observation; deactivation proceeds only under the confirmation state machine. | Parser + applier integration | FR-3 |
| TC-7 | Detail response has unexpected blank-date row, malformed table, auth failure, or incomplete page. | Parser/sync fails closed; local session and mapping stay active; no deletion progress advances. | Parser + applier integration | FR-3, INV-3 |
| TC-8 | One complete newer observation omits schedule `113373`. | Mapping becomes suspected-missing; session remains active and not tombstoned. | PostgreSQL integration | FR-3, INV-3 |
| TC-9 | Second consecutive complete omission occurs before/at/after grace boundary. | No tombstone before the boundary; at the approved boundary session is soft-deleted and mapping tombstoned atomically. | Unit + PostgreSQL integration | FR-3, INV-5 |
| TC-10 | Schedule `113373` reappears after suspected/tombstoned state. | `deleted_at` clears, mapping becomes active, missing progress resets. | PostgreSQL integration | FR-4, INV-4 |
| TC-11 | Generation 2 contains the schedule and applies; generation 1 omission then attempts apply. | Generation 1 is a no-op/rejected; schedule remains active. | Deterministic concurrency integration | NFR-2, INV-3 |
| TC-12 | Inject failure after session change but before snapshot/commit. | Transaction rolls back session, mapping, and observation state together. | PostgreSQL fault-injection integration | NFR-2 |

The local test coverage and remaining unrun cases are recorded in [`PRODUCTION_TEST_PLAN.md`](../../verification/absence-sit-in-legacy-sync/PRODUCTION_TEST_PLAN.md). The matrix still describes the complete release-oriented test set; not every row has been executed.

## 9. Rollout and recovery

1. Confirm whether multiple legacy-sync instances/workers can process the same course concurrently, how refresh jobs deduplicate, and where a monotonic observation token can be allocated. The code has configurable runner concurrency; per-course apply locking alone does not order fetches.
2. Add nullable/default-safe observation columns or table only if needed; deploy schema first. Pause/drain old sync workers before enabling a destructive-deletion guard so an old binary cannot bypass it.
3. Deploy sync code with deletion suppression counters and a dry/shadow comparison of parsed schedule IDs. Verify `113373` is present in the parsed aggregate before requesting a targeted refresh.
4. Resume workers. Monitor parser drift, incomplete observations, stale-generation rejection, suspected/confirmed missing counts, tombstones, and restored rows. Do not add a threshold until baseline data and ownership are confirmed.
5. Deploy candidate filtering and the form precheck. Check candidate and submit parity for one merge-group case before broad release.
6. Recovery for this record: after validating the source row identity, run the normal targeted course refresh and verify `sessions.deleted_at IS NULL`, mapping state active, correct date/time, and no duplicate session. If parser output still omits ID `113373`, hold the repair and investigate the fetched source response; do not manually mutate production from this plan.
7. Rollback: keep schema additive. If candidate filtering causes a false exclusion, revert resolver/client changes while retaining the final submit guard. Do not roll back to an old sync writer that immediately tombstones on one missing aggregate; keep sync paused or use a forward fix until guarded behavior is restored.

## 10. Open decisions and final gap review

- **Resolved in this implementation:** use two complete misses plus a 24-hour grace period; use an explicit `No schedules yet.` empty-page marker; allocate generations from the shared PostgreSQL sequence before fetch. Missing state is persisted on `external_refs` rather than delegated to `reconcile.Observe`.
- Confirm the live source's authenticated empty marker, populated-page variants, pagination, and any partial-render behavior.
- Confirm deployed workers share the migrated database and all writers use the generation guard before resuming legacy sync.
- If API consumers need separate inactive-versus-overlap error codes, make that a follow-up contract change; stale submissions currently retain the generic `invalid_sessions` response.
- Confirm the exact source schedule row and complete time details for both Oct 13 overlapping expected classes from the source/DB audit. The code path and times are verified; the historical HTML payload is not.
- Functional traceability: FR-1 → TC-1/3/5; FR-2 → TC-2/4; FR-3 → TC-6/7/8/9/11/12; FR-4 → TC-10; FR-5 → form precheck regression test.
- **Decision:** candidate-policy parity and guarded sync deletion were implemented together; deployment and production repair remain separate release steps. Production correctness is not established until the source-generation and parser behavior are confirmed against the live source and deployed writers.
