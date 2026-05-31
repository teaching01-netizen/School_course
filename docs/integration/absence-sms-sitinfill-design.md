# Absence → CRM → SMS → Sit-In Pre-Fill Integration Design

**Date:** 2026-05-30
**Author:** Systems Integration Architect
**Status:** Design Draft

---

## 1. Complete Revised Flow Diagram

```
FRONTEND (React SPA)                        BACKEND (Go modular monolith)                         EXTERNAL
                                                                                                  
┌──────────────────────────────┐           ┌──────────────────────────────────────────┐           ┌──────────────┐
│  AbsenceForm.tsx             │           │  absenceshttp/routes.go                    │           │              │
│                              │           │                                            │           │  SmartSMS    │
│  Step 0: Student Lookup      │           │  ┌──────────────────────────────────────┐  │           │  Platform    │
│  │ W-Code input              │──GET─────▶│  │ handleStudentLookup()                │  │           │              │
│  │                           │◀──────────│  │  → Q.StudentSubjectByWCode(wcode)    │  │           └──────────────┘
│  ▼                           │           │  └──────────────────────────────────────┘  │                 ▲
│                              │           │                                            │                 │
│  Step 1: Quick-Select        │           │  ┌──────────────────────────────────────┐  │                 │
│  │ Date range picker         │──GET─────▶│  │ handleSessionsInRange()              │  │                 │
│  │ Session grid             │◀──────────│  │  → raw SQL session + absence query    │  │                 │
│  │ Reason category          │           │  └──────────────────────────────────────┘  │                 │
│  ▼                           │           │                                            │                 │
│                              │           │  ┌──────────────────────────────────────┐  │                 │
│  Step 2: Sit-in Plan         │           │  │ handleSitInOptions()                 │  │                 │
│  │ Per-subject sit-in card  │──GET─────▶│  │  → resolveSitIn()                    │  │                 │
│  │ Show pre-filled org data │◀──────────│  │  → Q.CrmRowParentPhoneByWcode() [NEW] │  │                 │
│  │ (from CRM - parent name) │           │  └──────────────────────────────────────┘  │                 │
│  ▼                           │           │                                            │                 │
│                              │           │  ┌──────────────────────────────────────┐  │                 │
│  Step 3: Confirm             │           │  │ handleAbsenceCreate()                │  │                 │
│  │ Summary + Submit         │──POST────▶│  │   1. validate inputs                  │  │                 │
│  │                           │           │   2. lookup student                     │  │                 │
│  │                           │           │   3. find enrolled course               │  │                 │
│  │                           │           │   4. Q.CrmRowParentPhoneByWcode() [NEW]  │  │                 │
│  │                           │           │   5. BEGIN TX                           │  │                 │
│  │                           │           │   6. AbsenceCreate()                    │  │                 │
│  │                           │           │   7. AbsenceSetSubmissionMetadata()     │  │                 │
│  │                           │           │   8. AbsenceSitInsCreate()              │  │                 │
│  │                           │           │   9. AbsenceAuditInsert()               │  │                 │
│  │                           │           │   10. COMMIT TX                         │  │                 │
│  │                           │           │   11. ← SMS SEND (after commit)         │──POST──────────▶│
│  │                           │           │   12. return 201 + sms_status            │                 │
│  │                           │◀──────────│                                            │                 │
│  ▼                           │           │                                            │                 │
│                              │           │  ┌──────────────────────────────────────┐  │                 │
│  Post-submit screen          │           │  │ handleSitInCandidates()              │  │                 │
│  │ Shows SMS status badge   │           │  │  → Q.SitInCandidateSessions()         │  │                 │
│  │ "SMS sent to parent"     │           │  └──────────────────────────────────────┘  │                 │
│  │ or "SMS unavailable"     │           │                                            │                 │
└──────────────────────────────┘           └──────────────────────────────────────────┘
```

## 2. Data Flow

### 2.1 Parent Phone Resolution

**Where it's queried:** New SQL query via `crm_rows` table, keyed by `wcode`.

```sql
-- db/queries/crm_rows.sql (NEW QUERY)
-- name: CrmRowParentPhoneByWcode :one
SELECT parent_name, parent_phone, parent_email
FROM crm_rows
WHERE wcode = $1
  AND snapshot_id = (SELECT active_snapshot_id FROM crm_state WHERE singleton = true)
LIMIT 1;
```

