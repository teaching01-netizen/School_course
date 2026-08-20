package parser

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"warwick-institute/internal/legacysync/normalize"
)

// detailPage builds a minimal course-detail schedule page with the given
// rows, mirroring the v1 fixture structure (DOCTYPE, <h2>Schedule</h2>,
// table with class "table").
func detailPage(rows ...string) string {
	thead := "<thead><tr><th>Date</th><th>Begin</th><th>End</th><th>Duration</th><th>Classroom</th><th>Confirm</th><th>By</th></tr></thead>"
	return "<!DOCTYPE html>\n<html><body>\n<h2>Schedule</h2>\n<table class=\"table\">\n" +
		thead + "<tbody>" + strings.Join(rows, "") + "</tbody></table>\n</body></html>"
}

func detailRow(date, begin, end, duration, classroom, confirm, by string) string {
	return "<tr><td>" + date + "</td><td>" + begin + "</td><td>" + end + "</td><td>" + duration +
		"</td><td>" + classroom + "</td><td>" + confirm + "</td><td>" + by + "</td></tr>"
}

func TestCourseDetailParser_ProducesGolden(t *testing.T) {
	agg, err := ParseCourseDetail(readFixture(t, "course_detail_confirmed.html"))
	if err != nil {
		t.Fatalf("ParseCourseDetail: %v", err)
	}
	got, err := normalize.CanonicalJSON(agg)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	checkGolden(t, "course_detail_confirmed.golden.json", got)
}

func TestCourseDetailParser_ParsesConfirmationState(t *testing.T) {
	agg, err := ParseCourseDetail(readFixture(t, "course_detail_confirmed.html"))
	if err != nil {
		t.Fatalf("ParseCourseDetail: %v", err)
	}
	if len(agg.Schedules) != 2 {
		t.Fatalf("got %d schedules, want 2", len(agg.Schedules))
	}
	// Sorted by (Date, Begin, End): Sat 23 May 26 is row 1.
	s0 := agg.Schedules[0]
	if s0.Date != "2026-05-23" {
		t.Errorf("row 1 date = %s, want 2026-05-23", s0.Date)
	}
	if !s0.Confirmed {
		t.Errorf("row 1 Confirmed = false, want true")
	}
	if s0.ConfirmedBy != "AJ. TY" {
		t.Errorf("row 1 ConfirmedBy = %q, want AJ. TY", s0.ConfirmedBy)
	}
	if agg.Schedules[1].Confirmed {
		t.Errorf("row 2 Confirmed = true, want false")
	}

	// The unconfirmed fixture row must also parse as unconfirmed.
	agg2, err := ParseCourseDetail(readFixture(t, "course_detail_notset.html"))
	if err != nil {
		t.Fatalf("ParseCourseDetail(notset): %v", err)
	}
	if len(agg2.Schedules) != 1 {
		t.Fatalf("notset: got %d schedules, want 1", len(agg2.Schedules))
	}
	if agg2.Schedules[0].Confirmed {
		t.Errorf("notset row Confirmed = true, want false")
	}
}

func TestCourseDetailParser_ParsesUnassignedRoom(t *testing.T) {
	agg, err := ParseCourseDetail(readFixture(t, "course_detail_notset.html"))
	if err != nil {
		t.Fatalf("ParseCourseDetail: %v", err)
	}
	if len(agg.Schedules) != 1 {
		t.Fatalf("got %d schedules, want 1", len(agg.Schedules))
	}
	if agg.Schedules[0].Classroom != "" {
		t.Errorf("Classroom = %q, want \"\" for [NOT SET]", agg.Schedules[0].Classroom)
	}
	got, err := normalize.CanonicalJSON(agg)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	checkGolden(t, "course_detail_notset.golden.json", got)
}

func TestCourseDetailParser_EmptyScheduleIsValid(t *testing.T) {
	agg, err := ParseCourseDetail(readFixture(t, "course_detail_empty.html"))
	if err != nil {
		t.Fatalf("ParseCourseDetail(empty): %v", err)
	}
	if len(agg.Schedules) != 0 {
		t.Errorf("got %d schedules, want 0", len(agg.Schedules))
	}
	got, err := normalize.CanonicalJSON(agg)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	checkGolden(t, "course_detail_empty.golden.json", got)
}

func TestCourseDetailParser_RejectsMalformedHeaders(t *testing.T) {
	page := `<!DOCTYPE html><html><body><table class="table">
		<thead><tr><th>Date</th><th>Begin</th><th>End</th><th>Duration</th><th>Classroom</th><th>Confirm</th><th>Unknown</th></tr></thead>
		<tbody></tbody>
	</table></body></html>`
	_, err := ParseCourseDetail(page)
	if err == nil {
		t.Fatal("expected drift, got nil")
	}
	d, ok := AsDrift(err)
	if !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	}
	if !strings.Contains(d.Reason, "headers") {
		t.Errorf("drift reason %q does not mention headers", d.Reason)
	}
}

