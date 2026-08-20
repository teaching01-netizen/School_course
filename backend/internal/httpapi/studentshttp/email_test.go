package studentshttp

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestResolvedStudentEmailPrefersCRMThenSystemThenLegacy(t *testing.T) {
	crm := pgtype.Text{String: "crm@example.com", Valid: true}
	system := pgtype.Text{String: "system@example.com", Valid: true}
	legacy := pgtype.Text{String: "legacy@example.com", Valid: true}

	if got := resolvedStudentEmail(legacy, crm, system); got != crm.String {
		t.Fatalf("resolved email = %q, want CRM email", got)
	}
	if got := resolvedStudentEmail(legacy, pgtype.Text{}, system); got != system.String {
		t.Fatalf("resolved email = %q, want system email", got)
	}
	if got := resolvedStudentEmail(legacy, pgtype.Text{}, pgtype.Text{}); got != legacy.String {
		t.Fatalf("resolved email = %q, want legacy email", got)
	}
}
