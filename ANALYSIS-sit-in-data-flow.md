# Sit-In Session Data Flow Analysis

## Overview

This document analyzes how the frontend codebase handles sit-in session data — from API response consumption through to UI rendering. The sit-in system is part of the absence management feature, allowing students who miss classes to attend alternative "make-up" sessions.

---

## 1. API Endpoints

### Primary Data Source: `sessions-in-range`

**Endpoint:** `/api/v1/absences/sessions-in-range`

**Parameters:**
- `wcode` — student identifier
- `date_from` / `date_to` — date range
- `course_ids` — comma-separated course IDs (optional)
- `sat_verbal_after_priority` — priority level for SAT Verbal policy advancement (optional)

**Response Shape:**
```typescript
type SessionsInRangeResponse = {
  subjects: SubjectSessions[];
};
```

**Construction:** `src/pages/AbsenceForm.tsx:306-324` (`sessionsInRangePath` function)

### Submission Endpoint

**Endpoint:** `POST /api/v1/absences/batch`

**Payload includes:**
```typescript
type AbsenceBatchCreateItem = {
  subject_id: string;
  course_id: string;
  date_from: string;
  date_to: string;
  reason?: string;
  sit_in_method?: string;
  sit_in_course_id?: string;
  missed_session_ids: string[];
  sit_in_session_ids: string[];
};
```

### Configuration Endpoint

**Endpoint:** `GET /api/v1/absence-form-config`

Returns `AbsenceFormConfig` including sit-in settings:
```typescript
sit_in: {
  auto_resolve_enabled: boolean;
  zoom_description: string;
  max_sessions_per_absence: number;
}
```

### Admin Sit-In Rules Endpoint

**Endpoint:** `GET /api/v1/admin/sit-in-rules`

Returns `SitInRule[]` for admin management.

---

## 2. Type Definitions

**File:** `src/types/index.ts`

### Core Types

#### `SubjectSessions` (line 489)
The primary shape returned by the `sessions-in-range` API:
```typescript
type SubjectSessions = {
  subject_id: string;
  subject_code: string;
  subject_name: string;
  course_id: string;
  course_code: string;
  course_name: string;
  sessions: SessionInSubject[];
  sit_in?: SitInInfo;
};
```

#### `SitInInfo` (line 485)
Extends `SitInSessionInfo` with per-missed-session overrides:
```typescript
type SitInInfo = SitInSessionInfo & {
  sit_in_by_missed_session?: Record<string, SitInSessionInfo>;
};
```

#### `SitInSessionInfo` (line 462)
The core sit-in data structure:
```typescript
type SitInSessionInfo = {
  rule_name?: string;
  rule_type?: string;
  sit_in_method: "physical" | "zoom" | "teacher_case" | "none";
  priorities?: SitInPriority[];
  current_priority_level?: number;
  has_next_priority?: boolean;
  sit_in_course?: { id: string; code: string; name: string; subject_code?: string | null; subject_name?: string | null };
  available_sessions?: Array<{
    id: string;
    start_at: string;
    end_at: string;
    missed_session_id?: string | null;
    class_name?: string | null;
    subject_name?: string | null;
    subject_code?: string | null;
    course_name?: string | null;
    course_code?: string | null;
  }>;
  missed_sessions?: Array<{ id: string; start_at: string; end_at: string }>;
  missed_occurrence_number?: number;
};
```

#### `SitInPriority` (line 427)
Used for multi-level priority sit-in (SAT Verbal policy):
```typescript
type SitInPriority = {
  level: number;
  label: string;
  sit_in_course?: { id: string; code: string; name: string; subject_code?: string | null; subject_name?: string | null };
  available_sessions?: Array<{ /* same fields as SitInSessionInfo.available_sessions */ }>;
  pre_selected?: Array<{ id: string; start_at: string; end_at: string }>;
  unavailable_sessions?: Array<{
    session?: { /* session fields */ } | null;
    reason: string;
    reason_code: string;
    missed_session_id?: string | null;
    occurrence_number?: number | null;
  }>;
};
```

