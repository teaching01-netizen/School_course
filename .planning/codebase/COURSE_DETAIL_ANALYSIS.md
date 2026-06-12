# Course Detail Page — Complete Analysis

**Analysis Date:** Fri Jun 12 2026

## 1. Routes & Navigation

### Route definitions (`src/App.tsx`, lines 62-65)

| Route | Component | File |
|-------|-----------|------|
| `/courses` | `<Courses />` | `src/pages/Courses.tsx` |
| `/courses/create` | `<CourseCreate />` | `src/pages/CourseCreate.tsx` |
| `/courses/:id` | `<CourseDetail />` | `src/pages/CourseDetail.tsx` |
| `/courses/:id/edit` | `<CourseEdit />` | `src/pages/CourseEdit.tsx` |

All course routes are within a `<RequireAuth>` wrapper and rendered inside `<Layout>` via `<Outlet />`.

### Navigation (from `src/components/Layout.tsx`, line 20)
- The top nav has a "Course" link at `/courses` under the "Directory" group
- Clicking it goes to the Courses list page; each row has a "detail" link to `/courses/:id`

---

## 2. Course List Page (`src/pages/Courses.tsx`)

**Purpose:** Displays all courses in a table with search, teacher filter, and batch delete.

**Data model (inline `CourseRow`, lines 16-31):**
```typescript
type CourseRow = {
  id: string;
  course_no: number;
  code: string;
  name: string;
  year: number | null;
  teacher_id: string | null;
  teacher_name: string;
  subject_id: string | null;
  subject_code: string;
  subject_name: string;
  hour: number | null;
  student_count: number | null;
  course_type: string | null;
  legacy_course_id?: string | null;
};
```

**Data fetching:**
- `GET /api/v1/courses` via `useApiQuery` hook (line 45)
- Teachers loaded separately via `GET /api/v1/users?role=Teacher` (lines 51-55)

**Table columns:** C-ID, C-Code, Year, Teacher, Subject, Hour, Student, Type, Legacy, detail link
- Each row links to `/courses/${course.id}` (line 259) with a "detail" button

---

## 3. Course Detail Page (`src/pages/CourseDetail.tsx`) — 1366 lines

### 3.1 Data Types Used

From `src/types/index.ts`:

```typescript
// Line 124-131
export type Course = {
  id: string;
  code: string;
  name: string;
  deleted_at?: string | null;
  legacy_course_id?: string | null;
  legacy_last_synced_at?: string | null;
};

// Line 113-122
export type Session = {
  id: string;
  series_id?: string | null;
  course_id: string;
  room_id: string | null;
  teacher_id: string;
  start_at: string;
  end_at: string;
  version: number;
};

// Line 132
export type Room = { id: string; name: string; capacity: number | null };

// Line 133
export type User = { id: string; username: string; role: "Admin" | "Teacher" };

// Line 134-142
export type Student = {
  id: string; wcode: string; full_name: string;
  notes: string; status?: string;
  student_phone?: string | null; parent_phone?: string | null;
};
```

### 3.2 API Calls Made

All data fetching is done via `apiJson()` from `src/api/client.ts`.

| Endpoint | Method | Usage | Line(s) |
|----------|--------|-------|---------|
| `GET /api/v1/courses/{id}` | GET | Load course details | 331 |
| `GET /api/v1/rooms` | GET | Load room options | 344-345 |
| `GET /api/v1/users?role=Teacher` | GET | Load teacher options | 346 |
| `GET /api/v1/courses/{id}/students` | GET | Load student roster | 359 |
| `GET /api/v1/courses/{id}/sessions` | GET | Load schedule sessions | 372 |
| `GET /api/v1/courses/{id}/crm-filter` | GET | CRM filter state | 167-169 |
| `GET /api/v1/meta/time` | GET | Institute timezone + server time | 391 |
| `PUT /api/v1/courses/{id}` | PUT | Update course (code/name/legacy) | 61-63, 79-80 |
| `DELETE /api/v1/courses/{id}` | DELETE | Delete course | 405 |
| `DELETE /api/v1/courses/{id}/students/{student_id}` | DELETE | Remove student | 423 |
| `POST /api/v1/courses/{id}/students` | POST | Add student | 440 |
| `GET /api/v1/students/{wcode}` | GET | Lookup student by wcode | 439 |
| `PATCH /api/v1/sessions/{id}` | PATCH | Edit a session | 295-305 |
| `POST /api/v1/sessions` | POST | Create one-off session | 557-566 |
| `POST /api/v1/series` | POST | Create recurring series | 677-690 |
| `POST /api/v1/courses/{id}/legacy-sync` | POST | Trigger legacy sync | 94 |
| `POST /api/v1/scheduling/preflight` | POST | Preflight single session | usePreflight.ts |
| `POST /api/v1/scheduling/preflight_series` | POST | Preflight series | usePreflight.ts |

