# Absence Inbox & Detail UX Refresh — Interaction Patterns

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Polish every micro-interaction in the Absence Inbox table and Absence Detail page — hover states, reveal animations, timeline visual flow, modal UX, and responsive behavior — so the interface feels reactive, deliberate, and production-grade.

**Architecture:** Two independent sub-plans (no file overlap). Plan A touches only `Absences.tsx` + its tests. Plan B touches only `AbsenceDetail.tsx` + its tests. Both share new CSS keyframes in `src/index.css`. Wave 1 executes both plans in parallel.

**Tech Stack:** React 19 / TypeScript / Tailwind CSS v4 / Vitest + Testing Library.

**Existing animation primitives (from `src/index.css`):**
- `animate-modal-enter`: `scale(0.95) translateY(-8px)`→`scale(1)` over 200ms ease-out
- `animate-modal-overlay-enter`: `opacity 0→1` over 200ms ease-out
- `animate-fade-in`: `opacity 0→1` over 200ms ease-out
- `animate-dropdown-enter`: `translateY(-4px)`→`translateY(0)` over 150ms ease-out
- `transition-colors duration-150`: used on all Button variants
- `transition-all duration-300`: currently used on batch progress bar

---

## File Map

| File | Plan A | Plan B |
|------|--------|--------|
| `src/pages/Absences.tsx` | ✅ | — |
| `src/pages/AbsenceDetail.tsx` | — | ✅ |
| `src/components/absences/initials.ts` | ✅ (create) | — |
| `src/components/absences/KanbanView.tsx` | ✅ (import from new util) | — |
| `src/index.css` | ✅ | ✅ |
| `src/pages/__tests__/Absences.test.tsx` | ✅ | — |
| `src/pages/__tests__/Absence-cross-links.test.tsx` | ✅ | — |
| `src/pages/__tests__/AbsenceDetail.test.tsx` | — | ✅ |

---

## Plan A: Inbox Interaction Refinements

**Files modified:** `src/pages/Absences.tsx`, `src/components/absences/initials.ts` (create), `src/components/absences/KanbanView.tsx` (refactor import), `src/index.css`, `src/pages/__tests__/Absences.test.tsx`, `src/pages/__tests__/Absence-cross-links.test.tsx`

**Interaction behaviors targeted:** #1 (table hover/reveal), #2 (selection bar animation), #3 (batch progress), #4 (row click cursor), partially #7 (cancel modal structured categories — already done in inbox, just verify)

---

### Task A1: Column reduction, initials avatar, hover-reveal actions

**Behavioral target:** The table row becomes a deliberate interaction zone — cursor changes on entry, secondary actions fade in on hover, the student cell gets a visual anchor (initials avatar), and extraneous columns are removed.

**Modifies:** `src/pages/Absences.tsx`

- [ ] **Step 1: Extract `initials()` utility from KanbanView**

  Create `src/components/absences/initials.ts`:
  ```ts
  export function initials(name: string): string {
    return name.split(" ").map((part) => part.charAt(0)).join("").toUpperCase().slice(0, 2);
  }
  ```

  In `KanbanView.tsx`, replace the inline `initials()` function with:
  ```ts
  import { initials } from "./initials";
  ```
  Remove the old `function initials(name: string): string { ... }` block (lines 35-37).

