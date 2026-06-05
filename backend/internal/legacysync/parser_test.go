package legacysync

import (
	"testing"
	"time"
)

func TestParseScheduleTable_HtmlWithoutTable_ReturnsEmpty(t *testing.T) {
	rows, err := ParseScheduleTable("<html><body><p>No schedule here</p></body></html>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestParseScheduleTable_EmptyTable_ReturnsEmpty(t *testing.T) {
	html := `<html><body><table class="table"><thead><tr><th>Date</th></tr></thead><tbody></tbody></table></body></html>`
	rows, err := ParseScheduleTable(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestParseScheduleTable_ExtractsRows(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<h2>Schedule</h2>
<table class="table">
<thead>
<tr>
<th>Date</th><th>Begin</th><th>End</th><th>Duration</th><th>Classroom</th><th>Confirm</th><th>By</th>
</tr>
</thead>
<tbody>
<tr>
<td>Sat 23 May 26</td><td>13:00</td><td>16:20</td><td>03:20</td><td>[120204] 12A: Auditorium (XL)</td><td>Yes</td><td></td>
</tr>
<tr>
<td>Sun 24 May 26</td><td>09:00</td><td>11:30</td><td>02:30</td><td>[NOT SET]</td><td>No</td><td></td>
</tr>
</tbody>
</table>
</body></html>`

	rows, err := ParseScheduleTable(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Row 1: Sat 23 May 26 → 2026-05-23
	expectedDate1 := time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC)
	if !rows[0].Date.Equal(expectedDate1) {
		t.Errorf("row 0 date: expected %v, got %v", expectedDate1, rows[0].Date)
	}
	if rows[0].Begin != "13:00" {
		t.Errorf("row 0 begin: expected 13:00, got %s", rows[0].Begin)
	}
	if rows[0].End != "16:20" {
		t.Errorf("row 0 end: expected 16:20, got %s", rows[0].End)
	}
	if rows[0].Classroom != "[120204] 12A: Auditorium (XL)" {
		t.Errorf("row 0 classroom: expected full text, got %s", rows[0].Classroom)
	}

	// Row 2: Sun 24 May 26 → 2026-05-24, [NOT SET] classroom
	expectedDate2 := time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC)
	if !rows[1].Date.Equal(expectedDate2) {
		t.Errorf("row 1 date: expected %v, got %v", expectedDate2, rows[1].Date)
	}
	if rows[1].Classroom != "[NOT SET]" {
		t.Errorf("row 1 classroom: expected [NOT SET], got %s", rows[1].Classroom)
	}
}
