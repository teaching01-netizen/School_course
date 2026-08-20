package parser

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"warwick-institute/internal/legacysync/normalize"
)

// courseRow is a builder for one course-list data row (11 cells, matching
// the observed page layout).
type courseRow struct {
	id      string
	code    string
	teacher string // "[207] AJ. TY", or "" for an empty cell
	subject string
	hours   string
	expired string
	typ     string
	status  string
	hrefID  string
}

func (r courseRow) html() string {
	return "<tr>" +
		"<td>" + r.id + "</td>" +
		"<td>" + r.code + "</td>" +
		"<td>26</td>" +
		"<td>" + r.teacher + "</td>" +
		"<td>" + r.subject + "</td>" +
		"<td>" + r.hours + "</td>" +
		"<td>1</td>" +
		"<td>" + r.expired + "</td>" +
		"<td>" + r.typ + "</td>" +
		"<td>" + r.status + "</td>" +
		"<td><a href=\"/Admin/Courses/Detail?id=" + r.hrefID + "\">detail</a></td>" +
		"</tr>"
}

// courseListPage builds a minimal course_list page with the given rows.
func courseListPage(rows ...string) string {
	thead := "<thead><tr><th>C-ID</th><th>C-Code</th><th>Year</th><th>Teacher</th><th>Subject</th><th>Hour</th><th>Student</th><th>Expired</th><th>Type</th><th>Status</th><th></th></tr></thead>"
	return "<!DOCTYPE html><html><head><title>Course</title></head><body>" +
		"<table class=\"table table-hover\">" + thead + "<tbody>" + strings.Join(rows, "") + "</tbody></table>" +
		"</body></html>"
}

func readFixtureBytes(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join("testdata", name))
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := readFixtureBytes(name)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return string(b)
}

func TestCourseListParser_ProducesGolden(t *testing.T) {
	res, err := ParseCourseList(readFixture(t, "course_list.html"))
	if err != nil {
		t.Fatalf("ParseCourseList: %v", err)
	}
	got, err := normalize.CanonicalJSON(res)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	checkGolden(t, "course_list.golden.json", got)

	// The fixture rows are deliberately out of order (7187, 7184, 7183);
	// output must be sorted ascending by legacy_id.
	wantIDs := []string{"7183", "7184", "7187"}
	if len(res.Courses) != len(wantIDs) {
		t.Fatalf("got %d courses, want %d", len(res.Courses), len(wantIDs))
	}
	for i, c := range res.Courses {
		if c.LegacyID != wantIDs[i] {
			t.Errorf("course %d legacy_id = %s, want %s (sorted)", i, c.LegacyID, wantIDs[i])
		}
	}

	// The fixture carries one attendee sub-row ([W999901] Test Student (TS))
	// immediately after the 7184 row; the roster must be attached to it.
	if len(res.Courses[1].Attendees) != 1 || res.Courses[1].Attendees[0] != "W999901 Test Student (TS)" {
		t.Errorf("course 7184 attendees = %v, want [W999901 Test Student (TS)]", res.Courses[1].Attendees)
	}

	// Master data is distinct and sorted even though teachers repeat across
	// rows (78 appears twice, 207 once) and rows arrive out of id order.
	wantTeacherIDs := []string{"104", "207", "78"}
	wantSubjectIDs := []string{"06", "08", "811"}
	if len(res.Teachers) != len(wantTeacherIDs) || len(res.Subjects) != len(wantSubjectIDs) {
		t.Fatalf("got %d teachers / %d subjects, want %d / %d", len(res.Teachers), len(res.Subjects), len(wantTeacherIDs), len(wantSubjectIDs))
	}
	for i, teacher := range res.Teachers {
		if teacher.LegacyID != wantTeacherIDs[i] {
			t.Errorf("teacher %d legacy_id = %s, want %s (sorted)", i, teacher.LegacyID, wantTeacherIDs[i])
		}
		if teacher.Name == "" || !teacher.IsActive {
			t.Errorf("teacher %s missing name or inactive: %+v", teacher.LegacyID, teacher)
		}
	}
	for i, subject := range res.Subjects {
		if subject.LegacyID != wantSubjectIDs[i] {
			t.Errorf("subject %d legacy_id = %s, want %s (sorted, unpadded)", i, subject.LegacyID, wantSubjectIDs[i])
		}
		if subject.Name == "" {
			t.Errorf("subject %s missing name", subject.LegacyID)
		}
	}
}

