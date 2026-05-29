# UX/UI Overhaul Specification

Date: 2026-05-27

## Scope

Full frontend UX/UI overhaul across 20+ pages, ~40 items across 5 phases. Covers design system foundation, critical blockers, visual consistency, form UX, polish, and Schedule.tsx decomposition.

---

## Phase 0 — Design System Foundation

Establish shared primitives before any page-level work. Prevents rework.

### 0.1 CSS Token Additions (`src/index.css`)

Add to `@theme` block:

```
--color-wi-danger:       #DC2626
--color-wi-danger-bg:    #FEF2F2
--color-wi-amber:        #D97706
--color-wi-amber-bg:     #FFFBEB
--color-wi-text-light:   #6B7280
--color-wi-border:       #D1D5DB
--color-wi-bg:           #F9FAFB
--radius-sm:             2px
--radius-md:             6px
--transition-fast:       150ms ease
--font-size-xs:          0.75rem
--font-size-sm:          0.875rem
--font-size-base:        1rem
--font-size-lg:          1.125rem
--font-size-xl:          1.25rem
--font-size-2xl:         1.5rem
--font-size-3xl:         2rem
```

**Motivation**: 3-way color fragmentation (CSS vars, Tailwind tokens, hardcoded hex). Single source of truth.
**Effort**: XS

### 0.2 `<Input>` Component (`src/components/ui/Input.tsx`)

```tsx
interface InputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "size"> {
  size?: "sm" | "md";       // default "md"
  error?: boolean;
  describedBy?: string;
}
```

- `sm`: `px-2 py-1 text-sm rounded-sm` (~32px h)
- `md`: `px-3 py-2 text-sm rounded-sm` (~38px h)
- States: normal, focus (`ring-3 ring-wi-primary/15`), error (`border-wi-red`), disabled
- Forwards ref, spreads native props, `aria-invalid` when error

**Effort**: S (~40 lines)

### 0.3 `<Select>` Component (`src/components/ui/Select.tsx`)

```tsx
interface SelectProps extends Omit<SelectHTMLAttributes<HTMLSelectElement>, "size"> {
  size?: "sm" | "md";
  error?: boolean;
  placeholder?: string;
  describedBy?: string;
}
```

- Same sizing/state mapping as Input
- Custom chevron arrow via `background-image` SVG data URI
- `pr-8` for arrow space

**Effort**: XS (~50 lines)

### 0.4 `<Button>` Component (`src/components/ui/Button.tsx`)

```tsx
interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "secondary" | "danger" | "ghost";
  size?: "sm" | "md" | "lg";
  loading?: boolean;
}
```

| Variant | Normal | Hover |
|---------|--------|-------|
| primary | `bg-wi-primary text-white` | `bg-wi-primary-dark` |
| secondary | `bg-white text-wi-text border-wi-border` | `bg-gray-50` |
| danger | `bg-wi-red text-white` | `bg-wi-red-dark` |
| ghost | `bg-transparent text-wi-text` | `bg-gray-100` |

- Disabled: `opacity-50 cursor-not-allowed`
- Loading: spinner SVG + `aria-busy="true"` + `disabled`
- Sizes: `sm`=`px-2 py-1 text-xs`, `md`=`px-3 py-1.5 text-sm`, `lg`=`px-4 py-2 text-sm`
- `inline-flex items-center justify-center font-medium transition-colors duration-150`

**Effort**: S (~60 lines)

### 0.5 `<PageHeading>` Component (`src/components/ui/PageHeading.tsx`)

```tsx
interface PageHeadingProps {
  children: ReactNode;
  className?: string;
}
```

- Renders `text-[32px] font-bold text-wi-text`
- Unifies 7 different heading sizes (28px–56px)

**Effort**: XS (~10 lines)

### 0.6 `<SearchInput>` Component (`src/components/ui/SearchInput.tsx`)

```tsx
interface SearchInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}
```

- `<Input>` with lucide `Search` icon at left, `pl-8`
- `type="search"`, `aria-label="Search"`

**Effort**: XS (~30 lines)

### 0.7 `<EmptyState>` Component (`src/components/ui/EmptyState.tsx`)

```tsx
interface EmptyStateProps {
  message: string;
  icon?: ReactNode;
  action?: ReactNode;
}
```

- Renders `py-12 text-center text-gray-400 text-sm`
- Replaces inline `"No X found"` text blocks

**Effort**: XS (~20 lines)

