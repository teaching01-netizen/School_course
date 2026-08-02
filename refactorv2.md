## What will change

The main change is this:

**Course and schedule operations will stop behaving like simple database edits and start behaving like protected business operations.**

Today, some actions directly insert, update, or delete rows. After the changes, every action will validate the full impact, lock related data, save everything together, and keep a history of what happened.

---

## 1. Course teachers

### Today

A course stores a main teacher in one place and additional teachers in another place.

This can create inconsistent data. For example:

* The main teacher may not appear in the teacher list.
* One teacher update may succeed while another part fails.
* Invalid teacher IDs may be ignored.
* The user may see “Course updated” even though some teacher changes failed.

### After the change

There will be one official teacher list for each course.

One teacher can be marked as the primary teacher.

When updating teachers, the system will:

1. Validate every teacher.
2. Confirm each user exists and is allowed to teach.
3. Reject duplicates and invalid IDs.
4. Save the complete teacher list in one transaction.
5. Roll back everything if any part fails.

Removing a teacher who still has future classes will be blocked until those classes are reassigned.

---

## 2. Concurrent course editing

### Today

Two staff members can open the same course and save different changes.

The second person may accidentally overwrite the first person’s update.

### After the change

Every course will have a version number.

When someone saves:

* The system checks that they edited the latest version.
* If someone else already changed it, the save is rejected.
* The user sees the latest course data and can review the differences.

This prevents silent overwrites.

---

## 3. Course deletion

### Today

Deleting a course can physically remove the database record.

Depending on database relationships, this may remove related information or fail unexpectedly.

### After the change

A course will normally be **archived**, not deleted.

Archiving means:

* The course disappears from normal active-course screens.
* No new sessions or students can be added.
* Past sessions, attendance, audit records, and reports remain available.

A course with future sessions cannot be archived until those sessions are canceled or handled explicitly.

Permanent deletion will only be available as a restricted maintenance operation.

---

## 4. Session deletion

### Today

The interface says a session is permanently deleted, and the session row can be physically removed.

This is dangerous because other records may refer to that session.

### After the change

Users will **cancel a session** instead of deleting it.

The canceled session remains in history with:

* Who canceled it.
* When it was canceled.
* Why it was canceled.
* Its previous teacher, room, date, and time.
* Any students or operational issues affected.

A canceled session will no longer block teacher, room, or student availability.

---

## 5. Student roster ownership

### Today

A course roster may be managed manually or by CRM.

The system sometimes checks this before starting the transaction. That means CRM settings could change between the check and the actual student update.

### After the change

Every course will clearly declare its roster source:

* Manual
* CRM

When staff add or remove a student, the system will:

1. Lock the course.
2. Check the roster source inside the transaction.
3. Reject the change if CRM owns the roster.
4. Validate the student against every affected future session.
5. Save the roster and schedule occupancy together.

Manual changes and CRM synchronization will not be able to modify the same roster at the same time.

---

## 6. Adding a student becomes a scheduling operation

Adding a student is not only adding them to a list.

The student may already have another class at the same time as one of the course’s sessions.

### After the change

When adding or enrolling a student, the system checks:

* Every relevant future session.
* Student schedule conflicts.
* Session-specific inclusion or exclusion rules.
* Room capacity where applicable.

If one future session conflicts, the student is not added and the response explains which sessions caused the problem.

No partial roster update will occur.

---

## 7. CRM synchronization

### Today

CRM synchronization may use its own path for changing students or schedules.

Different paths can enforce different rules.

### After the change

CRM synchronization will use the same roster and scheduling services as normal staff actions.

The CRM sync will:

1. Compare the current roster with the CRM roster.
2. Calculate additions and removals.
3. Check schedule conflicts.
4. Apply the whole change atomically.
5. Record which CRM snapshot was applied.
6. Avoid applying the same snapshot twice.

A failed CRM sync will return a clear report instead of silently skipping students.

---

## 8. Recurring schedules

### Today

A recurring series creates real session rows.

When the recurrence changes, future sessions may be deleted and recreated.

That can change session IDs and break references to attendance, absence issues, or other operational data.

### After the change

Each recurring occurrence will have a permanent identity based on its intended recurrence date.

When a recurring schedule changes, the system will compare the old and new schedules:

* Existing occurrence still needed: update it in place.
* New occurrence required: create it.
* Old occurrence no longer required: cancel it.
* Past occurrence: do not change it.

This keeps existing session IDs whenever possible and preserves history.

---

## 9. “Edit this and future sessions”

### Today

The system splits a recurring series around a selected session.

The logic is already fairly strong, but the identity of a moved occurrence can become difficult to follow.

### After the change

The selected occurrence will be identified by its original recurrence date, not only by its current start time.

For example:

* A weekly Monday session originally belonging to August 3 is moved to August 4.
* It still remains the “August 3 occurrence.”
* Later series edits can correctly understand that it is an exception.

This prevents moved sessions from being duplicated or lost.

---

## 10. Attendance and session roster exceptions

### Today

The same table may contain both:

* Whether a student is included or excluded from a session.
* Whether the student attended, was absent, or was late.

These are different concepts.

### After the change

They will be separated.

One table will describe **who should attend**:

* Included
* Excluded

Another table will describe **what actually happened**:

* Present
* Absent
* Late
* Other attendance states

This makes scheduling, attendance, reporting, and absence handling easier to reason about.

---

## 11. Schedule preflight

### Today

The frontend checks whether a time appears available.

The backend checks again during the write, which is correct.

For a large recurring series, the backend may perform many repeated checks.

### After the change

The frontend check will still be treated as a preview, not a reservation.

The interface will say:

> No conflicts currently found

When saving, the server will recheck everything inside the transaction.

For recurring schedules, the backend will check all proposed occurrences in batches instead of running many separate queries.

This improves speed while keeping the database check as the final source of truth.

---

## 12. Pasting many sessions

### Today

The frontend creates pasted sessions one at a time.

For example, if there are 20 rows and row 11 fails:

* Rows 1–10 may already be created.
* Rows 11–20 are not created.
* The user is left with a partial result.

### After the change

The frontend sends one batch request to the backend.

The user can choose one of two behaviors:

### Atomic mode

Either all sessions are created or none are created.

### Best-effort mode

Valid sessions are created, invalid sessions are rejected, and the response clearly shows the result of every row.

