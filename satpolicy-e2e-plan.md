# SAT Verbal Leave Policy — E2E Test Plan

Source policy: [satpolicy.md](satpolicy.md)
Status: plan only, no tests written yet. Date: 2026-11-02.

## 0. Why the current E2E suite can't prove this policy

Every spec in `e2e/` uses `page.route(...)` to fake `/api/v1/absence-self-service/sessions`
(see `e2e/absence-makeup-selection.spec.ts`). Those specs test the UI, not the
policy, because the priorities they show are hand-written fixtures. The policy itself lives in the Go
backend:

| Concern | Code |
|---|---|
| Rule table (26 rule entries) | `src/features/leave-policy/domain/rules.ts` → applied via `POST /api/v1/admin/sat-verbal-policy/apply` (`backend/internal/httpapi/satverbalpolicyhttp/routes.go`) |
| Name matching / aliases / sections | `backend/internal/satverbalpolicy/policy.go` |
| Priority resolution, same-lesson, final-day, reveal | `backend/internal/httpapi/absenceshttp/sat_verbal_policy.go` |
| Derived targets (Mastery/Wisdom/KO/Intensive/Believe/RTP/Brush Up) | `satVerbalDerivedTargetNames` in the same file |
| Reveal param | `sat_verbal_after_priority` query param (`routes.go:1396`, `sessions_range_request.go:292`) |
| Submit-time validation | `validateSitInCandidates` (`sitin_validation.go`), `ValidSitInSessionOverlap` (`routes.go:807`) |
| 48 h rule | `absences.ValidateSessionTiming` (`backend/internal/absences/submission.go:142`), settings `min_hours_before_session` / `max_hours_after_session` |
| UI reveal | `handleNotAvailable` in `src/pages/AbsenceForm.tsx:709`, "See other times" / "Previous times" buttons |

So the plan has **two layers that run against the real stack** (Go API + Postgres + built frontend):

- **L1 — API contract E2E** (Playwright `request` fixture, no browser). This layer covers the full policy matrix. It is fast and checks exact results.
- **L2 — Browser E2E** (Playwright pages). These cover the student and staff journeys for each rule family, from opening the form to submitting.

The existing mocked specs stay as they are and keep covering visual and mobile UX.

---

## 1. Policy → testable rules

Each rule has an ID that the test cases below refer to.

### Global rules (apply to every row)

| ID | Rule (from satpolicy.md) | Testable statement |
|---|---|---|
| G1 | Offer 1st priority first; 2nd only after the student says they can't attend 1st | The first response holds **only** the lowest available priority level. Data for the next level is never in it. The next level appears only after `sat_verbal_after_priority=<current>`. |
| G2 | Reveal is progressive, per level | `has_next_priority` is true exactly when a higher level exists. The last level has no "See other times". |
| G3 | No sit-in may use the final class of a subject (End-of-class Meal) | No `available_sessions` entry falls on the **Bangkok calendar day** of the target course's last session. If it shows up at all, it is under `unavailable` with `reason_code=target_final_class`. |
| G4 | Students can pick the sit-in day ("same as Math") | A priority with several eligible sessions shows a day picker. The student can choose any eligible day, not just a pre-selected one. |
| G5 | Leave in advance / backdated, within 48 h | See §3 open question Q1. Boundary tests in §5.7. |
| G6 | Priority count: Beginner & Rank 3 = 3; Rank 4–5, Mastery, Wisdom = 2 | The number of reachable levels for each row matches. Rank 3 Sec 2/3 deliberately skip level 2. |

### Per-row rules

