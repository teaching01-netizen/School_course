# Production test plan

## Implemented policy fixes

AP-1 through AP-8 from [POLICY_AUDIT.md](POLICY_AUDIT.md) have been implemented and have focused local regressions. The tests cover filtered and hidden blockers, classes on a make-up date outside the absence window, selected-subject picker conflicts, different overlapping make-up IDs, merged-class components, mismatched policy targets, disabled automatic-resolution scopes, generic final-day exclusions, and cross-section source exclusion. Formal model results do not prove application code or production rollout.

The local `internal/db` run of `TestSitInCandidateSessionsAllowsAnyNonOverlappingDate` timed out at its 20-second context during the combined concurrent run, then passed when rerun by itself against the same local database. The focused policy audit and overlap integration tests passed.

## Executed locally

The local database was `warwick_local_fresh` on `127.0.0.1:5432`; the integration test applied migration 00130 and confirmed schema version 130.

- Sit-in candidate SQL and final overlap helper: `TestSitInCandidateSessionsAllowsAnyNonOverlappingDate` and `TestSitInSessionOverlapIgnoresCourse` passed. The fixture covers a sibling course in the missed course's merge group.
- Form precheck and picker: the focused `sitInResolution.test.ts` and `submissionPayload.test.ts` suites passed 108 tests, including same-subject conflict, overlapping make-ups, merged components, and already-absent classes.
- Resolver filter: unit test passed for top-level, priority, per-missed-session, and nested-priority option collections, including malformed interval fail-closed coverage.
- Legacy priority resolver: `TestLegacyPrioritySitInFiltersExpectedSiblingSession` passed against local PostgreSQL schema version 130. It confirms an overlapping expected sibling-class option is unavailable and a boundary-adjacent option remains selectable.
- Parser: `go test ./internal/legacysync/parser` passed, including explicit-empty and unexpected blank/placeholder cases.
- Legacy appliers: `go test ./internal/legacysync/apply -count=1` passed against local PostgreSQL, including confirmation/grace, stale-generation, reappearance, same-course concurrency, and existing fault-injection cases.
- Lean: `lake build` and `lake env lean proof/Model.lean` passed. The arbitrary policy/priority filter theorem is proved for all candidate lists and has standard axiom dependencies `[propext, Quot.sound]`; the concrete Oct 13 witness and other business theorems report no axioms.

The broader `internal/db` package was attempted on this populated local app database and has unrelated fixture/data failures; see [PROOF_REPORT.md](PROOF_REPORT.md). Production/source tests below remain pending.

## Still pending

- Add an endpoint-level assertion that a forged, policy-ineligible submit leaves no persisted absence, missed-session snapshot, or sit-in assignment after rollback.
- Run a deterministic concurrent submission test using two different overlapping session IDs under the student-row lock, plus a multi-item batch overlap case.
- Confirm deployed subject-policy mappings and decide whether the final *missed* class, the final *target* class, or both should be excluded.
- Verify production schema/deployment state and the live legacy source behavior for schedule `113373`; no production database was accessed.

## Deterministic functional tests

1. **Persisted rollback:** bypass the browser and submit an ineligible target, then assert absence, snapshot, and sit-in rows were all rolled back.
2. **Boundary semantics:** test exact adjacency on both sides of an expected class and one-minute overlap in the new authoritative picker query.
3. **Inactive option race:** fetch a candidate, soft-delete it, then submit its ID; assert rejection and no partial writes.
4. **Parser contract:** parse the real explicit empty marker, valid populated page, malformed cells, unexpected blank-date rows, auth/login content, and any page variants seen during the user-reported source check.

## Database and concurrency tests

1. **First missing generation:** start with active session + active mapping. Apply one newer complete observation without the ID. Assert the session remains active and the mapping is suspected-missing.
2. **Confirmation/grace boundaries:** apply a second complete omission before, at, and after the configured grace boundary. Assert no early deletion; after the boundary, assert one atomic transition to `deleted_at != NULL` and tombstoned mapping.
3. **Reappearance:** after suspected and tombstoned states, apply a newer complete observation containing the same source ID. Assert missing progress resets, session is restored, mapping active, and no duplicate session is created.
4. **Stale observation interleaving:** use barriers, not sleeps. Start generation 1 missing and generation 2 present; commit generation 2 first; then release generation 1 to apply. Assert generation 1 is rejected/no-op and session remains active.
5. **Per-course concurrent workers:** run two same-course sync applies against PostgreSQL. Confirm monotonic allocation and apply comparison use shared DB state across workers/instances, not a process-local counter.
6. **Fault injection:** fail after observation-state update, after session update, after mapping update, and before commit. Assert session, mapping, missing-progress state, snapshot, and audit event all roll back together.
7. **Retry after ambiguous acknowledgment:** rerun the same generation/hash after a simulated lost response. Assert idempotent final state and no duplicate audit/business effects.

## Source repair and monitoring

- First compare the legacy detail response, parsed schedule IDs, and local row for `113373`; record the observation generation, response status, parser version, row count, and resulting decision. Avoid retaining raw HTML unless the source-data retention/privacy owner approves it.
- After the guarded code is deployed, run a targeted refresh and verify local date/time, `deleted_at`, external mapping state, and `session_changes.change_source`.
- Track counts for parser/auth failures, incomplete observations, stale generations rejected, suspected/confirmed missing schedules, tombstones, and source-present restorations. Alerting thresholds require baseline and owner confirmation; none are invented here.
- Run a production-like multi-worker soak only after topology and normal concurrency are known. Record actual p50/p95/p99, error rate, transaction/lock waits, retry count, and invariant-query results; do not infer robustness from Lean proofs.