### 0.8 `<LoadingSkeleton>` Component (`src/components/ui/LoadingSkeleton.tsx`)

```tsx
interface LoadingSkeletonProps {
  lines?: number;       // default 3
  type?: "table" | "card" | "text";
}
```

- Pulsing gray bars matching shape, CSS `@keyframes pulse`
- Replaces all `"Loading…"` text placeholders

**Effort**: S (~40 lines)

---

## Phase 1 — Critical Blockers (Accessibility + UX Bugs)

### 1.1 Modal: Focus Trap + ARIA

Add to Modal.tsx:
- `role="dialog"`, `aria-modal="true"`, `aria-labelledby` linking to title
- Focus trap: cycle Tab/Shift+Tab within modal
- Escape key closes via `document.addEventListener("keydown", ...)`
- Body `overflow: hidden` on open, restore on unmount
- Auto-focus first focusable element on open
- Return focus to trigger element on close

**Effort**: M (~80 lines)

### 1.2 TypeaheadSelect: Combobox ARIA + Keyboard

Add to TypeaheadSelect.tsx:
- `role="combobox"`, `aria-expanded`, `aria-controls`, `aria-activedescendant`
- Keyboard: ↑↓ highlight, Enter selects, Escape closes
- `role="listbox"` on dropdown, `role="option"` on items
- `aria-selected` on highlighted option
- No-results state: `"No matches found"`

**Effort**: M (~100 lines)

### 1.3 Login Submit Fix

- `disabled={submitting}` on button
- Loading text `"Signing in…"`
- `autoComplete="username"` on username, `autoComplete="current-password"` on password

**Effort**: XS

### 1.4 Replace `window.confirm()` with `<ConfirmModal>`

New component:
```tsx
interface ConfirmModalProps {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;    // default "Confirm"
  variant?: "danger" | "primary";
  loading?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}
```

Replace 2 instances in Schedule.tsx (lines 202, 614).

**Effort**: S

### 1.5 Fix Preflight Button Gating

New hook `usePreflightGate`:
```tsx
function usePreflightGate(preflight, { requiredFields?, isFormValid? }) {
  const canSave = (status === "available" || status === "provisional")
                  && !preflight.loading && (isFormValid ?? true);
  return { canSave, reason, status, isChecking };
}
```

Fix bug: when status=`"blocked"`, `!status` is `false` → button was enabled. Now `canSave` is false.

**Effort**: XS

### 1.6 Toast ARIA Live Region

Toast container: `role="alert"` + `aria-live="assertive"` + `aria-atomic="true"`

**Effort**: XS

### 1.7 Remove No-Op Search Buttons

Courses, Subjects, Teachers, Home: remove Search buttons with no onClick handler (filtering is native keystroke).

**Effort**: XS

---

## Phase 2 — Visual Consistency Sweep

### 2.1 Unify Page Headings

Replace `text-[56px]`, `text-[44px]`, `text-[40px]`, `text-[32px]`, `text-[28px]` with `<PageHeading>` across 15 files.

**Effort**: S

### 2.2–2.4 Replace Hardcoded Hex with CSS Vars

- Teachers.tsx: `#2563EB`→`var(--color-wi-primary)`, `#059669`→`var(--color-wi-green)`, `#DC2626`→`var(--color-wi-red)`
- PreflightIndicator.tsx: `text-green-700`→`text-wi-green`, `text-amber-700`→`text-wi-amber`, `text-red-700`→`text-wi-red`
- Layout.tsx: `text-[#94A3B8]`→`text-wi-text-light`
- WILogo.tsx: hardcoded stroke colors→`currentColor` context

**Effort**: S (4 files)

### 2.5 Unify Form Inputs → `<Input>` / `<Select>`

Replace all ad-hoc input/select class declarations across every page. Standardize on `rounded-sm` and consistent padding.

Current fragmentation:
| Context | Padding | Radius |
|---------|---------|--------|
| Create pages (Course/Teacher/Subject) | `px-3 py-2` | `rounded-md` |
| Edit forms + modals | `px-2 py-1.5` | `rounded-sm` |
| Login | `px-2 py-1.5` | `rounded-sm` |

**Effort**: M (~15 files)

### 2.6 Unify Buttons → `<Button>`

Replace all button class declarations with `<Button variant={...} size={...}>` across every page.

Currently 4+ distinct button styles in Schedule.tsx alone.

**Effort**: M (~15 files)

### 2.7 Clean Up Dead Global CSS (`src/index.css`)

