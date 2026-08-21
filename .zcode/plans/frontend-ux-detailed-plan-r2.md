# Detailed Plan — R2 Overlay & Dialog + Timer Hardening
_Lane: R2 | Depends: R3 | Rebase: R3 | RequiresDetailedPlan: true | State: Plan Review pending_

## Objective
Replace brittle `setInterval` timers, fix native `<dialog>` lifecycle (`closedBy`), restore focus via `useMainInert` dual refcount without writing `main[inert]` directly, and make toasts a11y-correct with pause-on-hover. All via `vi.useFakeTimers` behavioral gates.

## Repo Truth (pre-R2)
- `src/components/Modal.tsx:40` / `SlideOver.tsx:28` currently use `if ('closedBy' in HTMLDialogElement.prototype)` vs fallback `getBoundingClientRect`/`composedPath` — roadmap v14 freezes to `hasOwnProperty('closedBy')` branch vs `composedPath`+`getBoundingClientRect` fallback (strict canon); R2 migrates `in` → `hasOwnProperty`.
- `src/components/absences/SidePanel.tsx` is `motion.aside role=dialog` not native `<dialog>`, currently `document.body.style.overflow="hidden"` at 41, `previousFocus.current?.focus()` no `|| document.body`, no `inert` on main
- `src/hooks/useToast.tsx:41` container `role="alert" aria-live="assertive" aria-atomic`, per-toast no `role="status"/alert`, icons without `aria-hidden`, `setTimeout 4000` at 15-17 no pause, button no `type`/`aria-label`
- `src/hooks/useOtp.ts:50` `setInterval(()=>...,1000)`
- Timers to replace: `AbsenceForm:344 100ms`, `useOtp:50 1000ms`, `StepCoverVerification:154`, `SmsSendButton:86` (type 77 `ReturnType<typeof setInterval>` → `setTimeout`). CRM polls allowlisted `CrmAdmin:122 1500`, `CrmFilterPanel:258 1500`

## Owned Surface (no token edits — rebase R3)
`Modal, SlideOver, SidePanel(motion.aside), MobileBottomSheet, useDialogModal (new), useToast, useOtp, StepCoverVerification:154, SmsSendButton:86`

## Architecture
### useDialogModal (new, `src/hooks/useDialogModal.ts`) — faithful to roadmap v14 `hasOwnProperty('closedBy')` canon
```ts
export function useDialogModal(ref: RefObject<HTMLDialogElement>, opts: { onClose?: () => void, closeOnBackdrop?: boolean })
  // branch 1: Object.prototype.hasOwnProperty.call(HTMLDialogElement.prototype, 'closedBy') → dialog.showModal(), dialog.closedBy = "any" or rely on native; backdrop click handled by native (roadmap frozen hasOwnProperty, not `in`)
  // branch 2: fallback → dialog.addEventListener('click', e => { const path = e.composedPath?.() ?? []; const isBackdrop = path.length ? path[0] === dialog : e.target === dialog; if (isBackdrop && !backdropContains) opts.onClose() }) using composedPath() + getBoundingClientRect fallback
  // open: dialog.removeAttribute("open"); try{showModal()} catch; trap focus (first focusable), Escape already native
  // close: dialog.dataset.closing="true"; await Promise.race( animationend event on dialog, setTimeout(200) ); delete dataset.closing; dialog.close(); restore focus (trigger || document.body); opts.onClose
  // PrefersReducedMotion: if matchMedia("(prefers-reduced-motion: reduce)").matches → still awaits 200ms race but net 0.01ms duration (assert delayed ≥150ms)
```
Tests: `useDialogModal.test.ts` — both branches (mock `closedBy` present vs absent), `data-closing` set, `Promise.race(animationend, setTimeout(200))` timing (vi fake timers, `0.01ms` branch still ≥150ms delay), focus return body fallback.

### Modal / SlideOver
Refactor to consume `useDialogModal`. Keep `try{showModal()}` fallback. Add `data-closing` CSS hook (already in plan). No `index.css` edits.

### SidePanel (motion.aside)
Prop `onInertChange?: (open:boolean)=>void`. On `open=true` → `onInertChange(true)` mount; `false` on unmount/close. Does NOT write `main[inert]` directly (R3 owns `useMainInert`). Handles `var(--scrollbar-width)` via R3, `body overflow` via R3 dual. Test mocks `onInertChange` asserts calls, not `main[inert]`.

