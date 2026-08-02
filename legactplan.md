# Production-Grade Legacy Near-Real-Time Synchronization Architecture

## 1. Architecture decision

Build a separate **Legacy Sync Service** that continuously mirrors the old Warwick site into the new system’s PostgreSQL database.

During the coexistence period:

* The **legacy site remains the system of record** for courses, sessions, attendance, students, enrollments, teachers, subjects, classrooms, confirmations, and P-Codes.
* The **new system reads only from its local PostgreSQL database**.
* The new frontend never waits for or directly calls the old site.
* Legacy-owned fields are read-only in the new system until cutover.
* New-system-only functionality, such as absences, notifications, snapshots, and new workflow metadata, remains owned by the new system.

Do not implement bidirectional HTML scraping. It would create unresolved conflicts, duplicate writes, and potentially corrupt the legacy production system.

The legacy analysis confirms that the old site has no usable API or change feed, uses authenticated server-rendered Razor Pages, and exposes authoritative course data through HTML detail and check-in pages.

---

## 2. Important limitation

A strict guarantee of both:

* every legacy change captured with 100% certainty, and
* every change visible in the new system within 1–2 seconds

is not technically possible through HTML scraping alone.

A hard guarantee requires one of:

1. Read-only Azure SQL access.
2. A database change feed.
3. Modifying the legacy application to emit events.
4. Routing all legacy writes through a controlled proxy.

Without those, the correct production model is:

### Fast path

Target **p95 freshness of 1–2 seconds** for operational data:

* Current check-ins.
* Current session confirmation.
* Classroom assignment.
* Today’s schedule.
* Enrollment changes affecting current courses.

### Correctness path

Continuously reconcile all entities so missed or unobservable changes are eventually repaired.

Recommended SLO:

| Data category                      |    Freshness target | Maximum repair window |
| ---------------------------------- | ------------------: | --------------------: |
| Running-session check-ins          |     p95 ≤ 2 seconds |            10 seconds |
| Today’s schedule and confirmation  |     p95 ≤ 2 seconds |            15 seconds |
| Current-course attendees           |     p95 ≤ 5 seconds |            60 seconds |
| Courses, teachers, rooms, subjects |    p95 ≤ 10 minutes |            30 minutes |
| Historical data                    | Nightly convergence |              24 hours |

If absolute 1–2 second consistency is a business requirement for every entity, obtaining read-only SQL access is mandatory.

---

## 3. Proposed system

```text
                         ┌──────────────────────────┐
                         │ Legacy Warwick site      │
                         │ ASP.NET Razor Pages      │
                         └────────────┬─────────────┘
                                      │
                         authenticated read-only HTTP
                                      │
              ┌───────────────────────▼───────────────────────┐
              │ Legacy Sync Service                           │
              │ backend/cmd/legacy-sync                       │
              │                                               │
              │  1. Session/auth manager                      │
              │  2. Change detectors                          │
              │  3. Fetch scheduler                           │
              │  4. HTML parsers                              │
              │  5. Schema-drift validator                    │
              │  6. Normalizer                                │
              │  7. Transactional applier                     │
              │  8. Reconciliation engine                     │
              └──────────┬──────────────────────┬─────────────┘
                         │                      │
                    durable jobs          raw/change snapshots
                         │                      │
              ┌──────────▼───────────┐   ┌────▼───────────────┐
              │ PostgreSQL job queue │   │ Sync staging/state │
              │ leases + retries     │   │ hashes + mappings  │
              └──────────┬───────────┘   └────┬───────────────┘
                         │                    │
                         └──────────┬─────────┘
                                    │ transactional apply
                         ┌──────────▼───────────┐
                         │ New domain database  │
                         │ courses, sessions,   │
                         │ students, attendance │
                         └──────────┬───────────┘
                                    │ outbox / NOTIFY
                         ┌──────────▼───────────┐
                         │ Existing realtime    │
                         │ fanout and hub        │
                         └──────────┬───────────┘
                                    │
                         ┌──────────▼───────────┐
                         │ React application    │
                         │ local reads only     │
                         └──────────────────────┘
```

The repository is already well positioned for this design:

* The backend uses Go, pgx, PostgreSQL, and Goose migrations.
* The server already runs a Postgres-backed CRM queue worker and initializes cross-instance realtime fanout.
* The existing queue supports leases, heartbeat renewal, retry backoff, unique-key deduplication, `FOR UPDATE SKIP LOCKED`, and PostgreSQL `LISTEN/NOTIFY`.
* The realtime hub already supports channels, local delivery, and cross-instance fanout.
* The React application already has replacements for the primary legacy administration routes.

No Kafka, RabbitMQ, Redis, or separate streaming platform is necessary at the current data volume.

---

## 4. Two synchronization loops

### 4.1 Fast operational loop

The fast loop watches only pages that can indicate currently relevant changes.

#### Audit-log detector

Poll `/Admin/Logs` every second.

Use it only as a **change accelerator**, not as the authoritative change journal, because:

* The page may show only the latest rows.
* Stable log IDs have not been verified.
* Two identical actions may occur in the same second.
* Some mutation paths may not create log rows.

For each unseen action:

1. Parse entity type and identifiers from the action.
2. Generate a deduplication key.
3. Enqueue a targeted refresh.
4. Coalesce repeated changes for the same entity for approximately 200–500 ms.

Examples:

```text
Add course attendee [7306, W260038]
    -> refresh course 7306 detail and its attendee set

Confirm course schedule [112741]
    -> refresh schedule 112741 and its parent course

Check in [...]
    -> refresh the corresponding check-in page

Edit teacher [78]
    -> refresh teacher list or teacher 78
```

When an action cannot be parsed, enqueue a bounded reconciliation job rather than silently ignoring it.

#### Today-schedule detector

Poll the read-only `/Home/Schedule` search operation every second for:

* Today.
* Tomorrow near the end of the operating day.
* Any date currently displayed by active staff users, when known.

Hash the canonicalized schedule result. Enqueue updates only when the hash changes.

#### Running-session detector

For each session currently active, or starting within a configured window:

* Poll its check-in page every second.
* Poll its course detail page less frequently for confirmation and classroom changes.
* Stop the high-frequency polling after the session ends plus a safety window.

Normally only a few sessions are active simultaneously, making this much safer than scanning every course.

### 4.2 Integrity reconciliation loop

This loop guarantees convergence when the fast detector misses a change.

Recommended schedule:

| Source                                | Strategy                           |
| ------------------------------------- | ---------------------------------- |
| Running session check-in pages        | Every 1 second                     |
| Today schedule                        | Every 1 second                     |
| Course details for today’s courses    | Rotating every 5–10 seconds        |
| Course affected by a log action       | Immediately                        |
| Active course details                 | Complete sweep every 10–15 minutes |
| Course list                           | Every 5–10 minutes                 |
| Teachers, subjects, classrooms, staff | Every 10–30 minutes                |
| Archived and historical courses       | Nightly                            |
| Full cross-entity validation          | After every complete generation    |

Do not poll the entire courses list every second. The legacy report found that the page is approximately 412 KB. Polling that one page every second would transfer roughly **35 GB per day**, before course-detail and check-in traffic, and would create unnecessary load on the old Azure application.

---

## 5. Legacy HTTP client

Use Go’s `net/http`, cookie jar, and `golang.org/x/net/html`. A headless browser should not be the normal ingestion mechanism because the site is server-rendered and does not require browser JavaScript for its data.

### Session lifecycle

```text
GET /Account/Login
    -> capture cookies
    -> parse __RequestVerificationToken

POST /Account/Login
    -> username/password/token
    -> capture Identity and ARRAffinity cookies

GET allowed read pages
    -> detect login redirect or expired session
    -> reauthenticate through a single synchronized refresh
```

Requirements:

* One shared authentication manager.
* Separate cookie jars only when multiple source sessions are proven safe.
* A mutex or singleflight mechanism around reauthentication.
* Maximum one automatic login retry per failed request.
* Circuit breaker after repeated authentication failures.
* Immediate alert on 401, 403, login-page redirect, or changed form signature.
* Secrets loaded from the deployment secret manager, never source control.
* No credentials, cookies, antiforgery tokens, or raw HTML containing secrets in application logs.

### Read-only endpoint policy

Create an explicit allowlist.

Allowed examples:

```text
GET  /Admin/Courses
GET  /Admin/Courses/Detail?id=...
GET  /Admin/Courses/CheckIn?courseScheduleId=...&courseId=...
GET  /Admin/Teachers
GET  /Admin/Subjects
GET  /Admin/Classrooms
GET  /Admin/Staffs
GET  /Admin/Logs
POST /Home/Schedule              read-only search
POST /Home/Summary?handler=search
POST /Admin/Courses?handler=search
```

Blocked examples:

```text
handler=confirm
handler=studentdelete
handler=coursedelete
handler=import
POST /Admin/Courses/CheckIn
POST /Admin/Courses/ClassroomSet
POST /Admin/Courses/New
POST /Admin/Courses/Edit
```

Reject any request not explicitly classified as a safe read.

---

## 6. Parsing and schema-drift safety

A parser must never continue with partially understood HTML.

Each page parser should verify a page signature before extracting data:

* Expected page title.
* Expected table headers.
* Expected form field names.
* Minimum and maximum expected column count.
* Required hidden inputs.
* Known date and time format.
* Known identifier patterns.

Example:

```go
type PageSignature struct {
    PageType        string
    RequiredHeaders []string
    RequiredFields  []string
    ParserVersion   int
}
```

When the signature fails:

