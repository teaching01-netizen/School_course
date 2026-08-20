# Legacy sync: (1) archived courses sync once then skip, (2) refresh path self-applies teacher/subject master data

## Behavior (what will change)

**A. Archived courses: sync once, then skip.**
Today the leader sweep (`runner.go:141-156`) enqueues a `legacy_refresh_course` job for EVERY linked course every ~30s — all 5,352 live courses, incl. 5,061 archived. "Already synced once" = `courses.legacy_last_synced_at IS NOT NULL` (set only by a real, non-shadow successful apply; never in shadow mode or on failure). Skip rule applied at three points:
1. **Sweeper filter** — `listLinkedLegacyCourses` (main.go:35) excludes `legacy_archived AND legacy_last_synced_at IS NOT NULL` → archived courses stop being enqueued every sweep (~94% of sweep load gone).
2. **Sync guard** — the per-course sync checks the same predicate up front and returns nil **before any fetch** (no old-site HTTP request). Covers jobs enqueued by reconcile, admin refresh, and the course-detail "Sync legacy" button (button on an already-synced archived course becomes a no-op — per your requirement; a force flag can be added later if wanted).
3. **Reconcile** — `full.go` stops enqueuing refresh jobs for courses that are archived **upstream** AND already synced, and the archived mirror becomes **bidirectional** (`legacy_status` + `legacy_archived` set from upstream status both ways), so a course un-archived on the old site is flipped back to active and re-enqueued (edge: between full reconciles the sweeper may skip a just-un-archived course; the next reconcile fixes it — documented trade-off).

**B. Refresh path auto-applies teacher + subject master data** (the fix I offered, which you approved).
Today `SyncCourse` resolves teacher/subject via `external_refs` and **fails** with "referenced master data is missing" (→ retry → dead letter after 5) if no full reconcile ran yet. Change: before applying the course, unconditionally call `masterData.ApplyTeacher` / `ApplySubject` (both idempotent via snapshot-hash fast path, advisory-locked, shadow-aware — exactly like the existing room loop) using the teacher/subject records from the cached course-list index (which today is discarded: `loadCourseIndex` keeps only courses). Unknown-to-catalogue ids keep today's retry behavior (self-heals when a full reconcile catches up).

## Tests first (RED → GREEN, scratch DB only — never `warwick`)

New/extended files (existing harness patterns: `TEST_DATABASE_URL` skip, goose-once, uuid/time suffixes for self-scoping):
1. `cmd/legacy-sync/main_test.go` — pure unit: skip predicate (archived+synced→skip; archived+unsynced→sync; active+synced→sync) and index builder (courses/teachers/subjects maps from `CourseListResult`).
2. `cmd/legacy-sync/main_integration_test.go` — `listLinkedLegacyCourses` returns only not-(archived+synced) linked ids.
3. `cmd/legacy-sync/syncer_integration_test.go` (new) — extract the per-course orchestration from the `main()` closure into a testable `courseSyncer` type (`cmd/legacy-sync/syncer.go`, same package, wiring unchanged). With an httptest fake legacy site (login + list + detail pages, atomic request counters) + scratch DB:
   - archived+synced course → `SyncCourse` makes **zero** detail requests;
   - archived+unsynced course → syncs once (one request), sets `legacy_last_synced_at`, second call skips;
   - active course with teacher/subject never mapped → sync **succeeds** (today: fails with `ErrMissingReference`), creates `legacy:<tid>` user + subject + `external_refs`, sets `teacher_id`/`subject_id`;
   - active course still refreshes on every call (no skip).
4. `internal/legacysync/reconcile/full_integration_test.go` — archived+synced course not re-enqueued (`stats.Enqueued`), and upstream-active course with stale local `legacy_archived=true` gets mirrored back to active + enqueued.

## Implementation (after RED confirmed)

- `cmd/legacy-sync/syncer.go` (new): `courseSyncer` struct holding pool/client/master/appliers/index cache/TZ/log; `syncCourse(ctx, legacyID)` = current closure body + (A2) skip guard + (B) teacher/subject auto-apply; `loadCourseIndex` returns `courseIndex{courses, teachers, subjects}` maps, same 5-min TTL cache.
- `cmd/legacy-sync/main.go`: `listLinkedLegacyCourses` SQL gains `AND NOT (legacy_archived AND legacy_last_synced_at IS NOT NULL)`; `findLinkedLegacyCourse` also returns `legacy_archived` + `legacy_last_synced_at` (test in file 1 covers the new signature); `main()` constructs the syncer and passes its method as the `SyncCourse` callback. No other wiring changes.
- `internal/legacysync/reconcile/full.go`: `linkCourse` returns `legacy_last_synced_at` from the already-linked SELECT; loop skips the refresh enqueue when `course.Status == "archived" && lastSynced.Valid`; the archived-only mirror becomes bidirectional (`legacy_status=$2, legacy_archived=($2='archived')`, idempotent WHERE). Roster import intentionally unchanged (add-only, no network, out of scope).
- No migrations (uses existing `legacy_archived`, `legacy_last_synced_at`), no frontend changes, no API changes, no C1/playwright changes.

## Verification

1. RED confirmed for each new test, then GREEN.
2. Scratch DB suite: `go test ./cmd/legacy-sync/... ./internal/legacysync/... -p 1 -count=1`; then full repo suite `-p 1 -count=1` (drop scratch DB after).
3. `go build ./...`, `go vet ./...`, gofmt clean.
4. Rebuild the worker binary → `/tmp/legacy-sync-new`, restart the local worker (same env as before: `.env` + `DATABASE_URL`/`REALTIME_DATABASE_URL` → `warwick_local_fresh`, fresh `OTP_HMAC_KEY`). Verify live: sweep enqueues only active courses (jobs for archived ids stop), a manual refresh of an archived course completes as a skip (worker log line), un-archived/active behavior unchanged.

## Trade-offs (accepted)

- A course with a `partial` snapshot (exclusion-skipped sessions) that is archived+synced will not auto-retry the skipped rows (frozen data; can be forced later via a force flag).
- Archived→active transition on the old site takes effect at the next full reconcile (mirror + enqueue), not the next 30s sweep.