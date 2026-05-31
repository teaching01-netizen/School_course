---
phase: absence-form-review
reviewed: 2026-05-31T00:00:00Z
depth: standard
files_reviewed: 2
files_reviewed_list:
  - src/pages/AbsenceForm.tsx
  - src/components/absences/DateRangePicker.tsx
findings:
  critical: 0
  warning: 3
  info: 4
status: issues_found
---

# Phase: AbsenceForm DateRange Integration — Code Review Report

**Reviewed:** 2026-05-31
**Depth:** standard
**Files Reviewed:** 2
**Status:** issues_found

## Summary

Reviewed AbsenceForm.tsx's integration of DateRangePicker — replacing `dateFrom`/`dateTo` string state with `dateRanges: DateRange[]` state. Derived `dateFrom`/`dateTo` min/max values are computed correctly. Session storage serialization/deserialization handles Date objects properly with legacy fallback. The integration is solid overall, but the validation layer has a gap: `validateStepTwo` omits the overlap check that DateRangePicker performs internally, and uses a different day-counting method that diverges on DST boundaries.

---

## Warnings

### WR-01: Validation gap — `validateStepTwo` misses overlapping range check

**File:** `src/pages/AbsenceForm.tsx:714-746`
**Issue:** `validateStepTwo()` checks that each range has `from ≤ to` and that total days ≤ `max_date_range_days`, but does **not** check for overlapping ranges. The `DateRangePicker` component runs `validateRanges()` which includes an overlap check (DateRangePicker.tsx:40-46) and displays the error, but the error is **informational only** — it doesn't block the parent's "Continue to sessions" button.

A user (or stale sessionStorage restore) could end up with overlapping ranges. `validateStepTwo` would pass, and the submission payload would be sent with a dateFrom/dateTo that spans overlapping ranges. The backend may reject, but the frontend validation is inconsistent with its own DateRangePicker contract.

**Fix:** Add overlap detection to `validateStepTwo`:
```ts
function validateStepTwo() {
  // ... existing checks ...

  // Add overlap check after from ≤ to loop
  for (let i = 0; i < validRanges.length; i++) {
    for (let j = i + 1; j < validRanges.length; j++) {
      const a = validRanges[i];
      const b = validRanges[j];
      if (a.from! <= b.to! && b.from! <= a.to!) {
        setPageError(`Date ranges ${i + 1} and ${j + 1} overlap.`);
        return false;
      }
    }
  }
  // ... rest of validation ...
}
```

---

### WR-02: `totalDays` calculation diverges from DateRangePicker on DST boundaries

**File:** `src/pages/AbsenceForm.tsx:733-735`
**Issue:** `validateStepTwo` calculates total days via:
```ts
Math.round((r.to!.getTime() - r.from!.getTime()) / (1000 * 60 * 60 * 24)) + 1
```
This assumes every day is exactly 24 hours (86,400,000 ms). On DST transition days, a day can be 23 or 25 hours. `DateRangePicker` uses `differenceInDays(r.to, r.from) + 1` from `date-fns`, which compares calendar dates correctly.

This means `validateStepTwo` could report a different total than `DateRangePicker` around DST boundaries — the user might see a validation error in the page error banner while DateRangePicker shows no error, or vice versa.

The same issue exists in the `daysBetween` helper (line 67-72), though that function is only used for session-fetch gating where the 24h assumption is acceptable (it just needs a rough date range).

**Fix:** Import and use `differenceInDays` from `date-fns`:
```ts
import { differenceInDays } from "date-fns";

// In validateStepTwo:
const totalDays = validRanges.reduce((sum, r) => {
  return sum + differenceInDays(r.to!, r.from!) + 1;
}, 0);
```

---

### WR-03: `onGoToVerification` in useMemo deps but unused inside memo body

**File:** `src/pages/AbsenceForm.tsx:166`
**Issue:** The `items` useMemo in `FormErrorSummary` includes `onGoToVerification` in its dependency array (line 166), but the function is never referenced inside the memoized computation (lines 132-165). It's only used in the JSX render below the memo (line 189). This causes the memo to recompute unnecessarily when the callback reference changes (which happens on every parent render since it's not wrapped in `useCallback`).

**Fix:** Remove `onGoToVerification` from the dependency array:
```ts
}, [pageError, submissionError, verificationBlocked, lookupError, sessionsError, lookup, online, justRestored, onClearPageError, onClearSubmissionError]);
```
The `onGoToVerification` callback is still accessible in the JSX closure — the memo only controls the `items` array computation, not the render output.

