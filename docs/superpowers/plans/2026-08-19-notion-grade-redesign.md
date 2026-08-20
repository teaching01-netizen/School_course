# Notion-Grade Redesign — Warwick Institute Admin App

**Date:** 2026-08-19
**Status:** Approved for execution (plan)
**Scope:** SPA frontend (`src/`), public absence form, login page. No backend changes.
**Constraint (non-negotiable):** Warwick brand colors are preserved. Every Notion design slot is filled with a Warwick color. No new hues.

---

## 1. Goals

1. **Adopt the Notion product design language** — structure, density, chrome, motion, and interaction grammar — so the app feels like a calm, powerful working surface instead of a classic admin console.
2. **Keep all Warwick brand colors** — slate ink `#0F172A`, primary blue `#2563EB`, green `#059669`, red `#DC2626`, amber `#D97706`, slate neutrals, white canvas — in every Notion slot.
3. **Preserve all functionality, data model, and accessibility behavior.** This is a visual/interaction redesign, not a feature redesign.
4. **Fix known design debt** discovered during inventory (Section 6).

### What "copy Notion" means (and does not mean)

We adopt Notion's *philosophy and grammar*, not its screens:

- Persistent resizable left sidebar + 44px breadcrumb topbar + white content canvas
- Compact density: 28–32px rows, 13–14px UI text, comfortable-but-tight spacing
- Chrome: hairline alpha borders instead of gray boxes; borders-as-structure
- Behavior: hover-reveal actions, selected-vs-hover surface states, `80–200ms` enter/exit motion, instant-feeling menus, focus-within controls
- Typography: system sans; the page title is the loudest element; restrained weights elsewhere
- **Not**: cards everywhere, giant whitespace, decorative animation, permanent toolbars, modal-heavy flows, hover-only controls on mobile

---

## 2. Reference — Notion design language (clean-room model)

Source of truthful reference: `/Users/rd-cream/Downloads/notion-functional-reconstruction-v2` (`src/notion-app/`). It is a behavior model, not source code to copy — our implementation is fresh against the Warwick codebase.

### 2.1 Reference app tokens

| Slot | Rendered | RGB triplet |
|---|---|---|
| ink (primary text) | `#37352f` | `55 53 47` |
| muted (secondary text) | `#5c5a56` | `92 90 86` |
| faint (labels/hints/placeholders) | `#8c8a85` | `140 138 133` |
| sidebar surface | `#f7f7f5` | `247 247 245` |
| hover surface | `#efefed` | `239 239 237` |
| selected surface | `#eaeae8` | `234 234 232` |
| line | `#e9e9e7` | `233 233 231` |
| accent blue | `#2383e2` | `35 131 226` |
| callout | `#f7f6f2` | `247 246 242` |
| base font | system sans, 14px, lh 1.5 | — |
| focus-visible | `2px solid rgba(35,131,226,.32)`, offset 1px | — |
| text selection | `rgba(35,131,226,.18)` | — |

### 2.2 Reference shell metrics

| Element | Value |
|---|---|
| Sidebar width | default 252px; drag-resize 220–420px; persisted |
| Sidebar border | `border-r border-black/[0.05]` |
| Sidebar row | `h-7 (28px) rounded-[5px] text-[13px] hover:bg-hover`; selected = `bg-selected` |
| Section label | `h-7 text-[11px] font-medium text-faint px-2`; trailing icon `opacity-0 group-hover:opacity-100` |
| Nav icon slot | `h-5 w-5 text-[14px]` |
| Workspace header | `h-11`; workspace pill 20px tile, name `text-[12px] font-semibold` |
| Tab strip | `h-8 rounded-[5px]`; active = `bg-selected` |
| Topbar | `h-11 border-b border-black/[0.035] bg-white/95 backdrop-blur-[6px] px-2` |
| Breadcrumb | `text-[12px] text-muted`, segments `max-w-[180px] truncate rounded-[4px] px-1 py-0.5 hover:bg-hover hover:text-ink`, separated by faint `/` |
| Hidden scrollbar | 8px, thumb `rgba(55,53,47,.13)` rounded, transparent track |
| Mobile sidebar | overlay `position:absolute inset:0` + shadow `10px 0 32px rgba(0,0,0,.08)` + backdrop fade |

### 2.3 Reference motion vocabulary

