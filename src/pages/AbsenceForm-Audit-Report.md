# Absence Form — UX Audit Report

**Audited:** 30 May 2026  
**Scope:** 4-step absence wizard flow (1382-line page + 9 supporting components)  
**Files Reviewed:** 10  

---

## Summary of Findings

| Severity | Count |
|----------|-------|
| HIGH     | 6     |
| MEDIUM   | 9     |
| LOW      | 3     |
| **Total** | **18** |

---

## HIGH Severity Findings

### H1 — `text-gray-400` on step labels fails WCAG AA contrast

**Location:** `AbsenceForm.tsx` line 923  
**Issue:** Unvisited step labels use `text-gray-400` on white background. At `text-sm` (~14px), this yields ~2.7:1 contrast ratio — fails WCAG AA minimum of 4.5:1.  
**Impact:** Users with low vision cannot read which steps remain. Orientation cues are invisible.  
**Fix:** Change to `text-gray-500` minimum for unvisited step labels. `text-gray-500` on white yields ~4.1:1 which is still borderline — `text-gray-600` (~5.5:1) is safer.

### H2 — `text-xs text-gray-500` fails WCAG AA for small text throughout

**Locations:**  
- `AbsenceForm.tsx` lines 781, 1100, 1190, 1217, 1254, 1319, 1323  
- `StepCoverVerification.tsx` lines 247, 350  
- `OtpInput.tsx` line 100  
- `ConfirmationSummary.tsx` lines 113, 136, 198  
- `DateRangeInput.tsx` line 82, 92  

**Issue:** `text-xs` (12px) with `text-gray-500` yields ~4.1:1 contrast ratio. WCAG AA for small text (<18px) requires 4.5:1. This pattern appears ~20+ times across the form for captions, helper text, labels, and section headers.  
**Impact:** Systemically, helper text and captions are hard to read for users with visual impairments. This is the most pervasive accessibility failure in the form.  
**Fix:** Replace all `text-gray-500` on small text with `text-gray-600`. Or increase caption text to `text-sm` where possible.

### H3 — CourseChip contradictory ARIA roles (`role="option"` + `aria-pressed`)

**Location:** `CourseChip.tsx` lines 30–32  
**Issue:** The chip declares `role="option"` (listbox option semantics) AND `aria-pressed` (toggle button semantics). These are mutually exclusive ARIA roles. `aria-selected` is the correct attribute for `role="option"`, but `aria-pressed` overrides its semantics. Screen readers get conflicting signals — NVDA reads it as a toggle button, VoiceOver as a listbox option.  
**Impact:** Screen reader users receive inconsistent or incorrect state announcements for course selection.  
**Fix:** Remove `aria-pressed={selected}`. Keep `role="option"` and `aria-selected`. The `role="listbox"` + `role="option"` pattern with `aria-multiselectable="true"` is already set up correctly on the parent container (line 1077).

### H4 — Error banner stacking creates cognitive wall

**Location:** `AbsenceForm.tsx` lines 843–908  
**Issue:** The form renders up to 5 distinct error/status banners stacked sequentially before any form content:
1. `pageError` (red, dismissible)
2. `verificationBlocked` (amber, with action button)
3. `lookupError` (red, animated)
4. `sessionsError` (red, animated)
5. `parentPhoneMissing` (amber)
6. `renderStatusBanner` (offline/restored, green/amber)

Each uses different visual treatment (static vs animated, dismissible vs persistent). No priority ordering logic exists — all render simultaneously if their conditions are met. On mobile (375px), this pushes the first form field 300–500px below the top of the viewport.  
**Impact:** Users see a wall of colored boxes before any actionable form content. This dramatically increases cognitive load, especially for neurodivergent users.  
**Fix:** 
- Consolidate into a single error summary region (like `<FormErrorSummary>`) near the top
- Show only the **most severe** error at a time, ordered by severity: submission errors > verification errors > lookup errors > informational banners
- Reserve `role="alert"` for the single most important error, use `role="status"` for lower-priority messages

### H5 — Step 1 conflates two distinct operations (lookup + verification) in one view

