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
- `REALTIME_DATABASE_URL` (required only when `DATABASE_URL` uses transaction pooling; use a direct or session-pooling endpoint)
- `AUTH_PEPPER` (required; long random secret)
- `OTP_HMAC_KEY` (required for direct `go run`; `make dev` provides a local fallback)
- `COOKIE_SECURE` (optional; defaults to `true`; production rejects `false`; set `false` only for local http; local mode uses a non-`__Host-` cookie name)
- `INSTITUTE_TZ` (optional; default `Asia/Bangkok`)
- `TRUSTED_PROXY_CIDRS` (optional; comma-separated proxy CIDRs; forwarded headers are ignored when the direct peer is outside these networks)

## Trusted proxy configuration

Set `TRUSTED_PROXY_CIDRS` to the exact network(s) of the reverse proxy or load
balancer that connects directly to the API, for example:

`TRUSTED_PROXY_CIDRS=10.0.0.0/8,192.0.2.0/24`

The server uses the direct TCP peer as the client address unless that peer is
inside a configured network. Only then does it parse `X-Forwarded-For`, walking
the chain from the application outward and selecting the first untrusted hop.
Do not configure public or broad ranges that could contain arbitrary clients:
an attacker who connects directly must not be able to choose their rate-limit
identity with a forwarded header. Leave the variable empty when the API is not
behind a trusted proxy.

The same resolved address is used by login, OTP, and public student-lookup rate
limits.

> **Must be set when the app is behind a reverse proxy.** When the API runs
> behind nginx, Cloudflare, Railway's edge routing, or any other reverse proxy
> and `TRUSTED_PROXY_CIDRS` is left empty, the server treats the direct TCP peer
> (the proxy itself) as the client address. Every user then shares the proxy IP,
> so per-IP rate limits silently collapse onto a single bucket: either all users
> are throttled together or one user's budget applies to everyone. The server
> logs a startup warning when `TRUSTED_PROXY_CIDRS` is empty; if the app is
> deployed behind a proxy, set it to the proxy's exact CIDR(s) (see the startup
> log and the example above). Leave it empty only when clients connect directly.

## Railway / Docker deploy

This repo includes a root `Dockerfile` that builds:

- SPA assets into `/app/dist`
- Go binaries (`/app/server`, `/app/legacy-sync`, `/app/migrate`, and the
  one-shot cleanup utilities)

### Main service

A `railway.toml` at repo root defines the build/deploy config:

- **Builder:** `DOCKERFILE`
- **Pre-Deploy Command:** `/app/migrate up`
- **Healthcheck:** `/api/v1/health` (120s timeout)
- **Restart:** on failure, max 3 retries

The `/app/server` entrypoint owns the always-on backend. It starts the API and
the `/app/legacy-sync` companion worker from the same image, and it runs the
database retention cleanup loops. No additional Railway service is required
for those jobs.

### Required env vars

| Var | Notes |
|---|---|
| `DATABASE_URL` | Auto-injected by Railway Postgres plugin |
| `AUTH_PEPPER` | `openssl rand -base64 48` (Railway secret) |
| `OTP_HMAC_KEY` | `openssl rand -base64 32` (Railway secret) |
| `ADDR` | `:8080` (Railway default) |

### Optional env vars

