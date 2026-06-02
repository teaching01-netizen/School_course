---
phase: absence-hard-delete-review
reviewed: 2026-06-02T00:00:00Z
depth: deep
files_reviewed: 3
files_reviewed_list:
  - backend/internal/httpapi/absenceshttp/management_routes.go
  - backend/internal/httpapi/absenceshttp/dispatch.go
  - backend/db/migrations/00031_absence_audit_cascade_delete.sql
findings:
  critical: 2
  warning: 4
  info: 2
  total: 8
status: issues_found
---

# Phase: Absence Hard Delete — Code Review Report

**Reviewed:** 2026-06-02T00:00:00Z
**Depth:** deep
**Files Reviewed:** 3
**Status:** issues_found

## Summary

Review of the hard-delete feature for absence submissions across three files: the handler (`handleAbsenceDelete`), the route dispatch, and the trigger migration. The implementation is structurally sound — admin auth, idempotency via `WithIdempotentTx`, UUID validation, and transactional safety are all present. However, two significant deviations from established project patterns introduce correctness risks, and four additional quality issues were identified. The migration correctly handles the append-only trigger's interaction with CASCADE deletes.

## Critical Issues

### CR-01: Missing optimistic concurrency control — no `expected_version` check

**File:** `backend/internal/httpapi/absenceshttp/management_routes.go:1303-1345`
**Issue:** The `handleAbsenceDelete` handler does not require or validate `expected_version`. Every other mutation handler in this module (`handleAbsenceStatusUpdate` at line 595, `handleAbsenceNotesUpdate` at line 698, `handleSitInOverride` at line 774) enforces optimistic concurrency by requiring `expected_version` in the request body and rejecting stale edits with a `409 stale_edit` response. The delete handler reads no request body at all.

This creates a real race condition: Admin A loads an absence (version=1), Admin B edits it (version bumps to 2), then Admin A clicks Delete. The delete succeeds despite Admin B's changes being silently destroyed. Because this is an **irreversible hard delete**, the stakes are higher than for a status edit (which can be reverted).

The project's `CONTEXT.md` explicitly requires optimistic concurrency for mutations: *"Concurrency (v1): require optimistic concurrency for series/session edits (e.g., version or updated_at precondition)."*

**Fix:** Add `expected_version` to the request body and validate it, consistent with the other handlers:

