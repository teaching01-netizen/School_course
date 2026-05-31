---
phase: 02-code-review-command
reviewed: 2026-05-31T12:30:00Z
depth: standard
files_reviewed: 2
files_reviewed_list:
  - src/pages/AbsenceForm.tsx
  - src/pages/__tests__/AbsenceForm.test.tsx
findings:
  critical: 1
  warning: 2
  info: 3
  total: 6
status: issues_found
---

# Phase 2: Task 2 — Split Step 1 into Lookup + Verify (re-review after fixes)

**Reviewed:** 2026-05-31T12:30:00Z
**Depth:** standard
**Files Reviewed:** 2
**Status:** issues_found

## Summary

Re-review of `src/pages/AbsenceForm.tsx` and `src/pages/__tests__/AbsenceForm.test.tsx` after fix commit `ee8df91` applied 3 fixes from the previous TASK-2-REVIEW.md (WR-01: button text, WR-02: dead validateStepTwo, IN-01: misnamed function).

**Prior fixes verified:** All 3 are correctly applied in the current file. The step labels, navigation guards, and component structure are clean.

**New findings:** 1 **blocker** (session storage step mapping corrupts new snapshot restores — a latent data-loss bug), 2 warnings (nested state setter inside updater callback; unused prop in FormErrorSummary), 3 info items (missing typeahead cleanup, duplicate reset logic, missing test coverage).

## Critical Issues

### CR-01: Session storage stepMap corrupts new snapshot restores (data-loss risk)

**File:** `src/pages/AbsenceForm.tsx:481-482`
**Issue:** The step-migration map `{0→0, 1→2, 2→3, 3→3}` correctly remaps OLD snapshot steps (pre-split: Lookup+Verify=0, Courses=1, Sessions=2) to the new 4-step layout. But there's no version marker in the snapshot — the same map is applied on EVERY restore, including snapshots created AFTER the deployment with NEW step numbering.

Tracing the effect on new snapshots:

| Saved step (new) | Saved intent     | stepMap lookup | Restored to  | Expected      |
|------------------|------------------|----------------|--------------|---------------|
| 0                | Student Lookup   | `map[0] → 0`  | Step 0 ✅    | Step 0        |
| 1                | Parent Verify    | `map[1] → 2`  | Step 2 ❌    | Step 1        |
| 2                | Courses & Dates  | `map[2] → 3`  | Step 3 ❌    | Step 2        |
| 3                | Sessions & Cover | `map[3] → 3`  | Step 3 ✅    | Step 3        |

A user who reaches step 2 (Courses & Dates), closes the browser, and returns next day will land on step 3 (Sessions & Cover). Since sessions are fetched based on `dateFrom`/`dateTo` (which ARE restored from storage), the user might see session data but their course selection context is lost — they never validated their courses and dates on this session. The form appeared to "skip" a step.

Similarly, a user who verified their parent (step 1), closes and returns, lands on step 2 — their verification context is restored via `VERIFICATION_STORAGE_KEY` but the UX expects them at the verification confirmation page, not courses.

**Root cause:** `SESSION_STORAGE_KEY` (`"warwick-absence-form-state-v2"`) is the same key pre- and post-split. No `version` or `format` field in the snapshot to distinguish old numbering from new.

**Fix: Option A (safe) — Change storage key, discarding old snapshots:**

```ts
// Line 34
const SESSION_STORAGE_KEY = "warwick-absence-form-state-v3"; // ← bump version
```

Delete the stepMap entirely — new snapshots use identity mapping `n→n` via the same `parsed.step`. Old snapshots are silently discarded (user restarts from step 0), which is acceptable because:
- Session storage is ephemeral (tab-session-scoped)
- Old snapshots' verification tokens were already expired
- The step split materially changes which data is available at each step

**Option B (preserve old snapshots) — Store a format version in the snapshot:**

```ts
const snapshot = { _format: 2, step, lookup, ... }; // line 420
```

Then in the restore block:

```ts
const stepMap: Record<number, StepIndex> = { 0: 0, 1: 1, 2: 2, 3: 3 };
if (!("_format" in parsed)) {
  // old format — apply migration
  Object.assign(stepMap, { 1: 2, 2: 3 });
}
goTo((stepMap[parsed.step] ?? 0) as StepIndex);
```

Prefer **Option A** — it's simpler and `-v2` snapshots predate the split, so they're already stale. Bumping to `-v3` is the clearest signal.

## Warnings

### WR-01: Nested state setter inside state updater callback (fragile in concurrent mode)

