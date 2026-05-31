---
phase: 03-re-review-task-3
reviewed: 2026-05-31T12:00:00Z
depth: standard
files_reviewed: 2
files_reviewed_list:
  - src/pages/AbsenceForm.tsx
  - src/pages/__tests__/AbsenceForm.test.tsx
findings:
  critical: 0
  warning: 0
  info: 1
  total: 1
status: issues_found
---

# Phase 3: Re-Review of Task 3 Fixes

**Reviewed:** 2026-05-31T12:00:00Z
**Depth:** standard
**Files Reviewed:** 2
**Status:** issues_found

## Summary

Verified three fixes applied to `AbsenceForm.tsx` and `AbsenceForm.test.tsx`. All functional fixes are correct — `setShowReasonFields(false)` added in both reset paths, `layout="position"` removed from motion.div, and new test covers reason collapsible behavior. One cosmetic issue remains (inconsistent indentation in injected code block).

## Issues

### Info

#### IN-01: Inconsistent indentation in `onWcodeChange` handler

**File:** `src/pages/AbsenceForm.tsx:1005-1008`
**Issue:** Lines 1005-1008 have one extra space of indentation (28 spaces vs 27 spaces on surrounding lines 992-1004). This is a formatting artifact from the injected fix not matching the surrounding indentation depth.

```typescript
// Lines 992-1004 use 27-space indent:
                           setLookup(null);           // 27 spaces
                           ...
                           setSubmissionError(null);   // 27 spaces
// Lines 1005-1008 use 28-space indent:
                            setShowReasonFields(false);  // 28 spaces
                            setVerificationSatisfied(false);
                            verification.clearStoredToken();
                            verification.setCode("");
```

While not breaking functionality, this inconsistency indicates the lines were inserted without matching surrounding context. If a formatter is introduced later, this will be auto-corrected, but as-is it reads as a sloppy patch.

**Fix:** Normalize indentation to match the surrounding block (26 spaces, matching the closure context):

```typescript
                        onWcodeChange={() => {
                          setLookup(null);
                          setLookupError(null);
                          setSelectedSubjectIds([]);
                          setActiveCourseIndex(0);
                          setDateFrom("");
                          setDateTo("");
                          setReasonCategory("");
                          setReason("");
                          setSessions([]);
                          setSelectedSessionIds(new Set());
                          setCoverSessionIds(new Set());
                          setPageError(null);
                          setSubmissionError(null);
                          setShowReasonFields(false);
                          setVerificationSatisfied(false);
                          verification.clearStoredToken();
                          verification.setCode("");
                        }}
```

## Fix Verification Matrix

| Check | Expected | Actual | Result |
|---|---|---|---|
| `setShowReasonFields(false)` in `handleReset()` | Present at line ~734 | Line 734 ✅ | Pass |
| `setShowReasonFields(false)` in `onWcodeChange` | Present in handler | Line 1005 ✅ | Pass |
| `layout="position"` removed from motion.div | Not present as prop | No `layout` prop on any motion.div ✅ | Pass |
| Test: reason hidden initially | `not.toBeInTheDocument()` | Line 284 ✅ | Pass |
| Test: reason visible after courses+dates | `toBeInTheDocument()` | Line 288 ✅ | Pass |
| Test: "Optional" badge shown | `toBeInTheDocument()` | Line 289 ✅ | Pass |
| Test: dropdown hidden before click | `not.toBeInTheDocument()` | Line 290 ✅ | Pass |
| Test: expandable on click | dropdown appears | Line 293 ✅ | Pass |
| No new issues introduced | No regression | One cosmetic indent issue ✅ | Pass (Info) |

## Assessment: **Approve**

All three fixes are correctly implemented. The reason state is properly reset on both form reset and wcode change (no stale state leak). The animation prop is correctly removed. The new test is structurally correct and covers all four states of the reason collapsible section. The only finding is a cosmetic indentation inconsistency (1 extra space on 4 injected lines), which does not affect functionality.

---

_Reviewed: 2026-05-31T12:00:00Z_
_Reviewer: gsd-code-reviewer_
_Depth: standard_
