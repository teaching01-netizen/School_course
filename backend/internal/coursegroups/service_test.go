package coursegroups

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestValidateCreate(t *testing.T) {
	validIDs := []pgtype.UUID{{Valid: true, Bytes: [16]byte{1}}, {Valid: true, Bytes: [16]byte{2}}}
	tests := []struct {
		name string
		cmd  CreateCommand
		code string
	}{
		{name: "blank name", cmd: CreateCommand{CourseIDs: validIDs}, code: "invalid_name"},
		{name: "missing course", cmd: CreateCommand{Name: "Verbal", CourseIDs: validIDs[:1]}, code: "invalid_course_ids"},
		{name: "duplicate course", cmd: CreateCommand{Name: "Verbal", CourseIDs: []pgtype.UUID{validIDs[0], validIDs[0]}}, code: "invalid_course_ids"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateCreate(test.cmd)
			if err == nil {
				t.Fatal("expected validation error")
			}
			domainErr, ok := err.(*Error)
			if !ok || domainErr.Code != test.code {
				t.Fatalf("expected error code %q, got %v", test.code, err)
			}
		})
	}
}

func TestValidateCreateAcceptsExactlyTwoCourses(t *testing.T) {
	err := ValidateCreate(CreateCommand{
		Name: "SAT Verbal",
		CourseIDs: []pgtype.UUID{
			{Valid: true, Bytes: [16]byte{1}},
			{Valid: true, Bytes: [16]byte{2}},
		},
	})
	if err != nil {
		t.Fatalf("expected valid command, got %v", err)
	}
}

func TestValidateNameTrimsAndRejectsBlankValues(t *testing.T) {
	if err := ValidateName("   "); err == nil {
		t.Fatal("expected blank name to be rejected")
	} else if domainErr, ok := err.(*Error); !ok || domainErr.Code != "invalid_name" {
		t.Fatalf("expected invalid_name, got %v", err)
	}

	if err := ValidateName(" Reading + Writing "); err != nil {
		t.Fatalf("expected trimmed non-blank name to be accepted, got %v", err)
	}
}
