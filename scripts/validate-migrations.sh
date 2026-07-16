#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# validate-migrations.sh
#
# Enforces goose migration conventions:
#   1. Every .sql file must have -- +goose Up
#   2. Every .sql file must have -- +goose Down
#   3. All CREATE TABLE must use IF NOT EXISTS
#   4. All CREATE INDEX must use IF NOT EXISTS
#
# Usage: bash scripts/validate-migrations.sh [migrations-dir]
#        Default migrations dir: backend/db/migrations
# ---------------------------------------------------------------------------
set -euo pipefail

MIGRATIONS_DIR="${1:-backend/db/migrations}"
cd "$(dirname "$0")/.."

errors=0
warnings=0

# ensure the directory exists
if [[ ! -d "$MIGRATIONS_DIR" ]]; then
  echo "ERROR: migrations directory not found: $MIGRATIONS_DIR"
  exit 1
fi

shopt -s nullglob
files=("$MIGRATIONS_DIR"/*.sql)
shopt -u nullglob

if [[ ${#files[@]} -eq 0 ]]; then
  echo "ERROR: no .sql files found in $MIGRATIONS_DIR"
  exit 1
fi

echo "=== Checking ${#files[@]} migration files in $MIGRATIONS_DIR ==="

for f in "${files[@]}"; do
  name="$(basename "$f")"

  # --- Check 1: must contain -- +goose Up ---
  if ! grep -q '^-- +goose Up' "$f"; then
    echo "FAIL [goose-up]  $name  — missing '-- +goose Up' annotation"
    ((errors++))
  fi

  # --- Check 2: must contain -- +goose Down ---
  if ! grep -q '^-- +goose Down' "$f"; then
    echo "FAIL [goose-down] $name  — missing '-- +goose Down' annotation"
    ((errors++))
  fi

  # --- Check 3: CREATE TABLE must use IF NOT EXISTS ---
  # Match lines with CREATE TABLE but not CREATE TABLE IF NOT EXISTS
  # Skip lines that are commented out
  while IFS= read -r line; do
    lineno="$(echo "$line" | cut -d: -f1)"
    content="$(echo "$line" | cut -d: -f2-)"
    # Skip comments and annotations
    if [[ "$content" =~ ^[[:space:]]*-- ]]; then
      continue
    fi
    echo "FAIL [bare-create] $name:$lineno  — bare CREATE TABLE; add IF NOT EXISTS"
    ((errors++))
  done < <(grep -n 'CREATE TABLE ' "$f" | grep -v 'CREATE TABLE IF NOT EXISTS' || true)

  # --- Check 4: CREATE INDEX must use IF NOT EXISTS ---
  while IFS= read -r line; do
    lineno="$(echo "$line" | cut -d: -f1)"
    content="$(echo "$line" | cut -d: -f2-)"
    if [[ "$content" =~ ^[[:space:]]*-- ]]; then
      continue
    fi
    echo "FAIL [bare-index]  $name:$lineno  — bare CREATE INDEX; add IF NOT EXISTS"
    ((errors++))
  done < <(grep -n 'CREATE INDEX ' "$f" | grep -Ev 'CREATE INDEX (CONCURRENTLY )?IF NOT EXISTS' || true)
done

echo ""
if [[ "$errors" -gt 0 ]]; then
  echo "FAILED — $errors error(s) found"
  exit 1
else
  echo "PASSED — all clean"
fi