**Location:** `AbsenceForm.tsx` lines 947–1046  
**Issue:** After student lookup succeeds, the verification section (OTP send, OTP input, countdown, verify button, skip button, admin contact info) appends below the student info card, all within one `<section>`. The "Continue to courses" CTA is hidden inside a plain `border border-gray-200 bg-white` card that visually merges with surrounding content until verification completes.  
**Impact:** Users experience a **wall of nested cards**: outer card → student info → verification card → OTP fields → countdown → verify button. The "what to do next" is unclear until the user reads down through all elements. The verification step adds significant complexity because it involves:
- Sending an SMS to a parent
- Waiting for the code
- Entering a 6-digit code
- Optionally skipping

This is essentially two steps compressed into one.  
**Fix:** Either:
- **Option A:** Split into 3 steps: Step 1 (Lookup), Step 2 (Verify), Step 3 (Courses & Dates). This is cleaner but changes the 4-step structure.
- **Option B:** Give verification its own distinct visual container with a clear progress indicator. Add a persistent "Continue" navigation footer that's visible once verification passes, not hidden inside a card. Use a visual separator line between lookup info and verification section.

### H6 — SessionGrid uses `rounded-lg` while all other cards use `rounded-sm`

**Location:** `SessionGrid.tsx` lines 65, 113  
**Issue:** The session grid component uses `rounded-lg` for its outer container and subject group containers. Every other card, section, and container in the absence form uses `rounded-sm`. This creates one visual outlier where corner radii are twice as large as the rest of the form.  
**Impact:** Visual inconsistency signals to users that this section works differently — but it doesn't. The inconsistency erodes trust in the UI's coherence.  
**Fix:** Change `rounded-lg` to `rounded-sm` in `SessionGrid.tsx` lines 65 and 113. Optionally also change the `p-6` on line 65 to `p-5` to match the form's `p-5` card standard.

---

## MEDIUM Severity Findings

### M1 — Step 2 overflows with four simultaneous sections

**Location:** `AbsenceForm.tsx` lines 1058–1164  
**Issue:** Step 2 (Courses & Dates) presents four distinct information sections simultaneously with no progressive disclosure:
1. Course selection grid (2-column grid of chips)
2. Date range input (2-column grid + 2 quick-preset buttons)
3. Reason category dropdown
4. Free-text reason textarea (with character counter)

None are collapsible. On a 14-day range with 6+ courses, the user must scroll through all four sections.  
**Impact:** Users must process course selection, date parameters, AND reason all at once. The reason fields (especially) are often secondary — the user's primary task at this step is selecting what courses and when.  
**Fix:** Collapse reason category + free text behind an "Add reason" expandable section. Hide free text behind reason category selection. The primary task (courses + dates) should occupy 80% of the viewport.

### M2 — Session row dual-checkbox horizontal scan distance

**Location:** `AbsenceForm.tsx` lines 1239–1270  
**Issue:** Each session row presents two checkboxes:
- Session checkbox (left-aligned, next to date/time label)
- Cover checkbox (right-aligned, with "Needs cover" label)

On desktop (~1024px container width), the horizontal distance between these checkboxes is ~400–500px. The user must scan across the full width to understand the row. The cover checkbox is disabled until the session checkbox is checked, creating a dependency that's invisible until interaction.  
**Impact:** High scanning cost per row. When a course has 10+ sessions across a range, the cumulative scan distance becomes fatiguing.  
**Fix:** Restructure each session row to stack controls vertically on mobile, OR place the cover checkbox directly next to the session checkbox (left side), OR use a clickable pill/button for "Cover this session" that appears after the session is selected.

### M3 — Date range preset buttons don't use Button component

**Location:** `DateRangeInput.tsx` lines 80–101  
**Issue:** "This week" and "Next 3 days" are raw `<button>` elements hand-styled with:
```
rounded-sm border border-gray-300 px-3 py-2 text-sm hover:bg-gray-50
```
They lack focus-visible ring styling, disabled states, and the consistent hover/focus patterns that `<Button variant="secondary" size="sm">` provides.  
**Impact:** Keyboard users get inconsistent focus indicators. Visual inconsistency with the rest of the form's button language.  
**Fix:** Replace with `<Button variant="secondary" size="sm">`.

### M4 — Course "Select all" / "Deselect all" is a raw text link, inconsistent with session select-all buttons

