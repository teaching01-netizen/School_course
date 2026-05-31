# Plan: Multi-Class, Multi-Day Absence Request Form

## Context

The current student absence form (`/absence`) is a 4-step wizard that only supports **one subject + one date range per submission**. Students often need leave for multiple classes across multiple days (e.g., "I'm sick Mon-Wed, missing MATH on Mon/Wed and PHYS on Tue/Wed"). Currently they must submit N separate forms — tedious and error-prone.

**Goal:** Redesign the form so a student can request absence for **multiple subjects × multiple dates** in a single flow. The UX approach is **Quick-Select with Smart Defaults** — the system auto-detects affected sessions from a date range and pre-selects all of them, so the most common case (sick for a contiguous stretch) requires only ~4 taps.

## Key Decisions

- **One new backend endpoint** (`GET /api/v1/absences/sessions-in-range`) to discover sessions across all enrolled subjects in a date range. The existing `POST /api/v1/absences` endpoint remains unchanged — each call creates one absence record (1 course + date range). The frontend fires parallel POSTs on submit. This is safe because the endpoint is idempotent-exempt (see `isIdempotencyExempt` in `src/api/client.ts:108`).
- **No DB schema changes.** Each absence record remains 1 course + 1 date range. The "multi-class" UX is purely a frontend batching concern.
- **Sit-in resolution is per-subject.** Each subject may have different sit-in options. The form resolves sit-in per selected subject after the user confirms which subjects they're absent from.
- **Shared reason.** One reason category + optional free text applies to the entire request (all selected subjects).
- **State management:** Use `useReducer` instead of 16+ `useState` calls. Multi-subject state (per-subject selection, sit-in results, session IDs) requires a structured state machine to prevent impossible state combinations.
- **Duplicate prevention:** The sessions-in-range endpoint flags sessions already covered by existing absences. The frontend disables selection for flagged sessions.
- **Types centralized** in `src/types/index.ts`, not co-located in `AbsenceForm.tsx`.

## Implementation Phases

---

### Phase 1: Backend — Session Discovery Endpoint

#### 1a. SQL Queries

**File:** `backend/db/queries/absences.sql`

Add two queries:

```sql
-- name: SessionsByStudentInRange :many
SELECT
  sess.id,
  sess.start_at,
  sess.end_at,
  c.id AS course_id,
  c.code AS course_code,
  c.name AS course_name,
  sub.id AS subject_id,
  sub.code AS subject_code,
  sub.name AS subject_name
FROM sessions sess
JOIN courses c ON c.id = sess.course_id
JOIN subjects sub ON sub.id = c.subject_id
JOIN course_students cs ON cs.course_id = c.id
JOIN students st ON st.id = cs.student_id
WHERE st.wcode = $1
  AND sess.start_at >= $2
  AND sess.start_at < ($3::date + interval '1 day')
  AND sess.deleted_at IS NULL
ORDER BY sub.code, sess.start_at;
```

```sql
-- name: AbsenceOverlappingSessions :many
SELECT DISTINCT sess.id AS session_id
FROM sessions sess
JOIN student_absences sa ON sa.course_id = sess.course_id
WHERE sa.wcode = $1
  AND sa.deleted_at IS NULL
  AND sess.start_at >= sa.date_from
  AND sess.start_at < (sa.date_to + interval '1 day')
  AND sess.start_at >= $2
  AND sess.start_at < ($3::date + interval '1 day');
```

#### 1b. HTTP Handler

**File:** `backend/internal/httpapi/absenceshttp/routes.go`

Add route registration:
```go
mux.HandleFunc("GET /api/v1/absences/sessions-in-range", s.handleSessionsInRange)
```

Handler logic (`handleSessionsInRange`):
1. Parse query params: `wcode`, `date_from`, `date_to`
2. Validate date range against `settings.Form.MaxDateRangeDays`
3. Call `SessionsByStudentInRange` to get all sessions
4. Call `AbsenceOverlappingSessions` to find already-absent sessions
5. Group results by subject, mark each session as `already_absent: true/false`
6. Return JSON:
```json
{
  "subjects": [
    {
      "subject_id": "...",
      "subject_code": "MATH",
      "subject_name": "Mathematics",
      "course_id": "...",
      "course_code": "MATH201",
      "sessions": [
        {
          "id": "...",
          "start_at": "2026-06-01T09:00:00Z",
          "end_at": "2026-06-01T10:30:00Z",
          "date": "2026-06-01",
          "already_absent": false
        }
      ]
    }
  ]
}
```

