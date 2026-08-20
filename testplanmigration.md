# Legacy Course and Schedule Synchronization

## Rigorous Core, Concurrency, and Failure Test Plan

## 1. Purpose

This plan verifies that the legacy synchronization platform can safely mirror:

* Courses.
* Course schedules.
* Teachers.
* Rooms.
* Subjects.
* Historical course and schedule data.
* Legacy management fields such as archive state, confirmation, room assignment, course type, and expiration.

The system must remain correct under:

* Duplicate source events.
* Concurrent workers.
* Process crashes.
* Network failures.
* Authentication expiry.
* HTML structure changes.
* Database errors.
* Queue delays.
* Partial source data.
* Reconciliation races.
* Realtime delivery failures.

The legacy system exposes authenticated HTML rather than a supported API or database change feed, so parser safety and reconciliation are critical correctness boundaries.

The existing repository provides:

* Versioned sessions with course, teacher, room, time, and soft-deletion fields.
* Normal schedule conflict checking and resource locking.
* PostgreSQL queue leases, retries, deduplication, heartbeat, `SKIP LOCKED`, and `LISTEN/NOTIFY`.
* Realtime channel delivery and cross-instance fanout.
* Course legacy linkage and last-synchronized metadata.

---

# 2. Critical system invariants

These invariants must be tested directly. A test suite that only checks API status codes is insufficient.

## INV-001 — Stable external identity

For one source, entity type, and legacy ID, there is exactly one internal record.

```text id="6adjo8"
legacy_warwick / course / 7306
    → exactly one courses.id
```

The same numeric ID may exist in another entity type.

```text id="7ey7np"
course / 78
teacher / 78
```

These must not collide.

## INV-002 — Schedule identity

One `legacy_schedule_id` maps to exactly one local session.

Repeated synchronization must update the same session rather than create another one.

## INV-003 — Aggregate atomicity

A course refresh must not expose a partially applied aggregate.

The following must commit together:

```text id="71kt68"
course fields
teacher reference
subject reference
series container
schedule rows
room references
confirmation states
source snapshot
external mappings
audit record
outbox event
```

If any required operation fails, none of the changes may remain.

## INV-004 — Idempotency

Applying identical canonical source data repeatedly must cause:

* No new domain row.
* No version bump.
* No repeated audit event.
* No repeated realtime event.
* No unnecessary `updated_at` change.

## INV-005 — Last-good-state preservation

Authentication failure, parser failure, source outage, or partial HTML must never replace valid local data with empty or partial data.

## INV-006 — Safe deletion

An entity missing from one scrape must not be deleted or tombstoned.

Tombstoning requires:

* Multiple complete successful generations.
* Archive and active coverage.
* No parser failure.
* No authentication failure.
* Grace period completion.

## INV-007 — Source ownership

Legacy synchronization may update legacy-owned fields only.

It must not overwrite:

* New-system absence state.
* Notification state.
* Native audit metadata.
* New-system-only preferences.
* Other explicitly native-owned fields.

## INV-008 — Native scheduling safety

Legacy synchronization must not weaken normal scheduling behavior.

Native schedule writes must continue using:

* Conflict checking.
* Teacher membership validation.
* Resource locks.
* Optimistic concurrency.
* Native series behavior.

Legacy historical import may preserve source conflicts, but only through the trusted legacy apply service.

## INV-009 — Commit-before-publish

No realtime event may be visible before the database transaction commits.

Rolled-back transactions must produce no user-visible event.

## INV-010 — Recovery without duplication

After a process crash or job lease expiration, another worker may retry the work without creating duplicate domain, audit, snapshot, or outbox rows.

## INV-011 — Bounded source traffic

Queue backlog, retries, or duplicate source events must not create unbounded requests against the old site.

## INV-012 — Historical traceability

When historical source data changes:

* The previous source snapshot remains available.
* The correction is audited.
* Downstream impact analysis is triggered where required.
* Dependent historical records are not silently deleted.

The repository already enforces immutable snapshot behavior in absence-related workflows; synchronized historical changes must preserve that safety model.

---

# 3. Test environments

## 3.1 Pure unit environment

No network and no database.

Use for:

* Normalization.
* Canonical sorting.
* Hash calculation.
* Date parsing.
* Event classification.
* Job-key generation.
* State-machine transitions.
* Retry classification.

## 3.2 Fake legacy server

Use `httptest.Server`.

It must simulate:

* Login page.
* Antiforgery token.
* Authentication cookie.
* Session expiration.
* Audit log.
* Course list.
* Course detail.
* Schedule page.
* Teachers.
* Rooms.
* Subjects.
* Archived data.
* Slow and malformed responses.

## 3.3 Real PostgreSQL integration environment

Use an isolated PostgreSQL database per test package or test process.

Do not replace PostgreSQL behavior with SQLite because the implementation depends on:

* Advisory locks.
* `FOR UPDATE SKIP LOCKED`.
* `LISTEN/NOTIFY`.
* PostgreSQL transaction semantics.
* JSONB.
* Partial indexes.
* Leases and interval operations.

## 3.4 Multi-process integration environment

Run at least:

* Two detector instances.
* Four queue workers.
* Two apply workers.
* Two realtime backend instances.
* One fake legacy source.
* One PostgreSQL database.

Use this environment for leadership, leases, duplicate prevention, and fanout tests.

## 3.5 Full end-to-end environment

Run:

```text id="jijtex"
fake legacy source
legacy sync service
PostgreSQL
backend API
realtime fanout
React frontend
Playwright
```

---

# 4. Deterministic test controls

Production code should support injected controls at I/O boundaries.

## Required abstractions