```go
func (s *server) handleAbsenceDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid absence ID")
		return
	}
	var body struct {
		ExpectedVersion *int32 `json:"expected_version"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.ExpectedVersion == nil || *body.ExpectedVersion < 1 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_expected_version", "expected_version is required")
		return
	}
	adminID := actorID(user.ID)
	s.a.WithIdempotentTx(w, r, user.ID, "absences", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		current, err := qtx.ManagedAbsenceGet(r.Context(), id)
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		if current.Version != *body.ExpectedVersion {
			s.writeStaleAbsence(w)
			return 0, nil, pgx.ErrNoRows
		}
		if current.Status == "cancelled" {
			s.a.WriteErr(w, http.StatusConflict, "already_cancelled", "This absence is already cancelled")
			return 0, nil, fmt.Errorf("already cancelled")
		}
		tag, err := tx.Exec(r.Context(), `DELETE FROM student_absences WHERE id = $1`, id)
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		if tag.RowsAffected() == 0 {
			s.writeStaleAbsence(w)
			return 0, nil, pgx.ErrNoRows
		}
		// ... audit insert unchanged
	})
}
```

This also addresses CR-02 below by adding the `RowsAffected` check and using the version check to distinguish "not found" from "stale edit."

---

### CR-02: Raw `DELETE` does not check `RowsAffected` — silent no-op on concurrent delete

**File:** `backend/internal/httpapi/absenceshttp/management_routes.go:1329`
**Issue:** The DELETE statement uses `tx.Exec` without checking `RowsAffected()`. If a concurrent request deletes the absence between the `ManagedAbsenceGet` check (line 1316) and the DELETE (line 1329), the DELETE affects 0 rows but returns no error. The handler then writes an audit log entry for a non-existent absence and returns `200 {"status": "deleted"}`.

While `ManagedAbsenceGet` prevents the 404 case on replay (it would fail first), the TOCTOU window between the GET and DELETE is real under concurrent load. The result: a misleading audit log entry `"absence.hard_deleted"` for an absence that was already deleted, and a 200 response to the client implying the request succeeded.

Compare with `handleAbsenceStatusUpdate` (line 626-634) which checks `sqldb.IsNoRows(err)` on the UPDATE result to detect stale edits.

**Fix:** Check `RowsAffected()` after the DELETE. If 0 rows affected, return a conflict/stale-edit error. (This is already shown in the CR-01 fix above.)

---

## Warnings

### WR-01: Absence-specific audit trail permanently destroyed with no pre-delete snapshot

**File:** `backend/internal/httpapi/absenceshttp/management_routes.go:1329-1344`
**Issue:** The handler writes to the **global** audit log (`qtx.AuditInsert`, line 1335) but does NOT write to the **absence-specific** audit timeline (`qtx.AbsenceAuditInsert`). The absence-specific timeline rows in `absence_audit_log` are CASCADE-deleted when `student_absences` is deleted.

The global audit log records `{absence_id, wcode}` but loses the full history: status transitions (submitted→reviewed→actioned), admin notes, sit-in overrides, OTP events, etc. For a system that maintains detailed absence timelines for accountability, this is a significant data loss gap.

The migration 00031 specifically modifies the trigger to *allow* this cascade to happen — meaning the intent is to destroy the timeline. But no pre-delete snapshot is preserved.

**Fix:** Before executing the DELETE, snapshot the absence timeline into the global audit log:

```go
// Inside the transaction, BEFORE the DELETE:
timeline, tErr := qtx.AbsenceAuditList(r.Context(), id)
if tErr == nil && len(timeline) > 0 {
    // Serialize the full timeline into the global audit entry
    timelineJSON, _ := json.Marshal(s.timelineDTO(timeline))
    details["deleted_timeline"] = json.RawMessage(timelineJSON)
}
```

Or alternatively, insert a synthetic summary of the absence's final state into the global audit log's `details` payload.

---

### WR-02: Raw SQL in handler bypasses the query layer — inconsistent with all other handlers

**File:** `backend/internal/httpapi/absenceshttp/management_routes.go:1329`
**Issue:** The DELETE uses a raw SQL string:
```go
tx.Exec(r.Context(), `DELETE FROM student_absences WHERE id = $1`, id)
```

Every other mutation in this module goes through `qtx` (the sqlc-generated query layer): `qtx.AbsenceStatusUpdate`, `qtx.AbsenceNotesUpdate`, `qtx.AbsenceSitInUpdate`, `qtx.AbsenceSitInsReplace`, etc. The raw SQL bypasses any query-layer instrumentation, logging, or future refactoring. It also makes the schema dependency invisible to sqlc's query tracking.

While there's no SQL injection risk (the query is parameterized with a validated UUID), this inconsistency means:
- Schema changes won't trigger sqlc regeneration warnings
- Query-level metrics/logging won't capture the delete
- Future developers may not find this query when auditing the data access layer

**Fix:** Add a custom query method in `absence_management_custom.go`:

```go
func (q *Queries) AbsenceHardDelete(ctx context.Context, id pgtype.UUID) (pgconn.CommandTag, error) {
    tag, err := q.db.Exec(ctx, `DELETE FROM student_absences WHERE id = $1`, id)
    if err != nil {
        return pgconn.CommandTag{}, err
    }
    return tag, nil
}
```

Then call `qtx.AbsenceHardDelete(r.Context(), id)` in the handler.

---

### WR-03: Migration trigger modification allows all cascade paths to bypass append-only protection

**File:** `backend/db/migrations/00031_absence_audit_cascade_delete.sql:7-15`
**Issue:** The migration replaces the `reject_absence_audit_mutation()` function to allow deletes when `pg_trigger_depth() > 1`:

```sql
IF pg_trigger_depth() > 1 THEN
    RETURN OLD;
