package parser

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"

	"warwick-institute/internal/legacysync/normalize"
)

// CourseListContract is the contract for /Admin/Courses (the course list
// page). Headers observed on the live page (legacy.html):
// C-ID, C-Code, Year, Teacher, Subject, Hour, Student, Expired, Type,
// Status, plus an empty action column (11 columns total).
var CourseListContract = PageContract{
	PageType:      "course_list",
	ParserVersion: 2,
	ExpectedTitle: "Course",
	RequiredHeaders: []string{
		"C-ID", "C-Code", "Year", "Teacher", "Subject", "Hour", "Student", "Expired", "Type", "Status",
	},
	RequiredFormFields: []string{
		"C-ID", "C-Code", "Year", "Teacher", "Subject", "Hour", "Student", "Expired", "Type", "Status",
	},
	MinColumns: 11,
	MaxColumns: 11,
}

// CourseListResult is the parsed course list. Teachers and Subjects are the
// distinct master-data references observed across the course rows (name =
// cell text after the bracketed id), deduplicated and sorted by legacy id.
type CourseListResult struct {
	Courses  []normalize.LegacyCourse  `json:"courses"`
	Teachers []normalize.LegacyTeacher `json:"teachers,omitempty"`
	Subjects []normalize.LegacySubject `json:"subjects,omitempty"`
}

// bracketPrefixRe matches the [<id>] prefix of teacher/subject cells,
// e.g. "[207] AJ. TY" -> "207".
var bracketPrefixRe = regexp.MustCompile(`^\[([^\]]+)\]`)

// detailLinkRe matches the course action cell's href exactly as observed
// on the live page: <a href="/Admin/Courses/Detail?id=N">detail</a>.
var detailLinkRe = regexp.MustCompile(`^/Admin/Courses/Detail\?id=(\d+)$`)

// attendeeEntryRe matches one roster entry inside an attendee sub-row cell:
// a "[W<digits>]" code followed by the attendee name, which runs up to the
// next "[" (or the end of the cell) so multiple <br>-separated entries
// concatenated by NormalizeText stay individually parseable. The code is
// matched case-insensitively (the live page has one lowercase "[w...]"
// entry) and normalized to uppercase downstream.
var attendeeEntryRe = regexp.MustCompile(`(?i)\[W\d+\][^[]*`)

// attendeeCodeRe extracts the wcode from a matched roster entry.
var attendeeCodeRe = regexp.MustCompile(`(?i)^\[(W\d+)\]`)

