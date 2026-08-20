package legacysync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// studentsPageBody builds a /Admin/Students body. includeSearchForm
// controls whether the page carries the student search form (SearchText +
// antiforgery token, no action attribute, exactly like the live old site).
func studentsPageBody(includeSearchForm bool) string {
	form := ""
	if includeSearchForm {
		form = `<form method="post" enctype="multipart/form-data">
<input class="form-control" placeholder="Search W-Code, Name" type="text" id="SearchText" name="SearchText" value="" />
<input name="__RequestVerificationToken" type="hidden" value="students-token-1" />
</form>`
	}
	return "<!DOCTYPE html><html><head><title>Student</title></head><body>" + form + "<p>students list</p></body></html>"
}

// studentsLegacySiteServer stands up a minimal old-site: the login flow,
// the students page GET, and the students search POST. It records how many
// search submissions arrived and the exact query + form of the last one.
func studentsLegacySiteServer(t *testing.T, pageBody string) (*httptest.Server, *atomic.Int32, *atomic.Value) {
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
		case r.Method == http.MethodGet && r.URL.Path == "/Admin/Students":
			_, _ = w.Write([]byte(pageBody))
		case r.Method == http.MethodPost && r.URL.Path == "/Admin/Students":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse students search submission: %v", err)
				return
			}
			searchPosts.Add(1)
			lastSubmission.Store(r.URL.RawQuery + "|" + r.Form.Encode())
			_, _ = w.Write([]byte(`<html><body>STUDENT ROWS</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &searchPosts, &lastSubmission
}

// The students search POST must carry the search handler, the requested
// SearchText and the page's antiforgery token, and its response is what
// callers see (the plain page shows no rows until searched).
func TestClient_SearchStudentsPageContext_SubmitsSearchForm(t *testing.T) {
	srv, searchPosts, lastSubmission := studentsLegacySiteServer(t, studentsPageBody(true))
	client, err := NewClient(srv.URL, "user", "pass")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	page, err := client.SearchStudentsPageContext(context.Background(), "W250025")
	if err != nil {
		t.Fatalf("SearchStudentsPageContext: %v", err)
	}
	if searchPosts.Load() != 1 {
		t.Fatalf("search handler submissions = %d, want 1", searchPosts.Load())
	}
	if !strings.Contains(page, "STUDENT ROWS") {
		t.Errorf("expected the search submission's response, got %q", page)
	}
	raw, ok := lastSubmission.Load().(string)
	if !ok {
		t.Fatal("search submission was not recorded")
	}
	queryGot, form, _ := strings.Cut(raw, "|")
	if queryGot != "handler=search" {
		t.Errorf("search query = %q, want %q", queryGot, "handler=search")
	}
	for _, want := range []string{"SearchText=W250025", "__RequestVerificationToken=students-token-1"} {
		if !strings.Contains(form, want) {
			t.Errorf("search form %q does not contain %q", form, want)
		}
	}
}

// A students page without the search form is an error, never a silent
// fallback: the student directory must not vanish unnoticed.
func TestClient_SearchStudentsPageContext_MissingFormErrors(t *testing.T) {
	srv, searchPosts, _ := studentsLegacySiteServer(t, studentsPageBody(false))
	client, err := NewClient(srv.URL, "user", "pass")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.SearchStudentsPageContext(context.Background(), ""); err == nil {
		t.Fatal("expected error when the search form is missing, got nil")
	}
	if searchPosts.Load() != 0 {
		t.Fatalf("search handler submissions = %d, want 0", searchPosts.Load())
	}
}

// parseStudentsSearchForm must ignore the navbar logout form (which also
// carries an antiforgery token but no SearchText) and find the students
// form, exactly like the live page layout.
func TestParseStudentsSearchForm_IgnoresOtherForms(t *testing.T) {
	page := `<!DOCTYPE html><html><head><title>Student</title></head><body>
<form method="post" id="logoutForm" class="navbar-right" action="/Account/Logout">
<input name="__RequestVerificationToken" type="hidden" value="logout-token" />
</form>
<form method="post" enctype="multipart/form-data">
<input type="text" id="SearchText" name="SearchText" value="W250025" />
<input name="__RequestVerificationToken" type="hidden" value="students-token-2" />
</form>
</body></html>`
	form, err := parseStudentsSearchForm(page)
	if err != nil {
		t.Fatalf("parseStudentsSearchForm: %v", err)
	}
	if form.token != "students-token-2" {
		t.Errorf("token = %q, want the students form token", form.token)
	}
	if form.searchText != "W250025" {
		t.Errorf("searchText = %q, want %q", form.searchText, "W250025")
	}
}

// The live-page fixture (captured and redacted) must keep yielding a usable
// search form; a broken fixture would silently break the student sync.
func TestParseStudentsSearchForm_LiveFixture(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("parser", "testdata", "students_page.html"))
	if err != nil {
		t.Fatalf("reading students_page.html fixture: %v", err)
	}
	form, err := parseStudentsSearchForm(string(b))
	if err != nil {
		t.Fatalf("parseStudentsSearchForm(fixture): %v", err)
	}
	if form.token == "" || strings.HasPrefix(form.token, "CfDJ8") {
		t.Errorf("fixture token = %q, want the placeholder token", form.token)
	}
}