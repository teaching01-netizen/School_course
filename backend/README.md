# Backend (Go)

This repo is a single-deployable “modular monolith” service:

- serves the built SPA from `../dist/`
- serves JSON API under `/api/v1/*`

## Local dev

1) Start Postgres:

`docker compose up -d db`

2) Run migrations:

`make -C backend migrate-up`

3) Run API:

`make -C backend dev`

Environment:

- `DATABASE_URL` (required)
- `AUTH_PEPPER` (required; long random secret)
- `COOKIE_SECURE` (`false` for local http; set `true` in prod)
- `INSTITUTE_TZ` (optional; default `Asia/Bangkok`)

## Railway / Docker deploy

This repo includes a root `Dockerfile` that builds:

- SPA assets into `/app/dist`
- Go binaries (`/app/server`, `/app/migrate`)

Suggested Railway settings (Dockerfile deploy):

- **Start Command:** (none; uses image entrypoint)
- **Pre-Deploy Command:** `/app/migrate up`
- **Healthcheck Path:** `/api/v1/health`

Required env vars:

- `DATABASE_URL`
- `AUTH_PEPPER`
- `COOKIE_SECURE=true`
- `ADDR=:8080` (Railway default port wiring)
