# Full-Stack Search Feature Audit — 2026-08-19

Audited against the *Rigorous Full-Stack Search Feature Engineer* standard.
Scope: every search / autocomplete / typeahead / filter surface in `src/` (React), `backend/` (Go), and `backend/db/migrations/` (PostgreSQL).

## Implementation status (same day)

| Finding | Fix | Status |
|---|---|---|
| P1-1 unbounded/uncapped results | students limit ≤200; courses legacy cap 1000; cross-study LIMIT/OFFSET + total count | ✅ Done |
| P1-2 no length bounds on search strings | `httpadapter.SearchQuery` (trim + 200-rune cap) applied to all 5 search endpoints + admin users | ✅ Done |
| P1-3 cross-study list races/error conflate | debounce 300 ms, AbortController, error state, min length 2, pagination UI | ✅ Done |
| P1-4 absences per-keystroke requests | 300 ms debounce on the query filter (URL stays immediate) | ✅ Done |
| P1-5 zero index support | migration `00095_search_indexes.sql`: pg_trgm + 11 GIN indexes; verified Bitmap Index Scan at 205k rows | ✅ Done |
| P2-1 nondeterministic ordering | `i.id DESC` / `a.id DESC` tie-breakers | ✅ Done |
| P2-2 Teachers no-op Search button | removed; fetch-error state distinct from empty | ✅ Done |
| P2-4 Users page fake search | server-side `q` on `/api/v1/admin/users` + debounced load + error state | ✅ Done |
| P3-1 MultiTeacherSelect a11y | wired `aria-controls`/listbox id + input label | ✅ Done |
| Verification | backend `go build`/`go vet` clean; new unit + integration + component tests pass; full vitest suite run | ✅ Done (scheduling-package flakes in parallel full-suite runs are pre-existing shared-DB contention; pass in isolation) |
| P2-3 SlotFinder/useLookups full-table fetches; P3-3 observability; §6 relevance ranking | deliberately out of scope this pass — see §7 follow-ups | ⏸ Follow-up |

---

## 1. Architecture Map

```
SearchInput (src/components/ui/SearchInput.tsx)
  → per-page state (URL searchParams or useState)
  → useApiQuery / useOperationalQuery (TanStack Query)  OR  raw apiJson fetch
  → GET endpoints:
       /api/v1/courses           courseshttp/routes.go:82       handleCoursesList
       /api/v1/students          studentshttp/routes.go:32      handleStudentsList
       /api/v1/absences          absenceshttp/management_routes.go:453 handleAbsenceInbox
       /api/v1/operations/schedule-impact  sessionchangehttp/workqueue_routes.go:13
       /api/v1/cross-study/assignments     crmhttp/crossstudy.go:55
  → SQL:
       courses_overview_custom.go:94-116  (hand-written, @courses/@users/@subjects/@course_teachers)
       db/queries/students.sql:26-35      (sqlc, @students)
       absence_management_custom.go:88-135 (hand-written, @student_absences + LATERAL aggregates)
       session_change_queue_custom.go:103-155 (hand-written, @absence_schedule_issues + joins)
       crmimport/crossstudy/store.go:834-887 (hand-written, @crm_cross_study_assignments + joins)
  → PostgreSQL (no search indexes; sequential scans)
```

Client-side "filter" surfaces (no API): Teachers, Subjects, Classrooms, Users, OperationsCalendar,
ActiveCoursesPanel/Section, CourseLevels, TypeaheadSelect, RoomAssignDropdown, MultiTeacherSelect,
SubjectPicker, SlotFinder (entire tables into `<select>`).

---

## 2. Current Search Semantics (what it actually does)

| Endpoint | Matches | Ranking | Bounds | Determinism |
|---|---|---|---|---|
| `GET /api/v1/courses?q=` | `course_no,id,code,name,subject code/name,teacher name/username,teacher membership` — all `ILIKE '%q%'` | `course_no DESC` | **Unbounded when `limit` param absent** (legacy path) | Yes (`course_no` unique) |
| `GET /api/v1/students?q=` | `wcode, full_name` `ILIKE '%q%'` | `wcode ASC` | `limit` default 50, **no upper cap** | Yes (`wcode` unique) |
| `GET /api/v1/absences?query=` | `wcode, student name, nickname` `ILIKE '%q%'` + subject/status/date/impact filters | impact severity then `created_at DESC, id DESC` | `limit` 1–100 | Yes |
| `GET /api/v1/operations/schedule-impact?q=` | `concat_ws(wcode, name, code, name)` `ILIKE '%q%'` | `severity, status, updated_at DESC` | `limit` ≤100 | **No tie-breaker** on `updated_at` |
| `GET /api/v1/cross-study/assignments?q=` | `wcode, student full_name` `ILIKE '%q%'` | `updated_at DESC` | **No LIMIT at all** | **No tie-breaker** |

