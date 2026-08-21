# Detailed Plan — Egress Minimal & No-Spike (v3 — repaired per dual review)

## Invariants
- No parsing/schedule semantics change; only when/how fetches happen.
- Upgrade-safe defaults: egress never higher than before.
- Concurrency: SKIP LOCKED, semaphore+singleflight, jobqueue SQL atomic.
- Additive filters; existing LegacySyncControl/snapshot hashes untouched.

## R-001 — Runner spike containment (repaired)
Files: backend/internal/legacysync/runner.go, backend/cmd/legacy-sync/main.go
- RunnerConfig adds:
  - RefreshInterval time.Duration (env LEGACY_SYNC_COURSE_REFRESH_INTERVAL default 30m via durationEnv)
  - Circuit func() (bool, time.Time) from client.CircuitState (breakerMu read)
  - Now func() time.Time + Rand func(int) int injected for tests (default time.Now, math/rand)
- listLinkedLegacyCourses(ctx, pool, refreshInterval): SQL `WHERE legacy_course_id IS NOT NULL AND NOT (legacy_archived AND legacy_last_synced_at IS NOT NULL) AND (legacy_last_synced_at IS NULL OR legacy_last_synced_at < now() - $1::interval)` pass refreshInterval.
- enqueueLinkedCourses: for each dueID RunAfter = now + jitter where jitter = Rand(SweepEvery) uniform; first enqueue spread over window. Note: ON CONFLICT `run_after=LEAST(existing.run_after, EXCLUDED.run_after)` means re-enqueue within queued window collapses to earliest — documented, spread only on first admission.
- Run loop: replace fixed NewTicker(SweepEvery) with loop using NewTimer(SweepEvery * jitterFactor) where jitterFactor = 1 + rand(-0.2,+0.2) per iteration; requires no lock on SweepEvery mutation, compute per-iteration duration locally.
- cycle(): read Controls; if !DetectionEnabled skip enqueue; if Circuit() open -> skip enqueueLinkedCourses entirely (no DB churn); still drain queued jobs but their Do will get ErrCircuitOpen and jobqueue Retry will use cooldown floor 10s (see R-002). If EgressBudget exhausted (client returns ErrEgressBudgetExceeded) also skip enqueue. Adaptive: consecutive cycle errors >2 doubles next sleep capped 5m with jitter.
- Acceptance: N=500, two sweeps 30s apart with refresh 30m -> 0 enqueues second tick (due filter); single sweep jobs RunAfter spread max-min >= SweepEvery*0.4; archived already-synced 0; circuit-open 0; deterministic via injected Now/Rand.
- Unhappy: ListCourses DB error -> cycle error, next sleep jittered; Leader false -> skip; clock skew monotonic via time.Since not wall diff.

## R-002 — Client choke point (repaired — single enforcement)
Files: backend/internal/legacysync/client/client.go, transport.go, client.go, jobqueue/memory.go, postgres.go, db/legacy_job_control.go + queries
- Config: MaxRequestsPerMinute, MaxEgressBytesPerMinute from env LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE default 120, LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE default 50MiB; zero disables; MaxConcurrent clamped <=8 via maxConcurrentFromEnv.
- Pacing: when LEGACY_SYNC_MIN_REQUEST_INTERVAL unset default 500ms (change wrapper minRequestIntervalFromEnv to return 500ms not 0); keep per-Client rateMu/nextRequestAt.
- Bucket: Client fields bucketMu sync.Mutex, bucketWindowStart time.Time, bucketReqCount int, bucketByteCount int64, bucketMaxReq int, bucketMaxBytes int64. Helpers:
  - reserveBudgetLocked(now) checks if now-bucketWindowStart >=1m reset; if bucketReqCount+1 > maxReq or byteCount+estimated > maxBytes -> return ErrEgressBudgetExceeded{ResetAt: bucketWindowStart+1m} without network; else increment reqCount and return.
  - recordBytes(n int) adds to byteCount (under bucketMu) after read; also called for header bytes estimate.
  - Enforced at top of request() before httpClient.Do (reserve) and after ReadAll (record actual). If over budget return without Do and ensure semaphore slot released (defer release in Do handles).
  - Ordering: acquire semaphore -> reserve budget -> waitForRequestSlot; on budget error release semaphore explicitly before return; waitForRequestSlot on ctx cancel must not leak nextRequestAt — capture previous nextRequestAt and restore on ctx error.
