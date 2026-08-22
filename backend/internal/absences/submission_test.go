package absences

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

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

func TestResolveClientNickname(t *testing.T) {
	ptr := func(value string) *string { return &value }
	tests := []struct {
		name          string
		raw           *string
		existing      pgtype.Text
		wantValid     bool
		wantPersisted bool
		wantErr       bool
	}{
		{name: "absent is a no-op", raw: nil, existing: pgtype.Text{}},
		{name: "blank is a no-op", raw: ptr("   "), existing: pgtype.Text{}},
		{name: "fills an empty record", raw: ptr("Bird"), existing: pgtype.Text{}, wantValid: true, wantPersisted: true},
		{name: "existing nickname wins", raw: ptr("Bird"), existing: pgtype.Text{String: "Fah", Valid: true}, wantErr: true},
		{name: "whitespace-only existing counts as empty", raw: ptr("Bird"), existing: pgtype.Text{String: "  ", Valid: true}, wantValid: true, wantPersisted: true},
		{name: "trimmed value is stored", raw: ptr("  Bird  "), existing: pgtype.Text{}, wantValid: true, wantPersisted: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, shouldPersist, err := ResolveClientNickname(tt.raw, tt.existing)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveClientNickname(%v) expected an error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveClientNickname(%v) error: %v", tt.raw, err)
			}
			if shouldPersist != tt.wantPersisted {
				t.Fatalf("shouldPersist = %v, want %v", shouldPersist, tt.wantPersisted)
			}
			if got.Valid != tt.wantValid {
				t.Fatalf("resolved.Valid = %v, want %v", got.Valid, tt.wantValid)
			}
		})
	}
}

func TestResolveClientNicknameRejectsOverlongAndControlCharacters(t *testing.T) {
	ptr := func(value string) *string { return &value }
	overlong := make([]rune, 51)
	for i := range overlong {
		overlong[i] = 'a'
	}
	for name, raw := range map[string]*string{
		"overlong":        ptr(string(overlong)),
		"newline":         ptr("Bird\nSmith"),
		"tab":             ptr("Bird\tSmith"),
		"carriage return": ptr("Bird\rSmith"),
	} {
		if _, _, err := ResolveClientNickname(raw, pgtype.Text{}); err == nil {
			t.Fatalf("%s: ResolveClientNickname expected an error", name)
		}
	}
}
