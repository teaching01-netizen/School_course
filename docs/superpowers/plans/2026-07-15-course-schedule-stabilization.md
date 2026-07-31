# Course Schedule Stabilization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the confirmed course-schedule corruption, destructive-series, Slot Finder, idempotency, recurrence-exhaustion, and high-concurrency defects without redesigning realtime, authorization, or bulk-update semantics.

**Architecture:** Keep the existing React → Go HTTP → scheduling/series services → PostgreSQL structure. Move correctness into small shared recurrence, slot, lock, availability, and series-invariant helpers; use read-committed transactions with deterministic parent-row lock ordering and exclusion constraints as final overlap gates. Apply online indexes before the behavior release and prove every defect with a failing test before implementation.

**Tech Stack:** React 18, TypeScript, Vitest, Go 1.25, pgx v5, sqlc 1.29, PostgreSQL 16, Goose 3.27, GitHub Actions.

---

## File map

**Create**

- `backend/internal/series/limits.go` — recurrence constants and typed validation errors.
- `backend/internal/series/split_bounds.go` — pure count partition and cutoff decisions.
- `backend/internal/series/split_bounds_test.go` — deterministic series-split unit coverage.
- `backend/internal/scheduling/slot_finder.go` — safe ordinality and slot generation.
- `backend/internal/scheduling/slot_finder_test.go` — pure Slot Finder regressions.
- `backend/internal/schedulelock/locks.go` — dependency-light canonical resource lock ordering shared by scheduling, series, CRM, and legacy sync.
- `backend/internal/schedulelock/locks_test.go` — normalization and lock-order unit coverage.
- `backend/internal/scheduling/availability.go` — transactional availability mutations and union validation.
- `backend/internal/scheduling/schedule_invariants_integration_test.go` — concurrency and series invariants.
- `backend/db/queries/scheduling_locks.sql` — ordered parent-row lock queries.
- `backend/db/migrations/00070_availability_union_policy.sql` — forward-only replacement of applied availability trigger logic.
- `backend/db/migrations/00071_schedule_stabilization_indexes.sql` — concurrent hot-path indexes.
- `backend/internal/db/schedule_indexes_integration_test.go` — index existence and representative plan checks.
- `backend/internal/httpapi/serieshttp/routes_integration_test.go` — series HTTP status contracts.
- `backend/internal/httpapi/availabilityhttp/routes_integration_test.go` — availability conflict contract.
- `src/features/scheduling/recurrenceLimits.ts` — frontend recurrence constants and validators.
- `src/components/__tests__/SeriesFormFields.test.tsx` — recurrence input limits.
- `src/components/__tests__/SessionActions.test.tsx` — historical action visibility.
- `src/pages/__tests__/Schedule.seriesStabilization.test.tsx` — edit-series integration behavior.

**Modify**

- `backend/internal/series/materialize.go`, `materialize_test.go`, `service.go`.
- `backend/internal/scheduling/service.go`, `student_roster.go`, `session_roster.go`.
- `backend/internal/httpapi/httpadapter/adapter.go`, `adapter_test.go`.
- `backend/internal/httpapi/sessionshttp/routes.go`, `routes_test.go`.
- `backend/internal/httpapi/serieshttp/routes.go`.
- `backend/internal/httpapi/schedulinghttp/routes.go`, `routes_preflight_test.go`.
- `backend/internal/httpapi/availabilityhttp/routes.go`.
- `backend/internal/httpapi/courseshttp/routes.go`.
- `backend/internal/legacysync/syncer.go`, `syncer_test.go`.
- `backend/db/queries/sessions.sql`, `series.sql`, `availability.sql` and generated sqlc files.
- `backend/Makefile`, `scripts/validate-migrations.sh`, `.github/workflows/test.yml`.
- `src/features/scheduling/hooks/usePreflight.ts`, `src/test/usePreflight.test.ts`.
- `src/components/SeriesFormFields.tsx`, `src/components/SessionActions.tsx`.
- `src/utils/preflight.ts`, `src/pages/Schedule.tsx`, `src/pages/CourseDetail.tsx`.
- `package.json`, `docs/idempotency.md`, `docs/migrations.md`.

Do not modify the tracked stale sibling `backend/internal/scheduling/service.go.modified`.

## Task 1: Bound and cancel recurrence materialization

**Files:**

- Create: `backend/internal/series/limits.go`
- Modify: `backend/internal/series/materialize.go`
- Modify: `backend/internal/series/materialize_test.go`
- Modify callers: `backend/internal/series/service.go`, `backend/internal/scheduling/service.go`
- Modify HTTP mapping: `backend/internal/httpapi/serieshttp/routes.go`, `backend/internal/httpapi/schedulinghttp/routes.go`

- [ ] **Step 1: Change existing tests to call `Materialize(context.Background(), input)`, then add failing boundary tests**

```go
func TestMaterialize_RejectsMoreThan1000Occurrences(t *testing.T) {
	count := MaxOccurrences + 1
	_, err := Materialize(context.Background(), MaterializeInput{
		Weekdays: []time.Weekday{time.Monday}, StartDate: date(2026, 8, 3), Count: &count,
		StartLocalTime: mustClock("09:00"), DurationMinutes: 60, Location: mustBangkok(t),
	})
	assertValidationCode(t, err, "recurrence_too_large")
}

func TestMaterialize_StopsAtEarlierOfCountAndEndDate(t *testing.T) {
	count := 10
	end := date(2026, 8, 10)
	got, err := Materialize(context.Background(), MaterializeInput{
		Weekdays: []time.Weekday{time.Monday}, StartDate: date(2026, 8, 3), EndDate: &end, Count: &count,
		StartLocalTime: mustClock("09:00"), DurationMinutes: 60, Location: mustBangkok(t),
	})
	if err != nil || len(got) != 2 { t.Fatalf("len=%d err=%v", len(got), err) }
}

func TestMaterialize_ReturnsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	count := 20
	_, err := Materialize(ctx, MaterializeInput{
		Weekdays: []time.Weekday{time.Monday}, StartDate: date(2026, 8, 3), Count: &count,
		StartLocalTime: mustClock("09:00"), DurationMinutes: 60, Location: mustBangkok(t),
	})
	if !errors.Is(err, context.Canceled) { t.Fatalf("err=%v", err) }
}
```

Also add exact-limit tests: 1,000 succeeds, 1,001 fails, five calendar years succeeds, one day beyond fails, sparse weekly count beyond the horizon fails, 1,440-minute duration succeeds, and 1,441 fails.

- [ ] **Step 2: Run RED recurrence tests**

Run: `go -C backend test ./internal/series -run 'TestMaterialize_' -count=1`

Expected: compile failure because `Materialize` does not accept context and limit symbols do not exist.

- [ ] **Step 3: Add typed limits and context-aware materialization**

