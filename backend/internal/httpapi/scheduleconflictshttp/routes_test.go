package scheduleconflictshttp

import (
	"net/http/httptest"
	"testing"
)

func TestParseListFiltersClampsPaginationAndReadsFilters(t *testing.T) {
	// Given: a request containing every supported filter and an excessive limit.
	req := httptest.NewRequest("GET", "/api/v1/schedule-conflicts?limit=999&offset=7&conflict_type=student_overlap&subject_id=11111111-1111-1111-1111-111111111111&teacher_id=22222222-2222-2222-2222-222222222222&student_id=33333333-3333-3333-3333-333333333333&date_from=2026-08-01&date_to=2026-08-31&q=math", nil)

	// When: the HTTP query is parsed at the boundary.
	got, err := parseListFilters(req)

	// Then: pagination is safe and all filter intent is retained.
	if err != nil {
		t.Fatal(err)
	}
	if got.Limit != 200 || got.Offset != 7 || got.ConflictType != "student_overlap" || got.SubjectID != "11111111-1111-1111-1111-111111111111" || got.TeacherID != "22222222-2222-2222-2222-222222222222" || got.StudentID != "33333333-3333-3333-3333-333333333333" || got.DateFrom != "2026-08-01" || got.DateTo != "2026-08-31" || got.Query != "math" {
		t.Fatalf("filters = %+v", got)
	}
}

func TestParseListFiltersRejectsInvalidConflictType(t *testing.T) {
	// Given: an unsupported conflict type.
	req := httptest.NewRequest("GET", "/api/v1/schedule-conflicts?conflict_type=unknown", nil)

	// When: the request is parsed.
	_, err := parseListFilters(req)

	// Then: the invalid boundary value is rejected.
	if err == nil {
		t.Fatal("expected invalid conflict type error")
	}
}
