# Codebase Concerns

**Analysis Date:** 2026-05-31

## Tech Debt

**Sit-in name resolution — fragile cross-entity lookup chain:**
- Issue: `resolveSitInSubjectName()` in `src/pages/AbsenceForm.tsx:112-114` resolves the sit-in subject name by cross-referencing `sit_in_course.id` against the `sessions` array's `course_id` field. This is an indirect, fragile mapping — if the sit-in course ID does not appear in the current `sessions` array (e.g., the sit-in course is not among the student's enrolled courses, or the array is stale), the function returns `undefined`, and the UI falls back to `group.subject_name` — which is the **absent** course's subject name, not the sit-in course.
- Files: `src/pages/AbsenceForm.tsx:112-114`, `src/pages/AbsenceForm.tsx:1181`
- Impact: The "Absence class:" label can display the wrong course name (the absent course instead of the sit-in course), or show no name at all when the lookup chain fails.
- Fix approach: The API should return a `sit_in_subject_name` field directly on `SubjectSessions.sit_in`, or `resolveSitInSubjectName` should be backed by a dedicated course→subject lookup map rather than a linear scan of the sessions array.

**Sit-in display fallback chains are inconsistent across views:**
- Issue: Three different views display sit-in course identity using three different fallback chains:
  - `AbsenceForm.tsx:1181`: `resolveSitInSubjectName(sit_in_course, sessions) || group.subject_name`
  - `AbsenceDetail.tsx:54`: `sit_in_subject_name ?? subject_name ?? subject_code ?? sit_in_course_name ?? sit_in_course_code ?? "Not assigned"`
  - `Absences.tsx:444`: `sit_in_course_code ?? "Physical"` (no name at all, only code)
- Files: `src/pages/AbsenceForm.tsx:1181`, `src/pages/AbsenceDetail.tsx:50-55`, `src/pages/Absences.tsx:441-445`
- Impact: The same absence can display differently in the form, detail, and list views. A sit-in that shows as "Math advanced" in the detail view may show as "0000000344" (a raw internal code) in the list view.
- Fix approach: Standardize a single helper function (e.g., `displaySitInLabel(absence: ManagedAbsence)`) that all views use, and ensure the API response includes all necessary name fields.

**`SitInResultCard` is orphaned — not imported by any app component:**
- Issue: `src/components/absences/SitInResultCard.tsx` defines a sit-in result display component with its own type system (`SitInResult`, `SitInResultCardProps`), but it is never imported by any production component. Only `src/components/absences/__tests__/SitInResultCard.test.tsx` imports it.
- Files: `src/components/absences/SitInResultCard.tsx`, `src/components/absences/__tests__/SitInResultCard.test.tsx`
- Impact: Dead code. The component defines its own parallel type `SitInResult` that duplicates `SitInInfo` from `src/types/index.ts:319-335`, creating a maintenance burden and potential drift.
- Fix approach: Either integrate `SitInResultCard` into the absence flow (e.g., use it in `AbsenceDetail.tsx` or `AbsenceForm.tsx`) or delete it. If integrating, reconcile the `SitInResult` type with `SitInInfo` from `src/types/index.ts`.

**`getSitInSessionLabel()` fallback chain includes raw codes as last resort:**
- Issue: `getSitInSessionLabel()` in `src/pages/AbsenceForm.tsx:116-135` tries 8 different fields in order, ultimately falling back to `sitInCourse?.code?.trim()` which can be an internal numeric code like `"0000000348"`. This raw code appears in a `<select>` option visible to students/parents.
- Files: `src/pages/AbsenceForm.tsx:116-135`
- Impact: Users see meaningless internal codes like "0000000348" in the make-up class dropdown when all name resolution fails.
- Fix approach: The API should always provide a human-readable name on available sessions. The fallback chain should never surface raw internal codes to end users.

## Known Bugs

**"Absence class:" label can show the absent course name instead of the sit-in course name:**
- Symptoms: When a student's sit-in course ID does not match any `course_id` in the `sessions` array, `resolveSitInSubjectName()` returns `undefined`, and the label falls back to `group.subject_name` — the name of the class the student is **missing**, not the one they should attend instead.
- Files: `src/pages/AbsenceForm.tsx:1181`
- Trigger: Sit-in course is external to the student's enrolled subjects (e.g., cross-level sit-in where the sit-in course is not in the student's subject list).
- Workaround: None — the wrong name is displayed.

## Security Considerations

