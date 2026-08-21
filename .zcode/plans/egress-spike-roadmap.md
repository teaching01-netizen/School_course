# Executable Roadmap — Legacy Sync Egress: Minimal & No-Spike (v2 — post review)

## Implementation status (branch close)
- R-EGRESS-001..R-005, R-008: implemented and verified (unit + race tests, `go vet` clean).
- R-EGRESS-006 (student profile cooldown/gate) and R-EGRESS-007 (full-reconcile cooldown): **deferred** — not in this branch's scope. The client choke point (R-002) still caps their egress, but the per-wcode/per-reconcile gates land in a follow-up.

## Scope
User request: Legacy sync scraper hitting old website must use minimal egress and must not spike egress due to provider egress pricing.
Out of scope: functional changes to parsing, schedule sync semantics, UI.

## Baseline (evidence)
- Runner sweeps every 30s and blindly enqueues N linked courses with identical RunAfter=now (runner.go:47,67,178). Dedup only covers queued/running, so completed courses resurrect every 30s -> N*2/min -> 60k-180k req/hr at N=500.
- Client wrapper disables pacing by default (minInterval 0) and ups MaxConcurrent to 16 with 2-16 MiB cap -> burst 32 MiB, no per-minute byte/request budget, no token bucket.
- Fetch is unconditional per course (scraper.go:41, syncer.go:226). Snapshot hash exists post-fetch (apply/schedule.go:147) but never gates fetch. No ETag, no If-None-Match. Form cache 5m in-memory only; course list fetch duplicates a GET.
- Retry is linear attempt*1s no jitter, no Retry-After; circuit 3->10s does not pause enqueue nor SweepEvery.
- Student profiles: per-wcode SearchStudentsPageContext for DISTINCT wcode every full reconcile, no cooldown/hash gate.
- Full reconcile always fetches course index + students regardless of staleness.

## Dependency DAG
R-002 (token bucket, Retry-After, circuit accessor, Content-Length gate) ─┐
R-001 (sweep cooldown, jitter, circuit-aware pause) ─────────────────────┤
                                                                          ├─> R-003 (dedup singleflight) ─> R-004 (pre-fetch hash gate) ─> R-006 (student) ─> R-007 (full reconcile cooldown) ─> R-005 (meter) ─> R-008 (regression gates)
Note: R-001 and R-002 can start in parallel; R-003 needs R-001/R-002 landed; R-004 needs R-001; R-005 enforcement is R-002 (R-005 is instrumentation only).

---

### R-EGRESS-001 — Eliminate N/30s sweep spike (Runner spike containment)
- ID: R-EGRESS-001
- OBJECTIVE: Sweep must not create an N-sized synchronized burst every 30s; only due courses are enqueued with jitter-spread RunAfter, and sweep itself is jittered with circuit/budget-aware pause.
- OWNED SURFACE: backend/internal/legacysync/runner.go, backend/cmd/legacy-sync/main.go (listLinkedLegacyCourses, RunnerConfig)
- DEPENDENCIES: none (parallelizable with R-002)
- IMPLEMENTATION INTENT: (a) Filter listLinkedLegacyCourses to due courses: WHERE legacy_last_synced_at IS NULL OR legacy_last_synced_at < now() - $refreshInterval (default 30m, env LEGACY_SYNC_COURSE_REFRESH_INTERVAL). Archived already-synced already excluded. (b) Enqueue with RunAfter = now + uniform jitter spread over sweep window (e.g., rand 0..SweepEvery) so N jobs do not share identical RunAfter. (c) Replace fixed Ticker(SweepEvery) with Timer loop with +/-20% jitter per iteration. (d) Add Circuit func() (bool, time.Time) injected from client.CircuitState() and EgressBudget check; when circuit open or budget exceeded, skip enqueueLinkedCourses entirely and apply adaptive SweepEvery backoff (exponential capped at e.g., 5m) for next cycle; still drain already-queued jobs with longer RunAfter. (e) Respect FetchEnabled==false -> skip enqueue.
- ACCEPTANCE CRITERIA: (observable) With N=500 and refreshInterval=30m, two consecutive sweeps 30s apart enqueue 0 jobs on second tick (proven via fake Store counting Enqueue calls). Jobs enqueued in a single sweep have RunAfter spread >= SweepEvery*0.5 between min and max. Archived already-synced courses produce 0 Enqueue calls. While circuit open, enqueue count ==0. Httptest byte counter after sweep shows <= expected not N*2.
- UNHAPPY PATHS: ListCourses DB error returns cycle error without panic and jitter remains monotonic; Leader==false skips enqueue entirely; clock skew does not break jitter; concurrent Runner workers via SKIP LOCKED do not duplicate due-course set.
- TESTS: runner pacing test with due filter, jitter spread test, archived skip test, circuit-open skip test, FetchEnabled==false skip test.
- REAL VERIFICATION: Fake Store + tick twice, assert Enqueue counts as above; go test -run TestRunnerEgressPacing -count=1 -race green.
- REQUIRES_DETAILED_PLAN: true (concurrency, jobqueue atomicity)