```go
package series

import "fmt"

const (
	MaxOccurrences     = 1000
	MaxHorizonYears    = 5
	MaxDurationMinutes = 24 * 60
)

type ValidationError struct { Code, Message string }
func (e *ValidationError) Error() string { return e.Message }
func invalid(code, format string, args ...any) error {
	return &ValidationError{Code: code, Message: fmt.Sprintf(format, args...)}
}
```

Change `Materialize` to `func Materialize(ctx context.Context, in MaterializeInput)`. Validate positive and bounded duration/count before allocation, reject an explicit end date after `start.AddDate(5,0,0)`, check `ctx.Err()` every loop iteration, and return `recurrence_horizon_exceeded` if a count cannot be satisfied within the five-year boundary. Never silently truncate.

- [ ] **Step 4: Pass request context at every production call site and map validation errors to HTTP 400**

Use the pointer form required by `errors.As` in series write and preflight routes; do not route these errors through `ClassifyDBErr`:

```go
var validationErr *series.ValidationError
if errors.As(err, &validationErr) {
	s.a.WriteErr(w, http.StatusBadRequest, "invalid_recurrence", validationErr.Message)
	return
}
```

Run: `rg -n '\bMaterialize\(' backend --glob '*.go'`

Expected: every production call passes its existing `ctx`; tests pass `context.Background()` or a canceled context.

Run: `go -C backend test ./internal/series ./internal/scheduling ./internal/httpapi/serieshttp ./internal/httpapi/schedulinghttp -count=1`

Expected: PASS.

- [ ] **Step 5: Commit Task 1**

```bash
git add backend/internal/series backend/internal/scheduling/service.go backend/internal/httpapi/serieshttp/routes.go backend/internal/httpapi/schedulinghttp/routes.go
git commit -m "fix: bound recurrence materialization"
```

## Task 2: Correct Slot Finder indexing and working-hour boundaries

**Files:**

- Create: `backend/internal/scheduling/slot_finder.go`
- Create: `backend/internal/scheduling/slot_finder_test.go`
- Modify: `backend/internal/scheduling/service.go`

- [ ] **Step 1: Add failing pure tests**

```go
func TestOrdinalityToIndex_ConvertsOneBasedValues(t *testing.T) {
	for ordinal, want := range map[int64]int{1: 0, 2: 1, 4: 3} {
		got, ok := ordinalityToIndex(ordinal, 4)
		if !ok || got != want { t.Fatalf("ordinal=%d got=%d ok=%v", ordinal, got, ok) }
	}
}

func TestOrdinalityToIndex_RejectsZeroAndOutOfRange(t *testing.T) {
	for _, ordinal := range []int64{0, -1, 5} {
		if _, ok := ordinalityToIndex(ordinal, 4); ok { t.Fatalf("accepted %d", ordinal) }
	}
}

func TestGenerateHourlySlots_DoesNotEndAfterDayEnd(t *testing.T) {
	for _, duration := range []int{30, 60, 90, 120} {
		got := generateHourlySlots(time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), 8, 20, duration)
		for _, slot := range got {
			if slot.End.Hour() > 20 || (slot.End.Hour() == 20 && slot.End.Minute() > 0) { t.Fatalf("duration=%d slot=%v", duration, slot) }
		}
	}
}
```

- [ ] **Step 2: Run RED Slot Finder tests**

Run: `go -C backend test ./internal/scheduling -run 'TestOrdinality|TestGenerateHourlySlots' -count=1`

Expected: compile failure because helpers do not exist.

- [ ] **Step 3: Implement safe helpers and use them for all three SQL result loops**

```go
func ordinalityToIndex(ordinal int64, size int) (int, bool) {
	idx := int(ordinal - 1)
	return idx, ordinal > 0 && idx >= 0 && idx < size
}

type candidateSlot struct { Start, End time.Time }

func generateHourlySlots(day time.Time, startHour, endHour, durationMinutes int) []candidateSlot {
	dayEnd := time.Date(day.Year(), day.Month(), day.Day(), endHour, 0, 0, 0, day.Location())
	var out []candidateSlot
	for hour := startHour; hour < endHour; hour++ {
		start := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, day.Location())
		end := start.Add(time.Duration(durationMinutes) * time.Minute)
		if end.After(dayEnd) { continue }
		out = append(out, candidateSlot{Start: start, End: end})
	}
	return out
}
```

Scan ordinality into `int64`, convert with the helper, return a descriptive internal error for an impossible out-of-range database result, and never index `blocked` directly with the SQL ordinal.

- [ ] **Step 4: Verify GREEN and full scheduling package**

Run: `go -C backend test ./internal/scheduling -count=1`

Expected: PASS.

- [ ] **Step 5: Commit Task 2**

```bash
git add backend/internal/scheduling/slot_finder.go backend/internal/scheduling/slot_finder_test.go backend/internal/scheduling/service.go
git commit -m "fix: align slot finder conflicts"
```

## Task 3: Fix frontend series preflight and recurrence controls

**Files:**

- Create: `src/features/scheduling/recurrenceLimits.ts`
- Create: `src/components/__tests__/SeriesFormFields.test.tsx`
- Create: `src/components/__tests__/SessionActions.test.tsx`
- Modify: `src/features/scheduling/hooks/usePreflight.ts`, `src/test/usePreflight.test.ts`
- Modify: `src/components/SeriesFormFields.tsx`, `src/components/SessionActions.tsx`
- Modify: `src/utils/preflight.ts`, `src/pages/Schedule.tsx`, `src/pages/CourseDetail.tsx`

- [ ] **Step 1: Add failing series-ID, limits, and historical-action tests**

```ts
it("series preflight includes series_id when supplied", async () => {
  mockApiJson.mockResolvedValue({ status: "available", occurrences_planned: 3 });
  const { result } = renderHook(() => usePreflight("preflight_series"));
  await act(() => result.current.check({
    series_id: "series-1", course_id: "course-1", teacher_id: "teacher-1", room_id: null,
    weekdays: [1], start_local_time: "09:00", duration_minutes: 60,
    start_date: "2026-08-03", count: 10, start_at: "", end_at: "",
  }));
  expect(JSON.parse(mockApiJson.mock.calls[0][1].body).series_id).toBe("series-1");
});
```

In `SessionActions.test.tsx`, freeze time with `vi.setSystemTime("2026-08-03T10:00:00Z")`; assert “This & Future” is absent for a session starting at or before that instant and present for a future series session.

- [ ] **Step 2: Run RED frontend tests**

Run: `npm test -- src/test/usePreflight.test.ts src/components/__tests__/SeriesFormFields.test.tsx src/components/__tests__/SessionActions.test.tsx`

Expected: series ID assertion fails and new component tests fail.

- [ ] **Step 3: Implement shared frontend limits and payload/action behavior**

```ts
export const MAX_SERIES_OCCURRENCES = 1000;
export const MAX_SERIES_HORIZON_YEARS = 5;
export const MAX_SESSION_DURATION_MINUTES = 24 * 60;

export function isFutureSession(startAt: string, nowMs = Date.now()): boolean {
  const startMs = Date.parse(startAt);
  return Number.isFinite(startMs) && startMs > nowMs;
}
```

