package parser

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"warwick-institute/internal/legacysync/normalize"
)

// PageContract describes the structure a legacy page must have for the
// parser to accept it. Any mismatch is a fail-closed DriftError.
type PageContract struct {
	PageType           string
	ParserVersion      int
	ExpectedTitle      string   // optional: "" skips the check
	RequiredHeaders    []string // first N column headers, exact order after normalization
	RequiredFormFields []string // label texts that must appear in the header region
	MinColumns         int      // inclusive
	MaxColumns         int      // inclusive
	AllowHeaderReorder bool     // when true, required headers may appear in any order
}

// ValidateContract parses pageHTML with the HTML5 parser and verifies it
// matches c. It returns nil on success, or a *DriftError (errors.Is-
// compatible with ErrLoginPage for login/redirect pages) on any mismatch.
// The check order is: login-page detection (only when the expected table
// cannot be found), <title> match, table class, header row match, header
// form fields, header column count.
func ValidateContract(c PageContract, pageHTML string) error {
	_, err := validateAndFindTable(c, pageHTML)
	return err
}

// validateAndFindTable runs the contract checks for c against pageHTML
// and returns the matched <table> node on success.
func validateAndFindTable(c PageContract, pageHTML string) (*html.Node, error) {
	doc, err := parseDocument(pageHTML)
	if err != nil {
		return nil, drift(c, fmt.Sprintf("html parse failed: %v", err))
	}

	table, anyTable, anyHeaderMatch := findTableByHeaders(doc, c)
	if table == nil {
		// Login-page detection FIRST: only when the expected table cannot
		// be found does a login signature turn the failure into
		// ErrLoginPage.
		if hasLoginSignature(doc) {
			return nil, &DriftError{PageType: c.PageType, ParserVersion: c.ParserVersion, Reason: loginPageReason}
		}
		switch {
		case anyHeaderMatch:
			return nil, drift(c, `table class mismatch (expected class containing "table")`)
		case anyTable:
			return nil, drift(c, "headers mismatch (want first headers "+strings.Join(c.RequiredHeaders, ", ")+")")
		default:
			return nil, drift(c, "expected table not found")
		}
	}

	if c.ExpectedTitle != "" {
		title := normalize.NormalizeText(documentTitle(doc))
		if title != normalize.NormalizeText(c.ExpectedTitle) {
			return nil, drift(c, fmt.Sprintf("title mismatch (want %q, got %q)", c.ExpectedTitle, title))
		}
	}

	ths := headerTexts(table)
	for _, f := range c.RequiredFormFields {
		want := normalize.NormalizeText(f)
		found := false
		for _, h := range ths {
			if h == want {
				found = true
				break
			}
		}
		if !found {
			return nil, drift(c, fmt.Sprintf("required form field %q missing from header row", f))
		}
	}

	if n := len(ths); n < c.MinColumns || n > c.MaxColumns {
		return nil, drift(c, fmt.Sprintf("column count %d outside [%d,%d]", n, c.MinColumns, c.MaxColumns))
	}
	return table, nil
}

// parseDocument parses pageHTML with golang.org/x/net/html (the HTML5
// parser). No regex-based HTML parsing is used anywhere.
func parseDocument(pageHTML string) (*html.Node, error) {
	return html.Parse(strings.NewReader(pageHTML))
}

// findTableByHeaders returns the first <table> in document order whose
// class attribute contains the token "table" and whose header row matches
// the required headers after normalization. Header order is enforced unless
// the contract explicitly allows reordering.
func findTableByHeaders(doc *html.Node, contract PageContract) (table *html.Node, anyTable, anyHeaderMatch bool) {
	walk(doc, func(n *html.Node) bool {
		if table != nil {
			return false
		}
		if n.Type != html.ElementNode || n.Data != "table" {
			return true
		}
		anyTable = true
		ths := headerTexts(n)
		if headersMatch(ths, contract.RequiredHeaders, contract.AllowHeaderReorder) {
			anyHeaderMatch = true
			if hasClass(n, "table") {
				table = n
				return false
			}
		}
		return true
	})
	return table, anyTable, anyHeaderMatch
}

