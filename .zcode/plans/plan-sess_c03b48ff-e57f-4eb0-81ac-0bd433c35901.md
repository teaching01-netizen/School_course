# Plan: Make `/admin/legacy-sync` (Health + Audit) Optimal at Heavy Import Scale

## Context
Heavy legacy imports create thousands of open conflicts, jobs, and skipped rows. Today the monitor loads everything eagerly:
- `GET /health` every 2s fetches `ConflictListOpen` unbounded (`SELECT *` with `source_payload/jsonb` + `local_payload/jsonb`) just to count, plus `SyncRunListRecent(20)` but uses only `latest`.
- `GET /conflicts` also unbounded, second copy of same scan at 30s cadence.
- `GET /audit` is a single 7-query blob (`250 + 250 + 100` rows) with JSONB `->>` extraction under seq scan, no cursor, silent truncation beyond 250.
- Health + jobs both poll at 2s per tab → `30 req/min/endpoint/tab` hitting aggregation + JSONB.
Goal: paginate every list, make health cheap, index the audit predicates, decouple progress polling, and trim payloads so the page stays fast at 10k+ rows.

## Goals / Non-Goals
- Goals: (1) True pagination on every monitor list, (2) optimal SQL with covering/partial indexes, (3) cheap health endpoint for 2s polling, (4) dedicated progress/log streaming that only polls when a run is active, (5) blob-free list payloads with on-demand detail.
- Non-Goals: Changing audit skip-count semantics (`NOT EXISTS (legacy_course_id)` stays), changing apply/syncer logic, or adding realtime websockets.

## Investigation Summary (line-referenced)
- `LegacySyncHealth.tsx:19-35`: health + jobs `refetchInterval:2_000` override `cache.ts:20 (30_000)`; `health.staleTime:0` so every mount is stale; conflicts at 30s but health also counts conflicts at 2s → duplicate unbounded scans.
- `LegacyAudit.tsx:12-16`: single `queryKeys.legacySync.audit` with no `limit/cursor`; backend hardcodes `routes.go:145,150,155` → `250/250/100`; `dto.go:182-190` serializes up to 600 rows + 5 aggregated buckets in one JSON → no incremental load.
- `routes.go:39-116`: health runs 4 queries per tick: `LegacySyncControlGet` + `LegacyJobCounts` (cheap) + `SyncRunListRecent(20)` (wastes 19 rows just to find `lastSuccessfulAt`) + unbounded `ConflictListOpen` + `LegacySyncRunProgressGet` N=1.
- `routes.go:121-207`: audit 7 serial queries: `LegacyAuditTotals` 11 `count(*)` subselects + `LegacyAuditRuns` full `sum()` + `LegacyAuditSkipCounts` with `source_payload->>'legacy_schedule_id'` + `NOT EXISTS` anti-join + `LegacyAuditSkipsByCause UNION ALL` + skipped sessions/courses with `LEFT JOIN courses` + dead letters fetching `payload jsonb` then discarding it.
- `legacy_sync.sql:90-93` / `legacy_sync.sql.go:106-112`: `ConflictListOpen` has no `LIMIT` — both `handleHealth` and `handleConflicts` call it unbounded.
- `00080:97-98,116-117`: only `legacy_sync_conflicts(status,created_at)` and `dead_letters(created_at)` exist; no index on `source_payload->>legacy_schedule_id`, no partial `entity_type='course'`, no covering index for `status='open'` ordering.
- `LegacySyncHealth.model.ts:57-86`: list DTO ships full `source_payload/local_payload` and `formatPayload` does `JSON.parse+stringify` per render at poll frequency.

## Design

### 1) Backend: New Paginated + Count Queries
Add to `backend/db/queries/legacy_sync.sql` (sqlc regeneration):
- `ConflictCountOpen :one` → `SELECT count(*) FROM legacy_sync_conflicts WHERE status='open'`
- `ConflictListOpenPaginated :many` → `SELECT id,entity_type,external_id,conflict_type,category,message,status,created_at,resolved_at` **without** `source_payload/local_payload` (`payload` trimmed for lists), `WHERE status='open' ORDER BY created_at DESC LIMIT $1 OFFSET $2`
- `ConflictGet :one` → full payloads by `id` for drawer/detail
- `LegacyJobListRecentPaginated` already `LIMIT $1` but add `OFFSET` variant and `LegacyJobCountByStatus` remains via `LegacyJobCounts` (keep)
- `SyncRunGetLatest :one` → `SELECT ... ORDER BY started_at DESC LIMIT 1` (replace `SyncRunListRecent(20)` in health)
- Keep existing `LegacyAudit*` but add offset params: `LegacyAuditSkippedSessionsPaginated(limit, offset)`, `LegacyAuditSkippedCoursesPaginated(limit, offset)`, `LegacyAuditDeadLettersPaginated(limit, offset)` — same SQL plus `OFFSET $2`.