**When queried:** Two points:
1. **GET /sit-in-options** — to pre-fill parent contact info in sit-in organizer card
2. **POST /absences** — during absence creation, to capture phone for SMS sending

**How phone is captured:** On absence creation, the handler stores the resolved phone into `student_absences.student_phone` (column already exists but currently never set).

### 2.2 SMS Send Trigger

**When:** After the DB transaction commits successfully (step 10 in handler), SMS is sent asynchronously but within the HTTP request (not fire-and-forget via goroutine — see transaction boundary reasoning below).

**Flow:**
```
1. tx.Commit() succeeds
2. if parent_phone is not empty:
     a. Format SMS message
     b. deps.SMS.SendSMS(ctx, req)
     c. On success: include sms_status: "sent" in 201 response
     d. On failure: include sms_status: "failed", sms_error: "..." in 201 response
     e. Log audit trail for SMS attempt
3. Return 201 response
```

**SMS Message Template:**
```
[Institute] Absence reported for {student_name} ({wcode})
Subject: {subject_name}
Dates: {date_from} - {date_to}
Reason: {reason_category}
Sit-in: {sit_in_method} {sit_in_course_code}
```

### 2.3 Frontend → Backend Data Flow (POST /absences)

**Request body — new optional fields:**
```json
{
  "wcode": "W250389",
  "subject_id": "uuid",
  "date_from": "2026-06-01",
  "date_to": "2026-06-07",
  "reason_category": "medical",
  "reason": "Doctor's appointment",
  "sit_in_method": "physical",
  "sit_in_course_id": "uuid",
  "sit_in_session_ids": ["uuid1", "uuid2"],
  "send_sms": true
}
```

- `send_sms` (optional boolean, default true) — allows frontend to opt out of SMS

**Response body — new fields:**
```json
{
  "id": "uuid",
  "wcode": "W250389",
  "subject_id": "uuid",
  "course_id": "uuid",
  "date_from": "2026-06-01",
  "date_to": "2026-06-07",
  "sit_in_method": "physical",
  "status": "pending",
  "version": 1,
  "sms_status": "sent",
  "sms_to": "0812345678"
}
```

### 2.4 Sit-In Pre-Fill Data Flow (GET /sit-in-options)

**Response body — new fields:**
```json
{
  "sit_in_method": "physical",
  "missed_count": 3,
  "sit_in_course": { "id": "uuid", "code": "ENG101", "name": "English 101" },
  "missed_sessions": [...],
  "available_sessions": [...],
  "pre_selected": [...],
  "organizer": {
    "parent_name": "Somchai Smith",
    "parent_phone": "0812345678",
    "parent_email": "somchai@email.com"
  }
}
```

The `organizer` block is populated from `crm_rows.parent_name` etc. This is displayed in `SitInResultCard.tsx` as pre-filled organizer information.

## 3. API Contract Changes

### 3.1 GET /api/v1/absences/sit-in-options

**No input changes**, but response gains optional `organizer` block:

```json
{
  "organizer": {
    "parent_name": "string | null",
    "parent_phone": "string | null",
    "parent_email": "string | null"
  }
}
```

### 3.2 POST /api/v1/absences

**Input — new optional field:**
```json
{
  "send_sms": true   // optional, defaults to true via pointer/nil check
}
```

**Output — new fields:**
```json
{
  "sms_status": "sent|failed|skipped|unavailable",
  "sms_error": "string | null",
  "sms_to": "string | null"
}
```

### 3.3 New Queries (sqlc)

Add to `backend/db/queries/crm_rows.sql`:

```sql
-- name: CrmRowParentPhoneByWcode :one
SELECT parent_name, parent_phone, parent_email
FROM crm_rows
WHERE wcode = $1
  AND snapshot_id = (SELECT active_snapshot_id FROM crm_state WHERE singleton = true)
LIMIT 1;
```

This returns a new generated type `CrmRowParentPhoneByWcodeRow`.

### 3.4 New Policy Setting

Add to `AbsencePolicies` JSON schema (in `absencePolicies` JSON column):

```json
{
  "student_self_service": {
    "can_view_own": true,
    "can_cancel_own": false,
    "sms_on_submit": true   // NEW — policy toggle for SMS notification
  }
}
```

