# State-Transition Points (Step 11)

Date: 2026-09-06. Branch: `deepswe-absence-consistency` (Steps 5/8/9 tree).
Method: every protocol below was read from the implementation (file:line cited).
Step 8's lock-order matrix (docs/sessions-range-lock-order.md) is the companion:
it says WHO locks WHAT in WHICH order; this doc says WHEN each operation
becomes externally visible and HOW waiters observe the winner.

Vocabulary (plan-mandated, applied strictly):

- **Decision/protection point**: the locked/validated write inside the tx.
- **Durable externally visible transition**: the successful tx COMMIT.
  An insert that later rolls back is NOT an assignment. A row visible only
  inside its own uncommitted tx is NOT observable by any waiter.

## 1. Transition table

| Operation | Decision/protection point | Durable externally visible transition |
|---|---|---|
| Sit-in assignment (4 create writers) | Course lock -> student lock (Step-9 F1) -> session fence + post-fence same-student recheck (Step-9.4) -> `AbsenceSitInsCreateWithSnapshot` insert, all inside one `WithIdempotentTx` tx | COMMIT of that tx |
| Absence-day consumption | Limit advisory `absence-limit:<wcode>:<course\|merge:group>` + course lock inside `projectedAbsenceDayStats` (submission_helpers.go:230-260), re-check `ProjectedLimitExceeded`, then `AbsenceCreate` in the same tx | COMMIT of that tx |
| Cancellation/reassignment (staff status) | `ManagedAbsenceGet` read + `ExpectedVersion` check + `validTransition` gate, then version-guarded `AbsenceStatusUpdate ... WHERE id=$1 AND version=$N` (single-statement atomic); on `cancelled`, `AbsenceSitInsReplaceWithSnapshot(id, nil, ...)` clears assignments in the SAME tx (management_routes.go:692-760) | COMMIT of that tx |
| Student self-cancel | `CancelOwn`: ownership check + conditional `UPDATE ... WHERE id=$1 AND lower(wcode)=lower($2) AND status = ANY('{pending,reviewed}')` (selfservice/service.go:94-146); losers get 0 rows -> re-read -> `ErrNotCancellable`; already-cancelled is idempotent success (returns row, nil) | COMMIT of that tx |
| Sit-in reassign (override) | Step-10 G3 closure: course lock -> student lock (F1 order, management_routes.go:888-907) -> overlap validation -> session fence + post-fence same-student recheck (management_routes.go:966-981) -> version-guarded `AbsenceSitInUpdate ... WHERE id=$1 AND version=$6` (absence_management_custom.go:663-674) -> `AbsenceSitInsReplaceWithSnapshot` (fences again idempotently inside) | COMMIT of that tx (own `WithIdempotentTx`, management_routes.go:876ff) |
| Idempotency ownership | `idempotency.Acquire` = first statement of the tx: `INSERT ... ON CONFLICT DO UPDATE` (idempotency_custom.go:43-50, adapter.go:332); same key + same fingerprint replays, same key + different fingerprint -> deterministic conflict | Durable ownership row + `Complete` (status+body) committed WITH the domain writes in the same tx; replay reads the stored bytes (adapter.go:350-379) |

## 2. How waiters observe the winner

- **Same-student submissions** serialize on the student row (`StudentsLockOrdered`
  via `LockStudentForAbsenceSubmission`, absence_custom.go:188-197). The waiter
  blocks until the winner commits or rolls back, then its post-fence recheck
  (`recheckSitInSessionsHeld`, submission_helpers.go:203ff) reads the winner's
  committed rows: committed winner -> loser aborts with 409
  `sit_in_session_already_used`; rolled-back winner -> rows absent, loser proceeds.
  Proven by `TestSubmission_SameStudentSameSessionRaceOneWinner` (barrier, 3x).
- **Version-guarded mutations** (status/notes/reassign) never block on each other:
  the loser's `UPDATE ... WHERE version=$N` affects 0 rows (`IsNoRows`) and maps
  to the established stale-absence conflict (Step 7: `writeStaleAbsence` /
  `writeSessionSnapshotResult` -> 409 `session_version_conflict`). No lock wait.
- **Self-cancel races** resolve by the conditional `UPDATE` row count, not by
  locking: exactly one canceller gets `RowsAffected()==1`; concurrent second
  cancel of a pending/reviwed row gets 0 rows -> re-reads current state.
- **Idempotent replays** never re-execute: `Acquire` returns the stored completed
  response (`cached.StatusCode != nil`), the no-op tx rolls back, and the cached
  bytes are written verbatim (adapter.go:357-362). First response and replays are
  byte-identical because the first response is read back from jsonb (adapter.go:379).
  Step-12 evidence: `TestIdempotencyMatrix_StaffCreate`
  (absence_limit_integration_test.go) pins same-key/same-payload byte-identical
  replay with one row, same-key/different-payload 409 `idempotency_key_reuse`
  with no new row, different-key/same-payload succeeding as a distinct operation
  (no cross-row duplicate guard exists - retry-with-new-key safety rests on the
  caller reusing one key per logical operation), and late replay of the first key
  still returning the original bytes with no new row. DB-level Acquire/Complete/
  concurrent-single-winner/scope-independence/stale-delete are covered in
  idempotency_integration_test.go.

## 3. Rollback / uncertain-commit semantics