| ID | Missed course | P1 | P2 | P3 |
|---|---|---|---|---|
| B-R | Reading Beginner Sec 1/2/3 | Other section, **same lesson #** (Sec2→Sec1, Sec3→Sec1, Sec1→Sec2[/Sec3]) | Reading Rank 5, any day | Reading Rank 4, any day |
| B-W | Writing Beginner Sec 1/2/3 | same as above, Writing | Writing Rank 5 | Writing Rank 4 |
| R5-R / R5-W | (SAT / SAT Verbal) Reading/Writing Rank 5 | Same-subject Rank 4, any day | Rank 3 Sec 1 or Sec 2 | — |
| R4-R / R4-W | (SAT / SAT Verbal) Reading/Writing Rank 4 | Same-subject Rank 5, any day | Rank 3 Sec 1 or Sec 2 | — |
| R3-S1 | Rank 3 Sec 1 | Other Rank 3 section, same lesson # | Rank 2 | Reading or Writing Rank 4 |
| R3-S2/S3 | Rank 3 Sec 2 (and Sec 3) | Rank 3 Sec 1, same lesson # | **none** (Rank 2 runs at the same time) | Reading or Writing Rank 4 |
| R2 | Rank 2 | Rank 3 | Rank 1 | Reading Mastery **or** Writing Wisdom (may not open) |
| R1 | Rank 1 | Rank 2 | Reading Mastery or Writing Wisdom | — |
| MW | Reading Mastery / Writing Wisdom | Opposite rank of the student's main course (R3→R2, R2→R3) | (see Q4) | — |
| KIB | Knock Out / Intensive / Believe | Opposite rank (R3→R2, R2→R3) | (see Q4) | — |
| RTP | Real-Time Practice | Based on main course: R3→R2, R2→R1, R1→R2 | — | — |
| BU | Brush Up | Main R4→R5, R5→R4 (same R/W subject) | Rank 3 Sec 1 or Sec 2 (see Q6) | — |

---

## 2. Gaps found while reading code (each one has a test that should fail if it's real)

| # | Observation | Test |
|---|---|---|
| X1 | **Submit validation resolves with `afterPriority=0`** (`sitin_validation.go:30` → `resolveSitInForCourse(..., 0, true)`). `satVerbalVisiblePriority` then keeps only the lowest level, so a P2/P3 session chosen after "See other times" may be rejected with `selected sit-in session is outside the allowed policy target`. | SV-SUB-02, SV-SUB-03 (**run first**) |
| X2 | Rank 3 Sec 1 `makeupTargets` contains `{section:"Section 3", subject:"Math"}` and Sec 2 `subject:"Writing"`. Mapped targets filter on subject (`satVerbalMappedCourseMatchesTarget`), so the Sec 3 target may never match a real Rank 3 Sec 3 course. | SV-R3-02 |
| X3 | Rank 2 P1 = "SAT Verbal Rank 3" matches **every** Rank 3 section by family. Rank 3 Sec 2 runs at the same time as Rank 2, so a Rank 2 student could be offered a slot that clashes with their own weekly class. | SV-R2-03 |
| X4 | Policy header says Mastery/Wisdom have **2** priorities. Code has `priorityCount: 1` for MW and KIB. | SV-MW-04 (blocked on Q4) |
| X5 | 48 h settings are **global** form settings (they also affect Math). Defaults are 0 (off). Nothing ties them to SAT Verbal. | SV-48-* (blocked on Q1) |
| X6 | MW/KIB derivation takes the **first** enrolled course with rank 2/3. If a student is in both, the order decides the target. | SV-MW-05 |
| X7 | Brush Up / RTP use `satVerbalMainCourse` = first enrolled course not matching the missed one. If the student has no rank course, there are no derived targets and the code falls back to all `eligibleTargets` (R4+R5 both). | SV-BU-04, SV-RTP-04 |
| X8 | Several resolver paths use `time.Now()` directly (`resolver.go:660,883,1151`), so a backend fake clock is impossible without code changes. | Infra §4.3 |

---

## 3. Open questions to settle with the business before writing assertions

Each one blocks the test IDs listed. Write those tests with `test.fixme` until there's an answer.

