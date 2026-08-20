package crmimport

import (
	"testing"

	"warwick-institute/internal/crmimport/xlsx"
)

func TestDeduplicateRowsKeepsConflictingStudentData(t *testing.T) {
	rows := []xlsx.Row{
		{
			WCode:        "w250001",
			CourseName:   "Math",
			CycleLabel:   "Cycle A",
			FirstName:    "Alice",
			LastName:     "Alpha",
			PrimaryEmail: "alice@example.com",
		},
		{
			WCode:        "W250001",
			CourseName:   "Math",
			CycleLabel:   "Cycle A",
			FirstName:    "Alicia",
			LastName:     "Alpha",
			PrimaryEmail: "alicia@example.com",
		},
	}

	got := deduplicateRows(rows)
	if len(got) != 2 {
		t.Fatalf("deduplicateRows kept %d rows, want both conflicting source rows", len(got))
	}
}

func TestDeduplicateRowsDropsExactDuplicate(t *testing.T) {
	row := xlsx.Row{WCode: "W250002", CourseName: "Science", CycleLabel: "Cycle A", FirstName: "Bob"}

	got := deduplicateRows([]xlsx.Row{row, row})
	if len(got) != 1 {
		t.Fatalf("deduplicateRows kept %d rows, want one exact duplicate", len(got))
	}
}
