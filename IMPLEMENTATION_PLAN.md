# Detailed Implementation Plan: Legacy Course Sync Roadmap v3

## Overview

This plan details the implementation of the roadmap v3 for the warwick-institute repository. The mission is: "Every legacy course gets a linked local row in our DB; schedule/field data apply best-effort; every deviation from full fidelity is recorded as an open legacy_sync_conflicts row (or dead letter) and surfaced as a warning on the course detail page."

The plan covers R1-R8, with exact intended behavior, file changes, state/data flow, invariants, edge cases, test-first sequence, verification commands, and rollback considerations.

---

## R1 — Reconcile: Ingest duplicate-code legacy courses with suffix

### Exact Intended Behavior
- When two legacy courses share the same `course.Code`, the first to be linked claims the local course with that code.
- The second legacy course cannot claim the same local course; instead, it creates a new local course with a suffixed code (e.g., `MATH101` becomes `MATH101-2`).
- The conflict is recorded in `legacy_sync_conflicts` with `conflict_type='code_claimed'` and `category='mapping_conflict'`.

### Relevant Files and Changes
- **`internal/legacysync/reconcile/full.go`**:
  - Modify `linkCourse` (lines 274-362) to handle suffix logic when a code collision is detected.
  - Add a helper `generateSuffixedCode` to create unique codes.
  - Update `FullReconcileStats` to include a `Suffixed` field (currently missing).

### State/Data Flow
1. `linkCourse` is called for each legacy course.
2. It attempts to claim a local course by code (lines 307-336).
3. If the code is already claimed (`claimedBy != nil`), instead of recording a conflict and skipping, it should:
   - Generate a suffixed code (e.g., `code + "-2"`).
   - Create a new local course with the suffixed code and link it.
   - Record a `code_claimed` conflict with details about the original code and suffix.
   - Increment `stats.Suffixed`.

### Invariants Maintained
- Each legacy course gets a linked local row.
- Code uniqueness is preserved (no two courses have the same code).
- Conflicts are recorded for audit.

### Edge Cases and Failure Behavior
- If the suffixed code also collides, continue appending suffixes until unique (with a limit, e.g., 10 attempts).
- If the limit is reached, record a conflict and skip (current behavior).
- Transaction rollback on any error.

### Test-First Sequence
1. Write a test that creates two legacy courses with the same code and verifies that both get linked (one with original code, one with suffixed code).
2. Confirm the test fails (current code records conflict and skips).
3. Implement suffix logic.
4. Confirm the test passes.
5. Refactor for clarity.

### Verification Commands
```bash
go test ./internal/legacysync/reconcile/ -run TestLinkCourseWithDuplicateCode -v
```

### Rollback Considerations
- The change is additive; if rolled back, duplicate-code courses will again be recorded as conflicts and not linked.

---

## R2 — Refresh apply: code collision keeps local code (apply/course.go, SAVEPOINT + code advisory lock)

### Exact Intended Behavior
- During `CourseApplier.Apply`, if a code collision occurs (unique violation on `courses_code_key`), the local course's existing code is retained.
- The legacy course's code is NOT updated to avoid overwriting the local code.
- The conflict is recorded, but the course remains linked and synced.

### Relevant Files and Changes
- **`internal/legacysync/apply/course.go`**:
  - Modify `updateCourse` (lines 272-288) to skip updating the `code` field when a code collision is detected.
  - Add a SAVEPOINT before the `updateCourse` call to allow rollback on collision without aborting the entire transaction.
  - Use an advisory lock on the code to serialize updates.

### State/Data Flow
1. `Apply` begins a transaction and takes an advisory lock on the legacy course ID.
2. It also takes an advisory lock on the course code (if present).
3. Before calling `updateCourse`, it creates a SAVEPOINT.
4. `updateCourse` attempts to update the course, including the code.
5. If a unique violation occurs (code collision), rollback to the SAVEPOINT.
6. Record the conflict using `recordCourseCodeConflict`.
7. Continue with the rest of the apply (schedules, etc.) using the existing code.

### Invariants Maintained
- The local course's code is never overwritten by a legacy code that collides.
- The course remains linked and synced.
- Conflicts are recorded for admin review.

### Edge Cases and Failure Behavior
- If the code collision is on a different constraint (not `courses_code_key`), treat as a normal error.
- If the SAVEPOINT rollback fails, abort the transaction.

