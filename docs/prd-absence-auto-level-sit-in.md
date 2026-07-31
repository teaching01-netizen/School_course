# PRD: Absence Auto-Level Sit-In

## Problem Statement

Students at Warwick Institute take courses organised within subjects by level (beginner, intermediate, advanced). When a student reports an absence, they should be automatically assigned to attend ("sit in") an appropriately levelled alternative class — a higher-level if they're mid-level, a lower-level if they're at the top, or a Zoom session if they're at the beginner level. Currently the absence form only offers a manual free-text sit-in course picker with no awareness of course levels, the student's own level within a subject, or scheduling conflicts between missed and sit-in sessions. Admins have no way to configure these assignment rules per subject.

## Solution

Introduce a course level system (`course_level` + `level_order` per subject) on the `courses` table. The public absence form becomes a multi-step wizard: enter W-Code → system resolves enrolled subjects → pick subject → system auto-determines sit-in (Zoom or physical) based on admin-configurable per-subject policies → student selects from pre-filtered, non-overlapping sit-in sessions → submit. Admins get a Course Levels page to assign levels/orders and configure per-subject absence policies (toggle auto-assignment, map each level to Zoom / sit-in-higher / sit-in-lower).

## User Stories

1. As a student reporting an absence, I want to enter my W-Code and see only my enrolled subjects, so that I don't have to search through irrelevant courses.
2. As a student, I want the system to auto-determine which course I should sit in during my absence, so that I don't need to know the level hierarchy myself.
3. As a student at the beginner level, I want to be told I'll receive a Zoom session, so that I know what to expect without needing to pick a physical class.
4. As a student at the intermediate level, I want to be assigned to the nearest higher-level course for my sit-in, so that I continue progressing.
5. As a student at the advanced level, I want to be assigned to the nearest lower-level course for my sit-in, so that I have a class to attend.
6. As a student, I want to see the actual scheduled sessions of my sit-in course within my absence dates, so that I can pick which ones to attend.
7. As a student, I want only non-overlapping sit-in sessions shown (those not clashing with my missed class times), so that I don't double-book myself.
8. As a student, I want the correct number of sit-in sessions pre-selected for me (matching the number of sessions I'll miss), to minimise manual work.
9. As a student, I want the ability to uncheck pre-selected sit-in sessions or check alternatives, so that I can customise my attendance plan.
10. As a student, I want to enter a date range and optional reason for my absence, so that the institute has the full context.
11. As an admin, I want to assign a level and ordering to each course within a subject, so that the level hierarchy is defined.
12. As an admin, I want to toggle auto sit-in assignment on/off per subject, so that I can opt out subjects from automatic handling.
13. As an admin, I want to configure what action each level triggers (Zoom, sit-in higher, sit-in lower) per subject, so that the policy fits the curriculum.
14. As an admin, I want to view all absences with their subject, sit-in method (Zoom/physical), and linked sit-in sessions, so that I have full visibility.
15. As an admin, I want absence policies to default to sensible values (beginner→Zoom, intermediate→higher, advanced→lower) with auto-assignment on, so that the system works out of the box.
16. As a developer, I want the level resolution logic to be a pure function of (student, subject, dates, policy) so that it can be unit tested without a database.

## Implementation Decisions

### Course Level Model

Two new nullable columns on `courses`:
- `course_level text CHECK IN ('beginner', 'intermediate', 'advanced')`
- `level_order smallint`
- Unique index `idx_courses_level_order_per_subject` on `(subject_id, level_order)` WHERE both are NOT NULL

Levels are per-subject. A subject can have any subset of the three levels. `level_order` determines the ordinal hierarchy — two courses in the same subject cannot share the same `level_order`.

### Absence Extensions

Modifications to `student_absences`:
- `sit_in_method text CHECK IN ('physical', 'zoom')` — how the student attends during absence
- `subject_id uuid REFERENCES subjects(id)` — the subject selected by the student (replaces manual course_id semantics)

New table `absence_sit_ins`:
- `id uuid PK`
- `absence_id uuid FK → student_absences ON DELETE CASCADE`
- `session_id uuid FK → sessions`
- `UNIQUE(absence_id, session_id)`

One row per attended sit-in session. Enables future attendance tracking.

### Policy Configuration

Stored as `absence_policies jsonb` column on the existing `app_settings` singleton row.

Shape:
```json
{
  "subjects": {
    "<subject-uuid>": {
      "auto_sit_in_enabled": true,
      "level_action_map": {
        "beginner": "zoom",
        "intermediate": "sit_in_higher",
        "advanced": "sit_in_lower"
      }
    }
  },
  "zoom": {
    "description": "Zoom session — no physical class attendance required."
  }
}
```

Default when empty: auto-assignment on, beginner→Zoom, intermediate→higher, advanced→lower.

### Level Resolution Algorithm (Deep Module)

Encapsulated in `resolveSitIn()` — a pure-ish function taking (Queries, wcode, subjectID, dateFrom, dateTo):

1. Look up student by W-Code
2. Find the student's enrolled courses in that subject
3. Pick the main course = the one with the highest `level_order`
4. Load absence policy for the subject from `app_settings`
5. If auto-assignment is off, return nil (fall through to manual)
6. Look up the action for the student's course level from `level_action_map`
7. If action is `zoom`, return zoom result immediately
8. If action is `sit_in_higher`, find the nearest course with `level_order > main.level_order`; if `sit_in_lower`, find the nearest with `level_order < main.level_order`
9. If no target course found, return error ("no sit-in course available")
10. Fetch all sessions of the main course in `[dateFrom, dateTo]` (= missed sessions)
11. Fetch all sessions of the target course in `[dateFrom, dateTo]` (= available sessions)
12. Filter available sessions to exclude any that time-overlap with missed sessions
13. Pre-select first N non-overlapping sessions where N = min(missedCount, nonOverlappingCount)

### API Endpoints

**Public (no auth):**
- `GET /api/v1/absences/student-lookup?wcode=...` — Returns student info + list of enrolled subjects
- `GET /api/v1/absences/sit-in-options?wcode=...&subject_id=...&date_from=...&date_to=...` — Returns resolved sit-in result (zoom or physical with session list)
- `POST /api/v1/absences` — Updated body shape: `{ wcode, subject_id, date_from, date_to, reason?, sit_in_method?, sit_in_session_ids[] }`

**Admin (auth required):**
- `GET /api/v1/admin/course-levels` — All courses with level/order/subject info, grouped by subject
- `PUT /api/v1/admin/courses/{id}/level` — Set course_level + level_order for a course
- `GET /api/v1/admin/absence-policies` — Current policy JSON
- `PUT /api/v1/admin/absence-policies` — Update policy JSON
- `GET /api/v1/absences` — Updated to include subject, sit_in_method, and sit-in session details

### Frontend — Course Levels Page (`/course-levels`)

Admin-only page with two-column layout:
- Left: table of all courses grouped by subject. Each row has level dropdown (beginner/intermediate/advanced/none), level order number input, and a Save button (enabled only when dirty).
- Right (sticky side panel): clicking a subject name shows per-subject absence policy editor with auto-sit-in toggle and a level-action-map selector for each of the three levels.

### Frontend — Absence Form (`/absence`, Public)

Multi-step state machine: `WCODE → SUBJECT → DATES → SIT_IN → CONFIRMATION`

Step progression:
1. User enters W-Code → calls student-lookup
2. System shows matched student name + subject dropdown → user picks subject
3. User picks date range + optional reason → calls sit-in-options
4. If Zoom: shows informational blue banner + "Submit Absence" button. If physical: shows green banner with target course name, missed-session count, and checkbox list of available sessions (pre-selected up to missedCount). Checkbox limit enforced. If no sessions available: shows empty state.
5. Submit → POST /api/v1/absences → confirmation page

### Frontend — Admin Absences Table (`/absences`)

Updated table columns: W-Code, Subject, Course, Dates, Reason, Sit-in (shows "Zoom" badge or course code + session count), Submitted.

## Testing Decisions

Good tests assert external behavior in domain language, not internal implementation details. For this feature:

### What Makes a Good Test
- Uses realistic input data (real W-Codes, real session timestamps)
- Asserts outcomes the user/API client can observe (response shape, HTTP status, DB records)
- For the resolver: asserts the correct sit-in course is selected given a known level hierarchy and policy
- For the absence form flow: asserts the correct sit-in method (Zoom vs physical) based on student's level
- Uses real DB queries where practical; mocks only where the dependency cannot be made deterministic

### Modules to Test

1. **Level Resolution Engine** (`resolver.go`): Unit-testable via the `resolveSitIn` function. Should test:
   - Beginner enrolled → returns Zoom
   - Intermediate enrolled → returns next higher level
   - Advanced enrolled → returns next lower level
   - No target course → returns error
   - Non-contiguous level orders (gap in ordering) → picks nearest, not exact +/-1
   - Time overlap filtering: sit-in sessions that clash with missed sessions are excluded
   - Pre-selection: exactly N sessions pre-selected, where N = missed count
   - Auto-assignment disabled → returns nil
   - No policy for this level → returns nil
   - Student not enrolled → returns error

2. **DB Queries** (`absence_custom.go`, `app_settings_custom.go`): Integration tests. Prior art: `backend/internal/db/invariants_integration_test.go`, `backend/internal/db/availability_policy_test.go`

3. **HTTP Handlers** (`routes.go`): Integration tests with real DB. Prior art: `backend/internal/httpapi/schedulinghttp/routes_preflight_test.go`, `backend/internal/httpapi/sessionshttp/routes_test.go`

4. **Frontend Form** (`AbsenceForm.tsx`): Component tests. Prior art: lacks frontend test examples but should follow vitest + jsdom patterns from `src/test/`.

## Out of Scope

- Teacher portal or authentication on the absence form (it remains public, no auth)
- Drag-and-drop scheduling or bulk operations on absences
- Automated email/notification when an absence is submitted
- "Repeat forever" recurrence (series must have explicit end)
- Room-level conflict checking for sit-in sessions (the sit-in is informational, not a hard schedule)
- Tracking actual attendance of sit-in sessions (the join table supports it, but attendance marking is future scope)
- Custom workflow builder UI — the per-subject level-action-map is the extent of admin configurability
- Non-SAT subjects: the level system is generic (any subject can use it), but auto sit-in is per-subject opt-in

## Further Notes

- The level system extends the existing `courses` schema without breaking existing queries — new columns are nullable
- The `absence_sit_ins` join table is designed to later support attendance tracking (marking whether a student actually showed up for their sit-in)
- Migration order matters: `00015_course_levels.sql` must run before any course-level-dependent logic; `00016_absence_extensions.sql` must run before the new absence endpoints are used
- The uuidString helper in the resolver bypasses the `google/uuid` library to avoid an import dependency in the resolver module — this is intentional
