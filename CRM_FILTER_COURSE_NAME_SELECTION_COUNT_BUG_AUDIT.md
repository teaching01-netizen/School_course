# CRM Filter Course Name Selection Count Bug — Audit Report

## Bug Report
> At CRM filter at Course Name it has a bug — sometimes I only select 5 course names but at summary it shows 6 selections.

## Change Chain (Discovered Graph)

```
UI (MultiSelect checkboxes)
  ↑ checked={safeSelected.includes(opt)}
  ↑ onChange={() => toggle(opt)}
  ↑
MultiSelect.toggle(v)
  → safeSelected.includes(v) ? filter out : append
  → onChange(newArray)
  ↑
CrmFilterPanel state
  → filter.course_name_values (React useState)
  → setFilter((f) => ({ ...f, course_name_values: v }))
  ↑
Button text: safeSelected.length + " selected"
  ↑
Preview: computePreview(filter) → POST /api/v1/courses/{id}/crm-filter/preview
  ↑
Save: PUT /api/v1/courses/{id}/crm-filter → body: { enabled, filter }
  ↑
Backend: handleCourseFilterPut
  → CRMReconcileV2.SetCourseFilterAndEnqueueApply(ctx, worker, courseID, body.Enabled, string(body.Filter))
  → SQL: UPDATE courses SET crm_filter = $3::jsonb WHERE id = $1
  (NO Normalize() call before saving!)
  ↑
Load on mount: GET /api/v1/courses/{id}/crm-filter
  → setFilter({ ...defaultFilter, ...res.filter })
  ↑
Backend: handleCourseFilterGet
  → CRMReconcileV2.GetCourseFilterState(ctx, courseID)
  → Returns raw JSON from crm_filter column

--- Options source chain ---

GET /api/v1/crm/options
  → handleCrmOptions
  → CrmDistinctOptions(ctx, snapshotID)
  → SQL: jsonb_agg(DISTINCT course_name ORDER BY course_name) FILTER (WHERE course_name IS NOT NULL)
  → Returns JSON array of unique course names
  ↑
Frontend: loadOptions() → setOptions(opts)
  → MultiSelect options={options?.course_names ?? []}
```

## Root Causes Identified

### BUG 1 — Backend save path does NOT normalize/deduplicate filter values (HIGH)

**Location:** `backend/internal/httpapi/crmhttp/routes.go:310-351` + `backend/internal/crmimport/reconcile/reconcile.go:1039-1058`

**Evidence:**
- `handleCourseFilterPut` receives the raw filter JSON from the frontend and passes it directly to `SetCourseFilterAndEnqueueApply`
- `SetCourseFilterAndEnqueueApply` stores the filter JSON directly in the database: `SET crm_filter = $3::jsonb`
- **`filter.Normalize()` is NEVER called on the save path**
- `Normalize()` IS called in `PreviewCountForFilter` (line 832) and `queryDesiredStudentsV2` (line 165) but NOT in the save path

**Impact:** If the frontend ever sends duplicate or whitespace-malformed values in `course_name_values`, they are persisted as-is. When the filter is later loaded back, the MultiSelect button count (`safeSelected.length`) reflects the raw (un-normalized) array length.

**What Normalize() does (crmtypes/types.go:41-79):**
```go
func (f *CourseFilter) Normalize() {
    // trims whitespace, deduplicates, removes empty strings, sorts
}
```

### BUG 2 — SQL options query doesn't btrim course_name before DISTINCT (MEDIUM)

**Location:** `backend/internal/db/crm_rows.sql.go:14-22`

**Evidence:**
```sql
-- course_name: NO btrim, NO empty-string filter
COALESCE(jsonb_agg(DISTINCT course_name ORDER BY course_name) FILTER (WHERE course_name IS NOT NULL), '[]'::jsonb) AS course_names,

-- academic_level: HAS btrim + empty-string filter
COALESCE(jsonb_agg(DISTINCT academic_level ORDER BY academic_level) FILTER (WHERE academic_level IS NOT NULL AND btrim(academic_level) <> ''), '[]'::jsonb) AS academic_levels,

-- secondary_school: HAS btrim + empty-string filter
COALESCE(jsonb_agg(DISTINCT secondary_school ORDER BY secondary_school) FILTER (WHERE secondary_school IS NOT NULL AND btrim(secondary_school) <> ''), '[]'::jsonb) AS secondary_schools,
```

