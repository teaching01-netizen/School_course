package sessionshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"warwick-institute/internal/auth"
	"warwick-institute/internal/httpapi/httpdeps"
)

type fakeAuth struct {
	user auth.AuthenticatedUser
	err  error
}

func (f fakeAuth) RequireUser(ctx context.Context, r *http.Request) (auth.AuthenticatedUser, error) {
	return f.user, f.err
}

func (fakeAuth) HandleLogin(w http.ResponseWriter, r *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(w http.ResponseWriter, r *http.Request) error { return nil }

func TestRegister_GetSessions_BadStart_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "t", Role: "Teacher"}},
	})

	req := httptest.NewRequest("GET", "/api/v1/sessions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_start" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_start")
	}
}

func TestRegister_PostSessions_TeacherForbidden_Returns403(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "t", Role: "Teacher"}},
	})

	req := httptest.NewRequest("POST", "/api/v1/sessions", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "forbidden" {
		t.Fatalf("code = %q, want %q", got.Code, "forbidden")
	}
}

func TestRegister_PatchSession_BadID_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("PATCH", "/api/v1/sessions/not-a-uuid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_id" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_id")
	}
}

func TestRegister_BulkUpdate_TeacherForbidden_Returns403(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "t", Role: "Teacher"}},
	})

	req := httptest.NewRequest("POST", "/api/v1/sessions/bulk-update", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "forbidden" {
		t.Fatalf("code = %q, want %q", got.Code, "forbidden")
	}
}

func TestRegister_BulkUpdate_EmptyUpdates_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	body := `{"updates":[]}`
	req := httptest.NewRequest("POST", "/api/v1/sessions/bulk-update", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "no_updates" {
		t.Fatalf("code = %q, want %q", got.Code, "no_updates")
	}
}

func TestRegister_BulkUpdate_TooManyUpdates_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	updates := make([]map[string]any, 101)
	for i := range updates {
		updates[i] = map[string]any{"id": uuid.New().String(), "expected_version": 1, "teacher_id": uuid.New().String(), "room_id": nil, "start_at": "2026-01-01T10:00:00Z", "end_at": "2026-01-01T11:00:00Z"}
	}
	payload, _ := json.Marshal(map[string]any{"updates": updates})
	req := httptest.NewRequest("POST", "/api/v1/sessions/bulk-update", strings.NewReader(string(payload)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "too_many" {
		t.Fatalf("code = %q, want %q", got.Code, "too_many")
	}
}

func TestRegister_BulkUpdate_BadJSON_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("POST", "/api/v1/sessions/bulk-update", strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_json" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_json")
	}
}
