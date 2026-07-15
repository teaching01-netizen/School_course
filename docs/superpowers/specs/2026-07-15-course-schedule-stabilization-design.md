# Course Schedule Stabilization Design

## Status

Approved scope. This specification covers the first schedule-stabilization release: deterministic schedule correctness, destructive series edge cases, student-occupancy concurrency, availability concurrency, Slot Finder correctness, server-side idempotency, recurrence bounds, supporting indexes, and regression coverage.

The following remain out of scope for this release: durable realtime delivery, institute-wide teacher read-authorization policy, and redesigning the bulk-update endpoint.

## Problem

The schedule currently has several independent failure modes that combine into a high risk of silent data corruption:

- Series edits can conflict with their own occurrences, duplicate sessions, expand count-bounded series, or delete historical attendance-linked data.
- Slot Finder maps PostgreSQL's one-based ordinality to zero-based Go slices, shifting conflict results and potentially panicking.
- Course-roster and session mutations use incompatible transaction strategies, allowing `student_busy_ranges` to become missing or stale.
- Availability mutations do not validate existing sessions and can race session writes.
- Idempotency fingerprints are computed after JSON decoding has consumed the request body.
- Recurrence expansion has no business cap or cancellation checks.
- Trigger and series hot paths lack supporting indexes.

Passing unit tests do not currently cover these behaviors. Database-backed concurrency tests require `TEST_DATABASE_URL` and are skipped when it is absent.

## Goals

1. Preserve the invariant that every effective student/session assignment has exactly one active busy range matching the session interval.
2. Preserve the invariant that every not-yet-started active session satisfies the current teacher and room availability policy.
3. Make series editing deterministic, non-destructive to history, and consistent for count-bounded recurrence.
4. Make Slot Finder return correctly aligned, in-hours results without panics.
5. Restore the documented server-side idempotency contract.
6. Bound recurrence CPU, memory, transaction duration, and query volume.
7. Keep database access predictable as schedule history grows.

## Non-goals

- Rewriting the scheduling domain or recurrence engine.
- Changing hourly Slot Finder start increments.
- Making realtime invalidation durable.
- Changing the bulk-update endpoint's partial-success contract.
- Deciding whether teachers may read the institute-wide schedule.
- Retrofitting idempotency onto bulk update in this release.

## Domain Invariants

### Sessions and occupancy

- An active session has `end_at > start_at`.
- An active effective student assignment has one active `student_busy_ranges` row with the same start and end as its session.
- A student cannot have overlapping active busy ranges.
- A room or teacher cannot have overlapping active sessions.

### Availability

- If a teacher or room has no active availability windows, scheduling remains default-open.
- If at least one active window exists, every not-yet-started active session for that resource must be fully contained in the union of active windows. Adjacent or overlapping windows may collectively cover one session.
- An availability mutation that would invalidate a not-yet-started session is rejected atomically with HTTP `409`. Historical and already-started sessions are not reinterpreted using a new availability policy.

### Series

- “This & Future” may only pivot on an existing active session row in that series whose stored `start_at` is strictly in the future at transaction time.
- Existing historical session rows and their attendance/absence children are never deleted by a series edit.
- The split deletion cutoff uses the original series start time, never the proposed new start time.
- For a count-bounded series, `count` means the target total number of occurrences across the retained prefix and new suffix.
- If both `end_date` and `count` are supplied internally, recurrence stops at whichever bound is reached first.
- A split at the first occurrence is treated as an entire-series future edit rather than creating an empty predecessor.
- A public session create request that supplies `series_id` must match the locked parent series' course, teacher, room, recurrence date/time, duration, and active bounds.

## Proposed Architecture

### 1. Transaction and lock model

Schedule correctness will use explicit database row locks under read-committed transactions, with database exclusion constraints remaining the final overlap gate.

All affected mutations acquire locks before preflight reads and before trigger-producing writes. Locks use a single global order to prevent deadlocks:

1. Course rows, ordered by UUID.
2. Student rows when an operation targets explicit students, ordered by UUID.
3. Teacher user rows, ordered by UUID.
4. Room rows, ordered by UUID.
5. Session or series rows, ordered by UUID.

Affected operations include session create/edit/delete, series create/split/edit/cancel, course-roster add/remove, attendance include/exclude, bulk session edits through the existing service, legacy schedule synchronization, and teacher/room availability create/delete. Lock acquisition is centralized in scheduling helpers rather than reimplemented independently by HTTP handlers.

Session and series HTTP mutations will use the ordinary idempotent read-committed wrapper. Explicit locks serialize conflicting domain changes, while PostgreSQL exclusion constraints protect room, teacher, and student overlap. Constraint-producing statements will use savepoints where the handler needs to re-run preflight and return explainable conflict details without querying an aborted transaction.

The lock must be acquired in a statement before the mutation statement. This ensures that a read-committed transaction waiting on another writer receives a fresh snapshot for subsequent preflight and trigger queries.

### 2. Student busy-range consistency

Session and roster/attendance mutations for the same course serialize on the course row. After acquiring the lock, each operation re-reads the relevant roster/session state before writing.