// ParseCourseList parses the /Admin/Courses page. It returns a
// *DriftError on any contract mismatch and never returns partial data.
//
// Row shapes:
//   - Course row: 11 cells (the header count); column mapping is positional
//     (see the contract). Year (2) and Student count (6) are dropped.
//   - Attendee sub-row (observed on the live page): two cells,
//     <td colspan="5"></td><td colspan="6">[W250025] Nutnicha Marungrueng (Nicha)</td>.
//     Its roster entries ("W<digits> <name>", nickname kept) belong to the
//     course row that precedes it. A sub-row with no preceding course row
//     is drift; a sub-row whose cell has no "[W" prefix is not recognized
//     as one and drifts like any other unexpected row shape.
func ParseCourseList(pageHTML string) (*CourseListResult, error) {
	table, err := validateAndFindTable(CourseListContract, pageHTML)
	if err != nil {
		return nil, err
	}
	wantCols := len(headerTexts(table))

	tbody := firstTbody(table)
	if tbody == nil {
		return nil, drift(CourseListContract, "tbody not found")
	}

	courses := make([]normalize.LegacyCourse, 0)
	teachers := map[string]normalize.LegacyTeacher{}
	subjects := map[string]normalize.LegacySubject{}
	lastCourseIdx := -1
	for _, tr := range rowNodes(tbody) {
		tds := tdChildren(tr)

		// Attendee sub-row: first td colspan="5" with empty content, second
		// td colspan="6" whose text starts with "[W". The roster belongs to
		// the course row that precedes it.
		if len(tds) == 2 &&
			colspan(tds[0]) == "5" && normalize.NormalizeText(textOf(tds[0])) == "" &&
			colspan(tds[1]) == "6" && strings.HasPrefix(strings.ToLower(normalize.NormalizeText(textOf(tds[1]))), "[w") {
			if lastCourseIdx < 0 {
				return nil, drift(CourseListContract, "attendee sub-row without a preceding course row")
			}
			entries, err := parseAttendeeEntries(tds[1])
			if err != nil {
				return nil, err
			}
			courses[lastCourseIdx].Attendees = append(courses[lastCourseIdx].Attendees, entries...)
			continue
		}
		if len(tds) != wantCols {
			return nil, drift(CourseListContract, fmt.Sprintf("row has %d cells, want %d", len(tds), wantCols))
		}

		// Column 0: C-ID — a plain legacy id (digits only).
		first := normalize.NormalizeText(textOf(tds[0]))
		if !isAllDigits(first) {
			return nil, drift(CourseListContract, fmt.Sprintf("unexpected first cell %q (not a plain legacy id)", first))
		}
		id, err := normalize.NormalizeID(first)
		if err != nil {
			return nil, drift(CourseListContract, fmt.Sprintf("invalid C-ID %q", first))
		}

		// Column 1: C-Code.
		code := normalize.NormalizeText(textOf(tds[1]))

		// Column 3: Teacher — [<id>] name prefix; empty cell -> "".
		teacherID, teacherName, err := bracketRef(CourseListContract, tds[3], "teacher cell format")
		if err != nil {
			return nil, err
		}
		if teacherID != "" && teacherName != "" {
			teachers[teacherID] = normalize.LegacyTeacher{LegacyID: teacherID, Name: teacherName, IsActive: true}
		}
		// Column 4: Subject — same bracket rule.
		subjectID, subjectName, err := bracketRef(CourseListContract, tds[4], "subject cell format")
		if err != nil {
			return nil, err
		}
		if subjectID != "" && subjectName != "" {
			subjects[subjectID] = normalize.LegacySubject{LegacyID: subjectID, Name: subjectName}
		}

		// Column 5: Hour (empty allowed).
		hours := normalize.NormalizeText(textOf(tds[5]))

		// Column 7: Expired — DD/MM/YY or DD/MM/YYYY, canonical YYYY-MM-DD;
		// empty -> "". Parse failure is drift.
		expire, err := parseExpired(CourseListContract, tds[7])
		if err != nil {
			return nil, err
		}

		// Column 8: Type.
		typ := normalize.NormalizeText(textOf(tds[8]))

		// Column 9: Status — Active|Draft|Archived (case-insensitive after
		// normalization); empty allowed; anything else drifts.
		status, err := mapStatus(CourseListContract, tds[9])
		if err != nil {
			return nil, err
		}

		// Column 10: action cell — the detail href id must equal the C-ID.
		linkID, err := detailLinkID(CourseListContract, tds[10])
		if err != nil {
			return nil, err
		}
		if linkID != id {
			return nil, drift(CourseListContract, "detail link id mismatch")
		}

		courses = append(courses, normalize.LegacyCourse{
			LegacyID:   id,
			Code:       code,
			Status:     status,
			Type:       typ,
			Hours:      hours,
			ExpireDate: expire,
			TeacherID:  teacherID,
			SubjectID:  subjectID,
		})
		lastCourseIdx = len(courses) - 1
	}

	// Rosters are deterministic: ascending by wcode, independent of the
	// source page's row order (the same rule the aggregate applies).
	for i := range courses {
		sort.Strings(courses[i].Attendees)
	}

	// Deterministic output: ascending by legacy id.
	sort.Slice(courses, func(i, j int) bool { return courses[i].LegacyID < courses[j].LegacyID })
	return &CourseListResult{
		Courses:  courses,
		Teachers: sortedTeachers(teachers),
		Subjects: sortedSubjects(subjects),
	}, nil
}

// MergeCourseLists combines the plain /Admin/Courses listing (active and
// draft courses; the old site hides archived ones there) with the
// archive-search listing (archived courses only). Courses are deduplicated
// by legacy id with the plain entry winning; teachers and subjects are
// unioned by legacy id with the plain entry winning. Inputs are never
// mutated and either may be nil. The output is sorted ascending by legacy
// id exactly like ParseCourseList.
func MergeCourseLists(plain, archived *CourseListResult) *CourseListResult {
	merged := &CourseListResult{
		Courses:  make([]normalize.LegacyCourse, 0),
		Teachers: make([]normalize.LegacyTeacher, 0),
		Subjects: make([]normalize.LegacySubject, 0),
	}
	seenCourses := map[string]bool{}
	seenTeachers := map[string]bool{}
	seenSubjects := map[string]bool{}
	for _, src := range []*CourseListResult{plain, archived} {
		if src == nil {
			continue
		}
		for _, course := range src.Courses {
			if seenCourses[course.LegacyID] {
				continue
			}
			seenCourses[course.LegacyID] = true
			merged.Courses = append(merged.Courses, course)
		}
		for _, teacher := range src.Teachers {
			if seenTeachers[teacher.LegacyID] {
				continue
			}
			seenTeachers[teacher.LegacyID] = true
			merged.Teachers = append(merged.Teachers, teacher)
		}
		for _, subject := range src.Subjects {
			if seenSubjects[subject.LegacyID] {
				continue
			}
			seenSubjects[subject.LegacyID] = true
			merged.Subjects = append(merged.Subjects, subject)
		}
	}
	sort.Slice(merged.Courses, func(i, j int) bool { return merged.Courses[i].LegacyID < merged.Courses[j].LegacyID })
	sort.Slice(merged.Teachers, func(i, j int) bool { return merged.Teachers[i].LegacyID < merged.Teachers[j].LegacyID })
	sort.Slice(merged.Subjects, func(i, j int) bool { return merged.Subjects[i].LegacyID < merged.Subjects[j].LegacyID })
	return merged
}

