# Warwick Institute Design System

## 1. Atmosphere & Identity

Warwick Institute is a calm academic operations console: dependable, compact, and easy to scan under time pressure. Its signature is quiet structure: white work surfaces, restrained slate borders, and a single institutional blue that directs attention to the next safe action.

## 2. Color

| Role | Token | Value | Usage |
| --- | --- | --- | --- |
| Navigation | `--color-wi-nav` | `#0F172A` | Top navigation and strong anchors |
| Primary action | `--color-wi-primary` | `#2563EB` | Buttons, links, focus |
| Primary hover | `--color-wi-primary-dark` | `#1D4ED8` | Hover and active primary actions |
| Text | `--color-wi-text` | `#0F172A` | Main text |
| Muted text | `--color-wi-text-light` | `#64748B` | Helper text and metadata |
| App background | `--color-wi-bg` | `#F8FAFC` | Page background |
| Border | `--color-wi-border` | `#CBD5E1` | Tables, cards, field boundaries |
| Warning | `--color-wi-amber` | `#D97706` | Warnings and time-sensitive work |
| Warning surface | `--color-wi-amber-bg` | `#FFFBEB` | Warning callouts |
| Danger | `--color-wi-red` | `#DC2626` | Critical and destructive states |
| Danger hover | `--color-wi-red-dark` | `#991B1B` | Active destructive controls |
| Success | `--color-wi-green` | `#059669` | Confirmed operations |

Color is semantic. Blue is reserved for navigation and the recommended next action; amber and red communicate urgency, never decoration.

## 3. Typography

- Primary: `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`
- Page title: 32px, 700
- Section title: 20–24px, 600–700
- Body: 15px, 400, 1.5 line height
- Secondary body: 14px, 400, 1.5 line height
- Metadata and labels: 12px, 500–600; tabular figures for dates, counts, and urgency

Body text stays at 14px or above. Operational data uses clear labels rather than relying on colour alone.

## 4. Spacing & Layout

The base unit is 4px. Existing patterns use 8px for compact control groups, 12px for row internals, 16px for cards and fields, 20–24px for sections, and 32px between major blocks. Content is normally capped at 1100–1152px.

The Schedule Impact page uses a responsive list-detail layout: the browser document owns vertical scrolling, the queue remains the primary column, and the resolution panel becomes an overlay on narrower screens. At 375px, controls wrap and the panel takes the full available width without horizontal scrolling.

## 5. Components

### Button
- **Variants:** primary, secondary, danger, ghost.
- **States:** default, hover, focus-visible, disabled, loading.
- **Accessibility:** native button semantics, visible 2px focus ring, loading exposed through `aria-busy`.

### Input and select
- **States:** default, hover, focus-visible, invalid, disabled.
- **Accessibility:** labels are always visible or programmatically associated.

### Modal and slide-over
- **Structure:** labelled dialog with an explicit close button and an optional footer.
- **States:** open, processing, error, confirmation.
- **Accessibility:** Escape closes when safe; focus remains inside the dialog; the close control has an accessible label.

### Schedule Impact queue row
- **Structure:** severity and urgency, student and course, concise problem statement, current arrangement, recommended action, and secondary actions.
- **States:** selected, resolving, resolved-by-another-admin, empty, and error.
- **Accessibility:** the row is keyboard-selectable; severity has text as well as colour.

### Resolution panel
- **Structure:** decision explanation, current plan, replacement comparison list, notification state, activity, and technical details.
- **States:** candidate selection, confirmation, stale candidate, completed, notification not configured, and conflict.

## 6. Motion & Interaction

Interactions use the existing 150ms ease-out transition. Panels and confirmation feedback animate only opacity and transform and are disabled for `prefers-reduced-motion`. Every actionable control has hover, active, and focus-visible feedback.

## 7. Depth & Surface

Warwick uses a borders-first strategy: white surfaces against `--color-wi-bg`, 1px slate boundaries, and restrained contextual backgrounds. Elevated dialogs may use a soft shadow; ordinary queue rows do not.

## 8. Accessibility Constraints & Accepted Debt

- Target: WCAG 2.2 AA, with visible focus indicators and at least 4.5:1 text contrast.
- Keyboard users can search, move through queue items, open/close the resolution panel, and invoke the safe primary actions.
- Urgency, processing, and delivery status are expressed in text and colour.
- Reduced motion is honoured globally.

### Accepted Debt

| Item | Location | Why accepted | Owner / Exit |
| --- | --- | --- | --- |
| No dark theme | Whole application | Existing product is light-only | Revisit with a product-wide theme initiative |
