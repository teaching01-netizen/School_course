package absenceshttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type publicContractError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodePublicContractError(t *testing.T, recorder *httptest.ResponseRecorder) publicContractError {
	t.Helper()
	var got publicContractError
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode error response %q: %v", recorder.Body.String(), err)
	}
	return got
}

func assertPublicContractError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, status, recorder.Body.String())
	}
	got := decodePublicContractError(t, recorder)
	if got.Code != code {
		t.Fatalf("error code = %q, want %q; body = %s", got.Code, code, recorder.Body.String())
	}
}

func TestPublicStudentLookupRejectsBlankWCodeBeforeDataAccess(t *testing.T) {
	tests := []struct {
		name     string
		rawQuery string
	}{
		{name: "missing", rawQuery: ""},
		{name: "empty", rawQuery: "?wcode="},
		{name: "whitespace", rawQuery: "?wcode=%20%20%09"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &server{a: httpadapter.Adapter{}}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/absences/student-lookup"+tt.rawQuery, nil)
			recorder := httptest.NewRecorder()

			s.handleStudentLookup(recorder, req)

			assertPublicContractError(t, recorder, http.StatusBadRequest, "bad_wcode")
		})
	}
}

func TestPublicSitInOptionsValidatesRequestBeforeDataAccess(t *testing.T) {
	subjectID := uuid.NewString()
	tests := []struct {
		name string
		path string
		code string
	}{
		{
			name: "missing wcode",
			path: "/api/v1/absences/sit-in-options?subject_id=" + subjectID,
			code: "bad_params",
		},
		{
			name: "blank wcode",
			path: "/api/v1/absences/sit-in-options?wcode=%20%09&subject_id=" + subjectID,
			code: "bad_params",
		},
		{
			name: "missing subject",
			path: "/api/v1/absences/sit-in-options?wcode=w123",
			code: "bad_params",
		},
		{
			name: "malformed subject",
			path: "/api/v1/absences/sit-in-options?wcode=w123&subject_id=not-a-uuid",
			code: "bad_subject_id",
		},
		{
			name: "malformed date from",
			path: "/api/v1/absences/sit-in-options?wcode=w123&subject_id=" + subjectID + "&date_from=not-a-date&date_to=2030-01-02",
			code: "bad_date_from",
		},
		{
			name: "malformed date to",
			path: "/api/v1/absences/sit-in-options?wcode=w123&subject_id=" + subjectID + "&date_from=2030-01-01&date_to=not-a-date",
			code: "bad_date_to",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &server{deps: httpdeps.Deps{InstituteTZ: "UTC"}, a: httpadapter.Adapter{}}
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			recorder := httptest.NewRecorder()

			s.handleSitInOptions(recorder, req)

			assertPublicContractError(t, recorder, http.StatusBadRequest, tt.code)
		})
	}
}

func TestPublicSessionsInRangeValidatesRequiredFieldsAndDateSyntaxBeforeDataAccess(t *testing.T) {
	tests := []struct {
		name string
		path string
		code string
	}{
		{
			name: "missing wcode",
			path: "/api/v1/absences/sessions-in-range",
			code: "bad_params",
		},
		{
			name: "blank wcode",
			path: "/api/v1/absences/sessions-in-range?wcode=%20%09",
			code: "bad_params",
		},
		{
			name: "malformed date from",
			path: "/api/v1/absences/sessions-in-range?wcode=w123&date_from=not-a-date&date_to=2030-01-02",
			code: "bad_date_from",
		},
		{
			name: "malformed date to",
			path: "/api/v1/absences/sessions-in-range?wcode=w123&date_from=2030-01-01&date_to=not-a-date",
			code: "bad_date_to",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &server{deps: httpdeps.Deps{InstituteTZ: "UTC"}, a: httpadapter.Adapter{}}
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			recorder := httptest.NewRecorder()

			s.handleSessionsInRange(recorder, req)

			assertPublicContractError(t, recorder, http.StatusBadRequest, tt.code)
		})
	}
}

