---
phase: 02-code-review
reviewed: 2026-05-31T00:00:00Z
depth: deep
files_reviewed: 4
files_reviewed_list:
  - backend/internal/httpapi/absenceshttp/resolver.go
  - backend/internal/httpapi/absenceshttp/routes.go
  - src/types/index.ts
  - src/pages/AbsenceForm.tsx
findings:
  critical: 3
  warning: 5
  info: 3
  total: 11
status: issues_found
---

# Phase 02: Code Review Report — Sit-in Rule Integration

**Reviewed:** 2026-05-31T00:00:00Z
**Depth:** deep (cross-file call-chain analysis)
**Files Reviewed:** 4
**Status:** issues_found

## Summary

Phase 2 extends the sessions-in-range endpoint with per-subject sit-in resolution from backend rules. The backend `resolveSitIn()` correctly evaluates rules and returns structured results. The frontend consumes `group.sit_in` from the API response.

**Three blocker bugs found:** (1) sit-in course ID mismatch in submission payload causes physical sit-in requests to be rejected by the backend, (2) teacher_case method silently defaults to zoom, (3) resolveSitIn errors silently swallowed with no logging. Additionally, several dead code paths and unused fields suggest incomplete cleanup.

---

## Critical Issues

### CR-01: sit_in_course_id hardcoded to own course — physical sit-in submissions always fail

