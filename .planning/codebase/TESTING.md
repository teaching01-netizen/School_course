# Testing Patterns

**Analysis Date:** Sat Jun 06 2026

## Test Framework

### Backend (Go)

**Runner:**
- Standard library `testing` package (Go 1.25.7)
- No external test runner — plain `go test`

**Assertion:**
- Pure Go standard library `t.Fatal`/`t.Fatalf`/`t.Errorf`
- No testify, no gomega, no assertion library at all

**Config:** No `go.test` configuration files. Tests rely on `TEST_DATABASE_URL` env var.

**Run Commands:**
```bash
go test ./backend/...               # Run all backend tests
go test ./backend/internal/db/...   # Run specific package (skips if no TEST_DATABASE_URL)
TEST_DATABASE_URL=postgres://... go test ./backend/...  # Run with test DB
```

### Frontend (TypeScript/React)

**Runner:**
- Vitest v4.1.7
- Config: `vite.config.ts` (test section at the bottom)

**Test Environment:**
- `jsdom` (v29.1.1)
- `globals: true` (no explicit imports needed for `describe`/`it`)
- `css: false` (CSS not parsed in tests)

**Run Commands:**
```bash
npm test              # vitest run
npm run test:watch    # vitest (watch mode)
npx vitest --coverage # Coverage (no coverage config found, likely not enforced)
```

---

## Test File Organization

### Go Backend

**Location:** Tests live **in same package** as the code they test, co-located in the same directory.

**Naming:** `*_test.go` per Go convention.

**Structure:**
```
backend/internal/
├── db/
│   ├── active_courses_integration_test.go
│   ├── absence_integration_test.go
│   ├── invariants_integration_test.go
│   ├── availability_policy_test.go
│   ├── absence_cycle_level_test.go
│   └── ... (11 test files total)
├── scheduling/
│   ├── service_integration_test.go
│   └── errors_test.go
├── idempotency/
│   ├── idempotency_test.go
│   └── requirekey_test.go
├── httpapi/
│   ├── sessionshttp/routes_test.go
│   ├── schedulinghttp/routes_preflight_test.go
│   ├── courseshttp/routes_integration_test.go
│   ├── courselevelshttp/routes_test.go
│   ├── absenceshttp/routes_test.go
│   ├── absenceshttp/resolver_test.go
│   ├── absenceshttp/rule_evaluator_test.go
│   ├── absenceshttp/success_sms_test.go
│   ├── corehttp/routes_test.go
│   ├── adminusershttp/adminusershttp_test.go
│   ├── makeupsettingshttp/routes_test.go
│   └── httpadapter/adapter_test.go
├── auth/service_test.go
├── otp/service_test.go
├── legacysync/client_test.go, syncer_test.go, room_matcher_test.go, parser_test.go
├── crmimport/crm_v2_integration_test.go, types_test.go, xlsx/xlsx_parse_test.go
└── users/admin_provisioning_integration_test.go
```

### Frontend (TypeScript/React)

**Location:** Three patterns exist:

1. **Page tests**: `src/pages/__tests__/<Component>.test.tsx` (24 test files)
   - e.g., `src/pages/__tests__/CourseLevels.test.tsx`
   - e.g., `src/pages/__tests__/Absences.test.tsx`
2. **Component tests**: `src/components/__tests__/<Component>.test.tsx` or `src/components/<area>/__tests__/<Component>.test.tsx`
   - e.g., `src/components/__tests__/RulePredicateForm.test.tsx`
   - e.g., `src/components/absences/__tests__/KanbanView.test.tsx`
3. **Hook/util tests**: `src/test/<hook>.test.ts` or `src/utils/__tests__/<util>.test.ts`
   - e.g., `src/test/usePreflight.test.ts`
   - e.g., `src/utils/__tests__/schedulePaste.test.ts`

**Setup:** `src/test/setup.ts` — single import of `@testing-library/jest-dom` matchers.

---

## Go Backend Test Patterns

### Pattern 1: Pure Unit Tests (no DB, no HTTP)

Found in packages where logic is isolated. Uses only the standard library.

**Example** — `backend/internal/scheduling/errors_test.go`:
```go
package scheduling

import (
    "errors"
    "testing"
)

func TestErrStaleEdit_MatchViaErrorsAs(t *testing.T) {
    wrapped := &Err{Code: "stale_edit", Message: "stale edit"}
    var se *Err
    if !errors.As(wrapped, &se) {
        t.Fatal("errors.As failed to extract *Err")
    }
    if se.Code != "stale_edit" {
        t.Fatalf("Code = %q, want %q", se.Code, "stale_edit")
    }
}
```

