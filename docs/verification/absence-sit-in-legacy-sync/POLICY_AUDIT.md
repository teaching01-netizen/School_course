# Subject leave-policy and conflict audit — 2026-10-11

**Baseline verdict:** eight policy defects were reproduced before implementation. **Current worktree status:** AP-1 through AP-8 have been fixed and passed their focused local regressions. The reproduction evidence below describes the pre-fix code; it is retained to explain the changes. Formal proofs describe an abstract model, and production behavior remains **NOT_TESTED**.

## Verification target and boundary

Audit student and staff policy-generated make-up choices, priority reveal, public batch creation, and their conflict checks. PostgreSQL session/enrollment/attendance/assignment state is authoritative. Required outcomes are valid policy targets, no overlap with expected classes or other make-ups, and rejection inside the submission transaction when choices are invalid. Relevant actors are students, staff, and concurrent schedule/submission writers. This engagement demonstrates sequential counterexamples; it does not establish production deployment, concurrent schedule-edit safety, capacity, or source-import correctness.

## Current implementation status

| Finding | Fix recorded in this worktree | Evidence |
|---|---|---|
| AP-1 | Resolver output is filtered against a batched, authoritative attendance query for every emitted choice shape, regardless of presentation filters. Lookup failure fails closed. | `TestPolicyAuditV2KeepsAuthoritativeBlockers`; `TestFilterSitInResultByExpectedSessionsCoversEveryPolicyAndPriorityCollection` passed. |
| AP-2 | The same query checks all expected regular classes at candidate times, including make-up dates outside the absence window. | `TestPolicyAuditPickerChecksClassesOnMakeUpDate` passed against local PostgreSQL. |
| AP-3 | Picker conflict detection no longer excludes classes by selected subject; already-absent sessions remain excluded. | Frontend regression suite passed, including same-subject conflict and absent-session cases. |
| AP-4 | Submission validation checks exact IDs and interval overlap against persisted assignments and within one request. Same-merge-group components may overlap inside one absence selection; different absences still conflict. Student-row serialization and post-lock checks cover concurrent submitters. | `TestPolicyAuditDifferentMakeUpsCannotOverlap`, `TestMutation_ConflictCheckKillsSkip`, and frontend interval/merged-target tests passed. |
| AP-5 | Submission recomputes the server policy result and verifies the declared target and every submitted session against it before commit. | `TestPolicyAuditSessionMustMatchAllowedTarget` passed through the actual transaction handler. |
| AP-6 | Global and scoped automatic-resolution settings are enforced at V1 and V2 resolution boundaries; omitted settings retain the existing enabled default and explicit false vetoes. | Unit and local PostgreSQL global/root/merge scenarios passed. |
| AP-7 | Generic `last_class_excluded` predicates filter every final-local-day target session in V1 and V2 and are included in server policy membership validation. | `TestPolicyAuditGenericFinalDayExcluded` and final-day unit coverage passed. |
| AP-8 | Cross-section selection excludes the exact missed course and, for continuation sources, its continuation group. | Cross-section and continuation resolver regressions passed. |

The focused frontend suite passed 108 tests and `npm run typecheck` passed. Focused `absenceshttp` PostgreSQL tests passed. The `internal/db` candidate-session integration test timed out during the combined run under concurrent load, then passed when rerun by itself against the same local database. See [PROOF_REPORT.md](PROOF_REPORT.md).

**Baseline formal status:** the original eleven model theorems established the pre-fix counterexamples and filter/priority properties. The current `PolicyAudit.lean` also proves post-fix abstract guard properties. **Current correspondence status:** focused local regression tests passed for AP-1 through AP-8. **Production evidence:** NOT_TESTED. The local configuration snapshot is fixture data, not the deployed subject configuration.

[POLICY_AUDIT_REVISION.json](POLICY_AUDIT_REVISION.json) pins the base commit, Lean version, and inspected implementation/proof/reproduction hashes.

## Policy inventory

[POLICY_INVENTORY.json](POLICY_INVENTORY.json) captures all 26 checked-in SAT Verbal definitions and 54 priority entries from `src/features/leave-policy/domain/rules.ts`:

| Family | Definitions | Priority entries |
|---|---:|---:|
| Reading/Writing Beginner sections 1–3 | 6 | 18 |
| Reading/Writing Rank 5, including aliases | 4 | 8 |
| Reading/Writing Rank 4, including aliases | 4 | 8 |
| Rank 3 sections 1–3 | 3 | 7 |
| Rank 2 / Rank 1 | 2 | 5 |
| Reading Mastery / Writing Wisdom | 2 | 2 |
| Realtime Practice / Brush Up | 2 | 3 |
| Knock Out / Intensive / Believe | 3 | 3 |

The generic backend types are `level_ladder`, `cross_section`, `any_day_except_last`, `rank_chain`, and `teacher_case_by_case`. Their existing evaluator and SAT-policy tests passed. The shared-filter theorem quantifies over arbitrary policy-produced collections, but does not prove that a policy chooses the correct target or that its blocker list is complete.

