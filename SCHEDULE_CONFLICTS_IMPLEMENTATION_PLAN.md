# Schedule Conflicts Overview Page - Implementation Plan

## 1. Executive Summary

This plan outlines the implementation of a comprehensive **Schedule Conflicts Overview** page that displays all conflicts for both students and teachers. The page will show subject names and date/time information for pairs or multiple pairs of conflicts, with filters matching the existing Courses overview page pattern.

### Key Requirements
- Display ALL schedule conflicts (student overlaps, teacher overlaps, room overlaps)
- Show subject name and date/time of conflicting sessions
- Provide filters similar to the Courses overview page
- Read-only view (no edit functionality in this phase)
- Expandable rows to show conflict details

---

## 2. Change Chain Analysis

### 2.1 Entry Point
- **User Intent**: View all schedule conflicts across the system
- **Affected Actors**: Admin users (requires Admin role)
- **Trigger**: Navigation to `/schedule-conflicts`

### 2.2 Current Behavior
- `CrmConflicts.tsx` shows only CRM roster-related conflicts
- `SlotFinder.tsx` shows conflicts per student/subject search
- `CourseDetail.tsx` shows conflicts within a single course
- No unified view of ALL system conflicts exists

### 2.3 Target Behavior
- New page at `/schedule-conflicts` showing all conflict types
- Filterable by: conflict type, subject, teacher, student, date range
- Expandable rows showing detailed conflict information
- Summary statistics at the top

### 2.4 Source of Truth
- **Backend**: `/api/v1/schedule-conflicts` (new endpoint)
- **Frontend**: New `ScheduleConflicts.tsx` page

### 2.5 Dependency Graph

```
INTENT: Display all schedule conflicts
    ↓
ENTRY POINT: /schedule-conflicts route
    ↓
CONTROL FLOW: ScheduleConflicts.tsx page component
    ↓
DOMAIN LOGIC: Filter state, pagination, conflict grouping
    ↓
STATE: useSearchParams for URL-based filtering
    ↓
CONTRACTS: ConflictOverviewItem type, API response type
    ↓
PERSISTENCE: Backend API endpoint (new)
    ↓
CONSUMERS: Table rows, expandable panels
    ↓
PRESENTATION: Filter bar, data table, empty states
    ↓
TESTS: Component tests, API tests
```

---

## 3. Backend Implementation Plan

### 3.1 New API Endpoint

**Endpoint**: `GET /api/v1/schedule-conflicts`

**Query Parameters**:
```typescript
{
  // Pagination
  limit?: number;      // default 50, max 200
  offset?: number;     // default 0
  
  // Filters
  conflict_type?: 'room_overlap' | 'teacher_overlap' | 'student_overlap' | 'all';
  subject_id?: string;
  teacher_id?: string;
  student_id?: string;
  date_from?: string;  // ISO date
  date_to?: string;    // ISO date
  q?: string;          // Search by name/code
}
```

**Response Type**:
```typescript
type ConflictOverviewResponse = {
  items: ConflictOverviewItem[];
  total_count: number;
  offset: number;
  limit: number;
  summary: {
    total_conflicts: number;
    room_overlaps: number;
    teacher_overlaps: number;
    student_overlaps: number;
  };
};

type ConflictOverviewItem = {
  id: string;
  conflict_type: 'room_overlap' | 'teacher_overlap' | 'student_overlap';
  
  // Primary session (the one being scheduled)
  primary_session: {
    session_id: string;
    course_id: string;
    course_code: string;
    course_name: string;
    subject_id: string;
    subject_name: string;
    teacher_id: string;
    teacher_name: string;
    room_id: string | null;
    room_name: string | null;
    start_at: string;
    end_at: string;
  };
  
  // Conflicting session(s)
  conflicting_sessions: Array<{
    session_id: string;
    course_id: string;
    course_code: string;
    course_name: string;
    subject_name: string;
    teacher_name: string;
    room_name: string | null;
    start_at: string;
    end_at: string;
  }>;
  
  // For student overlaps: list of affected students
  affected_students?: Array<{
    student_id: string;
    wcode: string;
    full_name: string;
  }>;
  
  // Shared resource (room or teacher depending on conflict type)
  shared_resource: {
    type: 'room' | 'teacher';
    id: string;
    name: string;
  };
  
  detected_at: string;
};
```

### 3.2 Backend Implementation Steps

