# Executable Roadmap — Comprehensive Staff Course-Schedule Subsystem

Repository: warwick-institute (backend Go + sqlc/pgx, frontend React/Vite/vitest, e2e Playwright)
Author: delivery orchestrator (main session). Status: awaiting dual-approval gate.

## 0. Mission (paraphrase of the original request)

> "Work on the course schedule part — create course, edit, do operational tasks for sessions (reschedule, etc.) for staff. Make it comprehensive and match real staff functionality. Use ERP-implementation-cookbook principles and rigorous-gated-delivery. I allow changing the logic/entity model of how courses and subjects are created, or anything else, to make it maintainable with best-practice data modeling."

The delivery contract: staff can run the complete course-schedule lifecycle (create/edit course, create and maintain subjects, schedule one-off and recurring sessions, and perform operational session tasks — reschedule, cancel, restore, bulk operations, attendance/check-in, room assignment) against a data model that is maintainable and ERP-grade: every state change is an event, past data is never destroyed, closed/past operations are immutable, atomicity is enforced at the DB, and audit trails are first-class.

## 1. Current state (scoping findings)

Already implemented and treated as stable foundation (do not regress):
- Data model: `courses` (code UNIQUE, course_no, year, subject_id, teacher_id compat projection, hour, student_count, course_type, version, cohort_id, legacy_archived, legacy_course_id), `subjects`, `course_teachers` (is_primary, one-primary partial unique index), `sessions` (version, deleted_at, source_kind, GiST exclusion constraints for room/teacher overlap), `session_series` (weekly recurrence, materialized occurrences), `student_busy_ranges` (DB-enforced no-overlap, derived roster = course roster ∪ includes \ excludes), `session_changes` event log (changed_by, batch_id, change_source, changed_fields, before/after snapshots, UNIQUE(session_id, session_version)), `session_change_batches`, `outbox_events`, `absence_schedule_issues` impact queue, rooms (name, capacity — capacity unused), teacher_availability/room_availability.
- Domain: courseadmin (create/update teacher sets, stale_edit OCC, teacher_in_use guard, course.created/teachers_updated audit), scheduling (preflightSlot, advisory schedule locks, serializable retry, CreateSession/EditOccurrence/DeleteSession, series create/split-this-and-future/cancel/edit-entire with membership checks, SlotFinder FindAvailableSlots), series (materialize, limits: 1000 occurrences / 5 years / 24h), sessionchangeimpact (analysis, notifications via outbox, issues queue + resolution).
- HTTP: courses CRUD + students + batch-delete; sessions CRUD + bulk-update + attendance + change-preview; series POST/PATCH/PATCH-entire/cancel; scheduling preflight/preflight_series/find-slots; subjects list/create/delete (no edit); rooms CRUD. Idempotency-Key middleware on all mutations.
- Frontend: CourseCreate, CourseDetail (table/calendar, session popover create/edit, bulk edit, paste schedule, create series, roster, delete), Schedule (global list; create/edit/cancel session; attendance modal; series This&Future / Entire / Cancel; impact links), SlotFinder, SessionChanges (impact queue), Subjects/SubjectCreate, Classrooms, OperationsCalendar.
- Tests: extensive Go unit + integration (TEST_DATABASE_URL postgres running locally: `postgresql://rd-cream@127.0.0.1:5432/warwick_local_fresh`), vitest (two configs; `vitest.schedule.config.ts` enforces 100% statement/function/branch/line coverage on a fixed file list), Playwright e2e specs exist.

## 2. Confirmed gaps vs ERP principles ("comprehensive staff schedule")

