# Concurrency, faults, and production validation

## Executed local profile

- PostgreSQL 14.23 on the local host; READ COMMITTED, synchronous_commit=on, fsync=on; scratch database migrated through schema 133.
- Go race detector. Barrier-coordinated competing transactions, plus two HTTP handlers with independent pools in one Go process. This is not a multi-process deployment or a soak test.
- One physical submit whose final response body write returns `io.ErrClosedPipe`, then 16 simultaneous same-key replays; assert byte-identical stored responses and exactly one absence, one sit-in and one missed-session record.
- Existing barriers: same student/same session, submission versus reassignment, cancellation versus submission, different students sharing a class, and same-key replica handlers.
- Abort points: between domain writes, immediately before commit, and after acquiring but before completing the idempotency record. Read durable state after rollback.
- Seeded differential policy tests: coherent direct/merged catalogs; one/two missed sessions; blocked choices; cutoff dates; all selected courses; single and paired selected IDs. These are finite tests, not unbounded proofs.

Exact commands, results and limits are in PROOF_REPORT.md and evidence/.

## Required deployment evidence (not executed)

1. Record deployed SHA, DB version/isolation, primary/replica routing, pool sizes, instance count, key cleanup schedule/TTL, migrations, timeouts, clock/timezone settings and durability/failover configuration. Compare with A1–A11.
2. Query existing active assignments for same-student strict overlaps across different absences. Verify pair uniqueness, snapshot versions, orphan rows, and active direct/member mapping overlap. Do this read-only on an approved operational connection; no production connection was used here.
3. Force schedule/config races using barriers, not sleeps: pause ordinary staff submit after policy validation; edit the selected session through the real scheduling API before its later fence; resume with omitted and supplied session versions. Repeat for course visibility, enrollment, final-day changes, mapping replacement, and auto-resolve settings. Decide whether the contract is validation-snapshot or commit-time authorization before accepting outcomes.
4. Run separate service processes against the production-like database. Race identical keys and distinct keys for one student's overlapping selections. Also race cancellation, assignment replacement, scheduling changes and workers. Assert persisted invariants after every phase and verify that every writer obeys its required lock protocol.
5. Kill a process after domain writes but before commit, after commit before ACK, and after commit before notification dispatch. Retry the same key on another instance. Inject connection loss and a deadlock. Check no pre-commit partial effects, stable replay, and notification behavior without asserting reliable delivery unless a durable outbox is part of the contract.
6. Test expiry deliberately: replay inside retention; expire/clean the key; retry again. The latter is a new acquisition under the current API, not proof of all-time at-most-once. Reject fingerprint changes under a retained key.
7. Exercise stale reads, DB failover, mixed old/new application versions and schema rollout. The static snapshot/atomic DB assumptions must be revalidated before extending the proof's deployment envelope.
8. Measure a realistic student/staff/discovery/retry request mix at expected peak and overload, followed by a sustained soak. Record p50/p95/p99 end-to-end latency, error and retry rates, lock waits, pool queue time, transaction duration, duplicate effects, backlog and recovery. Traffic, duration and pass thresholds must come from actual operational requirements; they were not invented here.

## Rollout / rollback

No deployment or schema change is part of this verification. Roll out through the normal staged application release. Compare invalid-selection, timeout, lock-wait, conflict and retry metrics and audit persisted invariants. Roll back the optimization revision if authorization decisions diverge or duplicate/overlapping effects occur; reuse the same idempotency keys during retries. Do not claim capacity from the local validation timings or a safety theorem.