When `sms_on_submit` is false, SMS is never sent regardless of `send_sms` request field.

## 4. State Management (Frontend)

### 4.1 New Reducer Actions

Add to `FormAction` type in `AbsenceForm.tsx`:

```typescript
type FormAction =
  // ... existing actions ...
  | { type: "SET_SMS_OPT_IN"; optIn: boolean }
  | { type: "SET_SMS_STATUS"; subjectId: string; smsStatus: SmsStatus }
```

### 4.2 New FormState Fields

```typescript
type SmsStatus = { status: "sent" | "failed" | "skipped" | "unavailable"; to?: string; error?: string };

type FormState = {
  // ... existing fields ...
  smsOptIn: boolean;                          // default: true
  smsStatuses: Record<string, SmsStatus>;     // per-subject SMS result
};
```

### 4.3 New ConfirmationSummary Block

The `ConfirmationSummary` component (step 3 and post-submit) gets a new section:

```typescript
// ConfirmationSummary new props
smsStatus?: SmsStatus;
```

Shown as:

```
SMS Notification to Parent: ✅ Sent to 081-234-5678
                           ❌ Failed — parent phone not available
                           ⏭️ Skipped (disabled in settings)
```

### 4.4 SitInResultCard Changes

Add organizer pre-fill block to `SitInResultCard.tsx`:

```typescript
export type SitInResult = {
  // ... existing fields ...
  organizer?: {
    parent_name: string | null;
    parent_phone: string | null;
    parent_email: string | null;
  };
};
```

Display when present:

```
Organizer / Emergency Contact:
  Name: Somchai Smith
  Phone: 081-234-5678
  Email: somchai@email.com
```

### 4.5 New Step or Sub-Step?

**Decision: No new step.** SMS opt-in is a toggle on the Confirm step (step 3). Sit-in organizer pre-fill is shown on the Sit-in Plan step (step 2). The flow stays 4 steps; no additional wizard step is added.

## 5. Backend Handler Changes

### 5.1 `handleSitInOptions` (`routes.go`)

After resolving sit-in, query parent contact:

```go
func (s *server) handleSitInOptions(w http.ResponseWriter, r *http.Request) {
    // ... existing validation and resolveSitIn() call ...

    // NEW: Query parent contact from CRM
    var organizer *OrganizerInfo
    parentRow, err := s.deps.Q.CrmRowParentPhoneByWcode(r.Context(), wcode)
    if err == nil {
        organizer = &OrganizerInfo{
            ParentName:  nullIfEmpty(parentRow.ParentName),
            ParentPhone: nullIfEmpty(parentRow.ParentPhone),
            ParentEmail: nullIfEmpty(parentRow.ParentEmail),
        }
    } // silence error — parent contact is best-effort

    // Append to response
    s.a.WriteJSON(w, http.StatusOK, map[string]any{
        "sit_in_method":    result.SitInMethod,
        "sit_in_course":    result.SitInCourse,
        "missed_count":     result.MissedCount,
        "missed_sessions":  result.MissedSession,
        "available_sessions": result.Available,
        "pre_selected":     result.PreSelected,
        "organizer":        organizer,  // NEW
    })
}
```

### 5.2 `handleAbsenceCreate` (`routes.go`)

After commit, send SMS:

```go
func (s *server) handleAbsenceCreate(w http.ResponseWriter, r *http.Request) {
    // ... existing parsing and validation ...
    // NEW: read send_sms from body
    sendSMS := true
    if body.SendSMS != nil {
        sendSMS = *body.SendSMS
    }

    // ... existing lookup, enrolled course, TX creation ...

    // BEGIN TX
    tx, err := s.deps.DB.Begin(r.Context())
    // ... defer Rollback ...
    qtx := s.deps.Q.WithTx(tx)

    // ... existing AbsenceCreate, AbsenceSetSubmissionMetadata, sit-in sessions, audit ...

    // NEW: Capture parent phone and store on absence record BEFORE commit
    var parentPhone string
    if sendSMS {
        parentRow, err := s.deps.Q.CrmRowParentPhoneByWcode(r.Context(), body.Wcode)
        if err == nil && parentRow.ParentPhone.Valid {
            parentPhone = parentRow.ParentPhone.String
            // Save phone on absence record for audit trail
            if err := qtx.AbsenceSetStudentPhone(r.Context(), item.ID, parentPhone); err != nil {
                s.deps.Log.Warn("failed to store student phone", "absence_id", item.ID, "error", err)
            }
        }
    }

    // COMMIT TX
    if err := tx.Commit(r.Context()); err != nil { ... }

    // NEW: Send SMS after commit
    smsStatus := "skipped"
    var smsError string
    if sendSMS && parentPhone != "" {
        if smsEnabled, _ := s.smsEnabledForSubmit(r); smsEnabled {
            msg := buildSMSMessage(body, student)
            sendReq := smartsms.SendRequest{
                Campaign:   fmt.Sprintf("Absence-%s", body.Wcode),
                Message:    msg,
                Mobiles:    []string{parentPhone},
                CampaignNo: fmt.Sprintf("ABS-%s", id),
                RefNo:      id,
            }
            resp, err := s.deps.SMS.SendSMS(r.Context(), sendReq)
            if err != nil {
                smsStatus = "failed"
                smsError = err.Error()
                s.deps.Log.Error("sms send failed", "absence_id", id, "error", err)
            } else if resp.Success {
                smsStatus = "sent"
            } else {
                smsStatus = "failed"
                smsError = "provider returned success=false"
            }
        } else {
            smsStatus = "skipped"
        }
    } else if sendSMS && parentPhone == "" {
        smsStatus = "unavailable"
    }

    // Return 201 with SMS status
    s.a.WriteJSON(w, http.StatusCreated, map[string]any{
        "id":            id,
        "wcode":         item.Wcode,
        "subject_id":    body.SubjectID,
        "course_id":     courseIDStr,
        "date_from":     item.DateFrom.Time.Format("2006-01-02"),
        "date_to":       item.DateTo.Time.Format("2006-01-02"),
        "sit_in_method": body.SitInMethod,
        "status":        "pending",
        "version":       1,
        "sms_status":    smsStatus,     // NEW
        "sms_error":     smsError,      // NEW (null when empty)
        "sms_to":        maskPhone(parentPhone), // NEW
    })
}
```

### 5.3 New SQL Function

Add to `absence_management_custom.go`:

```go
func (q *Queries) AbsenceSetStudentPhone(ctx context.Context, id pgtype.UUID, phone string) error {
    _, err := q.db.Exec(ctx, `
        UPDATE student_absences
        SET student_phone = $2, updated_at = now()
        WHERE id = $1
    `, id, phone)
    return err
}
```

### 5.4 New Helper: `smsEnabledForSubmit`

```go
func (s *server) smsEnabledForSubmit(r *http.Request) (bool, error) {
    settings, err := s.readAbsenceSettings(r)
    if err != nil {
        return false, err
    }
    return settings.StudentSelfService.SmsOnSubmit, nil
}
```

Requires adding `SmsOnSubmit bool` to `studentSelfServiceSettings`.

### 5.5 New Helper: `buildSMSMessage`

```go
func buildSMSMessage(body *absenceCreateBody, student sqldb.Student) string {
    msg := fmt.Sprintf("Absence reported for %s (%s).", student.FullName, body.Wcode)
    if body.SubjectID != "" {
        msg += fmt.Sprintf(" Subject: %s.", body.SubjectID)
    }
    msg += fmt.Sprintf(" Dates: %s - %s.", body.DateFrom, body.DateTo)
    if body.ReasonCategory != nil && *body.ReasonCategory != "" {
        msg += fmt.Sprintf(" Reason: %s.", *body.ReasonCategory)
    }
    return msg
}
```

## 6. Transaction Boundaries

### 6.1 Decision: SMS is OUTSIDE the DB transaction

**Reasoning:**
- SmartSMS is an external HTTP call with unpredictable latency (can take seconds)
- Holding a PostgreSQL transaction open during HTTP calls would keep locks, exhaust pool connections
- SMS failure should not roll back a successfully-created absence record
- The SMS provider call is best-effort — absence creation is authoritative

**Pattern:**
```
1. BEGIN TX
2. Execute all DB writes (absence, sit-ins, audit, student_phone)
3. COMMIT TX  (fast, local DB)
4. If COMMIT succeeded → SMS send (external, may fail)
5. If COMMIT failed → return 500, no SMS attempt
```

### 6.2 CRM Phone Query: OUTSIDE the transaction

