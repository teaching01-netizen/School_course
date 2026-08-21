# Code review: repaired pagination diff

## Verdict

- codeQualityStatus: BLOCK
- recommendation: REQUEST_CHANGES
- Result: SMASH

## Scope and evidence inspected

- `backend/db/migrations/00100_admin_pagination_indexes.sql`
- `backend/internal/db/active_courses_custom.go`
- `backend/internal/httpapi/activecourseshttp/routes.go`
- `backend/internal/httpapi/courselevelshttp/routes.go`
- `src/pages/operations/ActiveCoursesSection.tsx`
- `src/pages/CourseLevels.tsx`
- focused backend/frontend pagination tests and the working-tree diff

There is no ULW plan: `omo ulw-loop status --json` returned `ULW_LOOP_PLAN_MISSING`, so this report uses the fallback evidence path.

## Findings

### CRITICAL

None.

### HIGH

1. Active-course pagination silently drops selectable courses. `backend/internal/db/active_courses_custom.go:119-125` pages subjects but limits each selected subject to its first 200 courses. The response has no truncation signal or per-subject total, while `src/pages/operations/ActiveCoursesSection.tsx:67-76` always requests this paginated form. A subject with more than 200 courses cannot have a later course selected in this UI; if its current active course is after the first 200, the UI renders it as unconfigured and can overwrite it. This is a user-visible data-selection regression, not just a memory bound.

### MEDIUM

1. There is still no focused behavior-level coverage for either new paginated endpoint. `backend/internal/httpapi/activecourseshttp/routes_test.go:8-37` and `backend/internal/httpapi/courselevelshttp/routes_test.go:55-84` only exercise the duplicated `parsePagination` helper. Repository search found no test of `ActiveCoursesListPaginated` and no frontend test that drives the new Previous/Next controls. The existing `ActiveCoursesSection` tests use the legacy response shape. This leaves the response envelope, totals, stable tie ordering, page navigation, and the course-cap regression unprotected.

### LOW

None.

## Re-review of prior findings

- Goose markers: repaired. `00100_admin_pagination_indexes.sql:1,6` has explicit `-- +goose Up` and `-- +goose Down`; migration validation passes.
- Unbounded active-course materialization: bounded, but with the high-severity silent-truncation regression above. A valid fix must retain all selectable courses or explicitly change the API/UI contract to page courses and expose that state.
- Course index: repaired sufficiently for the current query shape. The new `subjects(code, id)` index supports the subject ordering and the two courses indexes support per-subject ordering; no remaining index blocker found from source review.

## Skill-perspective check

Ran: yes. Consulted `omo:programming` (including the Go/TypeScript guidance) and `omo:remove-ai-slops` before judging test relevance and maintainability.

- `remove-ai-slops`: no deletion-only tests, prompt tests, tautologies, or implementation-constant tests were added. The parser-only tests are not useless, but they provide inadequate coverage for the new endpoint behavior.
- `programming`: no untyped escape hatch, needless abstraction, or misplaced boundary parsing was introduced. The per-subject `LIMIT 200` is needless production behavior because the requested subject pagination did not authorize silently changing course availability.

## Commands run

- `make -C backend migrate-validate`: PASS (35 pre-existing migration linter warnings, none from migration 00100).
- `go test -count=1 ./internal/httpapi/activecourseshttp ./internal/httpapi/courselevelshttp ./internal/db` from `backend/`: PASS.
- `npm test -- --run src/pages/__tests__/ActiveCoursesSection.test.tsx src/pages/__tests__/CourseLevels.test.tsx`: PASS (20 tests).
- `npm run typecheck`: PASS.
- `git diff --check` for the scoped pagination files: PASS.

## Required blockers before approval

1. Remove the silent 200-course truncation. Either return every course for a paged subject, or introduce an explicit, user-navigable course-pagination contract that preserves selection of the existing active course and exposes totals/truncation.
2. Add focused behavior tests for both paginated endpoint responses and frontend page navigation, including a subject with more than 200 courses so this regression cannot return.