Include `series_id: params.series_id ?? null` in the series preflight body. Pass the current series ID from both edit-series preflight calls. Apply `min={1}`/`max` attributes in `SeriesFormFields`, enforce the same limits in `validateSeriesPreflight`, and hide “This & Future” unless `isSeries && isFutureSession(session.start_at)`.

- [ ] **Step 4: Add the focused schedule test script and verify GREEN**

Add to `package.json`:

```json
"test:schedule": "vitest run src/pages/__tests__/Schedule.test.tsx src/pages/__tests__/Schedule.seriesStabilization.test.tsx src/components/__tests__/SessionActions.test.tsx src/components/__tests__/SeriesFormFields.test.tsx src/test/usePreflight.test.ts src/pages/__tests__/SlotFinder.test.tsx"
```

Run: `npm run typecheck && npm run test:schedule`

Expected: PASS.

- [ ] **Step 5: Commit Task 3**

```bash
git add package.json src/features/scheduling src/components src/utils/preflight.ts src/pages/Schedule.tsx src/pages/CourseDetail.tsx src/test/usePreflight.test.ts
git commit -m "fix: validate recurring schedule edits"
```

## Task 4: Restore exact-body idempotency and replay-before-CAS

**Files:**

- Modify: `backend/internal/httpapi/httpadapter/adapter.go`, `adapter_test.go`
- Modify: `backend/internal/httpapi/sessionshttp/routes.go`, `routes_test.go`
- Modify: `backend/internal/httpapi/serieshttp/routes.go`, `routes_integration_test.go`
- Modify: `docs/idempotency.md`

- [ ] **Step 1: Add failing decode/restoration and schedule replay tests**

```go
func TestDecodeJSON_RestoresExactOriginalBody(t *testing.T) {
	original := []byte("{\n  \"course_id\": \"course-1\"\n}")
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader(original))
	w := httptest.NewRecorder()
	var body map[string]any
	if err := (Adapter{}).DecodeJSON(w, r, &body); err != nil { t.Fatal(err) }
	restored, err := io.ReadAll(r.Body)
	if err != nil { t.Fatal(err) }
	if !bytes.Equal(restored, original) { t.Fatalf("got=%q want=%q", restored, original) }
}
```

Add database HTTP tests named `TestScheduleDB_PostSession_ReplaysSameKeySameExactBody`, `TestScheduleDB_PostSession_RejectsSameKeyDifferentBody`, `TestScheduleDB_PatchSession_ReplayPrecedesStaleVersion`, `TestScheduleDB_DeleteSession_ReplayPrecedesNotFound`, `TestScheduleDB_SplitSeries_ReplayPrecedesStaleVersion`, `TestScheduleDB_CancelSeries_ReplayPrecedesStaleVersion`, and `TestScheduleDB_EditEntireSeries_ReplayPrecedesStaleVersion`. Each replay test sends the identical actor, path, key, and raw body twice and asserts the second status and JSON body exactly equal the first response.

Use this shared HTTP helper so every RED test exercises exact raw bytes:

```go
func serveMutation(t *testing.T, mux http.Handler, method, path, key string, body []byte) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w.Code, append([]byte(nil), w.Body.Bytes()...)
}

func assertReplay(t *testing.T, mux http.Handler, method, path, key string, body []byte) {
	t.Helper()
	status1, response1 := serveMutation(t, mux, method, path, key, body)
	status2, response2 := serveMutation(t, mux, method, path, key, body)
	if status2 != status1 || !bytes.Equal(response2, response1) {
		t.Fatalf("replay=(%d,%s) original=(%d,%s)", status2, response2, status1, response1)
	}
}
```

- [ ] **Step 2: Run RED adapter and HTTP tests**

Run: `go -C backend test ./internal/httpapi/httpadapter -run TestDecodeJSON_RestoresExactOriginalBody -count=1`

Expected: FAIL because the restored body is empty.

Run with PostgreSQL configured:

```bash
TEST_DATABASE_URL="$TEST_DATABASE_URL" go -C backend test -p 1 \
  ./internal/httpapi/sessionshttp ./internal/httpapi/serieshttp \
  -run 'TestScheduleDB_(PostSession_ReplaysSameKeySameExactBody|PostSession_RejectsSameKeyDifferentBody|PatchSession_ReplayPrecedesStaleVersion|DeleteSession_ReplayPrecedesNotFound|SplitSeries_ReplayPrecedesStaleVersion|CancelSeries_ReplayPrecedesStaleVersion|EditEntireSeries_ReplayPrecedesStaleVersion)' \
  -count=1 -timeout=120s
```

Expected: the different-body test demonstrates the consumed-body fingerprint failure, and replay-before-CAS tests return `stale_edit` or `not_found` instead of the original cached response. Do not begin Step 3 until both failure modes have been observed.

- [ ] **Step 3: Decode from bounded bytes and restore the body**

```go
func (Adapter) DecodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	limited := http.MaxBytesReader(w, r.Body, 2*1024*1024)
	body, err := io.ReadAll(limited)
	if err != nil { return err }
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(v); err != nil { return err }
	return nil
}
```

- [ ] **Step 4: Move session/series CAS reads inside idempotent callbacks**

Decode and syntactically validate before the wrapper, acquire idempotency, then use `qtx.SessionGetByID`/`qtx.SeriesGetByID` and version checks inside the callback. Apply this ordering to `handleSessionsCreate`, `handleSessionsDelete`, `handleSessionEditOccurrence`, `handleSessionAttendanceUpsert`, `handleSessionAttendanceDelete`, `handleSeriesCreate`, `handleSeriesSplit`, `handleSeriesCancel`, and `handleSeriesEditEntire`. The create/attendance routes have no CAS read but still require body restoration and idempotency acquisition before durable work. Change schedule create/edit wrappers from `WithSerializableIdempotentTx` to `WithIdempotentTx`; Tasks 5–8 supply the explicit lock protocol before any schedule preflight/write.

> **Sequencing adjustment:** retain `WithSerializableIdempotentTx` for session create/edit through Task 4. Task 5 must switch these routes to `WithIdempotentTx` atomically with canonical resource locks and savepoint-safe writes, so no intermediate commit exposes unsafe read-committed schedule writes.

Document exact-byte fingerprints and committed-mutation-only caching in `docs/idempotency.md`.

- [ ] **Step 5: Verify GREEN**

Run: `go -C backend test ./internal/httpapi/httpadapter ./internal/httpapi/sessionshttp ./internal/httpapi/serieshttp -count=1`

Expected: PASS with `TEST_DATABASE_URL` configured; local non-DB tests pass otherwise.

- [ ] **Step 6: Commit Task 4**

```bash
git add backend/internal/httpapi docs/idempotency.md
git commit -m "fix: preserve schedule idempotency fingerprints"
```

## Task 5: Add cross-package canonical resource locks and savepoint-safe writes

