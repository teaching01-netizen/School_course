# Data Model & Database Structure — Remediation Roadmap

Repo: warwick-institute-ux-documentation copy 2 (Go backend, sqlc + goose, PostgreSQL)
Audit date: 2026-08-20. Evidence: live-DB introspection (65 tables), full migration history forensics (00001–00099, git-verified), query-layer audit.

## Scoring note

No P0 (data-loss/security) findings. The transactional core (scheduling, OTP, outbox claims, idempotency) is well-built. Risk concentrates in: migration-history trust, dead/dual-source schema, unindexed hot columns, legacy-sync data lifecycle.

---

## LANE A — Integrity & concurrency hardening (P1/P2)

### A1. Legacy-sync conflict dedupe backstop
- OBJECTIVE: `legacy_sync_conflicts` must be single-row per (entity_type, external_id, conflict_type) while open.
- SURFACE: migration 00100, `backend/db/queries/legacy_sync.sql` (`ConflictInsert`), generated `legacy_sync.sql.go`.
- INTENT: add partial unique index `(entity_type, external_id, conflict_type) WHERE status='open'`; rewrite `ConflictInsert` to `INSERT ... ON CONFLICT DO NOTHING` followed by a SELECT of the existing open row, returning its id (a bare DO NOTHING is not enough — `internal/legacysync/storage/migration_integration_test.go:290` asserts on the returned row).
- MECHANICS (per round-2 review): dedupe-UPDATE and `CREATE UNIQUE INDEX CONCURRENTLY` cannot share a transaction → goose `-- +goose NO TRANSACTION`, two sequential steps; live sync can mint new duplicates between dedupe and index completion → accept a retry window or quiesce writers; document which duplicate survives (oldest `created_at` wins).
- ACCEPTANCE: two concurrent `ConflictInsert` calls for the same open conflict produce exactly one row; index exists in live schema; `go test ./internal/legacysync/...` green (incl. the storage migration integration test asserting the returned row id).
- UNHAPPY: existing duplicate open conflicts in prod → migration must collapse them (dedupe UPDATE) before creating the index; index creation must be `CONCURRENTLY` with retry.
- TESTS: concurrent-insert integration test (2 goroutines, assert 1 row).
- REAL VERIFICATION: psql `\d legacy_sync_conflicts`; EXPLAIN on ConflictInsert; run test.
- REQUIRES_DETAILED_PLAN: false.

### A2. Hot-path missing indexes
- OBJECTIVE: all FK/join columns used by real queries have an index.
- SURFACE: migration 00101 (+00102 if concurrent) only. No ORM code change.
- INTENT: add indexes for (evidence: query-layer audit 3a–3f, FK audit 35 uncovered columns):
  - `course_students(student_id)` (P1 — hottest: per-student roster/absence/session queries)
  - `courses(teacher_id)`, `session_series(course_id)`, `session_series(room_id)`, `session_series(teacher_id)`
  - `audit_log(actor_user_id)`, `notification_outbox(absence_id)`, `notification_outbox(assignment_id)`
  - `session_changes(batch_id)`, `session_changes(changed_by)`, `session_change_batches(requested_by)`
  - `student_absences(reviewed_by)`, `student_absences(sit_in_overridden_by)`, `absence_sit_ins(assigned_by)`
  - `absence_schedule_issues(source_session_id)`, `(sit_in_session_id)`, `(missed_session_id)`, `(resolved_by)`, `(first_session_change_id)`
  - `course_roster_overrides(student_id)`, `course_roster_overrides(created_by_user_id)`, `(updated_by_user_id)`
  - `crm_cross_study_assignments(snapshot_id)`, `(dest_course_a_id)`, `(dest_course_b_id)`, `(assigned_course_id)`
  - `crm_pending_diffs(student_id)`, `student_parent_verification_sessions(consumed_absence_id)`, `email_workflows(template_id)`, `email_delivery_claims(workflow_id)`
  - `sessions(teacher_id, start_at)` partial non-deleted, `sessions(room_id, start_at)` partial non-deleted (availability queries)
  - `absence_sit_ins(snapshot_quality, assigned_at)` partial where snapshot_quality='unavailable' (backfill claim)
  - `absence_missed_sessions(session_id)` covered? verify; add `(absence_id, session_id)` already unique.
  - `session_change_impact_runs(session_change_id)` — PK, verify.
