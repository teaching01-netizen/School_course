# Legacy Sync — Maximum Throughput Roadmap (v2, after dual review)

## Scope

Make the legacy-site scraping pipeline as fast as the safety design allows, in **all**
modes: the persistent course-refresh sweep (`legacy_refresh_course` jobs), the full
reconcile / "full reconnect" mode (`legacy_full_reconcile` job, admin "full" refresh),
the archived-course index fetch, and student-profile / roster lookups.

The legacy site is assumed fragile (ASP.NET, session-cookie auth, antiforgery tokens):
existing safety machinery (auth cooldown, circuit breaker, egress budgets, retries with
backoff, advisory-locked applies, add-only roster semantics, shadow mode) is **preserved**.
Nothing here weakens an invariant; it removes artificial serialization and raises ceilings.

## Evidence — the current throughput ceiling

1. **Global politeness pacing serializes every request.** `waitForRequestSlot`
   (`backend/internal/legacysync/client/transport.go:111-138`) enforces
   `MinRequestInterval` as a *global* slot shared by every request path (pages, searches,
   login). The shipped default comes from `minRequestIntervalFromEnv()` = **500ms**
   (`backend/internal/legacysync/client.go:103-112`), capping the whole scraper at
   **2 requests/second** regardless of concurrency. A full reconnect of ~600 course pages
   plus per-student lookups at 2 rps is the dominant wall-time cost.
2. **Concurrency is clamped to 8 in two layers.** `maxConcurrentFromEnv()` defaults to
   16 but `NewClient` clamps to 8 (`backend/internal/legacysync/client.go:144-147`) and
   `sourceclient.New` clamps to 8 again (`client/client.go:117-123`). The refresh sweep
   is additionally capped by `LEGACY_SYNC_WORKERS` default 8
   (`backend/cmd/legacy-sync/main.go:154`), which sizes the runner's sequential
   fetch→apply job processors (`runner.go:181`).
3. **Per-minute egress budget matches the 2 rps ceiling**
   (`LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE` default 120, `LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE`
   default 50 MiB, `client.go:72-96`).
4. **Full-reconcile DB phases run strictly serial** (`reconcile/full.go`): master-data
   applies (one tx per teacher/subject, `full.go:106-138`), course linking (one tx +
   two advisory locks per course, `full.go:160-223`), and roster imports per course
   (`full.go:179-183`). Each is idempotent and advisory-lock-safe, so they can run in a
   bounded worker pool.
5. **Student-profile lookups are already parallel** (`cmd/legacy-sync/students.go:50-149`,
   pool sized by `client.MaxConcurrent()`), and progress reporting is already throttled
   (`cmd/legacy-sync/progress.go:20-41`). **But** `syncStudentProfiles` logs and skips
   every failed lookup (`students.go:85-96`): under a site-wide 429, open circuit
   breaker, or exhausted budget it silently drops thousands of profiles while the
   reconcile still reports success.
6. **The pgx pool has no explicit sizing.** `pgxpool.New` uses the runtime default
   (`max(4, NumCPU)` connections); with 32 parallel workers each holding a connection
   through multi-statement applies, the pool becomes a queue.

README.md:137-140 already documents `LEGACY_SYNC_MIN_REQUEST_INTERVAL` default
`0 (disabled)` and concurrency 16 — the docs are ahead of the code; the roadmap
converges code to docs and then updates the numbers that change.

## Roadmap items

Landing order for shared files: R2 and R3 each touch `legacysync/client.go` +
`client_env_test.go` — land R2 first, then R3, then R5; R1 is confined to
`client.go` env functions; R4 is independent of all of them.

### R1 — Disable the global serialized pacing by default

- OBJECTIVE: the site-wide 2 rps slot lock stops being the throughput governor; the
  per-minute budget + concurrency semaphore become the ceilings.
- OWNED SURFACE (load-bearing): `backend/internal/legacysync/client.go`
  `minRequestIntervalFromEnv()` — **both** fallback returns (`client.go:106` empty-env
  and `:111` unparseable/negative) change from `500ms` to `0` (disabled).
  `backend/internal/legacysync/client_env_test.go` (default + junk-input expectations
  → `0`).
  Fallback semantics, precisely: at the env layer, empty, unparseable, and negative
  `LEGACY_SYNC_MIN_REQUEST_INTERVAL` all resolve to `0`; the parse guard
  (`parsed >= 0`, `client.go:108`) is unchanged, so an explicit `0` disables and a
  positive value paces exactly as today. The preserved bit is the **low-level**
  negative-input fallback in `client/client.go:128-131`
  (`config.MinRequestInterval < 0 → defaultMinRequestInterval`), which keeps
  `TestMinRequestIntervalNegativeSelectsDefault` green for direct
  `sourceclient.New` users; `defaultMinRequestInterval` in `client/client.go:16`
  stays 500ms. `NewClient` always passes the env value explicitly, so the env
  function is the only production-facing change.
