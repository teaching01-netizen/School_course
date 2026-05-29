# Draft Students: Course Roster Status + Override Semantics

Date: 2026-05-21
Status: Design

## Context

Scheduling staff often work with two kinds of students who are not yet confirmed enrollees:

1. **Known student, not enrolled in this course** — a student who exists in `students` but has no row in `course_students` for a given course.
2. **Brand-new prospect** — someone who does not exist in `students` at all (e.g., a CRM lead, walk-in inquiry).

In both cases, staff need to:

- Evaluate **preflight** (slot availability, conflict checks) as if this prospect were enrolled.
- Optionally **add them to the course roster** so their busy ranges are visible to other preflight checks and the calendar — but without marking them as a paid, confirmed enrollee.
- Visually **distinguish** these tentative students from confirmed ones in the roster, calendar, and conflict details.

Today, `course_students` has no status field — all enrolled students are treated equally. Busy ranges are generated for all course students. There is no concept of "draft."

This spec defines the data model, overlap-conflict rules, override semantics, conversion flow, and visual indicators for **draft students**.

## Decisions / Constraints

- `course_students` gets a `status` column — not `students` — so a student can be enrolled in Course A and draft in Course B.
- Drafts generate `student_busy_ranges` (same as enrolled). They block overlaps in preflight.
- Override escape hatch: enrolled students can override draft busy ranges with a 2-step confirmation. The draft is then **kicked from the roster**.
- Draft vs draft is a hard block (preflight fails).
- Enrolled vs enrolled remains a hard block (as today).
- Brand-new prospects must be created as a minimal `students` record first (W-code + name) before they can be added as draft — this gives them a student ID for busy-range tracking.

## Data Model

### `course_students` table

Add a `status` column:

```sql
ALTER TABLE course_students
  ADD COLUMN status text NOT NULL DEFAULT 'enrolled'
  CHECK (status IN ('draft', 'enrolled'));

COMMENT ON COLUMN course_students.status IS 'enrolled: confirmed, paid student (default). draft: tentative prospect, blocks busy ranges.';
```

All existing rows get `status = 'enrolled'` via the default. No backfill migration needed.

**Migration file:** `backend/db/migrations/00013_course_students_draft_status.sql`

### Busy Ranges

Both `draft` and `enrolled` students participate in `student_busy_ranges`:

- The existing triggers on `course_students` (`INSERT`/`DELETE` → refresh busy ranges) already handle both statuses transparently. No trigger changes needed.
- The `student_busy_ranges` exclusion constraint already applies regardless of status — drafts block overlaps.

## Overlap Conflict Rules

| Adding / Checking | vs Draft | vs Enrolled |
|---|---|---|
| **Draft** | Hard block (fail) | Hard block (fail) |
| **Enrolled** | 2-step override → kicks draft(s) from roster | Hard block (fail) |

### Override workflow (Enrolled vs Draft)

When staff adds (or preflight-checks) an enrolled student whose busy ranges overlap with one or more draft students in the same course:

1. Preflight returns a blocking conflict. The conflict details include `kind: "student_overlap"` with each conflicting session enriched with which **student** caused the conflict and their `status: "draft"`.
2. UI shows the conflict details plus: "This will remove N draft students from the course roster. Continue?" listing the affected students.
3. Staff confirms with a 2-step action (e.g., click "Continue" → type confirmation or click a second button).
4. Backend, in a transaction:
   a. Delete **all** conflicting draft students from `course_students` (with `AND status = 'draft'` check to ensure we only auto-kick drafts, never enrolled).
   b. Add the enrolled student to `course_students` with `status = 'enrolled'`.
   c. Busy ranges are refreshed by existing triggers.
5. Response includes both the success and a summary of the action taken: `{"status": "added", "removed_draft_ids": [...]}`.

**Multiple drafts:** If the enrolled student's busy ranges overlap with multiple draft students, all are kicked from the roster in the same transaction. The UI lists all affected drafts before confirmation.