```go id="fi1ixa"
type Clock interface {
    Now() time.Time
}

type Sleeper interface {
    Sleep(context.Context, time.Duration) error
}

type SourceClient interface {
    Fetch(context.Context, Request) (Response, error)
}

type JobStore interface {
    Enqueue(...)
    Claim(...)
    Complete(...)
    Retry(...)
}

type FaultPoint interface {
    Hit(name string) error
}
```

Use a fault injector only in tests or test builds.

Example fault points:

```text id="vyr4ia"
after_course_upsert
after_session_upsert
after_snapshot_insert
after_audit_insert
after_outbox_insert
before_commit
after_commit_before_job_complete
after_job_claim
after_source_response_headers
during_source_body_read
```

Tests must not depend on arbitrary `time.Sleep`.

Use:

* Channels.
* Barriers.
* Latches.
* Injected clocks.
* Explicit transaction hooks.

---

# 5. Core unit test plan

## 5.1 Text normalization

Files:

```text id="4c1d9p"
backend/internal/legacysync/normalize/text_test.go
```

Tests:

| ID      | Scenario                      | Expected                      |
| ------- | ----------------------------- | ----------------------------- |
| TXT-001 | Normal spaces                 | Preserved semantically        |
| TXT-002 | Non-breaking spaces           | Converted consistently        |
| TXT-003 | Multiple spaces               | Collapsed                     |
| TXT-004 | Leading/trailing spaces       | Trimmed                       |
| TXT-005 | Thai text                     | Preserved                     |
| TXT-006 | Empty value                   | Normalized to empty/null rule |
| TXT-007 | `[NOT SET]`                   | Normalized to null room       |
| TXT-008 | Leading-zero ID `00`          | Preserved as string           |
| TXT-009 | Mixed Unicode representations | Canonicalized consistently    |
| TXT-010 | Very long input               | Bounded without panic         |

Add fuzz testing:

```go id="zwq9uv"
func FuzzNormalizeLegacyText(f *testing.F)
```

Properties:

* Never panic.
* Idempotent normalization.
* Valid UTF-8 output.
* Does not remove meaningful Thai characters.

## 5.2 Date and time parsing

Files:

```text id="xwr95k"
backend/internal/legacysync/normalize/datetime_test.go
```

Tests:

| ID      | Scenario                | Expected                                         |
| ------- | ----------------------- | ------------------------------------------------ |
| DAT-001 | `03/08/26`              | Correct 2026 date                                |
| DAT-002 | Four-digit year         | Correct date                                     |
| DAT-003 | Bangkok local time      | Correct UTC conversion                           |
| DAT-004 | Midnight                | Correct date boundary                            |
| DAT-005 | Cross-midnight session  | End moved to next day only when contract permits |
| DAT-006 | End before start        | Validation failure                               |
| DAT-007 | Missing start           | Validation failure                               |
| DAT-008 | Missing end             | Validation failure or explicit partial state     |
| DAT-009 | Invalid date            | Typed parse error                                |
| DAT-010 | Leap day                | Correct validation                               |
| DAT-011 | Historical year         | Preserved                                        |
| DAT-012 | Server timezone differs | Result remains Bangkok-based                     |

## 5.3 Canonical hashing

Files:

```text id="yu9a29"
backend/internal/legacysync/normalize/hash_test.go
```

Tests:

| ID      | Scenario                            | Expected                                                |
| ------- | ----------------------------------- | ------------------------------------------------------- |
| HSH-001 | Same data, same order               | Same hash                                               |
| HSH-002 | Same schedules, different row order | Same hash                                               |
| HSH-003 | Whitespace-only difference          | Same hash                                               |
| HSH-004 | Room changes                        | Different hash                                          |
| HSH-005 | Confirmation changes                | Different hash                                          |
| HSH-006 | Teacher changes                     | Different hash                                          |
| HSH-007 | Schedule time changes               | Different hash                                          |
| HSH-008 | Parser version changes              | Domain hash unchanged unless canonical semantics change |
| HSH-009 | Raw HTML changes only               | Domain hash unchanged                                   |
| HSH-010 | Duplicate schedule row              | Validation failure, not silently deduplicated           |

---

# 6. Parser contract test plan

## 6.1 Required fixture categories

Create sanitized HTML fixtures for:

```text id="onmsty"
login success
login failure
expired session
course list active
course list archived
course detail with schedules
course detail without schedules
course detail with unassigned room
confirmed schedule
unconfirmed schedule
teacher list
subject list
room list
today schedule
audit log
empty table
malformed table
unexpected redirect page
server error page
partially truncated HTML
```

## 6.2 Course detail parser

Files:

```text id="mjxg5v"
backend/internal/legacysync/parser/course_detail_test.go
```

Tests:

| ID          | Scenario                   | Expected                                        |
| ----------- | -------------------------- | ----------------------------------------------- |
| PAR-CRS-001 | Valid course detail        | Full aggregate                                  |
| PAR-CRS-002 | Multiple schedules         | Every row parsed                                |
| PAR-CRS-003 | Duplicate schedule ID      | Hard parser validation failure                  |
| PAR-CRS-004 | Missing course ID          | Failure                                         |
| PAR-CRS-005 | Missing required header    | Schema-drift failure                            |
| PAR-CRS-006 | Optional room absent       | Null room                                       |
| PAR-CRS-007 | Confirmation `Yes`         | Confirmed true                                  |
| PAR-CRS-008 | Confirmation blank         | Confirmed false/unknown per contract            |
| PAR-CRS-009 | Unknown confirmation value | Validation failure                              |
| PAR-CRS-010 | Login HTML returned        | Authentication-expired error                    |
| PAR-CRS-011 | Truncated last row         | Entire page rejected                            |
| PAR-CRS-012 | Unknown extra column       | Accepted only when contract permits             |
| PAR-CRS-013 | Column removed             | Rejected                                        |
| PAR-CRS-014 | Column reordered           | Parsed by semantic header, not brittle position |
| PAR-CRS-015 | Very large schedule count  | Bounded and correct                             |

