# Gate Review: Absence merged-course helper removal

- recommendation: APPROVE
- blockers: []
- originalIntent: Remove the student-facing merged-course helper sentence from the public absence form while keeping the course header and session cards visually coherent.
- desiredOutcome: The helper copy is absent; desktop and mobile layouts have no residual blank band, clipping, alignment regression, or unintended wrapping; existing course/quota/session information remains readable.
- userOutcomeReview: Satisfied. The helper row is absent in both supplied production-preview captures. The course header border transitions directly into the session-card area with normal padding. Desktop text is fully legible. On mobile, the long course title uses the existing single-line ellipsis while the quota remains fully visible; session labels and footer controls are unclipped and aligned.

## Checked artifact paths

- `/tmp/absence-qa-990x372.png`
- `/tmp/absence-qa-390x844.png`
- `/var/folders/my/ksg8mwx54gdb6xzdrmgvx7zh0000gn/T/TemporaryItems/NSIRD_screencaptureui_NxsGFS/Screenshot 2569-08-22 at 19.07.56.png`
- `src/pages/AbsenceForm.tsx`
- `src/pages/__tests__/AbsenceForm.sessionLimit.test.tsx`
- Git diff for the two scoped source files
- Focused Vitest execution: `npx vitest run src/pages/__tests__/AbsenceForm.sessionLimit.test.tsx` — 25/25 passed

## Evidence trace

- Helper removal: `AbsenceForm.tsx` diff removes the complete conditional helper-row element. Both QA captures show no helper text and no retained row background/divider.
- Desktop coherence: `/tmp/absence-qa-990x372.png`, merged-course block at center-right; header bottom border is immediately followed by normal session content padding. No blank band, clipping, baseline defect, or unexpected wrapping.
- Mobile coherence: `/tmp/absence-qa-390x844.png`, course block below “CLASSES TO MISS”; title truncates with an intentional ellipsis, quota remains readable, and cards fit within the viewport. No visible horizontal overflow or clipped text.
- Comparison context: the reference screenshot shows the removed helper row between the header and session cards, confirming the intended vertical space has collapsed.
- Behavioral evidence: focused suite reproduced locally with 1 file and 25 tests passing.

## Direct slop/overfit and programming pass

- Production diff is deletion-only and minimal; it adds no extraction, parser, normalization, abstraction, dependency, or unrelated behavior.
- The changed negative prose assertion in `AbsenceForm.sessionLimit.test.tsx:868` is a deletion-only/text-pinning assertion and provides limited behavioral value. This is a non-blocking NOTE because the stated criterion explicitly requires the helper copy to be absent, the assertion matches that criterion, and independent screenshot/source inspection establishes the outcome.
- Existing surrounding assertions still cover merged-block identity and shared-quota behavior. No implementation-mirroring or tautological production logic was introduced.
- No code review report, manual QA matrix, executor evidence file, or notepad path was supplied. Direct inspection and reproduced focused tests support completion; these absent reports are evidence gaps, not failures of a stated success criterion.

## Exact evidence gaps

- No standalone code review report was provided, so its skill-perspective coverage could not be confirmed.
- No manual QA matrix artifact was provided; the two named captures were inspected directly instead.
- The claimed live-DOM count of zero and mobile `scrollWidth <= clientWidth` measurement were not supplied as raw logs. Source inspection and screenshots support the visual result, but those exact browser values were not independently re-measured in this read-only visual review.
- No ULW loop plan exists; report uses the required fallback evidence path.
