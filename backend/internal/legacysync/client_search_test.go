package legacysync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// courseListPageBody builds a /Admin/Courses body. includeSearchForm
// controls whether the page carries the course search form. The rendered
// form has NO action attribute, exactly like the live old site; withAction
// adds the legacy action="/Admin/Courses?handler=search" attribute so both
// detection paths stay covered.
func courseListPageBody(includeSearchForm, withAction bool) string {
	form := ""
	if includeSearchForm {
		form = `<form method="post">
<input type="text" name="SearchText" value="" />
<input type="checkbox" data-val="true" data-val-required="The Archive field is required." id="IsArchive" name="IsArchive" value="true" />
<input name="__RequestVerificationToken" type="hidden" value="search-token-1" />
</form>`
		if withAction {
			form = strings.Replace(form, `<form method="post">`, `<form action="/Admin/Courses?handler=search" method="post">`, 1)
		}
	}
	return "<!DOCTYPE html><html><head><title>Course</title></head><body>" + form + "<p>course list</p></body></html>"
}

// legacySiteServer stands up a minimal old-site: the login flow, the
// course list GET, and the course search POST. It records how many search
// submissions arrived and the exact query + form of the last one.
func legacySiteServer(t *testing.T, listBody string) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var searchPosts atomic.Int32
	var lastSubmission atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Account/Login":
			http.SetCookie(w, &http.Cookie{Name: ".AspNetCore.Antiforgery.abc", Value: "af", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "s1", Path: "/"})
			_, _ = w.Write([]byte(`<html><form action="/Account/Login" method="post"><input name="__RequestVerificationToken" value="login-token" /></form></html>`))
		case r.Method == http.MethodPost && r.URL.Path == "/Account/Login":
			_, _ = w.Write([]byte(`<html><a href="/Account/Logout">logout</a></html>`))
		case r.Method == http.MethodGet && r.URL.Path == "/Admin/Courses":
			_, _ = w.Write([]byte(listBody))
		case r.Method == http.MethodPost && r.URL.Path == "/Admin/Courses":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse search submission: %v", err)
				return
			}
			searchPosts.Add(1)
			lastSubmission.Store(r.URL.RawQuery + "|" + r.Form.Encode())
			_, _ = w.Write([]byte(`<html><body>ARCHIVED LIST</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &searchPosts, &lastSubmission
}

func assertSearchSubmission(t *testing.T, lastSubmission *atomic.Value, query string, formParts ...string) {
	t.Helper()
	raw, ok := lastSubmission.Load().(string)
	if !ok {
		t.Fatal("search submission was not recorded")
	}
	queryGot, form, _ := strings.Cut(raw, "|")
	if queryGot != query {
		t.Errorf("search query = %q, want %q", queryGot, query)
	}
	for _, want := range formParts {
		if !strings.Contains(form, want) {
			t.Errorf("search form %q does not contain %q", form, want)
		}
	}
}

// The plain listing fetch must never submit the search form, even when the
// page carries one: the POST returns the archived-only list, which would
// silently shadow the active/draft courses.
func TestClient_FetchCourseListPageContext_NeverSubmitsSearchForm(t *testing.T) {
	body := courseListPageBody(true, false)
	srv, searchPosts, _ := legacySiteServer(t, body)
	client, err := NewClient(srv.URL, "user", "pass")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	page, err := client.FetchCourseListPageContext(context.Background())
	if err != nil {
		t.Fatalf("FetchCourseListPageContext: %v", err)
	}
	if searchPosts.Load() != 0 {
		t.Fatalf("search handler submissions = %d, want 0 (plain GET only)", searchPosts.Load())
	}
	if page != body {
		t.Errorf("expected the plain listing body to be returned untouched")
	}
}

func TestClient_FetchArchivedCourseListPageContext_SubmitsActionlessForm(t *testing.T) {
	srv, searchPosts, lastSubmission := legacySiteServer(t, courseListPageBody(true, false))
	client, err := NewClient(srv.URL, "user", "pass")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	page, err := client.FetchArchivedCourseListPageContext(context.Background())
	if err != nil {
		t.Fatalf("FetchArchivedCourseListPageContext: %v", err)
	}
	if searchPosts.Load() != 1 {
		t.Fatalf("search handler submissions = %d, want 1", searchPosts.Load())
	}
	if !strings.Contains(page, "ARCHIVED LIST") {
		t.Errorf("expected the search submission's response, got %q", page)
	}
	assertSearchSubmission(t, lastSubmission, "handler=search",
		"IsArchive=true", "__RequestVerificationToken=search-token-1", "SearchText=")
}

// The legacy action-based form must keep working alongside the
// action-less live-page shape.
func TestClient_FetchArchivedCourseListPageContext_SubmitsActionForm(t *testing.T) {
	srv, searchPosts, lastSubmission := legacySiteServer(t, courseListPageBody(true, true))
	client, err := NewClient(srv.URL, "user", "pass")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	page, err := client.FetchArchivedCourseListPageContext(context.Background())
	if err != nil {
		t.Fatalf("FetchArchivedCourseListPageContext: %v", err)
	}
	if searchPosts.Load() != 1 {
		t.Fatalf("search handler submissions = %d, want 1", searchPosts.Load())
	}
	if !strings.Contains(page, "ARCHIVED LIST") {
		t.Errorf("expected the search submission's response, got %q", page)
	}
	assertSearchSubmission(t, lastSubmission, "handler=search",
		"IsArchive=true", "__RequestVerificationToken=search-token-1", "SearchText=")
}

// A page without the search form is an error, never a silent fallback: the
// archived list must not vanish unnoticed.
func TestClient_FetchArchivedCourseListPageContext_MissingFormErrors(t *testing.T) {
	body := courseListPageBody(false, false)
	srv, searchPosts, _ := legacySiteServer(t, body)
	client, err := NewClient(srv.URL, "user", "pass")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.FetchArchivedCourseListPageContext(context.Background()); err == nil {
		t.Fatal("expected error when the search form is missing, got nil")
	}
	if searchPosts.Load() != 0 {
		t.Fatalf("search handler submissions = %d, want 0", searchPosts.Load())
	}
}
