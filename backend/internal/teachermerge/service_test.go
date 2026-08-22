package teachermerge

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func account(id uuid.UUID, username, role string, deleted, legacy bool) Account {
	return Account{ID: id, Username: username, Role: role, Deleted: deleted, IsLegacy: legacy}
}

func TestValidate(t *testing.T) {
	dupID, canID := uuid.New(), uuid.New()
	shell := account(dupID, "legacy:8842", "Teacher", false, true)
	real := account(canID, "somchai.c", "Teacher", false, false)

	cases := []struct {
		name      string
		duplicate Account
		canonical Account
		want      error
	}{
		{"valid shell into real", shell, real, nil},
		{"same account", shell, shell, ErrSameAccount},
		{"duplicate not a teacher", account(dupID, "legacy:1", "Admin", false, true), real, ErrNotTeacher},
		{"canonical not a teacher", shell, account(canID, "root", "Admin", false, false), ErrNotTeacher},
		{"canonical deactivated", shell, account(canID, "somchai.c", "Teacher", true, false), ErrCanonicalInactive},
		{"canonical is legacy shell", shell, account(canID, "legacy:9", "Teacher", false, true), ErrCanonicalLegacy},
		{"duplicate already merged", account(dupID, "legacy:8842", "Teacher", true, true), real, ErrAlreadyMerged},
		{"non-legacy duplicate allowed", account(dupID, "somchai.old", "Teacher", false, false), real, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(tc.duplicate, tc.canonical)
			if !errors.Is(err, tc.want) {
				t.Fatalf("validate() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAccountLegacy(t *testing.T) {
	if !(Account{Username: "legacy:8842"}).legacy() {
		t.Fatal("legacy: prefix should mark an account as a sync shell")
	}
	if (Account{Username: "somchai.c"}).legacy() {
		t.Fatal("native account must not be treated as a sync shell")
	}
	// "legacyx:1" must not match: only applyTeacher's exact prefix counts.
	if (Account{Username: "legacyx:1"}).legacy() {
		t.Fatal("prefix match must be exact")
	}
}
