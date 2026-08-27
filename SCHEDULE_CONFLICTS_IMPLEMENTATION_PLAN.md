Yes. I would implement this as a **domain fix**, not as three separate patches to Cross Study, conflicts, and absence.

The core design should be:

```text
course_students
= student belongs to the course

effective student session
= student is actually expected to attend this specific session
```

This preserves the intentional decision in migration `00061`: partial-week Cross Study students remain enrolled in both destination course rosters, while weekday scope controls actual attendance behavior.

## Target business rule

For every `(student, session)`:

```text
explicit excluded
        ↓
      FALSE

explicit included
        ↓
       TRUE

Cross Study assignment covers session course?
        ↓ yes
session weekday ∈ selected weekdays?
        ↓
    TRUE / FALSE

otherwise
        ↓
normal enrolled course_student?
        ↓
       TRUE
```

I would name the concept:

```text
EffectiveStudentSession
```

and the canonical predicate:

```text
student_is_expected_at_session(student_id, session_id)
```

It should become the source of truth for conflict detection, busy ranges, absence eligibility, attendance, calendars, and later any notification/reporting logic.

---

# Implementation plan

### 1. Lock the invariant with failing acceptance tests first

Before changing production behavior, extend the existing Cross Study fixture that already models exactly the useful scenario: destination A `{Tue}`, destination B `{Sat}`, with Tue/Wed and Sat/Sun sessions. Existing tests intentionally assert that the student remains a `course_students` member of both destination courses while only selected weekday sessions get Cross Study attendance rows.

Add acceptance cases covering:

```text
Course A: Tue + Wed
selected: Tue

Course B: Sat + Sun
selected: Sat

Expected:
course_students
  A = enrolled
  B = enrolled

Effective sessions
  Tue = YES
  Wed = NO
  Sat = YES
  Sun = NO

Busy ranges
  Tue = exists
  Wed = absent
  Sat = exists
  Sun = absent

Conflicts
  overlap Tuesday = conflict
  overlap Wednesday = no conflict

Absence
  Tuesday = selectable
  Wednesday = impossible
  Saturday = selectable
  Sunday = impossible
```

**Code locations**

| File                                                                          | Work                                                                   |
| ----------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `backend/internal/crmimport/crossstudy_test.go`                               | Extend existing Tue/Wed/Sat/Sun fixture                                |
| `backend/internal/scheduling/schedule_invariants_integration_test.go`         | Add effective-session scheduling invariant                             |
| `backend/internal/scheduling/course_overlap_add_student_integration_test.go`  | Verify partial Cross Study does not generate false enrollment conflict |
| `backend/internal/httpapi/scheduleconflictshttp/*_test.go`                    | Assert only selected-day conflict reaches `/schedule-conflicts`        |
| New `backend/internal/httpapi/absenceshttp/cross_study_weekday_scope_test.go` | End-to-end absence tests                                               |

The scheduling package already has dedicated invariant, overlap, busy-range, and preflight integration suites, so these tests fit the existing structure rather than creating a separate testing architecture.

---

### 2. Add one canonical DB-level effective-session predicate

The latest migration is currently `00119_schedule_conflicts_overview_indexes.sql`, so create:

```text
backend/db/migrations/
00120_effective_student_session_scope.sql
```

Create the database-level domain predicate:

```text
student_is_expected_at_session(
    student_id UUID,
    session_id UUID
) → BOOLEAN
```

Do **not** put this rule only in Go because `student_busy_ranges` is maintained by database triggers. A Go-only implementation would immediately create two competing definitions again.

The predicate should resolve:

```text
student
  ↓
session → course
  ↓
explicit session override?
  ↓
applicable Cross Study assignment?
  ↓
destination A / merge-group A
    → dest_course_a_weekdays

or

destination B / merge-group B
    → dest_course_b_weekdays
  ↓
ISO weekday in Asia/Bangkok
  ↓
fallback normal course enrollment
```

The timezone must remain `Asia/Bangkok`, matching current Cross Study weekday resolution. The existing Cross Study implementation already calculates weekday via `EXTRACT(ISODOW FROM start_at AT TIME ZONE 'Asia/Bangkok')`.

Merge groups must be supported. Cross Study already expands destination courses through `course_merge_group_members`; effective-session lookup has to reproduce that A-vs-B ownership rather than merely checking the raw destination IDs.

**Important semantic decision:** do not restrict this to `assignment.status='active'`. `SaveAssignment` immediately applies roster effects while the assignment can still be `pending`, so effective-session semantics need to follow any current non-deleted assignment whose roster effects are live.

---

### 3. Fix busy-range materialization at the source

This is the highest-priority downstream correction.

Current `refresh_student_busy_ranges_for_course_student()` materializes all sessions for an enrolled course unless a session has an explicit `excluded` attendance row. That is why a Monday-only Cross Study student becomes busy on Tuesday too.

