# Special Absence Type Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Special Absence" type to the staff absence creation flow, allowing staff to create absences with `special_approved` status directly. Use a separate Thai SMS template for special_approved cases. The table "Special Approve" action shows an SMS confirm modal with the special template.

**Architecture:**
- Frontend: Add absence type selection as Step 0 in `StaffCreateAbsenceModal`. The rest of the flow (subjects → sessions → sit-in → confirm) is identical for both types. Pass `status` field to backend on submit.
- Backend: Accept `status` field in staff-create request, set absence status after creation. Add `sms_special_approved_template` to notification settings. Select SMS template based on absence status in explicit staff/admin success-SMS preview/send paths. Keep public student submission SMS on the normal success template.
- Table flow: After special approve from table, show SmsConfirmModal with special template.

**Tech Stack:** React, TypeScript, Go, PostgreSQL, sqlc, SmartSMS

---

## Recheck Corrections — Must Apply Before Implementation

The original shape is sound, but the following corrections are required to avoid production bugs:

1. **Do not auto-send SMS inside `handleAbsenceStatusUpdate` for `special_approved`.** The table flow updates status, then shows `SmsConfirmModal`; sending during the status update would send one SMS before confirmation and another when the modal's Send button is clicked.
2. **Do not use the special SMS template when *any* item in a batch is special.** A mixed batch would send special wording for normal absences. Reject mixed normal/special batches with a clear `400 mixed_status_sms_templates`, or split into separate sends. This plan uses rejection because the endpoint returns one preview/message.
3. **Use `backend/internal/absences/status.go` as the status source of truth.** Do not duplicate valid status lists or transition logic in HTTP handlers. Staff create may only accept `pending`, blank/missing, or `special_approved`.
4. **Add the database/status invariants before API work.** The DB check constraints, audit action constraint, absence count query, list buckets, and stats need to know about `special_approved`; otherwise creation/status update can fail or records can disappear from normal views.
5. **Treat `special_approved` as archived/non-actionable, but allow cancellation for recovery.** It should not appear in the active pending/reviewed inbox after approval, but admins must be able to cancel an accidentally special-approved absence.
6. **Make notification settings backward compatible.** Existing `absence_policies.notifications` JSON will not contain `sms_special_approved_template`; `parseAbsenceSettings`, defaults, validation, frontend normalization, and the settings editor must preserve/add it.
7. **Keep student-submission SMS paths on the normal success template.** Student/public absence creation creates `pending` records only; the special template belongs to staff-created special absences and explicit sends/previews for `special_approved` records.

---

## File Map

| File | Purpose |
|------|---------|
| `backend/db/migrations/00068_special_approved_absence_status.sql` | Add `special_approved` to status and audit check constraints |
| `backend/internal/absences/status.go` | Central status constants, validation, transitions, audit action names |
| `backend/internal/db/absence_custom.go` | Exclude `special_approved` from student absence rate-limit counts |
| `backend/internal/db/absence_management_custom.go` | Include `special_approved` in stats and archived list bucket |
| `backend/internal/httpapi/absenceshttp/staff_create.go` | Accept `status` field, set status after creation, select SMS template |
| `backend/internal/httpapi/absenceshttp/management_routes.go` | Add `SmsSpecialApprovedTemplate` to settings and keep special approve status updates SMS-free |
| `src/features/absences/types.ts` | Add `status` to `StaffCreateAbsenceRequest`, add `sms_special_approved_template` to settings type |
| `src/features/absences/constants.ts` | Add frontend default notification field |
| `src/features/absences/api/absenceFormApi.ts` | Normalize `sms_special_approved_template` from settings responses |
| `src/components/absences/AbsenceFormEditor.tsx` | Add settings editor textarea for the special-approved SMS template |
| `src/components/absences/StaffCreateAbsenceModal.tsx` | Add type selection step (Step 0), pass status on submit |
| `src/pages/Absences.tsx` | Add SMS preview + SmsConfirmModal after table special approve |
| `src/components/absences/KanbanView.tsx` | Show/filter special-approved records consistently if board view includes archived statuses |

---

## Task 0: Backend — Add Status/Database Invariants First

**Files:**
- Add/verify: `backend/db/migrations/00068_special_approved_absence_status.sql`
- Add/verify: `backend/internal/absences/status.go`
- Modify: `backend/internal/httpapi/absenceshttp/management_routes.go`
- Modify: `backend/internal/db/absence_custom.go`
- Modify: `backend/internal/db/absence_management_custom.go`
- Modify tests in `backend/internal/httpapi/absenceshttp/management_routes_test.go` and `backend/internal/db/absence_integration_test.go`

### Step 1: Add the migration

The migration must update both check constraints:

```sql
ALTER TABLE student_absences
  DROP CONSTRAINT IF EXISTS student_absences_status_check,
  ADD CONSTRAINT student_absences_status_check
    CHECK (status IN ('pending', 'reviewed', 'actioned', 'cancelled', 'special_approved'));

ALTER TABLE absence_audit_log
  DROP CONSTRAINT IF EXISTS absence_audit_log_action_check,
  ADD CONSTRAINT absence_audit_log_action_check
    CHECK (action IN ('submitted', 'reviewed', 'reopened', 'actioned', 'cancelled', 'sit_in_overridden', 'note_added', 'created_by_staff', 'special_approved'));
```

The down migration must remove `special_approved` from both constraints.

### Step 2: Centralize status rules

Use `backend/internal/absences/status.go` for:

- `StatusPending`
- `StatusReviewed`
- `StatusActioned`
- `StatusCancelled`
- `StatusSpecialApproved`
- `ValidStatus`
- `ValidTransition`
- `StatusAuditAction`