1. Do not update domain data.
2. Preserve the previous last-good snapshot.
3. Store a redacted and compressed copy of the unexpected HTML.
4. Mark the page as `parser_drift`.
5. Alert immediately.
6. Pause related destructive reconciliation.

Normalization rules should include:

* Unicode normalization.
* Whitespace and non-breaking-space cleanup.
* Thai and English date parsing.
* Asia/Bangkok timezone conversion.
* Empty value normalization.
* Stable ordering before hashing.
* Preservation of original raw text for auditing.
* Exact legacy identifiers retained as strings when leading zeroes are possible.

Every normalized aggregate receives a deterministic SHA-256 hash. An unchanged hash results in a no-op.

---

## 7. Database model

### `external_refs`

Maps legacy identities to new UUID identities.

```sql
CREATE TABLE external_refs (
    source             text        NOT NULL,
    entity_type        text        NOT NULL,
    external_id        text        NOT NULL,
    internal_id        uuid        NOT NULL,
    source_hash        text,
    first_seen_at      timestamptz NOT NULL DEFAULT now(),
    last_seen_at       timestamptz NOT NULL DEFAULT now(),
    last_applied_at    timestamptz,
    last_generation    bigint,
    state              text        NOT NULL DEFAULT 'active',
    PRIMARY KEY (source, entity_type, external_id)
);
```

Examples:

```text
legacy_warwick / course          / 7306
legacy_warwick / schedule        / 112741
legacy_warwick / attendee        / 5586
legacy_warwick / student         / W260038
legacy_warwick / teacher         / 78
legacy_warwick / classroom       / 120201
legacy_warwick / attendance      / 112741:W260038
```

### `legacy_change_events`

Append-only detected changes.

```sql
CREATE TABLE legacy_change_events (
    id                uuid PRIMARY KEY,
    source_event_key  text UNIQUE NOT NULL,
    detector          text NOT NULL,
    entity_type       text,
    external_id       text,
    action            text,
    observed_at       timestamptz NOT NULL,
    raw_payload       jsonb,
    status            text NOT NULL,
    processed_at      timestamptz,
    last_error        text
);
```

### `legacy_entity_snapshots`

Stores normalized source state.

```sql
CREATE TABLE legacy_entity_snapshots (
    source            text NOT NULL,
    entity_type       text NOT NULL,
    external_id       text NOT NULL,
    canonical_data    jsonb NOT NULL,
    source_hash       text NOT NULL,
    parser_version    integer NOT NULL,
    observed_at       timestamptz NOT NULL,
    applied_at        timestamptz,
    quality           text NOT NULL,
    PRIMARY KEY (source, entity_type, external_id)
);
```

### `legacy_sync_runs`

Tracks every targeted refresh and full generation.

Important fields:

* Mode: `targeted`, `hot_sweep`, `full_sweep`.
* Started and completed timestamps.
* Pages requested.
* Entities parsed.
* Entities changed.
* Entities applied.
* Parse failures.
* Reconciliation mismatches.
* Source latency.
* Run status.

### Raw page retention

Do not persist every unchanged one-second poll.

Persist raw HTML only when:

* The normalized content changed.
* Parsing failed.
* A reconciliation mismatch was found.
* Debug sampling selected the request.

Compress the body and apply a short retention policy.

---

## 8. Transactional apply model

Treat a course and its child records as one aggregate.

For a course refresh:

```text
Course
├── schedules
├── attendees
│   └── student profile references
├── confirmations
├── classroom assignments
└── attendance/check-in state
```

Apply algorithm:

```text
BEGIN

1. Acquire an advisory lock for source/entity/external ID.
2. Lock the current external_refs row.
3. Compare the incoming source hash.
4. Return without changes when the hash is unchanged.
5. Validate all required parent references.
6. Upsert the legacy-owned domain aggregate.
7. Upsert external mappings.
8. Store the normalized source snapshot.
9. Write a sync audit record.
10. Write an outbox/realtime event.
11. Commit.

After commit:
12. Publish the realtime notification.
```

Never expose a partially updated aggregate where schedules were updated but attendees were not.

The repository already demonstrates strong transaction, concurrency, stale-edit, and rollback testing around course operations. The legacy applier should follow the same pattern rather than performing unstructured direct SQL writes.

---

## 9. Deletion and missing-record handling

Absence from one scrape must not mean deletion.

A source entity should be tombstoned only after:

1. It is absent from at least two complete successful generations.
2. Both generations covered the relevant archive filters.
3. The parent page was parsed successfully.
4. The missing entity is not referenced by a currently visible child.
5. No source-side error or partial page was detected.

Recommended states:

```text
active
suspected_missing
confirmed_missing
tombstoned
conflict
parser_error
```

Initially soft-delete or deactivate records. Do not hard-delete synchronized legacy records automatically.

---

## 10. Source ownership

Use an explicit field-ownership matrix.

| Entity or field                      | Owner before cutover |
| ------------------------------------ | -------------------- |
| Legacy course ID and C-Code          | Legacy               |
| Course teacher and subject           | Legacy               |
| Session date and time                | Legacy               |
| Classroom assignment                 | Legacy               |
| Student profile imported from legacy | Legacy               |
| Enrollment and P-Codes               | Legacy               |
| Legacy check-in and confirmation     | Legacy               |
| New-system absence workflows         | New system           |
| Notification delivery state          | New system           |
| Snapshot and audit metadata          | New system           |
| New-system UI preferences            | New system           |

In the React application:

* Disable editing for legacy-owned fields.
* Display “Managed by legacy system”.
* Show `Last synchronized at`.
* Show a warning when freshness exceeds the SLO.
* Keep new-system-only actions available.

This avoids a dual-writer architecture.

---

## 11. Realtime frontend behavior

The API should always return local database state immediately.

After the sync service commits a change, publish events such as:

```json
{
  "type": "legacy.course.updated",
  "channel": "course:7306",
  "id": "7306",
  "payload": {
    "synced_at": "2026-08-03T03:03:01+07:00"
  }
}
```

Suggested channels:

```text
legacy-sync
schedule:2026-08-03
course:<legacy-course-id>
session:<legacy-schedule-id>
student:<wcode>
```

The React Query cache can invalidate only the affected query instead of reloading every page.

The current realtime hub already handles channel subscriptions and cross-instance delivery, so the sync service should publish through that existing mechanism.

For QR and check-in screens:

* Generate QR data from local session records.
* Never wait for the scraper during page load.
* Trigger an optional high-priority refresh asynchronously.
* Push the refreshed state through realtime.
* Display the local last-sync timestamp.

This keeps the UI fast even when the old site is slow.

---

## 12. Queue integration

The current queue is technically suitable, but it is named and persisted as CRM-specific infrastructure through `crm_jobs` and CRM job enums.

Recommended refactor:

```text
backend/internal/crmimport/queue
        ↓
backend/internal/jobqueue
```

Parameterize:

* Table name.
* Notification channel.
* Job-type validation.
* Worker concurrency.
* Lease duration.
* Retry policy.

Keep a compatibility adapter for existing CRM jobs.

Do not simply add legacy scraping jobs to `crm_jobs`; this creates long-term domain coupling.

Legacy job types:

```text
legacy_refresh_log
legacy_refresh_schedule_day
legacy_refresh_course
legacy_refresh_checkin
legacy_refresh_teacher_list
legacy_refresh_subject_list
legacy_refresh_classroom_list
legacy_full_reconcile
legacy_verify_generation
legacy_publish_changes
```

Use unique keys such as:

```text
legacy:course:7306
legacy:checkin:112741
legacy:schedule:2026-08-03
legacy:teachers
```

Repeated events update the existing queued job rather than creating a backlog.

The existing worker processes one claimed job at a time per worker instance. For the legacy pipeline, run:

* One detector leader.
* Two to four bounded fetch workers.
* One or two transactional apply workers.

Concurrency must remain configurable and source-rate-limited.

---

## 13. Load protection

Start conservatively:

```text
Sustained requests: 2 requests/second
Burst:              4 requests
Concurrent requests: 2
Target timeout:      5 seconds
Hard timeout:        10 seconds
```

Adapt automatically:

* Reduce concurrency when legacy p95 latency increases.
* Reduce rate on 429 or 5xx responses.
* Open the circuit after repeated authentication or anti-bot failures.
* Add randomized jitter to periodic jobs.
* Prioritize active check-in pages over historical sweeps.
* Pause background sweeps when the hot queue is non-empty.

Priority order:

```text
P0 Running-session check-ins
P1 Today schedule, confirmation, classroom
P2 Changed course attendee sets
P3 Current active courses
P4 Master data
P5 Historical reconciliation
```

---

## 14. Failure handling

### Session expiry

* Detect login redirect.
* Stop normal fetch workers temporarily.
* Reauthenticate once.
* Resume queued work.
* Alert if reauthentication fails.

### Legacy site unavailable

* Continue serving last-good local data.
* Mark synchronization as degraded.
* Preserve jobs with exponential backoff and jitter.
* Do not block API requests.
* Alert when freshness exceeds thresholds.

### HTML schema changes

* Fail closed.
* Store the changed page.
* Keep last-good domain state.
* Stop deletion reconciliation.
* Alert with page type and parser version.

### Duplicate or reordered log rows

* Use a rolling overlapping window.
* Track multiplicity of identical fingerprints.
* Deduplicate targeted refresh jobs by entity key.
* Rely on full sweeps for correctness.

### Process crash

* Job leases expire.
* Another worker reclaims the job.
* Idempotent source hashes prevent duplicate mutation.

### Multiple sync instances