- [ ] **Step 2: Reduce columns from 11 to 8**

  In `Absences.tsx`, find the `<thead>` block (lines 418-433). Replace the header row:
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
    <th>Submitted</th>
    <th className="text-right">Actions</th>
  </tr>
  ```

  Removed: `<th>Email</th>`, `<th>Nickname</th>`, `<th>Reason</th>`

- [ ] **Step 3: Add initials avatar to Student cell + remove email/nickname columns**

  In the `<tbody>` map (line 436 onward), change the `<td>` sequence. Replace lines 447-452:

  ```tsx
  <td>
    <div className="flex items-center gap-2.5">
      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-xs font-bold text-white">
        {initials(absence.student_name ?? absence.wcode)}
      </span>
      <div className="min-w-0">
        <Link className="font-medium text-[var(--color-wi-primary)] hover:underline" to={`/absences/${absence.id}`} aria-label={`View ${absence.student_name ?? absence.wcode} absence`} onClick={(event) => event.stopPropagation()}>{absence.student_name ?? "Unknown student"}</Link>
        <div className="font-mono text-xs text-gray-500">{absence.wcode}</div>
      </div>
    </div>
  </td>
  ```

  Remove the `<td>` for Email (currently line 451) and Nickname (line 452) — they are no longer rendered.

  Update the `colSpan` in the empty state row (line 477): change `colSpan={11}` to `colSpan={8}`.

  Add import at top of file:
  ```ts
  import { initials } from "../components/absences/initials";
  ```

- [ ] **Step 4: Group row as hover container + reveal Cancel/Delete on hover**

  Change the `<tr>` element (line 437) to use the `group` pattern:

  ```tsx
  <tr key={absence.id} className="group cursor-pointer" onClick={() => navigate(`/absences/${absence.id}`)}>
  ```

  Modify the actions `<td>` (lines 464-471) to:
  ```tsx
  <td onClick={(event) => event.stopPropagation()}>
    <div className="flex justify-end gap-1">
      <Link to={`/absences/${absence.id}`} aria-label={`Open details for ${absence.wcode}`} className="inline-flex min-h-[28px] items-center rounded-sm px-2 text-xs text-gray-700 hover:bg-gray-100"><Eye className="mr-1 h-3.5 w-3.5" /> View</Link>
      {absence.status === "pending" ? <Button size="sm" loading={reviewing === absence.id} onClick={() => void setStatus(absence, "reviewed")}>Mark Reviewed</Button> : null}
      {absence.status === "reviewed" ? <Button size="sm" loading={reviewing === absence.id} onClick={() => void setStatus(absence, "actioned")}>Actioned</Button> : null}
      <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity duration-150">
        {absence.status !== "cancelled" ? <Button size="sm" variant="ghost" onClick={() => { setCancelTargets([absence]); setCancelReasonCategory(""); setCancelReasonDetail(""); }}>Cancel</Button> : null}
        <Button size="sm" variant="ghost" className="text-red-600 hover:bg-red-50" onClick={() => setDeleteTarget(absence)}>Delete</Button>
      </div>
    </div>
  </td>
  ```

  Key change: Cancel + Delete are wrapped in `<div className="opacity-0 group-hover:opacity-100 transition-opacity duration-150">`. View + status buttons remain always visible outside this wrapper.

  **Row `tr:hover` already exists in `index.css` (line 74: `background-color: var(--color-wi-row-alt)`) — no additional CSS needed for row highlight.**

- [ ] **Step 5: Update test assertions for removed columns + avatar presence**

  **In `src/pages/__tests__/Absences.test.tsx`:**

  The test block starting at line 105 (`"renders missed session dates"`) checks row content via `row.closest("tr")`. The row now has an avatar span. The existing text checks (`1 Jun`, `8 Jun`, `SAT Math Scholar C2`) remain valid — no change needed.

  Test at line 263 (`"shows retry failed button"`) uses `getByText("2 selected").parentElement!` to find the batch bar — no change needed after column removal since the batch bar (lines 385-398) is unchanged.

  Test at line 308 (`"shows delete button for non-cancelled absences"`) finds `getByRole("button", { name: /delete/i })` — now the delete button is wrapped in a `group-hover:opacity-100` div. No change needed since `getByRole` finds it regardless of opacity. Same for test at line 317.

  No test assertions reference the removed columns (Email, Nickname, Reason) by label, so no test removal needed.

  **In `src/pages/__tests__/Absence-cross-links.test.tsx`:** No changes needed — no assertions reference removed columns.

---

### Task A2: Selection bar animation, batch progress easing, row click cursor

**Behavioral target:** The selection bar expands/shrinks with a height animation instead of appearing/disappearing instantly. The batch progress bar glides smoothly. Rows have intentional cursor feedback.

**Modifies:** `src/pages/Absences.tsx`, `src/index.css`

- [ ] **Step 1: Animated selection bar with height transition**

  Replace the conditional render at lines 385-398:

  **Before:**
  ```tsx
  {selected.size > 0 ? (
    <div className="mb-3 flex items-center gap-3 rounded-sm border border-blue-100 bg-blue-50 px-3 py-2 text-sm">
      ...
    </div>
  ) : null}
  ```

  **After:**
  ```tsx
  <div
    className="grid transition-all duration-300 ease-out"
    style={{ gridTemplateRows: selected.size > 0 ? '1fr' : '0fr' }}
  >
    <div className="overflow-hidden">
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
    </div>
  </div>
  ```

  This uses a CSS grid row track transition — when `selected.size > 0` is true, the track expands from 0fr to 1fr, creating a smooth height reveal. The `overflow-hidden` clips content during collapse.

  Alternative simpler approach (preferred for browser compatibility):
  ```tsx
  <div
    className={`mb-3 overflow-hidden transition-all duration-300 ease-out ${selected.size > 0 ? 'max-h-20 opacity-100' : 'max-h-0 opacity-0'}`}
    aria-hidden={selected.size === 0}
  >
    <div className="flex items-center gap-3 rounded-sm border border-blue-100 bg-blue-50 px-3 py-2 text-sm">
      ...
    </div>
  </div>
  ```
  Use this `max-h` approach — it's more widely supported and doesn't rely on grid animation.

- [ ] **Step 2: Smooth batch progress bar**

  The current progress bar (lines 400-407):
  ```tsx
  {batchProcessing ? (
    <div className="mb-3 overflow-hidden rounded-sm bg-gray-100">
      <div
        className="h-1.5 rounded-sm bg-blue-500 transition-all duration-300"
        style={{ width: `${batchProgress.total > 0 ? (batchProgress.done / batchProgress.total) * 100 : 0}%` }}
      />
    </div>
  ) : null}
  ```

  Replace with smoother animation:
  ```tsx
  {/* Batch progress — animated bar with deliberate easing */}
  <div
    className={`mb-3 overflow-hidden rounded-sm bg-gray-100 transition-all duration-300 ease-out ${batchProcessing ? 'max-h-4 opacity-100' : 'max-h-0 opacity-0'}`}
    aria-hidden={!batchProcessing}
  >
    <div className="h-1.5 rounded-sm transition-[width] duration-500 ease-out bg-blue-500"
      style={{ width: `${batchProgress.total > 0 ? (batchProgress.done / batchProgress.total) * 100 : 0}%` }}
    />
  </div>
  ```

  Key changes:
  - Bar wrapper gets height reveal via `max-h-4` / `max-h-0` transition
  - Inner bar uses `transition-[width] duration-500 ease-out` (longer duration, smoother easing)
  - Progress bar container background always visible for the "empty track" look

- [ ] **Step 3: Row click with deliberate cursor indication**

  The row already has `cursor-pointer` (line 437). Add a subtle hover lift via box-shadow transition:

  In `src/index.css`, add:
  ```css
  /* Row hover depth cue for clickable table rows */
  tbody tr {
    transition: box-shadow 150ms ease;
  }
  tbody tr:hover {
    box-shadow: inset 0 1px 0 var(--color-wi-border), inset 0 -1px 0 var(--color-wi-border);
  }
  ```

  This creates a subtle inset border glow on hover, reinforcing that the row is interactive without being visually loud.

  Note: The existing `tr:hover` background in `index.css` (line 74) applies to ALL tables. Since only the absences table has clickable rows, scope this to tbody only (already scoped with `tbody tr` selector above).

---

### Task A3: Responsive card view, export to header, empty state icon

**Behavioral target:** On narrow viewports, the table collapses into stacked info cards. Export moves from the filter bar to the header area alongside other actions. The empty state gets a purpose-built illustration.

**Modifies:** `src/pages/Absences.tsx`

- [ ] **Step 1: Wrap table in responsive container with card fallback**

  The table container already has `overflow-x-auto` (line 416). Add responsive behavior using `hidden md:block` / `block md:hidden` pattern:

  **Before the table container (after line 415):**
  ```tsx
  {/* Desktop: table view */}
  <div className="hidden md:block overflow-x-auto rounded-sm border border-gray-200 bg-white">
    <table className="min-w-[860px] text-sm">
      ...existing table content...
    </table>
  </div>

  {/* Mobile: card list */}
  <div className="block md:hidden space-y-2">
    {items.map((absence) => (
      <div
        key={absence.id}
        className="group cursor-pointer rounded-sm border border-gray-200 bg-white p-3 text-sm shadow-sm transition-shadow hover:shadow-md"
        onClick={() => navigate(`/absences/${absence.id}`)}
        onKeyDown={(e) => { if (e.key === "Enter") navigate(`/absences/${absence.id}`); }}
        tabIndex={0}
        role="button"
        aria-label={`View ${absence.student_name ?? absence.wcode} absence`}
      >
        <div className="flex items-start gap-2.5">
          <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-xs font-bold text-white">
            {initials(absence.student_name ?? absence.wcode)}
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex items-start justify-between gap-2">
              <p className="truncate font-medium text-gray-900">{absence.student_name ?? "Unknown"}</p>
              <StatusBadge status={absence.status} />
            </div>
            <p className="font-mono text-xs text-gray-500">{absence.wcode}</p>
          </div>
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-gray-600">
          <span className="rounded-sm bg-slate-100 px-1.5 py-0.5 text-xs font-semibold">{absence.subject_name ?? absence.subject_code ?? "-"}</span>
          <span className="whitespace-pre-line">{formatAbsenceSummaryDates(absence)}</span>
        </div>
        <div className="mt-1.5 flex items-center justify-between text-xs">
          <span className="text-gray-500">
            {absence.sit_in_method === "zoom" ? (
              <span className="rounded-sm bg-blue-50 px-2 py-0.5 text-xs text-blue-700">Zoom</span>
            ) : (
              <span className="rounded-sm bg-emerald-50 px-2 py-0.5 text-xs text-emerald-700">{formatSitInLabel(absence)}{absence.sit_ins?.length ? ` (${absence.sit_ins.length})` : ""}</span>
            )}
          </span>
          <span className="text-gray-400">{submittedAgo(absence.created_at)}</span>
        </div>
        <div className="mt-2 flex gap-1.5" onClick={(e) => e.stopPropagation()} onKeyDown={(e) => e.stopPropagation()}>
          {absence.status === "pending" ? (
            <Button size="sm" loading={reviewing === absence.id} onClick={() => void setStatus(absence, "reviewed")}>Mark Reviewed</Button>
          ) : absence.status === "reviewed" ? (
            <Button size="sm" loading={reviewing === absence.id} onClick={() => void setStatus(absence, "actioned")}>Actioned</Button>
          ) : null}
          <div className="opacity-0 group-hover:opacity-100 transition-opacity duration-150 flex gap-1.5">
            {absence.status !== "cancelled" ? (
              <Button size="sm" variant="ghost" onClick={() => { setCancelTargets([absence]); setCancelReasonCategory(""); setCancelReasonDetail(""); }}>Cancel</Button>
            ) : null}
            <Button size="sm" variant="ghost" className="text-red-600 hover:bg-red-50" onClick={() => setDeleteTarget(absence)}>Delete</Button>
          </div>
        </div>
      </div>
    ))}
    {items.length === 0 ? (
      <EmptyState message="All caught up! No absences match these filters." action={
        <div className="flex justify-center gap-2">
          <Link to="/absences" className="text-sm text-[var(--color-wi-primary)] hover:underline">View all</Link>
          <Link to="/absences/dashboard" className="text-sm text-[var(--color-wi-primary)] hover:underline">View dashboard</Link>
        </div>
      } />
    ) : null}
  </div>
  ```

  Note: The `table` min-width shrinks from `min-w-[1060px]` to `min-w-[860px]` (fewer columns).

- [ ] **Step 2: Move Export CSV button from filter bar to header area**

  Remove the Export CSV button from the filter section (line 381, inside the filter grid):
  ```tsx
  {/* Remove this line: */}
  <Button variant="secondary" onClick={exportCsv}><Download className="mr-1.5 h-4 w-4" />Export CSV</Button>
  ```

  Add it to the header actions area, near the Refresh button (after line 361):
  ```tsx
  <Button variant="secondary" onClick={exportCsv}><Download className="mr-1.5 h-4 w-4" />Export</Button>
  ```

  The filter grid `grid-cols` also needs adjustment since we removed one column. Change line 366:
  ```tsx
  <div className="grid gap-3 md:grid-cols-[minmax(200px,2fr)_1fr_1fr_1fr]">
  ```

- [ ] **Step 3: Update empty state with purpose-built icon**

  The empty state at line 477 currently renders inside a table cell. After the responsive split (Step A3-1), it appears in both the table and mobile card view.

  Replace the table empty row to use `colSpan={8}` (from 11):
  ```tsx
  <td colSpan={8}><EmptyState message="All caught up! No absences match these filters." action={...} /></td>
  ```

  The `EmptyState` component already has an `Inbox` icon from `lucide-react` as default — the icon prop isn't needed unless we want a different icon. Keep the default.

- [ ] **Step 4: Update test for Export button location**

  In `src/pages/__tests__/Absences.test.tsx`, line 148:
  ```ts
  await user.click(await screen.findByRole("button", { name: /export csv/i }));
  ```
  Change to:
  ```ts
  await user.click(await screen.findByRole("button", { name: /export/i }));
  ```
  The button label changes from "Export CSV" to just "Export".

---

## Plan B: Detail Interaction Refinements

**Files modified:** `src/pages/AbsenceDetail.tsx`, `src/index.css`, `src/pages/__tests__/AbsenceDetail.test.tsx`

**Interaction behaviors targeted:** #5 (action bar scroll), #6 (timeline visual flow), #7 (modal UX refinements)

---

### Task B1: Layout expansion, profile header with avatar, remove mobile bar

**Behavioral target:** The page feels more spacious and welcoming. The student identity is anchored by a large initials avatar at the top. The duplicated mobile bottom bar (source of scroll confusion) is removed.

**Modifies:** `src/pages/AbsenceDetail.tsx`

- [ ] **Step 1: Expand layout container**

  Line 212: Change `max-w-4xl` to `max-w-6xl`:
  ```tsx
  <div className="mx-auto max-w-6xl">
  ```

- [ ] **Step 2: Replace inline name display with student profile header + large avatar**

  Replace lines 220-227 (the title block):
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
  <div className="mt-2 flex flex-wrap items-start justify-between gap-4">
    <div className="flex items-center gap-4">
      <span className="flex h-14 w-14 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-xl font-bold text-white">
        {initials(absence.student_name ?? absence.wcode)}
      </span>
      <div>
        <h1 className="text-2xl font-semibold text-gray-900">Absence Detail</h1>
        <div className="mt-1 flex items-center gap-2 text-sm text-gray-600">
          <span className="font-medium">{absence.student_name ?? "Unknown"}</span>
          <span className="font-mono text-xs text-gray-400">{absence.wcode}</span>
        </div>
      </div>
    </div>
    ...
  </div>
  ```

  Add import at top of AbsenceDetail.tsx:
  ```tsx
  import { initials } from "../components/absences/initials";
  ```