**Files:**

- Create: `backend/db/queries/scheduling_locks.sql`
- Create: `backend/internal/schedulelock/locks.go`, `locks_test.go`
- Modify generated: `backend/internal/db/scheduling_locks.sql.go`
- Modify later consumers: `backend/internal/scheduling`, `backend/internal/series`, `backend/internal/crmimport`, `backend/internal/legacysync`

- [ ] **Step 1: Add failing lock-order unit and integration tests**

```go
func TestNormalizeLockIDs_SortsDeduplicatesAndDropsNull(t *testing.T) {
	a, b := mustUUID("00000000-0000-0000-0000-000000000001"), mustUUID("00000000-0000-0000-0000-000000000002")
	got := normalizeLockIDs([]pgtype.UUID{b, {}, a, b})
	if diff := cmp.Diff([]pgtype.UUID{a, b}, got); diff != "" { t.Fatal(diff) }
}
```

Add a DB test that starts two resource-swap edits, waits with a finite context, and asserts neither ends with SQLSTATE `40P01`.

- [ ] **Step 2: Run RED lock tests**

Run: `go -C backend test ./internal/schedulelock ./internal/scheduling -run 'TestNormalizeLockIDs|TestScheduleDB_ResourceSwapEditsDoNotDeadlock' -count=1`

Expected: compile failure because lock helpers do not exist.

- [ ] **Step 3: Define ordered SQL locks and generate sqlc**

```sql
-- name: CoursesLockOrdered :many
SELECT id FROM courses WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;
-- name: StudentsLockOrdered :many
SELECT id FROM students WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;
-- name: UsersLockOrdered :many
SELECT id FROM users WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;
-- name: RoomsLockOrdered :many
SELECT id FROM rooms WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;
-- name: SessionsLockOrdered :many
SELECT id FROM sessions WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;
-- name: SeriesLockOrdered :many
SELECT id FROM session_series WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;
```

Run: `make -C backend sqlc`

Expected: generated `scheduling_locks.sql.go` is updated without unrelated generated changes.

- [ ] **Step 4: Implement one lock entry point with the global order**

```go
type ResourceLocks struct {
	CourseIDs, StudentIDs, TeacherIDs, RoomIDs, SessionIDs, SeriesIDs []pgtype.UUID
}

func LockResources(ctx context.Context, q *sqldb.Queries, ids ResourceLocks) error {
	if _, err := q.CoursesLockOrdered(ctx, normalizeLockIDs(ids.CourseIDs)); err != nil { return err }
	if _, err := q.StudentsLockOrdered(ctx, normalizeLockIDs(ids.StudentIDs)); err != nil { return err }
	if _, err := q.UsersLockOrdered(ctx, normalizeLockIDs(ids.TeacherIDs)); err != nil { return err }
	if _, err := q.RoomsLockOrdered(ctx, normalizeLockIDs(ids.RoomIDs)); err != nil { return err }
	if _, err := q.SessionsLockOrdered(ctx, normalizeLockIDs(ids.SessionIDs)); err != nil { return err }
	if _, err := q.SeriesLockOrdered(ctx, normalizeLockIDs(ids.SeriesIDs)); err != nil { return err }
	return nil
}
```

For edits, perform an unlocked identity read only to discover old resource IDs; call `schedulelock.LockResources` with sorted old and proposed resources plus the target session/series ID; then re-read the locked row using `qtx`, apply the expected-version check, and abort with `stale_edit` if identity/version changed. No preflight or write occurs before this re-read. Wrap exclusion-producing inserts/updates in the existing nested-transaction savepoint helper before re-preflight.

> **Series-lock sequencing:** Task 5 occurrence edits intentionally defer locking their optional parent series. The existing series edit/cancel paths still lock the series before mutating session rows, so adding a session-then-series lock here would invert that order. Task 8 must migrate all series writers to canonical resource ordering and add occurrence-to-series locking atomically with the attachment validation work.

- [ ] **Step 5: Verify generated code and focused tests**

Run: `make -C backend sqlc && git diff --check && go -C backend test ./internal/schedulelock ./internal/scheduling -count=1`

Expected: PASS.

- [ ] **Step 6: Commit Task 5**

```bash
git add backend/db/queries/scheduling_locks.sql backend/internal/db backend/internal/schedulelock backend/internal/scheduling
git commit -m "fix: serialize scheduling resources"
```

## Task 6: Close student busy-range races across every writer

**Files:**

- Modify: `backend/internal/scheduling/service.go`, `student_roster.go`, `session_roster.go`
- Modify: `backend/internal/httpapi/courseshttp/routes.go`, `sessionshttp/routes.go`
- Modify: `backend/internal/legacysync/syncer.go`, `syncer_test.go`
- Modify: `backend/internal/crmimport/crossstudy/store.go`, `backend/internal/crmimport/reconcile/reconcile.go` and focused tests in those packages
- Create/modify: `backend/internal/scheduling/schedule_invariants_integration_test.go`

- [ ] **Step 1: Add barrier-based failing integration tests**

Add these exact tests using dynamically generated future timestamps:

```go
func TestScheduleDB_ConcurrentRosterAddAndSessionCreateLeavesOneMatchingBusyRange(t *testing.T)
func TestScheduleDB_ConcurrentAttendanceUpdateAndSessionEditLeavesNewBusyInterval(t *testing.T)
func TestScheduleDB_ConcurrentRosterAddsAcrossCoursesReturnOneStableConflict(t *testing.T)
func TestScheduleDB_ConcurrentLegacySyncAndRosterChangePreservesBusyRanges(t *testing.T)
func TestScheduleDB_ConcurrentCRMReconcileAndSessionEditPreservesBusyRanges(t *testing.T)
```

Each test must use two independent pool connections, a barrier channel immediately before the write, a five-second context, and a final invariant query:

```go
func runRace(t *testing.T, left, right func(context.Context) error) (error, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ready := make(chan struct{})
	result := make(chan error, 2)
	var started sync.WaitGroup
	started.Add(2)
	run := func(fn func(context.Context) error) {
		started.Done()
		<-ready
		result <- fn(ctx)
	}
	go run(left)
	go run(right)
	started.Wait()
	close(ready)
	return <-result, <-result
}

func futureBangkok(days int, hour int) time.Time {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil { panic(err) }
	now := time.Now().In(loc).AddDate(0, 0, days)
	return time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, loc).UTC()
}
```

The roster/create RED test seeds one course, student, teacher, and room with existing sqlc helpers, then races `AddCourseStudentTx` against `CreateSessionTx`; after both return it executes the invariant query below and requires count `1` plus `bool_and=true`. The edit/attendance test creates a 09:00 session, races an included-attendance write against an edit to 11:00, and requires the busy row interval to equal 11:00–12:00. The CRM and legacy tests call their public reconciliation entry points in the right-hand closure and use the identical final invariant assertion.

