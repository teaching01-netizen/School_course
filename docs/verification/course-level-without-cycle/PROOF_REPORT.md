# Course levels without a cycle

## Specification

An administrator can add or edit a valid level for a standalone course without a cycle through the Course Levels workspace. The operation saves the level and preserves the cycle supplied by the course row, including a null cycle. The normal course group, sit-in rule, level range, and assigned-cycle duplicate checks still apply. A group can show Ready when its levels and sit-in rule are configured and there are no level gaps, even when a cycle is absent.

## Assumptions

The level editor uses the current course row returned by the API. The model covers a single save with that input and assumes the request succeeds where it specifies a saved state. The existing API writes level and cycle in one PostgreSQL update. Concurrent cycle editing can make the input stale, and this change does not add concurrency control to the existing API. Production rollout and production load are untested.

## Proof

The [Lean model](proof/CourseLevel.lean) distinguishes list eligibility, validation, the saved course state, and group readiness. It includes positive save and add outcomes for the no-cycle case and rejection cases for invalid levels, a blocked group, and duplicate levels in an assigned cycle. Both `lake build` and `lake env lean proof/CourseLevel.lean` checked all seven theorems. The axiom audit printed only `propext` for the valid-range and duplicate theorems. The other four have no axiom dependencies.

## Traceability

| Requirement | Correspondence | Evidence |
| --- | --- | --- |
| Add and edit without a cycle | `CourseLevelManagerPanel` row policy, Add level selector, and `saveLevel` | The two component tests failed on the old guard and pass with the fix. The Playwright flow submits `cycle_id: null` and reads the saved level after reload using mocked HTTP. |
| Persist the level with a null cycle | `courselevelshttp.handleUpdateLevel` and `CourseLevelUpdateV2` | PostgreSQL `TestCourseCycleLevel/SetLevelWithoutCycle` reads the actual row. A temporary HTTP probe registered the real handlers against the migrated database and verified add, edit, retry, and list readback. |
| Readiness permits optional cycles | `CourseLevelManagerPanel.groupStatusFor` and `configuredGroupReadyRegardlessOfCycles` | Standalone and continuation component checks plus Chromium desktop and phone readiness checks. Existing gap and rule guards remain covered. |
| Preserve validation | `saveLevel`, HTTP validation, and `idx_courses_subj_cycle_level` | Existing group-rule, duplicate, invalid-input, and PostgreSQL uniqueness tests. |

## Checker results

- Base `55a605719b7b043271df4a4b821220e61cdfe55d`. Lean `4.19.0` at `6caaee842e94`, Node `26.0.0`, Playwright `1.61.1`, local PostgreSQL `14.23`, and schema through migration `00133`.
- Frontend `npm test -- src/pages/__tests__/CourseLevels src/utils/__tests__/levels.test.ts` passed all 50 tests. `npm run typecheck` and `npm run typecheck:e2e` passed.
- Browser `playwright test e2e/course-levels-inline.spec.ts --project=chromium-desktop --project=chromium-phone-standard` passed all 12 cases.
- Backend `go test -p 1 ./internal/db ./internal/httpapi/courselevelshttp -run 'TestCourseCycleLevel|TestRegister_PutLevel' -count=1` with a disposable `TEST_DATABASE_URL`. The HTTP probe used the same database through a temporary Go overlay.
- Formal status **PROVED (scoped)**. Implementation correspondence **REVIEWED_WITH_TESTS**. Production evidence **NOT_TESTED**.

## Production check

After deployment, use an authorized test course in a rule-configured group. Assign its first level through Add level, edit it inline, reload, and confirm the level persists with No cycle. Check that another course with an assigned cycle still receives the existing duplicate-level rejection when applicable.

## Remediation

Remove the editor-only cycle gate and send a null cycle through the existing API for course rows without one. Treat missing cycles as informational in Course Levels. The existing database schema already supports this state.