func sortedTeachers(byID map[string]normalize.LegacyTeacher) []normalize.LegacyTeacher {
	out := make([]normalize.LegacyTeacher, 0, len(byID))
	for _, teacher := range byID {
		out = append(out, teacher)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LegacyID < out[j].LegacyID })
	return out
}

func sortedSubjects(byID map[string]normalize.LegacySubject) []normalize.LegacySubject {
	out := make([]normalize.LegacySubject, 0, len(byID))
	for _, subject := range byID {
		out = append(out, subject)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LegacyID < out[j].LegacyID })
	return out
}

// parseAttendeeEntries extracts every roster entry ("[W<digits>] <name>")
// from an attendee sub-row cell. The cell may hold several <br>-separated
// entries; NormalizeText collapses them to one run, and each entry is
// matched independently. The name keeps its "(nickname)" suffix and is
// trimmed. A cell with no entry is drift.
func parseAttendeeEntries(td *html.Node) ([]string, error) {
	text := normalize.NormalizeText(textOf(td))
	matches := attendeeEntryRe.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil, drift(CourseListContract, "attendee sub-row without a roster entry")
	}
	entries := make([]string, 0, len(matches))
	for _, m := range matches {
		code := attendeeCodeRe.FindStringSubmatch(m)
		if code == nil {
			// Unreachable: attendeeEntryRe only matches "[W<digits>]".
			return nil, drift(CourseListContract, fmt.Sprintf("malformed attendee entry %q", m))
		}
		name := strings.TrimSpace(strings.TrimPrefix(m, code[0]))
		entries = append(entries, strings.ToUpper(code[1])+" "+name)
	}
	return entries, nil
}

// bracketRef extracts the [<id>] prefix and the trailing name of a
// teacher/subject cell, e.g. "[207] AJ. TY" -> ("207", "AJ. TY"). An empty
// cell maps to ("", ""); a non-empty cell without the bracket is drift. A
// bracketed id with no trailing name yields an empty name.
func bracketRef(c PageContract, td *html.Node, reason string) (string, string, error) {
	text := normalize.NormalizeText(textOf(td))
	if text == "" {
		return "", "", nil
	}
	m := bracketPrefixRe.FindStringSubmatch(text)
	if m == nil {
		return "", "", drift(c, reason)
	}
	id, err := normalize.NormalizeID(m[1])
	if err != nil {
		return "", "", drift(c, fmt.Sprintf("%s: empty bracketed id in %q", reason, text))
	}
	name := strings.TrimSpace(strings.TrimPrefix(text, m[0]))
	return id, name, nil
}

// parseExpired parses the Expired cell: DD/MM/YY or DD/MM/YYYY to
// canonical YYYY-MM-DD; empty -> "".
func parseExpired(c PageContract, td *html.Node) (string, error) {
	text := normalize.NormalizeText(textOf(td))
	if text == "" {
		return "", nil
	}
	t, err := normalize.ParseLegacyDate(text)
	if err != nil {
		return "", drift(c, fmt.Sprintf("invalid expired date %q", text))
	}
	return t.Format("2006-01-02"), nil
}

// mapStatus maps the Status cell vocabulary: Active->active, Draft->draft,
// Archived->archived, Archive->archived (the live archive-search page writes
// "Archive" without the trailing d; both are case-insensitive after
// normalization); empty -> ""; any other non-empty value is drift.
func mapStatus(c PageContract, td *html.Node) (string, error) {
	text := normalize.NormalizeText(textOf(td))
	switch {
	case text == "":
		return "", nil
	case strings.EqualFold(text, "Active"):
		return "active", nil
	case strings.EqualFold(text, "Draft"):
		return "draft", nil
	case strings.EqualFold(text, "Archived"), strings.EqualFold(text, "Archive"):
		return "archived", nil
	default:
		return "", drift(c, fmt.Sprintf("invalid status %q", text))
	}
}

// detailLinkID extracts the course id from the action cell's detail href.
// A missing or non-matching href is drift.
func detailLinkID(c PageContract, td *html.Node) (string, error) {
	var linkID string
	found := false
	walk(td, func(n *html.Node) bool {
		if found {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			if m := detailLinkRe.FindStringSubmatch(attr(n, "href")); m != nil {
				linkID = m[1]
				found = true
				return false
			}
		}
		return true
	})
	if !found {
		return "", drift(c, "detail link missing")
	}
	return linkID, nil
}

// colspan returns the td's colspan attribute value, or "".
func colspan(td *html.Node) string {
	return attr(td, "colspan")
}

// isAllDigits reports whether s is non-empty and consists only of ASCII
// digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
