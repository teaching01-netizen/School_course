---
phase: code-review
reviewed: 2026-05-31T10:00:00Z
depth: standard
files_reviewed: 4
files_reviewed_list:
  - src/pages/AbsenceForm.tsx
  - backend/internal/httpapi/absenceshttp/resolver.go
  - backend/internal/httpapi/absenceshttp/routes.go
  - src/types/index.ts
findings:
  critical: 0
  warning: 3
  info: 2
  total: 5
status: issues_found
---

# Phase: Code Review Report

**Reviewed:** 2026-05-31T10:00:00Z
**Depth:** standard
**Files Reviewed:** 4
**Status:** issues_found

## Summary

Reviewed 4 files implementing absence form v2 — subject-level session grouping, sit-in selection per session, `subject_name` on `SitInCourseInfo`, and client-side `missingSitIn` validation. Three WARNING-level issues found: one data-completeness bug in `buildPhysicalSitInResult` (constructor omits `SubjectName`), one fragile date-extraction pattern using string slicing in `handleSessionsInRange`, and one type-vs-runtime disconnect in `handleSessionToggle`. The `missingSitIn` validation and frontend label changes are correct.

## Warnings

### WR-01: `buildPhysicalSitInResult` omits `SubjectName` on `SitInCourseInfo`

**File:** `backend/internal/httpapi/absenceshttp/resolver.go:95-99`
**Issue:** The newly added `SubjectName` field on `SitInCourseInfo` is never populated in the constructor function `buildPhysicalSitInResult`. It defaults to `""` in the JSON response. Only one consumer (`handleSessionsInRange` at routes.go:852) patches it after the fact via `sitIn.SitInCourse.SubjectName = g.SubjectName`. The other consumer (`handleSitInOptions` at routes.go:613-624) returns the `resolveSitIn` result directly without patching, so the sit-in options endpoint always emits `"subject_name": ""`.

**Risk:** Any code path consuming `SitInCourseInfo` from `resolveSitIn` without the post-hoc patch gets an empty string. While no frontend code currently calls `handleSitInOptions` directly, the endpoint contract is broken — an API consumer expecting `subject_name` receives `""`.

**Fix:** Pass the subject name as an explicit parameter to `buildPhysicalSitInResult` and set it at construction time:

```go
// resolver.go
func buildPhysicalSitInResult(
    target *sqldb.SubjectCourseV2,
    missed []sqldb.SessionInRange,
    available []sqldb.SessionInRange,
    subjectName string, // new parameter
) *SitInResult {
    // ...
    result := &SitInResult{
        SitInMethod: SitInMethodPhysical,
        SitInCourse: &SitInCourseInfo{
            ID:          targetIDStr,
            Code:        target.Code,
            Name:        target.Name,
            SubjectName: subjectName,  // set here, not patched later
        },
        // ...
    }
}
```

Then update the call site at line 228 to pass the subject name (or empty string if unavailable), and remove the post-hoc patch at routes.go:852.

---

### WR-02: Fragile `[:10]` string slicing for date extraction

**File:** `backend/internal/httpapi/absenceshttp/routes.go:833`
**Issue:** The `Date` field in the session response is derived by slicing the first 10 characters of `StartAt`:
```go
Date: sess.StartAt[:10],
```
`sess.StartAt` is a `string` scanned from a `timestamptz` column via pgx. The text representation depends on the pgx timestamptz codec and the PostgreSQL connection's timezone context. While the de facto output format includes the date as a prefix (`2026-05-31T...` or `2026-05-31 ...`), this is an implicit encoding assumption. Any change in the driver's text format for `timestamptz` → `string` conversion would silently produce wrong dates.

**Risk:** Non-obvious failure mode. A pgx version upgrade or configuration change that alters timestamp text encoding would silently shift date values without any type error.

**Fix:** Scan `sess.start_at` into `time.Time` and format explicitly, or add an explicit cast in SQL:

Option A (SQL CAST):
```sql
SELECT sess.id, sess.start_at::text, sess.end_at::text, ...
```
and then parse with `time.Parse` for the Date field.

Option B (Go scan + format):
```go
type sessionRow struct {
    ID          string
    StartAt     time.Time
    EndAt       time.Time
    CourseID    string
    // ...
}
// after scan:
Date: row.StartAt.Format("2006-01-02"),
```

---

### WR-03: `handleSessionToggle` silently drops sessions beyond `maxSessions`

**File:** `src/pages/AbsenceForm.tsx:586-588`
**Issue:** When the selected session count already equals `maxSessions`, calling `handleSessionToggle` returns the current set unchanged (line 587: `return current`). However, the UI checkbox for unselected sessions is disabled via `disabled={!selected && atMaxSessions}` (line 1158). This is visually correct for disabled checkboxes, but there is a mismatch: `handleSessionToggle` does not distinguish between a *disabled click* (which should be a no-op) and a *programmatic call* (which silently fails).

**Risk:** If `toggleAllSessionsForGroup` is called with `forceValue=true` when `atMaxSessions` is already true (e.g., if the API limit changes after initial render, or if a future code path invokes `handleSessionToggle` programmatically without checking the disabled state), the new session is silently dropped with no feedback to the user. The user sees the checkbox appear to toggle for an instant (since the checked state in `selected` never changed), but the toggle doesn't register.

**Fix:** Make the guard explicit and surface the limit condition:

```typescript
if (current.size >= maxSessions && !current.has(sessionId)) {
    setPageError(`You can only select up to ${maxSessions} sessions.`);
    return current;
}
```

This replaces the silent ignore with a visible user-facing message.

---

## Info

### IN-01: Subject grouping discards per-session `course_id`

**File:** `backend/internal/httpapi/absenceshttp/routes.go:781-796`
**Issue:** When sessions from multiple courses within the same subject are grouped, `subjectGroup.CourseID` is set from the first session only:
```go
if grouped[key] == nil {
    grouped[key] = &subjectGroup{
        CourseID: sess.CourseID,  // only from first session
        // ...
    }
}
```
Sessions from other courses under the same subject still appear in `Sessions[]` but the group-level `course_id` (which the frontend uses in `buildSubmissionPayloads` at AbsenceForm.tsx:733) may not match all sessions.

**Why this is INFO, not WARNING:** `handleAbsenceCreate` (routes.go) does not use `course_id` from the request body — it resolves via `subject_id` through `resolveAbsenceSelection`. So this is not a data corruption path. It creates confusing semantics in the API response: consumers see a `course_id` that may not correspond to every session in the group.

**Fix:** Either (a) remove `course_id` from the subject-level response if it's not authoritative, or (b) use the first course's ID but document that sessions from sibling courses within the same subject may be included.

---

### IN-02: `handleSitInOptions` returns `"subject_name": ""` for physical sit-ins

**File:** `backend/internal/httpapi/absenceshttp/routes.go:613-624`
**Issue:** Same root cause as WR-01, scoped to the `/api/v1/absences/sit-in-options` endpoint. This endpoint returns `resolveSitIn`'s result directly, so `SitInCourseInfo.SubjectName` is always empty for physical sit-in results. No frontend code currently consumes this endpoint, but the API contract is incomplete.

**Fix:** Resolve WR-01 at the source (set `SubjectName` in `buildPhysicalSitInResult`), which fixes both endpoints.

---

_Reviewed: 2026-05-31T10:00:00Z_
_Reviewer: gsd-code-reviewer_
_Depth: standard_