1. **Create new route handler** in `src/api/routes/scheduleConflicts.ts`
2. **Implement query builder** that joins sessions, courses, subjects, teachers, rooms
3. **Add conflict detection logic** to identify overlapping sessions
4. **Implement filtering** with proper SQL WHERE clauses
5. **Add pagination** with LIMIT/OFFSET
6. **Add summary aggregation** query
7. **Register route** in main API router
8. **Add authentication check** (Admin only)
9. **Add unit tests** for the endpoint

### 3.3 Database Queries Required

**Main query** (simplified):
```sql
SELECT 
  s1.id as session_id,
  s1.course_id,
  c.code as course_code,
  c.name as course_name,
  sub.id as subject_id,
  sub.name as subject_name,
  u.id as teacher_id,
  u.full_name as teacher_name,
  r.id as room_id,
  r.name as room_name,
  s1.start_at,
  s1.end_at,
  -- Conflicting session details
  s2.id as conflict_session_id,
  s2.course_id as conflict_course_id,
  -- ... etc
FROM sessions s1
JOIN courses c ON s1.course_id = c.id
JOIN subjects sub ON c.subject_id = sub.id
JOIN users u ON s1.teacher_id = u.id
LEFT JOIN rooms r ON s1.room_id = r.id
-- Find conflicts
JOIN sessions s2 ON (
  (s1.room_id = s2.room_id AND s1.room_id IS NOT NULL) OR
  (s1.teacher_id = s2.teacher_id)
) AND s1.id != s2.id
AND s1.start_at < s2.end_at
AND s1.end_at > s2.start_at
-- Apply filters
WHERE [filters]
ORDER BY s1.start_at DESC
LIMIT ? OFFSET ?
```

---

## 4. Frontend Implementation Plan

### 4.1 New Files to Create

1. **`src/pages/ScheduleConflicts.tsx`** - Main page component
2. **`src/features/scheduling/types/conflictOverview.ts`** - TypeScript types
3. **`src/features/scheduling/api/conflictOverviewApi.ts`** - API client functions
4. **`src/components/schedule-conflicts/ConflictFilters.tsx`** - Filter bar component
5. **`src/components/schedule-conflicts/ConflictTable.tsx`** - Data table component
6. **`src/components/schedule-conflicts/ConflictRow.tsx`** - Expandable row component
7. **`src/components/schedule-conflicts/ConflictSummary.tsx`** - Summary stats component
8. **`src/components/schedule-conflicts/ConflictDetailPanel.tsx`** - Expanded detail view
9. **`src/pages/__tests__/ScheduleConflicts.test.tsx`** - Component tests

### 4.2 Files to Modify

1. **`src/App.tsx`** - Add route for `/schedule-conflicts`
2. **`src/components/layout/navConfig.ts`** - Add navigation item
3. **`src/components/layout/Sidebar.tsx`** - Add badge for conflict count (optional)

### 4.3 Type Definitions

```typescript
// src/features/scheduling/types/conflictOverview.ts

export type ConflictType = 'room_overlap' | 'teacher_overlap' | 'student_overlap';

export type ConflictSession = {
  session_id: string;
  course_id: string;
  course_code: string;
  course_name: string;
  subject_name: string;
  teacher_name: string;
  room_name: string | null;
  start_at: string;
  end_at: string;
};

export type ConflictOverviewItem = {
  id: string;
  conflict_type: ConflictType;
  primary_session: ConflictSession;
  conflicting_sessions: ConflictSession[];
  affected_students?: Array<{
    student_id: string;
    wcode: string;
    full_name: string;
  }>;
  shared_resource: {
    type: 'room' | 'teacher';
    id: string;
    name: string;
  };
  detected_at: string;
};

export type ConflictOverviewFilters = {
  conflict_type: ConflictType | 'all';
  subject_id: string;
  teacher_id: string;
  student_id: string;
  date_from: string;
  date_to: string;
  q: string;
};

export type ConflictOverviewSummary = {
  total_conflicts: number;
  room_overlaps: number;
  teacher_overlaps: number;
  student_overlaps: number;
};
```

### 4.4 API Client Functions

