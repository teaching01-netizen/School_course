# Student Absence Mobile Craft — Implementation and Verification

- **Date:** 16 October 2026
- **Plan:** [Mobile UX/UI and Apple HIG craft implementation plan](2026-10-16-student-absence-mobile-apple-hig-craft.md)
- **Result:** Public-form implementation delivered; automated verification completed. Physical-device and VoiceOver acceptance remains outstanding.

## Implemented experience

The public flow remains **Student → Verify → Classes → Review → receipt**. Search, conditional email entry, explicit SMS sending, fresh-verification auto-advance, restored-session Continue, and submission boundaries retain their existing behavior.

### Visual foundation and navigation

- Public-mode styles preserve Warwick branding while introducing readable type, quieter surfaces, stronger secondary-text/status contrast, and consistent alignment.
- Completed-step targets are at least 44px; the first primary action uses full width and later actions share space with Back.
- The keyboard-open header emphasizes the current step instead of branding.
- Gutters and footer spacing stay usable at 200% text size. Minimum button width prevents labels degenerating into single-letter columns.
- Step changes reset the actual main scroller. Edit reason focuses the textarea; Edit classes expands and focuses a selected course.

### Classes

- Course selection stays first. Each selected public course has one responsive heading, a phone-only disclosure control, and its existing grouped dates.
- Dates and times have separate lines; course and teacher names wrap.
- Phone date rows and their make-up content use fewer nested surfaces.
- Course-local remaining allowance retains existing calculations. Selecting and collapsing a course remain independent operations.
- Missing make-up validation focuses the owning picker and associates a field-level error with its visible control.

### Make-up selection

- The public sheet opens with heading focus, preserving a browsing-first experience.
- It has missed-class context, date/time-first options, explicit Cancel, search recovery, a selected-choice summary, and confirmation outside the option scroller.
- Selection remains staged until confirmation. Cancel/Escape/backdrop do not commit it; changed/disabled options cannot be confirmed.
- Existing structured display data supplies presentation without parsing display strings or changing option values.
- The native dialog follows visual viewport height/offset changes. Closing restores focus; widening into the select layout closes the mobile modal and focuses the visible select.
- Closed sheets do not subscribe to viewport events.

### Other steps and receipt

- W-Code and email entry have appropriate keyboard/autofill attributes and clearer prerequisite guidance.
- The public resend timer uses minutes/seconds without changing its cooldown. SMS button transitions respect reduced motion.
- Reason remains required and capped at 500 characters; the character-progress bar is removed in public mode. A full valid reason is not styled as invalid.
- Review pairs each missing date with its existing make-up description. Receipt summaries stack on phones and retain the returned reference, grouping, and pending-review meaning.
- Staff workflows retain their existing APIs, step structure, and validation. Shared components keep default behavior unless the public presentation is requested.

## Verification results

| Check | Result |
| --- | --- |
| `npm run typecheck` | Passed on final code |
| `npm run typecheck:e2e` | Passed on final tests |
| `npm run build` | Passed on final code |
| TypeScript AST syntax and shared component-call mapping | Zero syntax diagnostics; public/staff callers inspected |
| Absence unit/integration suite plus public-form and viewport tests | 520 passed, 4 failed out of 524; all four failures also occurred in the pre-edit baseline |
| Full phone browser regression matrix | 66 passed: 22 scenarios each on 320px Chromium, 390px Chromium, and iPhone-sized WebKit |
| Additional complete-flow accessibility/viewport matrix | 10 passed across compact/large phones, landscape, tablets, desktop, and Firefox |
| Final layout/reduced-motion recheck | 15 mobile craft/accessibility cases passed, plus the 10 additional viewport cases passed again |
| Targeted verification/SMS recheck after reduced-motion refinement | 20 passed |
| Scoped implementation diff whitespace check | Passed |

The full absence command used was:

```bash
npm run test:absence -- src/components/absences/public-form/__tests__ src/hooks/useVisualViewport.test.tsx --reporter=dot
```

The full phone browser run used:

```bash
npx playwright test e2e/absence-form-mobile-craft.spec.ts e2e/absence-form-accessibility.spec.ts e2e/absence-form.spec.ts e2e/absence-form-critical-path.spec.ts e2e/absence-form-error-recovery.spec.ts e2e/absence-form-resilience.spec.ts e2e/absence-timezone.spec.ts --project=chromium-phone-small --project=chromium-phone-standard --project=webkit-phone --workers=2
```

After final layout and reduced-motion refinements, the directly affected mobile scenarios were rerun with:

```bash
npx playwright test e2e/absence-form-mobile-craft.spec.ts e2e/absence-form-accessibility.spec.ts --project=chromium-phone-small --project=chromium-phone-standard --project=webkit-phone --workers=2
```

Additional viewport coverage used:

