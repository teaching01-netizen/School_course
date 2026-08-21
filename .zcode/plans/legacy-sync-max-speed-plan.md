# Legacy Sync — Maximum Speed: Detailed Implementation Plan (v2, after dual plan review)

Source: approved roadmap `.zcode/plans/legacy-sync-max-speed-roadmap.md` (v3, dual-approved).
Goal: remove artificial serialization and raise ceilings so all legacy-sync scraping modes
(sweep, full reconcile / "full reconnect", archived index, student profiles) run at the
maximum rate the safety design tolerates. All safety machinery (auth cooldown, circuit
breaker, egress budgets, queue retries, advisory locks, shadow mode, add-only roster)
is preserved.

## Landing order and ownership

R1 → R2 → R3 → R5 → R4 (R4 last: wall-clock benefit depends on the network fixes; its
tests are DB-backed). R1–R3 each touch both `internal/legacysync/client.go` and
`client_env_test.go`; apply edits sequentially and re-run the affected tests after each.

---

## Shared helpers (needed by R2/R5; add first, no behavior change)

In `backend/cmd/legacy-sync/main.go` (or a small new file `cmd/legacy-sync/tuning.go`
in package `main`):

```go
// workerConcurrency resolves LEGACY_SYNC_WORKERS, falling back to the client's
// in-flight ceiling when the env var is unset or invalid (intEnv already treats
// 0 and junk as fallback; a worker count of 0 is meaningless).
func workerConcurrency(clientMax int) int { return intEnv("LEGACY_SYNC_WORKERS", clientMax) }

// reconcileWorkers resolves LEGACY_SYNC_RECONCILE_WORKERS with a zero-preserving
// parse so 0 selects the exact serial path. Unset/junk -> default; 0/1 -> serial.
func reconcileWorkers(clientMax int) int {
    raw := os.Getenv("LEGACY_SYNC_RECONCILE_WORKERS")
    if raw == "" {
        return min(clientMax, 16)
    }
    if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
        return parsed
    }
    return min(clientMax, 16)
}

// maxPoolConns returns the pgx pool connection budget.
func maxPoolConns(env, workers int) int {
    if env > 0 {
        return env
    }
    if pool := 2 * workers; pool > 64 {
        return pool
    }
    return 64
}
```

Unit tests in `cmd/legacy-sync/main_test.go` (package main):
- `workerConcurrency`: env unset/junk → clientMax; `"3"` → 3.
- `reconcileWorkers`: unset → `min(clientMax,16)`; `"0"` → 0 (serial); `"1"` → 1;
  `"8"` → 8; `"junk"` → default.
- `maxPoolConns(0,8)` → 64; `(0,40)` → 80; `(100,8)` → 100; `(-5,8)` → 64.

---

## R1 — Default global pacing off

### Behavior change
- `minRequestIntervalFromEnv()` (`backend/internal/legacysync/client.go:103-112`):
  both fallback returns `500 * time.Millisecond` → `0`. Empty, unparseable, and
  negative env all resolve to `0` (the `parsed >= 0` guard at `:108` is unchanged).
- No change to `client/client.go` (`defaultMinRequestInterval` stays 500ms; the
  `< 0` fallback at `:128-131` is preserved for direct `sourceclient.New` users).

### Files
- `backend/internal/legacysync/client.go` (two returns),
  `backend/internal/legacysync/client_env_test.go`,
  `backend/internal/legacysync/client/client_test.go`.

### Tests (RED first)
1. Edit `TestMinRequestIntervalFromEnv` (`client_env_test.go:27-40`): unset → `0`,
   `"500ms"` → 500ms, `"junk"` → `0`, `"0"` → `0`. Runs red before the fix.
2. Apply the two-return fix; re-run → green.
3. Add a `Do`-level no-pacing test in `client/client_test.go`: stub
   `http.RoundTripper` sleeping 100ms per request; build the source client with
   `MinRequestInterval: 0`; **set `client.authenticated = true` after construction**
   (the pattern already used in `client_test.go` around line 147) so the 4 concurrent
   `Do`s hit the slot path directly without login flows; assert total wall time
   < 300ms (no 500ms slotting). Passes only because `waitForRequestSlot`
   short-circuits at `wait <= 0` (`transport.go:122-124`).