```typescript
// src/features/scheduling/api/conflictOverviewApi.ts

import { apiJson } from '@/api/client';
import type {
  ConflictOverviewItem,
  ConflictOverviewFilters,
  ConflictOverviewSummary,
} from '../types/conflictOverview';

type ConflictOverviewResponse = {
  items: ConflictOverviewItem[];
  total_count: number;
  offset: number;
  limit: number;
  summary: ConflictOverviewSummary;
};

export async function getScheduleConflicts(
  filters: ConflictOverviewFilters,
  offset: number = 0,
  limit: number = 50
): Promise<ConflictOverviewResponse> {
  const params = new URLSearchParams();
  params.set('limit', String(limit));
  params.set('offset', String(offset));
  
  if (filters.conflict_type !== 'all') {
    params.set('conflict_type', filters.conflict_type);
  }
  if (filters.subject_id) params.set('subject_id', filters.subject_id);
  if (filters.teacher_id) params.set('teacher_id', filters.teacher_id);
  if (filters.student_id) params.set('student_id', filters.student_id);
  if (filters.date_from) params.set('date_from', filters.date_from);
  if (filters.date_to) params.set('date_to', filters.date_to);
  if (filters.q) params.set('q', filters.q);
  
  return apiJson<ConflictOverviewResponse>(
    `/api/v1/schedule-conflicts?${params.toString()}`,
    { method: 'GET' }
  );
}
```

---

## 5. UI Component Design

### 5.1 Page Layout

```
┌─────────────────────────────────────────────────────────────┐
│ Schedule Conflicts                                          │
│ All room, teacher, and student scheduling conflicts         │
├─────────────────────────────────────────────────────────────┤
│ Summary Cards                                               │
│ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐       │
│ │ Total    │ │ Room     │ │ Teacher  │ │ Student  │       │
│ │ 156      │ │ 42       │ │ 89       │ │ 25       │       │
│ └──────────┘ └──────────┘ └──────────┘ └──────────┘       │
├─────────────────────────────────────────────────────────────┤
│ Filters                                                     │
│ [Search...] [Type ▼] [Subject ▼] [Teacher ▼] [Date Range] │
├─────────────────────────────────────────────────────────────┤
│ ┌─┬────────────┬────────────┬────────────┬─────────┬─────┐ │
│ │▶│ Subject    │ Conflict   │ Time       │ Type    │ ... │ │
│ ├─┼────────────┼────────────┼────────────┼─────────┼─────┤ │
│ │▶│ [Math]     │ 2 overlaps │ Mon 9:00   │ Room    │     │ │
│ │ │            │            │            │         │     │ │
│ ├─┼────────────┼────────────┼────────────┼─────────┼─────┤ │
│ │▶│ [Physics]  │ 3 overlaps │ Tue 14:00  │ Teacher │     │ │
│ └─┴────────────┴────────────┴────────────┴─────────┴─────┘ │
│                                                             │
│ 156 records  [Previous] 1 of 4 [Next]                      │
└─────────────────────────────────────────────────────────────┘
```

### 5.2 Filter Bar Component

The filter bar will mirror the Courses page pattern:

```tsx
// ConflictFilters.tsx
<SearchInput
  value={searchInput}
  onChange={setSearchInput}
  placeholder="Search by subject, course, teacher..."
/>

<SearchableSelect
  aria-label="Conflict type filter"
  value={filters.conflict_type}
  onChange={(e) => updateFilter('conflict_type', e.target.value)}
>
  <option value="all">All types</option>
  <option value="room_overlap">Room overlaps</option>
  <option value="teacher_overlap">Teacher overlaps</option>
  <option value="student_overlap">Student conflicts</option>
</SearchableSelect>

<SearchableSelect
  aria-label="Subject filter"
  value={filters.subject_id}
  onChange={(e) => updateFilter('subject_id', e.target.value)}
>
  <option value="">All subjects</option>
  {subjects.map(s => (
    <option key={s.id} value={s.id}>{s.name}</option>
  ))}
</SearchableSelect>

<SearchableSelect
  aria-label="Teacher filter"
  value={filters.teacher_id}
  onChange={(e) => updateFilter('teacher_id', e.target.value)}
>
  <option value="">All teachers</option>
  {teachers.map(t => (
    <option key={t.id} value={t.id}>{t.full_name}</option>
  ))}
</SearchableSelect>

<SessionDateFilter
  value={{ from: filters.date_from, to: filters.date_to }}
  onChange={updateDateFilter}
  onClear={() => updateDateFilter({ from: '', to: '' })}
/>
```

