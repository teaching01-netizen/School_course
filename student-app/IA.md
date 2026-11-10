# Warwick Student PWA — IA (v1, clickable structure first)

Source: Rounds 1–8 approved. Separate PWA, calls this repo's API via future `/api/v2` Bearer. English-first, Asia/Bangkok, Warwick navy/blue. Home clones the reference super-app stacking.

## Auth (EDITED: no OTP — wcode + parent phone direct)

- `Auth / Sign in`: two fields on one screen — `Student Code (WCODE)` + `Parent Phone` — + `Sign In` CTA. No OTP step.
- Success → Home directly. Session: Bearer pair (24h max / 45m idle).

## App shell

- Mobile-first PWA, content max ~430px phone column centered, page cap 1100px on desktop.
- Header: search (courses+items+banners) + points pill (live balance) + bell (unread from active banners).
- Bottom nav (5): Home | Courses | [Center Check-in] | Scores | Profile. Center opens check-in sheet.
- Tokens: navy `#0F172A`, primary `#2563EB` / hover `#1D4ED8`, bg `#F8FAFC`, hairline `rgba(15,23,42,0.08)`. Measures mirror reference (pill search, 4-col quick actions, overlapping wallet card, 4×2 grid, peach→slate tint cards, floating nav + raised center).

## Screens (v1)

1. **Home** (sketch order): schedule strip (today) → banner carousel (backend `banners`, static tip fallback) → 4 quick actions (Schedule, Scores, Check-in, Leave) → next-class card (replaces wallet) → 6-tile shortcuts + 8-service grid w/ badges → Score block → divider → daily check-in card (one points grant/day, disables to Checked-in) → sticky `Item Rewards` + `Courses` buttons.
2. **My Courses**: Today / Week / Month tabs. Card: time, subject, room, teacher, status. Detail: Check-in button + Request Leave link + Score link.
3. **Rewards**: points header → item grid → item detail (cost, stock, Condition) → redeem confirm sheet → success (code + balance) / 409 fail.
4. **Scores**: subject chips (SAT/IELTS) → rows (test, date, score) → detail note. Empty: “No scores yet” → courses.
5. **Tests**: open slots (seats left) → register confirm → My Registrations + Cancel (blocked <48hr). Full disables.
6. **Leave**: same logic as `/absence` (Student→Classes→Reason→Review), PWA card/sheet skin, no OTP.
7. **Check-in**: center-button sheet (today's next session, tap confirm), staff can edit/delete; late recorded not rejected. Daily card + session check-ins both write `check_ins`.

## Search / bell / View All

- Search: unified courses+items+banners.
- Bell: active banners list, unread dot.
- View All: full service sheet.

## Staff (old console, quiet style)

Lists + create/edit for points/items/banners/scores/test slots. Admin-write, Teacher-read. Not in PWA theme.

## Build order

1. Token API + CORS. 2. Six tables + staff writes. 3. Student reads (home/score/points/items/banners). 4. Test-register + check-in.