- [ ] **Step 3: Remove duplicated mobile bottom bar**

  Remove the entire fixed bottom bar block (lines 244-259):
  ```tsx
  {/* Remove this entire block — lines 244-259 */}
  <div className="fixed bottom-0 left-0 right-0 z-20 border-t border-gray-200 bg-white p-3 shadow-lg md:hidden">
    ...
  </div>
  ```

  Adjust the padding on the grid below (line 261): Change `pb-16 md:pb-0` to `pb-0`:
  ```tsx
  <div className="mt-4 grid gap-4 pb-0">
  ```

- [ ] **Step 4: Make top action bar scroll-aware with shadow transition**

  The action bar at lines 228-241 uses `sticky top-4 z-20`. Replace its shadow class with a scroll-aware pattern:

  Wrap the bar with a scroll observer. Add this state variable near the other state declarations (before the `load` callback):
  ```tsx
  const [scrolled, setScrolled] = useState(false);
  ```

  Add a scroll listener in a `useEffect` (place after the existing `useEffect` at line 120):
  ```tsx
  useEffect(() => {
    const handleScroll = () => {
      setScrolled(window.scrollY > 80);
    };
    window.addEventListener("scroll", handleScroll, { passive: true });
    handleScroll();
    return () => window.removeEventListener("scroll", handleScroll);
  }, []);
  ```

  Update the action bar shadow class (line 228):
  ```tsx
  <div className={`sticky top-4 z-20 hidden md:flex flex-wrap items-center gap-2 rounded-sm border border-gray-200 bg-white p-3 transition-shadow duration-200 ${scrolled ? 'shadow-md' : 'shadow-sm'}`}>
  ```