**Edge case — student already has a row:** If the student being added as "enrolled" already has a `course_students` row with `status = 'draft'`, the flow is just an UPDATE to `status = 'enrolled'` (no DELETE + INSERT needed). The "kick draft" step is skipped. This is the "Convert Draft → Enrolled" scenario.

## sqlc Query Changes

### New: `CourseStudentAddDraft`
File: `backend/db/queries/course_students.sql`

```sql
-- name: CourseStudentAddDraft :exec
INSERT INTO course_students (course_id, student_id, status)
VALUES ($1, $2, 'draft')
ON CONFLICT (course_id, student_id) DO NOTHING;
```

Uses `ON CONFLICT DO NOTHING` to avoid accidentally downgrading an `enrolled` student to `draft`. If the student is already on the roster (any status), the insert silently succeeds as a no-op. The explicit "Convert to Enrolled" flow is the only way to change status.

### New: `CourseStudentUpdateStatus`
File: `backend/db/queries/course_students.sql`

```sql
-- name: CourseStudentUpdateStatus :exec
UPDATE course_students
SET status = $3
WHERE course_id = $1 AND student_id = $2;
```

Used by "Convert Draft → Enrolled" flow.

### Modified: `CourseStudentsListDetailed`
File: `backend/db/queries/course_students.sql`

The existing query returns `[]Student` (no status). Change to a new query that includes `cs.status`:

```sql
-- name: CourseStudentsListDetailedWithStatus :many
SELECT s.id, s.wcode, s.full_name, s.notes, s.created_at, s.updated_at,
       cs.status
FROM course_students cs
JOIN students s ON s.id = cs.student_id
WHERE cs.course_id = $1
ORDER BY s.wcode ASC;
```

This returns students with their roster status. The old `CourseStudentsListDetailed` can be kept for backwards compatibility or dropped after all callers are updated.

### Modified: `CourseStudentRemove` (status-aware variant)
File: `backend/db/queries/course_students.sql`

For the override flow (kicking drafts), we need to ensure we only delete drafts:

```sql
-- name: CourseStudentRemoveDraftIfStatus :execrows
DELETE FROM course_students
WHERE course_id = $1 AND student_id = $2 AND status = 'draft';
```

Returns `execrows` so the caller can verify a row was actually deleted. The general `CourseStudentRemove` stays unchanged (no status filter) for normal use.

## Overlap Conflict Details Enrichment

### Current `ConflictSession` struct (in `backend/internal/scheduling/errors.go`)

```go
type ConflictSession struct {
    SessionID string  `json:"session_id"`
    SeriesID  *string `json:"series_id,omitempty"`
    CourseID  string  `json:"course_id"`
    RoomID    *string `json:"room_id"`
    TeacherID string  `json:"teacher_id"`
    StartAt   string  `json:"start_at"`
    EndAt     string  `json:"end_at"`
}
```

**No student info.** When preflight detects a `student_overlap`, it returns conflicting sessions but doesn't indicate which student caused the conflict. We need to enrich this.

### Change: Add `ConflictingStudent` to `ConflictDetails`

Add a new struct and field to `ConflictDetails`:

```go
type ConflictingStudent struct {
    StudentID string `json:"student_id"`
    FullName  string `json:"full_name"`
    Status    string `json:"status"` // "draft" | "enrolled"
}

type ConflictDetails struct {
    Kind               ConflictKind         `json:"kind"`
    Conflicts          []ConflictSession    `json:"conflicts"`
    ConflictingStudents []ConflictingStudent `json:"conflicting_students,omitempty"` // NEW
    Requested          ConflictRequested    `json:"requested"`
}
```

### How to populate `ConflictingStudents`

The `overlappingSessionsByStudents` and `overlappingSessionsByStudentsInCourse` queries find conflicts via `student_busy_ranges`. We need to return which students caused the overlap:

1. Modify the SQL to also select `br.student_id`.
2. After retrieving conflicting sessions, do a second query to fetch the student details + status:
   ```sql
   SELECT DISTINCT br.student_id, s.full_name, cs.status
   FROM student_busy_ranges br
   JOIN students s ON s.id = br.student_id
   LEFT JOIN course_students cs ON cs.student_id = br.student_id AND cs.course_id = $1
   WHERE br.session_id = ANY($2::uuid[])
     AND br.deleted_at IS NULL
     AND br.start_at < $4
     AND br.end_at > $3
   ```
   (For the explicit-student-list path, the course_id parameter can match the course being preflighted.)

3. The `status` field on `ConflictingStudent` is `"draft"` or `"enrolled"` (from `course_students.status`), and `null`/absent for students not in the course roster (non-roster students explicitly included — they're implicitly treated as enrolled for conflict purposes).

### HTTP response shape change

```json
{
  "kind": "student_overlap",
  "conflicts": [
    {
      "session_id": "...",
      "course_id": "...",
      "start_at": "...",
      "end_at": "..."
    }
  ],
  "conflicting_students": [
    {
      "student_id": "...",
      "full_name": "John Doe",
      "status": "draft"
    }
  ],
  "requested": { ... }
}
```

## Calendar Draft-Only Badge: Backend Query

The spec says the calendar should show a `Draft` badge when all enrolled students in a session are draft. This needs a **backend endpoint** or a **response field** on the sessions list endpoint.

### Approach: Add `draft_only` to session response

Add a new field `draft_only` to the session list response (`GET /api/v1/sessions`):

```json
{
  "id": "...",
  "course_id": "...",
  "draft_only": true,
  ...
}
```

### Query to determine `draft_only`

For a given session, the effective roster is:
- The course's `course_students` roster, **minus** students explicitly excluded via `session_attendance`, **plus** students explicitly included via `session_attendance`.

```sql
WITH effective_roster AS (
  SELECT cs.student_id, cs.status
  FROM course_students cs
  WHERE cs.course_id = $1  -- session's course_id
    AND NOT EXISTS (
      SELECT 1 FROM session_attendance sa
      WHERE sa.session_id = $2  -- session_id
        AND sa.student_id = cs.student_id
        AND sa.status = 'excluded'
    )
  UNION
  SELECT sa.student_id, 'enrolled'  -- explicitly included students treated as enrolled
  FROM session_attendance sa
  WHERE sa.session_id = $2
    AND sa.status = 'included'
)
SELECT
  CASE
    WHEN count(*) = 0 THEN false  -- no students at all → not draft-only
    WHEN bool_or(status = 'enrolled') THEN false  -- at least one enrolled student
    ELSE true  -- all students are draft
  END AS draft_only
FROM effective_roster;
```

This is efficient enough to run per-session in a batch query for the sessions list endpoint.

## Full Backend API Changes

### `GET /api/v1/courses/{id}/students` (roster list)

**Change:** Response now includes `status` field on each student.

```json
[
  {
    "id": "...",
    "wcode": "W-1234",
    "full_name": "John Doe",
    "notes": "",
    "status": "enrolled"     // NEW
  },
  {
    "id": "...",
    "wcode": "W-5678",
    "full_name": "Jane Doe",
    "notes": "",
    "status": "draft"        // NEW
  }
]
```

- Backend: switch from `CourseStudentsListDetailed` to `CourseStudentsListDetailedWithStatus` (new sqlc query).
- Frontend: known students may also have `status: "draft"` to render the badge.

### `POST /api/v1/scheduling/preflight` (response)

See "Overlap Conflict Details Enrichment" section above. The response now includes `conflicting_students` with `status` field.

### New endpoint: `POST /api/v1/courses/{id}/students/draft`

Add a draft student to a course roster.

- **Auth:** Admin only.
- **CRM guard:** Check `crm_filter_enabled` on the course. If enabled, reject with `409`.
- **Body:** `{ "wcode": "W-1234" }`
- **If student not found by wcode:** `404`
- **Preflight:** Run preflight checking this student's busy ranges against the course's existing sessions.
  - If preflight passes → add to `course_students` with `status = 'draft'` via `CourseStudentAddDraft`.
  - If preflight fails (draft vs enrolled OR draft vs draft) → `409` with `ConflictDetails`.
- **On success:** `200` with `{ "student_id": "...", "status": "draft" }`.
- **Audit:** Log `course_students.add` with payload `{ "source": "draft" }`.

### New endpoint: `POST /api/v1/courses/{id}/students/{studentId}/convert`

Convert a draft to enrolled.

- **Auth:** Admin only.
- **CRM guard:** Check `crm_filter_enabled` on the course. If enabled, reject with `409`.
- **Body:** (empty)
- **Preflight check:** None — draft already generated busy ranges. Converting doesn't affect preflight (the student may already conflict with other drafts — that's handled by the override rules when another enrolled student is added).
- **Logic:** `UPDATE course_students SET status = 'enrolled' WHERE course_id = $1 AND student_id = $2 AND status = 'draft'`.
- **If not found or not draft:** `404`.
- **On success:** `200` with `{ "student_id": "...", "status": "enrolled" }`.
- **Audit:** Log `course_students.convert` with payload `{ "source": "manual" }`.

