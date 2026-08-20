package parser

import (
	"fmt"
	"regexp"

	"golang.org/x/net/html"

	"warwick-institute/internal/legacysync/normalize"
)

// CourseDetailContract is the contract for the course schedule table on
// /Admin/Courses/Detail?id=N. Headers observed on the page (see
// internal/legacysync/parser_test.go): Date, Begin, End, Duration,
// Classroom, Confirm, By. The current page adds a trailing headerless
// action column (a per-row check-in link), so the table carries either 7
// or 8 columns. There is NO schedule-ID column on this page.
var CourseDetailContract = PageContract{
	PageType:      "course_detail",
	ParserVersion: 1,
	ExpectedTitle: "", // unknown — the detail page title is not captured
	RequiredHeaders: []string{
		"Date", "Begin", "End", "Duration", "Classroom", "Confirm", "By",
	},
	RequiredFormFields: []string{
		"Date", "Begin", "End", "Duration", "Classroom", "Confirm", "By",
	},
	MinColumns:         7,
	MaxColumns:         8,
	AllowHeaderReorder: true,
}

const maxCourseDetailRows = 10_000

// ParseCourseDetail parses the schedule table of a course detail page.
// Course-level fields (ID/code/name) are NOT on this page; the caller
// supplies them later. It returns a *DriftError on any contract mismatch
// and never returns partial data.
//
// The page may contain other tables besides the schedule table; parsing
// is restricted to the table whose header row matches the contract.
// An empty <tbody> (headers present, zero rows) is VALID and yields an
// aggregate with no schedules.
func ParseCourseDetail(pageHTML string) (*normalize.LegacyCourseAggregate, error) {
	table, err := validateAndFindTable(CourseDetailContract, pageHTML)
	if err != nil {
		return nil, err
	}
	headers := headerTexts(table)
	columns := make(map[string]int, len(headers))
	for i, header := range headers {
		columns[header] = i
	}
	tbody := firstTbody(table)
	if tbody == nil {
		return nil, drift(CourseDetailContract, "tbody not found")
	}
	rows := rowNodes(tbody)
	if len(rows) > maxCourseDetailRows {
		return nil, drift(CourseDetailContract, fmt.Sprintf("schedule row count %d exceeds limit %d", len(rows), maxCourseDetailRows))
	}

	schedules := make([]normalize.LegacySchedule, 0, len(rows))
	seenIDs := make(map[string]struct{}, len(rows))
	for _, tr := range rows {
		tds := tdChildren(tr)
		// The page renders a single colspan cell ("No schedules yet.")
		// when the course has no schedule rows.
		if len(tds) == 1 && hasAttr(tds[0], "colspan") {
			continue
		}
		if len(tds) != len(headers) {
			return nil, drift(CourseDetailContract, fmt.Sprintf("row has %d cells, want %d", len(tds), len(headers)))
		}
		// Every table ends with summary footer rows (Confirmed hours / Booked
		// hours / Time remaining) that share the schedule columns but carry
		// no date; real schedule rows always have a date.
		if normalize.NormalizeText(textOf(tds[columns["Date"]])) == "" {
			continue
		}

		date, err := parseDateCell(CourseDetailContract, tds[columns["Date"]])
		if err != nil {
			return nil, err
		}
		begin, err := parseClockCell(CourseDetailContract, tds[columns["Begin"]], "begin")
		if err != nil {
			return nil, err
		}
		end, err := parseClockCell(CourseDetailContract, tds[columns["End"]], "end")
		if err != nil {
			return nil, err
		}
		// The current page renders the classroom as a screen-only link plus
		// a print-only label with the same text; prefer the link so the two
		// copies are not concatenated.
		classroomCell := tds[columns["Classroom"]]
		classroomRaw := textOf(classroomCell)
		if linkText := firstAnchorText(classroomCell); linkText != "" {
			classroomRaw = linkText
		}
		classroom, _ := normalize.NormalizeOptional(classroomRaw)
		classroomID := ""
		if classroom != "" {
			if match := bracketPrefixRe.FindStringSubmatch(classroom); match != nil {
				classroomID, err = normalize.NormalizeID(match[1])
				if err != nil {
					return nil, drift(CourseDetailContract, fmt.Sprintf("invalid classroom id %q", match[1]))
				}
			}
		}
		confirmed, err := parseConfirm(CourseDetailContract, tds[columns["Confirm"]])
		if err != nil {
			return nil, err
		}
		by := normalize.NormalizeText(textOf(tds[columns["By"]]))

		scheduleID := ""
		switch rawID := attr(tr, "data-schedule-id"); {
		case rawID != "":
			scheduleID, err = normalize.NormalizeID(rawID)
			if err != nil {
				return nil, drift(CourseDetailContract, "invalid schedule id")
			}
		default:
			// The current page exposes the stable schedule identity only
			// through per-row links (courseScheduleId=NNN in the ClassroomSet
			// and CheckIn hrefs); row position must never become identity.
			if linkID := rowScheduleIDFromLink(tr); linkID != "" {
				scheduleID, err = normalize.NormalizeID(linkID)
				if err != nil {
					return nil, drift(CourseDetailContract, "invalid schedule id")
				}
			}
		}
		if scheduleID != "" {
			if _, exists := seenIDs[scheduleID]; exists {
				return nil, drift(CourseDetailContract, fmt.Sprintf("duplicate schedule id %q", scheduleID))
			}
			seenIDs[scheduleID] = struct{}{}
		}
		schedules = append(schedules, normalize.LegacySchedule{
			LegacyScheduleID:  scheduleID,
			Date:              date,
			Begin:             begin,
			End:               end,
			Classroom:         classroom,
			ClassroomLegacyID: classroomID,
			Confirmed:         confirmed,
			ConfirmedBy:       by,
		})
	}

	agg := normalize.NewLegacyCourseAggregate(normalize.LegacyCourse{}, schedules, nil)
	return &agg, nil
}