### 3.3 Page Structure (Render Tree)

```
CourseDetail
├── HEADER SECTION (lines 710-727)
│   ├── PageHeading: "Course #{course.code}"
│   ├── Edit Link → `/courses/:id/edit`
│   └── Delete Button → ConfirmModal
│
├── COURSE NAME BAR (lines 724-727)
│   ├── course.name (text-sm)
│   └── course.id (text-xs monospace)
│
├── LEGACY LINK SECTION (lines 778-783) [Admin only]
│   └── LegacyLinkSection component (inline, lines 47-151)
│       └── Shows link/unlink UI for legacy system
│
├── SCHEDULE SECTION (lines 729-966)
│   ├── View toggle: Table | Calendar
│   ├── Table view (lines 833-965)
│   │   ├── Columns: Date | Begin | End | Duration | Classroom | By
│   │   ├── Each row has inline edit capability
│   │   │   ├── Edit button → inline date/time/room inputs
│   │   │   ├── PreflightBadge + PreflightIndicator
│   │   │   └── Save / Cancel buttons
│   │   ├── Duration uses `fmtDuration(minutesBetween(...))`
│   │   ├── Classroom shows room name badge or [NOT SET] badge
│   │   └── Links to `/schedule` for check-in
│   │   └── Empty state: "No sessions in range"
│   └── Calendar view (lines 786-832)
│       ├── Week navigation (prev/next/today)
│       ├── MON-FRI × 24-hour time grid
│       └── ScheduleSessionCard per session (shows name, room, time; tooltip on hover)
│
├── ADD SCHEDULE MODAL (lines 968-1327)
│   ├── Tab bar: "Recurring series" | "One-off session" | "Paste schedule"
│   ├── Series tab (lines 1178-1324)
│   │   ├── Room select (Select component)
│   │   ├── Teacher select (TypeaheadSelect component)
│   │   ├── Weekday buttons (S M T W T F S)
│   │   ├── Start time (time input, step=300)
│   │   ├── Duration (number, step=5)
│   │   ├── Start date, End date or count
│   │   └── PreflightIndicator
│   ├── Session tab (lines 1040-1105)
│   │   ├── Room select, Teacher select (TypeaheadSelect)
│   │   ├── Start/End datetime-local (step=300)
│   │   └── PreflightIndicator
│   └── Paste tab (lines 1106-1177)
│       ├── Teacher select (TypeaheadSelect)
│       ├── Textarea for pasted schedule rows
│       └── Preview table with room name matching
│
├── ATTENDEE SECTION (lines 1329-1342)
│   └── AttendeeSection component (src/components/AttendeeSection.tsx)
│       ├── Table: W-code | Name | Status | Notes | Actions
│       ├── DRAFT vs ENROLLED status badges
│       ├── Admin actions: Add Manual, +Draft, Add from Sage (CRM)
│       ├── Draft students can be converted to enrolled
│       └── CRM filter manages roster when enabled
│
├── DELETE CONFIRM MODAL (lines 1344-1353)
│   └── ConfirmModal: "Permanently delete this course?"
│
└── REMOVE STUDENT CONFIRM (lines 1355-1363)
    └── ConfirmModal: "Remove this student from the course roster?"
```

### 3.4 State Variables (lines 159-232)

| State | Type | Purpose |
|-------|------|---------|
| `course` | `Course \| null` | Loaded course data |
| `loading` | `boolean` | Initial course load |
| `deleting` | `boolean` | Delete in progress |
| `roster` | `Student[]` | Student roster list |
| `rosterLoading` | `boolean` | Roster loading indicator |
| `sessions` | `Session[]` | Schedule sessions |
| `sessionsLoading` | `boolean` | Sessions loading indicator |
| `rooms` | `Room[]` | Available rooms |
| `teachers` | `User[]` | Available teachers (role=Teacher) |
| `instituteTZ` | `string \| null` | Institute timezone (default "Asia/Bangkok") |
| `serverNow` | `string \| null` | Server current time |
| `viewMode` | `'table' \| 'calendar'` | Schedule view mode toggle |
| `weekStart` | `Date` | Calendar week start |
| `editingSessionId` | `string \| null` | Currently editing session |
| `editForm` | `{ date, begin, end, room_id }` | Inline edit form state |
| `editPreflight` | `UsePreflightReturn` | Preflight for inline edits |
| `editSaving` | `boolean` | Inline edit save in progress |
| `createOpen` | `boolean` | Add schedule modal open |
| `createTab` | `"series" \| "session" \| "paste"` | Active create tab |
| `sessionForm` | `{ room_id, teacher_id, start_local, end_local }` | One-off session form |
| `sessionPreflight` | `UsePreflightReturn` | Preflight for one-off |
| `seriesForm` | `{ room_id, teacher_id, weekdays[], ... }` | Series create form |
| `seriesPreflight` | `UsePreflightReturn` | Preflight for series |
| `seriesUseCount` | `boolean` | Toggle count vs end_date |
| `pasteText` | `string` | Paste schedule textarea |
| `pasteTeacherId` | `string` | Teacher for pasted sessions |
| `crmEnabled` / `crmLocked` | `boolean` | CRM filter state |
| `confirmDelete` / `confirmRemoveStudent` | `boolean \| string` | Confirm dialogs |

