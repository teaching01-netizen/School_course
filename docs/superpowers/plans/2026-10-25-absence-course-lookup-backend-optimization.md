# Absence course lookup: low-level backend optimization plan

Date: 2026-10-25. Mode: plan only. Inspected working tree: branch `deepswe-absence-consistency`, HEAD `6046f25`.

**Decision:** first remove enrollment × active-course multiplication in the shared student-subject query and remove the second JSON encoding in the V2 session response. Then measure whether moving the staff subject filter to native UUID comparison and decoding resolver policies outside the course loop justify their changes. **Do not implement caching.**

No application code or migrations were changed while preparing this plan. Existing frontend changes in the working tree are outside this plan.

## 1. Change boundary and repository evidence

### Current behavior → intended behavior

Students and staff select a student, load eligible subjects/courses and sessions, choose missed sessions and make-up options, then submit an absence. Keep that behavior and its contracts; reduce database intermediate rows, repeated CPU work and response allocations.

The inspected backend is Go with pgx v5 and SQL queries. Local Compose specifies PostgreSQL 16. The production PostgreSQL version and actual deployment flags are **Unknown**.

### Verified request paths

| Consumer | Lookup path and owner | Compatibility boundary |
| --- | --- | --- |
| Student public form | `AbsenceForm.tsx` → `lookupStudentByWcode()` → `POST /api/v1/absence-self-service/lookup` → `handleStudentLookup()` | Pre-verification lookup returns recognition/token information; it does **not** load courses. Preserve rate limits, token creation, idempotency and restricted personal data. |
| Verified student | `loadStudentProfile()` → `GET /api/v1/absence-self-service/me` → `handleStudentProfile()` → `StudentSubjectByWCodeForAbsenceForm()` | Identity comes from the verified student session. Subjects must satisfy the existing form-availability predicate. |
| Verified student sessions | `loadStudentSessions()` → `GET /api/v1/absence-self-service/sessions` → `handleStudentSessions()` → `handleSessionsInRangeForWCode()` | Session identity, timing, active-course visibility and forbidden parameter handling stay authoritative on the server. |
| Staff using the public form | `lookupStaffStudentByWcode(..., {studentView: true})` and `loadSessionsInRange(..., {studentView: true})` | Staff authorization with the student projection. It must continue matching student-visible choices. |
| Staff creation modal | `StaffCreateAbsenceModal.tsx:1235`, `lookupStudent()` → staff student lookup without `student_view` → `StudentSubjectByWCode()` | Broad enrolled-subject lookup remains available to authorized staff. The modal also loads `/api/v1/subjects`. |
| Staff modal sessions | `loadStaffSessionsForSubjects()` → `loadStaffLifetimeSessions()` → `/api/v1/absences/sessions-in-range` | Explicit 1970–2100 bounds, `lifetime=true`, `bypass_timing=true`; special subjects also use `include_all_subjects=true` and `subject_ids`. Preserve existing authorization. |
| Submission boundary | Student `/api/v1/absences/batch`; staff public form `/api/v1/absences/staff-form-batch`; staff modal `/api/v1/absences/staff-create` with `X-Staff-Batch: true` | These write paths revalidate current state. Lookup optimization must not replace their validation, transactions, locks, snapshots, audit or idempotency. |

Frontend/API evidence: `src/features/absences/api/absenceFormApi.ts:153`, `:175`, `:204`; `src/pages/AbsenceForm.tsx:352`, `:597`; `src/components/absences/StaffCreateAbsenceModal.tsx:101`, `:587`, `:1235`, `:1500`.

### Verified backend findings

