# Frontend UX/UI — Rigorous Gated Delivery Roadmap (v14)
_Generated: 2026-08-20 | State: ROADMAP v14 (repairing v13 SMASH 3 material + 1 superseded note, awaiting dual APPROVE: Roadmap Review + Blind Fresh Verification) | Workflow: rigorous-gated-delivery + subagent-driven-development + super-ultra-max-frontend-reasoning + notion-grade-product-designer_

## v10 → v11 Repairs (6 findings, retained)
| # | v10 Finding | v11 Repair (frozen) |
|---|---|---|
| 1 | R5 signal+AbortError weak | `useApiQuery queryFn: ({signal})=>apiJson(url,{signal})→fetch({signal})`, `retry: err.name!=='AbortError'`, `displayError` suppression, behavioral `0→1→2` abort gate. |
| 2 | Dual refcount ownership overlap | R3 sole writer `useMainInert.ts` dual `Map`+single `previousOverflowRef`; R2 via `onInertChange`. |
| 3 | Token codemod gameable | Deterministic mapping + `before.txt` + `expectedStrong/expectedSubtle` distribution. |
| 4 | useDirtyForm deep equality weak | Pure deep `Set/Record/Array/Date/null`, `warnBeforeUnload` removed, cheat ban expanded. |
| 5 | R8 dependency/DONE ambiguity | `R8 R2+R3+R5`, `DONE=(R1-7 green) AND (waiver OR R8 green)`, SLA 3d blocking. |
| 6 | Missing unhappy paths | Per-file storage try/catch, Sidebar a11y, backdrop, pagination, reduced-motion behavioral gates. |

## v11 → v12 Repairs (10 REJECT findings, retained — #7 superseded by v13 #3, see below)
| # | v11 REJECT | v12 Repair (frozen) |
|---|---|---|
| 1 | Token distribution file-pattern vs line-content | Line-content freeze: per-occurrence `grep -E "card|Modal|SlideOver|Sheet|Panel|container|box-shadow|shadow"` → `var(--border-strong)` 0.10 else `var(--color-wi-line)` 0.06; `expectedStrong` via line-content. |
| 2 | Malformed `rg var(--color` missing `)` | Split gates `rg "var\(--color-wi-line"` + `rg "var\(--border-strong"` separate, sum 568 occurrences. |
| 3 | Single-writer token freeze incomplete | Global freeze: no `border-wi-line`/`wi-line-soft` edits outside Wave 1a R3; rebase rule. |
| 4 | Wave serialization vs DAG | `Wave 1a R3 merges first; every subsequent wave rebases onto R3`. |
| 5 | Dual refcount generic cleanup/SSR | Generic cleanup + SSR inside `add/remove/update`, atomic both maps. |
| 6 | Signal `rg "signal"` string gate | Structural `queryFn.*signal` + `apiJson.*signal` + `fetch.*signal` + `AbortError` + required behavioral `0→1→2`. |
| 7 | useDirtyForm cheat ban narrow | **Superseded by v13 #3** — ban narrowed to remove `isEqual`, renamed `deepEqual` (see v13 #3). |
| 8 | Route matrix redirects not enumerated | Redirects listed with exact `?view=` params and `* → Navigate to="/"`. |
| 9 | Behavioral `vi` tests optional | `vi.useFakeTimers` lane tests blocking alongside `rg` gates. |
| 10 | PO waiver `exists` only | Content gate `waive`+`checklist`+`R8` case-insensitive. |

## v12 → v13 Repairs (3 material + 2 minor, retained)
| # | v12 REJECT | v13 Repair (frozen) |
|---|---|---|
| 1 | Route redirect destinations mis-specified | Matrix corrected: `/absences/board → /absences?view=board` (166), `/operations/session-changes → /operations/schedule-impact?view=history` (180), `* → <Navigate to="/" replace />` (189). |
| 2 | Token distribution counts lines not occurrences (568 occ vs 557 lines) | Occurrence-based gates: `rg -o --pcre2` 568 occ (76 soft occ, 492 strong occ) vs 557 lines (delta 11 multi-token lines); `expectedStrongOcc=14` / `expectedSubtleOcc=554`; `expectedStrongLines=20` audit. |
| 3 | `useDirtyForm` ban self-contradicts `isEqual` | Renamed `deepEqual`, ban `rg "JSON\.stringify|\.join\(|String\(|toString\(|lodash"` (removed `isEqual`). |
| 4 | Dual refcount double-counts | Deduped `new Set([...a.keys(),...b.keys()])` cleanup. |
| 5 | Lane Graph truncated + pre-state note | Full ASCII box, pre-state `scroll-behavior`/`var(--backdrop` noted as pre-R3 violations. |