// TestCourseListParser_ParsesArchivedPageFixture locks in the contract of
// the live archive-search results (testdata/course_list_archived.html): the
// archived table has the same title/headers/11-column shape as the plain
// listing, but the Status cell literally reads "Archive".
func TestCourseListParser_ParsesArchivedPageFixture(t *testing.T) {
	res, err := ParseCourseList(readFixture(t, "course_list_archived.html"))
	if err != nil {
		t.Fatalf("ParseCourseList: %v", err)
	}
	if len(res.Courses) != 3 {
		t.Fatalf("got %d courses, want 3", len(res.Courses))
	}
	// Rows arrive in id-descending order on the live page; output is sorted
	// ascending like the plain listing.
	wantIDs := []string{"7318", "7319", "7323"}
	for i, c := range res.Courses {
		if c.LegacyID != wantIDs[i] {
			t.Errorf("course %d legacy_id = %s, want %s (sorted)", i, c.LegacyID, wantIDs[i])
		}
		if c.Status != "archived" {
			t.Errorf("course %s status = %q, want archived (live page writes Archive)", c.LegacyID, c.Status)
		}
	}
	// The attendee sub-row after 7323 belongs to it (identity sanitized).
	if len(res.Courses[2].Attendees) != 1 || res.Courses[2].Attendees[0] != "W999902 Test Student (TS2)" {
		t.Errorf("course 7323 attendees = %v, want [W999902 Test Student (TS2)]", res.Courses[2].Attendees)
	}
	wantTeacherIDs := []string{"51", "78", "93"}
	for i, teacher := range res.Teachers {
		if teacher.LegacyID != wantTeacherIDs[i] {
			t.Errorf("teacher %d legacy_id = %s, want %s", i, teacher.LegacyID, wantTeacherIDs[i])
		}
	}
	// 7319 is a General/Activity course; hours parse as numbers, not drift.
	if res.Courses[1].Type != "General" || res.Courses[1].Hours != "2" {
		t.Errorf("course 7319 type/hours = %q/%q, want General/2", res.Courses[1].Type, res.Courses[1].Hours)
	}
}

func TestCourseListParser_MapsStatusVocabulary(t *testing.T) {
	page := courseListPage(
		courseRow{id: "1", code: "C1", teacher: "[1] T", subject: "[1] S", hours: "10", expired: "01/01/26", typ: "Private", status: "Active", hrefID: "1"}.html(),
		courseRow{id: "2", code: "C2", teacher: "[2] T", subject: "[2] S", hours: "", expired: "", typ: "General", status: "Draft", hrefID: "2"}.html(),
		courseRow{id: "3", code: "C3", teacher: "[3] T", subject: "[3] S", hours: "", expired: "", typ: "", status: "Archived", hrefID: "3"}.html(),
		// The live archive-search page writes "Archive" (no "d"); both spellings
		// must map to archived.
		courseRow{id: "4", code: "C4", teacher: "[4] T", subject: "[4] S", hours: "", expired: "", typ: "", status: "Archive", hrefID: "4"}.html(),
	)
	res, err := ParseCourseList(page)
	if err != nil {
		t.Fatalf("ParseCourseList: %v", err)
	}
	if len(res.Courses) != 4 {
		t.Fatalf("got %d courses, want 4", len(res.Courses))
	}
	if res.Courses[0].Status != "active" {
		t.Errorf("Active -> %q, want active", res.Courses[0].Status)
	}
	if res.Courses[1].Status != "draft" {
		t.Errorf("Draft -> %q, want draft", res.Courses[1].Status)
	}
	if res.Courses[2].Status != "archived" {
		t.Errorf("Archived -> %q, want archived", res.Courses[2].Status)
	}
	if res.Courses[3].Status != "archived" {
		t.Errorf("Archive -> %q, want archived", res.Courses[3].Status)
	}

	// Unknown status vocabulary must drift.
	bad := courseListPage(courseRow{id: "9", code: "C9", teacher: "[9] T", subject: "[9] S", hours: "", expired: "", typ: "", status: "Bogus", hrefID: "9"}.html())
	if _, err := ParseCourseList(bad); err == nil {
		t.Fatal("expected drift for unknown status, got nil")
	} else if d, ok := AsDrift(err); !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	} else if !strings.Contains(d.Reason, "status") {
		t.Errorf("drift reason %q does not mention status", d.Reason)
	}
}

