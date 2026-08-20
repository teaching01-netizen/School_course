package client

import (
	"net/http"
	"net/url"
	"strings"
)

type Request struct {
	Method string
	Path   string
	Query  url.Values
	Form   url.Values
}

func allowed(request Request) bool {
	if request.Path == "/Account/Login" && request.Method == http.MethodGet {
		return true
	}
	switch request.Method {
	case http.MethodGet:
		switch request.Path {
		case "/Admin/Courses", "/Admin/Courses/Detail", "/Admin/Courses/CheckIn",
			"/Admin/Teachers", "/Admin/Subjects", "/Admin/Classrooms", "/Admin/Staffs", "/Admin/Logs",
			"/Admin/Students":
			return true
		}
	case http.MethodPost:
		if request.Path != "/Home/Schedule" && request.Path != "/Home/Summary" && request.Path != "/Admin/Courses" && request.Path != "/Admin/Students" {
			return false
		}
		return safeHandler(request.Form.Get("handler"))
	}
	return false
}

func safeHandler(handler string) bool {
	if handler == "" {
		return true
	}
	for _, blocked := range []string{"confirm", "delete", "import", "edit", "new", "checkin", "classroomset", "studentdelete"} {
		if strings.Contains(strings.ToLower(handler), blocked) {
			return false
		}
	}
	return handler == "search"
}