* One detector instance obtains a PostgreSQL advisory leader lock.
* Fetch and apply jobs use queue leases and `SKIP LOCKED`.
* Per-entity advisory locks prevent concurrent aggregate application.

### Partial page or malformed data

* Reject the entire aggregate.
* Never mix old attendees with a newly parsed partial schedule.
* Preserve last-good data.

---

## 15. Observability

Expose metrics by entity and page type.

### Freshness

```text
legacy_sync_freshness_seconds
legacy_last_success_timestamp
legacy_last_complete_generation_timestamp
legacy_entity_apply_lag_seconds
```

### Source health

```text
legacy_http_request_duration_seconds
legacy_http_errors_total
legacy_auth_failures_total
legacy_rate_limit_total
legacy_session_refresh_total
```

### Parser health

```text
legacy_parse_failures_total
legacy_schema_drift_total
legacy_rows_parsed_total
legacy_page_hash_changes_total
```

### Queue health

```text
legacy_jobs_queued
legacy_jobs_running
legacy_job_oldest_age_seconds
legacy_job_retries_total
legacy_job_failures_total
```

### Correctness

```text
legacy_reconciliation_mismatches
legacy_missing_entity_suspicions
legacy_conflicts_total
legacy_entities_changed_total
legacy_noop_applies_total
```

Required alerts:

* Current-session freshness exceeds 5 seconds.
* Today-schedule freshness exceeds 10 seconds.
* Authentication failure.
* Parser drift.
* Queue oldest age exceeds the operational SLO.
* Reconciliation mismatch remains after a full generation.
* Full reconciliation has not completed successfully.
* Legacy 429 or 5xx error rate crosses its threshold.

Add a protected administration endpoint:

```text
GET /api/v1/admin/legacy-sync/health
GET /api/v1/admin/legacy-sync/runs
GET /api/v1/admin/legacy-sync/conflicts
POST /api/v1/admin/legacy-sync/refresh
```

The manual refresh endpoint should enqueue work, not scrape synchronously.

---

## 16. Security requirements

1. Create a dedicated read-only legacy account where possible.
2. Remove open self-registration from the legacy system.
3. Replace the existing short password.
4. Store credentials only in the deployment secret manager.
5. Redact cookies, antiforgery values, student contact details, and HTML bodies from logs.
6. Encrypt retained raw pages or store them in access-controlled object storage.
7. Apply retention limits to raw HTML and source snapshots.
8. Add an egress allowlist for the legacy host.
9. Use an explicit read-only route allowlist.
10. Record every synchronization action in a new-system audit log.
11. Remove legacy HTML dumps and embedded tokens from deployment artifacts after their test fixtures are sanitized.

The repository’s migration documentation already identifies legacy static files and embedded CSRF material as security cleanup priorities.

---

## 17. Repository layout

```text
backend/
├── cmd/
│   └── legacy-sync/
│       └── main.go
│
├── internal/
│   ├── legacysync/
│   │   ├── config.go
│   │   ├── service.go
│   │   ├── scheduler.go
│   │   ├── detector.go
│   │   ├── reconciler.go
│   │   ├── apply.go
│   │   ├── ownership.go
│   │   ├── metrics.go
│   │   ├── errors.go
│   │   │
│   │   ├── client/
│   │   │   ├── client.go
│   │   │   ├── auth.go
│   │   │   ├── allowlist.go
│   │   │   ├── ratelimit.go
│   │   │   └── circuitbreaker.go
│   │   │
│   │   ├── parser/
│   │   │   ├── signature.go
│   │   │   ├── courses.go
│   │   │   ├── course_detail.go
│   │   │   ├── checkin.go
│   │   │   ├── schedule.go
│   │   │   ├── teachers.go
│   │   │   ├── subjects.go
│   │   │   ├── classrooms.go
│   │   │   └── logs.go
│   │   │
│   │   ├── normalize/
│   │   │   ├── dates.go
│   │   │   ├── text.go
│   │   │   └── hashes.go
│   │   │
│   │   └── fixtures/
│   │       └── sanitized HTML fixtures
│   │
│   ├── jobqueue/
│   └── httpapi/
│       └── legacysynchtp/
│
└── db/
    ├── migrations/
    │   └── <next>_legacy_sync_infrastructure.sql
    └── queries/
        └── legacy_sync.sql

src/
└── features/
    └── legacySync/
        ├── api.ts
        ├── SyncStatusBadge.tsx
        ├── SyncHealthPage.tsx
        └── useLegacyFreshness.ts
```

The sync process should be deployed separately from `backend/cmd/server`. A legacy outage, parser panic, or long reconciliation must not affect the API server lifecycle.

---

## 18. Testing strategy

### Parser fixture tests

Capture and sanitize HTML for:

* Successful login.
* Expired login.
* Empty tables.
* `[NOT SET]` classroom.
* Active and archived courses.
* Private and general courses.
* Draft courses.
* Multiple attendees.
* Empty attendees.
* Confirmed and unconfirmed schedules.
* Checked and unchecked students.
* Duplicate log messages.
* Thai text and unusual whitespace.
* Malformed and changed table headers.

Use golden normalized JSON outputs.

### Fake legacy server integration tests

Implement an `httptest.Server` that supports:

* Antiforgery tokens.
* Cookie authentication.
* Login redirect.
* Session expiry.
* Slow responses.
* 429 throttling.
* 500 errors.
* Reordered log rows.
* Missing rows.
* Partially written HTML.
* Duplicate actions.
* Changing page signatures.

### Database integration tests

Verify:

* Idempotent repeated apply.
* Aggregate atomicity.
* Source-hash no-op.
* Concurrent refresh of the same course.
* Parent-before-child mapping.
* Crash after staging but before apply.
* Crash after apply but before job completion.
* Two full sweeps before tombstone.
* Reappearance of a suspected-missing entity.
* Conflict with a new-system-owned field.
* Realtime outbox written in the same transaction.
* No partial update after constraint failure.

### Reconciliation tests

Starting from intentionally corrupted local data:

* Restore missing attendee.
* Restore deleted schedule.
* Fix incorrect confirmation.
* Detect stale student details.
* Avoid deleting an entity during a partial scrape.
* Produce zero mismatches after complete reconciliation.

### Load tests

Test against a fake source first.

Production validation should begin in shadow mode with:

* Low request rate.
* No domain writes.
* Source latency monitoring.
* Page-size monitoring.
* Comparison against manually verified legacy pages.

---

## 19. Rollout plan

### Phase 1 — Source contract validation

Verify:

* Whether `/Admin/Logs` contains every mutation type.
* Whether logs have stable identifiers.
* How many rows the log page retains.
* Whether check-in changes appear in course detail or only on check-in pages.
* Session-cookie lifetime.
* Source response latency.
* Safe sustained request rate.
* Whether conditional headers such as ETag or Last-Modified are available.

### Phase 2 — Baseline migration

* Import all master data.
* Import all courses and course details.
* Preserve every legacy ID.
* Reconcile entity counts.
* Do not enable continuous writes yet.

### Phase 3 — Shadow synchronization

* Run detectors and parsers.
* Store snapshots and hashes.
* Do not update domain tables.
* Compare generated changes with the old site manually.
* Validate request load.

### Phase 4 — Static entities

Enable synchronization for:

* Teachers.
* Subjects.
* Classrooms.
* Students.
* Courses.
* Enrollments.

### Phase 5 — Operational entities

Enable:

* Sessions.
* Confirmation.
* Classroom assignments.
* Check-ins.
* Current-day schedule.

### Phase 6 — Realtime UI

* Publish granular invalidation events.
* Add freshness indicators.
* Disable legacy-owned edits.
* Add degraded-state UX.

### Phase 7 — Cutover

1. Announce a legacy write freeze.
2. Wait for in-flight legacy operations to complete.
3. Run a final full synchronization.
4. Require zero reconciliation mismatches.
5. Switch legacy-owned fields to new-system ownership.
6. Enable new-system writes.
7. Keep the sync service in read-only verification mode temporarily.
8. Decommission the legacy site only after sign-off.

---

## 20. Acceptance criteria

The implementation is production-ready when:

1. No API or frontend request synchronously accesses the old site.
2. Current check-in freshness meets the defined SLO under expected load.
3. The system continues serving last-good data during legacy outages.
4. Repeated source data produces no duplicate records or updates.
5. Parser drift cannot overwrite valid domain data.
6. Every synchronized record retains its legacy identity.
7. No entity is deleted because of one missing or partial scrape.
8. Every complete reconciliation reports zero unexplained mismatches.
9. Queue workers recover correctly after process crashes.
10. Multiple instances cannot concurrently corrupt the same aggregate.
11. Realtime events are emitted only after transaction commit.
12. Legacy-owned and new-system-owned fields cannot overwrite one another.
13. Credentials, cookies, tokens, and student data do not appear in logs.
14. A full source outage does not increase normal API response latency.
15. The old site receives bounded and observable request traffic.

---

## Final recommendation

Implement a **hybrid fast-path plus reconciliation architecture**:

* Poll only high-value operational pages every second.
* Use the audit log as an accelerator, not a correctness guarantee.
* Perform targeted refreshes after detected changes.
* Continuously reconcile all source entities.
* Apply updates idempotently and transactionally.
* Keep legacy data single-writer and read-only in the new application.
* Reuse the existing PostgreSQL queue and realtime patterns after extracting them from CRM-specific packages.
* Run synchronization as an isolated service.
* Require read-only SQL access if the business truly requires hard 100% capture with universal 1–2 second freshness.