- ACCEPTANCE: automated FK-index coverage query returns 0 uncovered FK columns (excluding soft-reference tables by policy); EXPLAIN for `StudentCoursesList`, `SessionsByStudentInRange`, `ListUncoveredFutureSessionsForTeacher/Room`, `BackfillEligibleAssignments`, `LegacyJobCounts` shows index/bitmap-index path, no seq scan on leading predicate.
- REAL VERIFICATION: psql index list diff; EXPLAIN ANALYZE on the named queries against dev DB.
- REQUIRES_DETAILED_PLAN: false.

### A3. Session-change resolution race (validate-then-write)
- OBJECTIVE: resolution cannot record a stale `session_version_at_assignment` against a concurrently-edited session.
- SURFACE: `backend/internal/db/session_change_resolution_custom.go` (~295–330).
- INTENT: acquire `SELECT ... FOR UPDATE` on the candidate session (or use its row lock) before validation + delete/re-insert assignment; verify version unchanged at insert. NOTE: production already wraps resolution in `WithIdempotentTx` and `validateResolutionCandidate` locks the absence_sit_ins row — this lane is defensive hardening against the version-check window, not a live-crash fix (per round-2 blind verification).
- ACCEPTANCE: concurrent session-edit vs resolution: exactly one wins; stale version never persisted (integration test with two goroutines + barrier).
- REQUIRES_DETAILED_PLAN: true (locking strategy choice).

### A4. AbsenceSitInsReplace always-transactional
- OBJECTIVE: delete-all + inserts + audit events atomic on every code path.
- SURFACE: `backend/internal/db/absence_management_custom.go` (641–700, deprecated method).
- INTENT: remove the `beginner` type-assertion fallback so a missing transaction is a hard error; keep or retire the deprecated method per callers.
- ACCEPTANCE: method runs inside a transaction when called with a DB handle; unit test asserts error when handle lacks Begin.
- REQUIRES_DETAILED_PLAN: false.

---

## LANE B — Dead & contradictory schema removal (P2 maintainability)

### B1. Dead schema removal (columns/tables with zero readers — verified live)
- OBJECTIVE: remove schema that nothing reads or writes. NOTE (corrected after blind-verification): the `courses.legacy_status/legacy_expire_date/legacy_source_hash/legacy_last_seen_at` and `sessions.legacy_confirmed/legacy_confirmed_by/legacy_source_hash/legacy_last_synced_at/legacy_last_seen_at` columns are **live legacy-sync domain columns** — written by `internal/legacysync/apply/course.go:118,346`, `reconcile/full.go:190-191`, `apply/schedule.go:275`; read by `cmd/legacy-sync/main.go:51-86` and `httpapi/courseshttp/routes.go`. They MUST NOT be dropped. They are handled by B6 instead.
- SURFACE: migration 00103 (additive-first: never drop before proving zero readers).
- INTENT: drop, in order: (1) `course_cohorts` + `courses.cohort_id` (0 rows, zero code reference outside generated models); (2) `courses.crm_pinned_snapshot_id` (zero references anywhere). Proof required per column: no reference in `backend/**`, `src/**`, e2e, or API DTOs; plus live row-count check for the table.
- ACCEPTANCE: `grep -r` for `course_cohorts`, `cohort_id`, `crm_pinned_snapshot_id` (and Go casing variants) returns zero hits outside the migration itself; `go build ./...` + frontend `tsc` green; sqlc regenerate succeeds; live DB columns/tables absent.
- UNHAPPY: a hidden reader (e.g., raw SQL string built at runtime) — mitigation: grep for string fragments of column names, not only identifiers; the migration is reversible if a reader appears during rollout.
- REQUIRES_DETAILED_PLAN: true (data-affecting removal).

### B6. Reframe live legacy-sync columns as canonical domain fields (no drop)
- OBJECTIVE: naming/docs reflect reality: legacy sync is the primary ingestion path (100% of courses/sessions/series are `source_kind='legacy'`), so the `legacy_*` prefix misleads future maintainers into treating live domain state as dead.
- SURFACE: documentation + comments only in this phase: `backend/internal/legacysync/**` comments, `models.go` field comments, a new `docs/DATA_MODEL.md` section marking each `legacy_*` column as canonical with its writer/reader; no schema change.
- INTENT: (a) document which columns are canonical domain state vs. import metadata; (b) explicitly note in `GOVERNANCE.md` that renaming these columns (e.g. `legacy_status` → `status`) is deferred to a separately-approved rewrite migration because it touches ~40 query/DTO/frontend surfaces; (c) retire the "transient" wording in `studentauth/service.go` only for the email topic (B4) — leave legacy-sync wording accurate.
- ACCEPTANCE: a reviewer can determine each legacy-synced column's writer, reader, and canonical status from `docs/DATA_MODEL.md` without grepping; no column renamed or dropped by this lane.
- REQUIRES_DETAILED_PLAN: false.

