# Split course ID detection with one-click confirmation

Status: final design plan; no implementation performed.

Date: 2026-10-06

## Decision

Build an on-demand, read-only SQL detector, one dismissal table, and an admin panel on Course Levels. Reuse the existing continuation creation endpoint and service, extending them with an evidence precondition and transactional revalidation for suggestion confirmations.

**Same subject is a hard gate. Teacher equality is not a matching requirement.** Different teachers are normal for the real split-course cases described by the user. Display teachers as context only; differing, missing, or changed teacher assignments must not independently exclude or downgrade a suggestion.

Never auto-link. A continuation shares enrollment and absence rules, and a mistaken link can become difficult to reverse once absences reference it. Reserve one-click linking for unambiguous, high-confidence candidates; review candidates use the existing manual linking workflow.

## 1. Requirements, assumptions, and unresolved decisions

### Challenge the requirements

Same subject, normalized name, and schedule compatibility establish similarity, not identity. Separate classes may share those attributes across teaching periods. Missing configuration does not prove that a course is a continuation.

One click means one explicit decision after sufficient evidence is visible. The server must verify that evidence again. An old browser tab cannot authorize a materially different pair or configuration.

Prefer precision over recall: missing a suggestion is less costly than creating a wrong continuation.

### Evidence status

| Status | Finding |
| --- | --- |
| User-confirmed | Different course IDs can contain different sessions belonging to one real class, with only one ID fully configured. |
| User-confirmed | Same subject is the required identity gate; the two IDs can have different teachers. |
| Verified in repository | Continuations read source rules live and extend enrollment through the existing views. |
| Verified in repository | Creation is admin-only, uses idempotency, locks course rows in UUID order, and audits within the transaction. |
| Verified in repository | Existing creation checks grouping and SAT Verbal policy but does not validate the proposed detector evidence. |
| Verified in repository | Group names are globally unique; the service accepts exactly two members. |
| Verified in repository | Unlinking is rejected once absences reference the continuation group. |
| Assumed | Dismissals remain permanent for the same pair until explicitly reversed through audited maintenance. |
| Assumed | High confidence requires reliable evidence of the same teaching period. |
| Assumed | Same-date sessions exclude high confidence; slot differences require review. |
| Unknown | Production session count, request volume, database capacity, and measured detector latency. |
| Unknown | Whether cycle dates reliably identify the teaching period when the split course has no cycle. |
| Unknown | Whether every relevant configuration, schedule, import, and policy writer participates in compatible concurrency controls. |
| Unknown | The approved recovery procedure for a wrong link already used by absences. |

### Questions that can change the design

1. Can a wrong continuation be separated after absence records exist, and who owns repairing those records?
2. What authoritative evidence establishes the same institute and teaching period when U has no cycle?
3. Can legitimate split IDs share a calendar date or have different weekly slot patterns?
4. Should a dismissal survive later name, configuration, and schedule edits?

Conservative defaults in this plan are permanent dismissal, strict period evidence for high confidence, no same-date sessions for high confidence, and no automatic repair of used links. Teachers remain context only under every default.

Read-only discovery can be developed while these questions are resolved. One-click confirmation must pass the concurrency and recovery release gates below.

## 2. Scale, latency, availability, and resource limits

These are planning targets, not measurements or guarantees.

| Dimension | Initial target |
| --- | --- |
| Reference workload | 13,000 courses, as stated in the original plan; provisionally 1 million sessions |
| Growth workload | 130,000 courses and 10 million sessions |
| Request load | 1 suggestion request/second sustained; burst of 10 concurrent requests |
| Detector SQL latency | p95 at or below 100 ms on the reference workload |
| Suggestion API latency | p95 at or below 300 ms; p99 at or below 1 second |
| Confirm/dismiss API latency | p99 at or below 1 second under normal contention |
| Response limit | 50 items by default, maximum 100 |
| Detector execution limit | 1-second database statement timeout, bounded by request cancellation |
| Availability | Target 99.9% successful valid requests monthly, subject to existing application/database availability |

