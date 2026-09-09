# Transaction Protocol - Lock-Order Matrix (Step 8)

Date: 2026-09-06. Branch: `deepswe-absence-consistency` (Step-5 tree + Steps 6-7).
Method: every sequence below was read from the implementation (file:line cited),
not inferred from file names. "Done when" for Step 8: this reviewed matrix
covering every mutation path. Fixes belong to Steps 9-10 + 13 (gated separately).

## 1. The two existing lock orders (they disagree - this is the headline finding)

### Order A: schedulelock.LockResources (backend/internal/schedulelock/locks.go:48-55)

Global order, one kind at a time, IDs sorted within kind (normalizeLockIDs):

1. course (CoursesLockOrdered)
2. student (StudentsLockOrdered)
3. teacher (UsersLockOrdered)
4. room (RoomsLockOrdered)
5. session (SessionsLockOrdered)
6. series (SeriesLockOrdered)

Users (all verified callers): scheduling/service.go (create session/series,
edit occurrence/entire series), series/service.go, crmimport/reconcile,
crmimport/crossstudy Save+Delete (courses+students in ONE call - order-clean),
legacysync/apply/schedule.go (advisory legacy-course key FIRST, then
LockResources{courses} - advisory-before-row-lock is consistent everywhere it
appears).

### Order B: absence submission path (all four create writers)

staff_create.go:173, staff_create_tx.go:144, routes.go:630 (public),
batch_routes.go:477 (per item). Identical sequence:

1. WithIdempotentTx acquire (INSERT ... ON CONFLICT DO UPDATE,
   idempotency_custom.go:43-50) - idempotency ownership FIRST. Matches the
   plan's proposed position 1.
2. LockStudentForAbsenceSubmission = StudentsLockOrdered single student
   (absence_custom.go:188-197).
3. ensureSitInSessionsAvailable - READ-ONLY conflict check, no locks taken.
4. AbsenceCreate (INSERT student_absences).
5. setAbsenceMergeGroupForCourse -> CourseMergeGroupLockCourses
   (submission_helpers.go:44-58; FOR UPDATE OF c, ORDER BY c.id).
6. projectedAbsenceDayStats (submission_helpers.go:152-212):
   CourseMergeGroupLockCourses AGAIN + AdvisoryLockForText
   "absence-limit:<wcode>:<course|merge:group>" (sat_verbal_policy_custom.go:144-145).
7. ValidSitInSessionOverlap / ValidMissedSessionCount (read-only checks).
8. AbsenceSitInsCreateWithSnapshot / AbsenceMissedSessionsCreateWithSnapshot
   (session reads via SessionGetByIDForSnapshot - NO FOR UPDATE, see Step-9 gap G2).
9. Audit inserts. Commit = externally visible transition.

So Order B = idempotency -> STUDENT -> course -> advisory -> dependent rows,
while Order A = COURSE -> student -> ... -> session. The student/course pair is
taken in OPPOSITE order by the two subsystems.

## 2. Writer x lock matrix