This delivers a fast user experience, protects the fragile legacy site, and provides a defensible path to eventual 100% convergence and safe cutover.
 Yes. The core sync architecture is good, but several **operational-control areas** are still needed before it is safe to run continuously against production.

The repository already provides strong foundations: PostgreSQL queue leases and retries, `LISTEN/NOTIFY`, realtime fanout, course versioning, transactional course operations, and schedule conflict controls.

## P0 — Required before production

### 1. Define the exact one-second SLO

“One-second sync” must have a precise start and end:

```text
source write completed
→ source change observable
→ detected
→ scraped
→ committed locally
→ visible in frontend
```

Measure separately:

| Metric                   | Suggested target |
| ------------------------ | ---------------: |
| Detection delay p95      |         ≤ 250 ms |
| Fetch and parsing p95    |         ≤ 500 ms |
| Database apply p95       |         ≤ 100 ms |
| Realtime delivery p95    |         ≤ 100 ms |
| End-to-end freshness p95 |       ≤ 1 second |
| End-to-end freshness p99 |      ≤ 3 seconds |

Also synchronize clocks on every server and store all measurements in UTC. Otherwise, freshness measurements will be unreliable.

### 2. Prove the legacy change-detection contract

Before building around `/Admin/Logs`, verify:

* Does every course edit create a log?
* Does every schedule creation, deletion, confirmation, and room change create a log?
* Are log entries ordered reliably?
* Are timestamps precise only to seconds?
* How many rows are retained?
* Can multiple identical actions occur in the same second?
* Can the page be filtered or paginated?

The legacy investigation only confirmed a small visible log set; it did not prove that logs are a complete journal.

Run a controlled test matrix:

```text
Create course
Edit course
Archive course
Add schedule
Change schedule time
Delete schedule
Change teacher
Change room
Confirm schedule
Unconfirm schedule, if supported
Edit teacher
Edit subject
Edit classroom
```

For each action, record which page changes and which log entry appears.

### 3. Correctness must not depend on audit logs

Use three levels:

```text
Level 1: audit-log targeted refresh
Level 2: today-schedule hash comparison
Level 3: periodic complete reconciliation
```

Logs provide speed. Reconciliation provides correctness.

Every source object should have:

* Canonical normalized representation.
* Source hash.
* Legacy ID.
* First-seen timestamp.
* Last-seen timestamp.
* Last-applied timestamp.
* Last successful generation.
* Parser version.
* Synchronization status.

### 4. Production-safe deletion policy

Never delete a course or session because it disappeared once.

Require:

1. Two complete successful source generations.
2. The entity is missing in both generations.
3. Active and archived filters were both checked.
4. No parser or authentication failure occurred.
5. Parent and child references agree.
6. A configurable grace period has passed.

Use:

```text
active
suspected_missing
confirmed_missing
tombstoned
restored
```

Hard deletion should require a manual operation or long retention period.

### 5. Separate legacy writes from normal domain writes

The existing scheduling service correctly validates teacher, room, student, and time conflicts when users create schedules.

Legacy import cannot always use those same rules because historical source data may already contain:

* Teacher conflicts.
* Room conflicts.
* Missing rooms.
* Invalid old combinations.
* Records that violate current rules.

Create a dedicated trusted synchronization write path:

```text
Normal staff write
    → full scheduling validation

Legacy synchronization write
    → source-shape validation
    → referential validation
    → preserve source truth
    → report domain conflicts
```

Do not let the sync service call normal HTTP scheduling endpoints.

### 6. Schema-drift fail-safe

Every parser needs a page contract:

```go
type PageContract struct {
    PageType           string
    ParserVersion      int
    ExpectedTitle      string
    RequiredHeaders    []string
    RequiredFormFields []string
    MinimumRows        int
}
```

On an unexpected HTML structure:

* Do not write partial data.
* Keep the last-good local version.
* Disable deletion processing.
* Store a redacted failure sample.
* Alert operations.
* Mark the page `parser_drift`.

This is essential because the old system only exposes server-rendered HTML, not a stable API.

---

## P1 — Reliability and availability

### 7. Leader election and duplicate-instance safety

Run only one detector leader, but allow multiple fetch and apply workers.

Use PostgreSQL advisory locks:

```text
Detector leader lock:
legacy-sync:detector

Entity apply lock:
legacy-sync:course:7306
legacy-sync:teacher:78
legacy-sync:schedule:112741
```

Queue leases already protect individual jobs, but detector leadership and per-aggregate serialization are separate concerns.

### 8. Explicit backpressure

The old site must be protected from overload.

Use separate queues or priority columns:

| Priority | Work                            |
| -------- | ------------------------------- |
| P0       | Schedule change affecting today |
| P1       | Any changed course detail       |
| P2       | Teacher, room, subject refresh  |
| P3       | Active-course sweep             |
| P4       | Historical reconciliation       |

Rules:

* P0 always runs before historical work.
* Pause historical crawling when P0/P1 backlog exists.
* Coalesce duplicate course jobs.
* Set maximum queue age.
* Set maximum concurrent legacy requests.
* Reject unlimited manual refresh requests.

### 9. Adaptive source rate limiting

Static polling rates are not enough.

Adjust automatically using:

* Legacy response p95.
* 429 responses.
* 5xx responses.
* Connection failures.
* Login failures.
* Queue backlog.
* Current course activity.

Example:

```text
Healthy:
2 concurrent requests, controlled burst

Source slowing:
1 concurrent request, reduced background work

Source failing:
circuit open, only periodic health probe
```

The high-frequency detector should request small pages. It must not download the large course list every second.

### 10. Graceful degradation

During a legacy outage:

* New application continues serving local data.
* Course and schedule pages remain usable.
* Users see “Last synchronized 48 seconds ago”.
* Native new-system workflows continue.
* Legacy-owned edits remain disabled.
* Alerts fire only after configured thresholds.
* Queue jobs remain durable.
* No API request waits on the legacy site.

The old site must never become a runtime dependency of the new frontend.

### 11. Disaster recovery

Back up more than domain tables.

Required backup scope:

* `external_refs`.
* Legacy snapshots.
* Source hashes.
* Sync run history.
* Conflict records.
* Job state.
* Parser versions.
* Audit events.

Test recovery:

1. Restore PostgreSQL into a new environment.
2. Restart synchronization.
3. Verify no duplicate courses or schedules.
4. Verify external IDs still map to the same internal UUIDs.
5. Verify historical tombstones are preserved.
6. Run complete reconciliation.
7. Confirm zero unexplained differences.

### 12. Poison-job handling

Some source records may fail repeatedly.

After the maximum retry count:

* Move the job to a dead-letter state.
* Preserve its payload and source page reference.
* Categorize the error.
* Stop automatic rapid retry.
* Show it in the management interface.
* Allow controlled manual retry.

Categories:

```text
authentication
source_unavailable
rate_limited
parser_drift
invalid_source_data
missing_reference
database_constraint
mapping_conflict
internal_bug
```

---

## P1 — Data integrity

### 13. Referential synchronization ordering

For a course detail:

```text
Teacher
Subject
Rooms
Course
Series containers
Sessions
Confirmation metadata
```

When a reference is missing:

1. Do not create an arbitrary duplicate.
2. Enqueue the missing master-data refresh.
3. Mark the course `pending_reference`.
4. Retry after master data succeeds.
5. Escalate if unresolved.

### 14. Duplicate-detection strategy

Do not match by display name alone.

Examples that can collide:

* Two teachers with similar names.
* Renamed rooms.
* Subjects with repeated names.
* Inactive and active versions of a teacher.

Use legacy numeric IDs as stable identity. For records lacking IDs, maintain an explicit manually approved alias table rather than guessing.

### 15. Historical immutability

Historical schedules may be referenced by:

* Absence records.
* Attendance.
* Notifications.
* Snapshot records.
* Audit logs.

The repository already uses immutable session snapshots in absence-related flows and enforces snapshot consistency.

Therefore:

* Do not physically rewrite historical meaning silently.
* Record every source-driven historical correction.
* Preserve previous normalized snapshots.
* Publish a `legacy.historical_record_corrected` audit event.
* Run downstream impact analysis when a synchronized historical session changes.

### 16. Time and timezone rules

Legacy dates appear to use local Thai date/time formats.

Define one normalization standard:

```text
Source parse zone: Asia/Bangkok
Database storage: timestamptz in UTC
Business date calculations: Asia/Bangkok
Frontend display: institute timezone
```

Test:

* Midnight boundaries.
* Schedule crossing midnight.
* Date format ambiguity.
* Missing end time.
* End time earlier than start time.
* Thai and English month names.
* Server clock drift.
* Historical dates before current timezone rules.

### 17. Source-value preservation

Store both normalized and raw values:

```json
{
  "normalized": {
    "start_at": "2026-08-03T04:00:00Z"
  },
  "source": {
    "date": "03/08/26",
    "begin": "11:00",
    "end": "13:00"
  }
}
```

This makes parser bugs diagnosable without repeatedly requesting the old site.

---

## P1 — Deployment and change management

### 18. Shadow mode

Before domain writes:

* Scrape continuously.
* Parse and store snapshots.
* Calculate proposed changes.
* Do not apply to production tables.
* Compare against manually checked legacy pages.
* Measure source load.
* Measure one-second performance.
* Validate no false deletions.

Run shadow mode long enough to cover normal staff workflows.

### 19. Canary release

Enable synchronization incrementally:

```text
1 teacher
1 room
5 courses
today’s schedule
all active courses
all historical courses
```

Create allowlists:

```text
LEGACY_SYNC_COURSE_ALLOWLIST
LEGACY_SYNC_ENTITY_TYPES
LEGACY_SYNC_APPLY_ENABLED
LEGACY_SYNC_DELETION_ENABLED
```

Deletion should have a separate feature flag from update synchronization.

### 20. Instant rollback

Rollback must not require a redeployment.

Operational switches:

```text
Pause detection
Pause fetching
Pause applying
Disable tombstones
Disable realtime publishing
Run read-only reconciliation
Resume selected entity
```

When application is paused, the queue and checkpoints must remain intact.

### 21. Database migration rollout

Use expand-and-contract:

1. Add nullable source columns and mapping tables.
2. Deploy code capable of reading old and new shapes.
3. Backfill mappings.
4. Validate.
5. Add indexes concurrently where supported.
6. Enable writes.
7. Add constraints only after validation.
8. Remove transitional code later.

Do not add blocking constraints before historical source quality is understood.

---

## P1 — Security and access

### 22. Dedicated source identity

Create a dedicated read-only legacy account where possible.

It should not be able to:

* Confirm schedules.
* Change rooms.
* Edit courses.
* Delete students.
* Import data.
* Register accounts.
* Manage users.

If legacy permissions cannot provide true read-only access, the HTTP client’s route allowlist becomes a critical safety boundary.

### 23. Secret lifecycle

Operational requirements:

* Credentials in deployment secret storage.
* No cookies in structured logs.
* No antiforgery tokens in traces.
* Automatic redaction of request headers.
* Credential rotation procedure.
* Immediate alarm on unexpected login behavior.
* Separate credentials per environment.

### 24. PII retention

Historical HTML may contain student or staff information even when the immediate synchronization scope excludes students.

Therefore:

* Do not store raw HTML by default.
* Store only changed or failed samples.
* Encrypt retained samples.
* Restrict access.
* Define retention.
* Redact phone numbers and email addresses from operational diagnostics.

---

## P2 — Operational usability

### 25. Management dashboard

The operations page should answer:

```text
Is synchronization running?
Is today’s schedule fresh?
Which entity is stale?
Why did it fail?
What was the last successful source request?
What is the oldest queued job?
Which courses disagree with the legacy site?
Can this record be retried safely?
```

Minimum cards:

* Schedule freshness.
* Course freshness.
* Master-data freshness.
* Source availability.
* Authentication state.
* Queue age.
* Parser health.
* Reconciliation mismatches.
* Historical sweep progress.

### 26. Runbooks

Write exact procedures for:

* Legacy login failure.
* Legacy site outage.
* Parser drift.
* One-second SLO breach.
* Queue backlog.
* Duplicate mappings.
* Incorrect course schedule.
* Accidental tombstone.
* Failed database deployment.
* Restoring from backup.
* Final cutover.

Each runbook needs:

```text
Detection
Impact
Immediate containment
Diagnosis
Recovery
Validation
Escalation
```

### 27. Ownership and escalation

Assign owners for:

| Area                      | Owner                  |
| ------------------------- | ---------------------- |
| Legacy credentials        | Operations             |
| HTML parser               | Backend                |
| Course mapping conflicts  | School administrator   |
| Database correctness      | Backend/data           |
| One-second SLO            | Platform               |
| Legacy source outage      | Operations             |
| Historical reconciliation | Data operations        |
| Cutover decision          | Product/business owner |

Without clear ownership, conflict records will accumulate indefinitely.

### 28. Capacity and cost envelope

Estimate and enforce:

* Requests per second to legacy.
* Bytes downloaded per day.
* Queue writes per day.
* Snapshot storage growth.
* Raw failure-page retention.
* Database write amplification.
* Realtime event rate.
* Reconciliation duration.

Set budgets and alerts before production.

---

## Five biggest remaining launch blockers

1. **Prove whether the legacy audit log captures all relevant course and schedule mutations.**
2. **Implement generation-based reconciliation and safe tombstones.**
3. **Build parser schema-drift protection with last-good-state preservation.**
4. **Separate legacy synchronization writes from normal validated scheduling writes.**
5. **Add operational controls: pause, shadow mode, canary, rollback, conflict management, and freshness monitoring.**

After these five areas are complete, the system moves from “fast scraper” to an **operation-grade synchronization platform**.
 # Legacy Course and Schedule Synchronization

## Production-Grade TDD Implementation Plan

## 1. Objective

Build a read-only synchronization platform that continuously mirrors these legacy entities into the new Warwick system:

* Courses.
* Course schedules.
* Teachers.
* Rooms/classrooms.
* Subjects.
* Historical courses and schedules.
* Course management state:

  * Active, draft, and archived status.
  * Course type and hours.
  * Teacher and subject assignment.
  * Classroom assignment.
  * Schedule confirmation.
  * Confirmed-by information.
  * Expiration date where available.

The operational target is:

> A detectable legacy schedule change should normally be committed to the new database and reflected in the frontend within one second.

Correctness must not depend solely on the one-second path. Fast detection provides freshness; periodic reconciliation guarantees convergence.

The legacy system has no supported API or database change feed. Its authoritative data is exposed through authenticated Razor Pages, particularly course detail and schedule pages.

The current repository already has useful foundations:

* Courses expose `legacy_course_id` and `legacy_last_synced_at`.
* Sessions already contain course, teacher, room, start/end time, versioning, soft deletion, and attendance relationships.
* Normal scheduling uses conflict checks, resource locks, teacher membership checks, and transactional series creation.
* The current PostgreSQL queue pattern includes leases, heartbeats, retries, deduplication, `SKIP LOCKED`, and `LISTEN/NOTIFY`.
* The realtime hub supports local channel delivery and PostgreSQL-backed cross-instance fanout.

---

# 2. Non-negotiable design rules

## 2.1 Source ownership

Until final cutover:

```text
Legacy-owned:
courses
teachers
subjects
rooms
legacy schedule rows
legacy confirmation state

New-system-owned:
absence workflows
notification delivery
internal audit metadata
sync health and conflict state
new-system preferences
```

Legacy-owned fields must be read-only through normal application endpoints.

## 2.2 No runtime dependency

Frontend and API requests must never synchronously call the legacy site.

```text
Frontend → New API → Local PostgreSQL
```

Only the synchronization service communicates with the legacy source.

## 2.3 Separate legacy apply path

Legacy schedules must not pass through normal user scheduling commands.

Normal scheduling should continue to enforce:

* Conflict validation.
* Course-teacher membership.
* Room locks.
* Student locks.
* Optimistic concurrency.
* Series invariants.

Legacy synchronization instead:

* Validates source structure.
* Preserves source truth.
* Records conflicts.
* Applies idempotently.
* Never rejects historical data solely because it violates current scheduling rules.

## 2.4 Idempotency

Every source aggregate must have a deterministic canonical hash.

The same source input applied repeatedly must produce:

* No duplicate rows.
* No version increase.
* No repeated audit event.
* No repeated realtime event.
* No unnecessary `updated_at` change.

## 2.5 Fail closed

Unexpected HTML must not produce partial writes.

When parsing fails:

* Preserve last-good data.
* Disable deletion reconciliation.
* Store a sanitized diagnostic artifact.
* Record `parser_drift`.
* Alert operations.

## 2.6 Safe deletion

One missing scrape must never delete data.

Deletion requires multiple successful complete observations plus a grace period.

---

# 3. TDD workflow

Every pull request follows:

## RED

Write failing tests that describe externally observable behavior.

Tests should fail for the intended reason, not because of missing setup or compilation errors.

## GREEN

Implement only enough production code to pass the tests.

Avoid adding speculative abstractions during this step.

## REFACTOR

Improve names, remove duplication, strengthen interfaces, add metrics, and preserve test behavior.

## PR exit gate

A pull request cannot merge unless:

```text
go test ./...
go test -race ./...
go vet ./...
npm run typecheck
npm test
npm run migrate:validate
```

Relevant Playwright suites must also pass for frontend-visible changes. The repository already exposes scripts for type checking, Vitest, Playwright, builds, and migration validation.

---

# 4. Testing architecture

## 4.1 Test layers

### Unit tests

Use for:

* Text normalization.
* Date parsing.
* Canonicalization.
* Hashing.
* Event classification.
* Retry classification.
* Deduplication keys.
* State transitions.

No network and no database.

### Parser contract tests

Use sanitized HTML fixtures captured from each legacy page type.

Each fixture should produce a golden normalized result.

### HTTP integration tests

Use `httptest.Server` to simulate:

* Login.
* Cookies.
* Antiforgery tokens.
* Session expiration.
* Redirects.
* Slow responses.
* 429 and 5xx responses.
* Malformed HTML.
* Changed HTML structure.

### PostgreSQL integration tests

Use a real ephemeral PostgreSQL database.

Verify:

* Transactions.
* Constraints.
* Advisory locks.
* Queue claims.
* Concurrent workers.
* Rollbacks.
* Idempotency.
* Tombstone generations.
* Outbox writes.

### End-to-end tests

Use:

* Fake legacy server.
* Legacy sync process.
* PostgreSQL.
* Backend API.
* Realtime connection.
* React application.

Verify a legacy source change becomes visible in the UI.

### Performance tests

Measure:

* Detection latency.
* Fetch latency.
* Apply latency.
* Realtime latency.
* End-to-end freshness.
* Queue backlog.
* Source request rate.

### Chaos tests

Inject:

* Process termination.
* Database disconnect.
* Expired source session.
* Duplicate source event.
* Reordered source event.
* Legacy outage.
* Parser drift.
* Retry exhaustion.

