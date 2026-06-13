package adminusershttp

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
	"warwick-institute/internal/users"
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

func TestHandleAdminUsersCreate_RequiresIdempotencyKey(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth:       fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "admin", Role: "Admin"}},
		AdminUsers: &users.AdminProvisioningService{},
	})

	body := `{"username":"newuser","role":"Teacher","password":"secret123"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var resp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if resp.Code != "bad_idempotency_key" {
		t.Fatalf("code = %q, want %q", resp.Code, "bad_idempotency_key")
	}
}