- Durations: `--motion-instant: 80ms`, `--motion-fast: 110ms`, `--motion-standard: 150ms`, `--motion-deliberate: 200ms`
- Easings: enter `cubic-bezier(.2,.8,.2,1)`; exit `cubic-bezier(.4,0,1,1)`
- Dialog enter: `translateY(-4px) scale(.985) → 0/1`; exit inverse, slightly smaller scale
- Popover enter: `translateY(-2px) scale(.985) → 0/1`
- Tooltip: fade + `translateY(-2px) → 0`; side peek: `translateX(12px) → 0`
- Toast: `translateY(8px) scale(.98) → 0/1`
- `prefers-reduced-motion`: kill all animation/transition (`0.001ms`)
- AI FAB: hover `translateY(-1px) scale(1.03)`, active `scale(.97)` (not applicable — no FAB in WI)

---

## 3. Current-state inventory (Warwick app)

### 3.1 Stack & structure

- React 19.2 + TypeScript 5.9 strict + Vite 7 + Tailwind CSS **v4 CSS-first** (no `tailwind.config.js`; all tokens in `src/index.css` `@theme`)
- React Router 7 (lazy routes in `src/App.tsx`), TanStack Query 5, lucide-react, framer-motion (rarely used), react-day-picker 10, date-fns + luxon (Asia/Bangkok), clsx + tailwind-merge
- Hand-rolled UI kit in `src/components/ui/` — no shadcn/Radix
- Tests: Vitest + Testing Library (jsdom), Playwright + axe-core (`e2e/`)

### 3.2 Current tokens (`src/index.css` `@theme`)

```css
--color-wi-dark: #0F172A;        --color-wi-nav: #0F172A;
--color-wi-primary: #2563EB;     --color-wi-primary-dark: #1D4ED8;
--color-wi-green: #059669;       --color-wi-green-dark: #047857;
--color-wi-blue: #2563EB;        --color-wi-blue-dark: #1D4ED8;
--color-wi-red: #DC2626;         --color-wi-red-dark: #991B1B;
--color-wi-yellow: #F59E0B;      --color-wi-gray: #64748B;
--color-wi-border: #CBD5E1;      --color-wi-bg: #F8FAFC;
--color-wi-row-alt: #F1F5F9;     --color-wi-text: #0F172A;
--color-wi-text-light: #64748B;  --color-wi-danger-bg: #FEF2F2;
--color-wi-amber: #D97706;       --color-wi-amber-bg: #FFFBEB;
--font-sans: system stack;       --radius-sm: 2px; --radius-md: 6px;
--transition-fast: 150ms ease;
```

Effective palette today is fragmented: wi tokens coexist with heavy raw Tailwind usage (`text-gray-500` ~402, `border-gray-200` ~287, `text-gray-700` ~256, `bg-gray-50` ~199, plus `bg-blue-50`, `text-red-600`, `bg-amber-500`…) and hardcoded hex (`#ccc` inputs, `#EFF6FF` day-picker hover, WILogo `#c0392b`/`#2980b9`).

### 3.3 Shells

1. **Admin shell** — `src/components/Layout.tsx`: navy `#0F172A` 42px top nav (no sidebar), content `max-w-[1100px]`, border-top footer `© 2017`, mobile hamburger. Nav data lives in local arrays in this file (`navGroups`, `absenceSubItems`, `operationsNavItems`, `configNavItems`, `adminNavItems`, `pageTitles`).
2. **Public absence form** — `src/components/absences/public-form/AbsenceAppShell.tsx`: mobile-first grid, own header, step wizard, bottom sheet.

### 3.4 UI kit (`src/components/ui/`)

Button, Input, Select, FormField, FormErrorSummary, DropdownMenu, Tooltip, EmptyState, LoadingSkeleton, PageHeading (32px/700), SearchInput, calendar (`react-day-picker` wrapper), plus `src/components/`: Modal (native `<dialog>`), SlideOver (native dialog, 28rem), ConfirmModal, TypeaheadSelect, WILogo.

### 3.5 Full route catalog (from `src/App.tsx`)

**Public** — `/login` (Login), `/absence` (AbsenceForm wizard)

**Teacher/Admin** — `/` (Home), `/teacher-dashboard`, `/teacher-dashboard/absences/:id`

**Admin** — `/courses`·`/courses/create`·`/courses/:id`, `/students`·`/students/:wcode`, `/teachers`·`/teachers/create`·`/teachers/:id`, `/subjects`·`/subjects/create`, `/classrooms`, `/users`, `/schedule`, `/summary`, `/availability`, `/reports`, `/logs`, `/slot-finder`, `/absences` (inbox, `?view=board`), `/absences/dashboard`, `/absences/:id`, `/absences/calendar` (OperationsCalendar; `/operations/calendar` redirects), `/course-levels`, `/crm`·`/crm/conflicts`·`/crm/cross-study`, `/admin/absence-settings`, `/admin/operations` (OperationsHub), `/operations/schedule-impact` (SessionChanges; `/operations/session-changes` redirects), `/operations/session-changes/:id`, `/leave-policy`, `/email-reminders`, `/admin/sit-in-test`, `/admin/legacy-sync`