**Files to modify:**
- `backend/internal/httpapi/absenceshttp/routes.go` — add route + handler
- `backend/db/queries/absences.sql` — add 2 queries

---

### Phase 2: Frontend Types

**File:** `src/types/index.ts`

Add types (centralized, not in `AbsenceForm.tsx`):
```typescript
export type SessionInSubject = {
  id: string;
  start_at: string;
  end_at: string;
  date: string;
  already_absent: boolean;
};

export type SubjectSessions = {
  subject_id: string;
  subject_code: string;
  subject_name: string;
  course_id: string;
  course_code: string;
  sessions: SessionInSubject[];
};

export type SessionsInRangeResponse = {
  subjects: SubjectSessions[];
};
```

---

### Phase 3: State Management — useReducer

**File:** `src/pages/AbsenceForm.tsx`

Replace 16+ `useState` calls with a single `useReducer`:

```typescript
type FormState = {
  step: number;
  wcode: string;
  studentName: string;
  subjects: SubjectWithActiveCourse[];
  sessions: SubjectSessions[];
  selectedSessionIds: Set<string>;
  reasonCategory: string;
  reason: string;
  sitInResults: Map<string, SitInResult>;  // keyed by subject_id
  submitted: AbsenceRes[];
  errors: Record<string, string>;
  status: "idle" | "looking_up" | "fetching_sessions" | "resolving_sitins" | "submitting";
};

type FormAction =
  | { type: "LOOKUP_START" }
  | { type: "LOOKUP_SUCCESS"; studentName: string; subjects: SubjectWithActiveCourse[] }
  | { type: "LOOKUP_FAILURE"; error: string }
  | { type: "FETCH_SESSIONS_START" }
  | { type: "FETCH_SESSIONS_SUCCESS"; sessions: SubjectSessions[] }
  | { type: "FETCH_SESSIONS_FAILURE"; error: string }
  | { type: "TOGGLE_SESSION"; sessionId: string }
  | { type: "TOGGLE_ALL_SESSIONS" }
  | { type: "TOGGLE_SUBJECT_SESSIONS"; subjectId: string }
  | { type: "SET_DATE_RANGE"; dateFrom: string; dateTo: string }
  | { type: "SET_REASON"; category: string; detail: string }
  | { type: "SITIN_START" }
  | { type: "SITIN_SUCCESS"; subjectId: string; result: SitInResult }
  | { type: "SITIN_FAILURE"; subjectId: string; error: string }
  | { type: "SUBMIT_START" }
  | { type: "SUBMIT_PARTIAL_SUCCESS"; result: AbsenceRes }
  | { type: "SUBMIT_PARTIAL_FAILURE"; subjectId: string; error: string }
  | { type: "SET_STEP"; step: number }
  | { type: "RESET" };
```

Benefits:
- Dispatches are self-documenting
- Prevents impossible state combinations
- Loading state as enum instead of 4 separate booleans
- Per-subject sit-in results as Map enables independent success/failure

---

### Phase 4: Rewrite `AbsenceForm.tsx` — The Multi-Subject Flow

**File:** `src/pages/AbsenceForm.tsx`

Replace the current 4-step wizard with a **4-step flow**: (1) W-Code lookup, (2) Quick-Select sessions + reason, (3) Sit-in resolution per subject, (4) Confirmation + submit.

#### New Component Structure

```
AbsenceForm (rewritten)
├── Step 0: W-Code Lookup (same as current, minimal changes)
├── Step 1: Quick-Select (NEW — replaces "Subject & Dates")
│   ├── DateRangeInput (date_from + date_to)
│   ├── SessionGrid (auto-populated from sessions-in-range)
│   │   └── SubjectRow (per-subject: code, sessions list, toggle)
│   │       └── SessionChip (per-session: time, selected state)
│   └── ReasonSection (category + free text)
├── Step 2: Sit-in Resolution (per-subject, same logic as current Step 3)
│   └── SitInResultCard (one per selected subject)
├── Step 3: Confirmation (summary of all selected subjects/sessions)
└── ConfirmationScreen (post-submit)
```