This closes both known races:

- Roster add versus session create can no longer have both sides observe the other as absent.
- Roster/attendance update versus session-time edit can no longer preserve the old interval.

Database integration tests will run two transactions behind explicit barriers, release them concurrently, and assert both the busy-range row count and interval equality with `sessions`. Cross-course enrollment of the same student remains protected by the student exclusion constraint; constraint-producing writes run inside savepoints so the losing request returns a stable `409` rather than leaving the parent transaction aborted.

### 3. Availability consistency

Session mutations lock their old and proposed teacher and room resources before preflight. Availability mutations lock the same resource row before changing a window.

After an availability create or delete, the transaction checks not-yet-started active sessions for the resource using the transaction timestamp as the boundary. If active windows remain and any such session is not fully contained by their union, the transaction rolls back and returns `409 availability_conflict` with a bounded list of conflicting session IDs and intervals. Deleting the last active window is allowed because it restores default-open behavior.

### 4. Series editing

The frontend includes `series_id` in edit-series preflight requests. Before using it as an ignore filter, the backend verifies that the series exists, is active, and matches the request's course. A caller cannot suppress conflicts from an unrelated series.

Before splitting, the service loads the pivot session by series ID, acquires resource locks, locks the series and pivot session rows, then revalidates that the stored session is still active, still belongs to the series, and still starts strictly after the transaction timestamp. It counts retained occurrences strictly before the pivot. A recurrence date reconstructed from metadata is not accepted as proof that the pivot session exists. For count-bounded series:

```text
target_total = requested_count if supplied, otherwise original_count
remaining = target_total - retained_before_pivot
```

`remaining` must be positive and within recurrence limits. The predecessor stores the retained count; the successor stores the remaining count. If the retained count is zero, the existing series is edited in place rather than creating an invalid empty predecessor. For response compatibility, both `old_series_id` and `new_series_id` contain the existing series ID in this in-place case.

Future deletion uses the original occurrence time on the pivot date. Materialization of the successor uses the proposed time. Past sessions are never deleted or recreated.

Preflight applies the same count calculation, so the number displayed before saving matches the write path.

### 5. Public `series_id` validation

The public session-create endpoint retains `series_id` for compatibility but treats it as a constrained attachment, not arbitrary metadata. The parent series is locked and the request must match:

- course, teacher, and room;
- an allowed weekday and occurrence date within the series bounds;
- the configured local start time and duration;
- an occurrence not removed by the current count/end bound.

Mismatch returns `409 series_occurrence_mismatch`; a missing or inactive parent returns `400 invalid_series`. A concurrent series cancel/edit therefore completes either before validation, causing rejection, or after the attached session commits and accounts for it.

### 6. Slot Finder

Ordinality returned from PostgreSQL is converted with `idx - 1` and checked against slice bounds before use. Availability coverage and teacher/student overlap use the same helper so they cannot drift.

Slot generation continues to use hourly start increments. A candidate is included only when its calculated end instant is less than or equal to the configured day-end instant. This handles sub-hour and non-hour durations without integer-division errors.

### 7. Idempotency

`DecodeJSON` reads the bounded body into bytes, decodes from those bytes, and restores the request body. The idempotency wrapper therefore fingerprints the original method, path, query, and exact JSON bytes. Semantically equivalent JSON with different whitespace remains a different fingerprint, matching the existing documented exact-byte contract.

CAS/version checks move inside the idempotent transaction after key acquisition. Consequently:

- Same key and same request replays the committed response.
- Same key and different body returns `409 idempotency_key_reuse`.
- A lost-response retry does not fail early with `stale_edit` or `404`.

Schedule mutations no longer depend on serializable isolation, removing the unhandled `40001` path from these endpoints. Other serializable users remain unchanged and are outside this release unless directly exercised by schedule tests.

Only committed mutation responses are cached. Validation and conflict responses roll back the idempotency record and may be retried after correction with the same key. A replay may emit the existing best-effort realtime invalidation, but it must not repeat database mutations, audit inserts, or other durable side effects.

`docs/idempotency.md` is updated in the same release so its success/error caching description matches this committed-mutation-only contract.

### 8. Recurrence limits

The recurrence engine enforces shared constants at both preflight and write boundaries:

- Maximum 1,000 materialized occurrences.
- Maximum horizon of five calendar years from `start_date`.
- Maximum session duration of 24 hours.
- Count must fit the database `int32` representation before conversion.

Materialization accepts a context and checks cancellation during iteration. It returns typed validation errors that HTTP routes map to `400`, not generic `500`.

The frontend mirrors these limits for immediate feedback, while the backend remains authoritative.

### 9. Index migration

A non-transactional Goose migration creates indexes concurrently to avoid long blocking table locks:

- `sessions(series_id, start_at)`.
- `sessions(course_id, start_at)` for active rows.
- `student_busy_ranges(session_id)`.
- GiST on active `sessions(time_range)` for institute-wide range reads.
- GiST on active `teacher_availability(teacher_id, time_range)`.
- GiST on active `room_availability(room_id, time_range)`.