---

### Task B2: Timeline visual flow with connecting lines + stacked layout

**Behavioral target:** The timeline reads as a connected narrative, not a flat list. Each entry is anchored by a color-coded icon on a vertical stem line. Notes and Timeline sit one above the other for comfortable reading.

**Modifies:** `src/pages/AbsenceDetail.tsx`, `src/index.css`

- [ ] **Step 1: Change Notes + Timeline from side-by-side to stacked**

  Currently Notes and Timeline are in a `md:grid-cols-2` grid (line 311). Change to single column:
  ```tsx
  <div className="grid gap-4">
  ```

- [ ] **Step 2: Add CSS for timeline vertical connecting line**

  In `src/index.css`, add:
  ```css
  /* Timeline vertical stem */
  .timeline-item {
    position: relative;
    padding-left: 1.75rem;
  }
  .timeline-item::before {
    content: "";
    position: absolute;
    left: 0.5625rem;   /* centers under the 18px icon (9px radius) */
    top: 1.25rem;
    bottom: -0.75rem;
    width: 1.5px;
    background-color: #CBD5E1;  /* matches --color-wi-border */
  }
  .timeline-item:last-child::before {
    display: none;
  }
  ```

  Note: `--color-wi-border: #CBD5E1` is already defined in `@theme`. For the inline style we'll use the raw hex since pseudo-elements can't reference CSS custom properties in Tailwind v4's `@apply` as easily.

