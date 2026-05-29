#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ -f ".env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source ".env"
  set +a
fi

if [[ -n "${TEST_DATABASE_URL:-}" ]]; then
  export DATABASE_URL="$TEST_DATABASE_URL"
fi

exec bash -lc 'cd backend && go run ./cmd/admincheck'