Required transition policy:

```text
pending/reviewed/actioned -> special_approved: allowed
special_approved -> cancelled: allowed
special_approved -> pending/reviewed/actioned: blocked
cancelled -> special_approved: blocked
```

This keeps special approval recoverable while preventing it from re-entering the normal review workflow.

### Step 3: Delegate HTTP helpers to the status package

`validAbsenceStatus`, `validTransition`, and `statusAuditAction` in `management_routes.go` should call `absences.ValidStatus`, `absences.ValidTransition`, and `absences.StatusAuditAction`.

### Step 4: Exclude special-approved absences from rate-limit counts

Update `StudentAbsenceCountForCourse`:

```sql
WHERE wcode = $1
  AND course_id = $2
  AND status NOT IN ('cancelled', 'special_approved')
```

Add/keep an integration test proving pending counts and cancelled/special-approved do not count.

### Step 5: Keep special-approved records visible

Update the list bucket query in `absence_management_custom.go`:

```sql
($9 = 'active' AND sa.status IN ('pending', 'reviewed'))
OR ($9 = 'archived' AND sa.status IN ('actioned', 'cancelled', 'special_approved'))
```

Also include `special_approved_count` in absence stats and update frontend `AbsenceStats` types if the API response type is modeled.

### Step 6: Run invariant tests

```bash
cd backend && go test ./internal/absences ./internal/db ./internal/httpapi/absenceshttp -run 'Status|AbsenceCount|Stats|List'
```

Expected: status transitions, count exclusion, stats, and list bucket behavior all pass.

---

## Task 1: Backend — Add Status Field to Staff Create Request

**Files:**
- Modify: `backend/internal/httpapi/absenceshttp/staff_create.go:18-30` (request struct)
- Modify: `backend/internal/httpapi/absenceshttp/staff_create.go:57-64` (validation)
- Modify: `backend/internal/httpapi/absenceshttp/staff_create.go:240-278` (post-create logic + SMS preview)

### Step 1: Add Status field to request struct

In `staff_create.go`, add `Status` field to `staffCreateAbsenceRequest` (line 18):

```go
type staffCreateAbsenceRequest struct {
	Wcode            string   `json:"wcode"`
	SubjectID        *string  `json:"subject_id"`
	CourseID         *string  `json:"course_id"`
	DateFrom         string   `json:"date_from"`
	DateTo           string   `json:"date_to"`
	MissedSessionIDs []string `json:"missed_session_ids"`
	SitInMethod      *string  `json:"sit_in_method"`
	SitInCourseID    *string  `json:"sit_in_course_id"`
	SitInSessionIDs  []string `json:"sit_in_session_ids"`
	Reason           *string  `json:"reason"`
	ReasonCategory   *string  `json:"reason_category"`
	Status           *string  `json:"status"` // optional: "pending" (default) or "special_approved"
}
```

### Step 2: Add status validation

Import the status package:

```go
	"warwick-institute/internal/absences"
```

After the existing validation block (around line 64), normalize the request status once and use the normalized value everywhere else in the handler:

```go
		requestedStatus := absences.StatusPending
		statusVal := ""
		if body.Status != nil {
			statusVal = strings.TrimSpace(*body.Status)
		}
		switch absences.Status(statusVal) {
		case "", absences.StatusPending:
			requestedStatus = absences.StatusPending
		case absences.StatusSpecialApproved:
			requestedStatus = absences.StatusSpecialApproved
		default:
			s.a.WriteErr(w, http.StatusBadRequest, "bad_status", "status must be 'pending' or 'special_approved'")
			return 0, nil, fmt.Errorf("bad status")
		}
```

### Step 3: Set status after creation and audit

After the `AbsenceAuditInsert` for `"created_by_staff"` (around line 249), update status only when the normalized request status is `special_approved`. Fetch `ManagedAbsenceGet` **after** this block so the DTO, version, timeline, and SMS preview all reflect the final status.

```go
		if requestedStatus == absences.StatusSpecialApproved {
			newVersion, err := qtx.AbsenceStatusUpdate(r.Context(), row.ID, string(absences.StatusSpecialApproved), actorID(user.ID), row.Version)
			if err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not set special approved status")
				return 0, nil, err
			}
			if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{
				AbsenceID: row.ID,
				Action:    string(absences.StatusSpecialApproved),
				ActorID:   actorID(user.ID),
				ActorRole: "admin",
				Details:   map[string]any{"from": "pending", "to": "special_approved", "staff_created": true, "wcode": body.Wcode},
			}); err != nil {
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write absence timeline")
				return 0, nil, err
			}
			row.Version = newVersion
		}
```

### Step 4: Update DTO status to reflect final status

Do **not** manually patch `dto.Status`. Instead, fetch the managed row after the optional status update and use the status from the database:

```go
		managed, err := qtx.ManagedAbsenceGet(r.Context(), row.ID)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		dto := s.managedAbsenceDTO(managed)
```

### Step 5: Select SMS template based on final status

Add a small helper (or equivalent local logic) so every explicit success-SMS path uses the same fallback rule:

```go
func successSMSTemplateForStatus(settings absenceSettings, status string) string {
	if absences.Status(status) == absences.StatusSpecialApproved && strings.TrimSpace(settings.Notifications.SmsSpecialApprovedTemplate) != "" {
		return settings.Notifications.SmsSpecialApprovedTemplate
	}
	return settings.Notifications.SmsSuccessTemplate
}
```

In the staff-create SMS preview section (around line 264-278), select the template from the final database status:

```go
		smsTemplate := successSMSTemplateForStatus(settings, managed.Status)

		if smsTemplate != "" {
			if contactRows, contactErr := qtx.StudentSubjectByWCode(r.Context(), body.Wcode); contactErr == nil && len(contactRows) > 0 {
				phones := successSMSPhones(contactRows[0].ParentPhone, contactRows[0].StudentPhone)
				if len(phones) > 0 {
					sess, _ := qtx.ManagedAbsenceSessions(r.Context(), row.ID)
					mis, _ := qtx.ManagedAbsenceMissedSessions(r.Context(), row.ID)
					loc, _ := time.LoadLocation(s.deps.InstituteTZ)
					if loc == nil {
						loc = time.UTC
					}
					rendered := renderSuccessSMSTemplate(smsTemplate, managed, sess, mis, loc)
					dto.SmsPreview = &smsPreviewDTO{Phones: phones, Message: rendered}
				}
			}
		}
```

### Step 6: Run Go tests

```bash
cd backend && go test ./internal/httpapi/absenceshttp/... -v -run TestStaffCreate
```

Expected: Tests pass (existing tests don't send status field, so default behavior unchanged)

### Step 7: Commit

```bash
git add backend/internal/httpapi/absenceshttp/staff_create.go
git commit -m "feat(backend): accept status field in staff-create endpoint for special absences"
```

---

## Task 2: Backend — Add Special Approved SMS Template to Settings

**Files:**
- Modify: `backend/internal/httpapi/absenceshttp/management_routes.go:1216-1221` (notifications struct)
- Modify: `backend/internal/httpapi/absenceshttp/management_routes.go:1249-1254` (default settings)
- Modify: `backend/internal/httpapi/absenceshttp/management_routes.go:1259-1288` (settings parsing)
- Modify: `backend/internal/httpapi/absenceshttp/management_routes.go:1318-1324` (settings validation)

### Step 1: Add field to absenceNotificationsSettings struct

```go
type absenceNotificationsSettings struct {
	SmsParentEnabled           bool   `json:"sms_parent_enabled"`
	SmsParentTemplate          string `json:"sms_parent_template"`
	SmsSuccessTemplate         string `json:"sms_success_template"`
	SmsSpecialApprovedTemplate string `json:"sms_special_approved_template"`
	AllowSubmitWithoutOtp      bool   `json:"allow_submit_without_otp"`
}
```

### Step 2: Add default template in defaultAbsenceSettings

```go
		Notifications: absenceNotificationsSettings{
			SmsParentEnabled:           true,
			SmsParentTemplate:          "Your Warwick verification code is {{code}}.",
			SmsSuccessTemplate:         "Warwick Institute: {{nickname}} ได้แจ้งลาเรียน {{absence_summary}} และมีกำหนดเข้าเรียนชดเชย {{sit_in_summary}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ",
			SmsSpecialApprovedTemplate: "Warwick Institute: {{nickname}} จะมีเรียนชดเชย {{absence_summary}} และมีกำหนดเข้าเรียน {{sit_in_summary}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ",
			AllowSubmitWithoutOtp:      false,
		},
```

### Step 3: Preserve the new default for old settings JSON

Existing rows may have `absence_policies.notifications` without `sms_special_approved_template`. Do not let unmarshalling an old notifications object wipe the new default to `""`.

Implement one of these approaches:

- Preferred: unmarshal notifications into pointer fields and merge only fields present in JSON.
- Acceptable: after unmarshalling, detect whether `notifications.sms_special_approved_template` is absent in the raw JSON and restore `defaultAbsenceSettings().Notifications.SmsSpecialApprovedTemplate`.

Add a test for a legacy JSON payload like:

```json
{"notifications":{"sms_parent_enabled":true,"sms_parent_template":"OTP {{code}}","sms_success_template":"Normal {{absence_summary}}","allow_submit_without_otp":false}}
```

Expected: parsed settings keep the normal success template and fill the special-approved template from defaults.

### Step 4: Validate special template length

Add validation:

```go
	if len([]rune(settings.Notifications.SmsSpecialApprovedTemplate)) > 500 {
		return fmt.Errorf("sms_special_approved_template must not exceed 500 characters")
	}
```

### Step 5: Run Go build/tests

```bash
cd backend && go test ./internal/httpapi/absenceshttp -run 'Settings|Special'
cd backend && go build ./...
```

Expected: Build succeeds

### Step 6: Commit

```bash
git add backend/internal/httpapi/absenceshttp/management_routes.go
git commit -m "feat(backend): add sms_special_approved_template to notification settings"
```

---

## Task 3: Backend — Use Correct Template in Explicit SMS Send/Preview Paths

**Files:**
- Modify: `backend/internal/httpapi/absenceshttp/management_routes.go:737-755` (status update handler)
- Modify: `backend/internal/httpapi/absenceshttp/staff_create.go:349-407` (single SMS send)
- Modify: `backend/internal/httpapi/absenceshttp/staff_create.go:409-508` (batch SMS send)

### Step 1: Keep status update auto-SMS only for `actioned`

Do not send SMS when `body.Status == "special_approved"` here. The table flow must update the status first, then fetch a dry-run preview and let the admin confirm in `SmsConfirmModal`.

The existing `actioned` path may keep its automatic send behavior. If you touch it, render from the final status but keep the condition as `body.Status == "actioned"` only:

```go
		if body.Status == string(absences.StatusActioned) {
			recipients := successSMSPhones(current.ParentPhone, current.StudentPhone)
			if len(recipients) > 0 {
				sessions, sessErr := qtx.ManagedAbsenceSessions(r.Context(), id)
				if sessErr == nil {
					missed, missedErr := qtx.ManagedAbsenceMissedSessions(r.Context(), id)
					currentForSms := current
					currentForSms.Status = body.Status
					smsTemplate := successSMSTemplateForStatus(settings, currentForSms.Status)
					if missedErr == nil {
						sendSuccessSMS(s.deps.SMS, s.deps.Log, smsTemplate, currentForSms, sessions, missed, recipients, s.deps.InstituteTZ)
					} else {
						if s.deps.Log != nil {
							s.deps.Log.Error("failed to load missed sessions for sms", "absence_id", r.PathValue("id"), "error", missedErr)
						}
						sendSuccessSMS(s.deps.SMS, s.deps.Log, smsTemplate, currentForSms, sessions, nil, recipients, s.deps.InstituteTZ)
					}
				} else if s.deps.Log != nil {
					s.deps.Log.Error("failed to load absence sessions for sms", "absence_id", r.PathValue("id"), "error", sessErr)
				}
			}
		}
```

Add/keep a test proving a `special_approved` status update does **not** call the SMS provider.

### Step 2: Update handleSendSuccessSMS to use special template

In `handleSendSuccessSMS` (staff_create.go, around line 378-397), select template based on the absence's current status:

```go
		smsTemplate := successSMSTemplateForStatus(settings, managed.Status)
		if strings.TrimSpace(smsTemplate) == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "sms_disabled", "SMS notifications are not configured")
			return 0, nil, fmt.Errorf("sms not configured")
		}

		// ... (contact lookup unchanged) ...

		sent := sendSuccessSMS(s.deps.SMS, s.deps.Log, smsTemplate, managed, sessions, missed, phones, s.deps.InstituteTZ)
```

### Step 3: Update handleBatchSendSuccessSMS to reject mixed-template batches

`handleBatchSendSuccessSMS` sends one SMS message for all requested absence IDs. For mixed statuses, there is no single safe template. Reject mixed normal/special batches when the special template is configured.

```go
		if settings.Notifications.SmsSuccessTemplate == "" && settings.Notifications.SmsSpecialApprovedTemplate == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "sms_disabled", "SMS notifications are not configured")
			return 0, nil, fmt.Errorf("sms not configured")
		}

		// ... (item loading unchanged, around line 458-476) ...

		hasSpecial := false
		hasNormal := false
		for _, item := range items {
			if absences.Status(item.row.Status) == absences.StatusSpecialApproved {
				hasSpecial = true
			} else {
				hasNormal = true
			}
		}
		if hasSpecial && hasNormal && strings.TrimSpace(settings.Notifications.SmsSpecialApprovedTemplate) != "" {
			s.a.WriteErr(w, http.StatusBadRequest, "mixed_status_sms_templates", "Send normal and special-approved SMS notifications separately")
			return 0, nil, fmt.Errorf("mixed status sms templates")
		}

		smsTemplate := settings.Notifications.SmsSuccessTemplate
		if hasSpecial && strings.TrimSpace(settings.Notifications.SmsSpecialApprovedTemplate) != "" {
			smsTemplate = settings.Notifications.SmsSpecialApprovedTemplate
		}
		if strings.TrimSpace(smsTemplate) == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "sms_disabled", "SMS notifications are not configured")
			return 0, nil, fmt.Errorf("sms not configured")
		}

		if body.DryRun {
			loc, _ := time.LoadLocation(s.deps.InstituteTZ)
			if loc == nil {
				loc = time.UTC
			}
			rendered := renderBatchSuccessSMSTemplate(smsTemplate, items, loc)
			return http.StatusOK, map[string]any{"preview": map[string]any{"phones": phones, "message": rendered}}, nil
		}

		sent := sendBatchSuccessSMS(s.deps.SMS, s.deps.Log, smsTemplate, items, phones, s.deps.InstituteTZ)
```

Add tests for:

- all normal IDs use the normal success template,
- all special-approved IDs use the special template,
- mixed normal/special IDs return `400 mixed_status_sms_templates`,
- if the special template is blank, special-approved falls back to the normal success template.

### Step 4: Run Go tests

```bash
cd backend && go test ./internal/httpapi/absenceshttp/... -v
```

Expected: Tests pass

### Step 5: Commit

```bash
git add backend/internal/httpapi/absenceshttp/management_routes.go backend/internal/httpapi/absenceshttp/staff_create.go
git commit -m "feat(backend): use special_approved SMS template for explicit sends"
```

---

## Task 4: Frontend — Add Status and Notification Types/Defaults

**Files:**
- Modify: `src/features/absences/types.ts:112-124` (request type)
- Modify: `src/features/absences/types.ts:131-136` (notifications type)
- Modify: `src/features/absences/types.ts` (`AbsenceStats`)
- Modify: `src/features/absences/constants.ts`
- Modify: `src/features/absences/api/absenceFormApi.ts`
- Modify: `src/components/absences/AbsenceFormEditor.tsx`
- Modify tests in `src/pages/__tests__/AbsenceSettings.test.tsx` and `src/features/absences/api/__tests__/absenceFormApi.test.ts`

### Step 1: Add status field to request type

```typescript
export type StaffCreateAbsenceRequest = {
  wcode: string;
  subject_id?: string;
  course_id?: string;
  date_from: string;
  date_to: string;
  missed_session_ids: string[];
  sit_in_method?: string;
  sit_in_course_id?: string;
  sit_in_session_ids: string[];
  reason?: string;
  reason_category?: string;
  status?: "pending" | "special_approved";
};
```

### Step 2: Add sms_special_approved_template to settings type

```typescript
export type AbsenceNotificationsSettings = {
  sms_parent_enabled: boolean;
  sms_parent_template: string;
  sms_success_template?: string;
  sms_special_approved_template?: string;
  allow_submit_without_otp: boolean;
};
```

### Step 3: Add special-approved count to stats type

```typescript
export type AbsenceStats = {
  total_count: number;
  pending_count: number;
  reviewed_count: number;
  actioned_count: number;
  cancelled_count: number;
  special_approved_count: number;
  today_count: number;
};
```

### Step 4: Add frontend defaults and normalization

In `DEFAULT_NOTIFICATIONS`:

```typescript
sms_special_approved_template: "",
```

In `normalizeAbsenceFormConfig`, preserve the field:

```typescript
sms_special_approved_template:
  data.notifications?.sms_special_approved_template ??
  DEFAULT_NOTIFICATIONS.sms_special_approved_template,
```

### Step 5: Add settings editor textarea

In `AbsenceFormEditor.tsx`, add a textarea below the normal success template:

```tsx
<label className="block text-sm">
  Special approved SMS template
  <textarea
    className="mt-1 block w-full rounded-sm border border-gray-300 p-2 text-sm"
    maxLength={500}
    rows={3}
    value={settings.notifications?.sms_special_approved_template ?? ""}
    onChange={(e) =>
      onChange({
        ...settings,
        notifications: {
          ...settings.notifications,
          sms_special_approved_template: e.target.value,
          sms_parent_enabled: settings.notifications?.sms_parent_enabled ?? true,
          sms_parent_template: settings.notifications?.sms_parent_template ?? "",
          sms_success_template: settings.notifications?.sms_success_template ?? "",
          allow_submit_without_otp: settings.notifications?.allow_submit_without_otp ?? false,
        },
      })
    }
  />
  <span className="mt-1 text-xs text-gray-500">
    Placeholders: {"{{nickname}}"}, {"{{absence_summary}}"}, {"{{sit_in_summary}}"}, {"{{class_name}}"}, {"{{absence_date}}"}, {"{{sit_in_class}}"}, {"{{sit_in_date_time}}"}
  </span>
</label>
```

When updating the existing notification checkbox/template handlers, preserve `sms_special_approved_template` in the spread/defaults so saving OTP or normal success settings does not drop the special template.

### Step 6: Add tests

- Normalization fills `sms_special_approved_template` from defaults when absent.
- Absence settings save includes `sms_special_approved_template`.
- Editing OTP or normal success SMS does not drop the special template.

### Step 7: Run typecheck

```bash
npm run typecheck
```

Expected: No new errors

### Step 8: Commit

```bash
git add src/features/absences/types.ts src/features/absences/constants.ts src/features/absences/api/absenceFormApi.ts src/components/absences/AbsenceFormEditor.tsx src/pages/__tests__/AbsenceSettings.test.tsx src/features/absences/api/__tests__/absenceFormApi.test.ts
git commit -m "feat(frontend): add special absence status and SMS settings types"
```

---

## Task 4.5: Frontend — Keep Special-Approved Absences Discoverable

**Files:**
- Modify: `src/pages/Absences.tsx`
- Modify: `src/components/absences/KanbanView.tsx`
- Modify tests in `src/pages/__tests__/Absences.test.tsx` and `src/components/absences/__tests__/KanbanView.test.tsx`

### Step 1: Add Special Approved to archived filter options

Special-approved records are no longer active work. They should be visible under archived filters:

```typescript
const bucket: AbsenceBucket =
  bucketParam === "archived" ||
  (!bucketParam && (statusParam === "actioned" || statusParam === "cancelled" || statusParam === "special_approved"))
    ? "archived"
    : "active";
```

Archived status options should include:

```typescript
{ value: "special_approved", label: "Special Approved" }
```

### Step 2: Board view visibility

Add a `special_approved` column and include it in initial/realtime reloads:

```typescript
export const COLUMNS: { key: AbsenceStatus; label: string }[] = [
  { key: "pending", label: "Pending" },
  { key: "reviewed", label: "Reviewed" },
  { key: "actioned", label: "Actioned" },
  { key: "special_approved", label: "Special Approved" },
];
```

Also add a `COLUMN_STYLES.special_approved` entry and update every `loadColumn(...)` call set to include `special_approved`.

### Step 3: Align Cancel actions with backend transition policy

Because Task 0 allows `special_approved -> cancelled`, existing Cancel buttons can remain visible for special-approved records. Add a test that cancelling a special-approved record sends `status: "cancelled"` with the expected version.

### Step 4: Run tests

```bash
npx vitest run src/pages/__tests__/Absences.test.tsx src/components/absences/__tests__/KanbanView.test.tsx
```

---

## Task 5: Frontend — Add Absence Type Selection Step to StaffCreateAbsenceModal

**Files:**
- Modify: `src/components/absences/StaffCreateAbsenceModal.tsx:41` (ModalStep type)
- Modify: `src/components/absences/StaffCreateAbsenceModal.tsx:71` (STEP_KEYS)
- Modify: `src/components/absences/StaffCreateAbsenceModal.tsx:195` (initial step)
- Modify: `src/components/absences/StaffCreateAbsenceModal.tsx:914-946` (handleNext/handleBack)
- Modify: `src/components/absences/StaffCreateAbsenceModal.tsx:1141-1170` (footer)
- Add: type selection step rendering before Step 1

### Step 1: Update ModalStep type

Change from:
```typescript
type ModalStep = "subjects" | "sessions" | "confirm";
```

To:
```typescript
type ModalStep = "type" | "subjects" | "sessions" | "confirm";
```

### Step 2: Update STEP_KEYS constant

Change from:
```typescript
const STEP_KEYS: ModalStep[] = ["subjects", "sessions", "confirm"];
```

To:
```typescript
const STEP_KEYS: ModalStep[] = ["type", "subjects", "sessions", "confirm"];
```

### Step 3: Add absenceType state

After the existing state declarations (around line 195), add:

```typescript
const [absenceType, setAbsenceType] = useState<"normal" | "special">("normal");
```

### Step 4: Update initial step

Find where `step` is initialized (line 195) and change to:

```typescript
const [step, setStep] = useState<ModalStep>("type");
```

### Step 5: Update handleNext for type step

Update `handleNext` function (line 914):

```typescript
function handleNext() {
    if (step === "type") {
      // Type is already selected via radio buttons, advance to subjects
      setStep("subjects");
    } else if (step === "subjects") {
      if (!canAdvanceFromSubjects()) {
        addToast(
          "error",
          !student ? "Look up a student first" : "Select at least one subject",
        );
        return;
      }
      setSelectedSessionIds(new Set());
      setSitInSelections({});
      clearSpecialSitInState();
      setSitInPriorityLevels({});
      setSitInPriorityHistory({});
      setSessions([]);
      setStep("sessions");
    } else if (step === "sessions") {
      if (selectedSessionCount === 0) {
        addToast("error", "Select at least one missed class");
        return;
      }
      if (!canAdvanceFromSessions()) {
        addToast("error", "Select a special sit-in subject and session");
        return;
      }
      setStep("confirm");
    }
  }
```

### Step 6: Update handleBack for type step

```typescript
function handleBack() {
    if (step === "sessions") setStep("subjects");
    else if (step === "confirm") setStep("sessions");
    else if (step === "subjects") setStep("type");
  }
```

### Step 7: Update footer navigation

In the modal footer, update the Back button logic (around line 1160):

```typescript
{step !== "type" ? (
  <Button variant="secondary" onClick={handleBack}>
    Back
  </Button>
) : null}
```

### Step 8: Add type selection step rendering

Before the "Step 1: Subjects" section (before line 1174), add the type selection step:

```tsx
      {/* Step 0: Absence Type */}
      {step === "type" && (
        <div className="space-y-5">
          <h2 ref={headingRef} tabIndex={-1} className="sr-only">
            Step 1: Select Absence Type
          </h2>
          <p className="text-sm text-gray-600">
            Choose the type of absence to create:
          </p>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <button
              type="button"
              aria-pressed={absenceType === "normal"}
              onClick={() => setAbsenceType("normal")}
              className={`rounded-lg border-2 p-6 text-left transition-colors ${
                absenceType === "normal"
                  ? "border-blue-500 bg-blue-50"
                  : "border-gray-200 hover:border-gray-300"
              }`}
            >
              <div className="flex items-center gap-3">
                <div className={`rounded-full p-2 ${
                  absenceType === "normal" ? "bg-blue-100" : "bg-gray-100"
                }`}>
                  <Info className={`h-5 w-5 ${
                    absenceType === "normal" ? "text-blue-600" : "text-gray-500"
                  }`} />
                </div>
                <div>
                  <p className="font-medium text-gray-900">Normal Absence</p>
                  <p className="text-sm text-gray-500">Requires review and approval</p>
                </div>
              </div>
            </button>
            <button
              type="button"
              aria-pressed={absenceType === "special"}
              onClick={() => setAbsenceType("special")}
              className={`rounded-lg border-2 p-6 text-left transition-colors ${
                absenceType === "special"
                  ? "border-purple-500 bg-purple-50"
                  : "border-gray-200 hover:border-gray-300"
              }`}
            >
              <div className="flex items-center gap-3">
                <div className={`rounded-full p-2 ${
                  absenceType === "special" ? "bg-purple-100" : "bg-gray-100"
                }`}>
                  <Info className={`h-5 w-5 ${
                    absenceType === "special" ? "text-purple-600" : "text-gray-500"
                  }`} />
                </div>
                <div>
                  <p className="font-medium text-gray-900">Special Absence</p>
                  <p className="text-sm text-gray-500">Pre-approved, skips review</p>
                </div>
              </div>
            </button>
          </div>
          {absenceType === "special" && (
            <div className="rounded-lg border border-purple-200 bg-purple-50 p-3 text-sm text-purple-700">
              <p>This absence will be created with <strong>Special Approved</strong> status and will not count toward the student's absence rate limit.</p>
            </div>
          )}
        </div>
      )}
```

Note: `Info` is already imported from lucide-react (line 2). No new icon imports needed.

### Step 9: Run typecheck

```bash
npm run typecheck
```

Expected: No new errors

### Step 10: Commit

```bash
git add src/components/absences/StaffCreateAbsenceModal.tsx
git commit -m "feat(frontend): add absence type selection step to staff create modal"
```

---

## Task 6: Frontend — Pass Status to Backend on Submit

**Files:**
- Modify: `src/components/absences/StaffCreateAbsenceModal.tsx:1033-1052` (handleSubmit)

### Step 1: Pass status in the API request

In `handleSubmit`, update the request body (around line 1038-1050) to include status:

```typescript
        const res = await apiJson<{ id: string; sms_preview?: SmsPreview }>(
          "/api/v1/absences/staff-create",
          {
            method: "POST",
            body: JSON.stringify({
              wcode: student.wcode,
              subject_id: group.subject_id,
              course_id: group.course_id,
              date_from: dateFrom,
              date_to: dateTo,
              missed_session_ids: missedIds,
              sit_in_method: sitInMethod,
              sit_in_course_id: sitInCourseId,
              sit_in_session_ids: uniqueSitInSessionIds,
              reason_category: reasonCategory || undefined,
              reason: reason || undefined,
              status: absenceType === "special" ? "special_approved" : undefined,
            }),
          },
        );
```

### Step 2: Run typecheck

```bash
npm run typecheck
```

Expected: No new errors

### Step 3: Commit

```bash
git add src/components/absences/StaffCreateAbsenceModal.tsx
git commit -m "feat(frontend): pass absence status to backend in staff create flow"
```

---

## Task 7: Frontend — Add SMS Confirm Modal to Table Special Approve Flow

**Files:**
- Modify: `src/pages/Absences.tsx` (state, handleSpecialApprove, render)

### Step 1: Add SMS-related state variables

After the existing `specialApproving` state (around line 161), add:

```typescript
const [specialApprovedSmsPreview, setSpecialApprovedSmsPreview] = useState<SmsPreview | null>(null);
const [specialApprovedSendingSms, setSpecialApprovedSendingSms] = useState(false);
const [specialApprovedCreatedIds, setSpecialApprovedCreatedIds] = useState<string[]>([]);
```

### Step 2: Import SmsPreview type and SmsConfirmModal

Add `SmsPreview` to the existing type imports (line 6):
```typescript
import type { AbsencePage, AbsenceStatus, ManagedAbsence, SmsPreview } from "../types";
```

Add import for SmsConfirmModal (after line 15):
```typescript
import SmsConfirmModal from "../components/absences/SmsConfirmModal";
```

### Step 3: Update handleSpecialApprove to fetch SMS preview

Replace the existing `handleSpecialApprove` function (lines 299-318):

```typescript
  async function handleSpecialApprove() {
    if (!specialApprovedTarget) return;
    setSpecialApproving(true);
    try {
      await apiJson(`/api/v1/absences/${specialApprovedTarget.id}/status`, {
        method: "PUT",
        body: JSON.stringify({ status: "special_approved", expected_version: specialApprovedTarget.version }),
      });
      // The status endpoint must not send SMS for special_approved.
      // SMS is sent only after this preview modal is confirmed.
      addToast("success", "Absence marked as special approved");
      setSpecialApprovedCreatedIds([specialApprovedTarget.id]);

      // Fetch SMS preview
      try {
        const preview = await apiJson<{ preview?: SmsPreview }>(
          "/api/v1/absences/batch-send-success-sms",
          {
            method: "POST",
            body: JSON.stringify({ ids: [specialApprovedTarget.id], dry_run: true }),
          },
        );
        if (preview.preview && preview.preview.phones.length > 0) {
          setSpecialApprovedSmsPreview(preview.preview);
          setSpecialApprovedTarget(null);
          return;
        }
      } catch {
        // SMS preview not critical
      }

      setSpecialApprovedTarget(null);
      await load();
    } catch (err) {
      const msg = err instanceof ApiRequestError && err.code === "stale_edit"
        ? "Absence was changed by another user. Reload and try again."
        : err instanceof Error ? err.message : "Special approve failed";
      addToast("error", msg);
    } finally {
      setSpecialApproving(false);
    }
  }
```

### Step 4: Add SMS send/skip handlers

After `handleSpecialApprove`, add:

```typescript
  async function handleSpecialApprovedSendSms() {
    if (specialApprovedCreatedIds.length === 0) return;
    setSpecialApprovedSendingSms(true);
    try {
      const res = await apiJson<{ sent: boolean; recipient_count: number }>(
        "/api/v1/absences/batch-send-success-sms",
        { method: "POST", body: JSON.stringify({ ids: specialApprovedCreatedIds }) }
      );
      if (res.sent) {
        addToast("success", `SMS notification sent to ${res.recipient_count} recipient(s)`);
      } else {
        addToast("error", "SMS was not sent");
      }
      setSpecialApprovedSmsPreview(null);
      setSpecialApprovedCreatedIds([]);
      await load();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "SMS send failed");
    } finally {
      setSpecialApprovedSendingSms(false);
    }
  }

  function handleSpecialApprovedSkipSms() {
    if (specialApprovedSendingSms) return;
    addToast("success", "Absence marked as special approved (SMS skipped)");
    setSpecialApprovedSmsPreview(null);
    setSpecialApprovedCreatedIds([]);
    void load();
  }
```

### Step 5: Render SmsConfirmModal for special approve

After the special approve confirmation modal (after line 742), add:

```tsx
      {specialApprovedSmsPreview && specialApprovedCreatedIds.length > 0 ? (
        <SmsConfirmModal
          phones={specialApprovedSmsPreview.phones}
          message={specialApprovedSmsPreview.message}
          onSend={() => void handleSpecialApprovedSendSms()}
          onSkip={handleSpecialApprovedSkipSms}
          sending={specialApprovedSendingSms}
        />
      ) : null}
```

### Step 6: Run typecheck

```bash
npm run typecheck
```

Expected: No new errors

### Step 7: Run frontend tests

```bash
npx vitest run src/pages/__tests__/Absences.test.tsx
```

Expected: Tests pass. Include assertions that special approval makes exactly these API calls in order:

1. `PUT /api/v1/absences/:id/status`
2. `POST /api/v1/absences/batch-send-success-sms` with `dry_run: true`
3. only after clicking Send, `POST /api/v1/absences/batch-send-success-sms` without `dry_run`

There must be no implicit SMS send during the status update.

### Step 8: Commit

```bash
git add src/pages/Absences.tsx
git commit -m "feat(frontend): add SMS confirm modal to table special approve flow"
```

---

## Task 8: Update Existing Tests

**Files:**
- Modify: `backend/internal/httpapi/absenceshttp/staff_create_test.go`
- Modify: `backend/internal/httpapi/absenceshttp/management_routes_test.go`
- Modify: `backend/internal/httpapi/absenceshttp/success_sms_test.go`
- Modify: `backend/internal/db/absence_integration_test.go`
- Modify: `src/components/absences/__tests__/StaffCreateAbsenceModal.test.tsx`
- Modify: `src/pages/__tests__/Absences.test.tsx`
- Modify: `src/pages/__tests__/AbsenceSettings.test.tsx`
- Modify: `src/components/absences/__tests__/KanbanView.test.tsx`

### Step 1: Backend tests

Add or update tests for:

- `staff-create` rejects unsupported `status` and trims valid values.
- `staff-create` with `status: "special_approved"` persists that status, writes both `created_by_staff` and `special_approved` timeline entries, returns a special-template preview, and publishes the created ID.
- `special_approved` is accepted by status validation and follows the Task 0 transition policy, including `special_approved -> cancelled`.
- `PUT /status` to `special_approved` does not call the SMS provider.
- Single success SMS send uses the special template for `special_approved`.
- Batch success SMS uses the special template for all-special IDs.
- Batch success SMS rejects mixed normal/special IDs when the special template is configured.
- Legacy settings JSON without `sms_special_approved_template` receives the default field.
- `StudentAbsenceCountForCourse` excludes cancelled and special-approved absences.
- Archived list bucket includes special-approved absences; active bucket does not.

### Step 2: Frontend tests

In `StaffCreateAbsenceModal.test.tsx`, cover:

- default type is normal and the request omits `status`,
- choosing Special Absence sends `status: "special_approved"`,
- going back from subjects returns to the type step without losing selected type unexpectedly.

In `Absences.test.tsx`, the special approve test should mock both the status update and the batch-send-success-sms dry-run endpoint. Add assertions for the call sequence from Task 7 and for Send/Skip behavior.

In settings tests, cover editing and saving `sms_special_approved_template`.

In Kanban/list tests, cover special-approved visibility according to Task 4.5.

### Step 3: Run all affected tests

```bash
cd backend && go test ./internal/absences ./internal/db ./internal/httpapi/absenceshttp
npx vitest run src/pages/__tests__/Absences.test.tsx
npx vitest run src/components/absences/__tests__/StaffCreateAbsenceModal.test.tsx src/pages/__tests__/AbsenceSettings.test.tsx src/components/absences/__tests__/KanbanView.test.tsx
```

Expected: All tests pass

### Step 4: Commit

```bash
git add backend/internal/httpapi/absenceshttp backend/internal/db src/components/absences/__tests__/StaffCreateAbsenceModal.test.tsx src/pages/__tests__/Absences.test.tsx src/pages/__tests__/AbsenceSettings.test.tsx src/components/absences/__tests__/KanbanView.test.tsx
git commit -m "test: update tests for special absence type feature"
```

---

## Task 9: End-to-End Verification

### Step 1: Run full Go test suite

```bash
cd backend && go test ./...
```

Expected: All tests pass

### Step 2: Run full frontend test suite

```bash
npm test
```

Expected: All tests pass

### Step 3: Run typecheck

```bash
npm run typecheck
```

Expected: No errors

### Step 4: Run build

```bash
npm run build
```

Expected: Build succeeds

### Step 5: Manual smoke test

1. Open the app, go to Absences page
2. Click "Create Absence"
3. Verify the type selection step appears with "Normal Absence" and "Special Absence" options
4. Select "Special Absence" → verify purple info box appears
5. Click Next → verify it goes to subjects step
6. Complete the full flow (subjects → sessions → sit-in → confirm) → verify absence is created with `special_approved` status
7. Verify SMS preview shows the special template
8. In the table, find a pending absence → click "Special Approve"
9. Verify confirmation modal appears → confirm
10. Verify SMS confirm modal appears with the special template
11. Verify no SMS is sent until clicking "Send SMS" in `SmsConfirmModal`
12. Click Send SMS → verify exactly one SMS send request is made
13. Repeat with Skip → verify status remains `special_approved`, no SMS send request is made, and the list refreshes
14. Switch to archived/status filters → verify `special_approved` records are discoverable
15. Cancel a special-approved absence → verify it becomes `cancelled` and no invalid-transition error appears
16. Try a mixed normal + special batch SMS dry-run via API/test fixture → verify `400 mixed_status_sms_templates`

### Step 6: Final commit (if any fixes needed)

```bash
git add -A && git commit -m "fix: address review feedback for special absence type"
```

---

## Summary

| Task | Description | Files Changed |
|------|-------------|---------------|
| 0 | Backend: Add status/database invariants | migration, status package, DB queries/tests |
| 1 | Backend: Accept status in staff-create | `staff_create.go` |
| 2 | Backend: Add special SMS template setting | `management_routes.go` |
| 3 | Backend: Use correct template in explicit SMS send/preview paths | `management_routes.go`, `staff_create.go` |
| 4 | Frontend: Add status/settings types and defaults | `types.ts`, `constants.ts`, `absenceFormApi.ts`, `AbsenceFormEditor.tsx` |
| 4.5 | Frontend: Keep special-approved records discoverable | `Absences.tsx`, `KanbanView.tsx` |
| 5 | Frontend: Add type selection step | `StaffCreateAbsenceModal.tsx` |
| 6 | Frontend: Pass status to backend | `StaffCreateAbsenceModal.tsx` |
| 7 | Frontend: Add SMS modal to table flow | `Absences.tsx` |
| 8 | Update tests | Test files |
| 9 | End-to-end verification | All |

**Net result:** Staff can create "Special Absences" that start as `special_approved`, with a separate Thai SMS template. The table "Special Approve" action shows an SMS confirm modal. The flow is identical to normal absences (subjects → sessions → sit-in → confirm) — only the status and SMS template differ.