In migration `00120`, replace the relevant condition with:

```text
only materialize when
student_is_expected_at_session(student_id, session_id) = true
```

Do this for every trigger/function path that builds or refreshes `student_busy_ranges`, including:

```text
course student added
session created
session time changed
session course changed
session restored
session attendance changed
session cancellation/restoration
```

Do not edit old migrations `00008`, `00096`, or `00114`. Override their current function definitions in `00120`.

**Affected historical locations**

| Location                                                                             | Reason                                               |
| ------------------------------------------------------------------------------------ | ---------------------------------------------------- |
| `backend/db/migrations/00008_course_students_incremental_busy.sql`                   | Original incremental busy-range lifecycle            |
| `backend/db/migrations/00096_session_cancel_refresh_busy_ranges.sql`                 | Session cancellation refresh                         |
| `backend/db/migrations/00114_incremental_busy_ranges_preserve_conflict_override.sql` | Current/latest course-student refresh implementation |
| **new `00120_effective_student_session_scope.sql`**                                  | Final authoritative definitions                      |

This makes:

```text
effective student session
          ↓
student_busy_ranges
          ↓
preflight
/schedule-conflicts
```

instead of:

```text
course_students
      ↓
every session
      ↓
student_busy_ranges
```

---

### 4. Rebuild existing incorrect busy ranges

Changing the trigger only fixes future writes. Existing bad rows must be corrected.

Inside the migration or a controlled migration step:

```text
find students with non-deleted partial Cross Study assignments
        ↓
lock affected students/courses consistently
        ↓
delete/rebuild their student_busy_ranges
        ↓
use new effective-session predicate
```

Preserve relevant `conflict_override` semantics when rebuilding; migration `00114` specifically exists to preserve those flags.

After the migration, this invariant must hold:

```text
student_busy_ranges row exists
⇔
student_is_expected_at_session(student_id, session_id)
```

for normal enrolled attendance-derived ranges.

---

### 5. Make scheduling preflight consume the same truth

`AddCourseStudentWithWarningsTx` currently runs a course-level preflight before adding the roster member.

That creates the false warning path:

```text
Cross Study saved
    ↓
student joins full Course A
    ↓
preflight Course A
    ↓
checks Mon + Tue
    ↓
false Tuesday conflict
```

The important files are:

| File                                            | Change                                                                                      |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `backend/internal/scheduling/student_roster.go` | Keep full roster membership, but make course-add preflight scope-aware                      |
| `backend/internal/scheduling/preflight.go`      | Ensure student/course overlap queries operate on effective busy ranges / effective sessions |
| `backend/internal/scheduling/service.go`        | Audit shared overlap helpers for direct roster fallback                                     |
| `backend/internal/scheduling/session_roster.go` | Verify session roster semantics use effective membership                                    |

`preflight.go` already obtains student conflicts through `student_busy_ranges`, so fixing busy-range materialization handles a large part automatically.

However, specifically inspect any fallback that expands `course_students → all sessions`; those must use the effective predicate instead of assuming whole-course attendance.

---

### 6. Fix direct course conflict queries

There is a second path independent from the `/schedule-conflicts` materialized view.

Current:

```text
backend/internal/db/student_conflicts_custom.go
```

`StudentConflictsByCourse` starts from `course_students`, joins all sessions, and only checks for explicit `status='excluded'`.

Change both sides of the comparison:

```text
current session:
student_is_expected_at_session(student, current_session)

other session:
student_is_expected_at_session(student, other_session)
```

Do not duplicate Cross Study weekday SQL directly inside this query.

Afterward:

```text
Cross Study Monday only

Monday × another course
→ conflict

Tuesday × another course
→ no result
```

---

### 7. Let `/schedule-conflicts` remain a consumer of correct busy ranges

The schedule-conflicts page itself already bases student overlaps on `student_busy_ranges`.

Therefore I would **not add Cross Study-specific joins to**:

```text
backend/internal/httpapi/scheduleconflictshttp/queries.go
```

That would leak domain logic into a reporting endpoint.

Instead:

```text
EffectiveStudentSession
        ↓
correct busy ranges
        ↓
existing schedule conflict SQL
```

Only add regression tests to `scheduleconflictshttp`.

This keeps `/schedule-conflicts` generic.

---

### 8. Fix the absence-form session source

This is the direct absence bug.

Current:

```text
backend/internal/httpapi/absenceshttp/routes.go
```

` sessionsInRangeSelectSQL()` currently does:

```text
student
→ course_students
→ course
→ all sessions
```

with active-course and visibility checks, but no Cross Study session-scope check.

Change the student query to require:

```text
student_is_expected_at_session(st.id, sess.id)
```

Keep all existing gates:

```text
course_students.status = enrolled
AND effective session
AND absence_form_visible
AND active course
AND date range
AND session not deleted
```

Apply the same effective-session condition to `sessionsInRangeStaffSelectSQL()` when the query is answering:

> Which classes was this student supposed to attend?

Staff authorization should broaden visibility/access, **not fabricate attendance obligations**.

So staff can see hidden/inactive administrative information where appropriate, but a Monday-only Cross Study student still should not be considered expected on Tuesday.

---

### 9. Add a hard server-side absence submission invariant

Filtering the UI/API listing is insufficient.

A caller could still manually submit:

```text
missed_session_ids = [TuesdaySession]
```

Therefore absence creation must validate:

```text
for every missed_session_id:
    student_is_expected_at_session(student_id, session_id)
    MUST be true
```

If false, reject the transaction with a domain error such as:

```text
session_not_expected
```

Do this in both:

```text
POST /api/v1/absences
POST /api/v1/absences/batch
```

The invariant should be:

> A non-staff-special absence cannot be created for a session the student was not expected to attend.

`routes.go` currently owns both absence routes and the student sessions lookup, making it the immediate integration point.

Longer-term, I would extract that validation from the HTTP handler into the absence application/domain service so staff APIs, batch APIs, imports, and future callers cannot bypass it.

---

### 10. Correct absence-day denominator/business limits

This part is easy to miss.

The current limit math uses:

```text
maximum absence days ≈ round(total course days / 5)
```

Therefore `total_course_days` must mean:

```text
number of course days this student was expected to attend
```

not:

```text
number of days the course itself runs
```

Audit/change:

```text
backend/internal/db/absence_custom.go
```

particularly:

```text
AbsenceDayCountsForCourse
AbsenceDayCountsForMergeGroup
candidate/projected absence-day calculations
```

Current routes call these counters when constructing the absence-form response.

For:

```text
Course:
Mon + Tue for 10 weeks = 20 course dates

Student:
Cross Study Monday only = 10 expected dates
```

the student's absence allowance denominator should be **10**, not 20.

This is important business logic, not just UI correctness.

---

### 11. Keep Cross Study responsible for scope assignment, not downstream interpretation

Keep these existing structures:

```text
backend/internal/crmimport/crossstudy/models.go
backend/internal/httpapi/crmhttp/crossstudy.go
```

They already correctly capture and validate weekday selections.

In:

```text
backend/internal/crmimport/crossstudy/store.go
```

retain:

```text
course_students enrollment
course_roster_overrides
dest_course_a_weekdays
dest_course_b_weekdays
cross-study session_attendance provenance
```

Do **not** change partial students back into non-roster students.

I would rename/refactor conceptually:

```text
insertCrossStudySessionAttendanceWithWarnings
```

toward:

```text
reconcileCrossStudySessionScope
```

because the behavior is really maintaining the student's scoped sessions rather than ordinary attendance entry.

Current save already deletes stale Cross Study attendance and rebuilds selected weekday rows, which is a good starting lifecycle.

---

### 12. Do not solve this by creating `excluded` rows for every unselected session

That is tempting:

```text
Monday selected → included
Tuesday unselected → excluded
```

It would fix several existing queries quickly, but I would **not use it as the source of truth**.

Why:

```text
assignment created
    ↓
exclusions generated

later
    ↓
new session generated
or session moves Wed → Tue
or series regenerated
    ↓
no exclusion exists
    ↓
bug returns
```

The weekday rule belongs to the assignment and should be evaluated against the current session.

`session_attendance` can remain useful for explicit/manual overrides and Cross Study provenance, but correctness should not require every future session to have been pre-materialized.

---

### 13. Define override precedence explicitly

Make this a documented invariant in `00120` and tests:

```text
1. explicit excluded        → not expected
2. explicit included        → expected
3. Cross Study scope        → selected weekday only
4. normal enrolled roster   → expected
5. otherwise                → not expected
```

That gives sensible behavior for exceptional cases:

```text
Cross Study selects Monday
Teacher explicitly excludes Monday
→ false

Cross Study does not select Tuesday
Staff explicitly includes Tuesday as one-off
→ true
```

Make sure ownership/source semantics are considered so Cross Study-generated rows do not accidentally override a manual staff decision.

---

### 14. Handle session edits and newly generated sessions automatically

A Cross Study assignment may exist before a future occurrence is created.

Therefore these lifecycle operations must remain correct without resaving the assignment:

```text
create new session
edit session start date
move session Monday → Tuesday
move session Tuesday → Monday
move session to another course
series regenerate
restore cancelled/deleted session
```

`backend/internal/scheduling/session_change.go` already records occurrence changes including old/new times and course IDs.

The busy-range trigger/predicate solution makes the rule naturally reevaluate from current session data.

Acceptance test:

```text
Student = Monday only

session initially Tuesday
→ not busy

edit session → Monday
→ busy range appears

edit Monday → Tuesday
→ busy range disappears
```

No Cross Study resave should be necessary.

---

### 15. Regression-test merge groups

