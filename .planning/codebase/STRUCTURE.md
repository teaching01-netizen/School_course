# Codebase Structure

**Analysis Date:** 2026-06-06

## Directory Layout

```
warwick-institute-ux-documentation copy 2/
├── src/                    # Frontend source (React SPA)
│   ├── api/                # HTTP client layer
│   ├── components/         # Reusable UI components
│   │   ├── __tests__/      # Component unit tests
│   │   ├── absences/       # Absence-specific components (16 files)
│   │   ├── crm/            # CRM components (1 file)
│   │   ├── tier-makeup/    # Empty - reserved for future
│   │   └── ui/             # Reusable UI primitives (12 files)
│   ├── constants/          # Empty - reserved for future
│   ├── data/               # Empty - reserved for future
│   ├── hooks/              # Custom React hooks (21 files)
│   ├── lib/                # Empty - reserved for future
│   ├── pages/              # Route-specific page components (33 files)
│   │   ├── __tests__/      # Page unit tests
│   │   └── operations/     # Operations sub-pages
│   ├── test/               # Test setup and shared tests (11 files)
│   ├── types/              # TypeScript type definitions
│   │   └── index.ts        # All domain types
│   ├── utils/              # Pure utility functions
│   │   ├── __tests__/      # Utility unit tests
│   │   └── *.ts            # Utility modules
│   ├── App.tsx             # Main app with routing
│   ├── index.css           # Tailwind config + custom styles
│   └── main.tsx            # Entry point
├── backend/                # Go backend (modular monolith)
│   ├── cmd/                # Main binary entry points
│   ├── db/
│   │   ├── migrations/     # 34 goose migration files
│   │   └── queries/        # 12 sqlc query files
│   ├── internal/           # 19 internal modules
│   │   ├── auth/           # Authentication
│   │   ├── config/         # Configuration
│   │   ├── httpapi/        # HTTP handlers
│   │   ├── scheduling/     # Scheduling logic
│   │   ├── series/         # Series recurrence logic
│   │   └── ...             # Other modules
│   └── go.mod              # Go module dependencies
├── docs/                   # Documentation
├── scripts/                # Build/dev scripts
├── specs/                  # Specifications
├── .planning/              # Planning documents (GSD)
├── index.html              # Vite HTML entry
├── package.json            # Node.js dependencies
├── tsconfig.json           # TypeScript config
├── vite.config.ts          # Vite build config
├── docker-compose.yml      # Docker compose
├── Dockerfile              # Docker build
├── railway.toml            # Railway deployment config
└── CONTEXT.md              # Project context document
```

## Directory Purposes

**`src/pages/`:**
- Purpose: Route-specific page components (one per route)
- Contains: 33 page files + `__tests__/` + `operations/` sub-pages
- Key files: `Schedule.tsx`, `Courses.tsx`, `Subjects.tsx`, `Teachers.tsx`, `Students.tsx`, `Absences.tsx`

**`src/components/`:**
- Purpose: Reusable UI components used across pages
- Contains: 37+ components organized by domain
- Key files: `Layout.tsx`, `Modal.tsx`, `TypeaheadSelect.tsx`, `SessionOccurrenceForm.tsx`, `SeriesFormFields.tsx`

**`src/components/ui/`:**
- Purpose: Low-level UI primitives (buttons, inputs, selects, etc.)
- Contains: 12 reusable components
- Key files: `Button.tsx`, `Input.tsx`, `Select.tsx`, `FormField.tsx`, `SearchInput.tsx`, `EmptyState.tsx`

**`src/hooks/`:**
- Purpose: Custom React hooks for data fetching, auth, validation
- Contains: 21 hooks
- Key files: `useApiQuery.ts`, `useApiMutation.ts`, `useFormValidation.ts`, `useLookups.ts`, `usePreflight.ts`, `useAuth.tsx`

**`src/api/`:**
- Purpose: HTTP client with idempotency, auth, error handling
- Contains: 1 file (`client.ts`)
- Key functions: `apiJson<T>()`, `apiUpload<T>()`, `ApiRequestError`

**`src/types/`:**
- Purpose: All TypeScript type definitions
- Contains: 1 file (`index.ts`, 535 lines)
- Key types: `Session`, `Course`, `Subject`, `Teacher`, `Room`, `Student`, `ConflictDetails`

**`src/utils/`:**
- Purpose: Pure utility functions (no React dependencies)
- Contains: 7 utility modules
- Key files: `cn.ts` (class merging), `time.ts`, `timezone.ts`, `preflight.ts`

**`backend/`:**
- Purpose: Go backend API server
- Contains: `cmd/`, `internal/`, `db/`
- Key modules: `httpapi/`, `scheduling/`, `series/`, `auth/`

**`backend/db/migrations/`:**
- Purpose: Database schema migrations (goose)
- Contains: 34 migration files
- Key files: `00001_init.sql`, `00003_scheduling.sql`, `00007_subjects_and_course_fields.sql`

