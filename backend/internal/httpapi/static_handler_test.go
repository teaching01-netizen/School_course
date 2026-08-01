package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
	})

	t.Run("client route falls back to index.html with no-cache", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/operations/schedule-impact", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Body.String(); got != index {
			t.Fatalf("body = %q, want index.html", got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("cache-control = %q, want no-cache", got)
		}
	})

	t.Run("index.html itself serves with no-cache", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("cache-control = %q, want no-cache", got)
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
