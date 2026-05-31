---
phase: 02-code-review
reviewed: 2026-05-31T23:00:00Z
depth: standard
files_reviewed: 1
files_reviewed_list:
  - src/pages/AbsenceForm.tsx
findings:
  critical: 0
  warning: 2
  info: 2
  total: 4
status: issues_found
---

# Phase 2: Code Review Report — Task 3: Step 1 Layout Compact Badge

**Reviewed:** 2026-05-31T23:00:00Z
**Depth:** standard
**Files Reviewed:** 1
**Status:** issues_found

## Summary

Reviewing the Task 3 changes to `AbsenceForm.tsx` lines 1045-1130: the full student profile card was removed from Step 1 and replaced with a compact inline badge next to the "Parent Verify" heading. StepCoverVerification now flows directly in the section without card nesting. The "Continue to courses" button is preserved after verification success.

The implementation is clean and matches the plan spec. Two warnings and two info items identified — no critical issues.

## Warnings

### WR-01: `maskPhone` fallback wording inconsistency with Step 0

**File:** `src/pages/AbsenceForm.tsx:1059`
**Issue:** The Step 1 badge uses `maskPhone(lookup.parent_phone) || "not on file"` (lowercase), while Step 0's student profile card at line 1027 uses `"No parent phone on file"` (capitalized, different phrasing). Both render when `parent_phone` is null/empty, but the text is inconsistent: `"not on file"` vs `"No parent phone on file"`.

**Why it matters:** A user navigating between steps sees two different fallback messages for the same data condition. This creates a subtle UX inconsistency that undermines polish.

**Fix:** Align wording in one direction. If the compact badge should match Step 0's voice:
```tsx
<span className="text-sm text-gray-600 font-normal">
  · {lookup.full_name} ({lookup.wcode}) · {lookup.parent_phone ? `Parent phone ${maskPhone(lookup.parent_phone)}` : "No parent phone on file"}
</span>
```

### WR-02: Session storage key bump (v2→v3) without step value validation on restore

**File:** `src/pages/AbsenceForm.tsx:34, 478-479`
**Issue:** The `SESSION_STORAGE_KEY` was bumped from `v2` to `v3`, which correctly prevents stale state restore. However, the restore path at line 478-479 does `goTo(parsed.step as StepIndex)` without validating that `parsed.step` is one of `0 | 1 | 2 | 3`. The `useWizard.goTo` (useWizard.ts:11) casts the value directly to state without clamping. If a corrupted or malicious sessionStorage value contained `step: 99`, the wizard would render an empty state (no matching `{step === N}` block).

**Why it matters:** Low risk in practice (sessionStorage is same-origin), but an unclamped restore is a defensive programming gap. A future code change adding `step === 4` logic could silently break.

**Fix:** Clamp on restore:
```typescript
if (typeof parsed.step === "number" && parsed.step >= 0 && parsed.step <= 3) {
  goTo(parsed.step as StepIndex);
}
```

## Info

### IN-01: `onWcodeChange` inline callback resets 12 state variables — extract to named handler

**File:** `src/pages/AbsenceForm.tsx:1073-1092`
**Issue:** The `onWcodeChange` callback is a 20-line inline arrow function that resets lookup, subjects, dates, reason, sessions, verification, and navigation state. This creates a new function reference every render and is a maintenance trap — any new state variable added to the form must remember to add a reset here.

**Why it matters:** Not a bug, but a correctness risk over time. When new form state is added (e.g., a future "notes" field), forgetting to reset it here would cause stale data to persist across W-code changes.

**Fix:** Extract to a named callback with `useCallback`, or consolidate into a single `resetForm()` function that initializes all state to defaults:
```typescript
const handleWcodeChange = useCallback(() => {
  setLookup(null);
  setLookupError(null);
  // ... other resets
  goTo(0);
}, [goTo, verification, ...otherDeps]);
```

### IN-02: Heading container renders `<span>` badge as non-semantic text — acceptable but worth noting

**File:** `src/pages/AbsenceForm.tsx:1057-1061`
**Issue:** The student context badge is a plain `<span>` inside a flex container alongside the `<h2>`. This is semantically acceptable (supplementary info next to a heading), but the leading `· ` character in the span means the heading reads as "Parent Verify · John Smith..." which could confuse screen readers into reading it as a single heading.

**Why it matters:** Very minor accessibility consideration. Screen readers will concatenate the h2 text and the span text since they're siblings in a flex container. The `·` separator helps sighted users but may read as punctuation noise for assistive technology.

**Fix:** If accessibility polish is desired, wrap the badge in a `<span role="separator" aria-hidden="true">` or use CSS `::before` pseudo-element for the dots instead of literal characters. Low priority.

## Structural Findings (fallow)

No structural findings were provided for this review cycle.

## Summary of Changes Reviewed

The diff (HEAD~1) shows this changeset also modified `StepCoverVerification.tsx` and `OtpInput.tsx` (Tasks 1 and 2). For reference, those companion changes include:
- **OtpInput.tsx**: Hero digit boxes (h-16 w-16, text-3xl, blinking cursor, active position highlight)
- **StepCoverVerification.tsx**: Card wrapper removed, verify button promoted to full-width with CheckCircle icon, countdown de-emphasized, `maskPhone` removed (moved to AbsenceForm.tsx), "Parent verification" h3 removed, error alerts now have AlertCircle icons, `variant="ghost"` on "Change W-code"

---

_Reviewed: 2026-05-31T23:00:00Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
