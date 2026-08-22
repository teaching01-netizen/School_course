# Course Levels — Manage Groups clone-fidelity review

**Recommendation:** APPROVE  
**Review type:** B — focused visual fidelity and precision  
**Notepad:** none found in the workspace.

## Scope and acceptance criteria

Reviewed `Course Levels → Manage groups` against the stated outcome: a centered manager, all 18 groups in one scrollable unpaginated list, group selection, and a global course picker searchable by course code or subject name. The supplied old right-side, paginated screenshot was inspected as visual context only; its placement and pagination are intentionally superseded.

## Evidence trace

### Rendered artifacts inspected directly

- `/tmp/course-levels-manage-groups-final-unselected.png` — PNG, 1440×900. Shows the centered 896px modal, initial no-selection hierarchy, and the bounded group-list viewport.
- `/tmp/course-levels-manage-groups-final-selected.png` — PNG, 1440×900. Confirms the same 896px desktop modal width, selected `SAT Math` row, visible assignment hierarchy, course-code/subject-name presentation, and updated `2 courses` count.
- `/tmp/course-levels-manage-groups-mobile-selected.png` — PNG, 390×844. Shows the responsive 358px inner modal, readable selected state, and the `PHYS-201` search affordance without horizontal clipping.
- `/var/folders/my/ksg8mwx54gdb6xzdrmgvx7zh0000gn/T/TemporaryItems/NSIRD_screencaptureui_nNZJla/Screenshot 2569-08-22 at 17.54.08.png` — PNG, 1680×1562. Inspected as the legacy structural/content reference only; right-side placement and pagination were not evaluated as defects.

### Code and diff inspected directly

- `src/pages/CourseLevels.tsx:391-409,1310-1321` — the shared live `Modal` owns the dialog and renders `RootGroupManagerPanel`.
- `src/components/RootGroupManagerPanel.tsx:126-156,189-227` — course assignment sends the scoped PUT, updates live state, and composes the list/assignment panels.
- `src/components/RootGroupList.tsx:41-115` — the complete `groups` collection is rendered in one scrollable table with real selectable buttons; no page slice or pagination control remains.
- `src/components/GroupCourseAssignmentPanel.tsx:19-84` — global options are built from every loaded course; labels and keywords include course code and subject name; assigned courses render as live list items.
- `src/components/TypeaheadSelect.tsx:17-29` and `src/components/ui/SearchableSelect.tsx:160-168` — the reused typeahead primitive filters label, value, keywords, and description.
- `src/components/Modal.tsx:84-92`, `src/components/ui/Input.tsx`, and `src/components/ui/Button.tsx` — shared dialog and input/button primitives are used rather than one-off visual stand-ins.
- `src/index.css:322-348` and `DESIGN.md` §§2–6 — the manager-specific width is viewport safe; the new UI uses existing semantic color tokens, standard spacing/type utility scale, and existing dialog/scroll conventions.
- `src/hooks/useRootCourseGroups.ts` — obsolete pagination state was removed.
- Full working-tree diff for all of the above and the added `src/pages/__tests__/CourseLevels-manage-groups.test.tsx`.

### Independent validation run in this review

- `npm test -- --run src/pages/__tests__/CourseLevels-manage-groups.test.tsx` — PASS (1 file, 1 test): proves group selection, subject-name search, PUT body `{"root_course_group_id":"group-math"}`, and `PHYS-201` rendering.
- `npm run build` — PASS.
- `git diff --check` — PASS.
- `npm run typecheck` — not clean because of two unrelated pre-existing errors in `src/components/crm/__tests__/CrossStudyAssignmentList.test.tsx` (lines 25 and 109); neither file is in this feature's diff.

## Integrity assessment

- **Live component tree:** Confirmed. The reviewed UI is a `Modal` containing live React panels, table rows, buttons, input, and a reusable typeahead. No reviewed feature code uses a screenshot, raster image, canvas, or `background-image` as UI substitution.
- **Token-driven system:** Confirmed. New colors are semantic `--color-wi-*` tokens; controls reuse existing primitives; spacing, type, borders, radii, selected/hover/focus states, and scroll treatment follow the existing design system. The manager width is a single responsive layout rule (`min(56rem, calc(100vw - 2rem))`), not a visual fake.
- **Layer/layout structure:** Confirmed. The centered native dialog contains a title bar, add-group control, bounded scrollable group table, then a selected-group assignment section. This is an intentional replacement for the superseded right-side panel and pagination.
- **Visual fidelity and responsive precision:** Confirmed for the requested behavior. Desktop captures show a stable 896px inner modal before/after selection, centered horizontal placement, legible hierarchy, scroll hand-off, and no pagination. The mobile capture shows a 358px inner modal with safe side margins, wrapped group names, readable actions, and no horizontal clipping.

## Findings

### CRITICAL

None.

### HIGH

None.

### MEDIUM

None.

### LOW

None.

## Blocking before approval

None. The only validation caveat is the unrelated existing CRM-test type-check failure recorded above; it does not affect the reviewed feature or this visual-fidelity verdict.