// The sessions endpoint accepts either a complete date range or no date range.
// A partial or reversed range must be rejected before settings/session queries.
func TestPublicSessionsInRangeRejectsIncompleteOrReversedDateRangeBeforeDataAccess(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "date from only",
			path: "/api/v1/absences/sessions-in-range?wcode=w123&date_from=2030-01-01",
		},
		{
			name: "date to only",
			path: "/api/v1/absences/sessions-in-range?wcode=w123&date_to=2030-01-02",
		},
		{
			name: "date to before date from",
			path: "/api/v1/absences/sessions-in-range?wcode=w123&date_from=2030-01-02&date_to=2030-01-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("handler reached data access instead of rejecting the invalid range: %v", recovered)
				}
			}()

			s := &server{deps: httpdeps.Deps{InstituteTZ: "UTC"}, a: httpadapter.Adapter{}}
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			recorder := httptest.NewRecorder()

			s.handleSessionsInRange(recorder, req)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 for an incomplete or reversed date range; body = %s", recorder.Code, recorder.Body.String())
			}
			if got := decodePublicContractError(t, recorder); strings.TrimSpace(got.Code) == "" {
				t.Fatal("invalid date range response must include an error code")
			}
		})
	}
}

func TestDefaultPublicAbsenceConfigPassesItsOwnValidation(t *testing.T) {
	settings := defaultAbsenceSettings()
	if err := validateAbsenceSettings(settings); err != nil {
		t.Fatalf("default absence config is invalid: %v", err)
	}
}

func TestPublicAbsenceConfigRejectsUnsafeSessionLimits(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*absenceSettings)
	}{
		{name: "zero date range", mutate: func(s *absenceSettings) { s.Form.MaxDateRangeDays = 0 }},
		{name: "date range over one year", mutate: func(s *absenceSettings) { s.Form.MaxDateRangeDays = 366 }},
		{name: "negative before-session cutoff", mutate: func(s *absenceSettings) { s.Form.MinHoursBeforeSession = -1 }},
		{name: "before-session cutoff over one week", mutate: func(s *absenceSettings) { s.Form.MinHoursBeforeSession = 169 }},
		{name: "negative post-session grace", mutate: func(s *absenceSettings) { s.Form.MaxHoursAfterSession = -1 }},
		{name: "post-session grace over one week", mutate: func(s *absenceSettings) { s.Form.MaxHoursAfterSession = 169 }},
		{name: "zero sessions per absence", mutate: func(s *absenceSettings) { s.SitIn.MaxSessionsPerAbsence = 0 }},
		{name: "too many sessions per absence", mutate: func(s *absenceSettings) { s.SitIn.MaxSessionsPerAbsence = 101 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := defaultAbsenceSettings()
			tt.mutate(&settings)
			if err := validateAbsenceSettings(settings); err == nil {
				t.Fatalf("validateAbsenceSettings accepted unsafe config: %+v", settings)
			}
		})
	}
}

func TestPublicStudentLookupNormalizesCaseAndWhitespace(t *testing.T) {
	fixture := newPublicAbsenceContractFixture(t)
	queryWCode := " \t" + strings.ToUpper(fixture.wcode) + "\n "
	req := httptest.NewRequest(http.MethodGet, "/api/v1/absences/student-lookup?wcode="+url.QueryEscape(queryWCode), nil)
	recorder := httptest.NewRecorder()

	fixture.server.handleStudentLookup(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
	}
	var got struct {
		WCode    string `json:"wcode"`
		Subjects []any  `json:"subjects"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode student lookup response: %v", err)
	}
	if got.WCode != fixture.wcode {
		t.Fatalf("wcode = %q, want canonical %q", got.WCode, fixture.wcode)
	}
	if len(got.Subjects) != 1 {
		t.Fatalf("subjects = %d, want 1", len(got.Subjects))
	}
}

func TestPublicStudentLookupRejectsNicknameQuery(t *testing.T) {
	fixture := newPublicAbsenceContractFixture(t)
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/absences/student-lookup?wcode="+url.QueryEscape(fixture.wcode)+"&nickname=Johnny",
		nil,
	)
	recorder := httptest.NewRecorder()

	fixture.server.handleStudentLookup(recorder, req)

	assertPublicContractError(t, recorder, http.StatusBadRequest, "nickname_not_supported")
}

func TestPublicFormConfigExposesOnlyPublicSections(t *testing.T) {
	fixture := newPublicAbsenceContractFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/absence-form-config", nil)
	recorder := httptest.NewRecorder()

	fixture.server.handleFormConfigGet(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode form config response: %v", err)
	}
	for _, key := range []string{"form", "sit_in", "notifications", "admin_contact"} {
		if _, ok := got[key]; !ok {
			t.Errorf("public form config is missing %q", key)
		}
	}
	if _, ok := got["student_self_service"]; ok {
		t.Error("public form config leaked student_self_service settings")
	}
}
