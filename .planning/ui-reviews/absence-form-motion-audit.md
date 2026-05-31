# Absence Form — Visual Design & Motion Audit

**Audited:** 2026-05-30
**Files:** `src/pages/AbsenceForm.tsx` (1333 lines), 10 components in `src/components/absences/`
**UI System:** `src/components/ui/` (Button, Input, LoadingSkeleton, EmptyState, FormField, etc.)
**Stack:** React + Tailwind CSS v4 + framer-motion v12 + lucide-react
**Screenshots:** Not captured (no dev server detected)

---

## Executive Summary

The form is functionally complete with strong accessibility (focus management, aria attributes, 44px touch targets) but visually sterile. framer-motion is imported yet used in only **2 places**: step transitions and CourseChip scale-down. The "not engaging, no motion" complaint is accurate — 90% of state transitions, selection changes, and micro-interactions are instant with zero animation. Loading states are raw text despite a `LoadingSkeleton` component existing in the UI library. The form's `Continue` buttons bypass the shared `Button` component with duplicated inline styles, creating a maintenance hazard.

---

## 1. Motion Audit

### 1.1 Where framer-motion IS used

| Location | Lines | What | Issues |
|----------|-------|------|--------|
| Step transitions | 879-884 | `AnimatePresence mode="popLayout"` with opacity+x slide | Directional logic correct (forward/back). Duration 0.18-0.25s reasonable. `useReducedMotion` respected. **But**: no spring easing (uses `easeOut`), no scale/rotate to distinguish from other page transitions. |
| CourseChip `whileTap` | CourseChip.tsx:36 | `scale: 0.95` on tap | Good touch-feedback micro-interaction. **But**: only `whileTap`, no `whileHover`, no `animate` for selection state. Selected state is instant — no border/color transition animation. |

### 1.2 Where framer-motion is MISSING (critical gaps)

**Every item below contributes directly to "not engaging / no motion".**

#### SessionChip — zero motion (SessionChip.tsx)
- No `whileHover` or `whileTap`.
- Selected/unselected toggle is instant class swap (line 61-62) — no border transition, no background fade.
- A `⊘` character appears instantly for already-absent sessions — could fade/slide in.
- **Fix:** Wrap in `<motion.button>`, add `whileHover={{ scale: 1.02 }}`, animate `backgroundColor` and `borderColor` on selection.

#### Step indicator — static (AbsenceForm.tsx:851-877)
- Active step highlight is instant: `bg-wi-primary/10` appears with zero transition.
- Completed step checkmark appears instantly.
- No pill-sliding animation when active step changes.
- **Fix:** Animate the active indicator with `layoutId="step-indicator"` for a smooth sliding pill. Animate the Check icon appearance with `scale` spring.

#### Checkbox toggles — all native HTML, zero animation
- Step 2 session checkboxes (line 1197-1201): native `<input type="checkbox">` — no micro-interaction.
- Cover checkboxes (line 1210-1218): same.
- Course selection circles: use `Check` icon from lucide but it appears/disappears instantly (CourseChip.tsx:46).
- **Fix:** Replace native checkboxes with animated `<motion.div>` + lucide `Check` with `pathLength` animation, or wrap in a container with `animate={{ scale: selected ? 1 : 0.8 }}`.

#### Selection highlight transitions — instant (AbsenceForm.tsx:1192-1194)
- Session rows in Step 2 toggle between `border-gray-200 bg-white` and `border-wi-primary bg-wi-primary/5` instantly.
- No `transition-colors duration-150` class on session row div (line 1192).
- **Fix:** Add `transition-colors duration-150` to the session row. Use framer-motion's `layout` prop for smooth reflow when session count changes.

#### Error/success banners — instant appearance (multiple locations)
- Every `<div role="alert">` appears with no fade/ slide.
- StepCoverVerification "Verification complete" banner (line 273) appears instantly despite being a significant state milestone.
- **Fix:** Wrap in `<AnimatePresence>` with fadeIn + slideDown (height: auto → needs `layout` or fixed height).

#### CountdownTimer — no visual pulse (CountdownTimer.tsx)
- Timer digits update via `useMemo` — plain text re-render.
- No scale/color animation when timer hits milestones (60s, 30s, 10s).
- **Fix:** Add a framer-motion `animate` that pulses the timer text (`scale: [1, 1.05, 1]`) at each minute boundary. Change text color from gray to amber at ≤10s.

