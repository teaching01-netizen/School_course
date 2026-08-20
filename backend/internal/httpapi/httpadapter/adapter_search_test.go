package httpadapter

import (
	"strings"
	"testing"
)

func TestSearchQuery_TrimsAndCaps(t *testing.T) {
	a := New(nil, nil)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"whitespace only collapses to empty", "   \n\t ", ""},
		{"trims surrounding whitespace", "  alice  ", "alice"},
		{"keeps internal spaces", "john smith", "john smith"},
		{"short query unchanged", "w260010", "w260010"},
		{"long query capped at 200 runes", strings.Repeat("a", 500), strings.Repeat("a", 200)},
		{"long multi-byte query caps at 200 runes without splitting a rune", strings.Repeat("ศุภ", 150), string([]rune(strings.Repeat("ศุภ", 150))[:200])},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := a.SearchQuery(tc.in)
			if got != tc.want {
				t.Fatalf("SearchQuery(%q) = %q (len %d), want %q (len %d)", tc.in, got, len([]rune(got)), tc.want, len([]rune(tc.want)))
			}
		})
	}
}
