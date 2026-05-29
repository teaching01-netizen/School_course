package corehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"warwick-institute/internal/auth"
	"warwick-institute/internal/httpapi/httpdeps"
)

type fakeAuth struct {
	user auth.User
	err  error
}

func (f fakeAuth) RequireUser(ctx context.Context, r *http.Request) (auth.User, error) {
	return f.user, f.err
}

func (fakeAuth) HandleLogin(w http.ResponseWriter, r *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(w http.ResponseWriter, r *http.Request) error { return nil }

func TestRegister_GetMetaTime_ReturnsInstituteTZAndServerNow(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth:        fakeAuth{user: auth.User{ID: uuid.New(), Username: "u", Role: "Teacher"}},
		InstituteTZ: "Asia/Bangkok",
	})

	req := httptest.NewRequest("GET", "/api/v1/meta/time", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var got struct {
		InstituteTZ string `json:"institute_tz"`
		ServerNow   string `json:"server_now"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.InstituteTZ != "Asia/Bangkok" {
		t.Fatalf("institute_tz = %q, want %q", got.InstituteTZ, "Asia/Bangkok")
	}
	if got.ServerNow == "" {
		t.Fatalf("server_now empty")
	}
}
