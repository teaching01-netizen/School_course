package xlsx

import (
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestParseXLSX_HeaderDiscoveryAndRows(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Bangkok")
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)

	_ = f.SetCellValue(sheet, "A1", "Total Students Each Cycle (New 2021)")
	_ = f.SetCellValue(sheet, "A2", "metadata")

	headers := []string{
		"Order Quote Updated Date", "Student Id", "First Name", "Last Name", "Nickname", "Secondary School", "Academic level",
		"Mobile Phone", "Course Name", "Cycle", "Hours", "Teacher(s)", "Primary E-mail", "Parent Name", "phoneparent", "emailparent",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		_ = f.SetCellValue(sheet, cell, h)
	}
	row := []any{"22/02/2026 01:58 PM", "W250084", "Jitirada", "Limsitisarn", "Jib", ".Satit Chula", "M.4", "092-6766560", "SAT Verbal", "May-Aug 2026", 56, "Nice", "a@b.com", "Parent", "081", "p@x.com"}
	for i, v := range row {
		cell, _ := excelize.CoordinatesToCellName(i+1, 5)
		_ = f.SetCellValue(sheet, cell, v)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	parsed, err := ParseXLSX(buf.Bytes(), loc)
	if err != nil {
		t.Fatalf("ParseXLSX: %v", err)
	}
	if len(parsed.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(parsed.Rows))
	}
	if parsed.Rows[0].WCode != "W250084" {
		t.Fatalf("unexpected wcode: %q", parsed.Rows[0].WCode)
	}
	if parsed.Rows[0].FirstName != "Jitirada" {
		t.Fatalf("unexpected first name: %q", parsed.Rows[0].FirstName)
	}
	if parsed.Rows[0].CourseName != "SAT Verbal" {
		t.Fatalf("unexpected course name: %q", parsed.Rows[0].CourseName)
	}
	if parsed.Rows[0].CycleLabel != "May-Aug 2026" {
		t.Fatalf("unexpected cycle: %q", parsed.Rows[0].CycleLabel)
	}
	if parsed.Rows[0].Hours == nil || *parsed.Rows[0].Hours != 56 {
		t.Fatalf("unexpected hours: %v", parsed.Rows[0].Hours)
	}
	if parsed.Rows[0].TeachersRaw != "Nice" {
		t.Fatalf("unexpected teachers: %q", parsed.Rows[0].TeachersRaw)
	}
}

func TestParseXLSX_RejectsMissingHeaders(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellValue(sheet, "A1", "Student Id")
	_ = f.SetCellValue(sheet, "B1", "Course Name")
	buf, _ := f.WriteToBuffer()
	_, err := ParseXLSX(buf.Bytes(), time.UTC)
	if err == nil {
		t.Fatal("expected error for missing headers")
	}
}

func TestParseXLSX_EmptyFile(t *testing.T) {
	f := excelize.NewFile()
	buf, _ := f.WriteToBuffer()
	_, err := ParseXLSX(buf.Bytes(), time.UTC)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestCleanPhoneSuffix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "clean number with hyphens", input: "081-9814300", want: "081-9814300"},
		{name: "number with trailing label", input: "081-5351563 Mom", want: "081-5351563"},
		{name: "different number with trailing label", input: "061-8159889 Mom", want: "061-8159889"},
		{name: "another clean hyphen number", input: "089-7808290", want: "089-7808290"},
		{name: "country code with spaces", input: "+66 81 234 5678", want: "+66 81 234 5678"},
		{name: "empty string", input: "", want: ""},
		{name: "string with no digits", input: "N/A", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanPhoneSuffix(tt.input)
			if got != tt.want {
				t.Errorf("cleanPhoneSuffix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseXLSX_SkipsEmptyRows(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Bangkok")
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)

	headers := []string{"Student Id", "First Name", "Last Name", "Course Name", "Cycle"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	row := []any{"W250001", "Test", "User", "Course A", "Cycle 1"}
	for i, v := range row {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet, cell, v)
	}
	partial := []any{"W250002", "No", "Course"}
	for i, v := range partial {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		_ = f.SetCellValue(sheet, cell, v)
	}

	buf, _ := f.WriteToBuffer()
	parsed, err := ParseXLSX(buf.Bytes(), loc)
	if err != nil {
		t.Fatalf("ParseXLSX: %v", err)
	}
	if len(parsed.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(parsed.Rows))
	}
}