The server also checks conflicts between rows in the pasted batch itself.

---

## 13. Course schedule loading

### Today

Opening a course can load every active session for that course.

A course with thousands of historical sessions will become slow.

### After the change

The frontend will request only the required date range.

For example:

* Calendar view loads the visible week.
* Upcoming table loads one page.
* Historical sessions load only when the user opens history.
* Canceled sessions can be filtered separately.

This reduces response size and frontend work.

---

## 14. Seven-day calendar

### Today

Recurring schedules support Monday through Sunday, but the course calendar displays only Monday through Friday.

Weekend sessions can exist but may not appear in the calendar view.

### After the change

The same seven-day weekday definition will be used everywhere:

* Recurrence form.
* Calendar.
* Session grouping.
* Tests.

Saturday and Sunday sessions will be visible.

---

## 15. Overnight sessions

### Today

A form may use one date with a start and end time.

A session such as 11:00 PM to 1:00 AM is unclear because the end time is technically on the next day.

### After the change

The interface will explicitly support either:

* A separate end date, or
* An “Ends next day” option.

The system will never silently guess.

---

## 16. Roomless sessions

### Today

A session without a room is marked provisional, but it can look similar to a complete schedule.

### After the change

A roomless session becomes an explicit operational state.

The user will see:

* “Room not assigned.”
* A provisional badge.
* Filters and counts.
* A warning before check-in.
* An action to assign a room.

It remains allowed if the business wants provisional scheduling, but it will not look complete.

---

## 17. Legacy schedule synchronization

### Today

Legacy synchronization may directly create or update records through a separate implementation path.

### After the change

Legacy sync will first build a proposed schedule difference.

It will then use the normal scheduling rules to:

* Validate teachers, rooms, and students.
* Detect conflicts.
* Create, update, or cancel sessions.
* Preserve manual exceptions.
* Apply changes atomically.
* Record the source snapshot.

Repeated synchronization of the same source data will not create duplicates.

---

## 18. Audit and realtime events

### Today

The database operation may commit successfully, but publishing the notification or realtime event can fail afterward.

This can leave other screens unaware of the change.

### After the change

The database transaction will also write an event into an outbox table.

A background worker will reliably publish that event.

This means:

* The business change and its event are saved together.
* Failed event delivery can be retried.
* Duplicate event processing can be prevented.
* Operational teams can monitor delayed events.

---

## 19. Error messages

Errors will become stable and actionable.

Instead of a generic update failure, the user may see:

* Another user changed this course. Reload the latest version.
* This teacher still owns 14 future sessions.
* This roster is managed by CRM.
* The room is too small for the effective roster.
* The student conflicts with three future sessions.
* The course must cancel future sessions before being archived.
* This local time does not exist because of a timezone transition.

The frontend will use the error code to show the correct next action.

---

# The biggest logic changes

In simple terms, these are the most important changes:

1. **Delete becomes archive or cancel.**
2. **Teacher updates become all-or-nothing.**
3. **Concurrent edits cannot silently overwrite each other.**
4. **Adding students checks their full future schedule.**
5. **CRM and staff cannot update the same roster simultaneously.**
6. **Recurring schedule edits update existing sessions instead of destroying and recreating them.**
7. **Attendance is separated from who is expected to attend.**
8. **Bulk session creation moves from the browser to one backend transaction.**
9. **Large schedule checks use batch queries.**
10. **Every important change keeps a durable history and reliable event.**

The result is a system that prefers **rejecting an unsafe operation with a clear explanation** over accepting it and leaving inconsistent data.

# Production-Grade Course Teacher Implementation Plan

## 1. Confirmed Scope

### Multiple teachers

A course may have:

* No assigned teacher while it is being prepared.
* One teacher.
* Multiple teachers.
* Optionally, one teacher marked as the primary/default teacher.

All assigned teachers are legitimate course teachers. The primary teacher is only the default selection for new sessions; it does not mean the other teachers are less valid.

### Session teachers

A course may have several teachers, but each individual session currently has one responsible `teacher_id`.

Therefore:

* A session teacher must normally belong to that course’s assigned teacher set.
* Different sessions in the same course may use different assigned teachers.
* Historical sessions keep their original teacher even if that teacher is later removed from the course.
* Removing a teacher is blocked when that teacher still owns future sessions.

Session-level co-teaching is not included in this implementation.

### Deletion

Deletion remains unchanged.

This implementation must not modify:

* `handleCoursesDelete`
* `CourseDelete`
* Session delete behavior
* Delete API routes
* Delete confirmation language
* Database cascade behavior

Deletion should be handled in a separate future decision after the business behavior is confirmed.

---

# 2. Problems Being Fixed

The current course create and update handlers directly mutate `course_teachers`.

Some failures are logged and ignored, which can allow the request to return success even when the complete teacher update did not succeed. The current update path also stores the first teacher in `courses.teacher_id`, creating two representations that can drift.

The implementation must fix these problems:

1. Invalid teacher IDs can be skipped.
2. Teacher insert failures can be ignored.
3. Teacher delete failures can be ignored.
4. Course data and teacher data can partially diverge.
5. `courses.teacher_id` and `course_teachers` can disagree.
6. Two administrators can overwrite each other’s teacher changes.
7. A teacher can potentially be removed while future sessions still use them.
8. Scheduling does not have one explicit guarantee that the selected session teacher belongs to the course.
9. HTTP handlers contain business logic and raw mutation SQL.
10. Course reads assemble teacher data separately instead of using one domain query.

---

# 3. Target Business Rules

These are the invariants the implementation must enforce.

## 3.1 Teacher-set rules

A course has a teacher set containing zero or more unique teachers.

```text
course.teacher_count >= 0
```

Each assignment contains:

```text
teacher_id
is_primary
```

Rules:

1. Teacher IDs must be unique.
2. Every teacher must exist.
3. Every teacher must be active.
4. Every teacher must have an allowed teaching role.
5. Zero or one teacher may be primary.
6. A primary teacher must belong to the teacher set.
7. An empty teacher set must have no primary teacher.
8. The entire teacher set is replaced atomically.
9. Invalid teachers reject the entire request.
10. No invalid teacher is silently skipped.

## 3.2 Primary teacher rules

The primary teacher is optional.

Examples:

```text
Teachers: [A, B]
Primary: A
```

Valid.

