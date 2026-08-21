package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/legacysync"
	"warwick-institute/internal/legacysync/apply"
	sourceclient "warwick-institute/internal/legacysync/client"
	"warwick-institute/internal/legacysync/normalize"
)

// studentSearchServer builds a legacy site that answers the student directory
// search with a one-row page for the exact wcode searched. failWcode, when
// non-empty, makes the server return a non-searchable page for that wcode so
// the per-wcode failure path (skip, not abort) is exercised. The searches
// log is mutex-guarded because the parallel profile sync issues lookups
// concurrently.
func studentSearchServer(t *testing.T, failWcode string) (*httptest.Server, *[]string) {
	t.Helper()
	var searches []string
	var mu sync.Mutex
	recordSearch := func(text string) {
		mu.Lock()
		searches = append(searches, text)
		mu.Unlock()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Account/Login":
			http.SetCookie(w, &http.Cookie{Name: ".AspNetCore.Antiforgery.abc", Value: "af", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "s1", Path: "/"})
			_, _ = w.Write([]byte(`<html><form action="/Account/Login" method="post"><input name="__RequestVerificationToken" value="login-token" /></form></html>`))
		case r.Method == http.MethodPost && r.URL.Path == "/Account/Login":
			_, _ = w.Write([]byte(`<html><a href="/Account/Logout">logout</a></html>`))
		case r.URL.Path == "/Admin/Students":
			if r.Method == http.MethodPost {
				if err := r.ParseForm(); err != nil {
					t.Errorf("parse search: %v", err)
					return
				}
				text := r.Form.Get("SearchText")
				recordSearch(text)
				if strings.EqualFold(text, failWcode) {
					w.Write([]byte(studentsDirectoryPage(failWcode, false)))
					return
				}
				w.Write([]byte(studentsDirectoryPage(text, true)))
				return
			}
			// GET: the page must carry the search form (input + antiforgery
			// token) — the client parses it before it can POST a search.
			w.Write([]byte(studentsDirectoryPage("", false)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &searches
}

// studentsDirectoryPage builds a directory page: the search form (always,
// exactly like the live page) plus, when rowOf is non-empty, a one-student
// table row for that wcode.
func studentsDirectoryPage(rowOf string, withRow bool) string {
	body := `<form method="post" enctype="multipart/form-data">
<input id="SearchText" name="SearchText" type="text" value="" />
<input name="__RequestVerificationToken" type="hidden" value="students-token-1" />
</form>`
	if withRow {
		body += `<table class="table"><thead><tr><th>W-Code</th><th>Name</th><th>Nickname</th><th>School</th><th>Level</th><th>Year</th><th>Phone</th><th>Email</th><th>Mobile</th><th></th></tr></thead><tbody>` +
			`<tr><td>` + strings.ToUpper(rowOf) + `</td><td>Alice A. Alpha</td><td>Ali</td><td>Bangkok Prep</td><td>G1</td><td>2026</td><td>081-0000008</td><td>alice@example.com</td><td>False</td><td><a href="/Admin/Students/Schedule?studentId=` + strings.ToUpper(rowOf) + `">schedule</a></td></tr>` +
			`</tbody></table>`
	}
	return `<html><head><title>Student</title></head><body>` + body + `</body></html>`
}

func newStudentSyncer(t *testing.T, pool *pgxpool.Pool, srv *httptest.Server) *courseSyncer {
	t.Helper()
	// Pacing is covered by the client's own tests; keep these tests fast.
	t.Setenv("LEGACY_SYNC_MIN_REQUEST_INTERVAL", "0")
	client, err := legacysync.NewClient(srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	q := sqldb.New(pool)
	log := slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelWarn}))
	// Sequential profile lookups (1 worker) keep these tests deterministic.
	return newCourseSyncer(pool, q, client, apply.NewMasterDataService(pool, q, "studenttest"), nil, nil, "Asia/Bangkok", log, 1, 30*time.Minute)
}

func TestSyncStudentProfiles_FetchesOneRowPerKnownWCode(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	cleanupStudentRows(t, pool)
	t.Cleanup(func() { cleanupStudentRows(t, pool) })

	srv, searches := studentSearchServer(t, "")
	syncer := newStudentSyncer(t, pool, srv)

	suffix := digitsOnly(fmt.Sprintf("%d", time.Now().UnixNano()))
	wcodeA := "W7001" + suffix
	wcodeB := "W7002" + suffix
	if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ($1, 'A Student', ''), ($2, 'B Student', '')`, wcodeA, wcodeB); err != nil {
		t.Fatal(err)
	}
	// A non-W-code student (e.g. a placeholder) must be skipped — the legacy
	// directory has no key for it and the search would be noise.
	if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ('nonsense', 'No code', '')`); err != nil {
		t.Fatal(err)
	}

	profiles, err := syncer.syncStudentProfiles(ctx)
	if err != nil {
		t.Fatalf("syncStudentProfiles: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("profiles = %d, want 2 (one per W-code student)", len(profiles))
	}
	byCode := map[string]normalize.LegacyStudent{}
	for _, p := range profiles {
		byCode[p.WCode] = p
	}
	for _, code := range []string{wcodeA, wcodeB} {
		p, ok := byCode[code]
		if !ok {
			t.Fatalf("profile for %s missing from %v", code, byCode)
		}
		if p.Name != "Alice A. Alpha" || p.Nickname != "Ali" || p.School != "Bangkok Prep" || p.Level != "G1" || p.Year != "2026" || p.Phone != "081-0000008" {
			t.Fatalf("profile for %s = %+v, want directory values", code, p)
		}
	}

	// Exactly one search per known W-code; the placeholder was never queried.
	if len(*searches) != 2 {
		t.Fatalf("searches = %v, want exactly 2 (only W-code students)", *searches)
	}
}

func TestSyncStudentProfiles_SearchFailuresAreSkippedNotFatal(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	cleanupStudentRows(t, pool)
	t.Cleanup(func() { cleanupStudentRows(t, pool) })

	suffix := digitsOnly(fmt.Sprintf("%d", time.Now().UnixNano()))
	goodWcode := "W7011" + suffix
	badWcode := "W7012" + suffix
	srv, _ := studentSearchServer(t, badWcode)
	syncer := newStudentSyncer(t, pool, srv)

	if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ($1, 'Good', ''), ($2, 'Bad', '')`, goodWcode, badWcode); err != nil {
		t.Fatal(err)
	}

	profiles, err := syncer.syncStudentProfiles(ctx)
	if err != nil {
		t.Fatalf("syncStudentProfiles: %v (a failing single search must not abort the run)", err)
	}
	// The lookup for badWcode vanished from the directory ("No students yet.")
	// — it is skipped, and the good student still syncs.
	if len(profiles) != 1 || profiles[0].WCode != goodWcode {
		t.Fatalf("profiles = %+v, want exactly the good student %s", profiles, goodWcode)
	}
}

// TestSyncStudentProfiles_ParallelWorkersCollectEveryProfile pins the
// parallel lookup path: a worker pool (here 4) must query every known wcode
// exactly once, return every matched profile, and report progress through
// the callback with monotonic processed counts that end at the exact totals —
// the same guarantees the sequential path had, under concurrency.
func TestSyncStudentProfiles_ParallelWorkersCollectEveryProfile(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	cleanupStudentRows(t, pool)
	t.Cleanup(func() { cleanupStudentRows(t, pool) })

	srv, searches := studentSearchServer(t, "")
	syncer := newStudentSyncer(t, pool, srv)
	syncer.studentProfileWorkers = 4

	suffix := digitsOnly(fmt.Sprintf("%d", time.Now().UnixNano()))
	var wcodes []string
	for i := 0; i < 8; i++ {
		wcodes = append(wcodes, "W70"+fmt.Sprintf("%02d", i)+suffix)
	}
	for _, wcode := range wcodes {
		if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ($1, 'Student', '')`, wcode); err != nil {
			t.Fatal(err)
		}
	}

	var updates []StudentProfileProgress
	profiles, err := syncer.syncStudentProfiles(ctx, func(update StudentProfileProgress) error {
		updates = append(updates, update)
		return nil
	})
	if err != nil {
		t.Fatalf("syncStudentProfiles: %v", err)
	}
	if len(profiles) != len(wcodes) {
		t.Fatalf("profiles = %d, want %d (one per W-code student)", len(profiles), len(wcodes))
	}
	byCode := map[string]normalize.LegacyStudent{}
	for _, p := range profiles {
		byCode[p.WCode] = p
	}
	for _, code := range wcodes {
		if _, ok := byCode[code]; !ok {
			t.Fatalf("profile for %s missing from %v", code, wcodes)
		}
	}
	if got := len(*searches); got != len(wcodes) {
		t.Fatalf("searches = %d, want %d (exactly one lookup per W-code)", got, len(wcodes))
	}
	if len(updates) != len(wcodes) {
		t.Fatalf("progress updates = %d, want one per wcode", len(updates))
	}
	for i, update := range updates {
		if i > 0 && update.Processed <= updates[i-1].Processed {
			t.Fatalf("progress Processed not monotonic: %v", updates)
		}
	}
	last := updates[len(updates)-1]
	if last.Processed != len(wcodes) || last.Total != len(wcodes) || last.ProfilesFound != len(wcodes) || last.Failures != 0 {
		t.Fatalf("final progress = %+v, want processed=%d total=%d found=%d failures=0", last, len(wcodes), len(wcodes), len(wcodes))
	}
}