#### OTP input — no digit animation (OtpInput.tsx)
- Digits appear instantly in boxes (line 79).
- No stagger, no scale, no border highlight animation on digit entry.
- Error state border swap is instant.
- **Fix:** Animate each digit box with `initial={{ scale: 0.8 }} animate={{ scale: 1 }}` staggered on mount. Animate error border with `transition` on `borderColor`.

#### Loading states — entirely static text
- "Loading form settings…" (AbsenceForm.tsx:822) — plain text, no skeleton.
- "Loading sessions…" (AbsenceForm.tsx:1145) — plain text.
- "Restoring saved verification session…" (StepCoverVerification.tsx:229) — plain text.
- `LoadingSkeleton` component exists at `src/components/ui/LoadingSkeleton.tsx` with `table`, `card`, and `text` variants — **never imported or used** in the absence form.
- **Fix:** Replace all text loading states with `<LoadingSkeleton type="card" lines={3} />`.

#### "Continue to…" buttons — no micro-interaction (AbsenceForm.tsx:971-979, 1097-1109, 1234-1246)
- All three step "continue" buttons are raw `<button>` with no `whileHover`/`whileTap`.
- They duplicate Button component styles inline (see Section 3).
- **Fix:** Use `<Button>` with loading state or add `whileHover`/`whileTap` to the raw buttons.

#### Empty states — no visual polish
- "No sessions found in this date range." (AbsenceForm.tsx:1151) — plain text in a gray box.
- `EmptyState` component exists at `src/components/ui/EmptyState.tsx` with lucide `Inbox` icon — **never used** in absence form.
- **Fix:** Use `<EmptyState message="No sessions found in this date range." />`.

#### Confirmation/result screen — no celebration animation (AbsenceForm.tsx:738-796)
- "Submission complete" section appears instantly.
- Reference code box appears with no highlight animation.
- Green border on success section has no entrance motion.
- **Fix:** Wrap in `<motion.div initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} transition={{ type: "spring", bounce: 0.3 }}>`. Add a brief "success" checkmark animation (scale + rotate of a lucide `CheckCircle` icon).

---

## 2. Visual Hierarchy Issues

### 2.1 Step Indicator — too subtle (AbsenceForm.tsx:851-877)

- Active step gets `bg-wi-primary/10` — 10% opacity is nearly invisible on white backgrounds.
- Completed steps get no distinct visual cue beyond the Check icon (gray text vs active color).
- The current step label text is shown on mobile but step number circles are tiny (`h-5 w-5`, `text-[10px]`).
- **Fix:** Use `bg-wi-primary` (solid) with white text for the active step. Use `bg-green-50 border-green-300` for completed steps. Keep current step's circle at `h-7 w-7` (larger) to make it the focal point.

### 2.2 Primary CTAs vs secondary actions — insufficient contrast

- The "Continue to courses/sessions/review" buttons (raw buttons, lines 973, 1099, 1236) use `bg-[var(--color-wi-primary)]` — correct dominance.
- **But** the "Back" buttons (line 1093, 1230) use the `Button` component with `variant="secondary"` — they render as outlined buttons, which is correct for hierarchy. However, the inconsistency between raw-button-primary and component-button-secondary is visually jarring (slightly different heights, padding, border-radius).

### 2.3 Course selection — selection state too subtle (CourseChip.tsx)

- Selected: `bg-wi-primary/10 border-wi-primary` — 10% opacity tint.
- Unselected: `bg-white border-gray-200`.
- The only clear selected indicator is the `Check` icon in the circle — but the circle is tiny (`h-5 w-5`).
- **Fix:** Increase selected state to `bg-wi-primary/15` at minimum. Add a left-border accent (4px left border like a sidebar indicator). Make the check circle `h-6 w-6` with `bg-wi-primary text-white fill`.

### 2.4 Session rows — overloaded visual density (Step 2)