### B2. Dead/broken query code deletion
- OBJECTIVE: delete queries that crash (reference dropped columns) or have zero callers.
- SURFACE: `backend/internal/db/absence_custom.go` (4 functions referencing `course_level`/`level_order` — dropped in migration 00017; verify V2 variants exist and are used), `backend/db/queries/crm_rows.sql` (`CrmRowsInsert` violates `snapshot_id NOT NULL`, `CrmRowsDeleteAll`, `CrmRowsCount` — zero callers), empty dir `backend/internal/db/queries/`.
- INTENT: delete the broken query definitions + their generated code; keep V2 implementations; delete empty dir.
- ACCEPTANCE: `go build ./...` green; `go vet`; grep-verified absence of `course_level`/`level_order`/`CrmRowsInsert` outside tests or migrations; sqlc regenerate diff clean.
- REQUIRES_DETAILED_PLAN: false.

### B3. http_rate_limit_events: add PK + retention
- OBJECTIVE: every table has a primary key and bounded growth.
- SURFACE: migration 00104 + rate-limit pruning path.
- INTENT: add `bigserial id` PK (append-only event log), add `created_at`-based retention job (same cadence as existing cleanup cmds), keep `(key, created_at DESC)` index.
- ACCEPTANCE: PK exists; `pg_constraint` shows PK; cleanup job deletes rows older than retention window, idempotent, restartable.
- REQUIRES_DETAILED_PLAN: false.

### B4. Student email consolidation (triple-source → canonical)
- OBJECTIVE: one authoritative student email path with provenance, not three columns OR'd together in code.
- SURFACE: migration 00105 (backfill `email_crm` from `email` where null; then drop `students.email`), `backend/db/queries/students.sql` (write `email_crm`/`email_system`), `internal/httpapi/absenceshttp/**` resolve path (incl. `absenceshttp/self_service_routes.go` OR-chain at :94), `studentshttp` DTO, `internal/studentauth` (fix the misleading "transient" comment), `internal/db/absence_management_custom.go` COALESCE readers.
- INTENT: (a) backfill `email_crm = email WHERE email_crm IS NULL`; (b) Admin/CRM/student write paths target `email_crm` (CRM-sourced) or `email_system` (manually entered) with provenance; (c) readers use a single resolve helper `COALESCE(NULLIF(email_crm,''), NULLIF(email_system,''))`; (d) drop `email` column; (e) update the studentauth comment.
- ACCEPTANCE: end-to-end: create student via Admin UI → email lands in `email_crm`/`email_system`; CRM import row with email → `email_crm`; absence submission resolves the address; `students.email` absent from schema; all Go/frontend references updated; `go test ./internal/absences/... ./internal/httpapi/absenceshttp/...` green.
- UNHAPPY: LIVE-DB VERIFIED (2026-08-20): 1768/1899 students have `email` set, 0 have `email_crm`/`email_system` — migration 00064's own backfill ran when emails were empty, and every later write path (students.sql upsert, CRM student sync) wrote only `email`. So the `email_crm = email` re-backfill is load-bearing AND idempotent; it must run before the column drop and be verified by row-count check. Note the audit evidence spans a dirty tree (51 uncommitted files incl. `db/queries/legacy_sync.sql`) — confirm the baseline against HEAD before the migration ships. OUT-OF-REPO CONSUMERS: before dropping `students.email`, confirm no external consumer (BI/warehouse/export scripts, the legacy site's own DB) reads the live column — coordinate with ops; if unverifiable, keep the column as a deprecated read-only copy for one release cycle (document in the migration) instead of dropping immediately.
- REQUIRES_DETAILED_PLAN: true (data-affecting, cross-layer).