### 5.3 Table Structure

```tsx
<table className="w-full text-[13px]">
  <thead>
    <tr className="border-b border-wi-line">
      <th scope="col" className="w-8 px-2"></th>  {/* Expand toggle */}
      <th scope="col" className="text-left py-2 px-2">Subject</th>
      <th scope="col" className="text-left py-2 px-2">Course</th>
      <th scope="col" className="text-left py-2 px-2">Teacher</th>
      <th scope="col" className="text-left py-2 px-2">Room</th>
      <th scope="col" className="text-left py-2 px-2">Date/Time</th>
      <th scope="col" className="text-left py-2 px-2">Type</th>
      <th scope="col" className="text-left py-2 px-2">Conflicts</th>
    </tr>
  </thead>
  <tbody>
    {items.map(item => (
      <ConflictRow
        key={item.id}
        item={item}
        expanded={expandedIds.has(item.id)}
        onToggle={() => handleToggleExpand(item.id)}
      />
    ))}
  </tbody>
</table>
```

### 5.4 Expandable Row Component

```tsx
// ConflictRow.tsx
<Fragment key={item.id}>
  <tr className="border-b border-wi-line hover:bg-[var(--color-wi-row-alt)]">
    <td className="w-8 py-3 px-1">
      <button
        type="button"
        onClick={() => onToggle()}
        className="flex items-center justify-center h-6 w-6 rounded-sm"
        aria-expanded={expanded}
      >
        <ChevronRight className={`h-4 w-4 transition-transform ${expanded ? 'rotate-90' : ''}`} />
      </button>
    </td>
    <td className="py-3 px-2 font-medium">
      {item.primary_session.subject_name}
    </td>
    <td className="py-3 px-2">
      <span className="text-[var(--color-wi-text-light)]">
        [{item.primary_session.course_code}] {item.primary_session.course_name}
      </span>
    </td>
    <td className="py-3 px-2">{item.primary_session.teacher_name}</td>
    <td className="py-3 px-2">{item.primary_session.room_name ?? '—'}</td>
    <td className="py-3 px-2">
      <span className="inline-flex items-center gap-1">
        <Clock className="w-3.5 h-3.5 text-amber-500" />
        {formatConflictTime(item.primary_session.start_at, item.primary_session.end_at)}
      </span>
    </td>
    <td className="py-3 px-2">
      <ConflictTypeBadge type={item.conflict_type} />
    </td>
    <td className="py-3 px-2">
      <span className="text-sm font-medium">{item.conflicting_sessions.length}</span>
    </td>
  </tr>
  
  {expanded && (
    <tr className="border-b border-wi-line">
      <td colSpan={8} className="p-0">
        <ConflictDetailPanel item={item} />
      </td>
    </tr>
  )}
</Fragment>
```

### 5.5 Detail Panel Component

```tsx
// ConflictDetailPanel.tsx
<div className="px-8 py-4 bg-[var(--color-wi-row-alt)]/50">
  <h4 className="text-sm font-semibold mb-3">Conflicting Sessions</h4>
  
  <div className="space-y-2">
    {item.conflicting_sessions.map(session => (
      <div
        key={session.session_id}
        className="rounded-sm border border-wi-line bg-white p-3"
      >
        <div className="flex items-center justify-between">
          <div>
            <span className="font-medium">{session.subject_name}</span>
            <span className="text-[var(--color-wi-text-light)] ml-2">
              [{session.course_code}] {session.course_name}
            </span>
          </div>
          <ConflictTypeBadge type={item.conflict_type} />
        </div>
        <div className="mt-2 text-sm text-[var(--color-wi-text-light)]">
          <span>Teacher: {session.teacher_name}</span>
          {session.room_name && (
            <span className="ml-4">Room: {session.room_name}</span>
          )}
          <span className="ml-4">
            Time: {formatConflictTime(session.start_at, session.end_at)}
          </span>
        </div>
      </div>
    ))}
  </div>
  
  {item.affected_students && item.affected_students.length > 0 && (
    <div className="mt-4">
      <h4 className="text-sm font-semibold mb-2">Affected Students</h4>
      <div className="flex flex-wrap gap-2">
        {item.affected_students.map(student => (
          <span
            key={student.student_id}
            className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-2 py-1 text-xs text-blue-700 border border-blue-200"
          >
            {student.full_name} ({student.wcode})
          </span>
        ))}
      </div>
    </div>
  )}
  
  <div className="mt-4 text-xs text-[var(--color-wi-text-light)]">
    Shared {item.shared_resource.type}: {item.shared_resource.name}
  </div>
</div>
```

