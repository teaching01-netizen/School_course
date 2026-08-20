package reconcile

import "testing"

func TestSplitRosterEntry(t *testing.T) {
	tests := []struct {
		name     string
		entry    string
		wcode    string
		fullName string
		nickname string
		wantErr  bool
	}{
		{name: "plain name", entry: "W250025 Nutnicha Marungrueng", wcode: "W250025", fullName: "Nutnicha Marungrueng"},
		{name: "nickname suffix", entry: "W250025 Nutnicha Marungrueng (Nicha)", wcode: "W250025", fullName: "Nutnicha Marungrueng", nickname: "Nicha"},
		{name: "single-word name with nickname", entry: "W999901 Test Student (TS)", wcode: "W999901", fullName: "Test Student", nickname: "TS"},
		{name: "name with internal parens", entry: "W250025 John (Johnny) Doe", wcode: "W250025", fullName: "John (Johnny) Doe"},
		{name: "empty bracket marker", entry: "W250025 Some Name (-)", wcode: "W250025", fullName: "Some Name (-)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wcode, name, nickname, err := splitRosterEntry(tc.entry)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("splitRosterEntry(%q) succeeded, want error", tc.entry)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitRosterEntry(%q): %v", tc.entry, err)
			}
			if wcode != tc.wcode || name != tc.fullName || nickname != tc.nickname {
				t.Fatalf("splitRosterEntry(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.entry, wcode, name, nickname, tc.wcode, tc.fullName, tc.nickname)
			}
		})
	}
}

func TestSplitRosterEntry_Malformed(t *testing.T) {
	for _, entry := range []string{"", "W250025", "X250025 Some Name", "some name"} {
		if _, _, _, err := splitRosterEntry(entry); err == nil {
			t.Fatalf("splitRosterEntry(%q) succeeded, want error", entry)
		}
	}
}