- DEPENDENCIES: none. Landing order: first.
- IMPLEMENTATION INTENT: unset or junk env → interval 0 (`waitForRequestSlot`
  short-circuits at `wait <= 0`, `transport.go:122-124`); explicit positive value
  paces exactly as today; explicit `0` disables.
- ACCEPTANCE CRITERIA:
  - `LEGACY_SYNC_MIN_REQUEST_INTERVAL` unset → `minRequestIntervalFromEnv() == 0`.
  - With interval 0, N concurrent `Do` calls complete without inter-request delays
    (a sleeping RoundTripper observes no pacing).
  - Positive env value still slots requests 1:1 as today (existing pacing tests,
    which pass explicit intervals, stay green).
  - README already documents `0 (disabled)`; no README change needed for R1.
- UNHAPPY PATHS: operator sets tiny positive interval (1ms) — accepted, explicit
  choice; 429/5xx still trip the breaker; jobs still retry with backoff.
- TESTS: update `TestMinRequestIntervalFromEnv` (default 0, junk → 0); add a
  concurrency test asserting no pacing delay at interval 0.
- REAL VERIFICATION: `go test ./internal/legacysync/... ./internal/legacysync/client/...`.
- REQUIRES_DETAILED_PLAN: no.

### R2 — Raise concurrency ceilings: client caps and sweep worker count

- OBJECTIVE: the client can carry 32 in-flight requests, and the refresh sweep's
  worker pool is sized to keep that pipeline full.
- OWNED SURFACE: `backend/internal/legacysync/client.go`
  (`maxConcurrentFromEnv` default → 32, `NewClient` clamp → 128),
  `backend/internal/legacysync/client/client.go` (`New` default → 32, clamp → 128),
  `backend/cmd/legacy-sync/main.go` (runner `Concurrency` default →
  `client.MaxConcurrent()`), `client_env_test.go`, README.md (concurrency numbers).