- Each session row (line 1190-1220) shows date, time range, a selected checkbox, and a "Needs cover" checkbox.
- At `px-3 py-3` with `gap-3` in flex columns — high density. Multiple rows stacked makes the page feel like a spreadsheet.
- **Fix:** Add alternating row background (even rows `bg-gray-50/50`). Add a narrow date-column strip on the left. Reduce `py-2` instead of `py-3` for tighter spacing.

---

## 3. Button Component Inconsistency (BLOCKER)

**Three raw `<button>` elements duplicate the `Button` component's primary variant styles:**

| Location | Lines | Style |
|----------|-------|-------|
| Step 0 → 1 | 971-979 | `bg-[var(--color-wi-primary)] text-white … hover:bg-[var(--color-wi-primary-dark)]` |
| Step 1 → 2 | 1097-1109 | Same duplicated string |
| Step 2 → 3 | 1234-1246 | Same duplicated string |

These replicate `variantClasses.primary` from `Button.tsx:13-14` exactly. This is a maintenance hazard — if primary button styling changes (e.g., adding `shadow-sm` or `rounded-md`), these three buttons will be forgotten.

**Visual consequence:** The raw buttons are `inline-flex` but the `Button` component adds `items-center justify-center`. The raw buttons are `rounded-sm` (same as Button) but if Button gains `gap-2` for icon spacing, these diverge. They also do not have `loading` state support — during form submission, these buttons offer no spinner feedback.

**Fix:** Replace all three raw buttons with:
```tsx
<Button variant="primary" onClick={...} aria-disabled={!canProceed}>
  Continue to ...
  <ChevronRight className="ml-2 h-4 w-4" />
</Button>
```

---

## 4. Color & State Differentiation

### 4.1 Selected vs unselected — too close in value

- CourseChip selected: background `wi-primary/10` (very light tint) + border `wi-primary` (full saturation).
- Session row selected: background `wi-primary/5` (even lighter) + border `wi-primary`.
- On a white background with gray-200 unselected borders, the selected state reads as "slightly different gray" at a glance.
- **Fix:** Use `wi-primary/15` for selected backgrounds (minimum 15% opacity). Add a subtle shadow or left-accent bar for selected items.

### 4.2 Disabled states — same as unselected

- Disabled CourseChip (CourseChip.tsx:42): `opacity-60 cursor-not-allowed`.
- Disabled session cover checkbox (AbsenceForm.tsx:1214): `disabled:cursor-not-allowed` but no opacity change.
- Disabled buttons: `disabled:opacity-50` — consistent.
- **Issue:** Disabled states don't have a distinct visual language (e.g., gray fill, no border color change).

### 4.3 Error states — all look identical

Every error alert uses the same pattern:
```
border-red-200 bg-red-50 p-4 text-sm text-red-900
```
Applied at lines 828, 834, 840, 1304 and in StepCoverVerification lines 235, 290, 296.

- No differentiation between validation errors (e.g., empty field), API errors, and system errors.
- No icon prefix to distinguish error type.
- **Fix:** Use lucide `AlertTriangle`, `XCircle`, or `Info` icon as a prefix. Use `border-l-4 border-l-red-500` for a left accent bar to make errors more scannable.

---

## 5. Iconography (lucide-react)

### 5.1 Underutilized

Only 3 lucide icons are used in the entire 1333-line form:
- `Check` — step indicator + CourseChip selection.
- `ChevronLeft` / `ChevronRight` — navigation.
- `Copy` — reference code copy.

**Missing opportunities:**
- **Step 0 (Lookup):** No `Search` or `User` icon on the lookup button.
- **Step 1 (Courses):** No `Calendar` icon near date inputs. No `BookOpen` for courses.
- **Step 2 (Sessions):** No `Clock` for session times. No `Shield` or `CheckCircle` for cover status.
- **Step 3 (Review):** No `FileText` or `Eye` icon.
- **Errors:** No `AlertTriangle` or `XCircle` icon prefixes (see 4.3).
- **Empty states:** No icon in the "No sessions found" text (see 1.2 empty states).

### 5.2 ConfirmationSummary uses emoji, not lucide (ConfirmationSummary.tsx:57-62)

```tsx
function StatusIndicator({ status }) {
  if (status === "success") return <span className="text-green-600" aria-hidden="true">✅</span>;
  if (status === "error") return <span className="text-red-600" aria-hidden="true">❌</span>;
  return <span className="text-gray-400" aria-hidden="true">⏳</span>;
}
```