**Reasoning:**
- `crm_rows` is a read-only snapshot table, no lock concern
- No need to include reads in the write transaction
- Perform the CRM query before `BEGIN TX`

### 6.3 What if SMS fails after commit?

**The absence is already saved.** SMS failure is surfaced in:
- The `201` response body (`sms_status: "failed"`)
- A server-side error log
- The `student_absences.student_phone` field (so admin can see phone was available)

**No automatic retry in v1.** Admin can manually note the SMS failure via the absence management UI. Future v2 could add a "Resend SMS" admin action.

## 7. Error Propagation

### 7.1 SMS Failure to Frontend

```
SMS Failure               Frontend Rendering
────────────────────────────────────────────────────────
Provider down (timeout) → sms_status: "failed"
                         → sms_error: "Provider timeout"
                         → Badge: ⚠️ SMS unavailable
                         → Toast: None (non-critical)
                         → Absence saved successfully

Missing parent phone    → sms_status: "unavailable"
                         → sms_error: null
                         → Badge: ℹ️ No parent phone
                         → Toast: None

Policy disabled         → sms_status: "skipped"
                         → Badge: ℹ️ SMS notifications off

Success                 → sms_status: "sent"
                         → sms_to: "081-234-****"
                         → Badge: ✅ SMS sent
```

### 7.2 Missing CRM Phone Error

**This is NOT an error** — it's a normal state. Many students may not have parent phone in CRM.

**Frontend handling:**
- `SitInResultCard` shows organizer section only when `organizer` block is present
- If `parent_phone` is null, show "No parent contact on file"
- The SMS is simply skipped; the absence is still submitted normally

### 7.3 user-visible error message strategy

| Scenario | Toast? | Inline? | Severity |
|---|---|---|---|
| SMS sent OK | None | ✅ Badge on post-submit | Info |
| SMS failed | Warning (non-blocking) | ⚠️ Badge on post-submit | Warning |
| No parent phone | None | ℹ️ Inline note | Info |
| Policy disabled | None | ℹ️ Inline note | Info |
| Absence DB write fails | Error | ❌ Inline + retry | Error |

**Principle:** Never block the absence submission on SMS. The absence is the primary action; SMS is a side-effect.

## 8. Sit-In Pre-Fill Specifics

### 8.1 What Gets Pre-Filled

The sit-in organizer pre-fill is about providing contact information for the person organizing the sit-in (typically a parent or guardian). This data comes from CRM `crm_rows.parent_name`, `parent_phone`, `parent_email`.

### 8.2 How It Appears in the UI

In `SitInResultCard.tsx`, below the sit-in course banner:

```
┌──────────────────────────────────┐
│ Sit-in at ENG101 — English 101  │
│ 3 missed session(s).            │
│ 2 sit-in session(s) available.  │
├──────────────────────────────────┤
│ Organizer / Emergency Contact:   │
│ Name:   Somchai Smith           │
│ Phone:  081-234-5678            │
│ Email:  somchai@email.com       │
└──────────────────────────────────┘
```

### 8.3 CRM Data Freshness

The `crm_rows` table is a snapshot that is replaced on each CRM import. The parent_phone data is as-of the last import. No real-time CRM lookup is performed.

**Design decision:** This is acceptable because:
1. Parent phone changes infrequently
2. CRM imports happen regularly (at least weekly per product scope)
3. The phone is a best-effort notification channel, not an identity-critical field

## 9. Testing Strategy

### 9.1 Unit Tests

**Backend (`absenceshttp` handler tests):**

| Test | Coverage |
|---|---|
| `TestAbsenceCreate_SMSSent` | Create absence with `send_sms: true`, mock SMS provider returns success → assert `sms_status: "sent"` |
| `TestAbsenceCreate_SMSFailed` | Mock SMS provider returns error → assert `sms_status: "failed"` + error message |
| `TestAbsenceCreate_SMSUnavailable` | No parent phone in CRM → assert `sms_status: "unavailable"` |
| `TestAbsenceCreate_SMSSkipped` | `send_sms: false` → assert `sms_status: "skipped"` |
| `TestAbsenceCreate_SMSDisabledByPolicy` | Policy `sms_on_submit: false` → assert `sms_status: "skipped"` regardless of `send_sms` |
| `TestSitInOptions_WithOrganizer` | CRM has parent phone → assert `organizer` block in response |
| `TestSitInOptions_WithoutOrganizer` | CRM has no parent phone or no matching wcode → assert `organizer` is null |

