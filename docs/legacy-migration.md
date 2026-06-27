# Legacy File Migration Guide

## Files

| File | Lines | Stack | Purpose |
|------|-------|-------|---------|
| `legacy.html` | 14,781 | ASP.NET MVC 5 + Bootstrap 3 + jQuery | Old course management frontend |
| `smsscappteapi.html` | 3,770 | Laravel + Bootstrap 4 + jQuery + DataTables | SmartSMS API testing interface |

## Issues Identified

### legacy.html
- Bootstrap 3 CDN (deprecated, no a11y support)
- Inline JS event handlers (`onclick`)
- No ARIA landmarks or roles
- Missing `lang="en"` (fixed in Phase 1b)
- Linked to `warwick.azurewebsites.net` — still referenced in `Courses.tsx` for legacy course links

### smsscappteapi.html
- Bootstrap 4 + jQuery (obsolete stack)
- Hardcoded Laravel CSRF token (`_token: "4kFQgxvknU5UTplGnsBN6sGL6A6OrpZ5W6xQaCMH"`) — security concern
- CDN scripts from unpkg, cdnjs, maxcdn (no SRI, stale versions)
- Inline JS (`onclick`, `onchange`)
- No focus management or ARIA
- SweetAlert2 for dialogs (no a11y)

## Migration Priority

1. **CSRF token rotation** — hardcoded token in `smsscappteapi.html` should be invalidated and the file removed from serving
2. **Legacy course links** — `Courses.tsx` links to `warwick.azurewebsites.net/Admin/Courses/Detail?id=...` — these should be migrated to the new React CourseDetail page
3. **Decommission legacy.html** — once all `/Admin/` routes are replicated in React, remove the static file from the deployment
4. **Decommission smsscappteapi.html** — API testing interface should be replaced by the new report/log pages or a dedicated API docs page via Swagger/OpenAPI

## Routes to Replicate

From `legacy.html` the following `/Admin/` routes exist:
- `/Admin/Courses` → React: `/courses` (done)
- `/Admin/Students` → React: `/students` (done)
- `/Admin/Teachers` → React: `/teachers` (done)
- `/Admin/Subjects` → React: `/subjects` (done)
- `/Admin/Classrooms` → React: `/classrooms` (done)

All covered in the current React app.