Cross Study destination expansion now includes merge-group sibling courses, so selected weekday scope must follow those expanded destinations as well. The repository added explicit merge-group destination support in migration `00118`.

Test:

```text
Destination A
├─ Course A1
└─ Course A2
selected weekday = Tue

A1 Tue → expected
A2 Tue → expected
A1 Wed → not expected
A2 Wed → not expected
```

Do the same for destination B.

This is why the effective predicate cannot simply compare:

```text
session.course_id = dest_course_a_id
```

It must understand the group membership.

---

### 16. Add production-data verification before enforcement

Before deploying the new hard absence validation, run an audit query to find historical inconsistencies:

```text
non-cancelled absence
        ↓
missed session
        ↓
student_is_expected_at_session = false
```

Do **not** silently delete old absences.

Produce counts grouped by:

```text
student
course
Cross Study assignment
absence
session date
```

Then decide whether those are historical mistakes or valid staff overrides.

Also audit:

```text
partial Cross Study assignment
×
student_busy_ranges on non-selected weekdays
```

Expected after migration:

```text
count = 0
```

---

## Code-location map

| Priority | Location                                                                             | Responsibility                     | Change                                                                                           |
| -------- | ------------------------------------------------------------------------------------ | ---------------------------------- | ------------------------------------------------------------------------------------------------ |
| P0       | **new** `backend/db/migrations/00120_effective_student_session_scope.sql`            | Canonical domain rule              | Add effective-session predicate; replace busy-range refresh definitions; rebuild affected ranges |
| P0       | `backend/db/migrations/00114_incremental_busy_ranges_preserve_conflict_override.sql` | Current busy-range behavior        | Reference only; supersede via 00120                                                              |
| P0       | `backend/internal/scheduling/student_roster.go`                                      | Course enrollment/preflight        | Make roster add respect effective session scope                                                  |
| P0       | `backend/internal/scheduling/preflight.go`                                           | Conflict preflight                 | Audit all student overlap paths against canonical effective sessions                             |
| P0       | `backend/internal/db/student_conflicts_custom.go`                                    | Direct course conflict query       | Filter both sessions through effective-session predicate                                         |
| P0       | `backend/internal/httpapi/absenceshttp/routes.go`                                    | Absence session listing/submission | Filter effective sessions + reject unowned/unexpected missed sessions                            |
| P0       | `backend/internal/db/absence_custom.go`                                              | Absence denominator/counters       | Count effective student course days, not raw course days                                         |
| P1       | `backend/internal/crmimport/crossstudy/store.go`                                     | Cross Study lifecycle              | Keep roster enrollment; refactor session-scope reconciliation                                    |
| P1       | `backend/internal/crmimport/crossstudy/models.go`                                    | Cross Study model                  | Mostly unchanged; document weekday semantics                                                     |
| P1       | `backend/internal/httpapi/crmhttp/crossstudy.go`                                     | API validation                     | Mostly unchanged                                                                                 |
| P1       | `backend/internal/httpapi/scheduleconflictshttp/queries.go`                          | Conflict overview                  | Ideally no business-rule change; consume corrected busy ranges                                   |
| P1       | `backend/internal/scheduling/session_change.go`                                      | Session lifecycle                  | Verify busy-range refresh correctly follows date/course changes                                  |
| P1       | `backend/internal/scheduling/session_roster.go`                                      | Session-specific roster            | Audit against new invariant                                                                      |
| Test     | `backend/internal/crmimport/crossstudy_test.go`                                      | Cross Study contract               | Extend existing partial-week fixture                                                             |
| Test     | `backend/internal/scheduling/schedule_invariants_integration_test.go`                | Scheduling invariant               | Expected-session ↔ busy-range tests                                                              |
| Test     | `backend/internal/scheduling/course_overlap_add_student_integration_test.go`         | Enrollment conflict                | No false conflict on unselected weekday                                                          |
| Test     | **new** `backend/internal/httpapi/absenceshttp/cross_study_weekday_scope_test.go`    | Absence ATDD                       | Listing, submit guard, limits, merge groups                                                      |

## Desired end-to-end chain

```text
Admin saves Cross Study
        ↓
dest A = Course A
dest A weekdays = Mon
dest B = Course B
dest B weekdays = Thu
        ↓
course_students
A = enrolled
B = enrolled
        ↓
EffectiveStudentSession
A Mon = yes
A Tue = no
B Thu = yes
B Fri = no
        ↓
┌─────────────────────────────┐
│ student_busy_ranges         │
│ scheduling preflight        │
│ schedule conflicts          │
│ absence form                │
│ absence submission          │
│ absence day allowance       │
│ attendance                  │
│ makeup / sit-in             │
│ student calendar            │
└─────────────────────────────┘
        ↓
all consume the SAME truth
```

The key rule for implementation is: **do not teach each feature what “Cross Study” means. Teach the scheduling domain what “student is expected at this session” means.** Cross Study then becomes only one producer of student-session scope, while conflict, absence, attendance, and future features remain provider-neutral.

