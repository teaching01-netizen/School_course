# Technology Stack

**Analysis Date:** 2026-05-31

## Languages

**Primary:**
- Go 1.25 — Backend API server, migrations, CLI tools
- TypeScript 5.9 — Frontend SPA (React 19)

**Secondary:**
- SQL (PostgreSQL) — Database schema and queries via `sqlc` codegen

## Runtime

**Environment:**
- Node.js 22 (bookworm) — Frontend build stage only
- Distroless base (`gcr.io/distroless/base-debian12:nonroot`) — Production runtime

**Package Manager:**
- npm (lockfile: `package-lock.json` present)

## Frameworks

**Core Backend:**
- `net/http` (stdlib) + `ServeMux` — HTTP routing, no third-party router
- `pgx/v5` — PostgreSQL driver and connection pooling

**Core Frontend:**
- React 19.2.6 — UI framework
- react-router-dom 7.15 — Client-side routing
- Vite 7.3.2 — Build tool and dev server
- Tailwind CSS 4.1.17 — Utility-first CSS (via `@tailwindcss/vite` plugin)

**Testing:**
- Go `testing` stdlib — Backend tests
- Vitest 4.1 — Frontend tests (configured via `vitest` config in `vite.config.ts`)

**Build/Dev:**
- `vite-plugin-singlefile` 2.3.0 — Inlines all frontend assets into a single `index.html` at build time
- `@vitejs/plugin-react` 5.1 — React Fast Refresh
- `concurrently` 9.2 — Runs frontend + backend in dev mode via `npm run dev:full`

## Key Dependencies

**Critical Backend:**
- `github.com/jackc/pgx/v5` — Database access layer
- `github.com/sqlc-dev/sqlc` — Type-safe SQL code generation
- `golang.org/x/crypto/argon2` — Password hashing (Argon2id)

**Critical Frontend:**
- `react-router-dom` — All client-side routing and navigation
- `date-fns` / `luxon` — Date/time handling and timezone-aware display
- `framer-motion` — Animation library
- `lucide-react` — Icon set
- `react-day-picker` — Calendar date picker component

## Configuration

**Environment:**
- `STATIC_DIR` — Path to built frontend assets (default `../dist`, overridden to `/app/dist` in Docker)
- `DATABASE_URL` — PostgreSQL connection string (required)
- `AUTH_PEPPER` — Server-side pepper for Argon2id (required)
- `OTP_HMAC_KEY` — HMAC key for OTP tokens (required)
- `ADDR` — Server listen address (default `:8080`)
- `LOG_LEVEL` — Logging verbosity (default `info`)
- `INSTITUTE_TZ` — Institute timezone for scheduling (default `Asia/Bangkok`)
- `APP_ORIGIN` — CORS origin for dev mode
- CRM credentials (`CRM_BASE_URL`, `CRM_USERNAME`, `CRM_PASSWORD`)
- SMS credentials (`SMS_SERVICE_*`)
- See `backend/internal/config/config.go` for full list

**Build Config:**
- `vite.config.ts` — Vite build config with singlefile plugin
- `tsconfig.json` — TypeScript config
- `Dockerfile` — Multi-stage production build

## Platform Requirements

**Development:**
- Go 1.25+ toolchain
- Node.js 22+
- PostgreSQL 16+ (local or Docker)
- `lsof` (optional, for port conflict detection in `scripts/dev.sh`)

**Production:**
- Docker (multi-stage build targets distroless base image)
- Deployment target: Railway (managed containers + PostgreSQL)

---

*Stack analysis: 2026-05-31*
