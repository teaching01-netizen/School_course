package normalize

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzNormalizeLegacyText(f *testing.F) {
	for _, seed := range []string{
		"Room 12",
		"\u00a0ห้องเรียน\u2003ภาษาไทย",
		"น้ำทำซื้อ",
		"[NOT SET]",
		string([]byte{'R', 'o', 'o', 'm', ' ', 0xff}),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		got := NormalizeText(input)
		if !utf8.ValidString(got) {
			t.Fatalf("NormalizeText returned invalid UTF-8: %q", got)
		}
		if again := NormalizeText(got); again != got {
			t.Fatalf("NormalizeText is not idempotent: first %q, second %q", got, again)
		}

		thai := "น้ำทำซื้อ"
		if strings.Contains(input, thai) && !strings.Contains(got, thai) {
			t.Fatalf("NormalizeText removed meaningful Thai text: input %q, output %q", input, got)
		}
	})
}