- DEPENDENCIES: none. Landing order: second (before R3).
- IMPLEMENTATION INTENT:
  - `LEGACY_SYNC_MAX_CONCURRENT`: default 32, hard clamp 128 in both layers (replaces
    the 8 cap). `httpTransportForConcurrency` already scales idle keep-alive conns.
  - `LEGACY_SYNC_WORKERS`: when unset, default to `client.MaxConcurrent()` (so the
    sweep drains at the client's ceiling); a positive env value still overrides;
    junk falls back to the client ceiling.
- ACCEPTANCE CRITERIA:
  - `Client.MaxConcurrent()` = env value clamped to `[1,128]`; unset → 32.
  - 32 concurrent `Do` calls run concurrently against a stub server.
  - The runner is constructed with `Concurrency == client.MaxConcurrent()` when
    `LEGACY_SYNC_WORKERS` is unset/invalid, and the env value otherwise.
  - README documents 32 / 128.
- UNHAPPY PATHS: `LEGACY_SYNC_MAX_CONCURRENT=1000` → clamped 128, no error.
  Session-expiry convoy at high concurrency: a mid-burst expiry sends in-flight `Do`
  calls through one `reauthenticate` login chain (authMu + generation check bound it
  to a single login); requests stall up to one login round trip, then retry once —
  expected and bounded. New acceptance: `auth_concurrency_test.go` extended to
  `MaxConcurrent: 32` and `128` asserting exactly one login chain and bounded stall.
- TESTS: update `TestMaxConcurrentFromEnv` (default 32, junk fallback, clamp 128);
  add clamp assertions in `client/client_test.go`; extend `auth_concurrency_test.go`
  to 32/128; runner-level test that worker default follows `client.MaxConcurrent()`.
- REAL VERIFICATION: `go test ./internal/legacysync/... ./internal/legacysync/client/...`
  plus `go test -race` on the auth-concurrency and client packages.
- REQUIRES_DETAILED_PLAN: no.

### R3 — Raise egress budget defaults and fail the profile phase on systemic errors

- OBJECTIVE: sustained crawl above 2 rps with a hard per-minute cap; profile lookups
  stop silently vanishing during site-wide failures.
- OWNED SURFACE: `backend/internal/legacysync/client.go`
  (`maxRequestsPerMinuteFromEnv` default → 720, `maxEgressBytesPerMinuteFromEnv`
  default → 200 MiB), `client_env_test.go`, `backend/cmd/legacy-sync/students.go`
  (systemic-error guard), README.md.
- DEPENDENCIES: none. Landing order: third.
- IMPLEMENTATION INTENT:
  - Budget defaults: 720 req/min (~12 rps average), 200 MiB/min. Env overrides
    preserved; `reserveBudget`/`BudgetExceeded`/window-roll logic unchanged.
  - Profile phase guard: in `syncStudentProfiles`, a lookup failing with a
    **systemic** error aborts the phase (returns the error → reconcile fails → queue
    retries with backoff) instead of logging-and-skipping. The systemic class is:
    `errors.Is(err, ErrRateLimited)` (429), `errors.Is(err, ErrEgressBudgetExceeded)`
    (per-minute budget exhausted — **the sentinel, not the `EgressBudgetError` struct
    type**, whose `Is` is pointer-receiver so `errors.Is(err, EgressBudgetError{})` is
    always false), `errors.Is(err, ErrCircuitOpen)` (breaker open), and
    `errors.Is(err, ErrAuthentication)` (credentials revoked — every lookup fails
    fast through the auth cooldown). Per-wcode parse failures, ordinary HTTP status
    errors, and not-found results keep skipping (single-wcode flakiness). Implemented
    as a pure predicate `isSystemicProfileError(err) bool` in
    `backend/cmd/legacy-sync/students.go` using `errors.Is` against those four
    sentinels, applied in the worker loop; the worker cancels the remaining lookups on
    first systemic error and records it, so `syncStudentProfiles` returns the error.
- ACCEPTANCE CRITERIA:
  - Env defaults resolve to 720 / 200 MiB; junk falls back.
  - Existing budget-behavior tests (explicit small caps) stay green: admission
    denied once the window is exhausted; `BudgetExceeded()` pauses and rolls forward.
  - A unit test of `isSystemicProfileError` maps the four abort classes to abort —
    tested with **wrapped** errors (`fmt.Errorf("search students: %w", &EgressBudgetError{ResetAt: …})`,
    `&RateLimitedError{StatusCode: 429}`, `ErrCircuitOpen`, `ErrAuthentication`) so a
    wrong `errors.Is` target (e.g. the value `EgressBudgetError{}`) cannot pass — and
    per-wcode errors (parse failure, `StatusError`, empty result) to skip.
  - Under a systemic error, `syncStudentProfiles` returns an error (reconcile fails,
    job retries) instead of completing with empty profiles.
- UNHAPPY PATHS: a burst hits the window cap mid-reconcile — the DB-only reconcile
  phases are unaffected (they make no client calls); only the profile phase aborts,
  failing the job so it retries after backoff instead of silently skipping thousands.
  A site-wide 500-storm: individual requests skip until the breaker opens (3
  failures), then `ErrCircuitOpen` aborts the phase.
- TESTS: extend `client_env_test.go` (two new defaults); add
  `cmd/legacy-sync` unit tests for `isSystemicProfileError` (all four abort classes —
  rate limit, egress budget, circuit open, authentication — plus the skip classes);
  add an integration test in `students_integration_test.go` proving a systemic error
  during a search aborts the phase with an error (job retries) instead of
  completing with empty profiles; keep existing budget tests untouched.
- REAL VERIFICATION: `go test ./internal/legacysync/... ./cmd/legacy-sync/...`
  plus a `TEST_DATABASE_URL`-backed run of the cmd/legacy-sync integration suite
  (the abort-path test lives there).
- REQUIRES_DETAILED_PLAN: no.

### R4 — Parallelize the full-reconcile DB phases

- OBJECTIVE: cut the serial DB wall time of `legacy_full_reconcile` (master data,
  course linking, roster imports) with a bounded worker pool, without changing apply
  semantics or the final converged state.
- OWNED SURFACE: `backend/internal/legacysync/reconcile/full.go` (+ tests),
  `backend/cmd/legacy-sync/main.go` (passes the knob),
  `backend/internal/legacysync/reconcile/full_integration_test.go`.
- DEPENDENCIES: wall-clock gains visible after R1–R3; code independent.
- IMPLEMENTATION INTENT:
  - Add `Concurrency int` to `FullReconcileOptions`; `0`/`1` = the historical serial
    loop exactly (same order, same stats, same side effects).
  - Phase A (master data): bounded pool over teachers+subjects (per-entity advisory
    locks + snapshot-hash fast paths make them safe in parallel).
  - Phase B (link + status mirror + enqueue + roster): bounded pool over the sorted
    course list. Per-course advisory locks (`source:course:<id>` then
    `source:code:<code>`, `full.go:285-296`) serialize same-course/same-code work;
    lock ordering (course → code) matches the apply path and prevents deadlock.
    Roster applies stay outside the link tx (add-only unique-constraint idempotent).
  - Stats: each worker keeps a **local** `FullReconcileStats`; the coordinator merges
    them into the returned stats after the pool drains (no shared counters, no
    atomics needed).
  - Progress: atomic processed/failure counters; the coordinator invokes
    `opts.Progress` strictly sequentially (main.go's reporter already throttles
    writes). `resolveCodeClaimConflicts` still runs once after the loop.
  - `main.go`: `Concurrency: min(client.MaxConcurrent(), 16)`, env override
    `LEGACY_SYNC_RECONCILE_WORKERS` (0/1 = serial).
- ACCEPTANCE CRITERIA (canonical, order-agnostic for collisions):
  - `Concurrency == 1`: behavior byte-identical to today — all existing reconcile
    tests green in serial mode.
  - Distinct-code fixtures, `Concurrency > 1`: final DB state and aggregated stats
    **equal** the serial run of the same fixture.
  - Collision fixtures, `Concurrency > 1`: the multiset of final course codes is
    equal to the serial run (every legacy course linked, exactly one non-colliding
    course per code, others suffixed); exactly one `code_collision` conflict row per
    colliding pair regardless of which worker won (this is the conflict type the
    reconciler actually records — `recordCodeClaimConflict` has no call sites today);
    conflict rows compared modulo which course lost (the loser's `ExternalID` may
    differ run-to-run); aggregate counters (`LinkedByCode`, `Created`, `Suffixed`,
    `Conflicts`) equal the serial run.
  - Progress callbacks never run concurrently; a worker error cancels remaining
    work and fails the reconcile; shadow mode writes nothing.
- UNHAPPY PATHS: worker error mid-batch → cancel remaining, return first error, job
  retried by the queue; already-applied work is idempotent on retry. Same-code
  nondeterminism (winner varies run-to-run) is documented and recorded in the
  conflict table — the system already tolerates it across concurrent instances.
- TESTS:
  - New parallel-mode integration test with its **own fixture**: pool
    `MaxConns ≥ workers + 1` (the existing `fullReconcileFixture` caps at 2,
    `full_integration_test.go:64-65`, which would degenerate into connection
    queueing); same fixture run serial vs parallel; assert canonical end state +
    totals; run under `-race`.
  - Unit test: progress invocations are sequential; worker count is bounded by the
    option.
  - Existing serial tests unchanged and green (they exercise `Concurrency: 0/1`).
- REAL VERIFICATION: `TEST_DATABASE_URL`-backed `go test -race
  ./internal/legacysync/reconcile/... -run TestFullReconcile -count=1`.
- REQUIRES_DETAILED_PLAN: yes (concurrency-sensitive, multi-file).

### R5 — Size the pgx pool for parallel applies

- OBJECTIVE: parallel workers do not queue on the default connection pool.
- OWNED SURFACE: `backend/cmd/legacy-sync/main.go` (pgxpool construction).
- DEPENDENCIES: none (independent; land after R2).
- IMPLEMENTATION INTENT: build the pool with an explicit connection budget
  (target: `MaxConns = max(64, 2 * LEGACY_SYNC_WORKERS)`), preserving any pool
  parameters already present in `cfg.DatabaseURL`, with env override
  `LEGACY_SYNC_POOL_MAX_CONNS` for fine-tuning.
- ACCEPTANCE CRITERIA: the pool reports `MaxConns` at least 64 by default; env
  override honored; no other pool behavior changed.
- UNHAPPY PATHS: Postgres `max_connections` smaller than the budget — the pool
  applies backpressure via normal connection acquisition (same as today, just a
  higher ceiling); operators can lower the env knob.
- TESTS: unit test for the budget computation (`maxPoolConns(workers, env)`).
- REAL VERIFICATION: `go build ./...`; integration tests under `TEST_DATABASE_URL`.
- REQUIRES_DETAILED_PLAN: no.

## Verification gates (all items)

- `go build ./...`
- `go vet ./...`
- `go test ./...` (unit suite; integration tests skip without `TEST_DATABASE_URL`)
- `TEST_DATABASE_URL`-backed `go test -count=1 -race ./internal/legacysync/...
  ./internal/legacysync/client/... ./internal/legacysync/reconcile/...
  ./cmd/legacy-sync/...` against the local Postgres.
- Inspect the final diff for scope creep / dead code; README numbers match code.

## Out of scope (deliberate)

- Merging `CourseApplier` + `ScheduleApplier` into one transaction per course (risk to
  apply semantics and fault-point tests; small gain once R1–R3 land).
- Batch job claiming (multi-row SKIP LOCKED): DB churn is negligible next to network.
- Replacing the fixed per-minute budget window with a token bucket: `BudgetExceeded()`
  semantics (runner enqueue pause) are tied to the window.
- Frontend / HTTP API changes.