Yes. I would implement this as a **domain fix**, not as three separate patches to Cross Study, conflicts, and absence.

The core design should be:

```text
course_students
= student belongs to the course

effective student session
= student is actually expected to attend this specific session
```

This preserves the intentional decision in migration `00061`: partial-week Cross Study students remain enrolled in both destination course rosters, while weekday scope controls actual attendance behavior.

## Target business rule

For every `(student, session)`:

```text
explicit excluded
        ↓
      FALSE

explicit included
        ↓
       TRUE

Cross Study assignment covers session course?
        ↓ yes
session weekday ∈ selected weekdays?
        ↓
    TRUE / FALSE

otherwise
        ↓
normal enrolled course_student?
        ↓
       TRUE
```

I would name the concept:

```text
EffectiveStudentSession
```

and the canonical predicate:

```text
student_is_expected_at_session(student_id, session_id)
```

It should become the source of truth for conflict detection, busy ranges, absence eligibility, attendance, calendars, and later any notification/reporting logic.

---

# Implementation plan

### 1. Lock the invariant with failing acceptance tests first

Before changing production behavior, extend the existing Cross Study fixture that already models exactly the useful scenario: destination A `{Tue}`, destination B `{Sat}`, with Tue/Wed and Sat/Sun sessions. Existing tests intentionally assert that the student remains a `course_students` member of both destination courses while only selected weekday sessions get Cross Study attendance rows.

Add acceptance cases covering:

```text
Course A: Tue + Wed
selected: Tue

Course B: Sat + Sun
selected: Sat

Expected:
course_students
  A = enrolled
  B = enrolled

Effective sessions
  Tue = YES
  Wed = NO
  Sat = YES
  Sun = NO

Busy ranges
  Tue = exists
  Wed = absent
  Sat = exists
  Sun = absent

Conflicts
  overlap Tuesday = conflict
  overlap Wednesday = no conflict

Absence
  Tuesday = selectable
  Wednesday = impossible
  Saturday = selectable
  Sunday = impossible
```

**Code locations**

| File                                                                          | Work                                                                   |
| ----------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `backend/internal/crmimport/crossstudy_test.go`                               | Extend existing Tue/Wed/Sat/Sun fixture                                |
| `backend/internal/scheduling/schedule_invariants_integration_test.go`         | Add effective-session scheduling invariant                             |
| `backend/internal/scheduling/course_overlap_add_student_integration_test.go`  | Verify partial Cross Study does not generate false enrollment conflict |
| `backend/internal/httpapi/scheduleconflictshttp/*_test.go`                    | Assert only selected-day conflict reaches `/schedule-conflicts`        |
| New `backend/internal/httpapi/absenceshttp/cross_study_weekday_scope_test.go` | End-to-end absence tests                                               |

The scheduling package already has dedicated invariant, overlap, busy-range, and preflight integration suites, so these tests fit the existing structure rather than creating a separate testing architecture.

---

### 2. Add one canonical DB-level effective-session predicate

The latest migration is currently `00119_schedule_conflicts_overview_indexes.sql`, so create:

```text
backend/db/migrations/
00120_effective_student_session_scope.sql
```

Create the database-level domain predicate:

```text
student_is_expected_at_session(
    student_id UUID,
    session_id UUID
) → BOOLEAN
```

Do **not** put this rule only in Go because `student_busy_ranges` is maintained by database triggers. A Go-only implementation would immediately create two competing definitions again.

The predicate should resolve:

```text
student
  ↓
session → course
  ↓
explicit session override?
  ↓
applicable Cross Study assignment?
  ↓
destination A / merge-group A
    → dest_course_a_weekdays

or

destination B / merge-group B
    → dest_course_b_weekdays
  ↓
ISO weekday in Asia/Bangkok
  ↓
fallback normal course enrollment
```

The timezone must remain `Asia/Bangkok`, matching current Cross Study weekday resolution. The existing Cross Study implementation already calculates weekday via `EXTRACT(ISODOW FROM start_at AT TIME ZONE 'Asia/Bangkok')`.

Merge groups must be supported. Cross Study already expands destination courses through `course_merge_group_members`; effective-session lookup has to reproduce that A-vs-B ownership rather than merely checking the raw destination IDs.

**Important semantic decision:** do not restrict this to `assignment.status='active'`. `SaveAssignment` immediately applies roster effects while the assignment can still be `pending`, so effective-session semantics need to follow any current non-deleted assignment whose roster effects are live.

---

### 3. Fix busy-range materialization at the source

This is the highest-priority downstream correction.

Current `refresh_student_busy_ranges_for_course_student()` materializes all sessions for an enrolled course unless a session has an explicit `excluded` attendance row. That is why a Monday-only Cross Study student becomes busy on Tuesday too.

In migration `00120`, replace the relevant condition with:

```text
only materialize when
student_is_expected_at_session(student_id, session_id) = true
```

Do this for every trigger/function path that builds or refreshes `student_busy_ranges`, including:

```text
course student added
session created
session time changed
session course changed
session restored
session attendance changed
session cancellation/restoration
```

Do not edit old migrations `00008`, `00096`, or `00114`. Override their current function definitions in `00120`.

**Affected historical locations**

| Location                                                                             | Reason                                               |
| ------------------------------------------------------------------------------------ | ---------------------------------------------------- |
| `backend/db/migrations/00008_course_students_incremental_busy.sql`                   | Original incremental busy-range lifecycle            |
| `backend/db/migrations/00096_session_cancel_refresh_busy_ranges.sql`                 | Session cancellation refresh                         |
| `backend/db/migrations/00114_incremental_busy_ranges_preserve_conflict_override.sql` | Current/latest course-student refresh implementation |
| **new `00120_effective_student_session_scope.sql`**                                  | Final authoritative definitions                      |

This makes:

```text
effective student session
          ↓
student_busy_ranges
          ↓
preflight
/schedule-conflicts
```

instead of:

```text
course_students
      ↓
every session
      ↓
student_busy_ranges
```

---

### 4. Rebuild existing incorrect busy ranges

Changing the trigger only fixes future writes. Existing bad rows must be corrected.

Inside the migration or a controlled migration step:

```text
find students with non-deleted partial Cross Study assignments
        ↓
lock affected students/courses consistently
        ↓
delete/rebuild their student_busy_ranges
        ↓
use new effective-session predicate
```

Preserve relevant `conflict_override` semantics when rebuilding; migration `00114` specifically exists to preserve those flags.

After the migration, this invariant must hold:

```text
student_busy_ranges row exists
⇔
student_is_expected_at_session(student_id, session_id)
```

for normal enrolled attendance-derived ranges.

---

### 5. Make scheduling preflight consume the same truth

`AddCourseStudentWithWarningsTx` currently runs a course-level preflight before adding the roster member.

That creates the false warning path:

```text
Cross Study saved
    ↓
student joins full Course A
    ↓
preflight Course A
    ↓
checks Mon + Tue
    ↓
false Tuesday conflict
```

The important files are:

| File                                            | Change                                                                                      |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `backend/internal/scheduling/student_roster.go` | Keep full roster membership, but make course-add preflight scope-aware                      |
| `backend/internal/scheduling/preflight.go`      | Ensure student/course overlap queries operate on effective busy ranges / effective sessions |
| `backend/internal/scheduling/service.go`        | Audit shared overlap helpers for direct roster fallback                                     |
| `backend/internal/scheduling/session_roster.go` | Verify session roster semantics use effective membership                                    |

`preflight.go` already obtains student conflicts through `student_busy_ranges`, so fixing busy-range materialization handles a large part automatically.

However, specifically inspect any fallback that expands `course_students → all sessions`; those must use the effective predicate instead of assuming whole-course attendance.

---

### 6. Fix direct course conflict queries

There is a second path independent from the `/schedule-conflicts` materialized view.

Current:

```text
backend/internal/db/student_conflicts_custom.go
```

`StudentConflictsByCourse` starts from `course_students`, joins all sessions, and only checks for explicit `status='excluded'`.

Change both sides of the comparison:

```text
current session:
student_is_expected_at_session(student, current_session)

other session:
student_is_expected_at_session(student, other_session)
```

Do not duplicate Cross Study weekday SQL directly inside this query.

Afterward:

```text
Cross Study Monday only

Monday × another course
→ conflict

Tuesday × another course
→ no result
```

---

### 7. Let `/schedule-conflicts` remain a consumer of correct busy ranges

The schedule-conflicts page itself already bases student overlaps on `student_busy_ranges`.

Therefore I would **not add Cross Study-specific joins to**:

```text
backend/internal/httpapi/scheduleconflictshttp/queries.go
```

That would leak domain logic into a reporting endpoint.

Instead:

```text
EffectiveStudentSession
        ↓
correct busy ranges
        ↓
existing schedule conflict SQL
```

Only add regression tests to `scheduleconflictshttp`.

This keeps `/schedule-conflicts` generic.

---

### 8. Fix the absence-form session source

This is the direct absence bug.

Current:

```text
backend/internal/httpapi/absenceshttp/routes.go
```

` sessionsInRangeSelectSQL()` currently does:

```text
student
→ course_students
→ course
→ all sessions
```

with active-course and visibility checks, but no Cross Study session-scope check.

Change the student query to require:

```text
student_is_expected_at_session(st.id, sess.id)
```

Keep all existing gates:

```text
course_students.status = enrolled
AND effective session
AND absence_form_visible
AND active course
AND date range
AND session not deleted
```

Apply the same effective-session condition to `sessionsInRangeStaffSelectSQL()` when the query is answering:

> Which classes was this student supposed to attend?

