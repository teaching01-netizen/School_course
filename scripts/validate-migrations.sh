#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# validate-migrations.sh
#
# Enforces goose migration conventions:
#   1. Every .sql file must have -- +goose Up
#   2. Every .sql file must have -- +goose Down
#   3. All CREATE TABLE must use IF NOT EXISTS
#   4. All CREATE INDEX must use IF NOT EXISTS
#   5. Migration numbering must be contiguous 00001..max with no gaps
#   6. Business-data INSERT lint (warning by default; --strict promotes to error)
#      Note: UPDATE/DELETE business backfills are covered by review (see GOVERNANCE.md C3).
#   7. ROADMAP-removal DROP COLUMN must carry proof comment (warning/strict)
#
# Usage: bash scripts/validate-migrations.sh [migrations-dir] [--strict]
#        Default migrations dir: backend/db/migrations
#        --strict: promote lint warnings (checks 6,7) to errors
#        Env VALIDATE_STRICT=1 also enables strict mode.
# Portable to macOS bash 3.2 (no associative arrays).
# ---------------------------------------------------------------------------
set -euo pipefail

MIGRATIONS_DIR="backend/db/migrations"
STRICT=0
if [[ "${VALIDATE_STRICT:-0}" == "1" ]]; then
  STRICT=1
fi
for arg in "$@"; do
  case "$arg" in
    --strict) STRICT=1 ;;
    --) ;;
    -*) echo "Unknown flag: $arg" >&2; exit 1 ;;
    *) MIGRATIONS_DIR="$arg" ;;
  esac
done
cd "$(dirname "$0")/.." || exit 1

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
    errors=$((errors+1))
  fi

  # --- Check 2: must contain -- +goose Down ---
  if ! grep -q '^-- +goose Down' "$f"; then
    echo "FAIL [goose-down] $name  — missing '-- +goose Down' annotation"
    errors=$((errors+1))
  fi

  # --- Check 3: CREATE TABLE must use IF NOT EXISTS ---
  while IFS= read -r line; do
    lineno="$(echo "$line" | cut -d: -f1)"
    content="$(echo "$line" | cut -d: -f2-)"
    if [[ "$content" =~ ^[[:space:]]*-- ]]; then
      continue
    fi
    echo "FAIL [bare-create] $name:$lineno  — bare CREATE TABLE; add IF NOT EXISTS"
    errors=$((errors+1))
  done < <(grep -n 'CREATE TABLE ' "$f" | grep -v 'CREATE TABLE IF NOT EXISTS' || true)

  # --- Check 4: CREATE INDEX must use IF NOT EXISTS ---
  while IFS= read -r line; do
    lineno="$(echo "$line" | cut -d: -f1)"
    content="$(echo "$line" | cut -d: -f2-)"
    if [[ "$content" =~ ^[[:space:]]*-- ]]; then
      continue
    fi
    echo "FAIL [bare-index]  $name:$lineno  — bare CREATE INDEX; add IF NOT EXISTS"
    errors=$((errors+1))
  done < <(grep -n 'CREATE INDEX ' "$f" | grep -Ev 'CREATE INDEX (CONCURRENTLY )?IF NOT EXISTS' || true)
done