- Empty/whitespace `q` is allowed everywhere and degrades to an unfiltered listing (intentional per UI, but combined with unbounded limits this is a full-table scan + full-payload response).
- Every predicate is leading-wildcard `ILIKE`; **zero indexes can serve any of them** (no pg_trgm, no GIN, no tsvector anywhere in the schema).
- Teachers are rows in `users`; there is no teacher search endpoint — the Teachers page loads `GET /api/v1/users?role=Teacher` in full and filters client-side (its Search button is a no-op).
- All search endpoints are authenticated (`MustUser`/`MustAdmin`); the only tenant/scope dimension is the single-institution model, so no cross-tenant leak exists — but the public absence self-service lookup is the only rate-limited search path.

---

## 3. Critical Findings

### P1-1 — Unbounded/uncapped search results
- **Layer:** Backend · **File:** `backend/internal/httpapi/studentshttp/routes.go:38-42` — `limit` parsed with no upper bound (`?limit=999999999` accepted).
- **File:** `backend/internal/httpapi/courseshttp/routes.go:105-126` — absence of the `limit` query param selects a "legacy bare array" path where `LIMIT NULLIF($5,0)` becomes `LIMIT NULL` → the entire course table, with the full `course_students` aggregation, in one response.
- **File:** `backend/internal/crmimport/crossstudy/store.go:834-887` — `ListAssignmentsWithCourseInfo` has **no LIMIT/OFFSET**; the handler responds `total: len(items)`.
- **Observed behavior:** one unbounded dynamic query per request; JSON payloads scale with table size; connection pool (max 10, `internal/pg/pg.go:39`) can be saturated by a handful of requests.
- **Root cause:** limits were treated as optional conveniences, not invariants.
- **User impact:** slow pages, huge payloads, memory pressure on admins.
- **Evidence:** source inspection above; `LIMIT NULLIF($5,0)` in `courses_overview_custom.go:172`.
- **Fix:** enforce `1..200` always in the handlers; make the Go SQL always apply `LIMIT $n OFFSET $m`; add `LIMIT 100 OFFSET $n` to the cross-study list.

### P1-2 — No length bound on any search string
- **Layer:** Backend · **Files:** `studentshttp/routes.go:48` (`q` untrimmed, unbounded), `crmhttp/crossstudy.go:59-60` (untrimmed), `sessionchangehttp/workqueue_routes.go:34`, `absenceshttp/management_routes.go:212` (trimmed only).
- **Observed behavior:** a multi-megabyte `q` is passed straight into `ILIKE '%…%'`; pattern cost grows with input size.
- **Fix:** trim + cap at 200 chars on every search parameter; reject or truncate beyond that.

### P1-3 — Frontend race conditions in cross-study assignment list
- **Layer:** Frontend · **File:** `src/components/crm/CrossStudyAssignmentList.tsx:25-46,85-91`.
- **Observed behavior:** every keystroke fires a request (no debounce); no AbortController/request-id, so a slow response for an older query can overwrite newer results; `catch` sets `assignments = []`, making **network failures indistinguishable from zero results** (empty-state copy tells the user to create the first assignment).
- **Root cause:** raw useEffect + fetch without the lifecycle discipline used elsewhere in the app (cf. `AbsenceForm`'s `lookupRequestId` guard, `usePreflight`'s `controllerRef`).
- **User impact:** stale rows, misleading "no assignments yet" on failure.
- **Fix:** 300 ms debounce, AbortController per request, `error !== null` state rendered distinctly, minimum query length 2.

### P1-4 — Absences page: one server request per keystroke
- **Layer:** Frontend · **File:** `src/pages/Absences.tsx:271-277,582` — `SearchInput onChange → updateFilter("query") → setSearchParams → useOperationalQuery` re-fetches every keystroke; first keystroke also resets `offset` and sends a full-list query (no min length).
- **Impact:** request storm at p95 latency; wasteful `count(*) OVER()` window + LATERAL aggregates per keystroke.
- **Fix:** debounce only the `query` filter into the URL (300 ms), like the Courses page.