---

# 5. Planned repository structure

```text
backend/
├── cmd/
│   └── legacy-sync/
│       └── main.go
│
├── internal/
│   ├── legacysync/
│   │   ├── config.go
│   │   ├── service.go
│   │   ├── errors.go
│   │   ├── models.go
│   │   ├── metrics.go
│   │   │
│   │   ├── client/
│   │   │   ├── client.go
│   │   │   ├── auth.go
│   │   │   ├── allowlist.go
│   │   │   ├── ratelimit.go
│   │   │   └── circuitbreaker.go
│   │   │
│   │   ├── parser/
│   │   │   ├── contract.go
│   │   │   ├── courses.go
│   │   │   ├── course_detail.go
│   │   │   ├── schedule.go
│   │   │   ├── logs.go
│   │   │   ├── teachers.go
│   │   │   ├── rooms.go
│   │   │   └── subjects.go
│   │   │
│   │   ├── normalize/
│   │   │   ├── text.go
│   │   │   ├── datetime.go
│   │   │   ├── canonical.go
│   │   │   └── hash.go
│   │   │
│   │   ├── detector/
│   │   │   ├── logs.go
│   │   │   ├── schedule.go
│   │   │   └── leader.go
│   │   │
│   │   ├── apply/
│   │   │   ├── masterdata.go
│   │   │   ├── course.go
│   │   │   └── schedule.go
│   │   │
│   │   ├── reconcile/
│   │   │   ├── generation.go
│   │   │   ├── compare.go
│   │   │   └── tombstone.go
│   │   │
│   │   └── fixtures/
│   │       ├── login/
│   │       ├── courses/
│   │       ├── course_detail/
│   │       ├── schedule/
│   │       └── masterdata/
│   │
│   ├── jobqueue/
│   └── httpapi/
│       └── legacysynchttp/
│
├── db/
│   ├── migrations/
│   └── queries/
│       └── legacy_sync.sql
│
src/
└── features/
    └── legacySync/
        ├── api.ts
        ├── types.ts
        ├── SyncStatusBadge.tsx
        ├── SyncHealthPage.tsx
        ├── SyncConflictPage.tsx
        └── useLegacyRealtime.ts
```

---

# 6. PR-by-PR TDD implementation plan

## PR 1 — Source contract discovery harness

### Purpose

Prove what changes are observable from the legacy site before building synchronization logic around assumptions.

### RED tests

Create a test specification describing every required legacy action:

```text
create course
edit course
archive course
create schedule
change schedule time
delete schedule
change teacher
change subject
change room
confirm schedule
edit teacher
edit room
edit subject
```

Create:

```text
backend/internal/legacysync/sourcecontract/contract_test.go
```

Tests should initially fail because no observations have been recorded.

### GREEN implementation

Create a read-only diagnostic command:

```text
backend/cmd/legacy-contract-check/main.go
```

It should:

* Authenticate.
* Capture `/Admin/Logs`.
* Capture today’s schedule.
* Capture a selected course detail.
* Compute before/after normalized differences.
* Record which log entry and page changed.

It must never send mutating requests.

### Exit criteria

Produce a checked-in, sanitized contract document:

```text
docs/legacy-source-contract.md
```

It must state:

* Observable action types.
* Log format.
* Timestamp precision.
* Log retention behavior.
* Whether stable identifiers exist.
* Which page is authoritative for each field.
* Which mutations cannot be detected through logs.

No one-second detector work begins until this contract exists.

---

## PR 2 — Core synchronization schema

### Purpose

Create stable source identity, snapshots, runs, conflicts, and safe state tracking.

### RED tests

Create migration integration tests:

```text
backend/internal/legacysync/storage/migration_integration_test.go
```

Tests:

```text
TestExternalRefs_RejectDuplicateExternalIdentity
TestExternalRefs_AllowSameNumericIDAcrossEntityTypes
TestEntitySnapshot_RequiresParserVersion
TestEntitySnapshot_SourceHashIsRequired
TestSyncRun_TracksGenerationLifecycle
TestConflict_PreservesSourcePayload
TestTombstoneState_RejectsInvalidTransition
TestSessions_EnforceUniqueLegacyScheduleID
TestCourseLegacyFields_DefaultSafely
```

### GREEN migration

Add the next Goose migration containing:

```text
external_refs
legacy_entity_snapshots
legacy_sync_runs
legacy_sync_conflicts
legacy_sync_dead_letters
legacy_sync_outbox
```

Extend:

```text
courses
sessions
session_series
users or teacher profile model
```

Recommended course additions:

```text
legacy_status
legacy_expire_date
legacy_archived
legacy_source_hash
legacy_last_seen_at
source_kind
```

Recommended session additions:

```text
legacy_schedule_id
legacy_confirmed
legacy_confirmed_by
legacy_source_hash
legacy_last_synced_at
legacy_last_seen_at
source_kind
```

Recommended series additions:

```text
source_kind
materialization_mode
legacy_group_key
```

### REFACTOR

Add CHECK constraints for:

```text
source_kind: native | legacy | hybrid
materialization_mode: generated | external
sync state: active | suspected_missing | confirmed_missing | tombstoned
```

Add indexes required by targeted lookup and reconciliation.

### Exit criteria

Migration up/down tests pass against a fresh database and a representative existing database snapshot.

---

## PR 3 — Canonical models and normalization

### Purpose

Ensure source-equivalent data always produces the same normalized representation and hash.

### RED tests

Create table-driven and property tests:

```text
backend/internal/legacysync/normalize/text_test.go
backend/internal/legacysync/normalize/datetime_test.go
backend/internal/legacysync/normalize/canonical_test.go
backend/internal/legacysync/normalize/hash_test.go
```

Required cases:

```text
normal and non-breaking spaces
multiple whitespace
Thai and English text
leading zero legacy IDs
empty and [NOT SET] values
DD/MM/YY dates
DD/MM/YYYY dates
local Asia/Bangkok times
sessions crossing midnight
stable ordering of schedule rows
same semantic data in different HTML order
changed confirmation value changes hash
changed room changes hash
```

Add fuzz tests:

```go
func FuzzNormalizeText(f *testing.F)
func FuzzParseLegacyDate(f *testing.F)
func FuzzCanonicalCourseHash(f *testing.F)
```

### GREEN implementation

Define immutable normalized source models:

```go
type LegacyCourse struct
type LegacySchedule struct
type LegacyTeacher struct
type LegacyRoom struct
type LegacySubject struct
type LegacyCourseAggregate struct
```

Do not expose HTML parsing structures to apply services.

### Exit criteria

Equivalent inputs always produce identical canonical JSON and SHA-256 hashes.

---

## PR 4 — HTML parser contracts

### Purpose

Parse every priority page with strict structural validation.

### RED tests

Create sanitized fixtures for:

* Course list.
* Course detail.
* Today’s schedule.
* Audit log.
* Teachers.
* Rooms.
* Subjects.
* Archived course list.
* Empty schedule.
* Unassigned room.
* Confirmed schedule.
* Unconfirmed schedule.
* Inactive teacher.
* Missing optional email.
* Malformed page.
* Login page returned instead of data.

Example tests:

```text
TestCourseDetailParser_ParsesAllScheduleRows
TestCourseDetailParser_PreservesLegacyScheduleID
TestCourseDetailParser_ParsesConfirmationState
TestCourseDetailParser_ParsesUnassignedRoom
TestCourseDetailParser_RejectsMissingScheduleHeaders
TestCourseDetailParser_RejectsLoginPage
TestScheduleParser_IsOrderIndependent
TestTeacherParser_PreservesLeadingZeroID
TestSubjectParser_AllowsDuplicateNamesWithDifferentIDs
TestRoomParser_RejectsDuplicateLegacyID
```

Golden files:

```text
testdata/<fixture>.golden.json
```

### GREEN implementation

Use `golang.org/x/net/html`.

Do not use:

* Regular expressions as the main HTML parser.
* Browser automation.
* CSS selectors tied to visual layout when stable semantic labels exist.
* Silent fallback column positions.

Each parser must validate:

```go
type PageContract struct {
    PageType        string
    ParserVersion   int
    RequiredHeaders []string
    RequiredFields  []string
}
```

### REFACTOR

Separate:

```text
DOM extraction
source-field parsing
normalization
domain validation
```

### Exit criteria

Every known fixture passes, malformed fixtures fail closed, and parser fuzz tests do not panic.

---

## PR 5 — Read-only authenticated legacy client

### Purpose

Create a safe HTTP boundary that physically prevents accidental production writes.

### RED tests

Using `httptest.Server`:

```text
TestClient_LoginCapturesCookiesAndAntiforgeryToken
TestClient_ReauthenticatesOnceAfterSessionExpiry
TestClient_ConcurrentExpiryTriggersSingleRelogin
TestClient_RejectsNonAllowlistedRoute
TestClient_RejectsMutatingHandler
TestClient_DoesNotRetryAuthenticationForever
TestClient_Classifies429AsRateLimited
TestClient_Classifies5xxAsSourceUnavailable
TestClient_UsesRequestDeadline
TestClient_RedactsCookiesFromErrors
TestClient_ClosesResponseBody
TestClient_CancelsRequestWithContext
```

Add race tests for concurrent reauthentication.

### GREEN implementation

Components:

```text
SessionManager
AllowlistTransport
RateLimiter
CircuitBreaker
LegacyClient
```

Explicitly allow only required read operations.

Any unrecognized method, route, or handler must return a local safety error without sending the request.

### Exit criteria