### Modified: `POST /api/v1/courses/{id}/students` (existing add enrolled)

**Changes:**

1. Preflight is already run on the backend when creating sessions. For this endpoint, we need to add preflight check if draft conflicts exist.
2. If preflight fails **only** due to draft conflicts (all conflicting students are `status = 'draft'`):
   - Return `409` with `ConflictDetails` + `"draft_conflict_ids": [...]`.
   - Include `conflicting_students` so the UI knows which are drafts.
3. Client re-sends with `X-Override-Draft-Conflict: true` header.
4. Backend in a transaction:
   a. `DELETE FROM course_students WHERE course_id = $1 AND student_id = ANY($2) AND status = 'draft'` (using `CourseStudentRemoveDraftIfStatus`).
   b. `INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, 'enrolled') ON CONFLICT DO NOTHING` (using existing `CourseStudentAdd` — default is 'enrolled').
   c. Busy ranges updated by existing triggers.
5. Response: `{ "ok": true, "removed_draft_ids": [...] }`.

**Note on transactional integrity:** The preflight is advisory — between the preflight check and the write, the situation could change. The DB exclusion constraint on `student_busy_ranges` is the final gate. If the DB rejects the write due to an enrolled student overlap, the handler should fall back to `explainFromDBErrByRepreflight` (existing pattern) to produce stable conflict details.

### Header-based override protocol

The override uses the same existing add-student endpoint with an additional header:

```
POST /api/v1/courses/{id}/students
Content-Type: application/json
X-Override-Draft-Conflict: true

{ "student_id": "..." }
```

The backend checks for the header only after a preflight check detected draft-only conflicts. If the header is present but the conflict is not draft-only (e.g., enrolled vs enrolled), the hard block still applies.

## Auditor / CRM Filter Guard on Draft Operations

All three endpoints (add draft, convert, add enrolled with override) must check `crm_filter_enabled` on the course before mutating the roster. Pattern (same as existing `handleCourseStudentsAdd`):

```go
var crmEnabled bool
_ = s.deps.DB.QueryRow(r.Context(), `SELECT crm_filter_enabled FROM courses WHERE id=$1`, courseID).Scan(&crmEnabled)
if crmEnabled {
    s.a.WriteErr(w, http.StatusConflict, "crm_managed_roster", "Roster is managed by CRM filter. Disable CRM filter to edit manually.")
    return
}
```

## Audit Events

All new mutations log audit events:

| Action | Source | When |
|---|---|---|
| `course_students.add` | `draft` | Add Draft endpoint succeeds |
| `course_students.convert` | `manual` | Convert Draft → Enrolled |
| `course_students.remove` | `draft_override` | Override kicks draft from roster |
| `course_students.add` | `draft_override` | Override adds enrolled student after kick |