Staff authorization should broaden visibility/access, **not fabricate attendance obligations**.

So staff can see hidden/inactive administrative information where appropriate, but a Monday-only Cross Study student still should not be considered expected on Tuesday.

---

### 9. Add a hard server-side absence submission invariant

Filtering the UI/API listing is insufficient.

A caller could still manually submit:

```text
missed_session_ids = [TuesdaySession]
```

Therefore absence creation must validate:

```text
for every missed_session_id:
    student_is_expected_at_session(student_id, session_id)
    MUST be true
```

If false, reject the transaction with a domain error such as:

```text
session_not_expected
```

Do this in both:

```text
POST /api/v1/absences
POST /api/v1/absences/batch
```

The invariant should be:

> A non-staff-special absence cannot be created for a session the student was not expected to attend.

`routes.go` currently owns both absence routes and the student sessions lookup, making it the immediate integration point.

Longer-term, I would extract that validation from the HTTP handler into the absence application/domain service so staff APIs, batch APIs, imports, and future callers cannot bypass it.

---

### 10. Correct absence-day denominator/business limits

This part is easy to miss.

The current limit math uses:

```text
maximum absence days ≈ round(total course days / 5)
```

Therefore `total_course_days` must mean:

```text
number of course days this student was expected to attend
```

not:

```text
number of days the course itself runs
```

Audit/change:

```text
backend/internal/db/absence_custom.go
```

particularly:

```text
AbsenceDayCountsForCourse
AbsenceDayCountsForMergeGroup
candidate/projected absence-day calculations
```

Current routes call these counters when constructing the absence-form response.

For:

```text
Course:
Mon + Tue for 10 weeks = 20 course dates

Student:
Cross Study Monday only = 10 expected dates
```

the student's absence allowance denominator should be **10**, not 20.

This is important business logic, not just UI correctness.

---

### 11. Keep Cross Study responsible for scope assignment, not downstream interpretation

Keep these existing structures:

```text
backend/internal/crmimport/crossstudy/models.go
backend/internal/httpapi/crmhttp/crossstudy.go
```

They already correctly capture and validate weekday selections.

In:

```text
backend/internal/crmimport/crossstudy/store.go
```

retain:

```text
course_students enrollment
course_roster_overrides
dest_course_a_weekdays
dest_course_b_weekdays
cross-study session_attendance provenance
```

Do **not** change partial students back into non-roster students.

I would rename/refactor conceptually:

```text
insertCrossStudySessionAttendanceWithWarnings
```

toward:

```text
reconcileCrossStudySessionScope
```

because the behavior is really maintaining the student's scoped sessions rather than ordinary attendance entry.

Current save already deletes stale Cross Study attendance and rebuilds selected weekday rows, which is a good starting lifecycle.

---

### 12. Do not solve this by creating `excluded` rows for every unselected session

That is tempting:

```text
Monday selected → included
Tuesday unselected → excluded
```

It would fix several existing queries quickly, but I would **not use it as the source of truth**.

Why:

```text
assignment created
    ↓
exclusions generated

later
    ↓
new session generated
or session moves Wed → Tue
or series regenerated
    ↓
no exclusion exists
    ↓
bug returns
```

The weekday rule belongs to the assignment and should be evaluated against the current session.

`session_attendance` can remain useful for explicit/manual overrides and Cross Study provenance, but correctness should not require every future session to have been pre-materialized.

---

### 13. Define override precedence explicitly

Make this a documented invariant in `00120` and tests:

```text
1. explicit excluded        → not expected
2. explicit included        → expected
3. Cross Study scope        → selected weekday only
4. normal enrolled roster   → expected
5. otherwise                → not expected
```

That gives sensible behavior for exceptional cases:

```text
Cross Study selects Monday
Teacher explicitly excludes Monday
→ false

Cross Study does not select Tuesday
Staff explicitly includes Tuesday as one-off
→ true
```

Make sure ownership/source semantics are considered so Cross Study-generated rows do not accidentally override a manual staff decision.

---

### 14. Handle session edits and newly generated sessions automatically

A Cross Study assignment may exist before a future occurrence is created.

Therefore these lifecycle operations must remain correct without resaving the assignment:

```text
create new session
edit session start date
move session Monday → Tuesday
move session Tuesday → Monday
move session to another course
series regenerate
restore cancelled/deleted session
```

`backend/internal/scheduling/session_change.go` already records occurrence changes including old/new times and course IDs.

The busy-range trigger/predicate solution makes the rule naturally reevaluate from current session data.

Acceptance test:

```text
Student = Monday only

session initially Tuesday
→ not busy

edit session → Monday
→ busy range appears

edit Monday → Tuesday
→ busy range disappears
```

No Cross Study resave should be necessary.

---

### 15. Regression-test merge groups

Cross Study destination expansion now includes merge-group sibling courses, so selected weekday scope must follow those expanded destinations as well. The repository added explicit merge-group destination support in migration `00118`.

