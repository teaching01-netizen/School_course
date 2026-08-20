package parser

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateContract_ValidCourseListPage(t *testing.T) {
	if err := ValidateContract(CourseListContract, readFixture(t, "course_list.html")); err != nil {
		t.Errorf("ValidateContract: %v", err)
	}
}

func TestValidateContract_TitleMismatch(t *testing.T) {
	page := "<html><head><title>Students</title></head><body>" +
		"<table class=\"table\"><thead><tr><th>C-ID</th></tr></thead><tbody></tbody></table>" +
		"</body></html>"
	c := PageContract{
		PageType: "t", ParserVersion: 1, ExpectedTitle: "Course",
		RequiredHeaders: []string{"C-ID"}, MinColumns: 1, MaxColumns: 1,
	}
	err := ValidateContract(c, page)
	if err == nil {
		t.Fatal("expected drift, got nil")
	}
	d, ok := AsDrift(err)
	if !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	}
	if !strings.Contains(d.Reason, "title") {
		t.Errorf("drift reason %q does not mention title", d.Reason)
	}
}

func TestValidateContract_HeadersMismatch(t *testing.T) {
	// A missing required header must still be rejected.
	page := "<html><body><table class=\"table\"><thead><tr>" +
		"<th>Date</th><th>Begin</th><th>End</th><th>Duration</th><th>Classroom</th><th>Confirm</th><th>Unknown</th>" +
		"</tr></thead><tbody></tbody></table></body></html>"
	err := ValidateContract(CourseDetailContract, page)
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

func TestValidateContract_MissingTable(t *testing.T) {
	page := "<html><body><p>no table here</p></body></html>"
	if err := ValidateContract(CourseListContract, page); err == nil {
		t.Fatal("expected drift, got nil")
	} else if _, ok := AsDrift(err); !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	}
}

func TestValidateContract_LoginPage(t *testing.T) {
	err := ValidateContract(CourseListContract, readFixture(t, "login_page.html"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrLoginPage) {
		t.Errorf("errors.Is(err, ErrLoginPage) = false, got %v", err)
	}
	if _, ok := AsDrift(err); !ok {
		t.Errorf("expected *DriftError, got %T", err)
	}
}

func TestValidateContract_MissingFormField(t *testing.T) {
	// The table matches the required headers, but a required form field
	// label is absent from the header row.
	page := "<html><body><table class=\"table\"><thead><tr><th>C-ID</th></tr></thead><tbody></tbody></table></body></html>"
	c := PageContract{
		PageType: "t", ParserVersion: 1,
		RequiredHeaders:    []string{"C-ID"},
		RequiredFormFields: []string{"C-ID", "Nope"},
		MinColumns:         1, MaxColumns: 1,
	}
	err := ValidateContract(c, page)
	if err == nil {
		t.Fatal("expected drift, got nil")
	}
	d, ok := AsDrift(err)
	if !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	}
	if !strings.Contains(d.Reason, "form field") {
		t.Errorf("drift reason %q does not mention form field", d.Reason)
	}
}

func TestValidateContract_ColumnCountOutOfRange(t *testing.T) {
	// Header count (2) exceeds MaxColumns (1).
	page := "<html><body><table class=\"table\"><thead><tr><th>C-ID</th><th>C-Code</th></tr></thead><tbody></tbody></table></body></html>"
	c := PageContract{
		PageType: "t", ParserVersion: 1,
		RequiredHeaders: []string{"C-ID"}, MinColumns: 1, MaxColumns: 1,
	}
	err := ValidateContract(c, page)
	if err == nil {
		t.Fatal("expected drift, got nil")
	}
	d, ok := AsDrift(err)
	if !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	}
	if !strings.Contains(d.Reason, "column count") {
		t.Errorf("drift reason %q does not mention column count", d.Reason)
	}
}

func TestValidateContract_TableWithoutClass(t *testing.T) {
	page := "<html><body><table class=\"other\"><thead><tr><th>C-ID</th></tr></thead><tbody></tbody></table></body></html>"
	c := PageContract{
		PageType: "t", ParserVersion: 1,
		RequiredHeaders: []string{"C-ID"}, MinColumns: 1, MaxColumns: 1,
	}
	err := ValidateContract(c, page)
	if err == nil {
		t.Fatal("expected drift, got nil")
	}
	d, ok := AsDrift(err)
	if !ok {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	}
	if !strings.Contains(d.Reason, "class") {
		t.Errorf("drift reason %q does not mention class", d.Reason)
	}
}

func TestValidateContract_FindsContractTableAmongOthers(t *testing.T) {
	// An unrelated table first, then the contract table: must pass.
	page := "<html><body>" +
		"<table class=\"table\"><thead><tr><th>Other</th></tr></thead><tbody><tr><td>x</td></tr></tbody></table>" +
		"<table class=\"table\"><thead><tr><th>Date</th><th>Begin</th><th>End</th><th>Duration</th><th>Classroom</th><th>Confirm</th><th>By</th></tr></thead><tbody></tbody></table>" +
		"</body></html>"
	if err := ValidateContract(CourseDetailContract, page); err != nil {
		t.Errorf("ValidateContract: %v", err)
	}
}
