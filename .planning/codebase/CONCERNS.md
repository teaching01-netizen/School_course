# Codebase Concerns: Sit-In Rules & Absence Handling

**Analysis Date:** 2026-06-06

## Tech Debt

**Sit-In Rule Evaluation - Missing Priority Logic:**
- Issue: Rule types are evaluated via a simple switch statement with no priority ordering. When multiple rules could theoretically apply to a student, only the single rule attached to the `root_course_groups.sit_in_rule_id` is evaluated.
- Files: `backend/internal/httpapi/absenceshttp/rule_evaluator.go` (lines 57-72), `backend/internal/httpapi/absenceshttp/resolver.go` (lines 204-213)
- Impact: The system assumes exactly one sit-in rule per root course group. If a student belongs to overlapping root course groups or if business rules evolve to require priority ordering (e.g., "if level_ladder matches, don't check cross_section"), there is no mechanism to support this.
- Fix approach: Introduce a `priority` column on `sit_in_rules` and iterate through rules in priority order until one returns `Eligible: true`. Alternatively, implement a composite evaluator that tries multiple rules per root course group.

**Rule Predicate Overloaded JSONB - No Validation:**
- Issue: The `sit_in_rules.predicate` column is a JSONB field with no schema validation at the DB or application level. Each rule type uses different fields from the same `RulePredicate` struct (e.g., `rank_chain` uses `chains`, `level_ladder` uses `min_level_for_sit_lower`, `teacher_case_by_case` uses `requires_teacher_approval`). Unused fields are silently ignored.
- Files: `backend/internal/httpapi/absenceshttp/rule_evaluator.go` (lines 12-25), `backend/db/migrations/00023_sit_in_rules.sql` (lines 3-11)
- Impact: Admin can create a `rank_chain` rule with empty `chains` array and no validation error — the rule silently returns `not eligible`. No feedback that the predicate is incomplete for the chosen rule type.
- Fix approach: Add per-rule-type predicate validation in `SitInRuleCreate`/`SitInRuleUpdate` handlers in `backend/internal/httpapi/sitinruleshttp/routes.go`. Validate that required fields for the specific `type` are present and well-formed.

**Cross-Section Evaluator Ignores `section_match` / `occurrence_match` / `day_match`:**
- Issue: `evaluateCrossSection()` in `rule_evaluator.go` (lines 177-211) ignores the `section_match`, `occurrence_match`, and `day_match` predicate fields entirely. It simply finds any other course at the same level. The predicate fields `cross_section` / `same_occurrence_number` / `any` are defined in seed data but never used in evaluation logic.
- Files: `backend/internal/httpapi/absenceshttp/rule_evaluator.go` (lines 177-211), `backend/db/migrations/00023_sit_in_rules.sql` (line 21)
- Impact: The cross-section sit-in may suggest sessions that don't match the intended occurrence number or day, leading to incorrect sit-in recommendations.
- Fix approach: Implement the actual cross-section filtering logic — match by occurrence number within the session series, filter by day-of-week, etc.

**`resolveSitIn` vs `resolveSitInForCourse` Duplication:**
- Issue: Two nearly identical resolver functions exist. `resolveSitIn()` (lines 285-425) is used by the `/sit-in-options` endpoint and picks the "main course" (lowest enrolled level). `resolveSitInForCourse()` (lines 148-283) is used by `/sessions-in-range` and takes a specific course ID. Both perform the same root course group lookup, rule loading, predicate parsing, and evaluation — with slightly different course selection logic.
- Files: `backend/internal/httpapi/absenceshttp/resolver.go` (lines 148-283, 285-425)
- Impact: Maintenance burden — a bug fix in one resolver may not be applied to the other. The "main course" selection in `resolveSitIn` (lowest level) differs from `resolveSitInForCourse` (the specific missed course), which could lead to inconsistent results.
- Fix approach: Extract shared logic into a common function. The difference is only in "which course is the missed course" — parameterize that and unify the resolution pipeline.

**`automaticSitInEnabled` Function Defined But Never Called in Resolver Path:**
- Issue: `automaticSitInEnabled()` (resolver.go lines 457-481) checks the `absence_policies` JSON for an `auto_sit_in_enabled` flag per root course group. However, neither `resolveSitIn` nor `resolveSitInForCourse` calls this function — sit-in resolution always proceeds regardless of this flag.
- Files: `backend/internal/httpapi/absenceshttp/resolver.go` (lines 457-481)
- Impact: The `auto_sit_in_enabled` policy flag in `app_settings.absence_policies` is effectively dead code in the sit-in resolution path. If an admin disables auto sit-in, the system still resolves sit-ins.
- Fix approach: Add an `automaticSitInEnabled()` check at the start of both resolver functions. If disabled, return `SitInMethodNone` / `nil` early.

**Sit-In Window Cutoff Applied Inconsistently:**
- Issue: `buildPhysicalSitInResult()` applies a `cutoff` time to filter available sessions (resolver.go lines 90-93). The cutoff is loaded from `absence_policies.root_course_groups[id].sit_in_window_weeks`. However, the cutoff only filters `Available` sessions — it does NOT filter `MissedSession` or affect the `MissedCount`. Also, when the cutoff is zero (no window configured), all sessions pass — there's no cap.
- Files: `backend/internal/httpapi/absenceshttp/resolver.go` (lines 72-127, 270-275, 412-417)
- Impact: If no sit-in window is configured, students can theoretically sit in sessions far in the future. The pre-selection count (line 96-100) uses `len(missed)` which may exceed available sessions within the window.
- Fix approach: Consider making the window required (non-zero default) or adding a maximum window cap. Also ensure pre-selection only counts sessions within the cutoff.

