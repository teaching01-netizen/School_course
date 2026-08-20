package legacysync

import (
	"errors"
	"strings"

	"golang.org/x/net/html"
)

// errNoCourseSearchForm marks a course list page that does not carry the
// course search form (or lacks a usable antiforgery token in it): callers
// fall back to the plain listing untouched.
var errNoCourseSearchForm = errors.New("course search form not found")

// errNoStudentsSearchForm marks a students page that does not carry the
// students search form (or lacks a usable antiforgery token in it): callers
// must not submit a search without a token.
var errNoStudentsSearchForm = errors.New("students search form not found")

// studentsSearchForm is the old site's student search form state on
// /Admin/Students. Submitting it with handler=search renders the student
// table rows, which the plain page leaves empty ("No students yet.").
type studentsSearchForm struct {
	searchText   string
	token        string
	hasSearchBox bool
}

// parseStudentsSearchForm locates the students search form and extracts the
// fields needed to submit it. The form is recognized by its SearchText input
// plus a __RequestVerificationToken input (the live page renders the form
// without an action attribute; the submit button carries
// formaction="/Admin/Students?handler=search"). A missing form or missing
// antiforgery token yields errNoStudentsSearchForm.
func parseStudentsSearchForm(pageHTML string) (*studentsSearchForm, error) {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil, errNoStudentsSearchForm
	}
	var found *studentsSearchForm
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "form" {
			form := &studentsSearchForm{}
			collectStudentsFormFields(n, form)
			if form.hasSearchBox && form.token != "" {
				found = form
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if found == nil {
		return nil, errNoStudentsSearchForm
	}
	return found, nil
}

func collectStudentsFormFields(n *html.Node, out *studentsSearchForm) {
	if n.Type == html.ElementNode && n.Data == "input" {
		switch attr(n, "name") {
		case "SearchText":
			out.searchText = attr(n, "value")
			out.hasSearchBox = true
		case "__RequestVerificationToken":
			out.token = attr(n, "value")
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectStudentsFormFields(child, out)
	}
}

// courseSearchForm is the old site's course search form state on
// /Admin/Courses. Submitting it with IsArchive=true makes the listing
// include archived courses, which the plain page hides.
type courseSearchForm struct {
	searchText   string
	token        string
	isArchive    bool
	hasSearchBox bool
}

// hasSearchFields reports whether the form carries the two inputs that
// identify the course search form on the live page, where the form element
// itself has no action attribute: the SearchText box and the IsArchive
// checkbox (the antiforgery token appears in other forms too).
func (f *courseSearchForm) hasSearchFields() bool {
	return f.hasSearchBox && f.isArchive
}

// parseCourseSearchForm locates the course search form and extracts the
// fields needed to submit it. The form is recognized by its action
// (action="/Admin/Courses?handler=search") or, failing that, by its fields
// (SearchText + IsArchive + __RequestVerificationToken inputs): the live
// page renders the form WITHOUT an action attribute. A missing form or
// missing antiforgery token yields errNoCourseSearchForm.
func parseCourseSearchForm(pageHTML string) (*courseSearchForm, error) {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil, errNoCourseSearchForm
	}
	var found *courseSearchForm
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "form" {
			form := &courseSearchForm{}
			collectFormFields(n, form)
			if isCourseSearchAction(attr(n, "action")) || form.hasSearchFields() {
				found = form
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if found == nil || found.token == "" {
		return nil, errNoCourseSearchForm
	}
	return found, nil
}

// isCourseSearchAction reports whether the form action targets the course
// search handler, e.g. action="/Admin/Courses?handler=search".
func isCourseSearchAction(action string) bool {
	const prefix = "/Admin/Courses"
	if !strings.HasPrefix(action, prefix) {
		return false
	}
	rest := strings.TrimPrefix(action, prefix)
	if rest == "" {
		return false
	}
	if rest[0] != '?' && rest[0] != '#' {
		return false
	}
	return strings.Contains(rest, "handler=search")
}

func collectFormFields(n *html.Node, out *courseSearchForm) {
	if n.Type == html.ElementNode && n.Data == "input" {
		switch attr(n, "name") {
		case "SearchText":
			out.searchText = attr(n, "value")
			out.hasSearchBox = true
		case "IsArchive":
			out.isArchive = true
		case "__RequestVerificationToken":
			out.token = attr(n, "value")
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectFormFields(child, out)
	}
}

func attr(n *html.Node, key string) string {
	for _, item := range n.Attr {
		if item.Key == key {
			return item.Val
		}
	}
	return ""
}