**File:** `src/pages/AbsenceForm.tsx:581-585` and `613-617`

**Issue:** `setCoverSessionIds` is called inside the functional updater callback of `setSelectedSessionIds` in two places:

```tsx
// Line 581 (in handleSessionToggle)
setSelectedSessionIds((current) => {
  const next = new Set(current);
  if (next.has(sessionId)) {
    next.delete(sessionId);
    setCoverSessionIds((currentCovers) => {  // ← nested setter
      const nextCovers = new Set(currentCovers);
      nextCovers.delete(sessionId);
      return nextCovers;
    });
  } else {
    next.add(sessionId);
  }
  return next;
});
```

This pattern is repeated in `toggleAllSessionsForGroup` (line 613):

```tsx
setSelectedSessionIds((current) => {
  ...
  setCoverSessionIds(...)  // ← nested
  ...
});
```

React's `setState` callbacks are expected to be **pure functions** — they should compute and return the next state without side effects. Calling another state setter from inside one:

1. **Violates the pure-updater contract** — React's concurrent mode (StrictMode, Suspense, transitions) may replay updaters, causing `setCoverSessionIds` to be called multiple times with stale state.
2. **Creates implicit coupling** — if the component is refactored to use `useReducer`, these side-effectful updaters silently break.
3. **Race condition on cover state** — when rapidly toggling sessions off, `currentCovers` in the nested setter could be based on an intermediate render, not the final committed state.

While React 18 batches state updates within event handlers (making this work in practice), the pattern is not resilient to future React concurrency features.

**Fix:** Decouple the state updates by reading the latest `coverSessionIds` from a ref or applying both changes outside the callback:

```tsx
// In handleSessionToggle (line 576):
const handleSessionToggle = (sessionId: string) => {
  setSelectedSessionIds((current) => {
    const next = new Set(current);
    if (next.has(sessionId)) {
      next.delete(sessionId);
      // Mark session for cover cleanup via stored reference
      pendingCoverCleanupRef.current.add(sessionId);
    } else {
      next.add(sessionId);
    }
    return next;
  });
  // Clean up covers outside the updater
  if (pendingCoverCleanupRef.current.size > 0) {
    setCoverSessionIds((current) => {
      const next = new Set(current);
      for (const id of pendingCoverCleanupRef.current) next.delete(id);
      pendingCoverCleanupRef.current.clear();
      return next;
    });
  }
};
```

Or simpler — use `useReducer` for the session/cover pair so updates are atomic.

---

### WR-02: Unused `onGoToStep` prop in FormErrorSummary (dead interface surface)

**File:** `src/pages/AbsenceForm.tsx:117` and `939`

**Issue:** The `onGoToStep: (step: number) => void` prop is destructured as `_onGoToStep` (explicit underscore prefix signals intentional non-use) but is never referenced in the `FormErrorSummary` function body. The parent passes `onGoToStep={goTo}` (line 939), but the component only uses `onGoToVerification` to navigate to the verify step.

A maintainer reading the component interface will see `onGoToStep` and assume some error type navigates to an arbitrary step — but nothing does. This is dead API surface that misleads.

**Fix:** Remove the prop from both the call site and the interface:

```tsx
// Remove from interface (lines 116-117):
// onGoToStep: (step: number) => void;

// Remove from call site (line 939):
// onGoToStep={goTo}
```

If future requirements need generic step navigation from errors, reintroduce it with a concrete use case.

## Info

### IN-01: Missing typeahead timer cleanup on unmount

**File:** `src/pages/AbsenceForm.tsx:686-689`

**Issue:** The keyboard typeahead for course selection creates a `setTimeout` on `window`:

```tsx
typeaheadRef.current.timer = window.setTimeout(() => {
  typeaheadRef.current.buffer = "";
  typeaheadRef.current.timer = null;
}, 500);
```

If the component unmounts before the 500ms timer fires (e.g., user navigates away while typing), the callback runs on a stale ref. While no crash occurs (setting `buffer = ""` on a detached ref is harmless), it's an unreleased timer handle. In strict mode / dev double-mount, multiple timers can accumulate.

**Fix:** Clear the timer in a cleanup effect or in the `handleCourseKeyDown` exit path:

```tsx
useEffect(() => {
  return () => {
    if (typeaheadRef.current.timer !== null) {
      window.clearTimeout(typeaheadRef.current.timer);
    }
  };
}, []);
```

---

### IN-02: Duplicated form reset logic (`onWcodeChange` vs `handleReset`)

**File:** `src/pages/AbsenceForm.tsx:1083-1101` vs `791-817`

