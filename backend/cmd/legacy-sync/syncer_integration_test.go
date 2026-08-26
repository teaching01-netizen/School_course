package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync"
	"warwick-institute/internal/legacysync/apply"
	"warwick-institute/internal/schedulepolicy"
)

// digitsOnly strips everything but ASCII digits from s, keeping legacy ids
// numeric-shaped: the course list parser requires all-digit C-IDs.
func digitsOnly(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

// numericLegacyID builds a unique all-digit legacy id (the old site exposes
// plain integer ids on the course list page).
func numericLegacyID(prefix string) string {
	return prefix + digitsOnly(uuid.NewString()+strconv.FormatInt(time.Now().UnixNano(), 10))
}

// fakeLegacySite serves the minimal old-site surface the course syncer
// touches: login (GET sets the antiforgery + session cookies, POST returns
// the logged-in page), the course list (GET and the IsArchive search POST),
// and the course detail page. Request counters distinguish "fetched at all"
// from "fetched again", which is exactly what the sync-once-then-skip rule
// must not violate.
type fakeLegacySite struct {
	srv            *httptest.Server
	listRequests   atomic.Int32
	detailRequests atomic.Int32
	detailBody     atomic.Value
}

func newFakeLegacySite(t *testing.T, listBody, detailBody string) *fakeLegacySite {
	t.Helper()
	s := &fakeLegacySite{}
	s.detailBody.Store(detailBody)
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Account/Login":
			http.SetCookie(w, &http.Cookie{Name: ".AspNetCore.Antiforgery.abc", Value: "af", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "s1", Path: "/"})
			_, _ = w.Write([]byte(`<html><form action="/Account/Login" method="post"><input name="__RequestVerificationToken" value="login-token" /></form></html>`))
		case r.Method == http.MethodPost && r.URL.Path == "/Account/Login":
			_, _ = w.Write([]byte(`<html><a href="/Account/Logout">logout</a></html>`))
		case r.URL.Path == "/Admin/Courses":
			s.listRequests.Add(1)
			_, _ = w.Write([]byte(listBody))
		case r.URL.Path == "/Admin/Courses/Detail":
			s.detailRequests.Add(1)
			_, _ = w.Write([]byte(s.detailBody.Load().(string)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *fakeLegacySite) setDetailBody(body string) {
	s.detailBody.Store(body)
}

// syncerListPage builds a contract-valid /Admin/Courses page: the search
// form (SearchText + IsArchive + __RequestVerificationToken, no action
// attribute like the live page) and the 11-column course table.
func syncerListPage(rows ...string) string {
	body := "<!DOCTYPE html><html><head><title>Course</title></head><body>" +
		`<form method="post"><input type="text" name="SearchText" value="" />` +
		`<input type="checkbox" name="IsArchive" value="true" />` +
		`<input name="__RequestVerificationToken" type="hidden" value="search-token-1" /></form>` +
		`<table class="table"><thead><tr><th>C-ID</th><th>C-Code</th><th>Year</th><th>Teacher</th><th>Subject</th><th>Hour</th><th>Student</th><th>Expired</th><th>Type</th><th>Status</th><th></th></tr></thead><tbody>`
	for _, row := range rows {
		body += row
	}
	return body + "</tbody></table></body></html>"
}

func syncerListRow(id, code, teacherCell, subjectCell, status string) string {
	return "<tr><td>" + id + "</td><td>" + code + "</td><td>2026</td><td>" + teacherCell +
		"</td><td>" + subjectCell + "</td><td>40</td><td>0</td><td></td><td>Private</td><td>" + status +
		"</td><td><a href=\"/Admin/Courses/Detail?id=" + id + "\">detail</a></td></tr>"
}

// syncerDetailPage builds a contract-valid schedule table (7 headers, one
// schedule row without a source-exposed identity, like older page formats).
func syncerDetailPage() string {
	return "<!DOCTYPE html><html><body><table class=\"table\"><thead><tr><th>Date</th><th>Begin</th><th>End</th><th>Duration</th><th>Classroom</th><th>Confirm</th><th>By</th></tr></thead><tbody>" +
		"<tr><td>Sat 23 May 26</td><td>13:00</td><td>16:20</td><td>03:20</td><td></td><td>Yes</td><td>AJ. TY</td></tr>" +
		"</tbody></table></body></html>"
}

func syncerAssignedRoomDetailPage(scheduleID, roomID string) string {
	return "<!DOCTYPE html><html><body><table class=\"table\"><thead><tr><th>Date</th><th>Begin</th><th>End</th><th>Duration</th><th>Classroom</th><th>Confirm</th><th>By</th></tr></thead><tbody>" +
		"<tr><td>Sat 23 May 26</td><td>13:00</td><td>16:20</td><td>03:20</td><td><a href=\"ClassroomSet?courseScheduleId=" + scheduleID + "\">[" + roomID + "] Room</a></td><td>Yes</td><td>AJ. TY</td></tr>" +
		"</tbody></table></body></html>"
}

// newSyncerUnderTest wires a courseSyncer to the fake site and a scratch
// pool with live-apply controls (shadow off), mirroring the deployment's
// wiring (same master data source for the syncer and the course applier so
// the refresh path's self-applied master data resolves).
func newSyncerUnderTest(t *testing.T, pool *pgxpool.Pool, srv *httptest.Server) *courseSyncer {
	t.Helper()
	// Pacing is covered by the client's own tests; keep these tests fast.
	t.Setenv("LEGACY_SYNC_MIN_REQUEST_INTERVAL", "0")
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE legacy_sync_controls SET shadow_mode = false, realtime_enabled = false`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE legacy_sync_controls SET shadow_mode = true, realtime_enabled = true`)
	})
	client, err := legacysync.NewClient(srv.URL, "user", "pass", legacysync.WithMaxBodyBytes(16<<20))
	if err != nil {
		t.Fatal(err)
	}
	source := "syncer_" + uuid.NewString()
	q := sqldb.New(pool)
	master := apply.NewMasterDataService(pool, q, source)
	policyReader := schedulepolicy.NewDBReader()
	courseApp := apply.NewCourseApplier(pool, q, source, policyReader)
	scheduleApp := apply.NewScheduleApplier(pool, q, source, policyReader)
	return newCourseSyncer(pool, q, client, master, courseApp, scheduleApp, "Asia/Bangkok",
		slog.New(slog.NewTextHandler(io.Discard, nil)), 1, 30*time.Minute)
}

