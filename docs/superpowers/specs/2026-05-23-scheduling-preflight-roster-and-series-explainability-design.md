# Scheduling Preflight: Explicit Rosters + Series Conflict Explainability

Date: 2026-05-23

## Context

The scheduling system provides:

- Preflight endpoints for UX (`POST /api/v1/scheduling/preflight`, `POST /api/v1/scheduling/preflight_series`).
- DB-enforced integrity via exclusion constraints on `sessions` (room/teacher overlap) and `student_busy_ranges` (student overlap).
- Write endpoints that re-run preflight logic inside transactions, and on DB constraint rejection attempt to re-hydrate a stable `ConflictDetails` shape.

Two concrete edge cases cause correctness/UX issues:

1) **Explicit empty effective roster**: `effective = (course roster ∪ included) \ excluded` can be intentionally empty (0-student session allowed). Current code treats `len(StudentIDs)==0` as "use course roster", which can produce false positives/incorrect blocks.

2) **Series edit explainability on DB reject**: In some series write paths, a DB constraint rejection is explained by re-running preflight against the global pool (`s.db`) rather than the transaction that performed cancellations/changes, leading to confusing or incorrect conflict details (e.g., appearing to conflict with sessions that are being deleted within the transaction).

This spec defines the contract and implementation approach to address both.

## Decisions / Constraints

- 0-student sessions are allowed.
- `included_student_ids` may contain students not on the course roster.
- Preflight remains UX-only; DB is the final gate.
- Error responses must preserve the stable `ConflictDetails` schema (`kind`, `conflicts[]`, `requested`).

## Goals

- Make preflight roster semantics explicit: distinguish "no roster provided (use course roster)" vs "explicit roster (may be empty)".
- Ensure series write paths return conflict explanations consistent with the transaction's view when the DB rejects a write.
- Validate `included_student_ids` / `excluded_student_ids` exist as `students` to avoid confusing FK failures later.

## Non-goals (for this iteration)

- Returning a full list of all conflicting occurrences for long series operations (we may add an opt-in detailed mode later).
- Changing teacher/room NULLability. `teacher_id` remains required by schema; `room_id` remains required in DB but can be omitted at UX-level by using the "provisional" behavior already present.

## Design

### 1) Explicit roster semantics (single-session preflight)

**Problem:** `preflightSlot()` currently chooses between:

- `StudentIDs` overlap check when `len(in.StudentIDs) > 0`, else
- by-course roster overlap check when `len(in.StudentIDs) == 0`.

This makes an explicit empty effective roster indistinguishable from "not supplied".

**Change:** Make roster mode explicit in scheduling service APIs:

- In `scheduling.PreflightParams`, change `StudentIDs []pgtype.UUID` to `StudentIDs *[]pgtype.UUID`.
  - `nil` means "no explicit roster provided; use course roster".
  - non-nil means "use exactly this roster; may be empty".

Corresponding internal preflight input:

- In `preflightInput`, change `StudentIDs []pgtype.UUID` to `StudentIDs *[]pgtype.UUID` (or add a boolean flag like `RosterExplicit`).

**Behavior:**

- If `StudentIDs != nil`:
  - Run student overlap checks only when `len(*StudentIDs) > 0`.
  - If empty, skip student overlap checks and return success (availability/room/teacher checks still apply).
- If `StudentIDs == nil`:
  - Run by-course student overlap check (`overlappingSessionsByStudentsInCourse`), preserving current default behavior for callers that do not compute rosters.

### 2) Student ID existence validation (preflight HTTP)

**Problem:** `included_student_ids` / `excluded_student_ids` are parsed as UUIDs and used in effective roster computation without verifying the students exist.

**Change:** In `POST /api/v1/scheduling/preflight` handler:

- Collect all unique UUIDs from `included_student_ids` and `excluded_student_ids`.
- Query `students` for existence; if any are missing, return `400` with a stable code (e.g. `unknown_student_id`) and message.

**Notes:**

- We do not require membership in course roster (explicitly allowed).
- We do not block empty effective rosters (0-student sessions allowed).

### 3) Series write conflict explainability on DB rejection

**Problem:** Some write operations attempt to map DB constraint errors (e.g. exclusion constraint violations) back into `ConflictDetails` by re-running preflight, but do so against `s.db/s.q` which does not reflect in-transaction changes (notably series edits that soft-delete future sessions before re-creating new ones).

**Change:** Introduce a tx-aware "explain DB error by re-preflight" helper and use it in tx-based write paths:

- New helper signature (conceptually):
  - `explainFromDBErrByRepreflightTx(ctx, err, tx, qtx, candidates)` which uses `preflightSlot(ctx, tx, qtx, candidate)`
- Use this in tx write endpoints that can have transaction-local changes affecting conflicts, especially:
  - `EditEntireSeriesFutureOnlyTx`
  - `CreateSeriesAndMaterializeTx` (optional; less likely to diverge but consistent)
  - `SplitThisAndFutureTx` if it re-materializes/cancels in-tx (as applicable)

This ensures the conflict details correspond to what actually prevented commit at that point in time.

### 4) Series preflight contract (fail-fast)

Series preflight remains fail-fast:

- It iterates occurrences; on first blocking conflict it returns `409` with `ConflictDetails.requested.start_at/end_at` set to the failing occurrence.

This is acceptable for now; callers may iterate if they need multi-conflict reporting.

## Acceptance criteria

- **Explicit empty roster**: A course with students can be preflighted with all of them excluded (or no students included) and should not fall back to course-roster student overlap checks.
- **Included non-roster students**: Including a valid student not on the course roster results in that student being checked for overlaps.
- **Unknown student IDs**: Preflight returns `400` if any included/excluded IDs do not exist in `students`.
- **Series edit DB rejection explanation**: When a series edit fails due to conflicts, the returned `ConflictDetails.requested` corresponds to the failing occurrence and does not cite conflicts that only exist outside the transaction's view.

## Test plan

Add integration tests under `backend/internal/scheduling/`:

- `TestPreflight_ExplicitEmptyRosterDoesNotFallbackToCourse`
- `TestPreflight_IncludedNonRosterStudentChecked`
- `TestPreflight_UnknownStudentIDRejected`

Add an incident-memory entry to `docs/failure-cases.md` describing the explicit-empty-roster fallback bug and the regression tests added.

