# Failure Cases (Repo Memory)

This file records real failures as durable “system memory”.
Each entry should include a reproducible artifact when possible (test/fixture/replay/script).

Template:

## YYYY-MM-DD: <short title>

### Symptom

- What users/staff saw

### Root cause

- What invariant was violated, and where

### Fix

- What changed (small and specific)

### Regression / Memory

- Link to at least one: test, fixture, replay, script, rule

## 2026-05-24: Explicit-empty-roster fallback to course roster in scheduling preflight

### Symptom

- A course with enrolled students could not preflight a 0-student session (all students excluded or none included).
- The preflight endpoint fell back to checking the course roster's student busy ranges even when the user explicitly excluded all students.
- This produced false-positive student overlap conflicts for intentionally empty sessions.

### Root cause

- `preflightInput.StudentIDs` was `[]pgtype.UUID` — a non-nil empty slice was indistinguishable from "not provided".
- `preflightSlot()` used `len(in.StudentIDs) > 0` to decide between explicit-student vs course-roster checks.
- An empty-but-non-nil slice (from the HTTP handler always computing the effective roster) had `len() == 0`, triggering course-roster fallback.

### Fix

- Changed both `PreflightParams.StudentIDs` and `preflightInput.StudentIDs` from `[]pgtype.UUID` to `*[]pgtype.UUID`.
- `preflightSlot()` now checks:
  - `in.StudentIDs != nil` → explicit roster mode:
    - `len(*in.StudentIDs) > 0` → check explicit students for overlaps
    - `len(*in.StudentIDs) == 0` → skip student checks entirely (allow empty sessions)
  - `in.StudentIDs == nil` → use course-roster student overlap check (existing default)
- HTTP handler passes `nil` when no `included_student_ids`/`excluded_student_ids` are provided; passes computed effective roster when they are provided.

### Regression / Memory

- `TestPreflight_ExplicitEmptyRosterDoesNotFallbackToCourse` in `backend/internal/scheduling/service_integration_test.go`
- `TestPreflight_IncludedNonRosterStudentChecked` in `backend/internal/scheduling/service_integration_test.go`

## 2026-05-22: Sage CRM account-lock Continue flow discarded SID response

### Symptom

- When Sage CRM returns an account-lock interstitial page (e.g. "someone else is logged in" or too many rapid attempts), the Go CRM client correctly detected the page and clicked "Continue" via `followContinue`, but:
  - Login failed with `"account locked (continue followed); retrying login"` even though Continue had actually logged the user in.
  - All retries (up to 5 with 20s backoff) would hit the same lock page until the 15–30 min auto-unlock.
  - Manually inspecting the HTML after clicking Continue showed a **full CRM dashboard** with a valid `SID=145799055929449`, yet the client never extracted it.

### Root cause

- `followContinue` only consumed 2048 bytes of the Continue response via `io.ReadAll(io.LimitReader(resp.Body, 2048))` and **discarded the body**.
- Clicking "Continue" on the Sage CRM lock page actually completes the login — the server returns the main CRM dashboard HTML with a valid SID embedded in its URLs.
- After `followContinue` returned nil, `submitLoginForm` always returned `&accountLockedErr{...}`, triggering a retry instead of checking if a SID was now available.

### Fix

Two changes in `backend/internal/crmclient/client.go`:

1. **`followContinue` now reads the full response body and extracts the SID:**
   - Replaced `io.ReadAll(io.LimitReader(resp.Body, 2048))` with `io.ReadAll(resp.Body)`.
   - Runs `extractSID()` on the full body, fallback to request URL, then `Location` header.
   - If a SID is found, sets `c.sid` on the client.

2. **`submitLoginForm` checks if followContinue succeeded (SID found):**
   - After `followContinue` returns nil, checks `c.sid != ""`.
   - If SID is set → returns nil (login complete, no retry needed).
   - If SID is empty → returns `&accountLockedErr{}` to trigger retry loop.

### Key Insight (Sage CRM behavior)

When Sage CRM shows an interstitial after login POST (account locked / another session active), the page has an HTML `<form>` with a Continue button but **no `EWARE_USERID` hidden field**. This is how `isAccountLockedPage()` distinguishes it from the login form (which always carries `EWARE_USERID`). Submitting that Continue form returns the full dashboard HTML with a valid SID — the login is effectively complete.

### Regression / Memory

- Run `go run ./cmd/crmtest/main.go -step-login` against the live CRM.
- `isAccountLockedPage()` must return `false` for pages containing `eware_userid` (login form, "already logged on" form).
- If a lock page appears, `followContinue` must set `c.sid` from the Continue response (not trigger retries).

## 2026-05-21: TypeScript build blocked by nullable capacity check

### Symptom

- `./node_modules/.bin/tsc --noEmit` failed with `TS18047: 'cap' is possibly 'null'` in `src/pages/Classrooms.tsx`.

### Root cause

- A runtime check used `createForm.capacity.trim()` as the guard, but TypeScript did not narrow `cap` away from `null` for `cap <= 0`.

### Fix

- Guard on `cap != null` before numeric validation in `src/pages/Classrooms.tsx`.

### Regression / Memory

- Run `./node_modules/.bin/tsc --noEmit` to catch this class of issue.

## 2026-05-21: Schedule sessions never appear after creation

### Symptom

- After creating a session/series, the course schedule table shows “No sessions in range” even though creation succeeds.

### Root cause

- `SessionListByRange` and `SessionListActiveByRange` bound SQL parameters in the wrong order (passed `end` as `$1` and `start` as `$2`), in `backend/internal/db/sessions.sql.go`, producing an always-empty window for normal ranges.

### Fix

- Swap query arg order to pass `StartAt` then `EndAt` in `backend/internal/db/sessions.sql.go`.

### Regression / Memory

- Run `go test ./...` in `backend/` (scheduling integration tests exercise `SessionListActiveByRange`).
