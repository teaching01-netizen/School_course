# Warwick Design System Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish a source-owned Warwick design system on top of shadcn/ui, then migrate the public absence flow and admin application shell without introducing Mantine or a second styling system.

**Architecture:** Keep Tailwind v4 and `src/index.css` as the styling foundation. Add shadcn components as source code under `src/components/ui`, preserve existing APIs temporarily through compatibility adapters, and build Warwick-specific patterns under `src/components/patterns` and `src/features`. Migrate one vertical slice at a time, with tests and visual verification at every gate.

**Tech Stack:** Vite, React 19, TypeScript, Tailwind CSS v4, shadcn/ui, Radix primitives, `class-variance-authority`, `clsx`, `tailwind-merge`, Lucide React, TanStack Query, Vitest, Testing Library, and the existing Playwright E2E setup.

---

## Guardrails

- Do not install Mantine.
- Do not create a second global stylesheet. Extend `src/index.css`.
- Do not rewrite the entire application in one change.
- Do not delete the existing `Button`, `Input`, `Select`, `Modal`, or `SlideOver` APIs until their consumers have migrated or compatibility adapters are in place.
- Do not use raw color utilities in new UI. Use semantic tokens and component variants.
- Do not add new `space-y-*` or `space-x-*` layout utilities. Use `flex`, `grid`, and `gap-*`.
- Do not use template-literal class-conditionals. Use `cn()` and `cva()`.
- Do not use `--overwrite` with the shadcn CLI without first reviewing the generated diff and explicitly approving the overwrite.
- Treat the existing full-test failures as a baseline to reduce, not as permission to weaken assertions.
- Each task ends with a focused test, the full typecheck, and a small commit.

## File map

### Create

- `components.json` — shadcn project configuration.
- `src/components/ui/button.tsx` — canonical shadcn Button.
- `src/components/ui/input.tsx` — canonical Input.
- `src/components/ui/field.tsx` — canonical Field composition and validation semantics.
- `src/components/ui/select.tsx` — canonical Select.
- `src/components/ui/textarea.tsx` — canonical Textarea.
- `src/components/ui/card.tsx` — Card composition.
- `src/components/ui/badge.tsx` — status and metadata badges.
- `src/components/ui/alert.tsx` — page and inline alerts.
- `src/components/ui/skeleton.tsx` — layout-matching loading skeleton.
- `src/components/ui/empty.tsx` — empty-state composition.
- `src/components/ui/dialog.tsx` — modal primitive.
- `src/components/ui/sheet.tsx` — side-panel primitive.
- `src/components/ui/alert-dialog.tsx` — destructive confirmation primitive.
- `src/components/ui/popover.tsx` — anchored overlay primitive.
- `src/components/ui/command.tsx` — command/search primitive.
- `src/components/ui/combobox.tsx` — accessible typeahead composition.
- `src/components/ui/tabs.tsx` — tablist, tabs, and tab panels.
- `src/components/ui/table.tsx` — table composition.
- `src/components/ui/sonner.tsx` — toast host and theme bridge.
- `src/components/layout/navigation.ts` — typed navigation definitions.
- `src/components/layout/AppShell.tsx` — shell composition.
- `src/components/layout/Sidebar.tsx` — desktop navigation.
- `src/components/layout/MobileNav.tsx` — mobile navigation.
- `src/components/layout/PageHeader.tsx` — page title, description, and actions.
- `src/components/layout/useLayoutStats.ts` — layout-only stats query.
- `src/components/patterns/FilterBar.tsx` — filters and clear/reset behavior.
- `src/components/patterns/DataTable.tsx` — table layout and state contract.
- `src/components/patterns/KpiCard.tsx` — report and dashboard metric card.
- `src/components/patterns/StatusBadge.tsx` — domain status-to-variant mapping.
- `src/components/patterns/LoadingState.tsx` — page-level loading state.
- `src/components/patterns/ErrorState.tsx` — page-level error and retry state.
- `src/features/schedule/api/useHomeScheduleData.ts` — typed Home data query.
- `src/features/reports/api/useReportsData.ts` — typed reports query.
- `src/test/design-system.contract.test.ts` — semantic token and UI contract checks.
- `src/components/ui/__tests__/button.test.tsx` — Button behavior tests.
- `src/components/ui/__tests__/field.test.tsx` — Field accessibility tests.
- `src/components/ui/__tests__/dialog.test.tsx` — Dialog focus and dismissal tests.
- `src/components/ui/__tests__/combobox.test.tsx` — Combobox keyboard and selection tests.
- `src/components/patterns/__tests__/DataTable.test.tsx` — table state tests.
- `src/components/patterns/__tests__/ErrorState.test.tsx` — error/retry tests.

