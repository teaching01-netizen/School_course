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
- `OTP_HMAC_KEY` (required for direct `go run`; `make dev` provides a local fallback)
- `COOKIE_SECURE` (`false` for local http; set `true` in prod)
- `INSTITUTE_TZ` (optional; default `Asia/Bangkok`)

## Railway / Docker deploy

This repo includes a root `Dockerfile` that builds:

- SPA assets into `/app/dist`
- Go binaries (`/app/server`, `/app/migrate`, `/app/cleanup-idempotency`, `/app/cleanup-verification-sessions`)

### Main service

A `railway.toml` at repo root defines the build/deploy config:

- **Builder:** `DOCKERFILE`
- **Pre-Deploy Command:** `/app/migrate up`
- **Healthcheck:** `/api/v1/health` (120s timeout)
- **Restart:** on failure, max 3 retries

### Required env vars

| Var | Notes |
|---|---|
| `DATABASE_URL` | Auto-injected by Railway Postgres plugin |
| `AUTH_PEPPER` | `openssl rand -base64 48` (Railway secret) |
| `OTP_HMAC_KEY` | `openssl rand -base64 32` (Railway secret) |
| `COOKIE_SECURE` | `true` |
| `ADDR` | `:8080` (Railway default) |

### Optional env vars

| Var | Default | Notes |
|---|---|---|
| `INSTITUTE_TZ` | `Asia/Bangkok` | Institute timezone |
| `LOG_LEVEL` | `info` | |
| `ADMIN_USERNAME` | — | Creates admin on first start |
| `ADMIN_PASSWORD` | — | Pair with ADMIN_USERNAME |
| `APP_ORIGIN` | — | Railway domain for CSRF checks, e.g. `https://your-project.up.railway.app` |
| `CRM_BASE_URL` / `CRM_USERNAME` / `CRM_PASSWORD` | — | CRM integration |
| `OTP_SMS_PROVIDER` | `mock` | `smartsms` for real SMS |
| `SMS_SERVICE_*` | — | SmartSMS credentials |

### Cron jobs

Two cleanup binaries are built into the image. Create **separate Cron services** in Railway dashboard (same Docker image, different `startCommand`):

| Service | startCommand | Schedule | Purpose |
|---|---|---|---|
| cleanup-idempotency | `/app/cleanup-idempotency` | `0 */6 * * *` (every 6h) | GC stale idempotency keys |
| cleanup-verification-sessions | `/app/cleanup-verification-sessions` | `0 */6 * * *` | GC expired OTP sessions |

Both need `DATABASE_URL` — Railway cron services inherit env vars from the service.
