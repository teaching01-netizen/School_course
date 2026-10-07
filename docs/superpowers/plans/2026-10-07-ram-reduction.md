# RAM Reduction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `executing-plans` to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Reduce measured Postgres and application RAM under the same workload, with a provisional target of 30–50% lower project peak memory while preserving API reliability, sync freshness, and import correctness.

**Architecture:** Keep the existing Go API, companion legacy-sync process, PostgreSQL database, and static React deployment. First reduce concurrent background work using existing environment variables; then release idle database connections sooner. Apply further database and import optimizations only when the measurements identify their contribution.

**Tech stack:** Go 1.25.7, pgx v5.9.2, PostgreSQL (local Compose uses version 16; production version must be checked), Excelize v2.9.0, Railway, React/Vite static assets.

**Repository:** `/Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2`

**Status:** Planning only. No production settings or application code were changed. Production database settings, connection counts, replica counts, query statistics, and process memory have not been measured in this session. Existing uncommitted application work must be preserved.

---

## Decision and evidence

Start with the legacy-sync worker. It has the clearest code-backed opportunities and can increase both application memory and database memory.

| Finding | Evidence | Meaning |
| --- | --- | --- |
| Postgres is the main consumer at the selected screenshot timestamp | 7 October 2026, 08:44 GMT+7: Postgres 1.05 GB; School_course 113 MB | Approximately 90% of displayed project memory belongs to Postgres at that instant. |
| Postgres repeatedly rises from roughly 450–550 MB to around 1.0–1.1 GB | Attached memory graph | Recurring activity is plausible; this graph does not establish a memory leak or its cause. |
| Legacy HTTP concurrency defaults to 32 | [client.go](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/internal/legacysync/client.go>) | Multiple downloads and HTML parses can overlap. |
| Legacy queue concurrency inherits the HTTP concurrency by default | [tuning.go](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/cmd/legacy-sync/tuning.go>) | Up to 32 top-level jobs can run concurrently. |
| Full reconciliation defaults to up to 16 workers | Same tuning file | A job can create additional DB activity inside its own processing phases. |
| Legacy pool defaults to `max(64, 2 × workers)` | Same tuning file and [worker startup](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/cmd/legacy-sync/main.go>) | Default ceiling is 64 worker connections, before API connections. A ceiling is not a count of connections already opened. |
| Legacy pool does not override pgx idle defaults | Worker startup; installed pgx v5.9.2 source | Without URL overrides, idle connections can remain for 30 minutes; connection lifetime defaults to one hour. |
| API pool defaults to 10, dedicated realtime pool to 2 | [pg.go](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/internal/pg/pg.go>) | The default total pool ceiling is 76 connections per app replica. |
| Worker reserves two connections for long-lived tasks | Worker leadership connection and [Postgres fanout](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/internal/realtime/postgres_fanout.go>) | Pool sizing must leave room for leadership, `LISTEN`, progress, heartbeats, and outbox publication. |
| The API also runs CRM jobs and maintenance | [server startup](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/cmd/server/main.go>), [CRM queue](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/internal/crmimport/queue/queue.go>) | CRM `LISTEN` reserves a connection from the API pool. Keep the API pool at 10 initially. |
| CRM parsing materializes an entire worksheet | [xlsx_parse.go](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/internal/crmimport/xlsx/xlsx_parse.go>) | `GetRows` holds worksheet cells while parsed rows are also constructed. |
| CRM insertion constructs another full row collection | [snapshot_service.go](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/internal/crmimport/snapshot_service.go>) | `rowCopies` creates a `[][]any` before `CopyFrom`. |
| School_course includes two Go processes | [legacy_sync_process.go](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/cmd/server/legacy_sync_process.go>), backend README | A Go memory limit inherited by the child applies separately to both processes. |

These are repository defaults. Production environment variables and database URL parameters may already override them. Confirm the deployed values before claiming savings.

The runtime image serves static frontend assets with Go; Node is a build-stage dependency. Frontend bundle cleanup does not directly address the Postgres RAM shown here.

