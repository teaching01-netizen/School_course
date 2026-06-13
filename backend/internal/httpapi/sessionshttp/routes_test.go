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