### R-EGRESS-002 — Hard-cap client egress rate, burst bytes, retry storm, and Content-Length gate (single choke point)
- ID: R-EGRESS-002
- OBJECTIVE: No single pod can exceed configured RPS or bytes/min; 429 Retry-After honored; oversized pages aborted before paying egress; retries thinned with exponential jitter.
- OWNED SURFACE: backend/internal/legacysync/client/client.go, backend/internal/legacysync/client/transport.go, backend/internal/legacysync/client.go, backend/internal/jobqueue/memory.go, backend/internal/jobqueue/postgres.go, backend/internal/db/legacy_job_control.go (+ sql)
- DEPENDENCIES: none (but must land before R-005)
- IMPLEMENTATION INTENT: (a) Reinstate default pacing: when LEGACY_SYNC_MIN_REQUEST_INTERVAL unset, default 500ms (or 200ms) with env override; clamp MaxConcurrent env upper bound to 8. (b) Add per-minute sliding window token bucket in client (atomic requests+bytes, wire bytes including headers) enforced in waitForRequestSlot + request(). On budget exceeded return ErrEgressBudgetExceeded without network. Wire env LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE (default 50MiB) and LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE (default 120); zero/negative disables. (c) Content-Length pre-check: if Header ContentLength > maxBodyBytes, abort before ReadAll. Clamp WithMaxBodyBytes upper bound (e.g., 16MiB). (d) Parse Retry-After header on 429 (seconds or http-date) in transport.go and return RateLimitedError{RetryAfter time.Duration}; jobqueue Retry uses max(exponential base*2^attempt + jitter, RetryAfter) for RunAfter; circuit-open jobs get RunAfter >= cooldown not 1s. (e) Expose CircuitState() (open bool, openUntil time.Time) for Runner. (f) Configure transport DisableCompression optionally or account compressed vs wire size explicitly (document and test wire size).
- ACCEPTANCE CRITERIA: Default pacing active when env unset (test asserts min gap >=400ms for parallel Do burst). Burst 16 concurrent Do respects budget: 5th request blocked when budget 4*avgBody. Content-Length > cap aborts with 0 body bytes counted. 429 with Retry-After:60 causes job RunAfter >=59s (httptest). Exponential backoff gaps grow (attempt 1 ~1s, attempt 3 ~4s) with jitter >0. CircuitState reports open correctly.
- UNHAPPY PATHS: Malformed Retry-After falls back to exponential; context canceled during waitForRequestSlot returns ctx.Err without semaphore leak; auth cooldown does not bypass rate limit; DisableCompression vs not does not double-count.
- TESTS: client_pacing_test extended, transport Content-Length gate test, Retry-After parsing test, jobqueue retry exponential+jitter test (deterministic via injected clock/rand), circuit breaker still 3->10s, budget blocking test.
- REAL VERIFICATION: httptest firing 20 parallel Do, measure inter-request gap and byte budget blocking; go test -run TestClientEgressCaps -race green.
- REQUIRES_DETAILED_PLAN: true (concurrency, atomicity, external protocol)