### Modify

- `package.json` and `package-lock.json` — shadcn/runtime dependencies and scripts.
- `src/index.css` — semantic design tokens and Tailwind v4 theme mapping.
- `src/utils/cn.ts` — retain as the single class composition utility; do not create a duplicate `cn` helper.
- `src/components/ui/Button.tsx` — compatibility adapter during migration.
- `src/components/ui/Input.tsx` — compatibility adapter during migration.
- `src/components/ui/Select.tsx` — compatibility adapter during migration.
- `src/components/ui/FormField.tsx` — migrate from child cloning to Field composition.
- `src/components/ui/DropdownMenu.tsx` — replace with the canonical menu primitive or adapter.
- `src/components/ui/Tooltip.tsx` — replace with the canonical tooltip primitive or adapter.
- `src/components/ui/LoadingSkeleton.tsx` — compatibility wrapper around Skeleton.
- `src/components/ui/EmptyState.tsx` — compatibility wrapper around Empty.
- `src/components/Modal.tsx` — compatibility adapter around Dialog.
- `src/components/SlideOver.tsx` — compatibility adapter around Sheet.
- `src/components/Layout.tsx` — reduce to shell composition and route outlet responsibilities.
- `src/pages/Home.tsx` — consume typed schedule data and shared patterns.
- `src/pages/Reports.tsx` — consume PageHeader, FilterBar, Tabs, KpiCard, and DataTable.
- `src/pages/AbsenceForm.tsx` — consume Field, Dialog/Sheet, Alert, and unified feedback patterns.
- `src/components/absences/SidePanel.tsx` — migrate to Sheet.
- `src/components/absences/KanbanView.tsx` — use canonical disclosure and keyboard behavior.
- `src/components/teacher/SessionTable.tsx` — remove click-only row interactions.
- `src/components/teacher/CalendarMonth.tsx` — consume one calendar contract and scope keyboard listeners.
- Existing related tests under `src/components`, `src/pages/__tests__`, and `src/components/absences/__tests__` — preserve behavior while updating intentional copy and semantics.

## Task 0: Capture the baseline before changing dependencies

**Files:**

- Create: `docs/superpowers/plans/2026-07-23-warwick-design-system-migration.md`
- Test: existing repository scripts only.

- [ ] Run the existing checks from the repository root:

```bash
npm run typecheck
npm test -- --reporter=dot
npm run build
git diff --check
git status --short
```

Expected result: typecheck and build establish the current baseline; the full test run records known failures without changing tests; `git diff --check` reports no whitespace errors; the worktree is clean except for the plan file.

- [ ] Record the baseline test failures in the implementation PR description. Do not mark the migration complete while the full test run has more failures than this baseline.

- [ ] Commit only the plan if the team wants the plan tracked independently:

```bash
git add docs/superpowers/plans/2026-07-23-warwick-design-system-migration.md
git commit -m "docs: add Warwick design system migration plan"
```

## Task 1: Initialize shadcn without overwriting existing components

**Files:**

- Create: `components.json`
- Modify: `package.json`, `package-lock.json`, `src/index.css`
- Test: `src/test/design-system.contract.test.ts`

- [ ] Confirm the project context and current component inventory before initialization:

```bash
npx shadcn@latest info --json
rg --files src/components/ui
```

Expected result: Vite, TypeScript, Tailwind v4, `src/index.css`, and the `@` alias are detected; no `components.json` exists yet; existing PascalCase UI files are listed.

- [ ] Initialize with the Radix-based shadcn preset and the existing paths. Use these prompt decisions:

```text
Template: Vite
TypeScript: yes
Primitive base: Radix
Global CSS: src/index.css
CSS variables: yes
Component alias: @/components
Utility alias: @/utils/cn
```