---

## R2 — Concurrency ceilings

### Behavior change
- `maxConcurrentFromEnv()` (`internal/legacysync/client.go:57-66`): default/fallback
  `16` → `32`.
- `NewClient` (`internal/legacysync/client.go:144-147`): replace the `> 8` clamp with
  `> 128` (floor `>= 1` kept).
- `sourceclient.New` (`client/client.go:117-123`): `maxConcurrent <= 0` default `2` →
  `32`; `> 8` clamp → `> 128`.
- `cmd/legacy-sync/main.go:154`: `Concurrency: workerConcurrency(client.MaxConcurrent())`
  (see Shared helpers — client exists at main.go:138, before the runner).

### Files
- `backend/internal/legacysync/client.go`, `backend/internal/legacysync/client/client.go`,
  `backend/cmd/legacy-sync/main.go` (+ `tuning.go`, + `main_test.go`),
  `backend/internal/legacysync/client_env_test.go`,
  `backend/internal/legacysync/client/client_test.go`,
  `backend/internal/legacysync/client/auth_concurrency_test.go`, README.md (~137-140).

### Tests (RED first)
1. `TestMaxConcurrentFromEnv` (`client_env_test.go:12-25`): unset → 32, `"32"` → 32,
   `"junk"` → 32, `"1000"` → 1000.
2. New `NewClient`-level test: `LEGACY_SYNC_MAX_CONCURRENT=1000` →
   `c.MaxConcurrent() == 128` (clamp); unset/junk → 32.
3. New `sourceclient.New` test: `MaxConcurrent: 1000` → `MaxConcurrent() == 128`;
   `0` → 32.
4. `auth_concurrency_test.go`: extend to `MaxConcurrent: 32` and `128`; assert exactly
   one login chain per expiry episode (generation check bounds it) and all requests
   complete.
5. `workerConcurrency` unit tests (Shared helpers section): unset/junk →
   `client.MaxConcurrent()`; `"3"` → 3. This pins the roadmap acceptance "runner
   constructed with Concurrency == client.MaxConcurrent() when LEGACY_SYNC_WORKERS
   unset" at the seam where it is testable.

---

## R3 — Budgets + profile-phase systemic guard

### Behavior change
- `maxRequestsPerMinuteFromEnv()` (`internal/legacysync/client.go:72-81`): default
  `120` → `720`.
- `maxEgressBytesPerMinuteFromEnv()` (`:87-96`): default `50<<20` → `200<<20`.
- `cmd/legacy-sync/students.go`: add (imports: stdlib `errors` +
  `sourceclient "warwick-institute/internal/legacysync/client"`):

  ```go
  func isSystemicProfileError(err error) bool {
      return errors.Is(err, sourceclient.ErrRateLimited) ||
          errors.Is(err, sourceclient.ErrEgressBudgetExceeded) ||
          errors.Is(err, sourceclient.ErrCircuitOpen) ||
          errors.Is(err, sourceclient.ErrAuthentication)
  }
  ```

  In the `syncStudentProfiles` worker loop (`students.go:85-96`), when
  `SearchStudentsPageContext` returns an error: if `isSystemicProfileError(err)`,
  record it once (mutex-guarded `firstErr`), cancel `workCtx`, send the failed
  result, and return from the worker. The aggregator drains `results` (existing
  pattern at `students.go:140-143`); after the loop, if `firstErr != nil` return
  `nil, firstErr`. Per-wcode parse failures, `StatusError` (4xx), not-found results
  and transient `ErrSourceUnavailable` keep the skip behavior.

### Files
- `backend/internal/legacysync/client.go`, `backend/cmd/legacy-sync/students.go`,
  `backend/internal/legacysync/client_env_test.go`, unit tests in
  `backend/cmd/legacy-sync/`, `backend/cmd/legacy-sync/students_integration_test.go`,
  README.md.

### Tests (RED first)
1. Env default tests for the two budget functions (unset/junk → 720 / 200 MiB).
   (These land as new tests; the current file only covers concurrency and pacing.)