Alternative considered (keyset/cursor): rejected for now — offset is simplest and existing audit tests use `LIMIT` only; add `created_at < $cursor` overload later if deep pagination shows offset cost.

### 2) Backend: Indexes (new migration `00099_legacy_sync_monitor_opt.sql`)
```sql
-- CONCURRENTLY where possible (see 00071 pattern)
CREATE INDEX CONCURRENTLY IF NOT EXISTS legacy_sync_conflicts_open_created_idx
  ON legacy_sync_conflicts (created_at DESC) WHERE status='open';
CREATE INDEX CONCURRENTLY IF NOT EXISTS legacy_sync_conflicts_course_open_idx
  ON legacy_sync_conflicts (external_id) WHERE entity_type='course' AND status='open';
-- Expression index for audit skipped-session predicate (avoids seq scan on jsonb)
CREATE INDEX CONCURRENTLY IF NOT EXISTS legacy_sync_conflicts_schedule_id_idx
  ON legacy_sync_conflicts ((source_payload->>'legacy_schedule_id')) WHERE source_payload->>'legacy_schedule_id' IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS legacy_sync_dead_letters_course_created_idx
  ON legacy_sync_dead_letters (created_at DESC) WHERE entity_type='course';
CREATE INDEX CONCURRENTLY IF NOT EXISTS legacy_sync_dead_letters_created_desc_idx
  ON legacy_sync_dead_letters (created_at DESC);
-- Cover leaves payload out of heap fetches for counts; keep jsonb out of index
```
Note: GIN on `source_payload` not needed — predicate is equality on single extracted key, expression index suffices and is smaller.

### 3) Backend: Handler Changes (`backend/internal/httpapi/legacysynchttp/`)
- `routes.go:handleHealth`: replace `SyncRunListRecent(20)` loop with `SyncRunGetLatest` (1 row) + single `LegacySyncRunProgressGet` only if `latest != nil`; replace `ConflictListOpen` with `ConflictCountOpen` (int only, no JSONB). New health payload size: ~300 bytes + run, constant regardless of 10k conflicts. Keep `LegacyJobCounts` (already `FILTER` aggregates, cheap).
- `routes.go:handleConflicts`: add `limit/offset` via `limitFromRequest` + `offsetFromRequest` (new helper beside `limitFromRequest` in `requests.go:60`), cap `limit 50 default 20 max 100`, return `{items, total, limit, offset}` envelope. Use `ConflictListOpenPaginated` (payload-trimmed). Add `GET /conflicts/{id}` for detail with full payloads.
- `routes.go:handleJobs`: already `limitFromRequest(r,50,200)` — add `offset` and envelope `{items,total}` using `LegacyJobCounts` for total; frontend stops requesting `limit=12` hardcoded, instead paginated table with page size selector (20/50).
- `routes.go:handleRuns`: add `offset` envelope; health no longer fetches 20 runs.
- `routes.go:handleAudit`: split from single blob to:
  - `GET /audit/summary` → `LegacyAuditTotals` + `LegacyAuditRuns` + `LegacyAuditSkipCounts` + `SkipsByCause` (4 queries, small)
  - `GET /audit/skipped-sessions?limit=&offset=` + `skipped-courses` + `dead-letters` paginated lists
  - Keep legacy `GET /audit` with `?limit=` shim for backward compat but mark deprecated; new frontend calls summary + three lists in parallel with independent `useQuery` keys. Each list query `select`s only 4 `source_payload->>'...'` fields (already trimmed) — add `OFFSET` coverage.
  - `deadLetterToDTO` currently fetches `payload` then drops it (`legacy_audit_custom.go:371` vs `dto.go:252`); change SQL to `SELECT id,job_type,entity_type,external_id,error_category,last_error,attempts,created_at` (no `payload/unique_key`) to cut IO on large job payloads.

### 4) Frontend: Pagination + Polling Discipline
- `LegacySyncHealth.tsx`:
  - Health: raise `refetchInterval` to `10_000` when tab visible and run not `running`; keep `2_000` only while `status==='syncing'` or `latest_run.status==='running'` (adaptive interval via `refetchInterval: ({state}) => isSyncing ? 2000 : 10000`).
  - Conflicts table: replace unbounded `conflictsQuery` with `useInfiniteQuery` or `useQuery` with `{limit, offset}` state + pagination controls (`Previous/Next`, page size 20). List DTO without `source_payload/local_payload`; add click → fetch `GET /conflicts/{id}` drawer that calls `formatPayload` once. Remove `formatPayload` from list render path.
  - Jobs table: paginate with `limit=20, offset` state, show `total` from envelope, add status filter (`queued/running/dead/completed`) as query param.
  - `cachePolicies.operational` for health: set `staleTime: 5_000` (instead of 0) and `refetchOnWindowFocus:false` for health so focus doesn't storm; jobs/conflicts keep `operational` but with 30s stale.