The local PostgreSQL database is `warwick_local_fresh`, schema 130. It contains 1,415 fixture subjects, 273 rule rows, empty subject/root/merge policy settings, and no active mapped SAT policies. It cannot establish which rules are assigned to the user's actual deployed subjects. No production database was accessed.

## Confirmed baseline findings before the fix

### AP-1 — P1: V2 drops expected-class blockers when building the picker

`sessions_range_service.go:233` applies display timing/course filters, and line 391 builds overlap blockers from those filtered facts. The student facts query also excludes hidden/inactive courses (`sessions_range_facts.go:137`). These presentation gates do not cancel attendance obligations. With an expected sibling class overlapping a make-up, the V2 picker still offers the make-up when a course allow-list, notice-time filter, or course visibility hides the blocker. The authoritative SQL returns that blocker.

**Evidence:** `TestPolicyAuditV2KeepsAuthoritativeBlockers/{course_allow_list,timing_policy,hidden_expected_class}` fail in [backend-results.txt](repros/backend-results.txt). **Lean:** `incompleteBlockerProjectionBreaksEveryPolicyShape`. A related implementation hazard is that `applyTimingAndCourseFilter` compacts `facts[:0]`, overwriting the backing array also referenced by `preFilter`; simply switching to that slice would not preserve all original facts.

### AP-2 — P1: make-up dates outside the absence window are checked too narrowly

`expectedSitInConflictSessionsFromFacts` only considers the missed course/merge group inside the absence window. `sitinresolver.ValidateCandidate` additionally checks all expected regular classes at the candidate time. A next-day make-up can therefore appear available while overlapping another enrolled class on that next day. The student's lookup response only covers the original absence window, so the browser also lacks that blocker.

**Evidence:** `TestPolicyAuditPickerChecksClassesOnMakeUpDate` confirms the picker offers the exact candidate that `ValidateCandidate` rejects. **Lean:** `absenceWindowScopeIsWeakerThanGlobalAttendance`.

### AP-3 — P1: the picker still skips selected subjects

`findSitInSessionConflicts` in `src/features/absences/domain/sitInResolution.ts:229` skips every group whose subject is selected. Both `AbsenceForm.tsx:207` and `StaffCreateAbsenceModal.tsx:236` use it to disable choices. The previous submission-payload fix did not change this separate picker helper. An overlapping expected class from the same selected subject produces no picker conflict.

**Evidence:** the first assertion in [frontend-results.txt](repros/frontend-results.txt) expects one conflict but receives zero. **Lean:** `selectedSubjectStillHidesPickerConflict`.

### AP-4 — P1: two different make-up sessions can overlap and both be accepted

`ActiveSitInSessionIDsForStudentCandidates` and `ensureSitInSessionsAvailable` check reuse of an identical session ID. `ValidateCandidate` checks regular classes, but does not check other make-up assignments. The browser also checks duplicate IDs instead of the intervals of selected make-ups. Two different target-session IDs at 13:00–14:00 UTC and 13:30–14:30 UTC are accepted sequentially for the same student by `createAbsenceRecordTx`, even with the real `SitInResolver` configured. The student lock serializes submissions but cannot repair a missing interval check.

**Evidence:** `TestPolicyAuditDifferentMakeUpsCannotOverlap` fails after both actual creation calls return success; the second frontend assertion also returns `ok: true`. All fixture writes were rolled back. **Lean:** `differentIDsDoNotPreventOverlappingMakeUps`.

### AP-5 — P1: a session can come from a different class than the declared target

`batch_routes.go:530` verifies the supplied target class is active, but its selected session IDs are checked independently at lines 578–589. Neither check ties each session to that target or recomputes allowed policy targets. A request declaring target class C while supplying an active session from unrelated class D succeeds in `createAbsenceRecordTx`. This establishes a class/session integrity failure, in addition to the missing authoritative policy-target check.

**Evidence:** `TestPolicyAuditSessionMustMatchAllowedTarget` fails because the actual transaction handler accepts the unrelated session. **Lean:** `activeNonOverlappingSessionNeedNotBePolicyEligible`.

### AP-6 — P2: automatic make-up switches have no effect on generated choices

The administration UI saves `auto_sit_in_enabled` for root and merge groups. `automaticSitInEnabled` exists in `resolver.go:1182` but has no callers and only supports the root/global subset. Neither V2 nor the legacy resolution boundary enforces these switches. V2 produces automatic choices with global, root-group, or merge-group automatic resolution disabled.

**Evidence:** all three `TestPolicyAuditAutomaticPolicyDisabled` cases fail. **Lean:** `omissionOfEnableGateStillOffersChoice`; `disabledPolicyMustOfferNoPhysicalChoice` states the desired abstract guard, not the current implementation.

