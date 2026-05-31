---
phase: 02-code-review-command
reviewed: 2026-05-31T00:00:00Z
depth: standard
files_reviewed: 4
files_reviewed_list:
  - src/pages/AbsenceForm.tsx
  - src/components/absences/DateRangePicker.tsx
  - src/components/absences/DateRangeSlot.tsx
  - src/pages/__tests__/AbsenceForm.test.tsx
findings:
  critical: 1
  warning: 1
  info: 1
  total: 3
status: issues_found
---

# Phase 02: Code Review Report

**Reviewed:** 2026-05-31T00:00:00Z
**Depth:** standard
**Files Reviewed:** 4
**Status:** issues_found

## Summary

Wire DateRangePicker into AbsenceForm.tsx: replace old DateRangeInput, add `dateRanges` DateRange[] state, derive `dateFrom`/`dateTo`, update validation/session-storage/payload/reset. The component wiring and state management are structurally correct. One critical timezone bug in the date-string derivation would produce wrong dates for all users in the target timezone (Asia/Bangkok, UTC+7).

## Spec Compliance Checklist

| # | Spec Requirement | Status |
|---|---|---|
| 1 | Replace DateRangeInput import with DateRangePicker | ✅ Line 13 |
| 2 | Replace dateFrom/dateTo string state with dateRanges DateRange[] state | ✅ Lines 295-297 |
| 3 | Derive dateFrom/dateTo from dateRanges for session fetching | ✅ Lines 324-334 |
| 4 | Replace DateRangeInput JSX with DateRangePicker JSX | ✅ Lines 1199-1203 |
| 5 | Update validation in validateStepTwo() | ✅ Lines 714-746 |
| 6 | Update canProceedToSessions | ✅ Lines 340-344 |
| 7 | Update session storage save/restore | ✅ Lines 416-494 |
| 8 | Update buildSubmissionPayload() | ✅ Lines 748-762 |

## Critical Issues

### CR-01: `dateFrom`/`dateTo` derivation uses UTC conversion — dates shift back one day in positive-UTC timezones

**File:** `src/pages/AbsenceForm.tsx:327,333`
**Issue:** The derived `dateFrom` and `dateTo` use `.toISOString().slice(0, 10)` to convert Date objects to `yyyy-MM-dd` strings. `toISOString()` always returns UTC. When the browser is in Asia/Bangkok (UTC+7), a user selecting June 15 creates a Date at local midnight `2026-06-15T00:00:00+07:00`, which is `2026-06-14T17:00:00Z`. Calling `.toISOString().slice(0, 10)` yields `"2026-06-14"` — **off by one day**. This breaks:
  - Session fetching (wrong `date_from`/`date_to` query params → line 395)
  - Submission payload (`date_from`/`date_to` in POST body → lines 754-755)
  - Display in session review (line 1387)

The test mock (line 39, 55 of test file) masks this because jsdom runs in UTC.

**Fix:** Extract local calendar date components instead of using UTC conversion:

```typescript
const dateFrom = useMemo(() => {
  const validFroms = dateRanges.filter(r => r.from).map(r => r.from!);
  if (validFroms.length === 0) return "";
  const earliest = validFroms.sort((a, b) => a.getTime() - b.getTime())[0];
  const y = earliest.getFullYear();
  const m = String(earliest.getMonth() + 1).padStart(2, "0");
  const d = String(earliest.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}, [dateRanges]);

const dateTo = useMemo(() => {
  const validTos = dateRanges.filter(r => r.to).map(r => r.to!);
  if (validTos.length === 0) return "";
  const latest = validTos.sort((a, b) => b.getTime() - a.getTime())[0];
  const y = latest.getFullYear();
  const m = String(latest.getMonth() + 1).padStart(2, "0");
  const d = String(latest.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}, [dateRanges]);
```

Alternatively, use `format(date, 'yyyy-MM-dd')` from `date-fns` (already a project dependency, used in `DateRangePicker.tsx` and `DateRangeSlot.tsx`).

## Warnings

### WR-01: `verificationSatisfied` in save-effect dependency array but not in snapshot

**File:** `src/pages/AbsenceForm.tsx:446`
**Issue:** `verificationSatisfied` is listed in the session-storage save `useEffect` dependency array (line 446) but is not included in the snapshot object written to storage (lines 417-431). Every change to `verificationSatisfied` (OTP send, verify, reset) triggers an unnecessary sessionStorage write that produces the same output. This is wasteful I/O and makes the dependency array misleading. This is a pre-existing issue not introduced by this change, but it was carried through during the refactor.

**Fix:** Remove `verificationSatisfied` from the dependency array on line 446, or include it in the snapshot if it needs to be restored.

## Info

### IN-01: Duplicate validation between DateRangePicker internal validation and validateStepTwo()

**File:** `src/pages/AbsenceForm.tsx:714-746`
**Issue:** `validateStepTwo()` re-validates `from ≤ to` ordering and total-day limits that `DateRangePicker` already validates internally (via its `validateRanges` helper). This is belt-and-suspenders, which is fine for a form wizard, but worth noting that DateRangePicker displays its own errors visually while `validateStepTwo` blocks the wizard step. If DateRangePicker shows a validation error (e.g., overlap), the "Continue to sessions" button remains enabled (gated only by `canProceedToSessions` which doesn't check these constraints). The user could click through and see a `pageError` toast instead of relying on the inline error.

**Fix:** No action required — this is acceptable redundancy. If you want to tighten the UX, consider disabling the continue button when DateRangePicker's internal validation fails (pass the error back via a callback or use the `error` prop).

---

_Reviewed: 2026-05-31T00:00:00Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
