---
phase: 03-calendar-component
reviewed: 2026-05-31T00:00:00Z
depth: standard
files_reviewed: 4
files_reviewed_list:
  - src/components/absences/DateRangeSlot.tsx
  - src/components/ui/calendar.tsx
  - src/index.css
  - src/components/ui/__tests__/calendar.test.tsx
findings:
  critical: 1
  warning: 1
  info: 3
  total: 5
status: issues_found
---

# Phase 3: Code Review Report — DateRangeSlot Refactor to Calendar

**Reviewed:** 2026-05-31T00:00:00Z
**Depth:** standard
**Files Reviewed:** 4
**Status:** issues_found

## Summary

The refactor replaces raw `DayPicker` with the project's `Calendar` wrapper, removes the manual 15-line classNames mapping, adds `captionLayout="dropdown"`, and removes the `react-day-picker/style.css` import. The core prop pass-through is correct — `Calendar` type is `DayPickerProps` so all props flow through identically. Two substantive issues: the `Calendar` component hardcodes `p-3` creating double padding with the existing wrapper, and `captionLayout="dropdown"` introduces dropdown UI that has zero CSS coverage in `index.css` (the removed `style.css` was providing those styles).

## Critical Issues

### CR-01: `captionLayout="dropdown"` has no CSS support — dropdowns render unstyled

**File:** `src/index.css:190-274` + `src/components/absences/DateRangeSlot.tsx:121,163`
**Issue:** The refactor adds `captionLayout="dropdown"` to both calendars. This is a NEW feature (not in the original `DayPicker` instances). The `react-day-picker/style.css` import was removed, and `index.css` defines no rules for the dropdown-related classes that `getDefaultClassNames()` emits:

- `.rdp-dropdowns` — container layout for month/year selects
- `.rdp-dropdown` — individual `<select>` styling
- `.rdp-dropdown_root` — wrapper
- `.rdp-caption_label` — caption text styling
- `.rdp-chevron` — chevron icon in navigation
- `.rdp-months_dropdown` / `.rdp-years_dropdown` — the actual `<select>` elements

Without these rules, the month/year dropdowns render as unstyled native `<select>` elements with default browser borders, padding, and backgrounds — visually inconsistent with the rest of the styled calendar.

**Fix:** Add CSS rules to `src/index.css` for the dropdown classes. At minimum:

```css
/* Dropdown caption for month/year selection */
.rdp-dropdowns {
  display: flex;
  gap: 4px;
  justify-content: center;
  margin-bottom: 4px;
}
.rdp-dropdown {
  font-family: var(--font-sans);
  font-size: 0.875rem;
  font-weight: 600;
  padding: 4px 8px;
  border: 1px solid var(--color-wi-border, #E2E8F0);
  border-radius: var(--radius-sm);
  background: white;
  color: var(--color-wi-text);
  cursor: pointer;
}
.rdp-dropdown:focus-visible {
  outline: 2px solid var(--color-wi-primary);
  outline-offset: 1px;
}
.rdp-caption_label {
  font-weight: 600;
  font-size: 0.875rem;
  color: var(--color-wi-text);
}
```

Alternatively, if the spec intent was "copy-paste the old caption rendering," consider using `captionLayout="label"` (the default) instead of `"dropdown"`, which avoids the unstyled dropdown issue entirely. But if dropdowns are desired, the CSS must be added.

## Warnings

### WR-01: Double padding — Calendar hardcodes `p-3` on top of the wrapper's `p-3`

**File:** `src/components/absences/DateRangeSlot.tsx:111,153` + `src/components/ui/calendar.tsx:22`
**Issue:** `Calendar` hardcodes `className={cn("p-3", className)}` on the DayPicker root element. The `motion.div` wrapper in `DateRangeSlot` also has `p-3` in its className. This results in 24px total internal padding (12px from wrapper + 12px from Calendar) vs the original 12px (wrapper only). The calendar popover will have visibly more whitespace than before.

**Fix:** Remove `p-3` from the `motion.div` wrapper since `Calendar` now provides it internally:

```tsx
// Lines 111, 153 — change from:
className="absolute z-50 top-full left-0 mt-1 rounded-xl border border-gray-200 bg-white shadow-xl p-3"
// to:
className="absolute z-50 top-full left-0 mt-1 rounded-xl border border-gray-200 bg-white shadow-xl"
```

This preserves the same visual result: one `p-3` applied (by Calendar on the DayPicker root).

## Info

### IN-01: Import conflict handled correctly

**File:** `src/components/absences/DateRangeSlot.tsx:4,6`
**Issue:** `Calendar` is imported from both `lucide-react` (icon) and `@/components/ui/calendar` (component). The rename to `CalendarIcon` on line 4 is correct and avoids the naming collision. No action needed.

### IN-02: `showOutsideDays` / `fixedWeeks` defaults align

**File:** `src/components/ui/calendar.tsx:13-14` + `src/components/absences/DateRangeSlot.tsx`
**Issue:** The old code explicitly passed `showOutsideDays` and `fixedWeeks` to `DayPicker`. The `Calendar` component defaults both to `true` via destructuring defaults, so omitting them in `DateRangeSlot` is equivalent. No action needed.

### IN-03: No test files need updating

**Files:** `src/components/ui/__tests__/calendar.test.tsx`
**Issue:** `calendar.test.tsx` tests the `Calendar` component in isolation (renders, applies className, passes props). No test files directly reference `DateRangeSlot` or import `DayPicker` directly. The existing tests remain valid.

---

_Reviewed: 2026-05-31T00:00:00Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
