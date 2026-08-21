# Detailed Plan — R6 Schedule Decomposition
_Lane: R6 | Depends: R2+R5 (rebase R3) | RequiresDetailedPlan: true | State: Plan Review pending_

## Objective
Decompose `src/pages/Schedule.tsx` 1437 LOC → <300 LOC by extracting `ScheduleWeek` (week grid, 900px peek), `ScheduleTable` (sticky header table), `SessionFormModal` (single preflight SlideOver), `ScheduleFilters`, `ScheduleSessionCard`, `SessionActions`, `SessionOccurrenceForm` + `useScheduleModals`. Replace in-card `<form>` (200px shift) with `SlideOver`, collapse actions to 1 hover→`···`, single preflight in `SessionFormModal`, and calm loading `skeleton-week` vs `opacity-60`.

## Repo Truth (pre-R6)
- `src/pages/Schedule.tsx` 1437 LOC (wc -l), `viewMode` week grid 847-932 + table 932-1005, card `div border hover:bg` + nested `SessionActions` 872-921, `Inline Edit` + inline `<form>` 887-921 causing 200px shift
- `ScheduleFilters`, `ScheduleSessionCard`, `SessionActions`, `SessionOccurrenceForm` exist but coupled to Schedule page
- `src/components/SlideOver.tsx` native `<dialog>` with R2's `useDialogModal` (closeOnBackdrop, Promise.race) available after R2
- `test:schedule --coverage` + `check:scheduling-coverage` must stay green
- No token edits (rebase R3), density tokens via R5 `min-w-[960] sticky top-11` pattern reused

## Owned Surface (no token edits — rebase R3, needs R2+R5)
`Schedule(+ScheduleWeek, ScheduleTable, SessionFormModal), ScheduleFilters, ScheduleSessionCard, SessionActions, SessionOccurrenceForm` + `useScheduleModals` (new)

## Architecture
### File Map (new = `src/pages/schedule/*`, existing flat `src/components/*` unchanged)
- `src/pages/Schedule.tsx` (<300 LOC orchestrator) — owns `viewMode` state (`'week' | 'table'`), `useScheduleModals`, `loading` derivation, `ScheduleFilters` binding, conditional `ScheduleWeek` vs `ScheduleTable`, `SessionFormModal` mount. No inline form.
- `src/pages/schedule/ScheduleWeek.tsx` (new, ~180 LOC) — week grid with 900px peek, `min-w-[900]`, `scroll-snap-type: x mandatory` peek, sticky time gutter, `ScheduleSessionCard` per slot, `onEdit -> openModal(session)` not inline form.
- `src/pages/schedule/ScheduleTable.tsx` (new, ~150 LOC) — table `min-w-[960] sticky top-11 + scroll-shadow` (reusing R5 density), header `position: sticky top-11`, row `ScheduleSessionCard` or inline table row, `overflow-x-auto`.
- `src/pages/schedule/SessionFormModal.tsx` (new, ~120 LOC) — single `SlideOver` (R2) wrapping `SessionOccurrenceForm`, single preflight `apiJson` check before submit, `onClose` resets form, `Promise.race(animationend, setTimeout(200))` via SlideOver.
- `src/pages/schedule/useScheduleModals.ts` (new) — `const { openSession, closeSession, sessionModal } = useScheduleModals()` exposes `openModal(session?)` for create/edit, `closeModal()`, `modalProps` for SlideOver.
- Existing flat components (no `src/components/schedule/` subfolder — plan does NOT introduce one): `src/components/ScheduleFilters.tsx` (existing), `src/components/ScheduleSessionCard.tsx` (existing), `src/components/SessionActions.tsx` (existing), `src/components/SessionOccurrenceForm.tsx` (existing) — R6 sole writer of these flat files plus new `src/pages/schedule/*`; `ScheduleSessionCard` trimmed to single primary + overflow `···` `aria-label="More actions"` (`group-hover:opacity-100` / `focus-within`), `SessionActions` hidden behind `···`, `SessionOccurrenceForm` moved into `SessionFormModal` SlideOver (no inline `<form>`).

