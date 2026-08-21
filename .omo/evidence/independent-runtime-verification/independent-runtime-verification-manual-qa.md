# Independent runtime verification manual QA

Verdict: **FAILED**

The frontend and build surfaces pass, but the requested backend overlap/conflict and legacy materialization runtime paths cannot reach their assertions because migration `00103_legacy_conflict_materialization.sql` is rejected by PostgreSQL during test setup. No product files were edited.

## manualQa

### surfaceEvidence

| scenario id | criterion reference | surface | exact invocation | verdict | artifactRefs |
|---|---|---|---|---|---|
| BE-001 | legacy schedule overlap/conflict materialization | Go integration tests against configured `TEST_DATABASE_URL` | `cd backend && set -a; source ../.env; set +a; go test -count=1 -timeout=180s ./internal/legacysync/apply -run '^(TestScheduleApply_(OverlappingScheduleSkippedAndRecorded|PartialApplyRetriesAfterConflictResolution|ReusesStableLegacyScheduleIdentity|ShadowModeDoesNotStampOkSnapshot)|TestCourseApply_CodeCollisionRecordsSyncConflict)$'` | FAILED: migration setup aborts with unterminated dollar-quoted string before assertions | A1 |
| BE-002 | Courses API overlap/conflict response fields and legacy fields | Go Courses HTTP integration tests | `cd backend && set -a; source ../.env; set +a; go test -count=1 -timeout=180s ./internal/httpapi/courseshttp -run '^(TestCoursesList_|Test(GetCourse_ReturnsLegacyFields|ListCourses_IncludesLegacyFlag|UpdateLegacyLink_))'` | FAILED: same `00103` migration setup error | A1 |
| BE-003 | legacy conflict endpoint behavior | Go HTTP integration tests | `cd backend && set -a; source ../.env; set +a; go test -count=1 -timeout=180s ./internal/httpapi/legacysynchttp -run '^TestConflict(ResolveTransitions|IgnoreTransitions|SetStatusErrors)$'` | PASS | A1 |
| MIG-001 | migration validation | repository migration validator | `npm run migrate:validate` | PASS: 103 files, contiguous sequence, convention checks pass; 35 warnings | A2 |
| FE-001 | Courses overlap/conflict rendering and Courses regressions | Vitest/jsdom | `npm exec vitest run src/pages/__tests__/Courses.filters.test.tsx src/features/courses/components/CourseInfoStrip.test.tsx` | PASS: 2 files, 21 tests | A3 |
| FE-002 | production frontend artifact | Vite build | `npm run build` | PASS: 2,747 modules transformed | A3 |
| UI-001 | built Courses page shows both new fields | Chromium browser against Vite preview, read-only API mocks | `npm run preview -- --host 127.0.0.1 --port 4173`; Playwright navigates `/courses` and mocks only GET routes | PASS: one visible Conflict badge and one visible Overlap badge; screenshot captured | A4, A5 |

### adversarialCases

| scenario id | criterion reference | adversarial class | expected behavior | verdict | artifactRefs |
|---|---|---|---|---|---|
| ADV-001 | legacy schedule conflicts | overlapping room/time rows | one conflicting row is skipped and recorded as open `room_overlap`; other rows still materialize | FAILED: migration prevents the test from reaching apply logic | A1 |
| ADV-002 | legacy materialization retry | unchanged source hash after local blocker removal | partial snapshot remains retryable; refresh materializes the previously skipped row and reaches quality `ok` | FAILED: migration prevents setup | A1 |
| ADV-003 | legacy course conflict | duplicate legacy course code | local code is retained, a `database_constraint/course_code_conflict` row is open, and snapshot quality is partial | FAILED: migration prevents setup | A1 |
| ADV-004 | frontend field states | true/false overlap and conflict flags | red detected badges and green clear badges render with accessible labels | PASS in Courses test and built browser; both true state observed | A3, A4, A5 |
| ADV-005 | migration strictness | strict business-data lint | historical unallowlisted INSERTs are rejected rather than silently accepted | PASS: strict validator returned 35 errors; this is a repository-wide pre-existing lint condition, not a 00103-specific finding | A2 |

### artifactRefs

| id | kind | description | path |
|---|---|---|---|
| A1 | test log | Backend targeted integration, endpoint, unit, and static test results | `/Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/.omo/evidence/independent-runtime-verification/backend-targeted.log` |
| A2 | validation log | Normal and strict migration validator results plus direct 00103 diagnosis | `/Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/.omo/evidence/independent-runtime-verification/migration-validation.log` |
| A3 | test/build log | Frontend Courses tests and production build results | `/Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/.omo/evidence/independent-runtime-verification/frontend-courses.log` |
| A4 | screenshot | Built Courses page initial viewport | `/Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/.omo/evidence/independent-runtime-verification/courses-built-browser.png` |
| A5 | browser action log | Chromium invocation, mocked GET routes, observations, and screenshot list | `/Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/.omo/evidence/independent-runtime-verification/browser-actions.json` |

## Blocker

Fixing the Goose parsing issue in `00103_legacy_conflict_materialization.sql` and rerunning BE-001/BE-002/ADV-001/ADV-002/ADV-003 is required before this change can be VERIFIED. This QA run did not make that product change.
