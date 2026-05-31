package absenceshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
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

func TestParseAbsenceSettingsPreservesSuccessSMSTemplate(t *testing.T) {
	settings := parseAbsenceSettings([]byte(`{
		"notifications": {
			"sms_parent_enabled": true,
			"sms_parent_template": "OTP {{code}}",
			"sms_success_template": "Done {{class_name}} {{sit_in_class}}",
			"allow_submit_without_otp": false
		}
	}`))

	if settings.Notifications.SmsSuccessTemplate != "Done {{class_name}} {{sit_in_class}}" {
		t.Fatalf("sms_success_template = %q", settings.Notifications.SmsSuccessTemplate)
	}
}

func TestRenderSuccessSMSTemplateUsesSubjectNamesAndSelectedSitInDate(t *testing.T) {
	row := sqldb.ManagedAbsenceRow{
		StudentName:      pgtype.Text{String: "Ada", Valid: true},
		SubjectName:      pgtype.Text{String: "Math inter", Valid: true},
		SitInSubjectName: pgtype.Text{String: "Math advanced", Valid: true},
		DateFrom:         pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:           pgtype.Date{Time: time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	sessions := []sqldb.ManagedAbsenceSession{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 9, 11, 0, 0, 0, time.UTC), Valid: true},
	}}

	got := renderSuccessSMSTemplate(
		"{{nickname}}|{{class_name}}|{{absence_date}}|{{sit_in_class}}|{{sit_in_date_time}}",
		row,
		sessions,
		time.UTC,
	)
	want := "Ada|Math inter|9 Jun 2026|Math advanced|9 Jun, 09:00 - 11:00"
	if got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
}

func TestRenderParentSMSTemplateReplacesStudentNameAndCode(t *testing.T) {
	got := renderParentSMSTemplate(
		"Warwick Institute: {{student_name}} ได้แจ้งความประสงค์ขอลาเรียน กรุณาแจ้งรหัส {{code}}",
		"สมชาย ใจดี",
		"139809",
	)
	want := "Warwick Institute: สมชาย ใจดี ได้แจ้งความประสงค์ขอลาเรียน กรุณาแจ้งรหัส 139809"
	if got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
}

func TestRenderParentSMSTemplateSkipsUnknownPlaceholders(t *testing.T) {
	got := renderParentSMSTemplate("Hello {{student_name}}, code {{code}}, extra {{unknown}}", "Ada", "123")
	want := "Hello Ada, code 123, extra {{unknown}}"
	if got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
}

func TestValidTransitionAllowsCancellationAfterAction(t *testing.T) {
	if !validTransition("actioned", "cancelled") {
		t.Fatal("actioned absences must remain cancellable")
	}
}