```text
Teachers: [A, B]
Primary: none
```

Also valid.

```text
Teachers: [A, B]
Primary: C
```

Invalid because C is not assigned to the course.

The primary teacher should be used as:

* The default teacher in the create-session form.
* The default teacher in the create-series form.
* The first teacher displayed in course summaries.

It should not automatically replace teachers on existing sessions.

## 3.3 Future-session rule

A teacher cannot be removed from the course while that teacher owns active future sessions for the course.

The system returns:

```text
409 teacher_in_use
```

with:

* Teacher ID.
* Teacher name.
* Future-session count.
* Earliest affected session.
* A limited list of affected session IDs.
* Affected series IDs where relevant.

Past sessions do not block removal.

## 3.4 Scheduling rule

For new or changed future schedules:

```text
session.teacher_id must belong to course.teacher_ids
```

This applies to:

* One-off session creation.
* Recurring series creation.
* Individual session editing.
* Bulk session creation.
* Pasted schedule creation.
* Slot finding where a teacher is supplied.
* Legacy synchronization when creating or changing sessions.

The check must happen again inside the authoritative write transaction.

## 3.5 Concurrency rule

Every course has a version.

Updating a course requires:

```text
expected_version
```

If the course was changed after the user loaded it:

```text
409 stale_edit
```

The server returns the latest representation so the frontend can reload.

---

# 4. Target Data Model

## 4.1 Canonical source

`course_teachers` becomes the canonical teacher membership table.

`courses.teacher_id` remains temporarily as a compatibility projection.

During the migration period:

* `course_teachers` is authoritative.
* `courses.teacher_id` mirrors the optional primary teacher.
* Application code must never independently update one without the other.
* Only the new course service may update them.

Later, after all readers are migrated, `courses.teacher_id` can be removed in a separate migration. Do not remove it during the first implementation.

## 4.2 Database migration

Suggested file:

```text
backend/db/migrations/<timestamp>_course_teacher_integrity.sql
```

```sql
ALTER TABLE courses
    ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;

ALTER TABLE course_teachers
    ADD COLUMN IF NOT EXISTS is_primary boolean NOT NULL DEFAULT false;
```

Backfill existing primary assignments:

```sql
UPDATE course_teachers
SET is_primary = false
WHERE is_primary = true;

INSERT INTO course_teachers (
    course_id,
    teacher_id,
    is_primary
)
SELECT
    c.id,
    c.teacher_id,
    true
FROM courses c
WHERE c.teacher_id IS NOT NULL
ON CONFLICT (course_id, teacher_id)
DO UPDATE SET is_primary = true;
```

Create a partial unique index so a course can have many teachers but no more than one primary teacher:

```sql
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS
    ux_course_teachers_one_primary
ON course_teachers (course_id)
WHERE is_primary = true;
```

`CREATE INDEX CONCURRENTLY` should be placed in a non-transactional migration if the migration framework wraps migrations in transactions.

## 4.3 Pre-migration verification

Before creating the unique index, run:

```sql
SELECT
    course_id,
    count(*) AS primary_count
FROM course_teachers
WHERE is_primary = true
GROUP BY course_id
HAVING count(*) > 1;
```

Also detect disagreement between the legacy primary field and the teacher set:

```sql
SELECT
    c.id AS course_id,
    c.teacher_id
FROM courses c
LEFT JOIN course_teachers ct
    ON ct.course_id = c.id
   AND ct.teacher_id = c.teacher_id
WHERE c.teacher_id IS NOT NULL
  AND ct.teacher_id IS NULL;
```

The migration must fail or pause if unhandled anomalies remain.

---

# 5. API Contract

## 5.1 Recommended course update request

Introduce a new versioned update contract:

```text
PATCH /api/v1/courses/{course_id}
```

Request:

```json
{
  "expected_version": 12,
  "code": "MATH-101",
  "name": "Mathematics",
  "legacy_course_id": null,
  "teachers": [
    {
      "teacher_id": "11111111-1111-1111-1111-111111111111",
      "is_primary": true
    },
    {
      "teacher_id": "22222222-2222-2222-2222-222222222222",
      "is_primary": false
    }
  ]
}
```

An empty teacher set is valid:

```json
{
  "expected_version": 12,
  "code": "MATH-101",
  "name": "Mathematics",
  "teachers": []
}
```

## 5.2 Response

```json
{
  "id": "course-id",
  "version": 13,
  "code": "MATH-101",
  "name": "Mathematics",
  "primary_teacher_id": "11111111-1111-1111-1111-111111111111",
  "teachers": [
    {
      "id": "11111111-1111-1111-1111-111111111111",
      "username": "Teacher A",
      "is_primary": true
    },
    {
      "id": "22222222-2222-2222-2222-222222222222",
      "username": "Teacher B",
      "is_primary": false
    }
  ]
}
```

## 5.3 Stable errors

### Invalid teacher

```json
{
  "code": "invalid_teacher",
  "message": "One or more teachers are invalid.",
  "details": {
    "teachers": [
      {
        "teacher_id": "...",
        "reason": "not_found"
      }
    ]
  }
}
```

Possible reasons:

```text
invalid_id
not_found
inactive
role_not_allowed
duplicate
```

### Multiple primary teachers

```json
{
  "code": "multiple_primary_teachers",
  "message": "A course can have at most one primary teacher."
}
```

### Teacher owns future sessions

```json
{
  "code": "teacher_in_use",
  "message": "The teacher still owns future sessions for this course.",
  "details": {
    "teacher_id": "...",
    "future_session_count": 8,
    "earliest_session_start_at": "2026-08-05T09:00:00Z",
    "session_ids": ["...", "..."],
    "series_ids": ["..."]
  }
}
```

### Stale course edit

```json
{
  "code": "stale_edit",
  "message": "The course was changed by another user.",
  "details": {
    "current": {
      "id": "...",
      "version": 14,
      "teachers": []
    }
  }
}
```

---

# 6. Backend Package Structure

Create a course domain service instead of keeping business rules in HTTP handlers.

```text
backend/internal/courseadmin/
    service.go
    commands.go
    validation.go
    errors.go
    response.go
```

HTTP package:

```text
backend/internal/httpapi/courseshttp/
    routes.go
    requests.go
    error_mapping.go
```

Database queries:

```text
backend/db/queries/course_teachers.sql
backend/db/queries/courses.sql
```

