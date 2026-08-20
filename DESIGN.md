# Warwick Institute Design System

## 1. Atmosphere & Identity

Warwick Institute is a calm academic operations console: dependable, compact, and easy to scan under time pressure. The interface follows Notion's product language — a quiet left sidebar, a single lightweight top bar, hairline borders, and hover-revealed actions — while staying anchored in the Warwick brand palette. The signature is *quiet structure*: white work surfaces, restrained slate borders, and one institutional blue that directs attention to the next safe action.

## 2. Color

| Role | Token | Value | Usage |
| --- | --- | --- | --- |
| Navy anchor | `--color-wi-nav` | `#0F172A` | Sidebar workspace tile and identity mark (navy rule — see §8) |
| Primary action | `--color-wi-primary` | `#2563EB` | Buttons, links, focus |
| Primary hover | `--color-wi-primary-dark` | `#1D4ED8` | Hover and active primary actions |
| Text | `--color-wi-text` | `#0F172A` | Main text (Notion *ink*) |
| Muted text | `--color-wi-text-light` | `#64748B` | Helper text and metadata (Notion *muted*) |
| Faint text | `--color-wi-faint` | `#94A3B8` | Secondary icons, placeholder labels, section captions (Notion *faint*) |
| Sidebar background | `--color-wi-bg` | `#F8FAFC` | Sidebar and page background (Notion *sidebar*) |
| Row hover | `--color-wi-row-alt` | `#F1F5F9` | Row hover fills, subtle raised surfaces (Notion *hover*) |
| Selected | `--color-wi-selected` | `#E2E8F0` | Active nav item, selected rows (Notion *selected*) |
| Hairline border | `--color-wi-line` | `rgba(15,23,42,0.08)` | Field and table boundaries (Notion *line*) |
| Hairline border, soft | `--color-wi-line-soft` | `rgba(15,23,42,0.05)` | Panel separation at rest |
| Warning | `--color-wi-amber` | `#D97706` | Warnings and time-sensitive work |
| Warning surface | `--color-wi-amber-bg` | `#FFFBEB` | Warning callouts |
| Danger | `--color-wi-red` | `#DC2626` | Critical and destructive states |
| Danger surface | `--color-wi-danger-bg` | `#FEF2F2` | Destructive callouts, error summaries |
| Success | `--color-wi-green` | `#059669` | Confirmed operations |
| Callout | `--color-wi-callout` | `#F8FAFC` | Info callouts |

Color is semantic. Blue is reserved for navigation, links, and the recommended next action; amber and red communicate urgency, never decoration. **New or changed code must use tokens — never raw Tailwind color classes.**

## 3. Typography

- Primary: `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`
- Page heading: 24px, 600 (tier-1 pages); 20px, 600 for secondary pages
- Section title: 16px, 600
- Body: 15px, 400, 1.5 line height
- Secondary body: 14px, 400, 1.5 line height
- Metadata and labels: 12px, 500–600; tabular figures for dates, counts, and urgency
- Sidebar labels: 13px; sidebar section captions: 11px, medium weight, faint

Body text stays at 14px or above. Operational data uses clear labels rather than relying on colour alone.

## 4. Spacing, Radii & Layout

- Base unit: 4px. Compact control groups use 8px, row internals 12px, cards and fields 16px, sections 20–24px, major blocks 32px. Content is capped at 1100–1152px.
- Radii: `--radius-sm` 5px (fields, buttons, nav rows, tiles), `--radius-md` 6px (popovers, menus), `--radius-lg` 10px (dialogs, slide-overs, cards).
- The shell is a Notion-style frame:
  - **Sidebar:** 252px default (resizable 220–420px, persisted in `localStorage["wi.sidebar.width"]`), collapsible, sticky full-height, `--color-wi-bg` background, hairline right border.
  - **Topbar:** 44px, sticky, page title with section breadcrumb, mobile menu trigger and desktop collapse toggle.
  - **Nav rows:** 28px tall, 13px labels, 5px radius; active row uses `--color-wi-selected` with medium weight; hover uses `--color-wi-row-alt`; badges are 18px pills placed *beside* (never inside) the link.
  - **Mobile:** sidebar becomes a 280px overlay drawer with backdrop; body scroll locks while open; Escape and route change close it.