#### Step 1 — Quick-Select: Detailed Design

**After W-Code lookup succeeds:**

1. **Show date range picker** with smart defaults:
   - Default `date_from` = today
   - Default `date_to` = end of current week (Friday)
   - Both are `<input type="date">` (native picker)
   - Date presets as quick buttons: "This week", "Next 3 days", "Custom"

2. **Auto-fetch sessions** when date range changes (debounced with AbortController):
```typescript
useEffect(() => {
  if (!wcode || !dateFrom || !dateTo) return;
  const controller = new AbortController();
  const timer = setTimeout(() => {
    dispatch({ type: "FETCH_SESSIONS_START" });
    apiJson<SessionsInRangeResponse>(
      `/api/v1/absences/sessions-in-range?wcode=${wcode}&date_from=${dateFrom}&date_to=${dateTo}`,
      { signal: controller.signal }
    )
    .then(data => dispatch({ type: "FETCH_SESSIONS_SUCCESS", sessions: data.subjects }))
    .catch(err => {
      if (err.name !== "AbortError") dispatch({ type: "FETCH_SESSIONS_FAILURE", error: err.message });
    });
  }, 300);
  return () => { clearTimeout(timer); controller.abort(); };
}, [wcode, dateFrom, dateTo]);
```

3. **Display session grid** (grouped by subject):

```
Classes in range (Jun 2–6):

All selected ✓  ← master toggle

┌─ MATH 101 (Mathematics) ──────────────────────┐
│  ● Mon 2 Jun  09:00–10:30  Room 204           │
│  ● Wed 4 Jun  09:00–10:30  Room 204           │
└────────────────────────────────────────────────┘

┌─ PHYS 201 (Physics) ──────────────────────────┐
│  ✗ Tue 3 Jun  11:00–12:30  Room 305  (absent) │  ← already has absence
│  ● Wed 4 Jun  11:00–12:30  Room 305           │
└────────────────────────────────────────────────┘
```

