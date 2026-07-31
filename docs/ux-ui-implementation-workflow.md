# Multi-Agent Implementation Workflow

## Architecture Overview

```
                    ┌─────────────────────────────┐
                    │   ORCHESTRATOR (me)          │
                    │  - Assigns work packages     │
                    │  - Reviews completed work    │
                    │  - Resolves conflicts        │
                    │  - Runs verify commands      │
                    └───────────┬─────────────────┘
                                │
             ┌──────────────────┼──────────────────┐
             │                  │                  │
     ┌───────▼───────┐  ┌──────▼──────┐  ┌───────▼───────┐
     │  Track A      │  │  Track B    │  │  Track C      │
     │  Components   │  │  A11y+UX    │  │  Schedule     │
     │  (Phase 0→2)  │  │  (Phase 1)  │  │  Decomp       │
     └───────┬───────┘  └──────┬──────┘  └───────┬───────┘
             │                  │                  │
     ┌───────▼───────┐  ┌──────▼──────┐  ┌───────▼───────┐
     │  Track D      │  │  Track E    │  │  Track F      │
     │  Forms        │  │  Responsive │  │  Polish       │
     │  (Phase 3)    │  │  (Phase 4)  │  │  (Phase 5)    │
     └───────┬───────┘  └──────┬──────┘  └───────┬───────┘
             │                  │                  │
             └──────────────────┴──────────────────┘
                              │
                    ┌─────────▼─────────┐
                    │  INTEGRATION      │
                    │  Full suite run   │
                    │  Conflicts?       │
                    │  Ship             │
                    └───────────────────┘
```

## Agent Isolation Strategy

**No two agents edit the same file in parallel.** File ownership per track:

| File Pattern | Track A | Track B | Track C | Track D | Track E | Track F |
|-------------|---------|---------|---------|---------|---------|---------|
| `src/components/ui/*` | ✅ | | | | | |
| `src/components/Modal.tsx` | ✅ | | | | | |
| `src/components/TypeaheadSelect.tsx` | | ✅ | | | | |
| `src/components/Layout.tsx` | | | | | ✅ | |
| `src/hooks/*.ts` (new) | | ✅ | ✅ | ✅ | | |
| `src/pages/CourseCreate.tsx` | ✅ | | | ✅ | ✅ | |
| `src/pages/CourseEdit.tsx` | ✅ | | | ✅ | | |
| `src/pages/TeacherCreate.tsx` | ✅ | | | ✅ | ✅ | |
| `src/pages/SubjectCreate.tsx` | ✅ | | | ✅ | ✅ | |
| `src/pages/Classrooms.tsx` | ✅ | | | ✅ | ✅ | |
| `src/pages/Users.tsx` | ✅ | | | ✅ | | |
| `src/pages/Login.tsx` | ✅ | ✅ | | ✅ | | |
| `src/pages/Schedule.tsx` | ✅ | ✅ | ✅ | ✅ | | |
| `src/pages/*.tsx` (other) | ✅ | | | | ✅ | |
| `src/index.css` | ✅ | | | | | ✅ |

**Conflict detection**: If Track A and Track D both need to edit `CourseCreate.tsx`, they edit non-overlapping sections (A: swap class strings for components; D: wrap in FormField). Sequence them or merge after.

## Workflow Stages

### Stage 1: Foundation (2 agents in parallel)

| Agent | Task | Files Created | Files Modified |
|-------|------|---------------|----------------|
| **A1: Design System** | Create Input, Select, Button, PageHeading, SearchInput, EmptyState, LoadingSkeleton, FormField, FormErrorSummary | `src/components/ui/Input.tsx`, `Select.tsx`, `Button.tsx`, `PageHeading.tsx`, `SearchInput.tsx`, `EmptyState.tsx`, `LoadingSkeleton.tsx`, `FormField.tsx`, `FormErrorSummary.tsx` | `src/index.css` (add tokens) |
| **B1: A11y Fixes** | Modal focus trap, TypeaheadSelect ARIA, Toast a11y, ConfirmModal | `src/components/ConfirmModal.tsx` | `src/components/Modal.tsx`, `src/components/TypeaheadSelect.tsx`, `src/hooks/useToast.tsx` |

**Gate**: Both must complete. Then all subsequent tracks depend on A1 being done.

### Stage 2: Sweep + Schedule Decomp (3 agents in parallel)