2. Unit test `isSystemicProfileError` with **wrapped** errors so a wrong `errors.Is`
   target cannot pass:
   - abort: `fmt.Errorf("search students: %w", &sourceclient.EgressBudgetError{ResetAt: time.Now().Add(time.Minute)})`,
     `fmt.Errorf("search students: %w", &sourceclient.RateLimitedError{StatusCode: 429})`,
     `fmt.Errorf("search students: %w", sourceclient.ErrCircuitOpen)`,
     `fmt.Errorf("search students: %w", sourceclient.ErrAuthentication)`.
   - skip: `fmt.Errorf("search students: %w", sourceclient.ErrSourceUnavailable)`
     (a 5xx storm surfaces as this — `transport.go:84-86` — and must keep skipping
     until the breaker opens), `&sourceclient.StatusError{StatusCode: 400, Path: "/x"}`,
     a parse error, an unrelated sentinel, `nil`.
3. Integration test in `students_integration_test.go`: extend the `studentSearchServer`
   harness with a mode that returns HTTP 429 (or re-use a small inline server) —
   the 429 reply must carry `Content-Type: text/html`, otherwise
   `transport.go:76-79` returns the skip-class `ErrUnexpectedContentType` and the
   test exercises the wrong path; assert `syncStudentProfiles` returns a **non-nil
   error carrying a systemic sentinel** (e.g. `errors.Is(err, sourceclient.ErrRateLimited)`)
   when the stub 429s, and returns `nil` error with profiles on normal+skipped-failure
   runs. The reconcile-job failure and run-record "failed" marking are pinned by
   main.go's deferred logic (`main.go:188-218`: `processErr != nil` → status failed)
   and the runner's `Store.Retry` (`runner.go:210`) — verified by construction, not by
   this harness; the test asserts the phase-level contract directly. Run with
   `TEST_DATABASE_URL`.

---

## R5 — pgx pool sizing

### Behavior change (ordering matters)
Reorder `main()` in `backend/cmd/legacy-sync/main.go` so the client is constructed
**before** the pool: `legacysync.NewClient(...)` (currently at :138; it performs no
DB or network I/O — builds the transport, cookie jar, and source client) moves above
`pgxpool` construction (currently at :111). Then:

```go
workers := workerConcurrency(client.MaxConcurrent()) // one computation, reused below
poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
if err != nil { ... }
if envPool := intEnv("LEGACY_SYNC_POOL_MAX_CONNS", 0); envPool > 0 {
    poolConfig.MaxConns = int32(envPool)          // explicit env knob wins
} else if !strings.Contains(cfg.DatabaseURL, "pool_max_conns") {
    poolConfig.MaxConns = int32(maxPoolConns(0, workers)) // computed budget only when the URL never tuned it
}
pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
```

(`main.go` must import `strings` for the keyword check.)

and later `Concurrency: workers` in the runner config. `pgxpool.ParseConfig` always
sets a default `MaxConns` (`max(4, NumCPU)`), so URL presence must be detected by
inspecting `cfg.DatabaseURL` for the `pool_max_conns` keyword — never by
`MaxConns <= 0`. A URL-tuned pool is preserved exactly; env wins over both.

### Files
- `backend/cmd/legacy-sync/main.go`, `cmd/legacy-sync/main_test.go`
  (`maxPoolConns` tests in Shared helpers).

### Tests (RED first)
1. `maxPoolConns` unit tests (Shared helpers).
2. Build/vet gate; integration tests still pass under `TEST_DATABASE_URL`.

---

## R4 — Parallel full-reconcile DB phases

### Behavior change
- `FullReconcileOptions` gains `Concurrency int` (`0`/`1` = exact serial behavior).
- `cmd/legacy-sync/main.go`: `Concurrency: reconcileWorkers(client.MaxConcurrent())`
  (zero-preserving — see Shared helpers).
- `reconcile/full.go` restructure with the serial path preserved **verbatim**:

1. **Extract the per-course body** so both modes share it:

   ```go
   func (r *FullReconciler) reconcileOne(
       ctx context.Context,
       index, total int,
       course normalize.LegacyCourse,
       observedAt time.Time,
       opts FullReconcileOptions,
       stats *FullReconcileStats,
       report func(FullReconcileProgress) error, // nil = no per-item reporting
   ) error
   ```

   Contains exactly today's per-course work: `linkCourse`; the `!linked` report
   branch; `applyRoster` (when `opts.StudentEnabled && len(course.Attendees) > 0`);
   the status-mirror `UPDATE`; the archived-skip; the enqueue; and the per-course
   report **only when `report != nil`** (the caller decides, so the serial path's
   reporting is byte-identical and parallel workers never call `opts.Progress`).

2. **Serial path** (`Concurrency <= 1`): the existing loop order, calling
   `reconcileOne(index, total, ..., stats, report = indexBasedReporter(opts.Progress))`.
   The reporter closes over `index` exactly as today (`ProcessedEntities: index+1`,
   `full.go:166-175`, `212-222`) → output-identical to the current code.

3. **Parallel path** (`Concurrency > 1`):
   - **Phase A (master data)**: bounded pool over teachers+subjects; each worker runs
     the existing per-item apply with a **local** applied count; the coordinator
     receives per-worker completion events, increments a shared processed counter,
     and emits `applying_master_data` progress sequentially (merged applied total).
     First worker error cancels the pool and wins.
   - **Phase B (courses)**: bounded pool (`workers = min(Concurrency, len(ordered))`)
     over the sorted courses; each worker calls `reconcileOne` with:
     - a **local** `*FullReconcileStats` (all of `linkCourse`'s counter mutations
       already go through the stats pointer — race-free by construction),
     - `report = nil`,
     - and sends a completion event `{index int, delta FullReconcileStats}` to the
       single coordinator.
   - **Coordinator**: the only goroutine that calls `opts.Progress`. It receives
     completion events from Phase A and Phase B workers.
     **Emission policy (batched):** for each emit, the coordinator first drains
     **all completion events currently pending** in the event channel, merges their
     deltas into the running totals, then emits **one** progress callback per batch
     (`ProcessedEntities` jumps by the batch size). Under real parallelism batches of
     ≥2 occur with high probability, so parallel callbacks show deltas > 1; the
     serial path never batches (one event per course → delta exactly 1, identical to
     today's `full.go:169-171`, `214-216`). The coordinator also emits the phase
     boundaries and initial `TotalEntities` reports (`applying_master_data` and
     `reconciling_courses`) for parity with the serial path.
     **Error handling:** a progress-callback error cancels the pool, keeps draining
     events so workers unblock, and is returned — mirroring the serial path's
     abort-on-report-error (`full.go:93-96`). A worker error cancels the pool and
     wins (first error returned).
   - After the pool drains: field-wise merge of every worker's local stats into the
     returned `FullReconcileStats` — **Phase A applied counts are folded into
     `MasterData` explicitly** — with `Courses` set from `len(ordered)` (excluded
     from the sum; workers never touch it); call `resolveCodeClaimConflicts` once;
     emit one final progress report with exact totals.
   - Worker error: cancel the pool, return the first error; the job is retried by the
     queue; already-applied work is idempotent.

### Concurrency safety (invariants kept)
- Same-course/same-code work serialized by the existing advisory xact locks
  (`source:course:<id>` then `source:code:<code>`, `full.go:285-296`); lock order
  matches the apply path (`apply/course.go:99-104`, `apply/schedule.go:130`) → no
  deadlock, no duplicate links.
- `linkCourse` writes only through its `*FullReconcileStats` argument → local stats
  are race-free without atomics.
- Roster applies are outside the link tx and `ON CONFLICT DO NOTHING` → parallel adds
  idempotent (`reconcile/roster.go:55,71`).
- `opts.Progress` only ever called by the coordinator goroutine.
- Shadow mode (`full.go:140-150`) returns before Phase B in both modes — unchanged.

### Files
- `backend/internal/legacysync/reconcile/full.go`, `backend/cmd/legacy-sync/main.go`,
  `backend/internal/legacysync/reconcile/full_integration_test.go`,
  `backend/internal/legacysync/reconcile/full_test.go` (new unit file).

### Tests (RED first)
1. Existing reconcile tests (serial, `Concurrency` zero-value) stay green unchanged —
   they pin the serial path.
2. New integration test with its **own fixture** (pool `MaxConns >= Concurrency + 1`;
   the shared `fullReconcileFixture` caps at 2, `full_integration_test.go:64-65`).
   The fixture must contain **no archived-skip case** (no archived course with a
   valid `legacy_last_synced_at`), so every serial-mode Phase-B callback has delta
   exactly 1. Run the same fixture with `Concurrency: 1` and `Concurrency: 8`;
   assert:
   - final course-table state equal (multiset of codes; every legacy id linked once);
   - `FullReconcileStats` totals equal between modes (including `MasterData`);
   - exactly one `code_collision` conflict row per colliding pair (if the fixture has
     collisions), compared modulo which course lost;
   - progress callbacks never overlap (wrapper harness) and the final callback
     carries exact totals;
   - **parallelism demonstrably engages**: in the `Concurrency: 1` run every Phase-B
     callback delta is exactly 1; in the `Concurrency: 8` run at least one Phase-B
     callback shows a `ProcessedEntities` delta > 1 (the batched coordinator makes
     this reachable under true parallelism; serial can never produce it on this
     fixture). This discriminator cannot pass on the serial path, so it proves the
     pool runs concurrently.
   Run under `-race`.
3. Unit tests (no DB): `courseWorkers(concurrency, n) = min(concurrency, n)` pure
   helper used to size the pools (extract it in `full.go`); assert bounds
   (`courseWorkers(0,x)==0`, `courseWorkers(16,5)==5`, `courseWorkers(1,x)==1`).
   Serial-branch behavior and ordering are pinned by the existing green serial tests,
   and the new fixture's "every serial Phase-B delta == 1" assertion pins reporting.

---

## Cross-cutting verification (all items)

```bash
cd backend
go build ./...
go vet ./...
go test ./...                                   # unit suite; integration skips w/o TEST_DATABASE_URL
TEST_DATABASE_URL=<local-test-db> go test -count=1 -race \
  ./internal/legacysync/... ./internal/legacysync/client/... \
  ./internal/legacysync/reconcile/... ./cmd/legacy-sync/...
```

The test DB: discover the repo's documented test database (integration tests skip
without `TEST_DATABASE_URL`); create one locally if none exists.

