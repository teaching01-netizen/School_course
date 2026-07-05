package absenceshttp

import (
	"testing"
)

func TestProjectedAbsenceSessionLimitExceeded_BatchHappyPath(t *testing.T) {
	tests := []struct {
		name                   string
		totalSessions          int32
		existingMissedSessions int32
		submittingSessionCount int32
		wantExceeded           bool
	}{
		{
			name:                   "10 sessions, 0 existing, submitting 2 = allowed",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 2,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 1 existing, submitting 1 = allowed",
			totalSessions:          10,
			existingMissedSessions: 1,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "20 sessions, 0 existing, submitting 4 = allowed",
			totalSessions:          20,
			existingMissedSessions: 0,
			submittingSessionCount: 4,
			wantExceeded:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectedAbsenceSessionLimitExceeded(tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount)
			if got != tt.wantExceeded {
				t.Errorf("projectedAbsenceSessionLimitExceeded(%d, %d, %d) = %v, want %v",
					tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount, got, tt.wantExceeded)
			}
		})
	}
}

func TestProjectedAbsenceSessionLimitExceeded_BatchLimitExceeded(t *testing.T) {
	tests := []struct {
		name                   string
		totalSessions          int32
		existingMissedSessions int32
		submittingSessionCount int32
		wantExceeded           bool
	}{
		{
			name:                   "10 sessions, 0 existing, submitting 3 = blocked",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 3,
			wantExceeded:           true,
		},
		{
			name:                   "10 sessions, 1 existing, submitting 2 = blocked",
			totalSessions:          10,
			existingMissedSessions: 1,
			submittingSessionCount: 2,
			wantExceeded:           true,
		},
		{
			name:                   "10 sessions, 2 existing, submitting 1 = blocked",
			totalSessions:          10,
			existingMissedSessions: 2,
			submittingSessionCount: 1,
			wantExceeded:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectedAbsenceSessionLimitExceeded(tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount)
			if got != tt.wantExceeded {
				t.Errorf("projectedAbsenceSessionLimitExceeded(%d, %d, %d) = %v, want %v",
					tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount, got, tt.wantExceeded)
			}
		})
	}
}

func TestProjectedAbsenceSessionLimitExceeded_BatchSameCourseOverflow(t *testing.T) {
	tests := []struct {
		name                   string
		totalSessions          int32
		existingMissedSessions int32
		submittingSessionCount int32
		wantExceeded           bool
	}{
		{
			name:                   "10 sessions, 0 existing, submitting 2 = allowed (first batch item)",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 2,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 2 existing, submitting 1 = blocked (second batch item)",
			totalSessions:          10,
			existingMissedSessions: 2,
			submittingSessionCount: 1,
			wantExceeded:           true,
		},
		{
			name:                   "10 sessions, 1 existing, submitting 1 = allowed (second batch item)",
			totalSessions:          10,
			existingMissedSessions: 1,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 2 existing, submitting 1 = blocked (third batch item)",
			totalSessions:          10,
			existingMissedSessions: 2,
			submittingSessionCount: 1,
			wantExceeded:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectedAbsenceSessionLimitExceeded(tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount)
			if got != tt.wantExceeded {
				t.Errorf("projectedAbsenceSessionLimitExceeded(%d, %d, %d) = %v, want %v",
					tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount, got, tt.wantExceeded)
			}
		})
	}
}

func TestProjectedAbsenceSessionLimitExceeded_SubmittingSessionCountFallback(t *testing.T) {
	tests := []struct {
		name                   string
		totalSessions          int32
		existingMissedSessions int32
		submittingSessionCount int32
		wantExceeded           bool
	}{
		{
			name:                   "10 sessions, 0 existing, submitting 0 = not exceeded",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 0,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 0 existing, submitting 1 = allowed",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectedAbsenceSessionLimitExceeded(tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount)
			if got != tt.wantExceeded {
				t.Errorf("projectedAbsenceSessionLimitExceeded(%d, %d, %d) = %v, want %v",
					tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount, got, tt.wantExceeded)
			}
		})
	}
}

func TestProjectedAbsenceSessionLimitExceeded_SubmittingSessionCountFallbackZeroTo1(t *testing.T) {
	tests := []struct {
		name                   string
		totalSessions          int32
		existingMissedSessions int32
		submittingSessionCount int32
		wantExceeded           bool
	}{
		{
			name:                   "10 sessions, 0 existing, submitting 0 → treated as 1 → allowed",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 0,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 0 existing, submitting 0 → treated as 1 → allowed",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 0,
			wantExceeded:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectedAbsenceSessionLimitExceeded(tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount)
			if got != tt.wantExceeded {
				t.Errorf("projectedAbsenceSessionLimitExceeded(%d, %d, %d) = %v, want %v",
					tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount, got, tt.wantExceeded)
			}
		})
	}
}