# --- Check 5: sequence-contiguity — filenames must be 00001..max with no gaps ---
seen_nums=""
max_num=0
for f in "${files[@]}"; do
  base="$(basename "$f")"
  if [[ "$base" =~ ^0*([0-9]+)_ ]]; then
    raw="${BASH_REMATCH[1]}"
    num=$((10#$raw))
    if [[ -z "$seen_nums" ]]; then
      seen_nums="$num"
    else
      seen_nums="$seen_nums $num"
    fi
    if (( num > max_num )); then max_num=$num; fi
  else
    echo "FAIL [sequence]  $(basename "$f") — filename does not start with NNNNN_ prefix"
    errors=$((errors+1))
  fi
done

# Build sorted unique list for gap detection using portable tools
sorted_seen="$(printf '%s\n' $seen_nums | sort -n -u)"
missing=""
missing_count=0
for (( i=1; i<=max_num; i++ )); do
  if ! printf '%s\n' "$sorted_seen" | grep -qx "$i"; then
    if [[ -z "$missing" ]]; then
      missing="$(printf "%05d" "$i")"
    else
      missing="$missing $(printf "%05d" "$i")"
    fi
    missing_count=$((missing_count+1))
  fi
done

if (( missing_count > 0 )); then
  printf "FAIL [sequence]  gap detected — missing migration number(s): %s" "$missing"
  echo "  (expected 00001..$(printf "%05d" "$max_num") contiguous; found ${#files[@]} files, max $(printf "%05d" "$max_num"))"
  echo "       hint: create tombstone no-op files for never-allocated numbers; see GOVERNANCE.md"
  errors=$((errors+1))
else
  echo "OK   [sequence]  contiguous 00001..$(printf "%05d" "$max_num") (${#files[@]} files)"
fi

# --- Check 6: business-data INSERT lint ---
# Allowlist: seed/config tables whose INSERTs are expected in migrations.
SEED_CONFIG_TABLES="app_settings subjects sit_in_rules email_templates email_workflows crm_state legacy_sync_controls crm_snapshots"

is_seed_config() {
  local t="$1"
  for a in $SEED_CONFIG_TABLES; do
    if [[ "$t" == "$a" ]]; then
      return 0
    fi
  done
  return 1
}

for f in "${files[@]}"; do
  name="$(basename "$f")"
  if grep -qiE 'lint:allow-data|seed-config' "$f"; then
    continue
  fi
  while IFS= read -r line; do
    lineno="$(echo "$line" | cut -d: -f1)"
    content="$(echo "$line" | cut -d: -f2-)"
    if [[ "$content" =~ ^[[:space:]]*-- ]]; then
      continue
    fi
    tbl="$(echo "$content" | sed -nE 's/.*INSERT INTO[[:space:]]+"?([a-zA-Z_][a-zA-Z0-9_\.]*)"?.*/\1/p' | head -n1)"
    tbl="${tbl##*.}"
    tbl="$(echo "$tbl" | tr '[:upper:]' '[:lower:]')"
    if [[ -z "$tbl" ]]; then
      tbl="<unknown>"
    fi
    if ! is_seed_config "$tbl"; then
      if [[ "$STRICT" -eq 1 ]]; then
        echo "FAIL [data-insert] $name:$lineno  — INSERT INTO $tbl is business data; add '-- lint:allow-data' marker or move backfill to internal/backfill/"
        errors=$((errors+1))
      else
        echo "WARN [data-insert] $name:$lineno  — INSERT INTO $tbl looks like business data (not in seed-config allowlist); add '-- lint:allow-data' to silence or use --strict to enforce"
        warnings=$((warnings+1))
      fi
    fi
  done < <(grep -n -i 'INSERT INTO' "$f" | grep -vE '^[^:]*:[^:]*:.*--.*(lint:allow-data|seed-config)' || true)
done

# --- Check 7: ROADMAP-removal DROP COLUMN proof lint ---
for f in "${files[@]}"; do
  name="$(basename "$f")"
  if grep -qi 'ROADMAP' "$f" && grep -qi 'DROP COLUMN' "$f"; then
    if ! grep -qiE 'proof:|lint:allow-drop' "$f"; then
      if [[ "$STRICT" -eq 1 ]]; then
        echo "FAIL [drop-proof] $name  — DROP COLUMN inside ROADMAP-removal without proof comment; add '-- proof: <grep evidence + row-count>' or '-- lint:allow-drop: <reason>'"
        errors=$((errors+1))
      else
        echo "WARN [drop-proof] $name  — DROP COLUMN inside ROADMAP-removal without proof comment; add '-- proof:' marker (warn-only; --strict to enforce)"
        warnings=$((warnings+1))
      fi
    fi
  fi
done

echo ""
if [[ "$errors" -gt 0 ]]; then
  echo "FAILED — $errors error(s), $warnings warning(s)"
  exit 1
else
  if [[ "$warnings" -gt 0 ]]; then
    echo "PASSED — all clean ($warnings warning(s))"
  else
    echo "PASSED — all clean"
  fi
fi