### P1-5 — Zero index support for all search predicates
- **Layer:** PostgreSQL · **Evidence:** migrations `00001`–`00094` contain no `pg_trgm`, no GIN, no tsvector; all five search endpoints use leading-wildcard `ILIKE` which cannot use the existing B-tree indexes (verified per-index in §4).
- **Impact:** every search is a sequential scan over `students`, `courses`+joins, `student_absences`+LATERAL aggregates, `absence_schedule_issues`, `crm_cross_study_assignments`. Nonlinear degradation as the legacy-sync/CRM imports grow the tables.
- **Fix:** migration enabling `pg_trgm` + GIN trigram indexes on the searched columns; verify with `EXPLAIN ANALYZE` on production-like data.

### P2-1 — Nondeterministic ordering
- **Files:** `session_change_queue_custom.go:126-128` (`ORDER BY severity, status, updated_at DESC` — equal timestamps tie), `crossstudy/store.go:860` (`ORDER BY a.updated_at DESC`).
- **Impact:** indistinguishable pagination duplicates/skips; UI instability.
- **Fix:** append `, id DESC` (or `issue id`) to both.

### P2-2 — No-op Search button on Teachers page
- **File:** `src/pages/Teachers.tsx:62` — `<Button onClick={() => {}}>Search</Button>`; filtering is reactive per keystroke; the button does nothing and misleads.
- **Fix:** remove the button (reactive client-side filtering is the actual behavior) and give an empty-vs-error distinction.

### P2-3 — "Fake search" surfaces (client-side filter of a full fetch)
- **Files:** `src/pages/Teachers.tsx`, `src/pages/Subjects.tsx`, `src/pages/Classrooms.tsx`, `src/pages/Users.tsx`, `src/pages/operations/ActiveCoursesSection.tsx`, `src/components/ActiveCoursesPanel.tsx`, `src/pages/SlotFinder.tsx` (entire `students` + `courses` tables into `<select>`s), `src/features/scheduling/hooks/useLookups.ts` (full lists feed TypeaheadSelect).
- **Impact:** payloads scale with the full table; `Users` page cannot find users whose rows were never loaded; SlotFinder is unusable beyond a few thousand rows.
- **Fix (this pass):** keep client-side filtering for reference-data pages (subjects, rooms — small tables), but for `Users` and `Teachers` the honest fix is server-side `q`; Teachers is bounded by role and small; Users gets a server-side `q` param on `/api/v1/admin/users` and the page switches to URL-driven query.

### P2-4 — Error states conflated with empty/loading
- **Files:** `src/components/crm/CrossStudyAssignmentList.tsx:36-38`, `src/components/absences/OverrideSitInModal.tsx:84,98` (catch silently empties), `src/pages/Users.tsx:71-72` (toast only, list silently stays), `src/pages/Subjects.tsx` / `Classrooms.tsx` (single state).
- **Fix:** distinct `error` state with retry; never render "no results" copy for failures.

### P2-5 — AbortController gaps in lookups
- **Files:** `src/components/absences/OverrideSitInModal.tsx:84-98` (rapid course switching races — stale candidates overwrite new), `components/absences/KanbanView.tsx` (column fetches with no cancellation), `useEditSession` override fetch.
- **Fix:** add per-request abort/request-id in OverrideSitInModal; KanbanView is remount-keyed already (bounded risk).

### P3-1 — A11y gaps in search inputs
- **File:** `src/components/MultiTeacherSelect.tsx` — `combobox` role without a wired listbox; page-level search inputs (Teachers/Subjects/Classrooms/Users) are plain inputs without combobox semantics. TypeaheadSelect and RoomAssignDropdown are the good examples (full ARIA combobox + keyboard).
- **Fix:** wire `id`/`aria-controls` in MultiTeacherSelect; keep plain inputs (they are page filters, not autocompletes) but ensure labels are explicit.

### P3-2 — Min query length / empty-query semantics
- No search endpoint enforces a minimum length; first keystroke in Absences/CrossStudyAssignmentList fires a full-list query. Fix via `enabled:`-style gating in the list components (2 chars) plus keep backend authoritative (backend returns full list for empty `q` by design — that's the listing mode; the frontend simply shouldn't send `q` until 2 chars).

### P3-3 — Observability
- No request-logging middleware, metrics, or per-strategy tags; `slog` error logging only. Search latency/zero-result/error rates are not observable.
- **Fix (light):** this pass adds validation + bounds; full metrics instrumentation is tracked separately (out of scope for this pass).

