package teacherhttp

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

type fakeAuth struct{ user auth.AuthenticatedUser }

func (f fakeAuth) RequireUser(context.Context, *http.Request) (auth.AuthenticatedUser, error) {
	return f.user, nil
}
func (fakeAuth) HandleLogin(http.ResponseWriter, *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(http.ResponseWriter, *http.Request) error { return nil }

func TestTeacherAbsenceDetailInvalidIDIsIndistinguishableNotFound(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Role: "Teacher"}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/teacher/absences/not-a-uuid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":"not_found"`) {
		t.Fatalf("body = %s, want generic not_found", w.Body.String())
	}
}

func TestTeacherAbsenceDetailDTOCannotSerializeSensitiveAdminFields(t *testing.T) {
	payload, err := json.Marshal(teacherAbsenceDetailDTO{ID: uuid.NewString(), Wcode: "W1", Status: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"student_email", "student_phone", "parent_phone", "admin_notes", "timeline", "version", "override"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("teacher response contains forbidden field %q: %s", forbidden, payload)
		}
	}
}

func TestTeacherAbsenceDetailRejectsWrongMethod(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Role: "Teacher"}}})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/teacher/absences/"+uuid.NewString(), nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}