### 3.6 Known design debt (fix within this redesign)

| # | Debt | Location |
|---|---|---|
| D1 | `FormField` hint text renders **red** (`text-red-600`) — copy-paste bug | `src/components/ui/FormField.tsx:41` |
| D2 | Footer copyright still `© 2017` | `src/components/Layout.tsx:490` |
| D3 | Google **Inter preload is dead weight** — loaded, never applied (`--font-sans` = system stack) | `index.html` |
| D4 | `ui/calendar.tsx` rdp wrapper is **unused by any page** (only its own test) — adopt or delete (decision point, Phase D) | `src/components/ui/calendar.tsx` |
| D5 | `.rdp` day hover hardcodes `#EFF6FF`; today = 2px border; selected uses `!important` | `src/index.css` rdp block |
| D6 | Raw input/select border `#ccc`, radius 3px | `src/index.css` `@layer base` |
| D7 | WILogo uses legacy flat colors (`#c0392b`, `#2980b9`, `#7f8c8d`) that clash with tokens; needs a white/mono variant for the navy tile | `src/components/WILogo.tsx` |
| D8 | Palette fragmentation: raw `gray-*`/`blue-*`/`red-*` etc. used more than wi tokens | app-wide |

---

## 4. Design system — token architecture (the fusion layer)

### 4.1 Token mapping — Notion slot → Warwick color

| Notion slot | Notion value | Warwick value | Token (add/reuse) |
|---|---|---|---|
| ink (primary text) | `#37352f` | `#0F172A` | `--color-wi-text` (exists) |
| muted (secondary text) | `#5c5a56` | `#64748B` | `--color-wi-text-light` (exists) |
| faint (labels/hints/placeholders) | `#8c8a85` | `#94A3B8` | `--color-wi-faint` (**new**) |
| sidebar surface | `#f7f7f5` | `#F8FAFC` | `--color-wi-bg` (exists) |
| hover surface | `#efefed` | `#F1F5F9` | `--color-wi-row-alt` (exists) |
| selected surface | `#eaeae8` | `#E2E8F0` | `--color-wi-selected` (**new**) |
| line (hairline) | `black/.05` alpha | `rgba(15,23,42,.08)` | `--color-wi-line` (**new**) |
| line (topbar, softer) | `black/.035` | `rgba(15,23,42,.05)` | `--color-wi-line-soft` (**new**) |
| accent blue | `#2383e2` | `#2563EB` | `--color-wi-primary` (exists) |
| accent dark | — | `#1D4ED8` | `--color-wi-primary-dark` (exists) |
| focus ring | `rgba(35,131,226,.32)` | `rgba(37,99,235,.32)` | derive from primary |
| text selection | `rgba(35,131,226,.18)` | `rgba(37,99,235,.18)` | derive from primary |
| callout surface | `#f7f6f2` | `#F8FAFC` | `--color-wi-callout` (**new**, = wi-bg) |
| canvas | white | white | — |
| danger / warning / success | — | `#DC2626` / `#D97706` / `#059669` | existing wi tokens |
| navy brand | — | `#0F172A` | `--color-wi-nav` (exists; brand identity surfaces only) |

Semantic slot usage (unchanged roles): blue = accent/links/primary actions/selection; red = destructive/urgent; amber = warning; green = success; navy = brand identity (sidebar workspace tile, login, public form header).

### 4.2 Radii (Notion control-radius family replaces the 2px language)

| Surface | Current | New |
|---|---|---|
| controls (button/input/select) | 2–3px | 5px |
| menu/popover | 2px | 6px |
| dialog (Modal) | 2px | 10px |
| slide-over | 2px | 8px |
| day cells / chips / badges | 2px | 5px |
| cards (where they legitimately remain) | 2px | 8px |

### 4.3 Motion tokens (added to `@theme`)

```css
--motion-instant: 80ms;
--motion-fast: 110ms;
--motion-standard: 150ms;
--motion-deliberate: 200ms;
--ease-ui: cubic-bezier(.2,.8,.2,1);
--ease-exit: cubic-bezier(.4,0,1,1);
```

Keyframes added to `index.css`: `notion-backdrop-in/out`, `notion-dialog-in/out`, `notion-popover-in/out`, `notion-tooltip-in/out`, `notion-toast-in`, `notion-sidepeek-in` (values in §2.3). Existing `prefers-reduced-motion` kill-switch stays.