- Transport: before ReadAll if resp.ContentLength >0 && resp.ContentLength > maxBodyBytes -> discard body, return ErrResponseTooLarge with body bytes limited to headers; for ContentLength==-1 (chunked) rely on LimitReader maxBodyBytes+1 abort.
- RateLimitedError: type RateLimitedError struct { RetryAfter time.Duration; StatusCode int; Headers http.Header } implements error + Is(ErrRateLimited)=true via Is method; transport parses Retry-After header: if integer seconds parse, else http.ParseTime http.TimeFormat -> duration = retryDate.Sub(now). On 429 return &RateLimitedError{RetryAfter: dur}.
- Do: checkCircuit -> acquire -> ensureAuthenticated -> try doOnce; doOnce maps transport RateLimitedError through; recordFailure uses errors.Is(ErrRateLimited) still true.
- CircuitState(): breakerMu read returns (open bool, openUntil time.Time) for Runner.
- WithMaxBodyBytes clamps to 16MiB upper bound.
- Jobqueue Retry: new query legacy_job_control: LegacyJobRetryWithBackoff(ctx, id, workerID, runAfter timestamptz, lastError) with SQL `run_after = $4` instead of now()+interval; Go computes runAfter = now + delay where delay = max(exponential+jitter, RetryAfter, circuitFloor). Exponential: base 1s << min(attempt,6) + jitter 0..500ms. Circuit floor 10s when errors.Is(err, ErrCircuitOpen) or ErrAuthentication. MemoryStore same logic using injected Now/Rand. PostgresStore.Retry extracts RetryAfter via errors.As(RateLimitedError) from cause error string? Need to pass typed error not string — store lastError string but delay derived from cause before stringify. Add helper parseRetryAfter(cause).
- Wire vs decompressed: document accounting as decompressed body bytes + header estimate; optionally set Transport.DisableCompression=false but count after decompress (current). Header bytes counted via len(header canonical) estimate.

## R-003 — Dedup course-index (repaired)
Files: backend/cmd/legacy-sync/syncer.go, backend/internal/legacysync/client.go
- Dep: golang.org/x/sync/singleflight (add to go.mod)
- syncer.courseSyncer adds sf singleflight.Group.
- Change fetchCourseList to singleflighted: sf.Do("courseIndex", func(){ plainPage GET; archivedPage via token reuse }) wrapping whole build.
- Dedup double-GET: fetch plainPage first (GET /Admin/Courses). Then derive archived form token by parsing plainPage HTML via new helper extractCourseSearchToken(html) (extracts __RequestVerificationToken). Then submit archived search POST using that token directly via new Client method SubmitCourseSearchWithToken(token) -> no second GET. If token extraction fails, fall back to one GET for form (still at most 2 GETs total, not 3). loadCourseIndex double-checked lock: check cache unlocked fast path -> sf.Do -> set cache under lock with courseListAt.
- Client form caches similarly singleflighted behind formMu.

## R-004 — Pre-fetch cooldown gate (repaired — staleness documented)
Files: backend/cmd/legacy-sync/syncer.go, backend/internal/db (query LegacySnapshotGet)
- Decision: v1 is time-based cooldown gate, not remote hash compare. Document 30m staleness tradeoff: if remote changed within cooldown, staleness up to RefreshInterval; acceptable for egress vs freshness tradeoff; future ETag/If-None-Match extension noted.
- syncCourse before any fetch: after findLinkedLegacyCourse, query legacy_entity_snapshots for that course (source='legacy_warwick', entity_type='course', external_id=legacyID) fallback to courses.legacy_source_hash; also have lastSynced from linked.lastSyncedAt. If quality=="ok" && storedHash!="" && time.Since(lastSynced) < refreshInterval { skip fetch, return nil } else proceed to FetchSchedulePageContext.
- Reorder syncCourse to query snapshot before Fetch. Acceptance amended: within cooldown 0 fetches regardless of remote change; after cooldown expiry 1 fetch; changed remote after cooldown correctly fetched. Test for "changed hash -> 1 fetch" means after cooldown or with forced hash mismatch simulation via DB hash cleared.
- Missing hash -> fetch; quality!=ok -> fetch; shadowMode still gated.

## Metrics (R-005)
- client.EgressStats() returns (Requests, Bytes, RemainingReq, RemainingBytes, ResetAt) reading bucketMu; no enforcement, observability only.

## Test-first sequence (repaired DAG)
1. Add x/sync dep, define RateLimitedError, EgressStats, CircuitState.
2. RED: client budget + Retry-After + circuit tests.
3. Implement R-002 bucket+Content-Length+RateLimitedError -> GREEN.
4. Implement jobqueue exponential Retry with new query -> GREEN.
5. RED: runner pacing with injected Now/Rand, circuit skip.
6. Implement R-001 due filter + jitter Timer + due query -> GREEN.
7. Implement R-003 singleflight dedup -> GREEN (2 hits cold).
8. Implement R-004 cooldown gate -> GREEN (0 hits within window).
9. go vet / go test -race on affected pkgs.

## Verification
- go test ./backend/internal/legacysync/client -run TestEgress -count=1
- go test ./backend/internal/legacysync -run TestRunner -count=1
- go test ./backend/cmd/legacy-sync -run TestSyncer -count=1
- go vet ./backend/...

## Rollback
Revert env or code; no data loss.