## Targets and acceptance rules

Treat the following as proposed acceptance targets, not forecasts:

| Metric | Initial target | Stronger target, only if initial changes pass |
| --- | --- | --- |
| Matched-workload project peak | At least 30% lower than the measured baseline | At least 50% lower |
| Postgres peak | Approximately 650–750 MB or less, subject to production baseline | Approximately 500–600 MB |
| Average project memory over comparable 24-hour windows | At least 20% lower | At least 30% lower |
| API p95 and p99 latency | No more than 10% worse than baseline | Same |
| Request success rate | No new timeout, connection exhaustion, or 5xx failures in controlled tests | Same |
| Legacy freshness | Eligible course refreshes complete within the existing 30-minute refresh cadence under the normal course set | Same; do not extend the cadence to manufacture savings |
| Background queue | Backlog drains; oldest eligible queued job does not keep increasing | Same |
| Correctness | Same imported records, snapshot ordering, schedule identities, and realtime events | Same |

If the normal course set already exceeds the current refresh cadence, record that before rollout and require the new configuration to preserve or improve the measured freshness. Do not label a slower, accumulating queue a successful memory optimization.

A lower hard service limit is not the initial solution. Railway bills actual usage; reducing a limit alone does not reduce usage below it. Use comparable average usage to assess ongoing cost. [Railway right-sizing guide](https://docs.railway.com/guides/right-size-cpu-memory)

## Task 1: Establish a baseline and distinguish the memory categories

**Owner:** Engineer with read access to Railway metrics and the database.

**Create during implementation:** `backend/db/verification/memory_baseline.sql`. Store measurements outside Git if they contain operational details; commit only the query and an aggregate result summary.

- [ ] Record the deployed revision, active/overlapping replica count, database version, current service limits, configured legacy concurrency, and effective pool caps. Record configuration values without copying credentials or database URLs into logs or the report.
- [ ] Export a comparable 24-hour memory/CPU window, identify deployment markers, and correlate peaks with legacy refreshes, full reconciliation, CRM imports, and maintenance. Do not infer a cause from timestamp proximity alone.
- [ ] Run the SQL below in a private database console before, during, and after at least three recurring peaks. For the peak window, sample approximately every 15 seconds for 10 minutes. Do not reset cumulative production statistics.

```sql
BEGIN READ ONLY;
SET LOCAL statement_timeout = '5s';
SET LOCAL lock_timeout = '1s';

SELECT now() AS captured_at,
       current_setting('server_version') AS server_version,
       pg_postmaster_start_time() AS postgres_started_at;

SELECT name, setting, unit, source, context, reset_val, pending_restart
FROM pg_settings
WHERE name IN (
  'shared_buffers', 'work_mem', 'hash_mem_multiplier',
  'maintenance_work_mem', 'autovacuum_work_mem', 'autovacuum_max_workers',
  'max_connections', 'superuser_reserved_connections',
  'max_parallel_workers', 'max_parallel_workers_per_gather',
  'max_parallel_maintenance_workers', 'shared_preload_libraries'
)
ORDER BY name;

SELECT backend_type, application_name, COALESCE(state, 'n/a') AS state,
       count(*) AS connections,
       max(now() - xact_start) AS oldest_transaction,
       max(now() - query_start) FILTER (WHERE state = 'active') AS longest_active_query
FROM pg_stat_activity
WHERE datname = current_database()
GROUP BY backend_type, application_name, COALESCE(state, 'n/a')
ORDER BY connections DESC;

SELECT numbackends, xact_commit, xact_rollback,
       temp_files, temp_bytes, deadlocks, blks_read, blks_hit, stats_reset
FROM pg_stat_database
WHERE datname = current_database();

SELECT relname, n_live_tup, n_dead_tup,
       last_autovacuum, last_autoanalyze,
       pg_total_relation_size(relid) AS total_relation_bytes
FROM pg_stat_user_tables
ORDER BY pg_total_relation_size(relid) DESC
LIMIT 15;

SELECT job_type, status, count(*) AS jobs,
       min(created_at) FILTER (
         WHERE status = 'queued' AND run_after <= now()
       ) AS oldest_eligible_created_at,
       count(*) FILTER (
         WHERE status = 'running' AND locked_until < now()
       ) AS expired_running_leases
FROM legacy_sync_jobs
WHERE status IN ('queued', 'running')
GROUP BY job_type, status
ORDER BY job_type, status;

COMMIT;
```

Use deltas for cumulative statistics, checking `stats_reset`; the lifetime `temp_bytes` value is not a measurement of the current peak. PostgreSQL exposes connection/activity and cumulative database statistics through these views. [PostgreSQL statistics documentation](https://www.postgresql.org/docs/16/monitoring-stats.html)

- [ ] Where private container execution is available, inspect the service's own Linux cgroup, confirming the files describe that service rather than its host:

```sh
cat /sys/fs/cgroup/memory.current
cat /sys/fs/cgroup/memory.max
cat /sys/fs/cgroup/memory.stat
cat /sys/fs/cgroup/memory.events
```

Separate anonymous memory from filesystem cache and inspect OOM events. `shmem` overlaps other accounting categories; do not blindly sum every field in `memory.stat`. Process RSS totals can double-count Postgres shared pages; use proportional/private memory when available. Railway's public metrics description does not specify the cache accounting needed to equate its graph directly to private heap. [Linux cgroup memory accounting](https://docs.kernel.org/admin-guide/cgroup-v2.html), [Railway metrics](https://docs.railway.com/observability/metrics)

- [ ] Classify the evidence using these predictions:

| Candidate explanation | Prediction to test | Next step if supported |
| --- | --- | --- |
| Too much legacy concurrency / retained idle backends | Peaks coincide with worker connections and jobs; warm-run peaks fall after concurrency reduction | Tasks 2–3 |
| Query scratch memory / parallel workers | Active expensive queries and parallel workers rise at peaks; plans contain large sorts/hashes | Task 4 |
| Shared buffers or filesystem cache | Memory stays elevated without many active backends; file/shared memory accounts for most growth | Inspect cache/I/O tradeoff in Task 4; do not call it a leak |
| Maintenance | Autovacuum or retention activity aligns repeatedly with the same peaks | Tune maintenance memory only after observing it |
| Application import allocation | School_course peak and Go allocation profile correlate with import jobs | Task 5 |

**Exit:** A baseline with peak, average, connection counts, queue freshness, and application latency; or an explicit account of unavailable measurements. Static findings alone do not establish a production root cause.

## Task 2: Reduce background concurrency using existing controls

**Owner:** Deployment configuration. **Application files changed:** None for the first experiment.

- [ ] Save the exact previous environment values, including which variables were absent.
- [ ] First change only the work-concurrency group below on staging. Keep the existing database pool cap for this comparison:

```dotenv
LEGACY_SYNC_MAX_CONCURRENT=4
LEGACY_SYNC_WORKERS=2
LEGACY_SYNC_RECONCILE_WORKERS=2
```

- [ ] Replay normal API activity concurrently with a course-refresh sweep and one full reconcile. Use a staging snapshot with the current normal course count and a local legacy fixture or controlled staging source. Do not run a destructive full-reconcile load experiment against production data.
- [ ] Compare memory, completed jobs per minute, oldest queued work, and full sweep duration after the system has warmed. If two job workers cannot meet freshness, test four workers rather than extending the refresh interval.
- [ ] After the concurrency group passes, apply the worker pool cap as a separate experiment:

```dotenv
LEGACY_SYNC_POOL_MAX_CONNS=8
```

Keep `POOL_MAX_CONNS` at its current effective value, normally 10. The realtime API pool stays at 2. With the repository's API default, the new maximum becomes `10 + 2 + 8 = 20` connections per replica, down from 76. This is approximately a 74% reduction in the connection ceiling, not a promise of 74% lower RAM.

For a four-job-worker fallback, test `LEGACY_SYNC_POOL_MAX_CONNS=12`. Do not use an 8-connection cap with arbitrarily higher concurrency. The worker's leadership and realtime listener consume two slots; progress, heartbeat, outbox, and nested DB calls also need capacity.

The existing worker pool override takes precedence over a `pool_max_conns` URL parameter. The API pool follows `POOL_MAX_CONNS` and replaces the parsed URL cap. Inspect both paths rather than assuming one global setting controls every connection.

The 16 MiB per-response legacy limit remains intact. Reducing concurrent downloads from 32 to 4 lowers the illustrative concurrent raw-body allowance from 512 MiB to 64 MiB. This calculation excludes HTML nodes, temporary copies, cached data, and other allocations; it is not a process-memory bound.

- [ ] Confirm all of the following while the capped pool is saturated: leadership survives, `LISTEN` receives notifications, publication succeeds, heartbeat leases do not expire, and completed jobs are not duplicated.
- [ ] Roll out the passing profile to the current service and observe at least 24 hours, including ordinary refreshes and a representative import. Compare warm behavior after deployment, not just the lower cold-start graph.

**Rollback:** Restore the saved concurrency values first if freshness regresses. Restore the saved pool cap if heartbeats/publication/acquisitions fail. If increasing concurrency, increase its required pool budget in the same rollout; avoid a high-worker, low-connection combination.

**Exit:** Lower measured peak and/or average memory with stable throughput and correctness. If no material reduction occurs, retain only changes with a measured benefit and move to the supported diagnosis branch.

## Task 3: Release idle worker connections sooner

**Owner:** Worker pool creation boundary.

**Modify:** [backend/cmd/legacy-sync/main.go](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/cmd/legacy-sync/main.go>) immediately before `pgxpool.NewWithConfig`.

- [ ] Add the following pool settings after the existing maximum-connection precedence logic:

```go
poolConfig.MinConns = 0
poolConfig.MinIdleConns = 0
poolConfig.MaxConnIdleTime = time.Minute
poolConfig.HealthCheckPeriod = 30 * time.Second
poolConfig.ConnConfig.RuntimeParams["application_name"] = "warwick-legacy-sync"
```

Leave the connection lifetime unchanged for this experiment. A short idle timeout releases unused connections; repeatedly expiring active long-lived listener/leader connections does not achieve that purpose. No new general pool abstraction is needed.

- [ ] Add application names at the existing API/realtime pool boundary in [backend/internal/pg/pg.go](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/internal/pg/pg.go>), before pool creation:

```go
applicationName := "warwick-api"
if !honorPoolMaxConns {
	applicationName = "warwick-realtime-api"
}
cfg.ConnConfig.RuntimeParams["application_name"] = applicationName
```

- [ ] In staging, create a worker connection burst, stop admitting test jobs, and sample the baseline activity query at 0, 30, 60, 90, and 120 seconds. Expect unused worker connections to disappear around the idle timeout plus health-check interval; the two acquired long-lived worker connections may remain.
- [ ] Start another sweep after the idle period. Verify reconnect latency, heartbeat continuity, publication, and job completion; repeat three times. Measure TLS/network overhead as well as RAM.
- [ ] Run the existing relevant tests from the backend directory:

```sh
go test ./internal/pg ./internal/realtime ./cmd/legacy-sync ./cmd/server -count=1
```

Database-backed tests require a disposable `TEST_DATABASE_URL`; skipped integration tests are not passing integration evidence. Run these existing real-DB scenarios explicitly with that variable configured:

```sh
go test ./internal/legacysync/reconcile -run '^TestFullReconcile_(ParallelMatchesSerial|ParallelAndSerialConvergeSameCatalogue)$' -count=1 -timeout=120s
go test ./internal/realtime -run '^TestPostgresFanoutPublishesAndReceivesEnvelope$' -count=1 -timeout=120s
```

- [ ] Document the passing environment profile and worker idle policy in [backend/README.md](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/README.md>). Preserve unrelated existing edits. Commit only the files belonging to this task.

**Rollback:** Revert the idle-time changes if connection churn degrades latency. Application names can remain for diagnosis.

## Task 4: Tune the database only for the measured remaining pressure

**Owner:** Database operator and the offending query/connection boundary. These are conditional experiments, not a configuration bundle to apply blindly.

Postgres `work_mem` applies to individual sort/hash operations, can multiply across active queries and parallel workers, and hash operations use `hash_mem_multiplier`. Maintenance has a separate memory budget. Lower values can increase disk spills. [PostgreSQL resource settings](https://www.postgresql.org/docs/16/runtime-config-resource.html)

For scale, eight active queries with two hash operations each, 4 MB `work_mem`, and multiplier 2 can allow approximately 128 MB of scratch space before considering parallelism. This is an illustration rather than a complete upper bound.

- [ ] Capture current values and their sources. Prepare exact rollback SQL from the recorded original settings before changing anything. Check `pg_settings.context` and `pending_restart`; a successful reload does not prove a restart-only setting took effect. [pg_settings](https://www.postgresql.org/docs/16/view-pg-settings.html)

| Setting | Candidate | Apply only when |
| --- | --- | --- |
| Worker `work_mem` | 2 MB | Worker query plans or samples identify scratch memory as meaningful; measured current effective value is higher |
| Worker `max_parallel_workers_per_gather` | 0 | Worker query plans actually use parallel workers and disabling them passes elapsed-time/throughput gates |
| `shared_buffers` | 128 MB | Current value is higher and the smaller cache passes warm-query and disk-I/O checks |
| `maintenance_work_mem` | 32 MB | Maintenance is a demonstrated contributor and its current value is higher |
| `autovacuum_work_mem` | 16 MB per worker | Autovacuum pressure is demonstrated; dead tuples and vacuum completion remain healthy |
| `max_connections` | 50 for one active app replica with one overlapping deployment | The computed peak connection budget fits; no other consumers invalidate that budget |
| Go service soft memory limit | Profile-derived in Task 6 | Application allocations remain a meaningful contributor |

Do not raise a setting that is already below its candidate. Do not reduce all these settings at once.

For worker-only query settings, use the pool boundary from Task 3, applying one line at a time in separate measurements:

```go
poolConfig.ConnConfig.RuntimeParams["work_mem"] = "2MB"
```

```go
poolConfig.ConnConfig.RuntimeParams["max_parallel_workers_per_gather"] = "0"
```

This does not change API query budgets. Keep the two API realtime connections and the worker's session-capable listener/leadership connections operational.

For a confirmed shared-buffer case, the operator command is:

```sql
ALTER SYSTEM SET shared_buffers = '128MB';
```

This needs a database restart, not just an application redeploy. Preserve the provider's existing entrypoint, SSL setup, data directory, and volume. Railway documents `ALTER SYSTEM` as its configuration mechanism. Do not write a custom start command without inspecting the existing one. [Railway PostgreSQL configuration](https://docs.railway.com/databases/postgresql), [ALTER SYSTEM](https://www.postgresql.org/docs/16/sql-altersystem.html)

For individually justified maintenance experiments, execute one setting, then reload and verify it before evaluating the next:

```sql
ALTER SYSTEM SET maintenance_work_mem = '32MB';
SELECT pg_reload_conf();
```

```sql
ALTER SYSTEM SET autovacuum_work_mem = '16MB';
SELECT pg_reload_conf();
```

Leave autovacuum enabled. Inspect whether it still keeps up over the comparison window; lower memory is a failed experiment if dead tuples and long-running vacuum work keep growing.

Compute connection capacity before reducing the database-wide ceiling:

```text
required client slots = peak simultaneous app replicas × 20
                      + migration/manual-tool slots
                      + operations and reserved-slot allowance
```

For one app replica plus its overlapping replacement, allowing eight additional slots gives `2 × 20 + 8 = 48`; 50 is a candidate. Two active replicas plus two replacements require `4 × 20 + 8 = 88`, so 50 would be wrong. Recalculate using the actual effective API cap and the four-worker fallback if applicable. A lower `max_connections` is a protection ceiling; unopened slots do not each reserve a full backend's private heap.

Only after this capacity gate passes:

```sql
ALTER SYSTEM SET max_connections = '50';
```

Verify the required restart and recovery access before rollout. Restore the captured original value if deploy overlap or other clients cannot connect.

`effective_cache_size` is a planner estimate; lowering it does not free allocated memory. [PostgreSQL query planning settings](https://www.postgresql.org/docs/16/runtime-config-query.html)

### Expensive query branch

- [ ] If `pg_stat_statements` is already available, compare deltas from two snapshots and investigate the queries with the largest execution time and temporary-block growth. This query intentionally omits SQL text and request parameters:

```sql
SELECT queryid, calls, total_exec_time, mean_exec_time,
       rows, temp_blks_read, temp_blks_written,
       shared_blks_read, shared_blks_hit
FROM pg_stat_statements
WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
ORDER BY temp_blks_written DESC, total_exec_time DESC
LIMIT 20;
```

It is not a direct query-memory measurement. If unavailable, use bounded activity samples and existing query-plan tests first. Enabling `pg_stat_statements` can need preload configuration and a restart; do not overwrite an existing preload list merely to install it. [pg_stat_statements](https://www.postgresql.org/docs/16/pgstatstatements.html)

- [ ] Investigate a code path only if its query fingerprint or replayed plan supports it. Existing candidates include course overview, sessions-in-range scope loading, reconciliation, and retention cleanup. Course overview already pages before its expensive per-course calculations; preserve that optimization.
- [ ] For sessions-in-range, use [sessions_range_plan_test.go](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/internal/db/sessions_range_plan_test.go>) and the existing parity tests on a disposable production-scale dataset:

```sh
go test ./internal/db -run '^TestSessionsRangeFactsPlanBounded$' -count=1 -timeout=120s
```

- [ ] Capture `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` for the selected read query on staging. Identify actual row counts, sort/hash memory, batches, disk usage, parallel workers, and repeated loops. `ANALYZE` executes the statement; do not use it on production write statements.
- [ ] Change only the identified owner query or its missing index, after checking existing migrations. Compare the plan and the response/record set before and after. Do not add speculative indexes, remove meaningful `DISTINCT`, or truncate valid session scopes merely to reduce a graph.

The exact SQL rewrite belongs to the measured query. Without production query evidence, prescribing a replacement would invent the bottleneck; this branch deliberately requires that evidence before selecting a patch.

## Task 5: Remove duplicate CRM import allocations if imports drive the app peak

**Owner:** Existing parser and snapshot insertion boundaries.

**Modify:** [xlsx_parse.go](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/internal/crmimport/xlsx/xlsx_parse.go>), [snapshot_service.go](</Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2/backend/internal/crmimport/snapshot_service.go>), their focused tests. Do not change public endpoints or snapshot lifecycle as part of the initial optimization.

- [ ] Profile one representative import and the largest supported export on staging. Record compressed file size, row count, peak service memory, elapsed time, and the active snapshot before/after. Repeat the same inputs after each patch.
- [ ] First replace eager `CopyFromRows(rowCopies(...))` with pgx's installed lazy `CopyFromSlice` API. Replace the source argument in `PopulateRows` with:

```go
pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
	return snapshotRowCopy(snapshotID, i, rows[i]), nil
}),
```

Replace `rowCopies` with the following function; keep the existing column list and `nullIfEmpty` helper:

```go
func snapshotRowCopy(snapshotID pgtype.UUID, i int, r xlsx.Row) []any {
	var hours pgtype.Int4
	if r.Hours != nil {
		hours = pgtype.Int4{Int32: *r.Hours, Valid: true}
	}
	var updated pgtype.Timestamptz
	if r.OrderQuoteUpdatedAt != nil {
		updated = pgtype.Timestamptz{Time: *r.OrderQuoteUpdatedAt, Valid: true}
	}
	return []any{
		snapshotID, int32(i + 1), r.Hash(),
		r.CycleLabel, r.CourseName, r.WCode,
		nullIfEmpty(r.FirstName), nullIfEmpty(r.LastName),
		nullIfEmpty(r.Nickname), nullIfEmpty(r.SecondarySchool),
		nullIfEmpty(r.AcademicLevel), nullIfEmpty(r.MobilePhone),
		hours, nullIfEmpty(r.TeachersRaw), nullIfEmpty(r.PrimaryEmail),
		nullIfEmpty(r.ParentName), nullIfEmpty(r.ParentPhone),
		nullIfEmpty(r.ParentEmail), updated, r.ExtraNote,
	}
}
```

This removes the full secondary insertion collection. It still retains the parsed rows and upload blob; it does not make the entire pipeline constant-memory.

- [ ] Then replace `GetRows` with the iterator already used by `sheetHasRequiredHeaders` in the same file. Excelize supports row iteration and requires closing the iterator. [Excelize Rows API](https://xuri.me/excelize/en/sheet.html#Rows)

Replace the `allRows` read and first-20-row scan with:

```go
worksheetRows, err := f.Rows(sheet)
if err != nil {
	return ParsedXLSX{}, fmt.Errorf("read rows: %w", err)
}
defer func() { _ = worksheetRows.Close() }()

for i := 0; i < scanRows && worksheetRows.Next(); i++ {
	cells, err := worksheetRows.Columns(excelize.Options{RawCellValue: true})
	if err != nil {
		return ParsedXLSX{}, fmt.Errorf("read header row: %w", err)
	}
	if looksLikeHeaderRow(cells) {
		headerRowIdx = i
		headerCells = cells
		break
	}
}
if err := worksheetRows.Error(); err != nil {
	return ParsedXLSX{}, fmt.Errorf("scan header rows: %w", err)
}
```

Keep the existing missing-header check and header mapping. Replace the data loop's opening `for` and `cells` assignment with:

```go
for worksheetRows.Next() {
	cells, err := worksheetRows.Columns(excelize.Options{RawCellValue: true})
	if err != nil {
		return ParsedXLSX{}, fmt.Errorf("read data row: %w", err)
	}
```

Keep the existing row mapping, normalization, skip rules, and append body inside that loop. Immediately after it, before the zero-usable-rows check, add:

```go
if err := worksheetRows.Error(); err != nil {
	return ParsedXLSX{}, fmt.Errorf("scan data rows: %w", err)
}
```

- [ ] Verify the first-20-row boundary, blank rows, alternate/hidden worksheets, raw dates, normalized wcodes, duplicate preservation rules, null values, row hashes, and insertion row numbers with existing fixtures and the representative large export.
- [ ] Run existing parser, deduplication, snapshot, retry, and end-to-end import tests against the disposable database:

```sh
go test ./internal/crmimport/xlsx -count=1
go test ./internal/crmimport -run '^(TestDeduplicateRows|TestSnapshot_|TestImportUpload_|TestImportSnapshotHandlerResumesReadySnapshotWithoutUploadBlob$|TestCRMPipelineEndToEnd$)' -count=1 -timeout=180s
```

- [ ] Verify malformed/truncated uploads fail without activating an incomplete snapshot; a retry of an already-ready snapshot still works after its upload blob has been removed. Preserve the 50 MiB HTTP upload limit and current stable deduplication/ordering behavior.
- [ ] Check allocation profiles and peak service memory after each patch, rather than assuming total allocated bytes equal peak resident memory.

**Remaining ceiling:** Upload bytes, parsed `[]Row`, and the deduplication collection remain proportional to input size. If those still exceed the approved budget, a further staged, streaming import needs an explicit deduplication/order contract and ZIP-expansion limits tested against the largest valid export. Do not lower the service limit or discard supported rows to hide that remaining allocation. Keep sufficient RAM for accepted imports until that additional work is designed and verified.

**Rollback:** Revert the affected parser/insertion patch; the proposed changes do not alter schema or persisted snapshot format.

## Task 6: Test Go memory limits after the workload allocations are understood

**Owner:** Deployment environment; no new dependency required.

- [ ] Measure the API and legacy child separately. Identify peak live Go memory and non-Go/runtime-excluded resident memory during ordinary activity, sync, and the largest accepted import.
- [ ] On staging, test the existing Go runtime control with `GOGC` left at its default:

```dotenv
GOMEMLIMIT=128MiB
```

This is a provisional experiment, not a production recommendation before profiles exist. `GOMEMLIMIT` is a soft runtime limit, not an RSS or container hard cap. Very low settings can cause GC CPU pressure and still be exceeded. [Go GC guide](https://go.dev/doc/gc-guide)

The server currently starts its child with inherited environment variables. Thus `128MiB` applies to each of two processes; the combined runtime budgets can be about 256 MiB plus excluded allocations and other service memory. Setting the variable does not allocate that memory and does not guarantee either process remains below its limit.

- [ ] Reject this candidate if live memory already needs most of the budget, API latency regresses, CPU rises persistently, or accepted imports fail. Use the measured live-memory floor plus headroom to select a larger value or leave the limit unset.
- [ ] Keep the setting only if warm steady-state and import tests show lower resident/average memory with acceptable CPU. Do not change `GOGC` simultaneously; investigate it separately only if a profile justifies it.

**Rollback:** Restore the previous `GOMEMLIMIT` value or absence. No database change is involved.

## Final rollout and verification

- [ ] Record each experiment independently: changed value/patch, workload, replica count, peak and average memory, p95/p99 latency, CPU, temporary I/O, queue age, sync duration, and correctness result. Do not add percentage savings from overlapping experiments.
- [ ] Run the touched Go packages and the relevant real-DB tests after the last application patch. A final build must include both Go binaries:

```sh
go build ./cmd/server ./cmd/legacy-sync
```

- [ ] Exercise ordinary course browsing, absence submission, scheduling edits, realtime delivery, CRM import, course refresh, full reconcile, and an overlapping application deployment. Use the existing frontend/E2E flows where they reach these behaviors.
- [ ] Compare at least one warm 24-hour window with similar traffic and job volume. Require zero OOM events, no lost/duplicate background effects, no lease-expiry growth, and no increasing queue backlog.
- [ ] Reduce hard Railway service limits only after the largest supported workload and deployment-overlap tests pass. Keep headroom for validated bursts; a smaller cache or soft limit is not proof a smaller hard limit is safe.
- [ ] Update the backend runbook with the effective pool/concurrency settings, measured results, and exact restoration commands. Stage only task-owned changes; this workspace contains unrelated uncommitted feature work.

## Implementation order and stop rule

1. Baseline and actual deployed configuration.
2. Existing concurrency controls, then the worker pool cap.
3. Worker idle connection release and application names.
4. Measured database/query branch, if needed.
5. CRM allocation changes, if imports contribute materially.
6. Profile-derived Go limit, if it produces further useful savings.
7. Warm comparison and release verification.

Tasks 1–3 are the first implementation slice. Expect roughly one engineering day for the slice plus a 24-hour comparison window, assuming environment access and existing test fixtures. Database restarts and additional query/import work depend on the measured remaining contributor.

Stop when the approved memory target and behavior gates are met. Additional services, PgBouncer, a database migration to another engine, recurring restarts, forced GC loops, and disabling sync/retention are not required by the evidence gathered so far. If transaction pooling is later justified by replica growth, first separate the worker's `LISTEN` and session advisory-lock connections; they currently share its ordinary pool and cannot simply be routed through a transaction pooler.
