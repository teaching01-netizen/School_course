---
phase: sms-template-review
reviewed: 2026-05-31T00:00:00Z
depth: standard
files_reviewed: 5
files_reviewed_list:
  - src/types/index.ts
  - src/pages/AbsenceForm.tsx
  - src/components/absences/AbsenceFormEditor.tsx
  - src/pages/__tests__/AbsenceForm.test.tsx
  - src/pages/AbsenceSettings.tsx
findings:
  critical: 1
  warning: 3
  info: 2
  total: 6
status: issues_found
---

# Phase: SMS Template Updates — Code Review Report

**Reviewed:** 2026-05-31
**Depth:** standard
**Files Reviewed:** 5
**Status:** issues_found

## Summary

Reviewed the SMS template additions for the absence form feature: new Thai verification/success templates, `nickname` field on `StudentLookupResponse`, `sms_success_template` on `AbsenceNotificationsSettings`, editor UI in `AbsenceFormEditor`, display in `AbsenceSettings`, and updated test mocks.

Type definitions are clean. Thai template strings are well-formed with correct placeholders. The test mock updates are consistent with the production defaults. However, the `AbsenceFormEditor` notification onChange handlers contain a security-relevant default mismatch, the `AbsenceSettings` save violates the project's idempotency contract, and there is a confusing duplicate SMS display.

---

## Critical Issues

### CR-01: `allow_submit_without_otp` defaults to `true` in editor fallbacks — contradicts production default of `false`

**File:** `src/components/absences/AbsenceFormEditor.tsx:165,171,176`
**Issue:** All three SMS-related `onChange` handlers in `AbsenceFormEditor` use `allow_submit_without_otp: settings.notifications?.allow_submit_without_otp ?? true` as the fallback when `settings.notifications` is undefined/null. However, `AbsenceForm.tsx:41` defines the production default as `false` (parent verification required). On a fresh installation where `notifications` is null, toggling the SMS checkbox or editing either SMS textarea will set `allow_submit_without_otp` to `true`, silently disabling parent verification. This is a security-relevant semantic mismatch: the editor permits submission without OTP when the production form would require it.

**Fix:**
```tsx
// AbsenceFormEditor.tsx — all three onChange handlers
// Change the fallback from `?? true` to `?? false`:
allow_submit_without_otp: settings.notifications?.allow_submit_without_otp ?? false
```

---

## Warnings

### WR-01: Missing `Idempotency-Key` header on settings PUT request

**File:** `src/pages/AbsenceSettings.tsx:66`
**Issue:** The `save()` function sends a `PUT` request without an `Idempotency-Key` header. The project's `CONTEXT.md` mandates: *"all side-effecting POST/PUT/PATCH/DELETE HTTP endpoints (except auth + preflight) require Idempotency-Key header."* Network retries or double-clicks could create duplicate writes.

**Fix:**
```tsx
import { newIdempotencyKey } from "../api/client";
// ...
const updated = await apiJson<AbsenceSettingsModel>("/api/v1/admin/absence-settings", {
  method: "PUT",
  headers: { "Idempotency-Key": newIdempotencyKey() },
  body: JSON.stringify(settings),
});
```

### WR-02: Duplicate SMS template display — editable in AbsenceFormEditor, read-only in AbsenceSettings

**File:** `src/pages/AbsenceSettings.tsx:108-120`
**Issue:** `AbsenceSettings` passes `showTextEditors={true}` to `AbsenceFormEditor`, which renders an editable SMS Notifications section (lines 157–181 of `AbsenceFormEditor.tsx`). Lines 108–120 of `AbsenceSettings` then render a *second*, read-only "SMS Templates" section showing the same template text. Users see SMS templates displayed twice — once editable, once static — which is confusing and will drift out of sync if only one section is updated before save.

**Fix:** Remove the read-only "SMS Templates" section (lines 108–120) from `AbsenceSettings.tsx`. The editable section in `AbsenceFormEditor` already covers display and editing.

### WR-03: Fragile notification object reconstruction — same fallback values hardcoded in every onChange handler

**File:** `src/components/absences/AbsenceFormEditor.tsx:165,171,176`
**Issue:** Each of the three SMS-related `onChange` handlers manually reconstructs the full `notifications` object with identical hardcoded fallbacks for every field (`sms_parent_enabled`, `sms_parent_template`, `sms_success_template`, `allow_submit_without_otp`). If a field is added to `AbsenceNotificationsSettings` in the future, all three handlers must be updated in lockstep or the new field will be silently dropped on save. This is error-prone.

**Fix:** Extract a helper:
```tsx
function patchNotifications(
  current: AbsenceSettings["notifications"],
  patch: Partial<AbsenceNotificationsSettings>,
): AbsenceNotificationsSettings {
  return {
    sms_parent_enabled: current?.sms_parent_enabled ?? false,
    sms_parent_template: current?.sms_parent_template ?? "",
    sms_success_template: current?.sms_success_template ?? "",
    allow_submit_without_otp: current?.allow_submit_without_otp ?? false,
    ...patch,
  };
}
// Then in each handler:
onChange={{
  ...settings,
  notifications: patchNotifications(settings.notifications, { sms_parent_template: e.target.value }),
}}
```

---

## Info

### IN-01: Extremely long single-line onChange handlers reduce readability

**File:** `src/components/absences/AbsenceFormEditor.tsx:165,171,176`
**Issue:** The three SMS onChange handlers are each 200+ character single lines. Combined with the fragile reconstruction pattern (WR-03), these are difficult to review and easy to mistype.

**Fix:** Extract the notification patching logic (per WR-03) and break the JSX across lines. Each handler should be at most ~100 characters wide.

### IN-02: Cancel button uses `window.location.reload()` as undo

**File:** `src/pages/AbsenceSettings.tsx:123`
**Issue:** The Cancel button calls `window.location.reload()` to discard unsaved changes. While functional, this is a jarring full-page reload. A confirmation dialog or navigating back to a settings list would be less disruptive.

**Fix:** Consider `window.history.back()` or a confirmation prompt before reload.

---

## Structural Findings (fallow)

No structural findings were provided for this review cycle.

---

_Reviewed: 2026-05-31_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