- [ ] **Step 3: Rebuild timeline list with connecting line + color-coded icons**

  Replace the timeline section (lines 333-349):
  ```tsx
  <section className="rounded-sm border border-gray-200 bg-white">
    <h2 className="border-b border-gray-100 bg-gray-50/70 px-4 py-3 text-sm font-semibold text-gray-800">Timeline</h2>
    <div className="p-4">
      <ol className="space-y-0">
        {(absence.timeline ?? []).map((entry, index) => (
          <li key={entry.id} className={`timeline-item ${index === (absence.timeline ?? []).length - 1 ? '' : ''}`}>
            <div className="flex items-start gap-3 pb-3">
              <div className="relative z-10 mt-0.5 flex h-[18px] w-[18px] items-center justify-center rounded-full bg-white">
                {(() => {
                  switch (entry.action) {
                    case "submitted":
                    case "created":
                      return <Clock className="h-4 w-4 text-blue-500" />;
                    case "reviewed":
                      return <CheckCircle className="h-4 w-4 text-emerald-500" />;
                    case "actioned":
                      return <CheckCircle className="h-4 w-4 text-slate-500" />;
                    case "cancelled":
                      return <XCircle className="h-4 w-4 text-red-500" />;
                    case "overridden":
                      return <RotateCcw className="h-4 w-4 text-amber-500" />;
                    default:
                      return <Clock className="h-4 w-4 text-gray-400" />;
                  }
                })()}
              </div>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-gray-800">{titleCase(entry.action)}</p>
                <p className="text-xs text-gray-500">{displayDateTime(entry.created_at)} &mdash; {entry.actor_name ?? entry.actor_role}</p>
              </div>
            </div>
          </li>
        ))}
        {!absence.timeline?.length ? <li className="text-sm text-gray-500">No activity recorded.</li> : null}
      </ol>
    </div>
  </section>
  ```

  Key changes:
  - `space-y-3` → `space-y-0` (connecting line handles vertical rhythm)
  - Each `li` gets `timeline-item` class for the `::before` pseudo-element
  - Icon gets `relative z-10 bg-white` to overlap the connecting line cleanly
  - `TimelineIcon` function call replaced with inline switch for direct icon reference (avoids React component call syntax issue and keeps imports clean)
  - Imports needed: `Clock`, `CheckCircle`, `XCircle`, `RotateCcw` — already imported at line 3

