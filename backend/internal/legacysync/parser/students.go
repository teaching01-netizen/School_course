package parser

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/net/html"

	"warwick-institute/internal/legacysync/normalize"
)

// StudentsPageContract is the contract for /Admin/Students (the student
// directory page). Headers observed on the live page: W-Code, Name,
// Nickname, School, Level, Year, Phone, Email, Mobile, plus an empty action
// column (10 columns total). One extra legacy column (LastName) is rendered
// as an HTML comment between Name and Nickname and is not part of the
// element structure.
var StudentsPageContract = PageContract{
	PageType:      "students_page",
	ParserVersion: 1,
	ExpectedTitle: "Student",
	RequiredHeaders: []string{
		"W-Code", "Name", "Nickname", "School", "Level", "Year", "Phone", "Email",
	},
	MinColumns: 10,
	MaxColumns: 10,
}

// StudentsPageResult is the parsed student directory.
type StudentsPageResult struct {
	Students []normalize.LegacyStudent `json:"students"`
}

// wcodeRe matches a student wcode, e.g. "W250025". Reused by the roster
// entry splitter in the reconcile package (same shape, same source site).
var wcodeOnlyRe = regexp.MustCompile(`^W\d+$`)

// embeddedWcodeRe finds a student wcode inside a legacy-import cell that
// carries a date prefix ("29/04/2019 W190168").
var embeddedWcodeRe = regexp.MustCompile(`W\d+`)

// legacyMarkerRe matches the "no value" markers the old site renders in
// student cells: "-" and "- -" (whole cell only, whitespace-tolerant).
var legacyMarkerRe = regexp.MustCompile(`^-\s*-\s*$|^-$`)

// ParseStudentsPage parses the /Admin/Students page. It returns a
// *DriftError on any contract mismatch and never returns partial data.
//
// Row shapes (observed on the live page):
//   - Modern row: 10 cells, W-code in cell 0, then Name, Nickname, School,
//     Level, Year, Phone, Email, mobile-access flag, action link.
//   - Legacy-import row (older records): 10 cells whose profile columns are
//     shifted by the schema that existed when they were imported. Two
//     variants occur:
//   - "clean": [reference, date, W-code, Name, Nickname, School, Level,
//     Phone] (W-code in cell 2);
//   - "embedded": the W-code is glued to a date in cell 1
//     ("29/04/2019 W190168") and first/last names occupy two cells
//     ([..., "W-code", FirstName, LastName, Nickname, School, Level,
//     Phone]).
//
// Rows that match neither layout (the "Grand Total" footer row, Excel-
// corrupted imports, the "No students yet." placeholder) are skipped, not
// drift: the page legitimately contains them and the sync must not fail on
// them. Cells holding the legacy "-"/"- -" empty markers become "". The
// phone cell is kept only when it contains at least 7 digits, so corrupted
// values ("1200", "Mar-May 2021") never reach the students table. The
// output is deterministic: unique students (first page occurrence wins)
// sorted ascending by wcode.
func ParseStudentsPage(pageHTML string) (*StudentsPageResult, error) {
	table, err := validateAndFindTable(StudentsPageContract, pageHTML)
	if err != nil {
		return nil, err
	}
	tbody := firstTbody(table)
	if tbody == nil {
		return nil, drift(StudentsPageContract, "tbody not found")
	}

	byCode := make(map[string]normalize.LegacyStudent)
	order := make([]string, 0, len(byCode))
	for _, tr := range rowNodes(tbody) {
		student, ok := parseStudentRow(tr)
		if !ok {
			continue
		}
		if _, exists := byCode[student.WCode]; !exists {
			order = append(order, student.WCode)
		}
		byCode[student.WCode] = student
	}
	sort.Strings(order)
	out := &StudentsPageResult{Students: make([]normalize.LegacyStudent, 0, len(order))}
	for _, wcode := range order {
		out.Students = append(out.Students, byCode[wcode])
	}
	return out, nil
}

// parseStudentRow classifies one <tr> of the students table into a student
// profile. Rows that do not match a known layout are skipped (ok=false).
func parseStudentRow(tr *html.Node) (normalize.LegacyStudent, bool) {
	cells := tdChildren(tr)
	// The commented-out legacy LastName cell is a comment node, so
	// tdChildren (which selects elements only) already yields the 10
	// effective columns.
	if len(cells) != 10 {
		return normalize.LegacyStudent{}, false
	}
	cell := func(i int) string {
		return normalize.NormalizeText(textOf(cells[i]))
	}
	first := cell(0)
	switch {
	case wcodeOnlyRe.MatchString(first):
		return normalizeStudent(first, cell(1), cell(2), cell(3), cell(4), cell(5), cell(6), cell(7)), true
	case wcodeOnlyRe.MatchString(cell(2)):
		// Legacy-import row with the profile shifted two columns right.
		return normalizeStudent(cell(2), cell(3), cell(4), cell(5), cell(6), "", cell(7), ""), true
	default:
		// Legacy-import row with the wcode glued to a date in cell 1 and
		// first/last names in separate cells.
		embedded := embeddedWcodeRe.FindString(cell(1))
		if embedded == "" {
			return normalize.LegacyStudent{}, false
		}
		name := strings.TrimSpace(cell(2) + " " + cell(3))
		return normalizeStudent(embedded, name, cell(4), cell(5), cell(6), "", cell(7), ""), true
	}
}

// normalizeStudent applies the empty-marker and phone guards to one raw
// row's cells.
func normalizeStudent(wcode, name, nickname, school, level, year, phone, email string) normalize.LegacyStudent {
	clean := func(v string) string {
		v = normalize.NormalizeText(v)
		if legacyMarkerRe.MatchString(v) {
			return ""
		}
		return v
	}
	phone = clean(phone)
	if !hasEnoughDigits(phone, 7) {
		phone = ""
	}
	return normalize.LegacyStudent{
		WCode:    strings.ToUpper(strings.TrimSpace(wcode)),
		Name:     clean(name),
		Nickname: clean(nickname),
		School:   clean(school),
		Level:    clean(level),
		Year:     clean(year),
		Phone:    phone,
		Email:    clean(email),
	}
}

// hasEnoughDigits reports whether s contains at least n digit characters.
func hasEnoughDigits(s string, n int) bool {
	digits := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits++
			if digits >= n {
				return true
			}
		}
	}
	return false
}