#### `SitInRuleType` (line 5)
Admin-defined rule types:
```typescript
type SitInRuleType =
  | "level_ladder"
  | "cross_section"
  | "any_day_except_last"
  | "rank_chain"
  | "teacher_case_by_case";
```

### Calendar Types

#### `CalendarAbsence` (line 381)
For calendar/display views:
```typescript
type CalendarAbsence = {
  id: string;
  wcode: string;
  student_name: string | null;
  status: AbsenceStatus;
  sit_in_method: string | null;
  sit_in_course_code?: string | null;
  sit_in_course_name?: string | null;
  sit_in_subject_name?: string | null;
  missed_sessions?: CalendarSessionBrief[];
  sit_in_sessions?: CalendarSessionBrief[];
  // ... other fields
};
```

#### `CalendarSitInStudent` (line 354)
For displaying visitors in calendar sessions:
```typescript
type CalendarSitInStudent = {
  wcode: string;
  nickname?: string | null;
  student_name: string | null;
  absence_id: string;
  from_course_code: string;
  from_course_name: string | null;
};
```

---

## 3. Data Flow

### Flow Diagram

```
API: /api/v1/absences/sessions-in-range
  │
  ▼
AbsenceForm.tsx (state: sessions: SubjectSessions[])
  │
  ├─► GroupBy subject → render per-course sections
  │     │
  │     ▼
  │   Per-session row (session.id)
  │     │
  │     ├─ groupWithSitInForMissedSession() → resolves sit_in_by_missed_session[sessionId]
  │     │
  │     ▼
  │   Rendering decision based on sit_in.sit_in_method:
  │     │
  │     ├─ "physical" + has priorities → Priority-based sit-in selector
  │     ├─ "physical" no priorities → Flat sit-in selector
  │     ├─ "zoom" → Zoom info display
  │     ├─ "teacher_case" → "To arrange" display
  │     └─ default → "To arrange" display
  │
  ▼
Submission → POST /api/v1/absences/batch
  with payload: { sit_in_session_ids, sit_in_course_id, sit_in_method }
```

### State Variables in AbsenceForm

**File:** `src/pages/AbsenceForm.tsx`

| Variable | Type | Purpose |
|----------|------|---------|
| `sessions` | `SubjectSessions[]` | API response data |
| `selectedSessionIds` | `Set<string>` | Missed sessions user selected |
| `sitInSelections` | `Record<string, string>` | Maps missed session ID → selected sit-in session ID |
| `sitInPriorityLevels` | `Record<string, number>` | Maps missed session ID → current priority level being viewed |
| `sitInPriorityHistory` | `Record<string, Record<number, SubjectSessions>>` | Cache of fetched priority levels per session |
| `revealingPrioritySessionIds` | `Set<string>` | Sessions currently loading next priority |

---

## 4. Sit-In Method Handling

### Method: `"physical"`

Two sub-patterns based on whether `priorities` exist:

#### A. With Priorities (SAT Verbal Policy / Multi-Level)

**Files:** `src/pages/AbsenceForm.tsx:1702-1832`

Flow:
1. `hasServerPriorityReveal(group)` checks if `current_priority_level` or `has_next_priority` is defined
2. If server-revealed: API call with `sat_verbal_after_priority` param to get next level
3. If client-side: uses `nextPriorityLevel()` / `previousPriorityLevel()` to navigate cached levels
4. Renders priority badge, "Back" / "See other times" navigation buttons
5. Shows `unavailable_sessions` with reasons when no available sessions exist
6. Dropdown populated from `availableSessionsForMissedSession(priority, sessionId)`

Key functions:
- `handleNotAvailable(group, sessionId)` — advances to next priority level (line 964)
- `handlePreviousPriority(group, sessionId)` — goes back (line 1031)
- `prioritiesForLevel(group, level)` — filters priorities by level (line 251)
- `firstPriorityLevel(group)` — gets minimum level (line 231)

#### B. Without Priorities (Flat / Level Ladder)

**Files:** `src/pages/AbsenceForm.tsx:1835-1860`

Simple dropdown of `sit_in.available_sessions` with the sit-in course name as context.

### Method: `"zoom"`

**File:** `src/pages/AbsenceForm.tsx:1862-1869`

