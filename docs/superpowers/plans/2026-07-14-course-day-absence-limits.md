# Course-Day Absence Limits Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Count absence allowance by distinct Bangkok course days while retaining every underlying physical session for attendance and sit-in workflows.

**Architecture:** Add a pure absence-day limit policy in the absences domain and a single repository query that returns total, used, and projected distinct course-day counts. Public absence handlers use that query as the authoritative validator, and the sessions-in-range API returns day-named statistics that the React form consumes without recalculating percentages.

**Tech Stack:** Go 1.25, PostgreSQL/pgx, React 19, TypeScript 5.9, Vitest, Go testing.

---

## File Map

- Modify `backend/internal/absences/submission.go`: replace physical-session limit policy with pure course-day statistics and rounding functions.
- Modify `backend/internal/absences/submission_test.go`: define the day-based policy contract and rounding boundaries.
- Modify `backend/internal/db/absence_custom.go`: centralize distinct Bangkok course-day, historical usage, and projected usage queries.
- Create `backend/internal/db/absence_day_counts_integration_test.go`: verify query semantics for same-day sessions, history, legacy ranges, statuses, and timezone boundaries.
- Modify `backend/internal/httpapi/absenceshttp/submission_helpers.go`: expose day-policy helpers to HTTP handlers.
- Modify `backend/internal/httpapi/absenceshttp/routes.go`: use authoritative day statistics for single create and sessions-in-range responses.
- Modify `backend/internal/httpapi/absenceshttp/batch_routes.go`: use the same day statistics for batch creation.
- Modify `backend/internal/httpapi/absenceshttp/routes_test.go` and `batch_routes_test.go`: update unit contracts from sessions to days.
- Modify `backend/internal/httpapi/absenceshttp/absence_limit_integration_test.go`: cover public API behavior, historical recalculation, same-day deduplication, and serialized submissions.
- Modify `src/features/absences/types.ts`: replace ambiguous count fields with explicit course-day statistics.
- Modify `src/features/absences/domain/sessionGrouping.ts`: expose per-course selected-day counting for limit enforcement.
- Modify `src/features/absences/domain/__tests__/sessionGrouping.test.ts`: prove a multi-session day consumes one selected unit.
- Modify `src/pages/AbsenceForm.tsx`: consume backend day statistics and apply request caps to grouped days while still submitting all session IDs.
- Modify `src/pages/__tests__/AbsenceForm.sessionLimit.test.tsx`, `AbsenceForm.test.tsx`, and related absence-form fixtures: update API contracts, copy, and grouped-day behavior.

### Task 1: Define the Pure Course-Day Limit Policy

**Files:**
- Modify: `backend/internal/absences/submission.go`
- Modify: `backend/internal/absences/submission_test.go`

- [ ] **Step 1: Write failing policy tests**

Add table tests for the exact contract:

```go
func TestAbsenceDayLimitStats(t *testing.T) {
	tests := []struct {
		name, total, used, projected int32
		wantMax, wantRemaining       int32
		wantReached, wantExceeded    bool
	}{
		{"below half rounds down", 12, 1, 2, 2, 1, false, false},
		{"above half rounds up", 13, 2, 3, 3, 1, false, false},
		{"exact boundary reached", 10, 2, 2, 2, 0, true, false},
		{"projected over boundary", 10, 1, 3, 2, 1, false, true},
		{"zero total guarded", 0, 0, 1, 0, 0, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewAbsenceDayLimitStats(tt.total, tt.used, tt.projected)
			if got.MaximumAbsenceDays != tt.wantMax || got.RemainingAbsenceDays != tt.wantRemaining ||
				got.LimitReached != tt.wantReached || got.ProjectedLimitExceeded != tt.wantExceeded {
				t.Fatalf("stats = %+v", got)
			}
		})
	}
}
```

- [ ] **Step 2: Run the focused domain test and confirm RED**

Run: `cd backend && go test ./internal/absences -run TestAbsenceDayLimitStats -count=1`

