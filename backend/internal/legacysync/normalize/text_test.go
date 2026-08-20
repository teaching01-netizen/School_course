package normalize

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"already clean", "Room 12", "Room 12"},
		{"nbsp", "a\u00a0b", "a b"},
		{"em space", "a\u2003b", "a b"},
		{"ideographic space", "a\u3000b", "a b"},
		{"whitespace runs", "\t multi \n line  ", "multi line"},
		{"leading and trailing", "  padded  ", "padded"},
		{"fullwidth", "ＡＢＣ１２３", "ABC123"},
		{"fullwidth mixed", "Ｍath １０１", "Math 101"},
		{"thai", "ห้องเรียน ภาษาไทย", "ห้องเรียน ภาษาไทย"},
		{"thai tone marks", "ภาษาไทย ก่", "ภาษาไทย ก่"},
		{"thai vowels and tone marks", "ซื้อ", "ซื้อ"},
		{"thai sara am", "น้ำ", "น้ำ"},
		{"thai sara am word", "ทำ", "ทำ"},
		{"not set literal preserved", "[NOT SET]", "[NOT SET]"},
		{"tab between words", "a\tb", "a b"},
		{"newline only", "\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeText(tt.in); got != tt.want {
				t.Errorf("NormalizeText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
func TestNormalizeText_ReplacesInvalidUTF8(t *testing.T) {
	input := string([]byte{'R', 'o', 'o', 'm', ' ', 0xff, 0xfe})
	got := NormalizeText(input)
	if !utf8.ValidString(got) {
		t.Fatalf("NormalizeText returned invalid UTF-8: %q", got)
	}
	if got != "Room \uFFFD" {
		t.Fatalf("NormalizeText(%q) = %q, want invalid bytes replaced", input, got)
	}
}

func TestNormalizeOptional(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"empty", "", "", false},
		{"not set upper", "[NOT SET]", "", false},
		{"not set lower padded", " [not set] ", "", false},
		{"not set mixed case", "[Not Set]", "", false},
		{"room", "  Room 12 ", "Room 12", true},
		{"single word", "room", "room", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeOptional(tt.in)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("NormalizeOptional(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestNormalizeID(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"padded keeps leading zeros", " 0078 ", "0078", false},
		{"plain", "7306", "7306", false},
		{"internal spaces preserved", "A 12", "A 12", false},
		{"empty", "", "", true},
		{"whitespace only", "  ", "", true},
		{"tab only", "\t\n", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeID(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NormalizeID(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeID(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func FuzzNormalizeText(f *testing.F) {
	for _, seed := range []string{
		"ห้องเรียน ภาษาไทย",
		"น้ำ",
		"a\u00a0b",
		"ＡＢＣ１２３",
		"[NOT SET]",
		"ก่",
		"ซื้อ",
		"\t multi \n line  ",
		"",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		once := NormalizeText(s)
		twice := NormalizeText(once)
		if once != twice {
			t.Fatalf("NormalizeText not idempotent for %q: %q != %q", s, once, twice)
		}
		if strings.Contains(once, "\u00a0") || strings.Contains(once, "\u3000") || strings.Contains(once, "\t") || strings.Contains(once, "\n") {
			t.Fatalf("NormalizeText(%q) = %q still contains unicode whitespace", s, once)
		}
	})
}
