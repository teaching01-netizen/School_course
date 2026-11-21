# Legacy course refresh verification

## Specification

An accepted administrator refresh for a linked course must remain independently claimable when an automatic refresh is queued or running. Once claimed with fetching and applying enabled, it attempts to fetch the course even if the local course is archived or recently synced. A successful real apply can update Last Sync. Shadow mode and errors before any apply commit retain the prior timestamp. The leader sweep periodically compares frozen courses with the source directory. A linked course marked active or draft there must be eligible for background refresh, and a successful refresh must repair the local status even when the source payload is identical to its previous active snapshot. Archived or missing source entries provide no reactivation evidence. A legacy course metadata update must acquire schedule resource locks before a teacher foreign key can acquire a lock ahead of the student rows.

## Assumptions

- PostgreSQL owns queue uniqueness, transaction rollback, row locking, and the persisted sync timestamp. The code uses the existing partial unique index for open jobs. The lock argument covers the observed student and teacher inversion and presumes the competing writer requests resources in the canonical order.
- Progress requires a worker to claim the job, an available source with a parseable response, and a database that can commit the apply. The model assumes an available local link when the worker starts and abstracts retries and publication. It does not give a completion deadline or prove production throughput.
- The normalized directory status is the source signal for automatic reactivation. A status shown differently in another legacy view requires matching the course ID and checking which upstream record that view reads. A failed directory read leaves ordinary due-course selection available and defers reactivation to a later check.

## Proof

`proof/Refresh.lean` checks that manual keys cannot collide with any background course key, linked manual jobs pass the fetch gate, frozen background jobs skip without reactivation evidence, confirmed reactivations pass both automatic gates, and unlinked courses skip. It checks that modeled refusals, source failures, and shadow observations preserve Last Sync. It also checks that an eligible archived refresh with a newer applied observation can advance Last Sync, and that strictly increasing lock-rank paths cannot cycle.

## Traceability

| Property | Implementation boundary | Executable evidence |
| --- | --- | --- |
| Manual job survives an existing background job and duplicate requests share a job | Course detail and sync admin handlers, queue key, PostgreSQL enqueue conflict clause | `TestManualSync_QueuesSeparatelyFromRunningBackgroundRefresh`, `TestValidateRefreshRequest` |
| A claimed manual job applies an archived course and advances Last Sync, including when the old timestamp is within cooldown | `Runner.process`, `courseSyncer.syncCourse`, `CourseApplier.Apply`, `ScheduleApplier.Apply` | `TestCourseSyncer_ManualRefreshAppliesArchivedCourse` dispatches through the runner, fetches from a test legacy server, and reads persisted sessions and timestamps from PostgreSQL |
| Shadow stays timestamp-neutral and the background rules still apply | Same dispatch and apply paths | The archived refresh test repeats the observation in shadow mode. The syncer suite covers background archive, active, and cooldown cases. |
| A positive source status restores automatic syncing and repairs a stale local archive stamp even on the same hash | `courseSyncer.listSweepCourses`, `listLinkedLegacyCourses`, `courseSyncer.syncCourse`, `CourseApplier.fastPathIntegrityValid` | `TestCourseSyncer_SweepResumesReactivatedCourse` verifies archive and missing cases alongside recovery, reads PostgreSQL status, sessions and Last Sync, and checks the sweep interval. `TestCourseSyncer_SweepListsDueCoursesWhenReactivationCheckFails` exercises the directory-failure branch. |
| A course apply waiting for a student cannot already hold the proposed teacher from metadata writes | `CourseApplier.Apply`, `ScheduleApplier.lockScheduleResources` | `TestCourseApply_LocksStudentsBeforeTeacherForeignKeys` observes the real PostgreSQL wait, acquires the teacher in the competing transaction, then lets the course apply commit. Restoring the original order fails with `55P03`. |

## Checker results

- Baseline `8b6c74d1c5b2586809e4fb731269a2c82da79b85`. Lean `4.19.0` at `6caaee842e94`. `lake build` and `lake env lean proof/Refresh.lean` succeeded. The key-separation theorem uses `propext`. The other reported theorems have no axiom dependencies.
- Local tests used Go `1.26.3`, PostgreSQL `14.23`, and migrations through `00133`. Restoring the old archive guard produced a completed job with zero source fetches, zero imported sessions, and an unchanged timestamp. Restoring the old lock order reproduced the blocked teacher row while the apply waited on the student. Both counterexamples used scratch Go overlays.
- Reactivation tests rejected each old behavior independently. Restoring the frozen-course exclusion kept the reactivated ID out of the sweep. Removing the status check from the unchanged-snapshot path advanced Last Sync but left the course archived.
- With `TEST_DATABASE_URL` pointed to a disposable migrated database, `go test -p 1 ./cmd/legacy-sync ./internal/legacysync/... ./internal/jobqueue ./internal/httpapi/courseshttp ./internal/httpapi/legacysynchttp -count=1 -skip '^TestFullReconcile_(LinksCreatesAndRecordsConflicts|ParallelAndSerialConvergeSameCatalogue)$'` passed. The two excluded reconciliation tests also failed on the unmodified baseline because they expected open code-collision conflicts that the implementation marked ignored.
- Formal status **PROVED (scoped)**. Correspondence **REVIEWED_WITH_TESTS**. Production evidence **PARTIAL** for the incident and **NOT_TESTED** for the candidate deployment.

## Production verification

The original screenshot matches legacy detail `7291` by course name and all eight session dates ending October 8, 2026. Its source course code is `26362513009`. The configured legacy endpoint returned `Archive` for `7291` in its detail metadata and its directory classified the row as archived. The second screenshot names course `7290`, whose source schedule ends October 9. The legacy source returned `7290` as archived even in a targeted search with the archive filter off. Its screenshot shows `completed` while the course still shows October 9 as Last Sync. Railway production logs for the baseline also showed `40P01` refresh failures, including a cycle between `StudentsLockOrdered` and `UsersLockOrdered`.

After deployment, queue a refresh for `7290` or `7291` and confirm the job completes and the persisted Last Sync advances. Compare its schedule to the source. Verify that a linked course the source has reactivated resumes automatic refresh without a manual job. Check subsequent sync-worker logs for the observed student and teacher lock cycle during ordinary parallel processing. No production job, configuration, or deployment was changed during verification.

## Remediation

Keep manual and background refresh requests distinct in the existing queue. Recheck frozen courses against the source directory on the usual refresh interval. Acquire the existing schedule resource locks before writing teacher foreign keys. Validate the fixes against the deployed worker and live source before declaring recovery.