**Location:** `AbsenceForm.tsx` lines 1066–1072  
**Issue:** The course select-all uses a raw `<button>` styled as:
```
text-sm font-medium text-[var(--color-wi-primary)] hover:underline
```
But the session-grid select-all buttons on the same form use `<Button variant="secondary" size="sm">`. Two different visual patterns for the same interaction (select all / deselect all).  
**Impact:** Users learn one visual language for "select all" on step 2, then encounter different language on step 3. Cognitive overhead.  
**Fix:** Use `<Button variant="secondary" size="sm">` for the course select-all/deselect-all, matching the session grid pattern.

### M5 — Card padding inconsistency: `p-5` vs `p-4`

**Locations:**  
- Main step sections: `p-5` (`AbsenceForm.tsx` lines 948, 1050, 1175)  
- ConfirmationSummary: `p-4` (`ConfirmationSummary.tsx` lines 221, 231)  
- StepCoverVerification: `p-5` (line 212)  
- Nav footer strips: `p-4` (`AbsenceForm.tsx` lines 1278, 1330, 1345)  

**Issue:** No single padding standard across cards. Main sections use `p-5`, sub-sections and confirmation use `p-4`.  
**Impact:** Layout feels subtly uneven. Nested cards (p-5 in p-4 or vice versa) create asymmetric whitespace.  
**Fix:** Standardize on `p-5` for all top-level cards. Use `p-4` only for deeply nested secondary cards.

### M6 — Three different date formatting functions scattered across files

**Locations:**
- `AbsenceForm.tsx` lines 65–81: `formatDate()` (format: "9 Feb 2026", no weekday) and `formatDateTime()` (format: "Sat, 9 Feb, 14:30")  
- `SessionChip.tsx` lines 16–22: `formatDisplayDate()` (format: "Sat 9 Feb", no year)  
- `ConfirmationSummary.tsx` lines 37–44: `fmtDate()` (format: "Sat 9 Feb 2026", matches formatDate but duplicated)  

**Issue:** Four date formatting implementations across three files, producing three visual formats (with year, without year, with time). No shared utility.  
**Impact:** Dates appear differently in different parts of the form. The user sees one format in step 2 summary, another in step 3 session rows, another in step 4 confirmation. Subtly disorienting.  
**Fix:** Extract shared date formatting utilities into a `src/utils/date.ts` module. Use consistent format across all form views.

### M7 — Verification success CTA lacks visual emphasis when it appears

**Location:** `AbsenceForm.tsx` lines 1028–1041  
**Issue:** When parent verification succeeds, a "Continue to courses" button appears inside a plain white card (`border border-gray-200 bg-white p-4 text-sm`). No animation, no highlight, no color change — it simply appears. The user may not notice the new CTA if they're still reading the verification section.  
**Impact:** New action opportunity is visually invisible. Users may sit confused about what to do next.  
**Fix:** Use `motion.div` with a fade-in + slight lift animation when `verificationSatisfied` becomes true. Or change the card background to a subtle green tint.

### M8 — "No cover needed" badge is ambiguous

**Location:** `AbsenceForm.tsx` lines 1229–1231  
**Issue:** When `coveredCount === 0`, the form shows `<span className="rounded-full bg-gray-100 px-3 py-1 text-xs text-gray-600">No cover needed</span>`. It's unclear whether this means:
- Cover is optional (no requirement)
- Cover will be auto-resolved
- Cover is not applicable for this absence type

**Impact:** Users may be uncertain whether they need to take action on cover or not.  
**Fix:** Change text to "Cover optional" with a small info icon + tooltip explaining the policy (e.g., "Sit-in cover can be arranged by the school office, or you can request it here").

### M9 — Step transition animations have asymmetric exit offset

**Location:** `AbsenceForm.tsx` lines 815–820  
**Issue:** Forward direction: entry slides in from x=20, but exit slides out to x=-12. Backward: entry from x=-20, exit to x=12. The exit offset (12px) is 40% smaller than the entry offset (20px). The animation is perceptibly lopsided.  
**Impact:** Step transitions feel slightly off-balance. Users sensitive to motion may find this subtly disorienting.  
**Fix:** Make exit offset magnitude match entry offset (both 20px or both 12px).

---

## LOW Severity Findings

### L1 — LoadingSkeleton uses `Math.random()` for width