### 4.4 Global CSS changes (`src/index.css`)

1. `@theme`: add tokens from §4.1–4.3; change `--radius-sm` → 5px, `--radius-md` → 6px (keep names to avoid breaking existing usages; add `--radius-lg: 10px`).
2. `body`: `background-color: #fff` (canvas white); font-size stays ~15px for reading pages, but **UI shells set 14px locally** (`text-[14px]` on app frame) per Notion density.
3. Replace `@layer base` input/select raw styles: border `var(--color-wi-line)`, radius 5px, focus ring `0 0 0 3px rgba(37,99,235,.15)` → Notion-style `2px outline rgba(37,99,235,.32)` via `:focus-visible` (keep `:focus` ring for text inputs: `border wi-primary` + soft ring, since mouse users need it too — **keep existing focus ring, restyle to radius/colors**).
4. Tables (global `th,td`): padding `6px 10px`; header `12px font-medium color: wi-faint` + `border-bottom: 1px solid wi-line`; row borders `wi-line`; hover `wi-row-alt`. This single change carries most of the "Notion database" feel app-wide.
5. `.rdp` block (§5.4).
6. Scrollbar styling: replace with notion-scrollbar values (`rgba(15,23,42,.14)` thumb, 8px, transparent track).
7. Focus-visible: unify on `2px solid rgba(37,99,235,.32)` offset 1px (replace the current 2px primary outline).
8. `::selection`: `rgba(37,99,235,.18)`.
9. Remove `#ccc` hardcodes; audit remaining hardcoded hexes in this file (`#EFF6FF`, `#94A3B8` if duplicated, etc.).

---

## 5. Component specification

### 5.1 Shell architecture (replaces `Layout.tsx` nav block)

**New files:**

- `src/components/layout/navConfig.ts` — single source of nav truth, extracted from `Layout.tsx` arrays:
  - `sections: { id, label, items: {path,label,icon?,badge?,adminOnly?}[] }[]` → Schedule / Directory / Absences / Operations / Admin & Config / Settings & Audit
  - `pageTitles` map moves here (breadcrumb source)
  - role filter helpers (`visibleFor(user)`)
- `src/components/layout/Sidebar.tsx`
- `src/components/layout/Topbar.tsx`
- `src/components/layout/AppShell.tsx` — composes Sidebar + Topbar + `<main>` + skip-link
- `src/components/layout/WorkspaceTile.tsx` — navy `#0F172A` 20px rounded tile, white WILogo mono mark + "Warwick Institute" `12px semibold`

**Sidebar spec:**

| Aspect | Spec |
|---|---|
| Frame | `relative z-30 flex h-screen shrink-0 flex-col border-r border-wi-line-soft bg-wi-bg transition-[width] duration-150` |
| Width | default 252px; persisted `localStorage['wi.sidebar.width']`; range 220–420px |
| Resize | 6px grab handle at right edge (`hover:bg-primary/20`), pointer drag, double-click resets to 252; `min/max` clamp |
| Workspace header | `h-11` (44px): navy tile + name + collapse button (chevrons «) |
| Sections | label row `h-7 px-2 text-[11px] font-medium text-wi-faint`; trailing add/more icon `opacity-0 group-hover:opacity-100`; collapsible sections with 12px chevron |
| Rows | `h-7 w-full rounded-[5px] px-2 text-[13px] hover:bg-wi-row-alt`; active = `bg-wi-selected text-wi-text font-medium`; icons `h-5 w-5 text-[14px]`; aria-current |
| Badges | small pill `h-[18px] min-w-[18px] rounded-full px-1 text-[10px] font-semibold` — blue (`Absences/Inbox` pending), amber (dashboard), red (schedule-impact critical); keyboard-focusable rows keep focus ring |
| Bottom | `border-t border-wi-line-soft px-2 py-1.5`: navy avatar tile (user initial) + username `13px` + "Log out" (also in topbar menu); copyright line `text-[11px] text-wi-faint` moved here from footer |
| Collapse | width 0 / hidden; toggle from Topbar + `Cmd/Ctrl+\`; expanded state overlay on mobile |
| Mobile (<768px) | absolute overlay `inset-0 left-0 z-45`, shadow `10px 0 32px rgba(0,0,0,.08)`, backdrop fade 150ms; rows min-h-[44px] touch targets |
| Landmark | `<nav aria-label="Primary">`; section groups `<ul>/<li>` |

**Topbar spec:**

| Aspect | Spec |
|---|---|
| Frame | `sticky top-0 z-20 h-11 shrink-0 flex items-center border-b border-wi-line-soft bg-white/95 backdrop-blur-[6px] px-2` |
| Left | sidebar toggle (»/« 16px icon button) + breadcrumb trail from `navConfig.pageTitles`: `text-[12px] text-wi-text-light`, segments `rounded-[4px] px-1 py-0.5 hover:bg-wi-row-alt hover:text-wi-text`, faint `/` separators, `max-w-[180px] truncate` |
| Right | user chip (avatar tile + name, opens DropdownMenu: Profile→none today, Log out) |
| Role note | Teacher role: breadcrumb "Teacher Dashboard"; no admin-only controls here |

**Main content:**

| Aspect | Spec |
|---|---|
| Canvas | `bg-white`; `main` `flex-1 py-6` (was py-4); keeps `max-w-[1100px]`, safe-area padding, `id="main"` skip target |
| Footer | removed from bottom; copyright relocated into sidebar bottom |
| Loading fallback | restyle spinner to Notion grammar (neutral ring, primary arc) in `App.tsx` |

**Navy usage rule:** navy `#0F172A` appears only on: sidebar workspace tile, login page, public absence form header, WILogo wordmark. Everything else uses the light Notion chrome + blue accent.