### Test-First Sequence
1. Write a test that applies a legacy course with a code that collides with an existing local course's code.
2. Verify that the local code is retained and a conflict is recorded.
3. Confirm the test fails (current code aborts on collision).
4. Implement SAVEPOINT and code-skipping logic.
5. Confirm the test passes.

### Verification Commands
```bash
go test ./internal/legacysync/apply/ -run TestCourseApplyCodeCollision -v
```

### Rollback Considerations
- The change modifies transaction behavior; if rolled back, code collisions will again abort the apply.

---

## R3 — Missing teacher/subject reference ingests with NULL + warning (apply/course.go + schedule.go unconditional override)

### Exact Intended Behavior
- If a teacher or subject reference is missing (not found in `external_refs`), the course/schedule is still synced with `NULL` for the missing reference.
- A warning is recorded in `legacy_sync_conflicts` (or a similar mechanism) to alert admins.

### Relevant Files and Changes
- **`internal/legacysync/apply/course.go`**:
  - Modify `resolveReference` (lines 257-270) to return a zero UUID and a warning instead of an error when the reference is missing.
  - Record the missing reference in `legacy_sync_conflicts` with `conflict_type='missing_reference'`.
- **`internal/legacysync/apply/schedule.go`**:
  - Ensure that the teacher override at line 137 (`if currentTeacherID.Valid { request.TeacherID = currentTeacherID }`) is applied unconditionally, even if the incoming teacher reference is missing.

### State/Data Flow
1. `resolveReference` is called for teacher and subject.
2. If the reference is not found, instead of returning `ErrMissingReference`, it returns a zero UUID and logs a warning.
3. The warning is recorded in `legacy_sync_conflicts` (outside the transaction).
4. The course/schedule is updated with `NULL` for the missing reference.

### Invariants Maintained
- Every legacy course gets a linked local row, even with missing references.
- Missing references are surfaced as warnings.

### Edge Cases and Failure Behavior
- If recording the conflict fails, the apply still proceeds (best-effort).
- If the missing reference is critical (e.g., teacher), the course is still synced but flagged.

### Test-First Sequence
1. Write a test that applies a legacy course with a missing teacher reference.
2. Verify that the course is synced with `NULL` teacher and a warning is recorded.
3. Confirm the test fails (current code returns error).
4. Implement the change.
5. Confirm the test passes.

### Verification Commands
```bash
go test ./internal/legacysync/apply/ -run TestCourseApplyMissingTeacherReference -v
```

### Rollback Considerations
- If rolled back, missing references will again cause apply to fail.

---

## R4 — API GET /api/v1/courses/{id}/legacy-conflicts (courseshttp/routes.go + legacy_audit_custom.go)

### Exact Intended Behavior
- A new API endpoint returns all open legacy sync conflicts for a given course.
- The endpoint is `GET /api/v1/courses/{id}/legacy-conflicts`.
- The response includes conflict details (type, category, message, etc.).

### Relevant Files and Changes
- **`internal/legacysync/courseshttp/routes.go`**:
  - Register a new route for `GET /api/v1/courses/{id}/legacy-conflicts`.
  - Implement a handler `handleCourseLegacyConflicts` that queries `legacy_sync_conflicts` for the given course.
- **`internal/db/legacy_audit_custom.go`**:
  - Add a new query method `LegacyCourseConflicts` that returns conflicts for a course.

