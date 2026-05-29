package absenceshttp

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
	user auth.User
	err  error
}

func (f fakeAuth) RequireUser(_ context.Context, _ *http.Request) (auth.User, error) {
	return f.user, f.err
}

func (fakeAuth) HandleLogin(_ http.ResponseWriter, _ *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(_ http.ResponseWriter, _ *http.Request) error { return nil }

func adminDeps() httpdeps.Deps {
	return httpdeps.Deps{
		Auth: fakeAuth{user: auth.User{ID: uuid.New(), Username: "admin", Role: "Admin"}},
	}
}

func responseCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var response struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	return response.Code
}

func TestStatusUpdateRejectsUnsupportedStatus(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, adminDeps())

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/absences/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/status",
		strings.NewReader(`{"status":"deleted","expected_version":1}`),
	)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if got := responseCode(t, w); got != "bad_status" {
		t.Fatalf("code = %q, want bad_status", got)
	}
}

func TestStatusUpdateRequiresAdmin(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.User{ID: uuid.New(), Username: "teacher", Role: "Teacher"}},
	})

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/absences/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/status",
		strings.NewReader(`{"status":"reviewed","expected_version":1}`),
	)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestAbsenceSettingsRejectsInvalidMaximumDateRange(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, adminDeps())

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/admin/absence-settings",
		strings.NewReader(`{
			"form": {
				"max_date_range_days": 0,
				"require_reason": true,
				"reason_categories": [{"value":"medical","label":"Medical"}],
				"allow_free_text_reason": true,
				"intro_text": "",
				"confirmation_text": ""
			},
			"sit_in": {"auto_resolve_enabled": true, "zoom_description": "Zoom", "max_sessions_per_absence": 10},
			"student_self_service": {"can_view_own": false, "can_cancel_own": false}
		}`),
	)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if got := responseCode(t, w); got != "bad_settings" {
		t.Fatalf("code = %q, want bad_settings", got)
	}
}

func TestValidTransitionAllowsCancellationAfterAction(t *testing.T) {
	if !validTransition("actioned", "cancelled") {
		t.Fatal("actioned absences must remain cancellable")
	}
}