### 5.2 UI kit restyle (`src/components/ui/` + `src/components/`)

| Component | Change |
|---|---|
| `Button` | heights 28/32/36 (sm/md/lg); `rounded-[5px]`; `active:translate-y-px` keep; 150ms; focus-visible 2px ring `rgba(37,99,235,.32)` offset 1px; secondary = white + `border-wi-line` + hover `bg-wi-row-alt`; ghost hover `bg-wi-row-alt`; keep variants/sizes API (no prop changes → no call-site churn) |
| `Input` / `Select` | `h-7`/`h-8`, `rounded-[5px]`, border `wi-line`, focus border `wi-primary` + `ring-3 ring-primary/15` keep, 14px; error = `wi-red` border + `wi-danger-bg` tint |
| `FormField` | **Fix D1**: hint `text-red-600` → `text-wi-faint` (keep error text red) |
| `PageHeading` | 32px font-semibold (600), `text-wi-text`; add optional `description` slot render (muted 14px) |
| `Modal` | `rounded-[10px]`, backdrop fade 150ms, `notion-dialog-in/out`, shadow family `0 1px 2px rgb(0 0 0/.04), 0 8px 30px rgb(0 0 0/.05)`; keep native `<dialog>` + focus trap |
| `SlideOver` | `rounded-l-[8px]`, `notion-sidepeek-in` (translateX 12px), same shadow; keep native dialog semantics |
| `ConfirmModal` | inherits Modal changes; destructive stays quiet until final button (no red page-wide) |
| `DropdownMenu` | `rounded-[6px]`, `notion-popover-in/out` (exit 110ms), shadow family, active row `bg-wi-row-alt`; keep roving keyboard + focus restore |
| `Tooltip` | delay 150ms, fade + `translateY(-2px)` slide, `rounded-[5px]` |
| `EmptyState` | drop heavy icon color → `text-wi-faint` icon + muted copy; keep single CTA |
| `LoadingSkeleton` | muted surfaces (`bg-wi-row-alt`) instead of gray-200 |
| `TypeaheadSelect` | token pass only |
| `WILogo` | **Fix D7**: add `variant="mono"` (white marks for navy tile) + `variant="color"` (new tokens for palette alignment: use wi-red/wi-blue — recommend `#DC2626`/`#2563EB`); wordmark inherits currentColor |

### 5.3 Tables & data surfaces (global `index.css` + per-page in Phase C)