---

## 4. PostgreSQL Analysis

### The five search predicates (all unindexed)

1. `courses_overview_custom.go:94-116` — `c.course_no::text / c.id::text / c.code / c.name / s.code / s.name / u.full_name / u.username ILIKE '%q%'` + EXISTS over `course_teachers`. Indexes that exist (`courses_course_no_uniq`, `code UNIQUE`, `idx_courses_*`) are B-tree equality; casts (`::text`) and leading wildcards defeat them all. Plan: Seq Scan on `courses` (+ joins, + full `course_students` aggregation).
2. `db/queries/students.sql:26-35` — `wcode / full_name ILIKE '%q%'`. `idx_students_wcode_lower_unique` is a functional btree for equality only.
3. `absence_management_custom.go:123` — `sa.wcode / student_name / nickname ILIKE '%q%'`. `idx_absences_wcode` is B-tree; `student_name`/`nickname` have no index at all. The `count(*) OVER()` window and LATERAL aggregates run for every filtered page.
4. `session_change_queue_custom.go:125` — `concat_ws(...) ILIKE '%q%'` over 4 columns: expression is not sargable by any index.
5. `crossstudy/store.go:845-846` — `a.wcode / s.full_name ILIKE '%q%'`; join uses `LOWER(s.wcode) = LOWER(BTRIM(a.wcode))` (function on column, index-unfriendly). No LIMIT.