// attendeeRow builds one attendee sub-row as observed on the live page:
// <td colspan="5"></td><td colspan="6">[W250025] Nutnicha Marungrueng (Nicha)</td>.
// Multiple entries in one cell are joined with <br> (the source's
// per-cell entry separation is not stable across pages).
func attendeeRow(entries ...string) string {
	return "<tr><td colspan=\"5\"></td><td colspan=\"6\">" + strings.Join(entries, "<br>") + "</td></tr>"
}

func TestCourseListParser_ParsesAttendeeRosters(t *testing.T) {
	page := courseListPage(
		courseRow{id: "1", code: "C1", teacher: "[1] T", subject: "[1] S", hours: "10", expired: "01/01/26", typ: "Private", status: "Active", hrefID: "1"}.html(),
		// One attendee sub-row, then another for the same course carrying a
		// (nickname) suffix.
		attendeeRow("[W111111] Ana Garcia"),
		attendeeRow("[W222222] Bob Smith (Bobby)"),
		courseRow{id: "2", code: "C2", teacher: "[2] T", subject: "[2] S", hours: "", expired: "", typ: "General", status: "Draft", hrefID: "2"}.html(),
		// Course 2 has no roster.
		courseRow{id: "3", code: "C3", teacher: "[3] T", subject: "[3] S", hours: "", expired: "", typ: "", status: "Archived", hrefID: "3"}.html(),
		// One sub-row carrying two <br>-separated entries, deliberately out
		// of wcode order: attendees must come out sorted ascending.
		attendeeRow("[W444444] Dan Kim (DK)", "[W333333] Cat Nguyen"),
	)
	res, err := ParseCourseList(page)
	if err != nil {
		t.Fatalf("ParseCourseList: %v", err)
	}
	if len(res.Courses) != 3 {
		t.Fatalf("got %d courses, want 3", len(res.Courses))
	}
	want := map[string][]string{
		"1": {"W111111 Ana Garcia", "W222222 Bob Smith (Bobby)"},
		"2": nil,
		"3": {"W333333 Cat Nguyen", "W444444 Dan Kim (DK)"},
	}
	for _, course := range res.Courses {
		got := course.Attendees
		wantAtt := want[course.LegacyID]
		if len(got) != len(wantAtt) {
			t.Errorf("course %s attendees = %v, want %v", course.LegacyID, got, wantAtt)
			continue
		}
		for i := range got {
			if got[i] != wantAtt[i] {
				t.Errorf("course %s attendee %d = %q, want %q", course.LegacyID, i, got[i], wantAtt[i])
			}
		}
	}
}

func TestCourseListParser_AttendeeSubRowWithoutCourseDrifts(t *testing.T) {
	page := courseListPage(attendeeRow("[W111111] Ana Garcia"))
	if _, err := ParseCourseList(page); err == nil {
		t.Fatal("expected drift, got nil")
	} else if d, ok := AsDrift(err); !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	} else if !strings.Contains(d.Reason, "attendee") {
		t.Errorf("drift reason %q does not mention attendee", d.Reason)
	}
}