**No sit-in display concern — internal codes are non-sensitive:**
- Risk: Low. Internal course codes (e.g., "0000000348") are not secrets but are confusing to end users.
- Files: N/A
- Current mitigation: Fallback chains attempt to use names first.
- Recommendations: Ensure raw UUIDs or internal codes are never displayed in parent-facing or student-facing UI.

## Performance Bottlenecks

**Linear scan for sit-in subject name resolution:**
- Problem: `resolveSitInSubjectName()` performs a linear `Array.find()` scan over all `SubjectSessions` groups on every render of every session in the form.
- Files: `src/pages/AbsenceForm.tsx:112-114`, called at line 1181 (once per session in the selected group)
- Cause: No pre-built lookup map exists for course_id → subject_name.
- Improvement path: Build a `Map<string, string>` from `sessions` mapping `course_id` to `subject_name` once via `useMemo`, then use `map.get()` for O(1) lookups.

## Fragile Areas

**AbsenceForm sit-in state management:**
- Files: `src/pages/AbsenceForm.tsx:338,357-368,602-631,723-746`
- Why fragile: Sit-in state is spread across multiple local state variables (`sitInSelections`, `selectedSessionIds`, `selectedSubjectIds`) and recomputed on every interaction. The `missingSitIn` memo (lines 357-368) must stay in sync with `handleSessionToggle` (which clears sit-in selections on deselect) and `handleSitInSelect`. Adding or removing a session requires coordinated state updates in `handleSessionToggle` (lines 602-621) with no central reducer.
- Safe modification: Always update `sitInSelections` in the same state transition as `selectedSessionIds`. Never call `setSitInSelections` independently of `setSelectedSessionIds`.
- Test coverage: Covered by `src/pages/__tests__/AbsenceForm.test.tsx` but the sit-in name resolution tests (lines 285-399) only test two specific scenarios.

**AbsenceDetail override modal data flow:**
- Files: `src/pages/AbsenceDetail.tsx:160-193`
- Why fragile: The override modal loads candidate sessions via a separate API call (`/api/v1/absences/${id}/sit-in-candidates?course_id=...`) and the course list via `/api/v1/courses/public`. The `courseID` state is initialized from `absence.sit_in_course_id` but the courses list may not contain that ID if the sit-in course was deleted or is internal. The override save sends `sit_in_course_id` and `sit_in_session_ids` but the absence API response may not immediately reflect the change (requires `load()` refresh).
- Safe modification: Always await `load()` after `saveOverride()` before assuming the absence state is current.
- Test coverage: Partial — `src/pages/__tests__/AbsenceDetail.test.tsx` covers capacity warnings but not the full override save flow.

## Missing Critical Features

**Backend does not return `sit_in_subject_name` on the sessions-in-range response:**
- Problem: The `SitInInfo` type (`src/types/index.ts:319-335`) includes `sit_in_course?: { id: string; code: string; name: string }` but does not include a `subject_name` or `subject_code` for the sit-in course. The frontend must cross-reference against the sessions array to derive a human-readable subject name.
- Blocks: Clean sit-in name resolution in the absence form. Forces the fragile `resolveSitInSubjectName` workaround.

## Test Coverage Gaps

**Sit-in name resolution edge cases in AbsenceForm:**
- What's not tested: The scenario where `sit_in_course.id` matches no `course_id` in the sessions array (causing `resolveSitInSubjectName` to return `undefined` and falling back to `group.subject_name`). Also untested: what happens when `sit_in_course.name` is set but `sit_in_course.code` contains an internal code, and the available session has no `class_name`, `subject_name`, or `course_name`.
- Files: `src/pages/__tests__/AbsenceForm.test.tsx:285-399` (only two sit-in name resolution tests exist)
- Risk: Regression in name resolution that causes raw codes or wrong course names to appear in the make-up class dropdown.
- Priority: High — this is student/parent-facing UI.

**SitInResultCard not exercised in any production path:**
- What's not tested: The component is tested in isolation but never exercised via a parent component in integration tests.
- Files: `src/components/absences/__tests__/SitInResultCard.test.tsx`
- Risk: If the component is ever integrated, its type assumptions may not match the real API shape.
- Priority: Low (dead code).

**Absences list view sit-in display:**
- What's not tested: The sit-in column in the absences list (`src/pages/Absences.tsx:441-445`) displays `sit_in_course_code` for physical sit-ins. No test verifies what happens when `sit_in_course_code` is `null` (falls back to "Physical" string) or when it's an internal code.
- Files: `src/pages/Absences.tsx:441-445`
- Risk: Users see "Physical" with no course context, or see a raw internal code.
- Priority: Medium — admin-facing, but still a UX gap.

---

*Concerns audit: 2026-05-31*