**`backend/db/queries/`:**
- Purpose: SQL queries for sqlc code generation
- Contains: 12 query files
- Key files: `courses.sql`, `subjects.sql`, `sessions.sql`, `series.sql`

## Key File Locations

**Entry Points:**
- `src/main.tsx`: React app entry point
- `src/App.tsx`: Router and providers
- `index.html`: Vite HTML entry
- `backend/cmd/`: Go backend entry points

**Configuration:**
- `package.json`: Node.js dependencies and scripts
- `tsconfig.json`: TypeScript compiler options
- `vite.config.ts`: Vite build and dev server config
- `railway.toml`: Railway deployment config
- `backend/go.mod`: Go module dependencies
- `backend/sqlc.yaml`: sqlc code generation config

**Core Logic:**
- `src/pages/Schedule.tsx`: Main scheduling page (900+ lines)
- `src/components/SessionOccurrenceForm.tsx`: Session create/edit form
- `src/components/SeriesFormFields.tsx`: Series recurrence form fields
- `src/hooks/usePreflight.ts`: Scheduling preflight check
- `src/utils/preflight.ts`: Preflight validation utilities
- `backend/internal/scheduling/`: Backend scheduling logic
- `backend/internal/series/`: Backend series recurrence logic

**Testing:**
- `src/test/setup.ts`: Test setup file
- `src/test/*.test.tsx`: Shared test files
- `src/components/__tests__/`: Component tests
- `src/pages/__tests__/`: Page tests
- `src/utils/__tests__/`: Utility tests

## Naming Conventions

**Files:**
- Pages: PascalCase (e.g., `Schedule.tsx`, `CourseDetail.tsx`)
- Components: PascalCase (e.g., `TypeaheadSelect.tsx`, `FormField.tsx`)
- Hooks: camelCase with `use` prefix (e.g., `useApiQuery.ts`, `useFormValidation.ts`)
- Utils: camelCase (e.g., `cn.ts`, `time.ts`, `timezone.ts`)
- Types: camelCase (e.g., `index.ts`)
- Tests: `*.test.tsx` or `*.test.ts` (co-located or in `__tests__/` directories)

**Directories:**
- All lowercase with hyphens for multi-word (e.g., `absences/`, `tier-makeup/`)
- `__tests__/` for test files (co-located)
- `ui/` for reusable UI primitives

**Components:**
- Default exports for page components (e.g., `export default function Schedule()`)
- Named exports for UI primitives (e.g., `export default function Button()`)
- Props defined as interfaces (e.g., `interface ButtonProps`)

## Where to Add New Code

**New Page/Route:**
1. Create page component: `src/pages/NewPage.tsx`
2. Add route in `src/App.tsx`
3. Add nav link in `src/components/Layout.tsx`

**New Reusable Component:**
- Simple/atomic: `src/components/ui/ComponentName.tsx`
- Domain-specific: `src/components/ComponentName.tsx`
- Absence-related: `src/components/absences/ComponentName.tsx`

**New Custom Hook:**
- Data fetching: `src/hooks/useNewData.ts`
- UI state: `src/hooks/useNewState.ts`
- Form logic: `src/hooks/useNewForm.ts`

**New Utility Function:**
- Pure functions: `src/utils/newUtil.ts`
- Shared helpers: `src/utils/cn.ts`

**New Type Definition:**
- Domain types: `src/types/index.ts`
- Component props: Define in component file

**New Test File:**
- Component test: `src/components/__tests__/ComponentName.test.tsx`
- Hook test: `src/test/useNewHook.test.ts`
- Utility test: `src/utils/__tests__/newUtil.test.ts`

**New Backend Module:**
- Go module: `backend/internal/newmodule/`
- SQL queries: `backend/db/queries/newmodule.sql`
- Migration: `backend/db/migrations/XXXXX_description.sql`

## Special Directories

**`src/data/`:**
- Purpose: Static data or mock data (currently empty)
- Generated: No
- Committed: Yes (when populated)

**`src/constants/`:**
- Purpose: Shared constants (currently empty)
- Generated: No
- Committed: Yes (when populated)

**`src/lib/`:**
- Purpose: Shared library code (currently empty)
- Generated: No
- Committed: Yes (when populated)

**`src/components/tier-makeup/`:**
- Purpose: Tier makeup feature components (currently empty)
- Generated: No
- Committed: Yes (when populated)

**`node_modules/`:**
- Purpose: npm dependencies
- Generated: Yes
- Committed: No (in `.gitignore`)

**`dist/`:**
- Purpose: Vite build output
- Generated: Yes
- Committed: No (in `.gitignore`)

**`.planning/`:**
- Purpose: GSD planning documents
- Generated: Yes (by GSD commands)
- Committed: Yes

---

*Structure analysis: 2026-06-06*