```bash
npx playwright test e2e/absence-form-accessibility.spec.ts --project=chromium-phone-compact --project=chromium-phone-large --project=chromium-phone-landscape --project=chromium-tablet-portrait --project=chromium-tablet-landscape --project=chromium-desktop --project=webkit-tablet-portrait --project=webkit-tablet-landscape --project=webkit-tablet-large --project=firefox-phone --workers=2
```

### Exercised behavior

- Successful physical make-up submission, manual email entry, invalid W-Code, correction from Review, quota rejection, invalid saved verification, uncertain/duplicate submission paths covered by the existing suites, and Bangkok-timezone consistency.
- Dialog modality, keyboard traversal without focusing background application controls, cancellation, focus restoration, wide-layout transition, clear selection, search-hidden staged selection, and pending-option invalidation.
- Multiple courses, long names, 24 make-up options, keyboard-style visual viewport resize/offset, 200% root text size, reduced motion, and emulated forced colors.
- Focused textarea visibility and nonoverlapping/reachable action bar at **320px, 200% text, and a 420px simulated visual viewport**. A visual review found a collapsed-main layout that an overflow-only assertion missed; this scenario now has explicit focus-visibility assertions.
- axe serious/critical checks across the complete form and receipt in the viewport matrix. Selected-option text and success/warning text received contrast corrections during implementation.

## Four baseline unit failures

Before application edits, 514 tests passed and these four failed out of 518. After six new interaction tests and markup-aware assertion updates, the same four fail while 520 pass. They were not suppressed or skipped.

| File | Existing test | Observed failure |
| --- | --- | --- |
| `src/pages/__tests__/AbsenceForm.sessionLimit.test.tsx` | `blocks selecting more than 20% of sessions in one request (10-session course)` | The no-alert assertion matches an empty global toast live region with `role="alert"` |
| Same file | `blocks selecting more than 20% of sessions in one request (20-session course)` | The same empty toast region is matched |
| `src/pages/__tests__/AbsenceForm.test.tsx` | `shows a used sit-in session as unavailable in the public form` | Expected `You've already used this make-up class.` is absent |
| Same file | `shows sit-in target course name (not student's enrolled course) in make-up dropdown when sit_in_course has name populated` | Expected `Mathematics`; received `Not yet selectedscholar — Thu, 4 Jun · 15:00–17:00` |

The current worktree already contained scheduling/domain edits before this work. This implementation preserves that baseline rather than altering scheduling/display policy to turn those assertions green.

## Browser-fixture repairs

Existing browser fixtures needed three corrections before they could exercise the current form:

1. Verification timestamps had expired; fixture capability expiry is now relative to the test run.
2. The supposed valid make-up overlapped the missed class; it now occurs on the following day while retaining a Bangkok/UTC date-boundary case.
3. Course selectors used an obsolete pre-grouping DOM ID, and some radio helpers chose the empty option. Selectors now use the current grouped ID/accessibility semantics, and helpers explicitly choose an enabled nonempty option.

Timezone tests retain actual rendered date/time and submitted-ID assertions. Updated unit assertions check meaningful headings, checkbox names, grouped-day counts, and option descriptions instead of requiring the former concatenated text markup.

## Local visual evidence

Screenshots were captured and inspected. These are local ignored artifacts, not committed files:

- [Before: basic four-step flow and receipt at 320px/390px](../../../artifacts/absence-mobile-before/)
- [After: steps, sheet, receipt, larger text, and forced colors](../../../artifacts/absence-mobile-after/)

Example final captures:

- [320px Classes](../../../artifacts/absence-mobile-after/absence-form-mobile-craft--f2eac-obile-form-for-craft-review-chromium-phone-small/classes.png)
- [390px make-up sheet](../../../artifacts/absence-mobile-after/absence-form-mobile-craft--f2eac-obile-form-for-craft-review-chromium-phone-standard/make-up-sheet.png)
- [320px enlarged text with simulated keyboard](../../../artifacts/absence-mobile-after/absence-form-mobile-craft--b6fc5-oard-resize-and-larger-text-chromium-phone-small/classes-large-text.png)

Baseline screenshots cover the basic fixture; long-name/multiple-course stress screenshots were captured after implementation, not as a pre-change baseline.

## Remaining acceptance

- Physical iPhone Safari and Android Chrome walkthroughs, including actual on-screen keyboard/autofill and rotation behavior.
- Manual VoiceOver reading-order, announcement, and interaction review.
- Resolution of the four baseline unit failures in the appropriate scheduling/test-maintenance work.

Browser emulation, mocked routes, and axe are the exercised evidence. They do not establish real SMS delivery, backend transactional correctness, or physical-device assistive-technology usability. The plan's full T6/manual acceptance is therefore still outstanding; no claim of complete native HIG/WCAG certification is made.