```sql
SELECT count(*), bool_and(sbr.start_at = s.start_at AND sbr.end_at = s.end_at)
FROM student_busy_ranges sbr JOIN sessions s ON s.id = sbr.session_id
WHERE sbr.student_id = $1 AND sbr.deleted_at IS NULL AND s.deleted_at IS NULL;
```

- [ ] **Step 2: Run RED concurrency tests directly**

Run: `TEST_DATABASE_URL="$TEST_DATABASE_URL" go -C backend test -p 1 ./internal/scheduling ./internal/legacysync ./internal/crmimport/crossstudy ./internal/crmimport/reconcile -run '^TestScheduleDB_' -count=1 -timeout=120s`

Expected: at least one race invariant assertion fails on the original implementation. The fail-fast Make target is added in Task 9 after these tests exist.

- [ ] **Step 3: Route all roster/attendance/session writers through locked services**

At the beginning of each transactional service method, lock the course and explicit student IDs before any roster/session preflight. Roster remove/draft conversion and attendance delete must stop issuing direct mutation SQL from HTTP handlers. Legacy sync must acquire the same course/teacher/room locks before session reconciliation; it must use hard-delete semantics compatible with migration 00028. `crmimport/crossstudy/store.go` and `crmimport/reconcile/reconcile.go` must call `schedulelock.LockResources` before direct `course_students` or `session_attendance` writes.

Run this writer audit after wiring callers:

```bash
rg -n 'Session(Create|Update|HardDelete)|CourseStudent(Add|Remove)|SessionAttendance(Upsert|Delete)|INSERT INTO (sessions|course_students|session_attendance)|UPDATE sessions|DELETE FROM sessions' backend --glob '*.go' --glob '*.sql' | rg -v '_test.go|\.sql.go|db/migrations'
```

Every production result must be either inside `internal/scheduling`/`internal/series` after a `schedulelock.LockResources` call, or one of the explicitly locked CRM/legacy call sites. Add a code comment at each direct CRM/legacy write naming the lock already held.

- [ ] **Step 4: Verify GREEN concurrency invariants**

Run: `TEST_DATABASE_URL="$TEST_DATABASE_URL" go -C backend test -p 1 ./internal/scheduling ./internal/legacysync ./internal/crmimport/crossstudy ./internal/crmimport/reconcile -run '^TestScheduleDB_' -count=1 -timeout=120s`

Expected: PASS with no deadlock, missing busy row, stale interval, or raw database error.

- [ ] **Step 5: Commit Task 6**

```bash
git add backend/internal/scheduling backend/internal/httpapi/courseshttp backend/internal/httpapi/sessionshttp backend/internal/legacysync backend/internal/crmimport
git commit -m "fix: preserve student schedule occupancy"
```

## Task 7: Make availability mutations and schedule writes mutually consistent

**Files:**

- Create: `backend/internal/scheduling/availability.go`
- Create: `backend/db/migrations/00070_availability_union_policy.sql`
- Modify: `backend/db/queries/availability.sql` and generated `availability.sql.go`
- Modify: `backend/internal/db/availability_policy.go`
- Modify: `backend/internal/httpapi/availabilityhttp/routes.go`
- Create: `backend/internal/httpapi/availabilityhttp/routes_integration_test.go`

- [ ] **Step 1: Add failing union, future-boundary, and concurrency tests**

```go
func TestScheduleDB_AvailabilityUnionCoversSession(t *testing.T)
func TestScheduleDB_FirstAvailabilityWindowRejectsUncoveredFutureSession(t *testing.T)
func TestScheduleDB_LastWindowDeleteRestoresDefaultOpen(t *testing.T)
func TestScheduleDB_AvailabilityMutationIgnoresStartedHistory(t *testing.T)
func TestScheduleDB_ConcurrentAvailabilityAndSessionCreatePreservesPolicy(t *testing.T)
```

The union test uses two adjacent ranges whose combined multirange covers the session. The concurrency test accepts either writer as the loser but asserts that no committed future session is uncovered.

Use `runRace` from Task 6 for the concurrent case. After both closures return, assert this query returns zero:

```go
var uncovered int
err := pool.QueryRow(ctx, `
  SELECT count(*) FROM sessions s
  WHERE s.teacher_id=$1 AND s.deleted_at IS NULL AND s.start_at>transaction_timestamp()
    AND EXISTS (SELECT 1 FROM teacher_availability a WHERE a.teacher_id=s.teacher_id AND a.deleted_at IS NULL)
    AND NOT (COALESCE((SELECT range_agg(a.time_range) FROM teacher_availability a
                      WHERE a.teacher_id=s.teacher_id AND a.deleted_at IS NULL), '{}'::tstzmultirange) @> s.time_range)`, teacherID).Scan(&uncovered)
if err != nil || uncovered != 0 { t.Fatalf("uncovered=%d err=%v", uncovered, err) }
```

For the union test, insert `[09:00,10:00)` and `[10:00,11:00)` windows, then create `[09:30,10:30)` and require success. For first-window creation, create the future session while default-open, attempt `[12:00,13:00)`, require `availability_conflict`, and assert the window insert rolled back. For last-window deletion, delete the only window and require a subsequent out-of-window session to succeed.

- [ ] **Step 2: Run RED availability tests**

Run: `TEST_DATABASE_URL="$TEST_DATABASE_URL" go -C backend test -p 1 ./internal/scheduling ./internal/httpapi/availabilityhttp -run '^TestScheduleDB_.*Availability' -count=1`

Expected: at least the first-window or concurrency invariant fails.

- [ ] **Step 3: Add union-coverage SQL and transactional service methods**

Use PostgreSQL 16 multirange aggregation consistently in preflight, the new 00070 trigger replacement migration, and post-mutation validation. Never rewrite applied migration 00004:

```sql
-- Replace CheckTeacherAvailability; use the same shape for rooms.
SELECT EXISTS (
         SELECT 1 FROM teacher_availability
         WHERE teacher_id = @teacher_id AND deleted_at IS NULL
       ) AS has_windows,
       COALESCE((
         SELECT range_agg(time_range) FROM teacher_availability
         WHERE teacher_id = @teacher_id AND deleted_at IS NULL
       ), '{}'::tstzmultirange) @> tstzrange(@start_at, @end_at, '[)') AS is_available;
```

Migration 00070 replaces `enforce_session_availability()` with the same `teacher_has_windows`/`room_has_windows` branches already present in migration 00004, but each `teacher_ok`/`room_ok` assignment uses `COALESCE(range_agg(time_range), '{}'::tstzmultirange) @> tstzrange(NEW.start_at, NEW.end_at, '[)')`. Its Down section restores the original single-window `EXISTS (... time_range @> tstzrange(...))` implementation so rollback behavior is explicit.

Use this complete migration body:

```sql
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_session_availability()
RETURNS trigger AS $$
DECLARE
  teacher_has_windows boolean;
  room_has_windows boolean;
  teacher_ok boolean;
  room_ok boolean;
BEGIN
  IF NEW.deleted_at IS NOT NULL THEN RETURN NEW; END IF;

  SELECT EXISTS (SELECT 1 FROM teacher_availability WHERE teacher_id=NEW.teacher_id AND deleted_at IS NULL)
    INTO teacher_has_windows;
  IF teacher_has_windows THEN
    SELECT COALESCE(range_agg(time_range), '{}'::tstzmultirange)
           @> tstzrange(NEW.start_at, NEW.end_at, '[)')
      INTO teacher_ok
      FROM teacher_availability WHERE teacher_id=NEW.teacher_id AND deleted_at IS NULL;
    IF NOT teacher_ok THEN
      RAISE EXCEPTION 'teacher not available for requested time' USING ERRCODE='23514';
    END IF;
  END IF;

  IF NEW.room_id IS NOT NULL THEN
    SELECT EXISTS (SELECT 1 FROM room_availability WHERE room_id=NEW.room_id AND deleted_at IS NULL)
      INTO room_has_windows;
    IF room_has_windows THEN
      SELECT COALESCE(range_agg(time_range), '{}'::tstzmultirange)
             @> tstzrange(NEW.start_at, NEW.end_at, '[)')
        INTO room_ok
        FROM room_availability WHERE room_id=NEW.room_id AND deleted_at IS NULL;
      IF NOT room_ok THEN
        RAISE EXCEPTION 'room not available for requested time' USING ERRCODE='23514';
      END IF;
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_session_availability()
RETURNS trigger AS $$
DECLARE
  teacher_has_windows boolean;
  room_has_windows boolean;
  teacher_ok boolean;
  room_ok boolean;
BEGIN
  IF NEW.deleted_at IS NOT NULL THEN RETURN NEW; END IF;
  SELECT EXISTS (SELECT 1 FROM teacher_availability WHERE teacher_id=NEW.teacher_id AND deleted_at IS NULL)
    INTO teacher_has_windows;
  SELECT EXISTS (SELECT 1 FROM room_availability WHERE room_id=NEW.room_id AND deleted_at IS NULL)
    INTO room_has_windows;
  IF teacher_has_windows THEN
    SELECT EXISTS (SELECT 1 FROM teacher_availability
      WHERE teacher_id=NEW.teacher_id AND deleted_at IS NULL
        AND time_range @> tstzrange(NEW.start_at, NEW.end_at, '[)')) INTO teacher_ok;
    IF NOT teacher_ok THEN
      RAISE EXCEPTION 'teacher not available for requested time' USING ERRCODE='23514';
    END IF;
  END IF;
  IF room_has_windows THEN
    SELECT EXISTS (SELECT 1 FROM room_availability
      WHERE room_id=NEW.room_id AND deleted_at IS NULL
        AND time_range @> tstzrange(NEW.start_at, NEW.end_at, '[)')) INTO room_ok;
    IF NOT room_ok THEN
      RAISE EXCEPTION 'room not available for requested time' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
```

Availability create/delete service flow is: lock teacher/room parent → apply tentative mutation → query at most 25 uncovered sessions with `start_at > transaction_timestamp()` → return typed `availability_conflict` so the outer transaction rolls back.

- [ ] **Step 4: Delegate availability HTTP routes to the service and verify GREEN**

Run: `make -C backend sqlc && TEST_DATABASE_URL="$TEST_DATABASE_URL" go -C backend test -p 1 ./internal/scheduling ./internal/httpapi/availabilityhttp -count=1`

Expected: PASS.

- [ ] **Step 5: Commit Task 7**

```bash
git add backend/db backend/internal/db backend/internal/scheduling/availability.go backend/internal/httpapi/availabilityhttp
git commit -m "fix: enforce availability consistency"
```

## Task 8: Harden series split, preflight, and public series attachment

**Files:**

- Create: `backend/internal/series/split_bounds.go`, `split_bounds_test.go`
- Modify: `backend/internal/series/service.go`
- Modify: `backend/internal/scheduling/service.go`
- Modify: `backend/db/queries/series.sql`, `sessions.sql` and generated files
- Modify: `backend/internal/httpapi/serieshttp/routes.go`, `sessionshttp/routes.go`, `schedulinghttp/routes.go`
- Create/modify: series/session/scheduling HTTP and DB tests

- [ ] **Step 1: Add failing pure partition tests**

```go
type SplitPartition struct { Retained, Remaining int; InPlace bool }

func TestPartitionCountBoundedSplit_PreservesTargetTotal(t *testing.T) {
	got, err := partitionCountBoundedSplit(4, 10)
	if err != nil || got.Retained != 4 || got.Remaining != 6 || got.InPlace { t.Fatalf("got=%+v err=%v", got, err) }
}

func TestPartitionCountBoundedSplit_FirstOccurrenceUsesInPlaceEdit(t *testing.T) {
	got, err := partitionCountBoundedSplit(0, 10)
	if err != nil || !got.InPlace || got.Remaining != 10 { t.Fatalf("got=%+v err=%v", got, err) }
}

func TestPartitionCountBoundedSplit_RejectsTargetBelowRetainedPrefix(t *testing.T) {
	if _, err := partitionCountBoundedSplit(6, 6); err == nil { t.Fatal("expected invalid total") }
}
```

- [ ] **Step 2: Add failing DB/HTTP regressions**

Add exact tests:

```go
func TestScheduleDB_CountBoundedSplitAtFiveOfTenLeavesTenTotal(t *testing.T)
func TestScheduleDB_LaterClockSplitReplacesOriginalPivotOnce(t *testing.T)
func TestScheduleDB_SplitRejectsInvalidPivotWithoutChangingHistory(t *testing.T)
func TestScheduleDB_ConcurrentSeriesCancelAndAttachedCreatePreservesDefinition(t *testing.T)
func TestScheduleDB_PostSession_RejectsMissingOrInactiveSeries(t *testing.T)
func TestScheduleDB_PostSession_RejectsMismatchedSeriesOccurrence(t *testing.T)
func TestScheduleDB_PreflightCannotIgnoreUnrelatedSeries(t *testing.T)
```

The invalid-pivot test is table-driven for missing, deleted, wrong-series, exact-now, and past sessions and asserts child attendance/sit-in/missed rows remain unchanged.

All series tests seed dates with `futureBangkok(7, 10)` rather than fixed 2026 dates. The count test lists sessions for both returned series IDs and requires exactly ten distinct IDs. The later-clock test requires exactly one session on the pivot local date and requires its local start to equal the proposed time. The history test captures child-table counts before the request and compares the same counts afterward. The cancel/attach race uses `runRace`; after completion it queries every session with the series ID and fails if any start lies beyond the locked parent bound.

- [ ] **Step 3: Run RED series tests**

Run: `go -C backend test ./internal/series -run 'TestPartition|TestSplitDeletion' -count=1`

Run with DB: `TEST_DATABASE_URL="$TEST_DATABASE_URL" go -C backend test -p 1 ./internal/scheduling ./internal/httpapi/serieshttp ./internal/httpapi/sessionshttp ./internal/httpapi/schedulinghttp -run '^TestScheduleDB_' -count=1 -timeout=120s`