Test:

```text
Destination A
├─ Course A1
└─ Course A2
selected weekday = Tue

A1 Tue → expected
A2 Tue → expected
A1 Wed → not expected
A2 Wed → not expected
```

Do the same for destination B.

This is why the effective predicate cannot simply compare:

```text
session.course_id = dest_course_a_id
```

It must understand the group membership.

---

### 16. Add production-data verification before enforcement

Before deploying the new hard absence validation, run an audit query to find historical inconsistencies:

```text
non-cancelled absence
        ↓
missed session
        ↓
student_is_expected_at_session = false
```

Do **not** silently delete old absences.

Produce counts grouped by:

```text
student
course
Cross Study assignment
absence
session date
```

Then decide whether those are historical mistakes or valid staff overrides.

Also audit:

```text
partial Cross Study assignment
×
student_busy_ranges on non-selected weekdays
```

Expected after migration:

```text
count = 0
```

---

## Code-location map

| Priority | Location                                                                             | Responsibility                     | Change                                                                                           |
| -------- | ------------------------------------------------------------------------------------ | ---------------------------------- | ------------------------------------------------------------------------------------------------ |
| P0       | **new** `backend/db/migrations/00120_effective_student_session_scope.sql`            | Canonical domain rule              | Add effective-session predicate; replace busy-range refresh definitions; rebuild affected ranges |
| P0       | `backend/db/migrations/00114_incremental_busy_ranges_preserve_conflict_override.sql` | Current busy-range behavior        | Reference only; supersede via 00120                                                              |
| P0       | `backend/internal/scheduling/student_roster.go`                                      | Course enrollment/preflight        | Make roster add respect effective session scope                                                  |
| P0       | `backend/internal/scheduling/preflight.go`                                           | Conflict preflight                 | Audit all student overlap paths against canonical effective sessions                             |
| P0       | `backend/internal/db/student_conflicts_custom.go`                                    | Direct course conflict query       | Filter both sessions through effective-session predicate                                         |
| P0       | `backend/internal/httpapi/absenceshttp/routes.go`                                    | Absence session listing/submission | Filter effective sessions + reject unowned/unexpected missed sessions                            |
| P0       | `backend/internal/db/absence_custom.go`                                              | Absence denominator/counters       | Count effective student course days, not raw course days                                         |
| P1       | `backend/internal/crmimport/crossstudy/store.go`                                     | Cross Study lifecycle              | Keep roster enrollment; refactor session-scope reconciliation                                    |
| P1       | `backend/internal/crmimport/crossstudy/models.go`                                    | Cross Study model                  | Mostly unchanged; document weekday semantics                                                     |
| P1       | `backend/internal/httpapi/crmhttp/crossstudy.go`                                     | API validation                     | Mostly unchanged                                                                                 |
| P1       | `backend/internal/httpapi/scheduleconflictshttp/queries.go`                          | Conflict overview                  | Ideally no business-rule change; consume corrected busy ranges                                   |
| P1       | `backend/internal/scheduling/session_change.go`                                      | Session lifecycle                  | Verify busy-range refresh correctly follows date/course changes                                  |
| P1       | `backend/internal/scheduling/session_roster.go`                                      | Session-specific roster            | Audit against new invariant                                                                      |
| Test     | `backend/internal/crmimport/crossstudy_test.go`                                      | Cross Study contract               | Extend existing partial-week fixture                                                             |
| Test     | `backend/internal/scheduling/schedule_invariants_integration_test.go`                | Scheduling invariant               | Expected-session ↔ busy-range tests                                                              |
| Test     | `backend/internal/scheduling/course_overlap_add_student_integration_test.go`         | Enrollment conflict                | No false conflict on unselected weekday                                                          |
| Test     | **new** `backend/internal/httpapi/absenceshttp/cross_study_weekday_scope_test.go`    | Absence ATDD                       | Listing, submit guard, limits, merge groups                                                      |

## Desired end-to-end chain

```text
Admin saves Cross Study
        ↓
dest A = Course A
dest A weekdays = Mon
dest B = Course B
dest B weekdays = Thu
        ↓
course_students
A = enrolled
B = enrolled
        ↓
EffectiveStudentSession
A Mon = yes
A Tue = no
B Thu = yes
B Fri = no
        ↓
┌─────────────────────────────┐
│ student_busy_ranges         │
│ scheduling preflight        │
│ schedule conflicts          │
│ absence form                │
│ absence submission          │
│ absence day allowance       │
│ attendance                  │
│ makeup / sit-in             │
│ student calendar            │
└─────────────────────────────┘
        ↓
all consume the SAME truth
```

The key rule for implementation is: **do not teach each feature what “Cross Study” means. Teach the scheduling domain what “student is expected at this session” means.** Cross Study then becomes only one producer of student-session scope, while conflict, absence, attendance, and future features remain provider-neutral.
