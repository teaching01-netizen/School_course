# Absence Form — Cognitive Load Reduction Plan

**Date:** 2026-05-31
**Goal:** Reduce cognitive load across the 4-step absence wizard by applying progressive disclosure, visual consolidation, and consistency fixes.
**Files:** `src/pages/AbsenceForm.tsx`, `src/components/absences/*.tsx`

---

## Problem Statement

The absence form has **1422 lines** in the main page component + 9 child components. The existing audit identified 18 issues (6 HIGH, 9 MEDIUM, 3 LOW). The primary cognitive load problems are:

1. **Error banner wall** — up to 6 stacked error/status banners push form content below the fold
2. **Step 1 conflates two operations** — lookup + OTP verification in one view creates nested card soup
3. **Step 2 information dump** — 4 simultaneous sections (courses, dates, reason category, free text) with no progressive disclosure
4. **Session row dual-checkbox scan distance** — 400-500px horizontal gap between session checkbox and cover checkbox
5. **Inconsistent visual language** — raw `<button>` vs `<Button>`, 4 date formatting functions, emoji vs lucide icons, `p-4` vs `p-5` padding

---

## Plan: 6 Independent Subagent Tasks

Each task is a standalone vertical slice. They can be executed in parallel or sequentially. No task depends on another.

---

### Task 1: Consolidate Error Banners (H4)

**Goal:** Replace the wall of up to 6 stacked error/status banners with a single prioritized error summary region.

**Current state:** `AbsenceForm.tsx` renders `renderStatusBanner()` + `<FormErrorSummary>` + inline errors. On mobile, this pushes the first form field 300-500px below the viewport top.

**Changes:**
- `src/pages/AbsenceForm.tsx`:
  - Remove `renderStatusBanner()` function (lines 711-724)
  - Rewrite `<FormErrorSummary>` to show only the **single most severe** error at a time
  - Priority order: submission error > verification blocked > lookup error > sessions error > parent phone missing > offline status
  - Add a tiny "N more issues" disclosure below the single error if additional non-critical warnings exist
  - Keep `role="alert"` only for the single displayed error; use `role="status"` for the offline banner

**Acceptance criteria:**
- Only one error banner visible at a time
- Most severe error always shown first
- Offline banner merges into the single-error region (not a separate banner)
- Mobile: first form field appears within 150px of viewport top

---

### Task 2: Split Step 1 into Lookup + Verify (H5)

**Goal:** Separate the student lookup from parent OTP verification to reduce nested card complexity.

**Current state:** Step 0 shows W-Code input → student info card → verification section → OTP fields → countdown → verify button → "Continue to courses" CTA. All in one `<section>`.

**Changes:**
- `src/pages/AbsenceForm.tsx`:
  - Step 0 becomes **Lookup only**: W-Code input + student info card + "Verify parent" button
  - Step 1 becomes **Verify only**: OTP send → enter → verify → "Continue to courses"
  - Update `STEP_LABELS` to `["Student Lookup", "Parent Verify", "Courses & Dates", "Sessions & Cover"]`
  - Update `canProceedFromVerify` logic to advance from step 1 to step 2
  - Move the verification section into its own step with clear heading and distinct visual container

**Acceptance criteria:**
- Step 0 shows only: W-Code input + student profile card + "Verify parent" CTA
- Step 1 shows only: OTP send/input/verify + skip option + "Continue to courses" CTA
- Step labels updated to reflect 4 distinct steps
- No nested cards deeper than 2 levels

---

### Task 3: Progressive Disclosure on Step 2 (M1)

**Goal:** Collapse optional reason fields behind an expandable section so the primary task (courses + dates) dominates the viewport.

**Current state:** Step 2 shows courses, dates, reason category dropdown, and free-text textarea simultaneously. The reason fields are already collapsible (lines 1102-1179) — this task ensures they default to collapsed and the UI is clean.

**Changes:**
- `src/pages/AbsenceForm.tsx`:
  - Ensure reason section defaults to collapsed (already does via `showReasonFields` state)
  - Verify the collapsible disclosure has proper `aria-expanded` and smooth animation
  - Add a subtle "Optional" badge next to the "Add reason details" heading
  - Ensure the reason section only appears after courses + dates are filled (currently shows even when no courses selected)

**Acceptance criteria:**
- Reason section collapsed by default on fresh form load
- "Optional" badge visible on the expandable trigger
- Reason section only expands after at least one course is selected
- Smooth height animation on expand/collapse

---

### Task 4: Session Row Layout + Cover Proximity (M2)

**Goal:** Reduce horizontal scan distance between session checkbox and cover checkbox by restructuring the session row layout.

