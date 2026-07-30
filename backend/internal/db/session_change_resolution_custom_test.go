package db

import "testing"

func TestScheduleIssueIsResolvable_allowsOpenAndNeedsReview(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "open", status: "open", want: true},
		{name: "needs review", status: "needs_review", want: true},
		{name: "resolved", status: "resolved", want: false},
		{name: "superseded", status: "superseded", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			got := scheduleIssueIsResolvable(test.status)

			// Then
			if got != test.want {
				t.Fatalf("scheduleIssueIsResolvable(%q) = %t, want %t", test.status, got, test.want)
			}
		})
	}
}