Responsibilities:

```text
courseshttp:
    authentication
    request decoding
    UUID parsing
    HTTP error mapping
    HTTP response writing

courseadmin:
    business validation
    concurrency validation
    teacher eligibility
    future-session protection
    transaction orchestration
    auditing

sqlc queries:
    database reads
    database writes
    row locking
```

No teacher mutation SQL should remain inside `courseshttp/routes.go`.

---

# 7. Domain Types

Suggested file:

```text
backend/internal/courseadmin/commands.go
```

```go
package courseadmin

import "github.com/jackc/pgx/v5/pgtype"

const MaxTeachersPerCourse = 20

type TeacherAssignment struct {
	TeacherID pgtype.UUID
	IsPrimary bool
}

type UpdateCourseCommand struct {
	CourseID        pgtype.UUID
	ActorID         pgtype.UUID
	ExpectedVersion int32

	Code           string
	Name           string
	LegacyCourseID *string
	Teachers       []TeacherAssignment
}

type UpdateCourseResult struct {
	CourseID pgtype.UUID
	Version  int32
}
```

The maximum teacher count should be a business-configurable constant if the real limit is different.

The purpose of a limit is to protect:

* Request size.
* Query size.
* Accidental mass assignment.
* UI usability.

It is not meant to prevent legitimate multi-teacher courses.

---

# 8. Teacher-Set Validation

Suggested file:

```text
backend/internal/courseadmin/validation.go
```

```go
package courseadmin

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

func validateTeacherAssignments(
	assignments []TeacherAssignment,
) error {
	if len(assignments) > MaxTeachersPerCourse {
		return &Error{
			Code:    "too_many_teachers",
			Message: fmt.Sprintf("A course can have at most %d teachers.", MaxTeachersPerCourse),
			Details: map[string]any{
				"maximum": MaxTeachersPerCourse,
				"received": len(assignments),
			},
		}
	}

	seen := make(map[[16]byte]struct{}, len(assignments))
	primaryCount := 0

	for index, assignment := range assignments {
		if !assignment.TeacherID.Valid {
			return &Error{
				Code:    "invalid_teacher",
				Message: "One or more teachers are invalid.",
				Details: map[string]any{
					"index":  index,
					"reason": "invalid_id",
				},
			}
		}

		key := assignment.TeacherID.Bytes
		if _, exists := seen[key]; exists {
			return &Error{
				Code:    "duplicate_teacher",
				Message: "The same teacher cannot be assigned more than once.",
				Details: map[string]any{
					"index": index,
				},
			}
		}
		seen[key] = struct{}{}

		if assignment.IsPrimary {
			primaryCount++
		}
	}

	if primaryCount > 1 {
		return &Error{
			Code:    "multiple_primary_teachers",
			Message: "A course can have at most one primary teacher.",
		}
	}

	return nil
}

func primaryTeacherID(
	assignments []TeacherAssignment,
) pgtype.UUID {
	for _, assignment := range assignments {
		if assignment.IsPrimary {
			return assignment.TeacherID
		}
	}

	return pgtype.UUID{Valid: false}
}
```

This validation happens before database writes.

Database constraints remain the final safety layer.

---

# 9. SQLC Queries

## 9.1 Lock course

```sql
-- name: CourseLockForTeacherUpdate :one
SELECT
    id,
    version
FROM courses
WHERE id = $1
FOR UPDATE;
```

## 9.2 Fetch all submitted users in one query

```sql
-- name: UsersListForTeacherValidation :many
SELECT
    id,
    username,
    role,
    active
FROM users
WHERE id = ANY($1::uuid[]);
```

Use the project’s actual active-user and role columns if their names differ.

Do not perform one user query per teacher.

## 9.3 List existing assignments

```sql
-- name: CourseTeachersList :many
SELECT
    ct.course_id,
    ct.teacher_id,
    ct.is_primary,
    u.username
FROM course_teachers ct
JOIN users u ON u.id = ct.teacher_id
WHERE ct.course_id = $1
ORDER BY
    ct.is_primary DESC,
    u.username,
    ct.teacher_id;
```

## 9.4 Find future sessions owned by removed teachers

The active-session predicate must match the predicate currently used by the scheduling domain.

```sql
-- name: CourseFutureSessionUsageByTeachers :many
SELECT
    s.teacher_id,
    count(*) AS session_count,
    min(s.start_at) AS earliest_start_at,
    array_agg(s.id ORDER BY s.start_at, s.id)[:10] AS sample_session_ids,
    array_remove(array_agg(DISTINCT s.series_id), NULL) AS series_ids
FROM sessions s
WHERE s.course_id = $1
  AND s.teacher_id = ANY($2::uuid[])
  AND s.start_at > $3
  AND s.deleted_at IS NULL
GROUP BY s.teacher_id;
```

If the current schema uses a different active-session field, reuse that exact condition.

## 9.5 Replace assignments

```sql
-- name: CourseTeachersDeleteAll :exec
DELETE FROM course_teachers
WHERE course_id = $1;
```

```sql
-- name: CourseTeacherInsert :exec
INSERT INTO course_teachers (
    course_id,
    teacher_id,
    is_primary
)
VALUES ($1, $2, $3);
```

Deleting and reinserting is acceptable because:

* Teacher sets are small.
* The entire operation is inside one transaction.
* Every error is returned.
* Any failure rolls back the delete.

It is not acceptable to log an insert error and continue.

## 9.6 Update compatibility primary and version

```sql
-- name: CourseSetPrimaryAndIncrementVersion :one
UPDATE courses
SET
    teacher_id = $2,
    version = version + 1,
    updated_at = now()
WHERE id = $1
RETURNING version;
```

For full metadata updates:

```sql
-- name: CourseUpdateAggregate :one
UPDATE courses
SET
    code = $2,
    name = $3,
    legacy_course_id = $4,
    teacher_id = $5,
    version = version + 1,
    updated_at = now()
WHERE id = $1
RETURNING id, version;
```

The row is already locked, but version must still be checked before executing this update.

---

# 10. Production-Grade Service Logic

Suggested file:

```text
backend/internal/courseadmin/service.go
```

