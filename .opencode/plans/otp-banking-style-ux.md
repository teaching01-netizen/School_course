# OTP Banking-Style UX — Implementation Plan

**Date:** 2026-05-31
**Goal:** Make the OTP verification the hero of Step 1. Larger input, prominent verify CTA, reduced visual clutter, banking-style confidence.
**Files:** `src/components/absences/OtpInput.tsx`, `src/components/absences/StepCoverVerification.tsx`, `src/pages/AbsenceForm.tsx`

---

## Problem Statement

The parent verification OTP input is the smallest, least prominent element on Step 1. It's buried inside nested cards. The "Verify code" button competes visually with the countdown timer. The student profile card takes up space for context that should be minimal.

**Current hierarchy:** Student card → "Send code" button → small OTP boxes inside a card → countdown + small verify button

**Target hierarchy (banking-style):** Hero OTP input (large, centered) → prominent Verify CTA → compact student badge → de-emphasized resend

---

## Task 1: OtpInput.tsx — Hero Digit Boxes

### Size
- Digit boxes: `h-12 w-12` → `h-16 w-16` (33% larger)
- Text: `text-2xl` → `text-3xl`
- Gap: `gap-2` → `gap-3`

### Interactive states
- Container: add `cursor-pointer`
- Digit boxes: add `hover:border-[var(--color-wi-primary)]/40 hover:bg-gray-50 transition-all duration-150`
- Active position (current empty box): `border-[var(--color-wi-primary)] ring-2 ring-[var(--color-wi-primary)]/20`
- Blinking cursor: CSS `@keyframes blink` (opacity 0→1), applied only when `isFocused` state is true (toggled by hidden input `onFocus`/`onBlur`)
- Remove `•` placeholder dot — empty boxes show just the border

### Layout
- Center the digit boxes with more breathing room
- Add a visible label above: "Enter verification code" (not sr-only)
- Keep helper text below: "Enter the 6-digit code from the text message"

### Acceptance criteria
- Digit boxes are visually dominant on the page
- Cursor pointer shows on hover
- Active empty position has primary border + ring highlight
- Blinking cursor appears only when input is focused
- Empty boxes are clean (no dot placeholder)

---

## Task 2: StepCoverVerification.tsx — Prominent CTA

### Remove card wrapper
- Remove the OTP card wrapper (`rounded-sm border border-gray-200 bg-white p-4` at line 302) — flow content directly

### Verify button promoted
- `size="lg"` + `CheckCircle` icon from lucide-react
- Full-width or near full-width: `w-full` or `flex-1`
- Text: "Verify code"

### Resend / countdown de-emphasized
- Move countdown timer below the verify button
- Lighter text color: `text-gray-500`
- Keep `text-xs` but reduce visual weight

### Heading cleanup
- Remove "Parent verification" h3 (line 208) — step heading already covers this
- "Change W-code" button: change to `variant="ghost"` text link

### Error messages
- Add `AlertCircle` icon from lucide-react to error banners
- Keep `role="alert"` for accessibility

### Acceptance criteria
- OTP section flows directly without extra card nesting
- Verify button is the dominant CTA (large, full-width, icon)
- Countdown is visually secondary
- Error messages have icons

---

## Task 3: AbsenceForm.tsx — Step 1 Layout

### Student info → compact badge
- Remove full student profile card (lines 1059-1069)
- Replace with inline badge next to heading: `Parent Verify  ·  John Smith (W250389)  ·  Parent phone 089 *** 123`
- Badge styling: `text-sm text-gray-600 font-normal`

### Section structure
- Remove extra nesting — StepCoverVerification flows directly in the section
- "Continue to courses" button stays after verification success

### Acceptance criteria
- Student context is visible but minimal
- OTP zone has maximum visual real estate
- No nested cards competing for attention

---

## Verification

- `npm run test` — all 340+ tests pass (no test changes needed — hidden input `aria-label` unchanged)
- Visual: OTP boxes are 33% larger, cursor pointer on hover, active position highlighted, verify button is dominant CTA

---

## Files Modified

| File | Changes |
|------|---------|
| `src/components/absences/OtpInput.tsx` | Digit box size, cursor/hover/focus, active position, blinking cursor, label |
| `src/components/absences/StepCoverVerification.tsx` | Remove OTP card wrapper, promote Verify CTA, de-emphasize resend, compact heading |
| `src/pages/AbsenceForm.tsx` | Collapse student card into inline badge, restructure Step 1 layout |