| Writer | Tx wrapper | Lock sequence (in order taken) | Consistent with A? | Notes |
|---|---|---|---|---|
| Staff single create (staff_create.go:47ff) | WithIdempotentTx absences-staff | idem -> student -> course -> advisory(limit) -> inserts | NO (student before course) | F1 |
| Staff tx create (staff_create_tx.go) | caller tx | student -> ... -> course -> ... | NO | F1 |
| Public create (routes.go:488ff) | WithIdempotentTx absences-public | idem -> student -> course -> advisory -> inserts | NO | F1 |
| Batch create (batch_routes.go:115ff) | ONE WithIdempotentTx for whole batch | per item: student -> course -> advisory (request order) | NO + G1 | F1; items NOT pre-sorted (Step-13 item 9 target) |
| Status update (management_routes.go:692ff) | WithIdempotentTx | version-guarded UPDATE (no SELECT lock) | n/a (single-row atomic) | OK pattern; see G4 |
| Notes update (management_routes.go:773ff) | WithIdempotentTx | version-guarded UPDATE | n/a | OK pattern |
| Sit-in reassign (management_routes.go:876ff) | WithIdempotentTx | ManagedAbsenceGet (unlocked read) -> version check -> ValidSitInSessionOverlap -> AbsenceSitInUpdate (version-guarded) -> AbsenceSitInsReplaceWithSnapshot (locks assignment rows FOR UPDATE, absence_management_custom.go:788) | PARTIAL | G3: no student lock, no course lock, no limit-advisory; session reads unfenced |
| Student cancel own (selfservice/service.go:94ff) | caller WithIdempotentTx | ManagedAbsenceGet (unlocked) -> conditional UPDATE ... WHERE status IN (...) | n/a (single-statement atomic + idempotent re-cancel) | OK; releasing limit needs no advisory |
| Student cancel route (self_service_routes.go:195ff) | WithIdempotentTx | -> CancelOwn | - | OK |
| Session edit occurrence (sessionshttp/routes.go + scheduling EditOccurrenceTimeTx, service.go:1164ff) | WithSerializableIdempotentTx | discover (unlocked) -> LockResources{courses} -> LockResources{students,teachers,rooms,sessions,series} in ONE call (global order) -> RE-READ + version/identity check (service.go:1223-1231) -> writes | YES | Step-9 conformant reference pattern |
| Series create/edit (series/service.go) | tx | LockResources{courses} then {...students/teachers/rooms...} | YES | two-phase but order-preserving |
| Merge-group create/delete (coursegroups/service.go:76,175) | tx | CourseMergeGroupLockCourses (ORDER BY c.id) -> group row FOR UPDATE | YES (course-only scope) | lock-re-read-retry present on delete (members re-read after lock) |
| Cross-study save/delete (crossstudy/store.go:625,953) | tx | LockResources{courses, students} single call | YES | full set pre-collected |
| Reconcile (reconcile.go:356,384,1119) | tx | courses, then students, separate calls in order | YES | - |
| Session-change resolution (session_change_resolution_candidate_custom.go:37) | tx | FOR UPDATE OF candidate + ExpectedSessionVersion check | YES (session scope) | fenced reference pattern |
| Legacy-sync schedule apply | tx | pg_advisory_xact_lock(legacy course key) -> row locks -> LockResources | YES | advisory-first consistent |
| SAT-verbal policy update (satverbalpolicyhttp/routes.go:86) | tx | AdvisoryLockForText("sat-verbal-policy:course-rules") | n/a | mutable-rule lock present |

## 3. Findings (numbered for Steps 9/10/13 tasking)

- F1 (DEADLOCK HAZARD, cross-subsystem): absence writers lock student->course;
  scheduling/session-edit writers lock course->student. Concurrent absence
  submission for student S in course C + session edit on C touching S's roster
  can AB-BA deadlock. Fix direction (Step 9 task): move the absence path's
  course lock (setAbsenceMergeGroupForCourse) BEFORE LockStudentForAbsenceSubmission
  so both subsystems take course->student. Queued behind Step 8 review sign-off;
  NOT implemented here.
- G1 (batch): one idempotency scope covers N items; per-item student locks are
  taken in request order, full set never pre-collected/sorted. Opposite-order
  batches can deadlock. Fix (Step 9/13): collect+sort all student/course IDs
  up front, lock once. Test: Step-13 item 9 (opposite-order batches).
- G2 (Step-9 fencing): SessionGetByIDForSnapshot (sessions.sql.go:310-324) takes
  NO row lock; AbsenceSitInsCreateWithSnapshot version-checks an unfenced read.
  A session edit can commit between the snapshot read and the assignment insert.
  Fix (Step 9): SELECT ... FOR UPDATE on the session row inside the submission tx
  (compatible with EditOccurrenceTimeTx which already locks the session row -
  the two writers WILL serialize once submission also locks it).
- G3 (Step-10): management reassign path takes no student lock, no course lock,
  no absence-limit advisory, and never calls projectedAbsenceDayStats - unlike
  all four create paths. Verify whether reassignment can overconsume days;
  close by routing it through the same protocol (Step 10 task + Step-13 items 3-4).
