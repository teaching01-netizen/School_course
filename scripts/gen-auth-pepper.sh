#!/usr/bin/env bash
set -euo pipefail

# Generates a strong random value suitable for AUTH_PEPPER.
# Usage:
#   bash scripts/gen-auth-pepper.sh                 # prints a value
#   bash scripts/gen-auth-pepper.sh --export        # prints: export AUTH_PEPPER=...
#   bash scripts/gen-auth-pepper.sh --set-env       # sets/updates AUTH_PEPPER in .env

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mode="${1:-}"

gen() {
  # 32 bytes -> 43-ish chars base64url; good entropy and shell-friendly.
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 48 | tr -d '\n' | tr '+/' '-_' | tr -d '='
  else
    # macOS should have python3; fallback if openssl is unavailable.
    python3 - <<'PY'
import secrets, base64
raw = secrets.token_bytes(32)
print(base64.urlsafe_b64encode(raw).decode().rstrip("="))
PY
  fi
}

pepper="$(gen)"

case "$mode" in
  --export)
    printf 'export AUTH_PEPPER=%s\n' "$pepper"
    ;;
  --set-env)
    env_file=".env"
    touch "$env_file"
    if rg -q '^AUTH_PEPPER=' "$env_file" 2>/dev/null; then
      # BSD sed in-place requires a backup suffix.
      sed -i.bak -E "s/^AUTH_PEPPER=.*/AUTH_PEPPER=${pepper}/" "$env_file"
      rm -f "${env_file}.bak"
    else
      printf '\nAUTH_PEPPER=%s\n' "$pepper" >>"$env_file"
    fi
    echo "[gen-auth-pepper] updated .env"
    echo "[gen-auth-pepper] AUTH_PEPPER=${pepper}"
    ;;
  ""|--print)
    echo "$pepper"
    ;;
  *)
    echo "unknown arg: $mode" >&2
    echo "usage: bash scripts/gen-auth-pepper.sh [--print|--export|--set-env]" >&2
    exit 2
    ;;
esac