- [ ] **Step 4: Update test for stacked layout**

  In `src/pages/__tests__/AbsenceDetail.test.tsx`, the test at line 99 (`"shows absence date range"`) finds `<h2>Timeline</h2>` and `closest("section")`. No change needed — the section structure is preserved.

  No other tests reference the grid layout or mobile bar, so no test updates are required for the layout changes.

  Add a new test to verify the timeline visual order:
  ```tsx
  it("renders timeline entries in order with visual stem marks", async () => {
    const detailWithTimeline = {
      ...DETAIL,
      timeline: [
        { id: "tl-1", action: "submitted", actor_role: "student", details: {}, created_at: "2026-05-27T09:00:00Z" },
        { id: "tl-2", action: "reviewed", actor_role: "admin", actor_name: "Admin User", details: {}, created_at: "2026-05-28T10:00:00Z" },
      ],
    };
    mockApiJson.mockResolvedValueOnce(detailWithTimeline);
    renderDetail();

    expect(await screen.findByText("Submitted")).toBeInTheDocument();
    expect(screen.getByText("Reviewed")).toBeInTheDocument();
    const items = screen.getAllByText(/Submitted|Reviewed/);
    expect(items[0]).toHaveTextContent("Submitted");
    expect(items[1]).toHaveTextContent("Reviewed");

    // Verify the timeline container has visual stem elements
    const timeline = screen.getByText("Timeline").closest("section");
    expect(timeline?.querySelector(".timeline-item")).toBeInTheDocument();
  });
  ```

---

### Task B3: Modal UX refinements (cancel categories, override size) + action bar scroll

