# Architecture

**Analysis Date:** 2026-06-06

## System Overview

```text
┌─────────────────────────────────────────────────────────────┐
│                    Frontend (React SPA)                      │
├──────────────────┬──────────────────┬───────────────────────┤
│   Pages Layer    │  Components Layer │    Hooks Layer        │
│  `src/pages/`    │  `src/components/` │   `src/hooks/`       │
│  33 page files    │  37+ components   │   21 custom hooks     │
└────────┬─────────┴────────┬─────────┴──────────┬────────────┘
         │                  │                     │
         ▼                  ▼                     ▼
┌─────────────────────────────────────────────────────────────┐
│                      API Layer                               │
│         `src/api/client.ts` + custom hooks                  │
│         (idempotency, auth, error handling)                  │
└─────────────────────────┬───────────────────────────────────┘
                          │ HTTP (fetch)
                          ▼
┌─────────────────────────────────────────────────────────────┐
│              Backend (Go Modular Monolith)                    │
├──────────────────┬──────────────────┬───────────────────────┤
│   `internal/`    │   `db/queries/`   │   `db/migrations/`    │
│   19 modules      │   12 query files  │   34 migration files  │
│   (httpapi, auth, │   (sqlc generated) │   (goose)             │
│    scheduling,    │                    │                       │
│    series, etc)   │                    │                       │
└──────────────────┴──────────────────┴───────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                    PostgreSQL Database                        │
│    (scheduling, users, courses, subjects, absences)          │
└─────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| `Schedule` | Main scheduling page - calendar view, session CRUD, series CRUD | `src/pages/Schedule.tsx` |
| `SessionOccurrenceForm` | One-off session create/edit form | `src/components/SessionOccurrenceForm.tsx` |
| `SeriesFormFields` | Recurring series form fields (weekdays, time, duration, count) | `src/components/SeriesFormFields.tsx` |
| `TypeaheadSelect` | Searchable dropdown with keyboard navigation | `src/components/TypeaheadSelect.tsx` |
| `Modal` | Accessible modal dialog with focus trap | `src/components/Modal.tsx` |
| `FormField` | Form field wrapper with label, error, hint | `src/components/ui/FormField.tsx` |
| `Button` | Button with variants (primary/secondary/danger/ghost) and loading | `src/components/ui/Button.tsx` |
| `Select` | Native select dropdown with styling | `src/components/ui/Select.tsx` |
| `Input` | Input with sizes and error state | `src/components/ui/Input.tsx` |
| `SearchInput` | Search input with icon | `src/components/ui/SearchInput.tsx` |
| `PageHeading` | Page title heading | `src/components/ui/PageHeading.tsx` |
| `EmptyState` | Empty state placeholder with icon | `src/components/ui/EmptyState.tsx` |
| `LoadingSkeleton` | Loading placeholder (table/card/text variants) | `src/components/ui/LoadingSkeleton.tsx` |
| `FormErrorSummary` | Error summary with links to fields | `src/components/ui/FormErrorSummary.tsx` |
| `ConfirmModal` | Confirmation dialog for destructive actions | `src/components/ConfirmModal.tsx` |
| `PreflightIndicator` | Availability check indicator | `src/components/PreflightIndicator.tsx` |
| `ScheduleFilters` | Date range/time filters for schedule | `src/components/ScheduleFilters.tsx` |
| `SessionActions` | Session action buttons (edit, cancel, attendance) | `src/components/SessionActions.tsx` |
| `Layout` | App shell with nav, content area, footer | `src/components/Layout.tsx` |
| `AbsenceForm` | Student absence submission form | `src/pages/AbsenceForm.tsx` |
| `Absences` | Absence inbox with table/kanban views | `src/pages/Absences.tsx` |
| `Courses` | Course list with search/filter | `src/pages/Courses.tsx` |
| `Subjects` | Subject list with search | `src/pages/Subjects.tsx` |
| `Teachers` | Teacher list with search | `src/pages/Teachers.tsx` |
| `Students` | Student list with search | `src/pages/Students.tsx` |

## Pattern Overview

**Overall:** Single-Page Application with flat page routing and custom hooks for state/data.

**Key Characteristics:**
- React 19.2.6 with TypeScript 5.9.3
- Vite 7.3.2 build tool (single-file bundle for production)
- Tailwind CSS 4.1.17 for styling (no CSS modules)
- Custom UI primitives (shadcn-inspired but hand-rolled)
- Custom hooks for data fetching (useApiQuery, useApiMutation)
- Custom form validation hook (useFormValidation)
- No global state management (local state + context only)
- Backend proxy in dev mode (`/api` → `localhost:8080`)

## Layers

**Pages Layer:**
- Purpose: Route-specific page components
- Location: `src/pages/`
- Contains: 33 page components (Schedule, Courses, Subjects, Teachers, Students, Absences, etc.)
- Depends on: Components, Hooks, Types, API client
- Used by: Router in `src/App.tsx`

**Components Layer:**
- Purpose: Reusable UI components
- Location: `src/components/`
- Contains: 37+ components including `ui/` primitives, scheduling components, absence components
- Depends on: React, Tailwind, Lucide icons, custom hooks
- Used by: Pages layer

**Hooks Layer:**
- Purpose: Custom React hooks for data fetching, auth, form validation
- Location: `src/hooks/`
- Contains: 21 hooks
- Depends on: API client, Types
- Used by: Pages and Components

**API Layer:**
- Purpose: HTTP client with auth, idempotency, error handling
- Location: `src/api/client.ts`
- Contains: `apiJson<T>()`, `apiUpload<T>()`, `ApiRequestError`, `newIdempotencyKey()`
- Depends on: Browser fetch API
- Used by: All hooks and pages

**Types Layer:**
- Purpose: TypeScript type definitions
- Location: `src/types/index.ts`
- Contains: All domain types (Session, Course, Subject, Teacher, etc.)
- Used by: All layers

**Utils Layer:**
- Purpose: Pure utility functions
- Location: `src/utils/`
- Contains: `cn.ts`, `time.ts`, `timezone.ts`, `preflight.ts`, etc.
- Used by: Components and hooks

## Data Flow

### Primary Data Fetching Path

1. Page component calls custom hook (e.g., `useApiQuery<T>(url)`) (`src/hooks/useApiQuery.ts:11`)
2. Hook calls `apiJson<T>(path)` from API client (`src/api/client.ts:119`)
3. API client adds idempotency key for mutations, sends fetch request (`src/api/client.ts:128-138`)
4. Response parsed as JSON, returned to hook (`src/api/client.ts:143`)
5. Hook updates state, triggers re-render (`src/hooks/useApiQuery.ts:36-43`)

### Scheduling Preflight Flow

1. User fills form fields (course, teacher, time) (`src/pages/Schedule.tsx:375-392`)
2. `usePreflight` hook calls `POST /api/v1/scheduling/preflight` (`src/hooks/usePreflight.ts`)
3. Backend returns `Available|Provisional|Blocked` + structured conflicts
4. `PreflightIndicator` renders status (`src/components/PreflightIndicator.tsx`)
5. Save button enabled only when preflight passes (Available or Provisional)

### Session Create Flow

1. User opens modal, fills `SessionOccurrenceForm` (`src/pages/Schedule.tsx:678-715`)
2. Preflight runs automatically on form changes (`src/pages/Schedule.tsx:394-398`)
3. User clicks Create, `useCreateSession` hook sends `POST /api/v1/sessions` (`src/hooks/useCreateSession.ts`)
4. API client adds `Idempotency-Key` header (`src/api/client.ts:128-130`)
5. On success, `load()` refreshes session list (`src/pages/Schedule.tsx:52-73`)

**State Management:**
- Local state via `useState` for all page-level data
- `AuthProvider` context for auth state (`src/hooks/useAuth.tsx`)
- `ToastProvider` context for toast notifications (`src/hooks/useToast.tsx`)
- No Redux, Zustand, or other global state library

## Key Abstractions

**useApiQuery<T>:**
- Purpose: Generic data fetching hook with loading/error/refetch
- Examples: `src/pages/Courses.tsx:45`, `src/pages/Subjects.tsx:21`
- Pattern: Returns `{ data, loading, error, refetch }`

**useApiMutation<TBody, TResp>:**
- Purpose: Generic mutation hook with loading/error
- Examples: `src/pages/Subjects.tsx:22`
- Pattern: Returns `{ mutate, loading, error, reset }`

**useFormValidation<T>:**
- Purpose: Schema-based form validation with touch tracking
- Examples: `src/pages/Schedule.tsx:99-112`
- Pattern: Returns `{ errors, validate, validateAll, touched, touch }`

**usePreflight:**
- Purpose: Scheduling preflight check (availability/conflict detection)
- Examples: `src/pages/Schedule.tsx:97`
- Pattern: Returns `{ status, details, check, loading, reset }`

**TypeaheadSelect:**
- Purpose: Searchable dropdown with keyboard navigation
- Examples: `src/components/TypeaheadSelect.tsx`
- Pattern: Props-based (`value`, `onChange`, `options`, `placeholder`)

**Modal:**
- Purpose: Accessible modal dialog with focus trap
- Examples: `src/components/Modal.tsx`
- Pattern: Props-based (`title`, `onClose`, `footer`, `size`)

## Entry Points

**Frontend Entry:**
- Location: `src/main.tsx`
- Triggers: Browser loads `index.html` → Vite bundles → React renders
- Responsibilities: Mount React app with `<StrictMode>`

**App Router:**
- Location: `src/App.tsx`
- Triggers: Browser navigation
- Responsibilities: Route definitions, auth guard, toast/auth providers

**Backend Entry:**
- Location: `backend/cmd/` (Go binary)
- Triggers: Railway deployment
- Responsibilities: HTTP server, API handlers, DB access

## Architectural Constraints

- **Threading:** Single-threaded React event loop (browser); Go handles concurrency on backend
- **Global state:** No module-level singletons except `AuthContext` and `ToastContext` (React contexts)
- **Circular imports:** None detected (all imports are one-directional)
- **Bundle strategy:** `vite-plugin-singlefile` inlines all assets into single HTML file for production
- **Dev proxy:** Vite dev server proxies `/api` to `localhost:8080` (Go backend)

## Anti-Patterns

### Inline API Calls in Components

**What happens:** Some page components call `apiJson()` directly in event handlers instead of using hooks
**Why it's wrong:** Inconsistent error handling, no loading state management, harder to test
**Do this instead:** Use `useApiQuery` for reads and `useApiMutation` for writes (see `src/pages/Subjects.tsx:22`)

### Duplicate Form State

**What happens:** Schedule page manages 6+ separate form state objects for different modals
**Why it's wrong:** Complex state management, easy to introduce bugs when copying patterns
**Do this instead:** Extract form logic into custom hooks (already done for create/edit sessions, but series forms could benefit)

### No Error Boundary

**What happens:** React errors in page components crash the entire app
**Why it's wrong:** Poor UX, no graceful degradation
**Do this instead:** Add React Error Boundary around route content in `src/App.tsx`

## Error Handling

**Strategy:** API errors wrapped in `ApiRequestError` class with code/status/details; toast notifications for user feedback.

**Patterns:**
- `apiJson()` throws `ApiRequestError` on non-OK responses (`src/api/client.ts:37-49`)
- Custom error codes: `stale_edit`, `conflict`, `unauthorized` (`src/api/client.ts:25-35`)
- Toast notifications via `useToast().addToast(type, message)` (`src/hooks/useToast.tsx`)
- Form validation errors displayed via `FormErrorSummary` component (`src/components/ui/FormErrorSummary.tsx`)

## Cross-Cutting Concerns

**Logging:** Not implemented on frontend; Go backend has logging module (`backend/internal/logging/`)

**Validation:** Custom `useFormValidation` hook with schema-based rules (required, min, max, pattern, custom) (`src/hooks/useFormValidation.ts`)

**Authentication:** Session-based auth via `HttpOnly` cookie; `AuthProvider` context manages state (`src/hooks/useAuth.tsx`)

**Authorization:** Role-based (Admin/Teacher) checked server-side; frontend conditionally renders UI based on `user.role`

**Idempotency:** All mutation requests automatically include `Idempotency-Key` header via `apiJson()` wrapper (`src/api/client.ts:119-144`)

**Timezone:** Institute timezone (`Asia/Bangkok`) fetched from backend; all times stored UTC in DB, converted for display (`src/hooks/useInstituteMeta.ts`)

---

*Architecture analysis: 2026-06-06*