## Risks and mitigations
- **Session-expiry convoy** (32–128 in flight): bounded to one login chain by
  `authGeneration` + `authMu`; requests stall once per expiry, then retry once.
  Covered by the extended `auth_concurrency_test.go`.
- **First-systemic-error fails the profile phase**: intended trade (profile fidelity
  over completion); the reconcile job retries via the queue with backoff. Documented
  in the roadmap.
- **Pool budget vs Postgres `max_connections`**: backpressure via normal connection
  acquisition; URL-tuned pools preserved; env knob lowers the ceiling.
- **Parallel link winner nondeterminism**: only for shared-code collisions; the
  conflict table records the loser; state converges on re-run.

## Rollback (return to today's behavior exactly)
Set, in the service env: `LEGACY_SYNC_MIN_REQUEST_INTERVAL=500ms`,
`LEGACY_SYNC_MAX_CONCURRENT=8`, `LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE=120`,
`LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE=52428800` (50 MiB — the knob parses byte
counts only; `"50M"` was never parseable, even today),
`LEGACY_SYNC_WORKERS=8`, `LEGACY_SYNC_RECONCILE_WORKERS=1`. **R5 note:** today's pool
uses pgx's default `MaxConns = max(4, NumCPU)`; after R5 an unset
`LEGACY_SYNC_POOL_MAX_CONNS` with no `pool_max_conns` in `DATABASE_URL` yields the
computed budget (≥64), so exact pre-R5 pool sizing additionally requires
`LEGACY_SYNC_POOL_MAX_CONNS=<host's max(4, NumCPU)>` (e.g. 4 on a small host).
Code paths are additive; the only behavior change absent these envs is the raised
defaults plus the profile-phase systemic-error abort (which cannot be rolled back via
env — it is the intended new safety behavior, per roadmap R3).