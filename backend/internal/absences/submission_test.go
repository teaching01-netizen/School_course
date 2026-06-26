package absences

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestNormalizeWCode(t *testing.T) {
	if got := NormalizeWCode("  W123AbC  "); got != "w123abc" {
		t.Fatalf("NormalizeWCode = %q, want w123abc", got)
	}
}

func TestResolveClientStudentEmailRejectsStoredEmailOverride(t *testing.T) {
	stored := pgtype.Text{String: "stored@example.com", Valid: true}
	if _, _, err := ResolveClientStudentEmail(ptr("new@example.com"), pgtype.Text{}, stored); err == nil {
		t.Fatalf("client email should not override stored email")
	}
}

func TestValidateSessionTimingRejectsExpiredGracePeriod(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	err := ValidateSessionTiming(
		TimingSettings{MaxHoursAfterSession: 1},
		now,
		[]SessionTimingInfo{{
			StartAt: pgtype.Timestamptz{Time: now.Add(-3 * time.Hour), Valid: true},
			EndAt:   pgtype.Timestamptz{Time: now.Add(-61 * time.Minute), Valid: true},
		}},
	)
	if err == nil || err.Code != "grace_period_expired" {
		t.Fatalf("ValidateSessionTiming error = %#v, want grace_period_expired", err)
	}
}

func ptr(s string) *string { return &s }