The client cannot perform course edits, schedule confirmation, deletion, import, classroom assignment, or check-in mutation.

---

## PR 6 — Generic durable queue extraction

### Purpose

Reuse the repository’s proven queue pattern without coupling legacy synchronization to CRM domain names.

### RED tests

First lock the existing queue behavior with compatibility tests:

```text
TestQueue_ClaimsOldestEligibleJob
TestQueue_UsesSkipLocked
TestQueue_RecoversExpiredLease
TestQueue_HeartbeatsRunningJob
TestQueue_DeduplicatesActiveUniqueKey
TestQueue_AllowsSameKeyAfterTerminalJob
TestQueue_NotifyWakesWorkerImmediately
TestQueue_RetriesRetryableFailure
TestQueue_DeadLettersNonRetryableFailure
TestQueue_ShutdownStopsClaiming
```

### GREEN implementation

Extract a generic package:

```text
backend/internal/jobqueue
```

Keep a compatibility adapter in:

```text
backend/internal/crmimport/queue
```

Add a legacy-specific store using:

```text
legacy_sync_jobs
legacy_sync_jobs_notify
```

Required fields:

```text
priority
entity_type
external_id
payload
unique_key
deadline_at
attempt
max_attempts
locked_by
locked_until
heartbeat_at
run_after
last_error
```

### REFACTOR

No domain-specific job type enum belongs in the generic package.

### Exit criteria

All existing CRM queue tests remain green and legacy queue tests pass under concurrent workers.

---

## PR 7 — Master-data synchronization

### Purpose

Synchronize teachers, rooms, and subjects before course aggregates.

### RED tests

PostgreSQL integration tests:

```text
TestMasterDataApply_CreatesTeacherMapping
TestMasterDataApply_CreatesRoomMapping
TestMasterDataApply_CreatesSubjectMapping
TestMasterDataApply_IsIdempotent
TestMasterDataApply_UpdatesRenamedRoom
TestMasterDataApply_DoesNotMatchByNameOnly
TestMasterDataApply_PreservesInactiveTeacher
TestMasterDataApply_DisablesLoginForImportedTeacher
TestMasterDataApply_RollsBackSnapshotOnFailure
TestMasterDataApply_WritesAuditAndOutboxAtomically
TestMasterDataApply_ConcurrentSameEntitySerializes
```

### GREEN implementation

Create:

```text
apply.MasterDataService
storage.ExternalRefStore
storage.SnapshotStore
```

Apply each entity under a per-external-ID advisory lock.

Imported teachers must not receive fake working credentials.

### Exit criteria

A full repeated master-data import produces zero domain changes on the second run.

---

## PR 8 — Course aggregate synchronization

### Purpose

Synchronize course management fields and references transactionally.

### RED tests

```text
TestCourseApply_CreatesCourseWithLegacyMapping
TestCourseApply_ResolvesTeacherAndSubjectMappings
TestCourseApply_PendsWhenTeacherMappingMissing
TestCourseApply_PendsWhenSubjectMappingMissing
TestCourseApply_UpdatesCourseWithoutDuplicate
TestCourseApply_PreservesNativeOwnedFields
TestCourseApply_UpdatesLegacyLastSyncedAt
TestCourseApply_UnchangedHashIsNoOp
TestCourseApply_RollsBackOnSnapshotFailure
TestCourseApply_WritesSingleOutboxEvent
TestCourseApply_ConcurrentRefreshesDoNotInterleave
TestCourseApply_DoesNotUseNormalHTTPHandler
```

### GREEN implementation

Create:

```text
backend/internal/legacysync/apply/course.go
```

The service owns a complete transaction:

```text
lock
resolve references
load mapping
compare hash
upsert course
store snapshot
write audit
write outbox
commit
```

Do not place synchronization SQL inside HTTP handlers.

The existing course domain already emphasizes atomic teacher-set updates, versioning, and rollback behavior; the synchronization path should maintain equivalent transaction discipline.

### Exit criteria

A course is never visible with partially updated teacher, subject, or source metadata.

---

## PR 9 — External schedule synchronization

### Purpose

Import concrete legacy schedule rows without guessing recurrence rules.

### RED tests

```text
TestScheduleApply_CreatesExternalSeriesContainer
TestScheduleApply_CreatesConcreteSession
TestScheduleApply_PreservesLegacyScheduleID
TestScheduleApply_UpdatesTimeByLegacyID
TestScheduleApply_UpdatesRoom
TestScheduleApply_UpdatesTeacher
TestScheduleApply_UpdatesConfirmation
TestScheduleApply_AllowsMissingRoom
TestScheduleApply_RecordsHistoricalConflict
TestScheduleApply_DoesNotRejectExistingTeacherConflict
TestScheduleApply_DoesNotGenerateExtraOccurrences
TestScheduleApply_UnchangedHashIsNoOp
TestScheduleApply_ConcurrentSameCourseIsAtomic
TestScheduleApply_RollsBackEntireCourseAggregate
```

### GREEN implementation

Use:

```text
source_kind = legacy
materialization_mode = external
```

External series must not be passed to normal series materialization.

Add guards to normal scheduling operations:

```text
external series cannot:
edit entire generated series
rematerialize
split using native recurrence assumptions
silently change source-owned rows
```

### Historical correction tests

```text
TestHistoricalScheduleCorrection_PreservesPreviousSnapshot
TestHistoricalScheduleCorrection_WritesAuditEvent
TestHistoricalScheduleCorrection_TriggersImpactAnalysis
TestHistoricalScheduleCorrection_DoesNotDeleteDependentAbsenceData
```

### Exit criteria

Every legacy schedule ID maps to exactly one local session, including historical rows.

---

## PR 10 — Fast change detector

### Purpose

Meet the one-second target through targeted refreshes.

### RED tests

```text
TestLogDetector_RecognizesCourseChange
TestLogDetector_RecognizesScheduleChange
TestLogDetector_RecognizesRoomChange
TestLogDetector_RecognizesTeacherChange
TestLogDetector_RecognizesSubjectChange
TestLogDetector_UnknownActionSchedulesBoundedReconcile
TestLogDetector_DeduplicatesOverlappingWindows
TestLogDetector_PreservesDuplicateMultiplicity
TestLogDetector_CoalescesSameCourseWithinWindow
TestLogDetector_DoesNotLoseSameSecondEvents
TestDetector_OnlyLeaderPollsSource
TestDetector_LeadershipTransfersAfterFailure
```

Today-schedule detector tests:

```text
TestScheduleDetector_UnchangedHashEnqueuesNothing
TestScheduleDetector_ChangedRowRefreshesAffectedCourse
TestScheduleDetector_RemovedRowRefreshesAffectedCourse
TestScheduleDetector_ReorderedRowsAreUnchanged
TestScheduleDetector_ParserFailureDoesNotEmitDeletion
```

### GREEN implementation

Run:

```text
audit log detector: configurable 250–500 ms
today schedule detector: every 1 second
```

Unique keys:

```text
legacy:course:<legacy-id>
legacy:teacher:<legacy-id>
legacy:room:<legacy-id>
legacy:subject:<legacy-id>
legacy:schedule-day:<date>
```

Use PostgreSQL notification to wake workers immediately.

### Exit criteria

Under a controlled fake source:

```text
p95 detection-to-queue ≤ 250 ms
no duplicate active course jobs
no source event loss across overlapping polling windows
```

---

## PR 11 — Reconciliation generations and tombstones

### Purpose

Guarantee eventual correctness when fast detection misses an event.

### RED tests

```text
TestGeneration_RecordsCompleteSuccessfulCoverage
TestGeneration_FailureDoesNotAdvanceCheckpoint
TestGeneration_MissingOnceMarksSuspected
TestGeneration_MissingTwiceMarksConfirmed
TestGeneration_GracePeriodRequiredBeforeTombstone
TestGeneration_ParserFailureDisablesTombstone
TestGeneration_AuthFailureDisablesTombstone
TestGeneration_ArchivedFilterMustBeCovered
TestGeneration_ReappearingEntityRestoresActiveState
TestGeneration_TombstoneIsIdempotent
TestGeneration_NeverHardDeletesAutomatically
```

Comparison tests:

```text
TestReconcile_DetectsMissingCourse
TestReconcile_DetectsUnexpectedSession
TestReconcile_DetectsTeacherMismatch
TestReconcile_DetectsRoomMismatch
TestReconcile_DetectsConfirmationMismatch
TestReconcile_ZeroMismatchAfterRepair
```

### GREEN implementation

Schedules:

```text
active courses: every 5 minutes
teachers/rooms/subjects: every 5 minutes
archived courses: nightly
full historical verification: nightly
```

### Exit criteria

A deliberately corrupted local database converges to source state without deleting valid records after a partial scrape.

---

## PR 12 — Realtime outbox and frontend invalidation

### Purpose

Deliver committed source changes to the frontend without race conditions.

### RED tests

Backend:

```text
TestOutbox_WrittenInSameTransactionAsCourse
TestOutbox_NotWrittenWhenTransactionRollsBack
TestOutbox_PublishesAfterCommit
TestOutbox_RetryDoesNotDuplicateEvent
TestOutbox_MultipleInstancesPublishOnce
```

Frontend:

```text
SyncStatusBadge shows healthy freshness
SyncStatusBadge shows stale state
course event invalidates course detail
schedule event invalidates date schedule
room event invalidates room schedule
disconnected realtime falls back to query refresh
```

End-to-end:

```text
legacy fixture changes
detector emits targeted job
database updates
realtime event arrives
UI shows changed schedule
```

### GREEN implementation