```go
package courseadmin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"your-module/backend/internal/sqldb"
)

type Service struct {
	Now func() time.Time
}

func NewService() *Service {
	return &Service{
		Now: time.Now,
	}
}

func (s *Service) UpdateCourseTx(
	ctx context.Context,
	tx pgx.Tx,
	qtx *sqldb.Queries,
	command UpdateCourseCommand,
) (UpdateCourseResult, error) {
	if err := validateTeacherAssignments(command.Teachers); err != nil {
		return UpdateCourseResult{}, err
	}

	if command.ExpectedVersion <= 0 {
		return UpdateCourseResult{}, &Error{
			Code:    "invalid_expected_version",
			Message: "expected_version must be greater than zero.",
		}
	}

	lockedCourse, err := qtx.CourseLockForTeacherUpdate(
		ctx,
		command.CourseID,
	)
	if err != nil {
		return UpdateCourseResult{}, classifyCourseReadError(err)
	}

	if lockedCourse.Version != command.ExpectedVersion {
		current, readErr := loadCourseResponse(
			ctx,
			qtx,
			command.CourseID,
		)
		if readErr != nil {
			return UpdateCourseResult{}, fmt.Errorf(
				"load current course after stale edit: %w",
				readErr,
			)
		}

		return UpdateCourseResult{}, &Error{
			Code:    "stale_edit",
			Message: "The course was changed by another user.",
			Details: map[string]any{
				"current": current,
			},
		}
	}

	if err := validateTeachersExistAndCanTeach(
		ctx,
		qtx,
		command.Teachers,
	); err != nil {
		return UpdateCourseResult{}, err
	}

	existing, err := qtx.CourseTeachersList(
		ctx,
		command.CourseID,
	)
	if err != nil {
		return UpdateCourseResult{}, fmt.Errorf(
			"list existing course teachers: %w",
			err,
		)
	}

	removedTeacherIDs := calculateRemovedTeacherIDs(
		existing,
		command.Teachers,
	)

	if len(removedTeacherIDs) > 0 {
		usage, usageErr := qtx.CourseFutureSessionUsageByTeachers(
			ctx,
			sqldb.CourseFutureSessionUsageByTeachersParams{
				CourseID:  command.CourseID,
				TeacherIds: removedTeacherIDs,
				StartAt:   pgtype.Timestamptz{
					Time:  s.Now().UTC(),
					Valid: true,
				},
			},
		)
		if usageErr != nil {
			return UpdateCourseResult{}, fmt.Errorf(
				"check removed teacher usage: %w",
				usageErr,
			)
		}

		if len(usage) > 0 {
			return UpdateCourseResult{}, teacherInUseError(usage)
		}
	}

	if err := qtx.CourseTeachersDeleteAll(
		ctx,
		command.CourseID,
	); err != nil {
		return UpdateCourseResult{}, fmt.Errorf(
			"delete existing course teachers: %w",
			err,
		)
	}

	for _, assignment := range command.Teachers {
		err := qtx.CourseTeacherInsert(
			ctx,
			sqldb.CourseTeacherInsertParams{
				CourseID:  command.CourseID,
				TeacherID: assignment.TeacherID,
				IsPrimary: assignment.IsPrimary,
			},
		)
		if err != nil {
			return UpdateCourseResult{}, fmt.Errorf(
				"insert course teacher %x: %w",
				assignment.TeacherID.Bytes,
				err,
			)
		}
	}

	primaryID := primaryTeacherID(command.Teachers)

	updated, err := qtx.CourseUpdateAggregate(
		ctx,
		sqldb.CourseUpdateAggregateParams{
			ID:             command.CourseID,
			Code:           command.Code,
			Name:           command.Name,
			LegacyCourseID: nullableText(command.LegacyCourseID),
			TeacherID:      primaryID,
		},
	)
	if err != nil {
		return UpdateCourseResult{}, fmt.Errorf(
			"update course aggregate: %w",
			err,
		)
	}

	if err := insertCourseAudit(
		ctx,
		qtx,
		command,
		existing,
		updated.Version,
	); err != nil {
		return UpdateCourseResult{}, fmt.Errorf(
			"insert course audit: %w",
			err,
		)
	}

	return UpdateCourseResult{
		CourseID: command.CourseID,
		Version:  updated.Version,
	}, nil
}
```

Important behavior:

* Every failure returns immediately.
* No write error is ignored.
* The transaction owner rolls back on any returned error.
* Existing teachers are loaded before replacement.
* Removed teachers are checked against future sessions.
* Primary compatibility field and teacher set change together.
* Audit insertion is part of the same transaction.

If audit insertion is intentionally non-critical in the current architecture, document that decision explicitly. For production-grade administrative mutations, audit should normally be transactional.

---

# 11. Teacher Eligibility Validation

```go
func validateTeachersExistAndCanTeach(
	ctx context.Context,
	qtx *sqldb.Queries,
	assignments []TeacherAssignment,
) error {
	if len(assignments) == 0 {
		return nil
	}

	ids := make([]pgtype.UUID, 0, len(assignments))
	for _, assignment := range assignments {
		ids = append(ids, assignment.TeacherID)
	}

	rows, err := qtx.UsersListForTeacherValidation(ctx, ids)
	if err != nil {
		return fmt.Errorf("load teachers for validation: %w", err)
	}

	found := make(map[[16]byte]sqldb.UsersListForTeacherValidationRow, len(rows))
	for _, row := range rows {
		found[row.ID.Bytes] = row
	}

	invalid := make([]map[string]any, 0)

	for _, assignment := range assignments {
		row, exists := found[assignment.TeacherID.Bytes]
		if !exists {
			invalid = append(invalid, map[string]any{
				"teacher_id": uuidString(assignment.TeacherID),
				"reason":     "not_found",
			})
			continue
		}

		if !row.Active {
			invalid = append(invalid, map[string]any{
				"teacher_id": uuidString(assignment.TeacherID),
				"reason":     "inactive",
			})
			continue
		}

		if !teacherRoleCanTeach(row.Role) {
			invalid = append(invalid, map[string]any{
				"teacher_id": uuidString(assignment.TeacherID),
				"reason":     "role_not_allowed",
			})
		}
	}

	if len(invalid) > 0 {
		return &Error{
			Code:    "invalid_teacher",
			Message: "One or more teachers are invalid.",
			Details: map[string]any{
				"teachers": invalid,
			},
		}
	}

	return nil
}
```

Do not duplicate role rules in several services.

Create one shared function or policy:

```text
backend/internal/teacherpolicy/
```

```go
func CanTeach(role string) bool
```

Use the same policy in:

* Course teacher assignment.
* Session creation.
* Series creation.
* Teacher lookups shown in the frontend.

---

# 12. HTTP Request Parsing

Suggested request type:

```go
type teacherAssignmentRequest struct {
	TeacherID string `json:"teacher_id"`
	IsPrimary bool   `json:"is_primary"`
}

type updateCourseRequest struct {
	ExpectedVersion int32 `json:"expected_version"`

	Code           string  `json:"code"`
	Name           string  `json:"name"`
	LegacyCourseID *string `json:"legacy_course_id"`

	Teachers []teacherAssignmentRequest `json:"teachers"`
}
```

Strict parsing:

```go
func parseTeacherAssignments(
	a *httpapi.App,
	input []teacherAssignmentRequest,
) ([]courseadmin.TeacherAssignment, error) {
	assignments := make(
		[]courseadmin.TeacherAssignment,
		0,
		len(input),
	)

	for index, item := range input {
		teacherID, err := a.ParseUUID(item.TeacherID)
		if err != nil {
			return nil, &courseadmin.Error{
				Code:    "invalid_teacher",
				Message: "One or more teachers are invalid.",
				Details: map[string]any{
					"index":      index,
					"teacher_id": item.TeacherID,
					"reason":     "invalid_id",
				},
			}
		}

		assignments = append(
			assignments,
			courseadmin.TeacherAssignment{
				TeacherID: teacherID,
				IsPrimary: item.IsPrimary,
			},
		)
	}

	return assignments, nil
}
```

Invalid UUIDs are rejected.

They are never:

* Converted to `NULL`.
* Ignored.
* Removed from the request.
* Logged and continued.

---

# 13. HTTP Handler

The handler should remain thin.

```go
func (s *server) handleCoursesPatch(
	w http.ResponseWriter,
	r *http.Request,
) {
	actor, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}

	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(
			w,
			http.StatusBadRequest,
			"bad_id",
			"Invalid course id.",
		)
		return
	}

	var body updateCourseRequest
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(
			w,
			http.StatusBadRequest,
			"bad_json",
			"Invalid JSON.",
		)
		return
	}

	assignments, err := parseTeacherAssignments(
		s.a,
		body.Teachers,
	)
	if err != nil {
		s.writeCourseAdminError(w, err)
		return
	}

	command := courseadmin.UpdateCourseCommand{
		CourseID:        courseID,
		ActorID:         pgtype.UUID{Bytes: actor.ID, Valid: true},
		ExpectedVersion: body.ExpectedVersion,
		Code:            strings.TrimSpace(body.Code),
		Name:            strings.TrimSpace(body.Name),
		LegacyCourseID:  normalizeOptionalString(body.LegacyCourseID),
		Teachers:        assignments,
	}

	var response any
	var publishedCourseID string

	ok = s.a.WithIdempotentTx(
		w,
		r,
		actor.ID,
		"courses",
		s.deps.DB,
		s.deps.Q,
		func(tx pgx.Tx) (int, any, error) {
			qtx := s.deps.Q.WithTx(tx)

			result, updateErr := s.deps.CourseAdmin.UpdateCourseTx(
				r.Context(),
				tx,
				qtx,
				command,
			)
			if updateErr != nil {
				s.writeCourseAdminError(w, updateErr)
				return 0, nil, updateErr
			}

			current, readErr := loadCourseResponse(
				r.Context(),
				qtx,
				result.CourseID,
			)
			if readErr != nil {
				return 0, nil, fmt.Errorf(
					"load updated course: %w",
					readErr,
				)
			}

			publishedCourseID = current.ID
			response = current

			return http.StatusOK, current, nil
		},
	)

	if ok {
		s.publishCourseUpdated(publishedCourseID)
	}

	_ = response
}
```

The exact shape may need adjustment for the existing `WithIdempotentTx` error-writing contract.

The important rule is:

```text
service error → transaction rollback → mapped HTTP response
```

Never:

```text
service error → log → continue → success response
```

---

# 14. Course Creation

Course creation must use the same teacher validation and insertion helper.

Correct order:

```text
validate request
→ validate teacher-set structure
→ begin idempotent transaction
→ validate every teacher
→ insert course
→ insert all teacher assignments
→ set compatibility primary teacher
→ insert audit
→ commit
→ publish course update
```

Any teacher failure must roll back the newly created course.

Create and update should share:

```go
replaceCourseTeachersTx(...)
validateTeachersExistAndCanTeach(...)
primaryTeacherID(...)
```

Do not maintain two independent implementations.

---

# 15. Scheduling Integration

Course teacher integrity is incomplete unless scheduling uses it.

## 15.1 One-off session creation

Inside `CreateSessionTx`, after locking the course and before inserting the session:

```go
assigned, err := qtx.CourseTeacherExists(
	ctx,
	sqldb.CourseTeacherExistsParams{
		CourseID:  command.CourseID,
		TeacherID: command.TeacherID,
	},
)
if err != nil {
	return classifyTeacherMembershipReadError(err)
}

if !assigned {
	return &scheduling.Err{
		Code:    "teacher_not_assigned_to_course",
		Message: "The selected teacher is not assigned to this course.",
		Details: map[string]any{
			"course_id":  uuidString(command.CourseID),
			"teacher_id": uuidString(command.TeacherID),
		},
	}
}
```

SQL:

```sql
-- name: CourseTeacherExists :one
SELECT EXISTS (
    SELECT 1
    FROM course_teachers
    WHERE course_id = $1
      AND teacher_id = $2
);
```

## 15.2 Recurring series creation

Apply the same check after the course lock and before materialization writes.

## 15.3 Session edit

When changing:

* Teacher.
* Course.
* Both teacher and course.

Validate the new teacher against the new course teacher set.

The old historical teacher does not need to remain assigned to the course.

## 15.4 Preflight

Advisory preflight should also return:

```text
teacher_not_assigned_to_course
```

However, the transactional check remains authoritative.

## 15.5 Concurrency behavior

Both teacher-set updates and scheduling writes must lock the course first.

This creates deterministic outcomes.

### Scenario A: session creation obtains the course lock first

1. Session creation confirms teacher is assigned.
2. Session is created.
3. Teacher-removal transaction obtains the lock.
4. Removal detects the new future session.
5. Removal fails with `teacher_in_use`.

