# Code review: legacy conflict materialization (current worktree)

## Verdict

- **codeQualityStatus:** BLOCK
- **recommendation:** REQUEST_CHANGES
- **Scope:** current code only: migration `00103`, course/schedule apply, full reconciliation, course overview/DTO, and the Courses UI/test.
- **ULW attempt:** unavailable. `omo ulw-loop status --json` reports `ULW_LOOP_PLAN_MISSING`; report therefore uses the required fallback path.

## Findings

### CRITICAL

None.

### HIGH

1. **The database-enabled regression suite still asserts the superseded open-conflict lifecycle, so it will fail when its PostgreSQL lane runs.**
   - Production deliberately records a materialized code collision as `ignored` in `backend/internal/legacysync/reconcile/full.go:737-743`, and the refresh-course path records then closes the same class of collision in `backend/internal/legacysync/apply/course.go:203-219,324-332`. That matches the stated requirement that expected migration conflicts do not remain open or dead-lettered.
   - But the current integration tests require `status = 'open'` at `backend/internal/legacysync/reconcile/full_integration_test.go:162-169,520-524,539-543`, `backend/internal/legacysync/reconcile/full_parallel_integration_test.go:162-170,262-278`, and `backend/internal/legacysync/apply/course_integration_test.go:729-749`. The course-apply test also expects `quality = 'partial'` even though a fully materialized allowed-conflict apply writes `ok` at `backend/internal/legacysync/apply/course.go:269-275`.
   - Evidence: the focused tests pass only because they skip without `TEST_DATABASE_URL`; `go test -v` reported those skips. In a configured PostgreSQL CI lane, the assertions are directly inconsistent with current production behavior and the requirement.
   - Fix the tests to assert materialization, `ignored` (or the explicitly chosen non-open normal status), no dead letter, and the visible course conflict indicator. Do not restore an open status merely to satisfy these stale tests.

### MEDIUM

1. **The down migration is not a true reversal and retains the new session column.** `backend/db/migrations/00103_legacy_conflict_materialization.sql:33-40` restores the exclusion constraints but only resets the default for `sessions.legacy_conflict_override`; it never drops the column. A rollback therefore leaves schema introduced by 00103 in place. The down block should also restore the pre-00103 `enforce_session_availability()` definition before removing the column, if the column is removed.

### LOW

None.

## Requirement check

- Code collisions: the reconciler creates a suffixed, linked course and stores an ignored conflict (`backend/internal/legacysync/reconcile/full.go:673-747`). The refresh path retains the local code and records an ignored conflict (`backend/internal/legacysync/apply/course.go:188-219,324-332`).
- Overlaps: the configured syncer sets `AllowConstraintViolations: true` (`backend/cmd/legacy-sync/syncer.go:298-316`); the schedule applier retries with `legacy_conflict_override=true` after an overlap/availability constraint rejection (`backend/internal/legacysync/apply/schedule.go:280-320`).
- Conflict visibility: the course overview computes overlap and conflict booleans (`backend/internal/db/courses_overview_custom.go:187-197`), sends them in the list DTO (`backend/internal/httpapi/courseshttp/routes.go:195-282`), and `/courses` renders red/green badges (`src/pages/Courses.tsx:400-403`). The focused UI test exercises those rendered labels (`src/pages/__tests__/Courses.filters.test.tsx:151-158`).

## Skill-perspective check

- **Ran:** yes. Consulted `omo:programming` (Go and TypeScript references) and `omo:remove-ai-slops` before judging maintainability/test relevance.
- **Programming:** no untyped TypeScript escape hatch, brittle prompt test, needless abstraction, or unnecessary production parsing/normalization was found in the reviewed paths.
- **Remove-ai-slops:** the Courses badge test is not deletion-only, tautological, prompt-based, or constant-mirroring. The database tests above are nevertheless obsolete implementation/lifecycle assertions; they provide false confidence and will fail in the intended PostgreSQL environment. This is the HIGH finding, not an excuse to remove coverage.

## Validation evidence

- `go test -count=1 ./internal/legacysync/apply ./internal/legacysync/reconcile ./internal/db ./internal/httpapi/courseshttp` passed.
- `npm test -- --run src/pages/__tests__/Courses.filters.test.tsx` passed: 12 tests.
- Focused `go test -v` showed the materialization integration tests were skipped because `TEST_DATABASE_URL` is not set; no end-to-end database validation ran in this review.
- `git diff --check` passed.

## Blockers

1. Update the stale PostgreSQL integration assertions to the required non-open lifecycle and run them with `TEST_DATABASE_URL` configured.