Run:

```bash
npx shadcn@latest init --preset radix-nova --template vite
```

Expected result: `components.json` exists, the existing Tailwind v4 CSS file is configured, and no existing PascalCase component is overwritten.

- [ ] Read the generated `components.json` and verify these boundaries:

```json
{
  "tailwind": {
    "css": "src/index.css",
    "config": ""
  },
  "aliases": {
    "components": "@/components",
    "utils": "@/utils/cn",
    "ui": "@/components/ui",
    "lib": "@/lib",
    "hooks": "@/hooks"
  }
}
```

Do not copy this block over the generated file blindly; preserve the CLI’s current schema and verify the equivalent values.

- [ ] Add only dependencies that the generated components require. `clsx`, `tailwind-merge`, and `lucide-react` already exist. Add `class-variance-authority` and `sonner` only if the generated components require them:

```bash
npm install class-variance-authority sonner
```

- [ ] Run the initialization verification:

```bash
npx shadcn@latest info --json
npm run typecheck
npm run build
```

Expected result: the project reports the new config, typecheck passes, and the production build passes before any page migration begins.

- [ ] Commit the initialization separately:

```bash
git add components.json package.json package-lock.json src/index.css
git commit -m "chore: initialize shadcn design system"
```

## Task 2: Define the Warwick semantic token contract

**Files:**

- Modify: `src/index.css`
- Create: `src/test/design-system.contract.test.ts`
- Test: `src/test/design-system.contract.test.ts`

- [ ] Write the contract test first. It must read `src/index.css` and assert that each required semantic token exists:

```ts
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const css = readFileSync(resolve(process.cwd(), "src/index.css"), "utf8");

describe("Warwick design tokens", () => {
  it("defines the semantic color roles used by UI components", () => {
    for (const token of [
      "--background",
      "--foreground",
      "--card",
      "--card-foreground",
      "--muted",
      "--muted-foreground",
      "--border",
      "--input",
      "--ring",
      "--primary",
      "--primary-foreground",
      "--secondary",
      "--secondary-foreground",
      "--destructive",
      "--destructive-foreground",
      "--success",
      "--warning",
      "--info",
    ]) {
      expect(css).toContain(token);
    }
  });
});
```

- [ ] Run the new test and confirm it fails because the semantic tokens are not yet defined:

```bash
npx vitest run src/test/design-system.contract.test.ts
```

- [ ] Add a `:root` semantic layer to `src/index.css`. Map the current Warwick values rather than inventing new colors:

```css
:root {
  --background: var(--color-wi-bg);
  --foreground: var(--color-wi-text);
  --card: #ffffff;
  --card-foreground: var(--color-wi-text);
  --muted: var(--color-wi-row-alt);
  --muted-foreground: var(--color-wi-text-light);
  --border: var(--color-wi-border);
  --input: var(--color-wi-border);
  --ring: var(--color-wi-primary);
  --primary: var(--color-wi-primary);
  --primary-foreground: #ffffff;
  --secondary: var(--color-wi-gray);
  --secondary-foreground: var(--color-wi-text);
  --destructive: var(--color-wi-red);
  --destructive-foreground: #ffffff;
  --success: var(--color-wi-green);
  --warning: var(--color-wi-yellow);
  --info: var(--color-wi-primary);
  --radius: 0.375rem;
}
```

If a legacy variable has a different exact name, use the existing name from `src/index.css`; do not silently create a second brand palette.

- [ ] Add the Tailwind v4 mapping in the same file:

```css
@theme inline {
  --color-background: var(--background);
  --color-foreground: var(--foreground);
  --color-card: var(--card);
  --color-card-foreground: var(--card-foreground);
  --color-muted: var(--muted);
  --color-muted-foreground: var(--muted-foreground);
  --color-border: var(--border);
  --color-input: var(--input);
  --color-ring: var(--ring);
  --color-primary: var(--primary);
  --color-primary-foreground: var(--primary-foreground);
  --color-secondary: var(--secondary);
  --color-secondary-foreground: var(--secondary-foreground);
  --color-destructive: var(--destructive);
  --color-destructive-foreground: var(--destructive-foreground);
}
```

