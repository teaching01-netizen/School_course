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
