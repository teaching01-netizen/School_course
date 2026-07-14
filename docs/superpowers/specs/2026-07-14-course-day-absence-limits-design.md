# Course-Day Absence Limits Design

## Purpose

Change absence-limit accounting from physical session rows to course calendar days. A student who misses multiple sessions of the same course on the same institute-local date uses one absence unit, while every underlying session remains attached to the request for attendance, reporting, and sit-in behavior.

## Confirmed Business Rules

1. One absence unit is one course on one Bangkok calendar date.
2. Multiple sessions of the same course on the same Bangkok date count as one course day and one absence day.
3. Separate absence requests for the same student, course, and Bangkok date are deduplicated to one absence day.
4. The same course on different dates consumes one absence day per date.
5. Different courses on the same date are counted independently.
6. Existing historical absences are recalculated using the course-day rule.
7. Cancelled and `special_approved` absences do not consume absence days, preserving current policy.
8. The maximum is `round(total_course_days / 5)`, using standard half-up rounding for non-negative values: fractions below `.5` round down, `.5` and above round up.
9. The backend is authoritative. Frontend counters are advisory and must use backend-provided statistics.

## Terminology

- **Physical session:** One stored session row with its own start and end time.
- **Course day:** A distinct Bangkok calendar date on which a course has at least one non-deleted physical session.
- **Absence day:** A distinct course day missed by a student through one or more active absence records.
- **Active absence:** An absence whose status is neither `cancelled` nor `special_approved`.

## Chosen Approach

Introduce a centralized backend absence-day policy backed by query-time distinct-date calculation. Keep the existing detailed session associations and derive limit units from their Bangkok dates.

This is preferred over changing counters independently because it prevents UI/API/database drift. It is preferred over adding an `absence_days` table because a derived table would require synchronization whenever sessions, dates, statuses, or legacy records change.

## Architecture

The backend exposes one coherent statistics result for a student and course:

- `total_course_days`
- `used_absence_days`
- `maximum_absence_days`
- `remaining_absence_days`
- `absence_limit_reached`

Responsibilities are separated as follows:

1. The database/repository derives distinct Bangkok course dates and historical missed dates.
2. A small domain policy computes the rounded maximum, remaining allowance, and whether a projected set exceeds the limit.
3. HTTP handlers validate candidate session IDs, obtain projected day statistics, and reject invalid submissions.
4. The frontend groups sessions visually, submits all underlying session IDs, and renders the backend statistics.

The policy must not depend on HTTP or database types. HTTP handlers must not reproduce percentage or distinct-date calculations.

## Database Calculations

### Total course days

Count distinct Bangkok dates from all non-deleted sessions belonging to the course:

```text
COUNT(DISTINCT (sessions.start_at AT TIME ZONE institute_timezone)::date)
```

### Used historical absence days

Build a set of dates and count distinct values after combining:

1. Explicit missed-session history: active `student_absences` joined through `absence_missed_sessions` to sessions, converted to Bangkok dates.
2. Legacy absence records without missed-session links: active absence date ranges joined to the course's sessions whose Bangkok dates fall within the stored range.

The final set is deduplicated by student, course, and Bangkok date. This makes two separate same-day requests consume one unit and recalculates historical records without destructive migration.

### Projected submission

Convert validated candidate missed-session IDs to distinct Bangkok course dates. Compute the union of existing missed dates and candidate dates, then compare its size with the allowed maximum.

Do not calculate projection as `existing_count + submitted_session_count`, because that cannot deduplicate a submitted date already present in history.

## Limit Policy

For non-negative course-day counts:

```text
maximum_absence_days = round(total_course_days / 5)
remaining_absence_days = max(0, maximum_absence_days - used_absence_days)
limit_reached = used_absence_days >= maximum_absence_days
projected_limit_exceeded = projected_absence_days > maximum_absence_days
```

For a course with no course days, the limit check remains guarded and does not reject solely because the denominator is zero.

Examples:

| Total course days | Raw 20% | Maximum absence days |
|---:|---:|---:|
| 10 | 2.0 | 2 |
| 12 | 2.4 | 2 |
| 13 | 2.6 | 3 |

## Submission and Storage Behavior

The UI continues to send every selected physical session ID. The backend continues to store every missed-session association. This preserves accurate teacher rosters, absence details, notification content, and sit-in resolution.

Only allowance accounting changes:

```text
Two stored missed-session rows on one Bangkok course date = one absence day
```

One batch item per course remains valid. The request's date range remains descriptive and does not become the source of truth when explicit missed-session IDs exist.

## API and Frontend Changes

Replace ambiguous session-count limit fields with course-day terminology throughout the contract and UI. The sessions-in-range response should provide the centralized statistics rather than requiring the frontend to independently recalculate the 20% rule.

The frontend will:

1. Continue grouping same-course sessions by Bangkok day.
2. Count selected day groups, not selected physical session IDs, against the displayed remaining allowance.
3. Keep all physical session IDs selected and submitted when a day group is checked.
4. Display `day`/`days` rather than `session`/`sessions` for absence-limit messaging.
5. Treat backend rejection as authoritative and refresh statistics after a limit conflict.

## Concurrency and Transaction Safety

The projected-limit check and absence creation must run in the same transaction. Serialize calculations for the same normalized student WCode and course, using a transaction-scoped advisory lock or an equivalent row-locking mechanism, so concurrent non-idempotent requests cannot both pass against stale history.

Idempotency behavior remains unchanged and complements, but does not replace, course/student serialization.

## Compatibility and Historical Data

No destructive migration or historical rewrite is required. Historical usage is derived dynamically from session dates and legacy date ranges.

If external consumers depend on the existing response fields, add the new day-named fields first and temporarily retain old aliases during a documented compatibility window. Within this repository, all consumers should move to the day-named fields together to eliminate ambiguity.

## Error Handling

- Reject candidate session IDs that do not belong to the selected course or valid request dates before limit calculation.
- Preserve the existing structured `absence_limit_exceeded` response code.
- Log database calculation errors with student/course identifiers but no unnecessary personal data.
- Do not silently fall back to physical-session counting if day statistics fail.

## Test Strategy

### Domain tests

- Standard rounding below `.5`, at `.5`, and above `.5`.
- Remaining and exceeded calculations at boundaries.
- Zero-course-day guard behavior.

### Database/integration tests

- Two sessions on the same Bangkok date produce one total course day.
- Two missed sessions in one request produce one used absence day.
- Separate requests for different sessions on the same course date still produce one used absence day.
- The same course on two dates produces two used absence days.
- Different courses on the same date are independent.
- Historical explicit missed sessions are recalculated by distinct date.
- Legacy date-range-only absences derive dates from matching course sessions.
- Cancelled and `special_approved` records are excluded.
- UTC timestamps on opposite sides of a Bangkok midnight boundary map correctly.
- Concurrent requests cannot exceed the rounded maximum.

### Frontend tests

- One same-day group selects and submits every underlying session ID.
- Selecting a two-session group consumes one displayed day.
- Remaining allowance and limit messages use backend day statistics.
- Review and success screens preserve grouped-day presentation.

### Regression tests

- Teacher attendance still sees the student absent from every underlying physical session.
- Sit-in selection and validation continue to operate on physical session IDs.
- Notification details retain all relevant physical session times while absence-limit summaries use day terminology.

## Acceptance Criteria

1. A student missing any number of sessions for one course on one Bangkok date consumes exactly one absence day.
2. Existing and new absence records follow the same calculation.
3. Total allowance is based on distinct course days and standard half-up rounding of 20%.
4. Separate same-day requests cannot consume more than one unit or bypass projection logic.
5. Attendance, sit-in, reporting, and notification features retain the underlying physical-session details.
6. Backend, API, and UI use consistent day-based names and values.
7. Automated tests cover policy, historical calculation, timezone boundaries, concurrency, and affected UI behavior.