- [ ] Preserve legacy variables for the migration, but forbid new usage of them. Add the contract test, run it, typecheck, and build:

```bash
npx vitest run src/test/design-system.contract.test.ts
npm run typecheck
npm run build
```

Expected result: the contract test passes and no visual page migration has occurred yet.

- [ ] Commit the token layer:

```bash
git add src/index.css src/test/design-system.contract.test.ts
git commit -m "feat: add semantic Warwick design tokens"
```

## Task 3: Build canonical primitive components with compatibility adapters

**Files:**

- Create: `src/components/ui/button.tsx`, `input.tsx`, `field.tsx`, `select.tsx`, `textarea.tsx`, `card.tsx`, `badge.tsx`, `alert.tsx`, `skeleton.tsx`, `empty.tsx`
- Create: `src/components/ui/__tests__/button.test.tsx`, `field.test.tsx`
- Modify: `src/components/ui/Button.tsx`, `Input.tsx`, `Select.tsx`, `FormField.tsx`, `LoadingSkeleton.tsx`, `EmptyState.tsx`

- [ ] Fetch the current shadcn component documentation before adding components:

```bash
npx shadcn@latest docs button input field select textarea card badge alert skeleton empty
```

Read the returned URLs and verify the current Radix API before implementing each component.

- [ ] Preview additions before writing them:

```bash
npx shadcn@latest add button input field select textarea card badge alert skeleton empty --dry-run
```

Expected result: the dry run lists only new lowercase component files and required dependencies. Stop if it proposes overwriting existing PascalCase files.

- [ ] Add the components without `--overwrite`:

```bash
npx shadcn@latest add button input field select textarea card badge alert skeleton empty
```

- [ ] Define the canonical Button contract:

```ts
type ButtonVariant =
  | "default"
  | "secondary"
  | "destructive"
  | "outline"
  | "ghost"
  | "link";

type ButtonSize = "sm" | "default" | "lg" | "icon";
```

Use `cva()` for variants, `cn()` for class composition, `data-icon` for Lucide icons, and `disabled` plus Spinner composition for pending state.

- [ ] Convert the old `Button.tsx` into an adapter. Its `primary` variant must map to `default`, `danger` to `destructive`, and its existing `loading` prop must render a Spinner while preserving the current API for untouched consumers.

- [ ] Convert `Input.tsx` and `Select.tsx` into adapters. Preserve existing `size`, `error`, and `describedBy` props while forwarding `id`, `name`, `required`, `disabled`, `aria-invalid`, `aria-describedby`, and event handlers without dropping consumer attributes.

- [ ] Replace `FormField` child cloning with composition. The control must receive the field id and `aria-invalid`; hint and error ids must be combined rather than overwritten:

```tsx
const describedBy = [hintId, error ? errorId : null]
  .filter(Boolean)
  .join(" ") || undefined;

<Field data-invalid={Boolean(error)}>
  <FieldLabel htmlFor={id}>{label}</FieldLabel>
  {control}
  {hint ? <FieldDescription id={hintId}>{hint}</FieldDescription> : null}
  {error ? <FieldError id={errorId}>{error}</FieldError> : null}
</Field>
```

- [ ] Write primitive tests before refactoring consumers:

```tsx
it("maps the legacy danger variant to the destructive visual contract", () => {
  render(<LegacyButton variant="danger">Delete</LegacyButton>);
  expect(screen.getByRole("button", { name: "Delete" })).toHaveAttribute(
    "data-variant",
    "destructive",
  );
});

it("connects a field label, hint, and error to the input", () => {
  render(
    <FieldWithError label="Username" hint="Use your Warwick ID" error="Required" />,
  );
  const input = screen.getByRole("textbox", { name: "Username" });
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(input.getAttribute("aria-describedby")).toMatch(/hint/);
  expect(input.getAttribute("aria-describedby")).toMatch(/error/);
});
```

- [ ] Run focused tests, typecheck, and build:

```bash
npx vitest run src/components/ui/__tests__/button.test.tsx src/components/ui/__tests__/field.test.tsx src/test/FormField.test.tsx
npm run typecheck
npm run build
```

- [ ] Commit the primitive foundation:

```bash
git add src/components/ui package.json package-lock.json
git commit -m "feat: add canonical form and display primitives"
```