| Var | Default | Notes |
|---|---|---|
| `INSTITUTE_TZ` | `Asia/Bangkok` | Institute timezone |
| `LOG_LEVEL` | `info` | |
| `COOKIE_SECURE` | `true` | Production rejects `false`; set `false` only for local http |
| `TRUSTED_PROXY_CIDRS` | — | Exact CIDR(s) for directly connecting reverse proxies; **required when deployed behind a proxy** (Railway edge/nginx/Cloudflare) or per-IP rate limits collapse to the proxy IP; leave empty for direct clients |
| `APP_ENV` | — | Set `production` (or `prod`) for production startup checks |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | — | Not read by the server; production rejects either variable. Use `go run ./cmd/devseed` explicitly for local development only |
| `APP_ORIGIN` | — | Railway domain for CSRF checks, e.g. `https://your-project.up.railway.app` |
| `CRM_BASE_URL` / `CRM_USERNAME` / `CRM_PASSWORD` | — | CRM integration |
| `OTP_SMS_PROVIDER` | `mock` | `smartsms` for real SMS |
| `SMS_SERVICE_*` | — | SmartSMS credentials |
| `OTP_ASYNC_DELIVERY_ENABLED` | `false` | Enables PostgreSQL-backed durable OTP delivery after migration `00065` is applied |
| `OTP_DELIVERY_ENCRYPTION_KEYS` | — | Required when async delivery is enabled; comma-separated `version:base64` AES-256 keys, newest key last, e.g. `v1:...` |
| `REALTIME_DATABASE_URL` | `DATABASE_URL` | Direct or session-pooling PostgreSQL endpoint used for realtime `LISTEN/NOTIFY`; required when `DATABASE_URL` uses transaction pooling (commonly port 6543) |
| `LEGACY_SYNC_BINARY` | sibling `/app/legacy-sync` | Optional absolute path override for the worker launched by `/app/server` |

For a safe rollout, deploy migration `00065`, the backend, and the frontend while
`OTP_ASYNC_DELIVERY_ENABLED=false`. Configure `OTP_DELIVERY_ENCRYPTION_KEYS`, then
enable the flag after all server instances run the new version. Disable the flag
to return new OTP requests to the legacy synchronous path; queued delivery rows
remain available for operational inspection.

### Background workers and maintenance

`cmd/legacy-sync` is a **required worker** for the legacy refresh/reconcile jobs:
the API queues `legacy_refresh_course` and `legacy_full_reconcile` jobs, and only
this worker picks them up and applies the legacy-site aggregates. It must be
running alongside the server at all times:

- **Local dev:** `make -C backend legacy-sync` (or `npm run dev:full` / `scripts/dev.sh`, which start it automatically)
- **Deploy:** the main `/app/server` process starts `/app/legacy-sync` from the same image; do not create a separate Railway service

It needs `DATABASE_URL` plus the same `LEGACY_SYNC_*` env vars as the server (`LEGACY_SYNC_URL`, `LEGACY_SYNC_USERNAME`, `LEGACY_SYNC_PASSWORD`).

The server also runs the former six-hour cleanup jobs internally:

- expired idempotency keys
- expired OTP, lookup-token, and student-session rows

The `/app/cleanup-idempotency` and `/app/cleanup-verification-sessions`
binaries remain available for manual one-shot recovery, but they are not
separate deployment services.

Scrape throughput is tunable (all optional; defaults maximize speed while the
circuit breaker and the per-request timeout still protect the legacy site):

| Variable | Default | Meaning |
|---|---|---|
| `LEGACY_SYNC_MAX_CONCURRENT` | `32` | Requests in flight against the legacy site at once (clamped to 128); also sizes the student-profile lookup pool and the keep-alive connection pool |
| `LEGACY_SYNC_MIN_REQUEST_INTERVAL` | `0` (disabled) | Global pacing between requests; set e.g. `500ms` to restore the historical one-request-per-interval politeness |
| `LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE` | `720` | Per-minute request budget; when exhausted the run pauses until the window resets |
| `LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE` | `200 MiB` | Per-minute download budget; like the request budget, a systemic limit |
| `LEGACY_SYNC_WORKERS` | client `MaxConcurrent` (32) | Runner processes draining the legacy job queue in parallel |
| `LEGACY_SYNC_RECONCILE_WORKERS` | `min(MaxConcurrent, 16)` | Worker pool for the full-reconcile DB phases; `0`/`1` force the exact serial path |
| `LEGACY_SYNC_POOL_MAX_CONNS` | worker-derived budget (`max(64, 2×workers)`) | pgx pool connection cap; wins over a `pool_max_conns` URL parameter, which is otherwise preserved |
| `LEGACY_SYNC_HTTP_TIMEOUT` | `120s` | Per-request budget including redirects and body download |

Two request-reduction behaviors are always on: the search-form antiforgery
token (students directory and archived course list) is cached per session, so
lookups cost one request instead of a page read + search; and the token cache
auto-refreshes on re-login or on a failed search, so a rotated token never
silently drops students.