## 6.3 Audit log parser

Tests:

| ID          | Scenario                     | Expected                                       |
| ----------- | ---------------------------- | ---------------------------------------------- |
| PAR-LOG-001 | Course edit action           | Targeted course event                          |
| PAR-LOG-002 | Schedule edit action         | Targeted course/schedule event                 |
| PAR-LOG-003 | Room edit                    | Room refresh event                             |
| PAR-LOG-004 | Teacher edit                 | Teacher refresh event                          |
| PAR-LOG-005 | Subject edit                 | Subject refresh event                          |
| PAR-LOG-006 | Unknown action               | Bounded reconciliation event                   |
| PAR-LOG-007 | Duplicate identical rows     | Multiplicity preserved until event dedup layer |
| PAR-LOG-008 | Same-second actions          | No event loss                                  |
| PAR-LOG-009 | Reordered rows               | Overlapping-window logic remains correct       |
| PAR-LOG-010 | Old rows disappear from page | No inferred deletion                           |

---

# 7. Legacy HTTP client failure tests

Files:

```text id="xhdz1q"
backend/internal/legacysync/client/client_integration_test.go
backend/internal/legacysync/client/auth_concurrency_test.go
```

## 7.1 Authentication

| ID            | Scenario                   | Expected                           |
| ------------- | -------------------------- | ---------------------------------- |
| HTTP-AUTH-001 | Valid login                | Session established                |
| HTTP-AUTH-002 | Wrong credentials          | Non-retryable configuration error  |
| HTTP-AUTH-003 | Session expired            | One re-login, then retry           |
| HTTP-AUTH-004 | Ten workers see expiry     | Exactly one login attempt          |
| HTTP-AUTH-005 | Re-login fails             | Workers receive typed auth failure |
| HTTP-AUTH-006 | Login form changed         | Schema-drift/auth-contract failure |
| HTTP-AUTH-007 | Antiforgery cookie missing | Login rejected safely              |
| HTTP-AUTH-008 | Identity cookie absent     | Login rejected                     |
| HTTP-AUTH-009 | Redirect loop              | Bounded failure                    |
| HTTP-AUTH-010 | Login timeout              | Retry classification correct       |

## 7.2 Route safety

| ID            | Scenario                      | Expected        |
| ------------- | ----------------------------- | --------------- |
| HTTP-SAFE-001 | Allowed GET                   | Sent            |
| HTTP-SAFE-002 | Allowed read-only search POST | Sent            |
| HTTP-SAFE-003 | Confirm handler               | Blocked locally |
| HTTP-SAFE-004 | Delete handler                | Blocked locally |
| HTTP-SAFE-005 | Import handler                | Blocked locally |
| HTTP-SAFE-006 | Unknown route                 | Blocked locally |
| HTTP-SAFE-007 | URL encoded bypass attempt    | Blocked         |
| HTTP-SAFE-008 | Redirect to mutating path     | Blocked         |

## 7.3 Network and response failures

| ID           | Scenario                 | Expected                          |
| ------------ | ------------------------ | --------------------------------- |
| HTTP-NET-001 | DNS/connect failure      | Retryable source-unavailable      |
| HTTP-NET-002 | Connection reset         | Retryable                         |
| HTTP-NET-003 | Header timeout           | Retryable                         |
| HTTP-NET-004 | Body stalls              | Context cancellation              |
| HTTP-NET-005 | Partial body             | Parse failure, no apply           |
| HTTP-NET-006 | 429 with retry hint      | Rate-limited retry                |
| HTTP-NET-007 | 500                      | Retryable                         |
| HTTP-NET-008 | 404 expected entity      | Entity-specific missing candidate |
| HTTP-NET-009 | Unexpected content type  | Rejected                          |
| HTTP-NET-010 | Oversized response       | Bounded error                     |
| HTTP-NET-011 | Context cancelled        | Immediate exit                    |
| HTTP-NET-012 | Response body read error | No partial parse                  |

Verify cookies, tokens, and credentials never appear in error strings or logs.

---

# 8. Master-data core tests

Files:

```text id="kthz1c"
backend/internal/legacysync/apply/masterdata_integration_test.go
```

## Teachers

| ID        | Scenario                                     | Expected                         |
| --------- | -------------------------------------------- | -------------------------------- |
| MST-T-001 | New teacher                                  | Mapping and local record created |
| MST-T-002 | Same teacher repeated                        | No-op                            |
| MST-T-003 | Teacher renamed                              | Same identity updated            |
| MST-T-004 | Same name, different legacy ID               | Two distinct records             |
| MST-T-005 | Inactive teacher                             | Inactive state preserved         |
| MST-T-006 | Missing username                             | Non-login teacher still imported |
| MST-T-007 | Missing email                                | Accepted                         |
| MST-T-008 | Fake credential prevention                   | Login remains disabled           |
| MST-T-009 | Existing mapping points to missing local row | Conflict recorded                |
| MST-T-010 | Concurrent create                            | One local record                 |

## Rooms

| ID        | Scenario                       | Expected                                    |
| --------- | ------------------------------ | ------------------------------------------- |
| MST-R-001 | New room                       | Created                                     |
| MST-R-002 | Room renamed                   | Same mapping updated                        |
| MST-R-003 | Duplicate name/different IDs   | Distinct records                            |
| MST-R-004 | `[NOT SET]`                    | No synthetic room                           |
| MST-R-005 | Existing native room same name | No automatic merge without approved mapping |

## Subjects