**Other examples:**
- `backend/internal/otp/service_test.go` — tests `NormalizePhoneE164` with table-driven tests
- `backend/internal/idempotency/requirekey_test.go` — uses `struct { name, key, wantErr }` table pattern
- `backend/internal/httpapi/httpadapter/adapter_test.go` — tests error classification
- `backend/internal/httpapi/absenceshttp/resolver_test.go` — tests helper functions with inline JSON fixtures

### Pattern 2: HTTP Handler Tests (httptest + fakeAuth)

Tests route registration and request validation without a real database. Uses `httptest.NewRecorder()` and `httptest.NewRequest()`.

**Example** — `backend/internal/httpapi/courselevelshttp/routes_test.go`:
```go
type fakeAuth struct {
    user auth.User
    err  error
}

func (f fakeAuth) RequireUser(ctx context.Context, r *http.Request) (auth.User, error) {
    return f.user, f.err
}
func (fakeAuth) HandleLogin(w http.ResponseWriter, r *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(w http.ResponseWriter, r *http.Request) error { return nil }

func TestRegister_PutLevel_BadID_Returns400(t *testing.T) {
    mux := http.NewServeMux()
    Register(mux, httpdeps.Deps{
        Auth: fakeAuth{user: auth.User{ID: uuid.New(), Username: "a", Role: "Admin"}},
    })

    req := httptest.NewRequest("PUT", "/api/v1/admin/courses/not-a-uuid/level",
        strings.NewReader(`{"level": 3, "cycle_id": "cy2025a"}`))
    w := httptest.NewRecorder()
    mux.ServeHTTP(w, req)

    if w.Code != http.StatusBadRequest {
        t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
    }
    var got struct { Code string `json:"code"` }
    if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
        t.Fatalf("decode json: %v", err)
    }
    if got.Code != "bad_id" {
        t.Fatalf("code = %q, want %q", got.Code, "bad_id")
    }
}
```

**Key characteristics:**
- `fakeAuth` struct satisfies the `auth` interface with canned user/error
- Tests register routes on `http.NewServeMux()` with `httpdeps.Deps`
- Uses inline JSON strings for request body
- Asserts HTTP status codes and JSON error shapes (`code` field)
- This pattern is **not** an integration test — no DB involved

**Used by:** `sessionshttp`, `courselevelshttp`, `corehttp`, `adminusershttp`

### Pattern 3: DB Integration Tests (real PostgreSQL)

Tests that exercise real SQL queries and database interactions.

**Three helper functions are duplicated across test packages:**

```go
// backend/internal/db/invariants_integration_test.go (and replicated in scheduling,
// idempotency, courseshttp, schedulinghttp, users, legacysync, crmimport)

func requireTestDB(t *testing.T) string {
    t.Helper()
    url := os.Getenv("TEST_DATABASE_URL")
    if url == "" {
        t.Skip("set TEST_DATABASE_URL to run DB integration tests")
    }
    return url
}

var migrationsOnce sync.Once
var migrationsErr error

func migrateUpOnce(t *testing.T, databaseURL string) {
    t.Helper()
    migrationsOnce.Do(func() {
        // Appends DSN params for simple protocol (compatible with Supabase/PgBouncer)
        // Calls goose.Up to apply all migrations from backend/db/migrations/
    })
    if migrationsErr != nil {
        t.Fatal(migrationsErr)
    }
}

func newPool(t *testing.T, databaseURL string) *pgxpool.Pool {
    t.Helper()
    // Creates pgxpool with simple protocol mode
}
```

**Example** — `backend/internal/db/availability_policy_test.go`:
```go
func TestAvailabilityPolicy_Alignment(t *testing.T) {
    databaseURL := requireTestDB(t)
    migrateUpOnce(t, databaseURL)
    dbpool := newPool(t, databaseURL)
    t.Cleanup(dbpool.Close)
    q := New(dbpool)

    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    suffix := time.Now().UTC().Format("20060102150405.000000000")
    teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-policy-" + suffix, Role: "Teacher", PasswordHash: "x"})
    // ... test logic with subtests
}
```

**Key characteristics:**
- Each test function creates its own pool (some packages use `setupTestServer` helper instead)
- Uses `time.Now().UTC().Format("...")` as unique suffix for test data
- Uses `t.Run("subtest_name", ...)` for subtests within a function
- Subtests share the same DB pool but test distinct scenarios
- Some packages use `t.Cleanup()` for pool close; others use `defer`
- Uses `pgtype` package heavily (UUID, Timestamptz, Int4, Int2)

