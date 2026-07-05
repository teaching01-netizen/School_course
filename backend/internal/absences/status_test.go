package absences

import "testing"

func TestValidStatus(t *testing.T) {
	tests := []struct {
		status string
		valid  bool
	}{
		{"pending", true},
		{"reviewed", true},
		{"actioned", true},
		{"cancelled", true},
		{"special_approved", true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidStatus(tt.status); got != tt.valid {
			t.Errorf("ValidStatus(%q) = %v, want %v", tt.status, got, tt.valid)
		}
	}
}

func TestValidTransition(t *testing.T) {
	tests := []struct {
		from, to Status
		want     bool
	}{
		// Pending transitions
		{StatusPending, StatusReviewed, true},
		{StatusPending, StatusCancelled, true},
		{StatusPending, StatusSpecialApproved, true},
		{StatusPending, StatusActioned, false},
		// Reviewed transitions
		{StatusReviewed, StatusActioned, true},
		{StatusReviewed, StatusPending, true},
		{StatusReviewed, StatusCancelled, true},
		{StatusReviewed, StatusSpecialApproved, true},
		// Actioned transitions
		{StatusActioned, StatusReviewed, true},
		{StatusActioned, StatusCancelled, true},
		{StatusActioned, StatusSpecialApproved, true},
		// Special Approved transitions
		{StatusSpecialApproved, StatusCancelled, true},
		{StatusSpecialApproved, StatusPending, false},
		{StatusSpecialApproved, StatusReviewed, false},
		{StatusSpecialApproved, StatusActioned, false},
		// Cancelled is terminal
		{StatusCancelled, StatusPending, false},
		{StatusCancelled, StatusReviewed, false},
		{StatusCancelled, StatusActioned, false},
		{StatusCancelled, StatusSpecialApproved, false},
	}
	for _, tt := range tests {
		if got := ValidTransition(tt.from, tt.to); got != tt.want {
			t.Errorf("ValidTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestStatusAuditAction(t *testing.T) {
	tests := []struct {
		current, to Status
		want        string
	}{
		{StatusReviewed, StatusPending, "reopened"},
		{StatusPending, StatusReviewed, "reviewed"},
		{StatusPending, StatusSpecialApproved, "special_approved"},
		{StatusSpecialApproved, StatusCancelled, "cancelled"},
	}
	for _, tt := range tests {
		if got := StatusAuditAction(tt.current, tt.to); got != tt.want {
			t.Errorf("StatusAuditAction(%q, %q) = %q, want %q", tt.current, tt.to, got, tt.want)
		}
	}
}

func TestAllStatusesContainsSpecialApproved(t *testing.T) {
	statuses := AllStatuses()
	found := false
	for _, s := range statuses {
		if s == StatusSpecialApproved {
			found = true
			break
		}
	}
	if !found {
		t.Error("AllStatuses() should contain StatusSpecialApproved")
	}
}