### B5. Enum standardization (4 native enums → text+CHECK)
- OBJECTIVE: one enum mechanism across the schema. Native enums: `crm_job_type`, `crm_job_status` (`crm_jobs`), `override_action` (`course_roster_overrides.action`), `diff_action` (`crm_pending_diffs.diff_action`). All 60+ other status/type columns use text+CHECK.
- SURFACE: migration 00106 + regenerate sqlc models. CRITICAL: 8 runtime call sites cast to the enum types and break once types are dropped — `internal/crmimport/crossstudy/store.go:64,89,551,692` (`::override_action`), `internal/crmimport/queue/queue.go:168,183,287` (`::crm_job_status`, `::crm_job_type`), `internal/crmimport/reconcile/reconcile.go:810` (`::diff_action`). Each must drop its `::enum` cast (plain text param works against text columns).
- INTENT: convert the 4 columns to text with CHECK constraints preserving exact value sets and defaults; drop the enum types; remove the 8 Go casts in the same change set (they compile against the new text columns).
- ACCEPTANCE: `information_schema.columns` shows zero USER-DEFINED enum columns in app tables; values unchanged (row-count equality before/after); `grep -rn "::override_action|::crm_job_status|::crm_job_type|::diff_action" --include="*.go" backend/` returns zero non-test hits; `go test ./internal/crmimport/... ./internal/db/...` green.
- UNHAPPY: reconcile.go is also D2's surface — sequence B5's reconcile.go edit before/with D2's; enum values referenced in queue.Claim logic must map 1:1 to the CHECK lists.
- REQUIRES_DETAILED_PLAN: true (cross-package call-site rewrite + data conversion).

---

## LANE C — Migration-history hygiene (P2/P3 governance)

### C1. Numbering gap + governance lint
- OBJECTIVE: migration numbering is contiguous and trustworthy; future gaps impossible.
- SURFACE: new tombstone files `00057`–`00060` (comment-only no-ops with a note), `scripts/validate-migrations.sh` (add: sequence-contiguity check, forbid business-data INSERTs outside seeded-config section, forbid column drops inside a `ROADMAP-removal` without proof comment), `GOVERNANCE.md`.
- ACCEPTANCE: `bash scripts/validate-migrations.sh` passes and fails when a number is skipped; document 00053→00055 rename + 00049 in-file re-adds in GOVERNANCE.md.
- REQUIRES_DETAILED_PLAN: false.

### C2. Trust repair for phantom-history migrations
- OBJECTIVE: every migration's comments describe reality; no broken Down.
- SURFACE: `00018` (placeholder — delete or make real, per team decision), `00020` (Down adds NOT NULL columns to populated table — fix Down to add nullable + backfill or mark no-op), `00040`/`00041`/`00042` (annotate: "repairs prod-drift: columns these files alter were never created by this migration chain; kept for environment compatibility" — also fix the 00039/00040 duplicate-repair-body static test), `00025` (narrow annotation only: its OTP-column drops and `pending_otp` cleanup reference schema states never created in this chain; the rest of 00025 — parent_phone backfill, rate-limit tables, circuit breaker — is real and MUST NOT be relabeled phantom), `00061` (add advisory note; leave Down comment-only), `00099` (untracked in git — commit), stray root file `1784592000000` (delete), `00079` (document quiesce requirement at file top).
- ACCEPTANCE: reviewer reads each edited migration; comments state actual provenance; `00020` Down no longer fails on populated tables; repo has no untracked migration files; `1784592000000` removed.
- REQUIRES_DETAILED_PLAN: false.

### C3. Backfill split-brain policy
- OBJECTIVE: one home for future backfills.
- SURFACE: `CONTRIBUTING.md`/`MIGRATIONS.md` policy + `validate-migrations.sh` check.
- INTENT: policy: migrations = DDL + seed-config only; data backfills = `internal/backfill/` service or reversible data migration with explicit approval; lint flags non-seed INSERT/UPDATE/DELETE in migrations.
- ACCEPTANCE: policy doc exists; lint flags a crafted data-fix migration; existing exceptions listed in GOVERNANCE.md.
- REQUIRES_DETAILED_PLAN: false.

---

## LANE D — Performance & data lifecycle (P2/P3)

### D1. Sit-in suggestion N+1 → single query
- OBJECTIVE: suggestion endpoint issues O(1) heavy queries, not up to 50.
- SURFACE: `backend/internal/absences/sitinresolver/resolver.go` (169–201), `session_change_impact.sql` `SitInAssignmentFacts`.
- INTENT: batch all candidate checks in one SQL query (JOIN/EXISTS set-based), or one query per candidate type with `WITH`; remove per-candidate round trip.
- ACCEPTANCE: suggestion request executes ≤3 DB queries (instrumented test); behavior identical (fixture comparison); `go test ./internal/absences/sitinresolver/...` green.
- REQUIRES_DETAILED_PLAN: true (query redesign).

