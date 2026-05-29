# Staff Absence Management Design

**Status:** Approved for implementation by the user's supplied product design on 2026-05-27.

## Goal

Turn the admin absence list into an operational staff workflow: triage submitted absences, inspect student and sit-in context, action or override records with accountability, report on the data, and configure student-form behavior without deploying code.

## Existing System Fit

The application is a Vite/React admin UI backed by a Go modular monolith and PostgreSQL. It already provides:

- Public absence submission, student lookup, and auto sit-in resolution.
- `student_absences`, `absence_sit_ins`, root course groups, course levels, and JSON absence policies.
- A read-only admin absence table and course-level editor.
- Admin authentication, idempotent mutation infrastructure, and a general audit log.

The new staff-side contract extends these modules rather than replacing existing student submission behavior.

## Staff Interfaces

### Absence Inbox

`/absences` becomes a paginated triage inbox. It provides URL-backed filters for query, subject, status, and overlapping date range; status-first table rows; student name; compact sit-in representation; submitted time; CSV export; dashboard navigation; quick review/cancel actions; and multi-select review/export behavior.

### Absence Detail

`/absences/:id` displays student snapshot data, absence fields, sit-in sessions, workflow state, internal notes, and an append-only absence timeline. Staff can mark reviewed, actioned, reopen a reviewed record, cancel with a reason, and open a sit-in override dialog.

### Sit-In Override

An admin may choose automatic resolution, Zoom, or a manual physical course/session assignment. Every override requires a reason, persists the actor, and adds a timeline event. Manual sessions must belong to the selected course and fall within the absence dates.

### Dashboard And Awareness

`/absences/dashboard` reports monthly workflow totals and distributions by subject and reason. The admin navigation polls pending/today statistics and renders a pending badge.

### Absence Settings

`/admin/absence-settings` manages form rules stored in `app_settings.absence_policies`: maximum date range, reason requirement/categories/free-text, student-facing messages, auto-resolution wording, and maximum sessions. The public form obtains a safe form-config subset and obeys these rules.

### Course Levels

`/course-levels` preserves existing hierarchy editing while making auto sit-in policy visible, showing inline gap feedback, providing configuration verification, and supporting a bulk level-edit modal for a root group/cycle.

## Persistence Contract

Add workflow and snapshot fields to `student_absences`: `status`, `admin_notes`, `reason_category`, `student_name`, nullable student contact snapshots, review metadata, sit-in override metadata, `version`, and `updated_at`.

Add `absence_audit_log` with action, actor, details, and time. Timeline rows are append-only at database level. Existing general `audit_log` also records admin mutations for cross-feature administration history.

Contact limitation: canonical `students` currently contains `wcode`, `full_name`, and notes but not email or telephone fields. The feature exposes nullable snapshot fields and snapshots the known name; it does not manufacture unavailable contact values.

## API Contract

Admin-only:

- `GET /api/v1/absences?query=&subject_id=&status=&date_from=&date_to=&offset=&limit=`
- `GET /api/v1/absences/{id}`
- `PUT /api/v1/absences/{id}/status`
- `PUT /api/v1/absences/{id}/notes`
- `PUT /api/v1/absences/{id}/sit-in`
- `GET /api/v1/absences/{id}/sit-in-candidates?course_id=`
- `GET /api/v1/absences/{id}/timeline`
- `GET /api/v1/absences/stats`
- `GET /api/v1/absences/dashboard?month=YYYY-MM`
- `GET /api/v1/absences/export` with inbox filters
- `GET /api/v1/admin/absence-settings`
- `PUT /api/v1/admin/absence-settings`

Public:

- `GET /api/v1/absence-form-config`
- Existing `POST /api/v1/absences` additionally accepts structured reason configuration and creates submission timeline data.

All staff mutations require Admin authorization, an idempotency key, validation, and transactional audit logging.

## Workflow Rules

- New submissions start as `pending`.
- Permitted transitions are `pending -> reviewed|cancelled`, `reviewed -> actioned|pending|cancelled`, and `actioned -> reviewed` for correction.
- Cancellation requires a reason.
- Updates use `expected_version`; stale writes return a conflict.
- Notes are internal-only. Timeline/global audit records that a note changed without duplicating sensitive note body content.

## Testing Strategy

Use vertical observable-behavior slices:

- Go handler/DB integration behavior for workflow validation, paging/filter/export/stats/settings, audit events, stale updates, and override validation.
- React component behavior for inbox filters/actions/pagination, detail actions/override/notes, dashboard/settings/form configuration, nav pending badge, and course-level discoverability/bulk editing.
- Full typecheck/build/backend tests and browser-rendered validation for desktop and mobile staff paths.

## Security And Reliability

- Admin-only reads/writes protect student absence data and internal notes.
- Public form config exposes messaging/rules only, not admin metadata.
- CSV export is admin-only and generated with standard CSV escaping.
- Pagination and indexed filters bound inbox query cost.
- Timeline is append-only; idempotent staff writes prevent duplicate audit/action outcomes on retries.

