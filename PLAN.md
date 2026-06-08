---
phase: ux-refresh
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - src/index.css
  - src/pages/Absences.tsx
  - src/pages/AbsenceDetail.tsx
autonomous: true
requirements: []

must_haves:
  truths:
    - "Absence inbox table shows 8 columns with initials avatars in Student column"
    - "On screens ≤768px, inbox table rows become stacked cards (CSS-only transform)"
    - "Export CSV button lives in the header area, not inside filter grid"
    - "Selection bar enters/exits with animated height/opacity transition"
    - "Empty state shows a centered large SVG icon instead of table row text"
    - "Absence detail page uses max-w-6xl layout (was max-w-4xl)"
    - "Notes and Timeline sections stack vertically (not side-by-side)"
    - "Timeline items connected by vertical CSS line via ::before pseudo-element"
    - "Cancel modal has structured category dropdown with optional detail textarea"
    - "Override sit-in modal uses size='xl' (was 'lg')"
    - "Student profile header shows large initials avatar circle next to name"
    - "No duplicated mobile bottom action bar on detail page"
  artifacts:
    - path: "src/index.css"
      provides: "Responsive table-to-card breakpoint, timeline connecting line, selection bar animation, initials avatar utility, data-label card layout"
      contains: "@media (max-width: 768px)"
    - path: "src/pages/Absences.tsx"
      provides: "8-column table with responsive card transform, avatar, group-hover actions, animated selection bar, moved Export CSV, SVG empty state"
    - path: "src/pages/AbsenceDetail.tsx"
      provides: "Wider layout, stacked sections, profile avatar, CSS timeline lines, structured cancel modal dropdown, xl override modal"
  key_links:
    - from: "src/pages/Absences.tsx"
      to: "src/index.css"
      via: "responsive-table class on table wrapper, data-label attributes on td, animate-selection-enter/exit classes"
      pattern: "responsive-table|animate-selection|data-label|initials-avatar"
    - from: "src/pages/AbsenceDetail.tsx"
      to: "src/index.css"
      via: "initials-avatar initials-avatar-lg classes, timeline-line class on li"
      pattern: "initials-avatar|timeline-line"
---

<objective>
Refresh the Absence Inbox (Absences.tsx) and Absence Detail (AbsenceDetail.tsx) pages with CSS-first responsive transformations — no JS-based breakpoint detection, no dynamic style computation, no window resize listeners.

Purpose: Deliver a production-grade mobile experience where table-to-card transforms, timeline connecting lines, action visibility, and selection bar animations are driven entirely by CSS media queries, pseudo-elements, and Tailwind group-hover utilities.

Output:
- `src/index.css` — responsive table-to-card breakpoint styles, timeline connecting line, selection bar animations, initials avatar utility classes, data-label card layout
- `src/pages/Absences.tsx` — 8-column table (removed Email, Nickname, Submitted), initials avatar, group-hover secondary actions, moving Export CSV to header, empty state with SVG, animated selection bar
- `src/pages/AbsenceDetail.tsx` — max-w-6xl, stacked Notes+Timeline, profile avatar header, CSS timeline lines, category-dropdown cancel modal, size="xl" override modal
</objective>

<execution_context>
The plan targets three files. Execution order: Task 1 (CSS foundation) → Tasks 2 + 3 (components, parallel-safe as they modify different files).

Tailwind v4 is in use (no tailwind.config.js — theme via `@theme` in index.css). All custom CSS uses the `@media` breakpoint at 768px for the table-to-card transform.
</execution_context>

<context>
@src/index.css
@src/pages/Absences.tsx
@src/pages/AbsenceDetail.tsx
@src/components/absences/KanbanView.tsx (lines 35-37: existing `initials()` helper)
@src/components/ui/EmptyState.tsx
@src/components/Modal.tsx (size="xl" already supported in sizeMap)
@src/types/index.ts (ManagedAbsence, AbsenceStatus types)

<interfaces>
Existing `initials()` in KanbanView.tsx (line 35-37):
```tsx
function initials(name: string): string {
  return name.split(" ").map((part) => part.charAt(0)).join("").toUpperCase().slice(0, 2);
}
```

Modal sizeMap from Modal.tsx:
```tsx
const sizeMap: Record<ModalSize, string> = {
  sm: "max-w-sm", md: "max-w-md", lg: "max-w-lg",
  xl: "max-w-xl", full: "max-w-4xl",
};
```
</interfaces>
</context>

<tasks>