func TestListStudentWcodes_FiltersToDirectoryShape(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	cleanupStudentRows(t, pool)
	t.Cleanup(func() { cleanupStudentRows(t, pool) })
	deadSrv := httptest.NewServer(http.NotFoundHandler())
	defer deadSrv.Close()
	syncer := newStudentSyncer(t, pool, deadSrv)

	for _, wcode := range []string{"W8001", "w8002", "W8003", "nonsense 123"} {
		if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ($1, 'x', '')`, wcode); err != nil {
			t.Fatal(err)
		}
	}

	wcodes, err := syncer.listStudentWcodes(ctx)
	if err != nil {
		t.Fatalf("listStudentWcodes: %v", err)
	}
	want := map[string]bool{"W8001": true, "W8002": true, "W8003": true}
	for _, w := range wcodes {
		if !want[w] {
			t.Fatalf("listStudentWcodes returned %q, want only {W8001 W8002 W8003}", w)
		}
	}
	if len(wcodes) != 3 {
		t.Fatalf("listStudentWcodes = %v, want exactly 3 distinct W-codes", wcodes)
	}
}

// studentRateLimitedServer answers the student search with HTTP 429, carrying
// a text/html content type exactly like the live site (transport.go rejects
// non-HTML replies with the skip-class ErrUnexpectedContentType before it ever
// sees the status code). Login flows normally so the profile phase is what
// hits the rate limit.
func studentRateLimitedServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Account/Login":
			http.SetCookie(w, &http.Cookie{Name: ".AspNetCore.Antiforgery.abc", Value: "af", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "s1", Path: "/"})
			_, _ = w.Write([]byte(`<html><form action="/Account/Login" method="post"><input name="__RequestVerificationToken" value="login-token" /></form></html>`))
		case r.Method == http.MethodPost && r.URL.Path == "/Account/Login":
			_, _ = w.Write([]byte(`<html><a href="/Account/Logout">logout</a></html>`))
		case r.URL.Path == "/Admin/Students":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`<html><body>Too Many Requests</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestSyncStudentProfiles_SystemicRateLimitAbortsRun pins the profile-phase
// systemic guard: when the source rate-limits a lookup (429), the whole phase
// must fail with an error carrying the rate-limit sentinel instead of
// silently skipping that wcode and reporting success — otherwise every
// reconcile would keep hammering a throttled source with a green run record.
func TestSyncStudentProfiles_SystemicRateLimitAbortsRun(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	cleanupStudentRows(t, pool)
	t.Cleanup(func() { cleanupStudentRows(t, pool) })

	srv := studentRateLimitedServer(t)
	syncer := newStudentSyncer(t, pool, srv)

	suffix := digitsOnly(fmt.Sprintf("%d", time.Now().UnixNano()))
	if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ($1, 'Rate Limited', '')`, "W7021"+suffix); err != nil {
		t.Fatal(err)
	}

	profiles, err := syncer.syncStudentProfiles(ctx)
	if err == nil {
		t.Fatal("syncStudentProfiles: want error when the source rate limits, got nil")
	}
	if !errors.Is(err, sourceclient.ErrRateLimited) {
		t.Fatalf("syncStudentProfiles error = %v, want an error carrying the rate-limit sentinel", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("profiles = %d, want 0 on abort", len(profiles))
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	s := strings.TrimSpace(string(p))
	if s != "" {
		w.t.Log(s)
	}
	return len(p), nil
}

// cleanupStudentRows resets the student-side tables so repeated runs of this
// package against a shared test database start from a clean slate. TRUNCATE
// CASCADE clears referencing rows (enrollments, absences, refs) in one go.
func cleanupStudentRows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE students CASCADE`); err != nil {
		t.Logf("cleanup student rows: %v", err)
	}
}