Remove: `th,td` padding, `th font-weight`, `tr:hover`, `input` border (`#ccc`), `input` border-radius (`3px`). Keep focus-visible rule.

**Effort**: XS

### 2.8 Fix Footer Copyright

`© 2017 - Warwick Institute` → `© 2026 - Warwick Institute`

**Effort**: XS

---

## Phase 3 — Form UX Overhaul

### 3.1 `useFormValidation` Hook (`src/hooks/useFormValidation.ts`)

```tsx
interface ValidationRule {
  type: "required" | "minLength" | "maxLength" | "pattern" | "min" | "max" | "custom";
  value?: number | string | RegExp;
  message: string;
  validate?: (value: unknown, formValues: Record<string, unknown>) => string | null;
}

function useFormValidation<T>(schema: Record<keyof T, ValidationRule[]>, formValues: T): {
  errors: Record<string, string>;
  validate: (field: keyof T) => boolean;
  validateAll: () => boolean;
  isValid: boolean;
  touched: Record<string, boolean>;
  touch: (field: keyof T) => void;
  clearErrors: () => void;
  setErrors: (errors: Record<string, string>) => void;
}
```

- Field-level on blur, form-level on submit
- First-failing rule wins
- `required` on empty→error; other rules fire only when non-empty (optional fields)
- Errors only shown for touched fields

**Effort**: M (~100 lines)

### 3.2 `useDirtyForm` Hook

```tsx
function useDirtyForm<T>(initialValues: T, currentValues: T, options?: {
  warnBeforeUnload?: boolean;
}): {
  isDirty: boolean;
  dirtyFields: Record<string, boolean>;
  setInitialState: (values: T) => void;
  reset: () => void;
}
```

**Effort**: S (~60 lines)

### 3.3 `usePreflightGate` Hook (see 1.5)

### 3.4 `<FormField>` Component

```tsx
interface FormFieldProps {
  name: string;
  label: string;
  required?: boolean;
  error?: string;
  touched?: boolean;
  children: ReactNode;
}
```

- Label with `htmlFor`, required asterisk (`aria-hidden="true"`)
- Injects `aria-describedby` + `aria-invalid` into child via `cloneElement`
- Error message: `role="alert"`, icon, red text, `min-h-[1.25rem]` space reservation

**Effort**: S (~50 lines)

### 3.5 `<FormErrorSummary>` Component

```tsx
interface FormErrorSummaryProps {
  errors: Record<string, string>;
  touched?: Record<string, boolean>;
  autoFocus?: boolean;
}
```

- Top of form on validation failure
- Each error is a clickable button that focuses the field
- `role="alert"` + auto-focus for screen reader

**Effort**: S (~60 lines)

### 3.6 Integrate Validation into All Forms

Per-page: import hooks + FormField + FormErrorSummary → define schema → wrap fields → wire onBlur/onSubmit.

Affected: CourseCreate, CourseEdit, TeacherCreate, SubjectCreate, Classrooms, Users, Login, Schedule modals

**Effort**: M (8 pages)

### 3.7 Add Loading Skeletons

Replace `"Loading…"` with `<LoadingSkeleton type="table">` on 13 list pages.

**Effort**: S

### 3.8 Focus Management

Implemented in Modal (auto-focus, return focus) + FormErrorSummary (auto-focus on error).

**Effort**: S

---

## Phase 4 — Responsiveness

### 4.1 Navigation Responsive (Layout.tsx)

`<768px` → hamburger toggle → off-canvas slide-out menu.
- `useState` for `menuOpen`
- Mobile: lucide `Menu` icon → stacked vertical nav with `min-h-[44px]` touch targets
- Desktop (`md:`): current horizontal layout

**Effort**: M

### 4.2 Table Horizontal Scroll

Wrap every `<table>` in `<div className="overflow-x-auto">` across 15 files.

**Effort**: S

### 4.3 Touch Targets ≥ 44px

Centralized in `<Button>` component: add `min-h-[44px]` on mobile via responsive padding.

**Effort**: S

### 4.4 Fixed-Width Search Inputs

`w-[380px]`/`w-72`/`w-56` → `w-full max-w-sm`

**Effort**: XS

### 4.5 Responsive Create Form Grids

`grid-cols-[120px_1fr]` → `grid-cols-1 sm:grid-cols-[120px_1fr]` (CourseCreate, SubjectCreate, TeacherCreate)

**Effort**: XS

### 4.6 Modal Body Scroll

Modal body: `max-h-[60vh] overflow-y-auto`

