# Independent runtime verification rerun manual QA

Verdict: **FAILED**

Migration setup is now verified: Goose successfully migrated the test database to version 103. Runtime verification still fails in three legacy materialization assertions.

## manualQa

### surfaceEvidence

| scenario id | criterion reference | surface | exact invocation | verdict | artifactRefs |
|---|---|---|---|---|---|
| RERUN-001 | migration setup and legacy materialization | Go `./internal/legacysync/apply` integration tests against `.env` `TEST_DATABASE_URL` | `go test -count=1 -timeout=180s ./internal/legacysync/apply -run '^(TestScheduleApply_(ReusesStableLegacyScheduleIdentity|IdenticalAggregateIsNoOp|OverlappingScheduleSkippedAndRecorded|PartialApplyRetriesAfterConflictResolution|ShadowModeDoesNotStampOkSnapshot)|TestCourseApply_CodeCollisionRecordsSyncConflict)$'` | FAILED: setup passes, but overlap, partial-retry, and code-collision assertions fail | R1 |
| RERUN-002 | Courses API overlap/conflict and legacy response path | Go `./internal/httpapi/courseshttp` integration tests | `go test -count=1 -timeout=180s ./internal/httpapi/courseshttp -run '^(TestCoursesList_|Test(GetCourse_ReturnsLegacyFields|ListCourses_IncludesLegacyFlag|UpdateLegacyLink_))'` | PASS: package passed in 60.970s | R1 |
| RERUN-003 | legacy conflict endpoint | Go `./internal/httpapi/legacysynchttp` integration tests | `go test -count=1 -timeout=180s ./internal/httpapi/legacysynchttp -run '^TestConflict(ResolveTransitions|IgnoreTransitions|SetStatusErrors)$'` | PASS: package passed in 0.666s | R1 |
| RERUN-004 | migration validation | repository validator | `npm run migrate:validate` | PASS: 103 files, contiguous sequence, 35 warnings | R1 |

### adversarialCases

| scenario id | criterion reference | adversarial class | expected behavior | verdict | artifactRefs |
|---|---|---|---|---|---|
| RERUN-ADV-001 | overlap conflict recording | overlapping room/time legacy rows | conflict is recorded without aborting the transaction; clear rows still materialize | FAILED: SQLSTATE 25P02 while recording the overlap conflict | R1 |
| RERUN-ADV-002 | partial materialization retry | partial apply after a local blocker | partial conflict is recorded and later retry remains possible | FAILED: SQLSTATE 25P02 while recording the partial conflict | R1 |
| RERUN-ADV-003 | legacy code conflict | duplicate course code | conflict remains open for review | FAILED: observed status `ignored`, expected `open` | R1 |

### artifactRefs

| id | kind | description | path |
|---|---|---|---|
| R1 | test log | Exact one-run migration-backed backend and migration-validator output | `/Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/.omo/evidence/independent-runtime-verification-rerun/backend-rerun.log` |

## Remaining blocker

The migration parsing defect is resolved. The remaining failures are in legacy conflict transaction/status behavior: overlap and partial conflict recording runs after the transaction is already aborted, and code-collision conflict status is observed as `ignored` instead of `open`.