## Task 4: Standardize overlays, menus, selection, tabs, and feedback

**Files:**

- Create: `src/components/ui/dialog.tsx`, `sheet.tsx`, `alert-dialog.tsx`, `popover.tsx`, `command.tsx`, `combobox.tsx`, `tabs.tsx`, `table.tsx`, `sonner.tsx`
- Create: `src/components/ui/__tests__/dialog.test.tsx`, `combobox.test.tsx`
- Modify: `src/components/Modal.tsx`, `src/components/SlideOver.tsx`, `src/components/ui/DropdownMenu.tsx`, `src/components/ui/Tooltip.tsx`, `src/hooks/useToast.tsx`, `src/components/absences/SidePanel.tsx`
- Test: `src/components/__tests__/Modal.test.tsx`, `src/components/__tests__/DropdownMenu.test.tsx`, `src/components/__tests__/Tooltip.test.tsx`

- [ ] Read the current shadcn documentation before implementation:

```bash
npx shadcn@latest docs dialog sheet alert-dialog popover command tabs table sonner tooltip
```

- [ ] Add the components after reviewing the dry run:

```bash
npx shadcn@latest add dialog sheet alert-dialog popover command tabs table sonner tooltip --dry-run
npx shadcn@latest add dialog sheet alert-dialog popover command tabs table sonner tooltip
```

- [ ] Replace the native dialog lifecycle in `Modal.tsx` and `SlideOver.tsx` with adapters. Every dialog-like surface must have a title, support Escape, trap focus, restore focus to the trigger, lock body scrolling, and expose `aria-modal` through the primitive.

- [ ] Route destructive actions through `AlertDialog`. Do not use a normal Dialog for irreversible deletion, cancellation, or override actions.

- [ ] Consolidate `TypeaheadSelect.tsx` and `MultiTeacherSelect.tsx` on the Combobox contract. The input must expose `role="combobox"`, `aria-expanded`, `aria-controls`, and `aria-activedescendant`; options must use one consistent option id scheme.

- [ ] Replace the custom toast provider with the shadcn Sonner host. Maintain existing call sites temporarily through a `useToast` adapter, but ensure dismiss buttons have accessible names and duplicate server errors are deduplicated.

- [ ] Add interaction tests:

```tsx
it("returns focus to the trigger after Escape closes a dialog", async () => {
  const user = userEvent.setup();
  render(<DialogFixture />);
  const trigger = screen.getByRole("button", { name: "Open" });
  await user.click(trigger);
  await user.keyboard("{Escape}");
  expect(trigger).toHaveFocus();
});

it("supports ArrowDown and Enter selection in the combobox", async () => {
  const user = userEvent.setup();
  render(<ComboboxFixture />);
  await user.click(screen.getByRole("combobox"));
  await user.keyboard("{ArrowDown}{Enter}");
  expect(screen.getByRole("combobox")).toHaveValue("Selected option");
});
```

- [ ] Run focused tests and the existing overlay/typeahead tests:

```bash
npx vitest run src/components/ui/__tests__/dialog.test.tsx src/components/ui/__tests__/combobox.test.tsx src/components/__tests__/Modal.test.tsx src/components/__tests__/DropdownMenu.test.tsx src/test/TypeaheadSelect.test.tsx
npm run typecheck
```

- [ ] Commit the behavior primitives:

```bash
git add src/components src/hooks/useToast.tsx package.json package-lock.json
git commit -m "feat: standardize overlays selection and feedback"
```

## Task 5: Refactor the application shell and page-level layout patterns

**Files:**

- Create: `src/components/layout/navigation.ts`, `AppShell.tsx`, `Sidebar.tsx`, `MobileNav.tsx`, `PageHeader.tsx`, `useLayoutStats.ts`
- Create: `src/components/patterns/FilterBar.tsx`, `LoadingState.tsx`, `ErrorState.tsx`, `StatusBadge.tsx`, `KpiCard.tsx`
- Modify: `src/components/Layout.tsx`, `src/components/ui/PageHeading.tsx`
- Test: `src/components/__tests__/Layout.test.tsx`

- [ ] Write a shell test that verifies navigation semantics before refactoring:

```tsx
it("exposes the mobile navigation state and returns focus on close", async () => {
  const user = userEvent.setup();
  render(<LayoutFixture />);
  const trigger = screen.getByRole("button", { name: /menu/i });
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  await user.click(trigger);
  expect(trigger).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByRole("navigation", { name: /mobile/i })).toBeVisible();
  await user.keyboard("{Escape}");
  expect(trigger).toHaveFocus();
});
```

- [ ] Move navigation definitions into `navigation.ts`. Keep route, label, icon component, required role, and active-match logic as typed data; do not keep navigation configuration inside the shell component.

- [ ] Move stats fetching into `useLayoutStats.ts`. `AppShell` must own layout composition only; it must not fetch unrelated page data or perform domain mutations.

- [ ] Implement `PageHeader` with this contract:

```ts
type PageHeaderProps = {
  title: string;
  description?: string;
  breadcrumbs?: React.ReactNode;
  actions?: React.ReactNode;
  children?: React.ReactNode;
};
```

- [ ] Replace `PageHeading` usage with `PageHeader` on Home, Reports, Absences, Schedule, Courses, and CRM screens. Keep `PageHeading` as a compatibility wrapper until all page consumers are migrated.

- [ ] Run shell and route tests:

```bash
npx vitest run src/components/__tests__/Layout.test.tsx src/pages/__tests__/Home.test.tsx
npm run typecheck
npm run build
```

- [ ] Commit the shell migration:

```bash
git add src/components/Layout.tsx src/components/layout src/components/patterns src/components/ui/PageHeading.tsx
git commit -m "refactor: build shared application shell patterns"
```

## Task 6: Create data, table, filter, and state patterns

**Files:**

- Create: `src/components/patterns/DataTable.tsx`, `FilterBar.tsx`, `KpiCard.tsx`, `StatusBadge.tsx`, `LoadingState.tsx`, `ErrorState.tsx`, `src/components/patterns/__tests__/DataTable.test.tsx`, `ErrorState.test.tsx`
- Create: `src/features/schedule/api/useHomeScheduleData.ts`, `src/features/reports/api/useReportsData.ts`
- Modify: `src/pages/Home.tsx`, `src/pages/Reports.tsx`, `src/components/teacher/SessionTable.tsx`

- [ ] Define the table state contract before migrating markup:

```ts
type DataTableState =
  | { kind: "loading" }
  | { kind: "error"; message: string; onRetry: () => void }
  | { kind: "empty"; title: string; description?: string; action?: React.ReactNode }
  | { kind: "ready" };
```

- [ ] Write tests for loading, error, empty, and ready states. The error test must verify one visible error message and one retry button, not duplicated alerts.

- [ ] Implement `DataTable` with semantic `<table>`, caption support, responsive overflow, empty/error/loading slots, and an explicit row-action contract. A row must not be made clickable unless it is also keyboard reachable and exposes an accessible name.

- [ ] Convert `SessionTable` row navigation to one of these two valid contracts:

```tsx
<tr>
  <td><Link to={detailsUrl}>Session details</Link></td>
</tr>
```

or:

```tsx
<tr tabIndex={0} onKeyDown={handleEnterOrSpace} aria-label={rowLabel}>
```

Prefer the link contract because it exposes native browser behavior.

- [ ] Extract Home fetching into `useHomeScheduleData`. It must use the existing TanStack Query client and return typed values for sessions, courses, rooms, and teachers. Remove manual page-level loading/error state once the hook is in use.

- [ ] Extract Reports data into `useReportsData`. Preserve existing query keys and cache behavior; do not introduce a second query client.

- [ ] Replace raw Reports tabs with the canonical `Tabs` component and the fixed `grid-cols-3` KPI layout with responsive grid behavior.

- [ ] Run focused tests:

```bash
npx vitest run src/components/patterns/__tests__/DataTable.test.tsx src/components/patterns/__tests__/ErrorState.test.tsx src/pages/__tests__/Home.test.tsx src/pages/__tests__/Schedule.test.tsx
npm run typecheck
```

- [ ] Commit the data/display patterns:

```bash
git add src/components/patterns src/features/schedule/api src/features/reports/api src/pages/Home.tsx src/pages/Reports.tsx src/components/teacher/SessionTable.tsx
git commit -m "feat: add shared data and page state patterns"
```

