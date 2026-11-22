# Staff Special Sit-in verification

## Specification

On the authenticated staff Create Absence routes, staff can explicitly select an active session from another course or subject, including the final class. The server checks that the override names a distinct selected session in the stated target course. It rejects a missed-class overlap, an expected-class overlap, and duplicate or conflicting bookings under the existing booking rules. Suggested sessions in the same submission retain normal eligibility checks. The approval status follows the absence type. A rejected item rolls back the batch.

This covers the staff modal's explicit per-session contract. Older staff API callers that omit the field retain their existing status-based behavior. The student submission paths use their existing policy.

## Assumptions and implementation correspondence

| Property | Implementation and local oracle | Remaining obligation |
| --- | --- | --- |
| Authorized, all-or-nothing batch | Both staff handlers require `MustAdmin`. Staff batch creates each item in one idempotent PostgreSQL transaction. Integration tests check that a later invalid item leaves zero absence and assignment rows. | Confirm deployed route configuration and middleware. |
| Explicit override and approval independence | The modal marks selected IDs, and scope merging retains them. `validateStaffSitInSelection` partitions the request by UUID and checks membership before running either policy. The handlers persist the requested status through their existing status path. Tests exercise client payload, mixed eligible/ineligible selection, malformed membership, and persisted pending status. | The parser, route permissions, and status persistence are reviewed and tested Go code. |
| Special Sit-in safety | The shared staff validator checks active status, target course, and missed-class overlap using `ValidSpecialSitInSessionCount`. PostgreSQL supplies expected-class blockers and existing-booking conflicts. The submit transaction holds the student lock and fences selected session rows before validation. Integration tests exercise conflict reasons, both create routes, and two concurrent requests for the same session. | The model treats these checks as observed facts. A concurrent edit to an independently scheduled blocker or enrollment change needs separate validation. |
| Reachable success | The staff lookup returns a final session in another subject. The modal sends its ID, and the service stores the assignment with pending status on local PostgreSQL. | The live subject from the screenshot and its data were not inspected. |

## Lean model and results

The model treats an explicit request as a list of selections after UUID membership validation. Each selection carries the policy and safety facts supplied by the implementation. The model's outcome is an optional approval status. This boundary covers admission and preserves the input status across any model-accepted batch.

| Theorem | Verified property | Axiom audit |
| --- | --- | --- |
| `authorized_batch_only` | A successful model batch has authorization, keeps its input approval, and accepts every selection. | `propext`, `Quot.sound` |
| `special_requires_safety` | Acceptance of a Special Sit-in requires all five safety facts. | `propext` |
| `suggested_in_mixed_batch_requires_eligibility` | Every Suggested Sit-in in a successful batch has normal eligibility and a clear booking. | `propext`, `Quot.sound` |
| `safe_special_can_override` | A safe Special Sit-in succeeds with ordinary eligibility false, for either approval status. | `propext` |

`lake build` and `lake env lean StaffSpecialSitIn.lean` both exited 0 on Lean 4.19.0 (commit `6caaee842e94`). `leanchecker` is unavailable in the pinned toolchain. The model uses Lean's standard logical axioms listed above.

## Application checks

Base source commit is `69cccef7ff9bb980f9a01a2db63cabadc40ac6f0` with this worktree change applied. PostgreSQL integration ran on a fresh local PostgreSQL 14.23 database with migration 133 and Go 1.26.3.

```sh
cd backend
TEST_DATABASE_URL=<disposable-postgresql-url> go test ./internal/httpapi/absenceshttp -run 'Test(StaffCreate|SitInSubmitValidation|StaffAbsenceFormBatch|Idempotency_SameKeyAcrossReplicasOneRow|Batch_OppositeOrderNoDeadlock|MergeMembership_ChangeSerializesWithSubmissionLock)' -count=1
```

The suite passed. It covers a duplicate assignment race, failed-batch rollback, and existing idempotency and lock behavior. An older test fixture initially failed on the base commit because its missed class belonged to an unenrolled course. The corrected fixture uses an enrolled missed course and a separate unenrolled make-up course.

## Production test plan and verdict

For a deployed-style staging check, run the same two-request assignment race through separate app instances. Verify one committed assignment, one conflict, and zero partial rows. Retry a committed request under the same idempotency key after dropping its response. Stage an edit to a regular class that overlaps the chosen Special Sit-in near submission, then verify the resulting assignment and schedule-impact behavior. Confirm behavior across a frontend/backend version transition. Deployment topology, production contention, and crash recovery under load remain unmeasured.

- Formal status **PROVED** for the admission model.
- Correspondence status **REVIEWED_WITH_TESTS** for the staff submission paths.
- Production evidence **NOT_TESTED**. Production correctness **NOT ESTABLISHED**.

The smallest application change is to apply the override only to explicitly marked session IDs at the shared staff write validator. Further guarantees about concurrent changes to regular schedules require evidence from that workflow.
