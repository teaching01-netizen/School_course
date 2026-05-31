# Code Review: DateRangePicker.tsx

**Reviewed:** 2026-05-31
**Depth:** standard
**Files Reviewed:** 1
**Status:** issues_found

## Summary

`DateRangePicker.tsx` is a well-structured controlled component managing multiple `DateRangeSlot` instances with add/remove, overlap validation, and animated entry/exit. The code is clean, readable, and follows the patterns established by sibling components (`DateRangeInput`, `DateRangeSlot`, `SessionGrid`). TypeScript types are well-defined, naming is clear, and the validation logic is correct.

Three issues warrant attention: a React key bug that will produce incorrect exit animations, a missing accessibility attribute that creates inconsistency with sibling components, and a missing ARIA role on the container. No critical bugs or security issues found.

## Warning Issues

### WR-01: Array index used as key in AnimatePresence — incorrect exit animations

**File:** `src/components/absences/DateRangePicker.tsx:142`

**Issue:** `key={idx}` uses the array index as the React key for each `motion.div`. When a slot is removed from the middle of the list (e.g., removing index 1 from `[0, 1, 2]`), React sees keys shift from `[0,1,2]` to `[0,1]` and incorrectly matches the old element at key 2 (slot C) as the "removed" element, animating out the wrong slot. `AnimatePresence` relies on stable keys to correctly pair old and new elements for exit animations.

**Fix:** Generate stable unique keys per slot. The cleanest approach is adding an `id` field to the `DateRange` type:

```ts
// In DateRangePicker.tsx
import { useId } from "react";

// Add a stable ID when creating a new range
const handleAdd = useCallback(() => {
  onChange([...value, { id: crypto.randomUUID(), from: undefined, to: undefined }]);
}, [value, onChange]);
```

Or, if modifying the `DateRange` interface is undesirable, use `useId` in `DateRangeSlot` and expose the key upward. At minimum, add a comment documenting the known limitation.

---

### WR-02: Error message missing `role="alert"` — inconsistent with sibling components

**File:** `src/components/absences/DateRangePicker.tsx:182-184`

**Issue:** The error `<p>` element lacks `role="alert"`. The sibling component `DateRangeInput.tsx` (line 108) applies `role="alert"` to its error message, and other components in the codebase (`FormField.tsx:36`, `FormErrorSummary.tsx:33`) follow the same pattern. Without `role="alert"`, screen readers won't announce validation errors when they appear, breaking the experience for assistive technology users.

**Fix:**
```tsx
{displayError && (
  <p className="mt-2 text-sm text-red-650 font-medium" role="alert">
    {displayError}
  </p>
)}
```

---

### WR-03: Container lacks ARIA role and label

**File:** `src/components/absences/DateRangePicker.tsx:137`

**Issue:** The root `<div>` has no ARIA role or accessible label. `SessionGrid.tsx` (a sibling component) uses `role="group"` with `aria-label` (line 112) to communicate its purpose to assistive technology. Without a role, screen reader users have no semantic context for this component's purpose or structure.

**Fix:**
```tsx
<div className="flex flex-col" role="group" aria-label="Date ranges">
```

## Info Issues

### IN-01: Dead animation code on add button wrapper

**File:** `src/components/absences/DateRangePicker.tsx:164-166`

**Issue:** `<motion.div initial={false} animate={{ opacity: 1 }}>` is a no-op — it starts at opacity 1 and animates to opacity 1, with `initial={false}` skipping the initial animation. This appears to be leftover from an earlier iteration or intended to be wrapped in `AnimatePresence`.

**Fix:** Either remove the `motion.div` and use a plain `<div>`, or remove the `initial` and `animate` props and keep the motion wrapper only if future animation is planned.

---

### IN-02: Near-duplicate handlers `handleFromChange` and `handleToChange`

**File:** `src/components/absences/DateRangePicker.tsx:102-119`

**Issue:** These two handlers are structurally identical — the only difference is which field (`from` vs `to`) they update. Minor DRY violation.

**Fix:**
```tsx
const handleDateChange = useCallback(
  (index: number, field: "from" | "to", date: Date | undefined) => {
    const next = value.map((r, i) =>
      i === index ? { ...r, [field]: date } : r,
    );
    onChange(next);
  },
  [value, onChange],
);
```

---

### IN-03: External error silently overrides all internal validation errors

**File:** `src/components/absences/DateRangePicker.tsx:92`

**Issue:** `displayError = externalError ?? internalError` hides all internal validation (overlaps, date ordering, max days) when an external error is present. If a server error is displayed while the user has also introduced an internal validation issue, the user gets no feedback about the internal problem. This is consistent with `DateRangeInput.tsx:50` (`error ?? localError`), so it matches the existing pattern, but it's worth noting as a design trade-off.

**Fix:** No code change required — document the behavior. Consider showing both errors if UX allows (e.g., external as a toast, internal inline).

---

### IN-04: Magic number `12` in animation `marginTop`

**File:** `src/components/absences/DateRangePicker.tsx:68`

**Issue:** `marginTop: 12` is a hardcoded pixel value with a comment `// matches space-y-3 = 0.75rem ≈ 12px`. The `≈` acknowledges this is an approximation. If the root font size changes (user zoom, accessibility settings), the animated margin won't match the Tailwind `mt-3` used on the adjacent "Add" button wrapper (line 167). The mismatch is subtle but could cause inconsistent spacing.

**Fix:** Acceptable as-is given the animation constraint (Framer Motion uses inline styles, not Tailwind classes). The comment is clear. Alternatively, compute from a CSS variable if precision matters.

## Strengths

1. **Clean separation of concerns** — validation helpers are pure functions extracted from the component, making them independently testable.
2. **Correct validation logic** — the overlap check (`a.from <= b.to && b.from <= a.to`) is the standard interval overlap formula and handles all edge cases correctly. The `+1` inclusive day count is correct.
3. **Well-typed exports** — `DateRange` and `DateRangePickerProps` are exported alongside the default export, giving consumers proper types.
4. **Consistent patterns** — error display (`text-red-650 font-medium`), button usage, and animation style match sibling components.
5. **Proper memoization** — `useMemo` for derived state and `useCallback` for handlers prevent unnecessary re-renders.
6. **Good component decomposition** — the parent (`DateRangePicker`) manages state, while `DateRangeSlot` handles individual slot UI and interaction, following single-responsibility.

## Assessment

The component is well-built and ready for use with minor fixes. The key bug (WR-01) is the most important — it produces incorrect animations on remove, which is a visible UX defect. The accessibility gaps (WR-02, WR-03) are important for inclusive design and should be addressed before shipping. The info items are quality-of-life improvements that can be addressed opportunistically.