Expected: compilation failure because `NewAbsenceDayLimitStats` does not exist.

- [ ] **Step 3: Implement the pure policy**

Add an intention-revealing value type:

```go
type AbsenceDayLimitStats struct {
	TotalCourseDays        int32
	UsedAbsenceDays        int32
	ProjectedAbsenceDays   int32
	MaximumAbsenceDays     int32
	RemainingAbsenceDays   int32
	LimitReached           bool
	ProjectedLimitExceeded bool
}

func NewAbsenceDayLimitStats(total, used, projected int32) AbsenceDayLimitStats {
	if total <= 0 {
		return AbsenceDayLimitStats{TotalCourseDays: total, UsedAbsenceDays: used, ProjectedAbsenceDays: projected}
	}
	maximum := int32(math.Round(float64(total) / 5.0))
	remaining := maximum - used
	if remaining < 0 {
		remaining = 0
	}
	return AbsenceDayLimitStats{
		TotalCourseDays: total, UsedAbsenceDays: used, ProjectedAbsenceDays: projected,
		MaximumAbsenceDays: maximum, RemainingAbsenceDays: remaining,
		LimitReached: used >= maximum, ProjectedLimitExceeded: projected > maximum,
	}
}
```

Remove `ProjectedAbsenceSessionLimitExceeded` after all callers migrate in Task 3. Keep unrelated record-limit behavior only if another caller still uses it.

- [ ] **Step 4: Run domain tests and confirm GREEN**

Run: `cd backend && go test ./internal/absences -count=1`

Expected: PASS.

### Task 2: Centralize Distinct-Day Database Calculations

**Files:**
- Modify: `backend/internal/db/absence_custom.go`
- Create: `backend/internal/db/absence_day_counts_integration_test.go`

- [ ] **Step 1: Write failing database integration tests**

Create fixtures that seed 10 distinct Bangkok course dates, with two physical sessions on the first date. Assert:

```go
counts, err := q.AbsenceDayCountsForCourse(ctx, AbsenceDayCountsForCourseParams{
	Wcode: wcode, CourseID: courseID, CandidateSessionIDs: []pgtype.UUID{firstAM, firstPM},
	DateFrom: firstDate, DateTo: firstDate, InstituteTZ: "Asia/Bangkok",
})
if err != nil { t.Fatal(err) }
if counts.TotalCourseDays != 10 || counts.UsedAbsenceDays != 0 ||
	counts.CandidateAbsenceDays != 1 || counts.ProjectedAbsenceDays != 1 {
	t.Fatalf("counts = %+v", counts)
}
```

Add independent cases proving:

- explicit historical missed-session rows on the same date deduplicate;
- separate active absence records on the same date deduplicate;
- legacy records without missed-session rows derive dates by joining course sessions within `date_from..date_to`;
- cancelled and `special_approved` records are excluded;
- timestamps around UTC/Bangkok midnight are converted using `InstituteTZ`;
- empty candidate IDs use the submitted date range and count distinct matching course days.

- [ ] **Step 2: Run the database test and confirm RED**

Run: `cd backend && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/db -run TestAbsenceDayCountsForCourse -count=1`

Expected: compilation failure because the query type and method do not exist. If `TEST_DATABASE_URL` is absent, record the test as skipped and continue with unit/static verification.

- [ ] **Step 3: Implement one repository contract**

Add:

```go
type AbsenceDayCountsForCourseParams struct {
	Wcode               string
	CourseID            pgtype.UUID
	CandidateSessionIDs []pgtype.UUID
	DateFrom            pgtype.Date
	DateTo              pgtype.Date
	InstituteTZ         string
}

type AbsenceDayCounts struct {
	TotalCourseDays      int32
	UsedAbsenceDays      int32
	CandidateAbsenceDays int32
	ProjectedAbsenceDays int32
}
```

Implement one CTE query with these sets:

```sql
WITH course_days AS (
  SELECT DISTINCT (s.start_at AT TIME ZONE $6)::date AS day
  FROM sessions s WHERE s.course_id = $2 AND s.deleted_at IS NULL
), explicit_absence_days AS (
  SELECT DISTINCT (s.start_at AT TIME ZONE $6)::date AS day
  FROM student_absences sa
  JOIN absence_missed_sessions ams ON ams.absence_id = sa.id
  JOIN sessions s ON s.id = ams.session_id
  WHERE lower(sa.wcode) = lower($1) AND sa.course_id = $2
    AND sa.status NOT IN ('cancelled', 'special_approved')
), legacy_absence_days AS (
  SELECT DISTINCT cd.day
  FROM student_absences sa
  JOIN course_days cd ON cd.day BETWEEN sa.date_from AND sa.date_to
  WHERE lower(sa.wcode) = lower($1) AND sa.course_id = $2
    AND sa.status NOT IN ('cancelled', 'special_approved')
    AND NOT EXISTS (SELECT 1 FROM absence_missed_sessions ams WHERE ams.absence_id = sa.id)
), used_days AS (
  SELECT day FROM explicit_absence_days UNION SELECT day FROM legacy_absence_days
), candidate_days AS (
  SELECT DISTINCT (s.start_at AT TIME ZONE $6)::date AS day
  FROM sessions s
  WHERE s.course_id = $2 AND s.deleted_at IS NULL
    AND ((cardinality($3::uuid[]) > 0 AND s.id = ANY($3::uuid[]))
      OR (cardinality($3::uuid[]) = 0 AND (s.start_at AT TIME ZONE $6)::date BETWEEN $4 AND $5))
), projected_days AS (
  SELECT day FROM used_days UNION SELECT day FROM candidate_days
)
SELECT (SELECT count(*) FROM course_days)::int4,
       (SELECT count(*) FROM used_days)::int4,
       (SELECT count(*) FROM candidate_days)::int4,
       (SELECT count(*) FROM projected_days)::int4
```

Use `Asia/Bangkok` when the configured timezone is blank. Retain old count methods temporarily only until every caller is migrated, then delete them if unused.

- [ ] **Step 4: Add transaction-scoped serialization helper coverage**

Reuse the existing `AdvisoryLockForText` repository method with a stable key:

```go
func absenceLimitLockKey(wcode, courseID string) string {
	return "absence-limit:" + normalizeWCode(wcode) + ":" + courseID
}
```

Add a unit test proving normalization makes case variants produce the same key.

- [ ] **Step 5: Run database and package tests**

Run: `cd backend && go test ./internal/db ./internal/absences -count=1`

Expected: PASS, or DB-only cases SKIP with an explicit missing `TEST_DATABASE_URL` message.

### Task 3: Make Single and Batch Submission Use Projected Course Days

**Files:**
- Modify: `backend/internal/httpapi/absenceshttp/submission_helpers.go`
- Modify: `backend/internal/httpapi/absenceshttp/routes.go`
- Modify: `backend/internal/httpapi/absenceshttp/batch_routes.go`
- Modify: `backend/internal/httpapi/absenceshttp/routes_test.go`
- Modify: `backend/internal/httpapi/absenceshttp/batch_routes_test.go`
- Modify: `backend/internal/httpapi/absenceshttp/absence_limit_integration_test.go`

- [ ] **Step 1: Replace handler-unit tests with day-based contracts**

Test `absenceDayLimitLockKey` normalization and `NewAbsenceDayLimitStats` forwarding. Remove assertions whose names encode physical-session accounting.

- [ ] **Step 2: Add failing same-day API tests**

Extend the integration fixture so the first course date has two session IDs. Submit both IDs in one request and assert HTTP success when one absence day remains. Submit different same-day IDs in separate requests and assert the second request does not consume another unit. Then submit a different date and assert `absence_limit_exceeded` when the rounded maximum is exceeded.

- [ ] **Step 3: Parse candidate IDs before the limit check**

Add a focused helper that converts request strings to UUIDs once and returns `bad_missed_session_id` at the HTTP boundary. Reuse the parsed slice for day statistics and later missed-session validation/storage; do not parse the same IDs twice.