| Q | Question | Blocks |
|---|---|---|
| Q1 | "ลาล่วงหน้าและย้อนหลังได้ไม่เกิน 48 hrs". (a) Backdated leave allowed up to 48 h after class **and** advance leave at most 48 h before class? or (b) backdated ≤ 48 h only? The system can only express "≥ N h before" (`min_hours_before_session`) and "≤ N h after" (`max_hours_after_session`). It has no "≤ N h in advance" setting. Should SAT Verbal have its own setting, separate from Math? | SV-48-* |
| Q2 | Beginner Sec 1: is the P1 target Sec 2 only, or Sec 2 **and** Sec 3 (code)? | SV-BEG-03 |
| Q3 | If the **missed** lesson is itself the final class, is there any make-up? Code allows missing it but blocks the same-lesson target (final), then falls through to P2. | SV-FIN-04 |
| Q4 | MW/KIB P2: what is it, given both columns have the same "opposite rank" note? | SV-MW-04, SV-KIB-04 |
| Q5 | Rank 3 same-lesson mapping: the policy says "use the teacher's Rank 3 schedule sent to TC each cycle". Is lesson # = session order in the system, or a separate per-cycle mapping? | SV-R3-05 |
| Q6 | Brush Up: is "Rank 3 Sec 1 or Sec 2" a P2, or an alternative P1 for students whose main course is Rank 3? | SV-BU-03 |
| Q7 | "ต้องทำตัวเลือก 1.R mastery 2.W wisdom" for Rank 1/2: should Mastery appear before Wisdom (ordered), or both under one level (code)? | SV-R2-05, SV-R1-03 |
| Q8 | Staff override: can staff book a priority the student hasn't revealed, or a final-day class (special approval)? | SV-STF-04 |

---

## 4. Test infrastructure

### 4.1 Stack

```
docker compose up -d db                       # postgres:16 (docker-compose.yml)
go run ./backend/cmd/migrate                  # schema
go run ./backend/cmd/e2eseed --manifest e2e/.sat-seed.json   # NEW (see 4.2)
go run ./backend/cmd/server                   # API
npm run build && npm run preview              # frontend
```

- Add a Playwright **project** `sat-policy`, with `testMatch: /sat-verbal\/.*\.spec\.ts/` and `fullyParallel: false`. It runs on `chromium-desktop`, `chromium-phone-standard`, and `webkit-phone` only. The full policy matrix is not run on all 13 viewports, because it is viewport-independent.
- `globalSetup` runs the seeder and writes the manifest. `globalTeardown` deletes the rows by run suffix.
- Check how `vite preview` reaches the API (proxy or `VITE_API_BASE`) and set it for this project.
- **Rate limits.** The public lookup is limited to 20/hour per wcode and 50/hour per IP (`pending_routes.go:402,411`). Give each test its own wcode, or add an env flag that turns rate limits off outside production.

### 4.2 Seeder `backend/cmd/e2eseed` (new)

It reuses the `sqldb` queries the integration tests already call (`CourseCreate`, `AdminUserCreate`, …), the same way as `sat_verbal_policy_merge_group_integration_test.go`. Then it calls `POST /admin/sat-verbal-policy/apply` with the real `LEAVE_POLICY_COURSE_RULES`, so the mapping path is exercised as well. It refuses to run when `APP_ENV=production`, like `devseed`.

Its output is a JSON manifest with course IDs, session IDs per course in order, final session ID, student wcodes per scenario, and the institute TZ. Tests read IDs from the manifest and never hard-code them.

### 4.3 Clock strategy

The backend clock can't be faked (X8), so **all dates are seeded relative to the seeder's `now` in Asia/Bangkok**:

- `D0` = today 00:00 Bangkok. Every course runs a 6-session cycle on its own weekday. Sessions #1–#2 fall in the past and #3–#6 in the future. Session #6 is the **final class** (End-of-class Meal).
- For 48 h boundaries, set `end_at` to `now − 47h50m` / `now − 48h10m`. The ±10 min margins keep it reliable. Never assert on second-level boundaries end to end. Exact boundaries are already unit-tested in `absences/submission.go` tests.
- The browser runs with `timezoneId: "Asia/Bangkok"`. One extra run uses `America/Los_Angeles` to prove that the final-day check uses the institute's time zone, not the browser's.

