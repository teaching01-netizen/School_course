---
phase: 02-code-review
reviewed: 2026-05-31T14:00:00Z
depth: standard
files_reviewed: 2
files_reviewed_list:
  - src/pages/AbsenceForm.tsx
  - src/components/absences/StepCoverVerification.tsx
findings:
  critical: 0
  warning: 0
  info: 0
  total: 0
status: clean
---

# Phase 2: Code Review Report — Task 3: Step 1 Layout (Compact Badge)

**Reviewed:** 2026-05-31T14:00:00Z
**Depth:** standard
**Files Reviewed:** 2
**Status:** clean

## Summary

Verified Task 3 (AbsenceForm.tsx — Step 1 Layout) implementation against spec. The full student profile card has been removed from Step 1 and replaced with a compact inline badge next to the heading. StepCoverVerification flows directly in the section with no competing nested cards. All existing functionality is preserved. All acceptance criteria are met.

## Spec Compliance Checklist

| Requirement | Status | Evidence |
|---|---|---|
| Remove full student profile card from Step 1 | ✅ | No `rounded-sm border border-gray-250 bg-gray-50` card in step 1 (lines 1045-1130). Full card correctly lives in step 0 (lines 1019-1030). |
| Replace with inline badge next to heading | ✅ | Lines 1047-1062: `<h2>Parent Verify</h2>` followed by `<span>· {lookup.full_name} ({lookup.wcode}) · Parent phone {maskPhone(...)}</span>` |
| Badge styling `text-sm text-gray-600 font-normal` | ✅ | Line 1058: exact className match |
| Remove extra nesting — StepCoverVerification flows directly | ✅ | Only `<div className="space-y-6">` (zero-visual wrapper) wraps content. No nested card borders/backgrounds. |
| "Continue to courses" button stays after verification | ✅ | Lines 1095-1115: present with `verificationSatisfied` gate |
| Student context visible but minimal | ✅ | Badge shows name, wcode, masked parent phone in a single line |
| OTP zone has maximum visual real estate | ✅ | No card wrapper competing for space — just the section border |
| No nested cards competing for attention | ✅ | Single `<section>` border only. No inner `border bg-gray-50` cards. |
| All existing functionality preserved | ✅ | StepCoverVerification component fully intact with all props (wcode, parentPhone, allowSubmitWithoutOtp, adminContact, verification, completed, onSatisfied, onWcodeChange) |

## Notes

- The full student profile card is correctly placed in Step 0 (Student Lookup) at lines 1019-1030, providing detailed context during the lookup phase. Step 1 (Parent Verify) uses the compact badge for minimal context during OTP verification — this is the correct separation of concerns.
- The `maskPhone()` utility (lines 74-79) correctly formats phone as `089 *** 123` and falls back to `""` when no phone exists, with the `|| "not on file"` fallback on line 1059 handling that edge case.
- The `space-y-6` wrapper div (line 1064) is a standard Tailwind spacing utility, not "extra nesting" in the visual/card sense. It provides clean vertical rhythm without adding competing visual layers.

---

_Reviewed: 2026-05-31T14:00:00Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