// The live archived page contains one attendee entry with a lowercase w
// ("[w210035] ..."); the sub-row must still be recognized and the wcode
// normalized to uppercase so downstream matching stays consistent.
func TestCourseListParser_AttendeeLowercaseWCodeNormalized(t *testing.T) {
	page := courseListPage(
		courseRow{id: "1", code: "C1", teacher: "[1] T", subject: "[1] S", hours: "", expired: "", typ: "", status: "Active", hrefID: "1"}.html(),
		attendeeRow("[w210035] Kittisak Subroungtong (Geng)"),
	)
	res, err := ParseCourseList(page)
	if err != nil {
		t.Fatalf("ParseCourseList: %v", err)
	}
	if len(res.Courses) != 1 {
		t.Fatalf("got %d courses, want 1", len(res.Courses))
	}
	want := []string{"W210035 Kittisak Subroungtong (Geng)"}
	if len(res.Courses[0].Attendees) != 1 || res.Courses[0].Attendees[0] != want[0] {
		t.Errorf("attendees = %v, want %v (wcode uppercased)", res.Courses[0].Attendees, want)
	}
}

func TestMergeCourseLists(t *testing.T) {
	plain := &CourseListResult{
		Courses: []normalize.LegacyCourse{
			{LegacyID: "1", Code: "C1", Status: "active"},
			{LegacyID: "2", Code: "C2", Status: "draft"},
		},
		Teachers: []normalize.LegacyTeacher{
			{LegacyID: "11", Name: "T1", IsActive: true},
		},
		Subjects: []normalize.LegacySubject{
			{LegacyID: "21", Name: "S1"},
		},
	}
	archived := &CourseListResult{
		Courses: []normalize.LegacyCourse{
			// Same legacy id as a plain course: plain must win.
			{LegacyID: "2", Code: "C2", Status: "archived"},
			{LegacyID: "3", Code: "C3", Status: "archived"},
		},
		Teachers: []normalize.LegacyTeacher{
			{LegacyID: "11", Name: "ArchivedT1", IsActive: true},
			{LegacyID: "12", Name: "T2", IsActive: true},
		},
		Subjects: []normalize.LegacySubject{
			{LegacyID: "21", Name: "ArchivedS1"},
			{LegacyID: "22", Name: "S2"},
		},
	}

	got := MergeCourseLists(plain, archived)
	if len(got.Courses) != 3 {
		t.Fatalf("merged courses = %d, want 3", len(got.Courses))
	}
	for i, wantID := range []string{"1", "2", "3"} {
		if got.Courses[i].LegacyID != wantID {
			t.Errorf("merged course %d legacy_id = %s, want %s (sorted)", i, got.Courses[i].LegacyID, wantID)
		}
	}
	if got.Courses[1].Status != "draft" {
		t.Errorf("duplicate legacy id 2 status = %q, want draft (plain wins)", got.Courses[1].Status)
	}
	if got.Courses[2].Status != "archived" {
		t.Errorf("archived-only legacy id 3 status = %q, want archived", got.Courses[2].Status)
	}
	if len(got.Teachers) != 2 {
		t.Fatalf("merged teachers = %d, want 2", len(got.Teachers))
	}
	for i, wantID := range []string{"11", "12"} {
		if got.Teachers[i].LegacyID != wantID {
			t.Errorf("merged teacher %d legacy_id = %s, want %s (sorted)", i, got.Teachers[i].LegacyID, wantID)
		}
	}
	if got.Teachers[0].Name != "T1" {
		t.Errorf("duplicate teacher 11 name = %q, want T1 (plain wins)", got.Teachers[0].Name)
	}
	if len(got.Subjects) != 2 {
		t.Fatalf("merged subjects = %d, want 2", len(got.Subjects))
	}
	if got.Subjects[0].LegacyID != "21" || got.Subjects[1].LegacyID != "22" {
		t.Errorf("merged subject ids = [%s %s], want [21 22] (sorted)", got.Subjects[0].LegacyID, got.Subjects[1].LegacyID)
	}
	if got.Subjects[0].Name != "S1" {
		t.Errorf("duplicate subject 21 name = %q, want S1 (plain wins)", got.Subjects[0].Name)
	}

	// Nil or empty archived leaves the plain list intact.
	if got := MergeCourseLists(plain, nil); len(got.Courses) != 2 {
		t.Errorf("merge with nil archived = %d courses, want 2", len(got.Courses))
	}
	if got := MergeCourseLists(nil, archived); len(got.Courses) != 2 || got.Courses[0].LegacyID != "2" || got.Courses[1].LegacyID != "3" {
		t.Errorf("merge with nil plain = %+v, want archived courses 2 and 3", got.Courses)
	}
	if got := MergeCourseLists(nil, nil); got == nil || len(got.Courses) != 0 {
		t.Errorf("merge with both nil = %+v, want empty result", got)
	}
}

