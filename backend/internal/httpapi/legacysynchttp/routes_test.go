package legacysynchttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"warwick-institute/internal/auth"
	"warwick-institute/internal/httpapi/httpdeps"
)

type routeAuth struct {
	user auth.AuthenticatedUser
	err  error
}

func (f routeAuth) RequireUser(context.Context, *http.Request) (auth.AuthenticatedUser, error) {
	return f.user, f.err
}

func (routeAuth) HandleLogin(http.ResponseWriter, *http.Request) error  { return nil }
func (routeAuth) HandleLogout(http.ResponseWriter, *http.Request) error { return nil }

func TestAdminRoutesRequireAdmin(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		auth   routeAuth
		status int
	}{
		{name: "anonymous", method: http.MethodGet, path: "/api/v1/admin/legacy-sync/health", auth: routeAuth{err: errors.New("missing session")}, status: http.StatusUnauthorized},
		{name: "teacher", method: http.MethodGet, path: "/api/v1/admin/legacy-sync/health", auth: routeAuth{user: auth.AuthenticatedUser{Role: "Teacher"}}, status: http.StatusForbidden},
		{name: "audit anonymous", method: http.MethodGet, path: "/api/v1/admin/legacy-sync/audit", auth: routeAuth{err: errors.New("missing session")}, status: http.StatusUnauthorized},
		{name: "audit teacher", method: http.MethodGet, path: "/api/v1/admin/legacy-sync/audit", auth: routeAuth{user: auth.AuthenticatedUser{Role: "Teacher"}}, status: http.StatusForbidden},
		{name: "resolve anonymous", method: http.MethodPost, path: "/api/v1/admin/legacy-sync/conflicts/not-a-uuid/resolve", auth: routeAuth{err: errors.New("missing session")}, status: http.StatusUnauthorized},
		{name: "resolve teacher", method: http.MethodPost, path: "/api/v1/admin/legacy-sync/conflicts/not-a-uuid/resolve", auth: routeAuth{user: auth.AuthenticatedUser{Role: "Teacher"}}, status: http.StatusForbidden},
		{name: "ignore anonymous", method: http.MethodPost, path: "/api/v1/admin/legacy-sync/conflicts/not-a-uuid/ignore", auth: routeAuth{err: errors.New("missing session")}, status: http.StatusUnauthorized},
		{name: "student-import teacher", method: http.MethodPost, path: "/api/v1/admin/legacy-sync/student-import", auth: routeAuth{user: auth.AuthenticatedUser{Role: "Teacher"}}, status: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			Register(mux, httpdeps.Deps{Auth: tt.auth})
			request := httptest.NewRequest(tt.method, tt.path, nil)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.status, response.Body.String())
			}
		})
	}
}
