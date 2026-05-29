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
  echo "[dev] using TEST_DATABASE_URL for DATABASE_URL"
fi

ADDR="${ADDR:-:8080}"
backend_port=""
if [[ "$ADDR" == :* ]]; then
  backend_port="${ADDR#:}"
else
  backend_port="${ADDR##*:}"
fi

if command -v lsof >/dev/null 2>&1; then
  existing_listener="$(lsof -nP -iTCP:"$backend_port" -sTCP:LISTEN 2>/dev/null | awk 'NR==2 {print $1 " (pid=" $2 ")"}' || true)"
  if [[ -n "$existing_listener" ]]; then
    echo "[dev] port $backend_port is already in use by $existing_listener"
    echo "[dev] stop that process or run with a different port, e.g. ADDR=:8081 npm run dev:full"
    exit 1
  fi
fi

if command -v docker >/dev/null 2>&1; then
  if [[ "${USE_DOCKER_DB:-}" == "1" ]]; then
    echo "[dev] starting postgres (docker compose)..."
    docker compose up -d db
  else
    echo "[dev] docker found; skipping docker db (set USE_DOCKER_DB=1 to enable)"
  fi
else
  echo "[dev] docker not found; skipping docker db startup"
fi

echo "[dev] running migrations..."
(cd backend && make migrate-up)

backend_pid=""
frontend_pid=""

cleanup() {
  set +e
  if [[ -n "${frontend_pid:-}" ]] && kill -0 "$frontend_pid" 2>/dev/null; then
    echo "[dev] stopping spa (pid=$frontend_pid)..."
    kill -INT "$frontend_pid" 2>/dev/null || true
  fi
  if [[ -n "${backend_pid:-}" ]] && kill -0 "$backend_pid" 2>/dev/null; then
    echo "[dev] stopping api (pid=$backend_pid)..."
    kill -INT "$backend_pid" 2>/dev/null || true
  fi
  if [[ -n "${frontend_pid:-}" ]]; then
    wait "$frontend_pid" 2>/dev/null || true
  fi
  if [[ -n "${backend_pid:-}" ]]; then
    wait "$backend_pid" 2>/dev/null || true
  fi
}

trap cleanup INT TERM EXIT

echo "[dev] starting api first..."
(cd backend && make dev) &
backend_pid="$!"

backend_host="127.0.0.1"
backend_url="http://${backend_host}:${backend_port}/api/v1/health"

echo "[dev] waiting for api to be healthy at $backend_url ..."
deadline="$((SECONDS + 60))"
until curl -fsS "$backend_url" >/dev/null 2>&1; do
  if ! kill -0 "$backend_pid" 2>/dev/null; then
    echo "[dev] api exited before becoming healthy"
    wait "$backend_pid" || true
    exit 1
  fi
  if (( SECONDS >= deadline )); then
    echo "[dev] timed out waiting for api healthcheck"
    exit 1
  fi
  sleep 0.5
done

echo "[dev] api is healthy; starting spa..."
npm run dev &
frontend_pid="$!"

backend_status="0"
frontend_status="0"

while :; do
  backend_alive="0"
  frontend_alive="0"
  if [[ -n "${backend_pid:-}" ]] && kill -0 "$backend_pid" 2>/dev/null; then
    backend_alive="1"
  fi
  if [[ -n "${frontend_pid:-}" ]] && kill -0 "$frontend_pid" 2>/dev/null; then
    frontend_alive="1"
  fi

  if [[ "$backend_alive" == "0" ]]; then
    wait "$backend_pid" || backend_status="$?"
    echo "[dev] api exited; stopping spa..."
    if [[ "$frontend_alive" == "1" ]]; then
      kill -TERM "$frontend_pid" 2>/dev/null || true
      wait "$frontend_pid" || true
    fi
    exit "$backend_status"
  fi

  if [[ "$frontend_alive" == "0" ]]; then
    wait "$frontend_pid" || frontend_status="$?"
    echo "[dev] spa exited; waiting for api to finish..."
    kill -INT "$backend_pid" 2>/dev/null || true
    wait "$backend_pid" || backend_status="$?"

    if [[ "$frontend_status" != "0" ]]; then
      exit "$frontend_status"
    fi
    exit "$backend_status"
  fi

  sleep 0.2
done