---

## 4. Course Edit Page (`src/pages/CourseEdit.tsx`) — 94 lines

**Route:** `/courses/:id/edit`

**Data model:**
```typescript
type Course = { id: string; code: string; name: string };
```

**Fields edited:**
- `code` (required) — Input field
- `name` (required) — Input field

**Data flow:**
1. `GET /api/v1/courses/{id}` to load course (line 40)
2. `PUT /api/v1/courses/{id}` with `{ code, name }` to save (line 60)
3. On success: navigate to `/courses/{id}` (line 62)

**Validation:** Uses `useFormValidation` hook with schema requiring both `code` and `name`. Dirty form tracking via `useDirtyForm` with `warnBeforeUnload`.

**UI:** Simple two-field form with Cancel and Save buttons, `FormErrorSummary` for validation errors.

---

## 5. Course Create Page (`src/pages/CourseCreate.tsx`) — 161 lines

**Route:** `/courses/create`

**Fields:** Year, Teacher (Select), Subject (Select), Hour, Student Count, Type (Private/Group)

**API:** `POST /api/v1/courses` with body `{ year, teacher_id, subject_id, hour, student_count, course_type }`

---

## 6. Key Components Used on CourseDetail

### `components/ui/Button.tsx`
Variants: `primary`, `secondary`, `danger`, `ghost`
Sizes: `sm`, `md`, `lg`
Props: `loading`, `disabled`, `type`

### `components/ui/Select.tsx`
Standard `<select>` wrapper with `size` prop (`sm`/`md`)

### `components/ui/Input.tsx`
Standard `<input>` wrapper with `size` (`sm`/`md`), `type` (`text`, `date`, `time`), `step`

### `components/ui/PageHeading.tsx`
Renders `<h1>` with `text-[32px] font-bold`

### `components/TypeaheadSelect.tsx`
Searchable dropdown used for teacher selection. Props: `value`, `onChange`, `options` (array of `{ value, label, keywords }`), `placeholder`.

### `components/Modal.tsx`
Reusable modal with: title, children, footer, size (sm/md/lg/xl/full), focus trap, escape/overlay close, aria-modal.

### `components/ConfirmModal.tsx`
Confirmation dialog using Modal. Props: `open`, `title`, `message`, `variant`, `confirmLabel`, `loading`, `onConfirm`, `onCancel`.

### `components/PreflightIndicator.tsx`
Displays availability check results:
- **Available** (green checkmark)
- **Provisional** (amber) — student + teacher ✅, room ⏳
- **Blocked** (red) — with conflict details, suggested fixes, "Find Alternative Slots" link to `/slot-finder`

Exports: `PreflightIndicator`, `PreflightBadge`, `getSaveButtonLabel`, `isSaveDisabled`

### `components/ScheduleSessionCard.tsx`
Used in calendar view (line 809). Shows session name, room, time. Hover tooltip displays: course code/name, subject, teacher, room+caps, time.

### `components/AttendeeSection.tsx`
Student roster management. Supports:
- CRUD operations on course students
- Draft (tentative) enrollment with conversion
- CRM/Sage filter integration
- Admin-only controls

### `components/crm/CrmFilterPanel.tsx` (via AttendeeSection)
CRM filter configuration modal.

### `components/ui/LoadingSkeleton.tsx`
Loading placeholders with `type` (`card`, `table`) and `lines`.

### `components/ui/EmptyState.tsx`
Empty state display with optional `message` and `action`.

---

## 7. How Teacher Information is Displayed & Selected

**Display:**
- In table view columns: teacher shown under "By" column — either inline edit controls or a link to `/schedule`
- In calendar view: `teacherById` map resolves `teacher_id` to `username` for `ScheduleSessionCard` tooltip
- Preflight conflict displays: `teachersById` map resolves teacher names in conflict details