### Pattern 4: Full HTTP Integration Tests (real DB + httptest.Server)

The most comprehensive pattern. Uses a real PostgreSQL database, creates test fixtures, and spins up a full HTTP test server.

**Example** — `backend/internal/httpapi/courseshttp/routes_integration_test.go`:
```go
type testFixture struct {
    server        *httptest.Server
    q             *sqldb.Queries
    dbpool        *pgxpool.Pool
    adminID       uuid.UUID
    courseID      pgtype.UUID
    teacherID     pgtype.UUID
    roomID        pgtype.UUID
    schedulingSvc *scheduling.Service
}

func setupTestServer(t *testing.T) *testFixture {
    t.Helper()
    databaseURL := requireTestDB(t)
    migrateUpOnce(t, databaseURL)
    dbpool := newPool(t, databaseURL)
    t.Cleanup(dbpool.Close)

    q := sqldb.New(dbpool)
    // Create series service, scheduling service
    // Create admin user, course, teacher, room in DB
    // Set up fakeAuth, httpdeps.Deps
    // Register routes, start httptest.NewServer
    // Return fixture with all IDs
}

func TestCreateSession_Success(t *testing.T) {
    f := setupTestServer(t)
    body := fmt.Sprintf(`{"course_id":"%s","teacher_id":"%s","start_at":"...","end_at":"..."}`, ...)
    resp, err := f.server.Client().Post(f.server.URL+"/api/v1/admin/courses/"+f.courseIDStr+"/sessions", "application/json", strings.NewReader(body))
    // assert response
}
```

**Key characteristics:**
- `setupTestServer(t)` returns a typed fixture struct
- `fakeAuth` is always embedded in `httpdeps.Deps`
- Routes registered via `Register(mux, deps)` pattern
- Uses `httptest.NewServer(mux)` for a real HTTP server
- Makes real HTTP requests with `f.server.Client().Get/Post`

**Used by:** `schedulinghttp/routes_preflight_test.go`, `courseshttp/routes_integration_test.go`

---

## Frontend Test Patterns

### Pattern 1: Page Component Tests (vitest + @testing-library)

**Mock pattern for API calls:**
```tsx
// backend/internal/httpapi/sessionshttp/  (No — this is frontend)
const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});
```

**Two mocking styles exist:**

1. **Sequential resolved values** — `mockResolvedValueOnce` chain:
```tsx
mockApiJson
  .mockResolvedValueOnce([])
  .mockResolvedValueOnce(BASE_COURSES)
  .mockResolvedValueOnce(BASE_POLICIES);
```

2. **URL pattern matching** — `mockImplementation` with `if/throw`:
```tsx
mockApiJson.mockImplementation((path: string) => {
  if (path === "/api/v1/courses/course-1") return Promise.resolve({ id: "course-1", ... });
  if (path.startsWith("/api/v1/sessions?")) return Promise.resolve([]);
  throw new Error(`Unexpected API call: ${path}`);
});
```

3. **Route map pattern** (via shared helper):
```tsx
// src/pages/__tests__/helpers/index.tsx
mockApiByPattern(mockApiJson, {
  "absence-form-config": MOCK_CONFIG,
  "student-lookup": MOCK_STUDENT,
  "sessions-in-range": MOCK_SESSIONS,
});
```

**Render providers:**
```tsx
function renderPage(path = "/absences?status=pending") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <ToastProvider>
        <Absences />
      </ToastProvider>
    </MemoryRouter>,
  );
}
```

**Test structure:**
```tsx
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

describe("ComponentName", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("describes behavior", async () => {
    mockApiJson.mockResolvedValueOnce(DATA);
    renderWithProviders(<Component />);

    await waitFor(() => {
      expect(screen.getByText("Expected Text")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", ...));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/expected-path",
        expect.objectContaining({ method: "PUT", body: JSON.stringify(...) }),
      );
    });
  });
});
```

### Pattern 2: Hook Tests (renderHook)

Located in `src/test/`.

```tsx
// backend/internal/httpapi/absenceshttp/  (No again)
const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<...>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("usePreflight", () => {
  it("check() sets loading", async () => {
    mockApiJson.mockImplementation(() => new Promise(() => {}));
    const { result } = renderHook(() => usePreflight());

    act(() => {
      result.current.check({ course_id: "c1", teacher_id: "t1", ... });
    });

    expect(result.current.loading).toBe(true);
  });
});
```

### Pattern 3: Util Tests (plain TS, no rendering)

Located in `src/test/` and `src/utils/__tests__/`.