**File:** `src/pages/AbsenceForm.tsx:769`
**Issue:** `buildSubmissionPayloads` sets `sit_in_course_id: group.course_id` (the student's own enrolled course). For physical sit-in, the available sessions belong to the *target* course (e.g., a higher-level section). The backend `ValidSitInSessionCount` (`absence_management_custom.go:320`) runs:

```sql
SELECT count(*) FROM sessions sess
JOIN student_absences sa ON sa.id = $1
WHERE sess.id = ANY($3::uuid[])
  AND sess.course_id = $2          -- $2 = sit_in_course_id from body
  AND sess.deleted_at IS NULL
  ...
```

When `$2` is the student's own `course_id` but the session IDs are from the target course, the query returns count=0. The mismatch triggers `count != len(sessionUUIDs)` → `http.StatusBadRequest` → `"Sit-in sessions must be in the selected course and absence dates"`. **Every physical sit-in submission will fail at the DB validation gate.**

**Fix:** Use the target course from the sit-in response when available:

```typescript
// Line 769 — replace hardcoded group.course_id
const sitInCourseId = sitInMethod === "physical"
  ? (group.sit_in?.sit_in_course?.id ?? group.course_id)
  : undefined;

payloads.push({
  // ...other fields...
  sit_in_method: sitInMethod,
  sit_in_course_id: sitInCourseId,
  sit_in_session_ids: sitInSessionIds,
  // ...rest...
});
```

### CR-02: Teacher_case sit-in silently treated as "zoom" on submission

**Files:**
- `backend/internal/httpapi/absenceshttp/resolver.go:201-204`
- `src/pages/AbsenceForm.tsx:759`

**Issue:** `resolveSitIn()` returns `nil, nil` for `SitInMethodTeacher` (line 204 of resolver.go). In `handleSessionsInRange` (routes.go:838-839), nil result means the `sit_in` field is omitted from the response. On the frontend, `buildSubmissionPayloads` (line 759) falls back to `"zoom"`:

```typescript
const sitInMethod = group.sit_in?.sit_in_method ?? "zoom";
```

This submits a zoom sit-in method when the rule actually requires teacher_case approval. The backend validates the method but accepts "zoom" as valid, so the incorrect value persists in the database.

**Two root causes:**
1. Backend returns nil for teacher_case — should return the method info so the frontend knows the method
2. Frontend default to "zoom" is wrong when sit_in is absent due to teacher_case

**Fix (backend, resolver.go:201-204):** Return a result with the method set instead of nil:

```go
case SitInMethodTeacher:
    result = &SitInResult{SitInMethod: SitInMethodTeacher}
```

**Fix (frontend, AbsenceForm.tsx:759):** Remove default — the backend should always provide the method. If sit_in is missing, the submission should reflect that:

```typescript
const sitInMethod = group.sit_in?.sit_in_method;
// If sit_in is missing entirely, don't submit a sit_in_method at all
// (or keep current default but only after backend fix above)
```

### CR-03: resolveSitIn errors silently swallowed with no logging

**File:** `backend/internal/httpapi/absenceshttp/routes.go:838-839`

**Issue:** In `handleSessionsInRange`, the `resolveErr` from `resolveSitIn()` is checked (`resolveErr == nil`) but on error the code silently skips the sit-in for that subject. No error is logged and no indication reaches the frontend. This masks:
- DB connection failures
- Student/course lookup errors  
- Rule predicate parse errors
- Rule evaluation errors

**Fix:** Log the error before skipping:

```go
result, resolveErr := resolveSitIn(r.Context(), s.deps.Q, wcode, subjectID, dateFrom, dateTo)
if resolveErr != nil {
    s.deps.Log.Error("sit-in resolution failed for subject", "subject_id", g.SubjectID, "error", resolveErr)
}
if resolveErr == nil && result != nil && result.SitInMethod != SitInMethodNone {
    // ...existing mapping...
}
```

---

## Warnings

### WR-01: `atMaxSessions` variable defined but never used (dead code)

**File:** `src/pages/AbsenceForm.tsx:341`

**Issue:** Computed as `selectedSessionCount >= maxSessions` but never referenced in JSX or any handler — not used for disabling UI, styling, or aria attributes.

**Fix:** Remove the unused variable:

```typescript
// Delete line 341
```

If intended for future use, keep but suppress lint; otherwise delete.

### WR-02: `automaticSitInEnabled` function defined but never called

**File:** `backend/internal/httpapi/absenceshttp/resolver.go:237-261`

**Issue:** Function is fully implemented (parses absence policies, checks `AutoResolveEnabled`, checks `RootCourseGroups` override) but is never referenced anywhere — not in `resolveSitIn`, not in `handleSessionsInRange`, not in any handler. Dead code.

**Fix:** Either integrate into `resolveSitIn` (check before evaluating rules) or remove.

### WR-03: `"teacher_case"` and `"none"` sit_in_method values unreachable from backend response

**Files:**
- `src/types/index.ts:325`
- `src/pages/AbsenceForm.tsx:1283-1327`

**Issue:** The `SitInInfo` type union includes `"teacher_case"` | `"none"`, but the backend never returns these values:
- `"teacher_case"`: returns nil (CR-02)
- `"none"`: `SitInMethodNone` is checked at routes.go:839 and skipped unless set, but no code path ever sets it on a non-nil `SitInResult`

The rendering code for `"teacher_case"` (lines 1321-1324) and the implied `"none"` path (null render) are dead code that will never execute with the current backend.

**Fix:** Either (a) remove unreachable branches and simplify the type, or (b) backfill the backend to return these values as specified in the type contract. Option (b) is preferred for correctness — at minimum, teacher_case should be returned so the frontend knows the method.

### WR-04: `missed_sessions` populated by backend but never consumed by frontend

**Files:**
- `backend/internal/httpapi/absenceshttp/resolver.go:103-104`
- `src/types/index.ts:328`
- `src/pages/AbsenceForm.tsx` (no usage found)

**Issue:** `resolveSitIn` populates `MissedSession` (the student's own-course sessions in range) and the backend serializes it in the response as `missed_sessions`. The type definition includes it. But `AbsenceForm.tsx` never reads `group.sit_in?.missed_sessions`. The data flows through the full API round-trip without being used.

**Fix:** Either (a) consume it in the UI (show which sessions will be missed), or (b) omit from the response if the frontend doesn't need it (`json:"-"` or remove).

### WR-05: Default sit_in_method = "zoom" when no rule configured may submit incorrect method

**File:** `src/pages/AbsenceForm.tsx:759`

**Issue:** When `group.sit_in` is undefined (no rule configured, student not enrolled, evaluation error), the frontend defaults to method `"zoom"`. This submits absences with `sit_in_method: "zoom"` even when no zoom arrangement exists, creating misleading data in the database.

Only mitigated by: the sit-in panel is not shown (line 1283: `{selected && sitIn ? (...)}`), so the user doesn't see zoom-related UI. But the submitted data records a zoom method that was never confirmed.

**Fix:** Don't send `sit_in_method` when no sit-in data is provided; let the backend use its default. Alternatively, require explicit user confirmation for the fallback method.

---

## Info

### IN-01: `uuidString` duplicate of `sUUIDString` in same package

**File:** `backend/internal/httpapi/absenceshttp/resolver.go:279-283`

**Issue:** Two functions in the same package format `pgtype.UUID` to string differently: `uuidString` uses manual `fmt.Sprintf("%x-%x-%x-%x-%x", ...)` while `sUUIDString` (routes.go:136-145) uses `uuid.FromBytes().String()`. Both produce the same output but violate DRY.

Not a bug (both produce correct UUID format 8-4-4-4-12), but the inconsistency should be resolved by consolidating into one.

### IN-02: `SitInMethodNone` check at routes.go:839 is dead logic

**File:** `backend/internal/httpapi/absenceshttp/routes.go:839`

**Issue:** The condition `result.SitInMethod != SitInMethodNone` is always true when `result != nil`. No code path in `resolveSitIn` returns a non-nil result with `SitInMethodNone`:
- `SitInMethodZoom` → non-nil
- `SitInMethodTeacher` → nil (returned before)
- `SitInMethodPhysical` → non-nil
- default → nil

The check is harmless but misleading — suggests "none" is a possible value when it isn't.

### IN-03: `rule_name` / `rule_type` serialized but never displayed in AbsenceForm

**File:** `src/types/index.ts:323-324`

**Issue:** Fields pass through the API response type but `AbsenceForm.tsx` doesn't display them. They're available for other consumers (e.g., `AbsenceDetail.tsx:253` shows `sit_in_rule_name` from a different source). Consider adding rule info display in the sit-in panel for explainability.

---

## Cross-File Analysis Summary

| Path | Findings |
|------|----------|
| `backend/internal/httpapi/absenceshttp/resolver.go` | CR-02 (teacher_case nil), WR-02 (dead code `automaticSitInEnabled`), IN-01 (duplicate uuid func), IN-02 (dead SitInMethodNone) |
| `backend/internal/httpapi/absenceshttp/routes.go` | CR-03 (error swallowed), IN-02 (dead check) |
| `src/types/index.ts` | WR-03 (unreachable union members), WR-04 (missed_sessions unused), IN-03 (rule_name/rule_type unused in form) |
| `src/pages/AbsenceForm.tsx` | CR-01 (wrong sit_in_course_id), CR-02 (zoom default), WR-01 (atMaxSessions), WR-05 (zoom fallback) |

---

_Reviewed: 2026-05-31T00:00:00Z_
_Reviewer: gsd-code-reviewer (deep)_
_Depth: deep_
