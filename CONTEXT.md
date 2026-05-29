# Context

This repository currently contains UX + frontend mock implementation for a Warwick Institute scheduling/admin system.

## Product Scope (Current Understanding)

- Single-tenant: one institute per deployment.
- Frontend scope (v1): Admin-only UI (no Teacher portal in v1).
- Scheduling UI entrypoint (v1): `/schedule` is the primary screen for scheduling create/edit/cancel (with scope).
- Schedule view (v1): calendar uses day + time grid (time-axis) rather than day-only columns.
- Schedule range (v1): allow custom date range, capped to max 14 days for performance.
- Schedule interactions (v1): modal form edits only (no drag-and-drop rescheduling).
- Bulk ops (v1): no bulk operations; edits/cancels are performed one series/session at a time.
- Selectors (v1): use searchable typeahead for course + teacher; simple dropdown is acceptable for room.
- UX focus (v1 “production grade”): `Admin` creates/edits recurring schedule series (scope edits like “this occurrence” vs “this & future”, with conflict prevention).
- Availability “trust contract” (v1): UI “Available” means no conflicts under current data; hard-block on student + teacher always, hard-block on room when selected; both `pending` and `confirmed` sessions are blocking.
- Session status blocking (v1): `pending` sessions block overlaps the same as `confirmed`.
- Availability UI semantics (v1): avoid “false green”; when `Classroom` is `[NOT SET]`, show **Provisional** with checklist (Student ✅ / Teacher ✅ / Room ⏳). Only show plain **Available** when all applicable checks are ✅.
- Room assignment (v1): `room_id` is optional on one-off and series creation; missing room yields **Provisional** availability semantics.
- Backend contract change (v1): make `room_id` nullable/optional end-to-end (DB + API) to support Provisional scheduling without fake rooms.
- Room null representation (v1): `room_id` is not auto-generated; when no room is chosen, persist `room_id = NULL` (no special “TBD room” row).
- Room API encoding (v1): frontend sends `"room_id": null` when no room is selected (not omitted).
- Time granularity (v1): 5-minute increments across time inputs, availability views, and slot suggestions.
- Time input enforcement (v1): UI hard-enforces 5-minute step for start/end times (one-off + series).
- Conflict explainability (v1): when a slot/session is Blocked/Provisional, show the exact conflicting session(s) (C-ID, date, time) and provide click-through to the relevant course/session.
- Error contract (v1): scheduling write failures return a stable machine-readable error shape (e.g., `code` + structured `conflicts`) that UI renders (not generic toast-only).
- Conflict UX (v1): error panel shows both attempted session details and conflicting sessions.
- Availability windows (v1): weekly recurring availability rules + date exceptions (e.g., holidays, teacher leave).
- Backend architecture: single deployable modular monolith (Go).
- Database: PostgreSQL.
- Auth: app-managed local usernames/passwords stored in PostgreSQL.
- Deployment target: Railway (managed containers + managed PostgreSQL).
- Migrations (prod): run via Railway Pre-Deploy Command.
- Background jobs (v1): Railway Cron Jobs (not an always-on worker).
- Idempotency policy: all side-effecting `POST`/`PUT`/`PATCH`/`DELETE` HTTP endpoints (except auth + preflight) **require** `Idempotency-Key` header. Replays are safe. See `docs/idempotency.md`.
- Authorization: simple RBAC with `Admin` and `Teacher` roles (Admin can do everything).
- User lifecycle (v1): admin provisions all users; no self-signup.
- Authorization enforcement (v1): centralized in Go API (application-layer), not DB RLS.
- Frontend delivery (prod): serve built SPA from the Go API service (same origin).
- API style: REST/JSON.
- Time policy: store timestamps in UTC in DB; store institute timezone (`Asia/Bangkok`) for display and scheduling semantics.
- Time sync (v1): backend exposes institute timezone + server-now for frontend to compute “now” consistently (immutability, pickers, scope prompts).
- Scheduling (v1): recurring sessions are required.
- Scheduling create policy (v1): Admin can create both one-off sessions and recurring series (series still supported and recommended).
- Recurrence edits: support explicit scope options like “this occurrence” and “this & future” / “entire series”.
- Recurrence scope options (v1): UI supports edits/cancellations for **This occurrence**, **This & future**, and **Entire series**.
- Session edit fields (v1): Admin can edit course/teacher/room/time (subject to scope: occurrence/future/entire series).
- Course edit roster semantics (v1): if course changes, session roster switches to the new course roster; attendance overrides are cleared/filtered when no longer applicable.
- Recurrence persistence: store a parent series record + materialized occurrence session rows linked to the series.
- Recurrence rules (v1): weekly-only (selected weekdays + fixed time window) + exceptions; support “this occurrence” / “this & future”.
- Recurrence editing model (v1): “this & future” edits split the series at the target occurrence; preserve existing per-occurrence exceptions.
- Scope boundary (v1): “this & future” includes the selected occurrence (effective from that occurrence’s start).
- Edit immutability (v1): past sessions are immutable; “this & future” / “entire series” edits apply only from the effective occurrence forward.
- Past definition (v1): a session is “past” when `end_at < now` (ongoing sessions are not past yet).
- Recurrence cancellation (v1): support “cancel this occurrence” and “cancel this & future” via soft delete.
- Cancellation immutability (v1): “cancel entire series” cancels future sessions only (past sessions remain for audit/history).
- Recurrence bounds (v1): every series must have an explicit end (`end_date` or occurrence `count`), no “repeat forever”.
- Soft delete semantics: soft-deleted sessions stop blocking overlaps (excluded from overlap enforcement).
- Deletion policy (v1): soft delete sessions and courses (at minimum).
- Scheduling invariants (v1): enforce room exclusivity, teacher exclusivity, and student exclusivity; no overrides.
- Room exclusivity rule (v1): enforce room overlap only when `room_id` is set; `NULL` room never blocks room conflicts. Teacher/student overlaps always block.
- Availability enforcement (v1): teacher + room availability windows are hard blocks at save time (cannot schedule outside availability).
- Availability enforcement (v1): preflight and save treat availability windows/exceptions as hard blocks (not warnings).
- Scheduling enforcement (v1): application validates, but DB is final gate via transactions + locking.
- Student conflicts: allow saving sessions with explicit per-session `included` and `excluded` student lists.
- Overlap enforcement (DB final gate): use PostgreSQL range exclusion constraints (`EXCLUDE USING gist`) plus application validation.
- Explainability vs correctness: `scheduling` may preflight for user-facing conflict details, but correctness is guaranteed only by DB final gates; on DB rejection (race), `scheduling` re-hydrates `ConflictDetails` and returns the same stable error shape.
- Student overlap DB gate: enforce via per-student range rows + exclusion constraint; keep in sync with triggers on session/attendance changes.
- Audit logging (v1): record scheduling changes (session + series edits, include/exclude students) and user/password reset actions.
- Concurrency (v1): require optimistic concurrency for series/session edits (e.g., `version` or `updated_at` precondition; stale-edit error on mismatch).
- Stale edit response (v1): return `409` `code:"stale_edit"` and include the current server copy in `details`.
- Precondition transport (v1): pass optimistic concurrency token in JSON body (e.g., `expected_version` / `expected_updated_at`).
- Concurrency token (v1): use integer `version` (increment on every write) rather than timestamp.
- Concurrency coverage (v1): enforce `version` precondition on both series edits and session/occurrence edits (including one-offs).
- Audit log integrity: DB-enforced append-only; ideally written via a separate insert-only DB role.
- Domains (v1): Railway-provided `*.up.railway.app` domain (no custom domain).
- Auth sessions: server-side sessions with `HttpOnly; Secure; SameSite=Strict` cookie (prefer `__Host-` prefix; no `Domain` attribute).
- Session lifecycle (v1): idle timeout ~8h; absolute timeout ~7d; force logout all sessions on password reset.
- Auth abuse protection (v1): soft throttling with separate per-username + per-IP buckets; avoid hard account lockouts by default.
- Password recovery (v1): admin-only resets; no teacher self-service “forgot password”.
- Data access (v1): SQL-first (migrations + hand-written SQL) with typed code generation via `sqlc`.
- Password hashing: Argon2id + server-side pepper stored separately (Railway secrets).
- Module ownership (backend): `scheduling` owns all scheduling writes (create/split series; create/edit/delete sessions) and the stable conflict/availability error contract; `series` is an implementation detail behind `scheduling` (no direct `httpapi -> series` usage).
- Public scheduling input types: `httpapi` parses scheduling inputs into `scheduling` types (e.g., `scheduling.LocalDate`, `scheduling.Clock`); `series` consumes these internally.
- Institute timezone source of truth (v1): `scheduling` receives `InstituteTZ` from config (fixed per deploy), persists it on series for audit/debug, and does not dynamically discover it from DB at runtime.
- Preflight policy: `scheduling` preflight is optional and never a correctness gate; every scheduling write path must also support a DB-rejection fallback that returns the same `ConflictDetails` shape.
- Frontend preflight (v1): UI requires availability/conflict preflight for UX before enabling save, but still handles DB-race rejection via the same stable error shape.
- Backend preflight (v1): add a dedicated scheduling preflight endpoint returning `Available|Provisional|Blocked` + structured conflicts (and attempted session details).
- Preflight contract (v1): preflight response uses the same `ConflictDetails` schema as scheduling write failures.
- Preflight input (v1): request includes `course_id`, `teacher_id`, `start_at`, `end_at`, optional `room_id`, optional `session_id` (for edits), and `included_student_ids` / `excluded_student_ids` for attendance override semantics.

## Open Questions

- TBD

## UX Decisions (Resolved)

- `New Course` form: `Expire Day` is not needed and should be removed from the create flow.
- Course detail schedule (v1): Add a single CTA on `/courses/:id` schedule panel that opens one modal with two tabs — **Recurring series** (default) and **One-off session** (secondary). Both are pre-filled with `course_id` and the save action is disabled until the corresponding scheduling preflight passes.
- Preflight gating (v1): allow saves when preflight returns **Available** or **Provisional** (Provisional is valid when `room_id` is not set).