Measure on representative hardware and data before adopting these targets as an SLO. Suggestion failure must not prevent loading the Course Levels table or using existing manual linking.

## 3. Options and selection

| Option | Complexity | Failure behavior | Consistency | Operability | Cost | Reversibility |
| --- | --- | --- | --- | --- | --- | --- |
| A. Minimal detector and dismissal table; unchanged create endpoint | Lowest | Stale or ambiguous evidence can reach creation | Atomic linking, without binding to displayed evidence | Few moving parts; weak decision traceability | Lowest | Feature easy to disable; used links difficult to undo |
| B. Hardened on-demand detector with evidence precondition and transactional revalidation | Low to medium | Changed evidence becomes a conflict; ambiguity requires manual review | Consistent reads and atomic decisions; writer compatibility explicitly verified | Small feature with useful audit and metrics | Existing database plus bounded query work | Feature easy to disable; underlying link-repair limitation remains |
| C. Persisted proposals, scanner, proposal revisions, and workflow states | Highest | Worker outages delay discovery; stale proposals still require revalidation | Eventually consistent discovery, transactional confirmation | Requires jobs, reconciliation, retention, and monitoring | Highest | Proposal history helps investigation but cannot reverse absence effects |

Choose **B**. It addresses stale decisions and ambiguous matches without a scanner, cache, queue, or stored proposal lifecycle.

## 4. Data ownership and boundaries

- Existing courses, sessions, cycle metadata, enrollment, and policy tables remain authoritative.
- Existing teacher data supplies display context only.
- `coursegroups.Service` owns continuation creation and permanent continuation invariants.
- The detector owns derived evidence only. It never changes course configuration, enrollment, attendance, or counters.
- `course_link_dismissals` owns durable negative decisions.
- The frontend displays evidence and submits intent. It cannot choose server confidence, override eligibility, or choose an unconfigured source.

Scope candidates to the existing institute boundary. Reuse the application's configured `InstituteTZ`; do not use the browser timezone. If the application supports only one institute, reuse that boundary rather than introducing tenancy infrastructure.

Retain pair-only continuations. Splits involving three or more IDs require a separate change to continuation semantics.

## 5. Detection rules

Let C be the configured source and U the partially or wholly unconfigured split course. C is always the continuation rule source.

### Hard exclusions and eligibility

1. C and U are distinct, non-deleted courses in the same institute scope and have the same non-null `subject_id`.
2. Apply the existing archive eligibility policy consistently. Confirm its behavior before implementation; do not invent a separate archive policy for this feature.
3. C has a valid level, cycle, and root course group.
4. U lacks at least one of level, cycle, or root course group.
5. Every non-null configuration field on U agrees with C, including root group. A conflicting non-null cycle, level, or root group excludes the pair.
6. Neither course belongs to `course_merge_group_members`.
7. Neither course has an active SAT Verbal policy mapping.
8. No dismissal exists for the canonical pair.
9. Both have non-deleted sessions with valid intervals.
10. Exact normalized names match, and the normalized key is non-empty.
11. Known evidence of different teaching periods excludes the pair. Unknown period evidence can produce review only, never high confidence.

Teacher IDs, teacher sets, teacher names, and missing teacher assignments do not participate in matching confidence or exclusion.

### Name normalization

Use the database's consistent lowercase/alphanumeric normalization for exact equality. Test Thai names, digits, punctuation, whitespace, empty results, and collisions. Keep normalization behavior identical between list and confirmation.

Document the deliberate ceiling: exact normalized-name matching misses typos; consider trigram matching only on the unconfigured set if staff report concrete misses.

### High confidence

Require all eligibility rules plus:

- Reliable evidence that both courses belong to the same teaching period. Use authoritative cycle bounds where available; a null U cycle is not evidence that arbitrary historical sessions belong to C's cycle.
- Every U slot belongs to C's slot set, using institute-local weekday and start time.
- Session durations are compatible. For the initial conservative rule, require equal durations for the matched slots rather than inventing an unapproved tolerance.
- No cross-course interval overlaps, treating intervals as half-open `[start_at, end_at)`.
- No cross-course sessions have the same institute-local start date under the conservative same-date rule.
- Both courses have exactly one plausible partner.

Calculate ambiguity over the full eligible candidate set, including review candidates, **before confidence filtering and pagination**. A hidden candidate must not make a visible pair appear unique.

High is a rule tier, not a calibrated probability of correctness.

### Review

Review candidates pass the hard exclusions and exact-name rule but have unknown period evidence, differing slot/duration patterns, or ambiguous partners.

Review candidates appear behind a toggle and open the existing manual linking workflow. Explain the missing or conflicting evidence. Do not enable direct one-click confirmation for them.

Overlapping or same-date sessions never qualify as high confidence. If exposed for manual investigation, mark the conflict explicitly and keep them in review.

## 6. Query and storage design

### Detector

Implement hand-written SQL following `course_groups_custom.go` conventions:

- Filter configured and unconfigured courses first.
- Join on subject and exact normalized name before reading detailed sessions.
- Read and aggregate non-deleted sessions for candidate course IDs.
- Derive period, slot, duration, and complementary-session evidence.
- Resolve full candidate ambiguity before selecting the requested confidence tier and page.
- Fetch teacher context for the returned courses without using it as an eligibility filter.

Use one SQL statement for a consistent result snapshot. Reuse the existing active-session course/start index before adding an index. A hash join is an optimization candidate, not a correctness or release requirement.

### Dismissals

Add `00129_course_link_dismissals.sql` with:

- Non-null `course_a` and `course_b`, ordered canonically by UUID.
- Composite primary key `(course_a, course_b)`.
- `CHECK(course_a < course_b)`.
- Course foreign keys with `ON DELETE CASCADE`.
- Server-assigned actor and creation time.
- Actor retention compatible with the existing user-deletion policy; do not cascade user deletion into removal of the decision.

Repeated dismissal is a successful no-op and preserves the original decision. Audit an actual insertion only. Do not expire dismissals automatically after edits.

Initially, mistaken dismissals are reversed through an approved, audited maintenance procedure. Do not add a management screen unless demand warrants it.

## 7. API contracts and flows

All operations require server-side admin authorization and existing institute-scope checks.

### List

`GET /api/v1/admin/course-link-suggestions`

Support bounded pagination and a review toggle. Return an envelope with:

- `items`, `has_more`, and stable pagination information.
- `evaluated_at`, detector version, and institute timezone.
- For both courses: IDs, codes, names, configuration, session counts, date ranges, slot summaries, and teacher display context.
- Source cycle label, level, root group, and the effective rule consequences relevant to staff.
- `confidence`, stable reason codes, ambiguity information, and an opaque evidence fingerprint.

Reason codes describe actual evidence, such as same subject, normalized name match, same period, compatible slots, complementary dates, and unique partner. Do not emit a teacher-match reason.

The fingerprint covers all decision-relevant evidence and detector version. Counts and maximum timestamps alone cannot detect all replacements or deletions. Context-only teacher edits do not independently invalidate the suggestion; exclude teacher fields from the eligibility fingerprint. Avoid relying on a broad course `updated_at` that changes for unrelated context edits.

The fingerprint is a precondition, not authorization. The server always recomputes eligibility. Return no student identities.

### Dismiss

`POST /api/v1/admin/course-link-suggestions/dismiss`

Accept exactly two distinct course IDs and the evidence precondition. Canonicalize IDs server-side. Lock, validate stale state, insert the dismissal, and audit within one transaction. Repeating an already successful dismissal returns success without another decision audit.

### Confirm

Reuse `POST /api/v1/course-groups` with:

- `kind: continuation`.
- Exactly the suggested pair.
- The configured source as `rule_source_course_id`.
- An optional suggestion precondition identifying this new flow.

The suggestion precondition is the necessary extension: the unchanged endpoint cannot verify which evidence staff approved. Preserve existing manual continuation behavior when the suggestion precondition is absent.

For suggestion confirmation, supply a deterministic default group name compatible with the existing globally unique constraint. Surface a meaningful name conflict rather than an opaque database error.

### Confirmation transaction

1. Authenticate and authorize before mutation.
2. Acquire the two course locks in UUID order.
3. Reread grouping, dismissal, policy, and configuration state.
4. Recompute matching evidence and full candidate ambiguity.
5. Reject materially changed evidence or lost high-confidence eligibility with `409 suggestion_changed`.
6. Call the existing continuation creation service.
7. Include detector version and approved decision evidence in the existing creation audit.
8. Commit membership, source, audit, and idempotency result together.
9. Return success and perform existing post-commit update/refetch behavior.

Use the same idempotency key and identical payload for retries of one user action. A refreshed, materially different decision is a new action.

## 8. Concurrency and consistency

Confirm and dismiss acquire the same ordered course locks. If dismissal commits first, suggestion confirmation is rejected. If linking commits first, dismissal must not record a misleading negative decision for that now-linked pair.

Reuse membership uniqueness as the final defense against two incompatible groups claiming the same course. Different administrators and different idempotency keys cannot bypass it.

Course-row locks do not automatically lock existing child sessions or prevent new child rows. Before enabling one-click confirmation:

- Trace configuration, schedule, import, deletion, and SAT-policy writers that can affect eligibility.
- Establish compatible locking or serialization at the existing shared write boundaries.
- Enforce SAT mapping/continuation exclusion in both directions.
- Define and test the ordering of concurrent evidence edits and the confirmation decision, including changes to competing candidates.

Use the existing serializable retry adapter where required. Retry callbacks return structured results and do not write HTTP responses or perform external side effects. Do not assume serializable isolation on confirmation alone proves whole-system serializability when other relevant writers use incompatible controls.

Eligibility applies to the accepted decision snapshot. Later legitimate schedule changes may change the evidence and do not automatically unlink a confirmed continuation. Permanent continuation invariants remain enforced independently of detector confidence.

**Release gate:** do not enable one-click confirmation while a demonstrated concurrency gap remains.

