package client

import (
	"net/http"
	"net/url"
	"testing"
)

func TestAllowed_ReadOnlyRoutes(t *testing.T) {
	tests := []Request{
		{Method: http.MethodGet, Path: "/Admin/Courses"},
		{Method: http.MethodGet, Path: "/Admin/Courses/Detail"},
		{Method: http.MethodGet, Path: "/Admin/Teachers"},
		{Method: http.MethodGet, Path: "/Admin/Students"},
		{Method: http.MethodPost, Path: "/Admin/Students", Form: url.Values{"handler": {"search"}}},
		{Method: http.MethodPost, Path: "/Admin/Courses", Form: url.Values{"handler": {"search"}}},
		{Method: http.MethodPost, Path: "/Home/Schedule", Form: url.Values{"handler": {"search"}}},
	}
	for _, request := range tests {
		if !allowed(request) {
			t.Errorf("allowed(%+v) = false, want true", request)
		}
	}
}

func TestAllowed_BlocksMutatingAndEncodedRoutes(t *testing.T) {
	tests := []Request{
		{Method: http.MethodPost, Path: "/Admin/Courses/CheckIn", Form: url.Values{"handler": {"checkin"}}},
		{Method: http.MethodPost, Path: "/Admin/Courses", Form: url.Values{"handler": {"delete"}}},
		{Method: http.MethodPost, Path: "/Admin/Courses", Form: url.Values{"handler": {"import"}}},
		{Method: http.MethodPost, Path: "/Admin/Students", Form: url.Values{"handler": {"import"}}},
		{Method: http.MethodPost, Path: "/Admin/Students", Form: url.Values{"handler": {"confirm"}}},
		{Method: http.MethodGet, Path: "/Admin/Courses%2FCheckIn"},
		{Method: http.MethodGet, Path: "/Admin/Unknown"},
		{Method: http.MethodPost, Path: "/Admin/Courses", Form: url.Values{"handler": {"confirm%2Bdelete"}}},
	}
	for _, request := range tests {
		if allowed(request) {
			t.Errorf("allowed(%+v) = true, want false", request)
		}
	}
}