**Behavioral target:** The Cancel modal switches from a freetext area to a structured dropdown + optional detail (matching the inbox pattern). The Override modal gets more breathing room with `size="xl"`. The action bar casts a deliberate shadow as the user scrolls past the profile header.

**Modifies:** `src/pages/AbsenceDetail.tsx`

- [ ] **Step 1: Replace Cancel modal freetext with category dropdown + optional detail**

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
    <Modal title="Cancel absence" onClose={() => { setCancelOpen(false); setCancelReasonCategory(""); setCancelReasonDetail(""); }}
      footer={<>
        <button type="button" className="text-sm text-red-600 hover:text-red-800 hover:underline" onClick={() => { setDeleteTarget(absence); setCancelOpen(false); setCancelReasonCategory(""); setCancelReasonDetail(""); }}>Delete Permanently</button>
        <Button variant="secondary" onClick={() => { setCancelOpen(false); setCancelReasonCategory(""); setCancelReasonDetail(""); }}>Back</Button>
        <Button variant="danger" disabled={!cancelReasonCategory} loading={saving} onClick={() => void updateStatus("cancelled", JSON.stringify({ category: cancelReasonCategory, detail: cancelReasonDetail })).then(() => setCancelOpen(false))}>Cancel Absence</Button>
      </>}>
      <p className="mb-3 text-sm text-gray-600">Assigned sit-in sessions will be released. This action is retained in the audit timeline.</p>
      <label className="block text-sm font-medium text-gray-700" htmlFor="detail-cancel-category">Cancellation reason</label>
      <select id="detail-cancel-category" className="mt-1 w-full rounded-sm border border-gray-300 p-2 text-sm" value={cancelReasonCategory} onChange={(event) => setCancelReasonCategory(event.target.value)}>
        <option value="">Select a reason...</option>
        {CANCEL_REASON_OPTIONS.map((opt) => <option key={opt.value} value={opt.value}>{opt.label}</option>)}
      </select>
      <label className="mt-3 block text-sm font-medium text-gray-700" htmlFor="detail-cancel-detail">Additional details (optional)</label>
      <textarea id="detail-cancel-detail" className="mt-1 w-full rounded-sm border border-gray-300 p-2 text-sm" rows={3} value={cancelReasonDetail} onChange={(event) => setCancelReasonDetail(event.target.value)} />
    </Modal>
  ) : null}
  ```

  This requires new state variables. Add near the other state declarations (after line 96):
  ```tsx
  const [cancelReasonCategory, setCancelReasonCategory] = useState("");
  const [cancelReasonDetail, setCancelReasonDetail] = useState("");
  ```

  And import `CANCEL_REASON_OPTIONS` from the absences utility. Since `CANCEL_REASON_OPTIONS` is currently defined in `Absences.tsx` (lines 45-51), extract it to a shared constants file or export it.

  **Best option:** Define the options in the existing `src/components/absences/dateSummary.ts` or create a new `src/components/absences/constants.ts`:

  Create `src/components/absences/constants.ts`:
  ```ts
  export const CANCEL_REASON_OPTIONS = [
    { value: "duplicate", label: "Duplicate submission" },
    { value: "student_requested", label: "Student requested cancellation" },
    { value: "admin_error", label: "Admin error" },
    { value: "incorrect_dates", label: "Incorrect dates" },
    { value: "other", label: "Other" },
  ];
  ```

  In `Absences.tsx`, replace the local `CANCEL_REASON_OPTIONS` with:
  ```tsx
  import { CANCEL_REASON_OPTIONS } from "../components/absences/constants";
  // Remove the local const CANCEL_REASON_OPTIONS block (lines 45-51)
  ```

  In `AbsenceDetail.tsx`, add:
  ```tsx
  import { CANCEL_REASON_OPTIONS } from "../components/absences/constants";
  ```

  Update the cancel modal save — the `cancelReason` string in the API call now uses `JSON.stringify({ category: cancelReasonCategory, detail: cancelReasonDetail })`, matching the inbox pattern. The `cancelReason` state can be removed (line 97) since we use `cancelReasonCategory` and `cancelReasonDetail` instead. Remove:
  ```tsx
  const [cancelReason, setCancelReason] = useState("");
  ```
  And update all references.

- [ ] **Step 2: Change Override modal size from "lg" to "xl"**

  Line 362: Change:
  ```tsx
  <Modal title="Override Sit-in" onClose={() => setOverrideOpen(false)} size="lg"
  ```
  To:
  ```tsx
  <Modal title="Override Sit-in" onClose={() => setOverrideOpen(false)} size="xl"
  ```

  Note: The current `sizeMap` in `Modal.tsx` maps `xl` to `max-w-xl` (line 9). If the design calls for wider than `max-w-xl`, modify `Modal.tsx`:
  ```tsx
  xl: "max-w-2xl",
  ```
  Change `max-w-xl` (448px) to `max-w-2xl` (672px) for a noticeably more spacious override form. The override form has a course selector, candidate session list, and textarea — `max-w-2xl` provides comfortable reading width.

- [ ] **Step 3: Update test for structured cancel modal**

  In `src/pages/__tests__/AbsenceDetail.test.tsx`, after the "warns administrators" test (ending at line 211), add:
  ```tsx
  it("cancels absence with structured reason category instead of free text", async () => {
    mockApiJson
      .mockResolvedValueOnce(DETAIL)
      .mockResolvedValueOnce({ status: "cancelled", version: 2 })
      .mockResolvedValueOnce({ ...DETAIL, status: "cancelled", version: 2 });
    renderDetail();
    const user = userEvent.setup();

    await user.click((await screen.findAllByRole("button", { name: /cancel/i }))[0]);

    // Category dropdown should appear instead of free-text textarea
    const category = screen.getByLabelText(/cancellation reason/i);
    expect(category.tagName).toBe("SELECT");
    await user.selectOptions(category, "admin_error");

    // Optional detail textarea
    const detail = screen.getByLabelText(/additional details/i);
    await user.type(detail, "Filed under wrong student");

    await user.click(screen.getByRole("button", { name: /^cancel absence$/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1/status",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({
            status: "cancelled",
            expected_version: 1,
            reason: JSON.stringify({ category: "admin_error", detail: "Filed under wrong student" }),
          }),
        }),
      );
    });
  });
  ```

  Update the existing cancel-related test (if any) — the old `cancelReason` string state no longer exists. The test at line 60 (`"shows the action context and marks a pending record reviewed"`) does not test cancellation, so no change needed.

  The "Delete Permanently" link appears in the new cancel modal (same as inbox pattern). Add assertion in the cancel test:
  ```tsx
  const cancelModal = screen.getByRole("dialog");
  expect(within(cancelModal).getByText(/delete permanently/i)).toBeInTheDocument();
  ```

- [ ] **Step 4: Update test assertion for expanded layout or override modal size**

  The test at line 197 (`"warns administrators when a manual sit-in session approaches room capacity"`) clicks "Override Sit-in" and uses `size="lg"` — now `size="xl"`. No test assertion checks the actual CSS class, so no change needed.

---

## CSS Additions Summary

All additions to `src/index.css`:

```css
/* === Interaction polish for absence inbox === */