| Finding | Inspected location | Consequence and evidence status |
| --- | --- | --- |
| The shared subject query joins each enrollment to every configured active course of its subject, then uses aggregates and `DISTINCT`. | `backend/internal/db/absence_custom.go:53`, `studentSubjectByWCode()` | **Verified structure. Inferred performance cost:** for one subject with E qualifying enrollments and A active courses, the joined aggregate input can contain E × max(A, 1) rows. Actual cost requires an execution plan. |
| Session V2 already batches reads and defaults on. | `backend/internal/httpapi/absenceshttp/sessions_range_service.go:83`, `:153`; `backend/internal/db/sessions_range_head_batch.go:66` | **Verified.** Reuse this pipeline; do not propose a second batch service or claim the active V2 path has the legacy per-course query pattern. Constant round trips do not imply constant CPU or row volume. |
| Both V2 response branches marshal the full payload for byte telemetry, then call `WriteJSON()`, which encodes it again. | `sessions_range_service.go:258`, `:438`; `backend/internal/httpapi/httpadapter/adapter.go:61` | **Verified redundant encoding.** One full encoding can be removed without changing successful response data. |
| Staff all-subject facts compare `sub.id::text` against a split string list. | `backend/internal/db/sessions_range_facts.go:198`; shared by `sessions_range_head_batch.go` | **Verified expression. Inferred opportunity:** compare UUIDs without casting each subject ID to text. Index selection and benefit are unmeasured. |
| V2 resolver helpers decode the same policy bytes inside course resolution. | `sessions_range_resolve.go:40`; `resolver.go:1205`, `:1243`, `:1282`; `sessions_range_mapped.go:75`; `sat_verbal_policy.go:783` | **Verified repeated work.** Consider decoding once for the V2 resolver loop after profiling. Existing `automaticSitInEnabledForPolicies()` already accepts the typed policy. |
| Relevant indexes already exist in migrations. | `00071_schedule_stabilization_indexes.sql`, `00115_sessions_in_range_lookup_indexes.sql`, `00066_wcode_case_insensitive_uniqueness.sql`, `00122_wcode_lookup_indexes.sql`, `00100_admin_pagination_indexes.sql` | **Verified definitions**, including active sessions by course/start, enrollment by student/status/course, lowercase W-code and courses by subject/code. Production installation/use is **Unknown**. Do not add duplicate indexes. |
| Continuation enrollment and effective course rules are database views. | `backend/db/migrations/00128_course_continuation_links.sql:21`, `:43` | **Verified.** Preserve `course_rule_configs` and `effective_course_students` semantics on paths already using them. |

The subject query is also used for parent verification and contact/notification reads in `pending_routes.go`, `routes.go`, `batch_routes.go`, `staff_create.go` and `staff_create_tx.go`. Its complete `StudentSubjectRow` projection and empty-result behavior must remain compatible, even though the optimization is motivated by the form.

### Scope

Include the shared subject query and V2 session lookup internals. Exclude frontend redesign, reducing the staff lifetime range, changing subject selection behavior, rewriting absence creation, changing enrollment/continuation policy, legacy-site scraping, new endpoints, schema changes, pool-size increases, queues and new dependencies.

No Redis, process/global maps, TTLs, materialized lookup copies, response caches, memoization or new prepared-statement cache configuration. Decoded arguments live only within the request that fetched their source row.

## 2. Requirements and invariants

| ID | Acceptance criterion | Owner |
| --- | --- | --- |
| FR-1 | Student profile and staff lookup return the same ordered subjects, active-course IDs, teacher labels, merge metadata and permitted identity fields as before. | `studentSubjectByWCode()` and existing response handlers |
| FR-2 | Student sessions, staff student-view, staff enrolled/lifetime and staff special-subject lookups retain their current response envelope, values and ordering. | Existing request parser, V2 service and assembly |
| FR-3 | Subject metadata aggregation occurs once per eligible subject rather than once per qualifying enrollment; query count remains one for that DB method. | Shared subject SQL |
| FR-4 | Each successful V2 session response performs one complete JSON encoding. `payload_bytes` retains its current meaning: JSON bytes excluding the trailing newline. | V2 response writer |
| FR-5 | Performance changes show reduced work and measured improvement on their target fixture, with no material regression in other modes. No unmeasured speedup claim. | Verification in section 8 |
| INV-1 | Students cannot choose another identity or obtain staff-only flags/hidden course access. Staff student-view cannot inherit operational overrides. | `requireStudentSession()`, `handleStudentSessions()`, prelim/final request parsing |
| INV-2 | A qualifying enrollment determines subject membership; active-course metadata still comes from **all configured active courses for that subject**, including active classes the student is not enrolled in. | Shared subject SQL |
| INV-3 | `active_course_id` remains `MIN(sac.course_id::text)::uuid`; teacher labels and merge annotations retain existing NULL/empty-string behavior. | Existing aggregate expressions, copied unchanged |
| INV-4 | Lookup data is advisory for submission. A concurrent change must be rechecked by the existing write transaction; no lookup result grants write authorization. | Existing submission handlers and DB constraints |
| INV-5 | Institute timezone, half-open windows, full-history absence-day counts, continuation rules, SAT occurrence matching, priority order, final-day exclusions and conflicts remain unchanged. | Existing facts queries, bundle, rules and write validators |
| ERR-1 | Preserve missing/invalid identity errors, unauthorized/forbidden outcomes, zero-subject versus unknown-student distinction and existing parameter-validation order. | Existing HTTP handlers |
| ERR-2 | Preserve request-fatal DB failures versus per-course sit-in degradation. A policy decode failure must not become a new endpoint-wide failure. | Existing V2 service and resolver branches |