// seedLinkedCourse inserts a local course linked to a legacy id with the
// given archive and one-time-sync state.
func seedLinkedCourse(t *testing.T, pool *pgxpool.Pool, code, legacyID string, archived, synced bool) {
	t.Helper()
	var lastSynced any
	if synced {
		lastSynced = time.Now().UTC()
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO courses (code, name, legacy_course_id, source_kind, legacy_archived, legacy_last_synced_at) VALUES ($1, 'Legacy course', $2, 'legacy', $3, $4)`, code, legacyID, archived, lastSynced); err != nil {
		t.Fatal(err)
	}
}

// TestCourseSyncer_SkipsArchivedCourseAlreadySynced pins the "sync once,
// then skip" guard at the per-course level: an archived course that already
// synced makes NO source request at all — not even the course list — so the
// sweep, reconcile jobs, and the manual refresh button all become no-ops
// for it.
func TestCourseSyncer_SkipsArchivedCourseAlreadySynced(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	legacyID := numericLegacyID("8")
	teacherID := numericLegacyID("9")
	subjectID := numericLegacyID("7")
	code := "SYNC-SKIP-" + uuid.NewString()
	seedLinkedCourse(t, pool, code, legacyID, true, true)

	site := newFakeLegacySite(t, syncerListPage(syncerListRow(legacyID, code, "["+teacherID+"] T", "["+subjectID+"] S", "Archived")), syncerDetailPage())
	syncer := newSyncerUnderTest(t, pool, site.srv)

	if err := syncer.syncCourse(ctx, legacyID); err != nil {
		t.Fatalf("syncCourse: %v", err)
	}
	if got := site.detailRequests.Load(); got != 0 {
		t.Fatalf("detail page fetches = %d, want 0 (already-synced archived course must not fetch)", got)
	}
	if got := site.listRequests.Load(); got != 0 {
		t.Fatalf("course list fetches = %d, want 0 (skip must happen before any source fetch)", got)
	}
}

// TestCourseSyncer_SyncsArchivedCourseOnceThenSkips pins the full lifecycle
// of the rule: an archived course that has NEVER synced gets exactly one
// refresh (which marks legacy_last_synced_at via the real apply), and the
// next sync call skips it entirely.
func TestCourseSyncer_SyncsArchivedCourseOnceThenSkips(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	legacyID := numericLegacyID("8")
	teacherID := numericLegacyID("9")
	subjectID := numericLegacyID("7")
	code := "SYNC-ARCH-" + uuid.NewString()
	seedLinkedCourse(t, pool, code, legacyID, true, false)

	site := newFakeLegacySite(t, syncerListPage(syncerListRow(legacyID, code, "["+teacherID+"] T", "["+subjectID+"] S", "Archived")), syncerDetailPage())
	syncer := newSyncerUnderTest(t, pool, site.srv)

	if err := syncer.syncCourse(ctx, legacyID); err != nil {
		t.Fatalf("first syncCourse: %v", err)
	}
	if got := site.detailRequests.Load(); got != 1 {
		t.Fatalf("detail page fetches after first sync = %d, want 1", got)
	}
	var lastSynced pgtype.Timestamptz
	if err := pool.QueryRow(ctx, `SELECT legacy_last_synced_at FROM courses WHERE legacy_course_id = $1`, legacyID).Scan(&lastSynced); err != nil {
		t.Fatal(err)
	}
	if !lastSynced.Valid {
		t.Fatal("archived course was not marked as synced after its one-time sync")
	}
	var mirrored bool
	if err := pool.QueryRow(ctx, `SELECT legacy_archived AND legacy_status = 'archived' FROM courses WHERE legacy_course_id = $1`, legacyID).Scan(&mirrored); err != nil {
		t.Fatal(err)
	}
	if !mirrored {
		t.Fatal("archived course not mirrored (legacy_archived / legacy_status should be archived)")
	}

	if err := syncer.syncCourse(ctx, legacyID); err != nil {
		t.Fatalf("second syncCourse: %v", err)
	}
	if got := site.detailRequests.Load(); got != 1 {
		t.Fatalf("detail page fetches after second sync = %d, want still 1 (second sync must skip)", got)
	}
}

// TestCourseSyncer_AppliesUnknownTeacherAndSubject pins the refresh-path
// fix: a targeted refresh of a course whose teacher/subject were never
// mapped (e.g. queued before a full reconcile ran) succeeds by applying the
// master data from the course index itself — creating the legacy teacher
// user, the subject, and the external_refs mappings — instead of failing
// with "referenced master data is missing".
func TestCourseSyncer_AppliesUnknownTeacherAndSubject(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	legacyID := numericLegacyID("8")
	teacherID := numericLegacyID("9")
	subjectID := numericLegacyID("7")
	code := "SYNC-MASTER-" + uuid.NewString()
	seedLinkedCourse(t, pool, code, legacyID, false, false)

	site := newFakeLegacySite(t, syncerListPage(syncerListRow(legacyID, code, "["+teacherID+"] Somchai", "["+subjectID+"] Maths", "Active")), syncerDetailPage())
	syncer := newSyncerUnderTest(t, pool, site.srv)

	if err := syncer.syncCourse(ctx, legacyID); err != nil {
		t.Fatalf("syncCourse with never-mapped teacher/subject: %v (the refresh must self-apply master data)", err)
	}

	var teacherUserID pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, "legacy:"+teacherID).Scan(&teacherUserID); err != nil {
		t.Fatalf("legacy teacher user not created: %v", err)
	}
	var subjectRowID pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM subjects WHERE code = $1`, "legacy:"+subjectID).Scan(&subjectRowID); err != nil {
		t.Fatalf("legacy subject row not created: %v", err)
	}
	for _, entityType := range []string{"teacher", "subject"} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_refs WHERE entity_type = $1 AND external_id = $2`, entityType, map[string]string{"teacher": teacherID, "subject": subjectID}[entityType]).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("external_refs for %s = %d, want 1", entityType, count)
		}
	}
	var subjectIDOnCourse, teacherIDOnCourse pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT subject_id, teacher_id FROM courses WHERE legacy_course_id = $1`, legacyID).Scan(&subjectIDOnCourse, &teacherIDOnCourse); err != nil {
		t.Fatal(err)
	}
	if !subjectIDOnCourse.Valid || !teacherIDOnCourse.Valid {
		t.Fatalf("course teacher/subject not linked after sync (teacher=%v subject=%v)", teacherIDOnCourse.Valid, subjectIDOnCourse.Valid)
	}
	if teacherIDOnCourse != teacherUserID {
		t.Fatalf("course teacher_id = %v, want the auto-created legacy user %v", teacherIDOnCourse, teacherUserID)
	}
}

