package absences

import (
	"testing"
)

func TestProjectedAbsenceSessionLimitExceeded(t *testing.T) {
	tests := []struct {
		name                   string
		totalSessions          int32
		existingMissedSessions int32
		submittingSessionCount int32
		wantExceeded           bool
	}{
		// Core path: 20% limit enforcement
		{
			name:                   "10 sessions, 0 existing, submitting 2 = exactly 20% allowed",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 2,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 0 existing, submitting 3 = 30% blocked",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 3,
			wantExceeded:           true,
		},
		{
			name:                   "10 sessions, 1 existing, submitting 1 = 20% total allowed",
			totalSessions:          10,
			existingMissedSessions: 1,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 1 existing, submitting 2 = 30% total blocked",
			totalSessions:          10,
			existingMissedSessions: 1,
			submittingSessionCount: 2,
			wantExceeded:           true,
		},
		{
			name:                   "10 sessions, 2 existing, submitting 1 = 30% total blocked",
			totalSessions:          10,
			existingMissedSessions: 2,
			submittingSessionCount: 1,
			wantExceeded:           true,
		},

		// Cumulative limit: after using 20%, further submissions blocked
		{
			name:                   "10 sessions, 2 existing, submitting 1 blocked (already at limit)",
			totalSessions:          10,
			existingMissedSessions: 2,
			submittingSessionCount: 1,
			wantExceeded:           true,
		},
		{
			name:                   "5 sessions, 0 existing, submitting 1 = 20% allowed",
			totalSessions:          5,
			existingMissedSessions: 0,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "5 sessions, 0 existing, submitting 2 = 40% blocked",
			totalSessions:          5,
			existingMissedSessions: 0,
			submittingSessionCount: 2,
			wantExceeded:           true,
		},
		{
			name:                   "5 sessions, 1 existing, submitting 1 = 40% total blocked",
			totalSessions:          5,
			existingMissedSessions: 1,
			submittingSessionCount: 1,
			wantExceeded:           true,
		},

		// Per-request: single submission exceeding 20% blocked
		{
			name:                   "20 sessions, 0 existing, submitting 5 = 25% blocked",
			totalSessions:          20,
			existingMissedSessions: 0,
			submittingSessionCount: 5,
			wantExceeded:           true,
		},
		{
			name:                   "20 sessions, 0 existing, submitting 4 = exactly 20% allowed",
			totalSessions:          20,
			existingMissedSessions: 0,
			submittingSessionCount: 4,
			wantExceeded:           false,
		},
		{
			name:                   "20 sessions, 3 existing, submitting 1 = 20% total allowed",
			totalSessions:          20,
			existingMissedSessions: 3,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "20 sessions, 3 existing, submitting 2 = 25% total blocked",
			totalSessions:          20,
			existingMissedSessions: 3,
			submittingSessionCount: 2,
			wantExceeded:           true,
		},

		// Edge cases: boundary conditions
		{
			name:                   "1 session, 0 existing, submitting 1 = 100% blocked",
			totalSessions:          1,
			existingMissedSessions: 0,
			submittingSessionCount: 1,
			wantExceeded:           true,
		},
		{
			name:                   "6 sessions, 0 existing, submitting 1 = 16.7% allowed",
			totalSessions:          6,
			existingMissedSessions: 0,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "6 sessions, 0 existing, submitting 2 = 33% blocked",
			totalSessions:          6,
			existingMissedSessions: 0,
			submittingSessionCount: 2,
			wantExceeded:           true,
		},
		{
			name:                   "11 sessions, 0 existing, submitting 2 = 18% allowed",
			totalSessions:          11,
			existingMissedSessions: 0,
			submittingSessionCount: 2,
			wantExceeded:           false,
		},
		{
			name:                   "11 sessions, 0 existing, submitting 3 = 27% blocked",
			totalSessions:          11,
			existingMissedSessions: 0,
			submittingSessionCount: 3,
			wantExceeded:           true,
		},
		{
			name:                   "15 sessions, 0 existing, submitting 3 = 20% allowed",
			totalSessions:          15,
			existingMissedSessions: 0,
			submittingSessionCount: 3,
			wantExceeded:           false,
		},
		{
			name:                   "15 sessions, 2 existing, submitting 1 = 20% total allowed",
			totalSessions:          15,
			existingMissedSessions: 2,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "15 sessions, 2 existing, submitting 2 = 26.7% total blocked",
			totalSessions:          15,
			existingMissedSessions: 2,
			submittingSessionCount: 2,
			wantExceeded:           true,
		},

		// Edge cases: zero/negative inputs
		{
			name:                   "0 total sessions",
			totalSessions:          0,
			existingMissedSessions: 0,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "negative total sessions",
			totalSessions:          -1,
			existingMissedSessions: 0,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "0 submitting sessions",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 0,
			wantExceeded:           false,
		},
		{
			name:                   "negative submitting sessions",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: -1,
			wantExceeded:           false,
		},

		// Large values
		{
			name:                   "100 sessions, 19 existing, submitting 1 = 20% total allowed",
			totalSessions:          100,
			existingMissedSessions: 19,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "100 sessions, 19 existing, submitting 2 = 21% total blocked",
			totalSessions:          100,
			existingMissedSessions: 19,
			submittingSessionCount: 2,
			wantExceeded:           true,
		},
		{
			name:                   "100 sessions, 0 existing, submitting 20 = exactly 20% allowed",
			totalSessions:          100,
			existingMissedSessions: 0,
			submittingSessionCount: 20,
			wantExceeded:           false,
		},
		{
			name:                   "100 sessions, 0 existing, submitting 21 = 21% blocked",
			totalSessions:          100,
			existingMissedSessions: 0,
			submittingSessionCount: 21,
			wantExceeded:           true,
		},

		// Bug regression: previously counted records not sessions
		// A single request with 8 sessions on a 10-session course should be blocked
		{
			name:                   "BUG REGRESSION: 10 sessions, 0 existing, 8 sessions in one request = blocked",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 8,
			wantExceeded:           true,
		},
		// After submitting 8 sessions, coming back should be blocked
		{
			name:                   "BUG REGRESSION: 10 sessions, 8 existing, submitting 1 = already over limit",
			totalSessions:          10,
			existingMissedSessions: 8,
			submittingSessionCount: 1,
			wantExceeded:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectedAbsenceSessionLimitExceeded(tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount)
			if got != tt.wantExceeded {
				t.Errorf("ProjectedAbsenceSessionLimitExceeded(%d, %d, %d) = %v, want %v",
					tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount, got, tt.wantExceeded)
			}
		})
	}
}

func TestProjectedAbsenceSessionLimitExceededVsRecordLimitExceeded(t *testing.T) {
	// The session-based check should be stricter than the record-based check
	// when multiple sessions are submitted at once
	totalSessions := int32(10)

	// Record-based: 1 record out of 10 sessions → allowed (1*5=5, not >10)
	// Session-based: 3 sessions out of 10 → blocked (3*5=15, >10)
	if !ProjectedAbsenceSessionLimitExceeded(totalSessions, 0, 3) {
		t.Error("session limit should block 3 sessions in one request on a 10-session course")
	}
	if ProjectedAbsenceRecordLimitExceeded(totalSessions, 0, 1) {
		t.Error("record limit should allow 1 record on a 10-session course")
	}

	// This verifies the core bug fix: the old system would allow submitting
	// 8 sessions at once (counted as 1 record), but the new system blocks it
	if !ProjectedAbsenceSessionLimitExceeded(totalSessions, 0, 8) {
		t.Error("session limit should block 8 sessions in one request (bug regression)")
	}
	if ProjectedAbsenceRecordLimitExceeded(totalSessions, 0, 1) {
		t.Error("record limit would have allowed 8 sessions counted as 1 record (the old bug)")
	}
}
