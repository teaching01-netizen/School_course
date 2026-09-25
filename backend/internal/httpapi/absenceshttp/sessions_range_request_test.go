package absenceshttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"warwick-institute/internal/auth"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

func staffSessionRangeParser() *server {
	staff := absenceLimitFakeAuth{user: auth.AuthenticatedUser{Role: "Admin"}}
	return &server{
		deps: httpdeps.Deps{Auth: staff},
		a:    httpadapter.New(staff, nil),
	}
}

func TestStaffStudentViewLookupUsesStudentProjectionAndTiming(t *testing.T) {
	s := staffSessionRangeParser()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/absences/sessions-in-range?wcode=w123&student_view=true", nil)
	recorder := httptest.NewRecorder()
	pre, ok := parseSessionsRangePrelim(s, recorder, request, "", true)
	if !ok {
		t.Fatalf("parse failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	lookup, ok := finalizeSessionsRangeLookup(s, recorder, request, pre, absenceSettings{})
	if !ok {
		t.Fatalf("finalize failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	staffLookup, ok := lookup.(StaffSessionLookup)
	if !ok {
		t.Fatalf("lookup type = %T, want staff-authorized lookup", lookup)
	}
	if !staffLookup.studentProjection() || !staffLookup.studentFacing() || staffLookup.bypassTiming() || staffLookup.isLifetime() {
		t.Fatalf("student-view lookup has incorrect projection or capabilities: %#v", staffLookup)
	}
}

func TestStaffStudentViewRejectsOperationalOverrides(t *testing.T) {
	for _, option := range []string{
		"bypass_timing=false",
		"include_all_subjects=false",
		"subject_ids=",
		"lifetime=false",
	} {
		t.Run(option, func(t *testing.T) {
			s := staffSessionRangeParser()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/absences/sessions-in-range?wcode=w123&student_view=true&"+option, nil)
			recorder := httptest.NewRecorder()
			if _, ok := parseSessionsRangePrelim(s, recorder, request, "", true); ok {
				t.Fatal("student-view request accepted an operational override")
			}
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestStaffStudentViewRejectsDuplicateFlag(t *testing.T) {
	s := staffSessionRangeParser()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/absences/sessions-in-range?wcode=w123&student_view=true&student_view=false", nil)
	recorder := httptest.NewRecorder()
	if _, ok := parseSessionsRangePrelim(s, recorder, request, "", true); ok {
		t.Fatal("duplicate student_view flags were accepted")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
}
