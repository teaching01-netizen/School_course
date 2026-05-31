# Absence Form Modern UX — Implementation Plan

## Goal
Modernise Step 2 ("Courses & Dates") of the absence form: replace the reason category dropdown with a free-text "Reason to leave" input (always visible, required), replace native date inputs with a modern popover date picker, support multiple date range slots with a + CTA, remove keyboard hint text and suggestion badges, and modernise overall calendar/date UI.

## Files to Modify
- `src/pages/AbsenceForm.tsx` — reason fields, date range section, keyboard hint removal
- `src/components/absences/DateRangeInput.tsx` — replace entirely with modern date picker + multiple slots
- `src/components/absences/CourseChip.tsx` — no changes needed (keyboard nav stays, hint text removed from parent)

## New Dependencies
- `react-day-picker` + `date-fns` — modern popover calendar component

---

## Task 1: Install Dependencies + Create DateRangeSlot Component

### 1a. Install packages
```bash
npm install react-day-picker date-fns
```

### 1b. Create `src/components/absences/DateRangeSlot.tsx`
A single date range slot with:
- Two popover date pickers (From date, To date) using `react-day-picker`
- Remove button (X icon) — only shown when >1 slot
- Framer Motion layout animation for add/remove

**Design:**
```
┌─────────────────────────────────────────────────────┐
│ 📅 From: [Click to select]  →  To: [Click to select]  │  [✕] │
└─────────────────────────────────────────────────────┘
```

**Props:**
```ts
interface DateRangeSlotProps {
  index: number
  fromDate: Date | undefined
  toDate: Date | undefined
  onFromChange: (date: Date | undefined) => void
  onToChange: (date: Date | undefined) => void
  onRemove: () => void
  canRemove: boolean
  maxDays: number
}
```

**Key details:**
- Popover calendar opens on click (not inline)
- From date picker: min=today, max= toDate or today+maxDays
- To date picker: min= fromDate or today, max=today+maxDays
- Display formatted date or "Select date" placeholder
- Remove button: `variant="ghost" size="sm"` with Trash2 icon, hidden when canRemove=false
- Responsive: stack on mobile, row on desktop

---

## Task 2: Create DateRangePicker (Multiple Slots Container)

### Create `src/components/absences/DateRangePicker.tsx`
Container that manages multiple `DateRangeSlot` instances:
- Renders list of DateRangeSlot components
- "+ Add another date range" CTA button at bottom
- Validates no overlapping ranges
- Manages state array of `{ from: Date, to: Date }` objects

**Props:**
```ts
interface DateRangePickerProps {
  value: DateRange[]
  onChange: (ranges: DateRange[]) => void
  maxDays: number
  error?: string
}

interface DateRange {
  from: Date | undefined
  to: Date | undefined
}
```

**Key details:**
- Initial state: one empty slot `[{ from: undefined, to: undefined }]`
- "+ Add" button: `variant="outline"`, CalendarPlus icon, disabled when last slot has empty dates
- Error display: red text below list
- Overlap validation: warn if ranges overlap
- Max total days across all ranges: validate against config limit

---

## Task 3: Replace Reason Category with Free Text

### Modify `src/pages/AbsenceForm.tsx`

**Remove:**
- `reasonCategory` state variable and its `onChange` handler
- The native `<select>` dropdown for reason categories
- The collapsible "Add reason details" toggle (`showReasonFields` state)
- The "Optional" badge
- The `config.form.reason_categories` mapping

**Add/Replace with:**
- Always-visible "Reason to leave" textarea (required, not optional)
- Label: "Reason to leave *"
- Same textarea styling as before (max 500 chars, visual progress bar, char count)
- Remove `config.form.require_reason` gating — always required
- Remove `config.form.allow_free_text_reason` check — always free text

**Validation changes:**
- `validateStepTwo()`: require `reason.trim().length > 0` (always)
- Remove `reasonCategory` from session storage save/restore
- Remove `reasonCategory` from confirmation summary

---

## Task 4: Replace DateRangeInput with DateRangePicker

### Modify `src/pages/AbsenceForm.tsx`

**Remove:**
- Import of old `DateRangeInput`
- `dateFrom` / `dateTo` state variables
- Old date range validation logic

**Add/Replace with:**
- Import new `DateRangePicker`
- `dateRanges` state: `DateRange[]` (array of `{ from, to }`)
- Derive `dateFrom` (min of all ranges) and `dateTo` (max of all ranges) for session fetching
- Update `canProceedToSessions` to check all ranges have valid from/to
- Update session fetch to use derived min/max dates
- Update validation: each range must have from ≤ to, total days across ranges ≤ max

**Session fetching:**
- Fetch sessions for the full span (min from → max to) across all ranges
- Filter displayed sessions to only those within any of the selected ranges

---

## Task 5: Delete Keyboard Hint + Suggestion Badges

### Modify `src/pages/AbsenceForm.tsx`
- Delete line 1179–1181: `"Use arrow keys to move, Space to toggle a course, and Enter to toggle the focused course."` text

### Old `DateRangeInput.tsx`
- Delete entire file (replaced by DateRangePicker)

---

## Task 6: Modernise Calendar Visual Design

### Global style updates in `AbsenceForm.tsx`:
- Date range slots: `rounded-lg border border-gray-200 bg-white p-4 shadow-sm` (softer corners)
- Popover calendar: modern shadow, rounded corners, clean grid
- "+ Add" button: `rounded-lg border-dashed border-2 border-gray-300` with hover effect
- Remove button: subtle `text-gray-400 hover:text-red-500` transition
- Session chips: keep existing styling (already clean)
- Heading hierarchy: consistent `text-lg font-semibold` for section titles

### react-day-picker custom theme:
- Use CSS variables for primary color (`var(--color-wi-primary)`)
- Clean hover states on calendar days
- Selected range highlight with primary color
- Today marker

---

## Verification
1. `npm run typecheck` — zero errors
2. `npm run test -- --run` — all 340+ tests pass
3. Manual: date picker popover opens, range selection works, multiple slots add/remove
4. Manual: reason textarea always visible, required validation triggers on empty
5. Manual: no suggestion badges, no keyboard hint text
6. Manual: calendar looks modern and polished
