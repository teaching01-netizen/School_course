# Migration & Schema Governance

Source of truth: `backend/db/migrations/` (goose, PostgreSQL). All changes land through numbered `NNNNN_name.sql` files with `-- +goose Up` / `-- +goose Down` annotations.

## Numbering rule

Migrations are contiguous `00001`..`max` with no gaps. `scripts/validate-migrations.sh` enforces this (check 5) and fails the build when a number is missing.

- Never reuse or skip a number. If a number was never allocated, preserve contiguity with a tombstone file (see below).
- Never rename a file after it has shipped to `main`; renames rewrite history for every downstream branch.

## Provenance — resolved history anomalies

### 00053 -> 00055 rename (R100 at 39a9a37)

`git log --follow --diff-filter=R -- backend/db/migrations` shows a single rename in the entire history:

```
R100  backend/db/migrations/00053_cross_study_destination_only_review.sql
   -> backend/db/migrations/00055_cross_study_destination_only_review.sql
commit 39a9a37220d0e9f38b5fee9842a0c2285e664e7c  "op"  (2026-06-14)
```

The file was introduced as `00053` and moved to `00055` in the same sweep that introduced `00054_email_delivery_status.sql`. No content change — only the filename. Current canonical name is `00055_cross_study_destination_only_review.sql`.

### 00057–00060: never-allocated gap

`00057` through `00060` never existed on any branch or commit. Verified by:

```bash
ls backend/db/migrations | sort          # jumps 00056 -> 00061
git log --follow --diff-filter=R -- backend/db/migrations   # only rename is 00053->00055
python3 -c "import os,re; ..."
# gaps == [57, 58, 59, 60], max == 99
```

Lane C1 sealed the gap with tombstone no-ops:

- `00057_tombstone_gap_placeholder.sql`
- `00058_tombstone_gap_placeholder.sql`
- `00059_tombstone_gap_placeholder.sql`
- `00060_tombstone_gap_placeholder.sql`

Each file is `+goose Up` / `+goose Down` with explanatory comments and no DDL. They exist only to make the chain contiguous; `validate-migrations.sh` will fail if they are removed or if any future gap appears.

### 00049 in-file re-adds

`00049_cross_study.sql` ("cross_study") introduced `crm_cross_study_assignments` and related columns. Two commits touch the same file without renaming it:

- `916ee459  crosscourse` (2026-06-13): initial `00049_cross_study.sql` — creates `crm_cross_study_assignments` with `assigned_course_enrollment_created` / `source_course_enrollment_removed` plus `course_roster_overrides` wiring.
- `91a67c80  fixemailandcrosssec` (2026-06-14): same file, in-place re-add of `dest_course_a_enrollment_created` and `dest_course_b_enrollment_created` both inside the `CREATE TABLE` and as `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` after it.

No file rename, no gap — the table definition grew in place. Both amendments are idempotent (`IF NOT EXISTS`). Provenance:

```bash
git log --follow --oneline -- backend/db/migrations/00049_cross_study.sql
# 91a67c8 fixemailandcrosssec
# 916ee45 crosscourse
git log -p --follow -- backend/db/migrations/00049_cross_study.sql
```

## Lint policy — `scripts/validate-migrations.sh`

Run: `bash scripts/validate-migrations.sh [migrations-dir] [--strict]` (env `VALIDATE_STRICT=1` also enables strict mode). Default is warning-only for the two governance checks so existing backfill migrations keep passing; `--strict` promotes warnings to errors for CI gating of new migrations.

| Check | ID | Mode |
|---|---|---|
| goose Up/Down present | `goose-up`/`goose-down` | error |
| CREATE TABLE uses IF NOT EXISTS | `bare-create` | error |
| CREATE INDEX uses IF NOT EXISTS | `bare-index` | error |
| Numbering contiguous 00001..max | `sequence` | error |
| Business-data INSERT outside seed-config | `data-insert` | warn (default) / error (--strict) |
| ROADMAP-removal DROP COLUMN without proof comment | `drop-proof` | warn (default) / error (--strict) |

### Business-data INSERT rule (check 6)

