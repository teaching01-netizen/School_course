# Code Quality

This repo aims to stay a single deployable app while keeping **clear module boundaries** and **clean, testable code**.

## Quick commands

Frontend (React/Vite):

- `npm run typecheck`
- `npm run build`

Backend (Go):

- `make -C backend fmt`
- `make -C backend lint`
- `make -C backend test`

## Idempotency policy

All side-effecting `POST`/`PUT`/`PATCH`/`DELETE` endpoints (except auth + preflight) **require** an `Idempotency-Key` header. The server persists the result and replays it on retries. See `docs/idempotency.md`.

## Architectural guardrails (modular monolith)

For backend work, prefer:

- Put business rules in the owning package under `backend/internal/<module>/...`
- Route/HTTP code in `backend/internal/httpapi`
- DB wiring and invariants in `backend/internal/db` + `backend/internal/pg`
- Scheduling orchestration + explainable conflicts in `backend/internal/scheduling`

When a change touches multiple modules:

- Define a small contract surface (DTO/functions) in the owning module, not by importing internals everywhere.
- Prefer explicit “application/use-case” functions in the owning module over calling repositories directly from handlers.

## Scheduling conflict explainability

When scheduling writes are blocked (overlaps or availability windows), return an inline JSON error with stable `details`:

- `code`: `schedule_conflict` or `availability_violation`
- `message`: human readable
- `details.kind`: `room_overlap` | `teacher_overlap` | `student_overlap` | `teacher_availability` | `room_availability`
- `details.requested`: `{ start_at, end_at, course_id, room_id, teacher_id, series_id? }` (RFC3339 UTC)
- `details.conflicts[]`: list of conflicting sessions, each `{ session_id, series_id?, course_id, room_id, teacher_id, start_at, end_at }` (RFC3339 UTC)

## Linting philosophy

- Use linters to catch boundary leaks, unsafe patterns, and accidental complexity early.
- Prefer small, local refactors over large rewrites.