- [ ] **Step 4: Lock and calculate projected day statistics**

Add one shared helper in `submission_helpers.go` so the single and batch handlers cannot drift:

```go
func projectedAbsenceDayStats(
	ctx context.Context,
	q *sqldb.Queries,
	wcode string,
	courseID pgtype.UUID,
	missedSessionIDs []pgtype.UUID,
	dateFrom pgtype.Date,
	dateTo pgtype.Date,
	instituteTZ string,
) (absences.AbsenceDayLimitStats, int32, error) {
	courseIDString, err := sUUIDString(courseID)
	if err != nil {
		return absences.AbsenceDayLimitStats{}, 0, err
	}
	if err := q.AdvisoryLockForText(ctx, absenceDayLimitLockKey(wcode, courseIDString)); err != nil {
		return absences.AbsenceDayLimitStats{}, 0, err
	}
	counts, err := q.AbsenceDayCountsForCourse(ctx, sqldb.AbsenceDayCountsForCourseParams{
		Wcode: wcode, CourseID: courseID, CandidateSessionIDs: missedSessionIDs,
		DateFrom: dateFrom, DateTo: dateTo, InstituteTZ: instituteTZ,
	})
	if err != nil {
		return absences.AbsenceDayLimitStats{}, 0, err
	}
	return absences.NewAbsenceDayLimitStats(
		counts.TotalCourseDays,
		counts.UsedAbsenceDays,
		counts.ProjectedAbsenceDays,
	), counts.CandidateAbsenceDays, nil
}
```

After resolving the course and before creating the absence, both handlers call the helper. On error, write HTTP 500 with code `internal`; on `ProjectedLimitExceeded`, write HTTP 403 with code `absence_limit_exceeded`. The single handler returns `(0, nil, err)` from its idempotent transaction callback; the batch item helper returns `(createdAbsenceRecord{}, false)`.

- [ ] **Step 5: Enforce request caps in absence-day units**

Keep `len(SitInSessionIDs) > MaxSessionsPerAbsence` unchanged because sit-ins remain physical sessions. Replace the missed-session `len(...)` cap with `candidateAbsenceDays > int32(MaxSessionsPerAbsence)` so any number of physical sessions on one course date consumes one request unit.

- [ ] **Step 6: Run focused HTTP tests**

Run: `cd backend && go test ./internal/httpapi/absenceshttp -run 'AbsenceLimit|AbsenceDay|Projected' -count=1`

Expected: PASS; DB integration cases may SKIP only when the test database is unavailable.

### Task 4: Return Explicit Day Statistics from Sessions-in-Range

**Files:**
- Modify: `backend/internal/httpapi/absenceshttp/routes.go`
- Modify: `backend/internal/httpapi/absenceshttp/routes_test.go`
- Modify: `backend/internal/httpapi/absenceshttp/absence_limit_integration_test.go`

- [ ] **Step 1: Write failing JSON-contract tests**

Require these response fields:

```go
TotalCourseDays      int32 `json:"total_course_days"`
UsedAbsenceDays      int32 `json:"used_absence_days"`
MaximumAbsenceDays   int32 `json:"maximum_absence_days"`
RemainingAbsenceDays int32 `json:"remaining_absence_days"`
AbsenceLimitReached  bool  `json:"absence_limit_reached"`
```

Assert that a 10-day course with two used days returns `10, 2, 2, 0, true`, including when those historical records contain multiple physical sessions on a used date.

- [ ] **Step 2: Replace response construction**

Call `AbsenceDayCountsForCourse` with no candidate IDs and an invalid/empty candidate date range, then construct domain stats with `projected = used`. Return the five explicit fields and remove frontend dependence on `existing_absence_count`, `total_session_count`, and `absence_rate_exceeded`.

- [ ] **Step 3: Run contract tests**

Run: `cd backend && go test ./internal/httpapi/absenceshttp -run 'SessionsInRange|CourseDay' -count=1`

Expected: PASS.

