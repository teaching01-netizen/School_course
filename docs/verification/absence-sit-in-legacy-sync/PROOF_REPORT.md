# Verification report

**Feature:** absence sit-in conflict parity and legacy schedule deletion safety  
**Implementation:** working tree on 2026-10-10  
**Lean:** Lean 4.19.0, commit `6caaee842e94`; core Lean/Std only  
**Local PostgreSQL:** `warwick_local_fresh` at `127.0.0.1:5432`; schema migrated from 129 to 130  
**Production database:** not accessed during implementation verification  

## 1. Result

The reported same-merge-group sit-in conflict is now applied at candidate generation, backend resolver, and form precheck boundaries. Submit-time database validation remains authoritative. Legacy schedule removal now requires a complete parsed observation, a newer shared database generation, two consecutive misses, and at least 24 hours since the first miss. The schema and affected applier/parser paths were exercised against the local PostgreSQL database.

This verifies the changed code in the local environment. It does not establish the current production deployment version, prove that every deployed writer uses the guard, or verify the source page for schedule `113373`.

## 2. Lean guarantees and limits

| Property | Model result | Limit |
|---|---|---|
| The old picker/form predicates allow the Oct 13 example while final validation rejects it. | `currentPickerAndFormMissTheConflict` proved; `#eval` returned `true`. | The model abstracts away the production resolver and date conversion. |
| Picker and form reject any strict overlap in the defined expected-session scope. | `candidatePickerRejectsAnyScopedOverlap` and `formPrecheckRejectsAnyScopedOverlap` proved. | Code correspondence is supported by tests and review, not generated from Lean. |
| One omission, duplicate generation, incomplete observation, or a second miss before grace cannot tombstone. | `oneCompleteMissingObservationDoesNotTombstone`, `duplicateGenerationDoesNotAdvanceMissingCount`, `incompleteObservationDoesNotAdvanceState`, and `twoMissingObservationsBeforeGraceDoNotTombstone` proved. | The model represents grace as a Boolean input; Go/PostgreSQL tests verify the configured 24-hour rule. |
| Two distinct complete misses after grace may tombstone; source presence clears missing state; stale absence cannot overwrite a newer presence. | `twoMissingObservationsAfterGraceMayTombstone`, `newerPresentObservationClearsMissingAndTombstone`, and `staleMissingObservationCannotUndoNewerPresent` proved. | The model does not prove source authenticity or the completeness of a live page. |

Lean proves the predicates and transitions in `proof/Model.lean`; it does not import or verify the production Go, TypeScript, SQL, network, or deployment code.

## 3. Commands and observed results

Run from `docs/verification/absence-sit-in-legacy-sync/`:

```text
lake build
Build completed successfully.

lake env lean proof/Model.lean
exit 0
```

The model printed `true`, `false`, `true`, `true` for the old counterexample, corrected server decision, old one-omission delete predicate, and proposed two-miss-plus-grace transition. All 12 named business theorems reported that they do not depend on any axioms.

Local PostgreSQL checks:

```text
go test ./internal/db -run '^(TestSitInCandidateSessionsAllowsAnyNonOverlappingDate|TestSitInSessionOverlapIgnoresCourse)$' -count=1
ok

go test ./internal/legacysync/apply -count=1
ok
```

The first local test run applied `00130_legacy_sync_observation_generations.sql`; the local schema reports version 130. The full applier package includes repeated-missing/grace, stale-generation, restoration, and concurrency coverage.

Other checks passed:

- `npx vitest run src/features/absences/domain/__tests__/submissionPayload.test.ts` — 27 tests passed.
- `npm run typecheck` — passed.
- `go test ./internal/legacysync/parser ./internal/httpapi/absenceshttp ./cmd/legacy-sync` — passed.
- `git diff --check` — passed.

## 4. Broader database-suite limitation

`go test -p 1 ./internal/db ./internal/legacysync/apply -count=1` was also attempted against the populated local app database. The changed sit-in tests passed, but unrelated tests in the broader `internal/db` package failed: a nickname assertion in `TestSitInsBySessionIDs`, cleanup blocked by existing `crm_cross_study_assignments` foreign keys in `TestActiveCoursesList_*`, several `TestCourseOverview_*` fixture/timeout failures, and an index-plan assertion selecting `sessions_active_teacher_range_idx`. The full `internal/legacysync/apply` package subsequently passed after fixing the one affected standalone-mapping path. These broad-suite failures are retained as environment/test-suite limitations; they are not represented as passes.

`npm run migrate:validate` also reports the pre-existing bare `CREATE TABLE` in migration `00129_course_link_dismissals.sql`; migration numbering reaches 130 and the new migration applied successfully through Goose on the local database.

## 5. Code-to-model correspondence

See [TRACEABILITY.md](TRACEABILITY.md). Candidate conflict scope is now shared through SQL/query-backed expected-session facts and resolver filtering; the form precheck retains attending sessions from the selected course instead of dropping the subject wholesale. Legacy sync records observation generations and missing-state transitions transactionally. Integration tests exercise the main PostgreSQL behaviors. The Lean proof remains an abstraction and is not a proof of production code.

## 6. Remaining production evidence

- Confirm the current source page format, including authenticated empty-page and populated-page variants, against the live legacy service.
- Confirm deployment schema version, all legacy-sync writers, and whether all instances share this database sequence and migration.
- Verify schedule `113373` against a fresh parsed source response, then use a normal targeted refresh after deployment and confirm its row and mapping recover. No production repair was made.
- Exercise direct forged-submit rollback and local-midnight/date-boundary cases if those are required as release gates.

**Verification verdict:** targeted local code, PostgreSQL, frontend, and Lean checks pass. **Production correctness and repair remain unverified.**
