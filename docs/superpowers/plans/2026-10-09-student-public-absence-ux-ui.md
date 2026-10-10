# Student Public Absence Form — Final UX/UI Implementation Plan

**Date:** 9 October 2026  
**Status:** Ready for implementation; this document does not authorize implementation or deployment.  
**Goal:** Make `/absence` look and behave like a familiar consumer form, using Google Forms as the dominant visual reference and Google Calendar appointment booking as the reference for make-up choices.  
**Architecture:** Retain `AbsenceForm` as the state and workflow owner. Refine its existing public-form components, preserve server-authoritative identity and scheduling rules, and reuse the existing modal dialog instead of introducing a second selection system.  
**Stack:** React 19, TypeScript, Tailwind CSS 4, existing CSS variables, native form controls and `<dialog>`, Vitest/Testing Library, Playwright/axe. No new runtime dependency is required.

Implementation instructions: execute the numbered tasks in dependency order. Define the specified acceptance tests before changing the corresponding behaviour. Do not treat a screenshot, passing mocked test, or visual resemblance as proof of server correctness.

## 1. Goal and scope

Deliver a familiar, calm, mobile-friendly form with four steps:

**Student ID → Parent verification → Choose classes → Review and submit → Receipt**

The receipt is the result of submission, not a fifth input step. Preserve the public step indices `0 | 1 | 2 | 3` and existing draft schema.

Required outcomes:

| Requirement | Outcome | Acceptance coverage |
| --- | --- | --- |
| R1 — Familiar visual system | Google Forms-style single-column sections, conventional controls, restrained Warwick branding | AT-01, AT-18, AT-19, AT-26 |
| R2 — Clear entry | One public primary action; no stale student identity after editing | AT-02–AT-05 |
| R3 — Understandable verification | Masked destination, usable help, existing OTP behaviour and safeguards | AT-06, AT-07, AT-20, AT-21 |
| R4 — Simpler class selection | One course section per existing display block; dates and make-up choices easy to scan | AT-08–AT-12, AT-22 |
| R5 — Clear review and receipt | Accurate paired summaries, useful Edit actions, pending status rather than implied approval | AT-13–AT-16 |
| R6 — Safe recovery and compatibility | Preserve draft, authorization, retries, timezone, staff flow, and failure handling | AT-03, AT-05, AT-07, AT-15–AT-17, AT-20–AT-25 |

Out of scope: replacement with hosted Google Forms; Google sign-in; Google branding; new calendar integrations; absence-policy changes; OTP bypass; arbitrary phone-number replacement; new notifications; tracking; new cancellation/history flows; reason-category redesign; API/database migrations; broad React refactoring; staff-workflow redesign.

Shared components must retain their existing staff behaviour by default. Public-specific presentation changes must be explicitly opted into by the public route.

## 2. Current-system understanding

Repository root: `/Users/rd-cream/Downloads/warwick-institute-ux-documentation copy 2`.

Paths below are relative to that root. They were inspected during planning.

### 2.1 Existing execution chain

```text
src/App.tsx: /absence and /staff/absence
  → src/pages/AbsenceForm.tsx: public/staff mode, step and selection state
  → src/features/absences/api/absenceFormApi.ts
      public lookup → parent OTP send/status/verify → verified student profile
      → verified session availability → pre-submit refresh → batch submission
  → backend/internal/httpapi/absenceshttp/self_service_routes.go
      server derives student identity from verified session
  → backend/internal/httpapi/absenceshttp/batch_routes.go
      idempotent transaction → canonical pending records
      → consume/revoke verification → queue notifications
  → response.items → grouped receipt → clear recoverable draft
```

The public batch endpoint rejects client-supplied student identity and verification-token fields. `submitAbsenceBatch` sends existing payload fields and `Idempotency-Key`; preserve that contract. Batch creation and verification consumption are transactional. UI changes must not simulate partial success or produce a success receipt before an authoritative response.

### 2.2 Existing responsibilities

- `AbsenceForm.tsx`: `handleLookup`, lookup request sequencing, `togglePickerEntry`, `handleSessionGroupToggle`, make-up priority/history state, `validateClasses`, `handleSubmitAbsence`, step navigation, grouped review and receipt.
- `public-form/AbsenceAppShell.tsx` and `src/index.css`: header/main/footer grid, independently scrolling main region, visual viewport and keyboard handling, safe areas.
- `AbsenceAppHeader.tsx` / `StepProgress.tsx`: institute identity and public four-step/staff three-step progress.
- `SubjectRow.tsx` / `SessionDayCard.tsx`: real checkbox semantics, day grouping, disabled explanations.
- `MakeUpPicker.tsx`: desktop searchable select; mobile staged radio choices in `MobileBottomSheet.tsx`.
- `StepCoverVerification.tsx`, `OtpInput.tsx`, `SmsSendButton.tsx`: OTP delivery states, cooldowns, paste/autofill, verification, phone enrollment when no phone exists, session restoration.
- `useAbsenceDraft.ts` / `absenceDraftStorage.ts`: best-effort debounced **sessionStorage** snapshots, schema validation, pagehide flush, clearing on success. A comment mentioning localStorage does not change the actual storage boundary.
- `studentResumeStorage.ts`: limited same-tab lookup resume data and student-session hint.
- `domain/submissionPayload.ts` and existing scheduling/display helpers: payload mapping, merged selections, grouping and rule enforcement. Preserve these owners.
- `src/utils/date.ts`: established institute-timezone formatting, `Asia/Bangkok`.

### 2.3 Observed problems and evidence limits

The entry has both Search and a separate disabled Continue. The class step repeats course identification across the chooser, mobile selected-course controls, and session-block headers. Make-up controls are nested inside selected date cards; desktop labels concatenate several details. The reason has a visual character-progress bar. The shell footer uses a wider alignment container than the form content. Verification accepts configured contact details but does not render those details as actionable help.

The entry screen was inspected in the local preview. The backend was unavailable, so authenticated later steps were examined through source and existing acceptance fixtures. Existing screenshot artifacts predate the current shell and must not be used as the redesign baseline. No application tests were executed as part of writing this plan; task 0 establishes the implementation baseline.

## 3. Behavioural and visual contract

### 3.1 Reference fidelity: what “similar/same” means

Use **Google Forms as the dominant reference throughout**. Familiarity must come from consistent structure and conventional interactions, not a mixture of unrelated application styles.

| Surface | Reference pattern | Required adaptation |
| --- | --- | --- |
| Page and questions | Google Forms respondent view | Neutral background, centered column, white question sections, readable labels, clear required fields, consistent action placement |
| Dates and make-up times | Google Calendar appointment booking | Date/time first, grouped available choices, visibly unavailable choices with reasons, no free-form booking dates |
| Verification | Conventional SMS verification | Masked destination, one real OTP input, familiar send/resend controls, recovery near the issue |
| Review and receipt | Consumer booking summaries; Airbnb request-status clarity | Pair each missed class with its make-up arrangement; distinguish submitted/pending from approved |

Reference sources, checked during planning:

- [Google Forms response validation and Next behaviour](https://support.google.com/docs/answer/15473134?hl=en): official behavioural reference. Our decision to show actionable validation on attempted continuation is an intentional accessibility adaptation.
- [Google Forms conditional sections](https://support.google.com/docs/answer/141062?hl=en): supports the concept of revealing only relevant sections.
- [Google Forms autosave](https://support.google.com/docs/answer/10952360?hl=en): its account-backed saving is **not** our persistence contract. Do not copy its cross-device or 30-day promises.
- [Google Calendar appointment schedules](https://support.google.com/calendar/answer/11608416?hl=en): available times and conflict avoidance are the relevant scheduling reference.
- [Airbnb reservation status](https://www.airbnb.com/help/article/234): status distinguishes requests from confirmed outcomes; no Airbnb response-time promise transfers here.
- [W3C form notifications](https://www.w3.org/WAI/tutorials/forms/notifications/) and [WCAG status messages](https://www.w3.org/WAI/WCAG21/Understanding/status-messages): labels, associated errors, and programmatically available status feedback.

These sources support interaction principles. The dimensions below are project design decisions, not measurements of those products. Do not claim a pixel-identical clone.

### 3.2 Visual specifications

| Property | Final specification |
| --- | --- |
| Column | Maximum 720px; same width for header inner content, form, action bar, and receipt |
| Page padding | 16px below 640px; 24px at and above 640px; preserve safe-area padding |
| Question sections | White, 1px existing border token, 12px radius, 16px internal padding on mobile / 24px desktop |
| Separation | 24px between sections; 16px between field groups; 8px between label and control |
| Typography | Existing system font; h1 24px/32px mobile and 28px/36px desktop; section heading 18px/26px; labels 16px/24px; supporting text 14px/20px |
| Inputs/actions | Input text 16px; standard controls at least 48px tall; visible labels; interactive targets at least 44×44px |
| Colour | Existing Warwick background/text/primary tokens; use colour with text/check/radio cues; no new global palette |
| Decoration | Flat sections, restrained borders, no default card shadows, no gradients, no oversized illustrations |
| Selection | Existing primary tint plus checked control; never indicate selection only through colour |
| Motion | Existing reduced-motion support; use existing timing tokens; no shaking or decorative motion added |
| Contrast | Text at least 4.5:1, qualifying large text at least 3:1; control boundaries and focus indicators at least 3:1 against adjacent colour |

Use CSS variables scoped to `.absence-app-shell` and the receipt wrapper for any new sizing/spacing tokens. Do not change global typography/colour tokens to satisfy this route. Muted and warning text must be measured; existing tokens are not automatic proof of sufficient contrast.

On mobile, show `Step 2 of 4 · Parent verification` plus one quiet progress indicator. Remove the duplicate row of numbered circles at phone widths. At desktop widths retain readable step labels and completed-step navigation; future steps cannot be activated. Progress appearance must not misrepresent OTP delivery as verification completion.

Preserve the header/main/footer grid and visual viewport integration. The footer remains outside the scrolling main region, with one primary action and Back where applicable. Do not replace this with a viewport-fixed overlay. Align its content with the form. Use full remaining width for the mobile primary action; keep a smaller secondary Back target.

### 3.3 Screen behaviour and copy

**Student ID.** Title: “Report an absence”. Intro: “Enter your Student ID to get started.” Label: “Student ID”; supporting text: “Your Warwick W-Code, for example W250389.” The public footer starts with “Find student”. Enter in the ID field invokes the same guarded lookup. While pending: “Finding student…” and repeat actions disabled. A successful result retains only the server-provided privacy-safe hints; email appears only if the server requires it. The footer becomes “Continue”. Missing/invalid email is explained at its field when continuation is attempted; loading and identity mismatch remain hard blockers. Keep the existing staff lookup interaction unchanged.

Changing the **normalized** ID away from the successful lookup ID immediately makes that lookup unusable for continuation. Advance the request sequence on such edits so a late response cannot reinstate stale identity. Clear local verification capability/hints and hide previous private profile data; do not invent server logout endpoints. Do not clear a matching recoverable draft on mere typing: keep the existing deliberate lookup boundary for committing a different student. Changed identity cannot reuse earlier student selections, phone details, email, reason, or capability.

**Parent verification.** Title: “Verify with your parent”. Supporting text explains that a code is sent to the parent's phone and entered here. Show the masked server destination, not a client-unmasked number. Keep the single real numeric input with six visual cells, paste, `one-time-code` autofill, server-controlled delivery states, resend cooldown and automatic progression on successful fresh verification. A restored verified session retains the existing Continue behaviour. No automatic SMS send on navigation. When no phone exists, retain the current enrollment-and-OTP path and explain that the verified number will be saved. When SMS is disabled or unrecoverable, show configured Call/Email help and hours without implying a bypass or guaranteed response time.

**Choose classes.** Title: “Choose classes to miss”. Intro: “Select your course and the class dates you will miss.” Replace the desktop course/work columns and duplicate mobile selected-course list with a single stack of course sections. Reuse the existing combined picker entries and session-block grouping: one section per existing display block, not one per raw subject ID. Each header contains the course checkbox and a distinct expand/collapse button; never nest a button inside the checkbox label. Selecting a course opens it. At most one selected course is expanded at a time; collapsed selected headers show their actual selected-day count. Expanding does not change selection. Deselected courses are excluded from counts, review and payloads through existing authoritative selection derivation.

Date rows use real checkboxes with date first, time on a second line, and a visible disabled reason when applicable. Preserve whole-row activation without capturing clicks on its make-up action. Break long course/teacher names rather than truncating essential information. Show the course's remaining absence allowance locally and a total expressed as `2 class days selected`; merged occurrences still count as existing logical days.

Selected dates reveal one compact make-up row: `Make-up: [date/time or arrangement]` and `Choose` / `Change`. Physical make-up choices remain required exactly where the existing rules require them. Zoom and teacher-arranged paths are explanatory arrangements, not fake selectable time slots. Existing “See other times” and “Previous times” remain subordinate actions governed by current priority rules.

**Make-up dialog.** Public mode uses the existing `MobileBottomSheet` at all widths; its CSS already makes it a centered dialog on desktop. Introduce an optional `presentation` prop on `MakeUpPicker`: default preserves existing responsive behaviour; public route opts into dialog presentation. Use structured real radio rows with date, time, course and supporting details separately. Pass optional missed-class context for the dialog description so the family knows which absence it is arranging. Show search only when the current option list has more than eight entries; search does not fetch or reveal additional priority levels. Preserve disabled conflict reasons. Open with current committed value staged; Confirm commits once, Cancel/Escape/backdrop commits nothing. Provide the existing explicit empty selection using the clearer label “Clear selection”; it does not satisfy required physical make-up validation. Confirmation is disabled if a staged nonempty option has vanished or become disabled. Close restores focus to its trigger. Remove public reliance on the desktop combobox without adding a hidden mirror control solely for old tests.

**Reason.** Keep the required reason in the classes step and its existing 500-character boundary. Label: “Reason for absence”; helper: “Briefly tell us why you will miss these classes.” Remove the character-progress bar. Keep a quiet count and associated inline error; do not announce every keystroke as a status change. No invented reason chips or new categories.

**Review.** Title: “Review and submit”. Show verified identity, each missed date paired with its make-up arrangement, and reason. Keep optional nickname entry only when `nickname_set === false`, behind a clearly labelled optional details disclosure; retain its masked echo and nonblocking submission fallback. “Edit classes” returns to and reveals the relevant selected course; “Edit reason” focuses the reason field. Both preserve entered choices. Primary: “Submit absence”. Helper: “Your request will be reviewed by staff.”

**Receipt.** Reuse the same width, section styling and typography. Title: “Absence submitted” or the existing plural equivalent. Status: “Awaiting staff review.” Keep the existing reference, returned class grouping and accurate make-up descriptions. Display each missed class beside its make-up arrangement. Do not label submission as approval, promise notification delivery, or add a new booking reference format. Focus the receipt heading after successful response. Clear existing drafts only at the existing successful-submission boundary.

### 3.4 Wireframes

```text
Warwick Institute                           Report an absence
Step 3 of 4 · Choose classes                 [quiet progress]

Choose classes to miss
Select your course and the class dates you will miss.

┌─────────────────────────────────────────────────────────┐
│ ☑ Mathematics · Teacher name           Hide dates        │
│ 1 class day selected · 2 absence days remaining           │
│                                                         │
│ ☑ Fri, 16 Oct 2026                                       │
│   16:00–17:00                                           │
│   Make-up: Tue, 20 Oct · 17:00–18:00          Change      │
│                                                         │
│ ☐ Fri, 23 Oct 2026                                       │
│   16:00–17:00                                           │
└─────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────┐
│ ☐ Physics · Teacher name                                │
└─────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────┐
│ Reason for absence · Required                           │
│ [Multiline text field]                    0/500          │
└─────────────────────────────────────────────────────────┘

1 class day selected
Back                                         Review absence
```

```text
Choose a make-up class                                  Close
For Mathematics · Fri, 16 Oct · 16:00–17:00

○ Tue, 20 Oct 2026
  17:00–18:00 · Mathematics · Teacher name
○ Thu, 22 Oct 2026
  16:00–17:00 · Mathematics · Teacher name
○ Fri, 23 Oct 2026                            Unavailable
  Overlaps another selected class
○ Clear selection

Cancel                              Confirm make-up class
```

Dates/allowances in these wireframes are illustrations, not fixtures or hardcoded production content. Always render authoritative values through existing date/grouping helpers.

### 3.5 Invariants

1. Private identity/course/session information appears only after the existing verified-session boundary.
2. Public and staff identity, endpoints, capabilities and drafts remain separate.
3. Existing course grouping, priority progression, ownership, quotas, overlap and already-absent rules remain authoritative.
4. A selected physical arrangement is valid only if still available; browser styling never makes a disabled choice bookable.
5. Existing selection state owns the request. Counts, summaries, validation and payloads derive from it; no parallel form store.
6. Existing idempotency and retry logic are retained. Do not change a key merely to rerender, reopen a dialog, or retry an uncertain request. Preserve the deliberate existing nickname-fallback exception.
7. No success, approval, SMS-delivery or cross-device-save claim is inferred from client activity.
8. Required reason, expiry checks, pre-submit refresh, focus management, safe areas and keyboard support survive the redesign.

## 4. Acceptance scenarios

Each row is Given/When/Then acceptance behaviour. `Component` means React/Testing Library; `E2E` means real-browser UI with existing route fixtures unless explicitly marked live.

| ID | Given / When | Then | Planned verification |
| --- | --- | --- | --- |
| AT-01 | Open public form at phone/tablet/desktop widths | Same column alignment, defined typography/sections, clear step label and one primary action; no horizontal overflow | E2E, screenshots, measured visual review |
| AT-02 | Blank or malformed ID / Find student or Enter | Associated error; no lookup request, no step advance; entered value retained | Student page tests, critical-path E2E |
| AT-03 | Lookup pending / edit ID, including old response resolving last | Old response cannot restore identity/continue; no duplicate request for same in-flight action | Student race tests and E2E |
| AT-04 | Successful lookup / required email missing or invalid / Continue | Inline actionable email error, focus email, stay at entry; saved-email path asks for no redundant email | Student tests and critical-path E2E |
| AT-05 | Student A found / normalized ID changes to B | Continue cannot use A; private data and A capability hidden/cleared; valid B lookup owns later selections | Student tests, error-recovery E2E |
| AT-06 | Existing phone / explicitly send code / paste six digits or autofill | Server-masked destination, delivery feedback, cooldown; fresh success advances once; wrong code remains recoverable | Existing OTP/SMS/verification component tests, E2E |
| AT-07 | Missing phone, SMS disabled, or expired/invalid verification | Enrollment only on permitted path; no bypass; actionable configured help; expiry returns to verification without losing recoverable choices | Student/verification tests and error-recovery E2E |
| AT-08 | Multiple course blocks / select, collapse, reopen and deselect | One section per existing display block; expansion independent of selection; excluded courses absent from review/count/payload | Page integration tests, accessibility E2E |
| AT-09 | Grouped/merged class occurrences / choose a date | Existing complete session-ID group selected; count represents logical days; review and payload agree | SessionLimit/finalSession page tests, E2E |
| AT-10 | Already-absent, exhausted allowance or conflict / inspect or try selection | Visible specific reason; real disabled control; cannot enter payload; valid selected date remains deselectable | Existing page and SessionDayCard tests, E2E |
| AT-11 | Open dialog with existing selection / stage another / cancel or Escape / reopen | Committed value unchanged; current value staged on reopen; focus restored; Confirm commits exactly once | MakeUpPicker/MobileBottomSheet tests, E2E |
| AT-12 | Option becomes invalid while dialog open, search no matches, or clear selection | Invalid pending choice cannot commit; clear search recovers; explicit clear empties selection and required physical validation still blocks review | Picker tests, error-recovery E2E |
| AT-13 | Missing physical make-up or blank reason / Review absence | Correct expanded course and visible picker or reason receives focus; error associated; no review advance; reason max 500 | Error-recovery E2E, ReasonField tests |
| AT-14 | Review / Edit classes or Edit reason / return to review | Target course/field revealed and focused; selections/reason unchanged; summary matches current values | Critical-path E2E and page tests |
| AT-15 | Valid request / rapid duplicate submit or transport retry | Existing key/request semantics retained; one logical batch; wait UI; receipt only for authoritative success | Existing duplicate/idempotency E2E; live controlled test for actual record count |
| AT-16 | Successful returned batch / receipt | Reference retained, all returned grouped classes shown, paired arrangements, awaiting-review status, heading focus and draft clearing | Critical-path E2E and page tests |
| AT-17 | Refresh in same tab, corrupt/blocked storage, or stale saved option | Existing validated restore; fresh authorization needed; invalid choices discarded; in-memory flow still usable; no cross-device-save claim | Student/storage tests, resilience E2E |
| AT-18 | Keyboard and screen reader / every step and dialog | Logical focus order; native labels/checkboxes/radios; dialog isolation/Escape/focus return; current step and errors announced without noise | axe E2E plus manual keyboard and VoiceOver |
| AT-19 | 320px width, landscape, keyboard open, 200% zoom or reduced motion | Controls reachable; no footer overlap or clipped dialog action; long names wrap; focus visible; unnecessary motion absent | Existing viewport/visualViewport helpers and manual device checks |
| AT-20 | API failure, verification-check failure, uncertain SMS delivery, or network event | Existing retry states preserve input; no false sent/verified/success; navigator offline event alone does not introduce a new global block | Verification/page tests; unchanged resilience semantics |
| AT-21 | Before verification / inspect DOM/network; submit unauthorized | No new private data; OTP capabilities not added to URLs; no staff/lifetime options sent on student requests | Existing URL/API/ownership tests plus network review |
| AT-22 | Physical priority ladder, Zoom, teacher case or no current bookable times | Existing priority boundaries respected; choose only permitted physical times; nonphysical paths correctly described; unresolved physical requirements not waived | Page/finalSession tests and multi-rule fixture E2E |
| AT-23 | Optional nickname present, absent or rejected due to concurrent authoritative update | Existing conditional collection/echo/fallback; nickname cannot block otherwise valid absence | Page tests and submit API regression |
| AT-24 | Student browser timezone differs from institute / same fixture | Dates/times remain institute-correct; selection, dialog, review and receipt agree | Existing absence-timezone E2E |
| AT-25 | Staff route or modal uses shared components / full staff submission | Existing staff lookup, three-step flow, responsive picker default, staff endpoint and draft isolation preserved | Staff page/modal tests and staff browser smoke |
| AT-26 | User reviews all rendered states against section 3 | Main form consistently resembles Google Forms; structured time selection consistently uses one booking pattern; no unrelated dashboard/chat UI | Manual reference checklist and full screenshot set |

## 5. Change-surface and dependency map

```text
Acceptance examples + current baseline
  → public-only presentation contract
  → shared shell/action semantics + guarded lookup
  → course sections + dialog picker + reason
  → review editing + receipt
  → fixture/helper alignment + full acceptance verification

Selection state → existing validation → existing payload builder
  → existing verified APIs/transaction → existing returned items → receipt
```

| Boundary | Classification | Required action |
| --- | --- | --- |
| Public route and step labels | CHANGE | Refine copy and interaction in existing owner |
| Shared shell/header/progress/action bar | CHANGE | Scoped styling; optional props with compatible defaults |
| Public course/date/make-up/reason presentation | CHANGE | Remove duplicate layouts; reuse grouping and native controls |
| Optional nickname and receipt presentation | CHANGE | Keep data contract; improve hierarchy |
| Public dialog presentation prop and relevant tests | ADD | Small presentation option; no new booking/state service |
| Duplicate selected-course list / public desktop concatenated select / reason bar | REMOVE | Remove superseded presentation only |
| Draft/storage/selection/payload/domain/formatting | VERIFY | Preserve canonical state, schema, grouping, cleanup and timezone |
| Verified APIs/auth/transaction/idempotency/notifications | VERIFY | Contract already exists; no planned server edits |
| Database/generated contracts/migrations | VERIFY | Existing fields suffice; no data transformation required |
| New dependencies, analytics, jobs, feature flags | N/A | No requirement justifies adding them |

If implementation discovers that a required behaviour needs an API/policy change, stop that change, document the concrete conflict, and revise scope. Do not silently broaden this UX plan into backend work.

## 6. Design decisions

| Decision | Reason / alternatives rejected | Invariant protected |
| --- | --- | --- |
| Google Forms dominates visual language | Recognizable question/answer structure; an Airbnb-heavy booking design adds imagery and checkout concepts unrelated to school absence | R1 and task clarity |
| Four existing steps, reason remains in classes | Avoid draft/state migration and keep complete validation before review; one-question-per-screen Typeform-style flow multiplies navigation for multi-class requests | State/schema and review correctness |
| One course stack, one open selected course | Removes current duplication and dense side-by-side layout; no additional nested substeps | Native control semantics and grouping |
| Existing dialog at every public width | Same structured choices and staged Confirm/Cancel semantics; avoid custom popover/listbox or calendar implementation | Input parity and focus lifecycle |
| Guard lookup at identity boundary | Every public continuation crosses the same owner; no duplicated identity checks in each child | Authorization and stale-response safety |
| Derived summaries, no new global state | Existing state already owns all request fields; local state only for open/disclosure/query/pending choice | Counts/payload consistency |
| Keep current API-driven recovery | Browser online/offline events are unreliable and existing resilience tests intentionally avoid global blocking | Failure semantics and compatibility |
| No “Draft saved” badge in this release | Storage is best-effort and its hook does not report persistence success; a truthful badge would require a separate persistence-result contract | Privacy and truthful feedback |

## 7. Detailed implementation tasks

### Phase 0 — Baseline and acceptance preparation

**Depends on:** this document. **Enables:** all subsequent phases.

- [ ] **0.1 Establish baseline.** Record working-tree status and run the existing typecheck, focused absence tests, and critical-path/error-recovery browser tests before editing. Save actual results; distinguish pre-existing failures from introduced failures. Do not claim later-step live verification if only fixtures are available.
- [ ] **0.2 Record current state inventory.** Using existing `e2e/fixtures/absence.ts`, capture entry, lookup success with/without email, verification send/code/error, courses with multiple blocks, physical picker, review and receipt at 390px and 1440px. Include long names and disabled choices. Save under `artifacts/absence-public-ux/`; these are fresh evidence, not design assets.
- [ ] **0.3 Define acceptance test deltas.** Add failing behaviour tests for public one-action lookup, identity edits during a pending lookup, unified course expansion, dialog selection on desktop, edit-target focus and actionable help. Extend existing suites listed in section 8. Do not assert that a label change alone proves the workflow.
- [ ] **0.4 Verify fixture coverage.** Existing fixtures must distinguish unavailable physical slots from Zoom/teacher arrangement, merged session groups, optional nickname and independent course allowances. Extend fixtures only where a scenario lacks data. Use supplied availability and dates; no production-hardcoded values.

**Gate:** baseline recorded; each new test failure corresponds to a specified new behaviour, not broken fixture setup. Covers AT-01–AT-26.

### Phase 1 — Shared presentation foundation

**Depends on:** 0.1–0.4. **Enables:** phases 2–5.

- [ ] **1.1 Scope layout tokens.** In `src/index.css`, introduce only absence-specific sizing/section rules needed for section 3.2. Align header/main/footer/receipt to 720px. Keep the existing grid, visual-viewport styles, safe areas and scroll owner. Do not change body or global theme tokens.
- [ ] **1.2 Simplify public progress.** In `AbsenceAppHeader.tsx` / `StepProgress.tsx`, add an optional public presentation variant; compact phone step text/indicator, readable desktop labels, backwards navigation only. Keep staff's default three-step behaviour and accessible current-step announcements.
- [ ] **1.3 Refine action bar.** In `AbsenceActionBar.tsx`, allow an optional busy label and concise description supplied by the owner; associate blocking/context help with the action. Keep compatible default `Submitting…`. Apply broad mobile primary button without losing Back target or keyboard reachability.
- [ ] **1.4 Apply readable hierarchy.** Update existing step wrappers to use specified typography and question-section spacing, without shadow-heavy nested cards. Confirm focus outlines remain visible and are not removed to make screenshots cleaner.

**Verify:** shell/progress component tests, measured column alignment, 320px/landscape/keyboard helper checks. AT-01, AT-18, AT-19, AT-25, AT-26.

### Phase 2 — Public entry and verification

**Depends on:** phase 1.

- [ ] **2.1 Consolidate entry action.** In `AbsenceForm.tsx`, derive public action mode from current lookup identity and lookup status: Find student → Finding student… → Continue. Remove the public inline Search button. Keep staff Search/Continue path intact. Route Enter through the same guarded lookup and block repeat pending actions.
- [ ] **2.2 Bind lookup to the edited ID.** At the existing ID change/lookup owner, normalize input, invalidate obsolete lookup completion, clear local verification capability/hints for changed identity and hide obsolete private state. Settle the obsolete lookup's loading indicator so an edit cannot leave the new ID permanently blocked. Preserve existing request-ID sequencing, matching-draft lookup boundary and student-change resets. A normalized equivalent edit must not unnecessarily destroy a valid lookup.
- [ ] **2.3 Make email validation actionable.** On attempted public Continue, verify the current lookup matches the current ID and show/focus the required email error if invalid. Keep server-authoritative saved-email behaviour. Use proper label, helper/error IDs and `aria-invalid`; do not show an invalid error merely on first empty render.
- [ ] **2.4 Present verification help.** Consume the already passed `adminContact` in `StepCoverVerification.tsx`; render configured phone/email/hours on blocked/unavailable verification paths. Use React text rendering and safe `tel:` / `mailto:` targets validated for the intended scheme; empty/unusable contact values must not create broken links. Keep no-phone enrollment separate from saved-phone identity.
- [ ] **2.5 Preserve verification mechanics.** Keep existing send/status/verify endpoints, cooldowns, paste/autofill, delivery uncertainty, restored-session continuation, expiry and fresh-success auto-advance. Do not add navigator offline blocking or automatically send SMS. Run the OTP/SMS/verification suites after presentation edits.

**Verify:** public entry integration/race tests; wrong-code/disabled-SMS/missing-phone/restored-session browser cases; inspect DOM/network pre-verification. AT-02–AT-07, AT-20, AT-21, AT-25.

### Phase 3 — Course and date hierarchy

**Depends on:** phases 1–2.

- [ ] **3.1 Compose the unified course stack.** Replace only the classes-step presentation in `AbsenceForm.tsx`. Remove the subjects/work columns and mobile selected-course duplicate controls. Reuse existing picker entries, selected blocks, grouping helpers and `expandedSubjectId`. Do not rebuild these from course names or create a second selected-day collection.
- [ ] **3.2 Separate selection and expansion.** Reuse `SubjectRow` checkbox semantics and render the disclosure button as a sibling control. Selected headers show logical-day count and local allowance. Selection opens the appropriate existing block; disclosure changes only expansion. Long teacher/course labels wrap. Keep stable course/checkbox IDs or update consumers explicitly.
- [ ] **3.3 Simplify date rows.** In `SessionDayCard.tsx`, render separate date/time lines and compact arrangement content. Use the existing day-group ID and all underlying session IDs. Keep disabled explanations, enabled deselection of selected dates, correct label activation and visible focus.
- [ ] **3.4 Preserve arrangement eligibility.** Keep existing physical priority/history calculations, “See other times” / “Previous times”, Zoom, teacher-case and unresolved-arrangement branches. Replace presentation only. Add fixture checks that merging and nonphysical arrangements do not accidentally demand a physical booking.
- [ ] **3.5 Simplify reason.** In `ReasonField.tsx`, remove the progress bar and retain required semantics, maxLength, character count, error association and controlled value. Keep the reason in this step; do not invent a config-controlled optional reason contrary to current server policy.
- [ ] **3.6 Reveal validation targets.** In `validateClasses` / existing focus handling, resolve the specific invalid selected block, expand it, then focus the now-visible checkbox/picker on the committed render. Do not focus a hidden desktop select. Preserve error summary focus where relevant and make “Edit reason” target the actual textarea.

**Verify:** course toggling/exclusion, grouped-day payloads, independent quotas, disabled/already-absent rows, long-name wrapping and missing-field focus. AT-08–AT-10, AT-13, AT-18, AT-19, AT-22, AT-25.

### Phase 4 — Public structured make-up dialog

**Depends on:** phase 3 and existing shared dialog baseline.

- [ ] **4.1 Add public presentation opt-in.** In `MakeUpPicker.tsx`, add the optional presentation prop with current responsive behaviour as default. Public callers opt into a visible choose/change trigger and existing dialog at all widths. Shared/staff callers retain their default behaviour.
- [ ] **4.2 Render structured options.** Extend `MakeUpOption` with optional `dateLabel`, `timeLabel` and `teacherLabel` strings; retain existing `label` as the class label and existing `details` for compatible default rendering. Populate them in `AbsenceForm.tsx: makeUpPickerOptions`: use `formatDate(groupByDay(optionGroup.items)[0].date)` when that group exists, join the existing normalized display model's `startTime`/`endTime`, and reuse its `teacherName`. The existing `normalizeSitInDisplayModel` already resolves merged time ranges and teachers: do not parse concatenated display strings or change that shared helper. Public dialog rows render these fields separately, with fallback to existing label/details for legacy options; do not repeat combined details when structured fields are present. Add optional missed-class `context` text to the picker, rendered as its dialog description. Preserve option values, descriptions/conflict reasons, disabled status and booking logic. No API DTO change.
- [ ] **4.3 Keep local staging authoritative.** Initialize pending choice from committed value on each open. Cancel/Escape/backdrop closes without mutation. Confirm checks pending membership and disabled status against the latest options before calling the existing change handler once. Preserve clearing a now-disabled committed selection through the existing owner callback.
- [ ] **4.4 Search current options only.** Render search above eight options; reset query on open, keep filtering local, present no-match feedback and clear-search action. Keep unavailable-choice explanation and explicit Clear selection understandable. Search cannot reveal next-priority data.
- [ ] **4.5 Verify modal reachability.** Reuse `MobileBottomSheet` semantics, initial focus and trigger restoration. Change its scroll layout only if needed to keep Cancel/Confirm reachable in short landscape and keyboard-open states; default callers must remain compatible. At phone widths it is a bottom sheet; at desktop it is a centered bounded dialog.

**Verify:** real-browser desktop and mobile radio selection, cancellation, stale options, search recovery, keyboard focus/Escape and action reachability. AT-11, AT-12, AT-13, AT-18, AT-19, AT-22, AT-25.

### Phase 5 — Review, receipt and meaningful failure feedback

**Depends on:** phases 2–4.

- [ ] **5.1 Render paired summaries.** In existing review rendering, pair each missed logical class day with its current arrangement and exact institute-formatted dates/times. Keep current selected-block filtering, reason and authoritative identity. Do not use pending dialog values in review.
- [ ] **5.2 Target review edits.** Add only ephemeral navigation intent needed for Edit classes/Edit reason. Consume that intent after the target renders, reveal the target selected block or focus the textarea, and retain canonical values. Do not persist DOM IDs or focus intent in the draft schema.
- [ ] **5.3 Reduce optional-detail distraction.** Place the conditional nickname input in an optional details disclosure. Keep its current maximum length, masked echo, submission inclusion condition and server-rejection fallback.
- [ ] **5.4 Unify the receipt.** Style the existing `finalResults` branch with the same column/sections; retain reference and grouping. Show pending status and paired make-up arrangements. Preserve result-heading focus and success-only draft cleanup. No new receipt-fetch endpoint or reload-persistence promise.
- [ ] **5.5 Preserve recovery semantics.** Keep pre-submit availability refresh, quota error, stale/conflicting make-up return to classes, expired-session return to verification and uncertain-network message. Describe actionable next steps without false approval/delivery claims. Do not silently change the existing breadth of conflict-related selection clearing as part of visual work.

**Verify:** edit preservation/focus, receipt from returned results only, duplicate taps, uncertainty, quota rejection, stale make-up, nickname race and timezone. AT-14–AT-17, AT-20–AT-24, AT-25.

### Phase 6 — Integration and release evidence

**Depends on:** phases 1–5; no gate may be bypassed by changing test assertions to match broken behaviour.

- [ ] **6.1 Align UI consumers.** Update `e2e/helpers/absenceFlow.ts` and local helpers in absence specs to public Find student/Continue and dialog selection at all public widths. Remove public expectations of a desktop combobox; keep staff/default-picker coverage. Update progress labels, course-layout assertions, receipt headings and existing fixtures only where the intended contract changed.
- [ ] **6.2 Execute the acceptance matrix.** Run commands in section 12; record pass/fail and link each AT ID to the exact automated test or manual evidence. Investigate new failures before broadening tests or changing unrelated code.
- [ ] **6.3 Produce final visual evidence.** Capture all states from task 0 plus no-phone, SMS-disabled, exhausted allowance, no-match search, missing reason, quota rejection and uncertain submission. Use mobile/desktop and short-landscape dialog captures. Review against the visual rubric, not only a single happy-path screenshot.
- [ ] **6.4 Manual accessibility and familiarity checks.** Execute section 12's keyboard, VoiceOver, real-device and reference checklists. Record limitations explicitly. Do not equate axe passing with accessible completion.
- [ ] **6.5 Confirm unchanged boundaries.** Review diff and intercepted payloads for no new auth fields, storage scope, URLs, API endpoints, migrations, global tokens or staff-only options. Exercise one controlled staging request and duplicate retry with real backend verification, without using a real family's phone or sending live SMS without explicit authorization.
- [ ] **6.6 Publish completion evidence.** Attach test results and screenshots to the implementation review. If a live environment or device is unavailable, mark the corresponding release gate unverified; mocked browser tests cannot substitute for those gates.

**Gate:** section 15 complete, with no unresolved material failure. AT-01–AT-26.

## 8. File-by-file change plan

| Existing file | Planned change / protected responsibility | AT IDs |
| --- | --- | --- |
| `src/pages/AbsenceForm.tsx` | Public action state, identity edit boundary, course stack, validation focus, public dialog opt-in, review edit targets, optional nickname disclosure, receipt; preserve domain/submission owners | 02–17, 20–25 |
| `src/components/absences/public-form/AbsenceAppShell.tsx` | Small wrapper/layout adjustments only if needed; preserve viewport/scroll lifecycle | 01, 18, 19, 25 |
| `src/components/absences/public-form/AbsenceAppHeader.tsx` | Public presentation variant and aligned identity/progress | 01, 18, 25, 26 |
| `src/components/absences/public-form/StepProgress.tsx` | Public phone indicator, desktop labels/current/backward navigation | 01, 18, 19, 25 |
| `src/components/absences/public-form/AbsenceActionBar.tsx` | Optional busy/help props, public mobile action sizing; compatible defaults | 01–04, 15, 18, 19, 25 |
| `src/components/absences/public-form/StudentStep.tsx` | Entry title and readable hierarchy | 01, 02, 26 |
| `src/components/absences/public-form/VerificationStep.tsx` | Verification title and simplified masked context | 06, 07, 26 |
| `src/components/absences/StepCoverVerification.tsx` | Render safe configured help; refine copy only; preserve verification mechanics | 06, 07, 20, 21 |
| `src/components/absences/public-form/ClassesStep.tsx` | Clear step question/title | 08, 26 |
| `src/components/absences/public-form/SubjectRow.tsx` | Reuse checkbox header semantics; optional presentation extension only if necessary | 08, 18, 25 |
| `src/components/absences/public-form/SessionDayCard.tsx` | Date/time hierarchy and compact arrangement; retain selection/disabled semantics | 09, 10, 18, 19, 24 |
| `src/components/absences/public-form/MakeUpPicker.tsx` | Optional dialog presentation/context, optional date/time/teacher display fields, structured rows, staging/availability and search rules | 11–13, 18, 19, 22, 25 |
| `src/components/absences/public-form/MobileBottomSheet.tsx` | Reuse; change scrolling/action containment only where demonstrated necessary | 11, 18, 19, 25 |
| `src/components/absences/public-form/ReasonField.tsx` | Remove progress decoration; keep count/limit/associated error | 13, 18, 19 |
| `src/components/absences/public-form/ReviewStep.tsx` | Review title/hierarchy | 14, 26 |
| `src/index.css` | Absence-scoped layout, sections, receipt and adaptive dialog; no global theme changes | 01, 18, 19, 25, 26 |
| `src/pages/__tests__/AbsenceForm.student.test.tsx` | Entry/action/race/identity/email/restore acceptance deltas | 02–05, 07, 17, 21 |
| `src/pages/__tests__/AbsenceForm.test.tsx` | Course/review/receipt interaction integration | 08–16, 22, 23 |
| `src/pages/__tests__/AbsenceForm.errorHandling.test.tsx` | Actionable failure/validation regression | 07, 13, 20 |
| `src/components/absences/__tests__/StepCoverVerification.test.tsx` | Help/rendering and preserved verification paths | 06, 07, 20 |
| `src/components/absences/public-form/__tests__/StepProgress.test.tsx` | Public variant and default compatibility | 01, 18, 25 |
| `src/components/absences/public-form/__tests__/SubjectRow.test.tsx` | Native header checkbox/label behaviour | 08, 18 |
| `src/components/absences/public-form/__tests__/SessionDayCard.test.tsx` | Date row selection/disabled regression | 09, 10 |
| `src/components/absences/public-form/__tests__/MakeUpPicker.test.tsx` | Dialog opt-in, staged confirmation/cancel, option invalidation/search | 11, 12, 22, 25 |
| `src/components/absences/public-form/__tests__/MobileBottomSheet.test.tsx` | Modal lifecycle regression | 11, 18, 19 |
| `src/components/absences/public-form/__tests__/ReasonField.test.tsx` | Required/limit/error semantics | 13, 18 |
| `e2e/fixtures/absence.ts` | Extend only missing scenario data | Relevant acceptance IDs |
| `e2e/helpers/absenceFlow.ts` | Public action/dialog flow; assert selected outcome rather than hidden control | 02–16 |
| `e2e/helpers/absenceAssertions.ts` | New single-column/public progress assertions; retain overflow/focus/viewport checks | 01, 18, 19 |
| `e2e/absence-form-critical-path.spec.ts` | Public lookup, full submission and targeted review edits | 02, 04, 14–16 |
| `e2e/absence-form-error-recovery.spec.ts` | Visible invalid target, stale identity/verification and correctable errors | 05, 07, 12, 13, 20 |
| `e2e/absence-form-accessibility.spec.ts` | Unified layout/dialog across existing viewport matrix | 01, 08, 18, 19 |
| `e2e/absence-form.spec.ts` | Update interactions; retain URLs/duplicate submission/quota regression | 10, 15, 20, 21 |
| `e2e/absence-form-resilience.spec.ts` | Preserve same-tab/viewport/no-global-offline-block contract | 17, 19, 20 |

Verify unchanged: `src/App.tsx`; `src/features/absences/api/absenceFormApi.ts`; `src/features/absences/domain/submissionPayload.ts`; `src/features/absences/domain/absenceMessageCopy.ts`; `src/features/absences/domain/sitInResolution.ts`; draft/resume storage; `src/utils/date.ts`; OTP/SMS components; public/shared types; backend self-service/batch handlers; staff modal; scheduling rules.

Expected new implementation source files: **none**. Existing component boundaries suffice. Expected deleted files: **none**; delete only superseded markup/CSS. New tests may remain in existing suites. New evidence files belong under `artifacts/absence-public-ux/`. This plan is the only file created during planning.

## 9. Data and state lifecycle

| State | Source of truth / lifecycle |
| --- | --- |
| Edited student ID | Local input → normalized validation → guarded public lookup; cannot impersonate server session identity |
| Lookup response | Latest matching request only → privacy-safe entry hint → invalidated on normalized ID mismatch |
| Verification | Existing token/cookie/session status → server validation → expiry or transaction consumption; never derived from progress UI |
| Course/session availability | Verified server response → existing grouping/rules → refresh before submission; invalid saved choices removed by existing restoration |
| Committed selection/reason | Existing `AbsenceForm` state → derived count/review/payload → best-effort same-tab draft → clear after authoritative success |
| Dialog query/pending value | Local ephemeral state → initialize on open → validate current membership → commit once or discard on cancel; never directly persisted |
| Course expansion/edit focus intent | Local ephemeral state → reveal current target → discard focus intent after navigation; no schema fields |
| Receipt | Canonical returned items → existing grouping/reference → rendered result; no new durable receipt cache |

No duplicate selected-day count state, copied server DTO store, query-cache rewrite, additional localStorage usage, OTP-in-URL field or schema upgrade. Preserve existing request idempotency-key ownership. Best-effort storage failure keeps the in-memory form usable and must not show an unverified saved indicator.

## 10. Error and edge-case matrix

| Condition | Expected behaviour and owner | AT |
| --- | --- | --- |
| Config/lookup network failure | Existing API error/retry behaviour; retain typed input; no advancement or leaked internal details added by new copy | 02, 20 |
| ID edit while request runs | Request sequencing invalidates obsolete completion; existing identity owner prevents continuation | 03, 05 |
| SMS queued/uncertain/failed/expired | Existing delivery state controls message/action; do not equate queueing with delivery | 06, 07, 20 |
| Missing/invalid contact config | No broken call/mail link; retain plain actionable explanation without fabricated contact data | 07 |
| No courses or no dates in selected block | Explain that no selectable classes are available; show configured help where useful; required review validation still applies | 08, 13, 22 |
| Absence limit reached | Course-local reason and real disabled dates; review-time rejection preserves the correctable request | 10, 20 |
| Physical selection missing | Reveal its course and focus visible choose trigger; no implicit staff-arranged substitute | 13, 22 |
| Make-up stale/conflicting | Existing refresh/conflict branch returns to classes; explain next choice; no false booked state | 12, 20, 22 |
| Dialog option disabled/removed | Cannot commit staged stale value; current options own validity | 12 |
| Reason blank/over limit | Required field/500-character boundary and focus; no request on invalid review | 13 |
| Verification expired/unauthorized | Existing verified-session recovery; private state not exposed; recoverable draft retained according to current owner | 07, 17, 21 |
| Submission uncertainty | Existing same-key retry contract and uncertainty copy; no optimistic receipt and no fresh key to hide uncertainty | 15, 20 |
| Server transaction fails | No fabricated per-item success; UI shows existing batch failure; real integration test verifies persisted outcome | 15, 20 |
| Notification queued but delivery unknown | Receipt reports submission, not SMS/email arrival | 16, 20 |
| Optional nickname races server update | Existing nonblocking fallback; no second unrelated form submission flow | 23 |
| Browser storage unavailable/corrupt | Existing safe reader/writer; usable in-memory form; restoration cannot bypass verification | 17 |
| Narrow viewport/keyboard/zoom | Existing shell keeps focused field and action reachable; dialog content scrolls within bounds | 19 |
| Browser timezone differs | Existing institute formatters used throughout; no new raw `Date` interpretation for display | 24 |

## 11. Compatibility and migration

- **No database or API migration:** only presentation and local interaction change; verified endpoints, payload shapes, transaction, response and notification fields remain sufficient.
- **Draft compatibility:** schema v1, keys, indices and same-tab sessionStorage unchanged. Old drafts must reload through existing authorization and availability validation. Expansion/disclosure state is not required for restoring submitted values.
- **Staff compatibility:** public variants are opt-in. The staff route and `StaffCreateAbsenceModal` retain existing capabilities, labels/step count where mode-specific, endpoint and separate draft key. Shared visual adjustments still require staff smoke verification.
- **Consumer/test compatibility:** expected public text and selection surface change intentionally; update browser helpers and inline selector branches explicitly. Do not preserve a hidden combobox or obsolete layout only to satisfy old assertions.
- **Global styles:** change absence-scoped rules only; no cross-app typography/palette replacement.
- **Notification/policy compatibility:** no template changes, new delivery promises, optional-reason policy, or new make-up eligibility.
- **Rollout ordering:** a frontend-only code change, although the repository deployment may package frontend/backend together. The existing backend remains contract-compatible; no deployment-order dependency is introduced.

## 12. Test strategy and commands

### 12.1 Automated acceptance and regression

From the repository root, capture the baseline before changes and run the same relevant checks after implementation:

```bash
npm run typecheck
npm run typecheck:e2e
npm run test:absence
npm run test -- src/components/absences/public-form/__tests__ src/components/absences/__tests__/StepCoverVerification.test.tsx src/components/absences/__tests__/OtpInput.test.tsx src/components/absences/__tests__/SmsSendButton.test.tsx src/components/absences/__tests__/StaffCreateAbsenceModal.test.tsx
npm run test:e2e -- e2e/absence-form-critical-path.spec.ts e2e/absence-form-error-recovery.spec.ts e2e/absence-form.spec.ts e2e/absence-form-accessibility.spec.ts e2e/absence-form-resilience.spec.ts e2e/absence-timezone.spec.ts
npm run build
npx react-doctor@latest --verbose --scope changed
```

Expected: successful compilation/build, all listed relevant tests passing, no introduced React Doctor regressions. Follow the local React Doctor skill when implementing. Diagnose baseline failures separately; no score-only claim of correctness.

For fast iteration, use the specific changed Vitest file or the `chromium-phone-standard` / `chromium-desktop` Playwright projects. The final gate uses the configured Chromium/WebKit/Firefox viewport matrix. Do not permanently reduce the matrix to make failures disappear. Playwright's existing webServer configuration builds and serves the app; route fixtures are deterministic and do not verify real SMS or server persistence.

Existing `test:absence` does not encompass every public-form/verification test directory, so retain the explicit component-test command above. Unit tests protect novel transitions and option staging; do not add tests that simply repeat CSS class strings.

Live boundary verification uses a controlled existing staging/test setup to prove student ownership, actual duplicate-request record count, verification consumption and batch failure semantics. Preserve existing backend contract/integration tests in `backend/internal/httpapi/absenceshttp`; do not change them merely because public UI labels changed. Use its configured database/test environment, never an assumed production connection. If unavailable, this live gate remains explicitly unverified.

### 12.2 Manual accessibility

- Traverse every step using keyboard only; select a course/date, open picker, move through radios, Confirm, Edit, submit in a fixture environment, and reach receipt.
- Confirm no action is nested inside another interactive label/control, future steps cannot be invoked, and focus never targets hidden content.
- Escape and Cancel discard pending dialog changes; dialog focus stays inside while open and returns to its trigger after closing.
- Use VoiceOver on Safari to check current-step announcements, labels/required fields, disabled reasons, associated errors, result heading and status messages. Avoid per-keystroke/count announcement noise.
- Measure text/control/focus contrast, including muted text and warning states. axe passing alone is insufficient.
- Test 200% browser zoom and reflow at 320 CSS px; check iOS Safari and Android Chrome keyboards plus short landscape. OTP autofill requires a controlled device/code setup; simulated paste is not proof of OS autofill.

### 12.3 Visual and familiarity acceptance rubric

Capture entry, found identity, verification, multi-course classes, dialog, review, receipt and major errors at 390×844 and 1440×900; include 320px and 844×390 stress views. Check:

1. One question-oriented column, consistent white sections and Warwick accent throughout.
2. Matching column alignment in header, content, footer and receipt; no desktop course sidebar.
3. No duplicate course selectors, stacked decorative containers, reason progress bar, or long concatenated public option labels.
4. Date/time is the first readable information in selection rows; unavailable options have understandable reasons.
5. Same Confirm/Cancel selection model across public desktop and mobile; no separate desktop interaction to learn.
6. Clear active step, primary action and next action in blocked states.
7. Long labels wrap and all controls remain reachable at the stated sizes.
8. Receipt clearly says awaiting review and contains actual returned classes/arrangements.

If representative students/parents are available, run a short unassisted task: find student, verify, select two course days, choose/change a make-up, edit reason and identify pending status. Record assistance points and misinterpretations; do not invent improvement percentages. This usability exercise informs refinement and does not replace required technical gates.

## 13. Observability

No new analytics or telemetry is needed for this presentation change. Existing API error codes and server logs remain diagnostic boundaries. Do not log ID/email/phone, reason, OTP or token data in new client events or screenshots used outside the controlled fixture environment.

Implementation review evidence must include: acceptance-to-test mapping, exact check results, current screenshots, a short list of manual devices/browser results, and any unverified live boundary. Production smoke checks inspect completion/error behaviour through existing mechanisms; no new PII-bearing tracking is introduced.

## 14. Rollout and rollback

1. Complete the automated, visual, accessibility and controlled live gates. Keep implementation separate from this planning change.
2. Review the candidate diff for public-only variants and unchanged API/storage contracts.
3. Deploy through the repository's existing staging/release process after authorization. No new feature-flag infrastructure is justified.
4. Smoke-test public lookup → verification → course selection → review → pending receipt and the staff flow using an authorized test identity. Avoid generating unsolicited live SMS.
5. Roll back on authorization leakage, duplicate records, unusable required controls, lost committed choices, inaccessible dialog actions, staff breakage or systematic completion failure.
6. Rollback mechanism: revert the implementation change through the normal deployment mechanism. Do not revert unrelated work or restore a database snapshot. Records submitted during the rollout remain valid; drafts remain schema-v1 compatible.

The transaction, session-consumption and uncertain-retry mechanisms are unchanged. If controlled live testing reveals a pre-existing failure there, report it as a separate blocker rather than changing backend policy in this UI release.

## 15. Final acceptance checklist

- [ ] R1–R6 each map to passing acceptance evidence; AT-01–AT-26 have explicit automated/manual results.
- [ ] Google Forms is the dominant visual reference; the rubric passes for all major states.
- [ ] Public lookup has one primary action, guards normalized identity and obsolete responses, and exposes no new private hints.
- [ ] OTP send/resend/paste/autofill, enrollment, expiry, delivery uncertainty and restoration retain their contracts.
- [ ] Course selection has one hierarchy; native controls and logical-day grouping agree with review/payload.
- [ ] Public desktop/mobile make-up choice shares structured dialog semantics; cancellation and stale choices are safe.
- [ ] Reason remains required with a 500-character boundary and meaningful focused validation.
- [ ] Review edits preserve values and reveal the intended target; nickname enrichment stays optional/nonblocking.
- [ ] Receipt uses returned data, retains reference, clearly reports awaiting review, and clears drafts only on success.
- [ ] Same-tab draft restoration is compatible; no cross-device-save or guaranteed-delivery claims were added.
- [ ] Typechecks, focused suites, configured browser matrix, build and changed-scope React Doctor checks pass without introduced regression.
- [ ] Keyboard, VoiceOver, contrast, reduced motion, zoom, long names, device keyboard and landscape evidence is recorded.
- [ ] Staff route/modal defaults, endpoint use and separate drafts remain compatible.
- [ ] API/session ownership, idempotency, transactional failure and verification consumption have controlled live evidence; any unavailable gate is marked unverified and blocks a full release-ready claim.
- [ ] No unrequested migration, dependency, global theme change, tracking or backend policy change is included.
- [ ] Rollback through the normal code/deployment path is documented; no existing data needs reversal.

Planning completion means this document is coherent and ready to execute. Implementation completion requires the evidence above; unchecked boxes are intentional execution tasks, not claims of completed work.