### R-EGRESS-003 — Deduplicate course-index fetches (singleflight + double-GET elimination)
- ID: R-EGRESS-003
- OBJECTIVE: Same course list page is never fetched twice in same window; concurrent callers share one wire fetch.
- OWNED SURFACE: backend/cmd/legacy-sync/syncer.go, backend/internal/legacysync/client.go (loadCourseForm / courseForm)
- DEPENDENCIES: R-002 (budget gate) , R-001 (sweep jitter already)
- IMPLEMENTATION INTENT: (a) Make fetchCourseList sequential-reuse: fetch plain GET once, parse token from plain page via parseCourseSearchForm instead of extra GET for archived form. (b) Guard courseIndex build with singleflight.Group keyed on "courseIndex" so concurrent loadCourseIndex/calls collapse to one wire observable. (c) Guard client form caches similarly. Cold start therefore does 2 requests (plain GET + archived POST) not 3. (d) Persist courseList hash or at least use 5m cache without thunder-herd.
- ACCEPTANCE CRITERIA: httptest counter: fetchCourseList cold does exactly 2 hits (assert via request log). Concurrent loadCourseIndex x10 produces exactly 1 wire batch (counter ==1). Archived form extraction does not issue second GET.
- UNHAPPY PATHS: Plain fetch fails -> archived also fails cleanly; singleflight error propagates; cache expiry still singleflights.
- TESTS: syncer dedup httptest counting hits, singleflight thunder-herd test.
- REAL VERIFICATION: Integration with httptest legacy server, two syncCourse parallel, assert hit count ==2 not 6.
- REQUIRES_DETAILED_PLAN: true (concurrency, external protocol)