func TestCourseDetailParser_RejectsLoginPage(t *testing.T) {
	_, err := ParseCourseDetail(readFixture(t, "login_page.html"))
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

func TestCourseDetailParser_IsOrderIndependent(t *testing.T) {
	row1 := detailRow("Sat 23 May 26", "13:00", "16:20", "03:20", "[120204] 12A: Auditorium (XL)", "Yes", "AJ. TY")
	row2 := detailRow("Sun 24 May 26", "09:00", "11:30", "02:30", "[120205] 12B: Lab", "No", "")

	agg1, err := ParseCourseDetail(detailPage(row1, row2))
	if err != nil {
		t.Fatalf("ParseCourseDetail(order 1): %v", err)
	}
	agg2, err := ParseCourseDetail(detailPage(row2, row1))
	if err != nil {
		t.Fatalf("ParseCourseDetail(order 2): %v", err)
	}
	j1, err := normalize.CanonicalJSON(agg1)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	j2, err := normalize.CanonicalJSON(agg2)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if !bytes.Equal(j1, j2) {
		t.Errorf("row order changed canonical output:\n%s\n%s", j1, j2)
	}
}

func TestCourseDetailParser_UsesSemanticColumns(t *testing.T) {
	page := `<!DOCTYPE html><html><body><table class="table">
		<thead><tr><th>Classroom</th><th>Date</th><th>Confirm</th><th>End</th><th>By</th><th>Begin</th><th>Duration</th></tr></thead>
		<tbody><tr data-schedule-id="S-1"><td>[120204] 12A: Auditorium</td><td>Sat 23 May 26</td><td>Yes</td><td>16:20</td><td>AJ. TY</td><td>13:00</td><td>03:20</td></tr></tbody>
	</table></body></html>`

	agg, err := ParseCourseDetail(page)
	if err != nil {
		t.Fatalf("ParseCourseDetail: %v", err)
	}
	if len(agg.Schedules) != 1 {
		t.Fatalf("schedule count = %d, want 1", len(agg.Schedules))
	}
	got := agg.Schedules[0]
	if got.LegacyScheduleID != "S-1" || got.Date != "2026-05-23" || got.Begin != "13:00" ||
		got.End != "16:20" || got.ClassroomLegacyID != "120204" || !got.Confirmed || got.ConfirmedBy != "AJ. TY" {
		t.Fatalf("parsed schedule = %+v", got)
	}
}

func TestCourseDetailParser_RejectsDuplicateScheduleIDs(t *testing.T) {
	page := `<!DOCTYPE html><html><body><table class="table">
		<thead><tr><th>Date</th><th>Begin</th><th>End</th><th>Duration</th><th>Classroom</th><th>Confirm</th><th>By</th></tr></thead>
		<tbody>
			<tr data-schedule-id="S-1"><td>Sat 23 May 26</td><td>13:00</td><td>16:20</td><td>03:20</td><td>[120204] 12A</td><td>Yes</td><td>AJ. TY</td></tr>
			<tr data-schedule-id="S-1"><td>Sun 24 May 26</td><td>09:00</td><td>11:30</td><td>02:30</td><td>[120205] 12B</td><td>No</td><td></td></tr>
		</tbody>
	</table></body></html>`

	_, err := ParseCourseDetail(page)
	if err == nil {
		t.Fatal("expected duplicate schedule identity error")
	}
	if !strings.Contains(err.Error(), "duplicate schedule") {
		t.Fatalf("error = %v, want duplicate schedule identity", err)
	}
}

func FuzzParseCourseDetail(f *testing.F) {
	for _, name := range []string{
		"course_detail_confirmed.html",
		"course_detail_notset.html",
		"course_detail_empty.html",
		"course_detail_malformed.html",
		"login_page.html",
	} {
		b, err := readFixtureBytes(name)
		if err != nil {
			f.Fatalf("reading seed %s: %v", name, err)
		}
		f.Add(string(b))
	}
	f.Fuzz(func(t *testing.T, page string) {
		agg, err := ParseCourseDetail(page)
		if err != nil {
			return
		}
		j1, err := normalize.CanonicalJSON(agg)
		if err != nil {
			t.Fatalf("CanonicalJSON: %v", err)
		}
		// Determinism: parsing the same page twice must yield
		// byte-identical canonical output.
		agg2, err := ParseCourseDetail(page)
		if err != nil {
			t.Fatalf("second parse failed: %v", err)
		}
		j2, err := normalize.CanonicalJSON(agg2)
		if err != nil {
			t.Fatalf("CanonicalJSON: %v", err)
		}
		if !bytes.Equal(j1, j2) {
			t.Fatalf("non-deterministic canonical output:\n%s\n%s", j1, j2)
		}
	})
}

// TestCourseDetailParser_ParsesV2PageLayout verifies the parser against the
// current legacy detail page: 8 columns (trailing headerless check-in
// column), the classroom rendered as a screen link plus a print-only label,
// and the confirm state rendered as a form button.
func TestCourseDetailParser_ParsesV2PageLayout(t *testing.T) {
	agg, err := ParseCourseDetail(readFixture(t, "course_detail_v2.html"))
	if err != nil {
		t.Fatalf("ParseCourseDetail(v2): %v", err)
	}
	if len(agg.Schedules) != 2 {
		t.Fatalf("got %d schedules, want 2", len(agg.Schedules))
	}
	s0 := agg.Schedules[0]
	if s0.Date != "2026-05-23" {
		t.Errorf("row 1 date = %s, want 2026-05-23", s0.Date)
	}
	if s0.Begin != "13:00" || s0.End != "16:20" {
		t.Errorf("row 1 time = %s-%s, want 13:00-16:20", s0.Begin, s0.End)
	}
	// The classroom must come from the link text once, not the link plus the
	// duplicated print-only label.
	if s0.ClassroomLegacyID != "120204" {
		t.Errorf("row 1 classroom id = %q, want 120204", s0.ClassroomLegacyID)
	}
	if s0.Classroom != "[120204] 12A: Auditorium (XL)" {
		t.Errorf("row 1 classroom = %q, want %q", s0.Classroom, "[120204] 12A: Auditorium (XL)")
	}
	if !s0.Confirmed {
		t.Errorf("row 1 Confirmed = false, want true (button shows Yes)")
	}

	got, err := normalize.CanonicalJSON(agg)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	checkGolden(t, "course_detail_v2.golden.json", got)
}

// TestCourseDetailParser_ExtractsScheduleIDFromRowLinks verifies the stable
// source schedule identity is recovered from the row's per-schedule links
// (courseScheduleId=NNN in the ClassroomSet/CheckIn hrefs). The live command
// previously replaced row identity with ordinal hashes, so inserting a row on
// the source silently re-keyed every following session.
func TestCourseDetailParser_ExtractsScheduleIDFromRowLinks(t *testing.T) {
	agg, err := ParseCourseDetail(readFixture(t, "course_detail_v2.html"))
	if err != nil {
		t.Fatalf("ParseCourseDetail(v2): %v", err)
	}
	if len(agg.Schedules) != 2 {
		t.Fatalf("got %d schedules, want 2", len(agg.Schedules))
	}
	if agg.Schedules[0].LegacyScheduleID != "109541" {
		t.Errorf("row 1 legacy schedule id = %q, want 109541", agg.Schedules[0].LegacyScheduleID)
	}
	if agg.Schedules[1].LegacyScheduleID != "109542" {
		t.Errorf("row 2 legacy schedule id = %q, want 109542", agg.Schedules[1].LegacyScheduleID)
	}
}

// TestCourseDetailParser_AttrScheduleIDWinsOverLink verifies the explicit
// data-schedule-id attribute takes precedence when a row also carries
// courseScheduleId links, and rows without either keep an empty ID.
func TestCourseDetailParser_AttrScheduleIDWinsOverLink(t *testing.T) {
	row1 := `<tr data-schedule-id="S-1"><td>Sat 23 May 26</td><td>13:00</td><td>16:20</td><td>03:20</td><td><a href="/Admin/Courses/CheckIn?courseScheduleId=999&amp;courseId=1">check-in</a></td><td>Yes</td><td></td></tr>`
	row2 := `<tr><td>Sun 24 May 26</td><td>09:00</td><td>11:30</td><td>02:30</td><td>[120205] 12B</td><td>No</td><td></td></tr>`
	agg, err := ParseCourseDetail(detailPage(row1, row2))
	if err != nil {
		t.Fatalf("ParseCourseDetail: %v", err)
	}
	if agg.Schedules[0].LegacyScheduleID != "S-1" {
		t.Errorf("row 1 legacy schedule id = %q, want S-1 (attribute precedence)", agg.Schedules[0].LegacyScheduleID)
	}
	if agg.Schedules[1].LegacyScheduleID != "" {
		t.Errorf("row 2 legacy schedule id = %q, want empty when row carries no identity", agg.Schedules[1].LegacyScheduleID)
	}
}

// TestCourseDetailParser_V2EmptyScheduleSkipsPlaceholder verifies the v2 page
// with no schedule rows: the colspan placeholder and the hours-summary
// footer rows must be skipped, yielding an empty aggregate.
func TestCourseDetailParser_V2EmptyScheduleSkipsPlaceholder(t *testing.T) {
	agg, err := ParseCourseDetail(readFixture(t, "course_detail_v2_empty.html"))
	if err != nil {
		t.Fatalf("ParseCourseDetail(v2 empty): %v", err)
	}
	if len(agg.Schedules) != 0 {
		t.Fatalf("got %d schedules, want 0", len(agg.Schedules))
	}
}