/* Hover depth cue for clickable table rows */
tbody tr {
  transition: box-shadow 150ms ease;
}
tbody tr:hover {
  box-shadow: inset 0 1px 0 var(--color-wi-border), inset 0 -1px 0 var(--color-wi-border);
}

/* === Timeline visual stem for absence detail === */

.timeline-item {
  position: relative;
  padding-left: 1.75rem;
}
.timeline-item::before {
  content: "";
  position: absolute;
  left: 0.5625rem;
  top: 1.25rem;
  bottom: -0.75rem;
  width: 1.5px;
  background-color: #CBD5E1;
}
.timeline-item:last-child::before {
  display: none;
}
```

---

## Verification Gate

After both plans are implemented:

```bash
npm run build        # must pass
npx tsc --noEmit     # typecheck
npm test             # full test suite — all existing + new tests green
```

Check specific interaction behaviors (manual):

1. **Inbox hover reveal**: Hover any table row — Cancel + Delete buttons should fade in with 150ms easing
2. **Selection bar animation**: Select a checkbox — bar should expand smoothly over 300ms. Deselect all — bar collapses over 300ms
3. **Batch progress bar**: Trigger a batch review — progress bar should appear smoothly and width animate over 500ms
4. **Row click cursor**: Row hover shows subtle inset shadow border
5. **Detail scroll shadow**: Scroll down detail page — action bar shadow deepens from `shadow-sm` to `shadow-md` after 80px scroll
6. **Timeline stem**: Visual vertical line connects each dot, stops before last item
7. **Cancel modal structure**: Cancel on detail page shows dropdown categories + optional textarea, matching inbox pattern
8. **Mobile cards**: Resize to <768px — table collapses to card list with avatar + status badge
