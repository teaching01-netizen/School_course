# Contributing — Migrations & Backfills

Source of truth: `backend/db/migrations/` (goose, PostgreSQL). See `GOVERNANCE.md` for numbering, lint policy, and provenance.

## Migration & backfill policy

**Migrations are DDL + seed-config only.** Seed-config allowlist: `app_settings`, `subjects`, `sit_in_rules`, `email_templates`, `email_workflows`, `crm_state`, `legacy_sync_controls`, `crm_snapshots`.

**Business-data backfills belong in `backend/internal/backfill/`** as idempotent, restartable jobs (or a dedicated `backend/cmd/*` command). They must be safe to re-run and to resume after a crash.

**Reversible data migrations are the exception, not the rule.** If a one-off data fix must live in a migration, it requires explicit reviewer approval, a `GOVERNANCE.md` entry, and a `-- lint:allow-data: <reason>` marker in the file so `scripts/validate-migrations.sh` check 6 does not flag it.

**Lint:** `bash scripts/validate-migrations.sh` (check 6: `data-insert`, WARN by default, FAIL with `--strict`/`VALIDATE_STRICT=1`). Any non-seed `INSERT INTO` without `-- lint:allow-data` or `-- seed-config` is flagged. Prefer moving the data out of migrations.

## Workflow

1. Number migrations contiguously (`NNNNN_name.sql`, zero-padded to 5 digits). Never skip or reuse a number.
2. Every file must have `-- +goose Up` and `-- +goose Down`; `CREATE TABLE/INDEX` must use `IF NOT EXISTS`.
3. Run `bash scripts/validate-migrations.sh --strict` before opening a PR.
4. For backfills, add the job under `backend/internal/backfill/` and wire it to a `cmd` entry point with dry-run support.

## References

- `GOVERNANCE.md` — lint table, provenance, lane coverage.
- `scripts/validate-migrations.sh` — all 7 checks.
- `backend/internal/backfill/` — existing backfill jobs.