Reference: [PostgreSQL transaction isolation](https://www.postgresql.org/docs/current/transaction-iso.html).

## 9. States and invariants

### Derived pair states

`ineligible -> high / review -> linked or dismissed`

Unconfirmed pairs may move between eligibility tiers as data changes. No pending-proposal row is persisted. Dismissed pairs remain suppressed until explicit reversal; linked pairs leave the detector.

### Frontend states

`loading -> ready / empty / unavailable`

A row moves to `submitting`, then success, changed-evidence conflict, or retryable failure. Disable its actions while submitting. Server-side controls remain authoritative.

### Permanent invariants

1. Explicit admin action is required for every link.
2. A continuation has exactly two distinct members and a rule source that is one of them.
3. A course belongs to at most one group.
4. SAT-policy exclusion is enforced at the authoritative write boundaries.
5. Dismissal is unordered logically and canonical physically.
6. Successful mutation, audit, and idempotency result are atomic.
7. A stale suggestion cannot silently authorize a different decision.
8. Rules remain live source references; configuration is not copied to bypass the unique index.
9. Teacher equality is never a detector or suggestion-confirmation requirement.

## 10. User experience

Add a "Possible same course" panel above the Course Levels table. Show:

- Both course codes and accessible IDs.
- Both teacher summaries as context, including differing or unavailable assignments.
- Session counts, date ranges, and slot summaries.
- Match reasons and the configured source's rules.
- High-confidence actions: "Link as same course" and "Not the same".
- Review toggle and a manual-review action for review candidates.

Place the consequence beside the linking action:

> Linking shares enrollment and absence rules. Once absences use the link, it cannot be unlinked through this screen.

Keep confirmation one click because the consequence and evidence are already visible. Use keyboard-operable controls, announced results, and predictable focus after row removal.

On a changed-evidence conflict, refetch and require a fresh decision. On a successful mutation followed by refresh failure, preserve the successful result and report the refresh failure separately.

Skip the navigation badge in v1. It would add detector calls outside the page that needs them.

## 11. Failure behavior

| Failure | Required behavior |
| --- | --- |
| False positive | Enforce period/configuration safeguards and ambiguity suppression; staff must still explicitly decide. |
| Concurrent confirmations | One compatible decision succeeds; incompatible attempts receive a conflict. |
| Partial database failure | Membership, rule source, audit, and idempotency result all roll back. |
| Lost response after commit | Same-key retry returns the recorded result without a duplicate group or audit. |
| Serialization/deadlock failure | Use bounded existing retry behavior; return a retryable failure when exhausted. |
| Stale evidence | Return `409 suggestion_changed`; never silently reinterpret the submitted decision. |
| Refresh failure after commit | Show mutation success separately; do not encourage repeating the write. |
| Overload | Bound response size and query execution; honor cancellation; avoid polling. |
| Database outage | Reads show "Suggestions unavailable"; writes fail closed. |
| Bad data | Empty name keys, invalid intervals, or unknown period evidence never qualify as high confidence. |
| Security violation | Reject unauthorized/scope-invalid requests; parameterize SQL and validate UUIDs and body shape. |
| Wrong link already used | Existing unlink restriction remains; use an approved data-repair procedure preserving history. |

Durability inherits the current PostgreSQL commit, backup, and failover guarantees. Verify those guarantees; do not claim a new recovery-point or recovery-time objective from this feature alone.

## 12. Change locations

1. `backend/db/migrations/00129_course_link_dismissals.sql`: dismissal table and reversible additive migration.
2. `backend/internal/db/course_link_suggestions_custom.go`: list/evidence queries and transactional dismissal operations, following existing custom query conventions.
3. `backend/internal/httpapi/courselevelshttp/routes.go`: admin list and dismissal endpoints.
4. `backend/internal/httpapi/courseshttp/course_group_routes.go`: suggestion precondition and transaction-runner integration for the existing create endpoint.
5. `backend/internal/coursegroups/service.go`: suggestion-specific validation at the existing creation boundary; preserve existing manual behavior.
6. `src/features/courses/api/courseApi.ts`: suggestion contracts, fetch/dismiss helpers, and optional confirmation precondition.
7. `src/pages/CourseLevels.tsx`: panel, confidence toggle, actions, and existing toast/refetch integration.
8. Existing relevant test locations: database, HTTP, continuation, and Course Levels tests.

Only extend other writer boundaries if concurrency tracing shows it is required. Do not scatter duplicate guards across callers.

## 13. Verification and acceptance

### Database integration

Use an isolated scratch database created from the representative fresh template, with `TEST_DATABASE_URL`. Confirm template migration state before applying pending migrations. Never run destructive test setup against production.

Cover:

- Same subject, different teachers, and all other high-confidence conditions: suggested as high.
- Different subject: excluded even when names, teachers, and slots match.
- Teacher edits alone: suggestion remains eligible at the same confidence.
- Positive pair; different cycle, level, or root group; both configured; both unconfigured.
- Already grouped, SAT Verbal mapped, dismissed, deleted, and archive-ineligible courses.
- Empty name keys, Thai normalization, punctuation collisions, and exact-name mismatches.
- Different historical periods, missing cycle bounds, duration differences, midnight/timezone boundaries, adjacent intervals, overlaps, and same-date sessions.
- Multiple partners, including candidates hidden by the confidence toggle or page limit.
- Idempotent canonical dismissal; linking removes the pair.

### HTTP and concurrency

Cover admin-only access, response shape, invalid input, scope checks, audit payloads, and no student identities. Add:

- Changed evidence between GET and POST.
- Context-only teacher changes do not independently reject confirmation.
- Confirm/confirm, confirm/dismiss, and confirm/policy-or-schedule races.
- Concurrent changes to competing candidates.
- Lost-response replay and same-key/different-payload rejection.
- Audit failure rollback and retry exhaustion.
- Existing manual merge and continuation behavior.

### Frontend

Cover panel rendering with different teachers; configured-source confirmation with `kind: continuation`; dismissal; review toggle/manual-review behavior; submitting state; conflicts; unavailable state; successful mutation followed by failed refetch; keyboard and focus behavior.

### Performance

Run `EXPLAIN (ANALYZE, BUFFERS)` on reference and growth data, including common-name collisions, many same-subject courses, and skewed session counts. Measure concurrency and cancellation, not only a warm single request.

Prefer candidate-course session access. Evaluate scans and indexes by measured latency, buffer reads, and resource use; do not require a specific join or forbid every sequential scan regardless of cost.

### Build checks

- From `backend`: `go build ./...` and `go vet ./internal/...`.
- Check changed Go files with `gofmt -l`.
- From the repository root: `npx tsc --noEmit -p tsconfig.json`.
- Run the relevant Go integration and Vitest suites.

No implementation, test execution, or performance measurement has been performed as part of authoring this plan.

## 14. Rollout, observability, and rollback

Use one server-side feature setting supporting disabled, read-only discovery, and confirmation-enabled stages.

1. Apply the additive schema and compatible backend changes.
2. Enable read-only discovery and validate known split pairs and counterexamples with staff.
3. Complete concurrency verification and establish the recovery owner/procedure.
4. Meet performance targets on representative workloads.
5. Enable one-click confirmation, retaining the feature switch for immediate disablement.

Track query latency/timeouts, candidates by tier, ambiguity, dismissal/confirmation counts, stale conflicts, and transaction retries. Keep metrics free of high-cardinality course IDs and unnecessary personal data; detailed decision evidence belongs in the authorized audit trail.

Feature rollback disables discovery/confirmation and leaves existing manual linking available. Preserve confirmed links, dismissals, and audits. Disabling the feature does not reverse shared absence effects. Prefer disabling code over immediately dropping the additive table.

## 15. Twelve-month pre-mortem

| Likely failure | Early signal | Response |
| --- | --- | --- |
| Repeated subject/name/slot patterns produce wrong suggestions | High dismissal rate or wrong-link reports | Tighten period evidence; disable confirmation when needed. |
| Legitimate schedule variation hides real splits | Staff continue linking manually | Review concrete missed examples before relaxing rules. |
| Imports race with confirmations | Conflict spikes or inconsistent race tests | Repair concurrency at authoritative write boundaries. |
| Data growth makes on-demand detection expensive | p99, buffer reads, and timeouts rise | Optimize measured bottlenecks; consider persisted proposals only afterward. |
| Three-ID splits become common | More ambiguous candidate clusters | Revisit pair-only continuation semantics separately. |
| Wrong links already have absences | Existing unlink endpoint rejects removal | Use reviewed data repair preserving history. |
| Permanent dismissals hide subsequently corrected pairs | Repeated support requests | Add audited dismissal reversal if demonstrated demand warrants it. |

## 16. Remaining risks and explicit exclusions

Exact matching can miss genuine splits. Matching signals can still describe separate classes, particularly without a trustworthy teaching-period boundary. Human confirmation reduces that risk but does not eliminate it. The largest unresolved consequence is repairing a mistaken continuation after it affects absence records.

Do not add background scanners, auto-linking, fuzzy/trigram matching, notification emails, navigation-wide detector calls, copied settings, or a persisted proposal workflow in this version.

Teacher equality remains explicitly excluded from all matching and confidence rules.