Emoji render differently across OS/browser. The rest of the app uses lucide-react. This must be consistent.

**Fix:** Replace emoji with lucide icons:
```tsx
import { CheckCircle, XCircle, Clock } from "lucide-react";
// success: <CheckCircle className="h-4 w-4 text-green-600" aria-hidden="true" />
// error: <XCircle className="h-4 w-4 text-red-600" aria-hidden="true" />
// pending: <Clock className="h-4 w-4 text-gray-400" aria-hidden="true" />
```

---

## 6. Mobile Responsiveness

### 6.1 What's good
- `min-h-[44px]` on all touch targets (44px is Apple HIG minimum).
- `hidden sm:inline` on step labels (good progressive disclosure).
- `max-sm:flex-col` patterns for session rows.
- `max-sm:w-full` on SessionChip (line 59).
- `max-sm:border-l-4` on SessionChip for mobile-friendly selection indicator.

### 6.2 What's problematic

- **Step indicator on small screens** (line 873): Labels hidden, only number circles visible. But circles are `h-5 w-5 text-[10px]` — very small. Step 4/4 circles barely distinguishable.
  - **Fix:** Make mobile step circles `h-7 w-7` with `text-xs`. Add a subtle active indicator underline.

- **Date range pickers** (line 1044-1063): `sm:grid-cols-2` — works. But `Input` components with `type="date"` on iOS Safari render with tiny text and no native styling hook.
  - **Fix:** Add `text-base` on mobile to prevent iOS zoom on focus (standard iOS fix).

- **Session rows on mobile** (line 1190-1220): `sm:flex-row` with checkboxes on left → on mobile stacks vertically with each checkbox on its own line. The "Needs cover" checkbox sits below the session info, which is correct. But the selected border accents don't exist on mobile (no `max-sm:border-l-4` like SessionChip has).
  - **Fix:** Add `max-sm:border-l-4 max-sm:border-l-wi-primary` to selected session rows.

- **Form container width** (line 808): `max-w-4xl` on a 4-step form. On a 375px mobile screen with `px-4` padding, content area is ~343px. The "Continue to courses/sessions/review" buttons at full width look fine, but the course grid (`sm:grid-cols-2`) collapses to single column — good.

---

## 7. Loading States

### 7.1 Text-only loading (no skeletons)

| Location | Text | Lines |
|----------|------|-------|
| Config fetch | "Loading form settings…" | 822 |
| Sessions fetch | "Loading sessions…" | 1145 |
| Verification restore | "Restoring saved verification session…" | StepCoverVerification:229 |
| Sessions page | "Loading..." | KanbanView:202 |

`LoadingSkeleton` component (`src/components/ui/LoadingSkeleton.tsx`) supports `type="card"`, `type="table"`, and `type="text"` — all with `animate-pulse`. The absence form imports **none** of them.

**Fix:** Replace each text loading state with the appropriate skeleton:
- Form config loading → `<LoadingSkeleton type="card" lines={3} />`
- Sessions loading → `<LoadingSkeleton type="table" lines={5} />`
- Verification restore → `<LoadingSkeleton type="text" lines={2} />`

### 7.2 Submission loading state

`handleSubmitAbsence` (line 642) sets `isSubmitting=true` which passes `loading={isSubmitting}` to the final "Submit absence" Button (line 1314-1319). This works correctly and shows the SVG spinner. **But** all three intermediate "Continue" buttons have no loading support because they're raw `<button>` elements.

---

## 8. Confirmation/Result Screen

### 8.1 Current state (lines 738-796)

- Green section: `border-emerald-200 bg-white` with `text-xl font-semibold` heading "Submission complete".
- Reference code box: `border-gray-200 bg-gray-50` with `font-mono`.
- `ConfirmationSummary` component below in `mode="result"`.

### 8.2 Missing visual cues