| Agent | Task | Depends On | Files |
|-------|------|-----------|-------|
| **A2: Visual Sweep** | Replace ad-hoc classes with Input/Select/Button/PageHeading across all pages, fix hex→vars, dead CSS, copyright | A1 | 20+ page files |
| **C1: Schedule Decomp** | Extract ConfirmModal usage, ScheduleFilters, useInstituteMeta, useLookups, SessionActions | A1 (for Button), B1 (for ConfirmModal replacement) | `src/pages/Schedule.tsx` + new files |
| **B2: UX Bug Fixes** | Login submit fix, preflight gate hook, remove no-op buttons, `usePreflightGate` hook | — | `src/pages/Login.tsx`, `src/pages/Courses.tsx`, `src/pages/Subjects.tsx`, `src/pages/Teachers.tsx`, `src/pages/Home.tsx`, `src/hooks/usePreflightGate.ts` |

**Gate**: A2 must complete before D1 (form integration needs components in place).

### Stage 3: Forms + Responsive (2 agents in parallel)

| Agent | Task | Depends On | Files |
|-------|------|-----------|-------|
| **D1: Form Validation** | Create useFormValidation, useDirtyForm hooks. Integrate FormField + FormErrorSummary + validation into all form pages | A1, A2 (components exist and are swept) | `src/hooks/useFormValidation.ts`, `src/hooks/useDirtyForm.ts`, 8 form pages |
| **E1: Responsive** | Navigation hamburger, table overflow-x-auto, touch targets, responsive grids, modal body scroll | — | `src/components/Layout.tsx`, 15 table files, 3 create form pages |

**Gate**: D1 and E1 can run in parallel — they touch different files.

### Stage 4: Schedule Continue + Polish (2 agents in parallel)

| Agent | Task | Depends On | Files |
|-------|------|-----------|-------|
| **C2: Schedule Deep** | Extract SessionOccurrenceForm, SeriesFormFields, AttendancePanel, all remaining hooks | A1, A2, C1 | Schedule.tsx + new hook files |
| **F1: Polish** | Modal animations, TypeaheadSelect animation, Toast animation | — | `src/components/Modal.tsx`, `src/components/TypeaheadSelect.tsx`, `src/hooks/useToast.tsx`, `src/index.css` |

### Stage 5: Integration

| Agent | Task |
|-------|------|
| **Orchestrator** | Run full build (`npm run build`), typecheck (`npm run typecheck` if available), lint. Run test suite. Resolve any merge conflicts between tracks. |

## Per-Agent Prompt Template

Each agent receives:

```
You are working on Track X: [track name].

Files YOU will create: [list]
Files YOU will modify: [list]
DO NOT touch: [list - files owned by other tracks]

Context from the spec: [relevant sections from ux-ui-overhaul-spec.md]

Return: Summary of what you created/changed, any issues encountered.
```

## File Ownership Matrix (Complete)

To prevent conflicts, a file is owned by exactly ONE track at a time:

| File | Stage 1 | Stage 2 | Stage 3 | Stage 4 | Stage 5 |
|------|---------|---------|---------|---------|---------|
| `src/components/ui/Input.tsx` | **A1** | — | — | — | — |
| `src/components/ui/Select.tsx` | **A1** | — | — | — | — |
| `src/components/ui/Button.tsx` | **A1** | — | — | — | — |
| `src/components/ui/PageHeading.tsx` | **A1** | — | — | — | — |
| `src/components/ui/SearchInput.tsx` | **A1** | — | — | — | — |
| `src/components/ui/EmptyState.tsx` | **A1** | — | — | — | — |
| `src/components/ui/LoadingSkeleton.tsx` | **A1** | — | — | — | — |
| `src/components/ui/FormField.tsx` | **A1** | — | — | — | — |
| `src/components/ui/FormErrorSummary.tsx` | **A1** | — | — | — | — |
| `src/index.css` | **A1** | — | — | — | **F1** |
| `src/components/Modal.tsx` | **B1** | — | — | **F1** | — |
| `src/components/TypeaheadSelect.tsx` | **B1** | — | — | **F1** | — |
| `src/components/ConfirmModal.tsx` | **B1** | — | — | — | — |
| `src/hooks/useToast.tsx` | **B1** | — | — | **F1** | — |
| `src/components/Layout.tsx` | — | — | **E1** | — | — |
| `src/pages/Login.tsx` | — | **B2** | — | — | — |
| `src/pages/Courses.tsx` | — | **B2, A2** | — | — | — |
| `src/pages/Subjects.tsx` | — | **B2, A2** | — | — | — |
| `src/pages/Teachers.tsx` | — | **B2, A2** | — | — | — |
| `src/pages/Home.tsx` | — | **B2, A2** | — | — | — |
| `src/pages/Schedule.tsx` | — | **A2, C1** | — | **C2** | — |
| `src/pages/CourseCreate.tsx` | — | **A2** | **D1, E1** | — | — |
| `src/pages/CourseEdit.tsx` | — | **A2** | **D1** | — | — |
| `src/pages/TeacherCreate.tsx` | — | **A2** | **D1, E1** | — | — |
| `src/pages/SubjectCreate.tsx` | — | **A2** | **D1, E1** | — | — |
| `src/pages/Classrooms.tsx` | — | **A2** | **D1, E1** | — | — |
| `src/pages/Users.tsx` | — | **A2** | **D1** | — | — |
| All other pages | — | **A2** | — | — | — |
| `src/hooks/usePreflightGate.ts` | — | **B2** | — | — | — |
| `src/hooks/useFormValidation.ts` | — | — | **D1** | — | — |
| `src/hooks/useDirtyForm.ts` | — | — | **D1** | — | — |
| Schedule decomp new files | — | **C1** | — | **C2** | — |