- `LegacyAudit.tsx`:
  - Split `auditQuery` into `summaryQuery` (`/audit/summary`) + `skippedSessionsQuery`, `skippedCoursesQuery`, `deadLettersQuery` each with `{limit=20, offset}` and `keepPreviousData`. Render three paginated tables with `Total / Showing X-Y` and `Next/Prev`. Keep summary cards (linked courses, sessions, etc.) on summary fetch.
  - `queryKeys.legacySync.audit` → `auditSummary`, `auditSkippedSessions({limit,offset})`, etc. so invalidate is granular; `Resolve/Ignore` only invalidates `conflicts` + `summary` counts, not all tables.
- `LegacySyncProgress.tsx`: decouple from `health.latest_run.progress` 2s loop — add dedicated `GET /runs/{id}/progress` query that polls `2_000` only while `run.status==='running'`, otherwise no poll. Health still carries `latest_run.progress` as initial seed but not the hot loop.

### 5) Logs / Process
- Dead letters and skipped rows: surface `last_error` truncated to 240 chars in table, full error in detail drawer (`GET /legacy-sync/jobs/{id}` or `conflicts/{id}`) to avoid shipping MBs per page.
- Add `GET /admin/legacy-sync/runs/{id}` detail with `last_error` + `progress.updated_at` so long imports don't require scrolling health.
- Optional: add `GET /admin/legacy-sync/logs?run_id=&limit=&offset=` backed by `legacy_sync_outbox` or application log table if needed; not in this cut but envelope ready.

### 6) Files to Touch
- Backend DB: `backend/db/queries/legacy_sync.sql`, `backend/internal/db/legacy_audit_custom.go` (add offset methods), new `backend/db/migrations/00099_legacy_sync_monitor_opt.sql`, regenerate `backend/internal/db/legacy_sync.sql.go` via `sqlc`.
- Backend HTTP: `backend/internal/httpapi/legacysynchttp/routes.go`, `dto.go` (new envelope types), `requests.go` (add `offsetFromRequest`), `legacy_sync_progress.go` unchanged.
- Frontend: `src/pages/LegacySyncHealth.tsx`, `LegSyncHealth.model.ts` (split `SyncConflict` into `SyncConflictSummary` vs `SyncConflictDetail`), `src/pages/LegacyAudit.tsx`, `LegacyAudit.model.ts`, `src/query/cache.ts` (operational stale tuning), `src/pages/LegacySyncProgress.tsx` (new progress hook).

### 7) Risks / Mitigations
- Deep OFFSET cost at 100k rows: mitigated by expression + partial indexes and keeping `limit` ≤ 50; add keyset cursor follow-up if `OFFSET 10k` shows `EXPLAIN` seq scan.
- Migration CONCURRENTLY can't run in transaction: use `-- +goose NO TRANSACTION` like `00071`.
- Backward compat: keep old `GET /audit` envelope field names, add `total`/`offset` alongside; frontend migrates incrementally.

### 8) Verification
- `go vet ./...` + `go test ./internal/httpapi/legacysynchttp -run TestAuditEndpointSummarizesImportsAndSkips -count=1` (seed 250-row limit still passes with new paginated query — add `offset=0` default).
- New integration tests: `TestConflictsPaginatedReturnsEnvelope`, `TestHealthDoesNotShipPayloads` (assert `source_payload` absent in health/conflicts list), `TestAuditSkippedSessionsPaginated`.
- Frontend: `npm run test -- LegacySyncHealth LegacyAudit` (existing `__tests__` assert pagination controls render).
- Manual: load `/admin/legacy-sync` with 5k open conflicts seeded; verify Network: health ~1KB/10s, conflicts page ~20 rows/payload-free, `EXPLAIN ANALYZE` shows `Index Scan` on `legacy_sync_conflicts_open_created_idx`.

### 9) Rollout Order
1. DB migration + sqlc regen (no behavior change).
2. Backend handlers (health cheap path + paginated lists + detail endpoints) — behind feature-flagless, old routes keep working.
3. Frontend pagination + adaptive polling.
4. Remove deprecated `GET /audit` blob after 1 release.

### Alternatives Considered
- GIN on `source_payload`: heavier write cost, not needed for single-key extraction.
- WebSocket for progress: overkill; adaptive polling at 2s only while `running` gives same UX with simpler ops.
- Materialized view for `LegacyAuditTotals`: stale on write-heavy import; keep live counts with indexes instead.