| ID        | Scenario                     | Expected             |
| --------- | ---------------------------- | -------------------- |
| MST-S-001 | New subject                  | Created              |
| MST-S-002 | Subject renamed              | Same mapping updated |
| MST-S-003 | Duplicate name/different IDs | Distinct             |
| MST-S-004 | Leading-zero code            | Preserved            |
| MST-S-005 | Invalid empty ID             | Rejected             |

---

# 9. Course aggregate tests

Files:

```text id="6wspjg"
backend/internal/legacysync/apply/course_integration_test.go
```

| ID      | Scenario                                 | Expected                              |
| ------- | ---------------------------------------- | ------------------------------------- |
| CRS-001 | New course                               | Course and mapping created            |
| CRS-002 | Existing linked course                   | Same row updated                      |
| CRS-003 | Same source hash                         | Complete no-op                        |
| CRS-004 | Teacher changed                          | Teacher reference updated             |
| CRS-005 | Subject changed                          | Subject reference updated             |
| CRS-006 | Archived state changed                   | Course state updated                  |
| CRS-007 | Expiration changed                       | Correct update                        |
| CRS-008 | Course type changed                      | Correct update                        |
| CRS-009 | Missing teacher mapping                  | `pending_reference`, no partial write |
| CRS-010 | Missing subject mapping                  | `pending_reference`, no partial write |
| CRS-011 | Duplicate legacy course mapping          | Constraint failure and conflict       |
| CRS-012 | Native-owned field exists                | Preserved                             |
| CRS-013 | Snapshot insert fails                    | Entire transaction rolls back         |
| CRS-014 | Audit insert fails                       | Entire transaction rolls back         |
| CRS-015 | Outbox insert fails                      | Entire transaction rolls back         |
| CRS-016 | Commit fails                             | No visible partial state              |
| CRS-017 | Historical correction                    | Previous snapshot preserved           |
| CRS-018 | Course reappears after suspected missing | Restored active                       |

---

# 10. Schedule core tests

Files:

```text id="wqnp2n"
backend/internal/legacysync/apply/schedule_integration_test.go
```

| ID      | Scenario                                | Expected                                         |
| ------- | --------------------------------------- | ------------------------------------------------ |
| SCH-001 | New schedule                            | External session created                         |
| SCH-002 | Same legacy ID repeated                 | Same session reused                              |
| SCH-003 | Time changed                            | Existing session updated                         |
| SCH-004 | Room changed                            | Existing session updated                         |
| SCH-005 | Teacher changed                         | Existing session updated                         |
| SCH-006 | Confirmation changed                    | Metadata updated                                 |
| SCH-007 | Room missing                            | Null room or pending reference per contract      |
| SCH-008 | Existing teacher conflict               | Source preserved; conflict recorded              |
| SCH-009 | Existing room conflict                  | Source preserved; conflict recorded              |
| SCH-010 | Historical schedule                     | Imported without native preflight rejection      |
| SCH-011 | External series                         | No automatic materialization                     |
| SCH-012 | Native edit external series             | Rejected                                         |
| SCH-013 | Duplicate legacy schedule ID in payload | Entire aggregate rejected                        |
| SCH-014 | Source removes one session once         | Suspected missing only                           |
| SCH-015 | Session reappears                       | Missing state cleared                            |
| SCH-016 | Session update fails midway             | Course aggregate rolls back                      |
| SCH-017 | Attendance dependency exists            | Historical correction does not delete dependency |
| SCH-018 | Start/end invalid                       | Source conflict, no invalid session write        |

The current native scheduling service performs conflict checks and resource locking; test explicitly that legacy imports do not alter those normal code paths.

---

# 11. Queue correctness tests

Files:

```text id="qb7w9f"
backend/internal/jobqueue/queue_integration_test.go
backend/internal/jobqueue/queue_concurrency_test.go
```

| ID      | Scenario                           | Expected                                |
| ------- | ---------------------------------- | --------------------------------------- |
| QUE-001 | One queued job                     | One worker claims                       |
| QUE-002 | Ten workers race                   | Exactly one claim                       |
| QUE-003 | Different jobs                     | Parallel claims permitted               |
| QUE-004 | Active unique key duplicate        | Existing job refreshed/coalesced        |
| QUE-005 | Terminal key reused                | New job allowed                         |
| QUE-006 | Worker dies after claim            | Lease expiry permits reclaim            |
| QUE-007 | Heartbeat healthy                  | No premature reclaim                    |
| QUE-008 | Heartbeat stops                    | Reclaim after lease                     |
| QUE-009 | Retryable failure                  | Retry with bounded backoff              |
| QUE-010 | Non-retryable failure              | Dead-letter/failed                      |
| QUE-011 | Max attempts reached               | Terminal failure                        |
| QUE-012 | `NOTIFY` lost                      | Polling fallback eventually claims      |
| QUE-013 | Database disconnect during claim   | No duplicate claim                      |
| QUE-014 | Priority jobs                      | P0 claimed before historical P4         |
| QUE-015 | Historical backlog with P0 arrival | P0 not starved                          |
| QUE-016 | Shutdown during idle               | Clean exit                              |
| QUE-017 | Shutdown during handler            | Context cancellation and lease recovery |
| QUE-018 | Poison job                         | Does not block following jobs           |

---

# 12. Concurrency test matrix

Concurrency tests should use barriers so operations reach the intended race point.

## 12.1 Same course refreshed by two workers

### Setup

Both workers receive different snapshots for course `7306`.

```text id="a26yue"
Worker A: source version with old room
Worker B: source version with new room
```

### Tests

