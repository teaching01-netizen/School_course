# Active course availability low-level design

## 1. Operation goal and scope

An admin sets a course **On** or **Off** for student absence booking. The operation commits one desired state. Operations, course details, and student booking use the same effective availability. The student still needs an eligible enrollment and session, and staff bookings keep their existing rules. PostgreSQL is the consistency boundary. This design covers activation, subject reassignment, continuation links, and the student submission check.

The reported `26577383000` request uses a valid course UUID and `active: true`. A local PostgreSQL regression reproduced its `409` by moving an active course to a new subject while leaving its activation record under the old subject. Operations joins on both IDs, so it shows **Off**. The insert also conflicts with the existing unique key on `course_id`. Separately, Operations can display the state of a continuation member while the student form uses the rule source. The initial upsert fix addressed the reproduced `409`. The implementation below eliminates the duplicate subject reference. The production row and group type for this course still need verification. [Current activation SQL](../../../backend/internal/db/active_courses_custom.go) · [Current continuation rules](../../../backend/db/migrations/00128_course_continuation_links.sql)

## 2. Threats

| Scenario | Consequence |
| --- | --- |
| An active course changes subject | The stale subject/course pair hides the course and blocks reactivation. |
| An admin changes a continuation member while the form reads the source | The UI can report success without changing student availability. |
| Two writes overlap, including a subject replacement and a bulk toggle | The final state can reflect a mix of both requests. |
| A student submits from an open form while an admin closes a source | An eligibility check taken before the write lock can accept a stale selection. |
| Migration treats every old membership or every old visibility flag as enabled | Previously hidden courses can become available to students. |
| The commit succeeds and the client loses the response | Retrying the original intent must avoid a second mutation or audit event. |

## 3. Invariants

| ID | Rule | Enforcement and observable result |
| --- | --- | --- |
| I1 | Direct student booking state is a property of a course UUID. An enabled course has a subject. | One course column and a `CHECK`. An active course moved between subjects stays active under its new subject. |
| I2 | A continuation has one setting. A regular merge keeps each member's own setting. | `course_rule_configs` resolves the existing `rule_course_id`. Both continuation rows report the source's state. |
| I3 | Admin display and student booking agree on availability. | The admin list, course details, student lookup, session selection, and submission use `course_rule_configs.absence_form_active`. Student-specific rules apply separately. |
| I4 | Every command changes only its declared scope and commits atomically. | Single and bulk writes use resolved source IDs. The subject picker has its own replacement scope. Data, audit, and the idempotency result commit together. |
| I5 | A student submission is authorized against the availability in effect at its transaction's position relative to a toggle. | Submission locks the relevant course and source rows, then rechecks availability before writing. A submission serialized after **Off** is rejected. |
| I6 | Migration preserves effective availability and never opens a hidden course. | Backfill uses the old conjunction of valid membership and visibility, and compares the effective view before and after. |

## 4. State transitions

| Current | Command | Guard | Result |
| --- | --- | --- | --- |
| Off or On | Set On | Admin, existing source with a subject | Source is On. Repeat requests for On are successful no-ops. |
| Off or On | Set Off | Admin, existing source | Source is Off. Repeat requests for Off are successful no-ops. |
| On or Off | Move to another subject | Valid destination | State follows the course UUID. |
| On | Remove subject | Clear availability in the same update | Course becomes Off. Reassigning a subject later requires an explicit On. |
| Two direct states | Create continuation | Existing link guards and explicit rule-source selection | Source retains its setting. Follower's direct setting is cleared and the follower inherits the source. Show the availability change before confirmation. |
| Inherited state | Unlink continuation | Existing unlink guards | Source retains its setting. The former follower starts Off. Show this in the unlink confirmation. |
| Any | Invalid request, missing course, or unauthorized actor | Reject the command | Every target retains its prior state. Bulk validation covers the entire request before any write. |

An ordinary merge changes grouping while each course keeps its own switch. Keep the existing course-deletion constraints, including the guard on deleting a linked rule source. [Continuation lifecycle](../../../backend/internal/coursegroups/service.go)

## 5. Data model

Add a direct setting to `courses`. Its default and constraint cover new courses, imports, and direct SQL subject updates.

```sql
ALTER TABLE courses
  ADD COLUMN student_booking_enabled boolean NOT NULL DEFAULT false,
  ADD CONSTRAINT courses_student_booking_requires_subject
    CHECK (NOT student_booking_enabled OR subject_id IS NOT NULL);
```

Use the existing `course_rule_configs` view to publish `rule_course_id` and compute `absence_form_active` from the rule course's `student_booking_enabled`. A continuation follower's direct value stays false through the link and activation write paths. Use the existing unique membership index on `course_merge_group_members(course_id)` and subject/course index on `courses(subject_id, code, id)`. Primary-key access covers the source lookup. The legacy activation table and visibility flag are temporary rollback data after cutover. Remove them in a later migration after all consumers have switched.

