package legacysync

import (
	"strings"
	"time"

	"golang.org/x/net/html"
)

func ParseScheduleTable(pageHTML string) ([]ParsedRow, error) {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil, err
	}

	table := findScheduleTable(doc)
	if table == nil {
		return nil, nil
	}

	tbody := findFirstChildTag(table, "tbody")
	var trs []*html.Node
	if tbody != nil {
		trs = findDescendantTags(tbody, "tr")
	} else {
		trs = findDirectChildTags(table, "tr")
	}

	var rows []ParsedRow
	for _, tr := range trs {
		cells := collectCells(tr)
		if len(cells) < 5 {
			continue
		}
		dateStr := strings.TrimSpace(cells[0])
		beginStr := strings.TrimSpace(cells[1])
		endStr := strings.TrimSpace(cells[2])
		durationStr := strings.TrimSpace(cells[3])
		classroomStr := strings.TrimSpace(cells[4])

		parsedDate, err := parseDate(dateStr)
		if err != nil {
			continue
		}

		rows = append(rows, ParsedRow{
			Date:      parsedDate,
			Begin:     beginStr,
			End:       endStr,
			Duration:  durationStr,
			Classroom: classroomStr,
		})
	}

	return rows, nil
}

func findScheduleTable(n *html.Node) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "table" {
			for _, a := range n.Attr {
				if a.Key == "class" && containsClass(a.Val, "table") {
					if hasHeaderWithDate(n) {
						found = n
						return
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return found
}

func hasHeaderWithDate(table *html.Node) bool {
	for c := table.FirstChild; c != nil; c = c.NextSibling {
		rows := findDescendantTags(c, "tr")
		for _, tr := range rows {
			headers := findDescendantTags(tr, "th")
			for _, th := range headers {
				if text := strings.TrimSpace(nodeText(th)); text == "Date" {
					return true
				}
			}
		}
	}
	return false
}

func findDescendantTags(n *html.Node, tag string) []*html.Node {
	var result []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			result = append(result, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return result
}

func findDirectChildTags(n *html.Node, tag string) []*html.Node {
	var result []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			result = append(result, c)
		}
	}
	return result
}

func findFirstChildTag(n *html.Node, tag string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

func findNextSiblingTag(n *html.Node, tag string) *html.Node {
	for s := n.NextSibling; s != nil; s = s.NextSibling {
		if s.Type == html.ElementNode && s.Data == tag {
			return s
		}
	}
	return nil
}

func collectCells(tr *html.Node) []string {
	var cells []string
	for td := findFirstChildTag(tr, "td"); td != nil; td = findNextSiblingTag(td, "td") {
		cells = append(cells, cellText(td))
	}
	return cells
}

func cellText(td *html.Node) string {
	text := nodeText(td)
	// Old ASP.NET sometimes duplicates content for screen + print elements.
	// Take only the first non-empty trimmed line.
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(text)
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func parseDate(s string) (time.Time, error) {
	// Format: "Sat 23 May 26"
	parts := strings.Fields(s)
	if len(parts) < 4 {
		return time.Time{}, errInvalidDate
	}
	day := parts[1]
	month := parts[2]
	yearStr := parts[3]

	// 2-digit year → 2000s
	if len(yearStr) == 2 {
		yearStr = "20" + yearStr
	}

	return time.Parse("2 Jan 2006", day+" "+month+" "+yearStr)
}

var errInvalidDate = &parseError{"invalid date format"}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }

func containsClass(classAttr, target string) bool {
	for _, c := range strings.Fields(classAttr) {
		if c == target {
			return true
		}
	}
	return false
}