<task type="auto" wave="1">
  <name>Task 1: CSS foundation — responsive table-to-card, timeline lines, selection animation, avatar utility</name>
  <files>src/index.css</files>
  <action>
    Append the following blocks to `src/index.css` (after the existing `@media print` block at line 331-334):

    **A. Utility function for initials (shared)**

    Extract the `initials()` function from KanbanView.tsx (line 35-37) into a shared utility at `src/utils/string.ts`:

    ```ts
    export function getInitials(name?: string | null): string {
      if (!name || !name.trim()) return "?";
      return name.trim().split(/\s+/).map((part) => part.charAt(0)).join("").toUpperCase().slice(0, 2);
    }
    ```

    Also add `export type { /* nothing extra */ }` so the file has a clean export.

    **B. Initials avatar utility classes**

    Add to index.css:
    ```css
    /* Initials avatar circle */
    .initials-avatar {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 36px;
      height: 36px;
      border-radius: 50%;
      background-color: var(--color-wi-primary);
      color: white;
      font-weight: 700;
      font-size: 0.8125rem;
      line-height: 1;
      flex-shrink: 0;
      user-select: none;
    }
    .initials-avatar-lg {
      width: 56px;
      height: 56px;
      font-size: 1.25rem;
    }
    ```

    **C. Selection bar animation**

    Add to index.css:
    ```css
    @keyframes selection-enter {
      from { max-height: 0; opacity: 0; transform: translateY(-4px); padding-top: 0; padding-bottom: 0; margin-bottom: 0; }
      to   { max-height: 48px; opacity: 1; transform: translateY(0); }
    }
    .animate-selection-enter {
      animation: selection-enter 200ms ease-out forwards;
      overflow: hidden;
    }
    ```

    Also add a complementary exit keyframe:
    ```css
    @keyframes selection-exit {
      from { max-height: 48px; opacity: 1; }
      to   { max-height: 0; opacity: 0; padding-top: 0; padding-bottom: 0; margin-bottom: 0; }
    }
    .animate-selection-exit {
      animation: selection-exit 200ms ease-in forwards;
      overflow: hidden;
    }
    ```

    **D. Timeline connecting line styles**

    Add to index.css:
    ```css
    /* Timeline vertical connecting line */
    .timeline-line {
      position: relative;
    }
    .timeline-line::before {
      content: '';
      position: absolute;
      left: 7px;          /* center of 16px icon (h-4 w-4) */
      top: 26px;          /* below icon: 2px mt-0.5 + 16px icon + 8px gap */
      bottom: -8px;       /* extend into padding-bottom of li */
      width: 2px;
      background-color: #e2e8f0;  /* gray-200 */
      pointer-events: none;
    }
    .timeline-line:last-child::before {
      display: none;
    }
    ```

    **E. Responsive table-to-card transform at 768px**

    Add to index.css:
    ```css
    @media (max-width: 768px) {
      /* Flatten table layout to block */
      .responsive-table table,
      .responsive-table thead,
      .responsive-table tbody,
      .responsive-table tr,
      .responsive-table th,
      .responsive-table td {
        display: block;
      }

      /* Hide header accessibly (screen-reader accessible) */
      .responsive-table thead {
        position: absolute;
        width: 1px; height: 1px; padding: 0; margin: -1px;
        overflow: hidden; clip: rect(0,0,0,0); white-space: nowrap;
        border: 0;
      }

      /* Each row becomes a card */
      .responsive-table tr {
        border: 1px solid var(--color-wi-border);
        border-radius: var(--radius-sm);
        margin-bottom: 12px;
        padding: 16px;
        position: relative;
        background-color: white;
        box-shadow: 0 1px 2px rgba(0,0,0,0.04);
      }
      .responsive-table tbody tr:hover {
        background-color: white;  /* override global tr:hover which uses var(--color-wi-row-alt) */
      }

      /* Each td becomes a labelled row */
      .responsive-table td {
        border: none;
        padding: 6px 0;
        display: flex;
        align-items: flex-start;
        gap: 8px;
      }

      /* Data label via ::before — value comes from data-label attribute */
      .responsive-table td::before {
        content: attr(data-label);
        font-weight: 600;
        font-size: 0.7rem;
        text-transform: uppercase;
        letter-spacing: 0.05em;
        color: var(--color-wi-text-light);
        width: 80px;
        flex-shrink: 0;
        padding-top: 1px;
      }

      /* Checkbox cell: absolute top-right, no label */
      .responsive-table td:first-child {
        position: absolute;
        top: 12px;
        right: 12px;
        padding: 0;
        z-index: 1;
      }
      .responsive-table td:first-child::before {
        display: none;
      }

      /* Actions cell: full width, no label, top border separator */
      .responsive-table td:last-child {
        padding-top: 12px;
        margin-top: 8px;
        border-top: 1px solid var(--color-wi-border);
        flex-wrap: wrap;
        justify-content: flex-start;
      }
      .responsive-table td:last-child::before {
        display: none;
      }
      .responsive-table td:last-child > div {
        width: 100%;
        display: flex;
        gap: 0.25rem;
        flex-wrap: wrap;
      }
    }
    ```

    **F. Empty state SVG styling**

    Add to index.css:
    ```css
    .empty-state-icon {
      color: #cbd5e1;  /* gray-300 */
    }
    ```

    **G. Remove the global `tr:hover` background when inside responsive-table**

    The existing rule at line 74-76 (`tr:hover { background-color: var(--color-wi-row-alt); }`) will interfere with the card layout on mobile. Add an override specifically scoped to `.responsive-table`:

    ```css
    .responsive-table tr:hover {
      background-color: white;
    }
    ```

    This overrides the broader `tr:hover` rule due to higher specificity, ensuring row highlight only applies to non-responsive tables.

    Verify the file is syntactically valid by checking that all `@keyframes`, `@media` blocks, and selector groups are well-formed.
  </action>
  <verify>
    <automated>npx tailwindcss --input src/index.css --output /dev/null 2>&1</automated>
    <check>Verify the CSS file parses: check that no `@media` or `@keyframes` block is missing a closing brace.</check>
  </verify>
  <done>
    `src/index.css` has the new blocks appended, `src/utils/string.ts` exists with exported `getInitials()`, and the CSS compiles without errors through Tailwind v4's parser.
  </done>