## Known Bugs

**`buildPhysicalSitInResult` Pre-Selection Can Over-Select:**
- Symptoms: `PreSelected` count is `min(len(missed), len(nonOverlapping))` (line 96-100). If there are 3 missed sessions but only 2 available non-overlapping sessions within the window, it pre-selects all 2 available. This may confuse the UI if the student expected to see 3 sit-in options.
- Files: `backend/internal/httpapi/absenceshttp/resolver.go` (lines 96-100)
- Trigger: Student misses 3 classes, only 2 sit-in slots available in target course within the window.
- Workaround: The UI should handle the case where `pre_selected.length < missed_count` and display accordingly.

## Security Considerations

**Sit-In Rule CRUD Endpoints Lack Rate Limiting:**
- Risk: The admin sit-in rule CRUD endpoints (`/api/v1/admin/sit-in-rules`) are wrapped with idempotent transactions but have no rate limiting beyond the standard idempotency key mechanism.
- Files: `backend/internal/httpapi/sitinruleshttp/routes.go` (lines 19-27)
- Current mitigation: Idempotency keys prevent duplicate writes. Admin auth required.
- Recommendations: Low priority — admin-only endpoints. Consider adding validation that prevents deleting a sit-in rule that is currently referenced by active root course groups.

**Public Sit-In Endpoints Expose Course Structure:**
- Risk: `handleSessionsInRange` and `handleSitInOptions` are public (no auth) and accept a `wcode` parameter. They return course codes, names, subject info, and session times for all courses in a root course group.
- Files: `backend/internal/httpapi/absenceshttp/routes.go` (lines 39-41, 687-981)
- Current mitigation: The wcode acts as a weak form of identification. Sessions are filtered to the student's enrolled courses.
- Recommendations: Consider whether this data exposure is acceptable for a public form. The sit-in options endpoint reveals the entire course ladder structure (all levels in the root course group).

## Fragile Areas

**`resolveSitIn` Course Selection Logic:**
- Files: `backend/internal/httpapi/absenceshttp/resolver.go` (lines 314-321)
- Why fragile: The "main course" is selected as the lowest-level enrolled course. But then root course group enrollment is loaded to get ALL enrolled levels. The logic assumes the first course found in the root course group is the right one. If a student is enrolled in multiple root course groups for the same subject, only the first one with a `RootCourseGroupID` is used.
- Safe modification: When modifying course selection, always test with students enrolled in multiple levels within the same root course group, and students enrolled across multiple root course groups.

**Rule Evaluator Switch Statement:**
- Files: `backend/internal/httpapi/absenceshttp/rule_evaluator.go` (lines 57-72)
- Why fragile: Adding a new rule type requires modifying the switch statement AND creating a new `evaluate*` function. There's no compile-time enforcement that all rule types are handled — a typo in `rule.Type` falls through to the `default` error case.
- Safe modification: Consider using a registry pattern or compile-time exhaustive switch.

## Missing Critical Features

**No Rule Type Priority / Ordering:**
- Problem: Each root course group has exactly one `sit_in_rule_id`. There's no way to define fallback rules (e.g., "try level_ladder first, if not eligible, try cross_section").
- Blocks: Complex sit-in scenarios where a student's eligibility depends on multiple rule types.

**No Sit-In Rule Validation on Assignment:**
- Problem: When assigning a `sit_in_rule_id` to a `root_course_group`, there's no validation that the rule type is compatible with the course group's structure (e.g., a `rank_chain` rule assigned to a course group with only 1 level will always return "not eligible").
- Blocks: Prevents admin misconfiguration that leads to confusing sit-in behavior.

**No Audit Trail for Sit-In Rule Changes:**
- Problem: `SitInRuleCreate`, `SitInRuleUpdate`, and `SitInRuleDelete` in `sit_in_rules_custom.go` don't write to the audit log. Changes to sit-in rules are not tracked.
- Blocks: Cannot determine when or why a sit-in rule was changed, making debugging sit-in resolution issues difficult.

## Test Coverage Gaps

**Rule Evaluator Has Good Unit Tests:**
- Files: `backend/internal/httpapi/absenceshttp/rule_evaluator_test.go` (656 lines)
- Coverage: Tests cover all 5 rule types, edge cases (level 1 zoom, enrolled level skipping, no matching chain, unknown rule type).
- Risk: Tests are thorough for the evaluator in isolation.

**Integration Test Gap - `resolveSitIn` and `resolveSitInForCourse`:**
- What's not tested: The resolver functions that wire together DB queries, rule loading, and evaluation are only tested via integration tests (`absence_sit_in_dates_integration_test.go`, `absence_sit_ins_calendar_integration_test.go`). No unit tests for the resolver functions themselves.
- Files: `backend/internal/httpapi/absenceshttp/resolver.go`
- Risk: The integration between rule evaluation and session filtering (overlap detection, cutoff window) is only covered by integration tests that require a running database.
- Priority: Medium

**`handleSessionsInRange` Has No Tests:**
- What's not tested: The `handleSessionsInRange` handler (routes.go lines 687-981) performs complex logic: session querying, absence flagging, sit-in resolution per course, and response assembly. This handler has zero test coverage.
- Files: `backend/internal/httpapi/absenceshttp/routes.go` (lines 687-981)
- Risk: Regression in the public-facing sessions-in-range endpoint would not be caught by tests.
- Priority: High

---

*Concerns audit: 2026-06-06*