**Effort**: XS

---

## Phase 5 — Polish

### 5.1 Modal Enter/Exit Animation

CSS `@keyframes` for fade+scale entrance. Not framer-motion (avoid runtime overhead).

**Effort**: S

### 5.2 Button Hover Transitions

Handled by `<Button>` base class `transition-colors duration-150`. Included in Phase 0.

### 5.3 TypeaheadSelect Dropdown Animation

`animate-dropdown-enter`: fade + `translateY(-4px)` → `translateY(0)`.

**Effort**: XS

### 5.4 Toast Enter/Exit Animation

Slide from right (translateX). CSS `@keyframes` + `animation-fill-mode: forwards`.

**Effort**: S

---

## Implementation Order & Dependencies

### Parallel Tracks

| Track | Phase 0 | Then |
|-------|---------|------|
| A: Components | 0.2–0.8 (Input, Select, Button, PageHeading, SearchInput, EmptyState, Skeleton) | → Phase 2 (sweep pages) |
| B: Accessibility | 0.1 (tokens) | → 1.1, 1.2, 1.3, 1.4, 1.6 |
| C: UX bugs | — | → 1.5, 1.7 |
| D: Forms | — | → Phase 3 (after A) |
| E: Responsive | — | → Phase 4 |
| F: Polish | — | → Phase 5 |

Tracks A+B start immediately in parallel. C independent. D starts after A. E+F after A or independently.

### Effort Summary

| Phase | Items | Effort | Track |
|-------|-------|--------|-------|
| 0: Design System | 8 components | 2 days | A |
| 1: Critical Blockers | 7 items | 2 days | B, C |
| 2: Visual Consistency | 8 items | 1.5 days | A |
| 3: Form UX | 8 items | 3 days | D |
| 4: Responsiveness | 6 items | 2 days | E |
| 5: Polish | 4 items | 1 day | F |

**Total: ~11.5 days** (parallel → ~6–7 calendar days)

---

## Schedule.tsx Decomposition (2111→~600 lines)

Extract during Phase 2–3:

| Step | Extraction | Effort | Dependencies |
|------|-----------|--------|-------------|
| 1 | `ConfirmModal` (replace window.confirm) | XS | None |
| 2 | `ScheduleFilters` (filter bar) | XS | Button component |
| 3 | `useInstituteMeta`, `useLookups` hooks | XS | None |
| 4 | `SessionActions` (shared between week+table) | XS | Button component |
| 5 | `SessionCard`, `SessionRow`, `WeekView`, `TableView` | S | SessionActions |
| 6 | `AttendancePanel` (modal content) | S | Button/Input/Select |
| 7 | `useCreateSession`, `useEditSession`, `useCancelSeries` hooks | S | usePreflight |
| 8 | `SessionOccurrenceForm` | S | Input/Select/FormField |
| 9 | `SeriesFormFields` (weekday+time+count) | M | Input/FormField |
| 10 | `useSeriesCreate`, `useSeriesEdit` hooks | L | usePreflightGate |

**File structure after decomposition:**
```
src/pages/Schedule.tsx                              ← orchestrator (~150 lines)
src/pages/components/schedule/
  ScheduleFilters.tsx
  WeekView.tsx + SessionCard.tsx
  TableView.tsx + SessionRow.tsx
  SessionActions.tsx
  SessionOccurrenceForm.tsx
  SeriesFormFields.tsx
  AttendancePanel.tsx
  ConfirmModal.tsx
  CancelSeriesModal.tsx
src/hooks/
  useInstituteMeta.ts
  useLookups.ts
  useCreateSession.ts
  useEditSession.ts
  useSeriesCreate.ts
  useSeriesEdit.ts
  useCancelSeries.ts
  useAttendanceModal.ts
```

---

## Risk Areas

1. **Series edit hooks (T&F vs entire)**: 80% form shape shared but different endpoints, field mutability, stale-edit recovery. Recommend keeping 2 modal shells, sharing only `SeriesFormFields`.
2. **Stale-edit recovery**: Duplicated 5x in Schedule.tsx. Unify into utility function during hook extraction.
3. **Preflight race condition**: Button gating + handler guard both needed. Handler guard stays as defense-in-depth.
4. **`color-mix(oklab)` in ScheduleSessionCard.tsx**: Limited browser support. Add fallback during Phase 2.
5. **Concurrent editing risk**: Multiple agents editing same pages (e.g., Schedule.tsx touched by Phase 1, 2, 3). Sequence carefully.