Displays a static info message: "Online make-up (Zoom)" with note about staff sending Zoom link. No session selection required.

### Method: `"teacher_case"`

**File:** `src/pages/AbsenceForm.tsx:1870-1873`

Displays "To arrange" — staff will contact to set up.

### Method: `"none"` / default

**File:** `src/pages/AbsenceForm.tsx:1874-1879`

Displays "To arrange" with explanatory text.

---

## 5. Per-Missed-Session Sit-In Resolution

**Function:** `sitInForMissedSession` (line 255)

```typescript
function sitInForMissedSession(group: SubjectSessions, missedSessionId: string) {
  return group.sit_in?.sit_in_by_missed_session?.[missedSessionId] ?? group.sit_in;
}
```

This allows the API to return different sit-in configurations per missed session. The `sit_in_by_missed_session` map on `SitInInfo` lets each missed session have its own:
- `sit_in_method`
- `priorities`
- `available_sessions`

**Wrapper:** `groupWithSitInForMissedSession` (line 259) creates a new group object with the resolved sit-in.

**Filtering:** `availableSessionsForMissedSession` (line 265) and `unavailableSessionsForMissedSession` (line 276) filter sessions by `missed_session_id` field when present.

---

## 6. Priority Level Navigation

### Client-Side Navigation

When `hasServerPriorityReveal()` returns `false` (no `current_priority_level`/`has_next_priority` from API), levels are managed client-side:

- Levels extracted from unique `priority.level` values
- `nextPriorityLevel(group, currentLevel)` — next higher level
- `previousPriorityLevel(group, currentLevel)` — next lower level
- Entire level set cached in `sitInPriorityHistory`

### Server-Side Navigation

When `hasServerPriorityReveal()` returns `true`:

1. User clicks "See other times"
2. `handleNotAvailable()` triggers API call: `sessionsInRangePath(wcode, dateFrom, dateTo, { courseIds: [group.course_id], satVerbalAfterPriority: currentLevel })`
3. Response replaces the group's sit-in data for that level
4. Previous levels preserved in `sitInPriorityHistory` for back-navigation

---

## 7. Display Name Resolution

**Function:** `getCurrentSitInDisplayName` (line 170)

Priority order for display label:
1. If not physical: "Zoom" or "To arrange"
2. If priorities exist: unique labels from `getPriorityTargetDisplayName()` for each priority
3. Fallback: `getSitInCourseDisplayName(sitIn.sit_in_course, ...)`

**Function:** `getSitInCourseDisplayName` (line 136)

Resolution chain:
1. `resolveSitInSubjectName()` — subject name from sit-in course or matching SubjectSessions
2. `sitInCourse.name`
3. `sitInCourse.subject_code`
4. `fallbackSubjectName`
5. `sitInCourse.code`

**Function:** `getSitInSessionLabel` (line 426)

For dropdown options:
```
[className] — [date] [time]-[time]
```

Where className resolves through the same chain as above.

---

## 8. Submission Logic

**Function:** `buildSubmissionPayloads` (line 1151)

For each selected group:
1. Collect selected session IDs
2. Map to sit-in session IDs via `sitInSelections[sessionId]`
3. Determine `sit_in_method` from `group.sit_in?.sit_in_method`
4. Determine `sit_in_course_id` via `selectedSitInCourseIDForGroup()`:
   - If no priorities: use `sit_in.sit_in_course.id`
   - If priorities: find which priority contains the selected sit-in session, use that priority's `sit_in_course.id`
   - If mixed course IDs across sessions: reject with error
5. Submit to `POST /api/v1/absences/batch`

**Validation:** `missingSitIn` computed property (line 679) blocks submission if any selected physical session lacks a sit-in selection.

---

## 9. Components

### AbsenceForm (Main Form)
**File:** `src/pages/AbsenceForm.tsx` (1955 lines)

The primary component handling sit-in data flow. Manages all state for session selection and sit-in choices.

### SitInResultCard
**File:** `src/components/absences/SitInResultCard.tsx` (220 lines)

Displays sit-in results after submission or in confirmation views. Handles:
- `zoom` — blue info card
- `pending` — amber "will be assigned" card
- `physical` — green card with day-by-day missed/available pairing with checkboxes

