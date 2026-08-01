package sessionchangehttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPageParams_Defaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/operations/schedule-issues", nil)
	limit, offset := pageParams(r)
	if limit != 50 {
		t.Errorf("expected default limit 50, got %d", limit)
	}
	if offset != 0 {
		t.Errorf("expected default offset 0, got %d", offset)
	}
}

func TestPageParams_ValidValues(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedLimit  int
		expectedOffset int
	}{
		{"limit_25_offset_10", "limit=25&offset=10", 25, 10},
		{"limit_100_offset_1", "limit=100&offset=1", 100, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/operations/schedule-issues?"+tt.query, nil)
			limit, offset := pageParams(r)
			if limit != tt.expectedLimit {
				t.Errorf("expected limit %d, got %d", tt.expectedLimit, limit)
			}
			if offset != tt.expectedOffset {
				t.Errorf("expected offset %d, got %d", tt.expectedOffset, offset)
			}
		})
	}
}

func TestPageParams_Clamping(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedLimit  int
		expectedOffset int
	}{
		{"limit_lowest_valid", "limit=1", 1, 0},
		{"limit_zero_to_default", "limit=0", 50, 0},
		{"limit_over_cap", "limit=101", 100, 0},
		{"limit_highest_valid", "limit=99", 99, 0},
		{"offset_negative_to_zero", "offset=-1", 50, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/operations/schedule-issues?"+tt.query, nil)
			limit, offset := pageParams(r)
			if limit != tt.expectedLimit {
				t.Errorf("expected limit %d, got %d", tt.expectedLimit, limit)
			}
			if offset != tt.expectedOffset {
				t.Errorf("expected offset %d, got %d", tt.expectedOffset, offset)
			}
		})
	}
}

func TestPageParams_NonNumeric(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedLimit  int
		expectedOffset int
	}{
		{"limit_non_numeric_to_default", "limit=abc", 50, 0},
		{"offset_non_numeric_to_zero", "offset=xyz", 50, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/operations/schedule-issues?"+tt.query, nil)
			limit, offset := pageParams(r)
			if limit != tt.expectedLimit {
				t.Errorf("expected limit %d, got %d", tt.expectedLimit, limit)
			}
			if offset != tt.expectedOffset {
				t.Errorf("expected offset %d, got %d", tt.expectedOffset, offset)
			}
		})
	}
}

func TestIssueStatusForAction(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		expected string
	}{
		{"dismiss", "dismiss", "dismissed"},
		{"mark_for_review", "mark_for_review", "needs_review"},
		{"keep", "keep", "resolved"},
		{"cancel", "cancel", "resolved"},
		{"reassign", "reassign", "resolved"},
		{"empty", "", "resolved"},
		{"unknown", "anything", "resolved"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := issueStatusForAction(tt.action); got != tt.expected {
				t.Errorf("expected status %q for action %q, got %q", tt.expected, tt.action, got)
			}
		})
	}
}