### State & Data Flow
```
Schedule (orchestrator)
  ├─ useQuery sessions (existing, R5 signal not needed here but typecheck via R3)
  ├─ useScheduleModals() → { sessionModalOpen, activeSession, openModal, closeModal }
  ├─ ScheduleFilters (controlled by Schedule query params)
  ├─ viewMode === 'week' ? <ScheduleWeek sessions onEdit={openModal} loading={loading && !sessions.length ? 'skeleton' : loading ? 'dim' : 'idle'} />
  │                      : <ScheduleTable ... />
  └─ <SessionFormModal open={sessionModalOpen} session={activeSession} onClose={closeModal} />
       └─ <SessionOccurrenceForm session onSubmit={preflight + mutate} />
```
- Loading states: `loading && !sessions.length` → `data-testid="skeleton-week"` visible, `data-testid="schedule-grid"` hidden; `loading && sessions.length` → `schedule-grid` with `opacity-60` (not skeleton).
- No `Inline Edit` text remains in Schedule (rg gate).

### Notion-Grade Density & Calm
- Week peek: `min-w-[900]` with `overflow-x-auto snap-x` peek at 320px, no layout shift on filter change
- Table sticky: `min-w-[960] sticky top-11` + `shadow-[inset_...] scroll-shadow` for overflow hint
- Actions: 1 visible primary (e.g. Edit) + `···` overflow; `group-hover` + `focus-within` reveal, not always-visible row of buttons
- Form: never inline `<form>` inside card; always `SlideOver` modal — eliminates 200px shift

## TDD Steps (RED→GREEN→REFACTOR)
1. RED: `Schedule.test.tsx` expects `wc -l src/pages/Schedule.tsx <300` fails (currently 1437).
2. GREEN: extract `ScheduleWeek` + `ScheduleTable` + `useScheduleModals` + `SessionFormModal`, move inline form out — Schedule drops to <300.
3. RED: `rg "Inline Edit" src/pages/Schedule.tsx` should be 0 but found; `SessionActions` should be behind `···` but multiple buttons visible.
4. GREEN: replace `Inline Edit` + inline `<form>` 887-921 with `onEdit -> openModal`, collapse actions to `···` with hover/focus reveal.
5. RED: `SessionFormModal` should be single `SlideOver` with preflight — currently inline form does direct mutate.
6. GREEN: implement `SessionFormModal` wrapping `SessionOccurrenceForm` in `SlideOver`, single preflight `apiJson` before mutate.
7. RED: loading states `skeleton-week` vs `opacity-60` — currently always grid.
8. GREEN: conditional `loading && !sessions.length` skeleton else dim.
9. REFACTOR: ensure `test:schedule --coverage` + `check:scheduling-coverage` green, no token edits.

## Acceptance Criteria (blocking)
- (S1) `wc -l src/pages/Schedule.tsx <300`
- (S2) `rg "Inline Edit" src/pages/Schedule.tsx =0`
- (S3) hover `···` — single `···` button per card, `SessionActions` hidden until `group-hover`/`focus-within` (test: `getByLabelText('More actions')` visible, actions hidden then `fireEvent.mouseEnter(card)` → actions visible)
- (S4) `SlideOver` — `SessionFormModal` uses `SlideOver` (R2) not inline `<form>` (test: `queryByTestId('inline-session-form')` null, `getByTestId('session-form-modal')` present when open)
- (S5) `skeleton-week` visible **and** `schedule-grid` hidden when `loading && !sessions.length` else `schedule-grid` with `opacity-60` when `loading && sessions.length`
- (S6) `npm run test:schedule --coverage` + `npm run check:scheduling-coverage` green

## Verification
- `wc -l src/pages/Schedule.tsx` <300
- `rg "Inline Edit" src/pages/Schedule.tsx` =0
- `npm run test:schedule --coverage` + `check:scheduling-coverage` green
- `npm run typecheck` 0, `npm run build` succeeds
- No `rg "border-wi-line|wi-line-soft"` diff (no token edits — rebase R3)

## Risks
- Week grid 900px peek must not break `min-w-[960]` table sticky pattern from R5 — keep widths distinct per mode.
- `useScheduleModals` must not duplicate `useDialogModal` logic — delegate to `SlideOver` which already uses it.
- `SessionOccurrenceForm` preflight must be single call, not double — gate via test counts `apiJson` calls.
- Schedule filters are R6-owned but consumed by both Week/Table — ensure controlled props, not duplicated state.