- **Failure before commit** (validation error, conflict, panic): tx rolls back;
  no partial domain rows survive (all inserts are inside the single tx). The
  idempotency record stays incomplete (`StatusCode IS NULL`); the documented
  recovery is `stale_idempotency_record` 409 telling the client to retry with a
  NEW key (adapter.go:348-356). Retry-with-new-key is safe ONLY because the
  logical-duplicate guards (same-student conflict, version predicate, limit
  advisory) still fire on the retry - the new key does not bypass domain conflicts.
- **Failure after commit** (crash between COMMIT and response): the idempotency
  row is already `Complete`d in the same tx (adapter.go:369-379 orders Complete
  BEFORE Commit), so a retry with the SAME key replays the stored response
  without creating another record. This is the timeout-after-commit case.
- **Uncertain commit result** (commit error after send): caller receives 500
  `internal`; the commit may or may not have persisted. Client retries with the
  SAME key: if committed, replay returns the stored result; if not, the stale
  path directs to a new key and domain guards prevent duplicates. No path depends
  on process-local state (all coordination is the DB idempotency row).
- **Cross-student shared capacity**: student locks do NOT protect it (plan Step 10
  caveat). The absence-day limit IS protected cross-tx by the limit advisory lock
  (`absence-limit:...` xact lock, sat_verbal_policy_custom.go:144-145) taken inside
  `projectedAbsenceDayStats` before counting. Unbounded room-capacity contention
  remains advisory-only (`capacity_warning` display in handleSitInCandidates) -
  recorded as Step-10/13 item 10, not claimed here.

## 4. What Step 11 does NOT claim

- Latent bug found BY the Step-10 demonstration test (fixed 2026-09-06):
  `AbsenceSitInsReplaceWithSnapshot` passed `snapshotJSON` as `[]byte` to a
  jsonb column (absence_management_custom.go:861). pgx encodes `[]byte` as
  bytea-hex, which jsonb rejects (`22P02 invalid input syntax for type json`).
  The create path already used `string(snapshotJSON)` (absence_custom.go:483).
  Production reassigns to sessions with snapshots therefore 500'd; fixed to
  `string(...)`. This is exactly the Step-13 demonstration-test value: the
  reassign path had no coverage exercising its insert.
- Reassign-path DAY-LIMIT revalidation CLOSED 2026-09-06 (matrix G3 remainder):
  the reassign tx takes course+student locks and fences/rechecks sessions but
  never calls `projectedAbsenceDayStats` - and now proves it does not need to.
  `TestReassignDoesNotChangeAbsenceDayCounts` (absence_day_counts_integration_test.go)
  captures UsedAbsenceDays before/after a reassign that swaps the sit-in session
  and requires bitwise-identical counts. Sound because reassign
  (`AbsenceSitInUpdate` + `AbsenceSitInsReplaceWithSnapshot`) writes only
  sit_in_method / sit_in_course_id / sit-in assignment rows - never date_from /
  date_to / absence_missed_sessions / status, the only inputs to used-day
  computation. If a future change makes reassign affect day consumption, the
  test fails and routes the path through the projection.
- Batch opposite-order deadlock freedom CLOSED 2026-09-06 (Step-13 item 9):
  `TestBatch_OppositeOrderNoDeadlock` (step13_race_test.go) races two
  lockBatchCourseSet acquisitions over the same two courses in opposite item
  order; both commit, no 40P01, no barrier timeout.
- Session-edit-vs-submission interleaving CLOSED 2026-09-06 (Step-13 item 5):
  `TestSessionEdit_SubmissionSeesCommittedEdit` (stale ExpectedVersion ->
  deterministic SessionVersionConflictError with both versions; version-free
  resubmission snapshots the post-edit state) + `TestSessionEdit_EditorBlocksOnSubmissionFence`
  (editor-side row lock blocks while the submission fence is held).
  Edit-side uses the version-guarded UPDATE shape; production takes a
  strictly stronger lock set, so serialization here implies serialization there.
- Cancel-vs-submission CLOSED 2026-09-06 (Step-13 item 3):
  `TestSubmission_CancelVsSubmissionRace` (submission_race_test.go) races a
  submission against a version-guarded cancel + Replace(nil) in one tx; the
  cancelled absence holds zero sit-in rows and reads cancelled afterwards.
- Same-key cross-replica CLOSED 2026-09-06 (Step-13 item 7):
  `TestIdempotency_SameKeyAcrossReplicasOneRow` (step13_race_test.go) races
  two server instances over one pool with the same key + identical body:
  exactly one absence row; loser gets 409-in-flight or byte replay; a later
  sequential retry replays the winner bytes. No process-local state.
- Merge-edit-vs-submission CLOSED 2026-09-06 as a sensitivity control
  (Step-13 item 6): `TestMergeMembership_ChangeSerializesWithSubmissionLock`
  proves the merge writer takes the shared course lock (blocks while a
  submission holds it, commits after release and observes scope).
- Failure injection CLOSED 2026-09-06 (Step-13 before/between/before-commit):
  `TestFailure_BetweenWrites_NoPartials` (garbage-session FK between absence
  insert and assignment insert -> 0/0 rows), `TestFailure_BeforeCommit_NoPartials`
  (full write set rolled back -> 0/0), `TestFailure_IdempotencyAcquireRollback_NoKeyHeld`
  (Acquire in an aborted tx holds no key; retry acquires as new). All assert
  committed reads after rollback. After-commit replay is Step-12 item 4
  (TestIdempotencyMatrix_StaffCreate late replay); response-delivery failure
  reduces to the same replay path (bytes already Complete+d before Commit).