</task>

<task type="auto" wave="2">
  <name>Task 2: Absences.tsx — 8-column table, initials avatar, responsive card classes, action visibility, selection animation, CSV position, SVG empty state</name>
  <files>src/pages/Absences.tsx</files>
  <action>
    Perform these changes to `src/pages/Absences.tsx` (546 lines). Each subsection is an exact edit.

    **2a. Add import for shared `getInitials`**

    Add at line 3 (after existing lucide-react import):
    ```tsx
    import { getInitials } from "../utils/string";
    ```

    Also add import for `SearchX` icon:
    ```tsx
    import { Download, Eye, LayoutGrid, RefreshCcw, SearchX, Table2 } from "lucide-react";
    ```

    **2b. Reduce columns from 11 to 8**

    In the `<thead>` section (lines 419-433), replace:
    ```tsx
    <tr className="text-left text-gray-500">
      <th className="w-8">
        <input aria-label="Select all absences" type="checkbox" checked={allSelected} onChange={(event) => setSelected(event.target.checked ? new Set(items.map((item) => item.id)) : new Set())} />
      </th>
      <th>Status</th>
      <th>Student</th>
      <th>Email</th>
      <th>Nickname</th>
      <th>Subject</th>
      <th>Dates</th>
      <th>Sit-in</th>
      <th>Reason</th>
      <th>Submitted</th>
      <th className="text-right">Actions</th>
    </tr>
    ```

    With (removing Email, Nickname, Submitted headers):
    ```tsx
    <tr className="text-left text-gray-500">
      <th className="w-8">
        <input aria-label="Select all absences" type="checkbox" checked={allSelected} onChange={(event) => setSelected(event.target.checked ? new Set(items.map((item) => item.id)) : new Set())} />
      </th>
      <th>Status</th>
      <th>Student</th>
      <th>Subject</th>
      <th>Dates</th>
      <th>Sit-in</th>
      <th>Reason</th>
      <th className="text-right">Actions</th>
    </tr>
    ```

    Also update the empty row `colSpan` from 11 to 8 (line 477):
    ```tsx
    <td colSpan={8}>
    ```

    **2c. Add `group` class to each data `<tr>` and add `responsive-table` wrapper**

    Change the table wrapper div (line 416) from:
    ```tsx
    <div className="overflow-x-auto rounded-sm border border-gray-200 bg-white">
    ```
    To:
    ```tsx
    <div className="overflow-x-auto rounded-sm border border-gray-200 bg-white responsive-table max-md:overflow-visible max-md:border-none max-md:bg-transparent">
    ```

    Change the `<table>` (line 417) from:
    ```tsx
    <table className="min-w-[1060px] text-sm">
    ```
    To:
    ```tsx
    <table className="min-w-[1060px] text-sm max-md:min-w-0 w-full">
    ```

    Change the `<tr>` on data rows (line 437) from:
    ```tsx
    <tr key={absence.id} className="cursor-pointer" onClick={...}>
    ```
    To:
    ```tsx
    <tr key={absence.id} className="cursor-pointer group max-md:cursor-default" onClick={...}>
    ```

    **2d. Add data-label attributes to each `<td>` and update the Student cell with initials avatar**

    Replace the existing cells in the table body (lines 438-472) with the following structure. Each `<td>` gets a `data-label` attribute matching its column header (uppercased shorthand matching the CSS `::before` content pattern).

    For each row `items.map((absence) => ...)`:

    - Checkbox cell (line 438-445): Add `data-label=""` (empty, CSS hides the label):
      ```tsx
      <td data-label="" onClick={(event) => event.stopPropagation()}>
      ```

    - Status cell (line 446): 
      ```tsx
      <td data-label="Status"><StatusBadge status={absence.status} /></td>
      ```

    - Student cell (lines 447-450): Replace with avatar + name:
      ```tsx
      <td data-label="Student">
        <div className="flex items-center gap-3 max-md:gap-2.5">
          <span className="initials-avatar" aria-hidden="true">{getInitials(absence.student_name)}</span>
          <div className="min-w-0">
            <Link className="font-medium text-[var(--color-wi-primary)] hover:underline truncate block" to={`/absences/${absence.id}`} aria-label={`View ${absence.student_name ?? absence.wcode} absence`} onClick={(event) => event.stopPropagation()}>{absence.student_name ?? "Unknown student"}</Link>
            <div className="font-mono text-xs text-gray-500 truncate">{absence.wcode}</div>
          </div>
        </div>
      </td>
      ```

    - Remove Email cell (was line 451) — delete entirely.
    - Remove Nickname cell (was line 452) — delete entirely.

    - Subject cell (was line 453, re-indexed):
      ```tsx
      <td data-label="Subject"><span className="rounded-sm bg-slate-100 px-1.5 py-0.5 text-xs font-semibold">{absence.subject_name ?? absence.subject_code ?? "-"}</span></td>
      ```

    - Dates cell (was line 454):
      ```tsx
      <td data-label="Dates" className="whitespace-pre-line align-top text-gray-700">{formatAbsenceSummaryDates(absence)}</td>
      ```

    - Sit-in cell (was lines 455-461):
      ```tsx
      <td data-label="Sit-in">
        {absence.sit_in_method === "zoom" ? (
          <span className="rounded-sm bg-blue-50 px-2 py-1 text-xs text-blue-700">Zoom</span>
        ) : (
          <span className="rounded-sm bg-emerald-50 px-2 py-1 text-xs text-emerald-700">{formatSitInLabel(absence)}{absence.sit_ins?.length ? ` (${absence.sit_ins.length})` : ""}</span>
        )}
      </td>
      ```

    - Reason cell (was line 462):
      ```tsx
      <td data-label="Reason" className="max-w-[140px] truncate text-gray-600">{absence.reason_category ?? absence.reason ?? "-"}</td>
      ```

    - Remove Submitted cell (was line 463) — delete entirely.

    - Actions cell (was lines 464-472) — restructure for primary/secondary visibility:
      ```tsx
      <td data-label="" onClick={(event) => event.stopPropagation()}>
        <div className="flex justify-end gap-1 items-center">
          <Link to={`/absences/${absence.id}`} aria-label={`Open details for ${absence.wcode}`}
                className="inline-flex min-h-[28px] items-center rounded-sm px-2 text-xs font-medium text-[var(--color-wi-primary)] hover:bg-blue-50">
            <Eye className="mr-1 h-3.5 w-3.5" /> View
          </Link>
          <div className="hidden group-hover:flex max-md:flex gap-1 items-center">
            {absence.status === "pending" ? <Button size="sm" loading={reviewing === absence.id} onClick={() => void setStatus(absence, "reviewed")}>Mark Reviewed</Button> : null}
            {absence.status === "reviewed" ? <Button size="sm" loading={reviewing === absence.id} onClick={() => void setStatus(absence, "actioned")}>Actioned</Button> : null}
            {absence.status !== "cancelled" ? <Button size="sm" variant="ghost" onClick={() => { setCancelTargets([absence]); setCancelReasonCategory(""); setCancelReasonDetail(""); }}>Cancel</Button> : null}
            <Button size="sm" variant="ghost" className="text-red-600 hover:bg-red-50" onClick={() => setDeleteTarget(absence)}>Delete</Button>
          </div>
        </div>
      </td>
      ```

    The key pattern for action visibility: the secondary actions div uses `hidden group-hover:flex max-md:flex` — hidden on desktop until row hover, always flex on mobile.

    **2e. Move Export CSV button from filter grid to header area**

    In the header actions section (line 348-362), remove the Settings link and add Export CSV:

    Replace the header action buttons block (lines 348-362) with:
    ```tsx
    <div className="flex flex-wrap gap-2">
      <div className="flex rounded-sm border border-gray-300 bg-white text-sm">
        <button onClick={() => setViewMode("table")} className="flex items-center gap-1 px-3 py-1.5 bg-gray-100 text-gray-900 font-medium"><Table2 className="h-4 w-4" /> Table</button>
        <button onClick={() => setViewMode("board")} className="flex items-center gap-1 px-3 py-1.5 text-gray-500 hover:text-gray-900"><LayoutGrid className="h-4 w-4" /> Board</button>
      </div>
      <Link to="/absences/dashboard" className="inline-flex min-h-[34px] items-center rounded-sm border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium hover:bg-gray-50">Dashboard</Link>
      <Button variant="secondary" onClick={() => void exportCsv()}><Download className="mr-1.5 h-4 w-4" />Export CSV</Button>
      <Button variant="secondary" onClick={() => setRefreshToken((value) => value + 1)}><RefreshCcw className="mr-1.5 h-4 w-4" /> Refresh</Button>
    </div>
    ```

    Remove the Export CSV button from the filter grid (line 381):

    Change the filter grid from (line 366):
    ```tsx
    <div className="grid gap-3 md:grid-cols-[minmax(200px,2fr)_1fr_1fr_1fr_1fr_auto]">
    ```
    To:
    ```tsx
    <div className="grid gap-3 md:grid-cols-[minmax(200px,2fr)_1fr_1fr_1fr_1fr]">
    ```

    And remove the line:
    ```tsx
    <Button variant="secondary" onClick={exportCsv}><Download className="mr-1.5 h-4 w-4" />Export CSV</Button>
    ```

    **2f. Selection bar with animated enter/exit**

    Replace the selection bar (lines 385-398) and the batch progress bar (lines 400-407):

    Current selection bar:
    ```tsx
    {selected.size > 0 ? (
      <div className="mb-3 flex items-center gap-3 rounded-sm border border-blue-100 bg-blue-50 px-3 py-2 text-sm">
        <span className="font-medium text-blue-800">{selected.size} selected</span>
        <Button size="sm" onClick={() => void markSelectedReviewed()} loading={batchProcessing}>
          {batchProcessing ? `Processing ${batchProgress.done}/${batchProgress.total}...` : "Mark Reviewed"}
        </Button>
        <Button size="sm" variant="secondary" onClick={() => void exportSelected()}>Export Selected</Button>
        <Button size="sm" variant="danger" onClick={() => {
          setCancelTargets(items.filter((item) => selected.has(item.id) && item.status !== "cancelled"));
          setCancelReasonCategory("");
          setCancelReasonDetail("");
        }}>Cancel Selected</Button>
      </div>
    ) : null}
    ```

    Replace with:
    ```tsx
    {selected.size > 0 || batchFailed.length > 0 ? (
      <div className={`mb-3 overflow-hidden ${selected.size > 0 ? 'animate-selection-enter' : 'animate-selection-exit'}`} key={selected.size > 0 ? 'sel-open' : 'sel-closed'}>
        <div className="flex items-center gap-3 rounded-sm border border-blue-100 bg-blue-50 px-3 py-2 text-sm">
          <span className="font-medium text-blue-800">{selected.size} selected</span>
          <Button size="sm" onClick={() => void markSelectedReviewed()} loading={batchProcessing}>
            {batchProcessing ? `Processing ${batchProgress.done}/${batchProgress.total}...` : "Mark Reviewed"}
          </Button>
          <Button size="sm" variant="secondary" onClick={() => void exportSelected()}>Export Selected</Button>
          <Button size="sm" variant="danger" onClick={() => {
            setCancelTargets(items.filter((item) => selected.has(item.id) && item.status !== "cancelled"));
            setCancelReasonCategory("");
            setCancelReasonDetail("");
          }}>Cancel Selected</Button>
        </div>
      </div>
    ) : null}
    ```

    The React `key` prop (`'sel-open'` / `'sel-closed'`) forces a remount each time selection state changes, ensuring the `animation` plays fresh on every enter/exit. On initial mount with selection, the animation runs once.

    **2g. Empty state with SVG icon**

    Replace the table-row empty state (lines 475-484) which currently embeds `<EmptyState>` inside a `<td colSpan={8}>`. Instead, place it after the table wrapper when items are empty:

    Remove lines 475-484 entirely (the `<tr>` empty state).

    After the table wrapper's closing `</div>` (line 487), add:
    ```tsx
    {items.length === 0 && !loading ? (
      <div className="py-16 text-center">
        <SearchX className="mx-auto mb-4 h-16 w-16 text-gray-300" aria-hidden="true" />
        <p className="text-gray-400 text-sm mb-4">All caught up! No absences match these filters.</p>
        <div className="flex justify-center gap-2">
          <Link to="/absences" className="text-sm text-[var(--color-wi-primary)] hover:underline">View all</Link>
          <Link to="/absences/dashboard" className="text-sm text-[var(--color-wi-primary)] hover:underline">View dashboard</Link>
        </div>
      </div>
    ) : null}
    ```

    This moves the empty state outside the table so it's not constrained by table layout, and uses the Lucide `SearchX` icon at h-16 w-16 (64px) in gray-300.

    **2h. Move the `loading` guard to also hide the empty state**

    No change needed — the `items.length === 0 && !loading` check already prevents showing the empty state while loading.
  </action>
  <verify>
    <automated>grep -c 'colSpan={8}\|responsive-table\|getInitials\|initials-avatar\|group-hover\|animate-selection-enter\|SearchX' src/pages/Absences.tsx</automated>
    <check>Visually verify on desktop: 8 columns, avatar in Student column, Export CSV in header, selection bar animates, empty state shows SearchX icon.</check>
    <check>Check on mobile (≤768px): table rows become stacked cards with data-label labels, actions visible, no overflow scroll.</check>
  </verify>
  <done>
    Absences.tsx renders an 8-column table with initials avatars, Export CSV in header, CSS-driven card layout on mobile, group-hover secondary actions, animated selection bar, and SVG-backed empty state.
  </done>
