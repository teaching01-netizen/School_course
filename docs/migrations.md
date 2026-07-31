# Migration Conventions

This project uses [goose v3](https://github.com/pressly/goose) for SQL migrations.

## Required structure

Every migration file **must** contain both `-- +goose Up` and `-- +goose Down` annotations:

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS widgets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_widgets_name ON widgets(name);

-- +goose Down
DROP TABLE IF EXISTS widgets;
```

## Rules

| Rule | Why |
|---|---|
| Always `-- +goose Up` + `-- +goose Down` | Goose refuses to parse without annotations; Down makes migrations reversible. |
| `CREATE TABLE IF NOT EXISTS` | Idempotent — won't fail if migration is re-run. |
| `CREATE INDEX IF NOT EXISTS` | Same reason. |
| `DROP TABLE IF EXISTS` / `DROP INDEX IF EXISTS` in Down | Safe no-op if the object doesn't exist. |
| `ALTER TABLE ... DROP COLUMN IF EXISTS` in Down | Safe no-op for the same reason. |

## Online index rollout

Indexes on production tables must be deployed before application code that depends on their query plans. Use a separate non-transactional migration so PostgreSQL can build and remove each index without blocking normal writes:

```sql
-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS example_active_idx ON example(created_at) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS example_active_idx;
```

Run these migrations with a finite `lock_timeout` at the deployment layer. A timeout is a safe failed rollout: investigate the blocker and retry the migration before enabling the dependent application release. Do not fall back to a blocking index build on a live table.

## Validation

Run the check before opening a PR:

```bash
make migrate-validate       # from backend/
npm run migrate:validate    # from root
```

The validator checks all 4 rules and exits non-zero on any violation.

## Naming

- Prefix: `NNNNN_description.sql` (zero-padded, sequential)
- Use snake_case for the description
- Keep descriptions short — the SQL itself is the spec
