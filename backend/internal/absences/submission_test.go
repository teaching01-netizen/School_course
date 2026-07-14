package absences

import "testing"

func TestAbsenceDayLimitStats(t *testing.T) {
	tests := []struct {
		name          string
		total         int32
		used          int32
		projected     int32
		wantMax       int32
		wantRemaining int32
		wantReached   bool
		wantExceeded  bool
	}{
		{name: "below half rounds down", total: 12, used: 1, projected: 2, wantMax: 2, wantRemaining: 1},
		{name: "above half rounds up", total: 13, used: 2, projected: 3, wantMax: 3, wantRemaining: 1},
		{name: "exact boundary reached", total: 10, used: 2, projected: 2, wantMax: 2, wantRemaining: 0, wantReached: true},
		{name: "projected over boundary", total: 10, used: 1, projected: 3, wantMax: 2, wantRemaining: 1, wantExceeded: true},
		{name: "zero total guarded", total: 0, used: 0, projected: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewAbsenceDayLimitStats(tt.total, tt.used, tt.projected)
			if got.MaximumAbsenceDays != tt.wantMax || got.RemainingAbsenceDays != tt.wantRemaining ||
				got.LimitReached != tt.wantReached || got.ProjectedLimitExceeded != tt.wantExceeded {
				t.Fatalf("NewAbsenceDayLimitStats(%d, %d, %d) = %+v", tt.total, tt.used, tt.projected, got)
			}
		})
	}
}