### 4.4 Seed catalog (one cycle, names must hit `NameAliases`)

| Course | Slot | Notes |
|---|---|---|
| SAT Verbal Reading Beginner Section 1 / 2 / 3 | Mon 17:00 / Sun 09:00 / Wed 17:00 | Sec 3 has only **5** sessions in variant `short` (to hit `same_occurrence_missing`) |
| SAT Verbal Writing Beginner Section 1 / 2 / 3 | Tue / Sun 13:00 / Thu | |
| SAT Verbal Reading Rank 5, SAT Verbal Writing Rank 5 | Sat 09:00 / Sat 13:00 | |
| SAT Reading Rank 4, SAT Writing Rank 4 | Fri 17:00 / Sun 15:00 | "SAT …" prefix on purpose: checks the `sat`↔`sat verbal` alias |
| SAT Verbal Rank 3-Section 1 / 2 / 3 | Sat 09:00 / **Sat 13:00** / Sun 13:00 | Sec 2 is at the **same time as Rank 2** |
| SAT Verbal Rank 2 | **Sat 13:00** | |
| SAT Verbal Rank 1 | Sun 09:00 | |
| Reading Mastery, Writing Wisdom | Wed / Thu | toggle `open=false` variant = not created this cycle |
| SAT Verbal Real-Time Practice, Brush Up, Knock Out, Intensive, Believe | various | |
| Final-day doubles: Reading Rank 5 variant with **two** sessions on the final day | | proves the whole day is excluded, not only the last session |
| Control: a Math course with a normal sit-in rule | | non-regression |

Students: one wcode per scenario, enrolled in the missed course plus the "main course" the derived rules need (for example, a Mastery student also enrolled in Rank 3 Sec 1).

### 4.5 Shared helpers (new, `e2e/sat-verbal/helpers.ts`)

- `sessions(wcode, courseId, afterPriority?)` → typed `/absence-self-service/sessions` response.
- `expectOnlyLevel(resp, level)`. It asserts `current_priority_level` and that every `priorities[*].level` equals that level. It also scans the **raw JSON text** for session IDs of other levels to catch leaks.
- `expectNoFinalDay(resp, manifest)`. No available session falls on the target's final Bangkok day.
- `revealAll(wcode, courseId)`. Walks `after_priority` until `has_next_priority=false` and returns the list of levels, e.g. `[1,3]`.
- `submit(wcode, payload)` → status + body.

---

## 5. Test cases

Format: **ID** — setup → action → expected. L1 = API, L2 = browser.

### 5.1 Progressive reveal (G1, G2) — L1 + L2