// courseScheduleLinkRe matches the stable schedule identity embedded in the
// row's per-schedule action URLs (?courseScheduleId=NNN).
var courseScheduleLinkRe = regexp.MustCompile(`[?&]courseScheduleId=([0-9]+)`)

// rowScheduleIDFromLink returns the schedule ID from the first anchor in the
// row whose href carries a courseScheduleId parameter, or "" when the row has
// none. Detail pages without data-schedule-id attributes expose identity only
// here; the ordinal row position must never be used as identity.
func rowScheduleIDFromLink(tr *html.Node) string {
	var id string
	walk(tr, func(m *html.Node) bool {
		if id != "" {
			return false
		}
		if m.Type == html.ElementNode && m.Data == "a" {
			if match := courseScheduleLinkRe.FindStringSubmatch(attr(m, "href")); match != nil {
				id = match[1]
				return false
			}
		}
		return true
	})
	return id
}

// parseDateCell parses a schedule date cell via normalize.ParseLegacyDate
// ("Sat 23 May 26", "23/05/26", ...) into canonical YYYY-MM-DD.
func parseDateCell(c PageContract, td *html.Node) (string, error) {
	text := normalize.NormalizeText(textOf(td))
	t, err := normalize.ParseLegacyDate(text)
	if err != nil {
		return "", drift(c, fmt.Sprintf("invalid date %q", text))
	}
	return t.Format("2006-01-02"), nil
}

// parseClockCell validates a clock cell via normalize.ParseClock and
// returns it zero-padded to HH:MM ("9:05" -> "09:05"; "13:00" round-trips
// exactly).
func parseClockCell(c PageContract, td *html.Node, field string) (string, error) {
	text := normalize.NormalizeText(textOf(td))
	mins, err := normalize.ParseClock(text)
	if err != nil {
		return "", drift(c, fmt.Sprintf("invalid %s clock %q", field, text))
	}
	return fmt.Sprintf("%02d:%02d", mins/60, mins%60), nil
}

// parseConfirm maps the Confirm cell: "Yes"->true, "No"->false, ""->false,
// any other value is drift.
// firstAnchorText returns the normalized text of the first <a> descendant of
// n, or "" when the cell has none. The legacy detail page renders the
// classroom cell as a screen link plus a print-only label with identical
// text; without this the two copies would be concatenated.
func firstAnchorText(n *html.Node) string {
	var text string
	walk(n, func(m *html.Node) bool {
		if text != "" {
			return false
		}
		if m.Type == html.ElementNode && m.Data == "a" {
			text = normalize.NormalizeText(textOf(m))
			return false
		}
		return true
	})
	return text
}

func parseConfirm(c PageContract, td *html.Node) (bool, error) {
	switch normalize.NormalizeText(textOf(td)) {
	case "":
		return false, nil
	case "Yes":
		return true, nil
	case "No":
		return false, nil
	default:
		return false, drift(c, fmt.Sprintf("invalid confirm value %q", normalize.NormalizeText(textOf(td))))
	}
}