### Task 5: Make React Limit State Day-Based

**Files:**
- Modify: `src/features/absences/types.ts`
- Modify: `src/features/absences/domain/sessionGrouping.ts`
- Modify: `src/features/absences/domain/__tests__/sessionGrouping.test.ts`
- Modify: `src/pages/AbsenceForm.tsx`
- Modify: `src/pages/__tests__/AbsenceForm.sessionLimit.test.tsx`
- Modify: `src/pages/__tests__/AbsenceForm.test.tsx`
- Modify: absence-form fixtures under `src/pages/__tests__/fixtures/` and `src/pages/__tests__/helpers/` as required by compiler failures.

- [ ] **Step 1: Update the TypeScript API contract in tests**

Replace fixture fields with:

```ts
total_course_days: 10,
used_absence_days: 1,
maximum_absence_days: 2,
remaining_absence_days: 1,
absence_limit_reached: false,
```

Add an assertion that selecting a grouped day containing `m1` and `m2` displays one selected day, consumes one remaining day, and still submits `missed_session_ids: ["m1", "m2"]`.

- [ ] **Step 2: Run frontend tests and confirm RED**

Run: `npm test -- src/features/absences/domain/__tests__/sessionGrouping.test.ts src/pages/__tests__/AbsenceForm.sessionLimit.test.tsx src/pages/__tests__/AbsenceForm.test.tsx`

Expected: failures referencing old fields and physical-session cap behavior.

- [ ] **Step 3: Introduce explicit selected-day helpers**

Rename `countSelectedSessions` to `countSelectedAbsenceDays`. Add a per-course helper:

```ts
export function countSelectedAbsenceDaysForGroup(group: SubjectSessions, selected: Set<string>): number {
  return groupByDay(group.sessions)
    .map((day) => ({ ...day, items: day.items.filter((session) => !session.already_absent) }))
    .filter((day) => day.items.length > 0 && isDayGroupSelected(day, selected)).length;
}
```

Keep `getSelectedSessionsForGroup` unchanged because payloads require all physical IDs.

- [ ] **Step 4: Consume backend-provided statistics**

Update `SubjectSessions` with the five day-named fields. Implement:

```ts
const remainingForGroup = (group: SubjectSessions) =>
  group.remaining_absence_days ?? maxSessions;
```

When toggling a grouped day, compare `currentlySelectedDays + 1` with both the backend remaining allowance and the configured per-request cap. Add every `sessionId` only after those day-based checks pass.

- [ ] **Step 5: Update copy and disabled states**

Use `day`/`days` for allowance messages and the backend `absence_limit_reached` flag. The review and payload builders continue to group for display and submit physical session IDs.

- [ ] **Step 6: Run focused frontend tests and typecheck**

Run: `npm test -- src/features/absences/domain/__tests__/sessionGrouping.test.ts src/pages/__tests__/AbsenceForm.sessionLimit.test.tsx src/pages/__tests__/AbsenceForm.test.tsx`

Run: `npm run typecheck`

Expected: PASS.

### Task 6: Regression, Concurrency, and Full Verification

**Files:**
- Modify tests only if a genuine contract regression is found.

- [ ] **Step 1: Run all absence backend tests**

Run: `cd backend && go test ./internal/absences ./internal/db ./internal/httpapi/absenceshttp -count=1`

Expected: PASS, with DB integration tests skipped only when `TEST_DATABASE_URL` is not configured.

- [ ] **Step 2: Run all absence frontend tests**

Run: `npm run test:absence`

Expected: PASS.

- [ ] **Step 3: Run static verification**

Run: `npm run typecheck && cd backend && go test ./... -count=1`

Expected: PASS.

- [ ] **Step 4: Inspect the final diff for scope and user-change safety**

Run: `git diff --check` and `git status --short`.

Confirm that no existing unrelated workspace modifications were reverted or staged. Because the working tree already contains extensive user changes in overlapping files, do not create implementation commits unless the user explicitly asks and the exact hunks can be isolated safely.