- Migrations are for DDL and seed-config only. Data backfills belong in `internal/backfill/` or a reversible data migration with explicit approval.
- Seed-config allowlist (INSERTs that never trigger a warning): `app_settings`, `subjects`, `sit_in_rules`, `email_templates`, `email_workflows`, `crm_state`, `legacy_sync_controls`, `crm_snapshots`.
- Any other `INSERT INTO` is flagged. To silence the flag intentionally, add a marker comment in the file:
  - `-- lint:allow-data: <reason>` or `-- seed-config` (file-level escape hatch; any occurrence silences check 6 for that file).
  - Prefer moving bulk business-data inserts out of migrations entirely.
- Existing migrations that insert into business tables (e.g., `00061`, `00075`, `00087`, `00092`, `00093`, `00096`, `00079` via `course_teachers`/`course_students`/`session_changes`/outbox/busy-ranges) are grandfathered — they pass in default mode and produce `WARN` lines so reviewers see them; new migrations should avoid this pattern or carry the marker.
- Use `--strict` in CI for newly introduced migrations that should not carry business inserts without sign-off.

### ROADMAP-removal DROP COLUMN rule (check 7)

- Triggered only when a file contains both the string `ROADMAP` and `DROP COLUMN`. This is the narrow guard for lane B1/B-removal migrations (those that explicitly reference the remediation roadmap).
- If triggered, the file must contain a proof comment: `-- proof:` or `-- lint:allow-drop:` explaining the grep evidence and live row-count that proves zero readers. Warn-only by default; `--strict` makes it an error.
- Regular (non-roadmap) `DROP COLUMN` migrations are not flagged by this check.

## Operational notes

- New migrations: copy the latest number + 1 (zero-padded to 5 digits: `00100_...`). Do not leave holes.
- Tombstones must never be edited to carry DDL — allocate a new number instead.
- Before dropping a column or table: prove zero readers with `grep -r` across `backend/**`, `src/**`, e2e, and API DTOs plus a live row-count check. Record the proof in the migration file and (if it is a ROADMAP removal) ensure `-- proof:` is present so the linter passes in strict mode.

## Lane C coverage

- C1 (this document + tombstones + linter) — done.
- C2 (phantom-history trust repair) and C3 (backfill split-brain policy) are separate lanes in `.zcode/plans/data-model-audit-roadmap.md`.

## Phantom-history / trust repair (C2)

The following migrations carry trust-repair annotations (added in lane C2) because they repair or reference schema states that never existed in this chain:

| Migration | Issue | Repair |
|---|---|---|
| 00018 | Tombstone: no DDL, kept for contiguity | Header clarifies no-op, points to GOVERNANCE.md |
| 00020 | Down added `text NOT NULL` / `uuid NOT NULL REFERENCES` on populated tables → would fail | Down now adds nullable columns with explanatory comment; restore NOT NULL manually if rollback needed |
| 00025 | `otp_code_hash` / `pending_otp` cleanup references pre-chain prod states | Narrow annotation: OTP cleanup is prod-drift; `parent_phone`, `student_parent_verification_sessions`, `http_rate_limit_events`, `sms_circuit_breaker_state` are canonical |
| 00039 | Primary repair for `sat_verbal_policy_mappings` (table from 00038) | Annotated as primary; 00040 is duplicate-retry |
| 00040 | Near-verbatim duplicate of 00039 | Annotated as duplicate-retry for partial-apply environments |
| 00041 | Relax legacy denormalized columns (`subject_id`, `root_course_group_id`, etc.) | Annotated as prod-drift repair; columns never created by this chain |
| 00042 | Relax legacy `policy` column | Annotated as prod-drift repair |
| 00061 | Down is intentional no-op (behavioral fix) | Annotated with advisory note |
| 00079 | `course_teachers.is_primary` backfill | Quiesce advisory at file top; `NO TRANSACTION` with `CONCURRENTLY` |
| 00099 | Untracked until C2 (`00099_legacy_sync_monitor_opt.sql`) | Now tracked; `NO TRANSACTION` + `CONCURRENTLY IF NOT EXISTS` indexes |
| 1784592000000 | Stray empty file at repo root | Deleted in C2 |

Reviewer check: `grep -n "C2 trust repair" backend/db/migrations/*.sql` should list the annotated files above.