**Selection:**
- **Create modals:** `TypeaheadSelect` component (searchable dropdown) populated from `GET /api/v1/users?role=Teacher`
- **Inline edit:** Teacher is NOT editable in the current inline edit form (only date, time, room can be changed)
- **Series form:** Teacher selection via `TypeaheadSelect`
- **One-off session form:** Teacher selection via `TypeaheadSelect`
- **Paste schedule:** Teacher selection via `TypeaheadSelect`
- Teacher options memoized as: `{ value: t.id, label: t.username, keywords: t.id }`

---

## 8. How Subject/Course Name is Displayed

**Course name** shown in:
- Page heading: `Course #{course.code}` (line 712)
- Below heading: `course.name` in `text-sm text-gray-700` (line 725)
- Calendar `ScheduleSessionCard`: `course?.name ?? session.course_id`
- Preflight indicators: `coursesById` map for conflict labels

**Subject info** is NOT displayed on the CourseDetail page. It's only visible on the Courses list page (`Courses.tsx`) where `subject_code` + `subject_name` are shown as columns.

---

## 9. Backend API Handlers (`backend/internal/httpapi/courseshttp/routes.go`)

| Endpoint | Handler | Purpose |
|----------|---------|---------|
| `GET /api/v1/courses` | `handleCoursesList` (line 42) | List with optional `include_archived` |
| `POST /api/v1/courses` | `handleCoursesCreate` (line 197) | Create course |
| `GET /api/v1/courses/{id}` | `handleCoursesGet` (line 586) | Get single course |
| `PUT /api/v1/courses/{id}` | `handleCoursesUpdate` (line 630) | Update course (code, name, legacy) |
| `DELETE /api/v1/courses/{id}` | `handleCoursesDelete` (line 687) | Soft delete course |
| `POST /api/v1/courses/batch-delete` | `handleCoursesBatchDelete` (line 762) | Batch delete |
| `GET /api/v1/courses/{id}/students` | `handleCourseStudentsList` | List students in course |
| `POST /api/v1/courses/{id}/students` | `handleCourseStudentsAdd` | Add student |
| `DELETE /api/v1/courses/{id}/students/{student_id}` | `handleCourseStudentsRemove` | Remove student |
| `POST /api/v1/courses/{id}/students/draft` | `handleCourseStudentsAddDraft` | Add draft student |
| `POST /api/v1/courses/{id}/students/{student_id}/convert` | `handleCourseStudentsConvert` | Convert draft to enrolled |
| `GET /api/v1/courses/{id}/sessions` | `handleCourseSessionsList` | List course sessions |
| `POST /api/v1/courses/{id}/legacy-sync` | `handleLegacySync` | Trigger legacy system sync |

---

## 10. Existing Edit Functionality

### Course-level editing:
- **`/courses/:id/edit`** (`CourseEdit.tsx`): Edit `code` and `name` fields only. Full page form with validation.

### Session-level editing (inline on CourseDetail):
- Each session row has an **Edit** button in the "By" column
- Inline editing of: **date** (type=date), **begin** (type=time, step=300), **end** (type=time, step=300), **room_id** (Select dropdown with "[NOT SET] (Provisional)" option)
- **Teacher is NOT editable** in the inline edit form — teacher stays at `s.teacher_id`
- Preflight check runs on every edit form change
- Save requires preflight status === "available" or "provisional"
- Optimistic concurrency: sends `expected_version: s.version`
- Handles `stale_edit` error (409) by reloading sessions and asking user to edit again

### Series editing:
- **Not available on CourseDetail.** The series edit/cancel functionality (with scope options like "this occurrence", "this & future", "entire series") is described in CONTEXT.md but no UI exists for it in the current codebase.

---

## 11. Key File Index

| File | Path | Purpose |
|------|------|---------|
| App routes | `src/App.tsx` | Route definitions (lines 62-65) |
| Course Detail | `src/pages/CourseDetail.tsx` | Main course detail page (1366 lines) |
| Course List | `src/pages/Courses.tsx` | Course listing with search/filter (300 lines) |
| Course Edit | `src/pages/CourseEdit.tsx` | Edit course code/name (94 lines) |
| Course Create | `src/pages/CourseCreate.tsx` | Create new course (161 lines) |
| Types | `src/types/index.ts` | Course, Session, Room, User, Student types |
| API Client | `src/api/client.ts` | `apiJson` wrapper with idempotency |
| Preflight Hook | `src/hooks/usePreflight.ts` | Availability/conflict checking hook |
| Preflight UI | `src/components/PreflightIndicator.tsx` | Conflict display component |
| Session Card | `src/components/ScheduleSessionCard.tsx` | Calendar session card |
| Attendee Section | `src/components/AttendeeSection.tsx` | Roster management |
| Modal | `src/components/Modal.tsx` | Reusable modal dialog |
| Layout | `src/components/Layout.tsx` | Navigation and shell |
| Backend routes | `backend/internal/httpapi/courseshttp/routes.go` | All course API handlers |
