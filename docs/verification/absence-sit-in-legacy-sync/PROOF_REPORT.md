# Verification report

**Implementation verification — 2026-10-11:** the eight subject-policy findings AP-1 through AP-8 in [POLICY_AUDIT.md](POLICY_AUDIT.md) are fixed in the current worktree and passed focused local regression checks. Lean proves the stated abstract model; Go, TypeScript, and PostgreSQL tests provide separate implementation evidence. Production behavior remains untested.

**Feature:** absence sit-in conflict parity and legacy schedule deletion safety  
**Implementation:** working tree on 2026-10-11
**Lean:** Lean 4.19.0, commit `6caaee842e94`; core Lean/Std only  
**Local PostgreSQL:** `warwick_local_fresh` at `127.0.0.1:5432`; schema migrated from 129 to 130  
**Production database:** not accessed during implementation verification  

## 1. Result

The current implementation applies authoritative expected-class blockers across policy result shapes and candidate dates; checks interval overlap across active and selected make-ups while preserving components of one merged target; recomputes allowed policy targets at submission; enforces automatic-resolution switches and generic final-day exclusions; and excludes the missed course from cross-section selection. Legacy schedule removal still requires a complete parsed observation, a newer shared database generation, two consecutive misses, and at least 24 hours since the first miss. The legacy-sync schema and applier/parser paths were exercised against local PostgreSQL.

These checks verify the changed paths in the local environment. They do not establish the current production deployment version, prove that every deployed writer uses the guard, or verify the source page for schedule `113373`.

## 2. Lean guarantees and limits

| Property | Model result | Limit |
|---|---|---|
| The old picker/form predicates allow the Oct 13 example while final validation rejects it. | `currentPickerAndFormMissTheConflict` proved; `#eval` returned `true`. | The model abstracts away the production resolver and date conversion. |
| Picker and form reject any strict overlap in the defined expected-session scope. | `candidatePickerRejectsAnyScopedOverlap` and `formPrecheckRejectsAnyScopedOverlap` proved. | Code correspondence is supported by tests and review, not generated from Lean. |
| After shared filtering, no selectable option overlaps a blocker, for arbitrary top-level, priority, per-missed-session, and nested-priority collections. | `sharedFilterMakesEveryPolicyAndPriorityChoiceSafe` proved for arbitrary lists; `sharedFilterRemovesConcreteOverlapAcrossPolicyAndPriorityPaths` checks a nonempty Oct 13 counterexample and preserves the adjacent priority option. | The model abstracts the Go result walker; unit and legacy-priority PostgreSQL integration tests provide implementation evidence. |
| Selected subjects do not hide overlapping expected sessions in the form picker. | `formRejectsOverlapRegardlessOfSelectedSubject` proved. | The model receives the complete expected-session set; frontend regressions check the actual picker helper. |
| Different overlapping make-up IDs conflict, with a narrow same-merge-group component allowance inside one absence. | `activeOverlappingAssignmentIsRejected`, `overlappingMergedComponentsInOneAbsenceAreOneChoice`, and `sameMergedGroupDoesNotHideOverlapAcrossAbsences` proved. | The model abstracts PostgreSQL bookings and merge membership; local mutation and policy integration tests exercise both paths. |
| Submitted session IDs belong to the server-authorized declared policy target. | `submittedSessionMustBelongToDeclaredAllowedTarget` proved. | The model uses an abstract target/session relation; local handler integration tests exercise the transaction path. |
| Automatic switches veto choices at global and narrower scopes; omission keeps the configured default. | `disabledPolicyMustOfferNoPhysicalChoice` and `scopedDisableVetoesAutomaticResolution` proved. | The model abstracts JSON setting resolution; local resolver tests exercise global/root/merge configuration. |
| Generic final-target exclusions and cross-section source exclusions are honored. | `genericFinalDayPredicateBlocksEveryFinalTarget` and `crossSectionChoosesEligibleSiblingAfterExcludingMissedCourse` proved. | Local resolver/DB tests check date and source identity handling. |
| One omission, duplicate generation, incomplete observation, or a second miss before grace cannot tombstone. | `oneCompleteMissingObservationDoesNotTombstone`, `duplicateGenerationDoesNotAdvanceMissingCount`, `incompleteObservationDoesNotAdvanceState`, and `twoMissingObservationsBeforeGraceDoNotTombstone` proved. | The model represents grace as a Boolean input; Go/PostgreSQL tests verify the configured 24-hour rule. |
| Two distinct complete misses after grace may tombstone; source presence clears missing state; stale absence cannot overwrite a newer presence. | `twoMissingObservationsAfterGraceMayTombstone`, `newerPresentObservationClearsMissingAndTombstone`, and `staleMissingObservationCannotUndoNewerPresent` proved. | The model does not prove source authenticity or the completeness of a live page. |

Lean proves the predicates and transitions in `proof/Model.lean`; it does not import or verify the production Go, TypeScript, SQL, network, or deployment code. `#print axioms` reports `[propext, Quot.sound]` for the generalized list-filter theorem; the concrete Oct 13 witness and other business theorems report no axioms. There is no `sorryAx` or custom axiom. These are Lean's standard foundational axioms, not proof of code correspondence.

## 3. Commands and observed results

Run from `docs/verification/absence-sit-in-legacy-sync/`:

```text
lake build
Build completed successfully.

lake env lean proof/Model.lean
exit 0
```