### Scenario B: teacher removal obtains the course lock first

1. Teacher is removed.
2. Transaction commits.
3. Session creation obtains the course lock.
4. Membership validation fails.
5. Session creation returns `teacher_not_assigned_to_course`.

No invalid state can commit.

---

# 16. Frontend Changes

The current course page already maintains an array of teacher IDs and uses `MultiTeacherSelect`.

## 16.1 Course type

```ts
export type CourseTeacher = {
  id: string;
  username: string;
  is_primary: boolean;
};

export type Course = {
  id: string;
  version: number;
  code: string;
  name: string;
  primary_teacher_id: string | null;
  teachers: CourseTeacher[];
};
```

## 16.2 Edit state

```ts
type EditableTeacher = {
  teacher_id: string;
  is_primary: boolean;
};

const [editTeachers, setEditTeachers] =
  useState<EditableTeacher[]>([]);
```

## 16.3 User interaction

The teacher editor should show:

* Multi-select for assigned teachers.
* One radio button or “Make primary” action per selected teacher.
* “No primary teacher” option.
* Clear distinction between:

  * Assigned teacher.
  * Primary/default teacher.

Example:

```text
Teachers

● Teacher A — Primary
○ Teacher B
○ Teacher C

[No primary teacher]
```

Do not use a checkbox for `is_primary`; checkboxes imply several primary teachers may be selected.

## 16.4 Save payload

```ts
await updateCourse(course.id, {
  expected_version: course.version,
  code: editCode.trim(),
  name: editName.trim(),
  legacy_course_id: course.legacy_course_id,
  teachers: editTeachers,
});
```

## 16.5 Stale edit handling

```ts
try {
  const updated = await updateCourse(course.id, payload);
  setCourse(updated);
  setIsEditing(false);
  addToast("success", "Course updated");
} catch (error) {
  if (
    error instanceof ApiRequestError &&
    error.code === "stale_edit"
  ) {
    const current = error.details?.current as Course | undefined;

    if (current) {
      setCourse(current);
    } else {
      await reloadCourse();
    }

    addToast(
      "error",
      "Another user changed this course. The latest version has been loaded."
    );
    return;
  }

  throw error;
}
```

Do not automatically retry a stale update because that may overwrite another administrator’s work.

## 16.6 Removing a teacher in use

When the backend returns `teacher_in_use`, show:

```text
Teacher B cannot be removed.

They are assigned to 8 future sessions.
Earliest affected session: 5 Aug 2026, 16:00.

Review or reassign those sessions before removing this teacher.
```

The current implementation should block removal.

Automatic reassignment should be a separate future operation.

---

# 17. Backward-Compatible Rollout

## Phase 1: Additive database migration

Deploy:

* `courses.version`
* `course_teachers.is_primary`
* Primary backfill
* Unique partial index

Do not remove any existing column or route.

Rollback:

* Revert application.
* Leave additive columns in place.
* No data loss occurs.

## Phase 2: Data audit

Run scheduled verification:

```sql
-- Legacy primary missing from teacher set
SELECT count(*)
FROM courses c
LEFT JOIN course_teachers ct
  ON ct.course_id = c.id
 AND ct.teacher_id = c.teacher_id
WHERE c.teacher_id IS NOT NULL
  AND ct.teacher_id IS NULL;
```

```sql
-- Multiple primary teachers
SELECT course_id
FROM course_teachers
WHERE is_primary
GROUP BY course_id
HAVING count(*) > 1;
```

```sql
-- Future session teacher not assigned to course
SELECT
    s.id,
    s.course_id,
    s.teacher_id
FROM sessions s
LEFT JOIN course_teachers ct
  ON ct.course_id = s.course_id
 AND ct.teacher_id = s.teacher_id
WHERE s.start_at > now()
  AND s.deleted_at IS NULL
  AND ct.teacher_id IS NULL;
```

Do not enable strict scheduling enforcement until the third query returns zero or every exception has an approved repair.

## Phase 3: Deploy new backend read model

Course responses include:

* `version`
* `teachers[].is_primary`
* `primary_teacher_id`

Old response fields remain temporarily.

## Phase 4: Deploy atomic teacher writes

All create and update routes call `courseadmin.Service`.

Remove raw `course_teachers` mutations from HTTP handlers.

During transition, the old `teacher_ids` request may be adapted:

```text
first teacher = primary
remaining teachers = non-primary
```

Record a metric each time the legacy request shape is used.

## Phase 5: Deploy frontend

Frontend sends:

* `expected_version`
* Explicit teacher assignments
* Explicit primary teacher

After frontend deployment is stable, stop accepting ambiguous teacher updates.

## Phase 6: Enable scheduling membership validation

Start with observation mode:

```text
log/metric teacher_not_assigned_to_course
```

Do not block for a brief verification period.

After zero unexpected violations:

```text
enforce teacher membership
```

## Phase 7: Remove compatibility code

Remove:

* Legacy `teacher_ids` request adaptation.
* Direct route SQL.
* Course-read raw teacher query.
* Any independent writes to `courses.teacher_id`.

Do not remove the database column yet unless a separate migration has been reviewed.

---

# 18. Test Plan

## 18.1 Unit tests

### Teacher-set validation

* Empty teacher set.
* One teacher.
* Several teachers.
* One primary among several.
* No primary among several.
* Duplicate teacher.
* Two primary teachers.
* Invalid UUID.
* More than maximum teachers.

### Removed-teacher calculation

* No changes.
* Add teacher.
* Remove one teacher.
* Replace all teachers.
* Change only primary.
* Reorder teachers without membership changes.

### Error mapping

* `invalid_teacher` → 400.
* `teacher_in_use` → 409.
* `stale_edit` → 409.
* Missing course → 404.
* Unexpected database error → 500.

## 18.2 Database integration tests

### Atomicity

Inject a failure after deleting old assignments but before inserting every new assignment.

Expected:

* Transaction rolls back.
* Original teacher set remains.
* Course version does not change.
* Primary compatibility field does not change.
* No success audit is written.

### Multiple teachers

Create a course with three teachers.

Expected:

* All three rows exist.
* Exactly one primary when specified.
* Course response returns all three.
* `courses.teacher_id` mirrors the primary.

### No primary

Create a course with two teachers and no primary.