</task>

<task type="auto" wave="2">
  <name>Task 3: AbsenceDetail.tsx — wide layout, stacked sections, profile avatar, CSS timeline lines, structured cancel modal, xl override</name>
  <files>src/pages/AbsenceDetail.tsx</files>
  <action>
    Perform these changes to `src/pages/AbsenceDetail.tsx` (416 lines). Each subsection is an exact edit.

    **3a. Add imports**

    Add at the top after existing imports (line 2):
    ```tsx
    import { getInitials } from "../utils/string";
    ```

    **3b. Add `cancelReasonDetail` state**

    After the existing `cancelReason` state (line 97), add:
    ```tsx
    const [cancelReasonDetail, setCancelReasonDetail] = useState("");
    ```

    **3c. Widen layout from max-w-4xl to max-w-6xl**

    Line 212:
    ```tsx
    <div className="mx-auto max-w-4xl">
    ```
    → 
    ```tsx
    <div className="mx-auto max-w-6xl">
    ```

    **3d. Remove the duplicated fixed mobile bottom bar**

    Delete entirely the fixed bottom bar section (lines 244-259):
    ```tsx
    <div className="fixed bottom-0 left-0 right-0 z-20 border-t border-gray-200 bg-white p-3 shadow-lg md:hidden">
      ...
    </div>
    ```

    **3e. Add student profile header with large initials avatar**

    Replace the current header section (lines 220-227):
    ```tsx
    <div className="mt-2 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 className="text-2xl font-semibold text-gray-900">Absence Detail</h1>
        <div className="mt-1 flex items-center gap-2 text-sm text-gray-600">
          <span className="font-medium">{absence.student_name ?? "Unknown"}</span>
          <span className="font-mono text-xs text-gray-400">{absence.wcode}</span>
        </div>
      </div>
      ...
    </div>
    ```

    With:
    ```tsx
    <div className="mt-4 flex flex-wrap items-start justify-between gap-4">
      <div className="flex items-center gap-4">
        <span className="initials-avatar initials-avatar-lg" aria-hidden="true">{getInitials(absence.student_name)}</span>
        <div>
          <h1 className="text-2xl font-semibold text-gray-900">Absence Detail</h1>
          <div className="mt-1 flex items-center gap-2 text-sm text-gray-600">
            <span className="font-medium">{absence.student_name ?? "Unknown"}</span>
            <span className="font-mono text-xs text-gray-400">{absence.wcode}</span>
          </div>
        </div>
      </div>
    ```

    **3f. Update the desktop action bar `hidden md:flex` class**

    The desktop action bar (line 228) currently has:
    ```tsx
    <div className="sticky top-4 z-20 hidden md:flex ...">
    ```

    Keep this as-is (it remains the only action bar after removing the mobile one). The `hidden md:flex` means it's visible on md+ screens. On mobile, there's now no action bar — this is acceptable per the spec (the spec says "Remove duplicated mobile bottom bar", not "add a replacement").

    **3g. Remove `pb-16 md:pb-0` padding — no longer needed without fixed mobile bar**

    Line 261:
    ```tsx
    <div className="mt-4 grid gap-4 pb-16 md:pb-0">
    ```
    →
    ```tsx
    <div className="mt-4 grid gap-4">
    ```

    **3h. Stack Notes + Timeline vertically (remove md:grid-cols-2)**

    Line 311:
    ```tsx
    <div className="grid gap-4 md:grid-cols-2">
    ```
    →
    ```tsx
    <div className="grid gap-4">
    ```

    **3i. Upgrade Timeline with CSS connecting lines**

    Replace the Timeline section (lines 333-349):
    ```tsx
    <section className="rounded-sm border border-gray-200 bg-white">
      <h2 className="border-b border-gray-100 bg-gray-50/70 px-4 py-3 text-sm font-semibold text-gray-800">Timeline</h2>
      <div className="p-4">
        <ol className="space-y-3">
          {(absence.timeline ?? []).map((entry) => (
            <li key={entry.id} className="flex gap-3">
              <div className="mt-0.5 shrink-0">{TimelineIcon({ action: entry.action })}</div>
              <div>
                <p className="text-sm font-medium text-gray-800">{titleCase(entry.action)}</p>
                <p className="text-xs text-gray-500">{displayDateTime(entry.created_at)} &mdash; {entry.actor_name ?? entry.actor_role}</p>
              </div>
            </li>
          ))}
          {!absence.timeline?.length ? <li className="text-sm text-gray-500">No activity recorded.</li> : null}
        </ol>
      </div>
    </section>
    ```

    With:
    ```tsx
    <section className="rounded-sm border border-gray-200 bg-white">
      <h2 className="border-b border-gray-100 bg-gray-50/70 px-4 py-3 text-sm font-semibold text-gray-800">Timeline</h2>
      <div className="p-4">
        <ol className="relative">
          {(absence.timeline ?? []).map((entry) => (
            <li key={entry.id} className="timeline-line flex gap-3 pb-4 last:pb-0">
              <div className="mt-0.5 shrink-0 z-10 bg-white rounded-full">{TimelineIcon({ action: entry.action })}</div>
              <div className="min-w-0">
                <p className="text-sm font-medium text-gray-800">{titleCase(entry.action)}</p>
                <p className="text-xs text-gray-500">{displayDateTime(entry.created_at)} &mdash; {entry.actor_name ?? entry.actor_role}</p>
              </div>
            </li>
          ))}
          {!absence.timeline?.length ? <li className="text-sm text-gray-500">No activity recorded.</li> : null}
        </ol>
      </div>
    </section>
    ```

    Key changes:
    - `space-y-3` → no spacing class (managed by `pb-4` on each `li`)
    - `timeline-line` class on each `<li>` triggers the CSS `::before` connecting line
    - Icon div gets `z-10 bg-white rounded-full` to sit above the connecting line and hide it behind the icon
    - `last:pb-0` removes extra padding on the last item

    **3j. Cancel modal with structured category dropdown**

    Replace the cancel modal (lines 353-359):
    ```tsx
    {cancelOpen ? (
      <Modal title="Cancel absence" onClose={() => setCancelOpen(false)}
        footer={<><Button variant="secondary" onClick={() => setCancelOpen(false)}>Back</Button><Button variant="danger" disabled={!cancelReason.trim()} loading={saving} onClick={() => void updateStatus("cancelled", cancelReason.trim()).then(() => setCancelOpen(false))}>Cancel Absence</Button></>}>
        <label className="block text-sm font-medium text-gray-700" htmlFor="detail-cancel-reason">Reason</label>
        <textarea id="detail-cancel-reason" className="mt-2 w-full rounded-sm border border-gray-300 p-2" rows={3} value={cancelReason} onChange={(e) => setCancelReason(e.target.value)} />
      </Modal>
    ) : null}
    ```

    With:
    ```tsx
    {cancelOpen ? (
      <Modal title="Cancel absence" onClose={() => {
        setCancelOpen(false);
        setCancelReason("");
        setCancelReasonDetail("");
      }}
        footer={<>
          <Button variant="secondary" onClick={() => { setCancelOpen(false); setCancelReason(""); setCancelReasonDetail(""); }}>Back</Button>
          <Button variant="danger" disabled={!cancelReason} loading={saving} onClick={async () => {
            const reasonPayload = JSON.stringify({ category: cancelReason, detail: cancelReasonDetail });
            await updateStatus("cancelled", reasonPayload);
            setCancelOpen(false);
            setCancelReason("");
            setCancelReasonDetail("");
          }}>Cancel Absence</Button>
        </>}>
        <p className="mb-3 text-sm text-gray-600">Assigned sit-in sessions will be released. This action is retained in the audit timeline.</p>
        <label className="block text-sm font-medium text-gray-700" htmlFor="detail-cancel-category">Cancellation reason</label>
        <select id="detail-cancel-category" className="mt-1 w-full rounded-sm border border-gray-300 p-2 text-sm" value={cancelReason} onChange={(e) => setCancelReason(e.target.value)}>
          <option value="">Select a reason...</option>
          <option value="duplicate">Duplicate submission</option>
          <option value="student_requested">Student requested cancellation</option>
          <option value="admin_error">Admin error</option>
          <option value="incorrect_dates">Incorrect dates</option>
          <option value="other">Other</option>
        </select>
        <label className="mt-3 block text-sm font-medium text-gray-700" htmlFor="detail-cancel-detail">Additional details (optional)</label>
        <textarea id="detail-cancel-detail" className="mt-1 w-full rounded-sm border border-gray-300 p-2 text-sm" rows={3} value={cancelReasonDetail} onChange={(e) => setCancelReasonDetail(e.target.value)} />
      </Modal>
    ) : null}
    ```

    **3k. Override modal size from "lg" to "xl"**

    Line 362:
    ```tsx
    <Modal title="Override Sit-in" onClose={() => setOverrideOpen(false)} size="lg"
    ```
    →
    ```tsx
    <Modal title="Override Sit-in" onClose={() => setOverrideOpen(false)} size="xl"
    ```

    **3l. Verify file integrity**

    After all edits, confirm:
    - No duplicate React fragment keys
    - All JSX elements properly closed
    - The `cancelReasonDetail` state is referenced in both the cancel modal and the `onClose` handlers
    - No trailing JSX that would cause syntax errors
  </action>
  <verify>
    <automated>grep -c 'max-w-6xl\|initials-avatar-lg\|timeline-line\|cancelReasonDetail\|detail-cancel-category\|size="xl"' src/pages/AbsenceDetail.tsx</automated>
    <check>Visual: student profile shows large avatar circle, timeline items connected by vertical gray line, cancel modal has category dropdown with "Select a reason..." placeholder, override modal has wider width.</check>
  </verify>
  <done>
    AbsenceDetail.tsx renders at max-w-6xl width, shows student profile avatar, has stacked Notes+Timeline sections with CSS timeline lines, structured cancel category modal, size="xl" override modal, and no duplicated mobile bottom bar.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| CSS class injection | User-controlled content in `student_name` rendered as text inside `getInitials()` — no XSS vector since it's text content, not innerHTML |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-01 | Spoofing | CSS-only responsive transform | accept | CSS media queries cannot be spoofed by users; viewport-based and benign |