END IF;
```

This correctly handles the `student_absences` → `absence_audit_log` CASCADE path. However, `pg_trigger_depth() > 1` is a blanket exemption for *any* cascade chain that reaches `absence_audit_log`. If a future migration or table adds another FK to `absence_audit_log` (e.g., a shared audit table pattern), deletes through that path would also bypass the append-only protection.

Currently the only FK referencing `absence_audit_log` is `absence_id` from `student_absences`, so this is safe today. But the trigger is now permissive of any second-level cascade.

**Fix:** This is acceptable for now given the current schema. Add a comment to the migration documenting the assumption:

```sql
-- NOTE: pg_trigger_depth() > 1 allows cascade deletes from any parent table.
-- The only parent table today is student_absences. If new FKs are added to
-- absence_audit_log, verify they should also bypass append-only protection.
```

---

### WR-04: No guard against deleting "actioned" absences that may have downstream effects

**File:** `backend/internal/httpapi/absenceshttp/management_routes.go:1322-1325`
**Issue:** The handler only blocks deletion of `cancelled` absences. Absences in `actioned` status (which have sit-in assignments and possibly sent SMS notifications) can be hard-deleted with no warning or guard. After deletion:
- `absence_sit_ins` rows are CASCADE-deleted, orphaning the sit-in session references
- SMS notifications that referenced this absence become untraceable beyond the global audit log
- The sit-in sessions themselves still exist but their link to the original absence is destroyed

While `absence_sit_ins.session_id` has `ON DELETE CASCADE` for session deletion (not the reverse), the absence_sit_ins rows are deleted via `absence_id` FK cascade. The sessions remain, but the sit-in assignment is lost.

**Fix:** Consider either:
1. Restricting hard delete to `pending`/`reviewed` statuses only, or
2. Adding a confirmation flag (e.g., `"force": true` in the body) for `actioned` absences with a business rule explanation in the response, or
3. At minimum, including the status in the audit log payload for traceability:

```go
details := map[string]any{
    "absence_id": r.PathValue("id"),
    "wcode":      current.Wcode,
    "status":     current.Status,  // was the absence pending/reviewed/actioned?
}
```

---

## Info

### IN-01: Test coverage is shallow — only route dispatch is tested

**File:** `backend/internal/httpapi/absenceshttp/routes_test.go:11-49`
**Issue:** The three tests (`TestDispatchDelete_RouteRegistered`, `TestDispatchDelete_WrongMethod`, `TestDispatchDelete_NotFoundOnSubpath`) only verify that the route is registered and dispatches correctly. They use a `&server{}` with nil dependencies, so the tests only check that the handler is called — not that it works correctly.

There are no tests for:
- Successful deletion (admin auth + valid ID + correct version)
- Missing Idempotency-Key (should return 400)
- Stale version (should return 409)
- Already-cancelled absence (should return 409)
- Non-existent absence (should return 404)
- Concurrent delete race condition

**Fix:** Add integration tests (or at minimum handler-level tests with mocked DB) covering the happy path, error paths, and concurrency scenarios.

---

### IN-02: The `DELETE` HTTP method typically has no body — idempotency fingerprint uses empty body

**File:** `backend/internal/httpapi/httpadapter/adapter.go:244-250`
**Issue:** `WithIdempotentTx` calls `ReadBodyBytes(r)` and computes a fingerprint from the body. For a DELETE request (which typically has no body per HTTP spec), the fingerprint is `SHA256("DELETE" + ":" + path + "?" + query + ":" + "")`. This works correctly, but the `WithIdempotentTx` wrapper was designed for POST/PUT/PATCH requests that always have a body. Using it for DELETE means:

- The fingerprint is less discriminating (all DELETE requests to the same path have the same fingerprint)
- If the handler is changed to accept a body in the future (e.g., for `expected_version`), the fingerprint changes, breaking existing idempotency keys

With the CR-01 fix (adding `expected_version` body), the fingerprint would include the body and become properly discriminating. This is another reason to implement CR-01.

**Fix:** Addressed by implementing CR-01 (adding request body with `expected_version`).

---

## Schema Cascade Analysis

Verified the CASCADE chain for `DELETE FROM student_absences WHERE id = $1`:

| Table | FK Column | FK Target | On Delete | Effect |
|-------|-----------|-----------|-----------|--------|
| `absence_sit_ins` | `absence_id` | `student_absences(id)` | CASCADE | Rows deleted ✓ |
| `absence_missed_sessions` | `absence_id` | `student_absences(id)` | CASCADE | Rows deleted ✓ |
| `absence_audit_log` | `absence_id` | `student_absences(id)` | CASCADE | Rows deleted ✓ (trigger fixed by migration 00031) |
| `student_parent_verification_sessions` | `consumed_absence_id` | `student_absences(id)` | SET NULL | Column nulled ✓ |

No cascade chains extend beyond the first level (deleting `absence_missed_sessions` or `absence_sit_ins` rows does NOT cascade to `sessions` because those FKs cascade in the *referenced* direction — session deletion cascades to child tables, not the reverse). The cascade is safe and complete.

---

## Migration Review (00031)

The migration is correct and minimal:
- Uses `CREATE OR REPLACE FUNCTION` so existing triggers automatically pick up the new logic
- `pg_trigger_depth() > 1` is the standard PostgreSQL idiom for distinguishing direct vs. cascade operations
- Down migration correctly reverts to the strict version
- The `+goose StatementBegin/End` is NOT used (correctly) because `CREATE OR REPLACE FUNCTION` is a single statement

No issues found in the migration itself.

---

## Route Registration Review (dispatch.go:118-120)

```go
case len(parts) == 1 && r.Method == http.MethodDelete:
    s.handleAbsenceDelete(w, r)
    return
```

The routing is correct:
- Only matches `DELETE /api/v1/absences/{id}` (single path segment)
- Sets `r.PathValue("id", parts[0])` at line 92 (in the `default` case)
- No ambiguity with other routes (status, notes, sit-in all require 2 path segments)
- `POST /api/v1/absences/{id}` correctly returns 405 at line 98

No issues found in dispatch.

---

_Reviewed: 2026-06-02T00:00:00Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: deep_