- No success animation (checkmark draw, confetti, scale-in).
- No visual emphasis on the reference code (it's in a gray-50 box — same as error banners).
- "Submission complete" heading is just text — no leading icon.

**Fix:**
```tsx
<motion.section
  initial={{ opacity: 0, y: 20, scale: 0.98 }}
  animate={{ opacity: 1, y: 0, scale: 1 }}
  transition={{ type: "spring", damping: 20, stiffness: 100 }}
  className="rounded-sm border border-emerald-200 bg-white p-5 shadow-sm"
>
  <div className="flex items-center gap-3">
    <motion.div
      initial={{ scale: 0, rotate: -90 }}
      animate={{ scale: 1, rotate: 0 }}
      transition={{ type: "spring", damping: 10, stiffness: 200, delay: 0.2 }}
    >
      <CheckCircle className="h-8 w-8 text-emerald-500" />
    </motion.div>
    <div>
      <h2 ref={resultHeadingRef} tabIndex={-1} className="text-xl font-semibold text-emerald-900">
        Submission complete
      </h2>
      <p className="text-sm text-emerald-700">Your absence has been saved and is waiting for review.</p>
    </div>
  </div>
  {/* reference code box with highlight animation */}
  <motion.div
    initial={{ opacity: 0, x: -10 }}
    animate={{ opacity: 1, x: 0 }}
    transition={{ delay: 0.4 }}
    className="mt-4 flex flex-wrap items-center gap-2 rounded-sm border border-emerald-200 bg-emerald-50/50 px-4 py-3"
  >
    ...
  </motion.div>
</motion.section>
```

---

## 9. Other Visual Issues

### 9.1 FormField hint bug (FormField.tsx:41)

Hints render with `text-red-600` — should be `text-gray-500`. This is a color-pillar defect that would mislead users into thinking hints are error messages.

### 9.2 Background gradient — single use only

The gradient `radial-gradient(circle_at_top, rgba(17,24,39,0.03), transparent 40%), linear-gradient(180deg, #f8fafc 0%, #ffffff 100%)` is applied to the outer container but has extremely subtle effect (3% black at top). The gradient could be stronger (5-8%) and paired with a `bg-fixed` property for a parallax-like feel.

### 9.3 Copy reference animation (line 680-683)

`copiedReference` state toggles the Copy button text to "Copied" for 2 seconds — but there's no checkmark animation or color change on the button. The icon stays as `Copy` (line 769). **Fix:** Swap icon to `Check` during copied state with a lucide `Check` icon animation.

---

## Top 5 Impactful Visual Improvements

### 1. Replace text loading states with LoadingSkeleton components
**Impact:** Instant perceived performance improvement. Users see skeleton shapes that communicate layout before content arrives.
**Effort:** Low (10 minutes, 4 replacements).
**Files:** AbsenceForm.tsx:822, 1145; StepCoverVerification.tsx:229; KanbanView.tsx:202.

### 2. Animate session and course selection with framer-motion
**Impact:** The most visually dense interaction (selecting/deselecting sessions) currently has zero feedback motion. Adding `whileTap`, `whileHover`, and selection-state transitions makes the form feel responsive.
**Effort:** Medium (add `<motion.button>` / `motion.div` wrappers, transition props on borderColor/backgroundColor).
**Files:** SessionChip.tsx, CourseChip.tsx, AbsenceForm.tsx session rows (line 1190).

### 3. Consolidate all primary action buttons to use the Button component
**Impact:** Removes code duplication (3 instances) and enables consistent hover/tap/loading animations across all CTAs. Raw buttons don't get the Spinner or disabled styles the Button component provides.
**Effort:** Low (replace 3 raw `<button>` with `<Button variant="primary">`).
**Files:** AbsenceForm.tsx:971-979, 1097-1109, 1234-1246.

### 4. Add success animation and icons to the confirmation screen
**Impact:** The "Submission complete" screen is the emotional peak of the flow — it's where the user gets relief. Currently it's a plain green box with no celebration motion.
**Effort:** Medium (add framer-motion spring entrance, CheckCircle icon, reference code highlight).
**Files:** AbsenceForm.tsx:738-796.

### 5. Replace emoji status indicators with lucide icons in ConfirmationSummary
**Impact:** Consistent visual language (the rest of the app uses lucide). Emoji render inconsistently across OS. Fixes a specific design-system violation.
**Effort:** Low (replace 3 `<span>` with lucide `CheckCircle`/`XCircle`/`Clock`).
**Files:** ConfirmationSummary.tsx:55-63.