- **SV-REV-01 (L1)** — Beginner Sec 2 student, missed #3 → initial `sessions` → only level 1. Raw JSON contains no Rank 5/Rank 4 course IDs or session IDs. `has_next_priority=true`.
- **SV-REV-02 (L1)** — same → `after_priority=1` → only level 2 (Rank 5). `has_next=true`. `after_priority=2` → level 3, `has_next=false`. `after_priority=3` → `sit_in_method=none`, `priorities=[]`.
- **SV-REV-03 (L1)** — Rank 3 Sec 2 → `revealAll` = `[1,3]`. No level 2 at any step. A Rank 2 course ID never shows up in any response.
- **SV-REV-04 (L1)** — a level with no eligible target (Mastery/Wisdom not open) still comes back as an empty priority. `has_next` is correct and the client is not stuck.
- **SV-REV-05 (L1)** — `after_priority=-1`, `abc`, `1.5` → 400 `bad_sat_verbal_after_priority`. `after_priority=99` → empty result, no 500.
- **SV-REV-06 (L2)** — student form: pick missed class → the sheet shows P1 times only, with no "Rank 5" text in the DOM. Tap "See other times" → P2 shows up. "Previous times" returns to P1, and the P1 choice is still checked.
- **SV-REV-07 (L2)** — two missed classes (#3, #4). Revealing P2 for #3 doesn't change the level shown for #4.
- **SV-REV-08 (L2)** — at the last level, "See other times" is gone. If no times exist, the sheet shows "No make-up times available" + "Staff will help arrange the next step."
- **SV-REV-09 (L2)** — the reveal request fails (route abort only on the `after_priority` call, with the real backend for everything else) → error text, selection unchanged, "Try again" recovers.
- **SV-REV-10 (L2)** — reload mid-flow → the form doesn't silently jump to P2. It starts again at P1, or restores a draft that clearly shows its level.

### 5.2 Final class / End-of-class Meal (G3) — L1

- **SV-FIN-01** — for **every** row, `revealAll` → `expectNoFinalDay` at every level. This is a parameterized loop over the whole seed catalog.
- **SV-FIN-02** — final-day doubles variant → both sessions on the final day are unavailable with `target_final_class`.
- **SV-FIN-03** — Beginner same-lesson: missed #6 → the Sec 1 #6 target is unavailable (`target_final_class`) → the reveal moves to P2. The Rank 5 list there also leaves out Rank 5's final day.
- **SV-FIN-04 (Q3)** — what is expected when the missed lesson is itself the final class.
- **SV-FIN-05** — a final day near midnight: session 23:30 Bangkok = 16:30 UTC on the previous UTC date → still treated as the final day. Run once with browser TZ = LA.
- **SV-FIN-06** — submit tampering: POST with the target's final-day session ID → 400 `invalid_sessions`.

### 5.3 Per-row matrix — L1 (each row also gets one L2 journey, §5.6)

**Beginner (B-R, B-W)**
- **SV-BEG-01** — Sec 2 missed #2 → P1 = **only** Sec 1 #2 (not #1/#3). The `missed_session_id` mapping is correct.
- **SV-BEG-02** — Sec 3 missed #3 → P1 = Sec 1 #3 only (the policy's "*** Section 3 → Section 1").
- **SV-BEG-03 (Q2)** — Sec 1 missed #3 → P1 targets = {Sec 2 #3} or {Sec 2 #3, Sec 3 #3}.
- **SV-BEG-04** — Sec 1 #3 already in the past (it ran before today) → the P1 entry is unavailable `before_request_date`. The level still counts and P2 is reachable.
- **SV-BEG-05** — target section has fewer sessions (`short`) → `same_occurrence_missing`.
- **SV-BEG-06** — P2 = Reading Rank 5 (Reading) / Writing Rank 5 (Writing) **all** non-final future dates. There is no cross-subject leak (Reading never sees Writing).
- **SV-BEG-07** — P3 = Rank 4, same subject, non-final.
- **SV-BEG-08** — future leave: missed Sec 2 #5 next month, Sec 1 #5 falls between today and the leave date → offered (it matches the unit test `BeginnerFutureLeaveCanUseSameLessonBeforeLeaveDate`).

**Rank 4/5 (R4, R5)**
- **SV-R45-01** — Reading Rank 5 → P1 = Reading Rank 4 only, any non-final day. More than one day is offered (G4).
- **SV-R45-02** — `SAT Reading Rank 4` (alias name) → P1 = Reading Rank 5.
- **SV-R45-03** — P2 = Rank 3 Sec 1 + Sec 2, **never** Sec 3. `revealAll` = `[1,2]`.
- **SV-R45-04** — Writing rows mirror Reading. Run as a parameterized loop over 8 rows (R/W × 4/5 × "SAT"/"SAT Verbal" prefix).

**Rank 3**
- **SV-R3-01** — Sec 1 missed #4 → P1 = other section #4 only.
- **SV-R3-02 (X2)** — Sec 1 → P1 includes Rank 3 Sec 3 #4 if the business says Sec 3 is a valid target. Expected to fail today because of the `subject:"Math"` filter.
- **SV-R3-03** — Sec 1 → P2 = Rank 2 (any non-final), P3 = Reading Rank 4 + Writing Rank 4.
- **SV-R3-04** — Sec 2 and Sec 3 → P1 = Sec 1 same #, then P3. Rank 2 never appears (G6).
- **SV-R3-05 (Q5)** — the lesson-# mapping follows the teacher's schedule, not the session index.

**Rank 2 / Rank 1**
- **SV-R2-01** — P1 = Rank 3, non-final. **SV-R2-02** — P2 = Rank 1.
- **SV-R2-03 (X3)** — P1 must leave out Rank 3 Sec 2 slots that clash with the student's own Rank 2 class (Sat 13:00 weekly), unless that week's Rank 2 class is the one being missed. Check what `sit_in_conflict_filter.go` really filters.
- **SV-R2-04** — P3 with Mastery and Wisdom both open → both are offered. **SV-R2-05 (Q7)** ordering/choice.
- **SV-R2-06** — Mastery and Wisdom not open → P3 empty, `has_next=false`, staff-help message (L2).
- **SV-R1-01** — Rank 1 → P1 = Rank 2. **SV-R1-02** — P2 = Mastery/Wisdom. **SV-R1-03** — closed → empty.

**Mastery / Wisdom / Knock Out / Intensive / Believe (MW, KIB)** — parameterized over 5 rows
- **SV-MW-01** — student main = Rank 3 → target Rank 2 only.
- **SV-MW-02** — student main = Rank 2 → target Rank 3 (all sections? confirm whether Sec 2 clashes, see X3).
- **SV-MW-03** — student has no Rank 2/3 enrollment → expected: no make-up / staff help. Code falls back to both ranks, so this checks X7.
- **SV-MW-04 (Q4/X4)** — second priority exists and is correct.
- **SV-MW-05 (X6)** — student enrolled in both Rank 2 and Rank 3 (transition) → the target is deterministic and documented.

**Real-Time Practice (RTP)**
- **SV-RTP-01/02/03** — main R3→Rank 2, R2→Rank 1, R1→Rank 2.
- **SV-RTP-04 (X7)** — no main rank course → defined behaviour.

**Brush Up (BU)**
- **SV-BU-01** — main Reading Rank 4 → Reading Rank 5 only (no Writing). **SV-BU-02** — main Writing Rank 5 → Writing Rank 4.
- **SV-BU-03 (Q6)** — Rank 3 Sec 1/Sec 2 placement.
- **SV-BU-04 (X7)** — no main course → defined behaviour (not "all four R4/R5 courses").

### 5.4 Slot eligibility rules (all rows) — L1

- **SV-ELG-01** — two missed sessions, one candidate slot left → the second missed session sees it as `already_offered`. Assigning the same slot to both is impossible.
- **SV-ELG-02** — a slot already booked by this student's other absence → `sit_in_session_already_used`.
- **SV-ELG-03** — a slot overlapping the missed class time → `overlaps_missed_class`.
- **SV-ELG-04** — a slot beyond the subject `sit_in_window_weeks` → `outside_cutoff`. Set the window to 2 weeks for SAT and check the Rank 4 week-3 session.
- **SV-ELG-05** — cancelled or inactive target session → not offered, and rejected on submit.
- **SV-ELG-06** — merge-group target → `merged_start_at/merged_end_at` cover the whole merged day. Selection and submit work (matches the existing integration test).
- **SV-ELG-07** — the pre-selected count is no larger than the missed count, and every pre-selected entry is also in `available`.

### 5.5 Submission (persisted outcome) — L1 + L2

- **SV-SUB-01** — P1 selection → 2xx. DB row has `sit_in_course_id` = P1 target, sit-in session IDs stored, snapshot written.
- **SV-SUB-02 (X1, run first)** — student reveals P2, picks a Rank 5 session, submits → **must be accepted**. If it is rejected with `outside the allowed policy target`, X1 is confirmed.
- **SV-SUB-03 (X1)** — same at P3, and for Rank 3 Sec 2 at level 3 (skipped level).
- **SV-SUB-04** — tampering: submit a session from a course outside the student's chain (e.g. Writing Rank 5 for a Reading Beginner) → 400.
- **SV-SUB-05** — tampering: submit a P2 session **without** revealing first, as a direct API call. Agree the expected result with Q8. The policy gate is a UX rule, but decide whether the server also enforces it.
- **SV-SUB-06** — stale: another request cancels the chosen session (or bumps its version) between viewing and submitting → clear error. L2: the user can pick again without losing other answers.
- **SV-SUB-07** — double-submit (double tap) → one absence only (idempotency).
- **SV-SUB-08** — after a successful submit, the next `sessions` call marks that slot `sit_in_session_already_used`.
- **SV-SUB-09** — confirmation email/SMS (if enabled) names the right sit-in course and date, in Bangkok time.

### 5.6 Journeys per rule family — L2 (3 viewports)

One complete run per family: open form → identify → pick missed class → choose make-up (the day picker shows more than one day where the rule allows it, G4) → submit → success screen.

| Journey | Covers |
|---|---|
| J1 Beginner Sec 2 → P1 Sec 1 same lesson | B, G1, G3 |
| J2 Beginner Sec 2 → can't attend → P2 Rank 5, pick 2nd available day | G1, G2, G4, X1 |
| J3 Rank 3 Sec 2 → P1 → can't → lands on "3rd" level (check the label text, not just "next") | G6, R3-S2 |
| J4 Reading Rank 4 → P1 Rank 5 → P2 Rank 3 Sec 1 | R4 |
| J5 Rank 2 with Mastery/Wisdom closed → walk to the end → staff-help state | R2, SV-R2-06 |
| J6 Mastery student (main Rank 3) → Rank 2 | MW |
| J7 Brush Up student (main Writing Rank 5) → Writing Rank 4 | BU |
| J8 Math control → normal sit-in flow unchanged | non-regression |

Each journey also runs `expectAccessiblePage`, `expectNoHorizontalOverflow`, and keyboard-only selection (`e2e/helpers/absenceAssertions.ts`).

### 5.7 48-hour rule (G5, blocked on Q1) — L1 + one L2

Setup: `max_hours_after_session=48` (plus `min_hours_before_session` or a new "max hours in advance" setting, depending on Q1).

- **SV-48-01** — class ended 47h50m ago → listed and submittable.
- **SV-48-02** — class ended 48h10m ago → not listed. Direct submit → 400 `grace_period_expired`.
- **SV-48-03** — class currently running → allowed (the backdated branch returns early in `sessionTimingPolicyError`).
- **SV-48-04 (Q1a)** — class starts in 47h50m → allowed. Class starts in 48h10m → expected rejection. This needs a new setting, so today it should fail.
- **SV-48-05** — the session list's lookback window (`routes.go:231`, ceil(48/24)=2 days) shows exactly the classes that pass SV-48-01.
- **SV-48-06** — scope: confirm whether Math also gets 48 h, since the setting is global (X5).
- **SV-48-07 (L2)** — an expired class shows the message "Request period ended 48 hours after class" and can't be selected.

### 5.8 Staff flows — L1 + L2

- **SV-STF-01** — staff create (`StaffCreateAbsenceModal`) for a Beginner student → same P1 as the student sees.
- **SV-STF-02** — staff "See other times" → P2/P3, then submit → accepted (X1 again on the staff path `staff_create_tx.go:241`).
- **SV-STF-03** — batch create (`batch_routes.go:588`) with mixed SAT rows → every row is validated by policy, and one bad row doesn't commit the others (or is reported per row, whichever is specified).
- **SV-STF-04 (Q8)** — special approval / override of a final-day class or an unrevealed priority.
- **SV-STF-05** — inbox shows the sit-in impact labels with subject names, never opaque course IDs (see recent commits `bf6488c`, `f55004c`).

### 5.9 Admin policy mapping — L1

- **SV-ADM-01** — apply the full `LEAVE_POLICY_COURSE_RULES` → `matched_courses` holds every seeded SAT course (26 rule entries, including the `SAT …` alias rows). `unmatched_policy_rows` is empty except intentionally closed Mastery/Wisdom, and those show up as warnings.
- **SV-ADM-02** — rename a mapped course → the policy still resolves through `rule_id` (matches the unit test `MappedCourseUsesRuleIDInsteadOfProductionCourseName`).
- **SV-ADM-03** — `DELETE /mapping/{rule_id}` → that course no longer returns `rule_type=sat_verbal_policy`.
- **SV-ADM-04** — invalid policy JSON / row missing id → 400 `bad_policy`, and existing mappings are unchanged.
- **SV-ADM-05** — non-admin (Teacher) → 403 on all three endpoints.
- **SV-ADM-06** — re-apply with the same policy → no duplicate mappings, and the hash is unchanged.

---

## 6. Traceability

| Rule | Tests |
|---|---|
| G1 | SV-REV-01, -02, -06, J2 |
| G2 | SV-REV-02, -03, -04, -08 |
| G3 | SV-FIN-01…06 |
| G4 | SV-R45-01, J2 |
| G5 | SV-48-01…07 |
| G6 | SV-REV-03, SV-R3-04, J3 |
| B-R/B-W | SV-BEG-01…08, J1, J2 |
| R4/R5 | SV-R45-01…04, J4 |
| R3 | SV-R3-01…05, J3 |
| R2/R1 | SV-R2-01…06, SV-R1-01…03, J5 |
| MW/KIB | SV-MW-01…05 (×5 rows), J6 |
| RTP | SV-RTP-01…04 |
| BU | SV-BU-01…04, J7 |
| Submit integrity | SV-SUB-01…09, SV-ELG-01…07 |
| Staff / Admin | SV-STF-01…05, SV-ADM-01…06 |

## 7. Build order

1. **Spike X1.** Write SV-SUB-02 as a Go integration test (`TEST_DATABASE_URL`) first. It's the cheapest way to confirm or rule out the most serious suspected bug before the E2E infrastructure exists.
2. Seeder `cmd/e2eseed` + manifest + Playwright `sat-policy` project + helpers (§4).
3. L1 suites in this order: `reveal`, `final-class`, `matrix` (parameterized), `eligibility`, `submit`, `admin`.
4. L2 journeys J1–J8.
5. Get answers to Q1–Q8, then remove the `test.fixme` markers on the blocked tests.
6. CI: run `sat-policy` on PRs touching `absenceshttp/`, `satverbalpolicy/`, `leave-policy/`, `AbsenceForm.tsx`, `StaffCreateAbsenceModal.tsx`.

Proposed files:
```
backend/cmd/e2eseed/main.go
e2e/sat-verbal/helpers.ts
e2e/sat-verbal/reveal.spec.ts
e2e/sat-verbal/final-class.spec.ts
e2e/sat-verbal/matrix.spec.ts
e2e/sat-verbal/eligibility.spec.ts
e2e/sat-verbal/submit.spec.ts
e2e/sat-verbal/timing-48h.spec.ts
e2e/sat-verbal/staff.spec.ts
e2e/sat-verbal/admin-mapping.spec.ts
e2e/sat-verbal/journeys.spec.ts
```

## 8. Exit criteria

- Every rule in §1 is covered by at least one passing L1 test and one L2 journey.
- `SV-FIN-01` passes for every seeded row at every level.
- X1–X7 are each either fixed, or confirmed as intended with the test updated and the decision written down.
- Q1–Q8 are answered, and no `test.fixme` is left in `e2e/sat-verbal/`.
- The suite passes 3 runs in a row with no retries (flake check), on chromium-desktop, chromium-phone-standard, and webkit-phone.
