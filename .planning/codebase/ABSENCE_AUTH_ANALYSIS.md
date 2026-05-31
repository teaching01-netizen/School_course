# Absence Endpoint Auth Analysis

**Analysis Date:** 2026-05-30

## Summary

The backend uses **per-handler auth** rather than middleware-level auth. No global `RequireAuth` middleware wraps the router. Auth is enforced by each handler function calling `s.a.MustUser()` or `s.a.MustAdmin()`. The student-facing absence endpoints are **intentionally public (unauthenticated)** — this matches the frontend routing where `/absence` is outside `RequireAuth`.

---

## 1. Route Registration

**File:** `backend/internal/httpapi/absenceshttp/routes.go`

All absence routes are registered on a bare `*http.ServeMux` (Go 1.22+ pattern routing) at lines 30–54:

```go
func Register(mux *http.ServeMux, deps httpdeps.Deps) {
    s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

    mux.HandleFunc("GET /api/v1/courses/public", s.handleCoursesPublic)

    mux.HandleFunc("GET /api/v1/absence-form-config", s.handleFormConfigGet)        // line 32 — PUBLIC
    mux.HandleFunc("/api/v1/absences", s.handleAbsencesDispatch)                     // line 33 — PUBLIC (dispatch)
    mux.HandleFunc("/api/v1/absences/", s.handleAbsencesDispatch)                    // line 34 — PUBLIC (dispatch)

    // Admin endpoints for absence policies
    mux.HandleFunc("GET /api/v1/admin/absence-policies", s.handlePoliciesGet)        // line 37 — ADMIN
    mux.HandleFunc("PUT /api/v1/admin/absence-policies", s.handlePoliciesUpdate)     // line 38 — ADMIN
    mux.HandleFunc("GET /api/v1/admin/absence-settings", s.handleAbsenceSettingsGet) // line 39 — ADMIN
    mux.HandleFunc("PUT /api/v1/admin/absence-settings", s.handleAbsenceSettingsUpdate) // line 40 — ADMIN

    // Staff-side operational absence workflow.
    mux.HandleFunc("GET /api/v1/absences/stats", s.handleAbsenceStats)               // line 43 — ADMIN
    mux.HandleFunc("GET /api/v1/absences/dashboard", s.handleAbsenceDashboard)       // line 44 — ADMIN
    mux.HandleFunc("GET /api/v1/absences/export", s.handleAbsenceExport)             // line 45 — ADMIN
    mux.HandleFunc("POST /api/v1/absences/batch-status", s.handleBatchStatus)        // line 46 — ADMIN
    mux.HandleFunc("GET /api/v1/absences/{id}", s.handleAbsenceGet)                  // line 47 — ADMIN
    mux.HandleFunc("GET /api/v1/absences/{id}/timeline", s.handleAbsenceTimeline)    // line 48 — ADMIN
    mux.HandleFunc("GET /api/v1/absences/{id}/sit-in-candidates", s.handleSitInCandidates) // line 49 — ADMIN
    mux.HandleFunc("PUT /api/v1/absences/{id}/status", s.handleAbsenceStatusUpdate)  // line 50 — ADMIN
    mux.HandleFunc("PUT /api/v1/absences/{id}/notes", s.handleAbsenceNotesUpdate)    // line 51 — ADMIN
    mux.HandleFunc("PUT /api/v1/absences/{id}/sit-in", s.handleSitInOverride)        // line 52 — ADMIN

    mux.HandleFunc("GET /api/v1/operations/calendar", s.handleCalendar)              // line 54 — ADMIN
}
```

**Registration call site:** `backend/internal/httpapi/handler.go` line 110:
```go
absenceshttp.Register(mux, deps)
```

### Route Dispatch: `/api/v1/absences/student-lookup`

`student-lookup` is **not** a standalone route — it goes through the dispatch handler.

**File:** `backend/internal/httpapi/absenceshttp/dispatch.go`, lines 8–117

The routes `/api/v1/absences` and `/api/v1/absences/` both route to `handleAbsencesDispatch`, which does manual path parsing:

```go
func (s *server) handleAbsencesDispatch(w http.ResponseWriter, r *http.Request) {
    const prefix = "/api/v1/absences"
    path := strings.TrimPrefix(r.URL.Path, prefix)
    // ...
    switch parts[0] {
    case "student-lookup":       // line 33
        if r.Method == http.MethodGet {
            s.handleStudentLookup(w, r)    // <— delegates to handler
            return
        }
    case "sit-in-options":       // line 38
    case "sessions-in-range":    // line 43
    // ... more sub-routes, including admin endpoints
    }
}
```

So `GET /api/v1/absences/student-lookup` → dispatch → `handleStudentLookup`.

---

## 2. Auth Enforcement — Per-Handler Pattern

**There is no auth middleware wrapping the entire mux.** The only wrapper is `withRequestTimeout` (timeout only):

**File:** `backend/internal/httpapi/handler.go` line 152:
```go
return withRequestTimeout(mux)
```

**File:** `backend/internal/httpapi/timeout.go` lines 15–27:
```go
func withRequestTimeout(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Sets 10s for GET, 15s for writes — NO auth logic
    })
}
```

### Auth mechanism is per-handler via the `httpadapter.Adapter`:

**File:** `backend/internal/httpapi/httpadapter/adapter.go`