### useToast (`src/hooks/useToast.tsx`)
- Container: `role="region" aria-live="polite"` (not `alert/assertive`)
- Per-toast: `role="status"` for `info/success`, `role="alert"` for `warning/error`; icon `aria-hidden="true"`
- Remove button: `type="button" aria-label="Dismiss ${title}"`
- Timer: `Map<id,{tid,startedAt,remaining}>` with `setTimeout(4000)` per toast; `onMouseEnter/focusIn` → `clearTimeout`, compute `remaining = 4000 - (now - startedAt)`; `onMouseLeave/focusOut` → `setTimeout(remaining)`
- Tests with `vi.useFakeTimers()`: hover pauses `remaining`, leave resumes, not `jest`.

### Timer hardening (4 UX calls)
Replace `setInterval` → `setTimeout(expiry - Date.now())` + `document.addEventListener('visibilitychange', () => { if (!document.hidden) reschedule })` + `clearTimeout` on unmount.
- `useOtp:50` : `expiry = Date.now() + remainingMs`, `tid = setTimeout(tick, expiry - Date.now())`, `visibilitychange` reschedules.
- `AbsenceForm:344` `enforceExpiry` every 100ms → `setTimeout(enforceExpiry, Math.max(0, expiry - Date.now()))`
- `StepCoverVerification:154` poll → same
- `SmsSendButton:86` loop → `setTimeout`, type `77 ReturnType<typeof setInterval>` → `ReturnType<typeof setTimeout>`
- CRM `122/258` allowlisted: `rg "setInterval\(" global ==2` gate.

## TDD Steps (RED→GREEN→REFACTOR)
1. RED: `useDialogModal.test.ts` fails (no hook) — both branches, `data-closing`, `Promise.race`, focus return body.
2. GREEN: implement `useDialogModal.ts` minimal.
3. RED: `Modal.test.tsx`/`SlideOver.test.tsx`/`MobileBottomSheet.test.tsx` trap+Escape+return.
4. GREEN: refactor Modal/SlideOver to use hook.
5. RED: `SidePanel.test.tsx` expects `onInertChange` calls.
6. GREEN: add prop, remove direct `main[inert]` write.
7. RED: `useToast.test.tsx` expects `region/polite`, per-toast `status/alert`, `vi` pause remaining.
8. GREEN: refactor `useToast.tsx` Map+remaining.
9. RED: `useOtp.test.ts` + `AbsenceForm`/`StepCoverVerification`/`SmsSendButton` timer tests expect 0 `setInterval\(`.
10. GREEN: replace intervals with `setTimeout(expiry-now)` + visibilitychange.

## Acceptance Criteria (blocking)
- (A1) native dialogs trap+Escape+return (trigger-unmounted→body)
- (A1b) SidePanel `onInertChange` → `main[inert]` via R3 dual; `var(--scrollbar-width)`; body outline fallback
- (A1c) Dual refcount matrix drawer(1/1)→panel(2/2)→1/1→0/0 + `remove('ghost')` no throw (owned by R3, verified via mock)
- (A2) `data-closing` + `Promise.race(animationend, setTimeout(200))` both branches (reduce false 100-150ms, true 0.01ms ≥150ms delay)
- (A3) backdrop 3 cases + negative jsdom
- (A4) `rg "setInterval\(" src/pages/AbsenceForm.tsx src/hooks/useOtp.ts src/components/absences/StepCoverVerification.tsx src/components/absences/SmsSendButton.tsx =0` + global `=2` CRM, using `vi`
- (A5) `region polite` + per-toast `status/alert` + `aria-hidden` + `type="button"` + `vi.useFakeTimers` pause

## Verification (every lane PR — BLOCKING per roadmap Cross-cutting)
- `npm run typecheck` 0
- `npm run build` succeeds
- `npm run test -- useDialogModal|useToast|useOtp` (vi fake timers) green
- `rg "setInterval\("` scoped/global gates + `rg -o "var\(--backdrop"` consumption + `rg "scrollbar-gutter"` pre-state (post-R3 =1)
- `npm run test:e2e:absence` axe `critical=0, serious=0` if applicable

## Risks
- `closedBy` detection — roadmap v14 freezes `Object.prototype.hasOwnProperty.call(HTMLDialogElement.prototype,'closedBy')` (not `'closedBy' in ...`); R2 migrates existing `in` checks to `hasOwnProperty`.
- `animationend` may not fire in jsdom — `Promise.race` with `setTimeout(200)` ensures close still proceeds (both branches: `prefers-reduced-motion false` 100-150ms, `true` 0.01ms still ≥150ms delay).
- SidePanel `motion.aside` not native dialog — focus trap must be manual (`querySelectorAll` + `Tab` loop); backdrop handling uses `composedPath()` + `getBoundingClientRect` fallback per roadmap.