The Schedule Impact page uses a responsive list-detail layout: the browser document owns vertical scrolling, the queue remains the primary column, and the resolution panel becomes an overlay on narrower screens. At 375px, controls wrap and the panel takes the full available width without horizontal scrolling.

## 5. Motion & Interaction

- Standard transition: 150ms; instant 80ms; fast 110ms; deliberate 200ms. Easing: `cubic-bezier(0.2, 0.8, 0.2, 1)` (entrance), `cubic-bezier(0.4, 0, 1, 1)` (exit).
- Dialogs and slide-overs animate opacity + transform (scale/translate) via `animate-notion-dialog-in` / `animate-notion-sidepeek-in`; popovers use `animate-notion-popover-in`; backdrop fades via `animate-notion-backdrop-in`; tooltips use `animate-notion-tooltip-in`.
- Tooltips delay 150ms for pointer hover and show instantly on keyboard focus.
- Every animated effect is disabled for `prefers-reduced-motion` (`motion-reduce:*` or `@media` guard). Every actionable control has hover, active, and focus-visible feedback.

## 6. Depth & Surface

Borders-first strategy: white surfaces separated by 1px `--color-wi-line` hairlines. Elevation is reserved for overlays — dialogs and menus use a soft layered shadow (`0 12px 32px rgba(0,0,0,0.14)` family); ordinary rows and panels do not cast shadows. Scrollable app panels use the `.notion-scrollbar` thin scrollbar utility.

## 7. Components

### Button
- **Variants:** primary, secondary, danger, ghost.
- **States:** default, hover, focus-visible, disabled, loading.
- **Accessibility:** native button semantics, visible 2px focus ring, loading exposed through `aria-busy`.

### Input, select, typeahead
- **States:** default, hover, focus-visible, invalid, disabled.
- **Accessibility:** labels are always visible or programmatically associated; comboboxes expose `aria-expanded`, `aria-activedescendant`, and a labelled listbox.

### Modal and slide-over
- **Structure:** labelled dialog with an explicit close button and an optional footer; radius `--radius-lg`.
- **States:** open, processing, error, confirmation.
- **Accessibility:** Escape closes when safe; focus remains inside the dialog; the close control has an accessible label.

### Schedule Impact queue row
- **Structure:** severity and urgency, student and course, concise problem statement, current arrangement, recommended action, and secondary actions.
- **States:** selected, resolving, resolved-by-another-admin, empty, and error.
- **Accessibility:** the row is keyboard-selectable; severity has text as well as colour.

### Resolution panel
- **Structure:** decision explanation, current plan, replacement comparison list, notification state, activity, and technical details.
- **States:** candidate selection, confirmation, stale candidate, completed, notification not configured, and conflict.

## 8. The Navy Rule

The navy (`#0F172A`) is the brand anchor, not a chrome color. It appears on the sidebar workspace tile, the wordmark, and as strong ink for headings and primary text — never as a full-screen navigation bar. The shell itself stays white and slate: sidebar `--color-wi-bg`, hairline separators, white topbar.

## 9. Accessibility Constraints & Accepted Debt

- Target: WCAG 2.2 AA, with visible focus indicators and at least 4.5:1 text contrast.
- Keyboard users can search, move through queue items, open/close the resolution panel, and invoke the safe primary actions.
- Urgency, processing, and delivery status are expressed in text and colour.
- Reduced motion is honoured globally.

### Accepted Debt

| Item | Location | Why accepted | Owner / Exit |
| --- | --- | --- | --- |
| No dark theme | Whole application | Existing product is light-only | Revisit with a product-wide theme initiative |
| Command palette | — | Notion's Cmd+K palette is out of scope | Optional stretch under Phase D |
| `react-day-picker` wrapper | `src/components/ui/calendar.tsx` | Kept and fully themed (`.rdp` styles, Notion motion); zero consumers today — the Operations and teacher calendars render custom month grids; the wrapper is ready for future date pickers | Adopt in the next date-picker need or delete |
| Calendar day-puck accent classes (`bg-gray-100`, `bg-red-500`) | `OperationsCalendar.tsx`, `CalendarDayCell.tsx` | Retained because e2e/unit tests assert the class names | Re-tokenize when the tests are migrated |
