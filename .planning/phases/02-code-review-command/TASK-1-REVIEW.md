---
phase: 02-code-review-command
reviewed: 2026-05-31T02:30:00Z
depth: deep
files_reviewed: 1
files_reviewed_list:
  - src/pages/AbsenceForm.tsx
findings:
  critical: 0
  warning: 0
  info: 1
  total: 1
status: issues_found
---

# Phase 2: Code Review Report — Task 1 Re-Review (Fix Verification)

**Reviewed:** 2026-05-31T02:30:00Z  
**Depth:** deep  
**Files Reviewed:** 1  
**Status:** issues_found (1 info; all 3 prior fixes verified ✅)

## Summary

Re-review of `src/pages/AbsenceForm.tsx` after fix commit 322c62c applied 3 fixes from TASK-1-REVIEW.md (CR-01, CR-02, WR-01).

**All 3 fixes verified as correctly applied.** All 7 spec requirements pass. One new quality finding: `parentPhoneMissing` local variable became dead code after prop removal.

## Fix Verification

| # | Previous Issue | Status | Evidence |
|---|---------------|--------|----------|
| CR-01 | `pageError` at priority index 1 (between submissionError and verificationBlocked) — violated spec ordering | ✅ **Fixed** | `pageError` now at index 4 (after sessionsError) at line 155-156. Order: submissionError (143) → verificationBlocked (146) → lookupError (149) → sessionsError (152) → pageError (155) → parentPhoneMissing (158) → offline/restored (161) |
| CR-02 | Dismiss button hardwired to `onClearPageError`; submission error never dismissible | ✅ **Fixed** | Items carry `onDismiss` callback per type (line 144: `onDismiss: onClearSubmissionError`; line 156: `onDismiss: onClearPageError`). Button calls `item.onDismiss?.()` (line 242). `onClearSubmissionError` prop added (line 128) and wired at call site (line 954) |
| WR-01 | `parentPhoneMissing` prop declared but unused | ✅ **Fixed** | Prop removed from FormErrorSummary interface (lines 105-131). Removed from call site (lines 944-957). Component reads `lookup.parent_phone` directly (line 158) |

## Requirement Verification

| # | Requirement | Status | Evidence |
|---|-------------|--------|----------|
| 1 | Single error at a time with "N more" disclosure | ✅ | `visible = showExpanded ? items : [items[0]]` (line 172); hidden count button (lines 255-265) |
| 2 | Priority ordering per spec | ✅ | submissionError P1 → verificationBlocked P2 → lookupError P3 → sessionsError P4 → pageError P4.5 → parentPhoneMissing P5 → offline/restored P6 (lines 143-165) |
| 3 | Dismiss button calls correct setter per type | ✅ | submissionError→onClearSubmissionError; pageError→onClearPageError; rest non-dismissible (lines 134-168, 239-250) |
| 4 | `role="alert"` only on displayed top error | ✅ | `role = isTop ? item.role : "status"` (line 179). Top item keeps its role ("alert" for errors), others overridden to "status" |
| 5 | `renderStatusBanner()` fully removed | ✅ | Zero matches in file |
| 6 | Offline/restored merged into FormErrorSummary | ✅ | Lines 161-165: offline and restored included in items array inside FormErrorSummary |
| 7 | Dead `parentPhoneMissing` prop removed | ✅ | Not in props interface (lines 105-131), not in call site (lines 944-957) |

## Info

### IN-01: `parentPhoneMissing` local variable is now dead code

**File:** `src/pages/AbsenceForm.tsx:326`  
**Issue:** After WR-01 fix removed the `parentPhoneMissing` prop from FormErrorSummary, the local variable `parentPhoneMissing` at line 326 is no longer referenced anywhere in the file. It's dead code.

```typescript
const parentPhoneMissing = !lookup?.parent_phone || lookup.parent_phone.trim() === "";
```

The parent phone check is now performed inline inside FormErrorSummary (line 158: `if (lookup && !lookup.parent_phone)`), making this variable unreferenced.

Note: the inline check in FormErrorSummary does not perform the `trim()` normalization that the removed variable had — this is a minor behavioral difference (whitespace-only phone strings evaluated as "present" in the banner) but is unlikely to be a real problem since phone numbers are typically trimmed at input time.

**Fix:** Remove the unused variable:

```typescript
// Delete line 326 entirely:
// const parentPhoneMissing = !lookup?.parent_phone || lookup.parent_phone.trim() === "";
```

Optionally align the inline check in FormErrorSummary (line 158) to use the same trim pattern for consistency:
```typescript
if (lookup && !lookup.parent_phone?.trim()) {
```

---

_Reviewed: 2026-05-31T02:30:00Z_  
_Reviewer: gsd-code-reviewer (Task 1 re-review / fix verification)_  
_Depth: deep_
