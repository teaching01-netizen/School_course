package absenceshttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
)

func decodeSelfServiceError(t *testing.T, recorder *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v; body=%s", err, recorder.Body.String())
	}
	return body.Code, body.Message
}

func TestStudentProfileRequiresVerifiedSession(t *testing.T) {
	s := &server{a: httpadapter.New(nil, nil)}
	recorder := httptest.NewRecorder()

	s.handleStudentProfile(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/absence-self-service/me", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	code, _ := decodeSelfServiceError(t, recorder)
	if code != "unauthorized" {
		t.Fatalf("error code = %q, want unauthorized", code)
	}
}

func TestStudentSessionsRequiresVerifiedSession(t *testing.T) {
	s := &server{a: httpadapter.New(nil, nil)}
	recorder := httptest.NewRecorder()

	s.handleStudentSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/absence-self-service/sessions", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStudentSessionsRejectsIdentityAndTimingOverrides(t *testing.T) {
	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "wcode", path: "/api/v1/absence-self-service/sessions?wcode=w999999", code: "identity_parameter_not_allowed"},
		{name: "bypass timing", path: "/api/v1/absence-self-service/sessions?bypass_timing=true", code: "bypass_not_allowed"},
		{name: "all subjects", path: "/api/v1/absence-self-service/sessions?include_all_subjects=true", code: "include_all_subjects_not_allowed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &server{a: httpadapter.New(nil, nil)}
			recorder := httptest.NewRecorder()
			s.handleStudentSessions(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
			}
			code, _ := decodeSelfServiceError(t, recorder)
			if code != test.code {
				t.Fatalf("error code = %q, want %q", code, test.code)
			}
		})
	}
}

func TestStaffStudentLookupRequiresAdminSession(t *testing.T) {
	s := &server{a: httpadapter.New(nil, nil)}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/absences/student-lookup?wcode=w250389",
		nil,
	)

	s.handleStaffStudentLookup(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	code, _ := decodeSelfServiceError(t, recorder)
	if code != "unauthorized" {
		t.Fatalf("error code = %q, want unauthorized", code)
	}
}

func TestIsAdminRequestTreatsMissingAuthAsUnauthenticated(t *testing.T) {
	if isAdminRequest(nil, httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("missing auth service was treated as an admin request")
	}
}

func TestLegacyStudentSessionsDoNotAuthorizeByWCode(t *testing.T) {
	s := &server{a: httpadapter.New(nil, nil)}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/absences/sessions-in-range?wcode=w250389&date_from=2030-01-01&date_to=2030-01-02",
		nil,
	)

	s.handleSessionsInRange(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestLegacySitInOptionsDoNotAuthorizeByWCode(t *testing.T) {
	s := &server{a: httpadapter.New(nil, nil)}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/absences/sit-in-options?wcode=w250389&subject_id=00000000-0000-0000-0000-000000000001",
		nil,
	)

	s.handleSitInOptions(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStudentAbsenceResponseOmitsStaffAndContactFields(t *testing.T) {
	absenceID := uuid.New()
	courseID := uuid.New()
	now := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	response := studentAbsenceResponse(sqldb.ManagedAbsenceRow{
		ID:           pgtype.UUID{Bytes: absenceID, Valid: true},
		Wcode:        "w250389",
		StudentName:  pgtype.Text{String: "Alex Smith", Valid: true},
		StudentEmail: pgtype.Text{String: "alex@example.edu", Valid: true},
		StudentPhone: pgtype.Text{String: "+66812345678", Valid: true},
		ParentPhone:  pgtype.Text{String: "+66812345679", Valid: true},
		CourseID:     pgtype.UUID{Bytes: courseID, Valid: true},
		CourseCode:   "MATH-101",
		CourseName:   "Mathematics",
		DateFrom:     pgtype.Date{Time: now, Valid: true},
		DateTo:       pgtype.Date{Time: now, Valid: true},
		Status:       "cancelled",
		AdminNotes:   pgtype.Text{String: "staff-only note", Valid: true},
		Version:      2,
		CreatedAt:    pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})

	for _, key := range []string{"student_name", "student_email", "student_phone", "parent_phone", "admin_notes", "wcode"} {
		if _, ok := response[key]; ok {
			t.Fatalf("student response contains staff/contact field %q", key)
		}
	}
	if response["id"] != absenceID.String() || response["course_id"] != courseID.String() {
		t.Fatalf("student response lost required object identifiers: %#v", response)
	}
}
