package schedulepolicyhttp

import (
	"testing"

	sqldb "warwick-institute/internal/db"
)

func TestNormalizePolicyUpdate(t *testing.T) {
	tests := []struct {
		name                                     string
		previous                                 sqldb.ScheduleConflictPolicyRow
		systemEnforced, legacySyncEnforced       bool
		wantSystem, wantLegacy, wantLegacyForced bool
	}{
		{
			name:               "system on transition protects legacy until separately disabled",
			previous:           sqldb.ScheduleConflictPolicyRow{},
			systemEnforced:     true,
			legacySyncEnforced: false,
			wantSystem:         true,
			wantLegacy:         true,
			wantLegacyForced:   true,
		},
		{
			name:               "system already on permits independent legacy off",
			previous:           sqldb.ScheduleConflictPolicyRow{SystemEnforced: true, LegacySyncEnforced: true},
			systemEnforced:     true,
			legacySyncEnforced: false,
			wantSystem:         true,
			wantLegacy:         false,
			wantLegacyForced:   false,
		},
		{
			name:               "turning system off preserves requested legacy state",
			previous:           sqldb.ScheduleConflictPolicyRow{SystemEnforced: true, LegacySyncEnforced: false},
			systemEnforced:     false,
			legacySyncEnforced: false,
			wantSystem:         false,
			wantLegacy:         false,
			wantLegacyForced:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			system, legacy, forced := normalizePolicyUpdate(tt.previous, tt.systemEnforced, tt.legacySyncEnforced)
			if system != tt.wantSystem || legacy != tt.wantLegacy || forced != tt.wantLegacyForced {
				t.Fatalf("normalizePolicyUpdate() = (%t, %t, %t), want (%t, %t, %t)", system, legacy, forced, tt.wantSystem, tt.wantLegacy, tt.wantLegacyForced)
			}
		})
	}
}