**Issue:** `onWcodeChange` (the callback passed to `StepCoverVerification` when the user wants to look up a different student) resets all form state — 17 state variables cleared, session storage removed, verification cleared. This is ~90% identical to `handleReset()`.

If a new state variable is added (e.g., `selectedSessionIds` was recently added), it must be cleared in both places. This has already been a maintenance issue — both blocks were independently updated.

**Fix:** Extract a shared `resetFormState()` function:

```tsx
const resetFormState = useCallback(() => {
  setLookupInput("");
  setLookup(null);
  setLookupError(null);
  setSelectedSubjectIds([]);
  setActiveCourseIndex(0);
  setDateFrom("");
  setDateTo("");
  setReasonCategory("");
  setReason("");
  setSessions([]);
  setSessionsError(null);
  setSelectedSessionIds(new Set());
  setCoverSessionIds(new Set());
  setPageError(null);
  setShowReasonFields(false);
  setCourseAnnouncement("");
  setVerificationSatisfied(false);
  setSubmissionError(null);
  setFinalResult(null);
  verification.clearStoredToken();
  verification.setCode("");
  try { window.sessionStorage.removeItem(SESSION_STORAGE_KEY); } catch { /* ignore */ }
}, [verification]);
```

Then both `onWcodeChange` and `handleReset` call `resetFormState()`, plus any unique actions (`goTo(0)`, `submissionIdempotencyKey.current = newIdempotencyKey()` for handleReset only).

---

### IN-03: Missing test coverage for back navigation and session storage restore

**File:** `src/pages/__tests__/AbsenceForm.test.tsx`

**Issue:** The test suite (299 lines, 6 tests) covers the happy-path forward flow and 4 edge cases. Two areas are untested:

1. **Back navigation at each step** — No test verifies that the back button from step 1 returns to step 0 (w/code input visible), or from step 2 back to step 1 (verification UI visible), or from step 3 back to step 2. The `back()` function in `useWizard` clamps at 0, but there's no guard against "going back too far" — the test suite doesn't confirm the behavior.

2. **Session storage restore with step mapping** — No test saves a snapshot to sessionStorage, reloads the component, and verifies the correct step is restored. This is where CR-01 (stepMap corruption) would be caught. The `beforeEach` clears sessionStorage, so the restore path is never exercised.

**Fix example — back navigation test:**

```tsx
it("can navigate back from step 1 to step 0", async () => {
  installHappyPathMocks();
  const user = userEvent.setup();
  renderWithProviders(<AbsenceForm />);
  await lookupStudent(user);
  await user.click(screen.getByRole("button", { name: /verify parent/i }));
  expect(screen.getByText("Parent Verify")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: /back/i }));
  expect(screen.getByText("Student Lookup")).toBeInTheDocument();
});
```

**Fix example — session storage restore test:**

```tsx
it("restores from session storage to the correct step", async () => {
  // Simulate a snapshot saved mid-flow
  const snapshot = {
    step: 2,  // new step 2 = Courses & Dates
    lookup: MOCK_STUDENT,
    lookupInput: "W250389",
    selectedSubjectIds: ["subj-1", "subj-2"],
    activeCourseIndex: 0,
    dateFrom: "2026-06-01",
    dateTo: "2026-06-07",
    reasonCategory: "",
    reason: "",
    selectedSessionIds: [],
    coverSessionIds: [],
  };
  window.sessionStorage.setItem("warwick-absence-form-state-v2", JSON.stringify(snapshot));
  installHappyPathMocks();
  renderWithProviders(<AbsenceForm />);
  await waitFor(() => {
    // Expect step 2 content, not step 3
    expect(screen.getByText("Select your courses")).toBeInTheDocument();
  });
});
```

---

## Prior Fix Verification

| # | Previous Finding | Status | Evidence in current code |
|---|---|---|---|
| WR-01 | "Go to Step 1" button text inconsistent with stepper numbering | ✅ **Fixed** | Button reads "Go to verification" (line 192), action-based text resolves the 1-index vs 0-index mismatch |
| WR-02 | `validateStepTwo` dead code — defined but never called | ✅ **Fixed** | `validateStepTwo()` at line 708 IS called at line 1295. Validates Courses & Dates before navigating to step 3 |
| IN-01 | `validateStepOne` function name misleading (validated step 2) | ✅ **Fixed** | Renamed to `validateStepTwo()` at line 708, reflects step 2 validation correctly |

---

_Reviewed: 2026-05-31T12:30:00Z_
_Reviewer: gsd-code-reviewer (adversarial re-review)_
_Depth: standard_