Expected:

* Both assignments exist.
* No assignment has `is_primary=true`.
* `courses.teacher_id` is `NULL`.

### Invalid user

Submit one valid teacher and one missing teacher.

Expected:

* Entire operation fails.
* No teacher changes commit.
* Version remains unchanged.

### Wrong role

Submit an active user who is not permitted to teach.

Expected:

* Entire operation fails.
* Stable `invalid_teacher` details returned.

### Inactive teacher

Expected:

* Entire operation fails.

### Future-session protection

Remove a teacher with future sessions.

Expected:

* `teacher_in_use`.
* Teacher remains assigned.
* Version remains unchanged.

### Historical session

Remove a teacher who owns only past sessions.

Expected:

* Removal succeeds.
* Past sessions retain the original teacher ID.

## 18.3 Concurrency tests

### Two course updates

Run two updates using the same `expected_version`.

Expected:

* Exactly one succeeds.
* Exactly one returns `stale_edit`.
* Final teacher set matches one complete request.
* No mixed teacher set exists.

### Teacher removal versus session creation

Run concurrently.

Expected valid outcomes:

```text
session succeeds + removal is blocked
```

or:

```text
removal succeeds + session fails membership validation
```

Invalid outcome:

```text
session succeeds using a teacher no longer assigned to the course
```

### Primary change versus course update

Expected:

* One complete update wins.
* Other receives stale response.
* At most one primary exists.

### Idempotency replay

Repeat the exact update with the same idempotency key.

Expected:

* Same response.
* Version increments once.
* Audit written once.

### Same key, different payload

Expected:

* Idempotency mismatch error.
* No second mutation.

## 18.4 Scheduling tests

* Create session using first assigned teacher.
* Create session using second assigned teacher.
* Reject unassigned teacher.
* Edit session from one assigned teacher to another.
* Reject edit to an unassigned teacher.
* Create recurring series using any assigned teacher.
* Reject series using an unassigned teacher.
* Course with no teachers cannot create a session until a teacher is explicitly selected and assigned.

## 18.5 Frontend tests

* Display multiple teachers.
* Primary badge is correct.
* Select several teachers.
* Change primary teacher.
* Remove primary while retaining other teachers.
* Choose no primary.
* Stale-edit reload behavior.
* Teacher-in-use message.
* API validation errors preserve the edit form.
* Saving state prevents duplicate submissions.

---

# 19. Observability

Add metrics:

```text
course_teacher_update_total
course_teacher_update_failed_total{reason}
course_teacher_stale_edit_total
course_teacher_in_use_block_total
course_teacher_invalid_total{reason}
session_teacher_membership_rejected_total
course_teacher_legacy_payload_total
course_teacher_invariant_violation_total{type}
```

Structured log fields:

```text
request_id
actor_id
course_id
expected_version
result_version
teacher_count_before
teacher_count_after
primary_teacher_before
primary_teacher_after
removed_teacher_count
error_code
duration_ms
```

Do not log:

* Student data.
* Authentication credentials.
* Full request payloads.
* Sensitive personal information.

---

# 20. Files to Change

Backend:

```text
backend/db/migrations/<timestamp>_course_teacher_integrity.sql
backend/db/queries/courses.sql
backend/db/queries/course_teachers.sql
backend/internal/courseadmin/commands.go
backend/internal/courseadmin/errors.go
backend/internal/courseadmin/validation.go
backend/internal/courseadmin/service.go
backend/internal/httpapi/courseshttp/routes.go
backend/internal/httpapi/courseshttp/requests.go
backend/internal/httpapi/courseshttp/error_mapping.go
backend/internal/scheduling/service.go
backend/internal/series/service.go
```

Frontend:

```text
src/features/courses/api/courseApi.ts
src/features/courses/types.ts
src/features/courses/components/CourseTeacherEditor.tsx
src/pages/CourseDetail.tsx
src/types.ts
```

Tests:

```text
backend/internal/courseadmin/validation_test.go
backend/internal/courseadmin/service_integration_test.go
backend/internal/scheduling/course_teacher_integration_test.go
backend/internal/scheduling/concurrency_integration_test.go
src/features/courses/components/CourseTeacherEditor.test.tsx
src/pages/CourseDetail.test.tsx
```

Files intentionally not changed:

```text
course deletion service/query behavior
session deletion service/query behavior
delete API contracts
delete confirmation behavior
```

---

# 21. Pull Request Breakdown

## PR 1 — Schema and verification

* Add version.
* Add `is_primary`.
* Backfill.
* Add verification SQL.
* Add unique primary index.
* No behavior change.

## PR 2 — Course teacher domain service

* Add validation.
* Add SQLC queries.
* Add atomic create/update.
* Add audit.
* Add optimistic concurrency.
* Remove ignored errors.

## PR 3 — API contract

* Add explicit teacher assignments.
* Add stable errors.
* Return course version.
* Maintain legacy request adapter temporarily.

## PR 4 — Frontend teacher editor

* Support multiple teachers.
* Add optional primary selection.
* Send expected version.
* Handle stale edit and teacher-in-use.

## PR 5 — Scheduling enforcement

* Validate teacher membership during one-off creation.
* Validate during series creation.
* Validate during occurrence edits.
* Add concurrency tests.

## PR 6 — Compatibility cleanup

* Remove old teacher mutation SQL.
* Remove legacy payload support.
* Remove duplicate teacher reads.
* Keep deletion unchanged.

Each PR should be deployable and reversible independently.

---

# 22. Definition of Done

The teacher-management implementation is complete when:

1. Courses support zero, one, or many teachers.
2. Zero or one teacher can be primary.
3. Teacher membership is stored canonically in `course_teachers`.
4. `courses.teacher_id` is only a compatibility projection.
5. Invalid teachers reject the entire operation.
6. No teacher write error is ignored.
7. Course and teacher changes commit or roll back together.
8. Course updates require `expected_version`.
9. Concurrent updates cannot silently overwrite each other.
10. A teacher with future sessions cannot be removed.
11. New sessions and series require a teacher assigned to the course.
12. Historical sessions preserve their original teacher.
13. Create and update use the same validation logic.
14. HTTP handlers contain no course-teacher mutation SQL.
15. Integration and concurrency tests pass.
16. Invariant monitoring reports zero unexpected violations.
17. Course and session deletion behavior remains unchanged.