### R-EGRESS-004 — Gate per-course detail fetch behind stored hash/cooldown (pre-fetch gate)
- ID: R-EGRESS-004
- OBJECTIVE: Unchanged course detail pages are not fetched at all; egress skipped before network.
- OWNED SURFACE: backend/cmd/legacy-sync/syncer.go (syncCourse), backend/internal/legacysync/apply/* (snapshot hash already stored), backend/internal/db (legacy_entity_snapshots / courses.legacy_source_hash read)
- DEPENDENCIES: R-001 (cooldown interval), R-003
- IMPLEMENTATION INTENT: In syncCourse, before FetchSchedulePageContext, load stored sourceHash + legacy_last_synced_at + snapshot quality for that legacyCourseID; if quality==ok && hash==storedCanonicaHash && now-lastSynced < refreshInterval, skip fetch entirely and return nil (job completes without egress). Missing hash or quality!=ok or stale -> fetch. Course index diff: if course metadata in index unchanged (teacher/subject hash) treat as supporting signal but not sole gate.
- ACCEPTANCE CRITERIA: httptest request count: second syncCourse within cooldown with unchanged remote does 0 detail fetches; changed hash does 1 fetch. Stored snapshot fast-path proven without network.
- UNHAPPY PATHS: Missing stored hash -> fetch; parse failure path not considered unchanged; shadow mode still respects gate but does not write hash; tombstone/partial quality forces fetch.
- TESTS: gate test with fake DB hash store, httptest hit count 0 vs 1, shadow mode test.
- REAL VERIFICATION: Integration httptest, two back-to-back syncCourse, assert second hit 0.
- REQUIRES_DETAILED_PLAN: true (external protocol, snapshot hash reuse)

### R-EGRESS-005 — Egress metering & observability (instrumentation only)
- ID: R-EGRESS-005
- OBJECTIVE: Every outbound byte/request is counted and observable; budget decisions are auditable.
- OWNED SURFACE: backend/internal/legacysync/client/* (meter struct), backend/cmd/legacy-sync/main.go (wire), metrics endpoint if present, structured logs
- DEPENDENCIES: R-002
- IMPLEMENTATION INTENT: Meter is the atomic counter owned by R-002's bucket; R-005 only exposes it via EgressStats() (bytesOut, requestsOut, budgetRemaining, windowResetAt), logs at debug, and /metrics if prometheus present. No enforcement here — enforcement lives in R-002. Document wire vs decompressed accounting.
- ACCEPTANCE CRITERIA: Meter counts match actual wire bytes via test (concurrent increments accurate under -race). Tiny budget 1KiB scenario blocked by R-002 is observable via meter.
- UNHAPPY PATHS: Window roll resets correctly; concurrent increments under race accurate; zero budget disables (explicit).
- TESTS: meter unit tests concurrent -race.
- REAL VERIFICATION: go test -run TestEgressMeter -race green.
- REQUIRES_DETAILED_PLAN: false

### R-EGRESS-006 — Student profile egress containment
- ID: R-EGRESS-006
- OBJECTIVE: Student directory sync does not re-fetch every wcode every reconcile; per-wcode cooldown/hash gates it.
- OWNED SURFACE: backend/cmd/legacy-sync/students.go, backend/internal/legacysync/client.go (students form), backend/internal/db (external_refs / students hash store)
- DEPENDENCIES: R-002, R-004
- IMPLEMENTATION INTENT: Add per-wcode lastFetchedAt + hash in external_refs/students; syncStudentProfiles only fetches wcodes where hash missing or lastFetched outside window (e.g., 24h) or wcode not yet hashed. Batch wcodes limited by budget: when budget exceeded, stop and re-enqueue remaining. Workers sized by R-002's MaxConcurrent budget. Fill-if-empty semantics remain but skip already-fresh wcodes.
- ACCEPTANCE CRITERIA: With 100 distinct wcodes, second reconcile within cooldown window does 0 SearchStudents fetches; changed wcode does 1. Budget exhaustion pauses remaining wcodes. httptest proves.
- UNHAPPY PATHS: Malformed wcode skipped without egress; budget exceeded mid-batch does not lose progress.
- TESTS: student gate httptest counting POSTs, budget pause test.
- REAL VERIFICATION: Integration httptest, two full reconciles back-to-back, assert second POST count 0.
- REQUIRES_DETAILED_PLAN: false (reuse pattern from R-004)

### R-EGRESS-007 — Full reconcile cooldown
- ID: R-EGRESS-007
- OBJECTIVE: Full reconcile (multi-MB course list + per-course enqueues) does not run uncontrolled; respects cooldown/hash.
- OWNED SURFACE: backend/cmd/legacy-sync/main.go (ProcessJob legacy_full_reconcile), backend/cmd/legacy-sync/syncer.go (fetchCourseList)
- DEPENDENCIES: R-003, R-005
- IMPLEMENTATION INTENT: Gate ProcessJob legacy_full_reconcile behind cooldown (env LEGACY_SYNC_FULL_RECONCILE_INTERVAL, default 6h) and course-list hash change: if now-lastFullReconcile < interval and list hash unchanged, skip heavy fetch and return early. Still allowed via admin trigger. Course-list hash stored in DB or local file; reconcile still respects budget.
- ACCEPTANCE CRITERIA: Second full reconcile within cooldown hits network 0 times; after interval or hash change hits 2. Admin override bypasses.
- UNHAPPY PATHS: Missing stored hash -> fetch; concurrent reconcile via SKIP LOCKED only one runs.
- TESTS: httptest proving 0 hits within window.
- REAL VERIFICATION: Integration.
- REQUIRES_DETAILED_PLAN: false

### R-EGRESS-008 — Spike & budget regression gates (verification artifact)
- ID: R-EGRESS-008
- OBJECTIVE: Provider-cost regressions are caught by CI, not production.
- OWNED SURFACE: backend/internal/legacysync/*_test.go, backend/cmd/legacy-sync/*_test.go
- DEPENDENCIES: R-001..R-007
- IMPLEMENTATION INTENT: Add TestEgress* regression tests asserting: (a) sweep N=500 simulation egress <= threshold, (b) burst bytes <= budget, (c) Retry-After honored, (d) dedup hit count exact, (e) student gate 0 hits. Deterministic via injected clock/rand, tolerance documented.
- ACCEPTANCE CRITERIA: go test ./... -run TestEgress -count=1 green; thresholds documented relative to provider pricing estimate (MiB/min).
- UNHAPPY PATHS: flaky timing handled with injected clock.
- TESTS: itself
- REAL VERIFICATION: go test evidence exit 0.
- REQUIRES_DETAILED_PLAN: false
