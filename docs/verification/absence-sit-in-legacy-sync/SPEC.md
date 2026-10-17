# Verification specification

**Spec revision:** 1  
**Application revision inspected:** `31733170966de2978484f2e0dbd0d1f5853c22f4`  
**Scope:** public absence make-up eligibility and legacy schedule soft deletion.

## Exact requirements

- **REQ-SI-1 (safety):** A selectable candidate must be active, belong to a server-recomputed allowed policy target, and not overlap any regular class the student is expected to attend at the candidate's make-up time. This attendance check is independent of display, course-allow-list, notice-time, and absence-window filters. Intervals are half-open and institute-local date rules are applied consistently.
- **REQ-SI-2 (safety):** A candidate must not overlap a sit-in already assigned to the same student. Distinct session IDs within one absence may overlap only when they are members of the same non-null merge group and represent one merged target; that exception does not apply across separate absences.
- **REQ-SI-3 (integrity):** An invalid, inactive, policy-ineligible, or conflicting sit-in candidate is rejected inside the same transaction that creates or updates the absence, so no partial absence/sit-in result commits.
- **REQ-SI-4 (policy):** Automatic-resolution settings at global, subject, root, and merge scopes are enforced at the resolution boundary. Explicit disable vetoes automatic choices; omitted settings keep the established enabled default. Generic final-target exclusions and cross-section source-course exclusions are enforced by both resolution and submission validation.
- **REQ-LS-1 (safety):** A legacy schedule is not soft-deleted from one missing ID alone. Deletion requires a complete, authenticated, parser-valid, monotonically newer source observation, two consecutive missing observations, and the configured grace period.
- **REQ-LS-2 (recovery):** A newer qualifying source-present observation clears missing progress and restores the local session and mapping.
- **REQ-LS-3 (ordering):** A source observation older than the last successfully applied observation cannot overwrite newer state, even if its HTTP request finishes later.

## Initial conditions and observable decisions

- The absence example has selected sessions on Oct 10 and Oct 17, 2026, making the server date window Oct 10–17. The candidate is Tue Oct 13, 13:00–16:20. Two expected sessions in the same merge scope cover 13:00–14:40 and 14:40–16:20.
- The source/local disagreement concerns legacy schedule ID `113373`. User reports it remains available in the legacy source; the database audit previously showed the local row soft-deleted by `legacy_sync` and its external mapping tombstoned.
- The historical fetched page for the tombstoning observation is unavailable in the reviewed evidence. The model does not claim which source/parser/concurrency failure occurred.

## Lean properties

- `currentPickerAndFormMissTheConflict`: the modeled current candidate predicate and current form precheck both allow the Oct 13 candidate, while the final server predicate rejects it.
- `sharedPolicyRejectsAnyScopedOverlap`: under the exact server blocker set, every strict overlap is rejected.
- `oneOmissionCanDeleteInCurrentRule`: the current set-difference condition treats a missing ID in one aggregate as a deletion candidate.
- `oneCompleteMissingObservationDoesNotTombstone`: the proposed observation model keeps the session active after one complete omission.
- `incompleteObservationDoesNotAdvanceState`: an incomplete observation does not advance missing state.
- `twoMissingObservationsAfterGraceMayTombstone`: two valid misses plus the grace condition can tombstone.
- `newerPresentObservationClearsMissingAndTombstone`: source presence restores the modeled state.
- `staleMissingObservationCannotUndoNewerPresent`: a lower generation cannot override a newer present observation.

## Explicit exclusions

Lean models integer-minute intervals and a small per-schedule state machine. It does not prove the Go/TypeScript/SQL implementation, timezone conversion, that a parsed response is truly complete, PostgreSQL transaction behavior, source-site behavior, browser rendering, multi-instance deployment, or latency/capacity.

## Revision 2 — broader subject-policy audit

The requirement is now evaluated against all 26 checked-in leave-policy definitions (54 priorities), five generic backend rule types, and both policy-resolution paths. The local database has fixture subjects and no active mapped policies; deployed subject configuration remains unknown. In addition to the original absence-scope guard, choices must pass the global regular-class check that the actual submission validator enforces on their make-up dates. Display filters must not remove attendance blockers. Distinct make-up IDs must not create overlapping bookings. Selected sessions must belong to the declared and policy-eligible target. Automatic-resolution switches, generic final-day exclusions, and cross-section sibling identity must be respected. AP-1 through AP-8 in `POLICY_AUDIT.md` bind these requirements to checked counterexamples and implementation tests.

This revision does not decide whether final *missed* classes or only final *target* classes are forbidden; repository preview and runtime currently implement different meanings. The priority-level gaps in Rank 3 section 2/3 are deliberate, not prohibited initial states.

## Revision 3 — post-fix verification

AP-1 through AP-8 are implemented in the current worktree and have focused local regression coverage. The updated Lean file proves the abstract guards and post-fix examples; local Go, TypeScript, and PostgreSQL checks provide separate correspondence evidence. These results do not establish deployed production behavior. The final-missed-class versus final-target-class meaning remains unresolved, as do the legacy-source and production rollout checks in `PRODUCTION_TEST_PLAN.md`.
