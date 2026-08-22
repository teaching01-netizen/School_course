# Gate Review: merged-course helper copy removal

- recommendation: APPROVE
- blockers: []
- originalIntent: Remove the student-facing helper sentence beginning “One merged course — absences share a single quota across” from the merged-course block in the public absence form, without redesigning or changing merged grouping, quota, title, status, or session selection.
- desiredOutcome: The merged-course card renders without the helper row on desktop and mobile while retaining its course header, shared remaining-days status, grouped session rows, and selection controls.
- userOutcomeReview: PASS. The production diff removes only the conditional helper JSX. Fresh desktop and mobile captures show the card header, remaining-days status, and session checkboxes intact, with no helper row and no mobile overflow. The focused regression test reproduces 25/25 passing and retains assertions for merged title/shared quota while asserting the removed copy is absent.

## Checked artifact paths

- `src/pages/AbsenceForm.tsx`
- `src/pages/__tests__/AbsenceForm.sessionLimit.test.tsx`
- `DESIGN.md`
- `/tmp/absence-qa-990x372.png`
- `/tmp/absence-qa-390x844.png`
- `/var/folders/my/ksg8mwx54gdb6xzdrmgvx7zh0000gn/T/TemporaryItems/NSIRD_screencaptureui_NxsGFS/Screenshot 2569-08-22 at 19.07.56.png`
- `git diff -- src/pages/AbsenceForm.tsx src/pages/__tests__/AbsenceForm.sessionLimit.test.tsx`
- `npm test -- --run src/pages/__tests__/AbsenceForm.sessionLimit.test.tsx` (1 file, 25 tests passed)

## Criterion review

- Copy absent: PASS. JSX containing the sentence is deleted; the fresh captures contain no helper row; the focused test's DOM query is absent.
- Merged grouping preserved: PASS. No grouping code changed, and each fresh capture retains one merged card containing the session rows.
- Shared quota/status preserved: PASS. No quota calculation or status JSX changed; captures show `2 days remaining`; the focused test retains the non-summed-quota assertion (`4 days remaining` absent).
- Course title preserved: PASS. Header JSX is unchanged and visible in both captures (responsive truncation on mobile follows the existing layout).
- Session selection preserved: PASS. Toggle/selection handlers and session-row JSX are unchanged; enabled checkbox controls remain visible in both captures; focused functional suite passes.
- Responsive integrity: PASS. Desktop and 390px mobile captures show no helper gap or overlap; supplied runtime evidence reports `mobileOverflow=false`.

## Direct remove-ai-slops / programming pass

- Production change is the minimum deletion; it adds no extraction, parsing, normalization, abstraction, comments, defensive branches, dependencies, or scope drift.
- The changed test assertion is deletion-focused and pins literal UI copy absence. This is narrow evidence for the explicit user-visible criterion, but by itself would not prove functional preservation. It is not treated as false confidence because adjacent assertions preserve the merged title/shared-quota contract, the complete focused suite passes, and the rendered controls were inspected.
- No tautological, implementation-mirroring, excessive, or deletion-only production test scaffolding was added.
- Unrelated dirty-worktree changes were excluded from this review.

## Evidence gaps / notes

- No dedicated code-review report or manual-QA matrix for this goal was found under `.omo/evidence`; direct artifact inspection and reproduced focused tests provide completion coverage, so this is not a blocker.
- The supplied `headingCount=2` and `helperCount=0` runtime measurements were not independently regenerated from a live browser in this gate. The same visual outcomes were independently checked in both supplied fresh captures, and helper absence was reproduced in the focused DOM test.
- The reported pre-existing accessibility contrast failures are outside this copy-removal criterion and are not blockers.