`lake build` compiled both proof modules, including all post-fix guard theorems. `#print axioms` reports no axioms for the new concrete guard theorems; the arbitrary list-filter theorem reports only Lean's standard `[propext, Quot.sound]`. No `sorryAx`, `native_decide`, or custom axiom is present. Lean checks the model, not the application code.

Local PostgreSQL checks:

The runner extracted `TEST_DATABASE_URL` from the project's `.env` and refused to run unless it parsed to `127.0.0.1/warwick_local_fresh`.

```text
go test ./internal/db -run '^(TestSitInCandidateSessionsAllowsAnyNonOverlappingDate|TestSitInSessionOverlapIgnoresCourse)$' -count=1
ok

go test ./internal/legacysync/apply -count=1
ok

go test ./internal/httpapi/absenceshttp -run 'TestFilterSitInResultByExpectedSessions|TestExpectedSessionOverlapFailsClosed' -count=1
ok

go test ./internal/httpapi/absenceshttp -run '^TestLegacyPrioritySitInFiltersExpectedSiblingSession$' -count=1 -v
ok (configured local TEST_DATABASE_URL; schema version 130)

go test ./internal/httpapi/absenceshttp ./internal/db -run '^(TestPolicyAudit.*|TestMutation_ConflictCheckKillsSkip|TestLegacyPrioritySitInFiltersExpectedSiblingSession|TestCourseContinuationSitInOffersBothIDs|TestAutomaticSitInEnabledForPoliciesHonorsGlobalAndScopedDisable|TestFinalClassPolicyMovesEverySessionOnFinalDayToUnavailable|TestFilterSitInResultByExpectedSessionsCoversEveryPolicyAndPriorityCollection|TestSitInCandidateSessionsAllowsAnyNonOverlappingDate)$' -count=1
absenceshttp passed; one internal/db test timed out during concurrent load

go test ./internal/db -run '^TestSitInCandidateSessionsAllowsAnyNonOverlappingDate$' -count=1 -v
ok (rerun alone)

go test ./internal/db -run '^TestResolveReassign_replacesAssignmentAndNotifies$' -count=1 -v
ok (student lock and reassign path)
```

The first local test run applied `00130_legacy_sync_observation_generations.sql`; the local schema reports version 130. The full applier package includes repeated-missing/grace, stale-generation, restoration, and concurrency coverage.

Current policy-fix regressions passed:

- `npm test -- src/features/absences/domain/__tests__/sitInResolution.test.ts src/features/absences/domain/__tests__/submissionPayload.test.ts` — 108 tests passed.
- `npm run typecheck` — passed.
- `go test ./internal/httpapi/absenceshttp ./internal/db -count=1` without `TEST_DATABASE_URL` — pure package tests passed; DB tests were skipped.

Other checks passed:

- `npx vitest run src/features/absences/domain/__tests__/submissionPayload.test.ts` — 27 tests passed.
- `npm run typecheck` — passed.
- `go test ./internal/legacysync/parser ./internal/httpapi/absenceshttp ./cmd/legacy-sync` — passed.
- `git diff --check` — passed.

The output-filter test exercises top-level options, priorities, per-missed-session options, and nested priorities. The local integration scenarios cover hidden and filtered blockers, next-day expected attendance, automatic switches, final target days, overlapping bookings, and policy target integrity. The legacy root-priority integration test confirms an overlapping sibling option is unavailable while an adjacent option remains selectable.

## 4. Broader database-suite limitation

`go test -p 1 ./internal/db ./internal/legacysync/apply -count=1` was also attempted against the populated local app database. The changed sit-in tests passed, but unrelated tests in the broader `internal/db` package failed: a nickname assertion in `TestSitInsBySessionIDs`, cleanup blocked by existing `crm_cross_study_assignments` foreign keys in `TestActiveCoursesList_*`, several `TestCourseOverview_*` fixture/timeout failures, and an index-plan assertion selecting `sessions_active_teacher_range_idx`. The full `internal/legacysync/apply` package subsequently passed after fixing the one affected standalone-mapping path. These broad-suite failures are retained as environment/test-suite limitations; they are not represented as passes.

`npm run migrate:validate` also reports the pre-existing bare `CREATE TABLE` in migration `00129_course_link_dismissals.sql`; migration numbering reaches 130 and the new migration applied successfully through Goose on the local database.

## 5. Code-to-model correspondence

See [TRACEABILITY.md](TRACEABILITY.md). Candidate blockers are loaded independently of presentation filters; legacy and V2 resolution filter their policy result shapes. The form picker checks selected-subject attendance. Submission recomputes policy eligibility, rejects overlapping booked intervals, honors generic final-day rules, and uses a student lock plus post-lock check. The Lean proof remains an abstraction and does not prove production code. The staff all-subjects lookup remains a manual availability view; submission validation still enforces conflicts.

## 6. Remaining production evidence

- Confirm the current source page format, including authenticated empty-page and populated-page variants, against the live legacy service.
- Confirm deployment schema version, all legacy-sync writers, and whether all instances share this database sequence and migration.
- Verify schedule `113373` against a fresh parsed source response, then use a normal targeted refresh after deployment and confirm its row and mapping recover. No production repair was made.
- Exercise direct forged-submit rollback and concurrent different-session / multi-item batch overlaps as release gates.
- Decide whether policy excludes the final missed class, the final target class, or both; this remains an open specification question.

**Verification verdict:** AP-1 through AP-8 are formally modeled and passed focused local Go, TypeScript, and PostgreSQL regressions. Lean does not certify application implementation. Production policy configuration, rollout, and live legacy-source behavior remain **NOT_TESTED**. The final-missed-class versus final-target-class specification remains unresolved.