func TestCourseListParser_DetailLinkMismatchDrifts(t *testing.T) {
	page := courseListPage(courseRow{id: "7", code: "C7", teacher: "[7] T", subject: "[7] S", hours: "", expired: "", typ: "", status: "Active", hrefID: "8"}.html())
	if _, err := ParseCourseList(page); err == nil {
		t.Fatal("expected drift, got nil")
	} else if d, ok := AsDrift(err); !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	} else if !strings.Contains(d.Reason, "detail link") {
		t.Errorf("drift reason %q does not mention detail link", d.Reason)
	}
}

func TestCourseListParser_TeacherCellWithoutBracketDrifts(t *testing.T) {
	// Teacher cell without the [<id>] prefix.
	page := courseListPage(courseRow{id: "7", code: "C7", teacher: "AJ. TY", subject: "[1] S", hours: "", expired: "", typ: "", status: "Active", hrefID: "7"}.html())
	if _, err := ParseCourseList(page); err == nil {
		t.Fatal("expected drift, got nil")
	} else if d, ok := AsDrift(err); !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	} else if !strings.Contains(d.Reason, "teacher cell format") {
		t.Errorf("drift reason %q does not mention teacher cell format", d.Reason)
	}

	// The same bracket rule applies to the subject cell.
	page = courseListPage(courseRow{id: "7", code: "C7", teacher: "[7] T", subject: "SAT Math", hours: "", expired: "", typ: "", status: "Active", hrefID: "7"}.html())
	if _, err := ParseCourseList(page); err == nil {
		t.Fatal("expected drift for un-bracketed subject cell, got nil")
	} else if _, ok := AsDrift(err); !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	}
}

func TestCourseListParser_RejectsLoginPage(t *testing.T) {
	_, err := ParseCourseList(readFixture(t, "login_page.html"))
	if err == nil {
		t.Fatal("expected error for login page, got nil")
	}
	if !errors.Is(err, ErrLoginPage) {
		t.Errorf("errors.Is(err, ErrLoginPage) = false, got %v", err)
	}
	if _, ok := AsDrift(err); !ok {
		t.Errorf("expected *DriftError, got %T", err)
	}
}

func TestCourseListParser_RejectsMissingTable(t *testing.T) {
	page := "<html><head><title>Course</title></head><body><p>no table</p></body></html>"
	if _, err := ParseCourseList(page); err == nil {
		t.Fatal("expected drift for missing table, got nil")
	} else if _, ok := AsDrift(err); !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	}
}

func FuzzParseCourseList(f *testing.F) {
	for _, name := range []string{"course_list.html", "login_page.html"} {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatalf("reading seed %s: %v", name, err)
		}
		f.Add(string(b))
	}
	f.Fuzz(func(t *testing.T, page string) {
		res, err := ParseCourseList(page)
		if err != nil {
			return
		}
		j1, err := normalize.CanonicalJSON(res)
		if err != nil {
			t.Fatalf("CanonicalJSON: %v", err)
		}
		// Determinism: parsing the same page twice must yield
		// byte-identical canonical output.
		res2, err := ParseCourseList(page)
		if err != nil {
			t.Fatalf("second parse failed: %v", err)
		}
		j2, err := normalize.CanonicalJSON(res2)
		if err != nil {
			t.Fatalf("CanonicalJSON: %v", err)
		}
		if !bytes.Equal(j1, j2) {
			t.Fatalf("non-deterministic canonical output:\n%s\n%s", j1, j2)
		}
	})
}
