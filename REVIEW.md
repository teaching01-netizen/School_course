---
phase: task-2-absence-inbox-review
reviewed: 2026-06-08T12:00:00Z
depth: standard
files_reviewed: 1
files_reviewed_list:
  - src/pages/Absences.tsx
findings:
  critical: 1
  warning: 3
  info: 3
  total: 7
status: issues_found
---

# Task 2: Code Review Report — Absence Inbox UI Refresh

**Reviewed:** 2026-06-08T12:00:00Z
**Depth:** standard
**Files Reviewed:** 1
**Status:** issues_found

## Summary

Reviewed `src/pages/Absences.tsx` (+25/−20) for the Absence Inbox UI refresh (commit `c790b35`). The 10 changes cover column reduction, avatar initials, hover-reveal actions, Export CSV button relocation, and an animated selection bar.

**Overall assessment:** The changes are mostly cosmetic and do not alter the core data-fetching, state management, or API interaction logic. However, one regression was introduced to the filter bar grid layout, and the animated selection bar has an accessibility concern that was not present with the original conditional-rendering approach. Several pre-existing patterns (duplicate utility function, missing ARIA on decorative elements) persist.

---

## Critical Issues

### CR-01: Filter bar grid has mismatched column count — "To date" wraps to a second row alone

**File:** `src/pages/Absences.tsx:371`
**Issue:** The table-view filter grid was changed from 6 columns (`[minmax(200px,2fr) 1fr 1fr 1fr 1fr auto]`) to 4 columns (`[minmax(200px,2fr) 1fr 1fr 1fr]`) when Export CSV was moved out. However, the grid still has **5 children** (Search, Subject, Status, From date, To date), while the template defines only **4 columns**.

The kanban-view grid (line 314) correctly uses 4 columns for 4 children (it lacks the Status dropdown). The table view has 5 children but was incorrectly reduced to 4 columns to match.

**Result:** CSS Grid's auto-placement puts the 5th child ("To date") on a second row in column 1, creating a broken layout — a single date input isolated at ~25% width on its own row.

**Fix:** Change the grid to 5 columns:
```tsx
-        <div className="grid gap-3 md:grid-cols-[minmax(200px,2fr)_1fr_1fr_1fr]">
+        <div className="grid gap-3 md:grid-cols-[minmax(200px,2fr)_1fr_1fr_1fr_1fr]">
```

---

## Warnings

### WR-01: Focusable hidden elements inside collapsed selection bar (keyboard a11y)

**File:** `src/pages/Absences.tsx:389-402`
**Issue:** The selection bar now uses `max-h-0` with `overflow-hidden` to hide when no items are selected. Unlike the previous conditional-rendering approach (`{selected.size > 0 ? ... : null}`), the DOM elements remain present and are **focusable via keyboard** (Tab) even when visually hidden. A user tabbing through the page when nothing is selected will encounter hidden "Mark Reviewed", "Export Selected", and "Cancel Selected" buttons.

This also affects screen readers — the buttons are announced even though they are not visible and not actionable (no items selected).

**Fix (option A):** Add `inert` attribute on the outer `<div>` when collapsed:
```tsx
<div
  className={`overflow-hidden transition-all duration-300 ease-in-out ${selected.size > 0 ? 'max-h-16 mb-3' : 'max-h-0'}`}
  inert={selected.size === 0 ? true : undefined}
>
```

**Fix (option B, simpler):** Restore conditional rendering for the outer wrapper and only animate when transitioning:
```tsx
{selected.size > 0 || batchFailed.length > 0 ? (
  <div className={`overflow-hidden transition-all duration-300 ease-in-out ${selected.size > 0 ? 'max-h-16 mb-3' : 'max-h-0'}`}>
    ...
  </div>
) : null}
```

### WR-02: `max-h-16` animation timing mismatch — content height is less than max-height

**File:** `src/pages/Absences.tsx:389`
**Issue:** `max-h-16` (4rem / 64px) is the animation target, but the actual content height of the selection bar is ~40–48px (a 32px-high bar with 8px vertical padding). Because CSS `max-height` transitions animate toward the `max-height` value, not the content height, the visual animation completes in ~75% of the 300ms duration, then stalls for the remaining ~75ms. This creates a perceptible "pause" at the final state.

**Fix:** Set `max-h` closer to the actual content height, or use a fragment-collapse pattern:
```tsx
- max-h-16
+ max-h-12
```
(`max-h-12` = 3rem / 48px, closer to the actual content height.)

### WR-03: Pre-existing `!important` on `absence-inbox-table` creates specificity trap

**File:** `src/index.css:332`
**Issue:** The addition of the `absence-inbox-table` class to `<table>` (line 421) now activates a CSS rule that uses `!important` unnecessarily:
```css
.absence-inbox-table { min-width: 0 !important; width: 100%; }
```
No competing rule sets `min-width` on the table. The `!important` creates a specificity trap: any future override must also use `!important` or higher specificity. This was already flagged in the prior Task 1 review but remains unaddressed.

**Fix:** Remove `!important`:
```css
.absence-inbox-table { min-width: 0; width: 100%; }
```

---

## Info

### IN-01: Avatar `<span>` lacks `aria-hidden="true"` (decorative image)

**File:** `src/pages/Absences.tsx:450`
**Issue:** The initials avatar is purely decorative — the student name is already rendered as a `<Link>` immediately to its right. But the `<span>` has no `aria-hidden="true"`, so screen readers may announce the initials text redundantly. This matches the existing pattern in `KanbanView.tsx` (lines 56, 233), but the better pattern from `PreflightIndicator.tsx` (`aria-hidden="true"` on decorative indicators) is available in the same codebase.

**Suggestion:**
```tsx
<span aria-hidden="true" className="flex h-7 w-7 ...">{initials(...)}</span>
```

### IN-02: `exportCsv` async callback lacks `void` prefix (inconsistent with peers)

**File:** `src/pages/Absences.tsx:365`
**Issue:** `onClick={exportCsv}` does not use the `void` operator, unlike other async handlers in the same component (e.g., `onClick={() => void markSelectedReviewed()}` at line 392, `onClick={() => void retryFailed()}` at line 416). While React handles the returned promise gracefully, the inconsistency suggests a copy-paste oversight.

**Suggestion:**
```tsx
- <Button variant="secondary" onClick={exportCsv}>
+ <Button variant="secondary" onClick={() => void exportCsv()}>
```

### IN-03: `initials()` utility function duplicated across two files

**Files:** `src/pages/Absences.tsx:28` and `src/components/absences/KanbanView.tsx:35`
**Issue:** The exact same `initials()` function appears in two places with identical implementation. This is pre-existing duplication (the `KanbanView.tsx` copy was there first). The new `Absences.tsx` copy should have been imported or extracted to a shared utility.

**Suggestion:** Extract to `src/lib/string.ts` or similar shared module:
```ts
// src/lib/string.ts
export function initials(name: string): string {
  return name.split(" ").map((part) => part.charAt(0)).join("").toUpperCase().slice(0, 2);
}
```
Then import in both components.

---

_Reviewed: 2026-06-08T12:00:00Z_
_Reviewer: gsd-code-reviewer (standard depth)_
_Depth: standard_
