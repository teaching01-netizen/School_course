# External Integrations

**Analysis Date:** 2026-05-31

## APIs & External Services

**None deployed as separate services.** All external API calls go to the CRM/SMS third-party systems.

## Data Storage

**Databases:**
- PostgreSQL — Single database via `DATABASE_URL` env var
  - Client: `pgx/v5` connection pool (`backend/internal/pg/`)
  - Migrations: custom `cmd/migrate` runner with advisory-lock serialization
  - Code generation: `sqlc` for type-safe SQL
  - Connection: `DATABASE_URL` env var (required)

**File Storage:**
- **None.** No uploads or file storage (XLSX imports read directly from a staging table via COPY).

**Caching:**
- **None.** No Redis, Memcached, or in-process cache.

## Authentication & Identity

**Auth Provider:**
- **Custom app-managed** — local usernames/passwords stored in PostgreSQL
  - Implementation: `backend/internal/auth/` — session-based with `HttpOnly; Secure; SameSite=Strict` cookies, Argon2id hashing with server-side pepper
  - Roles: `Admin` and `Teacher` (RBAC enforced at application layer)
  - No OAuth, SSO, or external identity provider

## Frontend Serving Architecture

**Production (Docker):**
- Multi-stage Docker build (`Dockerfile`):
  1. `node:22-bookworm` stage: `npm ci` → `npm run build` (Vite + `vite-plugin-singlefile`) → outputs single `dist/index.html` (~952KB)
  2. `golang:1.25` stage: compiles Go server binary
  3. `distroless/base-debian12:nonroot` stage: copies `dist/` and Go binary, sets `STATIC_DIR=/app/dist`, exposes port 8080
- Go server serves `dist/index.html` from **disk** (NOT embedded via `//go:embed`)

**Static File Serving Mechanism (`backend/internal/httpapi/handler.go:129-150`):**
- No `embed.FS` or `http.FileServer` used
- Custom handler on `/` that:
  1. Returns 404 for `/api/*` paths not matched by API routes
  2. Tries exact file match via `os.Stat` + `http.ServeFile`
  3. Falls back to serving `staticDir/index.html` for SPA client-side routing
- `staticDir` is from `STATIC_DIR` env (default `../dist` at dev, `/app/dist` in production)

**Development Mode (`scripts/dev.sh`):**
- Runs Go backend via `make dev` (listens on `:8080`)
- Runs Vite dev server via `npm run dev` (proxies `/api/*` to `:8080` via `vite.config.ts`)
- Vite provides HMR, the Go backend handles all `/api/*` endpoints
- No hot-reload for Go backend (restart on change required)

**Frontend Build Output:**
- Single file: `dist/index.html` (all JS/CSS/assets inlined via `vite-plugin-singlefile`)
- No separate JS chunks, CSS files, or image assets in production build
- `dist/` is gitignored

## Monitoring & Observability

**Error Tracking:**
- **None.** No Sentry, Datadog, or similar.

**Logs:**
- Structured JSON logging via `log/slog` (stdlib)
- Log level configurable via `LOG_LEVEL` env var
- No log aggregation service configured

## CI/CD & Deployment

**Hosting:**
- Railway (managed containers + managed PostgreSQL)

**CI Pipeline:**
- **Not detected.** No `.github/workflows/` or CI config found. Builds assumed to happen via Railway's Docker build.

**Deployment Configuration:**
- `Dockerfile` — Multi-stage build (frontend → Go → distroless)
- No `Railway.toml` found — Railway auto-detects Dockerfile
- Pre-deploy step assumed to run `/app/migrate` (binary available at `/migrate`)
- Railway Cron Jobs for background tasks (cleanup-idempotency, cleanup-verification-sessions)

## Environment Configuration

**Required env vars (all non-development):**
- `DATABASE_URL` — PostgreSQL DSN
- `AUTH_PEPPER` — Argon2id pepper
- `OTP_HMAC_KEY` — OTP signing key
- `STATIC_DIR` — Set to `/app/dist` via Dockerfile `ENV`

**Secrets location:**
- Railway environment variables/secret store

## Webhooks & Callbacks

**Incoming:**
- **None** detected. No webhook endpoints.

**Outgoing:**
- **None** detected. No webhook or callback dispatching.

---

*Integration audit: 2026-05-31*
