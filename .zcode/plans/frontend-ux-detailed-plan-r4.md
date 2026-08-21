# Detailed Plan — R4 Absence Wizard State Machine & Calm Loading
_Lane: R4 | Depends: R1+R2 (rebase R3) | RequiresDetailedPlan: true | State: Plan Review pending_

## Objective
Decompose `AbsenceForm.tsx` (1302 LOC, 750+ quoted) to <400 LOC by extracting `useAbsenceFlow` state machine, `useSitInPriorities`, `useAbsenceDraft`, and presentational slices (`Student`, `Verification`, `Classes`, `Review`). Replace shallow dirty check with pure `useDirtyForm` `deepEqual` (Set/Record/Array/Date/null) and move `useBlocker`+`beforeunload` out of the hook into the flow. Calm loading (button `loading` not overlay), inputs `rounded-md h-10 text-[16px]`, `StepProgress aria-disabled`.

## Repo Truth (pre-R4)
- `src/pages/AbsenceForm.tsx` 1302 LOC (`wc -l`), `setInterval(enforceExpiry,100)` at 344, `fixed inset-0 bg-white/80 backdrop-blur-sm` at 643, 750+ LOC early quote
- `src/hooks/useDirtyForm.ts:4` has `warnBeforeUnload` option, shallow `baseline[key] !== currentValues[key]` at 24-33 fails Set/Record, `beforeunload` effect at 39-46
- `src/hooks/useOtp.ts` / timers are R2, but AbsenceForm's 100ms interval is R4-owned via R2's timer hardening (coordination)
- Wizard chrome `AbsenceAppShell/StepProgress/MakeUpPicker/ReasonField/FormAlert` owned by R1 (consumed, not written)
- Draft storage `absenceDraftStorage` / `studentResumeStorage`, `useConnectivity`, `StepCoverVerification` polling are dependencies
- `src/pages/AbsenceForm.tsx` heading invalid wcode → `aria-invalid`+`aria-describedby`→`alert`, step change focus

## Owned Surface (no token edits — rebase R3, consumes R1 chrome)
`AbsenceForm(1302→<400), useAbsenceFlow (new), useSitInPriorities (new), useAbsenceDraft (new), absenceDraftStorage, Student/*, Verification/*, Classes/*, Review/*, useDirtyForm` (pure, R4 sole writer)

## Architecture
### State Machine — `src/hooks/useAbsenceFlow.ts` (new)
```ts
type Step = "idle" | "student" | "verify" | "classes" | "review" | "submitting" | "success" | "error"
type FlowState = { step: Step, wcode: string, collectedEmail: string, reason: string,
  selectedSessionIds: Set<string>, sitInSelections: Record<string,string>, sitInPriorityLevels: Record<string,string>,
  verificationStatus: "idle"|"pending"|"verified"|"failed", submitError: string | null }

type FlowAction = { type: "SET_WCODE", wcode: string } | { type: "SET_VERIFY", status: ... } | ...
  | { type: "NEXT" } | { type: "BACK" } | { type: "SUBMIT_START" } | { type: "SUBMIT_SUCCESS" } | { type: "SUBMIT_ERROR", error: string }
  | { type: "HYDRATE_DRAFT", draft: Partial<FlowState> } | { type: "RESET" }

export function useAbsenceFlow(opts: { draft?: Partial<FlowState>, onSuccessFocus?: () => void })
// reducer enforces allowed transitions:
// idle→student (SET_WCODE valid) →verify (pending→verified) →classes (require ≥1 selected) →review→submitting→success|error
// illegal NEXT when guard fails → no transition, return same state (test: step stays, reason exposed)
// side effects: visibilitychange → retry verification if pending; useConnectivity online → retry; submit via apiJson
// owns useBlocker(shouldBlock) where shouldBlock = isDirty && step !== "success"
// owns window.addEventListener("beforeunload", e => { if (shouldBlock) { e.preventDefault(); e.returnValue="" } })
// on success: headingRef.current?.focus(); announce via aria-live region
```
- Reducer is pure, tested without React.
- `useDirtyForm` drives `isDirty`; `useAbsenceFlow` derives `shouldBlock` and owns `useBlocker`+`beforeunload` (not `useDirtyForm`).

### Pure `useDirtyForm` — `src/hooks/useDirtyForm.ts` (R4 sole writer, frozen)
```ts
export function useDirtyForm<T extends Record<string,unknown>>(baseline: T, currentValues: T): boolean
// no warnBeforeUnload option, no useEffect for beforeunload
// deepEqual(a,b):
//  - primitives/null/undefined: ===
//  - Date: a.getTime()===b.getTime()
//  - Array: length=== && every(i => deepEqual(a[i],b[i]))
//  - Set: size=== && every(v => b.has(v)) order-independent (no JSON.stringify/.join/String/toString/lodash)
//  - Record: Object.keys same length && every(k => deepEqual(a[k],b[k])) shallow recursion for values (no deep nested beyond 1 level but handles our shape)
//  - fallback: ===
// isDirty = !deepEqual(baseline, currentValues)
// frozen gates: rg "warnBeforeUnload" =0, rg "JSON\.stringify|\.join\(|String\(|toString\(|lodash" =0, rg "instanceof Set|instanceof Date|Array\.isArray" ≥3
// name is deepEqual (not isEqual) to avoid self-ban
```

