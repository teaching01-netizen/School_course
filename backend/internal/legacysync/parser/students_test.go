package parser

import (
	"errors"
	"reflect"
	"testing"

	"warwick-institute/internal/legacysync/normalize"
)

func TestStudentsPageParser_CountsAndSortsRows(t *testing.T) {
	res, err := ParseStudentsPage(readFixture(t, "students_page_search.html"))
	if err != nil {
		t.Fatalf("ParseStudentsPage: %v", err)
	}
	// The fixture mixes 27 modern rows (W-code in the first cell), one clean
	// legacy-import row (W-code in the third cell) and 18 old-import rows
	// whose W-code is embedded in the second cell (W190168 appears three
	// times and is deduplicated). Bogus rows ("Grand Total", Excel-corrupted
	// imports) are skipped, not errors.
	if len(res.Students) != 46 {
		t.Fatalf("got %d students, want 46", len(res.Students))
	}
	if !studentsSortedAscending(res.Students) {
		t.Fatalf("students not sorted ascending by wcode: %+v", res.Students)
	}
}

func TestStudentsPageParser_ModernRow(t *testing.T) {
	res, err := ParseStudentsPage(readFixture(t, "students_page_search.html"))
	if err != nil {
		t.Fatalf("ParseStudentsPage: %v", err)
	}
	byCode := studentByCode(res.Students)

	// W250017: name is the legacy "- -" marker (cleared), phone/email are
	// redacted placeholders in the fixture.
	want := legacyStudent("W250017", "", "Aiko", "Others", "", "2026", "081-0000001", "")
	if got := byCode["W250017"]; !reflect.DeepEqual(got, want) {
		t.Errorf("W250017 = %+v, want %+v", got, want)
	}
	want = legacyStudent("W260287", "", "Pat", ".Satit Prasarnmit", "M.6", "2027", "081-0000002", "")
	if got := byCode["W260287"]; !reflect.DeepEqual(got, want) {
		t.Errorf("W260287 = %+v, want %+v", got, want)
	}
	// A name that is not an exact marker is preserved verbatim.
	want = legacyStudent("W250086", "- Kitcharenlarp", "", "Other", "G.10", "2026", "", "")
	if got := byCode["W250086"]; !reflect.DeepEqual(got, want) {
		t.Errorf("W250086 = %+v, want %+v", got, want)
	}
}

func TestStudentsPageParser_OldImportRows(t *testing.T) {
	res, err := ParseStudentsPage(readFixture(t, "students_page_search.html"))
	if err != nil {
		t.Fatalf("ParseStudentsPage: %v", err)
	}
	byCode := studentByCode(res.Students)

	// Clean legacy-import row: reference number + date prefix cells, then
	// the real profile shifted by two columns.
	want := legacyStudent("W180203", "Roungkhao Phuchsuwansakul", "Wernwern", "Satit Patumwan", "G.10", "", "081-0000003", "")
	if got := byCode["W180203"]; !reflect.DeepEqual(got, want) {
		t.Errorf("W180203 = %+v, want %+v", got, want)
	}

	// Old-import rows embed the wcode in the second cell; first and last
	// names live in separate cells and are joined. W190168 appears three
	// times (duplicate import rows) and yields exactly one student.
	want = legacyStudent("W190168", "Sethanun Pornvoravanich", "Mic", ".Assumption College", "M.4", "", "081-0000004", "")
	if got := byCode["W190168"]; !reflect.DeepEqual(got, want) {
		t.Errorf("W190168 = %+v, want %+v", got, want)
	}
	// Junk in the phone column ("Mar-May 2021", "1200") is rejected by the
	// phone guard instead of being stored.
	want = legacyStudent("W200373", "Sukittima Raseenual", "Dream", ".GED", "Walk-In", "", "", "")
	if got := byCode["W200373"]; !reflect.DeepEqual(got, want) {
		t.Errorf("W200373 = %+v, want %+v", got, want)
	}
	want = legacyStudent("W180317", "Apichai Chaiwinij", "Bank", "Triam Udom Suksa", "", "", "", "")
	if got := byCode["W180317"]; !reflect.DeepEqual(got, want) {
		t.Errorf("W180317 = %+v, want %+v", got, want)
	}
}

func TestStudentsPageParser_SkipsBogusRows(t *testing.T) {
	res, err := ParseStudentsPage(readFixture(t, "students_page_search.html"))
	if err != nil {
		t.Fatalf("ParseStudentsPage: %v", err)
	}
	// "Grand Total" and the Excel-corrupted row are not students; the
	// corrupted row's wcode (W200298) is buried in an unusable layout and
	// must not be guessed.
	if _, ok := studentByCode(res.Students)["Grand Total"]; ok {
		t.Errorf("Grand Total row parsed as a student")
	}
	if _, ok := studentByCode(res.Students)["W200298"]; ok {
		t.Errorf("corrupted row parsed as a student")
	}
}

func TestStudentsPageParser_EmptyListingIsNotDrift(t *testing.T) {
	// The plain (unsearched) page renders the same table with a "No
	// students yet." row: zero students, no error.
	res, err := ParseStudentsPage(readFixture(t, "students_page.html"))
	if err != nil {
		t.Fatalf("ParseStudentsPage(plain): %v", err)
	}
	if len(res.Students) != 0 {
		t.Fatalf("got %d students from empty listing, want 0", len(res.Students))
	}
}

func TestStudentsPageParser_LoginPageIsErrLoginPage(t *testing.T) {
	_, err := ParseStudentsPage(readFixture(t, "login_page.html"))
	if !errors.Is(err, ErrLoginPage) {
		t.Fatalf("ParseStudentsPage(login) error = %v, want ErrLoginPage", err)
	}
}

func TestStudentsPageParser_DriftsOnWrongHeaders(t *testing.T) {
	page := "<!DOCTYPE html><html><head><title>Student</title></head><body>" +
		"<table class=\"table table-hover\"><thead><tr><th>W-Code</th><th>Name</th></tr></thead>" +
		"<tbody><tr><td>W250001</td><td>A</td></tr></tbody></table></body></html>"
	_, err := ParseStudentsPage(page)
	if err == nil {
		t.Fatal("ParseStudentsPage = nil error, want DriftError")
	}
	if _, ok := AsDrift(err); !ok {
		t.Fatalf("ParseStudentsPage error = %v, want DriftError", err)
	}
}

func studentByCode(students []normalize.LegacyStudent) map[string]normalize.LegacyStudent {
	out := make(map[string]normalize.LegacyStudent, len(students))
	for _, s := range students {
		out[s.WCode] = s
	}
	return out
}

func studentsSortedAscending(students []normalize.LegacyStudent) bool {
	for i := 1; i < len(students); i++ {
		if students[i-1].WCode >= students[i].WCode {
			return false
		}
	}
	return true
}

func legacyStudent(wcode, name, nickname, school, level, year, phone, email string) normalize.LegacyStudent {
	return normalize.LegacyStudent{
		WCode:    wcode,
		Name:     name,
		Nickname: nickname,
		School:   school,
		Level:    level,
		Year:     year,
		Phone:    phone,
		Email:    email,
	}
}
