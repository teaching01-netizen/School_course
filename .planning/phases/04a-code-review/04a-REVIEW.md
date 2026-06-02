---
phase: 04a-code-review
reviewed: 2026-06-02T00:00:00Z
depth: deep
files_reviewed: 4
files_reviewed_list:
  - backend/internal/db/absence_management_custom.go
  - backend/internal/httpapi/absenceshttp/management_routes.go
  - backend/internal/httpapi/absenceshttp/routes_test.go
  - src/pages/Absences.tsx
  - src/components/absences/KanbanView.tsx
findings:
  critical: 1
  warning: 2
  info: 0
  total: 3
status: issues_found
---

# Phase 4A: Code Review Report — Requirement Coverage

**Reviewed:** 2026-06-02
**Depth:** deep
**Files Reviewed:** 5
**Status:** issues_found

## Summary

All 7 requirements (R1–R7) have **implementation code** that satisfies them. The handler, db method, and frontend clients are correct in isolation. However, there are **zero behavioral tests** for any of the 7 requirements. The only test file (`routes_test.go`) covers route registration only — not version validation, stale-edit response, cancelled blocking, audit logging, or the happy path. This is a critical coverage gap: every requirement can regress silently.

## Requirement-by-Requirement Coverage Matrix

| Req | Description | Code Present? | Test Present? | Verdict |
|-----|-------------|:---:|:---:|---------|
| R1 | DELETE requires `expected_version` in body | ✅ (management_routes.go:1350-1358) | ❌ | FAIL |
| R2 | Returns 409 `stale_edit` on version mismatch | ✅ (management_routes.go:1369-1372) | ❌ | FAIL |
| R3 | Checks rows deleted, returns 409 if 0 | ✅ (management_routes.go:1381-1388, RETURNING/Scan) | ❌ | FAIL |
| R4 | Uses sqlc query method, not inline SQL | ✅ (handler calls `qtx.AbsenceHardDelete`) | N/A (structural) | PASS |
| R5 | Works correctly with valid version + existing absence | ✅ (full happy path at lines 1360-1401) | ❌ | FAIL |
| R6 | Blocks cancelled absences with 409 `already_cancelled` | ✅ (management_routes.go:1373-1376) | ❌ | FAIL |
| R7 | Writes global audit log after successful delete | ✅ (management_routes.go:1391-1398) | ❌ | FAIL |

## Critical Issues

### CR-01: Zero behavioral tests for DELETE handler — all 7 requirements untested

**File:** `backend/internal/httpapi/absenceshttp/routes_test.go`
**Issue:** The test file contains only 3 route-registration tests (`TestDispatchDelete_RouteRegistered`, `TestDispatchDelete_WrongMethod`, `TestDispatchDelete_NotFoundOnSubpath`). These verify the route is wired but assert nothing about behavior. Specifically missing:

- **R1 test:** No test sends DELETE without `expected_version` and asserts 400 `bad_expected_version`.
- **R2 test:** No test sends a mismatched `expected_version` and asserts 409 `stale_edit`.
- **R3 test:** No test exercises the race-condition path where the DB-level version check fails (`AbsenceHardDelete` returns `NoRows`).
- **R5 test:** No test sends a valid version for an existing non-cancelled absence and asserts 200 `{"status":"deleted"}`.
- **R6 test:** No test sends a delete for a cancelled absence and asserts 409 `already_cancelled`.
- **R7 test:** No test verifies an audit log row is written with action `absence.hard_deleted`.

The handler interacts with the database via `qtx.ManagedAbsenceGet`, `qtx.AbsenceHardDelete`, and `qtx.AuditInsert` — all behind `WithIdempotentTx`. Tests should mock or use a test DB to exercise each branch.

**Fix:** Add at minimum these test cases (unit or integration):
```go
func TestHandleAbsenceDelete_MissingExpectedVersion(t *testing.T)
func TestHandleAbsenceDelete_VersionMismatch(t *testing.T)
func TestHandleAbsenceDelete_AlreadyCancelled(t *testing.T)
func TestHandleAbsenceDelete_HappyPath(t *testing.T)
func TestHandleAbsenceDelete_RaceCondition_DBVersionChanged(t *testing.T)
```
Each should assert the HTTP status code, response body shape, and (for happy path) that an audit log entry was created.

## Warnings

### WR-01: `AbsenceHardDelete` uses `RETURNING`/`Scan` pattern but R3 specifies "checks RowsAffected"

**File:** `backend/internal/db/absence_management_custom.go:385-393`
**Issue:** The requirement says "DELETE handler checks RowsAffected and returns 409 if 0 rows deleted." The implementation uses `DELETE ... RETURNING 1` + `Scan`, which returns `pgx.ErrNoRows` when zero rows match. The handler checks `sqldb.IsNoRows(err)` at line 1382 and returns 409 `stale_edit`. Functionally equivalent, but the implementation pattern diverges from the requirement's stated approach (`RowsAffected()`). This is acceptable if the team's convention is `RETURNING`-based detection (which avoids a separate query), but worth documenting since a future developer expecting `RowsAffected` may be confused.

**Fix:** No code change needed if team convention supports `RETURNING`. If strict requirement adherence is needed, change to `Exec` + `RowsAffected()`:
```go
func (q *Queries) AbsenceHardDelete(ctx context.Context, id pgtype.UUID, expectedVersion int32) (int64, error) {
    ct, err := q.db.Exec(ctx, `
        DELETE FROM student_absences
        WHERE id = $1 AND version = $2
    `, id, expectedVersion)
    if err != nil {
        return 0, err
    }
    return ct.RowsAffected(), nil
}
```
Then handler checks `rowsAffected == 0` → 409.

### WR-02: `testAbsenceDelete_RouteRegistered` sends nil body — will panic on real server

**File:** `backend/internal/httpapi/absenceshttp/routes_test.go:11-23`
**Issue:** `TestDispatchDelete_RouteRegistered` creates a `server{}` with zero-value fields and sends a DELETE request with `nil` body. The test passes because the zero-value `server` likely panics or returns a non-404 status before reaching body parsing. This is a fragile test — it doesn't actually prove the handler logic works, only that the dispatch table routes correctly. If the handler is ever wired to a real server, the nil body would cause a panic or 400 at `DecodeJSON`.

**Fix:** At minimum, send a valid body in this test, or remove it in favor of the behavioral tests recommended in CR-01.

## Info

No info-level findings.

---

_Reviewed: 2026-06-02_
_Reviewer: Agent 4A (gsd-code-reviewer, requirement coverage)_
_Depth: deep_