## Task 7: Migrate the absence workflow as the first vertical slice

**Files:**

- Modify: `src/pages/AbsenceForm.tsx`, `src/components/absences/SidePanel.tsx`, `src/components/absences/KanbanView.tsx`, `src/components/absences/StaffCreateAbsenceModal.tsx`, related absence components
- Test: `src/pages/__tests__/AbsenceForm.test.tsx`, `AbsenceForm.errorHandling.test.tsx`, `src/components/absences/__tests__/StaffCreateAbsenceModal.test.tsx`, `KanbanView.test.tsx`, `test:e2e:absence`

- [ ] Add a failing error-deduplication test for the absence workflow:

```tsx
it("renders one actionable server error instead of duplicate alerts", async () => {
  mockAbsenceFormConfigFailure();
  render(<AbsenceForm />);
  expect(await screen.findAllByRole("alert")).toHaveLength(1);
  expect(screen.getByRole("button", { name: /retry/i })).toBeVisible();
});
```

- [ ] Migrate all absence form fields to `Field` composition. Every control must have a label, stable id, validation state, and combined hint/error description.

- [ ] Replace the custom SidePanel and modal implementations with Sheet/Dialog adapters. Use AlertDialog for cancel/override/destructive actions.

- [ ] Fix Kanban disclosure semantics with `aria-expanded`, `aria-controls`, and keyboard support for Enter and Space.

- [ ] Keep the intentional copy change from `e.g. W250389` to `e.g. W250389 or a nickname` only if product has approved it; otherwise restore the old copy. Update tests to assert the product decision, not an incidental placeholder.

- [ ] Run all absence tests and E2E coverage:

```bash
npx vitest run src/pages/__tests__/AbsenceForm.test.tsx src/pages/__tests__/AbsenceForm.errorHandling.test.tsx src/components/absences/__tests__/StaffCreateAbsenceModal.test.tsx src/components/absences/__tests__/KanbanView.test.tsx
npm run test:e2e:absence
npm run typecheck
```

- [ ] Commit the vertical slice:

```bash
git add src/pages/AbsenceForm.tsx src/components/absences src/pages/__tests__ src/components/absences/__tests__
git commit -m "refactor: migrate absence workflow to design system"
```

## Task 8: Migrate the admin screens by pattern, not by page-specific styling

**Files:**

- Modify: `src/pages/Absences.tsx`, `src/pages/Schedule.tsx`, `src/pages/CourseDetail.tsx`, `src/pages/CourseLevels.tsx`, `src/pages/Reports.tsx`, `src/pages/Home.tsx`, `src/pages/EmailReminders.tsx`, `src/pages/OperationsCalendar.tsx`, `src/pages/CrmAdmin.tsx`
- Test: the existing page tests for each migrated screen.

- [ ] Migrate one route group per commit in this order:

```text
Absences -> Schedule -> Courses -> Reports -> Operations -> CRM
```

- [ ] For each route group, replace these patterns before changing visual decoration:

```text
raw button       -> Button
raw input        -> Field + Input
raw select       -> Field + Select
custom alert     -> Alert
custom badge     -> Badge or StatusBadge
custom loading   -> Skeleton or LoadingState
custom empty     -> Empty
custom modal     -> Dialog or AlertDialog
custom panel     -> Sheet
raw tab buttons  -> Tabs
raw table        -> DataTable
```

- [ ] Remove arbitrary text sizes only after typography roles exist. Replace `text-[...]` with named component typography or approved semantic utility classes.

- [ ] Replace raw gray utilities with semantic tokens. A migrated page may use `text-muted-foreground`, `bg-muted`, `border-border`, and component variants, but not `text-gray-*` or `bg-gray-*` for product UI.

- [ ] Verify each route at 390px, 768px, and 1280px widths. Check that filters wrap, tables scroll intentionally, KPI cards reflow, and action buttons remain reachable.

- [ ] Run the route-specific test set after each group. Do not batch unrelated route failures into one commit:

```bash
npm test -- --reporter=dot src/pages/__tests__/Absences.test.tsx
npm test -- --reporter=dot src/pages/__tests__/Schedule.test.tsx
npm test -- --reporter=dot src/pages/__tests__/CourseDetail.create.test.tsx
npm test -- --reporter=dot src/pages/__tests__/AbsenceDashboard.test.tsx
npm run typecheck
```

- [ ] Commit each route group with a focused message, for example:

```bash
git add src/pages/Absences.tsx src/components/absences src/pages/__tests__/Absences.test.tsx
git commit -m "refactor: migrate absences admin screens"
```

## Task 9: Enforce the system and remove compatibility debt

**Files:**

- Modify: `package.json`
- Create: `scripts/check-design-system.mjs`
- Modify: all remaining legacy UI files identified by the check.

- [ ] Add a guard script that checks only changed source files and exits nonzero when new violations are introduced. It must flag:

```text
new raw <button> in pages/features
new raw form controls without Field
new text-gray-* or bg-gray-* in product UI
new text-[...] typography utilities
new space-y-* or space-x-* layout utilities
new manual template-literal class conditionals
```

- [ ] Add the script to `package.json`:

```json
{
  "scripts": {
    "ui:check": "node scripts/check-design-system.mjs"
  }
}
```

- [ ] Run the guard against the current tree and write an explicit allowlist for legacy files still pending migration. The guard must fail for a newly introduced violation, but it must not hide existing debt.

- [ ] Migrate remaining consumers from compatibility wrappers to canonical lowercase shadcn components.

- [ ] Delete compatibility wrappers only after this search returns no imports:

```bash
rg "components/ui/(Button|Input|Select|FormField|LoadingSkeleton|EmptyState)|components/(Modal|SlideOver)" src
```

- [ ] Run:

```bash
npm run ui:check
npm run typecheck
npm run build
```

- [ ] Commit the enforcement and cleanup:

```bash
git add scripts/check-design-system.mjs package.json src
git commit -m "chore: enforce design system usage"
```

## Task 10: Final verification and rollout gate

**Files:**

- Test: the full repository test suite, E2E suite, and manual browser matrix.

- [ ] Run the complete verification sequence:

```bash
npm run ui:check
npm run typecheck
npm test -- --reporter=dot
npm run test:e2e:absence
npm run build
git diff --check
git status --short
```

Expected result: all required checks pass; the full test suite has no new failures compared with the baseline and preferably has fewer; the production build succeeds; `git diff --check` is clean.

- [ ] Manually verify the following viewport matrix:

```text
390x844   public absence flow, mobile navigation, dialogs, tables
768x1024  tablet filters, side panels, responsive tables
1280x720  admin shell, sidebar, reports, schedule, course pages
```

- [ ] Keyboard-verify these flows:

```text
skip link -> navigation -> page actions
open/close Dialog and Sheet with Enter, Escape, and focus return
DropdownMenu arrows, Enter, Escape
Combobox arrows, Home, End, Enter, Escape
Tabs arrows and selected-panel association
table row actions without click-only interaction
```

- [ ] Screen-reader-verify:

```text
every form control has one accessible name
every validation message is associated with its control
every Dialog/Sheet has a title
every status has text, not color alone
every toast has a clear announcement and dismiss label
```

- [ ] Verify reduced motion and no horizontal overflow at 390px:

```bash
rg "motion-reduce|prefers-reduced-motion" src/index.css src/components src/pages
```

- [ ] Remove obsolete adapters and update the design-system documentation with the final token, component, and migration rules.

- [ ] Only then create the release commit:

```bash
git add .
git commit -m "feat: complete Warwick design system migration"
```

## Completion criteria

The migration is complete only when all of the following are true:

- shadcn is the only UI component foundation.
- Semantic tokens are used by all migrated UI.
- No new raw controls, raw status colors, arbitrary typography, or `space-*` layout utilities can enter the codebase unnoticed.
- All overlay, field, menu, tab, combobox, table, toast, loading, and empty-state contracts are canonical.
- The public absence flow and admin shell pass desktop, tablet, and mobile review.
- Keyboard and screen-reader behavior is verified for every interactive primitive.
- The full test suite, typecheck, build, E2E absence flow, and `ui:check` pass.
- Compatibility wrappers and migration allowlists are removed or reduced to documented, intentional exceptions.