| # | Gap | Violated ERP principle |
|---|-----|------------------------|
| G1 | Staff session cancel (`DELETE /api/v1/sessions/{id}`) hard-deletes the row via `SessionHardDelete` — attendance, busy ranges, history destroyed; no `session_changes` event, no actor/reason. Soft-delete machinery (00092/93/96 triggers, restore branch) exists and is only wired for series cancel / legacy restore. | #2 every change is an event; #4 immutability of past ops; #5 audit |
| G2 | Staff cannot restore a cancelled session; no UI, no endpoint. | #2/#4 (history must be recoverable) |
| G3 | Subject delete cascades to courses (`ON DELETE CASCADE`), destroying courses/absences/history. No subject edit endpoint exists at all. | #1 never trust destructive writes; #2 |
| G4 | Course delete hard-cascades sessions, series, absences, crm diffs. Staff archive concept exists only as `legacy_archived` (legacy-sync flag); no staff archive/unarchive operation with invariants. | #2/#4 |
| G5 | Course creation has two divergent paths (plain code/name vs subject-generated `CourseCreateV2`); subject not required; code derivation lives in SQL and command layer inconsistently. | maintainability (#6 separate change-fast from change-slow) |
| G6 | Room `capacity` is defined but never enforced anywhere (preflight/write/UI). | #1/#3 invariants at data layer |
| G7 | No dedicated reschedule flow: rescheduling = in-place edit; SlotFinder can't search "next free slot for this session" (no ignore-session), so a session's own slot shows as blocked. | staff workflow gap |
| G8 | No bulk cancel; CourseDetail bulk modal edits fields only. `session_change_batches` infra exists unused for staff bulk. | ops efficiency + audit |
| G9 | CourseDetail lacks series operations (This&Future, Entire, Cancel series) and attendance/check-in ("Check in" row action is a dead placeholder that just navigates to /schedule). | staff workflow parity |
| G10 | No cancelled-session visibility (no list query, no restore affordance anywhere). | #5 audit visibility |

## 3. Roadmap items

Conventions: `R1..R8` = backend/data-model lanes; `U1..U7` = frontend lanes; `D1..D4` = verification/delivery lanes. Every write path stays behind the domain services (ERP #1); no UI/batch writes bypass services. All mutations keep idempotency keys. All new behavior is test-first.

### Backend & data model

---

#### R1 — Subject-required, single-path course creation
- OBJECTIVE: Remove the dual course-create path. `POST /api/v1/courses` (and the domain command) requires a valid `subject_id`; code is derived from the subject's course-generation template and the `course_no` sequence; name defaults from the subject template and may be overridden via an optional `name`.
- OWNED SURFACE: `backend/internal/courseadmin/{commands.go,service.go,validation.go}`, `backend/internal/httpapi/courseshttp/requests.go` + create handler, sqlc `CourseCreateV2` (reuse), integration tests in `internal/courseadmin/*_integration_test.go`, `internal/httpapi/courseshttp/routes_integration_test.go`.
- DEPENDENCIES: none (foundation lane).
- IMPLEMENTATION INTENT: `CreateCourseTx` rejects invalid/absent SubjectID with `invalid_subject` before any write; plain code/name branch is deleted from the service (legacy sync has its own apply path and is not affected); `name` optional override; `code` always derived; audit payload `course.created` gains `subject_id` + `code`.
- ACCEPTANCE CRITERIA:
  1. `POST /courses` without `subject_id` returns 400 `invalid_subject`, creates no rows.
  2. `POST /courses` with a subject creates one course whose `code` is derived from the subject template + `course_no`, `version=1`, teacher set valid, audit row `course.created` present with subject_id.
  3. Legacy sync apply path still passes its own integration tests unchanged.
- UNHAPPY PATHS: invalid/soft-deleted subject → 400; duplicate rapid creates → unique course_no constraint + idempotency; missing name override → template default.
- TESTS: unit (validation), integration (create+audit+derived code, missing subject 400), legacy-sync regression suite.
- REAL VERIFICATION: `go test ./internal/courseadmin/... ./internal/httpapi/courseshttp/... ./internal/legacysync/...` against local TEST_DATABASE_URL; curl the endpoint over the dev server.
- REQUIRES_DETAILED_PLAN: true (contract change).

---

#### R2 — Subject edit + delete protection
- OBJECTIVE: Add `PUT /api/v1/subjects/{id}` (admin) for name/edit; make subject delete safe: deleting a subject referenced by any course returns 409 `subject_in_use` (courses listed in details) instead of cascading; delete succeeds only when no courses reference it (cascade rows other than courses — none — still cleaned).
- OWNED SURFACE: `backend/internal/httpapi/subjectshttp/routes.go` (new PUT route + updated DELETE), new `internal/db` query `SubjectUpdate` (sqlc), audit event `subject.updated`/`subject.deleted`, `backend/internal/httpapi/subjectshttp/routes_test.go` + integration test.
- DEPENDENCIES: none.
- IMPLEMENTATION INTENT: `SubjectUpdate` validates uniqueness of code (409 `duplicate_code` on collision), updates name/code; update allowed when code unchanged even if courses reference it; DELETE path first counts referencing non-archived courses and returns 409 with `course_ids` details when > 0; audit insert in same tx.
- ACCEPTANCE CRITERIA:
  1. `PUT /subjects/{id}` succeeds and updates name; audit row written.
  2. `PUT` to an existing code of another subject → 409 `duplicate_code`, nothing changed.
  3. `DELETE /subjects/{id}` with referencing courses → 409 `subject_in_use` with details; zero rows removed.
  4. `DELETE` with no referencing courses removes the subject row only.
- UNHAPPY PATHS: nonexistent subject → 404; concurrent create of same code → unique violation classified as 409.
- TESTS: unit + integration for all four acceptance criteria + concurrent code collision.
- REAL VERIFICATION: `go test ./internal/httpapi/subjectshttp/... ./internal/db/...`; curl PUT/DELETE over dev server.
- REQUIRES_DETAILED_PLAN: false.

---

#### R3 — Staff course archive/unarchive (replaces destructive delete as the normal operation)
- OBJECTIVE: Add `courses.archived_at` (staff archive flag, distinct from `legacy_archived`); endpoints `POST /api/v1/courses/{id}/archive` and `POST /api/v1/courses/{id}/unarchive` with optimistic version; archiving is blocked while the course has any session starting in the future (error `course_has_future_sessions` with count + earliest); archived courses reject schedule mutations (create session/series, edit session of that course) with 409 `course_archived`; unarchive restores scheduling. Course delete (existing `DELETE /courses/{id}` and batch-delete) becomes admin-only (visible/usable by admins only); staff use archive.
- OWNED SURFACE: migration `00099_course_archive.sql` (archived_at + partial index), `backend/internal/courseadmin/service.go` (Archive/Unarchive + guard checks), `backend/internal/httpapi/courseshttp/routes.go` (PARSE routes), scheduling service archive guard in CreateSessionTx/EditOccurrenceTimeTx/CreateSeriesAndMaterializeTx, integration tests.
- DEPENDENCIES: R1 (schedule mutations share service seams).
- IMPLEMENTATION INTENT: archive/unarchive run in one tx with `SELECT ... FOR UPDATE`; archive checks future sessions (`start_at >= now UTC` and `deleted_at IS NULL`); guard helper `ensureCourseSchedulable(ctx, qtx, courseID)` called at the top of every schedule mutation; audit actions `course.archived`/`course.unarchived`; `CourseOverview` unchanged (archive bucket still legacy_archived for now; U7 reconciles).
- ACCEPTANCE CRITERIA:
  1. Archive of a course with a future session → 409 `course_has_future_sessions`, unchanged state.
  2. Archive of a course with only past/no sessions sets `archived_at`, bumps version, writes audit.
  3. Post-archive: create session / edit session / create series for that course → 409 `course_archived`.
  4. Unarchive succeeds with matching version; scheduling works again.
  5. `DELETE /courses/{id}` and `/courses/batch-delete` require admin role (403 for non-admins).
- UNHAPPY PATHS: stale version → `stale_edit` with current payload; concurrent archive/delete; legacy sync applies to archived course (allowed — legacy flag is separate; document).
- TESTS: integration: archive invariants, guards on all three mutation paths, admin gate on delete/batch-delete, legacy sync regression.
- REAL VERIFICATION: `go test ./internal/courseadmin/... ./internal/scheduling/... ./internal/httpapi/courseshttp/...`; curl flows.
- REQUIRES_DETAILED_PLAN: true (guard placement across scheduling service).

---

#### R4 — Session cancel is an event, not a delete (soft cancel with reason + actor)
- OBJECTIVE: Staff `DELETE /api/v1/sessions/{id}` (body gains optional `reason`) soft-deletes the session (`deleted_at` set, `version` bumped) and records a `session_changes` event (`change_source='session_cancel'`, `changed_fields=['deleted','cancelled']`, actor + reason), queues the impact run + outbox notification (students/parents), and soft-deletes the session's `student_busy_ranges` so the slot frees. No hard delete anywhere in the staff path.
- OWNED SURFACE: migration `00100_session_cancel_reason.sql` (`session_changes.reason text NULL`), extend `record_session_soft_delete_impact` trigger (00092/93) to read actor/reason/source from `current_setting('app.*')` (set by the service via `set_config`) so the DB trigger stays the single source of event truth; scheduling `DeleteSessionTx` switches `SessionHardDelete` → version-guarded soft delete; sessionshttp DELETE handler parses `reason`; queries `SessionSoftDelete` (sqlc); integration tests in `internal/scheduling/service_integration_test.go`, `internal/httpapi/sessionshttp/routes_test.go`.
- DEPENDENCIES: none (foundation lane).
- IMPLEMENTATION INTENT: service sets `app.actor_id`, `app.change_source='session_cancel'`, `app.cancel_reason` via `set_config` inside the tx before the UPDATE; trigger reads them (with the current hardcoded defaults as fallback so series-cancel/legacy paths keep working); busy-range soft-delete already handled by 00096 refresh branch invoked by the session UPDATE trigger chain; stale_edit semantics unchanged (expected_version gate); idempotency key reuse returns the same result.
- ACCEPTANCE CRITERIA:
  1. DELETE with wrong version → `stale_edit`, no change.
  2. DELETE cancels: row `deleted_at` set, version bumped, `session_changes` row exists (source session_cancel, changed_by actor, reason preserved, changed_fields contains deleted), impact run pending, outbox event queued; `student_busy_ranges` soft-deleted.
  3. After cancel, the same room/teacher slot is bookable for the students (busy ranges freed, exclusion constraints pass).
  4. Cancel with no reason succeeds; reason stored as NULL.
  5. Replaying the same idempotency key returns the original result and creates no duplicate events.
- UNHAPPY PATHS: concurrent edit during cancel → stale_edit; trigger double-record prevented (single event source — Go must NOT also call recordSessionChange for cancels); cancel of already-cancelled session → 404/stale.
- TESTS: integration R4.1–R4.5; regression: series cancel path still records series_cancel; legacy restore still records restored.
- REAL VERIFICATION: `go test ./internal/scheduling/... ./internal/httpapi/sessionshttp/... ./internal/db/...`; curl DELETE + inspect session_changes/busy ranges.
- REQUIRES_DETAILED_PLAN: true (trigger + set_config pattern, concurrency, no double-record).

---

#### R5 — Restore a cancelled session (admin) with re-preflight
- OBJECTIVE: `POST /api/v1/sessions/{id}/restore` (admin) body `{expected_version}` restores a soft-deleted session: re-preflight (room/teacher/student overlap at original times, capacity), clears `deleted_at`, bumps version, records `session_changes` event `change_source='session_restore'`, re-creates busy ranges, queues impact/outbox (00093 restore branch already retires delete-impact; extend for actor/reason via the same `current_setting` mechanism). Any conflict at the original times blocks restore with `schedule_conflict` details.
- OWNED SURFACE: scheduling service `RestoreSessionTx` (new), sessionshttp `POST /sessions/{id}/restore`, sqlc queries (`SessionGetByID` reuse, `SessionRestore`), integration tests.
- DEPENDENCIES: R4 (same trigger/settings machinery).
- IMPLEMENTATION INTENT: load session; if not deleted → 409 `session_not_cancelled`; lock resources (students/teacher/room) — reuse R4 pattern; run preflightSlot for the original interval (IgnoreSession=self); on conflict → `schedule_conflict`; UPDATE deleted_at=NULL + version bump with `set_config(app...)`; busy ranges rebuilt by refresh function (00096 non-deleted branch); impact run + outbox emitted by 00093 restore branch.
- ACCEPTANCE CRITERIA:
  1. Restore of a cancelled session with a free slot succeeds; `deleted_at` NULL; `session_changes` has a `restored` event with actor; busy ranges active again; pending delete-impact runs superseded.
  2. Restore when the slot is now occupied (room, teacher, or any roster student) → `schedule_conflict` with details, no change.
  3. Restore of an active session → 409 `session_not_cancelled`.
  4. Non-admin caller → 403.
  5. Wrong version → `stale_edit`.
- UNHAPPY PATHS: session hard-missing → 404; concurrent scheduler claims the slot → conflict on write + serializable retry returns conflict.
- TESTS: integration all criteria; concurrency test (slot claimed between preflight and write).
- REAL VERIFICATION: `go test ./internal/scheduling/... ./internal/httpapi/sessionshttp/...`; curl restore flows.
- REQUIRES_DETAILED_PLAN: true (concurrency window).

---

#### R6 — Reschedule proposal search (slot finder with ignore-session + identity overrides)
- OBJECTIVE: Extend `POST /api/v1/scheduling/find-slots` (or add `POST /api/v1/sessions/{id}/reschedule-proposals` thin wrapper) so staff can find the next free slots for an existing session, ignoring that session itself and optionally pinning a different room/teacher. This powers the reschedule modal (U1).
- OWNED SURFACE: `backend/internal/scheduling/slot_finder.go` + `service.go` `FindAvailableSlots` (accept optional `session_id` to ignore, optional `room_id`/`teacher_id`/`student_ids` overrides), `backend/internal/httpapi/schedulinghttp/routes.go` (extend body), tests.
- DEPENDENCIES: none (parallel lane).
- IMPLEMENTATION INTENT: keep the batch window/coverage logic; add ignore-session filter in the teacher/room/session overlap queries (exclude the session's own interval from the candidate-overlap check); when `session_id` given, default teacher/room/students from that session unless overridden; keep 14-day cap, provisional/blocked statuses; capacity-aware status via R8 helper.
- ACCEPTANCE CRITERIA:
  1. find-slots with `session_id` returns the session's own current slot as available (not blocked by itself).
  2. Overrides for room/teacher change the blocked-set accordingly.
  3. Existing SlotFinder behavior without `session_id` is unchanged (regression).
  4. Response shape stays backward-compatible (new optional request fields only).
- UNHAPPY PATHS: session_id of another course → 400; room override that is booked → blocked status shown; >14 days → existing error.
- TESTS: unit (slot_finder) + integration + frontend SlotFinder regression.
- REAL VERIFICATION: `go test ./internal/scheduling/... ./internal/httpapi/schedulinghttp/...`; curl.
- REQUIRES_DETAILED_PLAN: false.

---

#### R7 — Bulk cancel with batch audit
- OBJECTIVE: `POST /api/v1/sessions/bulk-cancel` body `{updates:[{id, expected_version, reason}], actor}` cancels (R4 semantics) many sessions in per-row results `[{id, status: cancelled|stale_edit|error, error?}]`, creating one `session_change_batch` row linking every event (existing table). Used by U5.
- OWNED SURFACE: scheduling service `BulkCancelSessionsTx` (per-row tx processing, batch row insert), sessionshttp route, integration tests.
- DEPENDENCIES: R4.
- IMPLEMENTATION INTENT: inside one DB tx: insert batch row (`requested_by=ActorID`, counts); per row: soft cancel + event (same trigger path); collect results; on any hard failure abort whole batch (atomicity principle #3); idempotency key returns the same batch result on replay.
- ACCEPTANCE CRITERIA:
  1. Mixed batch: valid rows cancelled with events linked to one batch; stale rows reported as `stale_edit`; invalid IDs reported as `error` without aborting the batch.
  2. All events carry the batch_id and changed_by.
  3. Replay with the same idempotency key returns identical results, no duplicate events.
- UNHAPPY PATHS: DB failure mid-batch → whole tx rolled back (nothing half-cancelled); batch exceeds 100 rows → 400 `too_many`.
- TESTS: integration mixed-batch, rollback, idempotency.
- REAL VERIFICATION: `go test ./internal/httpapi/sessionshttp/... ./internal/scheduling/...`; curl.
- REQUIRES_DETAILED_PLAN: false.

---

#### R8 — Room capacity invariant
- OBJECTIVE: Enforce `rooms.capacity` as a hard invariant: effective roster (course roster ∪ included overrides \ excluded overrides) must fit in the room for one-off session create/edit, series creation, and restores. Errors: `room_capacity_exceeded` with `{capacity, roster_size, room_name}`; preflight surfaces it as a blocked status with kind `room_capacity` so the UI can show it before saving. Series preflight checks once per series (same roster each occurrence) but reports with occurrence detail.
- OWNED SURFACE: `backend/internal/scheduling/preflight.go` (capacity check in preflightSlot and series batch), `service.go` (create/edit/series paths), `backend/internal/httpapi/schedulinghttp/routes.go` (preflight response), existing `effectiveStudentIDsForSession` helper, integration tests.
- DEPENDENCIES: none (can land with any lane).
- IMPLEMENTATION INTENT: when `RoomID.Valid` and room has `capacity` (non-null), count effective roster; > capacity → immediate `room_capacity_exceeded` Err (and preflight status blocked). No room (nil) → provisional, capacity not applicable. Join rooms in the lock query to fetch capacity under the same lock.
- ACCEPTANCE CRITERIA:
  1. Create/edit/series create with roster > capacity → 409 `room_capacity_exceeded`, no write.
  2. Preflight returns blocked + kind `room_capacity` with capacity and roster size.
  3. Room without capacity (NULL) never blocks for capacity.
  4. Attendance overrides (include/exclude) adjust the effective count used for the check.
- UNHAPPY PATHS: race (roster changes between preflight and write) → write path re-checks authoritatively; exactly-at-capacity passes; room deleted → existing FK error path.
- TESTS: unit + integration for all criteria incl. override adjustment.
- REAL VERIFICATION: `go test ./internal/scheduling/... ./internal/httpapi/schedulinghttp/...`; curl preflight + write.
- REQUIRES_DETAILED_PLAN: true (roster-count consistency across all write paths).

### Frontend (staff UX)

---

#### U1 — Reschedule flow (CourseDetail + Schedule)
- OBJECTIVE: A dedicated "Reschedule" action on a session opens a modal: pick new date/time (or pick from suggested slots from R6), see live preflight (conflicts/capacity via R8), acknowledge impact (existing change-preview + ImpactAcknowledgementModal), save via existing `PATCH /sessions/{id}`.
- OWNED SURFACE: `src/features/scheduling/components/RescheduleModal.tsx` (new), `src/pages/CourseDetail.tsx`, `src/pages/Schedule.tsx` (wire the action), `src/features/scheduling/hooks/useReschedule.ts` (new), api client (find-slots session_id param), tests.
- DEPENDENCIES: R6, R8, existing change-preview flow.
- IMPLEMENTATION INTENT: reuse SessionEditorPopover property-row grammar + usePreflight/usePreflightGate + existing stale_edit recovery; slot suggestions list (max 5) with quick-select; accessibility parity (focus trap, announcements).
- ACCEPTANCE CRITERIA:
  1. Reschedule shows current + proposed time, preflight status, and (when requested) N suggestions excluding the session itself.
  2. Conflicting/over-capacity selection disables save with reason text.
  3. Impact-requiring changes show the acknowledgement dialog and send `acknowledge_impact`.
  4. `stale_edit` from the server reseeds the form from `err.details.current`.
- UNHAPPY PATHS: preflight outage (existing failureRecovery behavior), suggestion list empty, session deleted mid-flow.
- TESTS: vitest (new `RescheduleModal.test.tsx` + Schedule/CourseDetail journeys); keep the fixed 100%-coverage file list green (add tests for touched covered files).
- REAL VERIFICATION: `npx vitest run --config vitest.schedule.config.ts`, full `npm test`.
- REQUIRES_DETAILED_PLAN: false.

---

#### U2 — Cancel with reason + cancelled-session visibility + restore (CourseDetail, admin)
- OBJECTIVE: Cancel dialogs gain an optional reason field (both pages); CourseDetail gains a "Cancelled" section listing recently cancelled sessions (via `include_cancelled` on the course sessions query) with muted styling, cancel info (who/when/reason from the event), and an admin-only Restore action calling R5.
- OWNED SURFACE: `src/features/courses/api/courseApi.ts` (include_cancelled param + typed cancelled rows), `src/components/CancelledSessionsSection.tsx` (new), `ConfirmModal` reason input, `src/pages/CourseDetail.tsx`, `src/pages/Schedule.tsx` cancel dialog, tests.
- DEPENDENCIES: R4, R5, backend `GET /courses/{id}/sessions` `include_cancelled` support (owned by R4/R5 lanes).
- IMPLEMENTATION INTENT: cancelled list limited to 50 recent, refresh on cancel/restore; restore confirm modal explains re-preflight; non-admin hides restore/delete-hard actions.
- ACCEPTANCE CRITERIA:
  1. Cancel modal sends reason; cancelled row appears in the Cancelled section with reason/actor/time.
  2. Admin restore succeeds and the row returns to the active schedule; non-admin sees no restore button.
  3. Restore conflict → conflict details displayed, row stays cancelled.
- UNHAPPY PATHS: cancelled query 404 (course deleted); restore stale_edit.
- TESTS: vitest journeys; backend integration for include_cancelled.
- REAL VERIFICATION: vitest + go tests + manual dev-server check.
- REQUIRES_DETAILED_PLAN: false.

---

#### U3 — Series operations from CourseDetail (parity with Schedule)
- OBJECTIVE: CourseDetail rows gain the same series ops that Schedule has: This & Future (split), Future-only/Entire edit, and Cancel series, by extracting the Schedule modals into shared components.
- OWNED SURFACE: `src/features/scheduling/components/` (extract `EditSeriesModal` variants + `CancelSeriesModal` from `src/pages/Schedule.tsx`), `src/pages/CourseDetail.tsx` row menu, tests.
- DEPENDENCIES: none (UI refactor only).
- IMPLEMENTATION INTENT: move modal logic + payload building into shared components used by both pages; identical payloads/onSuccess refresh; row menu gains the series actions only for series-sessions.
- ACCEPTANCE CRITERIA:
  1. CourseDetail shows This&Future/Edit Series/Cancel Series for series-session rows (future-gated as in Schedule).
  2. Payload contracts identical to Schedule (asserted by shared tests).
  3. Schedule behavior unchanged (regression journeys pass).
- UNHAPPY PATHS: none new (existing series error handling reused).
- TESTS: vitest — port Schedule.seriesJourneys assertions to CourseDetail context + keep Schedule journeys green.
- REAL VERIFICATION: vitest schedule config + full suite.
- REQUIRES_DETAILED_PLAN: false.

---

#### U4 — Attendance/check-in on CourseDetail (replace dead "Check in" action)
- OBJECTIVE: The CourseDetail row-menu "Check in" placeholder (currently navigates to /schedule) opens the real attendance include/exclude modal (reuse Schedule's Attendance modal as shared component).
- OWNED SURFACE: extract guidance from `src/features/scheduling` attendance modal into shared component, `src/pages/CourseDetail.tsx`, tests.
- DEPENDENCIES: none.
- IMPLEMENTATION INTENT: extract `AttendanceModal` shared; wire CourseDetail rows; attendance overrides continue feeding preflight in editors (existing behavior).
- ACCEPTANCE CRITERIA:
  1. "Check in" opens the attendance modal for that session; include/exclude + W-code add work; busy-range/preflight reacts.
  2. Schedule unchanged (shared component regression).
- UNHAPPY PATHS: attendance load failure toast (existing pattern).
- TESTS: vitest.
- REAL VERIFICATION: vitest.
- REQUIRES_DETAILED_PLAN: false.

---

#### U5 — Bulk cancel selected sessions (CourseDetail)
- OBJECTIVE: The CourseDetail bulk modal gains "Cancel selected" mode: pick reason, confirm count, call R7, show per-row results (cancelled / stale_edit / error), refresh schedule.
- OWNED SURFACE: `src/pages/CourseDetail.tsx` BulkEditModal extension, api client bulk-cancel, vitest.
- DEPENDENCIES: R7.
- IMPLEMENTATION INTENT: reuse the existing bulk selection (max 100) and result-tab display; success rows removed from the table (they are cancelled → moved to Cancelled section per U2); per-row failures surfaced.
- ACCEPTANCE CRITERIA:
  1. Cancel mode sends `{id, expected_version, reason}` per selected row; results rendered per row.
  2. Cancelled rows vanish from active table and appear in Cancelled section.
  3. Stale rows reported, selection retained for retry.
- UNHAPPY PATHS: batch 400 too_many; full failure leaves table unchanged.
- TESTS: vitest (bulk cancel journey).
- REAL VERIFICATION: vitest.
- REQUIRES_DETAILED_PLAN: false.

---

#### U6 — Subject management UX (edit + protected delete + course-create info)
- OBJECTIVE: Subjects page gains edit (name/code) with 409 duplicate handling; delete of an in-use subject shows a clear inline error (subject_in_use with course count); CourseCreate shows the derived-code preview and makes subject selection required.
- OWNED SURFACE: `src/pages/Subjects.tsx`, `src/pages/SubjectCreate.tsx` (derived-code note), `src/pages/CourseCreate.tsx` (required subject + derived code preview), `src/features/courses/api` (subject update), tests.
- DEPENDENCIES: R1, R2.
- IMPLEMENTATION INTENT: minimal dialogs consistent with existing form grammar; error mapping for the new 409s; derived code preview fetches nothing — displays the server-assigned code from the create response (no new endpoint).
- ACCEPTANCE CRITERIA:
  1. Edit subject saves name; duplicate code → inline 409 message.
  2. Deleting an in-use subject shows subject_in_use message with referencing course count; nothing deleted.
  3. CourseCreate enforces subject selection and explains auto-generated code.
- UNHAPPY PATHS: subject deleted concurrently → 404 state refresh.
- TESTS: vitest (Subjects edit/delete journeys, CourseCreate validation).
- REAL VERIFICATION: vitest.
- REQUIRES_DETAILED_PLAN: false.

---

#### U7 — Archive/unarchive UX + safe batch ops (Courses/CourseDetail)
- OBJECTIVE: CourseDetail header gains Archive (with future-session block message) / Unarchive for non-legacy-archived courses; Courses page "Delete Selected" becomes "Archive Selected" (staff) and true batch-delete is admin-only; archived banner and archived bucket list show archived_at; scheduling actions disabled on archived courses with a clear notice.
- OWNED SURFACE: `src/pages/Courses.tsx`, `src/pages/CourseDetail.tsx`, `src/features/courses/api` (archive/unarchive/archived_at in Course), `src/components/ArchivedCourseBanner.tsx` (new), tests.
- DEPENDENCIES: R3.
- IMPLEMENTATION INTENT: reuse ConfirmModal; archived courses keep rendering but schedule sections read-only with notice; batch archive calls archive per row (or new batch-archive endpoint if R3 review deems it cheap — default: loop archive calls with results).
- ACCEPTANCE CRITERIA:
  1. Archive blocked with count message when future sessions exist; succeeds otherwise (banner + unarchive available).
  2. Archived course: schedule add/edit/create-series actions disabled with notice.
  3. Courses page: staff see "Archive Selected"; admins additionally see "Delete Selected".
- UNHAPPY PATHS: stale version on archive; mixed batch results.
- TESTS: vitest (archive journeys on both pages).
- REAL VERIFICATION: vitest.
- REQUIRES_DETAILED_PLAN: false.

### Verification & delivery

#### D1 — Backend test-first discipline + full Go verification
- Every backend item lands RED (failing test) → minimal implementation → GREEN → refactor. Full runs: `go test ./...` (unit) and targeted integration packages against the local TEST_DATABASE_URL (already live). New migrations validated via `bash ../scripts/validate-migrations.sh` and goose up/down on the local DB. gofmt + golangci-lint.

#### D2 — Frontend vitest verification
- New tests for every new component/page; `vitest.schedule.config.ts` 100%-threshold files stay green (any touched covered file gets compensating tests); full suite via `npm test` (root config) and schedule config run.

#### D3 — e2e contribution (best-effort, honest)
- Add Playwright specs for the two highest-value journeys (cancel-with-reason+restore; reschedule modal) in `e2e/`. Execution depends on local server+browser availability; if the environment cannot run them, this is reported explicitly as NOT executed (never faked).

#### D4 — Sweep, docs, and goal check
- Update `docs/` course-schedule notes (if a doc covers these flows), final sweep across prompt→roadmap→diff→tests, adversarial goal check. Report DONE/NOT-DONE with evidence only.

## 4. Dependency graph

```
R1 ─┐        R4 ─┬─ R5 ─┬─ U2
R2 ─┤            └─ R7 ─┴─ U5
R3 ─┴─ U7        R6 ────── U1
R8 ─────────────────────── U1 (capacity status)
U3, U4, U6: independent UI lanes (no backend deps beyond existing API)
D1/D2/D3 run per-lane; D4 is final.
```

## 5. Explicitly out of scope

- New scheduling engine / calendar-sync / multi-instance replication; changing the legacy sync pipeline's data model; rooms CRUD overhaul; absences/self-service features; dark theme or design-system overhaul; offline mode; drag-and-drop calendar; periods/terms (closed-period immutability for financial periods is N/A — institute has no accounting periods; session atomicity + audit covers the principle).
- Teacher/room availability editing UX (exists as Availability page; untouched).
- Rewriting existing passing journeys (regression-only).

## 6. Verification environment (confirmed)

- Local Postgres 16 running at `postgresql://rd-cream@127.0.0.1:5432/warwick_local_fresh`; `TEST_DATABASE_URL` set; integration tests use it (goose migrations applied per test run harness).
- Go toolchain + node_modules present; `npm test` (vitest) and `npx vitest run --config vitest.schedule.config.ts` available; Playwright config present.
- Commands for gates: `go build ./...`, `go vet ./...`, `go test ./...`, targeted `-run` integration packages, `bash scripts/validate-migrations.sh`, vitest runs, optional dev-server curl checks (`make dev`).