Expose `rule_course_id`, `rule_course_code`, and effective `is_active` in the admin list. Show the editable switch on the source and a read-only **Follows [source course]** indicator on the follower. The API resolves follower IDs for existing single and bulk clients. Derive status filters, counts, and course-detail status from the same effective state. Keep legacy response booleans as projections during the transition. Inventory consumers of course PATCH `absence_form_visible` before cutover. Reject the deprecated input with a specific error and direct callers to the activation endpoint. Translating an old visibility-only `true` into activation could open a course that had no membership. [Course PATCH input](../../../backend/internal/httpapi/courseshttp/requests.go) · [Active Courses UI](../../../src/pages/operations/ActiveCoursesSection.tsx)

## 6. Core algorithm and consistency boundary

**Single and bulk PUT.** Keep the admin check, desired-state payloads, and existing actor/route/idempotency key. Parse and deduplicate UUIDs. Retain the 200-ID request cap. Return `404 course_not_found` for a missing course and `422 course_subject_required` when an On request resolves to a source without a subject. For a mixed valid/invalid batch, return the failing IDs and roll back the whole request. Resolve continuation sources before deduplicating the write targets. The UI sends the source IDs it displayed as preconditions. If a course now resolves to a different source, reject with `409 course_rule_source_changed` and reload before retrying. Existing clients that omit the precondition apply their intent to the current source. Return success after commit, with the source IDs, state, and affected course IDs. Keep the existing response fields during client rollout and document that `updated` counts changed source rows. The UI uses the response or reloads the current list and counters.

**Legacy subject picker.** Keep `PUT /api/v1/admin/active-courses` for its single-picker contract. The selected course and its resolved source must belong to the supplied subject. Return a specific mismatch error if the source is in a different subject. Replace the set of direct sources owned by that subject with the selected source, including when other subjects display inherited state from a source in this subject. Never clear a source owned by another subject. This rule prevents a subject replacement from modifying an unrelated subject's source.