**Current state:** Session checkbox is left-aligned, cover checkbox is right-aligned. On desktop, ~400-500px gap. Cover checkbox only appears when session is selected.

**Changes:**
- `src/pages/AbsenceForm.tsx` (step 2 session rows, lines 1190-1270):
  - Restructure each session row to place cover checkbox directly adjacent to session checkbox (left side)
  - New layout: `[checkbox] [date/time] [cover pill]` in a single horizontal flex row
  - Cover pill appears inline next to the session info, not at the far right
  - On mobile, stack vertically with cover pill indented below session info
- `src/components/absences/SessionChip.tsx`:
  - No changes needed (chips are used in SessionGrid, not in the main form step 2)

**Acceptance criteria:**
- Cover checkbox appears within 100px of session checkbox on desktop
- Visual grouping makes it obvious which cover belongs to which session
- Mobile layout stacks cleanly with cover pill indented
- Total scan distance reduced by >50%

---

### Task 5: Button + Visual Consistency Pass (H3, H6, M3, M4, M5)

**Goal:** Fix all visual inconsistencies: ARIA conflict on CourseChip, SessionGrid radius, raw buttons → Button component, padding standardization.

**Changes:**
- `src/components/absences/CourseChip.tsx`:
  - Remove `aria-pressed={selected}` (line 31) — keep `role="option"` + `aria-selected` only
- `src/components/absences/SessionGrid.tsx`:
  - Change `rounded-lg` to `rounded-sm` on lines 65, 113
  - Change `p-6` to `p-5` on line 65
- `src/pages/AbsenceForm.tsx`:
  - Replace 3 raw `<button>` primary CTAs (lines ~971, ~1097, ~1234) with `<Button variant="primary">`
  - Standardize all top-level card padding to `p-5`
  - Standardize nav footer strips to `p-4` (keep as secondary)
- `src/components/absences/ConfirmationSummary.tsx`:
  - Standardize padding to `p-5` on all cards
- `src/components/absences/DateRangeInput.tsx`:
  - No changes needed (already uses `<Button>`)

**Acceptance criteria:**
- CourseChip passes axe ARIA validation (no conflicting roles)
- All cards use `rounded-sm` consistently
- All primary CTAs use `<Button>` component (no raw `<button>` with inline styles)
- All top-level cards use `p-5`

---

### Task 6: Accessibility Contrast + Icon Consistency (H1, H2)

**Goal:** Fix WCAG AA contrast failures and replace emoji with lucide icons.

**Changes:**
- All files with `text-gray-400` on step labels:
  - Change to `text-gray-600` for unvisited steps
- All files with `text-xs text-gray-500`:
  - Change to `text-xs text-gray-600` (or `text-sm` where space allows)
- `src/components/absences/ConfirmationSummary.tsx`:
  - Already uses lucide icons (`CheckCircle`, `XCircle`, `Clock`) — verify no emoji remains
- `src/pages/AbsenceForm.tsx`:
  - Step indicator unvisited labels: `text-gray-400` → `text-gray-600`
  - Helper text: `text-gray-500` → `text-gray-600`
  - Course instruction text: verify contrast

**Acceptance criteria:**
- All text meets WCAG AA 4.5:1 contrast ratio
- No emoji used for status indicators (lucide only)
- axe or manual check confirms no contrast failures

---

## Execution Order

All 6 tasks are independent. Recommended order if sequential:

1. **Task 5** (Button + consistency) — foundation for other tasks
2. **Task 6** (Contrast + icons) — quick wins, broad impact
3. **Task 1** (Error consolidation) — highest UX impact
4. **Task 3** (Progressive disclosure) — medium effort, clean result
5. **Task 4** (Session row layout) — medium effort, scan distance reduction
6. **Task 2** (Step split) — largest structural change, most risk

---

## Files Modified Summary

| Task | Files |
|------|-------|
| 1 | `AbsenceForm.tsx` |
| 2 | `AbsenceForm.tsx` |
| 3 | `AbsenceForm.tsx` |
| 4 | `AbsenceForm.tsx` |
| 5 | `AbsenceForm.tsx`, `CourseChip.tsx`, `SessionGrid.tsx`, `ConfirmationSummary.tsx` |
| 6 | `AbsenceForm.tsx`, `ConfirmationSummary.tsx` |

---

## Testing

- Run existing tests: `src/pages/__tests__/AbsenceForm.test.tsx`, `src/components/absences/__tests__/*.test.tsx`
- Manual keyboard navigation: tab through all steps, verify focus order
- Screen reader check: NVDA/VoiceOver announcements for error regions, step changes, course selection
- Mobile check: 375px viewport, verify no error banner wall, cover checkbox proximity
