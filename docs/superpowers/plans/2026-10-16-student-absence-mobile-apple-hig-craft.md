# Student Absence Form — Mobile UX/UI and Apple HIG Craft Implementation Plan

- **Date:** 16 October 2026
- **Status:** Implemented with automated verification; physical-device/VoiceOver acceptance remains outstanding. See the [implementation and verification record](2026-10-16-student-absence-mobile-apple-hig-verification.md).
- **Primary route:** `/absence`
- **Goal:** Make the existing student absence form calm, readable, precise, and comfortable to complete on a phone, using Apple Human Interface Guidelines as the interaction and craft reference.
- **Core constraint:** Preserve the current business requirements, four-step flow, validation gates, selection semantics, and submission contract.
- **Stack:** Existing React/TypeScript components, Tailwind/CSS variables, native form controls, native `<dialog>`, Framer Motion, Vitest/Testing Library, and Playwright. No new runtime dependency is needed.

## 1. Design decision and relationship to earlier plans

This is the current mobile presentation brief for the request in this conversation. Where the visual direction differs from `2026-10-09-student-public-absence-ux-ui.md`, use this plan's Apple HIG–informed direction for this work. The earlier document remains historical context; its additional behavior proposals are not implicitly included.

The experience should feel like a carefully made, trustworthy form: clear hierarchy, generous touch areas, familiar controls, quiet surfaces, and accurate feedback. Keep Warwick's identity and existing blue accent.

Craftsmanship is judged by how well people can read, choose, correct, and finish. Rounded corners alone are not the acceptance criterion.

### 1.1 HIG principles translated to this web form

| Principle | Product application | Observable result |
| --- | --- | --- |
| Clear visual hierarchy | Distinguish course, absence date, time, and make-up arrangement | A student can identify the date they are missing and its replacement without decoding one long sentence |
| Alignment and grouping | Use shared leading edges, section spacing, and hairline separators | Related information reads as one group without multiple nested cards |
| Progressive disclosure | Reveal dates for the selected course and make-up controls for selected dates | Relevant choices remain visible without exposing every control at once |
| Familiar, bounded modality | Use the existing sheet for the single task of choosing a make-up class | Cancel returns unchanged; Confirm applies the staged selection |
| Adaptability | Respect available width, visible viewport, safe areas, and larger text | Keyboard, rotation, and text enlargement do not hide required actions |
| Perceivable feedback | Pair color with text, checked controls, and accessible status messages | Selection, unavailability, errors, and submission state are understandable without color alone |

Official references consulted on 16 October 2026:

- [Apple HIG: Layout](https://developer.apple.com/design/human-interface-guidelines/layout.md) — hierarchy, grouping, progressive disclosure, text-size changes, and safe areas.
- [Apple HIG: Sheets](https://developer.apple.com/design/human-interface-guidelines/sheets.md) — scoped tasks, clear confirmation/dismissal, and avoiding stacked sheets.
- [Apple HIG: Accessibility](https://developer.apple.com/design/human-interface-guidelines/accessibility.md) — readable text, comfortable controls, contrast, alternative interaction, and reduced motion.

The dimensions below are project specifications for responsive web, not claims that CSS pixels equal native points. Browser text enlargement is the relevant web behavior; do not claim native Dynamic Type integration. Keeping confirmation at the bottom of the web sheet is a deliberate thumb-reach adaptation of HIG's explicit completion principle, rather than a literal copy of native toolbar placement.

## 2. Fixed business and flow contract

### 2.1 Preserve the sequence

```text
1. Student
   W-Code → explicit Search → conditional email entry → Continue to verification

2. Verify
   Existing parent destination OR permitted missing-phone enrollment
   → explicit Send code → six-digit verification
   → current fresh-success auto-advance / restored-session Continue behavior

3. Classes
   Select courses → choose absence dates → choose required make-up classes
   → enter reason → Review absence

4. Review
   Check identity, classes, make-up arrangements, and reason
   → optional nickname when currently permitted → Submit absence

Receipt
   Authoritative submission result and reference; this is not a fifth form step
```

### 2.2 Invariants

1. Retain public step indices `0 | 1 | 2 | 3`, existing forward/back gates, and completed-step navigation. Future steps cannot be opened early.
2. Keep Search and Continue as separate existing actions. Typing or entering a step must not automatically send an SMS.
3. Keep the existing public lookup privacy boundary: use server-provided masked hints before verification; private profile and course data require the existing verified session.
4. Ask for email only when the lookup response requires it. Keep existing parent-phone enrollment conditions, OTP length, delivery states, cooldown duration, expiry handling, and retry behavior.
5. Keep `AbsenceForm` as the workflow and committed-selection owner. Course expansion, the selected make-up day, and pending sheet selection are presentation state only.
6. Use existing combined course blocks and logical day grouping. A merged class day must still select its complete underlying session-ID group and count according to existing rules.
7. Preserve quota calculations, already-absent restrictions, make-up priority progression, conflict handling, duplicate-selection restrictions, and server ordering. Display changes do not broaden eligibility.
8. Physical make-up requirements remain exactly as currently enforced, including when no valid option is offered. Keep Zoom and staff/teacher-arranged states distinct. Never infer a validation exception from explanatory copy.
9. Keep the current required reason and 500-character boundary. Configuration contains other reason settings, but this task does not change the form's current requirement or reconcile policy/configuration differences.
10. Keep optional nickname on Review under its existing eligibility condition, with its existing nonblocking submission fallback and privacy handling.
11. Keep pre-submit availability refresh, batch payloads, idempotency keys, uncertain-network retry behavior, receipt grouping, and successful-submission draft clearing intact.
12. Preserve existing same-tab draft restoration and fresh-data revalidation. No new storage, persistence promise, or cross-device saving behavior is introduced.
13. Preserve the `Asia/Bangkok` institute-timezone formatting boundary and the existing date/time helpers.
14. Shared components must preserve staff workflow behavior. Public-specific markup and sizing must be scoped to public mode; staff receives regression verification.

## 3. Current implementation and improvement targets

The scan was based on the current source, styles, and existing tests. No live-device review or application test run was performed during plan creation. Browser/keyboard effects described below are verification targets, not observed device-test results.

| Current evidence | User consequence | Planned improvement |
| --- | --- | --- |
| `src/pages/AbsenceForm.tsx`: course selector, separate mobile selected-course buttons, repeated block title | Repeated reading before reaching dates | Keep the selector first; combine each selected-course toggle and block header into one accordion header |
| `SessionDayCard.tsx` and nested make-up content in `AbsenceForm.tsx` | Borders and padding repeatedly reduce usable line width | Flat date rows inside one selected-course surface; make-up content aligned beneath its date |
| `MakeUpPicker.tsx`: search box above the option list (superseded 2026-10-18) | Browsing required typing to narrow a short list, and the keyboard opened before any time was read | Search removed: the sheet shows the month with a day strip that filters the list; heading receives focus on entry |
| `MakeUpPicker.tsx`: confirmation follows options in the scrolling body | Long option lists separate choice from confirmation | Persistent action area outside the option scroller |
| `AbsenceForm.tsx`: truncated course header; `SessionDayCard.tsx`: one-line date/time | Essential differences and times are harder to inspect | Wrapping course names and separate date/time lines |
| `AbsenceActionBar.tsx`: empty first-step spacer | Primary action has less width on compact phones | Full-width first-step primary; flexible primary beside Back afterward |
| `StepProgress.tsx`: 36px circles; priority actions in `AbsenceForm.tsx`: 32px height | Small targets for returning to steps or browsing other times | At least 44×44 CSS px actionable areas |
| `ReasonField.tsx`: count plus colored character-progress bar | A limit looks like a target that needs filling | Quiet character guidance without a progress bar |
| Review in `AbsenceForm.tsx`: absence and make-up concatenated in 12px paragraphs | Hard to compare the original and replacement | Stacked, explicitly labelled pairs |
| Receipt in `AbsenceForm.tsx`: side-by-side absence/make-up paragraphs | Long summaries compete for narrow width | Stack the same labelled information on phones |
| `goToStep` scrolls `window`, but `AbsenceAppShell.tsx` scrolls `<main>` | Step/edit navigation may retain an inappropriate scroll position | Reset the real scroll owner and apply intentional post-render focus |

Source locations are identified by component/symbol because line numbers will move during implementation. Unqualified public-form component filenames below resolve beneath `src/components/absences/public-form/`; `StepCoverVerification.tsx`, `OtpInput.tsx`, and `SmsSendButton.tsx` resolve beneath `src/components/absences/`.

## 4. Visual craftsmanship specification

### 4.1 Layout and rhythm

| Property | Required specification |
| --- | --- |
| Simple-step content width | Reuse the current `max-w-3xl` / 48rem maximum. Align header, action-bar inner content, and receipt to the same boundary |
| Phone horizontal gutters | 16px minimum, increased by the relevant safe-area inset where necessary |
| Section separation | 24px; increase to 32px only between clearly distinct tasks |
| Related field spacing | 16px between fields, 8px between label and control, 4–8px between primary and secondary text |
| Course surface padding | 16px at the outer level; use separators instead of a second set of padded bordered date cards |
| Date rows | At least 56px high before disclosed content; height grows with wrapping and text size |
| Inputs and primary actions | At least 48px high; minimum height rather than fixed height for text-bearing controls |
| Secondary/icon actions | At least 44×44 CSS px hit area; visible glyph may remain 20–24px |
| Radii | Public-only 12px for inputs, buttons, and grouped surfaces; up to 20px on sheet top corners |
| Elevation | Flat ordinary sections; one subtle overlay shadow for the sheet |

Keep the existing tablet/desktop Classes layout behavior. This task simplifies the phone layout without replacing the wide-screen interaction model. Existing responsive breakpoints remain the starting point; wrapping must remain safe at intermediate widths.

### 4.2 Typography

Use the existing system font stack. Express sizes in `rem` and allow browser scaling.

| Role | Size / line height | Weight |
| --- | --- | --- |
| Step heading | 24px / 32px | 600 |
| Course/date/option primary content | 16px / 24px | 500–600 |
| Entered text and main instructions | 16px / 24px; 17px is acceptable for introductory body copy | 400 |
| Supporting text, labels, status, errors | 14px / 20px | 400–500 |
| Primary action | 16px / 24px | 600 |

- Essential dates, times, remaining allowance, and errors must not be rendered as 10–12px metadata.
- Wrap meaningful names. Avoid truncation and line clamps on identifying course/teacher information.
- Use sentence case for instructions and section headings. Reduce uppercase tracking in form content.
- Use tabular numerals only where alignment helps, such as counts and countdowns.
- Support at least 200% text enlargement and reflow at a 320 CSS px content viewport without horizontal page scrolling.

### 4.3 Color, surfaces, and state

- Reuse Warwick's semantic tokens from `DESIGN.md`: primary, text, text-light, background, line/border, danger, warning, and success.
- Scope public typography, spacing, and surface overrides beneath an explicit public form/receipt root. Forward the existing form mode for this purpose; avoid a global restyle.
- Blue identifies the recommended action and selected controls. Checked controls and text must communicate the same state.
- Use neutral surfaces for ordinary make-up instructions; reserve amber/red for an actual warning or error.
- Measure text contrast at 4.5:1 minimum; qualifying large text may use 3:1. Essential control boundaries and focus indicators require 3:1 against adjacent colors.
- Disabled options retain readable explanations. Do not lower the opacity of all content until it becomes difficult to read.
- Provide pressed, focus-visible, loading, and disabled states. Touch users must not depend on hover.
- Maintain normal readability with forced colors and reduced motion. Use solid surfaces rather than introducing decorative glass, gradients, or glow.

### 4.4 Motion and finishing details

- Use existing motion infrastructure: approximately 150–180ms for restrained opacity/position changes.
- Avoid spring/bounce effects, animated character meters, and repeated shaking.
- Honor `prefers-reduced-motion`; feedback must remain visible without animation.
- Disclosure must not scroll a selected date unexpectedly out of view.
- Use the existing icon family with consistent size/stroke treatment. Decorative icons are hidden from assistive technology.
- Keep borders, checkbox centers, text baselines, and leading edges optically aligned across selector, dates, and summaries.

## 5. Screen-by-screen UX specification

### 5.1 Header, progress, and action bar

**Files:** `src/components/absences/public-form/AbsenceAppHeader.tsx`, `StepProgress.tsx`, `AbsenceActionBar.tsx`, `AbsenceAppShell.tsx`, and `src/index.css`.

1. Use a compact brand identity row and one coherent progress row. Avoid separately repeating the current step name beneath a large progress region.
2. Retain four public stages, numbered indicators, meaningful accessible names, `aria-current="step"`, and completed-stage buttons. Give completed indicators 44px minimum hit areas; their visible circles may be smaller.
3. Show a concise current-position label such as `Step 3 of 4`. Let it wrap at larger text sizes rather than squeezing indicators.
4. While the keyboard is open, preserve the current compact-header strategy and a readable current-step indication. Completed-step navigation must remain reachable again after keyboard dismissal.
5. Remove the empty first-step spacer. Make that step's primary action full width; later primaries flex beside a compact Back action. Allow wrapping/growth for long action labels.
6. Keep existing primary labels and enabled/disabled conditions. Explain a blocked prerequisite near its field; do not add another validation gate or enable unavailable navigation.
7. Preserve the header/main/footer grid and safe-area behavior. The footer stays outside the main scroller; it is not a new overlay covering form content.

### 5.2 Student

**Files:** `src/pages/AbsenceForm.tsx` Student section and `StudentStep.tsx`.

- Keep W-Code entry, explicit Search, the lookup result, conditional email, and footer Continue in their existing order.
- Give the W-Code input text-friendly entry behavior: uppercase capitalization hint, spellcheck/autocorrect off, and an appropriate Search keyboard action. Keep normalization and validation in their existing owner.
- Keep the full `W250389` format usable; a numeric-only keyboard is inappropriate for the W prefix.
- Retain the visible input label and example. Search must remain comfortable at 320px with the keyboard open; wrap its layout if needed.
- Present the found result as a quiet grouped section. Show masked hints only; do not expose additional identity information.
- Remove the redundant “Ready to verify” badge and retain the existing “Student ID found” status. Lookup success must not imply all step prerequisites are complete.
- Use `type="email"`, email autofill, 16px text, and a clear associated explanation for invalid email. Keep the existing Continue gating.
- Keep lookup loading, errors, and retry affordances close to the input. Do not add repeated live announcements during typing.

### 5.3 Parent verification

**Files:** `VerificationStep.tsx`, `src/components/absences/StepCoverVerification.tsx`, `OtpInput.tsx`, and `SmsSendButton.tsx`.

- Reduce the student identity block's visual weight; emphasize the masked destination and next action.
- Explain the task in one short sentence: `Enter the 6-digit code sent to your parent's phone.`
- Keep one real OTP input, six visual cells, numeric entry, paste, `one-time-code` autofill, and existing auto-verification.
- Keep explicit sending/resending and the current automatic transition after fresh success. A restored verified session keeps its existing Continue behavior.
- Render cooldown as a readable `Resend in 4:32` style time rather than a large raw-second count; preserve its actual duration and eligibility logic.
- Give retry and resend actions 44px minimum targets. Remove duplicate decorative cooldown feedback if the text already explains the wait.
- Keep queued, uncertain, failed, expired, and verified states truthful. Do not treat provider acceptance as guaranteed device delivery.
- Preserve the no-phone enrollment explanation and the existing SMS-disabled state. No verification bypass is added.

### 5.4 Classes and absence dates

**Files:** Classes rendering in `src/pages/AbsenceForm.tsx`, `ClassesStep.tsx`, `SubjectRow.tsx`, and `SessionDayCard.tsx`.

1. Keep the first section as the course selection list; use `Select courses` instead of ambiguous `Which classes?`.
2. In `Choose class dates`, render one accordion header per selected display block on phones. Replace the separate mobile toggle plus repeated panel title with a single header directly adjoining its panel.
3. Keep the existing single-expanded-course behavior. Selecting a course opens it; opening or closing a selected course never changes its checked state or chosen dates.
4. The header contains a wrapping course name, optional teacher line, chevron, and local counts. Avoid a separate trailing “Selected” badge if the checkbox already communicates selection.
5. Show `1 class day selected` and the current computed allowance with precise local wording, such as `2 more absence days available`. Never substitute a hardcoded cap or sum unrelated course allowances into one actionable quota.
6. Show each absence date on a primary line and time range on a secondary line. A row uses a native checkbox and a generous associated label; do not make the entire container a button around nested controls.
7. Selected rows reveal one make-up area without another full bordered card. The action remains inside its owning date's context.
8. Keep “See other times” and “Previous times” under the make-up content with 44px hit areas. Their handlers and priority semantics remain unchanged; browsing another level may clear the current choice exactly as it does today.
9. Keep specific explanations for already-submitted dates, exhausted allowance, and all remaining days selected. Selected valid rows must remain deselectable.
10. Preserve the existing retry, loading, and empty states. A re-render, disclosure action, or larger text setting must not refetch sessions merely for presentation.

### 5.5 Make-up picker and sheet

**Files:** `MakeUpPicker.tsx`, `MobileBottomSheet.tsx`, its two call sites and option builder in `src/pages/AbsenceForm.tsx`, and sheet styles in `src/index.css`.

> Superseded 2026-10-18: the sheet no longer carries a search box. The month and its days sit at the top of the sheet as a day strip (chips) that filters the option list, for the public form and the staff sheet alike. A single-day list shows a static full-date caption instead of chips, days whose times are all unavailable stay tappable but muted, and every disabled option keeps its own reason on its own day.

**Presentation**

- Keep the current responsive split: mobile sheet and existing wide-screen select. Use the same option values and committed selection for both.
- Structure the sheet as header, date strip, scrollable option list, and persistent confirmation footer. Give the list `min-block-size: 0` so it can actually shrink.
- Show a short context line identifying the absence being arranged, for example `For Mathematics · Mon, 19 Oct · 09:00–12:20`.
- Use date/time as the option's primary information, followed by class and teacher. Preserve conflict messages and details without ellipsis.
- Render the strip from institute-local day keys on the existing display model (never by parsing labels). Chip availability, the ambiguous-month suffix, and the day summary are presentation only: filtering must not reveal another priority tier, change option order within a day, or alter any reason copy.
- Remove the decorative grabber if the web sheet is not draggable. Keep an explicit, labelled close/cancel control.
- Keep the existing clear-selection behavior. In the public sheet, label it `Clear selection` and explain that it removes the current make-up choice; it does not fulfill a required selection. Preserve the default label for other callers.

**Selection and dismissal contract**

| Event | Required result |
| --- | --- |
| Open | Stage the committed value; focus heading with `tabIndex={-1}` and the existing initial-focus marker; no forced search keyboard |
| Choose an option | Change pending sheet selection only; radio and selected styling agree |
| Confirm | Commit once through the existing `onChange`, close, and restore focus |
| Cancel, close, Escape, backdrop | Discard staged changes; committed choice remains unchanged |
| Day filter hides the staged option | Keep the staged value and a concise visible selection summary so it is not invisible when confirming |
| Selected day has no available times | Keep the day tappable, mark its chip, and show its per-option reasons plus one day-level line; the whole-list empty state appears only when no day has a selectable time |
| Pending option disappears/becomes disabled | Disable confirmation and explain that another option is needed; do not commit an unavailable value |
| Close or confirm | Restore focus to the triggering make-up control without reopening the keyboard unnecessarily |

**Implementation boundary**

- Reuse native `<dialog>`, existing modal focus isolation, and restoration behavior.
- Add a footer slot to the existing sheet rather than duplicating it or placing another fixed viewport overlay inside it. Preserve existing behavior for callers that do not supply the slot.
- Reuse `useVisualViewport` at the sheet boundary for available height and offset; keep this concern in the sheet rather than each picker caller. Do not assume `dvh` alone shrinks for every mobile keyboard.
- Ensure the modal surface fits between the visible viewport top and bottom after safe-area treatment. Avoid double-applying safe-area padding.
- The header and action area may wrap at larger text sizes. In very short landscape/keyboard viewports, permit the full surface to scroll as a fallback so no control is clipped.
- Pass optional structured display data from the existing `normalizeSitInDisplayModel` into `MakeUpOption`; retain existing `value`, `label`, `details`, and disabled semantics for fallback callers and the wide select. Do not parse concatenated labels to reconstruct dates, teachers, or times.
- Pass missed-class context as display-only props. Keep eligibility and option construction in their existing owners.

### 5.6 Reason

**File:** `ReasonField.tsx`.

- Preserve required entry, the existing value owner, maximum length, and inline error association.
- Keep the visible label `Reason for absence` and add concise guidance: `Briefly tell us why you will miss these classes.`
- Remove the colored progress bar. At normal lengths, show `500 characters maximum`; after 450 characters, show the remaining count.
- Reaching the 500-character maximum is not an invalid value. Explain `Character limit reached` without falsely marking valid text as an error.
- Keep the count visual-only rather than a live region updated on every keystroke.
- Use a comfortable initial textarea height around 120px and preserve resizing/visibility with the keyboard open.

### 5.7 Review and receipt

**Files:** Review and receipt sections in `src/pages/AbsenceForm.tsx`, plus `ReviewStep.tsx`.

- Show each course with a clearly paired `Missing` and `Make-up` summary for every selected logical day. Stack labels and values on phones.
- Use the existing grouping, review-label, and result-summary helpers. Structured presentation must not invent an arrangement or split a merged day into duplicate requests.
- If the existing review helper returns a single arrangement label, render it as that value. Do not parse the string to infer booking status.
- Keep the required reason readable as its own section and preserve the optional nickname field on Review.
- Keep `Edit classes` and `Edit reason` in their current step. Returning to Classes should reveal and focus the intended course or reason field after rendering.
- Keep `Submit absence`, the existing loading state, and uncertain-result recovery. Receipt appears only after authoritative success.
- Keep the server-derived reference and all returned grouped results. State `Awaiting staff review` rather than implying approval, guaranteed attendance, or delivered notifications.
- Apply the same typography, spacing, and phone stacking to receipt content. Keep safe-area gutters and heading focus.

### 5.8 Navigation and validation feedback

**Files:** `goToStep`, `validateClasses`, and `focusFirstInvalid` in `src/pages/AbsenceForm.tsx`; the existing main element in `AbsenceAppShell.tsx`; `FormAlert.tsx` if association markup is required.

- Reset `.absence-app-shell__main`, not only `window`, on explicit step navigation.
- Centralize scroll/focus behavior at the existing navigation boundary. Use the existing main ID/ref; avoid a second navigation store or per-component scroll effects.
- After a normal step transition, focus the new step/main without automatically opening a text keyboard.
- After an explicit Edit reason action, focus the reason field and keep it above the footer/keyboard.
- After an invalid make-up check, expand the actual owning course and focus that specific visible trigger. Do not focus an unrelated valid picker merely because it is first in the DOM.
- Apply focus only after target disclosure and DOM commit. An error announcement and a field focus action must not race to steal focus from one another.
- Preserve existing error meaning, validation order, retry actions, and entered data.

## 6. Mobile reference compositions

Illustrative content only; production dates, names, counts, and arrangements come from existing data/helpers.

### 6.1 Classes

```text
Warwick Institute                         Student absence
[1] — [2] — [3] — [4]                       Step 3 of 4

Courses & classes

Select courses
┌──────────────────────────────────────────────────────┐
│ ☑ Mathematics                                       │
│   AJ. Nice                                          │
│ ☐ SAT Verbal                                        │
│   AJ. Ann                                           │
└──────────────────────────────────────────────────────┘

Choose class dates
┌──────────────────────────────────────────────────────┐
│ Mathematics                                      ▾  │
│ 1 class day selected                                │
│ 2 more absence days available                       │
│ ────────────────────────────────────────────────── │
│ ☑ Mon, 19 Oct 2026                                  │
│   09:00–12:20                                       │
│                                                     │
│   Make-up class                                     │
│   Sat, 24 Oct · 13:00–16:20                       ›  │
│   Mathematics · AJ. Nice                            │
│                                                     │
│   Previous times             See other times        │
│ ────────────────────────────────────────────────── │
│ ☐ Wed, 21 Oct 2026                                  │
│   09:00–12:20                                       │
└──────────────────────────────────────────────────────┘

Reason for absence *
Briefly tell us why you will miss these classes.
[ Medical appointment…                                 ]
500 characters maximum

────────────────────────────────────────────────────────
‹ Back                                 [Review absence]
```

The selector and selected-course section have different roles. The improvement removes the third repeated heading, not the existing course-selection stage.

### 6.2 Make-up sheet

```text
Cancel                           Choose a make-up class
For Mathematics · Mon, 19 Oct · 09:00–12:20

October 2026
[Sat 24] [Sun 25] [Mon 26]

  Scrollable options for the chosen day
  ○ Sat, 24 Oct · 13:00–16:20
    Mathematics · AJ. Nice

  ● Sun, 25 Oct · 09:00–12:20
    Mathematics · AJ. Nice

  ○ Sun, 25 Oct · 10:00–11:30              Unavailable
    Mathematics · AJ. Nice
    Overlaps another class: Chemistry · 10:00–11:30

  ○ Clear selection

────────────────────────────────────────────────────────
Selected: Sun, 25 Oct · 09:00–12:20
[                 Confirm make-up class                ]
```

Cancel is the header dismissal control, not an extra Back step. No nested sheet is opened.

## 7. Dependency-ordered implementation tasks

The checklists below retain the implementation acceptance criteria. The [implementation and verification record](2026-10-16-student-absence-mobile-apple-hig-verification.md) records execution results, baseline failures, and outstanding manual checks. Each task ends with a verifiable result; code appearance alone is not proof of acceptance.

### T0 — Establish the current behavior and visual baseline

**Depends on:** none.

- [ ] Inspect `git status` and preserve existing work, especially scheduling/domain changes already in progress.
- [ ] Reconfirm the call chain and shared public/staff callers using the repository's TypeScript/AST tooling before code edits.
- [ ] Capture the current four public steps, receipt, and open picker with existing route fixtures at 320px and 390px; include a long-name and multiple-course case.
- [ ] Record baseline typecheck and relevant absence-test results. Attribute any failures with evidence instead of assuming they predate the change.
- [ ] Record existing staff flow behavior before modifying shared components.

**Exit:** baseline evidence and affected-callers list exist; the implementer can distinguish a new regression from a baseline issue.

### T1 — Establish the public visual foundation and shell behavior

**Depends on:** T0.

**Files:** `src/index.css`, `AbsenceAppShell.tsx`, `AbsenceAppHeader.tsx`, `StepProgress.tsx`, `AbsenceActionBar.tsx`, and public shell/receipt roots in `src/pages/AbsenceForm.tsx`.

- [ ] Forward existing mode into an explicit root data attribute and scope public craft rules there; keep staff behavior intact.
- [ ] Implement the width alignment, typography, spacing, surfaces, and minimum hit areas in section 4.
- [ ] Compact the header while preserving progress semantics and completed-step navigation.
- [ ] Remove the first-step spacer and make primary width flexible without hardcoded label assumptions.
- [ ] Preserve viewport sizing and safe areas; verify focus remains visible while the keyboard is open.
- [ ] Correct main-scroller reset at the existing navigation boundary; establish intentional edit-target focus.

**Verify:** shell/progress tests; browser measurement at 320px/390px, keyboard-open resize, and 200% text enlargement.

**Exit:** no clipped controls or content/footer overlap; all four progress stages and existing actions remain available.

### T2 — Simplify Classes and date selection

**Depends on:** T1.

**Files:** Classes rendering in `src/pages/AbsenceForm.tsx`, `ClassesStep.tsx`, `SubjectRow.tsx`, and `SessionDayCard.tsx`.

- [ ] Keep course selection first and render one adjacent accordion header/panel per selected block on phones.
- [ ] Preserve existing expansion state, combined block keys, checkbox ownership, and wide-layout behavior.
- [ ] Replace inner date-card nesting with readable rows and subordinate make-up content.
- [ ] Split date/time lines; allow full course and teacher names to wrap.
- [ ] Display selected-day and remaining-allowance information from current derivations with unambiguous wording.
- [ ] Enlarge priority navigation actions and preserve disabled explanations.
- [ ] Make validation reveal and focus the actual incomplete course/date context.

**Verify:** SubjectRow/SessionDayCard tests; page session-limit/final-session tests; multiple-course, merged-day, exhausted-quota, and already-absent scenarios.

**Exit:** identical selection/count/payload semantics with less repetition and no essential name truncation.

### T3 — Refine the make-up sheet

**Depends on:** T2 for the final trigger/context placement; T1 for visual metrics.

**Files:** `MakeUpPicker.tsx`, `MobileBottomSheet.tsx`, option construction/call sites in `src/pages/AbsenceForm.tsx`, and `src/index.css`. Reuse `src/hooks/useVisualViewport.ts` and the existing display model without rewriting their policy logic.

- [ ] Add optional structured display/context props while preserving existing option values and fallback presentation.
- [ ] Add the reusable sheet footer slot; keep header, option scroller, and footer in a shrinkable layout.
- [ ] Focus the heading on open, keep the day-strip version of the sheet keyboard-operable, and retain native dialog semantics and restoration.
- [ ] Keep confirmation and the selected summary visible independently of a long option list.
- [ ] Verify pending selection against current options before confirmation and preserve cancel/clear behavior.
- [ ] Size/position the sheet against the actual visible viewport, including keyboard and offset changes.
- [ ] Remove the inactive grabber and retain a clear dismissal control.
- [ ] Keep the wide-screen select usable and equivalent; check a resize while a sheet is open does not leave an invisible modal trapping focus.

**Verify:** extend picker/sheet tests for cancel, Escape, pending invalidation, an off-day staged selection, focus, and single commit; browser long-list/day-strip/keyboard/rotation checks.

**Exit:** browsing does not force keyboard entry, confirmation stays reachable, and no unavailable or staged-only choice is submitted.

### T4 — Polish Student, verification, and reason

**Depends on:** T1; may begin after the Classes/sheet design is stable.

**Files:** Student markup in `src/pages/AbsenceForm.tsx`, `StudentStep.tsx`, `VerificationStep.tsx`, `StepCoverVerification.tsx`, `OtpInput.tsx`, `SmsSendButton.tsx`, and `ReasonField.tsx`.

- [ ] Apply readable labels, instructions, input attributes, and prerequisite explanations without changing gating or lookup/SMS triggers.
- [ ] Make lookup-result readiness wording accurate when email is missing.
- [ ] Reduce identity-card decoration and improve masked-destination/OTP hierarchy.
- [ ] Format the existing resend countdown as minutes/seconds and enlarge secondary actions without changing its timer.
- [ ] Remove the reason progress bar, implement quiet limit guidance, and keep 500 characters valid.
- [ ] Preserve phone enrollment, SMS-disabled, wrong-code, expired-code, and restore/retry states.

**Verify:** Student page tests, StepCoverVerification/OtpInput/SmsSendButton tests, ReasonField tests, and keyboard-open mobile walkthrough.

**Exit:** the same entry/verification requirements are clearer; optional/required fields and recovery states remain accurate.

### T5 — Refine Review, edit navigation, and receipt

**Depends on:** T2–T4.

**Files:** Review/receipt/navigation in `src/pages/AbsenceForm.tsx` and `ReviewStep.tsx`.

- [ ] Render labelled missing/make-up pairs for each existing grouped day and all returned receipt groups.
- [ ] Apply readable phone stacking, consistent section spacing, wrapping, and safe-area gutters.
- [ ] Preserve optional nickname placement, its masked echo, and submit fallback.
- [ ] Verify Edit classes reveals the relevant course and Edit reason focuses the field after render.
- [ ] Preserve submitting/uncertain/error states and authoritative-success-only receipt rendering.

**Verify:** critical-path and error-recovery E2E; multi-day/multi-course review; nickname paths; returned batch receipt and focus.

**Exit:** families can compare and correct their request easily; receipt continues to mean submitted and awaiting review.

### T6 — Run integrated mobile and craftsmanship acceptance

**Depends on:** T1–T5.

- [ ] Run the commands and scenarios in section 8, including staff regressions.
- [ ] Compare baseline and final screenshots using the same fixture data, viewport, and motion setting.
- [ ] Complete real iPhone Safari and Android Chrome walkthroughs, including keyboard and text enlargement.
- [ ] Complete VoiceOver and keyboard-only checks for the form and modal.
- [ ] Inspect final changed files for unrequested policy, storage, API, or global-design changes.
- [ ] Record exact passing checks, baseline failures, and any unexercised device scenarios.

**Exit:** the acceptance matrix is satisfied with evidence; document outstanding checks rather than claiming untested compliance.

## 8. Verification and acceptance

### 8.1 Existing automated checks to reuse

Run during implementation, not as evidence that plan writing changed application behavior:

```bash
npm run typecheck
npm run test:absence
npx vitest run src/components/absences/public-form/__tests__ src/hooks/useVisualViewport.test.tsx
npx playwright test e2e/absence-form.spec.ts e2e/absence-form-accessibility.spec.ts e2e/absence-form-critical-path.spec.ts e2e/absence-form-error-recovery.spec.ts e2e/absence-form-resilience.spec.ts e2e/absence-timezone.spec.ts --project=chromium-phone-small --project=chromium-phone-standard --project=webkit-phone
npx playwright test e2e/absence-form-accessibility.spec.ts --project=chromium-phone-landscape --project=chromium-tablet-portrait --project=chromium-tablet-landscape --project=chromium-desktop
npm run build
```

The explicit public-form test command matters because `test:absence` currently names `src/components/absences/__tests__`, not the nested public-form test directory. Playwright's configured web server builds the app when needed.

Extend existing tests for meaningful interaction changes. Pure spacing, radius, or decorative changes are verified through visual review rather than tests that simply assert implementation class strings. Keep payload/flow assertions intact when accessible labels or markup change. Do not retain hidden duplicate controls merely to satisfy old selectors.

### 8.2 Acceptance matrix

| ID | Scenario | Pass condition | Evidence |
| --- | --- | --- | --- |
| A01 | Complete the public form | Same four steps, conditions, payload values, and submission boundary | Existing critical-path/page tests |
| A02 | Student lookup, conditional email, and unavailable SMS | Same request triggers/gates; accurate guidance and no extra private data | Student/verification tests |
| A03 | Fresh and restored verification | Fresh success auto-advances once; restored session retains current Continue behavior; cooldown/expiry unchanged | Verification tests and browser flow |
| A04 | Select/collapse/reopen several courses | One adjacent selected-course header/panel per block; expansion independent from checked selection | Component/page tests and mobile review |
| A05 | Merged day, quota exhausted, already absent | Existing logical-day count and disabled semantics; selected valid date remains removable | Session-limit/final-session/domain tests |
| A06 | Physical/Zoom/teacher-arranged make-up | Same eligibility and validation; all arrangements have truthful readable descriptions | Existing page/domain tests |
| A07 | Open picker without typing | Modal is named; heading receives focus; no keyboard opens; the strip marks days without available times | Picker test and real-device check |
| A08 | Stage/cancel/reopen/confirm/clear | Cancel commits nothing; reopen stages committed value; Confirm commits once; clear remains empty | Picker/sheet tests |
| A09 | Switch make-up day and invalidate the staged option | The staged value stays summarised while its day is hidden; its reason is readable on its own day; an invalid option cannot be confirmed | Picker tests and browser check |
| A10 | Long day list with keyboard open | The day strip scrolls, and the last day's option, plus confirmation, stay reachable within the visible viewport; background cannot receive focus | Browser viewport checks and devices |
| A11 | Validation in a collapsed course | Actual incomplete course opens; its visible invalid control receives focus and associated feedback | Error-recovery E2E |
| A12 | Step changes and Edit reason/classes | Correct main scroll position/target; entered data retained; target not obscured by footer | Navigation E2E |
| A13 | Reason at 0, 450, 451, and 500 characters | Existing required/max behavior; quiet guidance; 500 valid characters not reported invalid; no bar | ReasonField tests and visual review |
| A14 | Multi-course/day Review and receipt | Every existing logical pair readable; returned groups/reference preserved; submitted is not approved | Critical-path/page tests and screenshots |
| A15 | Optional nickname and uncertain submit retry | Existing optional conditions/fallback, key handling, and truthful result recovery retained | Existing page/error-recovery tests |
| A16 | 320/360/390/412px, landscape, tablet, desktop | No horizontal page scroll, overlapping targets, essential truncation, or clipped footer; wide picker remains usable | Existing viewport matrix plus measured review |
| A17 | 200% text enlargement and long names | Rows/actions grow or stack; no hidden identifying content; all actions reachable | Manual browser/device check |
| A18 | VoiceOver, keyboard, reduced motion, forced colors | Names, state, reading order, dialog isolation, focus return, and errors usable without color/motion | axe plus manual checks |
| A19 | Shared staff form | Current staff steps, lookup, selection, validation, and submit behavior preserved | Existing staff/page tests |
| A20 | Same-tab restore and stale availability | Existing authorization and fresh-data revalidation remain; UI does not imply persistent/cross-device saving | Existing student/storage/resilience tests |

### 8.3 Visual craft review

Use a consistent 390px reference screenshot and then challenge it at 320px and larger text. Accept only when:

- A viewer can distinguish course, missed date, and replacement within a quick scan.
- All primary text shares intentional leading edges; subordinate make-up content has consistent indentation.
- Borders define groups rather than repeatedly boxing the same task.
- White space separates tasks while related labels stay close to controls.
- Blue directs the next action; warnings represent actual warnings.
- All actionable small controls have measured hit areas, not only large-looking icons.
- Long names, multi-line errors, and disabled explanations look intentionally designed.
- Loading/empty/error/success states use the same typographic and spatial system.
- The first-step action uses available width; later Back/primary controls remain balanced at larger text sizes.
- The sheet's confirmation action is easy to find, the selected choice is clear, and closing returns to the correct context.

### 8.4 Evidence boundaries

- Existing test coverage and source support this plan; they do not establish that the proposed implementation already passes.
- Mocked browser routes verify UI behavior and request shape, not real SMS delivery or backend scheduling correctness.
- Browser viewport simulation does not establish real iOS keyboard/autofill or VoiceOver usability.
- Record unavailable browsers/devices as incomplete verification, without substituting screenshot review for interaction testing.

## 9. Definition of done

The implementation is complete when T0–T6 are completed with evidence, A01–A20 have passing verification, and the final review demonstrates both:

1. **Business continuity:** the existing absence workflow, authorization, scheduling requirements, data ownership, and submission semantics are preserved.
2. **Visible craftsmanship:** simpler Classes, readable dates and summaries, a browsing-first reachable sheet, comfortable touch targets, precise hierarchy, and stable keyboard/focus behavior across the supported mobile sizes.

Any unresolved functional failure or inaccessible required action blocks completion. If device verification is unavailable, report the completed implementation/checks and the outstanding device acceptance separately; do not mark T6 or the affected acceptance rows complete.