// TestCourseSyncer_RefreshesActiveCourseOnEverySync pins the R-004 fallback
// path: without an ok-quality snapshot under the production source
// (legacy_warwick), an active course past its archived-skip guard still
// fetches on every sync. The applier in this fixture writes snapshots under
// a scratch source (see newSyncerUnderTest), so the cooldown gate sees no
// legacy_warwick snapshot and falls through to the network; the gated path
// itself is pinned by TestCourseSyncer_WithinCooldownSkipsDetailFetch.
func TestCourseSyncer_RefreshesActiveCourseOnEverySync(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	legacyID := numericLegacyID("8")
	teacherID := numericLegacyID("9")
	subjectID := numericLegacyID("7")
	code := "SYNC-ACTIVE-" + uuid.NewString()
	seedLinkedCourse(t, pool, code, legacyID, false, true)

	site := newFakeLegacySite(t, syncerListPage(syncerListRow(legacyID, code, "["+teacherID+"] T", "["+subjectID+"] S", "Active")), syncerDetailPage())
	syncer := newSyncerUnderTest(t, pool, site.srv)

	if err := syncer.syncCourse(ctx, legacyID); err != nil {
		t.Fatal(err)
	}
	if err := syncer.syncCourse(ctx, legacyID); err != nil {
		t.Fatal(err)
	}
	if got := site.detailRequests.Load(); got != 2 {
		t.Fatalf("detail page fetches = %d, want 2 (no legacy_warwick snapshot, cooldown falls through)", got)
	}
}