## v13 → v14 Repairs (3 material SMASH + 1 superseded clarification, deterministic)
| # | v13 SMASH Finding | v14 Repair (frozen, blocking) |
|---|---|---|
| 1 | Inventory drift + `expectedStrongOcc` pipeline non-executable: `rg -o --pcre2 "border-wi-line(?!-soft)" | grep -E … | wc -l` always 0 because `-o` strips line context; also `480 grep -v` vs `482 PCRE2` exclusive lines and `76 vs 77 wi-line-soft` miscount; `expectedStrongOcc` via `-o` before `grep -E` fails CI | **Executable occurrence gate + inventory reconciled.** Inventory now states both metrics: **`rg "border-wi-line" --no-filename | wc -l =556` lines substring** (480 via `grep -v wi-line-soft` is undercount by 2 mixed `soft+strong` lines; **`rg --pcre2 "border-wi-line(?!-soft)" --no-filename | wc -l =482` exclusive strong lines**, `rg -o --pcre2 "border-wi-line(?!-soft)" =492` exclusive strong **occurrences** (delta 10 multi-token `border-wi-line.*border-wi-line` lines); **`rg "wi-line-soft" =77` lines** (76 `rg -o` occurrences of `border-wi-line-soft` + 1 definition `src/index.css:18 --color-wi-line-soft`); combined **`rg -o --pcre2 "border-wi-line-soft|border-wi-line(?!-soft)" =568` occurrences** (557 lines, delta 11). **`before.txt` header now records `lines: 557 / occ: 568 / softOcc: 76 / strongOcc: 492 / strongLinesPCRE2: 482 / softLines: 77 / grepVExclusiveLines: 480`.** **Fixed gate:** `expectedStrongOcc = rg --pcre2 "border-wi-line(?!-soft)" src --no-filename \| grep -E "card|Modal|SlideOver|Sheet|Panel|container|box-shadow|shadow" \| rg -o --pcre2 "border-wi-line(?!-soft)" \| wc -l` **(=14)** — first `rg` retains line context, `grep -E` filters to strong-context lines (20 inclusive lines / 14 exclusive lines), final `rg -o` counts occurrences within those lines (14, each 1). Equivalently `python sum(len(strong_re.findall(line)) for line in strong_lines) =14`. `expectedStrongLinesInclusive=20` (via `rg "border-wi-line" | grep -E | wc -l`), `expectedStrongLinesExclusive=14` (via PCRE2 pipeline), `expectedStrongOcc=14`, `expectedSubtleOcc=554` (`568-14`). Gate now executable; legacy `expectedStrong=20` retained as `expectedStrongLinesInclusive` audit. |
| 2 | (Inventory drift see #1 — `wi-line-soft` 76 vs 77) | **Reconciled in #1:** 76 occurrences vs 77 lines (definition line). Scope Summary now distinguishes `occurrences` vs `lines` and notes `grep -v` undercount. |
| 3 | (Pipeline 0 due to `-o` before `grep`) | **Repaired in #1** — `grep -E` now before final `-o`. |
| 4 | v11→v12 #7 listed as `retained frozen` but intentionally superseded by v13 #3 | **Annotated:** v11→v12 #7 now reads `Superseded by v13 #3 — ban narrowed, renamed deepEqual` to avoid confusion. |

## Scope Summary (repo-truth, CI-freezable — frozen commands, occurrence-correct)
- **Verified inventory:**
  - `ls src/pages/*.tsx | grep -v '\.test\.' | wc -l` = **41 pages**
  - `ls src/pages/*.tsx | grep '\.test\.' | wc -l` = **1 test** (`SessionChangeDetail.test.tsx`)
  - `ls src/pages/operations/*.tsx | wc -l` = **6**
  - Total **47 pages, 48 files inc. test**; `find src/pages -name '*.tsx' | wc -l` = 99 inc. `__tests__`
  - `grep -o 'path="[^"]*"' src/App.tsx | sort -u | wc -l` = **45 inc. `*`** (44 excl `*`; 3 are `Navigate` redirects `App.tsx:166 /absences/board → /absences?view=board`, `:177 /operations/calendar → /absences/calendar`, `:180 /operations/session-changes → /operations/schedule-impact?view=history` + `* → <Navigate to="/" replace />` `:189`)
  - **Token inventory (occurrences + lines, frozen):** `rg -o --pcre2 "border-wi-line(?!-soft)" =492` exclusive strong occurrences (482 exclusive strong lines via `rg --pcre2 "border-wi-line(?!-soft)" | wc -l`); `rg -o --pcre2 "border-wi-line-soft" =76` occurrences (76 lines); combined `rg -o --pcre2 "border-wi-line-soft|border-wi-line(?!-soft)" =568` occurrences (557 lines, delta 11 multi-token lines: `rg "border-wi-line.*border-wi-line"` 10 lines + 2 mixed soft+strong lines =11). Legacy line counts for audit: `rg "border-wi-line" --no-filename | wc -l =556` substring lines (480 via `grep -v wi-line-soft` undercounts by 2 mixed lines; correct PCRE2 exclusive is 482), `rg "wi-line-soft" --no-filename | wc -l =77` lines (76 occurrences + 1 definition `src/index.css:18`), combined `rg "border-wi-line|wi-line-soft" =557` lines. `src/index.css:17 --color-wi-line 0.08`, `:18 --color-wi-line-soft 0.05`, `145,165 scrollbar-color 0.14` excluded. `rg "setInterval" 7 raw (1 type SmsSendButton:77 +6 calls)`, `rg "setInterval\(" 6 calls` (`AbsenceForm:344`, `useOtp:50`, `StepCoverVerification:154`, `SmsSendButton:86`, `CrmAdmin:122`, `CrmFilterPanel:258`). `rg "scrollbar-gutter" 0`, `rg "--backdrop" 0`, `rg "backdrop-blur" 3` — pre-R3 violations.
- **Architecture:** React 19 + Vite + Tailwind 4 + TanStack Query 5 + RR7 `useBlocker`, native `<dialog>` `closedBy` (`Modal:40/SlideOver:28`), `useAuth/useApiQuery` (signal not threaded yet), `RealtimeProvider`, per-page state, `localStorage` unguarded at `AppShell:11,17` + `Sidebar:24,37`, no `scrollbar-gutter`/`--backdrop-*` vars, `--color-wi-line 0.08` not `0.06`, reduced-motion `295-301` includes `scroll-behavior` (pre-R3 violation).
- **Invariants:** Keyboard+SR+touch+reduced-motion; no layout shift; focus landed+returned (→`body` outline + `inert` dual refcount single `previousOverflowRef` deduped Set); never color-only; no silent dirty loss; `prefers-reduced-motion` scoped to `transition, animation`; `scrollbar-gutter:stable`; backdrop distinct via `var()`; `AbortError` suppressed.
- **Out of scope (deferred, BLOCKING PO sign-off, R8 blocking by default):** Backend API, DB, deployment, telemetry. 30+ pages deferred inherit shell/token/overlay only. `LegacySyncProgress.tsx` unrouted dead → R3. `/admin/operations`+`/operations/schedule-impact` actively used → R8 blocking by default unless PO waiver at `.zcode/sign-offs/po-deferred-scope.md` (content gate: `waive`+`checklist`+`R8`).

## Route Coverage Matrix (enumerated, 45 inc `*`, destinations exact)
| Page file(s) | Route(s) | Lane | Notes |
|---|---|---|---|
| `AbsenceForm.tsx` | `/absence` | **R4** (R1 patterns pre-merge) | |
| `Absences`, `AbsenceDetail`, `AbsenceDashboard`, `AbsenceSettings` | `/absences*`, `/admin/absence-settings` | deferred | |
| `TeacherDashboard` | `/teacher-dashboard` | deferred | |
| `TeacherAbsenceDetail` | `/teacher-dashboard/absences/:id` | deferred | |
| `OperationsCalendar.tsx` | `/absences/calendar` | **R7 (R2+R3+R5)** | redirect `/operations/calendar → /absences/calendar` (`App.tsx:177` — exact) |
| `Schedule.tsx`+filters/cards | `/schedule` | **R6** | |
| `Courses.tsx` | `/courses` | **R5** | |
| `OperationsHub+5` | `/admin/operations` | **R8 blocking (R2+R3+R5)** | |
| `SessionChanges(+.test)` | `/operations/schedule-impact*` | **R8 blocking (R2+R3+R5)** | redirect `/operations/session-changes → /operations/schedule-impact?view=history` (`App.tsx:180`) |
| `LegacySyncProgress` | unrouted | **R3** | dead file — route or delete |
| `Absences` board redirect | `/absences/board → /absences?view=board` | deferred | `App.tsx:166 Navigate` |
| `*` catch-all | `* → <Navigate to="/" replace />` | deferred | `App.tsx:189` |
| Remaining 17 | per App.tsx | deferred | union + 3 redirects + `*` = 45 |
| `Sidebar,Topbar,AppShell,StickyFooter,index.css,navConfig,useMainInert` | shell | **R3 sole** | global codemod 568 occurrences; rebase source; `before.txt` occurrence-based (14/554) |
| `Modal,SlideOver,SidePanel,MobileBottomSheet,useDialogModal,useMainInert(consume),useToast,useOtp,StepCoverVerification,SmsSendButton` | overlays | **R2 sole (rebase onto R3, no token edits)** | `onInertChange` read-only |
| `StepProgress,MakeUpPicker,ReasonField,FormAlert,AbsenceAppShell` | wizard chrome | **R1 sole (rebase onto R3, no token edits)** | |

## Lane Graph (DAG) — serialized (R7 depends on R2, full)
```
                         ┌─────────────────────────────────────────────────┐
R3 (useMainInert dual + scrollbar-gutter + --backdrop vars) ──→ R5 (Courses + signal) ──┐
│                        │                                              │  │
│  R3 ──→ R2 (useDialogModal, toast, timers, onInertChange → R3) ──→ R4 (wizard needs R1+R2) ─┼──→ R7 (calendar needs R2+R3+R5) ──┐
│  │                     │         │                                    │  │                  │                                │
│  R1 (wizard chrome, rebase R3) ─┴────────→ R4                         │  │                  │                                ├─→ gates (rg -o occurrences + vi) → PO waiver or R8 → DONE
│                        R2+R5 ──→ R6 (schedule, rebase R3) ────────────┘  │                  │                                │
└────────────────────────────────────────────────────────────────────────┘  └────────────────────────────────────────────────┘
R8 blocking by default (OperationsHub+SessionChanges) depends on R2+R3+R5, runs unless PO waiver signed (rebase R3).
```

## Execution Waves (rebase rule, occurrence-based snapshot — executable)
- **Wave 1a: R3 alone** — `index.css` (`0.08→0.06`, deprecate `wi-line-soft`, `html {scrollbar-gutter:stable}`, `--scrollbar-width`, `--backdrop-modal 0.5 / --backdrop-sheet 0.42 / --backdrop-nav 0.30` + **consume vars in `::backdrop` rules** `background: var(--backdrop-*)`, rewrite reduced-motion (remove `scroll-behavior`), `useMainInert.ts` dual refcount `useRef<Map>` + single `previousOverflowRef` + **deduplicated** `new Set([...a.keys(),...b.keys()])` cleanup + SSR inside, `AppShell` dual + try/catch BOTH keys, `Sidebar` tabIndex+aria+onKeyDown+resize clamp, dead-file, global codemod **568 occurrences** with **line-content per-occurrence mapping** + `before.txt` occurrence snapshot (`expectedStrongOcc` via `rg --pcre2 "border-wi-line(?!-soft)" src --no-filename | grep -E "card|Modal|SlideOver|Sheet|Panel|container|box-shadow|shadow" | rg -o --pcre2 "border-wi-line(?!-soft)" | wc -l` **=14**, `expectedSubtleOcc=554`, audit `expectedStrongLinesInclusive=20` / `expectedStrongLinesExclusive=14` + `strongLinesPCRE2=482` / `grepVExclusiveLines=480` / `softOcc=76` / `softLines=77`), `rg -o "var\(--color-wi-line"` / `rg -o "var\(--border-strong"` occurrence gates, `scrollbar-color` excluded, `max-w 1080` safe-area. Produces `before.txt` with occurrence header + line audit.
- **Wave 1b: R5 (rebase onto R3, no token edits)** — consumes vars, threaded `signal` + `AbortError` suppression (structural + behavioral).
- **Wave 2: R1 (rebase R3, no token edits) || R2 (rebase R3, no token edits)** — disjoint; R2 consumes `useMainInert` via `onInertChange`.
- **Wave 3: R4 (rebase R3, no token edits) || R6 (rebase R3, no token edits) || R7 (rebase R3, no token edits)** — R4 needs R1+R2, R6 needs R2+R5, **R7 needs R2+R3+R5** (serialized after R2).
- **Wave 4 blocking: R8 (rebase R3, no token edits)** — runs by default unless PO waiver signed; depends on R2+R3+R5.

## Ownership (single-writer, global token freeze — 568 occurrences)
- **R1:** `AbsenceAppShell,StepProgress,MakeUpPicker,ReasonField,FormAlert`, `*.a11y.test.tsx`. NOT `SidePanel/useMainInert`. **No `border-wi-line`/`wi-line-soft` edits — rebase onto R3.**
- **R2:** `Modal,SlideOver,SidePanel(motion.aside),MobileBottomSheet,useDialogModal,useToast,useOtp,StepCoverVerification:154,SmsSendButton:86` (type 77→`ReturnType<typeof setTimeout>`). Reads `useMainInert` via `onInertChange`, NOT `index.css` or `useMainInert` definition. Tests `SidePanel.test.tsx` mocks `onInertChange`, asserts calls. **No token edits — rebase onto R3.**
- **R3:** `Sidebar,Topbar,AppShell,StickyFooter,index.css,navConfig,LegacySyncProgress,useMainInert.ts` (dual, sole writer of index.css/AppShell/StickyFooter/useMainInert). Global codemod **568 occurrences** (557 lines), sole `AppShell.test.tsx` + `useMainInert.test.ts`. Owns PO artifact + `::backdrop` var consumption + dual refcount deduped Set + try/catch BOTH keys. **Sole writer of `border-wi-line`/`wi-line-soft` tokens; produces `before.txt` occurrence-based.**
- **R4:** `AbsenceForm(1302→<400),useAbsenceFlow,useSitInPriorities,useAbsenceDraft,absenceDraftStorage,Student/Verification/Classes/Review,useDirtyForm (pure, no warnBeforeUnload, deep Set/Record/Array/Date/null, deepEqual)`. Consumes R1 chrome. Owns `useBlocker(shouldBlock)` + `beforeunload` wrapper. **No token edits — rebase onto R3.**
- **R5:** `Courses,useApiQuery,api/client:apiJson(signal)` (sole signal+AbortError, retry guard, no toast on abort). **No token edits — rebase onto R3.**
- **R6:** `Schedule(+ScheduleWeek,ScheduleTable,SessionFormModal),ScheduleFilters,ScheduleSessionCard,SessionActions,SessionOccurrenceForm`. **No token edits — rebase onto R3.**
- **R7:** `OperationsCalendar` (depends R2+R3+R5). **No token edits — rebase onto R3.**
- **R8:** `OperationsHub+5,SessionChanges` (blocking by default, depends R2+R3+R5). **No token edits — rebase onto R3.**

---

### R1 — Wizard Chrome Accessibility
- **ID:** R1 — **OWNED SURFACE:** `AbsenceAppShell,StepProgress,MakeUpPicker,ReasonField,FormAlert`. **No token edits.**
- **DEPENDENCIES:** none (rebase onto R3 for tokens)
- **INTENT:** `StepProgress` check SVG + `aria-current`; `MakeUpPicker` `aria-labelledby`+`aria-expanded`; `ReasonField` `aria-live` throttled; `FormAlert` single announcement; `AbsenceAppShell` `focus()`+`aria-live`.
- **AC:** (T1) `StepProgress.a11y` check+`aria-current`; (T2) `MakeUpPicker` `aria-labelledby`+`aria-expanded`; (T3) 10 rapid keystrokes ≤2 live; (T4) `AbsenceForm.a11y` invalid wcode→`aria-invalid`+`aria-describedby`→`alert` and step→`activeElement===contentEl`; (T5) axe `critical=0, serious=0`. All in tests, not string.
- **TESTS:** `StepProgress.a11y,MakeUpPicker.a11y,ReasonField.a11y,AbsenceForm.a11y`. TDD.
- **REQUIRES_DETAILED_PLAN:** false

### R2 — Overlay & Dialog + Timer Hardening (dual refcount deduped Set, vi timers, single previousOverflowRef)
- **ID:** R2 — **OWNED SURFACE:** `Modal,SlideOver,SidePanel(motion.aside),MobileBottomSheet,useDialogModal,useToast,useOtp,StepCoverVerification:154,SmsSendButton:86` (type 77→setTimeout). Reads `useMainInert` via `onInertChange`. **No token edits.**
- **DEPENDENCIES:** R3 (`html{scrollbar-gutter:stable}` + `--scrollbar-width` + `--backdrop-*` + `useMainInert` defined; rebase onto R3)
- **INTENT:** `useDialogModal` branch `hasOwnProperty('closedBy')` else `composedPath`/`getBoundingClientRect`, `data-closing="true"`, `await Promise.race(animationend, setTimeout(200))`, trap native `<dialog>` + SidePanel `onInertChange(true/false)` → R3 dual refcount (inert+scrollLock, single previousOverflowRef, clamped `Math.max(0,get-1)`, deduped `new Set` cleanup). Per-toast `status` vs `alert` + `aria-hidden` icons + `Map<id,{tid,startedAt,remaining}>` pause `clearTimeout`/`remaining` reschedule. Replace `setInterval\(` 4 UX calls (`AbsenceForm:344 100ms,useOtp:50 1000ms,StepCoverVerification:154,SmsSendButton:86`) → `setTimeout(expiry-now)` + `visibilitychange`+`document.hidden`+unmount cancel. CRM 2 polls allowlisted.
- **AC:** (A1) native dialogs trap+Escape+return (trigger-unmounted→body). (A1b) SidePanel `onInertChange` → `main[inert]` via dual refcount; `var(--scrollbar-width)` + body overflow dual; unmounted→body outline. (A1c) **Dual refcount matrix owned by R3** (not R2) — drawer(1/1)→panel(2/2)→close drawer 1/1→close panel 0/0 + underflow `remove('ghost')` no throw (both maps clamped, deduped Set). (A2) `data-closing` + `Promise.race` both branches (`reduce false` 100-150ms, `true` 0.01ms delayed ≥150ms). (A3) backdrop matrix 3 cases + negative jsdom. (A4) `rg "setInterval\(" src/pages/AbsenceForm.tsx src/hooks/useOtp.ts src/components/absences/StepCoverVerification.tsx src/components/absences/SmsSendButton.tsx =0`; global `=2` (CRM). Using `vi`. (A5) container `role="region" polite` + per-toast `status/alert` + `vi.useFakeTimers` pause while hovered. **Gate `vi` not `jest`.**
- **TESTS:** `useDialogModal.test.ts` (Promise.race+focus), `Modal/SlideOver/MobileBottomSheet/SidePanel.test.tsx` (SidePanel mocks `onInertChange`), `useOtp.test.ts/useToast.test.tsx` (`vi` timers, remaining), timer tests.
- **REQUIRES_DETAILED_PLAN:** true

### R3 — Shell Calming & Navigation (sole writer, global codemod 568 occurrences, dual refcount deduped, behavioral gates)
- **ID:** R3 — **OWNED SURFACE:** `Sidebar,Topbar,AppShell,StickyFooter,index.css,navConfig,LegacySyncProgress,useMainInert.ts` (dual, sole). Global codemod 568 occurrences; **sole token writer**.
- **INTENT:** Remove `Topbar:23/StickyFooter:24 backdrop-blur` → solid `bg-white`; define **and consume** backdrop vars `(:root --backdrop-modal 0.5 / sheet 0.42 / nav 0.30 --scrollbar-width 0px + `::backdrop {background: var(--backdrop-*) }` + `html{scrollbar-gutter:stable}`); **`useMainInert.ts` dual:** `inertCounts Ref<Map>`, `scrollLockCounts Ref<Map>`, single `previousOverflowRef=""`, `add(key)` stores `previousOverflow` once (lockCount 0→1), `remove(key)` clamped `Math.max(0,(get||0)-1)` on **both** maps atomically, updates `main[inert]=count>0` and `body.overflow=lockCount>0?'hidden':previousOverflowRef.current`, SSR guard **inside** `add/remove/update` (`typeof document==='undefined'` no-op, hooks unconditional), fallback `'inert' in HTMLElement ? 'inert' : 'aria-hidden'`, generic deduped cleanup `useEffect(()=>()=>{ for(const k of new Set([...inertCounts.current.keys(), ...scrollLockCounts.current.keys()])) remove(k) })`. `AppShell` integrates `useMainInert` and exposes `onInertChange` to SidePanel (R2 consumes). Tokenize `--color-wi-line 0.06` + `--border-strong 0.10`, deprecate `wi-line-soft 0.05` (`0.08→0.06`). **Per-occurrence line-content mapping** per v13→v14 #1 (`expectedStrongOcc=14` via `rg --pcre2 "border-wi-line(?!-soft)" --no-filename | grep -E "card|Modal|SlideOver|Sheet|Panel|container|box-shadow|shadow" | rg -o --pcre2 "border-wi-line(?!-soft)" | wc -l`, audit `expectedStrongLinesInclusive=20` / `expectedStrongLinesExclusive=14`); `scrollbar-color 0.14` at `145,165` excluded with `/* border-token-excluded: scrollbar-thumb */`. Toggles `h-8 w-8` ≥32px. `Sidebar` handle `tabIndex=0` `focus-visible:ring` `role=separator aria-orientation vertical aria-valuenow={w} aria-valuemin 220 aria-valuemax 420 aria-valuetext="${w}px"` + `onKeyDown ArrowLeft -10→220 / ArrowRight +10→420 / Home→220 / End→420` + `dblclick→252`, **`try/catch` BOTH `Sidebar:24,37` and `AppShell:11,17`**, `window resize` clamp `Math.min(width, innerWidth-80)` and `[220,420]`. `AppShell` content `max-w-[1100] px-4 py-5` inline `max(env` → `max-w-[1080] px-6 md:px-8 py-6` via CSS `padding-left: max(1.5rem, env(safe-area-inset-left))`. Rewrite `295-301` to `animation-duration:0.01ms; transition-duration:0.01ms` only (no `scroll-behavior` — pre-state violation until R3).
- **AC:** (C1) `rg backdrop-blur Topbar+StickyFooter=0`. (C2a) `rgba 0.06≥1, 0.10≥1, 0.08=0, wi-line-soft definitions 0`. (C2b) Pre 568 occurrences (557 lines) → post `rg border-wi-line grep -v wi-line-soft grep -v scrollbar-color grep -v border-token-excluded =0` **and** `rg wi-line-soft grep -v scrollbar-color grep -v border-token-excluded =0` **and** `rg -o --pcre2 "var\(--color-wi-line" src | wc -l` + `rg -o --pcre2 "var\(--border-strong" src | wc -l ==568` **and** **occurrence distribution** `rg -o --pcre2 "var\(--border-strong" ∈[expectedStrongOcc±5]` (=14±5) + `rg -o --pcre2 "var\(--color-wi-line" ∈[expectedSubtleOcc±5]` (=554±5) sum `==568` + `git diff --numstat` no net deletion + per-occurrence category preserved + `before.txt` committed with frozen occurrence header + line audit. (C2c) post-R3 `scrollbar-gutter:stable=1`, `--scrollbar-width=1`, `--backdrop-modal 0.5=1` etc + `rg -o "var\(--backdrop" src/index.css ≥2` (pre-R3 0). (C3) toggles ≥32px. (C4) **Behavioral:** `tabIndex=0` + `aria-valuenow` + `fireEvent.keyDown ArrowLeft→-10→220` `ArrowRight +10→420` `Home→220` `End→420` `dblclick→252` + `vi.spyOn(Storage.prototype,'getItem'/'setItem') throw` per file no crash + `window resize` clamp `Math.min(width, innerWidth-80)` + `focus-visible:ring` computed. (C4b) AppShell storage safe. (C5) post-R3 reduced-motion no `scroll-behavior` (pre-R3 has it). (C4c) **Dual refcount** drawer(1/1)→panel(2/2)→close drawer 1/1→close panel 0/0 + underflow `remove('ghost')` no throw on both maps, deduped Set cleanup, single `previousOverflow` restore (not per-key). (C6) LegacySyncProgress deleted/routed. (C7) maxWidth 1080 + `env(safe-area-inset-left)`. **Owner `AppShell.test.tsx` + `useMainInert.test.ts` sole R3.**
- **TESTS:** `Sidebar.test.tsx`, `AppShell.test.tsx`, `useMainInert.test.ts` (dual, underflow both maps, deduped Set cleanup, SSR inside, single previousOverflow), visual token snapshot (568 occurrences →0 diff + vars + occurrence distribution `expectedStrongOcc=14/expectedSubtleOcc=554` + audit lines).
- **REQUIRES_DETAILED_PLAN:** false

### R4 — Absence Wizard State Machine & Calm Loading (pure useDirtyForm deepEqual)
- **ID:** R4 — **Needs R1+R2** (rebase R3) — **OWNED SURFACE:** `AbsenceForm(1302→<400),useAbsenceFlow,useSitInPriorities,useAbsenceDraft,absenceDraftStorage,Student/Verification/Classes/Review,useDirtyForm (pure, no warnBeforeUnload, deep Set/Record/Array/Date/null, deepEqual)`. Consumes R1 chrome. **No token edits.**
- **INTENT:** `useAbsenceFlow` `idle→student→verify→classes→review→submitting→success|error` + `visibilitychange`+`useConnectivity` retry; **pure `useDirtyForm`** deep `deepEqual` (`Set` size+every order-independent, `Record` shallow, `Array` every, `Date` getTime, `null`) — **frozen, no string-join/String/toString/lodash alternative, no `warnBeforeUnload`**, structured `{selectedSessionIds:Set,sitInSelections:Record,sitInPriorityLevels:Record,reason,collectedEmail}` → `useDirtyForm` → `isDirty` drives **`useAbsenceFlow`/`AbsenceForm` owns `useBlocker(isDirty && step!=='success')` + `beforeunload` wrapper** (predicate same). Success `heading.focus()` + `aria-live`; submit button `loading` not overlay; inputs `rounded-md h-10 text-[16px]`; `StepProgress aria-disabled`.
- **AC:** (F1) `wc -l <400`. (F2) `rg setInterval\( AbsenceForm=0`. (F3) `rg backdrop-blur AbsenceForm=0`. (F4) deep `Set(['a'])vsSet(['a']) false` vs `Set(['a'])vsSet(['b']) true` + `Set(['a','b'])vsSet(['b','a']) false` vs `Set(['a'])vsSet(['a','b']) true` + `{x:1}vs{x:1} false` etc structured (no string join/String/toString), `Array`/`Date`/`null` covered + `rg "instanceof Set|instanceof Date|Array\.isArray" ≥3` + behavioral `deepEqual` tests. (F4b) after `step='success'` even if `isDirty` → `useBlocker(false)`, `beforeunload` removed, SPA back not blocked (`blocker.state!=='blocked'`). Gates `rg "warnBeforeUnload" src/hooks/useDirtyForm.ts=0` + `rg "JSON\.stringify|\.join\(|String\(|toString\(|lodash" src/hooks/useDirtyForm.ts=0`. (F5) `AbsenceForm.a11y` preserved.
- **TESTS:** `AbsenceForm.flow,useAbsenceFlow,useDirtyForm.test` (deep+useBlocker+F4b, `vi`), e2e. `test:absence --coverage` thresholds.
- **REQUIRES_DETAILED_PLAN:** true

### R5 — Table & Density Notion-Grade (signal+AbortError, behavioral)
- **ID:** R5 — **Depends on R3** — **OWNED SURFACE:** `Courses,useApiQuery,api/client:apiJson(signal)` (sole). **No token edits (rebase R3).**
- **INTENT:** Heading `Courses` plural; CTA `primary`; table `min-w-[960] sticky top-11 + scroll-shadow`; hide `C-ID/Legacy`; rows `py-2 px-2.5`; mono only `course_no`; pagination `jumpToPage` on `onBlur`+`Enter` not `onChange`; bulk sticky; 18px checkbox; **signal** `queryFn:({signal})=>apiJson(url,{signal})` → `fetch({signal})`; **AbortError suppressed** `error?.name==='AbortError'?null:error`, `retry:(n,err)=>err?.name!=='AbortError' && n<2`, no toast on abort.
- **AC:** (P1) `min-w-[960]`+shadow; (P2) sticky; (P3) `fireEvent.change "1"` NOT `setSearchParams` only `blur`/`Enter` does; (P3b) **behavioral blocking (required CI job):** rapid `0→1→2` → 2 signals `aborted===true`, `query.error===null`, `vi.spyOn(addToast)` not called with error, only final `total_count`. **Structural gates:** `rg "queryFn.*signal" src/hooks/useApiQuery.ts ≥1` **AND** `rg "apiJson.*signal" src/hooks/useApiQuery.ts ≥1` **AND** `rg "signal" src/api/client.ts ≥1` **AND** `rg "fetch.*signal|signal" src/api/client.ts ≥1` **AND** `rg "AbortError" src/hooks/useApiQuery.ts ≥1` + integration test required.
- **TESTS:** `Courses.table.test` (sticky+abort behavioral+suppression+mono). `vi`.
- **REQUIRES_DETAILED_PLAN:** false

### R6 — Schedule Decomposition
- **ID:** R6 — **Needs R2+R5** (rebase R3) — **OWNED SURFACE:** `Schedule(+ScheduleWeek,ScheduleTable,SessionFormModal),ScheduleFilters,ScheduleSessionCard,SessionActions,SessionOccurrenceForm`. **No token edits.**
- **INTENT:** `ScheduleWeek 900 peek`+`ScheduleTable` sticky + `useScheduleModals`; in-card `<form>`→`SlideOver`; 1 action hover→`···`; `SessionFormModal` single preflight; skeleton instead-of grid when `loading && !sessions.length` else `opacity-60`.
- **AC:** (S1) `Schedule<300`, (S2) `rg Inline Edit Schedule=0`, (S3) hover `···`, (S4) `SlideOver`, (S5) `skeleton-week` visible **and** `schedule-grid` hidden else `opacity-60`, (S6) `test:schedule --coverage`+`check:scheduling-coverage` green.
- **REQUIRES_DETAILED_PLAN:** true

### R7 — Operations Calendar Legibility (needs R2)
- **ID:** R7 — **Needs R2+R3+R5** (rebase R3) — **OWNED SURFACE:** `OperationsCalendar` only. **No token edits.**
- **DEPENDENCIES:** R2 (SidePanel `useMainInert`+trap) + R3 (vars) + R5 (density)
- **INTENT:** Title `22px semibold`; `role="tablist" aria-selected`; merge double cards→single toolbar; month `text-xs 12px` no `text-[10px]` content `line-clamp-2 + `+N more` button `aria-label`; `openPanel(day,tab,id)` highlight; today `bg-blue-bg`+left accent; puck neutral. Allowlist `AvailabilityStatus 68,126,226`.
- **AC:** (K1) `rg text-\[10px\] OperationsCalendar=0`, (K2) single toolbar, (K3) pill `openPanel` with id, (K4) today accent, (K5) `min-w-[640] overflow-x-auto snap-x` at 320.
- **REQUIRES_DETAILED_PLAN:** false

### R8 — Conditional OperationsHub/SessionChanges Legibility (blocking by default)
- **ID:** R8 — **Blocking by default, depends on R2+R3+R5** (rebase R3) — **OWNED SURFACE:** `OperationsHub+5,SessionChanges`. **No token edits.**
- **INTENT:** `h1→h2`, `text-xs` min no 10px content, single toolbar, sticky header via `var()` tokens.
- **AC:** (R8a) `rg text-\[10px\] operations/SessionChanges content 0` (badges allowlisted), (R8b) heading-order axe 0, (R8c) PO waiver file if present must satisfy content gate `waive`+`checklist`+`R8` (case-insensitive).
- **REQUIRES_DETAILED_PLAN:** false

## Cross-cutting Verification (every lane PR — BLOCKING)
- `npm run typecheck` 0 errors
- `npm run test` full green
- `npm run test:absence --coverage` thresholds lines85/fn85/br80/stmt85
- `npm run test:schedule --coverage` + `check:scheduling-coverage` green
- `npm run test:e2e:absence` axe `critical=0, serious=0` on `e2e/absence-form-accessibility.spec.ts`
- `npm run build` succeeds
- `rg` gates (occurrence-based): `setInterval\(` scoped 0 / global 2 CRM, `border-wi-line|wi-line-soft` 557 lines / 568 occurrences →0 + occurrence distribution `expectedStrongOcc=14/expectedSubtleOcc=554` via `rg -o --pcre2 "var\(--color-wi-line"` + `rg -o --pcre2 "var\(--border-strong"` sum 568 + `border-token-excluded`, `backdrop-blur` scoped, `text-\[10px\]` scoped, `scrollbar-gutter` vars, `--backdrop` vars+`rg -o "var\(--backdrop"` consumption, `scroll-behavior` post-R3 =0, `localStorage` try/catch per-file behavioral, **structural** `rg "queryFn.*signal" + "apiJson.*signal" + "fetch.*signal" + "AbortError"` + **behavioral signal test required**, `tabIndex/aria-valuenow/onKeyDown` behavioral, `useMainInert` dual refcount+deduped Set cleanup+single previousOverflow+underflow both maps+SSR inside, `warnBeforeUnload=0` + `rg "JSON\.stringify|\.join\(|String\(|toString\(|lodash"` in `useDirtyForm` + `instanceof` branches + `deepEqual` behavioral, `var\(--color-wi-line`/`var\(--border-strong"` occurrence counts.
- **Behavioral `vi.useFakeTimers` tests blocking CI:** `useDialogModal`+`useOtp`+`useToast`+`useDirtyForm`+`Courses.table` behavioral jobs required (not optional `rg`-only).
- **Toasts:** `npm run test -- useToast.test` blocking (`vi` timers, `region`+`status/alert`+pause remaining).
- **BLOCKING PO sign-off gate (owner R3, SLA 3d):** `.zcode/sign-offs/po-deferred-scope.md` required before DONE; if absent after SLA → auto-trigger R8 blocking (not forever blocked). R8 runs by default unless PO waiver signed with checklist + explicit waive. **Content gate:** `rg -i "waive" && rg -i "checklist" && rg -i "R8"` must pass. **DONE = (R1-7 green) AND ((waiver exists AND content gate green) OR R8 green).**
- Manual checklist: Tab/Shift+Tab/Enter/Escape/focus-return/320/200 high-contrast/reduced-motion 0.01ms + dual refcount

## Subagent-Driven Execution
Wave 1a: R3 → two-stage review (spec→quality, `useMainInert` dual deduped Set + occurrence snapshot `expectedStrongOcc=14/expectedSubtleOcc=554` via `rg --pcre2 "border-wi-line(?!-soft)" | grep -E | rg -o --pcre2 | wc -l`) → merge before Wave 1b. Snapshot `before.txt` (occurrence header + line audit) reviewed.
Wave 1b: R5 → review (structural signal+AbortError + behavioral).
Wave 2: R1 || R2 (disjoint, both rebase R3, no token edits) → two-stage review, `setInterval\(` gates, deduped dual refcount.
Wave 3: R4 (pure `deepEqual` + useBlocker behavioral, `String/toString/lodash` ban) || R6 || R7 (now after R2, rebase R3) — each detailed-plan before code.
Final: cross-lane `rg -o` occurrence sweep (scoped, call-only, PCRE2, occurrence distribution `var\(--color-wi-line`+`var\(--border-strong` 568, audit `expectedStrongLinesInclusive=20`) + `typecheck+test+build` + behavioral `vi` jobs + independent sweep + adversarial goal check + PO waiver content gate or R8 verified → DONE.

## Risks & Codemod
- `index.css` codemod `rg "border-wi-line|wi-line-soft"` **568 occurrences** (557 lines) **per-occurrence** mapping + `before.txt` occurrence snapshot (header `occ:568/lines:557/softOcc:76/strongOcc:492/strongLinesPCRE2:482/softLines:77/grepVExclusiveLines:480`) + `rg -o "var\(--color-wi-line"`/`rg -o "var\(--border-strong"` occurrence distribution gates prevent cheat, per-occurrence category preserved via line contexts, single-writer R3; **R1,R2,R4,R5,R6,R7,R8 rebase onto R3, no token edits outside Wave 1a**.
- Backdrop: R3 defines+consumes `var(--backdrop-*)` in `::backdrop`; R2 only asserts via `onInertChange`; dual refcount `useMainInert` single `previousOverflowRef` + deduped `new Set` cleanup + SSR inside prevents restore-to-hidden/hook-rule bugs.
- Timers: `visibilitychange`+`document.hidden`+unmount cancel; CRM 1500ms allowlisted; `SmsSendButton 77` type→`setTimeout`, `86` loop→`setTimeout`.
- `LegacySyncProgress` dead file in R3.
- `AvailabilityStatus text-[10px]` icon badges allowlisted (aria-hidden).
- Reduced-motion `transition, animation` only; R2 `Promise.race(animationend, setTimeout(200))`.
- `useDirtyForm` pure `deepEqual` deep `Set`/`Record`/`Array`/`Date`/`null`; ban `String/toString/lodash`; `useBlocker`+`beforeunload` moved to `useAbsenceFlow`; `warnBeforeUnload` removed.
- Pagination abort via **structural** `queryFn.*signal`/`apiJson.*signal`/`fetch.*signal` + `AbortError` suppression **behavioral**; AppShell 1100→1080 safe-area preserved; Sidebar tabIndex+aria+onKeyDown+resize clamp behavioral; R7 serialized after R2 for SidePanel trap.
- PO waiver requires `waive`+`checklist`+`R8` content gate, not mere existence; pre-state `scroll-behavior`/`var(--backdrop` violations fail until R3; `grep -v` undercount documented.
