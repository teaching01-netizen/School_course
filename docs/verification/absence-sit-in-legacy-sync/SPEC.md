# Verification specification

**Spec revision:** 1  
**Application revision inspected:** `31733170966de2978484f2e0dbd0d1f5853c22f4`  
**Scope:** public absence make-up eligibility and legacy schedule soft deletion.

## Exact requirements

- **REQ-SI-1 (safety):** A selectable candidate must be active and must not overlap any session the student is expected to attend in the absence course or its merge group, for dates in the inclusive institute-local absence window. Intervals are half-open. The client and candidate list may be stale; the server check at submission is authoritative.
- **REQ-SI-2 (safety):** An invalid or inactive sit-in candidate is rejected inside the same transaction that creates the absence, so no partial absence/sit-in result commits.
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