## Verification Gate Checklist

After each stage, orchestrator runs:

```bash
npm run build       # must pass
npm run lint        # if available
npx tsc --noEmit    # typecheck
npm test            # unit tests
```

**All four must pass** before dispatching next stage. If a stage fails, fix before proceeding.

## Rollback Plan

If a stage introduces regressions:
- Track A/B/C/D/E/F each owns a distinct file set → rollback by reverting those files
- `git checkout -- <file>` on affected files
- Re-dispatch the failing agent with the error context

## Stage-by-Stage Agent Dispatch Commands

### Stage 1 (parallel)

```
Task("Track A1: Design System — create Input, Select, Button, PageHeading, SearchInput, EmptyState, LoadingSkeleton, FormField, FormErrorSummary components + add CSS tokens to index.css — see docs/ux-ui-overhaul-spec.md Phase 0")
Task("Track B1: A11y Fixes — Modal focus trap + ARIA, TypeaheadSelect combobox + keyboard nav, Toast a11y, ConfirmModal component — see docs/ux-ui-overhaul-spec.md Phase 1")
```

### Stage 2 (parallel)

```
Task("Track A2: Visual Sweep — replace ad-hoc classes with Input/Select/Button/PageHeading across all pages, fix hex→vars, dead CSS, copyright — see docs/ux-ui-overhaul-spec.md Phase 2")
Task("Track C1: Schedule Decomp Part 1 — extract ConfirmModal usage, ScheduleFilters, useInstituteMeta, useLookups, SessionActions — see Schedule.tsx decomposition in spec")
Task("Track B2: UX Bug Fixes — Login submit fix, usePreflightGate hook, remove no-op search buttons — see Phase 1.3, 1.5, 1.7")
```

### Stage 3 (parallel)

```
Task("Track D1: Form Validation — create useFormValidation, useDirtyForm hooks. Integrate FormField + FormErrorSummary + validation into CourseCreate, CourseEdit, TeacherCreate, SubjectCreate, Classrooms, Users, Login — see Phase 3")
Task("Track E1: Responsive — nav hamburger, table overflow-x-auto, touch targets, responsive grids, modal body scroll — see Phase 4")
```

### Stage 4 (parallel)

```
Task("Track C2: Schedule Decomp Part 2 — extract SessionOccurrenceForm, SeriesFormFields, AttendancePanel, all remaining hooks — from Schedule.tsx")
Task("Track F1: Polish — modal/TypeaheadSelect/toast animations, button transitions — see Phase 5")
```

---

## Key Principles

1. **File isolation**: No two agents ever edit the same file simultaneously
2. **Dependency gating**: Stage N+1 only starts after Stage N passes verification
3. **Sequential per-file edits**: When a file is touched by 2 tracks (e.g., Schedule.tsx by A2 then C1), sequence them — never parallel
4. **Spec as source of truth**: All agents reference `docs/ux-ui-overhaul-spec.md` for API contracts
5. **Test gate**: Full build + typecheck + lint + test after each stage
6. **Rollback per-track**: Each track owns a distinct file set → isolated rollback