Index names are stable and creation is idempotent. Rollback uses `DROP INDEX CONCURRENTLY IF EXISTS`.

## API and Error Behavior

| Condition | Response |
|---|---|
| Missing or non-occurrence split pivot | `400 invalid_series_pivot` |
| Existing pivot session has started or became past | `409 series_occurrence_started` |
| Target total count is smaller than retained prefix | `400 invalid_recurrence_count` |
| Recurrence exceeds count, duration, or horizon limits | `400 invalid_recurrence` |
| Availability mutation invalidates existing sessions | `409 availability_conflict` |
| Supplied session `series_id` is missing/inactive | `400 invalid_series` |
| Supplied session `series_id` does not match definition | `409 series_occurrence_mismatch` |
| Idempotency key reused with another payload | `409 idempotency_key_reuse` |
| Room, teacher, or student overlap | Existing explainable `409 schedule_conflict` |

Error details remain bounded and must not expose raw SQL errors.

## Testing Strategy

All production changes follow test-first red-green-refactor cycles.

### Unit tests

- Slot ordinality converts one-based values and rejects zero/out-of-range values.
- Slot generation excludes candidates ending after day end for 30-, 60-, and 90-minute durations.
- JSON decoding restores the exact original bytes for fingerprinting.
- Materialization rejects excessive count, excessive horizon, excessive duration, and canceled context.
- Split-bound calculations preserve target total count, stop at the earlier of end/count bounds, and use the original pivot clock.
- First-occurrence split selects the in-place edit path.

### Frontend tests

- Edit-series preflight includes `series_id`.
- Recurrence inputs show the shared limits and block invalid submissions.
- Past-session actions do not offer “This & Future”.

### HTTP tests

- Same key/same body replays a successful schedule response.
- Same key/different body returns `409`.
- Lost-response-style retry reaches idempotency replay before CAS checks.
- Recurrence validation errors return `400`.
- Invalid `series_id` attachment returns `409`.

### Database integration tests

- Concurrent roster add/session create produces exactly one correct busy row.
- Concurrent roster/attendance update/session edit leaves the busy row at the new interval.
- Concurrent availability/session writes serialize; one valid outcome commits and no invalid state remains.
- Count-bounded split at occurrence 5 of 10 leaves ten total occurrences.
- Later start-time split deletes the old pivot occurrence and creates one replacement.
- Missing, deleted, wrong-series, exact-now, and past pivot sessions are rejected without changing sessions or child rows.
- Concurrent cancel and public series-attached create cannot leave an out-of-definition future occurrence.
- Two concurrent series writes for the same teacher/room produce one success and one stable conflict without deadlock.
- Resource-swap edits acquire locks in a consistent order and do not deadlock.
- Migration indexes exist after migration and representative production-like queries use the intended access paths.

Database concurrency tests form a dedicated CI-required suite. That suite fails immediately when `TEST_DATABASE_URL` is absent rather than silently skipping. Ordinary local unit-test runs remain database-independent, but the release cannot be declared complete without the configured database suite.

## Rollout and Compatibility

- Rehearse the concurrent migration with production-like cardinality and a finite lock timeout. Apply the index migration before the application release, then verify representative query plans before enabling the new write behavior.
- Existing valid schedule requests retain their response shapes.
- Invalid recurrence and series-attachment requests become explicit `400`/`409` responses instead of generic failures or silent corruption.
- Availability writes may newly return `409` when not-yet-started sessions violate the proposed policy; this is intentional protection.
- Existing data should be audited once for missing or stale busy ranges and sessions outside availability. Repair tooling is a separate operational step unless the audit discovers violations during implementation.
- Before enabling strict `series_id` compatibility checks, report existing sessions whose course, teacher, room, or occurrence interval does not match their parent series. Deployment must stop for manual repair if mismatches exist; it must not silently rewrite attendance-linked history.
- Mixed-version deployment is index-first, then application instances. The older application remains compatible with the new indexes; rollback of application behavior may re-admit invalid writes and therefore requires the same invariant audit.

## Observability

Log rejected availability changes, invalid series attachments, recurrence-limit violations, and lock-wait/timeout failures with request scope and resource IDs. Do not log request bodies or student personal data. Existing metrics should distinguish validation/conflict responses from internal failures.

## Acceptance Criteria

- Every deterministic audit finding included in scope has a failing regression test before its fix and a passing test afterward.
- The two known busy-range race timelines cannot produce missing or stale rows in database integration tests.
- Availability/session concurrency cannot leave a committed session outside active policy.
- Series split cannot alter historical attendance-linked sessions.
- Count-bounded series preserve the requested total occurrence count.
- Preflight and write paths apply identical recurrence limits and count partitioning.
- Slot Finder cannot shift indexes, panic on the final slot, or return an out-of-hours slot.
- Server idempotency fingerprints the original body and replays before CAS checks.
- Recurrence work is bounded and cancellation-aware.
- Required indexes are present.
- Frontend typecheck, focused frontend tests, full Go tests, database integration tests, and production builds pass.