---

## 6. Routing & Navigation

### 6.1 Add Route to App.tsx

```tsx
// In App.tsx, inside <RequireAdmin /> route block:
const ScheduleConflicts = lazy(() => import('./pages/ScheduleConflicts'));

// Add route:
<Route path="/schedule-conflicts" element={<ScheduleConflicts />} />
```

### 6.2 Add to Navigation Config

```tsx
// In navConfig.ts, add to 'operations' section:
{
  id: 'operations',
  label: 'Operations',
  items: [
    // ... existing items
    { path: '/schedule-conflicts', label: 'All Conflicts', icon: AlertOctagon },
  ],
}
```

### 6.3 Add Page Title

```tsx
// In navConfig.ts pageTitles:
'/schedule-conflicts': 'Schedule Conflicts',
```

---

## 7. Implementation Order

### Phase 1: Types & API Contract (Backend)
1. [ ] Define TypeScript types for API request/response
2. [ ] Create database migration if needed (likely not - using existing tables)
3. [ ] Implement backend endpoint with filtering and pagination
4. [ ] Add summary aggregation query
5. [ ] Write unit tests for the endpoint

### Phase 2: Frontend API Layer
1. [ ] Create `conflictOverview.ts` types file
2. [ ] Create `conflictOverviewApi.ts` API client
3. [ ] Add query cache keys

### Phase 3: UI Components
1. [ ] Create `ConflictSummary.tsx` - Summary cards
2. [ ] Create `ConflictFilters.tsx` - Filter bar
3. [ ] Create `ConflictTable.tsx` - Main table
4. [ ] Create `ConflictRow.tsx` - Expandable row
5. [ ] Create `ConflictDetailPanel.tsx` - Detail view
6. [ ] Create `ConflictTypeBadge.tsx` - Type indicator

### Phase 4: Page Assembly
1. [ ] Create `ScheduleConflicts.tsx` - Main page
2. [ ] Add route to `App.tsx`
3. [ ] Add navigation item to `navConfig.ts`

### Phase 5: Testing & Polish
1. [ ] Write component tests
2. [ ] Add loading skeletons
3. [ ] Add empty states
4. [ ] Add error handling
5. [ ] Test with large datasets
6. [ ] Accessibility audit

---

## 8. Testing Strategy

### 8.1 Unit Tests

```typescript
// ScheduleConflicts.test.tsx
describe('ScheduleConflicts', () => {
  it('renders summary cards with correct counts');
  it('applies filters correctly');
  it('handles empty state');
  it('expands row to show conflict details');
  it('paginates through results');
  it('searches by subject name');
  it('filters by conflict type');
  it('filters by date range');
});
```

### 8.2 Integration Tests

```typescript
describe('ScheduleConflicts API', () => {
  it('returns paginated results');
  it('filters by conflict_type');
  it('filters by subject_id');
  it('filters by teacher_id');
  it('filters by date range');
  it('returns correct summary counts');
  it('requires admin authentication');
});
```

---

## 9. Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Performance with large datasets | High | Implement pagination, add database indexes |
| Complex SQL queries | Medium | Use query builder, add explain plans |
| Filter state management | Low | Use URL params like Courses page |
| Accessibility | Medium | Follow existing patterns, add ARIA labels |
| Mobile responsiveness | Low | Use existing responsive patterns |

---

## 10. Success Criteria

- [ ] Page displays all conflict types (room, teacher, student)
- [ ] Filters work correctly and persist in URL
- [ ] Pagination works with large datasets
- [ ] Expandable rows show detailed conflict information
- [ ] Summary cards show accurate counts
- [ ] Page loads in < 2 seconds with 1000+ conflicts
- [ ] All existing tests continue to pass
- [ ] New tests achieve > 80% coverage

---

## 11. Future Enhancements (Out of Scope)

- Edit/resolve conflicts from this page (Phase 2)
- Bulk actions (select multiple, bulk resolve)
- Export to CSV/XLSX
- Real-time updates via WebSocket
- Conflict resolution suggestions (AI-powered)
- Calendar view of conflicts
- Drill-down to course/student profiles