### Recommended migration (this pass)

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- students: name + wcode search (students list, absence search via join)
CREATE INDEX IF NOT EXISTS idx_students_full_name_trgm ON students USING gin (full_name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_students_nickname_trgm ON students USING gin (COALESCE(nickname, '') gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_students_wcode_trgm ON students USING gin (wcode gin_trgm_ops);

-- courses + subjects + users: course list search
CREATE INDEX IF NOT EXISTS idx_courses_code_trgm ON courses USING gin (code gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_courses_name_trgm ON courses USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_subjects_code_trgm ON subjects USING gin (code gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_subjects_name_trgm ON subjects USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_users_full_name_trgm ON users USING gin (full_name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_users_username_trgm ON users USING gin (username gin_trgm_ops);

-- absences: inbox name search
CREATE INDEX IF NOT EXISTS idx_absences_student_name_trgm ON student_absences USING gin (COALESCE(student_name, '') gin_trgm_ops);

-- cross-study: assignments search
CREATE INDEX IF NOT EXISTS idx_cross_study_wcode_trgm ON crm_cross_study_assignments USING gin (wcode gin_trgm_ops);
```

Then verify each query with `EXPLAIN (ANALYZE, BUFFERS)` on production-like data and check `pg_trgm.similarity_threshold` behavior for typo tolerance. Candidate retrieval stays `ILIKE '%q%'` (pg_trgm makes it index-backed); whether to add `similarity()` ranking is a product decision, not this pass.

### Statement timeouts
The pool holds 10 connections (`internal/pg/pg.go:39`) and the server `WriteTimeout` is 30 s. Recommend `statement_timeout` (e.g. 5 s) on the search pool in a follow-up; bounds + indexes in this pass reduce the practical risk.

---

## 5. Frontend Interaction Analysis

| Surface | Debounce | Cancel/race | Loading | Empty | Error | Min len | Keyboard | ARIA |
|---|---|---|---|---|---|---|---|---|
| Students page | — (submit-gated) | query-keyed | skeleton | ✅ | toast | — | Enter | label |
| **Courses page** ✅ reference | **300 ms** | query-keyed | skeleton | ✅ | ✅ | — | — | label |
| **Absences page** ❌ | **none (per keystroke)** | query-keyed | spinner | ✅ | toast | none | — | label |
| SessionChanges ✅ | 350 ms | query-keyed | ✅ | ✅ | ✅ | — | `/`,`j`,`k` | label |
| **CrossStudyAssignmentList** ❌ | **none (per keystroke)** | **none — race** | ✅ | ❌ conflated | ❌ | none | — | label |
| CrossStudyStudentSearch ✅ | submit-gated | AbortController | ✅ | ✅ | ✅ | button-disabled | Enter | label |
| AbsenceForm/StaffCreate ✅ | submit-gated | AbortController + request-id | ✅ | ✅ | ✅ | format-validated | Enter | alert |
| TypeaheadSelect ✅ | n/a (client) | n/a | n/a | ✅ | n/a | — | ✅ full | ✅ full combobox |
| MultiTeacherSelect ❌ | n/a | n/a | n/a | ✅ | n/a | — | ✅ | combobox w/o listbox wiring |
| RoomAssignDropdown ✅ | n/a | n/a | n/a | ✅ | n/a | — | ✅ | ✅ |
| Teachers/Subjects/Classrooms/Users | n/a (client filter) | — | ✅ | ✅ | ❌ conflated | — | — | plain input |
| KanbanView | realtime 500 ms only | none (remount-keyed) | ✅ | ✅ | ❌ | — | — | — |
| OverrideSitInModal ❌ | — | **none — race** | ✅ | ✅ | ❌ catch→[] | — | native select | |
| SlotFinder ❌ | — | — | ✅ | — | — | — | — | 2 unsearchable `<select>`s of full tables |

**Stale-response races:** list fetches always have a response-ordering hazard; only CrossStudyAssignmentList and OverrideSitInModal currently exhibit it in practice (raw state set from unguarded async).

---

## 6. Ranking Analysis

- **Retrieval:** pure substring (`ILIKE '%q%'`), all fields weighted equally, no field weighting or fuzzy tiering. This is acceptable for a small internal institute but will degrade as tables grow (see §4).
- **Scoring:** none — ordering is domain-key ordering (`course_no`, `wcode`, timestamps), not relevance. "Exact match first" does not exist (e.g. searching a full course name does not rank that course above prefix-only matches of others — course_no ordering dominates).
- **Tie breakers:** missing in schedule-impact and cross-study lists (P2-1).
- **Product decision for follow-up:** pg_trgm `similarity()` hybrid scoring, prefix boosts, and field weights would require a small relevance-ranking change with an evaluation dataset; out of scope for the correctness pass but recorded here.

---

## 7. Implementation Plan (this pass, in priority order)

1. **Backend bounds/validation** (P1-1, P1-2): studentshttp, courseshttp (eliminate unbounded legacy path), cross-study (LIMIT + trim + cap), workqueue (`q` cap), absences `parseFilter` (`query` cap).
2. **Deterministic ordering** (P2-1): `id DESC` tie-breakers in schedule-impact and cross-study SQL.
3. **Migration `00095_search_indexes.sql`** (P1-5): pg_trgm + GIN indexes listed in §4.
4. **Frontend race/UX fixes** (P1-3, P1-4, P2-2, P2-4): CrossStudyAssignmentList (debounce+abort+error state+min length), Absences page (debounced query), Teachers (remove no-op button, error state), Users page (server-side `q` via `/api/v1/admin/users?q=` + URL state).
5. **A11y** (P3-1): wire MultiTeacherSelect listbox.
6. **Tests** (§8) and verification (`go build`, `go test`, `vitest`, typecheck).

## 8. Tests

- Go: unit tests for the new param parsing (extract `parseSearchQuery`/`boundLimit` helpers where testable), cross-study store pagination; existing integration harnesses where available (`*_integration_test.go` uses real PG).
- Frontend: Vitest for CrossStudyAssignmentList — slow-response race (request A resolves after B; B stays rendered), error state distinct from empty, debounce coalescing; Absences page debounce (query changes once after typing); Teachers button removed.
- Migration: `migrations_static_test.go` style check (extension + index names present), plus `EXPLAIN` verified manually on dev DB if available.

## 9. Acceptance Criteria

- [ ] Every search endpoint enforces `limit ∈ [1,200]` (or documented cap) and `q` ≤ 200 chars, trimmed.
- [ ] No code path executes `LIMIT NULL` or an unbounded result loop for search lists.
- [ ] Cross-study and schedule-impact lists order by a deterministic tie-breaker.
- [ ] `pg_trgm` extension + GIN trigram indexes exist for the searched columns; `EXPLAIN` shows Bitmap Index Scan for representative queries on production-like data.
- [ ] CrossStudyAssignmentList: no request per keystroke (debounced ≥250 ms), stale responses cannot overwrite newer ones, network errors render an error state (not "no assignments yet").
- [ ] Absences page fires at most one request per 300 ms of typing; first keystroke does not trigger a full-list query.
- [ ] Teachers page has no dead Search button; Users page search reaches the server.
- [ ] MultiTeacherSelect has a wired listbox for its combobox.
- [ ] Regression tests added for each fix above; `go build ./...`, `go test ./...`, `vitest run` pass.