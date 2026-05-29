# CRM XLSX Upload + Course-Level "Excel Filters" (Design)

Date: 2026-05-22
Status: Approved for implementation

## Context

This repo is a single Go backend + React SPA scheduling/admin app. Today:

- `students` are keyed by `wcode` (unique) with `full_name` and `notes`.
- Course rosters are managed via `course_students` (many-to-many).
- Admins currently add roster members manually (e.g., by WCode).

We need an authoritative import from **user-uploaded XLSX exports** (exported from the user's own CRM) where the uploaded data is the source of truth for course rosters.

## Goals

1. **User uploads XLSX export via web UI** — replaces all imported rows atomically.
2. **Store latest snapshot** in Postgres from the upload (single authoritative dataset).
3. Provide an **Admin-only, Excel-like filtering UI** inside Course Detail that:
   - lets admins filter imported CRM rows via column drop-downs,
   - saves the filter to the course,
   - and makes the course roster **CRM-managed and authoritative**.
4. On upload success and on filter save, **reconcile rosters** (add/remove) to match the filter.
5. Provide a **per-course roster lock** that prevents auto-reconciliation on future uploads.
6. Keep the UI responsive via **job + polling** (synchronous UX, async implementation).

## Non-goals (v1)

- Bi-directional sync (our app → CRM).
- Auto-creating courses based on CRM text.
- Full Excel filter feature parity (regex, complex boolean trees, advanced formula logic).
- Historical exports retention beyond the "latest snapshot".
- Multiple uploads accumulating (full replace only).

## Key Design Decisions (v1)

Decided via design grilling session:

| Decision | Choice |
|---|---|
| Upload model | Global XLSX upload → **full replace** of all `crm_rows` atomically |
| File format | Fixed canonical schema (same 16-column set from observed CRM export) |
| CRM integration | ❌ Stripped — no Sage CRM HTTP scraping, no scheduler |
| Auto-reconcile | ✅ Yes — after successful upload, auto-reconcile all courses with CRM filter enabled |
| Upload UX | Async job + polling (simple file picker on CRM Admin page) |
| Upload summary | ✅ Show rows parsed, courses reconciled, adds/removes |
| Per-course roster lock | Simple toggle on Course Detail filter panel — prevents auto-reconcile |
| Lock behavior | Filter editable while locked, roster frozen. Reconciles on unlock. |
| `crm_cycles` table | Kept — populated from uploaded data, provides filter dropdown options |
| Filter panel scope | Multi-select equals (Cycle, Course Name, Academic Level, Secondary School) + text search (Teacher(s)) + blank/non-blank toggles + preview count + lock toggle |
| Roster read-only | When CRM filter enabled, manual roster edits are blocked |

## Data Model

### 1) Cycle configuration (filter dropdown source)

We maintain a `crm_cycles` table populated from distinct cycle values found in uploaded data. This table provides the list of cycle values for filter dropdowns in Course Detail.

- `Active`: filter dropdown shows this cycle.
- `Frozen`: still available for existing filters, but hidden from new filter creation once support is added.

### 2) Imported CRM rows

We store the imported "enrollment-like" rows keyed by course name + WCode + cycle.

Stored columns (v1, as observed from CRM export):

- `wcode` (from `Student Id`)
- `first_name`, `last_name`, `nickname`
- `secondary_school`, `academic_level`
- `mobile_phone`, `primary_email`
- `parent_name`, `parent_phone`, `parent_email`
- `course_name`
- `cycle`
- `hours`
- `teachers_raw` (from `Teacher(s)`)
- `order_quote_updated_at` (used for debugging/optional future use)
- `row_hash` (dedupe within a single upload run)

Access control:

- Admins can view all columns.
- Teachers should see a reduced subset by default (WCode/name/course/cycle), if we surface any UI at all to them.

### 3) Course-level filter definition

Each course may have **one** saved CRM filter that defines its roster. This filter is stored as a JSON blob (v1) and edited in Course Detail (Admin-only).

Filter features (v1):

- Multi-select equals: `Cycle`, `Course Name`, `Academic level`, `Secondary School`
- Contains: `Teacher(s)` (substring match)
- Blank / non-blank toggles for selected columns

The roster becomes read-only when the filter is enabled ("Managed by CRM filter").

### 4) Per-course roster lock

Each course with CRM filter enabled may have a **roster lock** toggle:

- When **locked**: the course is excluded from auto-reconcile on future uploads. The filter remains editable, but changes do not reconcile the roster until unlocked.
- When **unlocked**: on the next upload (or on manual filter save), the roster auto-reconciles to match the current filter.

The lock is a simple boolean (`crm_roster_locked`) on the `courses` table, not tied to any specific cycle value.

## Upload Flow

### User journey

1. Admin goes to **CRM Admin** page (e.g., `/crm`).
2. Clicks "Upload XLSX" file picker.
3. Selects the XLSX file exported from their CRM.
4. A job starts asynchronously; the UI polls for progress.
5. On completion, the UI shows:
   - Rows parsed
   - Cycles discovered
   - Courses reconciled
   - Students added / removed
6. Admin goes to **Course Detail**, configures the filter, and enables CRM management.

### Upload processing

1. Receive multipart file upload.
2. Parse XLSX using `excelize`: discover header row, extract rows per canonical schema.
3. Begin DB transaction:
   - **Delete all** existing rows from `crm_rows`.
   - **Insert** all parsed rows (with dedupe by `row_hash`).
   - **Populate/update** `crm_cycles` from distinct cycle values found in data.
4. Commit transaction.
5. **Auto-reconcile** all courses with:
   - `crm_filter_enabled = true`
   - `crm_roster_locked = false`
6. Return job result summary.

### Single-flight

- Only one upload job can run at a time.
- If a job is running, the admin UI shows "Upload already in progress".

### Failure modes

- If XLSX parsing fails (missing headers, zero rows), do not mutate `crm_rows` or rosters.
- After 3 retries (on transient DB errors): mark job failed and surface error in UI.
- Invalid file type (not XLSX): immediate rejection before job starts.

## XLSX Schema (Canonical)

The uploaded XLSX must contain the following columns (by header name, not position):

| Column Header | Required | Notes |
|---|---|---|
| `Student Id` | ✅ | Mapped to `wcode` (e.g., `W250144`) |
| `First Name` | ✅ |  |
| `Last Name` | ✅ |  |
| `Nickname` | Optional |  |
| `Secondary School` | Optional |  |
| `Academic level` | Optional |  |
| `Mobile Phone` | Optional |  |
| `Primary E-mail` | Optional |  |
| `Parent Name` | Optional |  |
| `phoneparent` | Optional |  |
| `emailparent` | Optional |  |
| `Course Name` | ✅ |  |
| `Cycle` | ✅ |  |
| `Hours` | Optional |  |
| `Teacher(s)` | Optional | Stored as `teachers_raw` |
| `Order Quote Updated Date` | Optional | Used for dedupe ordering |

**Required headers** (file rejected if missing): `Student Id`, `Course Name`, `Cycle`

The parser scans the first N rows (e.g., 20) to find the header row containing all required headers.

## Roster reconciliation semantics

For a given course:

1) Evaluate the course's saved filter against `crm_rows` (SQL).
2) Produce the set of **distinct** `wcode`s that match.
3) Upsert `students` for those WCodes:
   - `wcode` is immutable and unique
   - `full_name = first_name + " " + last_name`
   - never overwrite `notes`