**Frontend (`AbsenceForm.test.tsx`):**

| Test | Coverage |
|---|---|
| `handles sms_status sent` | Post-submit shows "SMS sent" badge |
| `handles sms_status failed` | Post-submit shows warning badge, absence still succeeds |
| `handles sms_status unavailable` | Post-submit shows "No parent phone" note |
| `shows organizer in sit-in card` | `SitInResultCard` renders organizer block when data exists |
| `hides organizer block when null` | `SitInResultCard` does not show organizer section when absent |

### 9.2 Mocking the SMS Provider

Already exists: `smartsms.MockProvider` in `provider.go`.

```go
type MockProvider struct{}

func (m *MockProvider) SendSMS(_ context.Context, req SendRequest) (*SendResponse, error) {
    slog.Info("SMS mock send", "mobiles", len(req.Mobiles), "message_len", len(req.Message))
    return &SendResponse{Success: true, CreditsUsed: len(req.Mobiles), CorrectCount: len(req.Mobiles)}, nil
}
```

For failure testing, use a custom mock:

```go
type failingSMSProvider struct{}

func (f *failingSMSProvider) SendSMS(_ context.Context, _ SendRequest) (*SendResponse, error) {
    return nil, fmt.Errorf("provider timeout")
}
```

Wire into `httpdeps.Deps.SMS` during test setup:

```go
deps := httpdeps.Deps{
    // ... other deps ...
    SMS: &failingSMSProvider{},
}
```

### 9.3 Test Fixtures for CRM Data

**New test SQL fixture** (`testdata/crm_rows.sql`):

```sql
INSERT INTO crm_rows (row_hash, cycle_label, course_name, wcode,
  first_name, last_name, parent_name, parent_phone, parent_email,
  snapshot_id)
VALUES (
  'hash1', '2025-S1', 'English 101', 'W250389',
  'Somchai', 'Smith', 'Somchai Smith', '0812345678', 'somchai@email.com',
  (SELECT id FROM crm_snapshots ORDER BY created_at DESC LIMIT 1)
);
```

**Go test helper** (`absenceshttp/testhelpers_test.go`):

```go
func seedCRMPhone(t *testing.T, q *sqldb.Queries, wcode, phone string) {
    t.Helper()
    // Insert a CRM row with parent_phone for this wcode
}
```

### 9.4 Integration Test Flow

```
1. Seed: Insert student, course, subject, session, CRM row with parent_phone
2. Auth: Create admin user session
3. Execute: POST /api/v1/absences with valid body
4. Assert: 201 response with sms_status = "sent" or "unavailable"
5. Assert: student_absences.student_phone is set
6. Assert: MockProvider.SendSMS was called with correct phone
```

## 10. Implementation Order

| Phase | What | Depends On |
|---|---|---|
| 1 | Add `CrmRowParentPhoneByWcode` SQL query + sqlc generate | Nothing |
| 2 | Add `AbsenceSetStudentPhone` custom query | Phase 1 |
| 3 | Add `SmsOnSubmit` to absence policies JSON | Nothing |
| 4 | Modify `handleSitInOptions` to include `organizer` block | Phase 1 |
| 5 | Modify `handleAbsenceCreate` to query CRM phone, store it, send SMS | Phases 1, 2, 3 |
| 6 | Update frontend `SitInResultCard` for organizer pre-fill | Phase 4 |
| 7 | Update frontend state/submit flow for SMS status | Phase 5 |
| 8 | Tests | All phases |

## 11. Risk Register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| SmartSMS API slow | Medium | HTTP request timeout | 30s timeout on client; SMS is non-blocking for absence success |
| CRM data stale (wrong phone) | Low-medium | SMS goes to wrong person | Phone is best-effort; absence still recorded; admin can see phone in inbox |
| Missing `crm_rows` snapshot | Low | No parent phone | Graceful fallback to "unavailable" status |
| Policy toggle changes mid-request | Low | SMS sent when disabled | Read policy after DB commit to get latest value (but acceptable race) |
| SMS sent twice for same absence | Low | Duplicate message | Idempotency-Key prevents duplicate POST; SMS is sent once per 201 response |

---

*End of design document. All changes estimated at ~400 lines Go backend + ~150 lines React frontend.*