**Locks and transactions.** Use one transaction-scoped advisory lock for writes that can alter availability, subject ownership, or continuation topology. Acquire it before course locks on activation, subject reassignment, continuation create/unlink, course/subject deletion, and legacy course updates. This serializes the legacy replacement's subject scan against a concurrent move or toggle. After acquiring it, resolve the target set in a fresh Read Committed statement and lock affected course rows in UUID order. Reuse the course-row lock used by course edits and link mutations. Keep the transaction short and free of network calls. Student submissions acquire only their ordered row locks. Reuse the transaction adapter's bounded deadlock retry pattern for the whole transaction and include an explicit retry result for a changed source lock set. Retried callbacks return outcomes without writing to the HTTP response or emitting side effects. [Existing transaction adapter](../../../backend/internal/httpapi/httpadapter/adapter.go) · [PostgreSQL explicit locking](https://www.postgresql.org/docs/16/explicit-locking.html) · [PostgreSQL Read Committed](https://www.postgresql.org/docs/16/transaction-iso.html)

```text
begin idempotent transaction
return stored result on a matching replay
acquire admin availability advisory lock
resolve requested IDs to source IDs, then validate the full request
lock the affected course rows in ascending UUID order
update changed source rows and write one audit record if any state changed
persist the success response with the idempotency record
commit
publish best-effort invalidation for affected course IDs
```

**Student submission.** Resolve the actual missed course and the selected or implied sit-in course. Before locking the student or sessions, resolve their source IDs and lock all course and source rows in one ordered set. For batch submission, collect the union across all items before the first course lock or write. Reread the source mapping, selected subject, and eligibility predicate in a fresh statement while holding those locks. If the mapping now requires a course lock outside the acquired set, roll back the lock acquisition savepoint and resolve the full set again. Include the actual courses of the submitted sessions in the ordered lock set. After locking the sessions, verify their courses and sources still fall within that set and are enabled before committing. Reject a closed missed course or sit-in destination using the existing domain errors. Keep the existing enrollment, session, and staff authorization checks. The current single and batch submission paths check availability before taking the missed-course lock. [Single submission](../../../backend/internal/httpapi/absenceshttp/routes.go) · [Batch submission](../../../backend/internal/httpapi/absenceshttp/batch_routes.go) · [Session visibility predicate](../../../backend/internal/db/sessions_range_facts.go)

## 7. Failure behavior

| Failure | Expected behavior |
| --- | --- |
| Missing course or invalid member in bulk | Reject with the affected ID. No partial update. |
| Update or audit write fails before commit | Roll back all changes. Replaying the key can safely try again. |
| Client loses the response after commit | Retry the same request and key. Return the stored result without replaying the mutation, audit, or invalidation. Refresh for current state because another admin may have changed it since. |
| Same key is used with a different payload | Return the existing idempotency conflict. |
| Course move, group change, or toggle overlaps another write | The outcome matches one order of committed commands. On retry exhaustion, return a retryable error and retain the prior committed state. |
| Invalidation fails or a tab is stale | The database remains authoritative. Reload on tab focus or manual refresh. Submission revalidates even if its listing was stale. |

## 8. Verification

| Invariants | Test through the normal boundary |
| --- | --- |
| I1 | Move an On course through the course edit endpoint. Check both subjects in Operations and verify the enrolled student's form. Attempt an enabled subjectless row directly in SQL and verify PostgreSQL rejects it. Replace the existing malformed-row fixture with an upgrade fixture once the old schema is retired. |
| I2, I3 | Activate and deactivate from a continuation source and via the follower API. Check both Operations rows, course details, student profile, sessions, and submission. Test an ordinary merge as a separate case. Unlink and verify the former follower starts Off. |
| I4 | Test single, bulk, and replacement commands including source/follower duplicates, mixed invalid input, a stale source precondition, and an overlapping move or toggle. Use PostgreSQL transaction barriers for the race tests. Inject an audit failure and check that every target and the idempotency result roll back. Replay the same key and confirm one audit event. |
| I5 | Exercise both orders of Off versus a student submission on independent connections. Use a continuation follower with a different source and test both missed and sit-in courses. Test a multi-item student batch with one closed target and verify all items roll back. Check the committed absence or the existing rejection code. Verify the staff path still permits a hidden course. |
| I6 | Run the migration over correctly active, explicitly hidden, orphaned subject-pair, subjectless, ordinary merge, and continuation fixtures. Compare old and new effective availability per course, including both continuation IDs. |

Run the existing integration packages and add the above cases to their corresponding HTTP suites. Use a disposable migrated PostgreSQL database in `TEST_DATABASE_URL`.

```bash
cd backend
go test -p 1 ./internal/httpapi/activecourseshttp ./internal/httpapi/courseshttp ./internal/httpapi/absenceshttp -count=1
```

Local PostgreSQL checks passed for activation, continuation lifecycle, visibility, subject changes, and same-key replay. The submission race reproduced a `201` before the lock fix and now returns `403` after the source closes. The two previously failing visibility tests now pass with corrected fixtures. Migration 00132 preserved availability across nine pre-upgrade fixtures, rolled down and back up, and rejected direct attempts to enable a subjectless course. [Visibility tests](../../../backend/internal/httpapi/absenceshttp/absence_form_visibility_test.go)

## 9. Rollout, monitoring, and recovery

1. Inventory legacy readers and writers, including the `legacy_refresh_course` job priority query, the course-list filter, and the student lookup's subject metadata aggregate. The aggregate should derive active candidates from courses in their current subject and effective source state. Leave the student enrollment and date predicates intact. [Legacy job query](../../../backend/db/queries/legacy_sync.sql) · [Student subject lookup](../../../backend/internal/db/absence_custom.go)
2. Rehearse on a representative database copy. Calculate the direct backfill for source and independent courses as `valid_old_membership AND old_visibility`; initialize follower direct state to false. Snapshot the old view and compare effective state for every course after the view change. Record mismatched memberships separately. Treat those as Off during backfill because they were excluded by the student gate. Let an admin decide which to turn On after review.
3. Coordinate a short cutover that drains old API and worker processes before running the final backfill and switching readers and writers. Railway's predeploy migration can run while the previous build is live, so predeploy alone does not establish that boundary. Keep the old columns and membership rows through verification. If rollback is needed before reopening writes, restore the old view and application against that retained state. After reopening, prefer a forward repair. Reverting to stale legacy data would discard activation changes since cutover. [Railway deploy configuration](../../../railway.toml)
4. Use the existing audit log for actor, request scope, affected source IDs, and prior and resulting states. Record rejected activation writes by error code and use the existing request and database logs for contention. Invalidate both source and affected follower course IDs after commit. Audit data excludes student identifiers.

**Incident check before cutover.** Run this read-only query with the UUID from the request. It shows the member's current subject and old membership, the actual rule source, and the source's current and recorded subjects. Confirm with Operations which source should be On before making a production change. After cutover, verify both the Operations row and the form with an eligible student and date range.

```sql
SELECT c.code, c.subject_id AS current_subject_id,
       direct.subject_id AS recorded_subject_id,
       rc.code AS rule_source_code, rc.subject_id AS source_subject_id,
       source.subject_id AS source_recorded_subject_id,
       cfg.absence_form_visible, cfg.absence_form_active
FROM courses c
JOIN course_rule_configs cfg ON cfg.course_id = c.id
JOIN courses rc ON rc.id = cfg.rule_course_id
LEFT JOIN subject_active_courses direct ON direct.course_id = c.id
LEFT JOIN subject_active_courses source ON source.course_id = rc.id
WHERE c.id = 'dfa1cd73-e41a-4ced-8bf4-2e6a96f83472'::uuid;
```

## 10. Residual risks

The design assumes that an active course should stay On when it moves to a different subject and that unlinking should leave the former follower Off. It also assumes a short coordinated deployment window is acceptable. Validate those operational decisions before rollout. The admin lock serializes metadata writes that affect availability, so measure its wait time against representative bulk and course-edit workloads. The implementation and migration were exercised locally. Production contention and the live course record still need verification during rollout.