// headersMatch reports whether ths contains the required headers after
// normalization. With allowReorder false, required headers must be the first
// columns in the declared order. With allowReorder true, each required header
// must occur exactly once.
func headersMatch(ths, required []string, allowReorder bool) bool {
	if len(ths) < len(required) {
		return false
	}
	if !allowReorder {
		for i, r := range required {
			if ths[i] != r {
				return false
			}
		}
		return true
	}
	seen := make(map[string]struct{}, len(ths))
	for _, header := range ths {
		if _, exists := seen[header]; exists {
			return false
		}
		seen[header] = struct{}{}
	}
	for _, want := range required {
		if _, exists := seen[want]; !exists {
			return false
		}
	}
	return true
}

// headerTexts returns the normalized texts of the <th> cells in the first
// <thead> row of the table (nil if there is no such row). Nested labels
// are included in the text.
func headerTexts(table *html.Node) []string {
	for c := table.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "thead" {
			continue
		}
		for r := c.FirstChild; r != nil; r = r.NextSibling {
			if r.Type != html.ElementNode || r.Data != "tr" {
				continue
			}
			var out []string
			for th := r.FirstChild; th != nil; th = th.NextSibling {
				if th.Type == html.ElementNode && th.Data == "th" {
					out = append(out, normalize.NormalizeText(textOf(th)))
				}
			}
			return out
		}
	}
	return nil
}

// documentTitle returns the normalized text of the first <title> element.
func documentTitle(doc *html.Node) string {
	var title string
	walk(doc, func(n *html.Node) bool {
		if title != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "title" {
			title = normalize.NormalizeText(textOf(n))
			return false
		}
		return true
	})
	return title
}

// hasLoginSignature reports whether the document looks like a login page
// or session redirect: a <form> whose action contains "Login", or an
// input named __RequestVerificationToken whose enclosing form has no
// action attribute (or that is not inside a form at all).
func hasLoginSignature(doc *html.Node) bool {
	sig := false
	walk(doc, func(n *html.Node) bool {
		if sig {
			return false
		}
		if n.Type != html.ElementNode {
			return true
		}
		switch n.Data {
		case "form":
			if strings.Contains(attr(n, "action"), "Login") {
				sig = true
				return false
			}
		case "input":
			if attr(n, "name") == "__RequestVerificationToken" {
				if form := enclosingForm(n); form == nil || !hasAttr(form, "action") {
					sig = true
					return false
				}
			}
		}
		return true
	})
	return sig
}

// enclosingForm returns the nearest ancestor <form> of n, or nil.
func enclosingForm(n *html.Node) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "form" {
			return p
		}
	}
	return nil
}

// firstTbody returns the first <tbody> child of the table, or nil.
func firstTbody(table *html.Node) *html.Node {
	for c := table.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "tbody" {
			return c
		}
	}
	return nil
}

// rowNodes returns the direct <tr> children of a <tbody>.
func rowNodes(tbody *html.Node) []*html.Node {
	var out []*html.Node
	for c := tbody.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "tr" {
			out = append(out, c)
		}
	}
	return out
}

// tdChildren returns the direct <td> children of a <tr>.
func tdChildren(tr *html.Node) []*html.Node {
	var out []*html.Node
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "td" {
			out = append(out, c)
		}
	}
	return out
}

// hasClass reports whether the node's class attribute contains the token.
func hasClass(n *html.Node, token string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == token {
					return true
				}
			}
		}
	}
	return false
}

// hasAttr reports whether the node has the given attribute.
func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

// attr returns the value of the node's attribute, or "".
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// textOf returns the concatenated text content of n and its descendants.
func textOf(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(m *html.Node) {
		if m.Type == html.TextNode {
			b.WriteString(m.Data)
		}
		for c := m.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return b.String()
}

// walk visits n and all descendants depth-first; fn returning false
// prunes the subtree and stops the walk.
func walk(n *html.Node, fn func(*html.Node) bool) {
	if !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}