| T-02 | Tampering | `data-label` attributes | accept | Hardcoded string literals in JSX, not user-provided |
</threat_model>

<verification>

1. **CSS compilation**: `npx tailwindcss --input src/index.css --output /dev/null` must exit 0
2. **TypeScript compilation**: `npx tsc --noEmit src/pages/Absences.tsx src/pages/AbsenceDetail.tsx` must report zero type errors
3. **Component existence**: Both pages render without runtime errors on desktop and mobile viewports
4. **Responsive layout**: At 768px viewport width, the inbox table rows become stacked cards with visible `::before` label content from `data-label` attributes
5. **Avatars**: Initials circles render in both Student column (inbox) and profile header (detail)
6. **Export CSV**: Button appears in the header area, not inside the filter grid
7. **Selection bar**: When items are selected, selection bar animates in with vertical expansion
8. **Empty state**: When no items match filters, a 64px `SearchX` SVG icon + message replaces the table
9. **Timeline lines**: Detail page timeline shows vertical gray lines connecting each entry; last entry has no connector
10. **Cancel modal**: Detail page cancel modal shows a `<select>` with 5 category options plus an optional detail textarea
11. **Override modal**: Width matches `max-w-xl` (the xl size map value)

</verification>

<success_criteria>

- [ ] index.css has responsive breakpoint, timeline line, avatar utility, and selection animation blocks appended
- [ ] Absences.tsx renders 8 columns (checkbox, Status, Student, Subject, Dates, Sit-in, Reason, Actions)
- [ ] Every data-cell `<td>` has a `data-label` attribute
- [ ] Table wrapper has `responsive-table` class
- [ ] Each `<tr>` in the data body has `group` class
- [ ] Secondary action buttons are hidden on desktop until row hover (`hidden group-hover:flex max-md:flex`)
- [ ] Export CSV button is in header area, not in filter grid
- [ ] Selection bar uses `animate-selection-enter` / `animate-selection-exit` classes with React key-based remount
- [ ] Empty state uses Lucide `SearchX` at 64px outside the table
- [ ] AbsenceDetail.tsx has `max-w-6xl` (was max-w-4xl)
- [ ] AbsenceDetail.tsx no longer has the fixed mobile bottom action bar
- [ ] AbsenceDetail.tsx has `initials-avatar initials-avatar-lg` in student profile header
- [ ] Notes+Timeline wrapper has no `md:grid-cols-2`
- [ ] Timeline `<li>` elements use `timeline-line` class
- [ ] Cancel modal uses `<select>` with category options + detail textarea
- [ ] Override modal uses `size="xl"`
- [ ] `getInitials()` extracted to `src/utils/string.ts` and imported by both pages

</success_criteria>