For the optimized read operation: establish the same trusted actor; run the same authorization and feature gates; return the same choices or errors; introduce no domain writes. Existing authentication/token lifecycle side effects remain owned by their current code.

## 3. Entities, ownership and relationships

| Existing owner/state | Relationship and responsibility | Planned treatment |
| --- | --- | --- |
| `students`, `course_students`, `courses`, `subjects` | Student → qualifying enrollments → distinct subjects | Deduplicate subject membership before joining independent active-course metadata. Keep the current direct-enrollment source in this query. |
| `subject_active_courses`, teachers, merge membership | Subject → configured active classes → labels/merge annotation | Aggregate the same metadata once per subject. Do not select one arbitrary teacher or enrollment as the active class. |
| `course_rule_configs`, `effective_course_students` | Effective rules and continuation membership | Reuse existing views without rewriting them. |
| `app_settings.absence_policies` | Authoritative settings/rules document | Continue fetching current settings. Optionally decode a resolver input once per request. |
| `SitInBundleV2`, `sessionFact`, `courseJSON` | Loaded request facts → pure rules → response | Keep existing ownership and candidate-loading boundaries. |
| Absence transaction | Authoritative validation and mutation after selection | No implementation changes in this plan. |

No new persisted entity or lifecycle is required. Lookup does not transition an absence. Existing pending/approved/cancelled and special-approval transitions stay with their current writers.

## 4. Exact function-level design

All existing targets below were inspected. New helper/type names are **Proposed**, not existing repository symbols.

### A. Shared subject query — first database change

Location: `backend/internal/db/absence_custom.go`, `studentSubjectByWCode()`.

Keep these signatures and their callers unchanged:

```go
StudentSubjectByWCode(ctx context.Context, wcode string) ([]StudentSubjectRow, error)
StudentSubjectByWCodeForAbsenceForm(ctx context.Context, wcode string) ([]StudentSubjectRow, error)
studentSubjectByWCode(ctx context.Context, wcode string, requireAbsenceFormAvailability bool) ([]StudentSubjectRow, error)
```

Reads the same tables/views; writes nothing. Preserve case-insensitive matching, the boolean availability mode, scan column order/types, contact columns, `ORDER BY sub.code ASC`, DB error propagation and zero-row output.

Replace the multiplied join with two stages in one SQL statement:

1. For the matched student, select **distinct subject IDs** from direct `course_students` rows with `status = 'enrolled'`. Apply the existing optional `course_rule_configs.absence_form_active` check to the enrolled course **inside this stage**, before deduplication.
2. For each resulting subject, use a lateral aggregate over its `subject_active_courses` rows with the same course/teacher/merge joins and the same four metadata expressions. Select the student's original fields and that aggregate result without an outer enrollment-driven aggregate.

Keep subjects that have qualifying enrollments but no configured active course in ordinary staff mode. Preserve zero-subject fallback handling in `handleStaffStudentLookup()` and `handleStudentProfile()`; do not combine it with a new profile response or change which fields those fallbacks expose.

Extract the SQL into a private constant, proposed `studentSubjectByWCodeSQL`, in the same file only to let parity/EXPLAIN tests exercise the exact production statement. This is hand-written SQL; do not edit sqlc-generated files.

### B. Encode V2 responses once — first CPU/allocation change

Location: `backend/internal/httpapi/absenceshttp/sessions_range_service.go`, the ends of `serveSessionsRangeV2EnrolledHead()` and `serveSessionsRangeV2AllSubjectsHead()`.

Proposed local helper:

```go
func (s *server) writeSessionsRangeResponse(
    w http.ResponseWriter, r *http.Request,
    courses []courseJSON, tele sessionsRangeTelemetry, totalStart time.Time,
)
```

It owns V2 success encoding and the common final telemetry assignments. It reads the already assembled courses, writes only the HTTP response and existing telemetry, and performs no DB calls.