### SitInListView
**File:** `src/components/absences/SitInListView.tsx` (134 lines)

Table view of all sit-in visitors for a calendar day. Used in the Operations Calendar. Columns: Student, Leaving, Sit-in, Date/Time, Method, Status.

### SitInTableRow
**File:** `src/components/absences/SitInTableRow.tsx` (45 lines)

Single row in the sit-in list table.

### SidePanelSitInCard
**File:** `src/components/absences/SidePanelSitInCard.tsx` (75 lines)

Card displayed in the calendar side panel showing sit-in assignment for a specific absence.

### SidePanel
**File:** `src/components/absences/SidePanel.tsx` (216 lines)

Day-detail side panel with two tabs: "Sit-ins" and "Absences". Filters absences to show only those with sit-in assignments (physical/zoom method or assigned as visitors).

### AbsenceDetail
**File:** `src/pages/AbsenceDetail.tsx` (417 lines)

Detail view for a single absence. Displays sit-in plan label and missed session dates.

### SitInRuleInventoryPage
**File:** `src/pages/operations/SitInRuleInventoryPage.tsx` (251 lines)

Admin CRUD for sit-in rules. Rule types: `level_ladder`, `cross_section`, `any_day_except_last`, `rank_chain`, `teacher_case_by_case`.

### Supporting Files
- `src/components/absences/calendarDisplay.ts` — Display helpers: `getSitInLabel()`, `getSitInVisitorLabel()`, `absenceInlineClasses()`
- `src/components/absences/sitInLabel.ts` — `formatSitInLabel()` for managed absences
- `src/components/absences/dateSummary.ts` — `formatAbsenceSummaryDates()` using `missed_sessions`
- `src/components/absences/AbsenceFormEditor.tsx` — Admin config editor for sit-in settings
- `src/hooks/useSitInRules.ts` — CRUD hook for admin sit-in rules

---

## 10. How Different Rule Types Are Handled

### Frontend Treatment

The frontend does **not** differentiate between rule types (`level_ladder`, `cross_section`, etc.) at the UI level. Instead, the API response shape determines the UI:

| API Response Pattern | Frontend Behavior |
|---------------------|-------------------|
| `sit_in_method: "physical"` + `priorities: [...]` with multiple levels | Priority navigator with "Back"/"See other times" buttons |
| `sit_in_method: "physical"` + `priorities: [...]` single level | Flat dropdown of available sessions |
| `sit_in_method: "physical"` + no priorities, flat `available_sessions` | Simple dropdown |
| `sit_in_method: "zoom"` | Zoom info card |
| `sit_in_method: "teacher_case"` | "To arrange" message |
| `sit_in_by_missed_session` map present | Per-session sit-in resolution |
| `unavailable_sessions` with reasons | Shows why specific slots are blocked |

### Admin Side

Rule types are managed in the admin interface:
- `src/pages/operations/SitInRuleInventoryPage.tsx` — CRUD for rules
- `src/components/RuleSelector.tsx` — Dropdown to pick rule type
- `src/components/RulePredicateForm.tsx` — Type-specific predicate fields
- `src/components/RulePreviewPanel.tsx` — Visual preview of rule behavior
- `src/components/RuleExampleSection.tsx` — Example scenarios per rule type

The `level_ladder` rule type has special handling in the admin UI (min_level_for_sit_lower predicate field).

---

## 11. Key Patterns Summary

1. **Sit-in data is per-subject** — each `SubjectSessions` in the API response carries its own `sit_in` configuration
2. **Per-missed-session overrides** — `sit_in_by_missed_session` allows different sit-in options for each missed session within a subject
3. **Priority is a UI concept** — the frontend renders priority levels as a navigation experience, but the actual rule logic lives in the backend
4. **Server-revealed priorities** — when the API provides `current_priority_level`/`has_next_priority`, the frontend fetches new data on level changes rather than using cached data
5. **Submission is flat** — regardless of priority UI complexity, the submission payload is flat: `sit_in_session_ids` + `sit_in_course_id` + `sit_in_method`
6. **Display names are resilient** — multiple fallback chains ensure sit-in labels render even with partial data