func TestCourseSyncer_RoomAssignmentKeepsExistingSessionWhenSourceIDAppears(t *testing.T) {
	// Given: the first source response has no room and therefore no schedule ID.
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	legacyID := numericLegacyID("8")
	teacherID := numericLegacyID("9")
	subjectID := numericLegacyID("7")
	roomID := numericLegacyID("6")
	scheduleID := numericLegacyID("5")
	code := "SYNC-ROOM-" + uuid.NewString()
	seedLinkedCourse(t, pool, code, legacyID, false, false)
	site := newFakeLegacySite(t, syncerListPage(syncerListRow(legacyID, code, "["+teacherID+"] T", "["+subjectID+"] S", "Active")), syncerDetailPage())
	syncer := newSyncerUnderTest(t, pool, site.srv)
	if err := syncer.syncCourse(ctx, legacyID); err != nil {
		t.Fatal(err)
	}
	var originalSessionID pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM sessions WHERE course_id=(SELECT id FROM courses WHERE legacy_course_id=$1) AND legacy_schedule_id LIKE 'derived:%'`, legacyID).Scan(&originalSessionID); err != nil {
		t.Fatal(err)
	}

	// When: the source assigns a room and exposes the numeric schedule ID.
	site.setDetailBody(syncerAssignedRoomDetailPage(scheduleID, roomID))
	if err := syncer.syncCourse(ctx, legacyID); err != nil {
		t.Fatal(err)
	}

	// Then: the source ID and room are applied to the original single session.
	var syncedSessionID pgtype.UUID
	var sessionCount int
	if err := pool.QueryRow(ctx, `SELECT id FROM sessions WHERE legacy_schedule_id=$1 AND room_id IS NOT NULL`, scheduleID).Scan(&syncedSessionID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE course_id=(SELECT id FROM courses WHERE legacy_course_id=$1)`, legacyID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if syncedSessionID != originalSessionID || sessionCount != 1 {
		t.Fatalf("room transition session = %v count=%d, want original %v count=1", syncedSessionID, sessionCount, originalSessionID)
	}
}

// TestCourseSyncer_WithinCooldownSkipsDetailFetch pins the R-004 pre-fetch
// gate: with a fresh last_synced timestamp and an ok-quality snapshot under
// the production source, syncCourse returns before a single source request.
func TestCourseSyncer_WithinCooldownSkipsDetailFetch(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	legacyID := numericLegacyID("8")
	teacherID := numericLegacyID("9")
	subjectID := numericLegacyID("7")
	code := "SYNC-GATED-" + uuid.NewString()
	seedLinkedCourse(t, pool, code, legacyID, false, true) // fresh legacy_last_synced_at

	if _, err := pool.Exec(ctx, `INSERT INTO legacy_entity_snapshots
		(source, entity_type, external_id, canonical_data, source_hash, parser_version, observed_at, applied_at, quality)
		VALUES ('legacy_warwick', 'course', $1, '{}'::jsonb, 'hash-1', 1, now(), now(), 'ok')
		ON CONFLICT (source, entity_type, external_id) DO UPDATE SET
			canonical_data = EXCLUDED.canonical_data,
			source_hash    = EXCLUDED.source_hash,
			quality        = EXCLUDED.quality`, legacyID); err != nil {
		t.Fatal(err)
	}

	site := newFakeLegacySite(t, syncerListPage(syncerListRow(legacyID, code, "["+teacherID+"] T", "["+subjectID+"] S", "Active")), syncerDetailPage())
	syncer := newSyncerUnderTest(t, pool, site.srv)

	if err := syncer.syncCourse(ctx, legacyID); err != nil {
		t.Fatal(err)
	}
	if got := site.detailRequests.Load(); got != 0 {
		t.Fatalf("detail page fetches = %d, want 0 (within cooldown with ok legacy_warwick snapshot)", got)
	}
}