**Impact:** CRM data with trailing/leading whitespace (e.g., "SAT Math" vs "SAT Math ") would appear as TWO separate options in the dropdown. A user checking both would see "2 selected" for what looks like one course name. This also means an empty-string course name would appear as an option.

### BUG 3 — Frontend MultiSelect has no client-side dedup guard (LOW)

**Location:** `src/components/crm/CrmFilterPanel.tsx:106-112`

**Evidence:**
```tsx
const toggle = (v: T) => {
    if (safeSelected.includes(v)) {
      onChange(safeSelected.filter((x) => x !== v));
    } else {
      onChange([...safeSelected, v]);
    }
};
```

- Uses `Array.includes()` with strict equality
- No `Set`-based dedup on the result before calling `onChange`
- If options contain visually identical but string-different values, both can be independently selected
- The button count (`safeSelected.length`) would show the raw array length including duplicates

## How The Bug Manifests

1. CRM XLSX import parses course names using `strings.TrimSpace()` (xlsx_parse.go:61), but this only trims ASCII whitespace — non-breaking spaces and some Unicode whitespace pass through
2. The `CrmDistinctOptions` SQL query returns `DISTINCT course_name` without `btrim()`, so near-duplicate names (different whitespace) appear as separate options
3. User sees what looks like 5 unique course names but one appears twice with subtle whitespace differences
4. User checks all 5 visible unique names, but the duplicate is also checked (or the dropdown shows 6 items, 5 of which the user intends to check)
5. The MultiSelect button displays `safeSelected.length` = 6 instead of expected 5

**OR:**

1. User previously saved a filter that contained duplicates (because the save path doesn't normalize)
2. On reload, the filter loads 6 items from the database
3. User sees "6 selected" even though they only intended 5

## Must Change (if fixing)

| # | File | What | Why |
|---|------|------|-----|
| 1 | `backend/internal/httpapi/crmhttp/routes.go` | Call `filter.Normalize()` and `filter.Validate()` in `handleCourseFilterPut` before persisting | Deduplicates, trims, and validates filter values before storage |
| 2 | `backend/internal/db/crm_rows.sql` | Add `btrim()` and empty-string filter to `course_name` in `CrmDistinctOptions`, matching `academic_level`/`secondary_school` pattern | Prevents near-duplicate options from appearing in dropdown |
| 3 | `backend/internal/db/crm_rows.sql.go` | Regenerate sqlc after SQL change | Keeps generated code in sync |
| 4 | `src/components/crm/CrmFilterPanel.tsx` | Add client-side dedup in `toggle()` or use `Set` for `safeSelected` | Defense-in-depth against duplicates reaching the array |
| 5 | `backend/internal/crmimport/types_test.go` | Add test for Normalize dedup behavior with whitespace variants | Prevents regression |

## Must Verify

- [ ] Selecting N course names shows exactly "N selected" in button text
- [ ] Saving and reloading a filter preserves the same selection count
- [ ] Near-duplicate course names (with trailing spaces) from CRM data don't appear as separate options
- [ ] Empty course names from CRM data don't appear as selectable options
- [ ] Preview count still matches expected distinct student count after fix
- [ ] Existing filters with duplicates are handled gracefully on load

## Files In Chain (Not Exhaustive)

- `src/components/crm/CrmFilterPanel.tsx` — MultiSelect component + filter panel UI
- `src/components/crm/CrmFilterPanel.test.tsx` — Existing tests
- `backend/internal/httpapi/crmhttp/routes.go` — HTTP handlers for CRM filter CRUD
- `backend/internal/db/crm_rows.sql.go` — Generated SQL for CrmDistinctOptions
- `backend/internal/crmimport/crmtypes/types.go` — CourseFilter type + Normalize()
- `backend/internal/crmimport/reconcile/reconcile.go` — PreviewCountForFilter, SetCourseFilterAndEnqueueApply
- `backend/internal/crmimport/filter_builder.go` — SQL WHERE condition builder
- `backend/internal/crmimport/xlsx/xlsx_parse.go` — XLSX import with TrimSpace on course_name