Publish compact invalidation events:

```json
{
  "type": "legacy.schedule.updated",
  "channel": "schedule:2026-08-03",
  "id": "112741",
  "payload": {
    "legacy_course_id": "7306",
    "synced_at": "2026-08-03T03:47:00+07:00"
  }
}
```

Do not send full domain objects through realtime.

### Exit criteria

No realtime event is visible for a rolled-back transaction.

---

## PR 13 — Operations API and management UI

### Purpose

Make synchronization diagnosable and controllable by operations staff.

### RED tests

API authorization:

```text
non-admin cannot view sync health
non-admin cannot trigger refresh
admin can view runs
admin can inspect conflicts
admin can enqueue targeted refresh
manual refresh returns immediately
manual refresh is rate limited
```

UI tests:

```text
overview displays current freshness
overview displays source outage
overview displays authentication failure
conflict page groups by error category
course page shows legacy source ownership
course page shows last synchronized time
manual retry requires confirmation
pause control shows current state
```

### GREEN endpoints

```text
GET  /api/v1/admin/legacy-sync/health
GET  /api/v1/admin/legacy-sync/runs
GET  /api/v1/admin/legacy-sync/conflicts
GET  /api/v1/admin/legacy-sync/jobs
POST /api/v1/admin/legacy-sync/refresh
POST /api/v1/admin/legacy-sync/pause
POST /api/v1/admin/legacy-sync/resume
POST /api/v1/admin/legacy-sync/conflicts/{id}/retry
```

Operational switches:

```text
detection enabled
fetch enabled
apply enabled
tombstone enabled
realtime enabled
shadow mode
```

### Exit criteria

Operations can diagnose stale data without database access.

---

## PR 14 — Resilience, security, and production hardening

### Purpose

Prove safe behavior under realistic failures.

### Authentication chaos tests

```text
session expires during fetch
session expires for ten concurrent workers
wrong credentials
login HTML changes
source redirects unexpectedly
```

### Source chaos tests

```text
429 throttling
500 burst
connection reset
slow response
partial HTML
large response
unexpected content type
```

### Database chaos tests

```text
database unavailable before claim
database unavailable during apply
commit succeeds but worker dies before job completion
worker dies while holding lease
outbox publisher dies after reading event
```

### Security tests

```text
cookies never appear in logs
antiforgery token never appears in logs
PII is redacted from diagnostic pages
mutating source route is always blocked
manual refresh endpoint requires admin
credentials are absent from configuration output
raw fixture retention respects policy
```

### GREEN implementation

Add:

* Typed retry classification.
* Circuit breaker.
* Bounded exponential backoff with jitter.
* Dead-letter state.
* Source response-size limits.
* Diagnostic redaction.
* Config validation.
* Graceful shutdown.
* Health and readiness checks.
* Leader-election health.
* Queue-depth protection.
* Historical-work suspension during P0 backlog.

### Exit criteria

The new application remains available and serves last-good data throughout a complete legacy-source outage.

---

# 7. One-second performance test plan

## 7.1 Performance scenario

Use a fake legacy source configured with representative latency:

```text
audit log response: 50 ms
course detail response: 250 ms
database apply: real PostgreSQL
realtime: actual hub and fanout
```

Generate:

```text
1,000 source mutations
20 active courses
5 concurrent mutation bursts
duplicate and reordered log rows
```

## 7.2 Required measurements

```text
source_write_to_detection
detection_to_enqueue
enqueue_to_claim
claim_to_fetch_complete
fetch_to_commit
commit_to_realtime
realtime_to_ui_refresh
total_source_to_ui
```

## 7.3 Pass criteria

```text
p50 total ≤ 600 ms
p95 total ≤ 1 second
p99 total ≤ 3 seconds
zero lost events
zero duplicate domain rows
bounded queue depth
bounded source request rate
```

## 7.4 Degraded-source scenario

Repeat with course-detail latency of 800 ms.

Expected behavior:

* p95 one-second SLO may be missed.
* Source-latency metric identifies the cause.
* No job duplication.
* No unbounded concurrency increase.
* No impact on normal API latency.
* Freshness warning becomes visible.

---

# 8. CI pipeline gates

## Required on every PR

```text
format check
go vet
unit tests
parser fixture tests
frontend typecheck
frontend unit tests
migration validation
```

## Required for backend/domain PRs

```text
PostgreSQL integration tests
go test -race
transaction rollback tests
concurrency tests
```

## Required for parser changes

```text
all golden fixtures
fuzz smoke run
schema-drift fixtures
login-page rejection
```

## Required before staging deployment

```text
full integration suite
Playwright sync management suite
fake-source end-to-end suite
performance smoke test
security redaction tests
```

## Required before production enablement

```text
shadow-mode comparison
source load validation
one-second SLO report
nightly reconciliation report
backup and restore drill
rollback drill
operations runbook review
```

---

# 9. Coding standards

## Go

* Pass `context.Context` through every network and database operation.
* Apply explicit request and transaction deadlines.
* Wrap errors with operation and entity identity.
* Use typed errors for retry decisions.
* Avoid package-level mutable state.
* Avoid goroutines without owned cancellation and `WaitGroup`.
* No `time.Sleep` in deterministic unit tests.
* Use injected clocks only where time controls behavior.
* Keep interfaces at I/O boundaries, not around every struct.
* Do not log complete source payloads.
* Use `errors.Is` and `errors.As`.
* Always close HTTP response bodies.
* Prefer table-driven tests.
* Run critical concurrency suites under `-race`.

## SQL

* Use transactional aggregate writes.
* Use unique constraints as the final idempotency boundary.
* Use advisory locks for external aggregate serialization.
* Use `FOR UPDATE SKIP LOCKED` for queues.
* Avoid update-on-unchanged operations.
* Add indexes based on actual query patterns.
* Use soft deletion for synchronized entities.
* Preserve prior source snapshots for historical corrections.
* Add constraints after backfill and validation where historical quality is uncertain.

## React

* Use React Query as the server-state owner.
* Realtime events invalidate queries; they do not replace API responses.
* Show explicit freshness and stale state.
* Disable legacy-owned controls.
* Ensure management screens are keyboard accessible.
* Do not expose source credentials or raw HTML.
* Require confirmation for operational mutations such as pause, retry, or reconcile.

---

# 10. Rollout plan

## Stage 1 — Parser-only development

* Build fixtures.
* Run unit and parser tests.
* No production source access.

## Stage 2 — Read-only source contract

* Authenticate with dedicated source account.
* Measure source behavior.
* No local domain writes.

## Stage 3 — Shadow mode

```text
fetch = on
parse = on
snapshot = on
domain apply = off
realtime = off
tombstone = off
```

Compare proposed state to production manually and automatically.

## Stage 4 — Master-data canary

Enable:

* One teacher range.
* Selected rooms.
* Selected subjects.

## Stage 5 — Course canary

Enable five low-risk courses.

Verify:

* Stable mappings.
* Correct schedule rows.
* No duplicate entities.
* Source request load.
* Repeated no-op behavior.

## Stage 6 — Active course synchronization

Enable all active courses while historical crawling remains paused.

## Stage 7 — One-second detector

Enable log and today-schedule detection.

Measure the SLO without enabling deletion.

## Stage 8 — Historical backfill

Run bounded background workers.

Automatically pause historical work when the realtime queue has operational backlog.

## Stage 9 — Reconciliation

Enable active and archived generation comparison.

## Stage 10 — Tombstones

Enable only after:

* Multiple successful reconciliation cycles.
* Zero unexplained mismatches.
* Restore behavior tested.
* Operations approval.

---

# 11. Definition of done

The synchronization platform is production-ready when:

1. Every course, schedule, teacher, room, and subject has a stable legacy mapping.
2. Every historical schedule has a preserved source identity.
3. Repeated synchronization is idempotent.
4. Course aggregates update atomically.
5. Legacy schedules are not regenerated from guessed recurrence rules.
6. Normal scheduling invariants remain unchanged for native data.
7. Unexpected HTML cannot overwrite last-good state.
8. One missing source observation cannot delete data.
9. Authentication expiration does not create a login storm.
10. Queue jobs survive worker crashes.
11. Concurrent refreshes cannot interleave the same course aggregate.
12. Realtime events occur only after commit.
13. The frontend never calls the legacy source.
14. A legacy outage does not degrade normal API response time.
15. Operations can pause, inspect, retry, reconcile, and diagnose the pipeline.
16. Credentials, cookies, tokens, and PII do not appear in logs.
17. Shadow mode and rollback can be activated without redeployment.
18. p95 source-to-frontend freshness is at or below one second under the agreed source-latency envelope.
19. Nightly reconciliation reports zero unexplained differences.
20. Backup restoration preserves mappings and does not create duplicate domain records.

---

# 12. Recommended merge sequence

```text
PR 1   Source contract harness
PR 2   Synchronization schema
PR 3   Canonical models and normalization
PR 4   Parser contracts and fixtures
PR 5   Safe authenticated HTTP client
PR 6   Generic durable queue
PR 7   Teacher, room, subject synchronization
PR 8   Course aggregate synchronization
PR 9   External schedule synchronization
PR 10  Fast change detector
PR 11  Reconciliation and tombstones
PR 12  Realtime outbox and frontend invalidation
PR 13  Operations API and management UI
PR 14  Resilience, security, load, and rollout hardening
```

Do not combine the schema, parser, queue, course apply, schedule apply, and fast detector into one pull request. Smaller stages allow each invariant to be proven before the next layer depends on it.
 /main-agent-sidekick-protocol