Expected: partition helpers missing and original split/attachment behavior fails regression assertions.

- [ ] **Step 4: Implement pivot-row validation and total-count partitioning**

```go
func partitionCountBoundedSplit(retained, targetTotal int) (SplitPartition, error) {
	if retained < 0 || targetTotal <= retained {
		return SplitPartition{}, &ValidationError{Code: "invalid_recurrence_count", Message: "total count must exceed retained occurrences"}
	}
	return SplitPartition{Retained: retained, Remaining: targetTotal - retained, InPlace: retained == 0}, nil
}
```

Resolve the pivot from an actual active session row, acquire resource locks, lock the series and pivot session, then revalidate `start_at > transaction_timestamp()`. Delete future old sessions using the original series clock. Return the same series ID in both legacy response fields for an in-place first-occurrence edit.

Preflight verifies that `series_id` exists and matches the course before ignoring it, and applies the identical remaining-count calculation.

- [ ] **Step 5: Validate public `series_id` attachment under the series lock**

Before session insert, require matching course, teacher, nullable room, local weekday/date/time, duration, and active end/count bounds. Return typed `invalid_series` or `series_occurrence_mismatch`; never pass an unchecked foreign key to `SessionCreate`.

- [ ] **Step 6: Verify GREEN**

Run: `make -C backend sqlc && go -C backend test ./internal/series -count=1`

Run with DB: `TEST_DATABASE_URL="$TEST_DATABASE_URL" go -C backend test -p 1 ./internal/scheduling ./internal/httpapi/serieshttp ./internal/httpapi/sessionshttp ./internal/httpapi/schedulinghttp -run '^TestScheduleDB_' -count=1 -timeout=120s`

Expected: PASS and no historical row or child-table count changes.

- [ ] **Step 7: Commit Task 8**

```bash
git add backend/internal/series backend/internal/scheduling backend/db/queries backend/internal/db backend/internal/httpapi
git commit -m "fix: preserve recurring schedule invariants"
```

## Task 9: Add online indexes, migration validation, and required CI suites

**Files:**

- Create: `backend/db/migrations/00071_schedule_stabilization_indexes.sql`
- Modify: `scripts/validate-migrations.sh`, `docs/migrations.md`
- Create: `backend/internal/db/schedule_indexes_integration_test.go`
- Modify: `backend/Makefile`, `.github/workflows/test.yml`, `package.json`

- [ ] **Step 1: Add a failing static migration test before creating the migration**

The static test reads migration 00071 and asserts `NO TRANSACTION`, six named `CREATE INDEX CONCURRENTLY IF NOT EXISTS` statements, and matching concurrent downs. The integration test queries `pg_indexes` for all six names after migration.

The integration test seeds at least 20,000 sessions and 20,000 busy rows with `generate_series`, runs `ANALYZE`, then executes selective `EXPLAIN (FORMAT JSON)` statements for: series/start deletion lookup, active course/start lookup, busy rows by session, active global time-range lookup, teacher availability containment, and room availability containment. Use a helper with exact index-name assertions:

```go
func assertPlanUsesIndex(t *testing.T, db *pgxpool.Pool, query, indexName string, args ...any) {
	t.Helper()
	var planJSON []byte
	if err := db.QueryRow(context.Background(), "EXPLAIN (FORMAT JSON) "+query, args...).Scan(&planJSON); err != nil { t.Fatal(err) }
	if !bytes.Contains(planJSON, []byte(indexName)) {
		t.Fatalf("plan does not use %s: %s", indexName, planJSON)
	}
}
```

Do not use `enable_seqscan=off` as the only proof. The fixture makes each predicate selective; if the planner still chooses a sequential scan, treat it as a performance-review failure and inspect statistics/query shape.

- [ ] **Step 2: Run RED static migration test**

Run: `go -C backend test ./internal/db -run TestScheduleStabilizationIndexesMigrationIsConcurrentAndReversible -count=1`

Expected: FAIL because migration 00071 does not exist.

- [ ] **Step 3: Add migration 00071**

```sql
-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_series_start_idx ON sessions(series_id, start_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_course_start_idx ON sessions(course_id, start_at) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS student_busy_ranges_session_idx ON student_busy_ranges(session_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_active_time_range_idx ON sessions USING gist(time_range) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS teacher_availability_active_range_idx ON teacher_availability USING gist(teacher_id, time_range) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS room_availability_active_range_idx ON room_availability USING gist(room_id, time_range) WHERE deleted_at IS NULL;
-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS room_availability_active_range_idx;
DROP INDEX CONCURRENTLY IF EXISTS teacher_availability_active_range_idx;
DROP INDEX CONCURRENTLY IF EXISTS sessions_active_time_range_idx;
DROP INDEX CONCURRENTLY IF EXISTS student_busy_ranges_session_idx;
DROP INDEX CONCURRENTLY IF EXISTS sessions_active_course_start_idx;
DROP INDEX CONCURRENTLY IF EXISTS sessions_series_start_idx;
```

Update the validator regex to accept optional `CONCURRENTLY` while still requiring `IF NOT EXISTS`/`IF EXISTS`. Document index-first rollout and finite lock timeout in `docs/migrations.md`.

Run: `npm run migrate:validate`

Expected before the validator change: FAIL because the current script recognizes only `CREATE INDEX IF NOT EXISTS`. After the validator change: PASS.

- [ ] **Step 4: Add a fail-fast database test target**

```make
test-scheduling-db:
	@test -n "$$TEST_DATABASE_URL" || { echo "TEST_DATABASE_URL is required"; exit 1; }
	go test -p 1 ./internal/scheduling ./internal/httpapi/sessionshttp ./internal/httpapi/serieshttp ./internal/httpapi/availabilityhttp ./internal/legacysync ./internal/db -run '^TestScheduleDB_' -count=1 -timeout=120s
```

- [ ] **Step 5: Add required schedule frontend and database CI commands**

In `.github/workflows/test.yml`, run `npm run test:schedule` in the existing job and add a separate `schedule-db` job with the same PostgreSQL 16 service and `TEST_DATABASE_URL`, executing `make -C backend test-scheduling-db`. Keep the full Go suite as a separate gate.

```yaml
  schedule-db:
    runs-on: ubuntu-latest
    timeout-minutes: 20
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_USER: postgres
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: warwick_schedule_test
        ports: ["5432:5432"]
        options: >-
          --health-cmd "pg_isready -U postgres -d warwick_schedule_test"
          --health-interval 5s --health-timeout 5s --health-retries 10
    env:
      CI: true
      TEST_DATABASE_URL: postgres://postgres:postgres@127.0.0.1:5432/warwick_schedule_test?sslmode=disable
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: backend/go.mod
          cache-dependency-path: backend/go.sum
      - run: make -C backend test-scheduling-db
```