## Visual Indicators — Implementation Notes

### Roster table (Course Detail)

- The `GET /api/v1/courses/{id}/students` response now includes `status`.
- The `AttendeeSection` component renders a `Draft` badge (gray pill, `text-[10px]`) next to the student's name when `status === 'draft'`.
- A "Convert to enrolled" button appears on draft rows (admin only).
- An "Add Draft" button appears alongside the existing "Add Manual" button (admin only).

### Calendar (sessions)

- The `GET /api/v1/sessions` response includes a `draft_only` boolean (computed by the query in the "Calendar Draft-Only Badge" section above).
- `ScheduleSessionCard` renders a `Draft` badge next to the course name when `draft_only === true`.

### Preflight conflict details

- The conflict details response includes `conflicting_students` with `status` fields.
- The UI shows a `Draft` badge next to students whose `status === 'draft'`.
- For draft-only conflicts, the UI shows an additional override option (2-step confirmation).

## Acceptance Criteria

- **Draft blocks preflight**: Adding a draft student whose busy ranges overlap an enrolled student returns a blocking conflict (hard block).
- **Enrolled overrides draft**: Adding an enrolled student that only conflicts with drafts returns a conflict with override option. Confirming the override kicks the draft(s) from the roster.
- **Draft vs draft hard block**: Adding a draft student that conflicts with another draft returns a blocking conflict (no override).
- **Visual badges**: Draft students show a `Draft` badge in the roster, calendar, and preflight conflict details.
- **Conversion**: Converting a draft to enrolled is a one-click operation that changes the status.
- **Brand-new prospect**: Staff can create a minimal student record (W-code + name) and then add them as draft in two sequential actions.
- **CRM guard**: Adding/editing draft students is blocked when CRM filter is enabled on the course.
- **Audit trail**: All draft-related mutations are recorded with appropriate source labels.
- **Roster list returns status**: The `GET /api/v1/courses/{id}/students` response includes the `status` field.
- **Calendar draft badge**: Sessions where all effective students are draft show `draft_only: true` in the API response.

## Out of Scope (v1)

- Bulk add/convert of drafts.
- Automatic draft expiry / cleanup.
- CRM-triggered draft status changes.
- Per-session draft-only flag (sessions with only draft students are auto-detected, not explicitly flagged).
- Draft student notes or custom fields (uses existing `students.notes`).

## Migration Plan

1. **Migration `00013_course_students_draft_status.sql`**: `ALTER TABLE course_students ADD COLUMN status ...` with default `'enrolled'` and CHECK constraint.
2. **sqlc regenerate**: Add new queries, regenerate Go code.
3. **Go backend changes**: New endpoints, modified handlers, enriched conflict details, audit logging, CRM guard.
4. **Frontend changes**: Status-aware roster rendering, draft badge, convert button, add draft flow, override UI.
5. **Existing rows**: Already have `status = 'enrolled'` via the default — no backfill needed.

## Test Plan

Add integration tests under `backend/internal/scheduling/` and `backend/internal/httpapi/courseshttp/`:

- `TestDraftStudent_BlocksPreflight`: Add a draft that conflicts with an enrolled student → expect hard block.
- `TestDraftStudent_EnrolledCanOverrideDraft`: Add an enrolled student that conflicts with draft → expect conflict details with draft info. Follow with override request → expect success with removed_draft_ids.
- `TestDraftStudent_DraftVsDraft_HardBlock`: Add two drafts that conflict → expect hard block on second.
- `TestDraftStudent_ConvertToEnrolled`: Add draft, convert → expect status change.
- `TestDraftStudent_RosterListIncludesStatus`: Fetch roster → expect status field.
- `TestDraftStudent_CRMGuardBlocksDraftOps`: Enable CRM filter → expect 409 on draft operations.