```ts
import { describe, it, expect } from "vitest";
import { parseSchedulePaste } from "../schedulePaste";

describe("parseSchedulePaste", () => {
  it("handles single time entry", () => {
    const result = parseSchedulePaste("09:00");
    expect(result).toEqual({ start: "09:00", end: null });
  });
});
```

---

## Mocking

### Backend (Go)

**No mocking framework** — Dependency injection via interfaces or concrete type embedding.

- **Auth** is mocked via `fakeAuth` struct in every `httpapi` package (duplicated pattern)
- **Database** is real PostgreSQL — no mock DB layer
- **External services** (e.g., SmartSMS) — interface-based; tests use real HTTP or skip
- **No `gomock`/`mockgen`/`testify/mock`** anywhere in the codebase

**`fakeAuth` pattern (duplicated across packages):**
```go
// Found in: sessionshttp/routes_test.go, courselevelshttp/routes_test.go,
//           courseshttp/routes_integration_test.go, schedulinghttp/routes_preflight_test.go,
//           corehttp/routes_test.go, adminusershttp/adminusershttp_test.go, etc.
type fakeAuth struct {
    user auth.User
    err  error
}
func (f fakeAuth) RequireUser(_ context.Context, _ *http.Request) (auth.User, error) {
    return f.user, f.err
}
func (fakeAuth) HandleLogin(_ http.ResponseWriter, _ *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(_ http.ResponseWriter, _ *http.Request) error { return nil }
```

### Frontend (TypeScript)

**API mocking:** `vi.mock("@/api/client")` — mocks the `apiJson` function at the module level.

**What's NOT used:** MSW, nock, fetch-mock — none found. MSW is in `package-lock.json` (transitive dependency of `@testing-library/user-event` or similar), but not used directly by any test file.

**Additional mocks:**
- `localStorage`: manually mocked via `Object.defineProperty(globalThis, "localStorage", ...)`
- `useAuth`: mocked via `vi.mock("../../hooks/useAuth", () => ({ useAuth: () => {...} }))`

---

## Fixtures and Factories

### Go Backend

**No fixture library** — test data is constructed inline:

```go
suffix := time.Now().UTC().Format("20060102150405.000000000")
teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-policy-" + suffix, ...})
```

### Frontend (TypeScript)

**Test data helpers** live in `src/pages/__tests__/helpers/index.tsx`:
```tsx
export function createMockSessionsInRange(subjects?: SubjectSessions[]): SessionsInRangeResponse {
  return {
    subjects: subjects ?? [/* default shape */],
  };
}

export function createMockSitInResult(method: "zoom" | "physical" | "pending" = "zoom") {
  // Returns different shapes based on method parameter
}
```

**Inline fixtures** are the dominant pattern — each test file defines its own:
```tsx
const BASE_COURSES = [{ id: "c1", code: "MATH-101", ... }, ...];
const PAGE = { items: [...], total_count: 1, ... };
```

---

## Coverage

**No coverage target configured.** No `--coverage` flag in `package.json` scripts, no `threshold` in `vite.config.ts`. Coverage can be generated manually:
```bash
npx vitest --coverage
```

---

## Spec Patterns

### Table-Driven Tests (Go)

Used in pure unit tests:
```go
func TestNormalizePhoneE164_AllCases(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"bare digits", "0812345678", "+66812345678", false},
        {"empty", "", "", true},
        // ...
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) { ... })
    }
}
```

### Subtests (Go)

All integration tests use subtests for grouping related assertions:
```go
t.Run("DefaultOpenAvailability", func(t *testing.T) { ... })
t.Run("TeacherAvailabilityWindowBlocks", func(t *testing.T) { ... })
t.Run("RoomAvailabilityWindowBlocks", func(t *testing.T) { ... })
```

---

## Missing Pieces & Observations

- **No shared test helper package** — `requireTestDB`, `migrateUpOnce`, `newPool`, and `fakeAuth` are duplicated across packages. This means changes to migration setup (e.g., adding cleanup for new tables) must be made in every package independently.
- **No `backend/internal/db/db_test.go`** — there's no top-of-package file declaring the shared helpers for the `db` package. They're defined in `invariants_integration_test.go` and relied upon by other `db/*_test.go` files via file ordering in the same package.
- **Go test dependencies are minimal** — `go.mod` has no test-only dependencies (no `require (// test)` block). No testify, no mockgen.
- **No MSW on frontend** — all API mocking is at the `apiJson` function level, not the network level.
- **No E2E tests** found in either frontend or backend.
- **No snapshot testing** found.
- **Frontend tests are primarily integration-style** — they render full components with router + toast providers, mocking the API client layer.

---

*Testing analysis: 2026-06-06*
