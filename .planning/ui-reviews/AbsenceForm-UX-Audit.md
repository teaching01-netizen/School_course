# AbsenceForm — Interaction Design & UX Audit

**Audited:** 30 May 2026
**File:** `src/pages/AbsenceForm.tsx` (1333 lines)
**Supporting:** 8 components in `src/components/absences/`, 3 hooks
**Form type:** 4-step student-facing wizard for reporting an absence to parent/guardian

---

## Table of Contents

1. [Structural / Flow Issues](#1-structural--flow-issues)
2. [Step-by-Step Findings](#2-step-by-step-findings)
3. [Navigation & State Issues](#3-navigation--state-issues)
4. [Validation & Error UX](#4-validation--error-ux)
5. [Keyboard & Accessibility](#5-keyboard--accessibility)
6. [Technical Architecture Issues](#6-technical-architecture-issues)
7. [Top 5 Most Impactful Fixes](#7-top-5-most-impactful-fixes)
8. [Full Issue Register](#8-full-issue-register)

---

## 1. Structural / Flow Issues

### 1.1 Verification before selection is the wrong ordering

**Problem:** Step 0 forces parent OTP verification BEFORE the student selects courses, dates, or sessions (AbsenceForm.tsx lines 328-332). The page header (line 813) says: *"To submit this absence, your parent or guardian will need to confirm it by text message."*

**Why this fails:** The parent receives a text saying "Your child is reporting an absence" but NO details about what absence—because nothing has been selected yet. The student must pester their parent for a code before they even know what they're reporting. This creates friction for the primary user (student) and confusion for the secondary user (parent).

**Impact:** High abandonment at step 0. The most cognitively demanding step (coordinating with a parent) is the first thing the user sees, before they've invested any effort.

**Fix:** Move verification to step 2 (after course/date selection but before session detail). This lets the student build context and then say "I need to report these 3 biology sessions, can you verify?" to the parent with specifics.

### 1.2 Force-back to step 0 on verification loss

**Lines:** 328-332
```tsx
useEffect(() => {
  if (!verificationSatisfied && step > 0) {
    goTo(0);
  }
}, [goTo, step, verificationSatisfied]);
```

**Problem:** If `verificationSatisfied` becomes falsy while the user is on step 2 or 3, they are **brutally ejected back to step 0**. This can happen if:
- The sessionStorage restores a saved `step > 0` but the OTP token expired between visits
- The verification token expires mid-session (the `useOtp` hook has expiry tracking)
- Any intermediate state flip resets verification

**Why this fails:** The user loses all context. They were on step 2 selecting sessions; suddenly they're back at the W-Code input. The form data is preserved (sessionStorage) but the visual context is gone. No toast explains why they were moved.

**Fix:** Instead of a forced `goTo(0)`, show an inline banner: "Your verification has expired. Please verify again to continue." Block progression (disable "Continue" buttons) but do NOT force a navigation jump.

### 1.3 Only one course processed at submission

**Lines:** 625-640, 717-721
```tsx
const activeGroup = useMemo(
  () => activeGroupForLookup(lookup, selectedSubjectIds, activeCourseIndex),
  [lookup, selectedSubjectIds, activeCourseIndex],
);
const activeSessions = useMemo(() => {
  if (!activeSubjectId) return [];
  return sessions.filter((group) => group.subject_id === activeSubjectId);
}, [sessions, activeSubjectId]);
const primarySessionGroup = activeSessions[0] ?? null;
```

**Problem:** The UI allows selecting multiple courses in step 1 and shows sessions grouped by course. But `buildSubmissionPayload()` uses `primarySessionGroup` (the first active group only) for `course_id` and `sit_in_course_id`. Step 2 only shows the **active** course's sessions. The other selected courses' sessions are hidden.

**Why this fails:** A student taking Biology AND Chemistry who selects both courses in step 1 will only see Biology sessions in step 2. Chemistry is silently ignored. The "selected 2 courses" in step 1 creates an expectation that both will be reported.

**Fix:** Either (a) iterate all selected courses in step 2 with tabbed/accordion UI and submit each in sequence, or (b) simplify to single-course only and remove multi-select from step 1.

---

## 2. Step-by-Step Findings

### Step 0: Lookup & Verify (lines 886-986)

#### 2.1 OTP auto-verify is invisible

**Lines:** AbsenceForm.tsx (via StepCoverVerification.tsx lines 122-138, OtpInput.tsx lines 39-44)

**Problem:** When the 6th digit is entered, `handleVerify()` fires automatically via the effect in StepCoverVerification. The user sees no loading spinner or confirmation before the parent phone gets verified. The flow is:
1. User types 6th digit
2. Effect detects `code.length === 6`
3. API call fires
4. Either: parent gets verified (page updates) or error flashes

**Why this fails:** No user agency. The auto-verify can race against the user pressing the "Verify code" button. If it fails (wrong code), the error appears and the code is cleared (lines 187-189 of StepCoverVerification), but the user doesn't know their code was even sent. The user may re-type the same wrong code.

**Fix:** Remove auto-verify. Require explicit button press for "Verify code". Add a brief confirmation dialog: "Send this code to verify?"

#### 2.2 "Send verification code" / "Resend verification code" label swap

**Line:** StepCoverVerification.tsx line 308
```tsx
{sendCount > 0 ? "Resend verification code" : "Send verification code"}
```

**Problem:** On first send, button says "Send verification code". After one send, it says "Resend verification code". But the initial send also **starts the 60-second cooldown** (line 156). The label change is correct but leaves no indication of *how many* codes have been sent. No limit enforcement despite obvious abuse potential.

**Fix:** Add rate-limit awareness. Show "Code sent (1/5)" counter. Grey out the button after N attempts server-side.

#### 2.3 Skip-verification is hidden

**Lines:** StepCoverVerification.tsx lines 195-203, 311-315

**Problem:** The "Continue without verification" button only appears when `allowSubmitWithoutOtp` is true (admin config). This is correct, but when it appears, it's placed NEXT to the "Send verification code" button. The user's eye path:
1. Read "Enter the W-code first, then verify"
2. See "Send verification code" button
3. See "Continue without verification" button
4. The inconsistency creates confusion: "Do I need to verify or not?"

**Fix:** If skip is available, make it the tertiary option below the verification section, not alongside the primary CTA.

#### 2.4 OTP input has "overlay" pattern — visual affordance gap

**Lines:** OtpInput.tsx lines 82-97

**Problem:** The actual `<input>` has `className="absolute left-0 top-0 h-px w-px opacity-0"`. This visually-hidden-input pattern is legitimate for OTP but creates an invisible target. The visible divs (6 boxes) are `aria-hidden="true"` divs styled to look like input cells. Clicking them focuses the invisible input via the parent's `onClick` handler (line 65).

**Why this fails:** The `onClick` handler on the containing div is the only way to focus the input. If a user clicks a box margin or outside the grid, focus won't transfer. The 0-opacity input has `h-px w-px` which is dangerously small—some mobile browsers may refuse to focus such elements.

**Fix:** Use a single visible `<input>` with `font-mono text-2xl tracking-[1em]` and overlay the digit display as a decorative enhancement. Or make the hidden input `h-full w-full` with `absolute inset-0` so the full area is the hit target.

### Step 1: Courses & Dates (lines 988-1119)

#### 2.5 No loading placeholder while sessions fetch

**Lines:** None — there is no indication that sessions will load on the next step

**Problem:** When the user clicks "Continue to sessions" (after `validateStepOne()`), they navigate to step 2 where `sessionsLoading` shows "Loading sessions…". The user had no expectation that an API call was needed. The transition feels slow.

**Fix:** Show a loading message in the "Continue" button: "Loading sessions…" while the request is in-flight. Or defer the navigation until sessions resolve.

#### 2.6 Changing dates does not clear session selection

**Lines:** Session data refreshes via useEffect (lines 217-247) on dateFrom/dateTo change. But `selectedSessionIds` and `coverSessionIds` are NOT reset.

**Problem:** User selects 4 sessions in step 2. Goes back to step 1, changes date range. The new sessions load on return to step 2, but the old `selectedSessionIds` remain. The user sees all checkboxes unchecked (no sessions match old IDs) but the counter in the step header still shows old counts. This creates silent data corruption.

**Fix:** Reset `selectedSessionIds` and `coverSessionIds` when dateFrom or dateTo changes (in the same effect that fetches new sessions).

#### 2.7 No visual feedback for max date range on input

**Lines:** validateStepOne() lines 598-601 checks `daysBetween > config.form.max_date_range_days` but only on "Continue" click. The DateRangeInput component (DateRangeInput.tsx lines 41-47) has a local validation effect but the error it produces is disconnected from the form's `pageError`.

**Problem:** The DateRangeInput component independently calculates `localError` (lines 42-47) but never actually receives `error` prop from the parent. The parent shows errors in a banner at the top of the form (AbsenceForm.tsx line 828). The DateRangeInput's inline error (line 104-108) is never shown because `error` prop is never passed.

**Fix:** Pass `pageError` down as the `error` prop to DateRangeInput. Or remove the unused local validation.

### Step 2: Sessions & Cover (lines 1121-1255)

#### 2.8 "Needs cover" checkbox disabled state is confusing

**Lines:** 1210-1219
```tsx
<label className="flex items-center gap-2 text-sm text-gray-700">
  <input
    type="checkbox"
    checked={covered}
    disabled={!selected}
    onChange={() => handleCoverToggle(session.id)}
  />
  Needs cover
</label>
```

**Problem:** The "Needs cover" checkbox is disabled when the session is not selected. The disabled checkbox has `cursor-not-allowed` but no explanatory text. A user clicking it gets no feedback. The mental model: "I need to tick the session first, then tick cover" is not obvious.

**Why this fails:** Users naturally tick the cover checkbox first (it's the explicit action they want: "I need cover for this class"). They hit a dead end with no explanation.

**Fix:** When `!selected`, show the "Needs cover" label as gray text without a checkbox, or show a tooltip: "Select the session first, then mark it for cover." Or use a three-state toggle: unchecked → selected → selected+cover.

#### 2.9 Sit-in method semantics are confusing

**Lines:** 1134-1142, confirmation text

**Problem:** The distinction between "selected session" (reporting absence) and "cover" (needs substitute) is conveyed only via two separate checkboxes per row. The UI says "Needs cover" but the backend/model calls it "sit-in". The post-submission view says "Sit-in method: zoom" or "Sit-in method: physical" (ConfirmationSummary lines 121-123). The student doesn't know what "sit-in" means.

**Fix:** Use consistent language throughout: "Need a substitute" / "Cover needed". Remove "sit-in" from all visible labels. Pre-submission, show a brief explanation: "If you need someone to cover this session, tick 'Needs cover'."

#### 2.10 Cover toggle hidden when session not selected — no affordance

**Lines:** 537-538
```tsx
function handleCoverToggle(sessionId: string) {
  if (!selectedSessionIds.has(sessionId)) return;
```

**Problem:** The `handleCoverToggle` silently returns if the session isn't selected. The UI shows a disabled checkbox, but the interaction is silently absorbed. No feedback, no explanation.

**Fix:** When clicked while disabled, show a brief inline message: "First tick the session above."

### Step 3: Review & Submit (lines 1258-1323)

#### 2.11 ConfirmationSummary in "review" mode is NEVER shown

**Lines:** 1258-1323

**Problem:** Step 3 renders its own inline summary (student name, date range, reason, session count). The `ConfirmationSummary` component with `mode="review"` is NOT used in step 3. It's only used AFTER submission (lines 774-793) with `mode="result"`. So the `ReviewMode` JSX in ConfirmationSummary.tsx is dead code.

**Fix:** Either use ConfirmationSummary in step 3 (consistent UX) or delete the `mode="review"` path from ConfirmationSummary.

#### 2.12 No "Edit" button to jump back to specific steps

**Line:** Step 3 has no "Edit" links next to each section. The only navigation is a generic "Back" button (line 1310) that goes to step 2.

**Problem:** If the user notices an error in step 3 (wrong dates, wrong courses), they must navigate back one step at a time, change, then navigate forward again. This is tedious for a 4-step form.

**Fix:** Add "Edit" links or a compact step indicator (visible on all steps) that allows jumping to any past step.

#### 2.13 "Report another" / "Done" confusion post-submission

**Lines:** 746-751
```tsx
<Button variant="secondary" onClick={handleReset}>Report another</Button>
<Button variant="secondary" onClick={() => navigate("/")}>Done</Button>
```

**Problem:** Two buttons with different outcomes but same visual style. "Done" navigates to `/` (likely the dashboard). "Report another" resets the form. The user might click "Done" thinking it dismisses the confirmation, or click "Report another" thinking it's the primary action.

**Fix:** Make "Done" the primary button (it ends the flow). Make "Report another" secondary. Or show a single "Return to dashboard" button and hide "Report another" behind a "Submit another absence" link.

---

## 3. Navigation & State Issues

### 3.1 SessionStorage restore is invisible to the user

**Lines:** 284-316

**Problem:** When a user returns to the form (closes tab, reopens), their previous state is restored from `sessionStorage` but:
- No toast, no banner, no visible indication
- The restored step might be 2 or 3, but the OTP token might have expired
- The `verificationSatisfied` state is NOT stored in sessionStorage (lines 270-273 show it's in the dependency array but line 252-263 snapshot omits it)

**Impact:** The user lands on step 2 with selected sessions and dates, but verification might be silently lost. They try to submit at step 3 and it fails because the parent hasn't verified.

**Fix:** On restore, show a banner: "We've restored your progress from your last session." If the OTP token has expired, show prominently: "Your parent verification has expired. Please verify again."

### 3.2 Missing verificationSatisfied in snapshot

**Lines:** 249-282 (snapshot), 270-273 (deps array includes verificationSatisfied but snapshot omits it)

**Bug:** The `verificationSatisfied` value is listed in the effect's dependency array (line 280) but is never included in the snapshot object (lines 252-263). This means each time `verificationSatisfied` changes, the effect runs but saves an identical snapshot—wasteful and incorrect.

**Fix:** Add `verificationSatisfied` to the snapshot object and check it on restore.

### 3.3 `handleLookup` clears verification without warning

**Lines:** 500-503
```tsx
setVerificationSatisfied(false);
verification.clearStoredToken();
verification.setCode("");
```

**Problem:** If the user looks up a DIFFERENT student W-Code after already verifying the first one, the verification is silently cleared. No warning about losing verified status.

**Fix:** Before clearing, check if verification was satisfied and show a confirmation: "Changing the student will reset parent verification. Continue?"

### 3.4 `goTo` has stale closure over `step`

**Lines:** useWizard.ts line 11-15
```tsx
const goTo = useCallback((nextStep: number) => {
  setDirection(nextStep >= step ? "forward" : "back");
  setStep(nextStep);
  setIsTransitioning(true);
}, [step]);
```

**Problem:** `goTo` depends on `step` and is recreated every time step changes. The `back()` and `next()` functions also depend on `goTo` and `step`. This is fine for correctness but means all callbacks are unstable. More critically, the `useEffect` at lines 328-332 has `goTo` as a dependency, and calling `goTo(0)` inside an effect whose dep is `goTo` creates a potential infinite loop if `verificationSatisfied` flips.

**Fix:** Use a ref for step inside goTo, or remove step from dep array and use a ref comparison: `setDirection(nextStep >= stepRef.current ? "forward" : "back")`.

### 3.5 Back button in step 1 goes to step 0 always

**Line:** 1093
```tsx
<Button variant="secondary" onClick={() => back()}>
```

**Problem:** `back()` computes `Math.max(0, step - 1)`, so going back from step 1 always goes to step 0. If verification was already satisfied, step 0 shows the "Verification complete" state, which is fine. But step 0 also shows the W-Code lookup field, which is already populated and completed. The user sees: "You're back at step 0 even though verification is done and the student is found." This creates cognitive dissonance.

**Fix:** Consider making the "Back" from step 1 a "Change W-Code" action, not a full step regression. Or skip to the relevant part of step 0 (just the verification section, not the lookup).

---

## 4. Validation & Error UX

### 4.1 Errors appear at top of form, not inline

**Lines:** 827-831, 833-837, 839-843, 1303-1307

**Problem:** ALL errors (`pageError`, `lookupError`, `sessionsError`, `submissionError`) render as banners at the top of the form. None are placed next to the offending field. When the user clicks "Continue" and the `pageError` says "Select at least one course", they must scroll up to see the error banner, then scroll back down to find the course chips.

**Why this fails:** Error-to-field coupling is the single most important validation UX pattern. The user should see the error next to the action they just took.

**Fix:** Move field-specific errors inline. Only keep `submissionError` and `lookupError` as banners (they're at the step level, not field level). Use `aria-describedby` to associate errors with inputs.

### 4.2 pageError auto-dismiss is too aggressive

**Lines:** 353-357
```tsx
useEffect(() => {
  if (!pageError) return;
  const timer = window.setTimeout(() => setPageError(null), 5000);
  return () => window.clearTimeout(timer);
}, [pageError]);
```

**Problem:** Error messages auto-dismiss after 5 seconds. For a validation error like "Select at least one course", this means:
1. User clicks Continue → error appears at top
2. User finishes selecting a course → error disappears
3. User clicks Continue again → no error, but maybe another field is wrong

But step 2: User clicks Continue → error appears → user reads it → error disappears while they're thinking. Now they click Continue again and hit the SAME error. This feels like the form is "losing" their errors.

**Fix:** Only clear `pageError` on explicit user action (typing, selecting, clicking) or on successful validation. Never auto-dismiss validation errors.

### 4.3 `validateStepOne()` runs after `canProceedToSections` already failed

**Lines:** 1100-1105
```tsx
aria-disabled={!canProceedToSessions}
onClick={() => {
  if (validateStepOne()) {
    goTo(2);
  }
}}
```

**Problem:** The button is visually disabled (`aria-disabled` + `opacity-50`) when `canProceedToSessions` is false. But `validateStepOne()` is still called on click. If the user somehow clicks the disabled button (possible with CSS + pointer-events tricks), they get an error from `validateStepOne()`. But the disabled state already tells them they can't proceed—the error is redundant and confusing.

**Fix:** Use `disabled` prop instead of `aria-disabled` + CSS. This prevents the click handler from firing at all. Move `validateStepOne()` to run on the `canProceedToSessions` check itself so errors show proactively as the user fills fields.

### 4.4 Submission errors are generic

**Lines:** 668-669
```tsx
setSubmissionError(error instanceof Error ? error.message : "Could not submit the absence");
```

**Problem:** No structured error handling for API failures. The error shape defined in CONTEXT.md (stable error with `code` + structured `conflicts`) is never parsed. The user sees "Could not submit the absence" or a raw API error message.

**Fix:** Parse the `ApiRequestError` for `code` and `conflicts`. Render structured conflict details (which sessions conflict, with which student/teacher/room) rather than a generic message.

---

## 5. Keyboard & Accessibility

### 5.1 Course chip grid keyboard navigation has broken semantics

**Lines:** 430-474, 1014-1020

**Problem:** The course chip container is marked as `role="listbox"` with `aria-multiselectable="true"`. Each chip is `role="option"`. The keyboard handler uses `aria-activedescendant` to track focus. But:
1. ArrowRight and ArrowDown do the same thing (next item)
2. ArrowLeft and ArrowUp do the same thing (prev item)
3. In a 2-column grid layout, this means pressing ArrowRight from column 1 goes to column 2 (correct), but pressing ArrowDown goes to the NEXT ROW, which is ALSO column 1 (also correct?). Actually, the grid is `sm:grid-cols-2`. ArrowDown in a grid should go to the next row IN THE SAME COLUMN. But the handler treats Down/Right and Up/Left as equivalent. This is WRONG for a 2-column grid.
4. `Home` and `End` navigate within the subject list—correct.
5. Typeahead buffer resets after 500ms (line 461), which is too fast for multi-key searches.

**Fix:** Implement proper 2D grid navigation. Track column count. ArrowDown goes to `currentIndex + columnCount`, ArrowRight goes to `currentIndex + 1`. Use `role="grid"` instead of `role="listbox"` if doing 2D navigation.

### 5.2 Keyboard hint text is too subtle

**Lines:** 1039-1041
```tsx
<p className="text-xs text-gray-500">
  Use arrow keys to move, Space to toggle a course, and Enter to toggle the focused course.
</p>
```

**Problem:** The keyboard instructions are `text-xs text-gray-500` — small, low-contrast gray text. Sighted keyboard users will never see it. Screen reader users who tab into the `tabIndex={0}` listbox container won't hear it either (it's a `<p>` outside the listbox's `aria-describedby`).

**Fix:** Add the hint as `aria-describedby` on the listbox container. Increase font size to `text-sm` and use `text-gray-700` for better visibility.

### 5.3 Session rows lack keyboard navigation for "Needs cover"

**Lines:** 1210-1219

**Problem:** The "Needs cover" checkbox is a standard `<input type="checkbox">` which is natively keyboard-accessible. However, the entire row (lines 1190-1191) is a `<div>` with no `role`, no keyboard handler, and no focus management. The checkbox is inside a `<label>`, so clicking the label text works. But the row itself doesn't respond to clicks, and there's no keyboard shortcut to focus the cover checkbox.

**Fix:** Add a keyboard handler to the row that focuses the cover checkbox on Tab from the session checkbox. Or remove the wrapping div and use a `<label>` for the entire row.

### 5.4 OTP input has no focus indicator between cells

**Lines:** OtpInput.tsx lines 67-81, 82-97

**Problem:** The 6 visual "cells" are decorated `<div>` elements with `aria-hidden="true"`. The actual input is invisible. When the input is focused, none of the 6 cells show a focus ring. The user can't tell where their cursor is.

**Fix:** Use the CSS `:focus-within` or JavaScript `onFocus`/`onBlur` to add a focus ring to the current cell. Or show a blinking cursor indicator.

### 5.5 CountdownTimer uses `aria-live="off"`

**Lines:** CountdownTimer.tsx line 60
```tsx
role="timer" aria-live="off"
```

**Problem:** The timer display uses `aria-live="off"` which means screen readers won't announce changes. There IS a separate `aria-live="polite"` region for announcements (lines 65-67), but it only announces at specific milestones (60s, 30s, 10s, 5-1s). Between milestones, the timer decrements silently.

**Fix:** Change `aria-live` to `"polite"` on the timer display and reduce announcement frequency. Or keep `aria-live="off"` and announce every 10 seconds via the status region.

---

## 6. Technical Architecture Issues

### 6.1 1333-line monolith

**Problem:** `AbsenceForm.tsx` is 1333 lines of inline markup and logic. Four steps are conditionally rendered within the same component via `step === 0/1/2/3` checks. Each step re-renders all state hooks on every step change.

**Impact:** Impossible to test in isolation. Components like ConfirmationSummary are defined separately but never used in the review step. The file mixes API logic, form state, validation, and rendering.

**Fix:** Extract each step into its own component file (StepLookupVerify, StepCoursesDates, StepSessionsCover, StepReviewSubmit). Pass only required props and callbacks.

### 6.2 `useEffect` chain creates implicit step dependencies

**Lines:** 217-247 (sessions fetch), 328-332 (verification force-back), 334-338 (lookup force-back)

**Problem:** Three `useEffect` hooks create hidden dependencies between pieces of state:
- Changing dates → triggers session fetch (correct)
- Changing verificationSatisfied → can force step to 0 (jarring)
- Changing lookup → can force step to 0 (correct for guard, but jarring on restore)

These effects interact: restoring state from sessionStorage sets lookup, which triggers line 334-338, which forces step 0, which then clears verificationSatisfied (since restore doesn't include it), which triggers line 328-332 again. The user ends up at step 0 with all data restored but verification lost, no explanation.

**Fix:** Guard the force-back effects with an "initial restore complete" flag. Don't force navigation during state restoration. Show blocking banners instead.

### 6.3 `buildSubmissionPayload` only handles single course

**Lines:** 625-640

```tsx
function buildSubmissionPayload() {
  if (!lookup || !activeGroup || !primarySessionGroup) return null;
  return {
    wcode: lookup.wcode,
    subject_id: activeGroup.id,
    course_id: primarySessionGroup.course_id,
    ...
  };
}
```

**Problem:** Multi-course selection is supported in the UI but only one course gets submitted. This is a data loss bug waiting to happen. The user selects Chemistry + Biology → only Biology gets submitted.

**Fix:** Either remove multi-select from step 1 (single course only) or support multi-subject submission (POST per subject or batch endpoint).

### 6.4 Idempotency key is generated once per form lifecycle

**Line:** 122
```tsx
const submissionIdempotencyKey = useRef(newIdempotencyKey());
```

**Problem:** The idempotency key is generated on mount. If the user fails to submit (network error), retries, and the first request actually succeeded server-side, the retry with the same key will correctly be a no-op (safe). But if the user resets the form (line 708, `submissionIdempotencyKey.current = newIdempotencyKey()`) and starts a NEW absence, the original submission's key is lost. This is correct behavior.

However, if the user navigates away and comes back (sessionStorage restore), the idempotency key is ALSO regenerated (useRef initializer runs on mount). This means a partial submission from a previous session can't be retried with safety.

**Fix:** Store the idempotency key in sessionStorage alongside other form state. Restore it on mount.

### 6.5 `beforeunload` event fires regardless of save status

**Lines:** 359-367

**Problem:** The `beforeunload` handler fires when `lookup` is truthy and `finalResult` is null. But the form auto-saves to sessionStorage on every state change (lines 249-282). The user gets a "Leave site? Your changes will be lost" warning even though their data IS saved to sessionStorage. This is a false alarm.

**Fix:** Check if the sessionStorage snapshot matches current state. If it does (meaning data is saved), skip the `beforeunload` warning.

### 6.6 `canVerify` is stale during verification

**Line:** StepCoverVerification.tsx line 207
```tsx
const canVerify = !isSending && !isVerifying && verification.code.length === 6 && !!verification.token && !verified;
```

**Problem:** `canVerify` depends on `verification.code.length === 6`. But the auto-verify effect (lines 122-138) fires when `verification.code` changes. If auto-verify fires and then fails (e.g., network error), the code is preserved (only non-retryable errors clear it, lines 187-189). But `canVerify` is now true again (code is still 6 digits, `isVerifying` is false), so the button is enabled. The user clicks "Verify code" manually, which re-sends the same code. But `autoVerifyCodeRef.current` is still set (line 135), so the effect won't re-fire automatically. This creates confusion: sometimes auto works, sometimes manual works.

**Fix:** Remove auto-verify entirely. Always require manual "Verify code" button press.

---

## 7. Top 5 Most Impactful Interaction Fixes

### Fix 1: Reorder steps — move verification AFTER course/date selection

| Aspect | Detail |
|--------|--------|
| **What** | Move parent OTP verification from step 0 to step 2 (after course/date selection) |
| **Why** | Parent receives a text with specifics ("Biology, Mon-Wed") not a vague request. Student builds context before needing parent interaction. Reduces abandonment. |
| **Lines** | AbsenceForm.tsx lines 328-332, 886-986; whole step 0 restructure |
| **Effort** | Medium — component extraction + flow refactor |
| **Impact** | **Highest.** This is the #1 complaint driver. Front-loading the hardest step kills engagement. |

### Fix 2: Kill the force-back ejection; use blocking banners instead

| Aspect | Detail |
|--------|--------|
| **What** | Replace `useEffect` at lines 328-332 (force `goTo(0)`) with an inline blocking banner that prevents progression but preserves context |
| **Why** | Being forcefully moved from step 2 to step 0 is disorienting and loses mental context. A banner saying "Verification expired — verify again below" keeps the user in place. |
| **Lines** | 328-332 |
| **Effort** | Low — replace `goTo(0)` with `setPageError("…")` + disable Continue buttons |
| **Impact** | **Very high.** Eliminates the most jarring navigation experience. |

### Fix 3: Move validation errors inline, kill auto-dismiss

| Aspect | Detail |
|--------|--------|
| **What** | Attach each validation error to the specific field/course/session. Remove `setTimeout` auto-dismiss for `pageError`. |
| **Why** | "Select at least one course" at the top of the form is disconnected from the course chips. 5-second auto-dismiss means the error vanishes while the user is reading it. |
| **Lines** | 353-357, 585-603, 827-831 |
| **Effort** | Medium — refactor validation to return field-specific errors, render inline with `aria-describedby` |
| **Impact** | **High.** This is the #2 complaint driver. Errors that are invisible or disconnected make the form feel broken. |

### Fix 4: Fix multi-course handling — either support it or remove it

| Aspect | Detail |
|--------|--------|
| **What** | Either submit ALL selected courses (iterate in submission) or remove multi-select and make step 1 single-course-only |
| **Why** | Currently, selecting multiple courses in step 1 silently ignores all but the first. This is data loss. |
| **Lines** | 625-640, 717-721 |
| **Effort** | Medium for remove, High for support |
| **Impact** | **High.** Silently discarding user selections is the #3 trust-killer. |

### Fix 5: Sessions step — clear selections when dates change, improve cover affordance

| Aspect | Detail |
|--------|--------|
| **What** | (a) Reset `selectedSessionIds` when date range changes. (b) Make "Needs cover" show an explanation when disabled. (c) Remove auto-verify on 6th digit. |
| **Why** | Stale session selections cause data inconsistency. The disabled cover checkbox has no feedback. Auto-verify removes user agency. |
| **Lines** | 537-538, 1210-1219, StepCoverVerification.tsx 122-138 |
| **Effort** | Low |
| **Impact** | **High.** These three micro-interactions produce the most daily confusion. |

---

## 8. Full Issue Register

| # | Issue | Type | Severity | File:Lines |
|---|-------|------|----------|------------|
| 1 | Verification before selection (wrong step ordering) | Flow | Blocker | AbsenceForm.tsx:886-986 |
| 2 | Force-back to step 0 on verification loss | Flow | Blocker | AbsenceForm.tsx:328-332 |
| 3 | Multi-course selection silently ignored on submit | Data | Blocker | AbsenceForm.tsx:625-640 |
| 4 | pageError auto-dismiss after 5s | Validation | Major | AbsenceForm.tsx:353-357 |
| 5 | All errors at top, not inline | Validation | Major | AbsenceForm.tsx:827-831 |
| 6 | Changing dates doesn't clear session selection | Data | Major | AbsenceForm.tsx:247 |
| 7 | Cover checkbox disabled without explanation | Interaction | Major | AbsenceForm.tsx:1210-1219 |
| 8 | OTP auto-verify removes user agency | Interaction | Major | StepCoverVerification.tsx:122-138 |
| 9 | SessionStorage restore is invisible | UX | Major | AbsenceForm.tsx:284-316 |
| 10 | verificationSatisfied omitted from sessionStorage snapshot | Bug | Major | AbsenceForm.tsx:249-282 |
| 11 | ConfirmationSummary "review" mode is dead code | Maintenance | Medium | ConfirmationSummary.tsx:69-142 |
| 12 | DateRangeInput local validation error prop never passed | Bug | Medium | DateRangeInput.tsx:41-47 |
| 13 | Course chip keyboard nav treats 2-col grid as 1-col list | A11y | Medium | AbsenceForm.tsx:430-474 |
| 14 | Keyboard hint text too small + not associated with listbox | A11y | Medium | AbsenceForm.tsx:1039-1041 |
| 15 | OTP input invisible hit target + no focus indicator | A11y | Medium | OtpInput.tsx:82-97 |
| 16 | beforeunload fires even with sessionStorage saved | UX | Medium | AbsenceForm.tsx:359-367 |
| 17 | Idempotency key not persisted in sessionStorage | Reliability | Medium | AbsenceForm.tsx:122 |
| 18 | "Done" vs "Report another" same visual weight | UX | Medium | AbsenceForm.tsx:746-751 |
| 19 | Step 3 lacks edit links to specific steps | UX | Medium | AbsenceForm.tsx:1258-1323 |
| 20 | CountdownTimer aria-live="off" for display | A11y | Low | CountdownTimer.tsx:60 |
| 21 | "Sit-in" terminology not user-friendly | Copy | Low | Throughout |
| 22 | Reason textarea missing character counter | UX | Low | AbsenceForm.tsx:1083-1089 |
| 23 | 1333-line monolith component | Architecture | Low | AbsenceForm.tsx  |
| 24 | useWizard goTo has stale closure risk | Architecture | Low | useWizard.ts:11-15 |
| 25 | No loading state during step transition | UX | Low | AbsenceForm.tsx:879-884 |
| 26 | No structured submission error rendering | Validation | Medium | AbsenceForm.tsx:668-669 |
| 27 | Auto-verify effect race with manual verify | Interaction | Medium | StepCoverVerification.tsx:132-136 |
| 28 | validateStepOne/Two run on disabled button clicks | Validation | Low | AbsenceForm.tsx:1100-1105 |

---

## Files Audited

- `src/pages/AbsenceForm.tsx` — 1333-line main form wizard
- `src/hooks/useWizard.ts` — Step navigation state
- `src/hooks/useOtp.ts` — OTP state + localStorage persistence
- `src/hooks/useConnectivity.ts` — Online/offline detection
- `src/components/absences/StepCoverVerification.tsx` — OTP send/verify/skip
- `src/components/absences/ConfirmationSummary.tsx` — Review/result summary
- `src/components/absences/CourseChip.tsx` — Selectable course pill
- `src/components/absences/SessionGrid.tsx` — Session list with master toggle
- `src/components/absences/SessionChip.tsx` — Individual session pill
- `src/components/absences/OtpInput.tsx` — 6-digit OTP cell input
- `src/components/absences/CountdownTimer.tsx` — Resend cooldown display
- `src/components/absences/DateRangeInput.tsx` — Date range with presets
- `src/components/absences/SitInResultCard.tsx` — Sit-in plan display
- `src/components/absences/KanbanView.tsx` — Admin absence board (not audited in depth)
- `src/components/absences/AbsenceFormEditor.tsx` — Admin settings form (not audited in depth)
