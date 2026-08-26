# Schedule Conflicts Page - Implementation Checklist

## Files to CREATE

### Backend
- [ ] `src/api/routes/scheduleConflicts.ts` - New API endpoint

### Frontend Types & API
- [ ] `src/features/scheduling/types/conflictOverview.ts` - TypeScript types
- [ ] `src/features/scheduling/api/conflictOverviewApi.ts` - API client

### UI Components
- [ ] `src/components/schedule-conflicts/ConflictSummary.tsx` - Summary cards
- [ ] `src/components/schedule-conflicts/ConflictFilters.tsx` - Filter bar
- [ ] `src/components/schedule-conflicts/ConflictTable.tsx` - Main table
- [ ] `src/components/schedule-conflicts/ConflictRow.tsx` - Expandable row
- [ ] `src/components/schedule-conflicts/ConflictDetailPanel.tsx` - Detail view
- [ ] `src/components/schedule-conflicts/ConflictTypeBadge.tsx` - Type indicator

### Main Page
- [ ] `src/pages/ScheduleConflicts.tsx` - Main page component

### Tests
- [ ] `src/pages/__tests__/ScheduleConflicts.test.tsx` - Component tests

---

## Files to MODIFY

### Routing
- [ ] `src/App.tsx` - Add lazy import and route
- [ ] `src/components/layout/navConfig.ts` - Add navigation item + page title

---

## API Contract

**Endpoint**: `GET /api/v1/schedule-conflicts`

**Query Parameters**:
- `limit` (default 50, max 200)
- `offset` (default 0)
- `conflict_type` ('room_overlap' | 'teacher_overlap' | 'student_overlap' | 'all')
- `subject_id`
- `teacher_id`
- `student_id`
- `date_from` (ISO date)
- `date_to` (ISO date)
- `q` (search term)

**Response**:
```json
{
  "items": [...],
  "total_count": 156,
  "offset": 0,
  "limit": 50,
  "summary": {
    "total_conflicts": 156,
    "room_overlaps": 42,
    "teacher_overlaps": 89,
    "student_overlaps": 25
  }
}
```

---

## Implementation Phases

### Phase 1: Backend (Days 1-2)
1. Define types
2. Implement endpoint
3. Add filtering/pagination
4. Add summary aggregation
5. Write tests

### Phase 2: Frontend API (Day 3)
1. Create types file
2. Create API client
3. Add cache keys

### Phase 3: UI Components (Days 4-5)
1. Summary cards
2. Filter bar
3. Table
4. Expandable rows
5. Detail panel

### Phase 4: Page Assembly (Day 6)
1. Main page
2. Route
3. Navigation

### Phase 5: Testing & Polish (Days 7-8)
1. Component tests
2. Loading states
3. Error handling
4. Accessibility

---

## Key Design Decisions

1. **URL-based filtering**: Use `useSearchParams` like Courses page for shareable URLs
2. **Pagination**: Server-side pagination with `limit/offset` for performance
3. **Expandable rows**: Show conflict details without leaving the page
4. **Summary cards**: Quick overview of conflict distribution
5. **Filter bar**: Match existing Courses page pattern for consistency

---

## Dependencies

- Existing UI components: `SearchInput`, `SearchableSelect`, `SessionDateFilter`, `LoadingSkeleton`, `EmptyState`
- Existing utilities: `formatTimeRange`, `conflictKindLabel`
- Existing types: `Session`, `Course`, `User`, `Room`