### AP-7 — P2: generic “any day except last” offers final-day sessions

`evaluateAnyDayExceptLast` chooses a target class, but `buildPhysicalSitInResultWithBlockedSessions` does not exclude its final day. The `last_class_excluded` predicate is not applied in that generic path. The submission final-day check derives its flag from SAT mapping metadata, so a generic rule does not gain that protection.

**Evidence:** `TestPolicyAuditGenericFinalDayExcluded` enters the actual legacy generic resolver with `any_day_except_last` and `last_class_excluded: true`, then observes final-day target sessions in its available choices. **Lean:** `omittedFinalDayGateAllowsForbiddenTarget`.

### AP-8 — P2: generic cross-section can choose the missed class itself

`EvaluateRuleInput` does not include the missed class ID. `evaluateCrossSection` chooses the first course at the student's level, while both resolver paths pass a pool that includes the missed class. If that class comes first, the cross-section rule selects it instead of another section; root-query tie ordering does not make this safe.

**Evidence:** `TestPolicyAuditCrossSectionChoosesAnotherCourse` fails in [cross-section-results.txt](repros/cross-section-results.txt). **Lean:** `crossSectionCanSelectMissedCourseItself`.

## Requirements that need clarification, not another automatic bug label

- **Final missed class versus final target class:** the policy preview blocks a final *missed* lesson (`rules.ts:795`), while the backend explicitly allows a final missed lesson and excludes final *target* sessions (`TestResolveSatVerbalPolicy_SameOccurrenceTargetFinalUnavailableButMissedFinalAllowed`). These are different specifications. `missedFinalAndTargetFinalRulesAreDifferent` proves the predicates differ, not which is correct.
- **Rank 3 sections 2/3 skipping priority 2:** this is deliberate because Rank 2 shares the time slot. `priorityCount: 3` denotes the highest level, not two actual option rows. `gapInRank3PrioritiesStillRevealsLevel3` checks that `[1,3]` can advance to level 3. This is not counted as a defect.

## Baseline proof and test results

- The original audit run used `lake build` and `lake env lean proof/PolicyAudit.lean`: exit 0. Its eleven baseline counterexample/guard theorems report no axioms. The current worktree adds post-fix guard theorems; current Lean results are in [PROOF_REPORT.md](PROOF_REPORT.md). No `sorryAx`, native-evaluation admission, or custom axiom is used.
- Lake's default target and module roots were corrected so a normal build now compiles `proof.Model`, `proof.PolicyAudit`, and the library entrypoint. Earlier direct `lean proof/Model.lean` checks were valid, but the previous default `lake build` did not register the `proof.*` modules.
- Existing frontend suites: **115 tests passed** across leave-policy data, picker resolution, and submission payloads. Focused backend evaluator/SAT/filter suites passed. See the two `*-baseline.txt` logs.
- `npm run typecheck` also passed with the frontend reproduction temporarily included in the source tree; see [reproduction-typecheck.txt](repros/reproduction-typecheck.txt).
- At baseline, the retained safety assertions failed as expected: 11 backend scenarios and two frontend assertions. Those reproductions established the findings. The Go reproduction is now also covered by permanent passing regression tests; source reproductions remain under `repros/` as historical evidence.
- Integration fixtures used local PostgreSQL transactions and rolled back all application-row changes. The database remained at migration 130.

The baseline findings are concrete counterexamples plus separately reviewed implementation mappings. They are not a mechanized proof of production code or an exhaustive proof that no further defects exist. Nonempty choices, real expected classes, two distinct bookings, and actual transaction handlers keep the counterexamples non-vacuous.

## Repairs applied to the baseline findings

1. At the shared submission-validation boundary, enforce session-to-target membership, including permitted merged/continuation target members, and server-owned allowed policy targets; check interval overlaps against existing active make-ups and all candidates within the current batch while holding the existing student lock. Address AP-4/AP-5 first. Validate the full eligible target set rather than trusting a client priority cursor or only the currently displayed page.
2. Load complete expected-class blockers across the offered make-up date span independently of display/notice/visibility/course filters. Reuse that authoritative set for all V1/V2 policy output shapes and submit validation. Remove the selected-subject exception from the picker helper. Preserve unfiltered facts by allocating the filtered output separately. Addresses AP-1/AP-2/AP-3.
3. Enforce global/root/merge automatic-resolution switches at the common resolution boundary, with explicit scope precedence. Carry final-day eligibility into the generic candidate builder and submission policy check. Include the missed course ID when selecting a sibling target. Addresses AP-6/AP-7/AP-8.
4. Turn the retained safety reproductions into permanent regressions as each repair lands, then check the updated Lean definitions and their correspondence again. Confirm deployed subject mappings and decide the final-missed-class specification before declaring all-subject correctness.

The eight application fixes are present in the current worktree. The repairs list above records the implementation boundaries used; it is no longer a pending implementation plan. Production rollout and acceptance evidence remain outside this local verification.