### State/Data Flow
1. The frontend calls `GET /api/v1/courses/{id}/legacy-conflicts`.
2. The handler validates the course ID.
3. It queries `legacy_sync_conflicts` where `external_id` matches the legacy course ID (or the course's `legacy_course_id`).
4. Returns the list of open conflicts.

### Invariants Maintained
- The API provides visibility into conflicts for a specific course.
- Only open conflicts are returned (or optionally all, with a query parameter).

### Edge Cases and Failure Behavior
- If the course does not exist, return 404.
- If there are no conflicts, return an empty list.

### Test-First Sequence
1. Write an integration test that creates a course with conflicts and verifies the API returns them.
2. Confirm the test fails (endpoint does not exist).
3. Implement the endpoint.
4. Confirm the test passes.

### Verification Commands
```bash
go test ./internal/legacysync/courseshttp/ -run TestCourseLegacyConflicts -v
```

### Rollback Considerations
- If rolled back, the endpoint is removed; frontend must handle 404.

---

## R5 — Frontend amber banner on CourseDetail (types, api, component, test)

### Exact Intended Behavior
- On the `CourseDetail` page, if there are open legacy sync conflicts, display an amber banner with a summary of the conflicts.
- The banner should be dismissible but reappear on page reload.
- Clicking the banner should expand to show conflict details.

### Relevant Files and Changes
- **`src/features/courses/types.ts`**:
  - Add a `legacyConflicts` field to the `Course` type (or a separate type for conflicts).
- **`src/features/courses/api/courseApi.ts`**:
  - Add a function `getCourseLegacyConflicts` that calls the new API endpoint.
- **`src/pages/CourseDetail.tsx`**:
  - Fetch conflicts on component mount (alongside course data).
  - Render an amber banner if conflicts exist.
  - Implement expand/collapse functionality.
- **`src/pages/__tests__/CourseDetail.legacySync.test.tsx`**:
  - Add tests for the banner behavior.

### State/Data Flow
1. `CourseDetail` fetches course data and conflicts.
2. If conflicts exist, render a banner with amber styling.
3. The banner shows a summary (e.g., "2 open conflicts").
4. Clicking expands to show conflict details (type, message).

### Invariants Maintained
- Users are warned about legacy sync issues.
- The banner is non-intrusive but visible.

### Edge Cases and Failure Behavior
- If the conflict fetch fails, do not show the banner (or show an error).
- If there are no conflicts, no banner.

### Test-First Sequence
1. Write a test that mocks the API to return conflicts and verifies the banner is displayed.
2. Confirm the test fails (no banner logic).
3. Implement the banner.
4. Confirm the test passes.

### Verification Commands
```bash
npm test -- --testPathPattern=CourseDetail.legacySync
```

### Rollback Considerations
- If rolled back, the banner is removed; conflicts are only visible in the admin health view.

---

## R6 — Docs + audit fix (legacy_audit_custom.go allowlist discriminator)

### Exact Intended Behavior
- Update the audit queries in `legacy_audit_custom.go` to use an allowlist discriminator for conflict types.
- Ensure that only relevant conflict types are counted in skip totals.

### Relevant Files and Changes
- **`internal/db/legacy_audit_custom.go`**:
  - Update `LegacyAuditSkipCounts` (lines 106-125) to filter by allowed conflict types.
  - Update `LegacyAuditSkippedCourses` (lines 266-312) similarly.

### State/Data Flow
1. The audit queries now filter by a list of allowed conflict types (e.g., `code_claimed`, `missing_reference`, `course_code_conflict`).
2. This ensures that only relevant conflicts are counted.

### Invariants Maintained
- Audit counts are accurate and exclude irrelevant conflict types.

### Edge Cases and Failure Behavior
- If the allowlist is empty, no conflicts are counted (safe default).

### Test-First Sequence
1. Write a test that verifies the audit counts only include allowed conflict types.
2. Confirm the test fails (current code counts all).
3. Implement the allowlist filter.
4. Confirm the test passes.

### Verification Commands
```bash
go test ./internal/db/ -run TestLegacyAuditSkipCounts -v
```

### Rollback Considerations
- If rolled back, audit counts may include irrelevant conflict types.

---

## R7 — Auto-resolution in apply tx (resolve healed conflicts)

### Exact Intended Behavior
- During `CourseApplier.Apply`, if a conflict is resolved (e.g., a missing reference is now present), automatically resolve the conflict in the transaction.

### Relevant Files and Changes
- **`internal/legacysync/apply/course.go`**:
  - After successfully resolving a reference, call `resolveConflict` to mark any open `missing_reference` conflict as resolved.
- **`internal/legacysync/apply/schedule.go`**:
  - Similar logic for schedule conflicts (already partially implemented in `resolveScheduleConflict`).

### State/Data Flow
1. When a reference is successfully resolved, check for open conflicts of type `missing_reference` for this entity.
2. Update the conflict status to `resolved`.
3. This happens within the same transaction.

### Invariants Maintained
- Conflicts are automatically resolved when the underlying issue is fixed.
- The conflict ledger remains accurate.

### Edge Cases and Failure Behavior
- If resolving the conflict fails, log a warning but do not fail the apply.

### Test-First Sequence
1. Write a test that creates a conflict, then applies a course with a valid reference, and verifies the conflict is resolved.
2. Confirm the test fails (current code does not resolve conflicts).
3. Implement auto-resolution.
4. Confirm the test passes.

### Verification Commands
```bash
go test ./internal/legacysync/apply/ -run TestAutoResolveConflict -v
```

### Rollback Considerations
- If rolled back, conflicts remain open even when healed.

---

## R8 — Fast-path integrity check (verify code + references in unchanged-hash path)

### Exact Intended Behavior
- In the unchanged-hash fast path of `CourseApplier.Apply`, verify that the course code and references are still valid.
- If they are not, fall through to the full apply path.

### Relevant Files and Changes
- **`internal/legacysync/apply/course.go`**:
  - In the fast path (lines 109-119), add checks:
    - Verify the course code still exists in `courses` and matches.
    - Verify teacher and subject references are still valid in `external_refs`.
  - If any check fails, skip the fast path and proceed with full apply.

### State/Data Flow
1. In the fast path, after confirming the source hash is unchanged, run integrity checks.
2. If checks pass, commit the fast path.
3. If checks fail, proceed with the full apply (which will update references, etc.).

### Invariants Maintained
- The fast path only applies when the data is truly unchanged.
- Drift is detected and corrected.

### Edge Cases and Failure Behavior
- If the integrity check fails, the full apply runs (which may update the course).
- If the full apply also fails, return an error.

### Test-First Sequence
1. Write a test that sets up a course with an unchanged hash but invalid references, and verifies the fast path is skipped.
2. Confirm the test fails (current code does not check).
3. Implement the integrity checks.
4. Confirm the test passes.

### Verification Commands
```bash
go test ./internal/legacysync/apply/ -run TestFastPathIntegrityCheck -v
```

### Rollback Considerations
- If rolled back, the fast path may apply stale data.

---

## Summary of Changes by File

### Backend (Go)
- **`internal/legacysync/reconcile/full.go`**: R1 (suffix logic), R7 (auto-resolution).
- **`internal/legacysync/apply/course.go`**: R2 (SAVEPOINT, code skip), R3 (missing references), R7 (auto-resolution), R8 (integrity check).
- **`internal/legacysync/apply/schedule.go`**: R3 (unconditional override), R7 (auto-resolution).
- **`internal/legacysync/courseshttp/routes.go`**: R4 (new endpoint).
- **`internal/db/legacy_audit_custom.go`**: R4 (new query), R6 (allowlist discriminator).

### Frontend (TypeScript/React)
- **`src/features/courses/types.ts`**: R5 (add conflict type).
- **`src/features/courses/api/courseApi.ts`**: R5 (add API function).
- **`src/pages/CourseDetail.tsx`**: R5 (amber banner).
- **`src/pages/__tests__/CourseDetail.legacySync.test.tsx`**: R5 (add tests).

---

## Test Strategy

Each change follows test-first development:
1. Write a failing test that captures the desired behavior.
2. Implement the minimal code to make the test pass.
3. Refactor for clarity and maintainability.
4. Run the full test suite to ensure no regressions.

Integration tests should cover:
- Duplicate-code courses (R1).
- Code collision handling (R2).
- Missing references (R3).
- API endpoint (R4).
- Frontend banner (R5).
- Audit accuracy (R6).
- Auto-resolution (R7).
- Fast-path integrity (R8).

---

## Verification Commands

Run the full test suite:
```bash
go test ./...
npm test
```

Run specific tests for each feature:
```bash
go test ./internal/legacysync/reconcile/ -run TestLinkCourseWithDuplicateCode -v
go test ./internal/legacysync/apply/ -run TestCourseApplyCodeCollision -v
go test ./internal/legacysync/apply/ -run TestCourseApplyMissingTeacherReference -v
go test ./internal/legacysync/courseshttp/ -run TestCourseLegacyConflicts -v
npm test -- --testPathPattern=CourseDetail.legacySync
go test ./internal/db/ -run TestLegacyAuditSkipCounts -v
go test ./internal/legacysync/apply/ -run TestAutoResolveConflict -v
go test ./internal/legacysync/apply/ -run TestFastPathIntegrityCheck -v
```

---

## Rollback Considerations

All changes are additive and backward-compatible. If any change needs to be rolled back:
- Revert the specific commit.
- Ensure that the frontend gracefully handles missing API endpoints (404).
- The system will revert to previous behavior (conflicts recorded, fast path without integrity checks, etc.).

---

## Acceptance Criteria

- Every legacy course gets a linked local row.
- Schedule/field data apply best-effort.
- Every deviation is recorded as an open `legacy_sync_conflicts` row.
- Deviations are surfaced as warnings on the course detail page.
- All tests pass.
- No regressions in existing functionality.