- **All sessions pre-selected** by default (green chips)
- **Sessions marked `already_absent: true`** are grayed out and disabled (can't select)
- **Master toggle** "All selected" switches between "miss everything" and "pick sessions"
- Individual session chips toggle on/off (tap to deselect)
- Subject row header toggles all sessions in that subject
- **No sessions in range** → show muted message: "No classes found in this date range"
- **Accessibility:**
  - Each `SubjectRow`: `role="group"` `aria-label="MATH 101 sessions"`
  - `SessionChip`: `role="checkbox"` `aria-checked={selected}` `aria-label="Mon 2 Jun 09:00-10:30 Mathematics"`
  - Master toggle: `aria-label="Select all sessions"` `role="checkbox"` `aria-controls={subjectRowIds}`
  - Keyboard: Tab between chips, Enter/Space to toggle
  - Live region: announce "3 of 5 sessions selected" on change

4. **Reason section** (always visible, below session grid):
   - Reason category dropdown (from config)
   - Free text textarea (optional, from config)
   - Pre-selects "Sickness" if it's a category value

#### Step 2 — Sit-in Resolution

- After user taps "Next" from Quick-Select, fire sit-in resolution **per selected subject** using `Promise.allSettled()`:
```typescript
dispatch({ type: "SITIN_START" });
const results = await Promise.allSettled(
  selectedSubjects.map(s =>
    apiJson<SitInResult>(
      `/api/v1/absences/sit-in-options?wcode=${wcode}&subject_id=${s.subject_id}&date_from=${dateFrom}&date_to=${dateTo}`
    )
  )
);
results.forEach((result, i) => {
  if (result.status === "fulfilled") {
    dispatch({ type: "SITIN_SUCCESS", subjectId: selectedSubjects[i].subject_id, result: result.value });
  } else {
    dispatch({ type: "SITIN_FAILURE", subjectId: selectedSubjects[i].subject_id, error: result.reason?.message });
  }
});
```
- Display one `SitInResultCard` per subject. If sit-in failed for a subject, show inline error with "Skip sit-in" option (don't block the whole step)
- If sit-in is disabled (`auto_resolve_enabled: false`), skip this step entirely

#### Step 3 — Confirmation

- Summary card showing:
  - Student name + W-Code
  - Date range
  - List of subjects with session count per subject
  - Reason
  - Sit-in method per subject (or "pending" if sit-in failed)
- "Edit" button goes back to Step 1 (preserves all state)

#### Submit Flow

- Fire `Promise.allSettled()` with one `POST /api/v1/absences` per selected subject:
```typescript
dispatch({ type: "SUBMIT_START" });
const results = await Promise.allSettled(
  selectedSubjects.map(subject =>
    apiJson<AbsenceRes>("/api/v1/absences", {
      method: "POST",
      body: JSON.stringify({
        wcode,
        subject_id: subject.subject_id,
        course_id: subject.course_id,
        date_from,
        date_to,
        reason_category,
        reason,
        sit_in_method: subject.sit_in_method,
        sit_in_course_id: subject.sit_in_course_id,
        sit_in_session_ids: subject.selected_session_ids,
      }),
    })
  )
);
```
- **Partial failure handling:**
  - On full success → show combined confirmation screen
  - On partial failure → show per-subject result cards:
    ```
    ✅ MATH 101 — Absence submitted
    ❌ PHYS 201 — Submission failed (network error)
       [Retry] [Skip]
    ```
  - Form state is preserved — user can retry failed subjects without re-entering data
  - On full failure → show error banner with "Retry All"

---

### Phase 5: Shared Components

**New files to create:**

#### `src/components/absences/DateRangeInput.tsx`
- Date from/to pickers
- Quick preset buttons ("This week", "Next 3 days")
- Max range validation from config
- Smart default computation (today → Friday)

#### `src/components/absences/SessionGrid.tsx`
- Takes `SubjectSessions[]` + `selectedSessionIds` + `alreadyAbsentIds`
- Renders grouped subject rows with session chips
- Master toggle "All selected"
- Per-subject toggle
- Per-session toggle
- Empty state when no sessions found
- Accessibility: ARIA roles, keyboard navigation, live region

#### `src/components/absences/SessionChip.tsx`
- Compact chip showing: date, time range
- Green = selected, gray = deselected, striped = already absent (disabled)
- `role="checkbox"` with `aria-checked`
- Tap to toggle (Enter/Space for keyboard)

#### `src/components/absences/SitInResultCard.tsx`
- Per-subject sit-in result display
- Extracted from existing `buildTimeline()` function (lines 449-515 of current `AbsenceForm.tsx`)
- Handles zoom, physical, pending, and error states independently
- Note: `buildTimeline` uses a mutable `usedAvail` Set for pairing — when rendering multiple `SitInResultCard` simultaneously, each card must have its own `usedAvail` scope (already the case since each card is a separate component instance)

#### `src/components/absences/ConfirmationSummary.tsx`
- Summary card for review step + post-submit confirmation
- Shows all selected subjects, dates, reason, sit-in status
- Handles partial success/failure display with per-subject retry buttons

---

### Phase 6: Tests (TDD)

All tests use **URL-pattern-based mocking** instead of `mockResolvedValueOnce` chains:

```typescript
function mockApiByRoute(routes: Record<string, unknown>) {
  mockApiJson.mockImplementation(async (url: string) => {
    for (const [pattern, data] of Object.entries(routes)) {
      if (String(url).includes(pattern)) return data;
    }
    throw new Error(`Unmocked: ${url}`);
  });
}
```

**Test fixtures:**
```typescript
const MOCK_SESSIONS_IN_RANGE: SessionsInRangeResponse = {
  subjects: [
    {
      subject_id: "subj-1", subject_code: "MATH", subject_name: "Mathematics",
      course_id: "c-math201", course_code: "MATH201",
      sessions: [
        { id: "s1", start_at: "2026-06-01T09:00:00Z", end_at: "2026-06-01T10:30:00Z", date: "2026-06-01", already_absent: false },
        { id: "s2", start_at: "2026-06-03T09:00:00Z", end_at: "2026-06-03T10:30:00Z", date: "2026-06-03", already_absent: false },
      ],
    },
    {
      subject_id: "subj-2", subject_code: "PHYS", subject_name: "Physics",
      course_id: "c-phys301", course_code: "PHYS301",
      sessions: [
        { id: "s3", start_at: "2026-06-02T11:00:00Z", end_at: "2026-06-02T12:30:00Z", date: "2026-06-02", already_absent: false },
      ],
    },
  ],
};
```

#### `src/pages/__tests__/AbsenceForm.test.tsx` — rewrite

**Test cases:**
1. **Happy path (multi-subject):** W-Code → date range auto-loads sessions (2 subjects) → all pre-selected → reason → sit-in per subject (×2) → confirm → submit (×2, all resolved). Assert: all 2 subjects listed, both POST calls made with correct bodies.
2. **Single subject:** sessions-in-range returns 1 subject with 2 sessions. Assert: grid shows 1 row, submit fires 1 POST.
3. **No sessions in range:** sessions-in-range returns `{ subjects: [] }`. Assert: "No classes found" visible, submit button disabled.
4. **Deselect sessions:** deselect 1 session from subject A. Assert: only correct POST calls made with adjusted date range.
5. **Partial failure:** POST for subject A resolves, POST for subject B rejects. Assert: success message for A, retry button visible for B, clicking retry re-fires POST for B.
6. **All failures:** all POST calls reject. Assert: full error state shown, "Retry All" button visible.
7. **Sit-in per subject:** sit-in-options for subject A returns zoom, for subject B returns physical. Assert: step 2 shows zoom banner for A, physical timeline for B.
8. **Sit-in partial failure:** sit-in-options for subject A resolves, for subject B rejects. Assert: step 2 shows zoom result for A, inline error + "Skip sit-in" for B.
9. **Already absent sessions:** sessions-in-range returns sessions with `already_absent: true`. Assert: those chips are grayed out and disabled, not included in selection.
10. **Date range validation:** enter range > `max_date_range_days`. Assert: error message visible, Next button disabled.
11. **Per-subject toggle:** click subject header toggle → deselects all sessions in that subject. Assert: that subject's chips are gray, other subjects untouched.
12. **Master toggle:** click "All selected" OFF → all chips deselect. Click ON → all chips select. Assert: submit data reflects selection state.
13. **Reset form:** complete flow, submit, click "Submit Another". Assert: form resets to step 0.

#### `src/components/absences/__tests__/SessionGrid.test.tsx`
- Renders sessions grouped by subject with correct headers
- All sessions selected by default
- Master toggle: deselect all → all chips gray, submit data empty
- Master toggle: re-select all → all chips green
- Per-subject toggle: deselect MATH → MATH chips gray, PHYS untouched
- Per-session toggle: deselect one chip → subject toggle goes indeterminate
- Already absent sessions: rendered as disabled/grayed
- Empty state: renders "No classes found" message
- Empty state: submit button disabled
- Accessibility: ARIA attributes present on chips and toggles

#### `src/components/absences/__tests__/DateRangeInput.test.tsx`
- Default `date_from` = today
- Default `date_to` = end of current week (Friday)
- "This week" preset sets correct range
- "Next 3 days" preset sets correct range
- Max range validation enforces `config.form.max_date_range_days`

#### `src/components/absences/__tests__/SitInResultCard.test.tsx`
- Zoom method: renders blue banner, no session list
- Physical method: renders timeline with pre-selected checkboxes
- Pending method: renders amber "assigned by staff" message
- Error state: renders inline error with retry/skip buttons
- Checkbox toggle calls onChange with session id

**Shared test helpers** — `src/pages/__tests__/helpers/index.tsx`:
```typescript
renderWithProviders(ui)           — wraps ToastProvider + BrowserRouter
mockApiByPattern(patterns)        — routes mock responses by URL pattern
createMockSessionsInRange(data)   — returns fixture matching SessionsInRangeResponse
createMockSitInResult(method)     — returns fixture for zoom/physical/pending
```

---

### Phase 7: Mobile Responsiveness

- `<640px`: `SessionGrid` renders as vertical card stack (not grid)
- `<640px`: `SessionChip` full-width with left border color (green/gray) instead of background
- `<640px`: `DateRangeInput` stacks date pickers vertically
- Sticky bottom bar: fixed position, `z-10`, `safe-area-inset-bottom`
- Max session list height: `max-h-64 overflow-y-auto` per subject (prevents long scroll)
- Touch targets: 44px minimum for all interactive elements

---

## Parallel Execution Strategy

Tasks are grouped by dependency. Independent tasks run in parallel via subagents.

### Parallel Group A (no dependencies)
- **A1:** Backend SQL queries (`absences.sql`)
- **A2:** Frontend types (`src/types/index.ts`)
- **A3:** Test helpers (`src/pages/__tests__/helpers/index.tsx`)

### Parallel Group B (depends on A1)
- **B1:** Backend HTTP handler (`routes.go`) — needs SQL queries
- **B2:** `SessionChip.tsx` + tests — standalone component, no backend dependency
- **B3:** `DateRangeInput.tsx` + tests — standalone component

### Parallel Group C (depends on A2, B2, B3)
- **C1:** `SessionGrid.tsx` + tests — needs types + SessionChip + DateRangeInput
- **C2:** `SitInResultCard.tsx` + tests — needs types, can parallel with C1

### Parallel Group D (depends on B1, C1, C2)
- **D1:** `AbsenceForm.tsx` rewrite — needs backend endpoint + all components
- **D2:** `ConfirmationSummary.tsx` + tests — needs types, can parallel with D1 start

### Parallel Group E (depends on D1)
- **E1:** `AbsenceForm.test.tsx` rewrite — needs AbsenceForm complete
- **E2:** Mobile responsiveness pass — needs all components

### Per-Task Subagent Pattern
Each task spawns:
1. **Spec review subagent** — verifies implementation matches plan spec
2. **Code review subagent** — checks quality, patterns, accessibility
3. Both run after implementation completes; task only marked done if both approve

---

## Files to Modify

| File | Change |
|------|--------|
| `src/pages/AbsenceForm.tsx` | Full rewrite — multi-subject flow with useReducer |
| `src/pages/__tests__/AbsenceForm.test.tsx` | Rewrite tests for new flow (13 test cases) |
| `src/types/index.ts` | Add new types (SessionInSubject, SubjectSessions, SessionsInRangeResponse) |
| `backend/internal/httpapi/absenceshttp/routes.go` | Add `sessions-in-range` route + handler |
| `backend/db/queries/absences.sql` | Add 2 queries (SessionsByStudentInRange, AbsenceOverlappingSessions) |

## Files to Create

| File | Purpose |
|------|---------|
| `src/components/absences/DateRangeInput.tsx` | Date range picker with presets |
| `src/components/absences/SessionGrid.tsx` | Session selection grid |
| `src/components/absences/SessionChip.tsx` | Individual session chip |
| `src/components/absences/SitInResultCard.tsx` | Per-subject sit-in result |
| `src/components/absences/ConfirmationSummary.tsx` | Review + confirmation UI |
| `src/components/absences/__tests__/SessionGrid.test.tsx` | Session grid tests |
| `src/components/absences/__tests__/DateRangeInput.test.tsx` | Date range input tests |
| `src/components/absences/__tests__/SitInResultCard.test.tsx` | Sit-in result card tests |
| `src/pages/__tests__/helpers/index.tsx` | Shared test helpers |

## Acceptance Criteria

1. Student can request absence for 2+ subjects in one submission
2. Date range auto-detects which sessions are affected
3. All sessions pre-selected by default (4-tap happy path: W-Code → Look Up → Review → Submit)
4. Student can deselect individual sessions or entire subjects
5. Sessions already covered by existing absences are flagged and disabled
6. Sit-in resolution works per-subject, with independent failure handling
7. Partial submit failures are handled gracefully with per-subject retry
8. When only 1 subject has sessions, the grid works correctly (single-subject backward compat)
9. Mobile responsive (375px+ width)
10. All tests pass with URL-pattern-based mocking
11. Accessibility: ARIA roles, keyboard navigation, live region announcements
12. No DB schema changes required