- Margins/rows: 13px text, `6px 10px` cells, hairline `wi-line` row borders, header row `12px font-medium wi-faint` with 1px bottom border
- Row hover `wi-row-alt`; row selection `wi-selected`
- Action overflow (`•••` DropdownMenu trigger) hidden at rest, `opacity-0 group-hover:opacity-100` on rows (keep keyboard reachable: focus-within reveals)
- Card discipline: convert decorative `card` wrappers on list/settings pages to flat surfaces (spacing + hairline only); keep cards only where strong grouping/identity is needed (e.g., session cards on Home board, Kanban cards — restyle, don't remove)

### 5.4 Calendar (react-day-picker wrapper + custom calendars)

| Surface | Spec |
|---|---|
| `.rdp` theme (`index.css`) | cells 36px → 32px, `rounded-[5px]`, hover `wi-row-alt` (**replaces `#EFF6FF`**), selected = `wi-primary` white text (drop `!important` by using `rdp-selected` with sufficient specificity), today = `box-shadow: inset 0 0 0 1px wi-primary` hairline ring (replaces 2px border), month caption `14px font-semibold`, nav buttons 28px hover `wi-row-alt`, weekday header `text-[11px] text-wi-faint uppercase` |
| `ui/calendar.tsx` (wrapper) | keep API; default `showOutsideDays`/`fixedWeeks` stay; padding `p-2` |
| `OperationsCalendar` (`src/pages/OperationsCalendar.tsx`) | Phase C: hairline column borders instead of `border-gray-200` grid; weekday header `11px faint uppercase`; day cells: subtle hover, today outlined `wi-primary` ring; event chips → `text-[12px]` rows with semantic left dot (session = blue, absence = amber, sit-in = green) instead of heavy pill blocks; keep show-mode tabs (all/sessions/absences/sit-ins) restyled to Notion tab strip (`h-8 rounded-[5px]`, active `bg-selected`) |
| Teacher `CalendarMonth`/`CalendarDayCell` (`src/components/teacher/`) | same day-cell grammar; DayCell accent `border-l-*` bars → thin `border-l-2` semantic colors kept (red absences / amber visitors / green sessions) but softened to `wi-red/wi-amber/wi-green` with matching `bg-wi-row-alt` hover; overflow "+N" chip style matches Notion |
| Decision point (Phase D) | adopt `ui/Calendar` in date-picking surfaces (Schedule, AbsenceForm, side panels, Availability) **or** delete wrapper + dead `.rdp` CSS if permanent → document choice in `DESIGN.md` |

### 5.5 Absence-domain components (Phase C tier-1)

- Inbox table → data-surface grammar (§5.3); responsive card conversion at <768px keeps `data-label` technique but cards get `rounded-[8px] border-wi-line`
- KanbanView → Board grammar: quiet column headers (11px faint uppercase + count), cards = white, hairline border, `rounded-[8px]`, hover lift `translateY(-1px)` + soft shadow, drag opacity states
- SidePanel (absence detail) → right side-peek with `notion-sidepeek-in`; sections = muted 11px labels; sit-in suggestion rows = 13px with status dots
- StepIndicator (public form) → dots-with-connecting-line, 12px labels, active = `wi-primary`
- ToggleSwitch, StickyFooter, SessionChip, SubjectCard, CourseChip → token/radius pass (chips 5px, 12px)

### 5.6 Schedule-impact domain (Phase C tier-1)

- WorkQueue rows → data-surface rows with severity as quiet semantic tag (dot + 12px label; red = critical, amber = attention, green = resolved) — **no giant pill banners**
- IssueResolutionPanel / ResolutionComparison → restyle panels to flat surfaces + hairlines; keep the powerful comparison layout
- History view (`SessionChanges?view=history`) → table grammar

### 5.7 Nav + teacher dashboard (Phase C)

- TeacherDashboard: week/month toggle → Notion tab strip; DayPanel/SessionTable → data-surface grammar; WeekSummary cards → flat
- `Summary`, `Availability`, `SlotFinder`: Phase D token pass (they sit under Schedule section)

---

## 6. Page-by-page plan (tiering)

All route pages get at minimum a **token pass** (globals + kit handle most of it automatically). Tier-1 pages get a **bespoke pass**. Public surfaces get specific handling.

| Tier | Pages | Work |
|---|---|---|
| **T1 bespoke (Phase C)** | Home, Schedule, Absences inbox (+board), AbsenceDetail, OperationsCalendar, TeacherDashboard, CourseDetail, Students, Teachers, SessionChanges (queue + history) | Full §5.3/§5.5/§5.6 treatment; screenshot before/after per page |
| **T2 token pass (Phase D)** | Courses, CourseCreate, CourseLevels, StudentProfile, TeacherProfile, TeacherCreate, Subjects, SubjectCreate, Classrooms, Users, Summary, Availability, Reports, Logs, SlotFinder, AbsenceSettings, AbsenceDashboard, OperationsHub, SessionChangeDetail, LeavePolicy, EmailReminders, SitInTestPage, LegacySyncHealth, CrmAdmin, CrmConflicts, CrossStudyPage, TeacherAbsenceDetail | Globals + kit + targeted class swaps (border-gray-200 → wi-line, text-gray-500 → wi-faint, bg-gray-50 → wi-row-alt, text-gray-700 → wi-text…) |
| **Public: Login** | Phase C | Notion-grade centered card: white panel `rounded-[10px]`, hairline border, navy brand moment (logo), quiet labels; keep OTP flow untouched |
| **Public: AbsenceForm** | Phase C (chrome only) | Shell keeps navy header (brand moment for students); StepIndicator/chips/surfaces restyled per §5.5; form fields inherit kit |
| **Teacher role pages** | covered above | nav filtering must match today's behavior (teacher sees teacher-dashboard only in sidebar) |

### Palette consolidation gate (Phase D)

Targeted replace on visible surfaces, by class → token:

- `border-gray-200` → `border-wi-line` · `border-gray-300` → `border-wi-line`
- `bg-gray-50` → `bg-wi-row-alt` · `bg-white` stays
- `text-gray-500` → `text-wi-faint` · `text-gray-600` → `text-wi-text-light` · `text-gray-700/900` → `text-wi-text`
- `bg-blue-50` → `bg-primary/10` (or `wi-callout`) · `text-blue-600/700` → `text-wi-primary(-dark)` · `border-blue-*` → `border-wi-primary`
- `bg-amber-50`/`text-amber-700` → `wi-amber-bg`/`wi-amber` · `bg-red-50`/`text-red-600/700` → `wi-danger-bg`/`wi-red` · green equivalents → `wi-green`
- `bg-slate-*` → wi tokens; hardcoded hexes get a grep audit (no new hex allowed in changed files)

---

## 7. Accessibility requirements (unchanged bar, restyled)

- Keep skip-to-content link (relocate into AppShell)
- Focus-visible: unified `2px rgba(37,99,235,.32)` offset 1px everywhere; never removed
- Sidebar: `nav` landmark, `aria-current="page"` on active rows, collapse toggle `aria-expanded`, keyboard-resizable width (shift+arrow optional — minimum: toggle key `Cmd/Ctrl+\`, all rows tabbable)
- Hover-reveal actions must remain reachable via keyboard: `opacity-0 group-hover:group-focus-within:opacity-100`
- Native `<dialog>` semantics in Modal/SlideOver preserved (focus trap, Escape, `::backdrop`)
- `prefers-reduced-motion` kill-switch extended to all new keyframes
- Touch: mobile sidebar rows and any hover-reveal targets ≥44px on touch
- Color-independent meaning: severity always has label + dot, not color alone
- Contrast: `#94A3B8` faint text only for non-essential metadata; essential text uses `wi-text-light` `#64748B` (AA on white)

---

## 8. Testing & verification strategy

| Gate | Command | When |
|---|---|---|
| Typecheck | `npm run typecheck` + `typecheck:e2e` | after every phase |
| Unit | `npm test` (full Vitest), `test:schedule`, `test:schedule-impact`, `test:schedule:coverage` | after every phase |
| e2e + a11y | `npm run test:e2e` (Playwright + axe) | after Phase A (shell!), C, D |
| Absence e2e | `test:e2e:absence` | after Phase C (public form) |
| Build | `npm run build` | after every phase |
| Manual walk | screenshot each route in §6 tier; verify: active states, badges, sidebar resize/collapse/persist, mobile overlay, breadcrumbs, teacher vs admin nav | after Phase A, C, D |
| Palette gate | grep audit: `grep -rE "#[0-9a-fA-F]{3,6}" src --include="*.tsx" --include="*.css"` — new/changed files must only use tokens | end of Phase D |

**Test-update rule:** any test (unit or e2e) that selects on old shell markup (nav text, hamburger, footer) must be updated in the same phase that changes the markup; run the affected suite before closing the phase.

---

## 9. Risks & mitigations

| Risk | Mitigation |
|---|---|
| Palette fragmentation makes pages look mixed until Phase D | Global CSS + kit capture most of the look in Phase A; T1 bespoke in C; consolidation gate in D; accept partial progress between phases |
| Monolith pages (CourseDetail ~87KB, Schedule ~72KB, StaffCreateAbsenceModal ~81KB) resist sweeping | Restyle via shared components/globals first; only targeted class swaps in monoliths; no refactoring while restyling |
| e2e selectors break on shell rewrite | Update specs same-phase; run `test:e2e` + axe after Phase A before proceeding |
| Teacher-role nav regression | Sidebar role filtering reuses today's arrays/logic; manual walk for teacher account in every phase |
| Scope creep (command palette, search-as-navigation) | Stretch item only, Phase D-optional, explicitly gated behind `FEATURE_FLAG`-style constant or skipped — **not in core scope** |
| Density change (14px) hurts readability on 1100px pages | UI chrome at 14px; body reading text stays 15px; verify on Phase A manual walk |
| `!important` removal in `.rdp-selected` breaks selection styling | Use `[&_.rdp-selected_.rdp-day_button]`-level specificity in `index.css`; covered by existing rdp unit test (`ui/__tests__/calendar.test.tsx`) + manual calendar walk |

---

## 10. Execution phases (each shippable, each ends green)

### Phase A — Foundation & Shell
**Files:** `src/index.css` (tokens, motion, keyframes, globals, rdp theme, scrollbar, selection, focus) · `index.html` (remove Inter preload — D3) · `src/components/layout/navConfig.ts` (new) · `src/components/layout/Sidebar.tsx` (new) · `src/components/layout/Topbar.tsx` (new) · `src/components/layout/AppShell.tsx` (new) · `src/components/layout/WorkspaceTile.tsx` (new) · `src/components/Layout.tsx` (rewrite to use AppShell; footer removal — D2) · `src/App.tsx` (loading fallback restyle) · ui kit restyle: Button, Input, Select, FormField (D1), Modal, SlideOver, DropdownMenu, Tooltip, PageHeading, EmptyState, LoadingSkeleton · `src/components/WILogo.tsx` (D7 mono variant) · e2e specs touching shell

**Exit criteria:** typecheck + `test` + `test:e2e` + `build` green; manual walk of shell (sidebar resize/collapse/persist, mobile overlay, breadcrumbs, badges, teacher role); screenshots reviewed.

### Phase B — Shared patterns & debt finals
**Files:** remaining kit components not restyled in A (TypeaheadSelect pass) · badge/card/list pattern alignments app-wide where cheap (global classes) · `.rdp` fine-tune per calendar walk · `DESIGN.md` update (tokens, radii, motion, navy rule) · `docs/ux-ui-overhaul-spec.md` marked superseded where conflicting

**Exit criteria:** full unit + e2e green; no visual regressions found on a random 20-route sweep; DESIGN.md reflects the new system.

### Phase C — Tier-1 pages + public surfaces
**Files (one page at a time, screenshot + tests each):** Home, Schedule, Absences (+board), AbsenceDetail, OperationsCalendar (+ teacher calendars: CalendarMonth, CalendarDayCell), TeacherDashboard, CourseDetail, Students, Teachers, SessionChanges, Login, AbsenceForm chrome (public-form components)

**Exit criteria:** per-page: typecheck + page-related tests + screenshot review; end of phase: full suite + manual walk + axe pass.

### Phase D — Tier-2 pages, consolidation, decisions
**Files:** all T2 routes (§6) · palette consolidation gate (class→token swaps) · rdp wrapper decision (adopt in date pickers or delete wrapper + dead CSS — document in DESIGN.md) · optional stretch: `Cmd/Ctrl+K` page-navigation palette (gated, skip if scope pressure) · legacy hex audit

**Exit criteria:** full `npm test` + `test:e2e` + `build`; palette grep gate; axe sweep; final walkthrough of all 45 routes both roles; DESIGN.md final.

---

## 11. Definition of Done (accepted debt / non-goals)

- **In scope:** everything in §4–§10.
- **Explicitly out:** backend/API changes; new features (filters/relations); dark theme (existing accepted debt); command palette unless stretch lands; restructuring monolith components beyond class swaps.
- **Do-not list (from notion-grade guardrails):** no new cards where flat works; no permanent action toolbars; no hover-only affordances on mobile; no decorative animation; no new modals for inline-editable properties; keep density comfortable, not inflated; never use raw Tailwind colors in new/changed code (tokens only).

## 12. Acceptance criteria (final)

1. All 45 routes render in the new shell with correct active states, badges, and breadcrumbs — Admin and Teacher roles.
2. Full test matrix green: `typecheck`, `typecheck:e2e`, `test`, `test:schedule`, `test:schedule-impact`, `test:e2e` (incl. axe), `build`.
3. Palette constraint verified by diff/grep: every Notion slot filled by a Warwick token; zero new hues; zero new hardcoded hexes in changed files.
4. Sidebar: resizes 220–420px, persists via localStorage, collapses via toggle + `Cmd/Ctrl+\`, overlay+backdrop below 768px.
5. Motion vocabulary applied (dialog/popover/tooltip/toast/side-peek), all under 200ms, all killed by `prefers-reduced-motion`.
6. Known debt fixed: D1 (FormField hint), D2 (© year), D3 (Inter preload), D5 (rdp hardcodes), D6 (`#ccc`), D7 (WILogo) — D4 resolved by documented decision.
7. `DESIGN.md` updated and consistent with implementation.
8. Public absence form and login render in the new language while keeping navy brand identity.