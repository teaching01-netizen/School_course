# Calendar UX Redesign: Session-Centric with Sit-In Overlay

**Date:** 2026-06-02
**Status:** Approved
**Reference:** Google Calendar (session-centric with compact secondary indicators)

---

## Problem Statement

Admins cannot quickly see sit-in assignments on the calendar. Absence cards and session cards compete visually, causing clutter. Staff needs to notify teachers about:
1. Which students are **absent** from their course
2. Which students are **visiting** (sitting in) on their session

Current pain points:
- Flat list of cards in day modal (no hierarchy)
- Absence cards dominate the grid (amber/sky/rose colors)
- No inline sit-in visibility on session cards
- Staff must click into each absence to find sit-in details

---

## Design Principles

1. **Session-centric**: Session cards are the primary visual element
2. **Compact indicators**: Absences compressed to pills (not full cards)
3. **Inline context**: Sit-in visitors shown directly on session cards
4. **Two-perspective modal**: Sessions (top) + Absences (bottom)

---

## Visual Design

### 1. Calendar Grid (Week + Month)

**Month View:**
- Session cards: white background, left accent bar (course color), course name + time
- Absence indicator: compact amber pill at top of day cell: `3 absences`
- Max 2 session cards visible, `+N more` overflow
- Max 2 absence pills visible (same constraint as current max 2 cards)

**Week View:**
- Absence pill at top of day column (amber-50 bg, amber-700 text)
- Session cards below (white cards with left accent bar)
- Sit-in visitors shown inline on session cards

### 2. Session Card Design

```
┌─────────────────────────────────────┐
│ SAT Math Scholar        09:00-10:30 │
│ Room 101 · Teacher A                │
│ Visitors: W250389 (Physics)         │
└─────────────────────────────────────┘
```

- Left accent bar: course-specific color (amber for sit-in visitors)
- Course name (bold), time range
- Room + teacher (gray text)
- Sit-in visitors: amber text, `Visitors: W250389 (from course)`
- Max 2 visitors inline, `+N more` overflow
- Hover visitor → tooltip with absence context
- Click card → session detail modal

### 3. Absence Indicator Pill

```
┌─────────────┐
│ 3 absences  │  (amber-50 bg, amber-700 text)
└─────────────┘
```

- Compact pill at top of day column
- Click → opens day detail modal
- 0 absences = no pill rendered
- Hover → tooltip with student names

### 4. Day Detail Modal

**Layout:**
```
┌─────────────────────────────────────────────────┐
│ Tuesday, 2 June 2026 · 3 sessions · 2 absences │
├─────────────────────────────────────────────────┤
│ Sessions (3)                                    │
│ ┌─────────────────────────────────────────────┐ │
│ │ SAT Math Scholar        09:00-10:30        │ │
│ │ Room 101 · Teacher A                       │ │
│ │ Visitors: W250389 (Physics)                │ │
│ └─────────────────────────────────────────────┘ │
│ ┌─────────────────────────────────────────────┐ │
│ │ Physics Advanced       11:00-12:30         │ │
│ │ Room 201 · Teacher B                       │ │
│ └─────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────┤
│ Absences (2)                                    │
│ ┌─────────────────────────────────────────────┐ │
│ │ W250389 · John Smith          [Pending]    │ │
│ │ Leave: Mathematics · Sit-in: Physics        │ │
│ │ [View details]                              │ │
│ └─────────────────────────────────────────────┘ │
│ ┌─────────────────────────────────────────────┐ │
│ │ W250390 · Jane Roe            [Reviewed]   │ │
│ │ Leave: Mathematics · Sit-in: Zoom           │ │
│ │ [View details]                              │ │
│ └─────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────┤
│ [Close]                                         │
└─────────────────────────────────────────────────┘
```

**Features:**
- Sessions section (top): each session shows sit-in visitors inline
- Absences section (bottom): grouped list with status badges, click-through to `/absences/:id`
- Filter bar: subject, status, "show sessions with sit-ins only" toggle

### 5. Color Language

| Color | Usage |
|-------|-------|
| Amber-50/amber-700 | Sit-in/visitor related |
| Blue-50/blue-700 | Absence status/scheduling |
| Gray-50/gray-700 | Neutral session info |
| Green-50/green-700 | Reviewed status |
| Red-50/red-700 | Cancelled status |

---

## Backend Changes

### API Enrichment

**Endpoint:** `GET /api/v1/operations/calendar?start=...&end=...`

**Current response:**
```json
{
  "sessions": CalendarSessionBrief[],
  "absence_days": CalendarAbsenceDay[]
}
```

**Proposed change:** Enrich `CalendarSessionBrief` with sit-in student data.

```typescript
// New field on CalendarSessionBrief
sit_in_students?: Array<{
  wcode: string;
  student_name: string | null;
  absence_id: string;
  from_course_code: string;
  from_course_name: string | null;
}>
```

**SQL approach:** Join `absence_sit_ins` → `student_absences` → `sessions` to find which students are sitting in on each session. Group by session ID.

**Backward compatible:** New field is optional. Existing consumers unaffected.

---

## Implementation Plan

### Phase 1: Backend (TDD) ⏳
1. Add `sit_in_students` field to `CalendarSessionBrief` type ✅ (frontend type)
2. Write SQL query to join sit-ins by session
3. Update calendar handler to populate new field
4. Write integration tests

### Phase 2: Frontend (TDD) ✅
1. Update `CalendarSessionBrief` type with new field ✅
2. Refactor calendar grid to session-centric layout ✅
3. Create session card component with sit-in visitor display ✅
4. Create absence indicator pill component ✅
5. Redesign day detail modal ✅
6. Write unit tests for each component ✅

### Phase 3: Verification ⏳
1. Run all tests ✅
2. Verify visual design matches spec (needs manual review)
3. Test edge cases (no sit-ins, multiple visitors, overflow) ✅

---

## Files to Modify

| File | Change |
|------|--------|
| `src/pages/OperationsCalendar.tsx` | Refactor grid, add session-centric cards, absence pill, redesigned modal |
| `src/types/index.ts` | Add `sit_in_students` to `CalendarSessionBrief` |
| `backend/db/queries/absences.sql` | New query to join sit-ins by session |
| `backend/internal/db/calendar_custom.go` | Generated query code |
| `backend/internal/httpapi/absenceshttp/management_routes.go` | Enrich calendar endpoint response |

---

## Edge Cases

1. **No sit-ins:** Session card shows no visitor line
2. **Multiple visitors:** Show first 2, `+N more` overflow
3. **No sessions:** Show "No sessions" message
4. **No absences:** Hide absence pill and section
5. **Past sessions:** Immutable, no edit capability
6. **Concurrent edits:** Optimistic concurrency with version check

---

## Testing Strategy

### Backend Tests
- Integration test: calendar endpoint returns enriched session data
- Unit test: SQL query joins sit-ins correctly
- Edge case: no sit-ins returns empty array

### Frontend Tests
- Unit test: session card renders sit-in visitors
- Unit test: absence pill renders count
- Unit test: day modal shows sessions first, absences second
- Integration test: full calendar render with enriched data
