package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStaticHandler_ServesFilesAndFallsBackForClientRoutes guards the SPA static
// serving contract: exact files are served with their real MIME type, extension-less
// client routes fall back to index.html, and missing asset paths return 404 instead
// of index.html (which previously broke module loading under strict MIME checking).
func TestStaticHandler_ServesFilesAndFallsBackForClientRoutes(t *testing.T) {
	dir := t.TempDir()
	index := "<!doctype html><html><body>app</body></html>"
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	asset := "console.log('chunk')"
	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "index-HASH.js"), []byte(asset), 0o644); err != nil {
		t.Fatal(err)
	}

	h := staticHandler(dir)

	t.Run("existing asset serves with JS MIME and immutable cache", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/index-HASH.js", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
			t.Fatalf("content-type = %q, want text/javascript", got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Fatalf("cache-control = %q, want immutable", got)
		}
	})

	t.Run("missing asset returns 404 not index.html", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/SessionChanges-STALEHASH.js", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
		if got := rec.Body.String(); got == index {
			t.Fatalf("404 response body must not be index.html")
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("cache-control = %q, want no-store", got)
		}
	})

	t.Run("client route falls back to index.html with no-store", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/operations/schedule-impact", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Body.String(); got != index {
			t.Fatalf("body = %q, want index.html", got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("cache-control = %q, want no-store", got)
		}
	})

	t.Run("root serves index.html with no-store", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("cache-control = %q, want no-store", got)
		}
	})

	t.Run("api paths are not SPA-fallbacked", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

func TestStaticHandlerServesNewBuildOnNormalReload(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.html")
	buildTime := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	writeBuild := func(html string) {
		t.Helper()
		if err := os.WriteFile(indexPath, []byte(html), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(indexPath, buildTime, buildTime); err != nil {
			t.Fatal(err)
		}
	}
	writeBuild(`<!doctype html><script src="/assets/index-OLD.js"></script>`)
	newHTML := `<!doctype html><script src="/assets/index-NEW.js"></script>`
	writeBuild(newHTML)

	h := staticHandler(dir)
	for _, path := range []string{"/", "/operations/schedule-impact"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("If-Modified-Since", buildTime.Format(http.TimeFormat))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || rec.Body.String() != newHTML {
				t.Fatalf("reload returned %d with body %q, want 200 with the new build", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Last-Modified"); got != "" {
				t.Fatalf("last-modified = %q, want no timestamp validator", got)
			}
		})
	}
}