**Location:** `LoadingSkeleton.tsx` line 16  
**Issue:** `style={{ width: \`${60 + Math.random() * 40}%\` }}` — the random width changes on every render. In an SSR context this causes hydration mismatch. In client-only it means the skeleton shape is non-deterministic.  
**Fix:** Use a deterministic alternating pattern (e.g., cycle through `["60%", "75%", "50%", "80%"]`).

### L2 — Reason character counter threshold at 450 is invisible

**Location:** `AbsenceForm.tsx` line 1132  
**Issue:** The character counter turns amber at >450 characters and red at 500. The 450 threshold has no visual indicator — no progress bar, no border change on the textarea. The user gets no warning until they cross the threshold, at which point only the tiny counter text changes color.  
**Fix:** Add a thin progress bar under the textarea that fills proportionally, turning amber at 80% and red at 95%.

### L3 — Verification "skip" flow lands on "Continue to courses" with no state indicator

**Location:** `StepCoverVerification.tsx` lines 196–204, handled via `handleSkip()` → `onSatisfied()`  
**Issue:** When verification is skipped (via "Continue without verification"), the parent verification section transitions to the "Verification complete" state (line 272) with no visual distinction between "verified via OTP" and "verification skipped." Both show the same green banner.  
**Impact:** Admin reviewing the submission later cannot tell from the student's form state whether verification was completed or skipped.  
**Fix:** Show different messaging: "Verification was skipped. Parent was not contacted." vs "Parent verified via SMS code."

---

## Consistency Register

| Pattern | Step 1 | Step 2 | Step 3 | Step 4 | Verdict |
|---------|--------|--------|--------|--------|---------|
| Card border radius | `rounded-sm` | `rounded-sm` | `rounded-sm` | `rounded-sm` | ✅ Consistent |
| Card padding | `p-5` | `p-5` | `p-5` | `p-4` (summary) | ❌ `p-4` outlier |
| Button component usage | ✅ | ❌ select-all raw | ✅ | ✅ | ❌ Select-all raw |
| Date format | `formatDate()` + `formatDateTime()` | — | `formatDisplayDate()` | `fmtDate()` | ❌ 4 functions |
| Error animation | Static + AnimatePresence | — | — | — | ❌ Mixed pattern |
| Skeleton type | `card` lines=3 | — | `table` lines=5 | — | ⚠️ Context-appropriate |
| Chip border radius | `rounded-sm` (CourseChip) | — | `rounded-full` (SessionChip) | — | ⚠️ Intentional difference |

---

## Interaction Design Notes

**Positive — keep these:**
- Keyboard navigation on course chips (arrow keys, Home/End, typeahead) is excellent
- OTP auto-submit at 6 digits reduces friction
- `useReducedMotion` respected throughout
- Focus management moves to step heading on navigation
- `beforeunload` protection for in-progress forms
- Session storage persistence with restore on return

**Needs improvement:**
- No loading skeleton during form submission (step 3 → submit)
- Cover checkbox disabled state has no tooltip explaining why
- "What happens next?" panel on step 4 is static text — could be more reassuring with timeline illustration
- No unsaved-changes indicator in the step tracker itself

---

## Accessibility Snapshot

| Criterion | Status | Notes |
|-----------|--------|-------|
| Touch targets ≥ 44px | ✅ | All interactive elements meet this |
| Focus management across steps | ✅ | Headings receive focus |
| Focus-visible ring styles | ✅ | Buttons, chips have focus rings |
| Color contrast (normal text) | ⚠️ | Gray-400 on white fails (H1) |
| Color contrast (small text) | ❌ | Gray-500 at text-xs fails (H2) |
| ARIA attribute consistency | ❌ | CourseChip conflict (H3) |
| `role="alert"` usage | ⚠️ | 3+ simultaneous `role="alert"` elements |
| Screen reader live regions | ✅ | Course announcements, session count |
| Reduced motion support | ✅ | `useReducedMotion()` on all animations |

---

## Priority Remediation Order

1. **H3** — Fix ARIA conflict on CourseChip (5-minute code fix, eliminates screen reader confusion)
2. **H1 + H2** — Fix gray-400/500 contrast across all components (systematic find-and-replace, affects every page)
3. **H4** — Consolidate error banners (architectural change, highest UX impact)
4. **H5** — Restructure step 1 to separate lookup from verification (significant structural change)
5. **H6** — Fix SessionGrid radius inconsistency (2-line CSS fix)
6. **M1–M4** — Address information density and component consistency issues
