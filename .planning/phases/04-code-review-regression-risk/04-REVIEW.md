---
phase: 04-code-review-regression-risk
reviewed: 2026-06-02T00:00:00Z
depth: deep
files_reviewed: 11
files_reviewed_list:
  - backend/internal/httpapi/absenceshttp/management_routes.go
  - backend/internal/httpapi/absenceshttp/dispatch.go
  - backend/internal/httpapi/absenceshttp/routes.go
  - backend/internal/httpapi/absenceshttp/routes_test.go
  - backend/internal/httpapi/httpadapter/adapter.go
  - backend/internal/db/absence_management_custom.go
  - backend/internal/idempotency/idempotency.go
  - src/pages/Absences.tsx
  - src/components/absences/KanbanView.tsx
  - src/types/index.ts
  - src/pages/__tests__/Absences.test.tsx
  - src/components/absences/__tests__/KanbanView.test.tsx
findings:
  critical: 0
  warning: 2
  info: 1
  total: 3
status: issues_found
---

# Phase 4: Regression Risk Review Report

**Reviewed:** 2026-06-02
**Depth:** deep
**Files Reviewed:** 11
**Status:** issues_found

## Summary

Regression risk review of the optimistic-concurrency (expected_version) changes applied to the absence hard-delete flow. Traced the full call chain from frontend fetch through HTTP adapter, idempotency fingerprinting, and DB delete. Verified call sites, types, tests, and idempotency semantics. Found no blockers, but two warnings and one informational finding.

## Regression Risk Checklist

| # | Risk | Verdict |
|---|------|---------|
| 1 | Handler still returns `(int, any, error)` from WithIdempotentTx | ✅ PASS — Line 1361: callback signature `func(tx pgx.Tx) (int, any, error)` unchanged. Return `http.StatusOK, map[string]string{"status":"deleted"}, nil` at line 1400. |
| 2 | AuditInsert still happens after successful delete | ✅ PASS — Lines 1391-1399: `AuditInsert` runs after `AbsenceHardDelete`, same as before. |
| 3 | Cancelled-absence check still BEFORE version check | ⚠️ **NO — order is reversed** (see WR-01) |
| 4 | Frontend tests still pass with new body format | ✅ PASS — Tests use `expect.objectContaining({ method: "DELETE" })` and mock-based assertions that don't inspect body content. No assertion breakage. |
| 5 | TypeScript typecheck passes | ✅ PASS — `StudentAbsence` has `version: number` (types/index.ts:130); `ManagedAbsence` inherits it. `deleteTarget.version` is `number`. Go handler accepts `*int32` from JSON. |
| 6 | Other DELETE /absences/:id call sites updated | ✅ PASS — Only two callers: `Absences.tsx:186` and `KanbanView.tsx:191`. Both updated. `AbsenceDetail.tsx` has no DELETE call. No other frontend/backend callers. |
| 7 | Idempotency key computation when body changes | ✅ PASS (see IN-01 for detail) — `NewRequestFingerprint` hashes method+path+body. Body change from empty to `{"expected_version":N}` produces a different fingerprint, which is correct: same idempotency key + different body = different request = fresh execution. No risk of false replay from old empty-body records. |

## Warnings

### WR-01: Version check precedes cancelled-absence check — wrong error message for cancelled absences

**File:** `backend/internal/httpapi/absenceshttp/management_routes.go:1369-1376`
**Issue:** The version check (line 1369) runs BEFORE the cancelled-status check (line 1373). If a caller attempts to delete an already-cancelled absence with the correct `expected_version`, they receive a `409 stale_edit` error instead of the more accurate `409 already_cancelled` error. Before this change, the cancelled check was the first guard (no version check existed), so callers got the correct "already cancelled" message.

The UI hides the delete button for cancelled absences (confirmed by `Absences.test.tsx:316-325` and `KanbanView.test.tsx:105-121`), so this is not exploitable through normal UI flow. However, API callers (scripts, integrations, or future features like AbsenceDetail delete) would get the wrong error.

**Fix:** Move the cancelled check above the version check:

```go
// Line 1363-1376 — reorder to:
current, err := qtx.ManagedAbsenceGet(r.Context(), id)
if err != nil {
    status, code, message := s.a.ClassifyDBErr(err)
    s.a.WriteErr(w, status, code, message)
    return 0, nil, err
}
// Check status FIRST — gives a more accurate error before version mismatch
if current.Status == "cancelled" {
    s.a.WriteErr(w, http.StatusConflict, "already_cancelled", "This absence is already cancelled")
    return 0, nil, fmt.Errorf("already cancelled")
}
// THEN check version
if current.Version != *body.ExpectedVersion {
    s.writeStaleAbsence(w)
    return 0, nil, pgx.ErrNoRows
}
```

### WR-02: Frontend tests do not assert the `expected_version` body field

**File:** `src/pages/__tests__/Absences.test.tsx:354-357` and `src/components/absences/__tests__/KanbanView.test.tsx:95-98`
**Issue:** Both test suites assert `expect.objectContaining({ method: "DELETE" })` which verifies the HTTP method but does not validate that the body contains `{ expected_version: <number> }`. If a future refactor accidentally removes the body (e.g., reverting to `method: "DELETE"` with no body), the tests would still pass but the backend would reject the request with `400 bad_expected_version`.

**Fix:** Strengthen the assertion to verify body content:

```tsx
// Absences.test.tsx line 354
expect(mockApiJson).toHaveBeenCalledWith(
  "/api/v1/absences/abs-1",
  expect.objectContaining({
    method: "DELETE",
    body: JSON.stringify({ expected_version: 1 }),
  }),
);

// KanbanView.test.tsx line 95
expect(mockApiJson).toHaveBeenCalledWith(
  "/api/v1/absences/abs-1",
  expect.objectContaining({
    method: "DELETE",
    body: JSON.stringify({ expected_version: 1 }),
  }),
);
```

## Info

### IN-01: Idempotency fingerprint changes with body — safe but worth documenting

**File:** `backend/internal/idempotency/idempotency.go:105-118`
**Issue:** `NewRequestFingerprint` hashes `method:path:body`. The old DELETE sent an empty body (fingerprint = `DELETE:/api/v1/absences/{id}:`); the new DELETE sends `{"expected_version":N}` (fingerprint = `DELETE:/api/v1/absences/{id}:{"expected_version":N}`). This is correct and safe:

- Old cached idempotency records (empty body) won't match new requests (non-empty body) → new request executes normally, no false replay.
- Same key + same body (retry) → fingerprint matches → cached response returned. Correct.
- Same key + different body (version changed) → fingerprint differs → treated as new request → version check runs. Correct.

**No fix needed.** This is informational only. If a future change makes `expected_version` optional, the body shape will change again and the fingerprint analysis should be repeated.

---

_Reviewed: 2026-06-02_
_Reviewer: the agent (gsd-code-reviewer, regression risk)_
_Depth: deep_