| ID          | Interleaving                          | Expected                                                         |
| ----------- | ------------------------------------- | ---------------------------------------------------------------- |
| CON-CRS-001 | A locks first, B waits                | B applies after A or becomes no-op based on observation ordering |
| CON-CRS-002 | Both load before lock                 | Per-entity lock prevents interleaved write                       |
| CON-CRS-003 | A fails before commit                 | B proceeds after rollback                                        |
| CON-CRS-004 | A commits, dies before queue complete | Retry is idempotent                                              |
| CON-CRS-005 | Older observation arrives after newer | Older snapshot must not overwrite newer state                    |

The last test requires an observation sequence or source-observed timestamp monotonicity policy.

## 12.2 Same schedule updated concurrently

| ID          | Scenario                                   | Expected                           |
| ----------- | ------------------------------------------ | ---------------------------------- |
| CON-SCH-001 | Two workers create same legacy schedule    | One session                        |
| CON-SCH-002 | Create races with update                   | Final row correct                  |
| CON-SCH-003 | Room update races with confirmation update | Aggregate lock prevents lost field |
| CON-SCH-004 | Tombstone races with refresh               | Fresh observation wins             |
| CON-SCH-005 | Reconciliation races with targeted refresh | No stale overwrite                 |

## 12.3 Different courses

| ID           | Scenario                     | Expected                                                           |
| ------------ | ---------------------------- | ------------------------------------------------------------------ |
| CON-DIFF-001 | Two independent courses      | Parallel apply                                                     |
| CON-DIFF-002 | Two courses share teacher    | No unnecessary global lock                                         |
| CON-DIFF-003 | Two courses share room       | Legacy apply remains parallel unless same mapping is being created |
| CON-DIFF-004 | Same missing teacher mapping | One teacher mapping created                                        |

## 12.4 Detector leadership

| ID           | Scenario                        | Expected                                   |
| ------------ | ------------------------------- | ------------------------------------------ |
| CON-LEAD-001 | Two detectors start             | One leader                                 |
| CON-LEAD-002 | Leader crashes                  | Follower takes leadership                  |
| CON-LEAD-003 | Temporary DB disconnect         | No prolonged split brain                   |
| CON-LEAD-004 | Old leader reconnects           | Does not resume without reacquiring lock   |
| CON-LEAD-005 | Leadership transfer during poll | Duplicate observations safely deduplicated |

## 12.5 Authentication refresh storm

| ID           | Scenario                        | Expected                                       |
| ------------ | ------------------------------- | ---------------------------------------------- |
| CON-AUTH-001 | 20 requests see expiry          | One login                                      |
| CON-AUTH-002 | Login succeeds                  | All requests retry once                        |
| CON-AUTH-003 | Login fails                     | All requests fail without repeated login storm |
| CON-AUTH-004 | Context cancelled while waiting | Cancelled waiter exits                         |
| CON-AUTH-005 | New expiry during refresh       | Bounded failure                                |

## 12.6 Native admin edit versus legacy apply

This is a critical ownership test.

| ID          | Scenario                                                    | Expected                                        |
| ----------- | ----------------------------------------------------------- | ----------------------------------------------- |
| CON-OWN-001 | Native user edits legacy-owned field                        | Rejected or explicitly overridden               |
| CON-OWN-002 | Native user edits native-owned field while sync runs        | Native field preserved                          |
| CON-OWN-003 | Sync updates course while normal editor holds stale version | Stable conflict behavior                        |
| CON-OWN-004 | Course becomes legacy-managed during edit                   | Save rejected with current ownership details    |
| CON-OWN-005 | Cutover changes source ownership                            | Explicit migration, not race-dependent behavior |

## 12.7 Realtime concurrency

| ID         | Scenario                                      | Expected                                         |
| ---------- | --------------------------------------------- | ------------------------------------------------ |
| CON-RT-001 | Two workers produce same logical update       | One outbox event                                 |
| CON-RT-002 | Two backend instances publish                 | Cross-instance clients receive one logical event |
| CON-RT-003 | Slow client buffer full                       | Client disconnected without blocking publisher   |
| CON-RT-004 | Hub instance restarts                         | Database state remains source of truth           |
| CON-RT-005 | Event arrives before frontend query completes | Query invalidation still converges               |

---

# 13. Transaction fault-injection tests

Files:

```text id="qkodmd"
backend/internal/legacysync/apply/atomicity_integration_test.go
```

For each fault point, assert all domain and metadata tables remain consistent.

## Course aggregate fault points

| ID         | Failure point                       | Expected      |
| ---------- | ----------------------------------- | ------------- |
| TX-CRS-001 | After course insert                 | Full rollback |
| TX-CRS-002 | After teacher mapping resolution    | Full rollback |
| TX-CRS-003 | After subject mapping resolution    | Full rollback |
| TX-CRS-004 | After first schedule insert         | Full rollback |
| TX-CRS-005 | After last schedule insert          | Full rollback |
| TX-CRS-006 | After stale sessions marked missing | Full rollback |
| TX-CRS-007 | After snapshot insert               | Full rollback |
| TX-CRS-008 | After audit insert                  | Full rollback |
| TX-CRS-009 | After outbox insert                 | Full rollback |
| TX-CRS-010 | Before commit                       | Full rollback |

## Post-commit fault points

| ID          | Failure point                            | Expected                                       |
| ----------- | ---------------------------------------- | ---------------------------------------------- |
| TX-POST-001 | Commit succeeds, worker dies             | Retry becomes no-op                            |
| TX-POST-002 | Commit succeeds, queue complete fails    | Job reclaimed safely                           |
| TX-POST-003 | Outbox committed, publisher dies         | Another publisher delivers                     |
| TX-POST-004 | Event delivered, outbox completion fails | Consumer handles duplicate invalidation safely |

---

# 14. Reconciliation and deletion tests

Files:

```text id="o07j4o"
backend/internal/legacysync/reconcile/generation_integration_test.go
backend/internal/legacysync/reconcile/tombstone_integration_test.go
```