---

## Info

### IN-01: Duplicate validation between `validateStepTwo` and DateRangePicker

**File:** `src/pages/AbsenceForm.tsx:714-746` / `src/components/absences/DateRangePicker.tsx:28-60`
**Issue:** Both `validateStepTwo()` and `DateRangePicker`'s internal `validateRanges()` check `from > to` and total days exceed `max_date_range_days`. This is defense-in-depth, which is acceptable, but the two implementations use different formulas (see WR-02) and different error messages. If a future change updates one, the other may drift.

**Fix:** Consider exporting `validateRanges` from DateRangePicker or extracting the shared validation into a utility that both call. Low priority — current duplication is manageable for a single-file component.

---

### IN-02: `verificationSatisfied` serialized to session storage but never restored

**File:** `src/pages/AbsenceForm.tsx:446, 450-494`
**Issue:** The save effect (line 446) includes `verificationSatisfied` in its dependency array, and it's included in the snapshot object (though not explicitly — it's just not destructured out). Actually, reviewing more carefully: `verificationSatisfied` is listed in the dependency array but is **not** part of the snapshot object (lines 417-431 don't include it). The dependency inclusion just forces unnecessary re-serialization. The restore effect never reads it.

**Fix:** Remove `verificationSatisfied` from the save effect's dependency array — it doesn't affect the serialized output:
```ts
}, [
  lookup,
  lookupInput,
  selectedSubjectIds,
  activeCourseIndex,
  dateRanges,
  reason,
  selectedSessionIds,
  coverSessionIds,
  step,
]);
```

---

### IN-03: Session storage restore doesn't validate Date object validity

**File:** `src/pages/AbsenceForm.tsx:472-476`
**Issue:** When restoring dates from session storage, `new Date(r.from)` is called on the stored ISO string. If the string is malformed (e.g., corrupted storage), `new Date(...)` returns an `Invalid Date` object without throwing. This would propagate silently — `dateFrom`/`dateTo` derivation would produce `"Invalid Date"` strings (since `.toISOString()` throws on Invalid Date, but `.getTime()` returns NaN, so the sort would produce NaN comparisons).

In practice this is very unlikely (requires corrupted sessionStorage), but a defensive check would be more robust:
```ts
from: r.from ? (() => { const d = new Date(r.from); return isNaN(d.getTime()) ? undefined : d; })() : undefined,
```

**Fix:** Low priority — consider adding a validity check during restore. The current behavior degrades gracefully (user sees empty dates and must re-select).

---

### IN-04: `canProceedToSessions` doesn't gate on reason — minor UX inconsistency

**File:** `src/pages/AbsenceForm.tsx:341-344`
**Issue:** `canProceedToSessions` controls whether the "Continue to sessions" button is enabled. It checks `activeGroup`, `selectedSubjectCount > 0`, and `dateRanges.some(r => r.from && r.to)` — but not `reason`. The reason is only validated when the button is clicked (via `validateStepTwo` at line 1244). This means the button appears enabled even when the user hasn't entered a required reason, and they only discover the requirement after clicking.

This isn't a bug (the click handler catches it), but it's inconsistent with the other disabled-state checks. The user experience would be smoother if the button were disabled until reason is filled.

**Fix:** Add reason check to `canProceedToSessions`:
```ts
const canProceedToSessions =
  !!activeGroup &&
  selectedSubjectCount > 0 &&
  dateRanges.some(r => r.from && r.to) &&
  reason.trim().length > 0 &&
  !verificationBlocked;
```
(Note: if `config.form.require_reason` is false and an empty reason is acceptable, this should be conditional on that config value.)

---

## Assessment

The DateRange integration is well-structured. Strengths include:
- Correct min/max date derivation from multiple ranges
- Proper Date ↔ ISO string serialization for session storage
- Clean legacy fallback for old `dateFrom`/`dateTo` format
- AbortController cleanup for session fetch
- Defensive try/catch around all sessionStorage operations

The three warnings are all **validation-layer consistency** issues — the frontend's two validation paths (DateRangePicker internal + `validateStepTwo`) have drifted apart in capability and implementation. No critical security or data-loss issues found. The issues are fixable without architectural changes.

---

_Reviewed: 2026-05-31_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
