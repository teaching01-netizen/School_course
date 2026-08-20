package legacysync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// formCacheServer counts page reads (GET /Admin/Students) and search
// submissions (POST /Admin/Students) separately so tests can assert exactly
// when the cached search form is reused. failPosts makes every POST fail with
// a 500 once turned on, to exercise the drop-cache-and-refresh heal path.
type formCacheServer struct {
	srv       *httptest.Server
	pageGets  atomic.Int32
	postCount atomic.Int32
	failPosts atomic.Bool
}

func newFormCacheServer(t *testing.T) *formCacheServer {
	t.Helper()
	s := &formCacheServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Account/Login":
			http.SetCookie(w, &http.Cookie{Name: ".AspNetCore.Antiforgery.abc", Value: "af", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "s1", Path: "/"})
			_, _ = w.Write([]byte(`<html><form action="/Account/Login" method="post"><input name="__RequestVerificationToken" value="login-token" /></form></html>`))
		case r.Method == http.MethodPost && r.URL.Path == "/Account/Login":
			_, _ = w.Write([]byte(`<html><a href="/Account/Logout">logout</a></html>`))
		case r.Method == http.MethodGet && r.URL.Path == "/Admin/Students":
			s.pageGets.Add(1)
			_, _ = w.Write([]byte(studentsPageBody(true)))
		case r.Method == http.MethodPost && r.URL.Path == "/Admin/Students":
			if s.failPosts.Load() {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			s.postCount.Add(1)
			_, _ = w.Write([]byte(`<html><body>STUDENT ROWS</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// The search form's antiforgery token is session-stable, so a second search
// must reuse the cached form: one page read, two search POSTs. This is what
// turns the student directory sync from 2N requests into N+1.
func TestClient_SearchStudentsPageContext_ReusesCachedForm(t *testing.T) {
	srv := newFormCacheServer(t)
	client, err := NewClient(srv.srv.URL, "user", "pass")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	for _, wcode := range []string{"W250025", "W250026"} {
		page, err := client.SearchStudentsPageContext(context.Background(), wcode)
		if err != nil {
			t.Fatalf("SearchStudentsPageContext(%s): %v", wcode, err)
		}
		if !strings.Contains(page, "STUDENT ROWS") {
			t.Fatalf("search %s returned %q, want the search response", wcode, page)
		}
	}
	if got := srv.pageGets.Load(); got != 1 {
		t.Fatalf("students page reads = %d, want 1 (form must be cached)", got)
	}
	if got := srv.postCount.Load(); got != 2 {
		t.Fatalf("search submissions = %d, want 2", got)
	}
}

// A re-login re-issues the antiforgery cookie, so previously parsed tokens
// are invalid: the cache must be dropped and the page read again.
func TestClient_SearchStudentsPageContext_RefetchesFormAfterRelogin(t *testing.T) {
	srv := newFormCacheServer(t)
	client, err := NewClient(srv.srv.URL, "user", "pass")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.SearchStudentsPageContext(context.Background(), "W250025"); err != nil {
		t.Fatalf("first search: %v", err)
	}
	if err := client.source.Login(context.Background()); err != nil {
		t.Fatalf("forced re-login: %v", err)
	}
	if _, err := client.SearchStudentsPageContext(context.Background(), "W250026"); err != nil {
		t.Fatalf("second search: %v", err)
	}
	if got := srv.pageGets.Load(); got != 2 {
		t.Fatalf("students page reads = %d, want 2 (re-login must invalidate the cached form)", got)
	}
}

// A failed search drops the cached form and retries once with a freshly read
// page, so a stale token heals instead of silently losing students.
func TestClient_SearchStudentsPageContext_RefetchesFormAfterFailure(t *testing.T) {
	srv := newFormCacheServer(t)
	client, err := NewClient(srv.srv.URL, "user", "pass")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.SearchStudentsPageContext(context.Background(), "W250025"); err != nil {
		t.Fatalf("first search: %v", err)
	}
	srv.failPosts.Store(true)
	if _, err := client.SearchStudentsPageContext(context.Background(), "W250026"); err == nil {
		t.Fatal("search against a failing endpoint must error")
	}
	srv.failPosts.Store(false)
	if _, err := client.SearchStudentsPageContext(context.Background(), "W250027"); err != nil {
		t.Fatalf("search after failure healed: %v", err)
	}
	// First call: 1 read + 1 POST. Failed call: drop + 1 read + 2 failed
	// POSTs (original + retry). Healed call: cached fresh form, 1 POST only —
	// the page was read exactly 2 times total and 2 searches succeeded.
	if got := srv.pageGets.Load(); got != 2 {
		t.Fatalf("students page reads = %d, want 2 (one initial, one after the failure)", got)
	}
	if got := srv.postCount.Load(); got != 2 {
		t.Fatalf("successful search submissions = %d, want 2", got)
	}
}

// The per-host keep-alive capacity must follow the concurrency knob, so
// parallel scrapes reuse connections instead of paying a TLS handshake per
// request.
func TestNewClient_IdleConnectionsMatchConcurrency(t *testing.T) {
	t.Setenv("LEGACY_SYNC_MAX_CONCURRENT", "4")
	client, err := NewClient("https://example.com", "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.httpClient.Transport)
	}
	if transport.MaxIdleConnsPerHost != 4 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want 4 (the concurrency cap)", transport.MaxIdleConnsPerHost)
	}
}