- Marshal the existing `map[string]any{"subjects": courses}` once; preserve map/struct serialization, HTML escaping, omitted fields and empty slices.
- On an encoding error, use the existing `internal` error envelope before committing a success status; log no student payload.
- Record `len(raw)` as `payload_bytes`, append one newline, set `application/json; charset=utf-8`, write status 200 and write those bytes directly.
- Do not pass the object or `json.RawMessage` back through `Adapter.WriteJSON()`: that would add another encoding pass.
- After a transport write error, log through the existing logger and do not attempt a second response. Preserve final phase timings/counts and telemetry privacy.
- Keep the general `Adapter.WriteJSON()` API unchanged; only these two V2 success sites need this behavior.

The trailing newline preserves the current encoder output convention. See the [Go 1.25.7 Encoder contract](https://pkg.go.dev/encoding/json@go1.25.7#Encoder.Encode).

### C. Native UUID subject comparison — small, measurement-gated SQL change

Location: `backend/internal/db/sessions_range_facts.go`, `sessionsRangeFactsAllSubjectsSQLText`.

Keep `SessionsRangeFactsParams.SubjectIDs []string`, both DB method signatures and their existing arguments unchanged. Replace only:

```sql
-- Current
WHERE sub.id::text = ANY(string_to_array($1, ','))
-- Proposed
WHERE sub.id = ANY(string_to_array($1, ',')::uuid[])
```

This moves conversion to the bound filter value and retains a native UUID column comparison. It avoids an unnecessary internal type migration and is shared automatically by the standalone method and head batch. Keep the current time bounds, ordering, projection and permission checks.

`parseSubjectIDFilter()` already validates/canonicalizes/deduplicates UUID strings before the HTTP path reaches SQL. Preserve `bad_subject_ids` and the nonempty requirement in all-subject mode. Test valid unknown UUIDs returning no matches, duplicate filters and malformed HTTP filters being rejected before facts SQL. Do not depend on malformed internal UUID strings continuing to act like valid filters; audit direct DB callers before merging this slice.

An empty or NULL array must never mean “all subjects”; the HTTP gate remains mandatory. PostgreSQL documents these cases in [array comparison semantics](https://www.postgresql.org/docs/16/functions-comparisons.html#FUNCTIONS-COMPARISONS-ANY-SOME). Verify actual planner behavior before claiming an index improvement.

### D. Decode policies outside the V2 course loop — only if CPU profiles justify it

Inspected targets: `sessions_range_service.go`, `sessions_range_resolve.go`, `sessions_range_resolve_helpers.go`, `sessions_range_mapped.go`, `resolver.go` and `sat_verbal_policy.go`, all under `backend/internal/httpapi/absenceshttp/`.

Proposed request argument and constructor, placed beside `bundleSitInInputs`:

```go
type decodedSitInPolicies struct {
    value sqldb.AbsencePolicies
    err   error
}
func decodeSitInPolicies(raw []byte) decodedSitInPolicies
```

After the existing enrolled-mode reads and before the course loop, decode `policiesBytes` once when resolution is needed. Change the private `resolveEnrolledCourseSitInV2()` policy argument and `bundleSitInInputs.policies` field to this value. Do not store it on `server` or the DB query object.

In `resolveSitInForCourseFromBundle()`, inspect the stored decode error at the original automatic-policy decision point, **after** its current no-enrollment early return. Return the existing `policy parse: ...` error there; the service retains per-course log/omit behavior. Reuse `automaticSitInEnabledForPolicies()` rather than decoding bytes again.

Add typed helper variants in the existing helper owners, and have raw-byte wrappers reuse the same decisions after decoding:

```go
mergeGroupWindowWeeksForPolicies(p sqldb.AbsencePolicies, mergeID string) (int, bool)
rootGroupWindowWeeksForPolicies(p sqldb.AbsencePolicies, rootID string) int
scopeWindowWeeksForPolicies(p sqldb.AbsencePolicies,
    mergeScope sqldb.CourseMergeGroupScopeForCourseRow, hasMerge bool, root pgtype.UUID) int
subjectWindowWeeksWithMergeForPolicies(p sqldb.AbsencePolicies,
    subjectID, fallbackRootID, mergeID string) int
```

Preserve these distinct precedence rules: ordinary scope uses a present merge override, otherwise root; mapped SAT uses a present merge override, otherwise a positive subject window, otherwise root. A present zero merge window must not fall through to a positive fallback. Missing/invalid raw policies keep the existing wrapper defaults/error behavior.

Do not combine this with changing `parseAbsenceSettings()` defaults or the DB bundle discovery cutoff. Those owners parse different projections and have different failure rules. The goal is to remove **per-course** policy decoding, not to claim that all settings parsing across the endpoint becomes a single decode.

Update inspected test callers in `policy_audit_integration_test.go`, `sessions_range_mapped_visibility_integration_test.go` and `sessions_range_mapped_visibility_test.go` as needed for the private input contract. Retain mapping-rule decoding, ordering and course selection behavior; precomputing SAT results or adding per-course lookup maps is outside this slice.

## 5. Algorithm and failure branches

### Subject query control flow

The following is SQL design pseudocode; copy the existing select fields and aggregate expressions exactly during implementation:

```sql
SELECT s.<existing student/contact columns>,
       sub.id, sub.code, sub.name,
       meta.active_course_id, meta.active_teacher_name,
       meta.merge_group_id, meta.merge_group_name
FROM students s
CROSS JOIN LATERAL (
    SELECT DISTINCT c.subject_id
    FROM course_students cs
    JOIN courses c ON c.id = cs.course_id
    WHERE cs.student_id = s.id AND cs.status = 'enrolled'
      AND ($2 = false OR EXISTS (
          SELECT 1 FROM course_rule_configs cfg
          WHERE cfg.course_id = c.id AND cfg.absence_form_active
      ))
) enrolled_subject
JOIN subjects sub ON sub.id = enrolled_subject.subject_id
LEFT JOIN LATERAL (
    SELECT <the four existing metadata aggregate expressions unchanged>
    FROM subject_active_courses sac
    LEFT JOIN courses ac ON ac.id = sac.course_id
    LEFT JOIN users tu ON tu.id = ac.teacher_id
    LEFT JOIN course_merge_group_members mm ON mm.course_id = sac.course_id
    LEFT JOIN course_merge_groups mg ON mg.id = mm.group_id
    WHERE sac.subject_id = sub.id
) meta ON true
WHERE lower(s.wcode) = lower($1)
ORDER BY sub.code ASC;
```

Lateral references allow metadata to be computed for the preceding distinct subject; see [PostgreSQL 16 lateral subqueries](https://www.postgresql.org/docs/16/queries-table-expressions.html#QUERIES-LATERAL). The physical execution plan must still be measured. The intended aggregate input changes from E × A to A per subject; enrollment deduplication still reads the relevant E enrollments.

### V2 session control flow

```text
Existing authentication and forbidden-parameter guards
→ existing preliminary parse
→ existing settings read and final range/filter authorization
→ existing facts + student head batch
→ existing normalization and timing/course filters
→ existing bundle, expected-class/final-day checks and tail batch
→ optional one-time resolver policy decode, retained as a local argument
→ existing ordered course resolution and assembly
→ one JSON marshal, one response write, existing telemetry
```

Keep the DB phase/scan order and `pgx.Batch` draining behavior unchanged. Settings/facts/scope/count failures remain request-fatal; enrolled resolver failures remain degraded sit-ins; all-subject blocked-probe failures retain their existing fatal behavior.

Do not push `course_ids` or timing predicates into the fact query in this plan. Unfiltered sibling ranges, historical day counts, continuation membership and out-of-scope mapped SAT schedules have broader data requirements than the displayed course list. Narrowing them requires a separate parity-proven design.

No new read transaction or lock is needed for these changes. The subject membership and metadata stay in one SQL statement. Multi-statement session lookup retains its current consistency model; a previously displayed choice is not a reservation or commit guarantee.

## 6. Concrete implementation sequence

Implement as independently reversible slices; proceed to C/D only when the baseline identifies their cost.

| Step | Exact existing targets / proposed new files | Deliverable and exit condition |
| --- | --- | --- |
| 0. Record baseline | Existing HTTP `sessions_range_telemetry.go`, `sessions_range_shadow_test.go`, `sessions_range_scale_test.go`, `sessions_range_endpoint_test.go`; DB `sessions_range_plan_test.go`. Proposed DB test file: `backend/internal/db/student_subject_lookup_performance_test.go`. | Capture paired baseline fixtures, SQL plans, request timings, statement/trip counts, CPU and allocation measurements before changing algorithms. Record deployment V2 mode. |
| 1. Pin shared-query semantics | Extend `backend/internal/db/absence_integration_test.go` and `student_subject_merge_test.go`; use existing HTTP `absence_form_visibility_test.go`. | Baseline parity fixtures cover both availability modes, multiple enrollments, unowned active classes, NULL metadata and contact fields. Keep the old SQL only in the test oracle while comparing the rewrite. |
| 2. Implement A | `backend/internal/db/absence_custom.go` | Private SQL constant and distinct-subject/lateral query. Same rows and scan contract; one query. EXPLAIN confirms no multiplied metadata aggregate work. |
| 3. Implement B | `backend/internal/httpapi/absenceshttp/sessions_range_service.go`; extend `sessions_range_compat_test.go`. Proposed local benchmark/test file: `sessions_range_response_test.go` in the same package. | Both V2 branches use one response writer. Exact successful body parity, newline/header parity and reduced encoding allocations/CPU. |
| 4. Evaluate/implement C | `backend/internal/db/sessions_range_facts.go`; extend existing `sessions_range_plan_test.go` and HTTP request/all-subject tests. | Native comparison produces identical rows/order in standalone and head-batch paths; measured plan/work improves. Otherwise omit this slice. |
| 5. Evaluate/implement D | Existing resolver/service/helper files listed in section 4D; `resolver_test.go` and the three inspected private-contract test callers. | Typed policy argument removes repeated policy decode samples, retains policy precedence, errors, disabled behavior and mapped visibility. Otherwise omit this slice. |
| 6. Final compatibility/performance gate | Existing relevant DB/HTTP tests, frontend API tests and absence E2E flows below. | Record per-slice results, full-flow behavior, limitations and rollback instructions. Remove test-only old SQL once fixtures independently pin the contract, unless retained as an explicitly justified parity oracle. |

No migrations are planned. Before considering a missing index, inspect applied migrations and `pg_indexes` on the measured environment. A missing already-defined index is an environment/migration issue; do not create another equivalent index.

## 7. Worked traces

### Normal subject lookup — proposed deterministic fixture

Initial: student W1 has 40 enrolled courses in subject S. S has 20 configured active classes, all with teacher T and one common merge group M. Some active classes are outside W1's enrollments.

1. Staff lookup authorizes the staff actor and calls `StudentSubjectByWCode(W1)`.
2. The query matches W1, scans qualifying enrollments and produces subject S once.
3. Metadata aggregation reads S's 20 active classes once. It retains the existing minimum active ID, teacher T and merge M.
4. The handler returns the same one subject and contact/profile fields. The old query could supply 800 enrollment/active combinations to its aggregate; the proposed query supplies 20 metadata rows after membership deduplication. These are fixture cardinalities, **not measured production numbers**.
5. Verified-student lookup uses the same shape with the existing availability predicate. If all W1 enrollments fail that predicate, it returns the existing no-subject profile response.

Final: subject membership, labels and absence state are unchanged; no lookup data is retained across requests.

### Concurrent visibility change between lookup and submission

1. A verified student loads a currently available course/session.
2. Staff hides the effective course and commits the change after the lookup finishes.
3. The student submits the previously displayed session.
4. The existing student submission path rechecks course availability and returns `403 course_not_available`; the optimized lookup does not bypass that check.

Final: no successful absence is created by this rejected submission. Test this ordered scenario with explicit DB operations/barriers, not sleeps. The ordinary staff operational path continues to follow its own existing policy.

### Resolver policy decode failure

1. Seed a policy document that produces a resolver decode error under the existing typed policy model; run baseline and candidate against the same fixture.
2. Complete the same settings/facts/bundle/tail phases. Candidate carries the decode error as request-local input.
3. A course that reaches the automatic-policy guard returns the same parse error to the existing per-course handler.
4. The request retains its existing status, sessions/counts and omitted sit-in behavior. A course with no eligible enrollment keeps its existing early return.

JSONB prevents syntactically malformed stored JSON, but valid JSON with incompatible field types can exercise this branch. Raw-helper unit tests can also cover malformed bytes.

## 8. Verification and measurement

### Executed during planning

This existing-code check passed (`ok warwick-institute/internal/httpapi/absenceshttp`, package test time 2.019s):

```bash
cd backend
go test ./internal/httpapi/absenceshttp -run '^(TestStaffStudentViewLookupUsesStudentProjectionAndTiming|TestStaffStudentViewRejectsOperationalOverrides|TestStaffStudentViewRejectsDuplicateFlag|TestGolden_SubjectsContractShape|TestGolden_SubjectsContractMergeAndSitIn|TestRegression_AssemblyDeterministicOnSameInputs)$' -count=1
```

These six top-level tests provide a baseline for parser and response contracts, not proof that the proposed changes work. `TEST_DATABASE_URL` and `DATABASE_URL` were absent from the planning process environment. No DB integration tests, EXPLAIN runs, production reads, latency benchmarks or implementation tests were performed.

### Required verification matrix after implementation

| Cases / evidence | Existing or proposed test home | Requirements |
| --- | --- | --- |
| Both subject-query modes: no student; known student with no subjects; one/many enrollments per subject; mixed-case W-code; dropped/draft enrollment exclusion; all contact fields; no active classes; active class outside student's roster. Compare old/new rows including NULLs and order on the same transaction snapshot. | DB `absence_integration_test.go`; proposed `student_subject_lookup_performance_test.go` | FR-1, FR-3, INV-2, INV-3, ERR-1 |
| One teacher versus multiple/nonempty/blank teacher labels; all actives in one merge; mixed merged/unmerged actives; different merge groups; deterministic minimum active ID. | DB `student_subject_merge_test.go` and shared-query cases | FR-1, INV-3 |
| Student session required; identity/override rejection; staff authorization; public lookup remains restricted; staff student-view matches verified student visibility; unknown student differs from zero subjects. | HTTP `self_service_routes_test.go`, `absence_form_visibility_test.go`, `public_routes_contract_test.go` | INV-1, ERR-1 |
| All-subject valid, duplicate, unknown and malformed UUIDs; zero subject list rejected; identical standalone/head-batch results and order; staff lifetime 1970–2100 behavior retained. | Existing facts/endpoint tests plus `sessions_range_lifetime_test.go`, `sessions_range_request_test.go` | FR-2, INV-1, ERR-1 |
| Body bytes against the current `json.NewEncoder()` baseline: empty subjects, merged/mapped results, Unicode, HTML characters, omitted fields and final newline; `payload_bytes == len(body)-1`. A failing response writer must not trigger a second error response. | `sessions_range_compat_test.go`; proposed `sessions_range_response_test.go` | FR-2, FR-4 |
| Missing policy maps, global/scope disable, present zero merge window, positive subject fallback, missing root, incompatible policy field type; no-enrollment early return; same raw-wrapper results and course-level error behavior. | `resolver_test.go`, mapped visibility tests, `policy_audit_integration_test.go` | INV-5, ERR-2 |
| Continuation membership/source visibility, inactive mapped targets, priorities, timezone boundaries, full-history absence-day limits and final-day exclusions. | `course_continuation_integration_test.go`, mapped visibility tests, `policy_audit_integration_test.go`, `sessions_range_regression_test.go`, DB day-count parity tests | FR-2, INV-5 |
| Deterministic lookup→visibility change→submission rejection; existing same-session/idempotency races still pass; no partial batch writes. Shared subject rewrite still supports parent verification/contact consumers. | `absence_form_visibility_test.go`, `submission_race_test.go`, `step13_race_test.go`, `public_enrollment_contract_test.go`, staff batch tests | INV-4, ERR-1, ERR-2 |
| Traced SQL statements and network batches remain count-constant across course counts; privacy-safe timing logs remain populated. | `sessions_range_shadow_test.go`, `sessions_range_scale_test.go`; query tracer in proposed subject performance test | FR-3, FR-5 |

The existing DB test setup runs migrations, and some helpers delete shared fixture tables. Run DB tests **only against a disposable, isolated test database**, with packages serialized where helpers share tables. Do not point these commands at production or a shared development database.

Suggested commands after configuring that isolated `TEST_DATABASE_URL`:

```bash
cd backend
go test -p 1 ./internal/db ./internal/httpapi/absenceshttp -run 'Test(StudentSubject|StaffAbsenceStudentView|StudentProfile|StudentSessions|StaffStudentLookup|SessionsRange|Golden_|Regression_|ResolveMappedFromBundle|RootGroupWindowWeeks|MergeGroupWindowWeeks|AutomaticSitInEnabledForPolicies|PolicyAudit|AbsenceFormHiddenCourseGate|ParentVerification|BatchSubmission|Submission_|Idempotency_|Batch_OppositeOrder|MergeMembership_|StaffAbsenceFormBatch)' -count=1 -timeout=15m
go test -race ./internal/httpapi/absenceshttp -run 'Test(SessionsRangeV2_ConcurrentReads|Submission_|Idempotency_|Batch_OppositeOrder|MergeMembership_)' -count=1 -timeout=15m
```

Frontend contract smoke checks from repository root, with the existing test/server configuration:

```bash
npm run test -- src/features/absences/api/__tests__/absenceFormApi.test.ts src/pages/__tests__/AbsenceForm.student.test.tsx src/pages/__tests__/AbsenceForm.staff.test.tsx src/components/absences/__tests__/StaffCreateAbsenceModal.test.tsx
npx playwright test e2e/absence-form-critical-path.spec.ts e2e/absence-form-error-recovery.spec.ts
```

These are proposed implementation checks; they were not executed during planning. Do not change currently edited frontend files to implement this backend plan.

### Before/after performance procedure

1. Use the same isolated DB snapshot, PostgreSQL version, Go build, pool settings and application flags for paired baseline/candidate runs. Keep normal DB/OS buffer behavior constant; add no application caching and do not interpret repeated-run buffer warming as an optimization.
2. Record modes separately: verified student profile; staff lookup with/without student-view; student sessions; staff enrolled; staff lifetime; staff special subjects. Include the modal's separate subjects request in end-to-end timing rather than attributing its whole wait to course SQL.
3. For subject lookup, vary enrollments and active classes **independently** (for example E=1/10/100 and A=0/1/20), with unrelated subjects/enrollments as background. Include an unenrolled active class to detect accidental narrowing. Use moderate, constraint-valid fixtures before increasing scale.
4. For session lookup, reuse existing scale/mapped/lifetime fixtures; include realistic large sit-in payloads. Retain small/empty cases to catch overhead regressions.
5. On the exact old/new SQL, capture `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`. Compare actual rows × loops at the metadata aggregate input, reads/hits, temporary spill and execution time. For C inspect native UUID index conditions and total work. Do not assert one exact plan node name or demand index scans on tiny tables. [EXPLAIN ANALYZE executes the statement and reports actual work](https://www.postgresql.org/docs/16/using-explain.html#USING-EXPLAIN-ANALYZE).
6. Use the existing pgx test tracer to record statements separately from batch round trips. Add benchmark functions in the proposed test files for the exact shared query and full response-writing path; report `ns/op`, `B/op` and `allocs/op`. Benchmark both encodings/writes for the baseline, not just `json.Marshal()` in isolation.
7. Run at least five interleaved baseline/candidate measurement rounds, with the same request counts and concurrency. Report per-mode p50/p95, errors and allocation/CPU data. Request-local decode changes must remove per-course policy-decode work from profiles; JSON changes must remove the second full encoding.
8. **Release gate:** identical functional results/errors and stable query/trip budgets; subject aggregate work loses the E × A multiplier; one response encoding; target measurements improve beyond observed run-to-run variation. Compare non-target modes too. A latency difference within measurement noise is **inconclusive**, not a claimed speedup. Skip optional slices without evidence; do not assign an invented millisecond SLA or throughput gain.

Existing telemetry's `domain_ms` does not time the entire enrolled resolver/tail/assembly phase. Use endpoint measurements and CPU profiles for that work; do not treat the current phase field as total resolver CPU. Extend phase timing only if needed to diagnose the measured bottleneck, without logging W-codes, names, tokens, contacts or payloads.

## 9. Extensibility, rollout and remaining risks

- **Realistic extension:** adding another active class or teacher to a subject should change only authoritative rows. The distinct-subject query still aggregates the complete active set and requires no caller special cases.
- **Policy extension:** a new scope policy should be represented in `sqldb.AbsencePolicies` and its existing policy decision owner. If D ships, raw wrappers and the V2 typed path must share the same decision helpers; add a parity case instead of a second rule implementation.
- **Rollout:** ship A and B separately after their gates; C/D remain separately reviewable. All use the current schema and HTTP contracts, so mixed application versions can coexist. Revert the individual application slice if latency, errors or parity regress. Verify the deployment is actually using V2 before evaluating B/D; the existing V2 environment flag is unchanged. Turning V2 off does not revert shared-query change A or shared helper changes; use a code revert for those.
- **Unknown:** real query distributions, installed indexes/migration state, production PostgreSQL version, V2 rollout value, dominant bottleneck and p95 baseline. Resolve with the paired measurements above before promising an improvement. The staff lifetime response can remain intrinsically large; this plan preserves its requested data instead of silently truncating history.
- **Compatibility risk:** the subject query serves OTP/contact consumers as well as course lookup. Preserve every projection/null/empty behavior and run those regressions; do not opportunistically replace contact reads or alter fallback profile fields.
- **Consistency risk:** no read optimization guarantees a selection remains valid until submission. Existing write revalidation and concurrency tests remain the guarantee. No locks or validation steps may be removed to reduce latency.

Completion means an engineer can reproduce the old/new behavior and performance, identify the exact query/function changes, and demonstrate faster targeted work without caches or weakened absence rules. This document authorizes no claim that those implementation results already exist.
