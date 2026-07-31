# Staff Absence Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the complete staff absence triage, action, reporting, settings, and course-level refinement workflow on top of the existing student absence submission system.

**Architecture:** Extend PostgreSQL with workflow/audit/snapshot state and place querying and mutations behind focused custom DB methods. Add authenticated Go REST handlers using existing adapter/idempotency conventions. Implement React route surfaces and small shared absence UI helpers while preserving the established admin styling and existing public flow.

**Tech Stack:** PostgreSQL/goose, Go `net/http`/pgx/sqlc-style custom queries, React 19/Vite/TypeScript/Tailwind, Vitest/Testing Library, Go testing.

---

## File Map

- `backend/db/migrations/00021_absence_management.sql`: workflow, snapshot, override, audit timeline, and indexes.
- `backend/internal/db/absence_management_custom.go`: bounded query and mutation module for staff absence interfaces.
- `backend/internal/httpapi/absenceshttp/routes.go`: new public/admin HTTP surface and validation.
- `backend/internal/httpapi/absenceshttp/routes_test.go`: public endpoint-level validation/security behaviors.
- `src/types/index.ts` and `src/api/client.ts`: absence contracts and CSV download helper.
- `src/pages/Absences.tsx`: inbox.
- `src/pages/AbsenceDetail.tsx`: detail/action/override workflow.
- `src/pages/AbsenceDashboard.tsx`: reporting dashboard.
- `src/pages/AbsenceSettings.tsx`: form configuration.
- `src/pages/AbsenceForm.tsx`: settings-driven public form.
- `src/pages/CourseLevels.tsx`: policy visibility, validation, bulk levels.
- `src/components/Layout.tsx` and `src/App.tsx`: navigation badge/routes.
- `src/pages/__tests__/*.test.tsx`: observable frontend workflow behavior.

### Task 1: Workflow Persistence And Admin Query Contract

- [ ] Write a failing Go route test proving invalid staff status changes and invalid settings are rejected through the HTTP endpoint.
- [ ] Run `cd backend && go test ./internal/httpapi/absenceshttp` and confirm RED because the routes do not exist.
- [ ] Add migration `00021_absence_management.sql` and focused DB methods for list/detail/timeline/stats/dashboard/export inputs and transactional mutations.
- [ ] Add authenticated routes, request validation, version checks, and timeline/global audit writes.
- [ ] Run the Go route tests and relevant package tests until GREEN.

### Task 2: Student Form Settings Contract

- [ ] Add a failing React test proving the public form reads config and applies configured date/reason rules.
- [ ] Run `npm test -- src/pages/__tests__/AbsenceForm.test.tsx` and confirm RED.
- [ ] Implement public/admin settings endpoints and update `AbsenceForm.tsx` to consume safe configuration.
- [ ] Run the focused frontend test until GREEN.

### Task 3: Inbox Triage

- [ ] Add a failing React test for server-side filtered inbox rendering and a review mutation.
- [ ] Run the focused Vitest file and confirm RED.
- [ ] Implement `Absences.tsx`, types, CSV download, query-string filters, pagination, selection, status actions, and route links.
- [ ] Run the focused tests until GREEN.

### Task 4: Detail, Override, Timeline, And Dashboard

- [ ] Add failing React tests for detail status/notes/override interaction and dashboard statistics rendering.
- [ ] Run focused tests and confirm RED.
- [ ] Implement detail, override modal, dashboard route, and app routing.
- [ ] Run focused tests until GREEN.

### Task 5: Awareness And Configuration Usability

- [ ] Extend frontend tests for the nav badge and visible course-level policy/bulk/verify controls.
- [ ] Run focused tests and confirm RED for new expectations.
- [ ] Implement navigation polling, settings route entry, and course-level refinements.
- [ ] Run focused tests until GREEN.

### Task 6: Verification And Review

- [ ] Run `npm test`, `npm run typecheck`, and `npm run build`.
- [ ] Run `cd backend && go test ./...` and `cd backend && go build ./cmd/server`.
- [ ] Use the Browser plugin for the staff flow at desktop and mobile sizes if a runnable authenticated environment is available; otherwise report the concrete blocker.
- [ ] Review admin authorization, validation, export escaping, pagination/query cost, transactional audit behavior, and stale-edit handling.
- [ ] Report command evidence and any intentional limitations, especially nullable contact details without a canonical source.

