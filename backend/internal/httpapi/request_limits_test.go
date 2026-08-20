package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"warwick-institute/internal/httpapi/httpadapter"
)

type boundedBodyReader struct {
	remaining int64
	readBytes int64
}

func (r *boundedBodyReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.remaining {
		n = r.remaining
	}
	for i := int64(0); i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= n
	r.readBytes += n
	return int(n), nil
}

func TestWithRequestBodyLimitRejectsDefaultMutationBoundaries(t *testing.T) {
	const limit = httpadapter.MaxJSONBodyBytes
	for _, test := range []struct {
		name       string
		size       int64
		wantStatus int
		wantCalled bool
	}{
		{name: "below limit", size: limit - 1, wantStatus: http.StatusNoContent, wantCalled: true},
		{name: "at limit", size: limit, wantStatus: http.StatusNoContent, wantCalled: true},
		{name: "above limit", size: limit + 1, wantStatus: http.StatusRequestEntityTooLarge, wantCalled: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := bytes.Repeat([]byte{'x'}, int(test.size))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/batch", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			called := false
			recorder := httptest.NewRecorder()
			withRequestBodyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				got, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read body: %v", err)
				}
				if int64(len(got)) != test.size {
					t.Fatalf("body length = %d, want %d", len(got), test.size)
				}
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(recorder, req)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if called != test.wantCalled {
				t.Fatalf("handler called = %v, want %v", called, test.wantCalled)
			}
		})
	}
}

func TestWithRequestBodyLimitRejectsUnknownLengthWithoutReadingWholeBody(t *testing.T) {
	reader := &boundedBodyReader{remaining: 20 * 1024 * 1024}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absences", reader)
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	called := false
	withRequestBodyLimit(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
	if called {
		t.Fatal("handler ran for oversized chunked body")
	}
	if reader.readBytes != httpadapter.MaxJSONBodyBytes+1 {
		t.Fatalf("read %d bytes from 20 MiB body, want exactly limit+1=%d", reader.readBytes, httpadapter.MaxJSONBodyBytes+1)
	}
}

func TestWithRequestBodyLimitAllowsDocumentedUploadLimit(t *testing.T) {
	for _, test := range []struct {
		name       string
		size       int64
		wantStatus int
	}{
		{name: "at upload limit", size: documentedUploadBodyLimit, wantStatus: http.StatusNoContent},
		{name: "above upload limit", size: documentedUploadBodyLimit + 1, wantStatus: http.StatusRequestEntityTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/crm/upload", bytes.NewReader(bytes.Repeat([]byte{'x'}, int(test.size))))
			recorder := httptest.NewRecorder()
			called := false
			withRequestBodyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(recorder, req)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if test.size <= documentedUploadBodyLimit && !called {
				t.Fatal("handler did not run at documented upload limit")
			}
			if test.size > documentedUploadBodyLimit && called {
				t.Fatal("handler ran above documented upload limit")
			}
		})
	}
}