- G4 (documented OK): status/notes/cancel mutations use single-statement
  version-guarded UPDATEs (AbsenceStatusUpdate/AbsenceNotesUpdate/CancelOwn) -
  no SELECT FOR UPDATE needed; lost-update safe via version predicate + row-count
  check. Keep this pattern; do NOT add redundant row locks.
- OK (idempotency): Acquire is the first statement of every WithIdempotentTx
  (adapter.go: Bears out plan position 1). INSERT...ON CONFLICT DO UPDATE makes
  cross-replica key coordination database-backed (Step-12 input).

## 4. Adopted global order (proposed for review sign-off)

1. Idempotency ownership (Acquire, first statement).
2. Advisory xact locks with canonical keys (legacy-course keys, absence-limit
   keys, policy keys) - before row locks, after idempotency.
3. Course/scope rows, sorted by immutable ID (incl. FOR UPDATE OF c merge lock).
4. Student rows, sorted by immutable ID.
5. Teacher/room rows (scheduling scope), sorted.
6. Session rows, sorted (FOR UPDATE; add to snapshot reads per G2).
7. Series rows; dependent absence/assignment rows (FOR UPDATE on replace paths).
8. Merge-membership change: lock, RE-READ members, retry from discovery
   (merge delete already does this; extend to any path that discovers scope
   before locking - batch G1, submission sit-in discovery).

Batch rule: collect the FULL student+course set across items, sort, lock once
before processing item 1. Never acquire a new earlier-category lock mid-batch.

## 5. Review checklist (sign-off gates Step 9)

- [x] Confirm F1 fix direction (course-before-student in absence path) vs
      any writer that requires student-first (none found - confirmed 2026-09-06;
      implemented: course lock before student lock in staff_create.go:176,
      staff_create_tx.go:147, routes.go:633, batch_routes.go:491; post-create
      merge-group write switched to lock-free `setAbsenceMergeGroupIDOnly` so
      no course-after-student acquisition remains).
- [x] Confirm G3 is a real gap (reassign vs day limit) with a demonstration
      test before changing behavior. CLOSED 2026-09-06: reassign takes F1
      course+student locks plus session fence and post-fence recheck
      (management_routes.go handleSitInOverride); barrier demo test
      TestSubmission_ReassignVsSubmissionRaceOneWinner 3/3 PASS (exposed latent
      jsonb []byte bug, fixed at absence_management_custom.go:861); day-limit
      invariance pinned by TestReassignDoesNotChangeAbsenceDayCounts
      (absence_day_counts_integration_test.go) - reassign provably cannot change
      UsedAbsenceDays, so no projectedAbsenceDayStats call is needed.
- [x] Confirm batch full-set pre-locking does not break the single-tx
      idempotency scope (it nests inside; no new tx needed - implemented as
      `lockBatchCourseSet` inside the existing `WithIdempotentTx`,
      batch_routes.go:231; build+vet clean, evidence suite 10/10 PASS.)

Step-9 sign-off (2026-09-06): F1+G1+G2 implemented (fences in
absence_custom.go:431, absence_management_custom.go:795,1021 + post-fence
recheck `recheckSitInSessionsHeld` at all four writers; Replace path fences
replacement sessions BEFORE deleting old assignments). Evidence: build+vet
clean; 7/7 regression + shadow equivalence + concurrent reads + barrier race
`TestSubmission_SameStudentSameSessionRaceOneWinner` (1 winner + 1 conflict
+ exactly 1 live row) 3/3 PASS.

Step-10 sign-off (2026-09-06): G3 closed - reassign routed through F1 locks +
session fence + post-fence recheck; `TestSubmission_ReassignVsSubmissionRaceOneWinner`
3/3 PASS; `TestReassignDoesNotChangeAbsenceDayCounts` PASS (reassign cannot
change day counts by construction, projection call provably unnecessary).

Step-12 sign-off (2026-09-06): idempotency matrix pinned at both levels.
DB: same-key/same-payload replay, same-key/different-payload reuse detection,
concurrent single-winner, scope independence, stale delete/no-op
(idempotency_integration_test.go). HTTP: `TestIdempotencyMatrix_StaffCreate`
PASS - byte-identical replay, 409 reuse, fresh-key distinct-operation semantic,
late-replay stability.
