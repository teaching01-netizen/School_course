# Production test plan

## Executed locally

The local database was `warwick_local_fresh` on `127.0.0.1:5432`; the integration test applied migration 00130 and confirmed schema version 130.

- Sit-in candidate SQL and final overlap helper: `TestSitInCandidateSessionsAllowsAnyNonOverlappingDate` and `TestSitInSessionOverlapIgnoresCourse` passed. The fixture covers a sibling course in the missed course's merge group.
- Form precheck: `submissionPayload.test.ts` passed all 27 tests, including the same-selected-subject conflict.
- Resolver filter: `go test ./internal/httpapi/absenceshttp` passed, including malformed interval fail-closed coverage.
- Parser: `go test ./internal/legacysync/parser` passed, including explicit-empty and unexpected blank/placeholder cases.
- Legacy appliers: `go test ./internal/legacysync/apply -count=1` passed against local PostgreSQL, including confirmation/grace, stale-generation, reappearance, same-course concurrency, and existing fault-injection cases.
- Lean: `lake build` and `lake env lean proof/Model.lean` passed; all 12 named business theorems were axiom-free.

The broader `internal/db` package was attempted on this populated local app database and has unrelated fixture/data failures; see [PROOF_REPORT.md](PROOF_REPORT.md). Production/source tests below remain pending.

## Still pending

## Deterministic functional tests

1. **Same-subject merge-group overlap:** build the Oct 10/17 absence range with expected Oct 13 sessions 13:00–14:40 and 14:40–16:20. Assert both bundle-backed and any query-backed candidate response marks the 13:00–16:20 candidate unavailable. Assert the client precheck reports the same conflict.
2. **Authoritative bypass:** submit the same session IDs directly, bypassing the browser precheck. Assert HTTP rejection and query the database to confirm the absence, missed-session snapshot, and sit-in association did not commit.
3. **Boundary semantics:** test candidate ending exactly at blocker start and starting exactly at blocker end; both remain valid. Test one-minute overlap; it is blocked.
4. **Inactive option race:** fetch a candidate, soft-delete it, then submit its ID. Assert the existing `invalid_sessions` rejection and verify the transaction leaves no partial absence/sit-in rows.
5. **Parser contract:** parse the real explicit empty marker, a valid populated detail page, malformed table cells, unexpected blank-date rows, auth/login content, and any page variants seen during the user-reported source check. Only a complete valid page can advance absence observations.

## Database and concurrency tests

6. **First missing generation:** start with active session + active mapping. Apply one newer complete observation without the ID. Assert the session remains active and the mapping is suspected-missing.
7. **Confirmation/grace boundaries:** apply a second complete omission before, at, and after the configured grace boundary. Assert no early deletion; after the boundary, assert one atomic transition to `deleted_at != NULL` and tombstoned mapping.
8. **Reappearance:** after suspected and tombstoned states, apply a newer complete observation containing the same source ID. Assert missing progress resets, session is restored, mapping active, and no duplicate session is created.
9. **Stale observation interleaving:** use barriers, not sleeps. Start generation 1 missing and generation 2 present; commit generation 2 first; then release generation 1 to apply. Assert generation 1 is rejected/no-op and session remains active.
10. **Per-course concurrent workers:** run two same-course sync applies against PostgreSQL. Confirm monotonic allocation and apply comparison use shared DB state across workers/instances, not a process-local counter.
11. **Fault injection:** fail after observation-state update, after session update, after mapping update, and before commit. Assert session, mapping, missing-progress state, snapshot, and audit event all roll back together.
12. **Retry after ambiguous acknowledgment:** rerun the same generation/hash after a simulated lost response. Assert idempotent final state and no duplicate audit/business effects.

## Source repair and monitoring

- First compare the legacy detail response, parsed schedule IDs, and local row for `113373`; record the observation generation, response status, parser version, row count, and resulting decision. Avoid retaining raw HTML unless the source-data retention/privacy owner approves it.
- After the guarded code is deployed, run a targeted refresh and verify local date/time, `deleted_at`, external mapping state, and `session_changes.change_source`.
- Track counts for parser/auth failures, incomplete observations, stale generations rejected, suspected/confirmed missing schedules, tombstones, and source-present restorations. Alerting thresholds require baseline and owner confirmation; none are invented here.
- Run a production-like multi-worker soak only after topology and normal concurrency are known. Record actual p50/p95/p99, error rate, transaction/lock waits, retry count, and invariant-query results; do not infer robustness from Lean proofs.
