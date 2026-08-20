package parser

import (
	"strings"
	"testing"
)

func TestCourseDetailParser_RejectsTruncatedRow(t *testing.T) {
	page := detailPage(`<tr><td>Sat 23 May 26</td><td>13:00</td><td>16:20</td><td>03:20</td><td>[120204] 12A</td><td>Yes</td></tr>`)
	_, err := ParseCourseDetail(page)
	if err == nil {
		t.Fatal("expected truncated row drift")
	}
	if !strings.Contains(err.Error(), "row has") {
		t.Fatalf("error = %v, want row-shape detail", err)
	}
}

func TestCourseDetailParser_RejectsUnknownConfirmation(t *testing.T) {
	page := detailPage(detailRow("Sat 23 May 26", "13:00", "16:20", "03:20", "[120204] 12A", "Maybe", "AJ. TY"))
	_, err := ParseCourseDetail(page)
	if err == nil {
		t.Fatal("expected confirmation drift")
	}
	if !strings.Contains(err.Error(), "invalid confirm") {
		t.Fatalf("error = %v, want confirmation detail", err)
	}
}

func TestCourseDetailParser_BoundsScheduleRows(t *testing.T) {
	rows := make([]string, maxCourseDetailRows+1)
	row := detailRow("Sat 23 May 26", "13:00", "16:20", "03:20", "[120204] 12A", "Yes", "AJ. TY")
	for i := range rows {
		rows[i] = row
	}
	_, err := ParseCourseDetail(detailPage(rows...))
	if err == nil {
		t.Fatal("expected oversized schedule page drift")
	}
	if !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("error = %v, want row limit detail", err)
	}
}