| ID      | Scenario                                      | Expected                                        |
| ------- | --------------------------------------------- | ----------------------------------------------- |
| REC-001 | Complete successful generation                | Checkpoint advances                             |
| REC-002 | Source request fails midway                   | Generation incomplete                           |
| REC-003 | Parser fails on one page                      | Generation incomplete                           |
| REC-004 | Entity missing once                           | `suspected_missing`                             |
| REC-005 | Missing twice                                 | `confirmed_missing`                             |
| REC-006 | Grace period incomplete                       | No tombstone                                    |
| REC-007 | Grace complete                                | Soft tombstone                                  |
| REC-008 | Entity reappears before tombstone             | Active restored                                 |
| REC-009 | Entity reappears after tombstone              | Restored with audit                             |
| REC-010 | Active filter checked but archive not checked | No tombstone                                    |
| REC-011 | Authentication expires during generation      | No tombstone                                    |
| REC-012 | Source returns empty course list unexpectedly | Catastrophic-empty guard prevents mass deletion |
| REC-013 | Source count drops 90%                        | Safety threshold blocks tombstone               |
| REC-014 | Manual deletion disabled                      | State may advance, domain remains active        |
| REC-015 | Targeted refresh during full reconcile        | Newer observation retained                      |

Add catastrophic source guards:

```text id="p4ivtm"
expected minimum course count
maximum percentage drop
minimum page signature confidence
active and archive coverage flags
```

---

# 15. External failure situation tests

## 15.1 Complete legacy outage

### Inject

* Connection refused for 15 minutes.

### Verify

* New API continues serving local data.
* No frontend request waits for legacy.
* Queue grows within configured bounds.
* Retry delay increases.
* Historical crawling pauses.
* Freshness status becomes degraded.
* No local data is removed.
* Recovery drains high-priority jobs first.

## 15.2 Legacy source becomes slow

### Inject

* Audit log latency: 400 ms.
* Course detail latency: 2 seconds.

### Verify

* Source timeout and concurrency limits are respected.
* No runaway worker creation.
* One-second SLO alert identifies source latency.
* API latency remains unaffected.
* Queue remains bounded.
* P0 jobs remain prioritized.

## 15.3 Legacy returns 429

### Verify

* Rate limiter reduces traffic.
* Retry honors configured delay.
* Historical work pauses.
* No authentication retry occurs unnecessarily.
* Alert includes source throttling category.

## 15.4 Legacy returns 500 burst

### Verify

* Circuit opens after threshold.
* Health probes are bounded.
* Last-good data remains.
* Circuit closes after successful probes.
* Jobs are not lost.

## 15.5 Source authentication revoked

### Verify

* One synchronized login attempt.
* Authentication circuit opens.
* Fetch workers stop hammering login.
* Operations receives a clear alert.
* No parsing of login page as empty data.
* Queue remains durable.

## 15.6 Legacy HTML deployment changes structure

### Inject

* Rename a required schedule header.
* Remove schedule ID field.
* Add an unexpected nested table.

### Verify

* Parser rejects page.
* Last-good data stays unchanged.
* Tombstone processing is disabled.
* Redacted diagnostic fixture stored.
* `parser_drift` conflict created.
* Alert includes parser version and page type.

## 15.7 Partial or truncated source response

### Verify

* Entire page rejected.
* No partial aggregate written.
* Retry classification is correct.
* No schedules marked missing.

## 15.8 Source clock anomaly

### Inject

* Source timestamp moves backward.
* Two events share identical second.
* Event order changes.

### Verify

* Overlapping polling window avoids loss.
* Observation sequence protects against stale overwrite.
* Reconciliation eventually converges.

---

# 16. Internal failure situation tests

## 16.1 PostgreSQL unavailable before startup

Expected:

* Sync service fails readiness.
* API service remains independently deployable.
* No misleading healthy status.

## 16.2 PostgreSQL fails during job claim

Expected:

* No job marked running without a lease.
* Worker retries connection safely.
* No duplicate handler execution caused by local assumptions.

## 16.3 PostgreSQL fails during apply

Expected:

* Transaction rolls back.
* Job retries.
* No partial aggregate.
* No realtime event.

## 16.4 PostgreSQL commit result is ambiguous

Simulate connection loss during commit.

Expected strategy:

1. Treat execution outcome as unknown.
2. Retry by unique source key and canonical hash.
3. Database uniqueness and idempotency determine whether apply already succeeded.
4. No duplicate rows or events.

## 16.5 Queue notification fails

Expected:

* Poll fallback claims job.
* Latency may degrade, but correctness remains.

## 16.6 Worker panic

Expected:

* Panic is recovered at job boundary.
* Job lease eventually expires.
* Worker process health reflects failure according to policy.
* Other workers continue.
* Job retry is idempotent.

Do not recover panics deep inside domain logic and continue with possibly corrupt state.

## 16.7 Outbox publisher unavailable

Expected:

* Domain transaction still commits.
* Outbox remains pending.
* Publisher later retries.
* Frontend polling still reads correct database state.

## 16.8 Realtime fanout unavailable

Expected:

* Local database remains correct.
* API reads remain correct.
* Frontend reconnect or query refresh converges.
* Outbox/fanout retry is bounded.

## 16.9 Disk or storage failure for diagnostic HTML

Expected:

* Domain apply decision does not become unsafe.
* Parser failure remains recorded in PostgreSQL.
* No raw payload leak into logs as fallback.

## 16.10 Metrics backend unavailable

Expected:

* Synchronization continues.
* Metric emission failure does not fail domain apply.
* Logs record a bounded diagnostics warning.

---

# 17. Crash recovery scenarios

## CRASH-001 — Detector dies after observing event but before enqueue

Expected:

* Overlapping log window sees event again.
* Event is eventually enqueued.
* Duplicate observation deduplicated.

## CRASH-002 — Detector dies after enqueue

Expected:

* Durable queue retains job.

## CRASH-003 — Worker dies after claim, before source fetch

Expected:

* Lease expires.
* Another worker claims.

## CRASH-004 — Worker dies during source body read

Expected:

* No apply.
* Lease expires or job retries.

## CRASH-005 — Worker dies during transaction

Expected:

* PostgreSQL rolls back connection-owned transaction.
* Retry safe.

## CRASH-006 — Worker dies immediately after commit

Expected:

* Job may retry.
* Canonical hash makes retry a no-op.
* No duplicate audit or outbox.

## CRASH-007 — Outbox publisher dies after publishing but before marking complete

Expected:

* Duplicate invalidation may occur.
* Consumers remain idempotent.
* Domain state remains correct.

## CRASH-008 — Entire deployment restarts

Expected:

* Jobs recover.
* Leader election recovers.
* Authentication re-established.
* Reconciliation checkpoints remain.
* No mass refresh storm beyond configured rate.

---

# 18. Security failure tests

| ID      | Scenario                               | Expected                         |
| ------- | -------------------------------------- | -------------------------------- |
| SEC-001 | Log source request error               | No cookie/token                  |
| SEC-002 | Log login failure                      | No password                      |
| SEC-003 | Conflict payload contains email/phone  | Redacted according to policy     |
| SEC-004 | User calls sync admin API without role | Forbidden                        |
| SEC-005 | Manual refresh flood                   | Rate limited                     |
| SEC-006 | URL path traversal attempt             | Blocked                          |
| SEC-007 | Redirect to unexpected host            | Blocked                          |
| SEC-008 | HTTP downgrade                         | Rejected                         |
| SEC-009 | Credential missing                     | Startup configuration failure    |
| SEC-010 | Raw HTML retention expired             | Artifact deleted                 |
| SEC-011 | Imported teacher                       | Login disabled                   |
| SEC-012 | SQL injection-like source value        | Stored safely through parameters |

---

# 19. Performance and load tests

## 19.1 One-second path benchmark

Configuration:

```text id="xv6fj4"
audit polling: 250 ms
audit response: 50 ms
course detail response: 250 ms
database: real PostgreSQL
realtime: enabled
```

Measure:

```text id="a2ogfr"
source mutation → detection
detection → enqueue
enqueue → claim
claim → fetch complete
fetch complete → commit
commit → realtime
realtime → UI refreshed
```

Pass criteria:

```text id="zy14ma"
p50 ≤ 600 ms
p95 ≤ 1 second
p99 ≤ 3 seconds
zero lost source changes
zero duplicate sessions
```

## 19.2 Burst test

Inject:

* 100 course changes in 10 seconds.
* Ten repeated events per course.
* Five master-data updates.
* Historical reconciliation already running.

Verify:

* Course jobs coalesce.
* P0 schedule jobs are not starved.
* Historical workers pause.
* Source request concurrency remains bounded.
* Queue drains after burst.
* Final local state matches source.

## 19.3 Long-duration soak

Run for at least 24 hours with:

* Random source changes.
* Authentication expiry.
* Random 429/500 responses.
* Worker restarts.
* Reordered log rows.
* Nightly reconciliation.

Pass criteria:

* No memory growth trend.
* No connection leak.
* No goroutine leak.
* No unbounded job growth.
* No stale running leases.
* Zero unexplained reconciliation mismatch.

## 19.4 Historical backfill load

Use the approximate source volume identified in the legacy assessment, including hundreds of courses and hundreds of subjects.

Verify:

* Operational jobs retain priority.
* Backfill can resume from checkpoints.
* Re-running backfill is idempotent.
* Request rate respects source limits.
* Snapshot storage remains within budget.

---

# 20. Property and model-based tests

## 20.1 Tombstone state machine

Generate random valid and invalid event sequences:

```text id="cw8cxq"
seen
missing
generation_failed
seen_again
grace_elapsed
tombstone
restore
```

Properties:

* One missing event never tombstones.
* Failed generation never advances missing count.
* Seeing entity restores active state.
* Tombstone requires completed prerequisites.
* Restore is always possible.

## 20.2 Queue model

Randomly generate:

* Enqueues.
* Duplicate keys.
* Claims.
* Heartbeats.
* Worker deaths.
* Retries.
* Completions.

Compare actual database state against a simple reference model.

Properties:

* No two active claims for one job.
* Terminal jobs are not reclaimed.
* Expired running jobs become claimable.
* Unique active keys remain unique.

## 20.3 Aggregate apply model

Generate source snapshots with random:

* Teacher change.
* Subject change.
* Room change.
* Time change.
* Confirmation change.
* Row reordering.
* Duplicate rows.

Properties:

* Equivalent canonical source yields no-op.
* Legacy schedule ID uniqueness always holds.
* Invalid aggregate produces no domain changes.
* Successful aggregate exactly matches canonical source-owned fields.

---

# 21. End-to-end scenarios

## E2E-001 — Schedule time changed

1. Fake legacy source changes session time.
2. Audit log exposes action.
3. Detector observes it.
4. Queue coalesces course refresh.
5. Worker fetches course detail.
6. Transaction updates existing session.
7. Outbox emits event.
8. React Query invalidates schedule.
9. UI shows new time.

Assert total p95 latency is within target.

## E2E-002 — Room assignment changed

Verify:

* Same session ID.
* New room mapping.
* Room schedule and course detail both refresh.
* No duplicate session.

## E2E-003 — Legacy session removed

Verify:

* First complete generation marks suspected missing.
* UI does not immediately lose session.
* Second complete generation and grace period soft-tombstone it.
* Historical references remain.