4) Reconcile `course_students` to exactly match that WCode set:
   - add missing students,
   - remove students no longer matching.
5) Write audit events for adds/removes with a system actor:
   - `action`: `course_students.add` / `course_students.remove`
   - payload includes `course_id`, `student_id`, and `source: "crm_filter_sync"`

Important: reconciliation is per-course. If a student has multiple CRM rows for different courses, removing one row should only affect the app course(s) whose filter matched that row.

## UI/UX

### 1) Global Admin: CRM Upload

A simple admin page at `/crm`:

- **Upload section**: File picker button ("Upload CRM Export XLSX") + job status indicator
- **Job status**: Polling display of current/recent job progress
- **Completion summary**: Rows parsed, cycles discovered, courses reconciled, students added/removed
- **Error display**: Clear messaging if upload fails

### 2) Course Detail (Admin-only CRM Filter panel)

Inside Course Detail:

- Excel-like table of imported CRM rows (server-side pagination + filtering).
- Column filter dropdowns for the v1 feature set.
- A preview count: "This filter matches N distinct students".
- **Roster lock toggle**: "Lock roster — don't auto-update"
- **Save filter**:
  - persists JSON filter,
  - immediately reconciles this course roster (unless locked).

When filter is enabled:

- Roster UI becomes read-only
- Manual "add student by wcode" and remove buttons are hidden/disabled

When roster is locked:

- A banner shows: "Roster is locked — won't auto-update on future uploads"
- Filter panel remains editable
- Filter changes do not reconcile until unlocked

## Security & Operational Notes

- File uploads should be validated (file extension, magic bytes) before processing.
- XLSX parsing in Go: all input read into memory — set a reasonable upload size limit (e.g., 50MB).
- Admin-only access for all CRM import/upload and PII columns.
- Upload replaces all data — show a confirmation dialog before processing.