### D2. Bulk writes (CRM upload, roster reconcile, series materialization)
- OBJECTIVE: eliminate per-row round trips on bulk paths.
- SURFACE: `crmimport/import_service.go` (65–97, reuse `CopyFromRows` helper from snapshot_service), `crmimport/reconcile/reconcile.go` (223–258), `internal/series/service.go` (187–207, 444–486, 689–693), `legacysync/syncer.go` (107–140).
- INTENT: batch INSERTs with `pgx.CopyFromRows` (the `rowCopies`/`pgx.CopyFromRows` helper already in `crmimport/snapshot_service.go`) + `ON CONFLICT`, or single multi-VALUES statements with RETURNING. CAVEAT (per round-2 review): the `sessions` gist `EXCLUDE` constraints are not `ON CONFLICT` targets — CopyFrom+ON CONFLICT applies only to non-exclusion tables (crm_rows, student edits); series materialization must preserve the `SessionLockOverlappingForInsert` serialization (lock the overlap set once, then set-insert) rather than dropping the per-occurrence lock.
- ACCEPTANCE: no `N+1` loop INSERTs on these paths (code review + query-count test); integration tests green.
- REQUIRES_DETAILED_PLAN: true (series overlap semantics must be preserved).

### D3. Legacy-sync data lifecycle (retention & reconciliation)
- OBJECTIVE: bounded growth + actionable open conflicts.
- SURFACE: new `backend/cmd/legacy-sync-retention` (or extend legacy-sync cmd), `legacy_sync_outbox` (144k published rows), `legacy_sync_jobs` (21.6k completed/dead), `legacy_sync_conflicts` (10.6k open).
- INTENT: idempotent, restartable retention job: purge published outbox >30d, completed/dead jobs >30d, resolved/ignored conflicts >90d; keep open conflicts forever (operational queue) + a monitor query surfacing open-conflict age; document counts in LegacySyncHealth page (already exists).
- ACCEPTANCE: retention job dry-run + run: table sizes bounded post-run; second run deletes 0 rows (idempotent); restart mid-run resumes; tests green.
- REQUIRES_DETAILED_PLAN: true (data-affecting, ops window).

### D4. Absence status / enum drift prevention
- OBJECTIVE: single source of truth for vocabulary shared Go↔SQL↔frontend.
- SURFACE: `backend/internal/absences/status.go` (10–22, 70–78), migration 00068 CHECK (or the B5-generated text+CHECK replacement), frontend `types.ts`.
- INTENT: extract the status set to one Go constants file; add a test that reads the governing migration CHECK (00068, or whichever text+CHECK B5 produces) and asserts equality with Go + a generated TS type; eliminate string-built SQL IN-clauses in favor of parameterized arrays.
- ACCEPTANCE: drift test fails when either side changes; `go test ./internal/absences/...` green; no string-built IN clauses remain.
- REQUIRES_DETAILED_PLAN: false.

---

## Sequencing & dependencies

- S1: A1 (conflict dedupe index) and A2 (hot-path indexes) are both pure-additive migrations and can ship as one change set (they touch disjoint tables; no true coupling — combine for a single deploy window, not because of a dependency).
- S2: B2 (dead code deletion) before B1 (dead schema drop) — deleting queries first proves which columns are truly dead.
- S3: B4 (email) is independent of B1 but both touch `students` — sequence B1 → B4 to avoid concurrent edits to the same table.
- S4: B6 reframes (docs only) — can run anytime; its optional column-rename is explicitly deferred and NOT part of this roadmap.
- S5: B5's `reconcile.go` cast-removal must be sequenced with D2's reconcile.go bulk-write edit (same file) — do both in the same change set or B5 first.
- S6: D3 needs a deploy window + ops review; can run last.
- Lanes A/B/C independent and parallelizable after S1/S2.

## Explicitly out of scope (audit findings accepted for now)
- 00088/00073 design conflict (append-only triggers vs FK integrity) — requires product decision on absence-history retention; tracked as open design debt, not auto-changed.
- Full N+1 cleanup of every cold path (4e–4f) — covered by D2 bulk writes; remaining low-traffic loops documented.
- Frontend schema-facing type cleanup beyond D4.

## Verification infrastructure
- `make migrate-status` / `go run ./cmd/migrate up` against dev DB.
- `go test ./backend/...` (unit) + targeted integration tests per lane.
- Live-DB introspection queries (psql) for index coverage / enum inventory / dead-column grep.