| Method | Behavior |
|--------|----------|
| `MustUser(w, r)` (line 63) | Calls `a.auth.RequireUser()` which validates `__Host-warwick_session` cookie. Returns `401` if missing/invalid. |
| `MustAdmin(w, r)` (line 76) | Calls `MustUser` then checks `Role == "Admin"`. Returns `401` or `403`. |

Both ultimately call `auth.Service.RequireUser()`:

**File:** `backend/internal/auth/service.go` line 246:
```go
func (s *Service) RequireUser(ctx context.Context, r *http.Request) (User, error) {
    c, err := r.Cookie("__Host-warwick_session")
    // validates session in DB, checks expiry/idle timeout/password version
}
```

### Endpoint auth classification:

| Endpoint | Handler Function | Auth Check | File:Line |
|----------|-----------------|------------|-----------|
| `GET /api/v1/absence-form-config` | `handleFormConfigGet` | **None** — public | `management_routes.go:1116` |
| `GET /api/v1/absences/student-lookup` | `handleStudentLookup` | **None** — public | `routes.go:529` |
| `GET /api/v1/absences/sit-in-options` | `handleSitInOptions` | **None** — public | `routes.go:574` |
| `GET /api/v1/absences/sessions-in-range` | `handleSessionsInRange` | **None** — public | `routes.go:617` |
| `POST /api/v1/absences` (create) | `handleAbsenceCreate` | CSRF origin only, **no session auth** | `routes.go:140` |
| `POST/GET parent-verification/*` | `handleParentVerification*` | CSRF + rate limit, **no session auth** | `pending_routes.go` |
| `GET /api/v1/absences` (inbox) | `handleAbsenceInbox` | `MustAdmin` | `management_routes.go:340` |
| `GET /api/v1/absences/{id}` | `handleAbsenceGet` | `MustAdmin` | `management_routes.go:415` |
| `GET /api/v1/absences/stats` | `handleAbsenceStats` | `MustAdmin` | `management_routes.go:813` |
| `GET /api/v1/absences/dashboard` | `handleAbsenceDashboard` | `MustAdmin` | `management_routes.go:910` |
| `GET /api/v1/absences/export` | `handleAbsenceExport` | `MustAdmin` | `management_routes.go:939` |
| `POST /api/v1/absences/batch-status` | `handleBatchStatus` | `MustAdmin` | `management_routes.go:826` |
| `PUT /api/v1/absences/{id}/status` | `handleAbsenceStatusUpdate` | `MustAdmin` | `management_routes.go` |
| `PUT /api/v1/absences/{id}/notes` | `handleAbsenceNotesUpdate` | `MustAdmin` | `management_routes.go` |
| `PUT /api/v1/absences/{id}/sit-in` | `handleSitInOverride` | `MustAdmin` | `management_routes.go` |

---

## 3. Analysis: Should `student-lookup` / `absence-form-config` Require Auth?

### Frontend confirms these are intentionally public

**File:** `src/App.tsx` lines 57–58:
```tsx
<Route path="/login" element={<Login />} />
<Route path="/absence" element={<AbsenceForm />} />           {/* OUTSIDE RequireAuth — PUBLIC */}
<Route element={<RequireAuth />}>
    {/* all admin routes are inside this wrapper */}
</Route>
```

The `AbsenceForm` page calls these endpoints:
- `src/pages/AbsenceForm.tsx` line 183: `apiJson<AbsenceFormConfig>("/api/v1/absence-form-config", ...)`
- `src/pages/AbsenceForm.tsx` line 487: `/api/v1/absences/student-lookup?wcode=...`

### Design rationale

These endpoints are public **by design**:

1. **Student/Parent submits absences without logging in** — no student portal in v1.
2. **Wcode acts as a weak identifier** — you must know the student's wcode to look them up (not guessable).
3. **Protections at write time, not read time:**
   - `POST /api/v1/absences` (create) requires: CSRF origin check + rate limiting + OTP/parent verification (SMS)
   - The read endpoints (`student-lookup`, `absence-form-config`, `sessions-in-range`, `sit-in-options`) are low-risk — they only return course/session data scoped to a known wcode.
4. **CORS/CSRF**: The `requestOriginAllowed` check at `pending_routes.go:56–83` validates Origin/Referer headers against `AppOrigin` config — applied to `POST` but not `GET` endpoints (GET is not state-mutating).

### Backend contract alignment

CONTEXT.md states: *"Authorization enforcement (v1): centralized in Go API (application-layer), not DB RLS."*

This is upheld: all **admin** handlers explicitly call `MustAdmin()` at the Go layer. The student-facing handlers intentionally omit this because they serve unauthenticated users. The contract does not mandate that *every* endpoint requires auth — just that auth decisions are made in Go code (not DB RLS). The public absence endpoints satisfy this by *deliberately choosing* no auth at the application layer, backed by CSRF + rate limiting + OTP for writes.

### Risk assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| Wcode enumeration via `student-lookup` | Low | Rate-limited; requires knowing a valid wcode |
| Session schedule data leaked via `sessions-in-range` | Low | Requires valid wcode + date range; no PII returned |
| Form config is public | None | Contains UI hints (categories, max days) — not sensitive |
| Absence creation without auth | Medium | Mitigated by CSRF origin check + rate limiting + SMS OTP verification |
| Admin endpoints access | None | All guarded by `MustAdmin` (cookie session auth) |

### Recommendation

No change needed. The current design is correct for the stated v1 scope (no student portal, admin-only backend UI, public student absence submission form).