## E2E-004 — Parser drift during live operation

Verify:

* UI continues showing last-good schedule.
* Stale indicator appears.
* Operations conflict appears.
* No rows are deleted.

## E2E-005 — Source outage and recovery

Verify:

* Local application stays available.
* Freshness warning appears.
* Jobs retry with backoff.
* Recovery prioritizes today’s schedule.
* Final state converges.

## E2E-006 — Worker crash after commit

Verify:

* Retry occurs.
* No duplicate audit, outbox, or session.
* UI receives one logical update.

---

# 22. Required test files

```text id="h73b87"
backend/internal/legacysync/
├── normalize/
│   ├── text_test.go
│   ├── datetime_test.go
│   ├── hash_test.go
│   └── fuzz_test.go
│
├── parser/
│   ├── course_detail_test.go
│   ├── schedule_test.go
│   ├── logs_test.go
│   ├── teachers_test.go
│   ├── rooms_test.go
│   ├── subjects_test.go
│   └── schema_drift_test.go
│
├── client/
│   ├── client_integration_test.go
│   ├── auth_concurrency_test.go
│   ├── allowlist_test.go
│   └── failure_test.go
│
├── detector/
│   ├── logs_test.go
│   ├── schedule_test.go
│   ├── leader_integration_test.go
│   └── overlap_property_test.go
│
├── apply/
│   ├── masterdata_integration_test.go
│   ├── course_integration_test.go
│   ├── schedule_integration_test.go
│   ├── concurrency_integration_test.go
│   ├── atomicity_integration_test.go
│   ├── idempotency_integration_test.go
│   └── crash_recovery_integration_test.go
│
├── reconcile/
│   ├── generation_integration_test.go
│   ├── tombstone_integration_test.go
│   ├── property_test.go
│   └── catastrophic_empty_test.go
│
└── e2e/
    ├── sync_schedule_test.go
    ├── source_outage_test.go
    ├── parser_drift_test.go
    └── performance_test.go

backend/internal/jobqueue/
├── queue_integration_test.go
├── queue_concurrency_test.go
├── queue_property_test.go
└── queue_crash_recovery_test.go

e2e/
├── legacy-schedule-sync.spec.ts
├── legacy-sync-stale-state.spec.ts
└── legacy-sync-operations.spec.ts
```

---

# 23. CI execution strategy

## Every pull request

```text id="gyunf5"
go test ./...
go vet ./...
npm run typecheck
npm test
npm run migrate:validate
```

## Backend concurrency changes

```text id="cp2jdo"
go test -race ./backend/internal/legacysync/...
go test -race ./backend/internal/jobqueue/...
```

## Parser changes

Run:

* Every sanitized fixture.
* Golden output comparison.
* Login-page rejection.
* Schema-drift tests.
* Fuzz smoke tests.

## Database changes

Run:

* Fresh migration.
* Upgrade from previous migration.
* Down migration where supported.
* Integration suite using real PostgreSQL.
* Concurrent transaction tests.

## Nightly CI

Run:

* Full race suite.
* Property tests with higher iteration count.
* Fuzzing.
* Multi-process concurrency suite.
* Source failure chaos suite.
* Database failure chaos suite.
* 24-hour or shortened soak test in staging.
* Reconciliation corruption-and-repair suite.

---

# 24. Release-blocking acceptance gates

Production application must remain disabled until all of these pass:

## Core correctness

* Zero duplicate external mappings.
* Zero duplicate legacy schedule IDs.
* Aggregate rollback tests pass at every injected failure point.
* Idempotency tests pass after repeated retries.
* Parser drift cannot update domain state.
* Safe-deletion tests pass.

## Concurrency

* Same-course races cannot interleave.
* Queue jobs cannot have two simultaneous owners.
* Authentication expiry does not create a login storm.
* Detector leadership has no unsafe split brain.
* Reconciliation cannot overwrite a newer targeted refresh.
* Crash-after-commit retry creates no duplicate effects.

## External failures

* Complete source outage leaves application usable.
* 429 and 5xx behavior is bounded.
* Authentication revocation stops source hammering.
* Partial HTML cannot remove data.
* Source recovery converges automatically.

## Internal failures

* Database failure causes full rollback.
* Lost queue notification has a polling fallback.
* Outbox failure does not corrupt domain state.
* Worker panic does not permanently lose jobs.
* Deployment restart resumes checkpoints safely.

## Performance

* p95 source-to-frontend freshness is at or below one second under the agreed source latency.
* Request concurrency remains within configured limit.
* Burst load does not starve P0 work.
* Soak test shows no resource leak.
* Historical backfill does not degrade the API server.

## Operations

* Health page distinguishes source, parser, queue, apply, and realtime failures.
* Operations can pause detection, fetching, applying, tombstones, and realtime independently.
* Dead-letter jobs can be inspected and retried.
* Every historical correction is auditable.
* Backup restoration preserves all external mappings.

---

# 25. Highest-priority test implementation order

Implement in this order:

```text id="x60qkm"
1. Canonicalization and hash tests
2. Parser contract and schema-drift tests
3. Safe HTTP client and authentication concurrency tests
4. External mapping uniqueness tests
5. Course aggregate atomicity tests
6. Schedule identity and idempotency tests
7. Same-course concurrency tests
8. Queue lease and crash-recovery tests
9. Reconciliation and tombstone tests
10. Outbox commit-order tests
11. Source failure chaos tests
12. Database failure chaos tests
13. Full one-second end-to-end tests
14. Soak and historical-backfill tests
```

The first production milestone should not be “the scraper successfully downloads pages.” It should be:

> The same source aggregate can be fetched repeatedly, concurrently, interrupted at every important step, and retried after any failure without losing, duplicating, partially applying, or incorrectly deleting course and schedule data.