Add `- run: npm run test:schedule` immediately after `npm run typecheck` in the existing `test` job.

- [ ] **Step 6: Verify migration and CI-target behavior**

Run: `npm run migrate:validate`

Run without env: `env -u TEST_DATABASE_URL make -C backend test-scheduling-db`

Expected: immediate nonzero exit with `TEST_DATABASE_URL is required`.

Run with env: `make -C backend test-scheduling-db`

Expected: PASS.

- [ ] **Step 7: Commit Task 9**

```bash
git add backend/db/migrations/00071_schedule_stabilization_indexes.sql backend/internal/db/schedule_indexes_integration_test.go backend/Makefile scripts/validate-migrations.sh docs/migrations.md .github/workflows/test.yml package.json
git commit -m "perf: add schedule stabilization indexes"
```

## Task 10: Full verification and production reviews

**Files:** All files changed by Tasks 1–9; approved spec and this plan.

- [ ] **Step 1: Regenerate and prove generated code is current**

Run: `make -C backend sqlc && git diff --check`

Expected: no unexpected generated diff and no whitespace errors.

- [ ] **Step 2: Run complete frontend verification**

Run: `npm run typecheck && npm run typecheck:e2e && npm run test:schedule && npm test && npm run build`

Expected: all commands exit 0.

- [ ] **Step 3: Run complete backend and migration verification**

Run: `npm run migrate:validate && go -C backend test ./... -count=1 && make -C backend test-scheduling-db && make -C backend build`

Expected: all commands exit 0 with no skipped `TestScheduleDB_` tests.

- [ ] **Step 4: Run invariant audit queries against the test database**

```sql
WITH expected AS (
  SELECT s.id AS session_id, roster.student_id, s.start_at, s.end_at
  FROM sessions s
  CROSS JOIN LATERAL (
    SELECT student_id FROM course_students WHERE course_id=s.course_id
    UNION
    SELECT student_id FROM session_attendance WHERE session_id=s.id AND status='included'
  ) roster
  WHERE s.deleted_at IS NULL
    AND NOT EXISTS (SELECT 1 FROM session_attendance x
      WHERE x.session_id=s.id AND x.student_id=roster.student_id AND x.status='excluded')
), actual AS (
  SELECT session_id, student_id, start_at, end_at
  FROM student_busy_ranges WHERE deleted_at IS NULL
)
SELECT count(*) FROM (
  (SELECT * FROM expected EXCEPT SELECT * FROM actual)
  UNION ALL
  (SELECT * FROM actual EXCEPT SELECT * FROM expected)
) mismatch;

SELECT count(*) FROM sessions s JOIN session_series ss ON ss.id=s.series_id
WHERE s.course_id<>ss.course_id OR s.teacher_id<>ss.teacher_id OR s.room_id IS DISTINCT FROM ss.room_id;

SELECT count(*) FROM sessions s
WHERE s.deleted_at IS NULL AND s.start_at > transaction_timestamp()
  AND EXISTS (SELECT 1 FROM teacher_availability a WHERE a.teacher_id=s.teacher_id AND a.deleted_at IS NULL)
  AND NOT (COALESCE((SELECT range_agg(a.time_range) FROM teacher_availability a
                    WHERE a.teacher_id=s.teacher_id AND a.deleted_at IS NULL), '{}'::tstzmultirange) @> s.time_range);

SELECT count(*) FROM sessions s
WHERE s.deleted_at IS NULL AND s.room_id IS NOT NULL AND s.start_at > transaction_timestamp()
  AND EXISTS (SELECT 1 FROM room_availability a WHERE a.room_id=s.room_id AND a.deleted_at IS NULL)
  AND NOT (COALESCE((SELECT range_agg(a.time_range) FROM room_availability a
                    WHERE a.room_id=s.room_id AND a.deleted_at IS NULL), '{}'::tstzmultirange) @> s.time_range);
```

Expected: all four counts are zero after the test suite. Run the same read-only audit against production before enabling the behavior release; any nonzero count is a deployment stop requiring explicit data repair, not an automatic rewrite.

- [ ] **Step 5: Add and verify bounded operational logging**

At typed error boundaries, emit structured warnings without request bodies or student names:

```go
slog.WarnContext(ctx, "schedule availability mutation rejected", "resource_type", kind, "resource_id", resourceID, "conflict_count", len(conflicts))
slog.WarnContext(ctx, "schedule series attachment rejected", "series_id", seriesID, "reason", reasonCode)
slog.WarnContext(ctx, "schedule recurrence rejected", "code", validation.Code)
slog.ErrorContext(ctx, "schedule resource lock failed", "operation", operation, "error", err)
```

Tests capture the configured slog handler and assert fields are present while raw JSON bodies, full student records, and SQL text are absent.

- [ ] **Step 6: Complete mandatory reviews**

Architecture: confirm every writer follows course → student → teacher → room → session/series lock order.

QA: rerun each original reproduction, including split later, split count, past pivot, final-slot conflict, exact body reuse, and both concurrency barriers.

Security: confirm error bodies expose no SQL or student personal data and arbitrary `series_id`/ignore IDs are rejected.

Performance: inspect `EXPLAIN (ANALYZE, BUFFERS)` on production-like fixtures for series future delete, course session scan, busy-range delete, range calendar read, and availability containment.

Reliability: confirm savepoints preserve explainable errors, lock waits respect request contexts, idempotent replay performs no durable mutation, and CI database tests cannot skip.

Documentation: confirm `docs/idempotency.md` and `docs/migrations.md` match shipped behavior.

- [ ] **Step 7: Request independent code review and fix every blocker**

Use the requesting-code-review skill. Review the complete diff against `docs/superpowers/specs/2026-07-15-course-schedule-stabilization-design.md`; repeat verification after any fix.

- [ ] **Step 8: Confirm verification left no uncommitted changes**

```bash
git status --short
```

Expected: empty output. If verification changed a tracked file, return to the owning task, review that diff, rerun its red-green loop, and commit it there before repeating Task 10.

## Acceptance checklist

- [ ] Recurrence is capped, horizon-bounded, int32-safe, and cancellation-aware.
- [ ] Slot Finder correctly maps first/last ordinality and never extends past day end.
- [ ] Edit-series preflight includes and validates its own series ID.
- [ ] Past/started series actions are unavailable in UI and rejected by the API.
- [ ] Split count is total-across-prefix-and-suffix; later-time split cannot duplicate pivot.
- [ ] Historical sessions and attendance/absence children remain immutable.
- [ ] Public series attachments match locked active definitions.
- [ ] Student busy ranges cannot become missing or stale under the audited races.
- [ ] Availability and schedule writes serialize and use union coverage for future sessions.
- [ ] Original request bytes drive idempotency; replay precedes CAS.
- [ ] Online indexes exist and representative plans use them.
- [ ] Schedule frontend tests and fail-fast PostgreSQL concurrency tests are required in CI.
- [ ] Security, performance, reliability, QA, code-review, documentation, and final-verification gates approve the result.
