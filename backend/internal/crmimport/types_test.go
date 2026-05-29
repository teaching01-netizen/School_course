package crmimport

import (
	"testing"

	"warwick-institute/internal/crmimport/crmtypes"
	"warwick-institute/internal/crmimport/xlsx"
)

func TestCourseFilterNormalizeAndValidate(t *testing.T) {
	f := crmtypes.CourseFilter{
		CycleLabels: []string{" May-Aug 2026 ", "May-Aug 2026"},
	}
	f.Normalize()
	if err := f.Validate(); err != nil {
		t.Fatalf("expected valid filter, got %v", err)
	}
	if len(f.CycleLabels) != 1 || f.CycleLabels[0] != "May-Aug 2026" {
		t.Fatalf("expected deduped cycle labels, got %#v", f.CycleLabels)
	}
}

func TestCourseFilterDefaults(t *testing.T) {
	var f crmtypes.CourseFilter
	f.Normalize()
	if f.CycleBlankMode != crmtypes.BlankModeAny {
		t.Fatalf("expected default blank mode 'any', got %q", f.CycleBlankMode)
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("expected valid filter, got %v", err)
	}
}

func TestCourseFilterInvalidBlankMode(t *testing.T) {
	f := crmtypes.CourseFilter{
		CycleBlankMode: "invalid",
	}
	f.Normalize()
	if err := f.Validate(); err == nil {
		t.Fatal("expected invalid blank mode error")
	}
}

func TestRowHash(t *testing.T) {
	r := xlsx.Row{
		WCode:      "W250084",
		CourseName: "SAT Verbal",
		CycleLabel: "May-Aug 2026",
	}
	h1 := r.Hash()
	h2 := r.Hash()
	if h1 != h2 {
		t.Fatalf("expected same hash for same input, got %q vs %q", h1, h2)
	}

	r2 := r
	r2.TeachersRaw = "John"
	if r2.Hash() == h1 {
		t.Fatal("expected different hash for different teachers_raw")
	}
}
