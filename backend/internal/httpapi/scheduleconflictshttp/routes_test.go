package scheduleconflictshttp

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseListFiltersReadsTypedFilters(t *testing.T) {
	// Given: a request containing every supported filter and an excessive limit.
	req := httptest.NewRequest("GET", "/api/v1/schedule-conflicts?limit=999&conflict_type=student_overlap&subject_id=11111111-1111-1111-1111-111111111111&teacher_id=22222222-2222-2222-2222-222222222222&student_id=33333333-3333-3333-3333-333333333333&date_from=2026-08-01&date_to=2026-08-31&q=math", nil)

	// When: the HTTP query is parsed at the boundary.
	got, err := parseListFilters(req)

	// Then: values are retained in database-native types and the limit is safe.
	if err != nil {
		t.Fatal(err)
	}
	if got.Limit != 200 || got.ConflictType != "student_overlap" || got.SubjectID.String() != "11111111-1111-1111-1111-111111111111" || got.TeacherID.String() != "22222222-2222-2222-2222-222222222222" || got.StudentID.String() != "33333333-3333-3333-3333-333333333333" || got.DateFrom.Format(time.DateOnly) != "2026-08-01" || got.DateTo.Format(time.DateOnly) != "2026-08-31" || got.Query != "math" {
		t.Fatalf("filters = %+v", got)
	}
}

func TestParseListFiltersRejectsInvalidCursor(t *testing.T) {
	// Given: a cursor that is not a valid encoded conflict key.
	req := httptest.NewRequest("GET", "/api/v1/schedule-conflicts?cursor=broken", nil)

	// When: the request is parsed.
	_, err := parseListFilters(req)

	// Then: the invalid boundary value is rejected.
	if err == nil {
		t.Fatal("expected invalid cursor error")
	}
}

func TestConflictCursorRoundTrips(t *testing.T) {
	// Given: a complete key in the conflict sort order.
	want := conflictCursor{
		StartAt:       time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC),
		ConflictType:  "teacher_overlap",
		PrimaryID:     [16]byte{1},
		ConflictingID: [16]byte{2},
		Direction:     cursorNext,
	}

	// When: the cursor is encoded and decoded at the HTTP boundary.
	raw, err := encodeCursor(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeCursor(raw)

	// Then: every ordering component survives unchanged.
	if err != nil {
		t.Fatal(err)
	}
	if *got != want {
		t.Fatalf("cursor = %+v, want %+v", *got, want)
	}
}
