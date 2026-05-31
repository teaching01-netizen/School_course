# Absence Submission With Early Parent OTP

## Summary

Move parent verification to immediately after W-code lookup. The final review step should no longer ask for an OTP. Once the user submits successfully, the UI should show a completion state only.

This replaces the old draft-based `pending_otp` flow:

- no OTP at the final confirmation screen
- no draft absence created just to hold OTP state
- no public `pending_otp` status in the absence workflow
- final submission creates the absence directly in `pending`

## Goals

1. Verify the parent or guardian as soon as the student is found by W-code.
2. Keep OTP state separate from the absence record itself.
3. Let the rest of the wizard run without any OTP prompts.
4. Make final submission idempotent and replay-safe.
5. Show a plain completion screen after submit succeeds.

## Non-Goals

- No final-step OTP gate.
- No resume-later draft absence flow.
- No `pending_otp` absence record status in the user-facing flow.
- No hidden draft absence row just for verification.

## User Flow

### 1. Lookup

The user enters a W-code and the form looks up:

- student identity
- parent phone number
- available subjects/courses

If the W-code is valid, the lookup result card appears immediately.

### 2. Parent verification

If parent SMS verification is enabled and a parent phone exists:

- the form shows a verification panel right after lookup
- the user sends a code to the parent phone
- the user enters the 6-digit code in the OTP input
- verification succeeds before any course selection happens

The parent verification token is stored locally so the user can continue after refresh.

If the school policy allows submission without OTP, the form can show an explicit bypass path instead of blocking.

### 3. Course selection

Only after verification does the user continue to:

- select courses
- choose the date range
- load sessions

### 4. Sessions and cover

The user selects the sessions to mark absent and the sessions that need cover.

### 5. Review and submit

The final review step shows the summary only:

- student
- parent verification status
- date range
- selected courses and sessions
- cover choices
- reason

There is no OTP control here. On successful submit, the UI goes directly to a completion screen.

## Backend Design

### Data model

Keep `students.parent_phone` as the source of truth for parent contact.

Add a dedicated verification session table for OTP state instead of storing OTP state on `student_absences`.

Suggested fields:

- `id`
- `wcode`
- `parent_phone`
- `status` (`pending`, `verified`, `consumed`, `cancelled`)
- `otp_code_hash`
- `otp_attempt_count`
- `otp_locked_until`
- `otp_last_sent_at`
- `otp_code_expires_at`
- `verified_at`
- `consumed_at`
- `consumed_absence_id`
- `created_at`
- `updated_at`
- `version`

Keep `student_otp_lockouts` for student-level lockout tracking.

Remove the old absence OTP columns and remove `pending_otp` from the absence status check.

### Absence statuses

The absence status model should be:

- `pending`
- `reviewed`
- `actioned`
- `cancelled`

`pending_otp` is removed from the user-facing contract.

### API contract

#### Student lookup

`GET /api/v1/absences/student-lookup?wcode=...`

Unchanged. Returns student, parent phone, and available subjects/courses.

#### Parent verification send

`POST /api/v1/absences/parent-verification/send`

Input:

- `wcode`

Behavior:

- look up the student and parent phone
- create or refresh a verification session
- send a 6-digit OTP to the parent phone
- return a signed verification token, expiry metadata, and masked phone

Rules:

- rate limit by W-code and by IP
- if parent SMS is disabled, return a feature-disabled error
- if parent phone is missing, return a parent-phone-missing error
- if the code is resent, supersede the previous code

#### Parent verification verify

`POST /api/v1/absences/parent-verification/verify`

Input:

- `token`
- `code`

Behavior:

- verify the signed token
- validate expiry and lockout state
- compare the code hash
- mark the verification session as verified
- return the verified token/session metadata

#### Final absence create

`POST /api/v1/absences`

Input includes the absence payload plus the verification token when OTP is required.

Behavior:

- validate the absence payload as before
- require a verified parent token unless policy allows OTP bypass
- create the absence directly in `pending`
- write the submitted audit event
- on replay, return the already-created absence instead of creating a duplicate

### Cleanup

Replace the old orphan-draft cleanup with cleanup for stale verification sessions.

The old `cleanup_orphaned_pending_otp()` function should go away with the draft flow.

## Frontend Design

### Wizard steps

The user-facing flow becomes:

1. Lookup and parent verification
2. Courses and dates
3. Sessions and cover
4. Review and submit
5. Completion screen

The completion screen is not a verification step. It is just the terminal success state.

### Components

Rework the OTP UI so it is rendered in the lookup area, not on the final review screen.

Keep the reusable OTP primitives:

- `OtpInput`
- `CountdownTimer`

Replace the old draft-only verification flow with a parent-verification gate component that:

- owns the send / verify / resend interaction
- restores the verification token after refresh
- unlocks the rest of the form only after verification or an explicit bypass

Remove the draft-only persistence helper:

- `useTempCreate`

Add a new hook or equivalent state module for parent verification sessions. It should:

- send and resend the code
- verify the code
- persist the token locally
- restore the token after refresh
- handle resend cooldowns and expiry

### State persistence

Persist two separate things:

- wizard form state in `sessionStorage`
- parent verification token/session state in `localStorage`

Do not persist a draft absence record because there is no draft anymore.

### Success state

After final submit succeeds:

- show the submission reference
- show the completion headline
- do not show any OTP controls
- do not prompt for additional verification

## Admin and Reporting Impact

Update every user-facing absence status list to remove `pending_otp`:

- absence board / kanban
- status badge mappings
- list filters
- status transitions

The first public state is now `pending`.

## Error Handling

### Lookup / verification

- missing W-code: prompt for W-code
- missing parent phone: block verification and point the user to the office
- SMS disabled: show a clear policy message
- code expired: allow resend
- code invalid: keep the user on the verification panel
- code locked: show the lockout timer
- session expired or consumed: require a fresh lookup/verification

### Final submit

- if the verified token has already been consumed, return the existing created absence on replay
- if the token is invalid or missing when required, block submission with a policy error
- if OTP bypass is enabled, final submit can proceed without the verification token

## Testing

### Backend

- send code, verify code, resend code
- rate limiting by W-code and IP
- expired token and superseded token handling
- lockout behavior after repeated invalid codes
- token replay on final submission
- missing parent phone and SMS-disabled policy

### Frontend

- lookup -> parent verification -> course selection -> sessions -> review -> success
- no OTP step on the final review screen
- code resend and cooldown handling
- refresh restore from localStorage
- bypass path when policy allows submit without OTP

### Admin/UI

- absence board no longer renders `pending_otp`
- status badges and filters map only the new statuses

## Migration Notes

If any `pending_otp` rows already exist in a dev or staging database, migrate them out of the old state before removing the code path.

The schema and code should converge on:

- verification state stored separately from absence records
- final absence rows created directly as `pending`
- no public dependency on `pending_otp`