### Slices (presentational, <100 LOC each)
- `src/pages/absences/StudentStep.tsx` (new) — wcode input `rounded-md h-10 text-[16px]`, `aria-invalid`+`aria-describedby` on invalid, `StepCoverVerification` integration
- `src/pages/absences/VerificationStep.tsx` (new) — SmsSendButton, code input
- `src/pages/absences/ClassesStep.tsx` (new) — session list, sit-in priorities via `useSitInPriorities`
- `src/pages/absences/ReviewStep.tsx` (new) — summary, submit button `loading` prop (spinner inside button, not overlay `fixed inset-0`)
- `src/hooks/useSitInPriorities.ts` (new) — derives `sitInSelections`/`priorityLevels` from `selectedSessionIds`
- `src/hooks/useAbsenceDraft.ts` + `absenceDraftStorage.ts` (existing, `src/storage/absenceDraftStorage.ts` + `src/hooks/useAbsenceDraft.ts` pattern) — localStorage draft with try/catch, hydrate into `useAbsenceFlow`
- Note: repo has flat `src/components/ScheduleFilters.tsx` etc (R6) — R4 slices are new `src/pages/absences/*` namespace, not `src/components/schedule/*`

### `src/pages/AbsenceForm.tsx` (<400 LOC)
Composes: `AbsenceAppShell` (R1) + `StepProgress` (R1, `aria-disabled` when guard fails) + `useAbsenceFlow` + `useDirtyForm` (structured shape) + slices + `focus()` on heading after success + `aria-live` announcement. `total_count`/`loading` etc vs calm button loading.

## TDD Steps (RED→GREEN→REFACTOR)
1. RED: `useDirtyForm.test.ts` deep `Set(['a']) vs ['a'] false`, `['b'] true`, `['a','b'] vs ['b','a'] false`, `['a'] vs ['a','b'] true`, `{x:1} vs {x:1} false`, `Array/Date/null` — fails (shallow, warnBeforeUnload present).
2. GREEN: rewrite `useDirtyForm.ts` to `deepEqual` pure, remove `warnBeforeUnload`, satisfy `rg` gates + `instanceof` branches.
3. RED: `useAbsenceFlow.test.ts` reducer transitions `idle→student→verify→classes→review→submitting→success|error`, illegal NEXT blocked, `isDirty && step!=='success'` → `useBlocker(true)` else `false`, `beforeunload` only when blocking.
4. GREEN: implement `useAbsenceFlow.ts` reducer + `useBlocker`+`beforeunload` ownership.
5. RED: `AbsenceForm.flow.test.tsx` `wc -l <400`, `rg backdrop-blur AbsenceForm=0`, `StepProgress aria-disabled` when guard fails, `heading.focus()` on success, `rg setInterval\( AbsenceForm=0` (verifies R2's Wave 2 fix — R4 rebases onto R2, does not re-implement timer).
6. GREEN: extract slices, remove `backdrop-blur` (`fixed inset-0 bg-white/80 backdrop-blur-sm` at 643) from AbsenceForm, verify `setInterval(enforceExpiry,100)` at 344 already `=0` via R2 (rebase), wire `useAbsenceFlow`+`useDirtyForm`. No timer hardening in R4 (owned by R2).
7. REFACTOR: ensure `test:absence --coverage` thresholds, `AbsenceForm.a11y` preserved.

## Acceptance Criteria (blocking)
- (F1) `wc -l src/pages/AbsenceForm.tsx <400`
- (F2) `rg "setInterval\(" src/pages/AbsenceForm.tsx =0`
- (F3) `rg "backdrop-blur" src/pages/AbsenceForm.tsx =0`
- (F4) deep `Set(['a'])vsSet(['a']) false` / `['b'] true` + `['a','b']vs['b','a'] false` / `['a']vs['a','b'] true` + `{x:1}vs{x:1} false` etc (no `String`/`toString`/`JSON.stringify` join), `Array`/`Date`/`null` covered + `rg "instanceof Set|instanceof Date|Array\.isArray" ≥3` + behavioral `deepEqual` tests
- (F4b) after `step='success'` even if `isDirty` → `useBlocker(false)`, `beforeunload` removed, SPA back not blocked (`blocker.state!=='blocked'`); gates `rg "warnBeforeUnload" =0` + `rg "JSON\.stringify|\.join\(|String\(|toString\(|lodash" =0` in `useDirtyForm.ts`
- (F5) `AbsenceForm.a11y` preserved (invalid wcode `aria-invalid`+`aria-describedby`→`alert`, step change `activeElement===contentEl`)

## Verification
- `npm run typecheck` 0
- `npm run test -- useDirtyForm|useAbsenceFlow|AbsenceForm.flow` (vi, deepEqual behavioral + F4b) green
- `npm run test:absence --coverage` thresholds lines85/fn85/br80/stmt85
- `rg` gates `warnBeforeUnload`, `JSON.stringify|\.join\(|String\(|toString\(|lodash`, `instanceof`, `setInterval\(`, `backdrop-blur`
- `npm run build` succeeds

## Risks
- `useBlocker` is RR7; mock returns `{state:'blocked', proceed, reset}` and `ConfirmModal` wiring must be tested with `vi`.
- Draft hydration must not mark dirty on first render — baseline is initial `FlowState`, current is hydrated; `deepEqual` handles.
- `Set` order-independent: `every(v => b.has(v))` plus size check, not `Array.from(Set).sort().toString()` (banned).
